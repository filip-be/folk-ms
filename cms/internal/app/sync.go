package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/domain"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// ErrNeedsMetaLogin means "Sync now" needs a Meta sign-in: Autosync is off,
// so the CMS has no token.
var ErrNeedsMetaLogin = errors.New("sync: Meta sign-in needed")

// SyncBy values for runs nobody started by hand.
const SyncByAutosync = "autosync"

// SyncStatusView is what the Event list shows about Sync.
type SyncStatusView struct {
	domain.SyncStatus
	Autosync bool `json:"autosync"`
	// Running: a run holds the lease now.
	Running bool `json:"running"`
}

// SyncStatus returns an Organization's sync status.
func (a *App) SyncStatus(ctx context.Context, p domain.Principal, org string) (SyncStatusView, error) {
	if _, err := a.authorize(p, org); err != nil {
		return SyncStatusView{}, err
	}
	v := SyncStatusView{}
	st, _, err := load[domain.SyncStatus](ctx, a.Docs, syncStatusKey(org))
	switch {
	case err == nil:
		v.SyncStatus = *st
		v.Running = a.now().Before(st.LeaseUntil)
		v.LeaseHolder = ""
	case !errors.Is(err, domain.ErrNotFound):
		return v, err
	}
	conn, _, err := load[domain.Connection](ctx, a.Docs, connectionKey(org))
	if err == nil {
		v.Autosync = conn.Autosync != nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return v, err
	}
	return v, nil
}

// SyncNow starts a Sync by hand (at most once a minute). With Autosync on it
// runs with the stored token; otherwise it returns ErrNeedsMetaLogin and the
// caller sends the Moderator to the Meta sign-in, whose callback calls
// CompleteSyncLogin.
func (a *App) SyncNow(ctx context.Context, p domain.Principal, org string) (domain.SyncResult, error) {
	if _, err := a.authorize(p, org); err != nil {
		return domain.SyncResult{}, err
	}
	conn, err := a.connection(ctx, org)
	if err != nil {
		return domain.SyncResult{}, err
	}
	if conn.Autosync == nil {
		// Checked before the sign-in, so a Moderator is not sent to Meta for nothing.
		if st, _, err := load[domain.SyncStatus](ctx, a.Docs, syncStatusKey(org)); err == nil {
			probe := *st
			if err := probe.Claim("probe", domain.SyncNowInterval, a.now()); err != nil {
				return domain.SyncResult{}, err
			}
		}
		return domain.SyncResult{}, ErrNeedsMetaLogin
	}
	return a.runSync(ctx, p, conn, func(ctx context.Context) (string, error) {
		return a.storedToken(ctx, p, conn, "sync")
	}, domain.SyncNowInterval)
}

// CompleteSyncLogin runs a Sync after the Meta sign-in of "Sync now" with
// Autosync off: the code becomes a short-lived user token, which yields the
// Facebook page token (proving the Moderator has a role on the page); both
// are dropped when the Sync ends.
func (a *App) CompleteSyncLogin(ctx context.Context, p domain.Principal, org, code, redirectURI string) (domain.SyncResult, error) {
	if _, err := a.authorize(p, org); err != nil {
		return domain.SyncResult{}, err
	}
	conn, err := a.connection(ctx, org)
	if err != nil {
		return domain.SyncResult{}, err
	}
	ctx = context.WithoutCancel(ctx)
	userToken, err := a.Social.UserToken(ctx, code, redirectURI)
	if err != nil {
		return domain.SyncResult{}, fmt.Errorf("meta login: %w", err)
	}
	pageToken, _, err := a.Social.PageToken(ctx, userToken, conn.PageID)
	if err != nil {
		return domain.SyncResult{}, err
	}
	return a.runSync(ctx, p, conn, func(context.Context) (string, error) { return pageToken, nil }, domain.SyncNowInterval)
}

