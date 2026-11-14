// Package httpapi is the CMS's HTTP layer: the JSON API under /api, sign-in
// under /auth, the embedded admin SPA under /app, and the timer endpoint.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/app"
	"github.com/filip-be/folk-ms/cms/internal/domain"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// Server serves the CMS.
type Server struct {
	App *app.App
	IdP ports.IdentityProvider // Google
	// Facebook signs in with Facebook Login (nil: not offered).
	Facebook ports.IdentityProvider
	// BaseURL is the CMS origin, e.g. https://cms.example (no trailing slash).
	BaseURL string
	// Secure marks cookies Secure with the __Host- prefix and sends HSTS.
	Secure bool
	// TrustProxy takes the client IP from the last X-Forwarded-For hop
	// (the Functions front end appends the real client).
	TrustProxy bool
	Admin      fs.FS        // the built admin SPA
	PublicDir  string       // serve the public store at /pub/ (local only)
	DevLogin   http.Handler // dev OIDC login page (dev builds only)
	// PublicOrigin is the origin images are served from, allowed by the CSP.
	PublicOrigin string
	Log          *slog.Logger

	sign    signer
	limiter *limiter
	// writes bounds changes per user: a stolen session cannot flood the
	// store, the Audit Log or the Graph quota.
	writes *limiter
	// uploads holds the one image upload slot per instance.
	uploads chan struct{}
	static  *staticFS

	purgeMu   sync.Mutex
	lastPurge time.Time
	syncMu    sync.Mutex
	lastSync  time.Time
}

// NewServer validates the configuration.
func NewServer(s *Server, sessionKey []byte) (*Server, error) {
	if len(sessionKey) < 32 {
		return nil, errors.New("httpapi: session key must be at least 32 bytes")
	}
	if _, err := url.Parse(s.BaseURL); err != nil || s.BaseURL == "" {
		return nil, errors.New("httpapi: invalid base URL")
	}
	s.BaseURL = strings.TrimRight(s.BaseURL, "/")
	s.sign = signer{key: sessionKey}
	s.limiter = newLimiter(20, time.Minute)
	s.writes = newLimiter(MaxWritesPerMinute, time.Minute)
	s.uploads = make(chan struct{}, 1)
	if s.Log == nil {
		s.Log = slog.Default()
	}
	st, err := newStaticFS(s.Admin)
	if err != nil {
		return nil, err
	}
	s.static = st
	return s, nil
}

// Handler returns the root handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/app/", http.StatusFound) })
	mux.HandleFunc("GET /app", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/app/", http.StatusMovedPermanently)
	})
	mux.Handle("GET /app/", s.static)
	mux.HandleFunc("GET /privacy", s.privacy)
	mux.HandleFunc("GET /data-deletion", s.dataDeletion)

	auth := http.NewServeMux()
	auth.HandleFunc("GET /auth/login", s.login)
	auth.HandleFunc("GET /auth/callback", s.callback)
	auth.HandleFunc("POST /auth/logout", s.logout)
	auth.HandleFunc("GET /auth/meta/callback", s.metaCallback)
	if s.DevLogin != nil {
		auth.Handle("/auth/dev/login", s.DevLogin)
	}
	mux.Handle("/auth/", s.rateLimit(auth))

	// The Functions host invokes the timer function "purge" as POST /purge.
	// Purging only removes what is overdue, so this is safe to expose.
	mux.Handle("POST /purge", s.timerLimit(http.HandlerFunc(s.timerPurge)))
	// The timer function "sync" (hourly) is POST /sync: Autosync for the
	// Organizations that turned it on. Public like /purge; it takes no input,
	// and the per-Organization lease and interval in storage bound how often
	// Graph is called (T10).
	mux.Handle("POST /sync", s.timerLimit(http.HandlerFunc(s.timerSync)))

	if s.PublicDir != "" {
		mux.Handle("GET /pub/", s.publicFiles())
	}

	api := http.NewServeMux()
	s.routes(api)
	mux.Handle("/api/", s.requireSession(api))

	return s.recoverer(s.securityHeaders(s.csrf(mux)))
}

// apiHandler is an API handler with the resolved principal.
type apiHandler func(w http.ResponseWriter, r *http.Request, p domain.Principal) error

type principalKey struct{}

func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := s.principal(r)
		if err != nil {
			if !errors.Is(err, domain.ErrUnauthorized) {
				s.Log.Error("session", "err", err)
			}
			writeError(w, s.Log, domain.ErrUnauthorized)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !s.writes.allow(p.ID()) {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, http.StatusTooManyRequests, errorBody{Error: "rate_limited", Message: msg("zbyt wiele zmian, spróbuj za minutę", "too many changes, try again in a minute")})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	})
}

