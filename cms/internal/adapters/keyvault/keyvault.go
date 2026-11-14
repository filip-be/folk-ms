// Package keyvault implements ports.SecretStore on Azure Key Vault through its
// REST API (no SDK, ADR 0003), with an access token from the managed identity
// endpoint (ADR 0010).
//
// Configuration (read by cmd/cms):
//   - KEYVAULT_URL: the vault, e.g. https://folkms-kv-abc.vault.azure.net/
//   - AZURE_CLIENT_ID: client id of the CMS's user-assigned managed identity
//   - IDENTITY_ENDPOINT, IDENTITY_HEADER: set by App Service / Functions
//
// The access token is cached until shortly before it expires; secret values
// are read on every Get and never cached (T3). Errors carry the HTTP status
// and Key Vault's error code only: never a secret value, the bearer token or
// a response body.
package keyvault

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/adapters/azblob"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// Resource is the token audience for Key Vault.
const Resource = "https://vault.azure.net"

// APIVersion is the Key Vault REST version the adapter speaks.
const APIVersion = "7.4"

// maxBody bounds what the adapter reads from a response (a secret is at most
// 25 KiB).
const maxBody = 256 << 10

// TokenSource returns a bearer token for Resource.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// Client is a SecretStore on one vault.
type Client struct {
	base   string // https://name.vault.azure.net, no trailing slash
	tokens TokenSource
	http   *http.Client
	// PurgeTries and PurgeDelay bound the purge attempted right after a
	// delete: Key Vault answers 409 while the deletion is in progress.
	PurgeTries int
	PurgeDelay time.Duration
}

// New returns a client for vaultURL (https only). hc may be nil; redirects
// are never followed, so the bearer token goes only to the vault.
func New(vaultURL string, tokens TokenSource, hc *http.Client) (*Client, error) {
	u, err := url.Parse(vaultURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil ||
		u.RawQuery != "" || u.Fragment != "" || strings.Trim(u.Path, "/") != "" {
		return nil, errors.New("keyvault: vault URL must be https://<name>.vault.azure.net/")
	}
	if tokens == nil {
		return nil, errors.New("keyvault: no token source")
	}
	var c http.Client
	if hc != nil {
		c = *hc
	} else {
		c.Timeout = 30 * time.Second
	}
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{base: "https://" + u.Host, tokens: tokens, http: &c, PurgeTries: 5, PurgeDelay: 2 * time.Second}, nil
}

// NewManagedIdentity returns a client that authenticates with the App
// Service / Functions managed identity (endpoint and header from
// IDENTITY_ENDPOINT and IDENTITY_HEADER). clientID selects a user-assigned
// identity; empty uses the system-assigned one.
func NewManagedIdentity(vaultURL, endpoint, header, clientID string) (*Client, error) {
	if endpoint == "" || header == "" {
		return nil, errors.New("keyvault: no managed identity (IDENTITY_ENDPOINT, IDENTITY_HEADER)")
	}
	return New(vaultURL, &azblob.ManagedIdentity{
		Endpoint: endpoint, Header: header, ClientID: clientID, Resource: Resource,
	}, nil)
}

// StatusError is a non-2xx answer from Key Vault.
type StatusError struct {
	Status int
	Code   string // Key Vault's error code (innermost), e.g. SecretNotFound
}

func (e *StatusError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("keyvault: HTTP %d", e.Status)
	}
	return fmt.Sprintf("keyvault: HTTP %d %s", e.Status, e.Code)
}

var errName = errors.New("keyvault: invalid secret name")

// validName reports whether name matches ^[a-z0-9-]{1,127}$ (a subset of
// what Key Vault accepts, and safe in a URL path).
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

// errorCode returns the innermost error code of a Key Vault error body. A
// code that is not 1 to 64 letters, digits and dots is dropped, so nothing
// else of the body can leak through it.
func errorCode(body []byte) string {
	type inner struct {
		Code       string `json:"code"`
		InnerError *inner `json:"innererror"`
	}
	var e struct {
		Error *inner `json:"error"`
	}
	if json.Unmarshal(body, &e) != nil || e.Error == nil {
		return ""
	}
	code := e.Error.Code
	for in := e.Error.InnerError; in != nil; in = in.InnerError {
		if in.Code != "" {
			code = in.Code
		}
	}
	if len(code) > 64 {
		return ""
	}
	for i := 0; i < len(code); i++ {
		c := code[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.') {
			return ""
		}
	}
	return code
}

func (c *Client) do(ctx context.Context, method, name string, body []byte) ([]byte, error) {
	if !validName(name) {
		return nil, errName
	}
	return c.request(ctx, method, "/secrets/"+name, name, body)
}

