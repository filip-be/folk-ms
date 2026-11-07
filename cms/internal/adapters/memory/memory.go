// Package memory implements the store ports in memory, for tests and quick
// local runs.
package memory

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/filip-be/folk-ms/cms/internal/ports"
)

type entry struct {
	data         []byte
	etag         string
	contentType  string
	cacheControl string
}

type store struct {
	mu   sync.Mutex
	docs map[string]entry
	n    int
}

func newStore() store { return store{docs: map[string]entry{}} }

func (s *store) get(key string) (ports.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.docs[key]
	if !ok {
		return ports.Document{}, ports.ErrNotFound
	}
	return ports.Document{Key: key, Data: slices.Clone(e.data), ETag: e.etag}, nil
}

func (s *store) put(key string, data []byte, ct, cc, etag string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, exists := s.docs[key]
	switch {
	case etag == "" && exists:
		return "", ports.ErrPrecondition
	case etag != "" && etag != ports.AnyETag && (!exists || cur.etag != etag):
		return "", ports.ErrPrecondition
	}
	s.n++
	tag := fmt.Sprintf(`"m%d"`, s.n)
	s.docs[key] = entry{data: slices.Clone(data), etag: tag, contentType: ct, cacheControl: cc}
	return tag, nil
}

// DocumentStore is an in-memory ports.DocumentStore.
type DocumentStore struct{ s store }

// NewDocumentStore returns an empty store.
func NewDocumentStore() *DocumentStore { return &DocumentStore{s: newStore()} }

func (d *DocumentStore) Get(_ context.Context, key string) (ports.Document, error) {
	return d.s.get(key)
}

func (d *DocumentStore) Put(_ context.Context, key string, data []byte, etag string) (string, error) {
	return d.s.put(key, data, "", "", etag)
}

func (d *DocumentStore) List(_ context.Context, prefix string) ([]string, error) {
	d.s.mu.Lock()
	defer d.s.mu.Unlock()
	var keys []string
	for k := range d.s.docs {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys, nil
}

func (d *DocumentStore) Delete(_ context.Context, key, etag string) error {
	d.s.mu.Lock()
	defer d.s.mu.Unlock()
	cur, ok := d.s.docs[key]
	if !ok {
		return ports.ErrNotFound
	}
	if etag != "" && etag != ports.AnyETag && etag != cur.etag {
		return ports.ErrPrecondition
	}
	delete(d.s.docs, key)
	return nil
}

// PublicStore is an in-memory ports.PublicStore.
type PublicStore struct {
	s       store
	BaseURL string
}

// NewPublicStore returns an empty public store whose URLs start with baseURL.
func NewPublicStore(baseURL string) *PublicStore {
	return &PublicStore{s: newStore(), BaseURL: strings.TrimRight(baseURL, "/")}
}

func (p *PublicStore) Put(_ context.Context, path string, data []byte, ct, cc, etag string) (string, error) {
	return p.s.put(path, data, ct, cc, etag)
}

func (p *PublicStore) Get(_ context.Context, path string) (ports.Document, error) {
	return p.s.get(path)
}

func (p *PublicStore) URL(path string) string { return p.BaseURL + "/" + path }

// Meta returns the stored content type and cache control (for tests).
func (p *PublicStore) Meta(path string) (contentType, cacheControl string) {
	p.s.mu.Lock()
	defer p.s.mu.Unlock()
	e := p.s.docs[path]
	return e.contentType, e.cacheControl
}

// AppendLog is an in-memory ports.AppendLog.
type AppendLog struct {
	mu   sync.Mutex
	segs map[string][]byte
}

// NewAppendLog returns an empty log.
func NewAppendLog() *AppendLog { return &AppendLog{segs: map[string][]byte{}} }

func (a *AppendLog) Append(_ context.Context, segment string, line []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.segs[segment] = append(a.segs[segment], line...)
	return nil
}

func (a *AppendLog) Read(_ context.Context, segment string) ([]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.segs[segment]), nil
}

func (a *AppendLog) Segments(_ context.Context) ([]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []string
	for k := range a.segs {
		out = append(out, k)
	}
	slices.Sort(out)
	return out, nil
}
