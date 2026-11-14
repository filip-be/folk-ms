// Package vimeo reads a Vimeo video's thumbnail through Vimeo's oEmbed
// endpoint (https://vimeo.com/api/oembed.json). It needs no key.
package vimeo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// MaxBody bounds the oEmbed answer read.
const MaxBody = 64 << 10

// Client calls Vimeo oEmbed.
type Client struct {
	HTTP *http.Client
	// Endpoint is the oEmbed endpoint (tests point it elsewhere).
	Endpoint string
}

// New returns a Client with a 5 second timeout.
func New() *Client {
	return &Client{HTTP: &http.Client{Timeout: 5 * time.Second}, Endpoint: "https://vimeo.com/api/oembed.json"}
}

// ThumbnailHost is the only host a thumbnail may come from (the admin CSP
// allows it).
const ThumbnailHost = "i.vimeocdn.com"

// Vimeo implements ports.VideoInfo.
func (c *Client) Vimeo(ctx context.Context, id, hash string) (string, error) {
	page := "https://vimeo.com/" + url.PathEscape(id)
	if hash != "" {
		page += "/" + url.PathEscape(hash)
	}
	u := c.Endpoint + "?" + url.Values{"url": {page}, "width": {"640"}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("vimeo oembed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBody+1))
	if err != nil {
		return "", fmt.Errorf("vimeo oembed: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized:
		// 404: no such video; 403: private, or embedding is not allowed.
		return "", ports.ErrVideoUnavailable
	case resp.StatusCode != http.StatusOK:
		return "", fmt.Errorf("vimeo oembed: HTTP %d", resp.StatusCode)
	case len(body) > MaxBody:
		return "", fmt.Errorf("vimeo oembed: answer too large")
	}
	var o struct {
		ThumbnailURL string `json:"thumbnail_url"`
	}
	if err := json.Unmarshal(body, &o); err != nil {
		return "", fmt.Errorf("vimeo oembed: %w", err)
	}
	t, err := url.Parse(o.ThumbnailURL)
	if err != nil || t.Scheme != "https" || t.Host != ThumbnailHost || t.User != nil || len(o.ThumbnailURL) > 2048 {
		return "", fmt.Errorf("vimeo oembed: unexpected thumbnail %q", o.ThumbnailURL)
	}
	return t.String(), nil
}