// AutosyncAll syncs every Organization with Autosync on whose last run is
// older than AutosyncInterval. It is the hourly timer's work and is safe to
// call from anywhere: the lease and the interval bound how often Graph is
// called (T10).
func (a *App) AutosyncAll(ctx context.Context) (int, error) {
	var errs []error
	ran := 0
	for _, o := range a.orgs {
		conn, _, err := load[domain.Connection](ctx, a.Docs, connectionKey(o.ID))
		if errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if conn.Autosync == nil || conn.Autosync.NeedsReconnect {
			continue
		}
		_, err = a.runSync(ctx, system, conn, func(ctx context.Context) (string, error) {
			return a.storedToken(ctx, system, conn, "autosync")
		}, domain.AutosyncInterval)
		switch {
		case errors.Is(err, domain.ErrTooSoon), errors.Is(err, domain.ErrConflict):
		case err != nil:
			errs = append(errs, fmt.Errorf("%s: %w", o.ID, err))
		default:
			ran++
		}
	}
	return ran, errors.Join(errs...)
}

// storedToken reads Autosync's token from the secret store, per use (T3).
// Every use is written to the Audit Log first, and the token is not read
// when that fails (T8). It is refused once Meta's data access has expired,
// and the secret name must be one this Organization's Autosync could have
// issued, whatever the connection document says (T11).
func (a *App) storedToken(ctx context.Context, p domain.Principal, conn *domain.Connection, purpose string) (string, error) {
	if a.Secrets == nil || conn.Autosync == nil {
		return "", fmt.Errorf("%w: Autosync is off", domain.ErrInvalidState)
	}
	as := conn.Autosync
	if !a.ownSecretName(conn.OrgID, as.SecretName) {
		a.Log.Error("autosync secret name not of this organization", "org", conn.OrgID, "secret", as.SecretName)
		return "", fmt.Errorf("%w: the stored token's name is not this organization's", domain.ErrInvalidState)
	}
	if as.DataAccessExpiresAt != nil && !a.now().Before(*as.DataAccessExpiresAt) {
		a.markNeedsReconnect(ctx, p, conn.OrgID, "data access expired")
		return "", fmt.Errorf("%w: Meta's data access for the stored token expired; turn Autosync on again", domain.ErrInvalidState)
	}
	if err := a.auditStrict(ctx, p, "autosync.token_used", conn.OrgID, "", map[string]any{"purpose": purpose, "page": conn.PageID}); err != nil {
		return "", fmt.Errorf("stored token not used, the Audit Log is unavailable: %w", err)
	}
	token, err := a.Secrets.Get(ctx, as.SecretName)
	if errors.Is(err, ports.ErrNotFound) {
		a.markNeedsReconnect(ctx, p, conn.OrgID, "secret missing")
		return "", fmt.Errorf("%w: the stored token is gone; turn Autosync on again", domain.ErrInvalidState)
	}
	return token, err
}

// markNeedsReconnect records that Graph refused the stored token.
func (a *App) markNeedsReconnect(ctx context.Context, p domain.Principal, org, why string) {
	_, _, err := mutate(ctx, a.Docs, connectionKey(org), func(c *domain.Connection) error {
		if c.Autosync != nil {
			c.Autosync.NeedsReconnect = true
		}
		return nil
	})
	if err != nil {
		a.Log.Error("mark needs reconnect", "org", org, "err", err)
	}
	a.audit(ctx, p, "autosync.token_rejected", org, "", map[string]any{"why": why})
}

// claimSync takes the Organization's sync lease.
func (a *App) claimSync(ctx context.Context, org, holder string, minInterval time.Duration) error {
	key := syncStatusKey(org)
	st, etag, err := load[domain.SyncStatus](ctx, a.Docs, key)
	if errors.Is(err, domain.ErrNotFound) {
		st, etag = &domain.SyncStatus{OrgID: org}, ""
	} else if err != nil {
		return err
	}
	if err := st.Claim(holder, minInterval, a.now()); err != nil {
		return err
	}
	if _, err := save(ctx, a.Docs, key, st, etag); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return fmt.Errorf("%w: a sync is starting", domain.ErrConflict)
		}
		return err
	}
	return nil
}

