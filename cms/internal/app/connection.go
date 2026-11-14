package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/domain"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// ConnectionStatus describes an Organization's Social Account Connection.
type ConnectionStatus struct {
	Connected      bool       `json:"connected"`
	PageName       string     `json:"pageName,omitempty"`
	PageID         string     `json:"pageId,omitempty"`
	IGUsername     string     `json:"igUsername,omitempty"`
	CreatedByEmail string     `json:"createdByEmail,omitempty"`
	CreatedAt      *time.Time `json:"createdAt,omitempty"`
	PendingPages   []PageInfo `json:"pendingPages,omitempty"`
	// Autosync, when on.
	Autosync *AutosyncStatus `json:"autosync,omitempty"`
	// AutosyncLost: Autosync ended because the person who turned it on lost
	// access (decision 19).
	AutosyncLost *domain.AutosyncLost `json:"autosyncLost,omitempty"`
	// RecentSignIn: the current session may do guarded actions now.
	RecentSignIn bool `json:"recentSignIn"`
}

// AutosyncStatus is Autosync as the connection screen shows it. The token
// itself is never returned by any API (T5).
type AutosyncStatus struct {
	ByEmail             string     `json:"byEmail"`
	At                  time.Time  `json:"at"`
	DataAccessExpiresAt *time.Time `json:"dataAccessExpiresAt,omitempty"`
	ReconnectSoon       bool       `json:"reconnectSoon"`
	NeedsReconnect      bool       `json:"needsReconnect"`
}

// PageInfo is a Facebook page to choose from.
type PageInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	IGUsername string `json:"igUsername,omitempty"`
}

// pendingPages are the Facebook pages a Moderator may choose from after signing in
// with Meta. They hold no token.
type pendingPages struct {
	ExpiresAt time.Time          `json:"expiresAt"`
	Pages     []ports.SocialPage `json:"pages"`
}

// connection loads the Organization's connection or explains it is missing.
func (a *App) connection(ctx context.Context, org string) (*domain.Connection, error) {
	c, _, err := load[domain.Connection](ctx, a.Docs, connectionKey(org))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, problem("connection", "połącz najpierw stronę na Facebooku", "connect the Facebook page first")
	}
	return c, err
}

// requireRecentSignIn enforces decision 13 for actions that use or change
// the stored token.
func (a *App) requireRecentSignIn(p domain.Principal) error {
	if !p.RecentlySignedIn(a.now()) {
		return domain.ErrRecentSignIn
	}
	return nil
}

// SocialStatus returns the connection status of an Organization, including
// Facebook pages waiting for the current user to choose one.
func (a *App) SocialStatus(ctx context.Context, p domain.Principal, org string) (ConnectionStatus, error) {
	if _, err := a.authorize(p, org); err != nil {
		return ConnectionStatus{}, err
	}
	st := ConnectionStatus{RecentSignIn: p.RecentlySignedIn(a.now())}
	c, _, err := load[domain.Connection](ctx, a.Docs, connectionKey(org))
	switch {
	case err == nil:
		st.Connected, st.PageName, st.PageID, st.IGUsername = true, c.PageName, c.PageID, c.IGUsername
		st.CreatedByEmail, st.CreatedAt = c.CreatedByEmail, &c.CreatedAt
		st.AutosyncLost = c.AutosyncLost
		if as := c.Autosync; as != nil {
			st.Autosync = &AutosyncStatus{ByEmail: as.ByEmail, At: as.At, DataAccessExpiresAt: as.DataAccessExpiresAt,
				ReconnectSoon: as.ReconnectSoon(a.now()), NeedsReconnect: as.NeedsReconnect}
		}
	case !errors.Is(err, domain.ErrNotFound):
		return st, err
	}
	pages, err := a.pendingPages(ctx, org, p.ID())
	if err != nil {
		return st, err
	}
	for _, pg := range pages {
		st.PendingPages = append(st.PendingPages, PageInfo{ID: pg.ID, Name: pg.Name, IGUsername: pg.IGUsername})
	}
	return st, nil
}

// SocialLoginURL returns where to send the Moderator to sign in with Meta:
// to connect a Facebook page, complete a Post Request, run "Sync now" or
// turn Autosync on.
func (a *App) SocialLoginURL(p domain.Principal, org, state, redirectURI string) (string, error) {
	if _, err := a.authorize(p, org); err != nil {
		return "", err
	}
	return a.Social.LoginURL(state, redirectURI), nil
}

