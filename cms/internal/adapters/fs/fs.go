// Package fs implements the store ports on the local filesystem: documents
// written by atomic rename under a per-key lock file, an O_APPEND log, and a
// public directory meant to sit behind a web server.
package fs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/ports"
)

const lockSuffix = ".lock"

// staleLock is how old a lock file may get before it is considered left
// behind by a crashed process.
const staleLock = 30 * time.Second

// ValidKey reports whether a key is safe to map onto a path.
func ValidKey(key string) bool {
	if key == "" || len(key) > 512 || strings.HasPrefix(key, "/") || strings.HasSuffix(key, "/") {
		return false
	}
	for _, part := range strings.Split(key, "/") {
		if part == "" || part == "." || part == ".." || strings.HasSuffix(part, lockSuffix) || strings.HasPrefix(part, ".tmp-") {
			return false
		}
	}
	for _, c := range key {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-_./@", c)) {
			return false
		}
	}
	return true
}

func etagOf(data []byte) string {
	h := sha256.Sum256(data)
	return `"` + hex.EncodeToString(h[:12]) + `"`
}

type dir struct {
	root string
	mu   sync.Mutex // serializes writers within this process
}

func (d *dir) path(key string) (string, error) {
	if !ValidKey(key) {
		return "", fmt.Errorf("fs: invalid key %q", key)
	}
	return filepath.Join(d.root, filepath.FromSlash(key)), nil
}

func (d *dir) lock(p string) (func(), error) {
	lp := p + lockSuffix
	deadline := time.Now().Add(10 * time.Second)
	for {
		f, err := os.OpenFile(lp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			f.Close()
			return func() { os.Remove(lp) }, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		if st, err := os.Stat(lp); err == nil && time.Since(st.ModTime()) > staleLock {
			os.Remove(lp)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("fs: lock timeout on %s", lp)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func readFile(p string) ([]byte, error) {
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ports.ErrNotFound
	}
	return data, err
}

func (d *dir) get(key string) (ports.Document, error) {
	p, err := d.path(key)
	if err != nil {
		return ports.Document{}, err
	}
	data, err := readFile(p)
	if err != nil {
		return ports.Document{}, err
	}
	return ports.Document{Key: key, Data: data, ETag: etagOf(data)}, nil
}

func (d *dir) put(key string, data []byte, etag string) (string, error) {
	p, err := d.path(key)
	if err != nil {
		return "", err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	unlock, err := d.lock(p)
	if err != nil {
		return "", err
	}
	defer unlock()
	cur, err := readFile(p)
	exists := err == nil
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return "", err
	}
	switch {
	case etag == "" && exists:
		return "", ports.ErrPrecondition
	case etag != "" && etag != ports.AnyETag && (!exists || etagOf(cur) != etag):
		return "", ports.ErrPrecondition
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return "", err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	tmp.Close()
	if err := renameRetry(tmp.Name(), p); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return etagOf(data), nil
}

// renameRetry retries briefly: on Windows a rename over a file that a reader
// holds open fails transiently.
func renameRetry(from, to string) error {
	var err error
	for range 50 {
		if err = os.Rename(from, to); err == nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return err
}

// DocumentStore is a filesystem ports.DocumentStore.
type DocumentStore struct{ d *dir }

// NewDocumentStore stores documents under root.
func NewDocumentStore(root string) *DocumentStore { return &DocumentStore{d: &dir{root: root}} }

func (s *DocumentStore) Get(_ context.Context, key string) (ports.Document, error) {
	return s.d.get(key)
}

func (s *DocumentStore) Put(_ context.Context, key string, data []byte, etag string) (string, error) {
	return s.d.put(key, data, etag)
}

func (s *DocumentStore) List(_ context.Context, prefix string) ([]string, error) {
	var keys []string
	err := filepath.WalkDir(s.d.root, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if e.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.d.root, p)
		if err != nil {
			return err
		}
		k := filepath.ToSlash(rel)
		if strings.HasPrefix(k, prefix) && ValidKey(k) {
			keys = append(keys, k)
		}
		return nil
	})
	slices.Sort(keys)
	return keys, err
}

func (s *DocumentStore) Delete(_ context.Context, key, etag string) error {
	p, err := s.d.path(key)
	if err != nil {
		return err
	}
	s.d.mu.Lock()
	defer s.d.mu.Unlock()
	unlock, err := s.d.lock(p)
	if err != nil {
		return err
	}
	defer unlock()
	cur, err := readFile(p)
	if err != nil {
		return err
	}
	if etag != "" && etag != ports.AnyETag && etagOf(cur) != etag {
		return ports.ErrPrecondition
	}
	return os.Remove(p)
}

// PublicStore is a directory of public files.
type PublicStore struct {
	d       *dir
	baseURL string
}

// NewPublicStore stores files under root, served at baseURL.
func NewPublicStore(root, baseURL string) *PublicStore {
	return &PublicStore{d: &dir{root: root}, baseURL: strings.TrimRight(baseURL, "/")}
}

// Root returns the directory (for serving it locally).
func (s *PublicStore) Root() string { return s.d.root }

func (s *PublicStore) Put(_ context.Context, path string, data []byte, _, _ string, etag string) (string, error) {
	return s.d.put(path, data, etag)
}

func (s *PublicStore) Get(_ context.Context, path string) (ports.Document, error) {
	return s.d.get(path)
}

func (s *PublicStore) URL(path string) string { return s.baseURL + "/" + path }

// AppendLog writes each segment to <root>/<segment>.log with O_APPEND.
type AppendLog struct {
	root string
	mu   sync.Mutex
}

// NewAppendLog keeps segments under root.
func NewAppendLog(root string) *AppendLog { return &AppendLog{root: root} }

func (l *AppendLog) segPath(segment string) (string, error) {
	if !ValidKey(segment) || strings.Contains(segment, "/") {
		return "", fmt.Errorf("fs: invalid segment %q", segment)
	}
	return filepath.Join(l.root, segment+".log"), nil
}

func (l *AppendLog) Append(_ context.Context, segment string, line []byte) error {
	p, err := l.segPath(segment)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(l.root, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(line); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func (l *AppendLog) Read(_ context.Context, segment string) ([]byte, error) {
	p, err := l.segPath(segment)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

func (l *AppendLog) Segments(_ context.Context) ([]string, error) {
	entries, err := os.ReadDir(l.root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".log"); ok && !e.IsDir() {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out, nil
}
