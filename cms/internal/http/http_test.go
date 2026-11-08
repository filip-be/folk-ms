package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
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

	"github.com/filip-be/folk-ms/cms/internal/adapters/aesgcm"
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
	ts  *httptest.Server
	idp *fakeIdP
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	o := &domain.Organization{ID: "org-a", Name: domain.Text{PL: "A"}, Timezone: "Europe/Warsaw",
		Site: domain.Site{ID: "org-a", BaseURL: "https://a.example", PermalinkTemplate: "/{lang}/events/{shortId}-{slug}"}}
	a := app.New([]*domain.Organization{o})
	c, _ := aesgcm.New(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)))
	a.Docs, a.Pub, a.Audit, a.Social, a.Cipher, a.Images = memory.NewDocumentStore(), memory.NewPublicStore("http://x/pub"), memory.NewAppendLog(), &meta.Recorder{}, c, img.Processor{}
	a.SuperAdminEmail = "admin@example.org"
	h := &harness{idp: &fakeIdP{email: "admin@example.org"}}
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