// CompleteSocialLogin lists the Facebook pages the Moderator has a role on and keeps
// that list (no tokens) for 15 minutes until they choose one. The user
// token is used for this one call and dropped.
func (a *App) CompleteSocialLogin(ctx context.Context, p domain.Principal, org, code, redirectURI string) ([]PageInfo, error) {
	if _, err := a.authorize(p, org); err != nil {
		return nil, err
	}
	token, err := a.Social.UserToken(ctx, code, redirectURI)
	if err != nil {
		return nil, fmt.Errorf("meta login: %w", err)
	}
	pages, err := a.Social.Pages(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("meta pages: %w", err)
	}
	if _, err := save(ctx, a.Docs, pendingPagesKey(org, p.ID()), pendingPages{ExpiresAt: a.now().Add(15 * time.Minute), Pages: pages}, ports.AnyETag); err != nil {
		return nil, err
	}
	out := []PageInfo{}
	for _, pg := range pages {
		out = append(out, PageInfo{ID: pg.ID, Name: pg.Name, IGUsername: pg.IGUsername})
	}
	return out, nil
}

func (a *App) pendingPages(ctx context.Context, org, user string) ([]ports.SocialPage, error) {
	key := pendingPagesKey(org, user)
	pp, _, err := load[pendingPages](ctx, a.Docs, key)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if a.now().After(pp.ExpiresAt) {
		_ = a.Docs.Delete(ctx, key, "")
		return nil, nil
	}
	return pp.Pages, nil
}

// SelectPage makes a chosen Facebook page the Organization's connection,
// replacing any previous one. Replacing it with a different Facebook page
// needs confirm (decision 17: the old page's Events stay, frozen) and, while
// Autosync is on, a Recent sign-in; Autosync is turned off first.
func (a *App) SelectPage(ctx context.Context, p domain.Principal, org, pageID string, confirm bool) (ConnectionStatus, error) {
	if _, err := a.authorize(p, org); err != nil {
		return ConnectionStatus{}, err
	}
	pages, err := a.pendingPages(ctx, org, p.ID())
	if err != nil {
		return ConnectionStatus{}, err
	}
	var page *ports.SocialPage
	for i := range pages {
		if pages[i].ID == pageID {
			page = &pages[i]
		}
	}
	if page == nil {
		return ConnectionStatus{}, fmt.Errorf("%w: page not available, sign in with Meta again", domain.ErrNotFound)
	}
	old, oldEtag, err := load[domain.Connection](ctx, a.Docs, connectionKey(org))
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return ConnectionStatus{}, err
	}
	if old != nil {
		if old.Autosync != nil {
			if err := a.requireRecentSignIn(p); err != nil {
				return ConnectionStatus{}, err
			}
		}
		if old.PageID != page.ID && !confirm {
			return ConnectionStatus{}, fmt.Errorf("%w: replace %s with %s", domain.ErrNeedsConfirm, old.PageName, page.Name)
		}
		if old.Autosync != nil && old.PageID != page.ID {
			if err := a.turnOffAutosync(ctx, p, org, nil); err != nil {
				return ConnectionStatus{}, err
			}
			if old, oldEtag, err = a.connectionWithoutAutosync(ctx, org); err != nil {
				return ConnectionStatus{}, err
			}
		}
	}
	c := domain.Connection{
		OrgID: org, PageID: page.ID, PageName: page.Name, IGUserID: page.IGUserID, IGUsername: page.IGUsername,
		CreatedBy: p.ID(), CreatedByEmail: p.User.Email, CreatedAt: a.now(),
	}
	if old != nil && old.PageID == page.ID {
		// The same page chosen again (e.g. its Instagram link changed):
		// Autosync and its token stay.
		c.Autosync, c.AutosyncLost = old.Autosync, old.AutosyncLost
		c.CreatedBy, c.CreatedByEmail, c.CreatedAt = old.CreatedBy, old.CreatedByEmail, old.CreatedAt
	}
	// Conditional on the version read: a concurrent Autosync on must not be
	// overwritten, which would leave its new token unreferenced (T11).
	if _, err := save(ctx, a.Docs, connectionKey(org), c, oldEtag); err != nil {
		return ConnectionStatus{}, err
	}
	_ = a.Docs.Delete(ctx, pendingPagesKey(org, p.ID()), "")
	details := map[string]any{"page": page.Name, "instagram": page.IGUsername}
	if old != nil {
		details["previousPage"], details["previousPageId"] = old.PageName, old.PageID
	}
	a.audit(ctx, p, "social.connected", org, page.ID, details)
	return a.SocialStatus(ctx, p, org)
}

