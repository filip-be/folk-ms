package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

// Venue is a place where Events happen. Venues are archived, never deleted.
type Venue struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"orgId"`
	Name      string    `json:"name"`
	Address   string    `json:"address"`
	MapURL    string    `json:"mapUrl,omitempty"`
	Archived  bool      `json:"archived,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
	UpdatedBy string    `json:"updatedBy"`
}

// Normalize trims the fields.
func (v Venue) Normalize() Venue {
	v.Name = strings.TrimSpace(v.Name)
	v.Address = strings.TrimSpace(v.Address)
	v.MapURL = strings.TrimSpace(v.MapURL)
	return v
}

// Validate checks a Venue.
func (v Venue) Validate() error {
	var p problems
	if v.Name == "" {
		p.add("name", "nazwa jest wymagana")
	} else if len([]rune(v.Name)) > 200 {
		p.add("name", "nazwa jest za długa")
	}
	if v.Address == "" {
		p.add("address", "adres jest wymagany")
	} else if len([]rune(v.Address)) > 500 {
		p.add("address", "adres jest za długi")
	}
	if v.MapURL != "" && !isHTTPURL(v.MapURL) {
		p.add("mapUrl", "podaj adres zaczynający się od https://")
	}
	return p.err()
}

// SameContent reports whether two Venues show the same to visitors.
func (v Venue) SameContent(o Venue) bool {
	return v.Name == o.Name && v.Address == o.Address && v.MapURL == o.MapURL
}

// frozenKey is the snapshot key of a frozen copy of a Venue: the id plus a
// hash of what visitors see, so identical copies share one entry.
func (v Venue) frozenKey() string {
	h := sha256.Sum256([]byte(v.Name + "\x00" + v.Address + "\x00" + v.MapURL))
	return v.ID + "@" + hex.EncodeToString(h[:4])
}
