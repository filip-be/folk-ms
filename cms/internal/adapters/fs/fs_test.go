package fs_test

import (
	"context"
	"testing"

	"github.com/filip-be/folk-ms/cms/internal/adapters/fs"
	"github.com/filip-be/folk-ms/cms/internal/adapters/storetest"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

func TestContract(t *testing.T) {
	storetest.DocumentStore(t, func(t *testing.T) ports.DocumentStore { return fs.NewDocumentStore(t.TempDir()) })
	storetest.PublicStore(t, func(t *testing.T) ports.PublicStore { return fs.NewPublicStore(t.TempDir(), "http://x/pub") })
	storetest.AppendLog(t, func(t *testing.T) ports.AppendLog { return fs.NewAppendLog(t.TempDir()) })
}

func TestRejectsTraversal(t *testing.T) {
	s := fs.NewDocumentStore(t.TempDir())
	for _, k := range []string{"../x.json", "a/../../x", "/abs", "a//b", "a/b.lock", `a\b`} {
		if _, err := s.Put(context.Background(), k, []byte("{}"), ""); err == nil {
			t.Errorf("key %q must be rejected", k)
		}
	}
}