// Disconnect removes the Organization's connection, deleting Autosync's
// token first. Any of its Moderators (and the Super Admin) may do it; while
// Autosync is on it needs a Recent sign-in.
func (a *App) Disconnect(ctx context.Context, p domain.Principal, org string) error {
	if _, err := a.authorize(p, org); err != nil {
		return err
	}
	c, etag, err := load[domain.Connection](ctx, a.Docs, connectionKey(org))
	if err != nil {
		return err
	}
	if c.Autosync != nil {
		if err := a.requireRecentSignIn(p); err != nil {
			return err
		}
		if err := a.turnOffAutosync(ctx, p, org, nil); err != nil {
			return err
		}
		if c, etag, err = a.connectionWithoutAutosync(ctx, org); err != nil {
			return err
		}
	}
	// Conditional, so a concurrent Autosync on is not deleted with the
	// document that names its token (T11).
	if err := a.Docs.Delete(ctx, connectionKey(org), etag); err != nil {
		return storeErr(err)
	}
	a.audit(ctx, p, "social.disconnected", org, c.PageID, map[string]any{"page": c.PageName})
	return nil
}

// connectionWithoutAutosync reloads the connection after Autosync was
// turned off, refusing when it was turned on again meanwhile.
func (a *App) connectionWithoutAutosync(ctx context.Context, org string) (*domain.Connection, string, error) {
	c, etag, err := load[domain.Connection](ctx, a.Docs, connectionKey(org))
	if err != nil {
		return nil, "", err
	}
	if c.Autosync != nil {
		return nil, "", fmt.Errorf("%w: Autosync was turned on meanwhile", domain.ErrConflict)
	}
	return c, etag, nil
}

// StartAutosync checks that the Moderator may turn Autosync on now; the
// Meta sign-in that issues the token follows, and its callback calls
// CompleteAutosync.
func (a *App) StartAutosync(ctx context.Context, p domain.Principal, org string) error {
	if _, err := a.authorize(p, org); err != nil {
		return err
	}
	if a.Secrets == nil {
		return fmt.Errorf("%w: no secret store configured", domain.ErrInvalidState)
	}
	if err := a.requireRecentSignIn(p); err != nil {
		return err
	}
	_, err := a.connection(ctx, org)
	return err
}

// secretName is a new, never reused secret name for an Organization's token
// (T11: a soft-deleted name is never set again).
func (a *App) secretName(org string) string {
	return "fbpage-" + domain.Slugify(org) + "-" + a.newID()
}

// ownSecretName reports whether name is one secretName could have issued
// for org.
func (a *App) ownSecretName(org, name string) bool {
	id, ok := strings.CutPrefix(name, "fbpage-"+domain.Slugify(org)+"-")
	return ok && domain.ValidID(id)
}