// MaxWritesPerMinute is how many changing API requests one user may send a
// minute, per instance; far above what a person clicks.
const MaxWritesPerMinute = 60

func (s *Server) principal(r *http.Request) (domain.Principal, error) {
	var sd sessionData
	if err := s.readCookie(r, sessionCookie, "session", &sd); err != nil {
		return domain.Principal{}, domain.ErrUnauthorized
	}
	if time.Now().Unix() > sd.Expires {
		return domain.Principal{}, domain.ErrUnauthorized
	}
	return s.App.Principal(r.Context(), sd.UserID, time.Unix(sd.IssuedAt, 0), sd.Nonce)
}

// pathParams are the wildcards of the API routes.
var pathParams = []string{"org", "id", "day", "action"}

// safeSegment reports whether a path value is a plain name: ids,
// Organization ids, dates and actions never need more.
func safeSegment(v string) bool {
	if v == "" || len(v) > 64 {
		return false
	}
	for _, c := range v {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func (s *Server) h(fn apiHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The mux unescapes wildcards, so "..%2F..%2Fx" arrives as "../../x":
		// refuse anything but a plain name before it reaches a storage key
		// (T13; the use cases and the store adapters check again).
		for _, n := range pathParams {
			if v := r.PathValue(n); v != "" && !safeSegment(v) {
				writeError(w, s.Log, domain.ErrNotFound)
				return
			}
		}
		p, _ := r.Context().Value(principalKey{}).(domain.Principal)
		if err := fn(w, r, p); err != nil {
			writeError(w, s.Log, err)
		}
	})
}

// csrf rejects cross-site unsafe requests: Sec-Fetch-Site must be
// same-origin when present, otherwise Origin must be the CMS origin.
func (s *Server) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/purge" || r.URL.Path == "/sync" || r.URL.Path == "/auth/dev/login" && s.DevLogin != nil {
			// The timer call comes from the Functions host, not a browser;
			// the dev login form is local only.
			next.ServeHTTP(w, r)
			return
		}
		if sfs := r.Header.Get("Sec-Fetch-Site"); sfs != "" {
			if sfs != "same-origin" {
				writeJSON(w, http.StatusForbidden, errorBody{Error: "csrf", Message: msg("żądanie z innej strony zostało odrzucone", "cross-site request rejected")})
				return
			}
		} else if o := r.Header.Get("Origin"); o != s.BaseURL {
			writeJSON(w, http.StatusForbidden, errorBody{Error: "csrf", Message: msg("żądanie z innej strony zostało odrzucone", "cross-site request rejected")})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	// Video thumbnails come from YouTube and Vimeo (images only).
	img := "'self' data: blob: https://i.ytimg.com https://i.vimeocdn.com"
	if s.PublicOrigin != "" {
		img += " " + s.PublicOrigin
	}
	csp := "default-src 'self'; img-src " + img + "; connect-src 'self'; script-src 'self'; style-src 'self'; " +
		"object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		if !strings.HasPrefix(r.URL.Path, "/pub/") {
			h.Set("Content-Security-Policy", csp)
			h.Set("X-Frame-Options", "DENY")
		}
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if s.Secure {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/auth/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.Log.Error("panic", "path", r.URL.Path, "panic", v)
				writeJSON(w, 500, errorBody{Error: "internal", Message: msg("błąd serwera", "server error")})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) clientIP(r *http.Request) string {
	if s.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			ip := strings.TrimSpace(parts[len(parts)-1])
			if host, _, err := net.SplitHostPort(ip); err == nil {
				ip = host
			}
			// One bucket per address: a malformed hop or an IPv6 address
			// written without brackets must not give each source port its own.
			if a, err := netip.ParseAddr(strings.Trim(ip, "[]")); err == nil {
				a = a.Unmap()
				if a.Is6() {
					// A /64 is one subscriber, and an unbracketed
					// "address:port" differs only in its last bits.
					p, _ := a.Prefix(64)
					return p.String()
				}
				return a.String()
			}
			// An unbracketed IPv6 "address:port": drop the port and try again.
			if i := strings.LastIndex(ip, ":"); i > 0 {
				if a, err := netip.ParseAddr(ip[:i]); err == nil && a.Is6() {
					p, _ := a.Prefix(64)
					return p.String()
				}
			}
			return "unparsed-forwarded-for"
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// timerLimit rate-limits the public timer endpoints per client, except the
// Functions host's own timer call, which comes from loopback with no
// X-Forwarded-For: public callers cannot use up its budget. Their own
// intervals bound the work either way.
func (s *Server) timerLimit(next http.Handler) http.Handler {
	limited := s.rateLimit(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.TrustProxy && r.Header.Get("X-Forwarded-For") == "" {
			if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
				if a, err := netip.ParseAddr(host); err == nil && a.IsLoopback() {
					next.ServeHTTP(w, r)
					return
				}
			}
		}
		limited.ServeHTTP(w, r)
	})
}

