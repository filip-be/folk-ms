package keyvault_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/adapters/azblob"
	"github.com/filip-be/folk-ms/cms/internal/adapters/keyvault"
	"github.com/filip-be/folk-ms/cms/internal/adapters/storetest"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

const clientID = "11111111-2222-3333-4444-555555555555"

// fakeIdentity is the App Service identity endpoint.
type fakeIdentity struct {
	calls atomic.Int32
	srv   *httptest.Server
}

func newFakeIdentity(t *testing.T, ttl time.Duration) *fakeIdentity {
	f := &fakeIdentity{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Header.Get("X-IDENTITY-HEADER") != "id-header" || q.Get("resource") != keyvault.Resource ||
			q.Get("client_id") != clientID || q.Get("api-version") != "2019-08-01" {
			http.Error(w, "bad identity request", http.StatusBadRequest)
			return
		}
		n := f.calls.Add(1)
		fmt.Fprintf(w, `{"access_token":"bearer-token-%d","expires_on":"%d"}`, n, time.Now().Add(ttl).Unix())
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// fakeVault keeps secrets with soft delete, like Key Vault.
type fakeVault struct {
	mu      sync.Mutex
	secrets map[string]string
	deleted map[string]bool
	tokens  map[string]bool // tokens seen
	srv     *httptest.Server
	// noPurge refuses purges, like a vault whose caller lacks the right;
	// busy answers the next n purges with 409, as while a delete runs.
	noPurge bool
	busy    int
	purged  []string
}

func vaultError(w http.ResponseWriter, status int, code, inner, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	e := map[string]any{"code": code, "message": message}
	if inner != "" {
		e["innererror"] = map[string]any{"code": inner}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"error": e})
}

func newFakeVault(t *testing.T) *fakeVault {
	v := &fakeVault{secrets: map[string]string{}, deleted: map[string]bool{}, tokens: map[string]bool{}}
	v.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v.mu.Lock()
		defer v.mu.Unlock()
		auth, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || !strings.HasPrefix(auth, "bearer-token-") {
			vaultError(w, http.StatusUnauthorized, "Unauthorized", "", "no token")
			return
		}
		v.tokens[auth] = true
		if r.URL.Query().Get("api-version") != keyvault.APIVersion {
			vaultError(w, http.StatusBadRequest, "BadParameter", "", "api-version")
			return
		}
		list := func(m map[string]bool, base string) {
			var vals []any
			for n := range m {
				vals = append(vals, map[string]any{"id": v.srv.URL + base + n, "attributes": map[string]any{"created": 1790000000}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"value": vals})
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/secrets":
			m := map[string]bool{}
			for n := range v.secrets {
				m[n] = true
			}
			list(m, "/secrets/")
			return
		case r.Method == http.MethodGet && r.URL.Path == "/deletedsecrets":
			list(v.deleted, "/deletedsecrets/")
			return
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/deletedsecrets/"):
			n := strings.TrimPrefix(r.URL.Path, "/deletedsecrets/")
			switch {
			case v.noPurge:
				vaultError(w, http.StatusForbidden, "Forbidden", "ForbiddenByRbac", "no purge")
			case v.busy > 0:
				v.busy--
				vaultError(w, http.StatusConflict, "Conflict", "ObjectIsBeingDeleted", "deleting")
			case !v.deleted[n]:
				vaultError(w, http.StatusNotFound, "SecretNotFound", "", "not deleted")
			default:
				delete(v.deleted, n)
				v.purged = append(v.purged, n)
				w.WriteHeader(http.StatusNoContent)
			}
			return
		}
		name, ok := strings.CutPrefix(r.URL.Path, "/secrets/")
		if !ok || name == "" || strings.Contains(name, "/") {
			vaultError(w, http.StatusBadRequest, "BadParameter", "", "path")
			return
		}
		notFound := func() {
			vaultError(w, http.StatusNotFound, "SecretNotFound", "", "A secret with (name/id) "+name+" was not found in this key vault.")
		}
		switch r.Method {
		case http.MethodGet:
			val, ok := v.secrets[name]
			if !ok {
				notFound()
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"value": val, "id": v.srv.URL + "/secrets/" + name + "/v1"})
		case http.MethodPut:
			if v.deleted[name] {
				vaultError(w, http.StatusConflict, "Conflict", "ObjectIsDeletedButRecoverable", "Secret "+name+" is currently in a deleted but recoverable state.")
				return
			}
			var b struct {
				Value *string `json:"value"`
			}
			body, _ := io.ReadAll(r.Body)
			if r.Header.Get("Content-Type") != "application/json" || json.Unmarshal(body, &b) != nil || b.Value == nil {
				vaultError(w, http.StatusBadRequest, "BadParameter", "", "body")
				return
			}
			v.secrets[name] = *b.Value
			_ = json.NewEncoder(w).Encode(map[string]any{"value": *b.Value, "id": v.srv.URL + "/secrets/" + name + "/v2"})
		case http.MethodDelete:
			val, ok := v.secrets[name]
			if !ok {
				notFound()
				return
			}
			delete(v.secrets, name)
			v.deleted[name] = true
			_ = json.NewEncoder(w).Encode(map[string]any{"value": val, "recoveryId": v.srv.URL + "/deletedsecrets/" + name})
		default:
			vaultError(w, http.StatusMethodNotAllowed, "BadParameter", "", "method")
		}
	}))
	t.Cleanup(v.srv.Close)
	return v
}