func (a *App) finishSync(ctx context.Context, org, holder, by, email string, r domain.SyncResult, runErr error) {
	_, _, err := mutate(ctx, a.Docs, syncStatusKey(org), func(st *domain.SyncStatus) error {
		st.Finish(holder, by, r, runErr, a.now())
		st.LastRunByEmail = email
		return nil
	})
	if err != nil {
		a.Log.Error("sync status", "org", org, "err", err)
	}
}

// runSync runs one Sync of the connected page with a page token held in
// memory for this call only.
//
// The lease is claimed before the token is fetched, so the stored token is
// read only when a run really happens: repeated "Sync now" or /sync calls
// read no secret (T8, T10).
func (a *App) runSync(ctx context.Context, p domain.Principal, conn *domain.Connection, getToken func(context.Context) (string, error), minInterval time.Duration) (domain.SyncResult, error) {
	org := conn.OrgID
	holder := a.newID()
	if err := a.claimSync(ctx, org, holder, minInterval); err != nil {
		return domain.SyncResult{}, err
	}
	by := p.ID()
	if by == "" {
		by = SyncByAutosync
	}
	ctx = context.WithoutCancel(ctx)
	var r domain.SyncResult
	token, err := getToken(ctx)
	if err == nil {
		r, err = a.syncPage(ctx, org, conn.PageID, token)
	}
	email := ""
	if p.User != nil {
		email = p.User.Email
	}
	a.finishSync(ctx, org, holder, by, email, r, err)
	details := map[string]any{"page": conn.PageID, "listed": r.Listed, "added": r.Added, "updated": r.Updated, "binned": r.Binned,
		"conflicts": r.Conflicts, "token": "sign-in"}
	if conn.Autosync != nil {
		details["token"] = "stored" // every use of the stored token is audited (T8)
	}
	if len(r.Errors) > 0 {
		details["errors"] = r.Errors
	}
	if err != nil {
		details["error"] = err.Error()
		a.audit(ctx, p, "sync.failed", org, "", details)
		if errors.Is(err, ports.ErrTokenRejected) && conn.Autosync != nil {
			a.markNeedsReconnect(ctx, p, org, "graph")
		}
		return r, err
	}
	a.audit(ctx, p, "sync.ran", org, "", details)
	return r, nil
}

