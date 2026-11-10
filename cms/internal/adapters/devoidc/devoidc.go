//go:build dev

// Package devoidc is a local-only stand-in for Google sign-in: a page where
// you type any email. It is compiled only with the "dev" build tag, so a
// production binary cannot contain it.
package devoidc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// Issuer identifies dev identities; they never collide with Google ones.
const Issuer = "folk-ms-devoidc"

type grant struct {
	email, nonce, challenge string
	at                      time.Time
}

// Provider implements ports.IdentityProvider and serves the login page.
type Provider struct {
	// LoginPath is where Handler is mounted, e.g. /auth/dev/login.
	LoginPath string

	mu    sync.Mutex
	codes map[string]grant
}

// New returns a dev provider.
func New(loginPath string) *Provider {
	return &Provider{LoginPath: loginPath, codes: map[string]grant{}}
}

// AuthCodeURL implements ports.IdentityProvider.
func (p *Provider) AuthCodeURL(_ context.Context, state, nonce, challenge, redirectURI string) (string, error) {
	q := url.Values{"state": {state}, "nonce": {nonce}, "code_challenge": {challenge}, "redirect_uri": {redirectURI}}
	return p.LoginPath + "?" + q.Encode(), nil
}

// Exchange implements ports.IdentityProvider.
func (p *Provider) Exchange(_ context.Context, code, verifier, _, nonce string) (ports.Identity, error) {
	p.mu.Lock()
	g, ok := p.codes[code]
	delete(p.codes, code)
	p.mu.Unlock()
	if !ok || time.Since(g.at) > 5*time.Minute {
		return ports.Identity{}, errors.New("devoidc: unknown code")
	}
	sum := sha256.Sum256([]byte(verifier))
	if base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge {
		return ports.Identity{}, errors.New("devoidc: PKCE mismatch")
	}
	if g.nonce != nonce {
		return ports.Identity{}, errors.New("devoidc: nonce mismatch")
	}
	name, _, _ := strings.Cut(g.email, "@")
	return ports.Identity{Issuer: Issuer, Subject: "dev:" + g.email, Email: g.email, EmailVerified: true, Name: name}, nil
}

// pageText is the login page copy in one UI language.
type pageText struct{ Lang, Title, Note, Email, Submit string }

var (
	textPL = pageText{"pl", "Logowanie deweloperskie", "Tylko lokalnie. Wpisz dowolny e-mail – zostanie potraktowany jak zweryfikowane konto Google.", "E-mail", "Zaloguj"}
	textEN = pageText{"en", "Developer sign-in", "Local only. Type any email – it is treated as a verified Google account.", "Email", "Sign in"}
)

var page = template.Must(template.New("p").Parse(`<!doctype html>
<html lang="{{.T.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>{{.T.Title}}</title><link rel="stylesheet" href="/app/app.css"></head>
<body class="dev-login"><main class="card narrow">
<h1>{{.T.Title}}</h1>
<p class="muted">{{.T.Note}}</p>
<form method="post">
<input type="hidden" name="state" value="{{.State}}"><input type="hidden" name="nonce" value="{{.Nonce}}">
<input type="hidden" name="code_challenge" value="{{.Challenge}}"><input type="hidden" name="redirect_uri" value="{{.Redirect}}">
<div class="field"><label>{{.T.Email}} <input type="email" name="email" required autofocus value="{{.Email}}"></label></div>
<button class="primary" type="submit">{{.T.Submit}}</button>
</form></main></body></html>`))

// Handler serves the login page (GET) and issues a code (POST). polish
// reports whether a request's UI language is Polish (else English).
func (p *Provider) Handler(defaultEmail string, polish func(*http.Request) bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			q := r.URL.Query()
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			t := textEN
			if polish(r) {
				t = textPL
			}
			w.Header().Add("Vary", "Accept-Language, Cookie")
			_ = page.Execute(w, map[string]any{
				"T": t, "State": q.Get("state"), "Nonce": q.Get("nonce"), "Challenge": q.Get("code_challenge"),
				"Redirect": q.Get("redirect_uri"), "Email": defaultEmail,
			})
		case http.MethodPost:
			if err := r.ParseForm(); err != nil {
				http.Error(w, "bad form", http.StatusBadRequest)
				return
			}
			redirect := r.PostForm.Get("redirect_uri")
			if !strings.HasPrefix(redirect, "/") && !strings.HasPrefix(redirect, "http://localhost") && !strings.HasPrefix(redirect, "http://127.0.0.1") {
				http.Error(w, "bad redirect", http.StatusBadRequest)
				return
			}
			b := make([]byte, 16)
			_, _ = rand.Read(b)
			code := hex.EncodeToString(b)
			p.mu.Lock()
			p.codes[code] = grant{
				email: strings.ToLower(strings.TrimSpace(r.PostForm.Get("email"))), nonce: r.PostForm.Get("nonce"),
				challenge: r.PostForm.Get("code_challenge"), at: time.Now(),
			}
			p.mu.Unlock()
			u, err := url.Parse(redirect)
			if err != nil {
				http.Error(w, "bad redirect", http.StatusBadRequest)
				return
			}
			q := u.Query()
			q.Set("code", code)
			q.Set("state", r.PostForm.Get("state"))
			u.RawQuery = q.Encode()
			http.Redirect(w, r, u.String(), http.StatusSeeOther)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}
