package azblob_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/adapters/azblob"
)

func identityServer(t *testing.T, seen *[]url.Values, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("X-IDENTITY-HEADER") != "hdr" {
			http.Error(w, "no header", http.StatusBadRequest)
			return
		}
		*seen = append(*seen, r.URL.Query())
		fmt.Fprintf(w, `{"access_token":"tok-%d","expires_on":"%d"}`, calls.Load(), time.Now().Add(time.Hour).Unix())
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestManagedIdentityClientID(t *testing.T) {
	var seen []url.Values
	var calls atomic.Int32
	srv := identityServer(t, &seen, &calls)
	ctx := context.Background()

	system := &azblob.ManagedIdentity{Endpoint: srv.URL, Header: "hdr"}
	if _, err := system.Token(ctx); err != nil {
		t.Fatal(err)
	}
	user := &azblob.ManagedIdentity{Endpoint: srv.URL, Header: "hdr", ClientID: "0000-cms"}
	tok, err := user.Token(ctx)
	if err != nil {
		t.Fatal(err)
	}
	again, err := user.Token(ctx)
	if err != nil || again != tok {
		t.Fatalf("token not cached: %q %q %v", tok, again, err)
	}
	vault := &azblob.ManagedIdentity{Endpoint: srv.URL, Header: "hdr", ClientID: "0000-cms", Resource: "https://vault.azure.net"}
	if _, err := vault.Token(ctx); err != nil {
		t.Fatal(err)
	}

	if len(seen) != 3 {
		t.Fatalf("want 3 identity calls, got %d", len(seen))
	}
	if seen[0].Has("client_id") || seen[0].Get("resource") != azblob.StorageResource {
		t.Fatalf("system-assigned: %v", seen[0])
	}
	if seen[1].Get("client_id") != "0000-cms" || seen[1].Get("resource") != azblob.StorageResource {
		t.Fatalf("user-assigned: %v", seen[1])
	}
	if seen[2].Get("client_id") != "0000-cms" || seen[2].Get("resource") != "https://vault.azure.net" {
		t.Fatalf("vault resource: %v", seen[2])
	}
}

func TestManagedIdentityErrorHasNoQuery(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	endpoint := srv.URL
	srv.Close() // connection refused
	m := &azblob.ManagedIdentity{Endpoint: endpoint, Header: "hdr", ClientID: "client-id-secretish"}
	_, err := m.Token(context.Background())
	var ue *url.Error
	if err == nil || !errors.As(err, &ue) {
		t.Fatalf("want a url error, got %v", err)
	}
	if strings.Contains(err.Error(), "client-id-secretish") || strings.Contains(err.Error(), "resource=") {
		t.Fatalf("error leaks the query: %v", err)
	}
}
