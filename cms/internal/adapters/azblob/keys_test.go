package azblob_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/filip-be/folk-ms/cms/internal/adapters/azblob"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// Blob Storage resolves dot segments in the request path and a bearer token
// does not pin the path, so a name that could leave its prefix or container
// must never be sent (T13).
func TestNamesCannotLeaveTheirNamespace(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	key, err := azblob.NewSharedKey("acct", base64.StdEncoding.EncodeToString([]byte("k")))
	if err != nil {
		t.Fatal(err)
	}
	c := azblob.NewClient(srv.URL, key)
	docs := azblob.NewDocumentStore(c, "cms", "data/")
	pub := azblob.NewPublicStore(c, "pub", "https://x/pub")
	log := azblob.NewAppendLog(c, "cms", "audit/")
	ctx := context.Background()
	bad := "orgs/a/events/../../../../../pub/sites/x/events.json"
	checks := map[string]error{}
	_, checks["get"] = docs.Get(ctx, bad)
	_, checks["put"] = docs.Put(ctx, bad, []byte(`{}`), ports.AnyETag)
	checks["delete"] = docs.Delete(ctx, bad, "")
	_, checks["list"] = docs.List(ctx, "orgs/a/../")
	_, checks["public put"] = pub.Put(ctx, "../cms/data/users/x.json", []byte(`{}`), "application/json", "", ports.AnyETag)
	checks["append"] = log.Append(ctx, "../data/x", []byte("x\n"))
	for name, err := range checks {
		if !errors.Is(err, ports.ErrInvalidKey) {
			t.Errorf("%s: want ErrInvalidKey, got %v", name, err)
		}
	}
	if n := calls.Load(); n != 0 {
		t.Fatalf("%d requests sent", n)
	}
}

// A full append blob (50,000 blocks) does not stop the Audit Log: lines go
// on in a new part, and reading the day returns every part (T8).
func TestAuditLogRollsOverAFullBlob(t *testing.T) {
	blobs := map[string][]byte{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Path
		switch {
		case r.Method == http.MethodPut && r.URL.Query().Get("comp") == "appendblock":
			b, ok := blobs[name]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if strings.Count(string(b), "\n") >= 2 {
				w.Header().Set("x-ms-error-code", "BlockCountExceedsLimit")
				w.WriteHeader(http.StatusConflict)
				return
			}
			line, _ := io.ReadAll(r.Body)
			blobs[name] = append(b, line...)
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPut:
			blobs[name] = []byte{}
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodGet && r.URL.Query().Get("comp") == "list":
			var names []string
			for n := range blobs {
				names = append(names, "<Blob><Name>"+strings.TrimPrefix(n, "/cms/")+"</Name></Blob>")
			}
			sort.Strings(names)
			fmt.Fprintf(w, "<EnumerationResults><Blobs>%s</Blobs><NextMarker/></EnumerationResults>", strings.Join(names, ""))
		case r.Method == http.MethodGet:
			b, ok := blobs[name]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write(b)
		}
	}))
	t.Cleanup(srv.Close)
	key, _ := azblob.NewSharedKey("acct", base64.StdEncoding.EncodeToString([]byte("k")))
	log := azblob.NewAppendLog(azblob.NewClient(srv.URL, key), "cms", "audit/")
	ctx := context.Background()
	for i := range 5 {
		if err := log.Append(ctx, "2026-11-14", []byte(fmt.Sprintf("line %d\n", i))); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	data, err := log.Read(ctx, "2026-11-14")
	if err != nil || string(data) != "line 0\nline 1\nline 2\nline 3\nline 4\n" {
		t.Fatalf("read: %q %v", data, err)
	}
	segs, err := log.Segments(ctx)
	if err != nil || len(segs) != 1 || segs[0] != "2026-11-14" {
		t.Fatalf("segments: %v %v", segs, err)
	}
}
