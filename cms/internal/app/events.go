package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/domain"
)

// EventSummary is a row in the Event list.
type EventSummary struct {
	ID          string        `json:"id"`
	ShortID     string        `json:"shortId"`
	Slug        string        `json:"slug"`
	Source      domain.Source `json:"source"`
	Name        domain.Text   `json:"name"`
	Flags       domain.Flags  `json:"flags"`
	FirstStart  *time.Time    `json:"firstStart,omitempty"`
	NextStart   *time.Time    `json:"nextStart,omitempty"`
	LastEnd     *time.Time    `json:"lastEnd,omitempty"`
	Occurrences int           `json:"occurrences"`
	DeletedAt   *time.Time    `json:"deletedAt,omitempty"`
	BinReason   string        `json:"binReason,omitempty"`
	UpdatedAt   time.Time     `json:"updatedAt"`
	HasCover    bool          `json:"hasCover"`
	FacebookURL string        `json:"facebookUrl,omitempty"`
}

// List views.
const (
	ViewUpcoming = "upcoming"
	ViewPast     = "past"
	ViewHidden   = "hidden"
	ViewBin      = "bin"
)

func summarize(e *domain.Event, pageID string, now time.Time) EventSummary {
	v := e.Effective()
	s := EventSummary{
		ID: e.ID, ShortID: e.ShortID, Slug: e.Slug(), Source: e.Source, Name: v.Name, Flags: e.Flags(pageID),
		DeletedAt: e.DeletedAt, BinReason: e.BinReason, UpdatedAt: e.UpdatedAt, HasCover: v.Cover != nil,
		Occurrences: len(v.Occurrences),
	}
	if id := e.FacebookID(); id != "" {
		s.FacebookURL = domain.FacebookEventURL(id)
	}
	for i := range v.Occurrences {
		o := v.Occurrences[i]
		if s.FirstStart == nil {
			s.FirstStart = &o.Start
		}
		if s.NextStart == nil && !o.End.Before(now) {
			s.NextStart = &o.Start
		}
		s.LastEnd = &o.End
	}
	return s
}

func (a *App) events(ctx context.Context, org string) ([]*domain.Event, error) {
	return loadAll[domain.Event](ctx, a.Docs, eventsPrefix(org))
}

