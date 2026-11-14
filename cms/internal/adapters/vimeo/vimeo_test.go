package vimeo

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/filip-be/folk-ms/cms/internal/ports"
)

func TestVimeo(t *testing.T) {
	var gotURL string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.Query().Get("url")
		switch {
		case strings.HasSuffix(gotURL, "/1"):
			w.Write([]byte(`{"type":"video","thumbnail_url":"https://i.vimeocdn.com/video/1-d_640"}`))
		case strings.HasSuffix(gotURL, "/2"):
			http.Error(w, "nope", http.StatusNotFound)
		case strings.HasSuffix(gotURL, "/3"):
			http.Error(w, "private", http.StatusForbidden)
		case strings.HasSuffix(gotURL, "/4"):
			w.Write([]byte(`{"thumbnail_url":"http://evil.example/x.jpg"}`))
		case strings.HasSuffix(gotURL, "/5"):
			w.Write([]byte(`{"thumbnail_url":"https://i.vimeocdn.com/` + strings.Repeat("a", MaxBody) + `"}`))
		default:
			http.Error(w, "down", http.StatusBadGateway)
		}
	}))
	defer ts.Close()
	c := New()
	c.Endpoint = ts.URL
	ctx := context.Background()
	if th, err := c.Vimeo(ctx, "1", ""); err != nil || th != "https://i.vimeocdn.com/video/1-d_640" {
		t.Fatalf("ok: %q %v", th, err)
	}
	if _, err := c.Vimeo(ctx, "7", "abcdef12"); err == nil || errors.Is(err, ports.ErrVideoUnavailable) || gotURL != "https://vimeo.com/7/abcdef12" {
		t.Fatalf("server error: %v (url %s)", err, gotURL)
	}
	for _, id := range []string{"2", "3"} {
		if _, err := c.Vimeo(ctx, id, ""); !errors.Is(err, ports.ErrVideoUnavailable) {
			t.Fatalf("%s: %v", id, err)
		}
	}
	for _, id := range []string{"4", "5"} {
		if _, err := c.Vimeo(ctx, id, ""); err == nil || errors.Is(err, ports.ErrVideoUnavailable) {
			t.Fatalf("%s: %v", id, err)
		}
	}
}
