package google

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type fake struct {
	srv   *httptest.Server
	key   *rsa.PrivateKey
	token string
	form  url.Values
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func (f *fake) sign(t *testing.T, alg string, claims map[string]any) string {
	t.Helper()
	h, _ := json.Marshal(map[string]string{"alg": alg, "kid": "k1", "typ": "JWT"})
	c, _ := json.Marshal(claims)
	in := b64(h) + "." + b64(c)
	sum := sha256.Sum256([]byte(in))
	sig, err := rsa.SignPKCS1v15(rand.Reader, f.key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return in + "." + b64(sig)
}

func newFake(t *testing.T) *fake {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fake{key: key}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer": f.srv.URL, "authorization_endpoint": f.srv.URL + "/auth",
			"token_endpoint": f.srv.URL + "/token", "jwks_uri": f.srv.URL + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": "k1", "use": "sig", "alg": "RS256",
			"n": b64(key.N.Bytes()), "e": b64(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.form = r.PostForm
		_ = json.NewEncoder(w).Encode(map[string]string{"id_token": f.token})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func claims(iss string, now time.Time) map[string]any {
	return map[string]any{
		"iss": iss, "sub": "123", "aud": "client", "exp": now.Add(time.Hour).Unix(), "iat": now.Unix(),
		"nonce": "n1", "email": "ola@example.pl", "email_verified": true, "name": "Ola",
	}
}

func TestFlow(t *testing.T) {
	f := newFake(t)
	p := &Provider{Issuer: f.srv.URL, ClientID: "client", ClientSecret: "secret"}
	u, err := p.AuthCodeURL(context.Background(), "s1", "n1", "chal", "https://cms/auth/callback")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"code_challenge=chal", "code_challenge_method=S256", "nonce=n1", "state=s1", "scope=openid+email+profile"} {
		if !strings.Contains(u, want) {
			t.Errorf("auth url lacks %s: %s", want, u)
		}
	}
	f.token = f.sign(t, "RS256", claims(f.srv.URL, time.Now()))
	id, err := p.Exchange(context.Background(), "code1", "verifier1", "https://cms/auth/callback", "n1")
	if err != nil {
		t.Fatal(err)
	}
	if id.Subject != "123" || id.Email != "ola@example.pl" {
		t.Fatalf("%+v", id)
	}
	if f.form.Get("code_verifier") != "verifier1" || f.form.Get("client_secret") != "secret" {
		t.Fatalf("token request: %v", f.form)
	}
}

func TestRejections(t *testing.T) {
	f := newFake(t)
	p := &Provider{Issuer: f.srv.URL, ClientID: "client"}
	now := time.Now()
	mod := func(k string, v any) map[string]any {
		c := claims(f.srv.URL, now)
		if v == nil {
			delete(c, k)
		} else {
			c[k] = v
		}
		return c
	}
	cases := map[string]string{
		"wrong issuer":     f.sign(t, "RS256", mod("iss", "https://evil")),
		"wrong audience":   f.sign(t, "RS256", mod("aud", "other")),
		"expired":          f.sign(t, "RS256", mod("exp", now.Add(-time.Hour).Unix())),
		"future iat":       f.sign(t, "RS256", mod("iat", now.Add(time.Hour).Unix())),
		"wrong nonce":      f.sign(t, "RS256", mod("nonce", "other")),
		"unverified":       f.sign(t, "RS256", mod("email_verified", false)),
		"alg none":         f.sign(t, "none", claims(f.srv.URL, now)),
		"multi aud no azp": f.sign(t, "RS256", mod("aud", []string{"client", "x"})),
	}
	good := f.sign(t, "RS256", claims(f.srv.URL, now))
	parts := strings.Split(good, ".")
	tampered := claims(f.srv.URL, now)
	tampered["sub"] = "999"
	tb, _ := json.Marshal(tampered)
	cases["tampered payload"] = parts[0] + "." + b64(tb) + "." + parts[2]
	for name, tok := range cases {
		if _, err := p.Verify(context.Background(), tok, "n1"); err == nil {
			t.Errorf("%s: want rejection", name)
		}
	}
	if _, err := p.Verify(context.Background(), good, "n1"); err != nil {
		t.Fatalf("good token: %v", err)
	}
}
