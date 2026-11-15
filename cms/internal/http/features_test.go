package httpapi

import (
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"testing"
	"time"
)

// The admin SPA names the sign-in methods /auth/methods answers.
func TestSignInMethodsMatchAdmin(t *testing.T) {
	src, err := os.ReadFile("../../admin/src/api.ts")
	if err != nil {
		t.Fatal(err)
	}
	if want := `export type SignInMethod = "` + providerGoogle + `" | "` + providerFacebook + `";`; !strings.Contains(string(src), want) {
		t.Fatalf("cms/admin/src/api.ts does not declare %s", want)
	}
}

// Facebook sign-in is behind a Feature Flag, off by default: the sign-in
// screen is not offered it and its routes refuse it, while Google and the
// Meta sign-in of the Social Account Connection keep working. Only the
// Super Admin reads or changes the flag.
func TestFacebookSignInFeatureFlag(t *testing.T) {
	h := newHarness(t)
	anon := h.client(t)
	methods := func() string {
		t.Helper()
		_, b := h.do(t, anon, "GET", "/auth/methods", "", nil)
		return strings.TrimSpace(b)
	}
	if m := methods(); m != `{"methods":["google"]}` {
		t.Fatalf("methods while off: %s", m)
	}
	h.fb.email = "ola@example.org"
	if resp, body := h.do(t, anon, "GET", "/auth/login?provider=facebook", "", nil); resp.StatusCode != 403 || !strings.Contains(body, "not available") || h.fb.logins != 0 {
		t.Fatalf("facebook login while off: %d, logins %d, %s", resp.StatusCode, h.fb.logins, body)
	}

	admin := h.client(t)
	h.signIn(t, admin)
	for _, email := range []string{"ola@example.org", "mod@example.org"} {
		if resp, body := h.do(t, admin, "POST", "/api/invitations", `{"email":"`+email+`","orgs":["org-a"]}`, h.same()); resp.StatusCode != 200 {
			t.Fatalf("invite: %d %s", resp.StatusCode, body)
		}
	}
	// The Meta sign-in of the Social Account Connection is not gated.
	h.connect(t, admin)
	if _, body := h.do(t, admin, "GET", "/api/me", "", nil); !strings.Contains(body, `"signInMethods":["google"]`) {
		t.Fatalf("me while off: %s", body)
	}
	if resp, body := h.do(t, admin, "GET", "/api/feature-flags", "", nil); resp.StatusCode != 200 ||
		!strings.Contains(body, `"key":"facebookSignIn"`) || !strings.Contains(body, `"enabled":false`) {
		t.Fatalf("flags: %d %s", resp.StatusCode, body)
	}

	// A Moderator can neither read nor change the flags.
	mod := h.client(t)
	h.idp.email = "mod@example.org"
	h.signIn(t, mod)
	h.idp.email = "admin@example.org"
	if resp, _ := h.do(t, mod, "GET", "/api/feature-flags", "", nil); resp.StatusCode != 403 {
		t.Fatalf("moderator reads flags: %d", resp.StatusCode)
	}
	if resp, _ := h.do(t, mod, "PUT", "/api/feature-flags/facebookSignIn", `{"enabled":true}`, h.same()); resp.StatusCode != 403 {
		t.Fatalf("moderator sets a flag: %d", resp.StatusCode)
	}
	if resp, _ := h.do(t, admin, "PUT", "/api/feature-flags/appleSignIn", `{"enabled":true}`, h.same()); resp.StatusCode != 404 {
		t.Fatalf("unknown flag: %d", resp.StatusCode)
	}
	if resp, _ := h.do(t, admin, "PUT", "/api/feature-flags/facebookSignIn", `{}`, h.same()); resp.StatusCode != 400 {
		t.Fatalf("no value: %d", resp.StatusCode)
	}

	// On: offered, and it signs in.
	if resp, body := h.do(t, admin, "PUT", "/api/feature-flags/facebookSignIn", `{"enabled":true}`, h.same()); resp.StatusCode != 200 || !strings.Contains(body, `"enabled":true`) {
		t.Fatalf("turn on: %d %s", resp.StatusCode, body)
	}
	if m := methods(); m != `{"methods":["google","facebook"]}` {
		t.Fatalf("methods while on: %s", m)
	}
	if resp, _ := h.do(t, anon, "GET", "/auth/login?provider=facebook&return=/app/events", "", nil); resp.Request.URL.Path != "/app/events" || h.fb.logins != 1 {
		t.Fatalf("facebook sign-in while on landed on %s", resp.Request.URL)
	}

	// Turned off while a sign-in is under way: the callback refuses it.
	jar, _ := cookiejar.New(nil)
	stopped := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, _ := h.do(t, stopped, "GET", "/auth/login?provider=facebook&return=/app/events", "", nil)
	cb := resp.Header.Get("Location")
	if resp.StatusCode != 302 || !strings.HasPrefix(cb, h.ts.URL+"/auth/callback") {
		t.Fatalf("login: %d %s", resp.StatusCode, cb)
	}
	if resp, body := h.do(t, admin, "PUT", "/api/feature-flags/facebookSignIn", `{"enabled":false}`, h.same()); resp.StatusCode != 200 || !strings.Contains(body, `"enabled":false`) {
		t.Fatalf("turn off: %d %s", resp.StatusCode, body)
	}
	if resp, body := h.do(t, stopped, "GET", strings.TrimPrefix(cb, h.ts.URL), "", nil); resp.StatusCode != 403 || !strings.Contains(body, "not available") {
		t.Fatalf("callback while off: %d %s", resp.StatusCode, body)
	}
	// The provider remembered from the Facebook sign-in falls back to Google.
	h.do(t, anon, "POST", "/auth/logout", "", h.same())
	googleBefore := h.idp.logins
	h.do(t, anon, "GET", "/auth/login", "", nil)
	if h.idp.logins != googleBefore+1 || h.fb.logins != 2 {
		t.Fatalf("remembered provider while off: google %d fb %d", h.idp.logins-googleBefore, h.fb.logins)
	}

	_, body := h.do(t, admin, "GET", "/api/audit/"+time.Now().UTC().Format(time.DateOnly), "", nil)
	if strings.Count(body, `"action":"feature_flag.changed"`) != 2 {
		t.Fatalf("audit: %s", body)
	}
}
