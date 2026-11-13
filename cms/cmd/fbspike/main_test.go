package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	appSecret = "app-secret-XYZ"
	userTok   = "short-user-TOKEN"
	longTok   = "long-user-TOKEN"
	pageTok   = "page-TOKEN"
)

// fakeGraph answers like Graph with "Require App Secret" on.
func fakeGraph(t *testing.T, eventsOK bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		w.Header().Set("X-App-Usage", `{"call_count":1}`)
		fail := func(status, code int, msg string) {
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": msg, "code": code, "type": "OAuthException"}})
		}
		tok := q.Get("access_token")
		if tok != "" && !strings.Contains(tok, "|") && q.Get("appsecret_proof") == "" {
			fail(400, 100, "API calls from the server require an appsecret_proof argument")
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/v24.0")
		switch {
		case path == "/oauth/access_token":
			json.NewEncoder(w).Encode(map[string]any{"access_token": longTok, "expires_in": 5184000})
		case path == "/debug_token":
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"type": "PAGE", "is_valid": true, "expires_at": 0, "data_access_expires_at": 1790000000,
				"scopes": []string{"pages_show_list", "pages_read_engagement"},
			}})
		case path == "/42/events":
			if !eventsOK {
				fail(400, 10, "(#10) This endpoint requires the 'pages_read_engagement' permission or Page Public Content Access")
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"data": []any{
				map[string]any{"id": "1", "name": "Potańcówka", "start_time": "2026-12-01T19:00:00+0100",
					"owner":       map[string]any{"id": "42", "name": "Test"},
					"event_times": []any{map[string]any{"id": "11", "start_time": "2026-12-01T19:00:00+0100"}}},
				map[string]any{"id": "2", "name": "Warsztaty", "owner": map[string]any{"id": "99", "name": "Other"}},
			}})
		case path == "/42":
			json.NewEncoder(w).Encode(map[string]any{"id": "42", "name": "Test", "access_token": pageTok})
		case path == "/gone":
			fail(400, 100, "Unsupported get request. Object with ID 'gone' does not exist")
		default:
			json.NewEncoder(w).Encode(map[string]any{"id": strings.TrimPrefix(path, "/"), "name": "x"})
		}
	}))
}

func runFake(t *testing.T, eventsOK bool) string {
	srv := fakeGraph(t, eventsOK)
	defer srv.Close()
	var out bytes.Buffer
	cfg := config{AppID: "123", AppSecret: appSecret, Version: "v24.0", GraphURL: srv.URL, PageID: "42",
		UserToken: userTok, Probe: []string{"gone"}, CoHosted: []string{"2", "77"}, MaxPages: 2}
	if err := run(context.Background(), cfg, &out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestReportGo(t *testing.T) {
	rep := runFake(t, true)
	for _, want := range []string{
		"**GO**", "**2 events**", "refused (HTTP 400, code 100", "expires_at 0 (never)",
		"data_access_expires_at 2026-", "Read child 11 by id: OK", "co-hosted events appear in the list",
		"2: in the list **true**", "77: in the list **false**", "gone with page token: **failed**", "`X-App-Usage`",
	} {
		if !strings.Contains(rep, want) {
			t.Errorf("report lacks %q:\n%s", want, rep)
		}
	}
	for _, secret := range []string{appSecret, userTok, longTok, pageTok} {
		if strings.Contains(rep, secret) {
			t.Errorf("report leaks %q", secret)
		}
	}
}

func TestReportNoGo(t *testing.T) {
	rep := runFake(t, false)
	if !strings.Contains(rep, "**NO-GO**") || !strings.Contains(rep, "code 10") {
		t.Errorf("want NO-GO with the Graph error:\n%s", rep)
	}
}
