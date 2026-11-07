// Package azblob implements the store ports on Azure Blob Storage through its
// REST API (no SDK). It authenticates with a SharedKey (Azurite, local) or a
// managed identity token (Azure).
package azblob

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// APIVersion is the Blob service version the adapter speaks.
const APIVersion = "2023-11-03"

// maxBody bounds what the adapter reads from a response.
const maxBody = 64 << 20

// Auth signs requests.
type Auth interface {
	Sign(ctx context.Context, req *http.Request, contentLength int64) error
}

// Client is a Blob service client for one account.
type Client struct {
	// BaseURL is the account endpoint, e.g. https://acct.blob.core.windows.net
	// or http://127.0.0.1:10000/devstoreaccount1 for Azurite.
	BaseURL string
	Auth    Auth
	HTTP    *http.Client
}

// NewClient builds a client.
func NewClient(baseURL string, auth Auth) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Auth: auth, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// StatusError is a non-2xx answer from the service.
type StatusError struct {
	Status int
	Code   string
}

func (e *StatusError) Error() string { return fmt.Sprintf("azblob: HTTP %d %s", e.Status, e.Code) }

func blobPath(container, name string) string {
	parts := strings.Split(name, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return "/" + container + "/" + strings.Join(parts, "/")
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, hdr http.Header, body []byte) (*http.Response, []byte, error) {
	u := c.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	for k, v := range hdr {
		req.Header[k] = v
	}
	req.Header.Set("x-ms-version", APIVersion)
	req.Header.Set("x-ms-date", time.Now().UTC().Format(http.TimeFormat))
	req.ContentLength = int64(len(body))
	if body == nil {
		req.Body = http.NoBody
	}
	if err := c.Auth.Sign(ctx, req, int64(len(body))); err != nil {
		return nil, nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode >= 300 {
		return resp, data, &StatusError{Status: resp.StatusCode, Code: resp.Header.Get("x-ms-error-code")}
	}
	return resp, data, nil
}

func mapErr(err error) error {
	var se *StatusError
	if errors.As(err, &se) {
		switch {
		case se.Status == http.StatusNotFound:
			return fmt.Errorf("%w (%s)", ports.ErrNotFound, se.Code)
		case se.Status == http.StatusPreconditionFailed,
			se.Status == http.StatusConflict && se.Code == "BlobAlreadyExists":
			return fmt.Errorf("%w (%s)", ports.ErrPrecondition, se.Code)
		}
	}
	return err
}

func condHeaders(h http.Header, etag string, createOnly bool) {
	switch {
	case etag == "" && createOnly:
		h.Set("If-None-Match", "*")
	case etag != "" && etag != ports.AnyETag:
		h.Set("If-Match", etag)
	}
}

// CreateContainer creates a container if it does not exist. publicBlob
// grants anonymous read of blobs (for the pub container).
func (c *Client) CreateContainer(ctx context.Context, name string, publicBlob bool) error {
	h := http.Header{}
	if publicBlob {
		h.Set("x-ms-blob-public-access", "blob")
	}
	_, _, err := c.do(ctx, http.MethodPut, "/"+name, url.Values{"restype": {"container"}}, h, nil)
	var se *StatusError
	if errors.As(err, &se) && se.Status == http.StatusConflict {
		return nil
	}
	return err
}

// DeleteContainer removes a container (tests).
func (c *Client) DeleteContainer(ctx context.Context, name string) error {
	_, _, err := c.do(ctx, http.MethodDelete, "/"+name, url.Values{"restype": {"container"}}, nil, nil)
	return err
}

func (c *Client) get(ctx context.Context, container, name string) (ports.Document, error) {
	resp, data, err := c.do(ctx, http.MethodGet, blobPath(container, name), nil, nil, nil)
	if err != nil {
		return ports.Document{}, mapErr(err)
	}
	return ports.Document{Key: name, Data: data, ETag: resp.Header.Get("ETag")}, nil
}

func (c *Client) put(ctx context.Context, container, name string, data []byte, contentType, cacheControl, etag string) (string, error) {
	h := http.Header{}
	h.Set("x-ms-blob-type", "BlockBlob")
	h.Set("Content-Type", contentType)
	h.Set("x-ms-blob-content-type", contentType)
	if cacheControl != "" {
		h.Set("x-ms-blob-cache-control", cacheControl)
	}
	condHeaders(h, etag, true)
	resp, _, err := c.do(ctx, http.MethodPut, blobPath(container, name), nil, h, data)
	if err != nil {
		return "", mapErr(err)
	}
	return resp.Header.Get("ETag"), nil
}

type listResult struct {
	Blobs struct {
		Blob []struct {
			Name string `xml:"Name"`
		} `xml:"Blob"`
	} `xml:"Blobs"`
	NextMarker string `xml:"NextMarker"`
}

func (c *Client) list(ctx context.Context, container, prefix string) ([]string, error) {
	var names []string
	marker := ""
	for {
		q := url.Values{"restype": {"container"}, "comp": {"list"}, "prefix": {prefix}, "maxresults": {"5000"}}
		if marker != "" {
			q.Set("marker", marker)
		}
		_, data, err := c.do(ctx, http.MethodGet, "/"+container, q, nil, nil)
		if err != nil {
			return nil, mapErr(err)
		}
		var lr listResult
		if err := xml.Unmarshal(data, &lr); err != nil {
			return nil, fmt.Errorf("azblob: list: %w", err)
		}
		for _, b := range lr.Blobs.Blob {
			names = append(names, b.Name)
		}
		if lr.NextMarker == "" {
			break
		}
		marker = lr.NextMarker
	}
	sort.Strings(names)
	return names, nil
}

func (c *Client) delete(ctx context.Context, container, name, etag string) error {
	h := http.Header{}
	condHeaders(h, etag, false)
	_, _, err := c.do(ctx, http.MethodDelete, blobPath(container, name), nil, h, nil)
	return mapErr(err)
}

// DocumentStore keeps documents in a private container under a prefix.
type DocumentStore struct {
	c         *Client
	container string
	prefix    string
}

// NewDocumentStore returns a store in container, with keys under prefix
// (may be empty).
func NewDocumentStore(c *Client, container, prefix string) *DocumentStore {
	return &DocumentStore{c: c, container: container, prefix: prefix}
}

func (s *DocumentStore) Get(ctx context.Context, key string) (ports.Document, error) {
	d, err := s.c.get(ctx, s.container, s.prefix+key)
	d.Key = key
	return d, err
}

func (s *DocumentStore) Put(ctx context.Context, key string, data []byte, etag string) (string, error) {
	return s.c.put(ctx, s.container, s.prefix+key, data, "application/json", "", etag)
}

func (s *DocumentStore) List(ctx context.Context, prefix string) ([]string, error) {
	names, err := s.c.list(ctx, s.container, s.prefix+prefix)
	for i, n := range names {
		names[i] = strings.TrimPrefix(n, s.prefix)
	}
	return names, err
}

func (s *DocumentStore) Delete(ctx context.Context, key, etag string) error {
	return s.c.delete(ctx, s.container, s.prefix+key, etag)
}

// PublicStore writes to a public-read container.
type PublicStore struct {
	c         *Client
	container string
	publicURL string
}

// NewPublicStore returns a public store; publicURL is how visitors reach the
// container (account endpoint + container, or a CDN).
func NewPublicStore(c *Client, container, publicURL string) *PublicStore {
	return &PublicStore{c: c, container: container, publicURL: strings.TrimRight(publicURL, "/")}
}

func (s *PublicStore) Put(ctx context.Context, path string, data []byte, contentType, cacheControl, etag string) (string, error) {
	return s.c.put(ctx, s.container, path, data, contentType, cacheControl, etag)
}

func (s *PublicStore) Get(ctx context.Context, path string) (ports.Document, error) {
	return s.c.get(ctx, s.container, path)
}

func (s *PublicStore) URL(path string) string { return s.publicURL + "/" + path }

// AppendLog keeps one append blob per segment.
type AppendLog struct {
	c         *Client
	container string
	prefix    string
}

// NewAppendLog returns a log whose segments are <prefix><segment>.log.
func NewAppendLog(c *Client, container, prefix string) *AppendLog {
	return &AppendLog{c: c, container: container, prefix: prefix}
}

func (l *AppendLog) name(segment string) string { return l.prefix + segment + ".log" }

func (l *AppendLog) Append(ctx context.Context, segment string, line []byte) error {
	p := blobPath(l.container, l.name(segment))
	appendBlock := func() error {
		_, _, err := l.c.do(ctx, http.MethodPut, p, url.Values{"comp": {"appendblock"}}, nil, line)
		return err
	}
	err := appendBlock()
	var se *StatusError
	if errors.As(err, &se) && se.Status == http.StatusNotFound {
		h := http.Header{}
		h.Set("x-ms-blob-type", "AppendBlob")
		h.Set("x-ms-blob-content-type", "application/x-ndjson")
		h.Set("If-None-Match", "*")
		if _, _, err := l.c.do(ctx, http.MethodPut, p, nil, h, nil); err != nil {
			if !errors.As(err, &se) || se.Status != http.StatusConflict {
				return err
			}
		}
		err = appendBlock()
	}
	return err
}

func (l *AppendLog) Read(ctx context.Context, segment string) ([]byte, error) {
	d, err := l.c.get(ctx, l.container, l.name(segment))
	if errors.Is(err, ports.ErrNotFound) {
		return nil, nil
	}
	return d.Data, err
}

func (l *AppendLog) Segments(ctx context.Context) ([]string, error) {
	names, err := l.c.list(ctx, l.container, l.prefix)
	var out []string
	for _, n := range names {
		if s, ok := strings.CutSuffix(strings.TrimPrefix(n, l.prefix), ".log"); ok && !strings.Contains(s, "/") {
			out = append(out, s)
		}
	}
	return out, err
}

// SharedKey signs requests with the account key (Azurite or local only;
// Azure uses ManagedIdentity).
type SharedKey struct {
	Account string
	Key     []byte
}

// NewSharedKey decodes a base64 account key.
func NewSharedKey(account, keyB64 string) (*SharedKey, error) {
	k, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("azblob: account key: %w", err)
	}
	return &SharedKey{Account: account, Key: k}, nil
}

// Sign implements Auth.
func (s *SharedKey) Sign(_ context.Context, req *http.Request, contentLength int64) error {
	h := req.Header
	cl := ""
	if contentLength > 0 {
		cl = strconv.FormatInt(contentLength, 10)
	}
	var xms []string
	for k := range h {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "x-ms-") {
			xms = append(xms, lk)
		}
	}
	sort.Strings(xms)
	var canonHeaders strings.Builder
	for _, k := range xms {
		canonHeaders.WriteString(k + ":" + strings.TrimSpace(h.Get(k)) + "\n")
	}
	res := "/" + s.Account + req.URL.EscapedPath()
	q := req.URL.Query()
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := q[k]
		sort.Strings(v)
		res += "\n" + strings.ToLower(k) + ":" + strings.Join(v, ",")
	}
	toSign := strings.Join([]string{
		req.Method,
		h.Get("Content-Encoding"),
		h.Get("Content-Language"),
		cl,
		h.Get("Content-MD5"),
		h.Get("Content-Type"),
		"", // Date: x-ms-date is used
		h.Get("If-Modified-Since"),
		h.Get("If-Match"),
		h.Get("If-None-Match"),
		h.Get("If-Unmodified-Since"),
		h.Get("Range"),
	}, "\n") + "\n" + canonHeaders.String() + res
	mac := hmac.New(sha256.New, s.Key)
	mac.Write([]byte(toSign))
	req.Header.Set("Authorization", "SharedKey "+s.Account+":"+base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	return nil
}

