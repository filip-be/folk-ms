package secretfile_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/filip-be/folk-ms/cms/internal/adapters/secretfile"
	"github.com/filip-be/folk-ms/cms/internal/adapters/storetest"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

func TestContract(t *testing.T) {
	storetest.SecretStore(t, func(t *testing.T) ports.SecretStore {
		s, err := secretfile.New(filepath.Join(t.TempDir(), "secrets"))
		if err != nil {
			t.Fatal(err)
		}
		return s
	})
}

func TestModesAndNoLeftovers(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "secrets")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := secretfile.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, v := range []string{"one", "two"} {
		if err := s.Set(ctx, "org-1", v); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "org-1" {
		t.Fatalf("want only org-1, got %v", entries)
	}
	if runtime.GOOS == "windows" {
		return
	}
	di, _ := os.Stat(dir)
	fi, _ := os.Stat(filepath.Join(dir, "org-1"))
	if di.Mode().Perm() != 0o700 || fi.Mode().Perm() != 0o600 {
		t.Fatalf("modes: dir %v file %v", di.Mode().Perm(), fi.Mode().Perm())
	}
}

func TestNoTraversal(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := secretfile.New(filepath.Join(root, "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := s.Get(ctx, "../outside"); err == nil {
		t.Fatal("read outside the directory")
	}
	if err := s.Delete(ctx, "../outside"); err == nil {
		t.Fatal("deleted outside the directory")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal(err)
	}
}
