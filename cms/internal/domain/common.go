// Package domain holds the folk-ms domain model: Events (synced from Facebook
// or Manual), their Overrides and CMS-only fields, access rules, the Social
// Account Connection with Autosync, Social Post records, the snapshot builder
// and the Caption generator. It does no I/O.
package domain

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"unicode"
)

// Lang is a content language.
type Lang string

const (
	PL Lang = "pl"
	EN Lang = "en"
)

// Text is bilingual text. Polish is the authored language; English is optional
// and Sites fall back to Polish where it is missing (Translation fallback).
type Text struct {
	PL string `json:"pl"`
	EN string `json:"en,omitempty"`
}

// In returns the text in lang with the Translation fallback applied.
func (t Text) In(l Lang) string {
	if l == EN && t.EN != "" {
		return t.EN
	}
	return t.PL
}

// IsZero reports whether both languages are empty.
func (t Text) IsZero() bool { return t.PL == "" && t.EN == "" }

func (t Text) trimmed() Text {
	return Text{PL: strings.TrimSpace(t.PL), EN: strings.TrimSpace(t.EN)}
}

// Common errors. The application layer maps them to HTTP statuses.
var (
	ErrNotFound      = errors.New("not found")
	ErrConflict      = errors.New("conflict")
	ErrForbidden     = errors.New("forbidden")
	ErrInvalidState  = errors.New("invalid state")
	ErrNeedsConfirm  = errors.New("confirmation required")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrTooManyEvents = errors.New("too many")
	// ErrTooSoon: the action ran moments ago (Sync now).
	ErrTooSoon = errors.New("too soon")
	// ErrRecentSignIn: the action needs a CMS sign-in in the last 15 minutes.
	ErrRecentSignIn = errors.New("recent sign-in required")
	// ErrLimit: a daily limit is used up.
	ErrLimit = errors.New("limit reached")
)

// FieldError is one validation problem. Its Message is shown to Moderators
// in the admin UI language, so it always has both Polish and English.
type FieldError struct {
	Field   string `json:"field"`
	Message Text   `json:"message"`
}

// Problem returns a ValidationError with one FieldError.
func Problem(field, pl, en string) *ValidationError {
	return &ValidationError{Problems: []FieldError{{Field: field, Message: Text{PL: pl, EN: en}}}}
}

// ValidationError collects FieldErrors.
type ValidationError struct {
	Problems []FieldError `json:"problems"`
}

func (e *ValidationError) Error() string {
	parts := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		parts[i] = p.Field + ": " + p.Message.EN
	}
	return "validation: " + strings.Join(parts, "; ")
}

type problems []FieldError

// add appends a problem; pl and en are format strings sharing args.
func (p *problems) add(field, pl, en string, args ...any) {
	*p = append(*p, FieldError{Field: field, Message: Text{PL: fmt.Sprintf(pl, args...), EN: fmt.Sprintf(en, args...)}})
}

func (p problems) err() error {
	if len(p) == 0 {
		return nil
	}
	sort.SliceStable(p, func(i, j int) bool { return p[i].Field < p[j].Field })
	return &ValidationError{Problems: p}
}

// IsValidation reports whether err is a *ValidationError.
func IsValidation(err error) bool {
	var v *ValidationError
	return errors.As(err, &v)
}

// NewID returns a random 16-character lowercase hex identifier.
func NewID(r io.Reader) (string, error) {
	if r == nil {
		r = rand.Reader
	}
	b := make([]byte, 8)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

var shortIDEncoding = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

// NewShortID returns an 8-character lowercase base32 identifier (40 bits).
// The caller checks it for collisions within the Organization.
func NewShortID(r io.Reader) (string, error) {
	if r == nil {
		r = rand.Reader
	}
	b := make([]byte, 5)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}
	return shortIDEncoding.EncodeToString(b), nil
}

// IsShortID reports whether s looks like a shortId.
func IsShortID(s string) bool {
	if len(s) != 8 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= '2' && c <= '7') {
			return false
		}
	}
	return true
}

var polishFold = strings.NewReplacer(
	"ą", "a", "ć", "c", "ę", "e", "ł", "l", "ń", "n", "ó", "o", "ś", "s", "ź", "z", "ż", "z",
	"Ą", "a", "Ć", "c", "Ę", "e", "Ł", "l", "Ń", "n", "Ó", "o", "Ś", "s", "Ź", "z", "Ż", "z",
)

// MaxSlugLen is the longest slug allowed.
const MaxSlugLen = 80

// Slugify turns a title into a URL slug: lowercase ASCII letters, digits and
// single hyphens.
func Slugify(s string) string {
	s = polishFold.Replace(s)
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case unicode.IsLetter(r):
			// Other letters: drop diacritics we do not know by skipping.
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > MaxSlugLen {
		out = strings.TrimRight(out[:MaxSlugLen], "-")
	}
	return out
}

// IsSlug reports whether s is a valid slug.
func IsSlug(s string) bool {
	if s == "" || len(s) > MaxSlugLen || s[0] == '-' || s[len(s)-1] == '-' || strings.Contains(s, "--") {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

// IsHTTPSURL reports whether s is an absolute https URL without spaces,
// quotes or angle brackets, at most 2048 characters long.
func IsHTTPSURL(s string) bool {
	if len(s) > 2048 || !strings.HasPrefix(s, "https://") || strings.ContainsAny(s, " \t\r\n\"'<>\\") {
		return false
	}
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil
}

// cleanText makes untrusted text plain: valid UTF-8, no control characters
// except newlines and tabs, \r\n folded to \n, trimmed, and cut to max runes.
func cleanText(s string, max int) string {
	s = strings.ToValidUTF8(s, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case r == '\r':
			return '\n'
		case unicode.IsControl(r), r == '\u2028', r == '\u2029', r >= '\u202a' && r <= '\u202e', r >= '\u2066' && r <= '\u2069':
			return -1 // controls and bidi overrides
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > max {
		s = strings.TrimSpace(string(r[:max]))
	}
	return s
}

// CleanText is cleanText for other packages: Facebook text is untrusted (T7).
func CleanText(s string, max int) string { return cleanText(s, max) }

// FacebookEventURL is the canonical address of a Facebook Event.
func FacebookEventURL(id string) string { return "https://www.facebook.com/events/" + id + "/" }

// ParseFacebookEventLink accepts the address of a Facebook Event
// (facebook.com, www., m. or web. host; https) or a bare numeric id, and
// returns the numeric id. Nothing else is accepted (T9): the CMS never
// fetches the address.
func ParseFacebookEventLink(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if isDigits(s) {
		return s, len(s) <= 30
	}
	if !strings.HasPrefix(s, "https://") && !strings.HasPrefix(s, "http://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.User != nil || u.Port() != "" {
		return "", false
	}
	switch strings.ToLower(u.Host) {
	case "facebook.com", "www.facebook.com", "m.facebook.com", "web.facebook.com", "mbasic.facebook.com":
	default:
		return "", false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "events" || !isDigits(parts[1]) || len(parts[1]) > 30 {
		return "", false
	}
	return parts[1], true
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
