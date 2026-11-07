package azblob_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/filip-be/folk-ms/cms/internal/adapters/azblob"
	"github.com/filip-be/folk-ms/cms/internal/adapters/storetest"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// Azurite's well-known development account key (public, not a secret).
const azuriteKey = "Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw=="

func client(t *testing.T) *azblob.Client {
	t.Helper()
	base := os.Getenv("AZURITE_BLOB_URL") // e.g. http://127.0.0.1:10000/devstoreaccount1
	if base == "" {
		t.Skip("AZURITE_BLOB_URL not set; run Azurite (see cms/README.md)")
	}
	auth, err := azblob.NewSharedKey("devstoreaccount1", azuriteKey)
	if err != nil {
		t.Fatal(err)
	}
	return azblob.NewClient(base, auth)
}

func container(t *testing.T, c *azblob.Client, public bool) string {
	t.Helper()
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "t" + hex.EncodeToString(b)
	if err := c.CreateContainer(context.Background(), name, public); err != nil {
		t.Fatalf("create container: %v", err)
	}
	t.Cleanup(func() { _ = c.DeleteContainer(context.Background(), name) })
	return name
}

func TestContractAgainstAzurite(t *testing.T) {
	c := client(t)
	storetest.DocumentStore(t, func(t *testing.T) ports.DocumentStore {
		return azblob.NewDocumentStore(c, container(t, c, false), "data/")
	})
	storetest.PublicStore(t, func(t *testing.T) ports.PublicStore {
		name := container(t, c, true)
		return azblob.NewPublicStore(c, name, c.BaseURL+"/"+name)
	})
	storetest.AppendLog(t, func(t *testing.T) ports.AppendLog {
		return azblob.NewAppendLog(c, container(t, c, false), "audit/")
	})
}

func TestPublicHeaders(t *testing.T) {
	c := client(t)
	name := container(t, c, true)
	s := azblob.NewPublicStore(c, name, c.BaseURL+"/"+name)
	if _, err := s.Put(context.Background(), "sites/x/events.json", []byte(`{}`), "application/json", "public, max-age=60", ""); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(s.URL("sites/x/events.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "public, max-age=60" || resp.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("anonymous read: %d %v", resp.StatusCode, resp.Header)
	}
}