// syncPage is one Sync run: list, upsert, read missing ones by id, apply
// the gone rule, rebuild the snapshot once.
func (a *App) syncPage(ctx context.Context, org, pageID, token string) (domain.SyncResult, error) {
	var r domain.SyncResult
	now := a.now()
	listed, err := a.Events.PageEvents(ctx, pageID, token)
	if err != nil {
		return r, fmt.Errorf("list events: %w", err)
	}
	// The list succeeded with this token: from here on, Graph's "does not
	// exist or cannot be loaded" for one of the page's events means gone.
	r.Listed = len(listed)
	all, err := a.events(ctx, org)
	if err != nil {
		return r, err
	}
	byFB := map[string]*domain.Event{}
	for _, e := range all {
		if id := e.FacebookID(); id != "" {
			byFB[id] = e
		}
	}
	changed := false
	seen := map[string]bool{}
	for _, fe := range listed {
		seen[fe.ID] = true
		part := toPart(fe, pageID)
		e := byFB[fe.ID]
		switch {
		case e != nil && e.Source == domain.SourceManual:
			if !e.SyncConflict {
				if err := a.flagConflict(ctx, org, e.ID); err != nil {
					r.Errors = append(r.Errors, err.Error())
				}
			}
			r.Conflicts++
		case e != nil:
			goneBack := e.DeletedAt != nil && e.BinReason == domain.BinGone
			if !e.Followed(pageID, now) && e.Facebook.PageID == pageID && !goneBack {
				continue // over: it stays as it was (unless it was wrongly thought gone)
			}
			c, err := a.updateSynced(ctx, org, e.ID, part, fe.CoverURL, &r)
			if err != nil {
				r.Errors = append(r.Errors, fe.ID+": "+err.Error())
			}
			changed = changed || c
		default:
			if lastEnd(part).Before(now) {
				continue // no history backfill
			}
			c, err := a.importNew(ctx, org, part, fe.CoverURL, &r)
			if err != nil {
				r.Errors = append(r.Errors, fe.ID+": "+err.Error())
			}
			changed = changed || c
		}
	}
	for _, e := range all {
		if e.Source != domain.SourceFacebook || seen[e.Facebook.EventID] || e.DeletedAt != nil || !e.Followed(pageID, now) {
			continue
		}
		fe, err := a.Events.Event(ctx, e.Facebook.EventID, token)
		switch {
		case err == nil:
			c, err := a.updateSynced(ctx, org, e.ID, toPart(fe, pageID), fe.CoverURL, &r)
			if err != nil {
				r.Errors = append(r.Errors, e.Facebook.EventID+": "+err.Error())
			}
			changed = changed || c
		case errors.Is(err, ports.ErrEventGone):
			// Decision 11: definite only because the list above succeeded.
			if _, _, err := mutate(ctx, a.Docs, eventKey(org, e.ID), func(ev *domain.Event) error {
				if ev.DeletedAt != nil {
					return nil
				}
				return ev.Bin(domain.BinGone, "sync", a.now())
			}); err != nil {
				r.Errors = append(r.Errors, e.Facebook.EventID+": "+err.Error())
				continue
			}
			a.audit(ctx, system, "event.gone", org, e.ID, map[string]any{"facebookEventId": e.Facebook.EventID, "name": e.Effective().Name.PL})
			r.Binned++
			changed = true
		case errors.Is(err, ports.ErrTokenRejected):
			return r, err
		default:
			r.Errors = append(r.Errors, e.Facebook.EventID+": "+err.Error()) // changes nothing
		}
	}
	if changed {
		if err := a.RebuildSnapshot(ctx, org); err != nil {
			return r, fmt.Errorf("snapshot: %w", err)
		}
	}
	return r, nil
}

func (a *App) flagConflict(ctx context.Context, org, id string) error {
	_, _, err := mutate(ctx, a.Docs, eventKey(org, id), func(e *domain.Event) error {
		e.SyncConflict = true
		return nil
	})
	return err
}

// updateSynced applies Facebook's values to an existing synced Event with
// ETag retry, importing a new cover first. A synced Event in the Bin because
// it was gone comes back when Facebook lists it again.
func (a *App) updateSynced(ctx context.Context, org, id string, part domain.FacebookPart, coverURL string, r *domain.SyncResult) (bool, error) {
	cur, _, err := load[domain.Event](ctx, a.Docs, eventKey(org, id))
	if err != nil {
		return false, err
	}
	if part.CoverID != "" && (cur.Facebook.CoverID != part.CoverID || cur.Facebook.Cover == nil) {
		if img, err := a.importCover(ctx, org, coverURL); err == nil {
			part.Cover = img
		} else {
			r.Errors = append(r.Errors, part.EventID+": cover: "+err.Error())
		}
	}
	changed := false
	_, _, err = mutate(ctx, a.Docs, eventKey(org, id), func(e *domain.Event) error {
		c, err := e.ApplyFacebook(part, a.now())
		changed = c
		if e.DeletedAt != nil && e.BinReason == domain.BinGone {
			changed = true
			return e.Restore("sync", a.now())
		}
		return err
	})
	if err != nil {
		return false, err
	}
	if changed {
		r.Updated++
	}
	return changed, nil
}

