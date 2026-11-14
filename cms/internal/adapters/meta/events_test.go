package meta

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/filip-be/folk-ms/cms/internal/ports"
)

func TestEventsAndLongLivedToken(t *testing.T) {
	var revoked []string
	debug := map[string]any{"is_valid": true, "app_id": "app", "type": "PAGE", "profile_id": "p1",
		"scopes": []string{"pages_show_list", "pages_read_engagement", "pages_manage_posts"}, "data_access_expires_at": 1790000000}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		out := map[string]any{}
		fail := func(code, sub int) {
			w.WriteHeader(400)
			out["error"] = map[string]any{"message": "x", "code": code, "error_subcode": sub}
		}
		switch r.URL.Path {
		case "/v24.0/p1/events":
			if r.Form.Get("include_canceled") != "true" || r.Form.Get("appsecret_proof") == "" {
				t.Errorf("list params %v", r.Form)
			}
			if r.Form.Get("after") == "" {
				out["data"] = []any{map[string]any{"id": "1", "name": "Bal", "start_time": "2026-12-01T19:00:00+0100",
					"place":       map[string]any{"name": "Zamek", "location": map[string]any{"city": "Poznań", "latitude": 52.4, "longitude": 16.9}},
					"cover":       map[string]any{"id": "c1", "source": "https://scontent.xx.fbcdn.net/x.jpg"},
					"event_times": []any{map[string]any{"id": "11", "start_time": "2026-12-01T19:00:00+0100", "end_time": "2026-12-01T22:00:00+0100"}}}}
				out["paging"] = map[string]any{"cursors": map[string]string{"after": "A"}, "next": "https://graph/next"}
			} else {
				out["data"] = []any{map[string]any{"id": "2", "name": "Jam", "is_canceled": true}}
			}
		case "/v24.0/gone":
			fail(100, 33)
		case "/v24.0/expired":
			fail(190, 463)
		case "/v24.0/oauth/access_token":
			if r.Form.Get("grant_type") != "fb_exchange_token" {
				t.Error("exchange")
			}
			out["access_token"] = "long-user"
		case "/v24.0/p1":
			if bearer(r) != "long-user" {
				t.Error("page token derived from the long-lived user token")
			}
			out["id"], out["name"], out["access_token"] = "p1", "Page", "long-page"
		case "/v24.0/me/permissions":
			if bearer(r) != "long-user" {
				t.Error("permissions read with the long-lived user token")
			}
			out["data"] = []any{
				map[string]any{"permission": "pages_read_engagement", "status": "granted"},
				map[string]any{"permission": "business_management", "status": "granted"},
				map[string]any{"permission": "user_events", "status": "declined"},
			}
		case "/v24.0/me/permissions/business_management":
			if r.Method != http.MethodDelete || r.URL.Query().Get("appsecret_proof") == "" {
				t.Errorf("revoke: %s %v", r.Method, r.URL.Query())
			}
			revoked = append(revoked, "business_management")
			out["success"] = true
		case "/v24.0/debug_token":
			if bearer(r) != "app|sec" || r.Form.Get("input_token") != "long-page" {
				t.Error("debug_token with the app token")
			}
			out["data"] = debug
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()
	c := New("app", "sec", "")
	c.GraphURL = srv.URL
	ctx := context.Background()
	evs, err := c.PageEvents(ctx, "p1", "pt")
	if err != nil || len(evs) != 2 {
		t.Fatalf("%v %+v", err, evs)
	}
	e := evs[0]
	if e.Start.IsZero() || e.Place.City != "Poznań" || !e.Place.HasCoordinates || e.CoverID != "c1" || len(e.Times) != 1 || e.Times[0].ID != "11" {
		t.Fatalf("%+v", e)
	}
	if !evs[1].Cancelled {
		t.Fatal("cancelled")
	}
	if _, err := c.Event(ctx, "gone", "pt"); !errors.Is(err, ports.ErrEventGone) {
		t.Fatalf("100/33: %v", err)
	}
	if _, err := c.Event(ctx, "expired", "pt"); !errors.Is(err, ports.ErrTokenRejected) {
		t.Fatalf("190: %v", err)
	}
	tok, page, exp, err := c.LongLivedPageToken(ctx, "short", "p1")
	if err != nil || tok != "long-page" || page.Name != "Page" || exp.Year() != 2026 {
		t.Fatalf("%q %+v %v %v", tok, page, exp, err)
	}
	if len(revoked) != 1 {
		t.Fatalf("extra permissions revoked: %v", revoked)
	}
	// A token that is not this app's page token for this page, or still
	// carries a permission outside Scopes, is never handed over to be stored.
	for name, change := range map[string][2]any{
		"other app": {"app_id", "other"}, "user token": {"type", "USER"}, "other page": {"profile_id", "p2"},
		"extra scope": {"scopes", []string{"pages_read_engagement", "business_management"}}, "invalid": {"is_valid", false},
	} {
		old := debug[change[0].(string)]
		debug[change[0].(string)] = change[1]
		if tok, _, _, err := c.LongLivedPageToken(ctx, "short", "p1"); !errors.Is(err, ports.ErrTokenRejected) || tok != "" {
			t.Errorf("%s: %q %v", name, tok, err)
		}
		debug[change[0].(string)] = old
	}
}

func TestCoverHosts(t *testing.T) {
	c := New("app", "sec", "")
	for _, bad := range []string{"http://scontent.xx.fbcdn.net/x.jpg", "https://evil.example/x.jpg", "https://fbcdn.net.evil.example/x", "https://u@x.fbcdn.net/x", "https://169.254.169.254/x"} {
		if _, err := c.CoverImage(context.Background(), bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}
