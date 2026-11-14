package ports

import (
	"errors"
	"strings"
)

// ErrInvalidKey is returned by every store adapter for a key or prefix that
// could leave its namespace: an empty, "." or ".." segment, a leading or
// doubled slash, or a character outside [A-Za-z0-9-_.@/]. The check lives in
// the adapters, not only at the HTTP edge, so that no caller can address
// another Organization's documents or the public container (T13).
var ErrInvalidKey = errors.New("store: invalid key")

// ValidKey reports whether key is a safe document key, blob name or path.
func ValidKey(key string) bool {
	if key == "" || len(key) > 512 || strings.HasSuffix(key, "/") {
		return false
	}
	return ValidPrefix(key)
}

// ValidPrefix reports whether prefix is safe for List: like ValidKey, but it
// may be empty or end with a slash.
func ValidPrefix(prefix string) bool {
	if len(prefix) > 512 || strings.HasPrefix(prefix, "/") {
		return false
	}
	parts := strings.Split(prefix, "/")
	for i, part := range parts {
		if part == "." || part == ".." || part == "" && i != len(parts)-1 {
			return false
		}
	}
	for _, c := range prefix {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-_./@", c)) {
			return false
		}
	}
	return true
}
