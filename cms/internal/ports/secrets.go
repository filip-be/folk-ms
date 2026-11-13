package ports

import "context"

// SecretStore keeps small secrets by name: the Facebook page token of an
// Organization with Autosync on (ADR 0010). Names are lowercase letters, digits
// and hyphens, at most 127 characters, and are never reused after Delete.
type SecretStore interface {
	// Get returns ErrNotFound when the secret does not exist or was deleted.
	Get(ctx context.Context, name string) (string, error)
	// Set creates or replaces a secret.
	Set(ctx context.Context, name, value string) error
	// Delete removes a secret. Deleting a missing secret returns ErrNotFound.
	Delete(ctx context.Context, name string) error
}
