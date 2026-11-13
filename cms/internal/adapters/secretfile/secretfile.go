// Package secretfile implements ports.SecretStore as one file per secret in a
// single directory, for local runs and a VPS (ADR 0010). The directory is
// created 0700 and each file 0600; writes go to a temporary file that is
// renamed over the old one, so a reader sees the old or the new value, never
// half of one.
//
// Configuration (read by cmd/cms): SECRETS_DIR, the directory. Keep it out of
// backups that leave the machine, and outside any directory the CMS serves.
package secretfile

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// Store keeps secrets as files in one directory.
type Store struct {
	dir string
}

// New returns a store in dir, creating it with mode 0700 if needed. An
// existing directory is narrowed to 0700.
func New(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("secretfile: no directory")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("secretfile: %w", err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("secretfile: %w", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(abs, 0o700); err != nil {
			return nil, fmt.Errorf("secretfile: %w", err)
		}
	}
	return &Store{dir: abs}, nil
}

var errName = errors.New("secretfile: invalid secret name")

// validName reports whether name matches ^[a-z0-9-]{1,127}$. Such a name has
// no separator or dot, so it cannot leave the directory, and it never starts
// with "." like the temporary files.
func validName(name string) bool {
	if len(name) == 0 || len(name) > 127 {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

func (s *Store) path(name string) (string, error) {
	if !validName(name) {
		return "", errName
	}
	return filepath.Join(s.dir, name), nil
}

// pathErr maps a missing file to ErrNotFound. An os error names only the
// operation and the path, never the value.
func pathErr(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return ports.ErrNotFound
	}
	return fmt.Errorf("secretfile: %w", err)
}

// Get implements ports.SecretStore.
func (s *Store) Get(_ context.Context, name string) (string, error) {
	p, err := s.path(name)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", pathErr(err)
	}
	return string(b), nil
}

// Set implements ports.SecretStore.
func (s *Store) Set(_ context.Context, name, value string) error {
	p, err := s.path(name)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(s.dir, ".tmp-*") // created 0600
	if err != nil {
		return pathErr(err)
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			_ = f.Close()
			_ = os.Remove(tmp)
		}
	}()
	if runtime.GOOS != "windows" {
		if err := f.Chmod(0o600); err != nil {
			return pathErr(err)
		}
	}
	if _, err := f.WriteString(value); err != nil {
		return pathErr(err)
	}
	if err := f.Sync(); err != nil {
		return pathErr(err)
	}
	if err := f.Close(); err != nil {
		return pathErr(err)
	}
	if err := os.Rename(tmp, p); err != nil {
		return pathErr(err)
	}
	ok = true
	return nil
}

// Delete implements ports.SecretStore.
func (s *Store) Delete(_ context.Context, name string) error {
	p, err := s.path(name)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		return pathErr(err)
	}
	return nil
}
