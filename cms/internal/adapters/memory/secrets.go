package memory

import (
	"context"
	"errors"
	"sync"

	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// Secrets is an in-memory ports.SecretStore.
type Secrets struct {
	mu sync.Mutex
	m  map[string]string
}

// NewSecrets returns an empty secret store.
func NewSecrets() *Secrets { return &Secrets{m: map[string]string{}} }

var errSecretName = errors.New("memory: invalid secret name")

// validSecretName reports whether name matches ^[a-z0-9-]{1,127}$.
func validSecretName(name string) bool {
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

func (s *Secrets) Get(_ context.Context, name string) (string, error) {
	if !validSecretName(name) {
		return "", errSecretName
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[name]
	if !ok {
		return "", ports.ErrNotFound
	}
	return v, nil
}

func (s *Secrets) Set(_ context.Context, name, value string) error {
	if !validSecretName(name) {
		return errSecretName
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[name] = value
	return nil
}

func (s *Secrets) Delete(_ context.Context, name string) error {
	if !validSecretName(name) {
		return errSecretName
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[name]; !ok {
		return ports.ErrNotFound
	}
	delete(s.m, name)
	return nil
}
