package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/filip-be/folk-ms/cms/internal/domain"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

func (a *App) venueMap(ctx context.Context, org string) (map[string]domain.Venue, error) {
	vs, err := loadAll[domain.Venue](ctx, a.Docs, venuesPrefix(org))
	if err != nil {
		return nil, err
	}
	m := make(map[string]domain.Venue, len(vs))
	for _, v := range vs {
		m[v.ID] = *v
	}
	return m, nil
}

// VenueView is a Venue with its version.
type VenueView struct {
	domain.Venue
	ETag string `json:"etag"`
}

// ListVenues lists an Organization's Venues (archived ones last).
func (a *App) ListVenues(ctx context.Context, p domain.Principal, org string) ([]VenueView, error) {
	if _, err := a.authorize(p, org); err != nil {
		return nil, err
	}
	keys, err := a.Docs.List(ctx, venuesPrefix(org))
	if err != nil {
		return nil, err
	}
	out := []VenueView{}
	for _, k := range keys {
		v, etag, err := load[domain.Venue](ctx, a.Docs, k)
		if err != nil {
			return nil, err
		}
		out = append(out, VenueView{Venue: *v, ETag: etag})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Archived != out[j].Archived {
			return !out[i].Archived
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// CreateVenue adds a Venue.
func (a *App) CreateVenue(ctx context.Context, p domain.Principal, org string, v domain.Venue) (VenueView, error) {
	if _, err := a.authorize(p, org); err != nil {
		return VenueView{}, err
	}
	v = v.Normalize()
	v.ID, v.OrgID, v.Archived, v.UpdatedAt, v.UpdatedBy = a.newID(), org, false, a.now(), p.ID()
	if err := v.Validate(); err != nil {
		return VenueView{}, err
	}
	etag, err := save(ctx, a.Docs, venueKey(org, v.ID), v, "")
	if err != nil {
		return VenueView{}, err
	}
	a.audit(ctx, p, "venue.created", org, v.ID, map[string]any{"name": v.Name})
	return VenueView{Venue: v, ETag: etag}, nil
}

// UpdateVenue edits a Venue. The change applies at once to every Occurrence
// that has not happened yet; before it is saved, Occurrences that already
// happened get a frozen copy of the Venue as it was. Affected snapshots are
// rebuilt.
func (a *App) UpdateVenue(ctx context.Context, p domain.Principal, org, id, etag string, in domain.Venue) (VenueView, error) {
	o, err := a.authorize(p, org)
	if err != nil {
		return VenueView{}, err
	}
	old, cur, err := load[domain.Venue](ctx, a.Docs, venueKey(org, id))
	if err != nil {
		return VenueView{}, err
	}
	if etag != cur {
		return VenueView{}, fmt.Errorf("%w: the venue was changed by someone else", domain.ErrConflict)
	}
	in = in.Normalize()
	nv := *old
	nv.Name, nv.Address, nv.MapURL = in.Name, in.Address, in.MapURL
	nv.UpdatedAt, nv.UpdatedBy = a.now(), p.ID()
	if err := nv.Validate(); err != nil {
		return VenueView{}, err
	}
	rebuild := false
	if !old.SameContent(nv) {
		events, err := a.events(ctx, org)
		if err != nil {
			return VenueView{}, err
		}
		now := a.now()
		for _, e := range events {
			if !e.UsesVenue(id) || e.Published == nil || e.DeletedAt != nil {
				continue
			}
			rebuild = true
			if _, _, err := mutate(ctx, a.Docs, eventKey(org, e.ID), func(e *domain.Event) error {
				e.FreezePastVenue(*old, o.Location(), now)
				return nil
			}); err != nil {
				return VenueView{}, err
			}
		}
	}
	newTag, err := save(ctx, a.Docs, venueKey(org, id), nv, etag)
	if err != nil {
		return VenueView{}, err
	}
	a.audit(ctx, p, "venue.updated", org, id, map[string]any{"name": nv.Name, "address": nv.Address, "previous": old.Name + ", " + old.Address})
	if rebuild {
		if err := a.RebuildSnapshot(ctx, org); err != nil {
			return VenueView{}, fmt.Errorf("venue saved but snapshot rebuild failed: %w", err)
		}
	}
	return VenueView{Venue: nv, ETag: newTag}, nil
}

// SetVenueArchived archives or restores a Venue. Archived Venues are hidden
// from the picker but still shown where used.
func (a *App) SetVenueArchived(ctx context.Context, p domain.Principal, org, id string, archived bool) (VenueView, error) {
	if _, err := a.authorize(p, org); err != nil {
		return VenueView{}, err
	}
	v, etag, err := mutate(ctx, a.Docs, venueKey(org, id), func(v *domain.Venue) error {
		v.Archived, v.UpdatedAt, v.UpdatedBy = archived, a.now(), p.ID()
		return nil
	})
	if err != nil {
		return VenueView{}, err
	}
	action := "venue.archived"
	if !archived {
		action = "venue.unarchived"
	}
	a.audit(ctx, p, action, org, id, map[string]any{"name": v.Name})
	return VenueView{Venue: *v, ETag: etag}, nil
}

// SnapshotCacheControl is the Cache-Control of snapshots.
const SnapshotCacheControl = "public, max-age=60"

// RebuildSnapshot rebuilds an Organization's Site snapshot from its
// documents and writes it with If-Match, retrying on concurrent rebuilds.
//
// The snapshot's ETag is read before the Events: a concurrent rebuild that
// wrote a newer snapshot after we read the Events then makes our write fail
// and retry, so a stale build can never overwrite a fresher one.
func (a *App) RebuildSnapshot(ctx context.Context, org string) error {
	o, ok := a.orgByID[org]
	if !ok {
		return fmt.Errorf("%w: organization", domain.ErrNotFound)
	}
	path := SnapshotPath(o.Site.ID)
	for range 8 {
		cur, err := a.Pub.Get(ctx, path)
		etag := cur.ETag
		if errors.Is(err, ports.ErrNotFound) {
			etag = ""
		} else if err != nil {
			return err
		}
		events, err := a.events(ctx, org)
		if err != nil {
			return err
		}
		venues, err := a.venueMap(ctx, org)
		if err != nil {
			return err
		}
		snap := domain.BuildSnapshot(o, events, venues, a.Pub.URL, a.now())
		data, err := json.Marshal(snap)
		if err != nil {
			return err
		}
		_, err = a.Pub.Put(ctx, path, data, "application/json", SnapshotCacheControl, etag)
		if errors.Is(err, ports.ErrPrecondition) {
			continue
		}
		return err
	}
	return fmt.Errorf("%w: snapshot kept changing", domain.ErrConflict)
}

// RebuildAll rebuilds every Organization's snapshot (startup and repair).
func (a *App) RebuildAll(ctx context.Context) error {
	var errs []error
	for _, o := range a.orgs {
		errs = append(errs, a.RebuildSnapshot(ctx, o.ID))
	}
	return errors.Join(errs...)
}
