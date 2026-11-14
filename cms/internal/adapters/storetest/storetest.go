// Package storetest is the contract test suite every store adapter runs.
package storetest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// DocumentStore runs the DocumentStore contract. newStore must return an
// empty store (or one with an isolated prefix).
func DocumentStore(t *testing.T, newStore func(t *testing.T) ports.DocumentStore) {
	ctx := context.Background()

	t.Run("get missing", func(t *testing.T) {
		s := newStore(t)
		if _, err := s.Get(ctx, "a/missing.json"); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("want ErrNotFound, got %v", err)
		}
	})

	t.Run("create only and CAS", func(t *testing.T) {
		s := newStore(t)
		e1, err := s.Put(ctx, "a/doc.json", []byte(`{"v":1}`), "")
		if err != nil || e1 == "" {
			t.Fatalf("create: %q %v", e1, err)
		}
		if _, err := s.Put(ctx, "a/doc.json", []byte(`{"v":9}`), ""); !errors.Is(err, ports.ErrPrecondition) {
			t.Fatalf("second create must fail: %v", err)
		}
		d, err := s.Get(ctx, "a/doc.json")
		if err != nil || d.ETag != e1 || !bytes.Equal(d.Data, []byte(`{"v":1}`)) {
			t.Fatalf("get: %+v %v", d, err)
		}
		e2, err := s.Put(ctx, "a/doc.json", []byte(`{"v":2}`), e1)
		if err != nil || e2 == e1 {
			t.Fatalf("update: %q %v", e2, err)
		}
		if _, err := s.Put(ctx, "a/doc.json", []byte(`{"v":3}`), e1); !errors.Is(err, ports.ErrPrecondition) {
			t.Fatalf("stale etag must fail: %v", err)
		}
		if _, err := s.Put(ctx, "a/missing.json", []byte(`{}`), e1); !errors.Is(err, ports.ErrPrecondition) {
			t.Fatalf("etag on missing key must fail: %v", err)
		}
		if _, err := s.Put(ctx, "a/doc.json", []byte(`{"v":4}`), ports.AnyETag); err != nil {
			t.Fatalf("unconditional: %v", err)
		}
		d, _ = s.Get(ctx, "a/doc.json")
		if string(d.Data) != `{"v":4}` {
			t.Fatalf("data %s", d.Data)
		}
	})

	t.Run("list by prefix", func(t *testing.T) {
		s := newStore(t)
		for _, k := range []string{"orgs/x/events/b.json", "orgs/x/events/a.json", "orgs/x/venues/v.json", "orgs/y/events/c.json"} {
			if _, err := s.Put(ctx, k, []byte(`{}`), ""); err != nil {
				t.Fatal(err)
			}
		}
		keys, err := s.List(ctx, "orgs/x/events/")
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(keys, []string{"orgs/x/events/a.json", "orgs/x/events/b.json"}) {
			t.Fatalf("keys %v", keys)
		}
		none, err := s.List(ctx, "nothing/")
		if err != nil || len(none) != 0 {
			t.Fatalf("empty list: %v %v", none, err)
		}
	})

	t.Run("delete", func(t *testing.T) {
		s := newStore(t)
		e, _ := s.Put(ctx, "d/x.json", []byte(`{}`), "")
		if err := s.Delete(ctx, "d/x.json", `"nope"`); !errors.Is(err, ports.ErrPrecondition) {
			t.Fatalf("stale delete: %v", err)
		}
		if err := s.Delete(ctx, "d/x.json", e); err != nil {
			t.Fatal(err)
		}
		if err := s.Delete(ctx, "d/x.json", ""); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("delete missing: %v", err)
		}
	})

	t.Run("keys cannot leave their namespace", func(t *testing.T) {
		s := newStore(t)
		if _, err := s.Put(ctx, "orgs/b/events/x.json", []byte(`{}`), ""); err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{
			"orgs/a/events/../../b/events/x.json", "orgs/a/./x.json", "orgs//x.json", "/orgs/x.json",
			"orgs/a/x%2F..json", "orgs/a/x\\y.json", "orgs/a/x y.json", "orgs/a/", "",
		} {
			if _, err := s.Get(ctx, k); !errors.Is(err, ports.ErrInvalidKey) {
				t.Errorf("Get(%q): want ErrInvalidKey, got %v", k, err)
			}
			if _, err := s.Put(ctx, k, []byte(`{}`), ports.AnyETag); !errors.Is(err, ports.ErrInvalidKey) {
				t.Errorf("Put(%q): want ErrInvalidKey, got %v", k, err)
			}
			if err := s.Delete(ctx, k, ""); !errors.Is(err, ports.ErrInvalidKey) {
				t.Errorf("Delete(%q): want ErrInvalidKey, got %v", k, err)
			}
		}
		if _, err := s.List(ctx, "orgs/a/../b/"); !errors.Is(err, ports.ErrInvalidKey) {
			t.Errorf("List: want ErrInvalidKey, got %v", err)
		}
	})

	t.Run("concurrent CAS has one winner per round", func(t *testing.T) {
		s := newStore(t)
		etag, _ := s.Put(ctx, "c/n.json", []byte(`"init"`), "")
		var wg sync.WaitGroup
		var mu sync.Mutex
		wins := 0
		for i := range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := s.Put(ctx, "c/n.json", []byte(fmt.Sprint(i)), etag); err == nil {
					mu.Lock()
					wins++
					mu.Unlock()
				} else if !errors.Is(err, ports.ErrPrecondition) {
					t.Errorf("unexpected: %v", err)
				}
			}()
		}
		wg.Wait()
		if wins != 1 {
			t.Fatalf("want exactly one winner, got %d", wins)
		}
	})
}

