// Package google implements ports.IdentityProvider for Google OIDC: the
// authorization code flow with PKCE, nonce and state, and hand-written ID
// token verification (RS256 against the cached JWKS; iss, aud, exp, iat,
// nonce, email_verified).
package google

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// GoogleIssuer is Google's issuer.
const GoogleIssuer = "https://accounts.google.com"

// Provider is the Google OIDC client.
type Provider struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	HTTP         *http.Client
	Now          func() time.Time

	mu        sync.Mutex
	disc      *discovery
	discAt    time.Time
	keys      map[string]*rsa.PublicKey
	keysAt    time.Time
	keysTried time.Time
}

type discovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

// New returns a provider for Google.
func New(clientID, clientSecret string) *Provider {
	return &Provider{Issuer: GoogleIssuer, ClientID: clientID, ClientSecret: clientSecret}
}

func (p *Provider) client() *http.Client {
	if p.HTTP != nil {
		return p.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (p *Provider) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p *Provider) getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := p.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("google: GET %s: HTTP %d", u, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}

func (p *Provider) discovery(ctx context.Context) (*discovery, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.disc != nil && time.Since(p.discAt) < 24*time.Hour {
		return p.disc, nil
	}
	var d discovery
	if err := p.getJSON(ctx, strings.TrimRight(p.Issuer, "/")+"/.well-known/openid-configuration", &d); err != nil {
		return nil, err
	}
	if d.Issuer != p.Issuer || d.AuthorizationEndpoint == "" || d.TokenEndpoint == "" || d.JWKSURI == "" {
		return nil, errors.New("google: bad discovery document")
	}
	p.disc, p.discAt = &d, time.Now()
	return p.disc, nil
}

// AuthCodeURL implements ports.IdentityProvider.
func (p *Provider) AuthCodeURL(ctx context.Context, state, nonce, challenge, redirectURI string) (string, error) {
	d, err := p.discovery(ctx)
	if err != nil {
		return "", err
	}
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {p.ClientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {"openid email profile"},
		"state":                 {state},
		"nonce":                 {nonce},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"prompt":                {"select_account"},
	}
	return d.AuthorizationEndpoint + "?" + q.Encode(), nil
}

// Exchange implements ports.IdentityProvider.
func (p *Provider) Exchange(ctx context.Context, code, verifier, redirectURI, nonce string) (ports.Identity, error) {
	d, err := p.discovery(ctx)
	if err != nil {
		return ports.Identity{}, err
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {p.ClientID},
		"client_secret": {p.ClientSecret},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return ports.Identity{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.client().Do(req)
	if err != nil {
		return ports.Identity{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return ports.Identity{}, fmt.Errorf("google: token endpoint: HTTP %d", resp.StatusCode)
	}
	var tr struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &tr); err != nil || tr.IDToken == "" {
		return ports.Identity{}, errors.New("google: no id_token")
	}
	return p.Verify(ctx, tr.IDToken, nonce)
}

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func (p *Provider) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	p.mu.Lock()
	k, ok := p.keys[kid]
	fresh := time.Since(p.keysAt) < 6*time.Hour
	recentlyTried := time.Since(p.keysTried) < time.Minute
	p.mu.Unlock()
	if ok && fresh {
		return k, nil
	}
	if recentlyTried && ok {
		return k, nil
	}
	if recentlyTried {
		return nil, errors.New("google: unknown signing key")
	}
	d, err := p.discovery(ctx)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.keysTried = time.Now()
	p.mu.Unlock()
	var set struct {
		Keys []jwk `json:"keys"`
	}
	if err := p.getJSON(ctx, d.JWKSURI, &set); err != nil {
		return nil, err
	}
	keys := map[string]*rsa.PublicKey{}
	for _, j := range set.Keys {
		if j.Kty != "RSA" || (j.Use != "" && j.Use != "sig") {
			continue
		}
		nb, err1 := base64.RawURLEncoding.DecodeString(j.N)
		eb, err2 := base64.RawURLEncoding.DecodeString(j.E)
		if err1 != nil || err2 != nil || len(eb) > 4 {
			continue
		}
		e := 0
		for _, b := range eb {
			e = e<<8 | int(b)
		}
		pk := &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: e}
		if pk.N.BitLen() < 2048 {
			continue
		}
		keys[j.Kid] = pk
	}
	p.mu.Lock()
	p.keys, p.keysAt = keys, time.Now()
	p.mu.Unlock()
	if k, ok := keys[kid]; ok {
		return k, nil
	}
	return nil, errors.New("google: unknown signing key")
}

// Verify checks an ID token and returns the identity.
func (p *Provider) Verify(ctx context.Context, token, nonce string) (ports.Identity, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ports.Identity{}, errors.New("google: malformed token")
	}
	hb, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return ports.Identity{}, errors.New("google: malformed header")
	}
	var hdr struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(hb, &hdr); err != nil {
		return ports.Identity{}, errors.New("google: malformed header")
	}
	if hdr.Alg != "RS256" {
		return ports.Identity{}, fmt.Errorf("google: unexpected alg %q", hdr.Alg)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return ports.Identity{}, errors.New("google: malformed signature")
	}
	key, err := p.key(ctx, hdr.Kid)
	if err != nil {
		return ports.Identity{}, err
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sig); err != nil {
		return ports.Identity{}, errors.New("google: bad signature")
	}
	pb, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ports.Identity{}, errors.New("google: malformed payload")
	}
	var c struct {
		Iss           string          `json:"iss"`
		Sub           string          `json:"sub"`
		Aud           json.RawMessage `json:"aud"`
		Azp           string          `json:"azp"`
		Exp           int64           `json:"exp"`
		Iat           int64           `json:"iat"`
		Nonce         string          `json:"nonce"`
		Email         string          `json:"email"`
		EmailVerified any             `json:"email_verified"`
		Name          string          `json:"name"`
	}
	if err := json.Unmarshal(pb, &c); err != nil {
		return ports.Identity{}, errors.New("google: malformed claims")
	}
	issuers := []string{p.Issuer}
	if p.Issuer == GoogleIssuer {
		issuers = append(issuers, "accounts.google.com")
	}
	if !slices.Contains(issuers, c.Iss) {
		return ports.Identity{}, errors.New("google: wrong issuer")
	}
	var auds []string
	var one string
	if json.Unmarshal(c.Aud, &one) == nil {
		auds = []string{one}
	} else if json.Unmarshal(c.Aud, &auds) != nil {
		return ports.Identity{}, errors.New("google: malformed aud")
	}
	if !slices.Contains(auds, p.ClientID) || (len(auds) > 1 && c.Azp != p.ClientID) {
		return ports.Identity{}, errors.New("google: wrong audience")
	}
	now := p.now().Unix()
	const skew = 60
	if c.Exp == 0 || now > c.Exp+skew {
		return ports.Identity{}, errors.New("google: token expired")
	}
	if c.Iat > now+skew {
		return ports.Identity{}, errors.New("google: token issued in the future")
	}
	if nonce == "" || c.Nonce != nonce {
		return ports.Identity{}, errors.New("google: nonce mismatch")
	}
	verified := c.EmailVerified == true || c.EmailVerified == "true"
	if c.Sub == "" || c.Email == "" || !verified {
		return ports.Identity{}, errors.New("google: email not verified")
	}
	return ports.Identity{Issuer: c.Iss, Subject: c.Sub, Email: c.Email, EmailVerified: verified, Name: c.Name}, nil
}
