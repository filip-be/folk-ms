package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/domain"
)

func randToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// safeReturn keeps post-login redirects inside the admin SPA.
func safeReturn(r string) string {
	if strings.HasPrefix(r, "/app/") && !strings.HasPrefix(r, "//") && !strings.ContainsAny(r, "\\\r\n") {
		return r
	}
	return "/app/"
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	st := oidcState{
		State: randToken(), Nonce: randToken(), Verifier: randToken(),
		Return: safeReturn(r.URL.Query().Get("return")), Expires: time.Now().Add(10 * time.Minute).Unix(),
	}
	sum := sha256.Sum256([]byte(st.Verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	u, err := s.IdP.AuthCodeURL(r.Context(), st.State, st.Nonce, challenge, s.BaseURL+"/auth/callback")
	if err != nil {
		s.Log.Error("auth url", "err", err)
		s.authError(w, "Logowanie jest chwilowo niedostępne.")
		return
	}
	v, err := s.sign.seal("oidc", st)
	if err != nil {
		writeError(w, s.Log, err)
		return
	}
	s.setCookie(w, oidcCookie, v, 10*time.Minute)
	http.Redirect(w, r, u, http.StatusFound)
}

func (s *Server) callback(w http.ResponseWriter, r *http.Request) {
	var st oidcState
	err := s.readCookie(r, oidcCookie, "oidc", &st)
	s.setCookie(w, oidcCookie, "", -1)
	q := r.URL.Query()
	if err != nil || time.Now().Unix() > st.Expires || q.Get("state") == "" || q.Get("state") != st.State {
		s.authError(w, "Sesja logowania wygasła. Spróbuj ponownie.")
		return
	}
	if q.Get("error") != "" {
		s.authError(w, "Logowanie zostało przerwane.")
		return
	}
	id, err := s.IdP.Exchange(r.Context(), q.Get("code"), st.Verifier, s.BaseURL+"/auth/callback", st.Nonce)
	if err != nil {
		s.Log.Warn("oidc exchange", "err", err)
		s.authError(w, "Nie udało się potwierdzić tożsamości.")
		return
	}
	u, err := s.App.SignIn(r.Context(), id)
	if errors.Is(err, domain.ErrForbidden) {
		s.authError(w, "Konto "+id.Email+" nie ma dostępu do CMS. Poproś administratora o zaproszenie.")
		return
	}
	if err != nil {
		s.Log.Error("sign in", "err", err)
		s.authError(w, "Błąd logowania.")
		return
	}
	now := time.Now()
	v, err := s.sign.seal("session", sessionData{UserID: u.ID, IssuedAt: now.Unix(), Expires: now.Add(SessionTTL).Unix()})
	if err != nil {
		writeError(w, s.Log, err)
		return
	}
	s.setCookie(w, sessionCookie, v, SessionTTL)
	http.Redirect(w, r, st.Return, http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	s.setCookie(w, sessionCookie, "", -1)
	w.WriteHeader(http.StatusNoContent)
}

// metaCallback finishes Facebook Login for a Social Account Connection.
func (s *Server) metaCallback(w http.ResponseWriter, r *http.Request) {
	var st metaState
	err := s.readCookie(r, metaCookie, "meta", &st)
	s.setCookie(w, metaCookie, "", -1)
	q := r.URL.Query()
	if err != nil || time.Now().Unix() > st.Expires || q.Get("state") != st.State || st.State == "" {
		s.authError(w, "Sesja łączenia z Facebookiem wygasła. Spróbuj ponownie.")
		return
	}
	p, err := s.principal(r)
	if err != nil || p.ID() != st.UserID {
		s.authError(w, "Zaloguj się do CMS i spróbuj ponownie.")
		return
	}
	back := "/app/orgs/" + st.Org + "/social"
	if q.Get("error") != "" {
		http.Redirect(w, r, back+"?error=cancelled", http.StatusSeeOther)
		return
	}
	if _, err := s.App.CompleteSocialLogin(r.Context(), p, st.Org, q.Get("code"), s.BaseURL+"/auth/meta/callback"); err != nil {
		s.Log.Warn("meta login", "err", err)
		http.Redirect(w, r, back+"?error=login", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, back+"?choose=1", http.StatusSeeOther)
}

func (s *Server) startMetaLogin(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
	org := r.PathValue("org")
	st := metaState{State: randToken(), Org: org, UserID: p.ID(), Expires: time.Now().Add(10 * time.Minute).Unix()}
	u, err := s.App.SocialLoginURL(p, org, st.State, s.BaseURL+"/auth/meta/callback")
	if err != nil {
		return err
	}
	v, err := s.sign.seal("meta", st)
	if err != nil {
		return err
	}
	s.setCookie(w, metaCookie, v, 10*time.Minute)
	writeJSON(w, http.StatusOK, map[string]string{"url": u})
	return nil
}

var pageTmpl = template.Must(template.New("p").Parse(`<!doctype html>
<html lang="pl"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>{{.Title}}</title><link rel="stylesheet" href="/app/app.css"></head>
<body><main class="card narrow"><h1>{{.Title}}</h1>{{range .Paragraphs}}<p>{{.}}</p>{{end}}
{{if .Link}}<p><a class="button primary" href="{{.Link}}">{{.LinkText}}</a></p>{{end}}</main></body></html>`))

type page struct {
	Title      string
	Paragraphs []string
	Link       string
	LinkText   string
}

func renderPage(w http.ResponseWriter, status int, p page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = pageTmpl.Execute(w, p)
}

func (s *Server) authError(w http.ResponseWriter, msg string) {
	renderPage(w, http.StatusForbidden, page{Title: "Logowanie", Paragraphs: []string{msg}, Link: "/auth/login", LinkText: "Zaloguj ponownie"})
}

func (s *Server) privacy(w http.ResponseWriter, _ *http.Request) {
	renderPage(w, 200, page{Title: "Polityka prywatności", Paragraphs: []string{
		"folk-ms to system do zarządzania wydarzeniami organizacji folkowych. Przetwarzamy wyłącznie dane osób, które zarządzają wydarzeniami (moderatorów): adres e-mail, imię i identyfikator konta Google, oraz dziennik ich działań.",
		"Po połączeniu strony na Facebooku przechowujemy zaszyfrowany token strony, służący wyłącznie do publikowania ogłoszeń wydarzeń na prośbę moderatora. Nie przetwarzamy danych odwiedzających strony ani fanów.",
		"Aby usunąć swoje dane, skontaktuj się z administratorem organizacji lub skorzystaj z instrukcji na stronie /data-deletion.",
	}})
}

func (s *Server) dataDeletion(w http.ResponseWriter, _ *http.Request) {
	renderPage(w, 200, page{Title: "Usuwanie danych", Paragraphs: []string{
		"Aby usunąć połączenie z Facebookiem i Instagramem: zaloguj się do CMS, otwórz „Media społecznościowe” i kliknij „Rozłącz”. Token strony zostanie natychmiast usunięty. Możesz też usunąć aplikację w ustawieniach Facebooka (Ustawienia → Aplikacje i witryny).",
		"Aby usunąć swoje konto moderatora, poproś administratora CMS o odebranie dostępu; wpisy w dzienniku zmian są przechowywane jako zapis historii wydarzeń.",
	}})
}

// purgeInterval is the least time between two purges started via /purge.
// The Functions host cannot attach a secret to timer invocations, and the
// catch-all HTTP route makes /purge public; purging only removes overdue
// Bin items, so the only risk is load, which this bounds. A skipped call can
// never starve purging: every call that sets lastPurge runs a complete purge
// itself, so nothing overdue survives longer than purgeInterval after one.
const purgeInterval = 10 * time.Minute

func (s *Server) timerPurge(w http.ResponseWriter, r *http.Request) {
	logs := []string{}
	s.purgeMu.Lock()
	skip := time.Since(s.lastPurge) < purgeInterval
	if !skip {
		s.lastPurge = time.Now()
	}
	s.purgeMu.Unlock()
	if skip {
		writeJSON(w, http.StatusOK, map[string]any{"Outputs": map[string]any{}, "Logs": []string{"purge skipped: ran recently"}, "ReturnValue": nil})
		return
	}
	// Detached from the caller: a client hanging up must not cut a purge short.
	n, err := s.App.PurgeBin(context.WithoutCancel(r.Context()))
	if err != nil {
		s.Log.Error("purge", "err", err)
		logs = append(logs, "purge error: "+err.Error())
		// A failed run must not suppress the next attempt.
		s.purgeMu.Lock()
		s.lastPurge = time.Time{}
		s.purgeMu.Unlock()
	}
	s.Log.Info("purge", "purged", n)
	// Custom handler invocation response format.
	writeJSON(w, http.StatusOK, map[string]any{"Outputs": map[string]any{}, "Logs": logs, "ReturnValue": nil})
}
