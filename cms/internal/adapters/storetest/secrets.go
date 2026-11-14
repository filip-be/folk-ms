package storetest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// secretValue is the value the SecretStore contract writes; no error message
// may contain it.
const secretValue = "EAAG-contract-secret-value-7f3a"

// SecretStore runs the SecretStore contract. newStore must return an empty
// store. The suite never sets a name again after deleting it: Key Vault keeps
// a deleted name in soft delete, and callers never reuse names (ADR 0010).
func SecretStore(t *testing.T, newStore func(t *testing.T) ports.SecretStore) {
	ctx := context.Background()

	noValue := func(t *testing.T, err error) {
		t.Helper()
		if err != nil && strings.Contains(err.Error(), secretValue) {
			t.Fatalf("error leaks the secret value: %v", err)
		}
	}

	t.Run("get missing", func(t *testing.T) {
		s := newStore(t)
		_, err := s.Get(ctx, "missing")
		if !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("want ErrNotFound, got %v", err)
		}
	})

	t.Run("set, get, replace", func(t *testing.T) {
		s := newStore(t)
		if err := s.Set(ctx, "org-a-1", secretValue); err != nil {
			noValue(t, err)
			t.Fatalf("set: %v", err)
		}
		if err := s.Set(ctx, "org-b-1", "other"); err != nil {
			t.Fatalf("set other: %v", err)
		}
		v, err := s.Get(ctx, "org-a-1")
		if err != nil || v != secretValue {
			t.Fatalf("get: %q %v", v, err)
		}
		if err := s.Set(ctx, "org-a-1", secretValue+"-2"); err != nil {
			noValue(t, err)
			t.Fatalf("replace: %v", err)
		}
		v, err = s.Get(ctx, "org-a-1")
		if err != nil || v != secretValue+"-2" {
			t.Fatalf("get after replace: %q %v", v, err)
		}
		if v, err := s.Get(ctx, "org-b-1"); err != nil || v != "other" {
			t.Fatalf("other secret changed: %q %v", v, err)
		}
	})

	t.Run("delete", func(t *testing.T) {
		s := newStore(t)
		if err := s.Set(ctx, "gone-1", secretValue); err != nil {
			t.Fatalf("set: %v", err)
		}
		if err := s.Set(ctx, "kept-1", "kept"); err != nil {
			t.Fatalf("set: %v", err)
		}
		if err := s.Delete(ctx, "gone-1"); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := s.Get(ctx, "gone-1"); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("get after delete: want ErrNotFound, got %v", err)
		}
		if err := s.Delete(ctx, "gone-1"); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("delete again: want ErrNotFound, got %v", err)
		}
		if err := s.Delete(ctx, "never-set"); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("delete missing: want ErrNotFound, got %v", err)
		}
		if v, err := s.Get(ctx, "kept-1"); err != nil || v != "kept" {
			t.Fatalf("other secret: %q %v", v, err)
		}
	})

	t.Run("list by prefix", func(t *testing.T) {
		s := newStore(t)
		for _, n := range []string{"fbpage-a-1", "fbpage-b-1", "other-1"} {
			if err := s.Set(ctx, n, secretValue); err != nil {
				t.Fatalf("set: %v", err)
			}
		}
		got, err := s.List(ctx, "fbpage-")
		if err != nil || len(got) != 2 {
			t.Fatalf("list: %+v %v", got, err)
		}
		for _, g := range got {
			if !strings.HasPrefix(g.Name, "fbpage-") || g.Created.IsZero() || strings.Contains(g.Name, secretValue) {
				t.Fatalf("listed %+v", g)
			}
		}
		if err := s.Delete(ctx, "fbpage-a-1"); err != nil {
			t.Fatal(err)
		}
		if got, err := s.List(ctx, "fbpage-"); err != nil || len(got) != 1 || got[0].Name != "fbpage-b-1" {
			t.Fatalf("list after delete: %+v %v", got, err)
		}
		if _, err := s.PurgeDeleted(ctx, "fbpage-"); err != nil {
			t.Fatalf("purge deleted: %v", err)
		}
	})

	t.Run("name limits", func(t *testing.T) {
		s := newStore(t)
		for _, name := range []string{"a", "0-9", strings.Repeat("z", 127)} {
			if err := s.Set(ctx, name, secretValue); err != nil {
				t.Fatalf("valid name %q: %v", name, err)
			}
			if v, err := s.Get(ctx, name); err != nil || v != secretValue {
				t.Fatalf("valid name %q: %q %v", name, v, err)
			}
		}
	})

	t.Run("invalid names", func(t *testing.T) {
		s := newStore(t)
		for _, name := range []string{
			"", "Upper", "a_b", "a.b", "a b", "a/b", "../x", "..", ".", "/abs", `a\b`,
			"a?x=1", "a%2f", "a#b", "ą", "a\x00", strings.Repeat("z", 128),
		} {
			check := func(op string, err error) {
				t.Helper()
				if err == nil || errors.Is(err, ports.ErrNotFound) {
					t.Fatalf("%s %q: want a validation error, got %v", op, name, err)
				}
				noValue(t, err)
			}
			check("set", s.Set(ctx, name, secretValue))
			_, err := s.Get(ctx, name)
			check("get", err)
			check("delete", s.Delete(ctx, name))
		}
	})
}
