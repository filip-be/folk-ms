package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/adapters/img"
	"github.com/filip-be/folk-ms/cms/internal/adapters/memory"
	"github.com/filip-be/folk-ms/cms/internal/adapters/meta"
	"github.com/filip-be/folk-ms/cms/internal/app"
	"github.com/filip-be/folk-ms/cms/internal/domain"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// fakeIdP signs in whoever the test chooses.
type fakeIdP struct{ email string }

func (f *fakeIdP) AuthCodeURL(_ context.Context, state, nonce, challenge, redirect string) (string, error) {
	return redirect + "?" + url.Values{"code": {"c"}, "state": {state}}.Encode(), nil
}

func (f *fakeIdP) Exchange(context.Context, string, string, string, string) (ports.Identity, error) {
	return ports.Identity{Issuer: "test", Subject: f.email, Email: f.email, EmailVerified: true}, nil
}

type harness struct {
	ts     *httptest.Server
	idp    *fakeIdP
	app    *app.App
	social *meta.Recorder
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	o := &domain.Organization{ID: "org-a", Name: domain.Text{PL: "A"}, Timezone: "Europe/Warsaw",
		Site: domain.Site{ID: "org-a", BaseURL: "https://a.example", PermalinkTemplate: "/{lang}/events/{shortId}-{slug}"}}
	a := app.New([]*domain.Organization{o})
	rec := &meta.Recorder{}
	a.Docs, a.Pub, a.Audit, a.Social, a.Images = memory.NewDocumentStore(), memory.NewPublicStore("http://x/pub"), memory.NewAppendLog(), rec, img.Processor{}
	a.SuperAdminEmail = "admin@example.org"
	h := &harness{idp: &fakeIdP{email: "admin@example.org"}, app: a, social: rec}
	h.ts = httptest.NewUnstartedServer(nil)
	h.ts.Start()
	srv, err := NewServer(&Server{
		App: a, IdP: h.idp, BaseURL: h.ts.URL,
		Admin: fstest.MapFS{"index.html": {Data: []byte("<!doctype html>spa")}, "app.js": {Data: []byte("js")}},
	}, bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	h.ts.Config.Handler = srv.Handler()
	t.Cleanup(h.ts.Close)
	return h
}

func (h *harness) client(t *testing.T) *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

func (h *harness) signIn(t *testing.T, c *http.Client) {
	t.Helper()
	resp, err := c.Get(h.ts.URL + "/auth/login?return=/app/events")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.Request.URL.Path != "/app/events" {
		t.Fatalf("login landed on %s (%d)", resp.Request.URL, resp.StatusCode)
	}
}

func (h *harness) do(t *testing.T, c *http.Client, method, path, body string, hdr map[string]string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(method, h.ts.URL+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(b)
}

func TestAuthAndAPI(t *testing.T) {
	h := newHarness(t)
	c := h.client(t)
	if resp, _ := h.do(t, c, "GET", "/api/me", "", nil); resp.StatusCode != 401 {
		t.Fatalf("anonymous: %d", resp.StatusCode)
	}
	h.signIn(t, c)
	resp, body := h.do(t, c, "GET", "/api/me", "", nil)
	if resp.StatusCode != 200 || !strings.Contains(body, `"superAdmin":true`) {
		t.Fatalf("me: %d %s", resp.StatusCode, body)
	}
	if resp.Header.Get("Cache-Control") != "no-store" || !strings.Contains(resp.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatalf("headers: %v", resp.Header)
	}
	same := map[string]string{"Origin": h.ts.URL}
	resp, body = h.do(t, c, "POST", "/api/orgs/org-a/venues", `{"name":"Zamek","address":"Św. Marcin 80"}`, same)
	if resp.StatusCode != 200 {
		t.Fatalf("create venue: %d %s", resp.StatusCode, body)
	}
	resp, body = h.do(t, c, "POST", "/api/orgs/org-a/events", `{"title":{"pl":"Bal"},"schedule":{"kind":"single"}}`, same)
	if resp.StatusCode != 200 {
		t.Fatalf("create event: %d %s", resp.StatusCode, body)
	}
	var ev app.EventView
	_ = json.Unmarshal([]byte(body), &ev)
	resp, body = h.do(t, c, "POST", "/api/orgs/org-a/events/"+ev.Event.ID+"/publish", "", same)
	if resp.StatusCode != 422 || !strings.Contains(body, "schedule.startDate") {
		t.Fatalf("publish incomplete: %d %s", resp.StatusCode, body)
	}
	resp, _ = h.do(t, c, "PUT", "/api/orgs/org-a/events/"+ev.Event.ID, `{"title":{"pl":"Bal 2"}}`, map[string]string{"Origin": h.ts.URL, "If-Match": `"stale"`})
	if resp.StatusCode != 409 {
		t.Fatalf("stale put: %d", resp.StatusCode)
	}
	resp, _ = h.do(t, c, "POST", "/api/orgs/nope/events", `{}`, same)
	if resp.StatusCode != 404 {
		t.Fatalf("unknown org: %d", resp.StatusCode)
	}
	resp, _ = h.do(t, c, "POST", "/api/orgs/org-a/events", `{"bogus":1}`, same)
	if resp.StatusCode != 400 {
		t.Fatalf("unknown field: %d", resp.StatusCode)
	}

	// Logout clears the session.
	h.do(t, c, "POST", "/auth/logout", "", same)
	if resp, _ := h.do(t, c, "GET", "/api/me", "", nil); resp.StatusCode != 401 {
		t.Fatalf("after logout: %d", resp.StatusCode)
	}
}

func pngEncode(w io.Writer, width, height int) error {
	return png.Encode(w, image.NewRGBA(image.Rect(0, 0, width, height)))
}

func TestCSRF(t *testing.T) {
	h := newHarness(t)
	c := h.client(t)
	h.signIn(t, c)
	body := `{"name":"X","address":"Y"}`
	for name, hdr := range map[string]map[string]string{
		"cross-site fetch": {"Sec-Fetch-Site": "cross-site", "Origin": h.ts.URL},
		"same-site fetch":  {"Sec-Fetch-Site": "same-site"},
		"foreign origin":   {"Origin": "https://evil.example"},
		"no origin at all": {},
	} {
		if resp, _ := h.do(t, c, "POST", "/api/orgs/org-a/venues", body, hdr); resp.StatusCode != 403 {
			t.Errorf("%s: want 403, got %d", name, resp.StatusCode)
		}
	}
	if resp, _ := h.do(t, c, "POST", "/api/orgs/org-a/venues", body, map[string]string{"Sec-Fetch-Site": "same-origin"}); resp.StatusCode != 200 {
		t.Errorf("same-origin: %d", resp.StatusCode)
	}
}

func TestStrangerAndBadState(t *testing.T) {
	h := newHarness(t)
	h.idp.email = "stranger@example.org"
	c := h.client(t)
	req, _ := http.NewRequest("GET", h.ts.URL+"/auth/login", nil)
	req.Header.Set("Accept-Language", "pl-PL,pl;q=0.9,en;q=0.8")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 403 || !strings.Contains(string(b), "nie ma dostępu") || !strings.Contains(string(b), `lang="pl"`) {
		t.Fatalf("stranger: %d %s", resp.StatusCode, b)
	}
	// Callback without the state cookie.
	resp, _ = h.do(t, h.client(t), "GET", "/auth/callback?code=x&state=y", "", nil)
	if resp.StatusCode != 403 {
		t.Fatalf("forged callback: %d", resp.StatusCode)
	}
	// Open redirect is not possible.
	if safeReturn("//evil.example/app/") != "/app/" || safeReturn("https://evil") != "/app/" || safeReturn("/app/x") != "/app/x" {
		t.Fatal("safeReturn")
	}
}

func TestPostWithMetaSignIn(t *testing.T) {
	h := newHarness(t)
	c := h.client(t)
	h.signIn(t, c)
	ctx := context.Background()
	u, err := h.app.SignIn(ctx, ports.Identity{Issuer: "test", Subject: "admin@example.org", Email: "admin@example.org", EmailVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	p, err := h.app.Principal(ctx, u.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	v, err := h.app.CreateVenue(ctx, p, "org-a", domain.Venue{Name: "Zamek", Address: "Św. Marcin 80"})
	if err != nil {
		t.Fatal(err)
	}
	var png bytes.Buffer
	_ = pngEncode(&png, 1200, 600)
	cover, err := h.app.UploadImage(ctx, p, "org-a", ports.ImageCover, png.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	ev, err := h.app.CreateEvent(ctx, p, "org-a", domain.Content{Title: domain.Text{PL: "Bal"}, VenueID: v.ID, Cover: cover,
		Schedule: domain.Schedule{Kind: domain.KindSingle, StartDate: "2030-10-01", StartTime: "19:00", EndTime: "22:00"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.app.PublishEvent(ctx, p, "org-a", ev.Event.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.app.CompleteSocialLogin(ctx, p, "org-a", "code", h.ts.URL+"/auth/meta/callback"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.app.SelectPage(ctx, p, "org-a", "1001"); err != nil {
		t.Fatal(err)
	}
	same := map[string]string{"Origin": h.ts.URL}
	posts := "/api/orgs/org-a/events/" + ev.Event.ID + "/posts"
	start := func() string {
		t.Helper()
		resp, body := h.do(t, c, "POST", posts, `{"posts":[{"platform":"facebook","caption":"Hi"},{"platform":"instagram","caption":"IG"}],"confirm":true}`, same)
		var out struct{ URL string }
		if resp.StatusCode != 200 || json.Unmarshal([]byte(body), &out) != nil || out.URL == "" {
			t.Fatalf("request post: %d %s", resp.StatusCode, body)
		}
		return out.URL
	}

	// The Meta sign-in comes back to the callback, which posts on both
	// platforms and returns to the Post dialog.
	login := start()
	resp, _ := h.do(t, c, "GET", strings.TrimPrefix(login, h.ts.URL), "", nil)
	if got := resp.Request.URL; got.Path != "/app/orgs/org-a/events/"+ev.Event.ID+"/post" || got.Query().Get("facebook") != "posted" || got.Query().Get("instagram") != "posted" {
		t.Fatalf("landed on %s", got)
	}
	if len(h.social.Posts) != 2 {
		t.Fatalf("%+v", h.social.Posts)
	}
	// The state cookie is used once.
	if resp, _ := h.do(t, c, "GET", strings.TrimPrefix(login, h.ts.URL), "", nil); resp.StatusCode != 403 || len(h.social.Posts) != 2 {
		t.Fatalf("replayed callback: %d, %d posts", resp.StatusCode, len(h.social.Posts))
	}

	// Cancelling the Meta sign-in posts nothing and drops the request.
	login = start()
	lu, _ := url.Parse(login)
	resp, _ = h.do(t, c, "GET", "/auth/meta/callback?error=access_denied&state="+lu.Query().Get("state"), "", nil)
	if resp.Request.URL.Query().Get("refused") != app.RefusedCancelled || len(h.social.Posts) != 2 {
		t.Fatalf("cancelled: %s", resp.Request.URL)
	}
	if keys, _ := h.app.Docs.List(ctx, "orgs/org-a/social/requests/"); len(keys) != 0 {
		t.Fatalf("request left: %v", keys)
	}

	// Another browser, without the state cookie, cannot complete it.
	login = start()
	if resp, _ := h.do(t, h.client(t), "GET", strings.TrimPrefix(login, h.ts.URL), "", nil); resp.StatusCode != 403 || len(h.social.Posts) != 2 {
		t.Fatalf("foreign callback: %d", resp.StatusCode)
	}
}

func TestStaticAndTimer(t *testing.T) {
	h := newHarness(t)
	c := h.client(t)
	resp, body := h.do(t, c, "GET", "/app/orgs/org-a/events", "", nil)
	if resp.StatusCode != 200 || body != "<!doctype html>spa" {
		t.Fatalf("spa fallback: %d %s", resp.StatusCode, body)
	}
	resp, _ = h.do(t, c, "GET", "/app/missing.js", "", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("missing asset: %d", resp.StatusCode)
	}
	resp, body = h.do(t, c, "POST", "/purge", `{"Data":{},"Metadata":{}}`, nil)
	if resp.StatusCode != 200 || !strings.Contains(body, `"Outputs"`) {
		t.Fatalf("timer: %d %s", resp.StatusCode, body)
	}
	if _, body = h.do(t, c, "POST", "/purge", "{}", nil); !strings.Contains(body, "skipped") {
		t.Fatalf("a second purge right away must be skipped: %s", body)
	}
}

func TestSessionTampering(t *testing.T) {
	s := signer{key: bytes.Repeat([]byte{1}, 32)}
	v, _ := s.seal("session", sessionData{UserID: "u", IssuedAt: time.Now().Unix(), Expires: time.Now().Add(time.Hour).Unix()})
	var sd sessionData
	if err := s.open("session", v, &sd); err != nil || sd.UserID != "u" {
		t.Fatal(err)
	}
	if err := s.open("oidc", v, &sd); err == nil {
		t.Fatal("purpose must bind the cookie")
	}
	forged := base64.RawURLEncoding.EncodeToString([]byte(`{"u":"admin","iat":1,"exp":9999999999}`)) + v[strings.Index(v, "."):]
	if err := s.open("session", forged, &sd); err == nil {
		t.Fatal("forged payload accepted")
	}
}

func TestPageLanguage(t *testing.T) {
	h := newHarness(t)
	for _, tc := range []struct {
		name, accept, cookie, want string
	}{
		{"no preference", "", "", "Privacy policy"},
		{"browser prefers Polish", "pl-PL,pl;q=0.9,en;q=0.8", "", "Polityka prywatności"},
		{"English ranked above Polish", "de-DE,en;q=0.8,pl;q=0.5", "", "Privacy policy"},
		{"only unsupported languages", "de-DE,fr;q=0.8", "", "Privacy policy"},
		{"cookie overrides the browser", "pl-PL", "en", "Privacy policy"},
		{"cookie chooses Polish", "en-US", "pl", "Polityka prywatności"},
		{"unknown cookie is ignored", "pl", "xx", "Polityka prywatności"},
	} {
		req, _ := http.NewRequest("GET", h.ts.URL+"/privacy", nil)
		if tc.accept != "" {
			req.Header.Set("Accept-Language", tc.accept)
		}
		if tc.cookie != "" {
			req.AddCookie(&http.Cookie{Name: langCookie, Value: tc.cookie})
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !strings.Contains(string(b), "<h1>"+tc.want+"</h1>") {
			t.Errorf("%s: want %q in %s", tc.name, tc.want, b)
		}
	}
}

func TestErrorMessagesAreBilingual(t *testing.T) {
	h := newHarness(t)
	c := h.client(t)
	h.signIn(t, c)
	resp, body := h.do(t, c, "POST", "/api/orgs/org-a/venues", `{"name":""}`, map[string]string{"Sec-Fetch-Site": "same-origin"})
	if resp.StatusCode != 422 {
		t.Fatalf("want 422, got %d %s", resp.StatusCode, body)
	}
	var e struct {
		Message  domain.Text
		Problems []domain.FieldError
	}
	if err := json.Unmarshal([]byte(body), &e); err != nil {
		t.Fatal(err)
	}
	if e.Message.PL == "" || e.Message.EN == "" || len(e.Problems) == 0 {
		t.Fatalf("message %+v", e)
	}
	for _, p := range e.Problems {
		if p.Message.PL == "" || p.Message.EN == "" {
			t.Errorf("problem %s lacks a language: %+v", p.Field, p.Message)
		}
	}
}

// TestLangCookieMatchesAdmin keeps the cookie name shared with the admin SPA,
// which sets it.
func TestLangCookieMatchesAdmin(t *testing.T) {
	src, err := os.ReadFile("../../admin/src/i18n.ts")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `const COOKIE = "`+langCookie+`";`) {
		t.Fatalf("cms/admin/src/i18n.ts does not use the cookie %q", langCookie)
	}
}

// The Meta callback reports outcomes in the query of the Post dialog, which
// shows a message for each; RefusedError falls to its default message.
func TestPostOutcomesMatchAdmin(t *testing.T) {
	src, err := os.ReadFile("../../admin/src/views/post.ts")
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{
		app.RefusedCancelled, app.RefusedExpired, app.RefusedLogin, app.RefusedPage, app.RefusedInvalid,
		app.RefusedInstagram, app.RefusedQuota, app.RefusedConfirm, string(domain.PostPosted), string(domain.PostFailed),
	} {
		if !strings.Contains(string(src), `case "`+code+`":`) {
			t.Errorf("cms/admin/src/views/post.ts has no message for the outcome %q", code)
		}
	}
}
