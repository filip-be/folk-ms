package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/domain"
)

// UserView is a user as listed for the Super Admin.
type UserView struct {
	domain.User
	SuperAdmin bool `json:"superAdmin"`
}

// ListUsers lists every user (Super Admin only).
func (a *App) ListUsers(ctx context.Context, p domain.Principal) ([]UserView, error) {
	if err := requireSuperAdmin(p); err != nil {
		return nil, err
	}
	users, err := loadAll[domain.User](ctx, a.Docs, "users/")
	if err != nil {
		return nil, err
	}
	out := []UserView{}
	for _, u := range users {
		sa, err := a.isSuperAdmin(ctx, u)
		if err != nil {
			return nil, err
		}
		out = append(out, UserView{User: *u, SuperAdmin: sa})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Email < out[j].Email })
	return out, nil
}

// InvitationView adds the effective status.
type InvitationView struct {
	domain.Invitation
	EffectiveStatus domain.InvitationStatus `json:"effectiveStatus"`
}

// ListInvitations lists Invitations, newest first (Super Admin only).
func (a *App) ListInvitations(ctx context.Context, p domain.Principal) ([]InvitationView, error) {
	if err := requireSuperAdmin(p); err != nil {
		return nil, err
	}
	invs, err := loadAll[domain.Invitation](ctx, a.Docs, "invitations/")
	if err != nil {
		return nil, err
	}
	now := a.now()
	out := []InvitationView{}
	for _, i := range invs {
		out = append(out, InvitationView{Invitation: *i, EffectiveStatus: i.EffectiveStatus(now)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// CreateInvitation invites an email as Moderator of chosen Organizations.
// The Super Admin sends the CMS link to the person; the CMS sends no email.
func (a *App) CreateInvitation(ctx context.Context, p domain.Principal, email string, orgs []string) (*domain.Invitation, error) {
	if err := requireSuperAdmin(p); err != nil {
		return nil, err
	}
	inv, err := domain.NewInvitation(a.newID(), email, orgs, func(o string) bool { _, ok := a.orgByID[o]; return ok }, p.ID(), a.now())
	if err != nil {
		return nil, err
	}
	if _, err := save(ctx, a.Docs, invitationKey(inv.ID), inv, ""); err != nil {
		return nil, err
	}
	a.audit(ctx, p, "invitation.created", "", inv.ID, map[string]any{"email": inv.Email, "orgs": inv.Orgs})
	return inv, nil
}

// RevokeInvitation cancels a pending Invitation.
func (a *App) RevokeInvitation(ctx context.Context, p domain.Principal, id string) (*domain.Invitation, error) {
	if err := requireSuperAdmin(p); err != nil {
		return nil, err
	}
	inv, _, err := mutate(ctx, a.Docs, invitationKey(id), func(i *domain.Invitation) error { return i.Revoke(a.now()) })
	if err != nil {
		return nil, err
	}
	a.audit(ctx, p, "invitation.revoked", "", id, map[string]any{"email": inv.Email})
	return inv, nil
}

// SetUserOrgs changes a Moderator's Organizations. Removing an Organization
// flags its Social Account Connection if this user created it.
func (a *App) SetUserOrgs(ctx context.Context, p domain.Principal, userID string, orgs []string) (*domain.User, error) {
	if err := requireSuperAdmin(p); err != nil {
		return nil, err
	}
	for _, o := range orgs {
		if _, ok := a.orgByID[o]; !ok {
			return nil, problem("orgs", "nieznana organizacja "+o, "unknown organization "+o)
		}
	}
	var removed []string
	u, _, err := mutate(ctx, a.Docs, userKey(userID), func(u *domain.User) error {
		if u.Revoked {
			return domain.ErrInvalidState
		}
		removed = u.SetOrgs(orgs, a.now())
		return nil
	})
	if err != nil {
		return nil, err
	}
	a.audit(ctx, p, "user.orgs_changed", "", userID, map[string]any{"email": u.Email, "orgs": u.Orgs, "removed": removed})
	return u, a.flagConnections(ctx, p, u, removed)
}

// RevokeUser removes all of a Moderator's access, effective on their next
// request.
func (a *App) RevokeUser(ctx context.Context, p domain.Principal, userID string) (*domain.User, error) {
	if err := requireSuperAdmin(p); err != nil {
		return nil, err
	}
	if userID == p.ID() {
		return nil, problem("user", "nie można odebrać dostępu samemu sobie", "you cannot revoke your own access")
	}
	var removed []string
	u, _, err := mutate(ctx, a.Docs, userKey(userID), func(u *domain.User) error {
		removed = u.Revoke(a.now())
		return nil
	})
	if err != nil {
		return nil, err
	}
	a.audit(ctx, p, "user.revoked", "", userID, map[string]any{"email": u.Email, "removed": removed})
	return u, a.flagConnections(ctx, p, u, removed)
}

func (a *App) flagConnections(ctx context.Context, p domain.Principal, u *domain.User, orgs []string) error {
	var errs []error
	for _, org := range orgs {
		flagged := false
		_, _, err := mutate(ctx, a.Docs, connectionKey(org), func(c *domain.Connection) error {
			flagged = c.FlagIfCreatedBy(u.ID, u.Email)
			return nil
		})
		if errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if flagged {
			a.audit(ctx, p, "social.needs_reconnect", org, u.ID, map[string]any{"email": u.Email})
		}
	}
	return errors.Join(errs...)
}

// AuditDays lists the days that have Audit Log entries (Super Admin only).
func (a *App) AuditDays(ctx context.Context, p domain.Principal) ([]string, error) {
	if err := requireSuperAdmin(p); err != nil {
		return nil, err
	}
	days, err := a.Audit.Segments(ctx)
	if days == nil {
		days = []string{}
	}
	return days, err
}

// ReadAudit returns one day of the Audit Log, newest first.
func (a *App) ReadAudit(ctx context.Context, p domain.Principal, day string) ([]AuditEntry, error) {
	if err := requireSuperAdmin(p); err != nil {
		return nil, err
	}
	if _, err := time.Parse(time.DateOnly, day); err != nil {
		return nil, problem("day", "nieprawidłowa data", "invalid date")
	}
	data, err := a.Audit.Read(ctx, day)
	if err != nil {
		return nil, err
	}
	out := []AuditEntry{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var e AuditEntry
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			out = append(out, e)
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, sc.Err()
}