func newClient(t *testing.T, vaultURL string, id *fakeIdentity, hc *http.Client) *keyvault.Client {
	t.Helper()
	c, err := keyvault.New(vaultURL, &azblob.ManagedIdentity{
		Endpoint: id.srv.URL, Header: "id-header", ClientID: clientID, Resource: keyvault.Resource,
	}, hc)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestContract(t *testing.T) {
	storetest.SecretStore(t, func(t *testing.T) ports.SecretStore {
		v := newFakeVault(t)
		return newClient(t, v.srv.URL+"/", newFakeIdentity(t, time.Hour), v.srv.Client())
	})
}

func TestAccessTokenCachedUntilNearExpiry(t *testing.T) {
	ctx := context.Background()
	v := newFakeVault(t)

	id := newFakeIdentity(t, time.Hour)
	c := newClient(t, v.srv.URL, id, v.srv.Client())
	for range 3 {
		if err := c.Set(ctx, "org-1", "v"); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Get(ctx, "org-1"); err != nil {
			t.Fatal(err)
		}
	}
	if n := id.calls.Load(); n != 1 {
		t.Fatalf("token fetched %d times, want 1", n)
	}

	// A token within five minutes of expiry is replaced.
	short := newFakeIdentity(t, 2*time.Minute)
	c = newClient(t, v.srv.URL, short, v.srv.Client())
	for range 2 {
		if _, err := c.Get(ctx, "org-1"); err != nil {
			t.Fatal(err)
		}
	}
	if n := short.calls.Load(); n != 2 {
		t.Fatalf("short-lived token fetched %d times, want 2", n)
	}
}

// A deleted token is purged at once, so it cannot be recovered from the
// vault; a purge Key Vault refuses while the delete runs is retried, and one
// that never goes through is left to PurgeDeleted.
func TestDeletePurges(t *testing.T) {
	ctx := context.Background()
	v := newFakeVault(t)
	c := newClient(t, v.srv.URL, newFakeIdentity(t, time.Hour), v.srv.Client())
	c.PurgeDelay = time.Millisecond
	for _, n := range []string{"fbpage-a-1", "fbpage-a-2", "session-key"} {
		if err := c.Set(ctx, n, "v"); err != nil {
			t.Fatal(err)
		}
	}
	got, err := c.List(ctx, "fbpage-")
	if err != nil || len(got) != 2 || got[0].Created.IsZero() {
		t.Fatalf("list: %+v %v", got, err)
	}
	v.busy = 2
	if err := c.Delete(ctx, "fbpage-a-1"); err != nil {
		t.Fatal(err)
	}
	if len(v.purged) != 1 || v.deleted["fbpage-a-1"] {
		t.Fatalf("purged %v, deleted %v", v.purged, v.deleted)
	}
	v.noPurge = true
	if err := c.Delete(ctx, "fbpage-a-2"); err != nil {
		t.Fatal(err)
	}
	if !v.deleted["fbpage-a-2"] {
		t.Fatal("refused purge: still in soft delete")
	}
	v.noPurge = false
	if n, err := c.PurgeDeleted(ctx, "fbpage-"); err != nil || n != 1 || v.deleted["fbpage-a-2"] {
		t.Fatalf("purge deleted: %d %v", n, err)
	}
	if _, err := c.Get(ctx, "session-key"); err != nil {
		t.Fatal("other secrets are untouched")
	}
}

func TestDeletedNameIsNotReusable(t *testing.T) {
	ctx := context.Background()
	v := newFakeVault(t)
	v.noPurge = true
	c := newClient(t, v.srv.URL, newFakeIdentity(t, time.Hour), v.srv.Client())
	c.PurgeTries = 1
	if err := c.Set(ctx, "org-1", "v"); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(ctx, "org-1"); err != nil {
		t.Fatal(err)
	}
	err := c.Set(ctx, "org-1", "v2")
	var se *keyvault.StatusError
	if !errors.As(err, &se) || se.Status != http.StatusConflict || se.Code != "ObjectIsDeletedButRecoverable" {
		t.Fatalf("want 409 ObjectIsDeletedButRecoverable, got %v", err)
	}
	if errors.Is(err, ports.ErrNotFound) {
		t.Fatal("409 must not be ErrNotFound")
	}
}

func TestErrorsLeakNothing(t *testing.T) {
	const value = "EAAGpagetokenvalue"
	ctx := context.Background()
	id := newFakeIdentity(t, time.Hour)
	// A vault that echoes the request body and the Authorization header.
	echo := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		vaultError(w, http.StatusInternalServerError, "Internal "+string(body), "x-"+value, r.Header.Get("Authorization")+string(body))
	}))
	defer echo.Close()
	c := newClient(t, echo.URL, id, echo.Client())
	check := func(err error) {
		t.Helper()
		if err == nil {
			t.Fatal("want an error")
		}
		for _, leak := range []string{value, "bearer-token", "api-version"} {
			if strings.Contains(err.Error(), leak) {
				t.Fatalf("error leaks %q: %v", leak, err)
			}
		}
	}
	check(c.Set(ctx, "org-1", value))
	_, err := c.Get(ctx, "org-1")
	check(err)

	// Connection refused: the *url.Error carries no query.
	dead := httptest.NewTLSServer(http.NotFoundHandler())
	deadURL, deadClient := dead.URL, dead.Client()
	dead.Close()
	c = newClient(t, deadURL, id, deadClient)
	check(c.Set(ctx, "org-1", value))
}