// importNew creates an Event for a Facebook Event Sync sees first. The
// create-only index makes one Facebook Event one Event even if two runs race.
func (a *App) importNew(ctx context.Context, org string, part domain.FacebookPart, coverURL string, r *domain.SyncResult) (bool, error) {
	id := a.newID()
	holder, err := a.claimFacebookID(ctx, org, part.EventID, id)
	if err != nil {
		return false, err
	}
	if holder != id {
		ev, _, err := load[domain.Event](ctx, a.Docs, eventKey(org, holder))
		if err != nil {
			return false, err
		}
		if ev.Source == domain.SourceManual {
			r.Conflicts++
			return false, a.flagConflict(ctx, org, holder)
		}
		return a.updateSynced(ctx, org, holder, part, coverURL, r)
	}
	sid, err := a.reserveShortID(ctx, org, id)
	if err != nil {
		return false, err
	}
	if part.CoverID != "" {
		if img, err := a.importCover(ctx, org, coverURL); err == nil {
			part.Cover = img
		} else {
			r.Errors = append(r.Errors, part.EventID+": cover: "+err.Error())
		}
	}
	part.SyncedAt = a.now()
	e := domain.NewSyncedEvent(id, org, sid, part, a.now())
	if _, err := save(ctx, a.Docs, eventKey(org, id), e, ""); err != nil {
		return false, err
	}
	a.audit(ctx, system, "event.imported", org, id, map[string]any{"facebookEventId": part.EventID, "name": part.Name})
	r.Added++
	return true, nil
}

// importCover downloads a Facebook cover and stores it re-encoded (T7).
func (a *App) importCover(ctx context.Context, org, coverURL string) (*domain.Image, error) {
	if coverURL == "" {
		return nil, errors.New("no cover address")
	}
	data, err := a.Events.CoverImage(ctx, coverURL)
	if err != nil {
		return nil, err
	}
	return a.storeImage(ctx, org, ports.ImageCover, data)
}

func lastEnd(f domain.FacebookPart) time.Time {
	var t time.Time
	for _, o := range f.Occurrences {
		if o.End.After(t) {
			t = o.End
		}
	}
	return t
}

// toPart turns what Graph returned into the Event's Facebook part.
func toPart(fe ports.FacebookEvent, pageID string) domain.FacebookPart {
	f := domain.FacebookPart{
		PageID: pageID, EventID: fe.ID, Name: fe.Name, Description: fe.Description,
		Online: fe.Online, Cancelled: fe.Cancelled, TicketURL: fe.TicketURL, CoverID: fe.CoverID, UpdatedTime: fe.Updated,
	}
	if pl := fe.Place; pl != nil {
		var addr []string
		for _, s := range []string{pl.Street, strings.TrimSpace(pl.Zip + " " + pl.City)} {
			if s != "" {
				addr = append(addr, s)
			}
		}
		l := &domain.Location{Name: pl.Name, Address: strings.Join(addr, ", ")}
		if pl.HasCoordinates {
			l.MapURL = "https://www.openstreetmap.org/?mlat=" + strconv.FormatFloat(pl.Latitude, 'f', 6, 64) +
				"&mlon=" + strconv.FormatFloat(pl.Longitude, 'f', 6, 64)
		}
		f.Location = l
	}
	end := func(s, e time.Time) time.Time {
		if e.IsZero() {
			return s
		}
		return e
	}
	if len(fe.Times) > 0 {
		for _, t := range fe.Times {
			f.Occurrences = append(f.Occurrences, domain.Occurrence{FacebookID: t.ID, Start: t.Start.UTC(), End: end(t.Start, t.End).UTC()})
		}
	} else if !fe.Start.IsZero() {
		f.Occurrences = []domain.Occurrence{{FacebookID: fe.ID, Start: fe.Start.UTC(), End: end(fe.Start, fe.End).UTC()}}
	}
	return domain.SanitizeFacebook(f)
}