// PublicStore runs the PublicStore contract.
func PublicStore(t *testing.T, newStore func(t *testing.T) ports.PublicStore) {
	ctx := context.Background()
	s := newStore(t)
	if _, err := s.Get(ctx, "sites/x/events.json"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	e1, err := s.Put(ctx, "sites/x/events.json", []byte(`{"a":1}`), "application/json", "public, max-age=60", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(ctx, "sites/x/events.json", []byte(`{"a":2}`), "application/json", "public, max-age=60", `"stale"`); !errors.Is(err, ports.ErrPrecondition) {
		t.Fatalf("stale: %v", err)
	}
	if _, err := s.Put(ctx, "sites/x/events.json", []byte(`{"a":2}`), "application/json", "public, max-age=60", e1); err != nil {
		t.Fatal(err)
	}
	d, err := s.Get(ctx, "sites/x/events.json")
	if err != nil || string(d.Data) != `{"a":2}` {
		t.Fatalf("get: %s %v", d.Data, err)
	}
	if u := s.URL("media/a.jpg"); u == "" || u[len(u)-len("media/a.jpg"):] != "media/a.jpg" {
		t.Fatalf("url %q", u)
	}
}

// AppendLog runs the AppendLog contract.
func AppendLog(t *testing.T, newLog func(t *testing.T) ports.AppendLog) {
	ctx := context.Background()
	l := newLog(t)
	empty, err := l.Read(ctx, "2026-01-01")
	if err != nil || len(empty) != 0 {
		t.Fatalf("missing segment must read empty: %v", err)
	}
	for _, line := range []string{"one\n", "two\n"} {
		if err := l.Append(ctx, "2026-01-02", []byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.Append(ctx, "2026-01-03", []byte("three\n")); err != nil {
		t.Fatal(err)
	}
	got, err := l.Read(ctx, "2026-01-02")
	if err != nil || string(got) != "one\ntwo\n" {
		t.Fatalf("read %q %v", got, err)
	}
	segs, err := l.Segments(ctx)
	if err != nil || !slices.Equal(segs, []string{"2026-01-02", "2026-01-03"}) {
		t.Fatalf("segments %v %v", segs, err)
	}
}
