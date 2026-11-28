// Package ports declares the interfaces the application depends on. Adapters
// in internal/adapters implement them; only cmd/cms chooses which.
package ports

import (
	"context"
	"errors"
	"time"
)

// Errors adapters return.
var (
	ErrNotFound = errors.New("store: not found")
	// ErrPrecondition means the ETag did not match, or a create-only Put
	// found an existing document.
	ErrPrecondition = errors.New("store: precondition failed")
	// ErrNoPageAccess means the signed-in Meta user has no role on the Facebook page
	// that would let them post.
	ErrNoPageAccess = errors.New("social: no access to the Facebook page")
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
	Put(ctx context.Context, path string, data []byte, contentType, cacheControl, etag string, opts ...PutOption) (string, error)
	// Get returns the file and its ETag (ErrNotFound when missing).
	Get(ctx context.Context, path string) (Document, error)
	// Delete removes a file unconditionally (ErrNotFound when missing).
	Delete(ctx context.Context, path string) error
	// URL returns the absolute public URL of a path.
	URL(path string) string
}

// PutOptions are the optional properties of a public file.
type PutOptions struct {
	// ContentDisposition is served with the file, e.g. `inline; filename="a.pdf"`.
	ContentDisposition string
}

// PutOption sets a PutOptions field.
type PutOption func(*PutOptions)

// WithContentDisposition serves the file with a Content-Disposition header.
func WithContentDisposition(v string) PutOption {
	return func(o *PutOptions) { o.ContentDisposition = v }
}

// ApplyPutOptions collects the options of a Put.
func ApplyPutOptions(opts []PutOption) PutOptions {
	var o PutOptions
	for _, f := range opts {
		f(&o)
	}
	return o
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
	// NoSuperAdmin marks a provider whose email check is not strong enough
	// for the Super Admin: such an identity can never bind or act as it.
	NoSuperAdmin bool
}

// IdentityProvider runs an authorization code flow with PKCE and state (and,
// for OIDC, a nonce) and returns the verified identity.
type IdentityProvider interface {
	// AuthCodeURL returns where to send the browser.
	AuthCodeURL(ctx context.Context, state, nonce, codeChallenge, redirectURI string) (string, error)
	// Exchange redeems the code and verifies the ID token, including nonce.
	Exchange(ctx context.Context, code, codeVerifier, redirectURI, nonce string) (Identity, error)
}

// SocialPage is a Facebook page the signed-in Meta user has a role on, with
// its linked Instagram professional account if any. It carries no token.
type SocialPage struct {
	ID         string
	Name       string
	IGUserID   string
	IGUsername string
}

// PostResult identifies a published Social Post.
type PostResult struct {
	ID  string
	URL string
}

// SocialPublisher talks to Meta. Tokens it returns are short-lived and are
// kept in memory for one request only: the CMS never stores them.
type SocialPublisher interface {
	LoginURL(state, redirectURI string) string
	// UserToken redeems a login code for a short-lived user token.
	UserToken(ctx context.Context, code, redirectURI string) (string, error)
	// Pages lists the Facebook pages the user has a role on.
	Pages(ctx context.Context, userToken string) ([]SocialPage, error)
	// PageToken returns a token for one Facebook page the user has a role on, and
	// the Facebook page as that user sees it (its linked Instagram account included).
	PageToken(ctx context.Context, userToken, pageID string) (string, SocialPage, error)
	PostFacebookPhoto(ctx context.Context, pageID, pageToken, imageURL, caption string) (PostResult, error)
	PostInstagramImage(ctx context.Context, igUserID, pageToken, imageURL, caption string) (PostResult, error)
	// InstagramQuota returns posts used and allowed in the current window.
	InstagramQuota(ctx context.Context, igUserID, pageToken string) (used, limit int, err error)
}

// Clock tells the time.
type Clock interface{ Now() time.Time }

// ImageKind is what an uploaded image is for.
type ImageKind string

const (
	ImageCover  ImageKind = "cover"
	ImageSocial ImageKind = "social"
	// ImageAttachment is a Tune's Attachment: widths 480, 960 and 1600
	// (never upscaled), not cropped, any size.
	ImageAttachment ImageKind = "attachment"
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

// ErrVideoUnavailable means the video provider does not know the video or
// does not let it be embedded.
var ErrVideoUnavailable = errors.New("video: not found or not embeddable")

// VideoInfo asks video providers about a video. Only Vimeo needs asking:
// YouTube thumbnails are derived from the video id.
type VideoInfo interface {
	// Vimeo returns the https thumbnail URL (on i.vimeocdn.com) of a Vimeo
	// video; hash is the unlisted hash or "". ErrVideoUnavailable when Vimeo
	// does not know the video or it cannot be embedded.
	Vimeo(ctx context.Context, id, hash string) (thumbnail string, err error)
}
