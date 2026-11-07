package meta

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGraphFlow(t *testing.T) {
	var calls []string
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.URL.Path != "/v24.0/oauth/access_token" {
			if r.Form.Get("appsecret_proof") != (&Client{AppSecret: "sec"}).proof(r.Form.Get("access_token")) {
				t.Errorf("missing appsecret_proof on %s", r.URL.Path)
			}
		}
		out := map[string]any{}
		switch {
		case r.URL.Path == "/v24.0/oauth/access_token" && r.Form.Get("code") != "":
			out["access_token"] = "short"
		case r.URL.Path == "/v24.0/oauth/access_token":
			if r.Form.Get("fb_exchange_token") != "short" {
				t.Error("long-lived exchange")
			}
			out["access_token"] = "long"
		case r.URL.Path == "/v24.0/me/accounts":
			out["data"] = []any{map[string]any{"id": "p1", "name": "Page", "access_token": "pt",
				"instagram_business_account": map[string]string{"id": "ig1", "username": "bal"}}}
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
	if u := c.LoginURL("st", "https://cms/auth/meta/callback"); !strings.Contains(u, "pages_manage_posts") || !strings.Contains(u, "state=st") {
		t.Fatal(u)
	}
	pages, err := c.Pages(ctx, "code", "https://cms/auth/meta/callback")
	if err != nil || len(pages) != 1 || pages[0].IGUserID != "ig1" || pages[0].AccessToken != "pt" {
		t.Fatalf("%+v %v", pages, err)
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
