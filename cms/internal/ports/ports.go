// Package ports declares the interfaces the application depends on. Adapters
// in internal/adapters implement them; only cmd/cms chooses which.
package ports

import (
	"context"
	"errors"
	"time"
)

// Errors every store adapter returns.
var (
	ErrNotFound = errors.New("store: not found")
	// ErrPrecondition means the ETag did not match, or a create-only Put
	// found an existing document.
	ErrPrecondition = errors.New("store: precondition failed")
)

// AnyETag makes Put unconditional.
const AnyETag = "*"

// Document is a stored JSON document with its version.
type Document struct {
	Key  string
	Data []byte
	ETag string
}

// DocumentStore keeps JSON documents with optimistic concurrency.
type DocumentStore interface {
	// Get returns ErrNotFound when the key does not exist.
	Get(ctx context.Context, key string) (Document, error)
	// Put writes data. etag "" means create-only (ErrPrecondition if the key
	// exists); AnyETag writes unconditionally; any other value must match the
	// current ETag. It returns the new ETag.
	Put(ctx context.Context, key string, data []byte, etag string) (string, error)
	// List returns the keys with the prefix, sorted.
	List(ctx context.Context, prefix string) ([]string, error)
	// Delete removes a key; etag as for Put except "" means unconditional.
	// Deleting a missing key returns ErrNotFound.
	Delete(ctx context.Context, key, etag string) error
}

// PublicStore holds publicly readable files: snapshots and media.
type PublicStore interface {
	// Put writes a file. etag as for DocumentStore.Put.
	Put(ctx context.Context, path string, data []byte, contentType, cacheControl, etag string) (string, error)
	// Get returns the file and its ETag (ErrNotFound when missing).
	Get(ctx context.Context, path string) (Document, error)
	// URL returns the absolute public URL of a path.
	URL(path string) string
}

// AppendLog is an append-only log made of named segments (one per day).
type AppendLog interface {
	Append(ctx context.Context, segment string, line []byte) error
	// Read returns a whole segment; a missing segment is empty.
	Read(ctx context.Context, segment string) ([]byte, error)
	// Segments lists segment names, sorted.
	Segments(ctx context.Context) ([]string, error)
}

// Identity is a verified sign-in.
type Identity struct {
	Issuer        string
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

// IdentityProvider runs the OIDC authorization code flow.
type IdentityProvider interface {
	// AuthCodeURL returns where to send the browser.
	AuthCodeURL(ctx context.Context, state, nonce, codeChallenge, redirectURI string) (string, error)
	// Exchange redeems the code and verifies the ID token, including nonce.
	Exchange(ctx context.Context, code, codeVerifier, redirectURI, nonce string) (Identity, error)
}

// SocialPage is a Facebook Page the signed-in Meta user administers.
type SocialPage struct {
	ID          string
	Name        string
	AccessToken string
	IGUserID    string
	IGUsername  string
}

// PostResult identifies a published Social Post.
type PostResult struct {
	ID  string
	URL string
}

// SocialPublisher talks to Meta.
type SocialPublisher interface {
	LoginURL(state, redirectURI string) string
	// Pages exchanges the login code and lists the Pages the user
	// administers, each with a long-lived Page token.
	Pages(ctx context.Context, code, redirectURI string) ([]SocialPage, error)
	PostFacebookPhoto(ctx context.Context, pageID, pageToken, imageURL, caption string) (PostResult, error)
	PostInstagramImage(ctx context.Context, igUserID, pageToken, imageURL, caption string) (PostResult, error)
	// InstagramQuota returns posts used and allowed in the current window.
	InstagramQuota(ctx context.Context, igUserID, pageToken string) (used, limit int, err error)
}

// Clock tells the time.
type Clock interface{ Now() time.Time }

// Cipher encrypts secrets at rest. aad binds a ciphertext to its use.
type Cipher interface {
	Encrypt(plaintext, aad []byte) ([]byte, error)
	Decrypt(ciphertext, aad []byte) ([]byte, error)
}

// ImageKind is what an uploaded image is for.
type ImageKind string

const (
	ImageCover  ImageKind = "cover"
	ImageSocial ImageKind = "social"
)

// ImageRendition is one encoded JPEG produced from an upload.
type ImageRendition struct {
	Name   string // e.g. "960" or "ig"
	Width  int
	Height int
	Data   []byte
}

// ImageProcessor decodes an upload and produces JPEG renditions.
type ImageProcessor interface {
	Process(data []byte, kind ImageKind) ([]ImageRendition, error)
}

// SystemClock is the real clock.
type SystemClock struct{}

// Now returns time.Now.
func (SystemClock) Now() time.Time { return time.Now() }