// CompleteAutosync turns Autosync on after the Meta sign-in: the user token
// becomes a long-lived one, the Facebook page token is derived from it and
// stored under a new secret name, and the user tokens are dropped. A token
// stored before (turning Autosync on again, e.g. before the data access
// expires) is replaced and its secret deleted.
func (a *App) CompleteAutosync(ctx context.Context, p domain.Principal, org, code, redirectURI string) error {
	if err := a.StartAutosync(ctx, p, org); err != nil {
		return err
	}
	ctx = context.WithoutCancel(ctx)
	conn, err := a.connection(ctx, org)
	if err != nil {
		return err
	}
	userToken, err := a.Social.UserToken(ctx, code, redirectURI)
	if err != nil {
		return fmt.Errorf("meta login: %w", err)
	}
	token, page, expires, err := a.Events.LongLivedPageToken(ctx, userToken, conn.PageID)
	if err != nil {
		return err
	}
	name := a.secretName(org)
	if err := a.Secrets.Set(ctx, name, token); err != nil {
		return fmt.Errorf("store token: %w", err)
	}
	var previous string
	_, _, err = mutate(ctx, a.Docs, connectionKey(org), func(c *domain.Connection) error {
		if c.PageID != conn.PageID {
			return fmt.Errorf("%w: the connection changed meanwhile", domain.ErrConflict)
		}
		previous = ""
		if c.Autosync != nil {
			previous = c.Autosync.SecretName
		}
		as := &domain.Autosync{SecretName: name, By: p.ID(), ByEmail: p.User.Email, At: a.now()}
		if !expires.IsZero() {
			as.DataAccessExpiresAt = &expires
		}
		c.Autosync, c.AutosyncLost = as, nil
		if page.IGUserID != "" && c.IGUserID == "" {
			c.IGUserID, c.IGUsername = page.IGUserID, page.IGUsername
		}
		return nil
	})
	if err != nil {
		// Never leave a token nobody points to.
		if derr := a.Secrets.Delete(ctx, name); derr != nil && !errors.Is(derr, ports.ErrNotFound) {
			a.Log.Error("orphaned autosync secret", "org", org, "err", derr)
		}
		return err
	}
	if previous != "" && a.ownSecretName(org, previous) {
		if err := a.Secrets.Delete(ctx, previous); err != nil && !errors.Is(err, ports.ErrNotFound) {
			a.Log.Error("delete previous autosync secret", "org", org, "err", err)
		}
	}
	details := map[string]any{"page": conn.PageID}
	if !expires.IsZero() {
		details["dataAccessExpiresAt"] = expires
	}
	a.audit(ctx, p, "autosync.on", org, conn.PageID, details)
	return nil
}

// DisableAutosync turns Autosync off and deletes the stored token.
func (a *App) DisableAutosync(ctx context.Context, p domain.Principal, org string) error {
	if _, err := a.authorize(p, org); err != nil {
		return err
	}
	if err := a.requireRecentSignIn(p); err != nil {
		return err
	}
	return a.turnOffAutosync(ctx, p, org, nil)
}

// turnOffAutosync deletes the stored token and clears Autosync. lost
// records why, when nobody asked (decision 19).
func (a *App) turnOffAutosync(ctx context.Context, p domain.Principal, org string, lost *domain.AutosyncLost) error {
	c, _, err := load[domain.Connection](ctx, a.Docs, connectionKey(org))
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if c.Autosync == nil {
		if lost != nil {
			_, _, err = mutate(ctx, a.Docs, connectionKey(org), func(c *domain.Connection) error {
				c.AutosyncLost = lost
				return nil
			})
		}
		return err
	}
	secret, byEmail := c.Autosync.SecretName, c.Autosync.ByEmail
	// A name that is not this Organization's (a tampered document) is never
	// deleted: Autosync is only cleared, and the daily sweep removes any
	// secret left unreferenced.
	own := a.ownSecretName(org, secret)
	if !own {
		a.Log.Error("autosync secret name not of this organization", "org", org, "secret", secret)
	}
	// The secret goes first: while it may still exist, the document keeps
	// naming it, so a failed delete is retried by turning Autosync off again
	// (T11). A document left naming a deleted secret is safe: storedToken
	// answers "needs reconnect".
	if !own {
		// nothing of this Organization's to delete
	} else if err := a.Secrets.Delete(ctx, secret); err != nil && !errors.Is(err, ports.ErrNotFound) {
		a.Log.Error("autosync secret not deleted", "org", org, "secret", secret, "err", err)
		a.audit(ctx, p, "autosync.secret_delete_failed", org, "", map[string]any{"secret": secret})
		return err
	}
	_, _, err = mutate(ctx, a.Docs, connectionKey(org), func(c *domain.Connection) error {
		if c.Autosync != nil && c.Autosync.SecretName != secret {
			return nil // turned on again meanwhile with a new token: keep it
		}
		c.Autosync, c.AutosyncLost = nil, lost
		return nil
	})
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	details := map[string]any{"enabledBy": byEmail}
	if lost != nil {
		details["reason"] = lost.Reason
	}
	a.audit(ctx, p, "autosync.off", org, "", details)
	return nil
}

