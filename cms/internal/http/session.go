package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// SessionTTL is how long a session lasts.
const SessionTTL = 8 * time.Hour

// signer HMAC-signs small JSON payloads for cookies.
type signer struct{ key []byte }

func (s signer) mac(purpose string, payload []byte) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(purpose))
	m.Write([]byte{0})
	m.Write(payload)
	return m.Sum(nil)
}

// seal encodes v as payload.signature; purpose separates cookie kinds so one
// cannot be replayed as another.
func (s signer) seal(purpose string, v any) (string, error) {
	p, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding
	return enc.EncodeToString(p) + "." + enc.EncodeToString(s.mac(purpose, p)), nil
}

func (s signer) open(purpose, value string, v any) error {
	enc := base64.RawURLEncoding
	ps, ss, ok := strings.Cut(value, ".")
	if !ok {
		return errors.New("malformed")
	}
	p, err1 := enc.DecodeString(ps)
	sig, err2 := enc.DecodeString(ss)
	if err1 != nil || err2 != nil {
		return errors.New("malformed")
	}
	if !hmac.Equal(sig, s.mac(purpose, p)) {
		return errors.New("bad signature")
	}
	return json.Unmarshal(p, v)
}

type sessionData struct {
	UserID   string `json:"u"`
	IssuedAt int64  `json:"iat"`
	Expires  int64  `json:"exp"`
}

type oidcState struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
	Return   string `json:"r"`
	Expires  int64  `json:"exp"`
}

type metaState struct {
	State   string `json:"s"`
	Org     string `json:"o"`
	UserID  string `json:"u"`
	Expires int64  `json:"exp"`
}

func (s *Server) cookieName(base string) string {
	if s.Secure {
		return "__Host-" + base
	}
	return base
}

func (s *Server) setCookie(w http.ResponseWriter, base, value string, ttl time.Duration) {
	c := &http.Cookie{
		Name: s.cookieName(base), Value: value, Path: "/", HttpOnly: true, Secure: s.Secure,
		SameSite: http.SameSiteLaxMode, MaxAge: int(ttl.Seconds()),
	}
	if ttl < 0 {
		c.MaxAge = -1
	}
	http.SetCookie(w, c)
}

func (s *Server) readCookie(r *http.Request, base, purpose string, v any) error {
	c, err := r.Cookie(s.cookieName(base))
	if err != nil {
		return err
	}
	return s.sign.open(purpose, c.Value, v)
}

const (
	sessionCookie = "folkms"
	oidcCookie    = "folkms-oidc"
	metaCookie    = "folkms-meta"
)