// request calls path (below the vault, without the query) and names the
// call by label in errors.
func (c *Client) request(ctx context.Context, method, path, name string, body []byte) ([]byte, error) {
	tok, err := c.tokens.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("keyvault: token: %w", err)
	}
	u := c.base + path
	if !strings.Contains(path, "?") {
		u += "?api-version=" + APIVersion
	}
	var rd io.Reader = http.NoBody
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, errors.New("keyvault: bad request")
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("keyvault: %s %s: %w", method, name, azblob.RedactURLError(err))
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("keyvault: %s %s: reading response failed", method, name)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		se := &StatusError{Status: resp.StatusCode, Code: errorCode(data)}
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%w (%s %s: %v)", ports.ErrNotFound, method, name, se)
		}
		return nil, fmt.Errorf("keyvault: %s %s: %w", method, name, se)
	}
	return data, nil
}

// Get implements ports.SecretStore: the current version of the secret.
func (c *Client) Get(ctx context.Context, name string) (string, error) {
	data, err := c.do(ctx, http.MethodGet, name, nil)
	if err != nil {
		return "", err
	}
	var b struct {
		Value *string `json:"value"`
	}
	if json.Unmarshal(data, &b) != nil || b.Value == nil {
		return "", fmt.Errorf("keyvault: GET %s: bad response", name)
	}
	return *b.Value, nil
}

// Set implements ports.SecretStore: a new version of the secret. Setting a
// name that is deleted but still in soft delete fails with HTTP 409; callers
// never reuse a deleted name (ADR 0010).
func (c *Client) Set(ctx context.Context, name, value string) error {
	body, err := json.Marshal(struct {
		Value string `json:"value"`
	}{value})
	if err != nil {
		return errors.New("keyvault: encoding failed")
	}
	_, err = c.do(ctx, http.MethodPut, name, body)
	return err
}

// Delete implements ports.SecretStore: the secret moves to soft delete, and
// is then purged, so a deleted token cannot be recovered from the vault.
// The purge is tried a few times (Key Vault answers 409 while it is still
// deleting); if it does not go through, the secret stays in soft delete
// and PurgeDeleted, run daily, removes it later.
func (c *Client) Delete(ctx context.Context, name string) error {
	if _, err := c.do(ctx, http.MethodDelete, name, nil); err != nil {
		return err
	}
	for i := 0; i < c.PurgeTries; i++ {
		err := c.purge(ctx, name)
		var se *StatusError
		if err == nil || errors.Is(err, ports.ErrNotFound) || !errors.As(err, &se) || se.Status != http.StatusConflict {
			break
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(c.PurgeDelay):
		}
	}
	return nil
}

func (c *Client) purge(ctx context.Context, name string) error {
	_, err := c.request(ctx, http.MethodDelete, "/deletedsecrets/"+name, name, nil)
	return err
}

// list pages through a collection ("/secrets" or "/deletedsecrets") and
// returns the names that start with prefix.
func (c *Client) list(ctx context.Context, collection, prefix string) ([]ports.SecretInfo, error) {
	out := []ports.SecretInfo{}
	path := collection + "?api-version=" + APIVersion + "&maxresults=25"
	for range 200 {
		data, err := c.request(ctx, http.MethodGet, path, strings.TrimPrefix(collection, "/"), nil)
		if err != nil {
			return nil, err
		}
		var page struct {
			Value []struct {
				ID         string `json:"id"`
				Attributes struct {
					Created int64 `json:"created"`
				} `json:"attributes"`
			} `json:"value"`
			NextLink string `json:"nextLink"`
		}
		if json.Unmarshal(data, &page) != nil {
			return nil, fmt.Errorf("keyvault: list %s: bad response", collection)
		}
		for _, v := range page.Value {
			i := strings.LastIndex(v.ID, "/")
			name := v.ID[i+1:]
			if !validName(name) || !strings.HasPrefix(name, prefix) {
				continue
			}
			out = append(out, ports.SecretInfo{Name: name, Created: time.Unix(v.Attributes.Created, 0).UTC()})
		}
		if page.NextLink == "" {
			return out, nil
		}
		// Follow only links on this vault, so the bearer token goes nowhere else.
		next, ok := strings.CutPrefix(page.NextLink, c.base+collection+"?")
		if !ok {
			return nil, fmt.Errorf("keyvault: list %s: next link off the vault", collection)
		}
		path = collection + "?" + next
	}
	return out, nil
}

// List implements ports.SecretStore.
func (c *Client) List(ctx context.Context, prefix string) ([]ports.SecretInfo, error) {
	return c.list(ctx, "/secrets", prefix)
}

// PurgeDeleted implements ports.SecretStore.
func (c *Client) PurgeDeleted(ctx context.Context, prefix string) (int, error) {
	deleted, err := c.list(ctx, "/deletedsecrets", prefix)
	if err != nil {
		return 0, err
	}
	n := 0
	var errs []error
	for _, d := range deleted {
		if err := c.purge(ctx, d.Name); err != nil && !errors.Is(err, ports.ErrNotFound) {
			errs = append(errs, err)
			continue
		}
		n++
	}
	return n, errors.Join(errs...)
}