// autosyncLostAccess turns Autosync off in each Organization the user can no
// longer manage, when they are the one who turned it on (decision 19).
func (a *App) autosyncLostAccess(ctx context.Context, p domain.Principal, user *domain.User, orgs []string) error {
	var errs []error
	for _, org := range orgs {
		c, _, err := load[domain.Connection](ctx, a.Docs, connectionKey(org))
		if errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if c.Autosync == nil || c.Autosync.By != user.ID {
			continue
		}
		errs = append(errs, a.turnOffAutosync(ctx, p, org, &domain.AutosyncLost{Reason: "access", ByEmail: user.Email, At: a.now()}))
	}
	return errors.Join(errs...)
}

// purgeSocial removes expired Post Requests and Facebook page lists of an
// Organization; neither holds a secret. It also rewrites a connection saved
// by an early version, which kept an encrypted Facebook page token, without it.
func (a *App) purgeSocial(ctx context.Context, org string, now time.Time) error {
	var errs []error
	if d, err := a.Docs.Get(ctx, connectionKey(org)); err == nil && bytes.Contains(d.Data, []byte(`"tokenEnc"`)) {
		var c domain.Connection
		if err := json.Unmarshal(d.Data, &c); err != nil {
			errs = append(errs, err)
		} else if _, err := save(ctx, a.Docs, connectionKey(org), c, d.ETag); err != nil {
			errs = append(errs, err)
		} else {
			a.audit(ctx, system, "social.token_removed", org, c.PageID, nil)
		}
	} else if err != nil && !errors.Is(err, ports.ErrNotFound) {
		errs = append(errs, err)
	}
	drop := func(key string, expired bool, err error) {
		if err == nil && expired {
			err = a.Docs.Delete(ctx, key, "")
		}
		if err != nil && !errors.Is(storeErr(err), domain.ErrNotFound) {
			errs = append(errs, err)
		}
	}
	keys, err := a.Docs.List(ctx, postRequestsPrefix(org))
	errs = append(errs, err)
	for _, k := range keys {
		r, _, err := load[domain.PostRequest](ctx, a.Docs, k)
		drop(k, err == nil && r.Expired(now), err)
	}
	keys, err = a.Docs.List(ctx, pendingPagesPrefix(org))
	errs = append(errs, err)
	for _, k := range keys {
		pp, _, err := load[pendingPages](ctx, a.Docs, k)
		drop(k, err == nil && now.After(pp.ExpiresAt), err)
	}
	keys, err = a.Docs.List(ctx, "orgs/"+org+"/social/daily/")
	errs = append(errs, err)
	for _, k := range keys {
		pc, _, err := load[domain.PostCounter](ctx, a.Docs, k)
		old := false
		if err == nil {
			day, perr := time.Parse(time.DateOnly, pc.Day)
			old = perr == nil && now.Sub(day) > 48*time.Hour
		}
		drop(k, old, err)
	}
	return errors.Join(errs...)
}

// orphanAge is how old an unreferenced token secret must be before the
// sweep deletes it: CompleteAutosync stores the secret a moment before the
// connection names it.
const orphanAge = time.Hour

// sweepSecrets deletes token secrets no connection names (left by a crash
// or a failed delete) and purges deleted ones still in Key Vault's soft
// delete, so no copy of a page token outlives Autosync (T11).
func (a *App) sweepSecrets(ctx context.Context, now time.Time) error {
	inUse := map[string]bool{}
	for _, o := range a.orgs {
		c, _, err := load[domain.Connection](ctx, a.Docs, connectionKey(o.ID))
		if errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if err != nil {
			// Unsure what is in use: delete nothing.
			return fmt.Errorf("secret sweep: %w", err)
		}
		if c.Autosync != nil {
			inUse[c.Autosync.SecretName] = true
		}
	}
	secrets, err := a.Secrets.List(ctx, "fbpage-")
	if err != nil {
		return fmt.Errorf("secret sweep: %w", err)
	}
	var errs []error
	for _, sec := range secrets {
		if inUse[sec.Name] || now.Sub(sec.Created) < orphanAge {
			continue
		}
		if err := a.Secrets.Delete(ctx, sec.Name); err != nil && !errors.Is(err, ports.ErrNotFound) {
			errs = append(errs, err)
			continue
		}
		a.audit(ctx, system, "autosync.orphan_deleted", "", "", map[string]any{"secret": sec.Name})
	}
	if _, err := a.Secrets.PurgeDeleted(ctx, "fbpage-"); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
