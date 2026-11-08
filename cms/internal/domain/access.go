package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"net/mail"
	"slices"
	"strings"
	"time"
)

// InvitationTTL is how long a pending Invitation stays valid.
const InvitationTTL = 14 * 24 * time.Hour

// User is a person who signed in: the Super Admin or a Moderator. It is
// identified by the identity provider's issuer and subject, never by email.
type User struct {
	ID           string     `json:"id"`
	Issuer       string     `json:"issuer"`
	Subject      string     `json:"subject"`
	Email        string     `json:"email"`
	Name         string     `json:"name,omitempty"`
	Orgs         []string   `json:"orgs"`
	InvitationID string     `json:"invitationId,omitempty"`
	Revoked      bool       `json:"revoked,omitempty"`
	RevokedAt    *time.Time `json:"revokedAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	LastLoginAt  time.Time  `json:"lastLoginAt"`
	// SessionsValidAfter invalidates sessions issued before it.
	SessionsValidAfter time.Time `json:"sessionsValidAfter"`
}

// UserID derives the stable user id from issuer and subject.
func UserID(issuer, subject string) string {
	h := sha256.Sum256([]byte(issuer + "\x00" + subject))
	return hex.EncodeToString(h[:16])
}

// SuperAdminBinding records which identity the configured Super Admin email
// was bound to on first sign-in.
type SuperAdminBinding struct {
	UserID  string    `json:"userId"`
	Email   string    `json:"email"`
	BoundAt time.Time `json:"boundAt"`
}

// NormalizeEmail lowercases and trims an email.
func NormalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// ValidEmail reports whether s is a plain email address.
func ValidEmail(s string) bool {
	a, err := mail.ParseAddress(s)
	return err == nil && a.Address == s && a.Name == ""
}

// Principal is who performs a use case.
type Principal struct {
	User       *User
	SuperAdmin bool
}

// ID returns the user id.
func (p Principal) ID() string {
	if p.User == nil {
		return ""
	}
	return p.User.ID
}

// CanManageOrg reports whether the principal may manage the Organization's
// Events, Venues and Social Account Connection.
func (p Principal) CanManageOrg(org string) bool {
	if p.User == nil || p.User.Revoked {
		return false
	}
	return p.SuperAdmin || slices.Contains(p.User.Orgs, org)
}

// InvitationStatus is the lifecycle state of an Invitation.
type InvitationStatus string

const (
	InvitationPending InvitationStatus = "pending"
	InvitationBound   InvitationStatus = "bound"
	InvitationExpired InvitationStatus = "expired"
	InvitationRevoked InvitationStatus = "revoked"
)

// Invitation grants Moderator access to an email for chosen Organizations.
type Invitation struct {
	ID          string           `json:"id"`
	Email       string           `json:"email"`
	Orgs        []string         `json:"orgs"`
	Status      InvitationStatus `json:"status"`
	CreatedBy   string           `json:"createdBy"`
	CreatedAt   time.Time        `json:"createdAt"`
	ExpiresAt   time.Time        `json:"expiresAt"`
	BoundUserID string           `json:"boundUserId,omitempty"`
	BoundAt     *time.Time       `json:"boundAt,omitempty"`
	RevokedAt   *time.Time       `json:"revokedAt,omitempty"`
}

// NewInvitation validates and creates a pending Invitation.
func NewInvitation(id, email string, orgs []string, knownOrg func(string) bool, by string, now time.Time) (*Invitation, error) {
	email = NormalizeEmail(email)
	var p problems
	if !ValidEmail(email) {
		p.add("email", "podaj poprawny adres e-mail", "enter a valid email address")
	}
	if len(orgs) == 0 {
		p.add("orgs", "wybierz co najmniej jedną organizację", "choose at least one organization")
	}
	for _, o := range orgs {
		if !knownOrg(o) {
			p.add("orgs", "nieznana organizacja %s", "unknown organization %s", o)
		}
	}
	if err := p.err(); err != nil {
		return nil, err
	}
	o := slices.Clone(orgs)
	slices.Sort(o)
	return &Invitation{
		ID: id, Email: email, Orgs: slices.Compact(o), Status: InvitationPending,
		CreatedBy: by, CreatedAt: now, ExpiresAt: now.Add(InvitationTTL),
	}, nil
}

// EffectiveStatus applies expiry.
func (i *Invitation) EffectiveStatus(now time.Time) InvitationStatus {
	if i.Status == InvitationPending && now.After(i.ExpiresAt) {
		return InvitationExpired
	}
	return i.Status
}

// Usable reports whether the Invitation can be bound now by email.
func (i *Invitation) Usable(email string, now time.Time) bool {
	return i.EffectiveStatus(now) == InvitationPending && i.Email == NormalizeEmail(email)
}

// Bind marks the Invitation as used by a user.
func (i *Invitation) Bind(userID string, now time.Time) {
	i.Status = InvitationBound
	i.BoundUserID = userID
	i.BoundAt = &now
}

// Revoke cancels a pending Invitation.
func (i *Invitation) Revoke(now time.Time) error {
	if i.EffectiveStatus(now) != InvitationPending {
		return ErrInvalidState
	}
	i.Status = InvitationRevoked
	i.RevokedAt = &now
	return nil
}

// SetOrgs changes a Moderator's Organizations and returns the ones removed.
func (u *User) SetOrgs(orgs []string, now time.Time) (removed []string) {
	o := slices.Clone(orgs)
	slices.Sort(o)
	o = slices.Compact(o)
	for _, old := range u.Orgs {
		if !slices.Contains(o, old) {
			removed = append(removed, old)
		}
	}
	u.Orgs = o
	if len(removed) > 0 {
		u.SessionsValidAfter = now
	}
	return removed
}

// Revoke removes all access and returns the Organizations removed.
func (u *User) Revoke(now time.Time) []string {
	removed := slices.Clone(u.Orgs)
	u.Orgs = nil
	u.Revoked = true
	u.RevokedAt = &now
	u.SessionsValidAfter = now
	return removed
}
