package meta

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// bearer is the token a request carries; it must never be in the URL.
func bearer(r *http.Request) string {
	if r.URL.Query().Has("access_token") {
		panic("access token in the URL: " + r.URL.Path)
	}
	tok, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return tok
}

// A token or the app secret never reaches an error: not through a URL that
// cannot be built, a transport failure, or Graph echoing it back (T16).
func TestErrorsCarryNoSecrets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad token ` + bearer(r) + ` for app secret-of-the-app","code":1}}`))
	}))
	defer srv.Close()
	c := New("app", "secret-of-the-app", "")
	c.GraphURL = srv.URL
	const tok = "PAGE-TOKEN-SECRET"
	_, err1 := c.PostFacebookPhoto(context.Background(), "p1", tok, "https://x/i.jpg", "hi")
	c.GraphURL = "http://%zz"
	_, err2 := c.PostFacebookPhoto(context.Background(), "p1", tok, "https://x/i.jpg", "hi")
	c.GraphURL = "http://127.0.0.1:1"
	_, err3 := c.PostFacebookPhoto(context.Background(), "p1", tok, "https://x/i.jpg", "hi")
	for i, err := range []error{err1, err2, err3} {
		if err == nil || strings.Contains(err.Error(), tok) || strings.Contains(err.Error(), "secret-of-the-app") || strings.Contains(err.Error(), "appsecret_proof") {
			t.Errorf("error %d: %v", i+1, err)
		}
	}
}

func TestGraphFlow(t *testing.T) {
	var calls []string
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.URL.Path != "/v24.0/oauth/access_token" {
			if r.Form.Get("appsecret_proof") != (&Client{AppSecret: "sec"}).proof(bearer(r)) {
				t.Errorf("missing appsecret_proof on %s", r.URL.Path)
			}
		}
		out := map[string]any{}
		switch {
		case r.URL.Path == "/v24.0/oauth/access_token" && r.Form.Get("code") != "":
			out["access_token"] = "short"
		case r.URL.Path == "/v24.0/oauth/access_token":
			t.Error("the user token must not be exchanged for a long-lived one")
		case r.URL.Path == "/v24.0/me/accounts":
			if strings.Contains(r.Form.Get("fields"), "access_token") {
				t.Error("listing Facebook pages must not fetch their tokens")
			}
			out["data"] = []any{map[string]any{"id": "p1", "name": "Page",
				"instagram_business_account": map[string]string{"id": "ig1", "username": "bal"}}}
		case r.URL.Path == "/v24.0/p1" && bearer(r) == "short":
			out["id"], out["name"], out["access_token"] = "p1", "Page", "pt"
			out["instagram_business_account"] = map[string]string{"id": "ig1", "username": "bal"}
		case r.URL.Path == "/v24.0/p2":
			out["id"], out["name"] = "p2", "Not mine" // visible, but no role: no token
		case r.URL.Path == "/v24.0/p3":
			w.WriteHeader(400)
			out["error"] = map[string]any{"message": "Unsupported get request", "code": 100}
		case r.URL.Path == "/v24.0/p1/photos":
			if r.Form.Get("url") != "https://img/x.jpg" || r.Form.Get("message") != "Hi" {
				t.Errorf("photo form %v", r.Form)
			}
			out["id"], out["post_id"] = "ph1", "p1_99"
		case r.URL.Path == "/v24.0/ig1/media":
			out["id"] = "c1"
		case r.URL.Path == "/v24.0/c1":
			polls++
			out["status_code"] = map[bool]string{true: "IN_PROGRESS", false: "FINISHED"}[polls < 2]
		case r.URL.Path == "/v24.0/ig1/media_publish":
			out["id"] = "m1"
		case r.URL.Path == "/v24.0/m1":
			out["permalink"] = "https://www.instagram.com/p/abc/"
		case r.URL.Path == "/v24.0/ig1/content_publishing_limit":
			out["data"] = []any{map[string]any{"quota_usage": 3, "config": map[string]int{"quota_total": 100}}}
		case r.URL.Path == "/v24.0/bad/photos":
			w.WriteHeader(400)
			out["error"] = map[string]any{"message": "Invalid", "code": 100}
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()
	c := New("app", "sec", "")
	c.GraphURL, c.PollDelay = srv.URL, time.Millisecond
	ctx := context.Background()
	if u := c.LoginURL("st", "https://cms/auth/meta/callback"); !strings.Contains(u, "pages_manage_posts") || !strings.Contains(u, "state=st") || !strings.Contains(u, "auth_type=rerequest") {
		t.Fatal(u)
	}
	ut, err := c.UserToken(ctx, "code", "https://cms/auth/meta/callback")
	if err != nil || ut != "short" {
		t.Fatalf("%q %v", ut, err)
	}
	pages, err := c.Pages(ctx, ut)
	if err != nil || len(pages) != 1 || pages[0].IGUserID != "ig1" {
		t.Fatalf("%+v %v", pages, err)
	}
	pt, page, err := c.PageToken(ctx, ut, "p1")
	if err != nil || pt != "pt" || page.IGUserID != "ig1" || page.IGUsername != "bal" {
		t.Fatalf("%q %+v %v", pt, page, err)
	}
	for _, id := range []string{"p2", "p3"} {
		if _, _, err := c.PageToken(ctx, ut, id); !errors.Is(err, ports.ErrNoPageAccess) {
			t.Fatalf("%s: want ErrNoPageAccess, got %v", id, err)
		}
	}
	fb, err := c.PostFacebookPhoto(ctx, "p1", "pt", "https://img/x.jpg", "Hi")
	if err != nil || fb.URL != "https://www.facebook.com/p1_99" {
		t.Fatalf("%+v %v", fb, err)
	}
	ig, err := c.PostInstagramImage(ctx, "ig1", "pt", "https://img/x.jpg", "Hi")
	if err != nil || ig.URL != "https://www.instagram.com/p/abc/" || polls != 2 {
		t.Fatalf("%+v %v polls=%d", ig, err, polls)
	}
	used, limit, err := c.InstagramQuota(ctx, "ig1", "pt")
	if err != nil || used != 3 || limit != 100 {
		t.Fatal(used, limit, err)
	}
	_, err = c.PostFacebookPhoto(ctx, "bad", "pt", "https://img/x.jpg", "Hi")
	if ge, ok := err.(*GraphError); !ok || ge.Code != 100 {
		t.Fatalf("graph error: %v", err)
	}
}

func TestErrorsDoNotLeakSecrets(t *testing.T) {
	c := New("app", "very-secret", "")
	c.GraphURL = "http://127.0.0.1:1" // nothing listens: the request fails
	_, err := c.UserToken(context.Background(), "code", "https://cms/cb")
	if err == nil {
		t.Fatal("want error")
	}
	if strings.Contains(err.Error(), "very-secret") || strings.Contains(err.Error(), "client_secret") {
		t.Fatalf("error leaks the app secret: %v", err)
	}
	_, _, err = c.PageToken(context.Background(), "user-token-xyz", "p1")
	if err == nil || strings.Contains(err.Error(), "user-token-xyz") {
		t.Fatalf("error leaks the user token: %v", err)
	}
	_, err = c.PostFacebookPhoto(context.Background(), "p1", "page-token-xyz", "https://img", "Hi")
	if err == nil || strings.Contains(err.Error(), "page-token-xyz") {
		t.Fatalf("error leaks the page token: %v", err)
	}
}
