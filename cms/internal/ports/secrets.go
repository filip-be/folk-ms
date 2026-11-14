package ports

import (
	"context"
	"time"
)

// SecretStore keeps small secrets by name: the Facebook page token of an
// Organization with Autosync on (ADR 0010). Names are lowercase letters, digits
// and hyphens, at most 127 characters, and are never reused after Delete.
type SecretStore interface {
	// Get returns ErrNotFound when the secret does not exist or was deleted.
	Get(ctx context.Context, name string) (string, error)
	// Set creates or replaces a secret.
	Set(ctx context.Context, name, value string) error
	// Delete removes a secret, for good where the store can (Key Vault:
	// soft delete, then a purge attempt). Deleting a missing secret returns
	// ErrNotFound.
	Delete(ctx context.Context, name string) error
	// List names the secrets whose names start with prefix, never their
	// values.
	List(ctx context.Context, prefix string) ([]SecretInfo, error)
	// PurgeDeleted removes for good the deleted secrets whose names start
	// with prefix and that a store keeps recoverable (Key Vault's soft
	// delete); it returns how many. Stores without soft delete return 0.
	PurgeDeleted(ctx context.Context, prefix string) (int, error)
}

// SecretInfo describes a stored secret without its value.
type SecretInfo struct {
	Name    string
	Created time.Time
}