// connectedPage returns the Organization's connected Facebook page id ("" if none).
func (a *App) connectedPage(ctx context.Context, org string) (string, error) {
	c, _, err := load[domain.Connection](ctx, a.Docs, connectionKey(org))
	if errors.Is(err, domain.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return c.PageID, nil
}

// ListEvents lists an Organization's Events in a view: upcoming and past
// leave out Hidden Events and the Bin; hidden and bin show only those.
func (a *App) ListEvents(ctx context.Context, p domain.Principal, org, view string) ([]EventSummary, error) {
	if _, err := a.authorize(p, org); err != nil {
		return nil, err
	}
	all, err := a.events(ctx, org)
	if err != nil {
		return nil, err
	}
	page, err := a.connectedPage(ctx, org)
	if err != nil {
		return nil, err
	}
	now := a.now()
	out := []EventSummary{}
	for _, e := range all {
		s := summarize(e, page, now)
		binned, hidden := e.DeletedAt != nil, e.Hidden
		var in bool
		switch view {
		case ViewBin:
			in = binned
		case ViewHidden:
			in = hidden && !binned
		case ViewUpcoming:
			in = !binned && !hidden && s.NextStart != nil
		case ViewPast:
			in = !binned && !hidden && s.NextStart == nil
		default:
			in = !binned
		}
		if in {
			out = append(out, s)
		}
	}
	at := func(s EventSummary) time.Time {
		switch {
		case view == ViewUpcoming && s.NextStart != nil:
			return *s.NextStart
		case s.LastEnd != nil:
			return *s.LastEnd
		}
		return s.UpdatedAt
	}
	sort.SliceStable(out, func(i, j int) bool {
		if view == ViewUpcoming {
			return at(out[i]).Before(at(out[j]))
		}
		if view == ViewBin {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return at(out[i]).After(at(out[j]))
	})
	return out, nil
}

// EventView is an Event as shown on its screen.
type EventView struct {
	Event       *domain.Event          `json:"event"`
	ETag        string                 `json:"etag"`
	Effective   domain.Effective       `json:"effective"`
	Flags       domain.Flags           `json:"flags"`
	Permalink   map[domain.Lang]string `json:"permalink"`
	FacebookURL string                 `json:"facebookUrl,omitempty"`
}

func (a *App) view(ctx context.Context, o *domain.Organization, e *domain.Event, etag string) (EventView, error) {
	page, err := a.connectedPage(ctx, o.ID)
	if err != nil {
		return EventView{}, err
	}
	slug := e.Slug()
	v := EventView{Event: e, ETag: etag, Effective: e.Effective(), Flags: e.Flags(page),
		Permalink: map[domain.Lang]string{
			domain.PL: o.Permalink(e.ShortID, slug, domain.PL),
			domain.EN: o.Permalink(e.ShortID, slug, domain.EN),
		}}
	if id := e.FacebookID(); id != "" {
		v.FacebookURL = domain.FacebookEventURL(id)
	}
	return v, nil
}

// GetEvent returns one Event.
func (a *App) GetEvent(ctx context.Context, p domain.Principal, org, id string) (EventView, error) {
	o, err := a.authorize(p, org)
	if err != nil {
		return EventView{}, err
	}
	e, etag, err := load[domain.Event](ctx, a.Docs, eventKey(org, id))
	if err != nil {
		return EventView{}, err
	}
	return a.view(ctx, o, e, etag)
}

// reserveShortID draws a shortId and claims it with a create-only index
// document, so two Events of one Organization never share one.
func (a *App) reserveShortID(ctx context.Context, org, eventID string) (string, error) {
	for range 10 {
		sid, err := domain.NewShortID(a.Rand)
		if err != nil {
			return "", err
		}
		_, err = save(ctx, a.Docs, shortIDKey(org, sid), map[string]string{"eventId": eventID}, "")
		if errors.Is(err, domain.ErrConflict) {
			continue
		}
		return sid, err
	}
	return "", fmt.Errorf("%w: no free shortId", domain.ErrConflict)
}

// fbIndex points a Facebook Event id to the one Event that holds it.
type fbIndex struct {
	EventID   string    `json:"eventId"`
	CreatedAt time.Time `json:"createdAt"`
}

// indexTakeover is how old an index document whose Event does not exist
// must be before another Event takes it over: the claimant writes its Event
// right after the index, so a younger one may still be in progress.
const indexTakeover = time.Minute

// claimFacebookID creates the index document for fbID pointing to eventID.
// It returns the Event already holding the id when another one does. An
// index whose Event no longer exists (purged, or a crash between the index
// and the Event) is taken over with If-Match, so two claimants cannot both win.
func (a *App) claimFacebookID(ctx context.Context, org, fbID, eventID string) (holder string, err error) {
	claim := fbIndex{EventID: eventID, CreatedAt: a.now()}
	for range 3 {
		_, err = save(ctx, a.Docs, fbIndexKey(org, fbID), claim, "")
		if !errors.Is(err, domain.ErrConflict) {
			return eventID, err
		}
		idx, etag, err := load[fbIndex](ctx, a.Docs, fbIndexKey(org, fbID))
		if errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if err != nil {
			return "", err
		}
		if idx.EventID == eventID {
			return eventID, nil
		}
		_, _, lerr := load[domain.Event](ctx, a.Docs, eventKey(org, idx.EventID))
		if !errors.Is(lerr, domain.ErrNotFound) {
			return idx.EventID, lerr
		}
		if a.now().Sub(idx.CreatedAt) < indexTakeover {
			return idx.EventID, nil // its Event is probably being written
		}
		_, err = save(ctx, a.Docs, fbIndexKey(org, fbID), claim, etag)
		if !errors.Is(err, domain.ErrConflict) {
			return eventID, err
		}
	}
	return "", fmt.Errorf("%w: Facebook Event %s kept changing hands", domain.ErrConflict, fbID)
}

func (a *App) rebuildAfter(ctx context.Context, org string) error {
	if err := a.RebuildSnapshot(ctx, org); err != nil {
		return fmt.Errorf("event saved but snapshot rebuild failed: %w", err)
	}
	return nil
}

// CreateManualEvent adds a Manual Event (decision 20). Its Facebook Event
// must not be held by another Event of the Organization.
func (a *App) CreateManualEvent(ctx context.Context, p domain.Principal, org string, in domain.ManualInput) (EventView, error) {
	o, err := a.authorize(p, org)
	if err != nil {
		return EventView{}, err
	}
	m, x, err := domain.ValidateManual(in, o.Location())
	if err != nil {
		return EventView{}, err
	}
	id := a.newID()
	holder, err := a.claimFacebookID(ctx, org, m.FacebookEventID, id)
	if err != nil {
		return EventView{}, err
	}
	if holder != id {
		return EventView{}, domain.Problem("facebookUrl", "to wydarzenie na Facebooku jest już w CMS", "this Facebook event is already in the CMS")
	}
	sid, err := a.reserveShortID(ctx, org, id)
	if err != nil {
		return EventView{}, err
	}
	e := domain.NewManualEvent(id, org, sid, m, x, p.ID(), a.now())
	etag, err := save(ctx, a.Docs, eventKey(org, id), e, "")
	if err != nil {
		return EventView{}, err
	}
	a.audit(ctx, p, "event.manual_created", org, id, map[string]any{"name": m.Name.PL, "facebookEventId": m.FacebookEventID})
	if err := a.rebuildAfter(ctx, org); err != nil {
		return EventView{}, err
	}
	return a.view(ctx, o, e, etag)
}

// UpdateManualEvent replaces a Manual Event's content. etag must be the one
// the screen loaded, so two Moderators' edits conflict instead of overwriting.
func (a *App) UpdateManualEvent(ctx context.Context, p domain.Principal, org, id, etag string, in domain.ManualInput) (EventView, error) {
	o, err := a.authorize(p, org)
	if err != nil {
		return EventView{}, err
	}
	m, x, err := domain.ValidateManual(in, o.Location())
	if err != nil {
		return EventView{}, err
	}
	return a.edit(ctx, p, o, id, etag, "event.manual_updated", func(e *domain.Event) error {
		x.WrittenAgainst = nil
		return e.UpdateManual(m, x, p.ID(), a.now())
	})
}

// edit applies a Moderator's change with If-Match on etag and rebuilds the
// snapshot. An empty etag loads the latest version (single-field changes
// that cannot lose another Moderator's edit).
func (a *App) edit(ctx context.Context, p domain.Principal, o *domain.Organization, id, etag, action string, fn func(*domain.Event) error) (EventView, error) {
	var e *domain.Event
	var tag string
	var err error
	if etag == "" {
		e, tag, err = mutate(ctx, a.Docs, eventKey(o.ID, id), fn)
	} else {
		var cur string
		e, cur, err = load[domain.Event](ctx, a.Docs, eventKey(o.ID, id))
		if err != nil {
			return EventView{}, err
		}
		if cur != etag {
			return EventView{}, fmt.Errorf("%w: the event was changed meanwhile", domain.ErrConflict)
		}
		if err := fn(e); err != nil {
			return EventView{}, err
		}
		tag, err = save(ctx, a.Docs, eventKey(o.ID, id), e, etag)
	}
	if err != nil {
		return EventView{}, err
	}
	a.audit(ctx, p, action, o.ID, id, map[string]any{"name": e.Effective().Name.PL})
	if err := a.rebuildAfter(ctx, o.ID); err != nil {
		return EventView{}, err
	}
	return a.view(ctx, o, e, tag)
}

// SetOverride sets, confirms or clears one Override of a synced Event.
func (a *App) SetOverride(ctx context.Context, p domain.Principal, org, id string, in domain.OverrideInput) (EventView, error) {
	o, err := a.authorize(p, org)
	if err != nil {
		return EventView{}, err
	}
	action := "event.override_set"
	switch {
	case in.Follow:
		action = "event.override_cleared"
	case in.Keep:
		action = "event.override_kept"
	}
	return a.edit(ctx, p, o, id, "", action, func(e *domain.Event) error {
		return e.SetOverride(in, p.ID(), a.now())
	})
}

// SetExtras replaces the CMS-only fields of an Event.
func (a *App) SetExtras(ctx context.Context, p domain.Principal, org, id, etag string, in domain.ExtrasInput) (EventView, error) {
	o, err := a.authorize(p, org)
	if err != nil {
		return EventView{}, err
	}
	if etag == "" {
		return EventView{}, fmt.Errorf("%w: missing version", domain.ErrConflict)
	}
	return a.edit(ctx, p, o, id, etag, "event.extras_updated", func(e *domain.Event) error {
		return e.SetExtras(in, p.ID(), a.now())
	})
}

// DismissTextFlag confirms English texts and the Summary (decision 16).
func (a *App) DismissTextFlag(ctx context.Context, p domain.Principal, org, id string) (EventView, error) {
	o, err := a.authorize(p, org)
	if err != nil {
		return EventView{}, err
	}
	return a.edit(ctx, p, o, id, "", "event.text_flag_dismissed", func(e *domain.Event) error {
		return e.DismissTextFlag(p.ID(), a.now())
	})
}

// SetHidden switches an Event's Hide.
func (a *App) SetHidden(ctx context.Context, p domain.Principal, org, id string, hidden bool) (EventView, error) {
	o, err := a.authorize(p, org)
	if err != nil {
		return EventView{}, err
	}
	action := "event.shown"
	if hidden {
		action = "event.hidden"
	}
	return a.edit(ctx, p, o, id, "", action, func(e *domain.Event) error {
		return e.SetHidden(hidden, p.ID(), a.now())
	})
}

// RemoveManualEvent moves a Manual Event to the Bin. Synced Events cannot be
// removed: Hide them, or delete the Facebook Event on Facebook.
func (a *App) RemoveManualEvent(ctx context.Context, p domain.Principal, org, id string) (EventView, error) {
	o, err := a.authorize(p, org)
	if err != nil {
		return EventView{}, err
	}
	return a.edit(ctx, p, o, id, "", "event.deleted", func(e *domain.Event) error {
		if e.Source != domain.SourceManual {
			return fmt.Errorf("%w: only manual events can be removed", domain.ErrInvalidState)
		}
		return e.Bin(domain.BinRemoved, p.ID(), a.now())
	})
}

// RestoreEvent takes an Event out of the Bin (Super Admin only).
func (a *App) RestoreEvent(ctx context.Context, p domain.Principal, org, id string) (EventView, error) {
	if err := requireSuperAdmin(p); err != nil {
		return EventView{}, err
	}
	o, err := a.authorize(p, org)
	if err != nil {
		return EventView{}, err
	}
	return a.edit(ctx, p, o, id, "", "event.restored", func(e *domain.Event) error {
		return e.Restore(p.ID(), a.now())
	})
}

// PurgeBin removes Events that have been in the Bin for longer than the
// retention, with their Social Post records and index documents. The Audit
// Log keeps their history, including Facebook and Instagram URLs. It is
// idempotent and safe to call from anywhere.
func (a *App) PurgeBin(ctx context.Context) (int, error) {
	now := a.now()
	purged := 0
	var errs []error
	for _, o := range a.orgs {
		all, err := a.events(ctx, o.ID)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, e := range all {
			if !e.Purgeable(now) {
				continue
			}
			details := map[string]any{"name": e.Effective().Name.PL, "shortId": e.ShortID, "deletedAt": e.DeletedAt, "reason": e.BinReason}
			if id := e.FacebookID(); id != "" {
				details["facebookEventUrl"] = domain.FacebookEventURL(id)
			}
			if posts, _, err := load[domain.EventPosts](ctx, a.Docs, postsKey(o.ID, e.ID)); err == nil {
				var urls []string
				for _, sp := range posts.Posts {
					if sp.URL != "" {
						urls = append(urls, sp.URL)
					}
				}
				details["socialPostUrls"] = urls
			}
			keys := []string{postsKey(o.ID, e.ID), shortIDKey(o.ID, e.ShortID)}
			if id := e.FacebookID(); id != "" {
				if idx, _, err := load[fbIndex](ctx, a.Docs, fbIndexKey(o.ID, id)); err == nil && idx.EventID == e.ID {
					keys = append(keys, fbIndexKey(o.ID, id))
				}
			}
			// The Event document goes last, so a crash leaves it to be retried.
			for _, k := range keys {
				if err := a.Docs.Delete(ctx, k, ""); err != nil && !errors.Is(storeErr(err), domain.ErrNotFound) {
					errs = append(errs, err)
				}
			}
			if err := a.Docs.Delete(ctx, eventKey(o.ID, e.ID), ""); err != nil && !errors.Is(storeErr(err), domain.ErrNotFound) {
				errs = append(errs, err)
				continue
			}
			a.audit(ctx, system, "event.purged", o.ID, e.ID, details)
			purged++
		}
		if err := a.purgeSocial(ctx, o.ID, now); err != nil {
			errs = append(errs, err)
		}
	}
	return purged, errors.Join(errs...)
}