// ManagedIdentity gets tokens from the App Service / Functions identity
// endpoint (IDENTITY_ENDPOINT and IDENTITY_HEADER).
type ManagedIdentity struct {
	Endpoint string
	Header   string
	ClientID string // optional, for a user-assigned identity
	HTTP     *http.Client

	mu      sync.Mutex
	token   string
	expires time.Time
}

// Sign implements Auth.
func (m *ManagedIdentity) Sign(ctx context.Context, req *http.Request, _ int64) error {
	tok, err := m.Token(ctx)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return nil
}

// Token returns a cached token for https://storage.azure.com/.
func (m *ManagedIdentity) Token(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.token != "" && time.Until(m.expires) > 5*time.Minute {
		return m.token, nil
	}
	q := url.Values{"resource": {"https://storage.azure.com/"}, "api-version": {"2019-08-01"}}
	if m.ClientID != "" {
		q.Set("client_id", m.ClientID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.Endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-IDENTITY-HEADER", m.Header)
	hc := m.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("azblob: identity endpoint: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("azblob: identity endpoint: HTTP %d", resp.StatusCode)
	}
	var t struct {
		AccessToken string `json:"access_token"`
		ExpiresOn   string `json:"expires_on"`
	}
	if err := json.Unmarshal(body, &t); err != nil || t.AccessToken == "" {
		return "", fmt.Errorf("azblob: identity endpoint: bad response")
	}
	exp := time.Now().Add(time.Hour)
	if sec, err := strconv.ParseInt(t.ExpiresOn, 10, 64); err == nil {
		exp = time.Unix(sec, 0)
	}
	m.token, m.expires = t.AccessToken, exp
	return m.token, nil
}
