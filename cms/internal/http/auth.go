package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/app"
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
		s.authError(w, r, msg("Logowanie jest chwilowo niedostępne.", "Sign-in is temporarily unavailable."))
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
		s.authError(w, r, msg("Sesja logowania wygasła. Spróbuj ponownie.", "The sign-in session expired. Try again."))
		return
	}
	if q.Get("error") != "" {
		s.authError(w, r, msg("Logowanie zostało przerwane.", "Sign-in was cancelled."))
		return
	}
	id, err := s.IdP.Exchange(r.Context(), q.Get("code"), st.Verifier, s.BaseURL+"/auth/callback", st.Nonce)
	if err != nil {
		s.Log.Warn("oidc exchange", "err", err)
		s.authError(w, r, msg("Nie udało się potwierdzić tożsamości.", "Your identity could not be confirmed."))
		return
	}
	u, err := s.App.SignIn(r.Context(), id)
	if errors.Is(err, domain.ErrForbidden) {
		s.authError(w, r, msg("Konto "+id.Email+" nie ma dostępu do CMS. Poproś administratora o zaproszenie.",
			"The account "+id.Email+" has no access to the CMS. Ask the administrator for an invitation."))
		return
	}
	if err != nil {
		s.Log.Error("sign in", "err", err)
		s.authError(w, r, msg("Błąd logowania.", "Sign-in failed."))
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

// metaCallback finishes a Meta sign-in: it connects a Page, or completes a
// Post Request and posts.
func (s *Server) metaCallback(w http.ResponseWriter, r *http.Request) {
	var st metaState
	err := s.readCookie(r, metaCookie, "meta", &st)
	s.setCookie(w, metaCookie, "", -1)
	q := r.URL.Query()
	if err != nil || time.Now().Unix() > st.Expires || q.Get("state") != st.State || st.State == "" {
		s.authError(w, r, msg("Sesja logowania do Meta wygasła. Spróbuj ponownie.", "The Meta sign-in session expired. Try again."))
		return
	}
	p, err := s.principal(r)
	if err != nil || p.ID() != st.UserID {
		s.authError(w, r, msg("Zaloguj się do CMS i spróbuj ponownie.", "Sign in to the CMS and try again."))
		return
	}
	if st.Request != "" {
		s.completePost(w, r, p, st)
		return
	}
	back := "/app/orgs/" + url.PathEscape(st.Org) + "/social"
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

// completePost posts a Post Request and sends the Moderator back to the
// Post dialog with the outcome in the query: refused=<reason>, or
// <platform>=posted|failed|<reason> for each platform.
func (s *Server) completePost(w http.ResponseWriter, r *http.Request, p domain.Principal, st metaState) {
	back := "/app/orgs/" + url.PathEscape(st.Org) + "/events/" + url.PathEscape(st.Event) + "/post"
	if r.URL.Query().Get("error") != "" {
		s.App.CancelPost(r.Context(), p, st.Org, st.Request)
		http.Redirect(w, r, back+"?refused="+app.RefusedCancelled, http.StatusSeeOther)
		return
	}
	out, err := s.App.CompletePost(r.Context(), p, st.Org, st.Request, r.URL.Query().Get("code"), s.BaseURL+"/auth/meta/callback")
	if err != nil {
		s.Log.Error("complete post", "err", err)
		http.Redirect(w, r, back+"?refused="+app.RefusedError, http.StatusSeeOther)
		return
	}
	q := url.Values{}
	if out.Refused != "" {
		q.Set("refused", out.Refused)
	}
	for _, res := range out.Results {
		v := res.Status
		if res.Reason != "" {
			v = res.Reason
		}
		q.Set(string(res.Platform), v)
	}
	http.Redirect(w, r, back+"?"+q.Encode(), http.StatusSeeOther)
}

// startMetaLogin answers the URL of the Meta sign-in and binds it, in a
// signed cookie, to the user, the Organization and the Post Request if any.
func (s *Server) startMetaLogin(w http.ResponseWriter, p domain.Principal, st metaState) error {
	st.State, st.UserID, st.Expires = randToken(), p.ID(), time.Now().Add(10*time.Minute).Unix()
	u, err := s.App.SocialLoginURL(p, st.Org, st.State, s.BaseURL+"/auth/meta/callback")
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
<html lang="{{.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>{{.Title}}</title><link rel="stylesheet" href="/app/app.css"></head>
<body><main class="card narrow"><h1>{{.Title}}</h1>{{range .Paragraphs}}<p>{{.}}</p>{{end}}
{{if .Link}}<p><a class="button primary" href="{{.Link}}">{{.LinkText}}</a></p>{{end}}</main></body></html>`))

// page is a server-rendered page with bilingual texts, shown in the
// request's UI language.
type page struct {
	Title      domain.Text
	Paragraphs []domain.Text
	Link       string
	LinkText   domain.Text
}

func renderPage(w http.ResponseWriter, r *http.Request, status int, p page) {
	l := RequestLang(r)
	paras := make([]string, len(p.Paragraphs))
	for i, t := range p.Paragraphs {
		paras[i] = t.In(l)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Add("Vary", "Accept-Language, Cookie")
	w.WriteHeader(status)
	_ = pageTmpl.Execute(w, struct {
		Lang, Title, Link, LinkText string
		Paragraphs                  []string
	}{string(l), p.Title.In(l), p.Link, p.LinkText.In(l), paras})
}

func (s *Server) authError(w http.ResponseWriter, r *http.Request, m domain.Text) {
	renderPage(w, r, http.StatusForbidden, page{Title: msg("Logowanie", "Sign-in"), Paragraphs: []domain.Text{m},
		Link: "/auth/login", LinkText: msg("Zaloguj ponownie", "Sign in again")})
}

func (s *Server) privacy(w http.ResponseWriter, r *http.Request) {
	renderPage(w, r, 200, page{Title: msg("Polityka prywatności", "Privacy policy"), Paragraphs: []domain.Text{
		msg("folk-ms to system do zarządzania wydarzeniami organizacji folkowych. Przetwarzamy wyłącznie dane osób, które zarządzają wydarzeniami (moderatorów): adres e-mail, imię i identyfikator konta Google, oraz dziennik ich działań.",
			"folk-ms is a system for managing the events of folk organizations. We process only the data of the people who manage events (moderators): their email address, name and Google account identifier, and the log of their actions."),
		msg("Po połączeniu strony na Facebooku przechowujemy tylko jej nazwę i identyfikator oraz nazwę i identyfikator powiązanego konta Instagram. Moderator, który publikuje ogłoszenie wydarzenia, za każdym razem loguje się do Meta; token dostępu zwrócony przez Meta służy wyłącznie do opublikowania tego ogłoszenia i nie jest przechowywany. Nie przetwarzamy danych odwiedzających strony ani fanów.",
			"When a Facebook Page is connected we store only its name and identifier, and those of its linked Instagram account. A moderator who publishes an event announcement signs in with Meta every time; the access token Meta returns is used only to publish that announcement and is not stored. We do not process data of Page visitors or fans."),
		msg("Opcjonalny pomocnik Autofill (rozszerzenie przeglądarki lub skrypt użytkownika) przechowuje przekazane mu pola wydarzenia lokalnie w przeglądarce moderatora przez najwyżej 15 minut, a adres utworzonego wydarzenia na Facebooku do czasu przekazania go do CMS. Nie wysyła żadnych danych do stron trzecich ani nie kontaktuje się samodzielnie z CMS.",
			"The optional Autofill helper (a browser extension or userscript) keeps the event fields handed to it locally in the moderator's browser for at most 15 minutes, and the address of the created Facebook event until it is passed to the CMS. It sends no data to third parties and never contacts the CMS by itself."),
		msg("Aby usunąć swoje dane, skontaktuj się z administratorem organizacji lub skorzystaj z instrukcji na stronie /data-deletion.",
			"To delete your data, contact the organization's administrator or follow the instructions at /data-deletion."),
	}})
}

func (s *Server) dataDeletion(w http.ResponseWriter, r *http.Request) {
	renderPage(w, r, 200, page{Title: msg("Usuwanie danych", "Data deletion"), Paragraphs: []domain.Text{
		msg("CMS nie przechowuje tokenów dostępu Meta. Aby usunąć powiązanie organizacji ze stroną na Facebooku i kontem Instagram: zaloguj się do CMS, otwórz „Media społecznościowe” i kliknij „Rozłącz”. Możesz też usunąć aplikację w ustawieniach Facebooka (Ustawienia → Aplikacje i witryny).",
			"The CMS does not store Meta access tokens. To remove the link between an organization and its Facebook Page and Instagram account: sign in to the CMS, open “Social media” and click “Disconnect”. You can also remove the app in your Facebook settings (Settings → Apps and websites)."),
		msg("Aby usunąć swoje konto moderatora, poproś administratora CMS o odebranie dostępu; wpisy w dzienniku zmian są przechowywane jako zapis historii wydarzeń.",
			"To delete your moderator account, ask the CMS administrator to revoke your access; audit log entries are kept as a record of the events' history."),
	}})
}

// purgeInterval is the least time between two purges started via /purge.
// The Functions host cannot attach a secret to timer invocations, and the
// catch-all HTTP route makes /purge public; purging only removes overdue
// Bin items, so the only risk is load, which this bounds. Skipping cannot
// starve purging (every call that sets lastPurge runs a complete purge), but
// an item that becomes overdue just after a public call and before a skipped
// timer run waits for the next purge: at most a day past retention.
const purgeInterval = 10 * time.Minute

// purgeRetry is the wait before another attempt after a failed purge.
const purgeRetry = time.Minute

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
		// Retry a failed run after a minute rather than after purgeInterval,
		// while still bounding how often public calls can trigger a scan.
		s.purgeMu.Lock()
		s.lastPurge = time.Now().Add(-purgeInterval + purgeRetry)
		s.purgeMu.Unlock()
	}
	s.Log.Info("purge", "purged", n)
	// Custom handler invocation response format.
	writeJSON(w, http.StatusOK, map[string]any{"Outputs": map[string]any{}, "Logs": logs, "ReturnValue": nil})
}
