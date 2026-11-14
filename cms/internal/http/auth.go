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
	"github.com/filip-be/folk-ms/cms/internal/ports"
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

// Sign-in providers, as named in /auth/login?provider=.
const (
	providerGoogle   = "google"
	providerFacebook = "facebook"
)

// idp returns the identity provider for a name, or nil.
func (s *Server) idp(name string) ports.IdentityProvider {
	switch name {
	case providerGoogle:
		return s.IdP
	case providerFacebook:
		return s.Facebook
	}
	return nil
}

// loginProvider is the provider asked for, else the one used last, else
// Google.
func (s *Server) loginProvider(r *http.Request) string {
	if p := r.URL.Query().Get("provider"); p != "" {
		return p
	}
	if c, err := r.Cookie(s.cookieName(idpCookie)); err == nil && s.idp(c.Value) != nil {
		return c.Value
	}
	return providerGoogle
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	// Another site must not start a sign-in silently: with Facebook an
	// authorized account comes back with no interaction, which would renew
	// a Recent sign-in or push out the user's other sessions. A link from
	// elsewhere gets a page whose button starts it from this origin.
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		renderPage(w, r, http.StatusOK, page{Title: msg("Logowanie", "Sign-in"),
			Paragraphs: []domain.Text{msg("Zaloguj się do CMS.", "Sign in to the CMS.")},
			Link:       "/auth/login?" + r.URL.RawQuery, LinkText: msg("Zaloguj się", "Sign in")})
		return
	}
	provider := s.loginProvider(r)
	idp := s.idp(provider)
	if idp == nil {
		s.authError(w, r, msg("Ten sposób logowania jest niedostępny.", "This sign-in method is not available."))
		return
	}
	st := oidcState{
		Provider: provider, State: randToken(), Nonce: randToken(), Verifier: randToken(),
		Return: safeReturn(r.URL.Query().Get("return")), Expires: time.Now().Add(10 * time.Minute).Unix(),
	}
	sum := sha256.Sum256([]byte(st.Verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	u, err := idp.AuthCodeURL(r.Context(), st.State, st.Nonce, challenge, s.BaseURL+"/auth/callback")
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
	idp := s.idp(st.Provider)
	if err != nil || idp == nil || time.Now().Unix() > st.Expires || q.Get("state") == "" || q.Get("state") != st.State {
		s.authError(w, r, msg("Sesja logowania wygasła. Spróbuj ponownie.", "The sign-in session expired. Try again."))
		return
	}
	if q.Get("error") != "" {
		s.authError(w, r, msg("Logowanie zostało przerwane.", "Sign-in was cancelled."))
		return
	}
	id, err := idp.Exchange(r.Context(), q.Get("code"), st.Verifier, s.BaseURL+"/auth/callback", st.Nonce)
	if err != nil {
		s.Log.Warn("oidc exchange", "err", err)
		s.authError(w, r, msg("Nie udało się potwierdzić tożsamości.", "Your identity could not be confirmed."))
		return
	}
	nonce := randToken()
	signedInAs := ""
	if cur, err := s.principal(r); err == nil {
		signedInAs = cur.ID()
	}
	u, err := s.App.SignIn(r.Context(), id, nonce, signedInAs)
	if errors.Is(err, app.ErrLinkRequired) {
		s.authError(w, r, msg("To konto Facebook nie jest jeszcze połączone z Twoim kontem w CMS. Zaloguj się kontem, którego używasz dotąd, a potem – w tej samej przeglądarce – zaloguj się przez Facebooka, aby je połączyć.",
			"This Facebook account is not linked to your CMS account yet. Sign in with the account you have used so far, then, in the same browser, sign in with Facebook to link it."))
		return
	}
	if errors.Is(err, app.ErrSuperAdminIdentity) {
		s.authError(w, r, msg("Administrator loguje się wyłącznie kontem Google, z którym jest powiązany.",
			"The administrator signs in only with the Google account they are bound to."))
		return
	}
	if errors.Is(err, domain.ErrForbidden) {
		if id.Email == "" {
			s.authError(w, r, msg("Konto nie udostępniło adresu e-mail, więc nie możemy sprawdzić dostępu. Zaloguj się ponownie i zezwól na dostęp do adresu e-mail.",
				"The account did not share an email address, so access cannot be checked. Sign in again and allow access to your email address."))
			return
		}
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
	v, err := s.sign.seal("session", sessionData{UserID: u.ID, IssuedAt: now.Unix(), Expires: now.Add(SessionTTL).Unix(), Nonce: nonce})
	if err != nil {
		writeError(w, s.Log, err)
		return
	}
	s.setCookie(w, sessionCookie, v, SessionTTL)
	s.setCookie(w, idpCookie, st.Provider, 365*24*time.Hour)
	http.Redirect(w, r, st.Return, http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	var sd sessionData
	if err := s.readCookie(r, sessionCookie, "session", &sd); err == nil {
		if err := s.App.SignOut(r.Context(), sd.UserID, sd.Nonce); err != nil {
			s.Log.Error("sign out", "err", err)
		}
	}
	s.setCookie(w, sessionCookie, "", -1)
	w.WriteHeader(http.StatusNoContent)
}

// metaCallback finishes a Meta sign-in: it connects a Facebook page, or completes a
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
	switch {
	case st.Request != "":
		s.completePost(w, r, p, st)
		return
	case st.Sync:
		s.completeSync(w, r, p, st)
		return
	case st.Autosync:
		s.completeAutosync(w, r, p, st)
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

// completeSync runs "Sync now" with the Meta sign-in's token and sends the
// Moderator back to the Event list: sync=ok, or sync=<reason>.
func (s *Server) completeSync(w http.ResponseWriter, r *http.Request, p domain.Principal, st metaState) {
	back := "/app/orgs/" + url.PathEscape(st.Org) + "/events"
	if r.URL.Query().Get("error") != "" {
		http.Redirect(w, r, back+"?sync=cancelled", http.StatusSeeOther)
		return
	}
	_, err := s.App.CompleteSyncLogin(r.Context(), p, st.Org, r.URL.Query().Get("code"), s.BaseURL+"/auth/meta/callback")
	reason := "ok"
	switch {
	case err == nil:
	case errors.Is(err, ports.ErrNoPageAccess):
		reason = "page"
	case errors.Is(err, domain.ErrTooSoon):
		reason = "too_soon"
	case errors.Is(err, domain.ErrConflict):
		reason = "running"
	default:
		s.Log.Warn("sync now", "err", err)
		reason = "error"
	}
	http.Redirect(w, r, back+"?sync="+reason, http.StatusSeeOther)
}

// completeAutosync turns Autosync on with the Meta sign-in and sends the
// Moderator back to the connection screen: autosync=on, or autosync=<reason>.
func (s *Server) completeAutosync(w http.ResponseWriter, r *http.Request, p domain.Principal, st metaState) {
	back := "/app/orgs/" + url.PathEscape(st.Org) + "/social"
	if r.URL.Query().Get("error") != "" {
		http.Redirect(w, r, back+"?autosync=cancelled", http.StatusSeeOther)
		return
	}
	err := s.App.CompleteAutosync(r.Context(), p, st.Org, r.URL.Query().Get("code"), s.BaseURL+"/auth/meta/callback")
	reason := "on"
	switch {
	case err == nil:
	case errors.Is(err, ports.ErrNoPageAccess):
		reason = "page"
	case errors.Is(err, domain.ErrRecentSignIn):
		reason = "recent_sign_in"
	default:
		s.Log.Warn("autosync", "err", err)
		reason = "error"
	}
	http.Redirect(w, r, back+"?autosync="+reason, http.StatusSeeOther)
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
		msg("folk-ms to system do zarządzania dynamiczną treścią stron internetowych. Przetwarzamy wyłącznie dane osób, które zarządzają wydarzeniami (moderatorów): adres e-mail, imię i identyfikator konta Google lub Facebook, którym się logują, czas ich logowań oraz dziennik ich działań.",
			"folk-ms is a system for dynamic content management for websites. We process only the data of the people who manage events (moderators): their email address, name and the identifier of the Google or Facebook account they sign in with, the times of their sign-ins, and the log of their actions."),
		msg("Wydarzenia pobieramy ze strony organizacji na Facebooku: nazwę, opis, daty, miejsce, zdjęcie w tle i link do biletów wydarzeń utworzonych przez tę stronę. Pokazujemy je na stronie internetowej organizacji. Nie pobieramy danych uczestników, odwiedzających ani fanów.",
			"We read events from the organization's Facebook page: the name, description, dates, place, cover photo and ticket link of events the page created. We show them on the organization's website. We do not read data of attendees, visitors or fans."),
		msg("Po połączeniu strony na Facebooku przechowujemy jej nazwę i identyfikator oraz nazwę i identyfikator powiązanego konta Instagram. Jeśli organizacja nie włączyła automatycznej synchronizacji, nie przechowujemy żadnego tokenu Meta: moderator loguje się do Meta przy każdej synchronizacji i przy każdym ogłoszeniu, a zwrócony token służy tylko do tej operacji.",
			"When a Facebook page is connected we store its name and identifier, and those of its linked Instagram account. Unless the organization turned automatic sync on, we store no Meta token: a moderator signs in with Meta for every sync and every announcement, and the token returned is used only for that action."),
		msg("Jeśli moderator włączy automatyczną synchronizację, przechowujemy token dostępu strony na Facebooku w Azure Key Vault, dopóki synchronizacja jest włączona. Służy on do cogodzinnego pobierania wydarzeń strony i do publikowania ogłoszeń, które moderator zatwierdzi. Wyłączenie synchronizacji, zmiana lub odłączenie strony albo odebranie dostępu osobie, która ją włączyła, usuwa token.",
			"If a moderator turns automatic sync on, we keep the Facebook page's access token in Azure Key Vault for as long as sync stays on. It is used to read the page's events every hour and to publish announcements a moderator approves. Turning sync off, changing or disconnecting the page, or removing the access of the person who turned it on deletes the token."),
		msg("Aby usunąć swoje dane, skontaktuj się z administratorem organizacji lub skorzystaj z instrukcji na stronie /data-deletion.",
			"To delete your data, contact the organization's administrator or follow the instructions at /data-deletion."),
	}})
}

func (s *Server) dataDeletion(w http.ResponseWriter, r *http.Request) {
	renderPage(w, r, 200, page{Title: msg("Usuwanie danych", "Data deletion"), Paragraphs: []domain.Text{
		msg("Aby usunąć przechowywany token strony na Facebooku: zaloguj się do CMS, otwórz „Social media” i wyłącz automatyczną synchronizację lub kliknij „Rozłącz”. Token jest wtedy usuwany z Azure Key Vault. Token strony nie może sam się unieważnić, więc osoba, która włączyła synchronizację, powinna także usunąć aplikację w ustawieniach Facebooka (Ustawienia → Integracje biznesowe).",
			"To delete the stored Facebook page token: sign in to the CMS, open “Social media” and turn automatic sync off, or click “Disconnect”. The token is then deleted from Azure Key Vault. A page token cannot revoke itself, so the person who turned sync on should also remove the app in their Facebook settings (Settings → Business integrations)."),
		msg("Odłączenie strony usuwa też powiązanie organizacji ze stroną na Facebooku i kontem Instagram. Wydarzenia pobrane wcześniej zostają na stronie internetowej, dopóki moderator ich nie ukryje.",
			"Disconnecting also removes the link between the organization and its Facebook page and Instagram account. Events read before stay on the website until a moderator hides them."),
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
		// /purge is public: the details go to the log only.
		logs = append(logs, "purge failed")
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

// syncTimerInterval bounds how often /sync scans the Organizations from
// one instance; the per-Organization interval in storage bounds Graph calls
// across instances.
const syncTimerInterval = 5 * time.Minute

func (s *Server) timerSync(w http.ResponseWriter, r *http.Request) {
	logs := []string{}
	s.syncMu.Lock()
	skip := time.Since(s.lastSync) < syncTimerInterval
	if !skip {
		s.lastSync = time.Now()
	}
	s.syncMu.Unlock()
	if skip {
		writeJSON(w, http.StatusOK, map[string]any{"Outputs": map[string]any{}, "Logs": []string{"sync skipped: ran recently"}, "ReturnValue": nil})
		return
	}
	n, err := s.App.AutosyncAll(context.WithoutCancel(r.Context()))
	if err != nil {
		s.Log.Error("autosync", "err", err)
		// /sync is public: the details (Organization ids, secret names,
		// Graph's answers) go to the log only.
		logs = append(logs, "autosync failed")
	}
	s.Log.Info("autosync", "synced", n)
	writeJSON(w, http.StatusOK, map[string]any{"Outputs": map[string]any{}, "Logs": logs, "ReturnValue": nil})
}
