package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/domain"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// ErrNoAccess is returned at sign-in to people with no Invitation.
var ErrNoAccess = fmt.Errorf("%w: no access", domain.ErrForbidden)

// ErrSuperAdminIdentity is returned when the Super Admin signs in with a
// provider or identity other than the one it is bound to.
var ErrSuperAdminIdentity = fmt.Errorf("%w: the super admin signs in with its bound identity only", ErrNoAccess)

func (a *App) superAdminEmail() string { return domain.NormalizeEmail(a.SuperAdminEmail) }

// isSuperAdmin reports whether u is bound as the configured Super Admin.
func (a *App) isSuperAdmin(ctx context.Context, u *domain.User) (bool, error) {
	sa := a.superAdminEmail()
	if sa == "" {
		return false, nil
	}
	b, _, err := load[domain.SuperAdminBinding](ctx, a.Docs, superAdminKey)
	if errors.Is(err, domain.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return b.UserID == u.ID && b.Email == sa, nil
}

// SignIn turns a verified identity into a user: an existing user (by this
// identity, or by email from another provider), the configured Super Admin
// (bound on first sign-in), or a person with a usable Invitation (bound now).
// Everyone else is refused.
func (a *App) SignIn(ctx context.Context, id ports.Identity) (_ *domain.User, retErr error) {
	email := domain.NormalizeEmail(id.Email)
	if !id.EmailVerified || id.Subject == "" || email == "" {
		return nil, ErrNoAccess
	}
	isSA := a.superAdminEmail() != "" && email == a.superAdminEmail()
	if isSA && id.NoSuperAdmin {
		a.Log.Warn("super admin email from a provider not trusted for it", "issuer", id.Issuer)
		return nil, ErrSuperAdminIdentity
	}
	now := a.now()
	uid := domain.UserID(id.Issuer, id.Subject)
	user, etag, err := load[domain.User](ctx, a.Docs, userKey(uid))
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	if user == nil && !isSA {
		// The Super Admin stays bound to one identity; anyone else is the
		// same person whichever provider returns their email.
		if user, etag, err = a.userByEmail(ctx, email); err != nil {
			return nil, err
		}
		if user != nil {
			// Never the Super Admin, also when its email changed after it
			// was bound (the binding is by user id).
			if sa, err := a.isSuperAdmin(ctx, user); err != nil {
				return nil, err
			} else if sa {
				a.Log.Warn("super admin matched by email from another identity", "issuer", id.Issuer)
				return nil, ErrSuperAdminIdentity
			}
			uid = user.ID
			if a.addIdentity(user, id) {
				matched := user
				defer func() {
					if retErr == nil {
						a.audit(ctx, domain.Principal{User: matched}, "identity.matched", "", matched.ID, map[string]any{"email": email, "issuer": id.Issuer})
					}
				}()
			}
		}
	}
	isNew := user == nil
	if isNew {
		user = &domain.User{ID: uid, Issuer: id.Issuer, Subject: id.Subject, CreatedAt: now, Orgs: []string{}}
	}
	user.Email, user.Name, user.LastLoginAt = email, id.Name, now

	bindSA := false
	if sa := a.superAdminEmail(); isSA {
		b, bEtag, err := load[domain.SuperAdminBinding](ctx, a.Docs, superAdminKey)
		switch {
		case errors.Is(err, domain.ErrNotFound):
			bindSA, bEtag = true, ""
		case err != nil:
			return nil, err
		case b.Email != sa:
			bindSA = true // the configured email changed: rebind
		case b.UserID != uid:
			a.Log.Warn("super admin email used by a different identity", "user", uid)
			return nil, ErrNoAccess
		}
		if bindSA {
			if _, err := save(ctx, a.Docs, superAdminKey, domain.SuperAdminBinding{UserID: uid, Email: sa, BoundAt: now}, bEtag); err != nil {
				return nil, err
			}
			user.Revoked, user.RevokedAt = false, nil
		}
		if _, err := save(ctx, a.Docs, userKey(uid), user, etagOrCreate(etag, isNew)); err != nil {
			return nil, err
		}
		if bindSA {
			a.audit(ctx, domain.Principal{User: user, SuperAdmin: true}, "superadmin.bound", "", uid, map[string]any{"email": email})
		}
		return user, nil
	}

	if isNew || user.Revoked {
		inv, invEtag, err := a.findInvitation(ctx, email, now)
		if err != nil {
			return nil, err
		}
		if inv == nil {
			return nil, ErrNoAccess
		}
		inv.Bind(uid, now)
		if _, err := save(ctx, a.Docs, invitationKey(inv.ID), inv, invEtag); err != nil {
			return nil, err
		}
		user.Orgs, user.InvitationID = slices.Clone(inv.Orgs), inv.ID
		user.Revoked, user.RevokedAt = false, nil
		if _, err := save(ctx, a.Docs, userKey(uid), user, etagOrCreate(etag, isNew)); err != nil {
			return nil, err
		}
		a.audit(ctx, domain.Principal{User: user}, "invitation.bound", "", inv.ID, map[string]any{"email": email, "orgs": inv.Orgs})
		return user, nil
	}
	if _, err := save(ctx, a.Docs, userKey(uid), user, etag); err != nil {
		return nil, err
	}
	return user, nil
}

// userByEmail finds the user with this email for an identity seen for the
// first time: the active one if any, else the most recent sign-in.
func (a *App) userByEmail(ctx context.Context, email string) (*domain.User, string, error) {
	keys, err := a.Docs.List(ctx, "users/")
	if err != nil {
		return nil, "", err
	}
	var best *domain.User
	bestEtag, n := "", 0
	for _, k := range keys {
		u, etag, err := load[domain.User](ctx, a.Docs, k)
		if errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		if domain.NormalizeEmail(u.Email) != email {
			continue
		}
		n++
		if best == nil || best.Revoked && !u.Revoked ||
			best.Revoked == u.Revoked && u.LastLoginAt.After(best.LastLoginAt) {
			best, bestEtag = u, etag
		}
	}
	if n > 1 {
		a.Log.Warn("several users share an email; using the most recent", "user", best.ID, "count", n)
	}
	return best, bestEtag, nil
}

// addIdentity records an identity matched to u by email; it reports whether
// the identity is new.
func (a *App) addIdentity(u *domain.User, id ports.Identity) bool {
	ref := domain.Identity{Issuer: id.Issuer, Subject: id.Subject}
	if (u.Issuer == ref.Issuer && u.Subject == ref.Subject) || slices.Contains(u.Identities, ref) {
		return false
	}
	u.Identities = append(u.Identities, ref)
	return true
}

func etagOrCreate(etag string, isNew bool) string {
	if isNew {
		return ""
	}
	return etag
}

// findInvitation returns the newest usable Invitation for an email.
func (a *App) findInvitation(ctx context.Context, email string, now time.Time) (*domain.Invitation, string, error) {
	keys, err := a.Docs.List(ctx, "invitations/")
	if err != nil {
		return nil, "", err
	}
	var best *domain.Invitation
	bestEtag := ""
	for _, k := range keys {
		inv, etag, err := load[domain.Invitation](ctx, a.Docs, k)
		if err != nil {
			continue
		}
		if inv.Usable(email, now) && (best == nil || inv.CreatedAt.After(best.CreatedAt)) {
			best, bestEtag = inv, etag
		}
	}
	return best, bestEtag, nil
}

// Principal resolves a session into a Principal, re-reading the user so
// that revocation takes effect on the next request.
func (a *App) Principal(ctx context.Context, userID string, issuedAt time.Time) (domain.Principal, error) {
	u, _, err := load[domain.User](ctx, a.Docs, userKey(userID))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Principal{}, domain.ErrUnauthorized
		}
		return domain.Principal{}, err
	}
	if u.Revoked || issuedAt.Before(u.SessionsValidAfter.Truncate(time.Second)) {
		return domain.Principal{}, domain.ErrUnauthorized
	}
	sa, err := a.isSuperAdmin(ctx, u)
	if err != nil {
		return domain.Principal{}, err
	}
	if !sa && len(u.Orgs) == 0 {
		return domain.Principal{}, domain.ErrUnauthorized
	}
	return domain.Principal{User: u, SuperAdmin: sa}, nil
}

// Me describes the signed-in user for the admin SPA.
type Me struct {
	ID         string    `json:"id"`
	Email      string    `json:"email"`
	Name       string    `json:"name"`
	SuperAdmin bool      `json:"superAdmin"`
	Orgs       []OrgInfo `json:"orgs"`
	PublicURL  string    `json:"publicUrl"`
}

// OrgInfo is an Organization as listed in the UI.
type OrgInfo struct {
	ID      string      `json:"id"`
	Name    domain.Text `json:"name"`
	SiteURL string      `json:"siteUrl"`
}

// Me returns the principal's profile and Organizations.
func (a *App) Me(p domain.Principal) Me {
	m := Me{ID: p.User.ID, Email: p.User.Email, Name: p.User.Name, SuperAdmin: p.SuperAdmin, Orgs: []OrgInfo{},
		PublicURL: strings.TrimSuffix(a.Pub.URL(""), "/")}
	for _, o := range a.orgs {
		if p.CanManageOrg(o.ID) {
			m.Orgs = append(m.Orgs, OrgInfo{ID: o.ID, Name: o.Name, SiteURL: o.Site.BaseURL})
		}
	}
	return m
}
