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
type fakeIdP struct {
	email, issuer string
	noSA          bool // like Facebook
	logins        int
}

func (f *fakeIdP) AuthCodeURL(_ context.Context, state, nonce, challenge, redirect string) (string, error) {
	f.logins++
	return redirect + "?" + url.Values{"code": {"c"}, "state": {state}}.Encode(), nil
}

func (f *fakeIdP) Exchange(context.Context, string, string, string, string) (ports.Identity, error) {
	iss := f.issuer
	if iss == "" {
		iss = "test"
	}
	return ports.Identity{Issuer: iss, Subject: f.email, Email: f.email, EmailVerified: f.email != "", NoSuperAdmin: f.noSA}, nil
}

type harness struct {
	ts      *httptest.Server
	idp     *fakeIdP
	fb      *fakeIdP
	app     *app.App
	social  *meta.Recorder
	secrets *memory.Secrets
}

const sessionKeyByte = 3

func newHarness(t *testing.T) *harness {
	t.Helper()
	o := &domain.Organization{ID: "org-a", Name: domain.Text{PL: "A"}, Timezone: "Europe/Warsaw",
		Site: domain.Site{ID: "org-a", BaseURL: "https://a.example", PermalinkTemplate: "/{lang}/events/{shortId}-{slug}"}}
	loc, _ := time.LoadLocation(o.Timezone)
	o.SetLocation(loc)
	a := app.New([]*domain.Organization{o})
	rec := &meta.Recorder{}
	secrets := memory.NewSecrets()
	a.Docs, a.Pub, a.Audit, a.Social, a.Events, a.Secrets, a.Images =
		memory.NewDocumentStore(), memory.NewPublicStore("http://x/pub"), memory.NewAppendLog(), rec, rec, secrets, img.Processor{}
	a.SuperAdminEmail = "admin@example.org"
	h := &harness{idp: &fakeIdP{email: "admin@example.org"}, fb: &fakeIdP{issuer: "fb", noSA: true}, app: a, social: rec, secrets: secrets}
	h.ts = httptest.NewUnstartedServer(nil)
	h.ts.Start()
	srv, err := NewServer(&Server{
		App: a, IdP: h.idp, Facebook: h.fb, BaseURL: h.ts.URL,
		Admin: fstest.MapFS{"index.html": {Data: []byte("<!doctype html>spa")}, "app.js": {Data: []byte("js")}},
	}, bytes.Repeat([]byte{sessionKeyByte}, 32))
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

// same marks a same-origin request for the CSRF check.
func (h *harness) same() map[string]string { return map[string]string{"Origin": h.ts.URL} }

// follow GETs an absolute CMS URL (a Meta sign-in that comes straight back
// to the callback) and returns where the redirects ended.
func (h *harness) follow(t *testing.T, c *http.Client, u string) *url.URL {
	t.Helper()
	if !strings.HasPrefix(u, h.ts.URL) {
		t.Fatalf("not a CMS URL: %s", u)
	}
	resp, _ := h.do(t, c, "GET", strings.TrimPrefix(u, h.ts.URL), "", nil)
	return resp.Request.URL
}

// metaLogin expects {"url": …} and follows it.
func (h *harness) metaLogin(t *testing.T, c *http.Client, path, body string) *url.URL {
	t.Helper()
	resp, b := h.do(t, c, "POST", path, body, h.same())
	var out struct{ URL string }
	if resp.StatusCode != 200 || json.Unmarshal([]byte(b), &out) != nil || out.URL == "" {
		t.Fatalf("%s: want a Meta sign-in URL, got %d %s", path, resp.StatusCode, b)
	}
	return h.follow(t, c, out.URL)
}

// connect connects org-a to the Recorder's page 1001 through the API.
func (h *harness) connect(t *testing.T, c *http.Client) {
	t.Helper()
	if got := h.metaLogin(t, c, "/api/orgs/org-a/social/connect", ""); got.Path != "/app/orgs/org-a/social" || got.Query().Get("choose") != "1" {
		t.Fatalf("connect landed on %s", got)
	}
	if resp, b := h.do(t, c, "POST", "/api/orgs/org-a/social/select", `{"pageId":"1001"}`, h.same()); resp.StatusCode != 200 || !strings.Contains(b, `"connected":true`) {
		t.Fatalf("select: %d %s", resp.StatusCode, b)
	}
}

func (h *harness) meID(t *testing.T, c *http.Client) string {
	t.Helper()
	_, b := h.do(t, c, "GET", "/api/me", "", nil)
	var me struct{ ID string }
	if json.Unmarshal([]byte(b), &me) != nil || me.ID == "" {
		t.Fatalf("me: %s", b)
	}
	return me.ID
}

func manualBody(fbID string) string {
	return `{"name":{"pl":"Bal"},"facebookUrl":"https://www.facebook.com/events/` + fbID + `/","occurrences":[{"startDate":"2030-10-01","startTime":"19:00","endTime":"22:00"}]}`
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
	same := h.same()
	resp, body = h.do(t, c, "POST", "/api/orgs/org-a/events", manualBody("777"), same)
	if resp.StatusCode != 200 {
		t.Fatalf("create manual event: %d %s", resp.StatusCode, body)
	}
	var ev app.EventView
	_ = json.Unmarshal([]byte(body), &ev)
	if ev.FacebookURL != "https://www.facebook.com/events/777/" || ev.ETag == "" {
		t.Fatalf("%s", body)
	}
	resp, body = h.do(t, c, "POST", "/api/orgs/org-a/events", manualBody("777"), same)
	if resp.StatusCode != 422 || !strings.Contains(body, `"field":"facebookUrl"`) {
		t.Fatalf("same Facebook Event twice: %d %s", resp.StatusCode, body)
	}
	resp, body = h.do(t, c, "POST", "/api/orgs/org-a/events", `{"name":{"pl":"Bal"},"occurrences":[{"startDate":"2030-10-01","startTime":"19:00"}]}`, same)
	if resp.StatusCode != 422 || !strings.Contains(body, `"field":"facebookUrl"`) {
		t.Fatalf("no Facebook link: %d %s", resp.StatusCode, body)
	}
	manual := "/api/orgs/org-a/events/" + ev.Event.ID + "/manual"
	resp, _ = h.do(t, c, "PUT", manual, manualBody("777"), map[string]string{"Origin": h.ts.URL, "If-Match": `"stale"`})
	if resp.StatusCode != 409 {
		t.Fatalf("stale put: %d", resp.StatusCode)
	}
	resp, body = h.do(t, c, "PUT", manual, strings.Replace(manualBody("777"), `"Bal"`, `"Bal 2"`, 1), map[string]string{"Origin": h.ts.URL, "If-Match": ev.ETag})
	if resp.StatusCode != 200 || !strings.Contains(body, `"Bal 2"`) {
		t.Fatalf("put: %d %s", resp.StatusCode, body)
	}
	if resp, body = h.do(t, c, "POST", "/api/orgs/org-a/events/"+ev.Event.ID+"/hide", "", same); resp.StatusCode != 200 || !strings.Contains(body, `"hidden":true`) {
		t.Fatalf("hide: %d %s", resp.StatusCode, body)
	}
	// Removed features answer 404.
	for _, path := range []string{"/api/orgs/org-a/venues", "/api/orgs/org-a/events/" + ev.Event.ID + "/publish", "/api/orgs/org-a/events/" + ev.Event.ID + "/duplicate"} {
		if resp, _ := h.do(t, c, "POST", path, "", same); resp.StatusCode != 404 {
			t.Errorf("%s: %d", path, resp.StatusCode)
		}
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
	body := manualBody("777")
	for name, hdr := range map[string]map[string]string{
		"cross-site fetch": {"Sec-Fetch-Site": "cross-site", "Origin": h.ts.URL},
		"same-site fetch":  {"Sec-Fetch-Site": "same-site"},
		"foreign origin":   {"Origin": "https://evil.example"},
		"no origin at all": {},
	} {
		if resp, _ := h.do(t, c, "POST", "/api/orgs/org-a/events", body, hdr); resp.StatusCode != 403 {
			t.Errorf("%s: want 403, got %d", name, resp.StatusCode)
		}
	}
	if resp, _ := h.do(t, c, "POST", "/api/orgs/org-a/events", body, map[string]string{"Sec-Fetch-Site": "same-origin"}); resp.StatusCode != 200 {
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
	p, err := h.app.Principal(ctx, h.meID(t, c), time.Now(), "")
	if err != nil {
		t.Fatal(err)
	}
	var png bytes.Buffer
	_ = pngEncode(&png, 1200, 600)
	cover, err := h.app.UploadImage(ctx, p, "org-a", ports.ImageCover, png.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	ev, err := h.app.CreateManualEvent(ctx, p, "org-a", domain.ManualInput{Name: domain.Text{PL: "Bal"}, Cover: cover,
		FacebookURL: "https://www.facebook.com/events/777/", Occurrences: []domain.OccurrenceInput{{StartDate: "2030-10-01", StartTime: "19:00", EndTime: "22:00"}}})
	if err != nil {
		t.Fatal(err)
	}
	h.connect(t, c)
	same := h.same()
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
	// platforms with its page token and returns to the Post dialog.
	login := start()
	got := h.follow(t, c, login)
	if got.Path != "/app/orgs/org-a/events/"+ev.Event.ID+"/post" || got.Query().Get("facebook") != "posted" || got.Query().Get("instagram") != "posted" {
		t.Fatalf("landed on %s", got)
	}
	if len(h.social.Posts) != 2 || h.social.PostTokens[0] != "page-token-1001" {
		t.Fatalf("%+v %v", h.social.Posts, h.social.PostTokens)
	}
	// The state cookie is used once.
	if resp, _ := h.do(t, c, "GET", strings.TrimPrefix(login, h.ts.URL), "", nil); resp.StatusCode != 403 || len(h.social.Posts) != 2 {
		t.Fatalf("replayed callback: %d, %d posts", resp.StatusCode, len(h.social.Posts))
	}

	// Cancelling the Meta sign-in posts nothing and drops the request.
	login = start()
	lu, _ := url.Parse(login)
	resp, _ := h.do(t, c, "GET", "/auth/meta/callback?error=access_denied&state="+lu.Query().Get("state"), "", nil)
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

// forge returns a client whose session cookie is signed with the session
// key but carries a nonce of no real sign-in (T5).
func (h *harness) forge(t *testing.T, userID, nonce string) *http.Client {
	t.Helper()
	now := time.Now()
	v, err := signer{key: bytes.Repeat([]byte{sessionKeyByte}, 32)}.seal("session", sessionData{UserID: userID, IssuedAt: now.Unix(), Expires: now.Add(time.Hour).Unix(), Nonce: nonce})
	if err != nil {
		t.Fatal(err)
	}
	c := h.client(t)
	u, _ := url.Parse(h.ts.URL)
	c.Jar.SetCookies(u, []*http.Cookie{{Name: sessionCookie, Value: v, Path: "/"}})
	return c
}

// A session counts as recently signed in only when its nonce is on the
// user document: a cookie forged with the session key alone reads but
// cannot do guarded actions.
func TestRecentSignInNeedsTheSessionNonce(t *testing.T) {
	h := newHarness(t)
	c := h.client(t)
	h.signIn(t, c)
	h.connect(t, c)
	id := h.meID(t, c)

	// The real session cookie carries a nonce whose hash is on the user.
	u, _ := url.Parse(h.ts.URL)
	var sd sessionData
	for _, ck := range c.Jar.Cookies(u) {
		if ck.Name == sessionCookie {
			if err := (signer{key: bytes.Repeat([]byte{sessionKeyByte}, 32)}).open("session", ck.Value, &sd); err != nil {
				t.Fatal(err)
			}
		}
	}
	d, err := h.app.Docs.Get(context.Background(), "users/"+id+".json")
	if err != nil {
		t.Fatal(err)
	}
	if sd.Nonce == "" || !strings.Contains(string(d.Data), domain.HashNonce(sd.Nonce)) || strings.Contains(string(d.Data), sd.Nonce) {
		t.Fatalf("nonce %q, user %s", sd.Nonce, d.Data)
	}

	for _, nonce := range []string{"not-a-real-sign-in", ""} {
		f := h.forge(t, id, nonce)
		for _, path := range []string{"/api/me", "/api/orgs/org-a/social", "/api/orgs/org-a/events", "/api/orgs/org-a/sync"} {
			if resp, body := h.do(t, f, "GET", path, "", nil); resp.StatusCode != 200 {
				t.Fatalf("forged %q GET %s: %d %s", nonce, path, resp.StatusCode, body)
			}
		}
		if _, body := h.do(t, f, "GET", "/api/orgs/org-a/social", "", nil); !strings.Contains(body, `"recentSignIn":false`) {
			t.Fatalf("forged session counts as recent: %s", body)
		}
		resp, body := h.do(t, f, "POST", "/api/orgs/org-a/social/autosync", `{"on":true}`, h.same())
		if resp.StatusCode != 403 || !strings.Contains(body, `"error":"recent_sign_in"`) {
			t.Fatalf("forged %q autosync on: %d %s", nonce, resp.StatusCode, body)
		}
	}

	// The real session turns Autosync on through a Meta sign-in.
	if _, body := h.do(t, c, "GET", "/api/orgs/org-a/social", "", nil); !strings.Contains(body, `"recentSignIn":true`) {
		t.Fatalf("real session: %s", body)
	}
	if got := h.metaLogin(t, c, "/api/orgs/org-a/social/autosync", `{"on":true}`); got.Path != "/app/orgs/org-a/social" || got.Query().Get("autosync") != "on" {
		t.Fatalf("autosync landed on %s", got)
	}
	f := h.forge(t, id, "not-a-real-sign-in")
	for _, call := range [][2]string{{"/api/orgs/org-a/social/autosync", `{"on":false}`}, {"/api/orgs/org-a/social/disconnect", ""}} {
		if resp, body := h.do(t, f, "POST", call[0], call[1], h.same()); resp.StatusCode != 403 || !strings.Contains(body, "recent_sign_in") {
			t.Fatalf("forged %s: %d %s", call[0], resp.StatusCode, body)
		}
	}

	// With Autosync on, "Sync now" runs at once; the API never returns the token.
	resp, body := h.do(t, c, "POST", "/api/orgs/org-a/sync", "", h.same())
	if resp.StatusCode != 200 || !strings.Contains(body, `"result"`) || strings.Contains(body, `"url"`) {
		t.Fatalf("sync now: %d %s", resp.StatusCode, body)
	}
	resp, body = h.do(t, c, "POST", "/api/orgs/org-a/sync", "", h.same())
	if resp.StatusCode != 429 || !strings.Contains(body, `"error":"too_soon"`) {
		t.Fatalf("sync now again: %d %s", resp.StatusCode, body)
	}
	var events []app.EventSummary
	_, body = h.do(t, c, "GET", "/api/orgs/org-a/events", "", nil)
	if json.Unmarshal([]byte(body), &events) != nil || len(events) == 0 {
		t.Fatalf("events: %s", body)
	}
	paths := []string{"/api/orgs/org-a/social", "/api/orgs/org-a/events", "/api/orgs/org-a/sync", "/api/orgs/org-a/events/" + events[0].ID,
		"/api/orgs/org-a/events/" + events[0].ID + "/posts", "/api/me"}
	for _, path := range paths {
		resp, body := h.do(t, c, "GET", path, "", nil)
		if resp.StatusCode != 200 || strings.Contains(body, "page-token") || strings.Contains(body, "fbpage-") {
			t.Fatalf("GET %s: %d %s", path, resp.StatusCode, body)
		}
	}
	if got := h.social.Calls[0]; !strings.HasPrefix(got, "list 1001 long-page-token-1001-") {
		t.Fatalf("sync used %q", got)
	}
}

// "Sync now" with Autosync off answers the Meta sign-in URL; the callback
// with the sync state runs it with the sign-in's page token.
func TestSyncNowWithMetaSignIn(t *testing.T) {
	h := newHarness(t)
	c := h.client(t)
	h.signIn(t, c)
	resp, body := h.do(t, c, "POST", "/api/orgs/org-a/sync", "", h.same())
	if resp.StatusCode != 422 {
		t.Fatalf("without a connection: %d %s", resp.StatusCode, body)
	}
	h.connect(t, c)
	got := h.metaLogin(t, c, "/api/orgs/org-a/sync", "")
	if got.Path != "/app/orgs/org-a/events" || got.Query().Get("sync") != "ok" {
		t.Fatalf("sync landed on %s", got)
	}
	if len(h.social.Calls) != 1 || h.social.Calls[0] != "list 1001 page-token-1001" {
		t.Fatalf("calls: %v", h.social.Calls)
	}
	_, body = h.do(t, c, "GET", "/api/orgs/org-a/events", "", nil)
	var events []app.EventSummary
	if json.Unmarshal([]byte(body), &events) != nil || len(events) != 3 {
		t.Fatalf("events: %s", body)
	}
	resp, body = h.do(t, c, "POST", "/api/orgs/org-a/sync", "", h.same())
	if resp.StatusCode != 429 || !strings.Contains(body, `"error":"too_soon"`) {
		t.Fatalf("again within a minute: %d %s", resp.StatusCode, body)
	}
	_, body = h.do(t, c, "GET", "/api/orgs/org-a/sync", "", nil)
	if !strings.Contains(body, `"autosync":false`) || !strings.Contains(body, `"lastSuccessAt"`) || strings.Contains(body, "leaseHolder") {
		t.Fatalf("status: %s", body)
	}
}

// POST /sync is public: it takes no input, runs Autosync only, and is
// bounded per instance, per client and per Organization (T10).
func TestTimerSync(t *testing.T) {
	h := newHarness(t)
	c := h.client(t)
	h.signIn(t, c)
	h.connect(t, c)
	h.metaLogin(t, c, "/api/orgs/org-a/social/autosync", `{"on":true}`)
	anon := h.client(t)
	resp, body := h.do(t, anon, "POST", "/sync", `{"org":"org-a","token":"x"}`, nil)
	if resp.StatusCode != 200 || !strings.Contains(body, `"Outputs"`) {
		t.Fatalf("timer: %d %s", resp.StatusCode, body)
	}
	if len(h.social.Calls) != 1 || !strings.HasPrefix(h.social.Calls[0], "list 1001 long-page-token-") {
		t.Fatalf("calls: %v", h.social.Calls)
	}
	if _, body = h.do(t, anon, "POST", "/sync", "", nil); !strings.Contains(body, "skipped") || len(h.social.Calls) != 1 {
		t.Fatalf("a second sync right away must be skipped: %s", body)
	}
	limited := false
	for range 30 {
		if resp, _ := h.do(t, anon, "POST", "/sync", "", nil); resp.StatusCode == 429 {
			limited = true
			break
		}
	}
	if !limited || len(h.social.Calls) != 1 {
		t.Fatalf("rate limit: %v, calls %v", limited, h.social.Calls)
	}
}

// The pages Meta reviews say a token is kept only while automatic sync is on.
func TestPrivacyPagesMentionTheToken(t *testing.T) {
	h := newHarness(t)
	for path, want := range map[string][]string{
		"/privacy":       {"Unless the organization turned automatic sync on, we store no Meta token", "for as long as sync stays on", "Azure Key Vault"},
		"/data-deletion": {"turn automatic sync off", "Business integrations"},
	} {
		resp, body := h.do(t, h.client(t), "GET", path, "", nil)
		for _, w := range want {
			if resp.StatusCode != 200 || !strings.Contains(body, w) {
				t.Errorf("%s lacks %q", path, w)
			}
		}
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
	resp, body := h.do(t, c, "POST", "/api/orgs/org-a/events", `{"name":{"pl":""}}`, map[string]string{"Sec-Fetch-Site": "same-origin"})
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
		app.RefusedInstagram, app.RefusedQuota, app.RefusedLimit, app.RefusedToken, app.RefusedConfirm, string(domain.PostPosted), string(domain.PostFailed),
	} {
		if !strings.Contains(string(src), `case "`+code+`":`) {
			t.Errorf("cms/admin/src/views/post.ts has no message for the outcome %q", code)
		}
	}
}

func TestFacebookSignIn(t *testing.T) {
	h := newHarness(t)
	admin := h.client(t)
	h.signIn(t, admin)
	if resp, body := h.do(t, admin, "POST", "/api/invitations", `{"email":"ola@example.org","orgs":["org-a"]}`,
		map[string]string{"Origin": h.ts.URL}); resp.StatusCode/100 != 2 {
		t.Fatalf("invite: %d %s", resp.StatusCode, body)
	}

	c := h.client(t)
	get := func(path string) (*http.Response, string) {
		t.Helper()
		return h.do(t, c, "GET", path, "", nil)
	}
	h.fb.email = "ola@example.org"
	if resp, _ := get("/auth/login?provider=facebook&return=/app/events"); resp.Request.URL.Path != "/app/events" || h.fb.logins != 1 {
		t.Fatalf("facebook sign-in landed on %s", resp.Request.URL)
	}
	if _, body := get("/api/me"); !strings.Contains(body, `"email":"ola@example.org"`) {
		t.Fatalf("me: %s", body)
	}
	// An expired session goes back to the provider used last.
	h.do(t, c, "POST", "/auth/logout", "", map[string]string{"Origin": h.ts.URL})
	googleBefore := h.idp.logins
	get("/auth/login")
	if h.fb.logins != 2 || h.idp.logins != googleBefore {
		t.Fatalf("remembered provider: fb %d google %d", h.fb.logins, h.idp.logins)
	}

	if resp, body := get("/auth/login?provider=apple"); resp.StatusCode != 403 || !strings.Contains(body, "not available") {
		t.Fatalf("unknown provider: %d %s", resp.StatusCode, body)
	}
	h.fb.email = ""
	if resp, body := get("/auth/login?provider=facebook"); resp.StatusCode != 403 || !strings.Contains(body, "did not share an email") {
		t.Fatalf("no email: %d %s", resp.StatusCode, body)
	}
	h.fb.email = "admin@example.org"
	if resp, body := get("/auth/login?provider=facebook"); resp.StatusCode != 403 || !strings.Contains(body, "only with the Google account") {
		t.Fatalf("super admin via facebook: %d %s", resp.StatusCode, body)
	}
}