func TestRedirectNotFollowed(t *testing.T) {
	ctx := context.Background()
	var followed atomic.Bool
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		followed.Store(true)
	}))
	defer other.Close()
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/steal", http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	c := newClient(t, redirect.URL, newFakeIdentity(t, time.Hour), redirect.Client())
	if _, err := c.Get(ctx, "org-1"); err == nil || errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("want an error, got %v", err)
	}
	if followed.Load() {
		t.Fatal("redirect was followed with the bearer token")
	}
}

func TestVaultURL(t *testing.T) {
	id := newFakeIdentity(t, time.Hour)
	ts := &azblob.ManagedIdentity{Endpoint: id.srv.URL, Header: "id-header", ClientID: clientID, Resource: keyvault.Resource}
	for _, u := range []string{"", "http://x.vault.azure.net/", "https://", "https://x.vault.azure.net/secrets", "https://x.vault.azure.net/?a=b", "https://u:p@x.vault.azure.net/"} {
		if _, err := keyvault.New(u, ts, nil); err == nil {
			t.Fatalf("accepted %q", u)
		}
	}
	for _, u := range []string{"https://x.vault.azure.net", "https://x.vault.azure.net/"} {
		if _, err := keyvault.New(u, ts, nil); err != nil {
			t.Fatalf("rejected %q: %v", u, err)
		}
	}
	if _, err := keyvault.NewManagedIdentity("https://x.vault.azure.net/", "", "", clientID); err == nil {
		t.Fatal("accepted a missing identity endpoint")
	}
	if _, err := keyvault.NewManagedIdentity("https://x.vault.azure.net/", "http://127.0.0.1:1/msi", "h", clientID); err != nil {
		t.Fatal(err)
	}
}