func (s *Server) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.limiter.allow(s.clientIP(r)) {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, http.StatusTooManyRequests, errorBody{Error: "rate_limited", Message: msg("zbyt wiele prób, spróbuj za minutę", "too many attempts, try again in a minute")})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// limiter is a fixed-window counter per key.
type limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string]*window
}

type window struct {
	start time.Time
	n     int
}

func newLimiter(max int, w time.Duration) *limiter {
	return &limiter{max: max, window: w, hits: map[string]*window{}}
}

func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.hits) > 10000 {
		for k, w := range l.hits {
			if now.Sub(w.start) > l.window {
				delete(l.hits, k)
			}
		}
	}
	w, ok := l.hits[key]
	if !ok || now.Sub(w.start) > l.window {
		l.hits[key] = &window{start: now, n: 1}
		return true
	}
	w.n++
	return w.n <= l.max
}

type errorBody struct {
	Error    string              `json:"error"`
	Message  domain.Text         `json:"message"`
	Problems []domain.FieldError `json:"problems,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, log *slog.Logger, err error) {
	var ve *domain.ValidationError
	var mbe *http.MaxBytesError
	var bre *badRequest
	switch {
	case errors.As(err, &ve):
		writeJSON(w, http.StatusUnprocessableEntity, errorBody{Error: "validation", Message: msg("popraw zaznaczone pola", "correct the marked fields"), Problems: ve.Problems})
	case errors.As(err, &mbe):
		writeJSON(w, http.StatusRequestEntityTooLarge, errorBody{Error: "too_large", Message: msg("przesłane dane są za duże", "the uploaded data is too large")})
	case errors.Is(err, domain.ErrUnauthorized):
		writeJSON(w, http.StatusUnauthorized, errorBody{Error: "unauthorized", Message: msg("zaloguj się", "sign in")})
	case errors.Is(err, domain.ErrForbidden):
		writeJSON(w, http.StatusForbidden, errorBody{Error: "forbidden", Message: msg("brak dostępu", "access denied")})
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorBody{Error: "not_found", Message: msg("nie znaleziono", "not found")})
	case errors.Is(err, domain.ErrNeedsConfirm):
		writeJSON(w, http.StatusConflict, errorBody{Error: "needs_confirmation", Message: msg("potwierdź, aby kontynuować", "confirm to continue")})
	case errors.Is(err, domain.ErrRecentSignIn):
		writeJSON(w, http.StatusForbidden, errorBody{Error: "recent_sign_in", Message: msg("zaloguj się ponownie, aby potwierdzić tę operację", "sign in again to confirm this action")})
	case errors.Is(err, domain.ErrTooSoon):
		writeJSON(w, http.StatusTooManyRequests, errorBody{Error: "too_soon", Message: msg("synchronizacja była przed chwilą – spróbuj za minutę", "a sync ran moments ago – try again in a minute")})
	case errors.Is(err, domain.ErrConflict):
		writeJSON(w, http.StatusConflict, errorBody{Error: "conflict", Message: msg("ktoś inny zmienił te dane – odśwież i spróbuj ponownie", "someone else changed this data – refresh and try again")})
	case errors.Is(err, domain.ErrInvalidState):
		writeJSON(w, http.StatusConflict, errorBody{Error: "invalid_state", Message: msg("ta operacja nie jest teraz możliwa", "this action is not possible now")})
	case errors.As(err, &bre):
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "bad_request", Message: bre.msg})
	default:
		log.Error("request failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, errorBody{Error: "internal", Message: msg("błąd serwera", "server error")})
	}
}

// badRequest is a malformed request; msg is shown to the user.
type badRequest struct {
	msg   domain.Text
	cause error
}

func (e *badRequest) Error() string {
	if e.cause != nil {
		return e.msg.EN + ": " + e.cause.Error()
	}
	return e.msg.EN
}

func (e *badRequest) Unwrap() error { return e.cause }
