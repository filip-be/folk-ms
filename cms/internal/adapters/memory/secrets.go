package memory

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// Secrets is an in-memory ports.SecretStore.
type Secrets struct {
	mu      sync.Mutex
	m       map[string]string
	created map[string]time.Time
	// Gets counts reads of a secret (tests).
	Gets int
}

// NewSecrets returns an empty secret store.
func NewSecrets() *Secrets { return &Secrets{m: map[string]string{}, created: map[string]time.Time{}} }

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
	s.Gets++
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
	if _, ok := s.m[name]; !ok {
		s.created[name] = time.Now()
	}
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

// List implements ports.SecretStore.
func (s *Secrets) List(_ context.Context, prefix string) ([]ports.SecretInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []ports.SecretInfo{}
	for name := range s.m {
		if strings.HasPrefix(name, prefix) {
			out = append(out, ports.SecretInfo{Name: name, Created: s.created[name]})
		}
	}
	slices.SortFunc(out, func(a, b ports.SecretInfo) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// SetCreated backdates a secret (tests).
func (s *Secrets) SetCreated(name string, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.created[name] = t
}

// PurgeDeleted implements ports.SecretStore: deleted secrets are gone.
func (s *Secrets) PurgeDeleted(context.Context, string) (int, error) { return 0, nil }
