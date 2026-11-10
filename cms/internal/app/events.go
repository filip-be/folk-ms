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
	ID                 string        `json:"id"`
	ShortID            string        `json:"shortId"`
	Slug               string        `json:"slug"`
	Title              domain.Text   `json:"title"`
	Kind               domain.Kind   `json:"kind"`
	Status             domain.Status `json:"status"`
	UnpublishedChanges bool          `json:"unpublishedChanges"`
	FirstStart         *time.Time    `json:"firstStart,omitempty"`
	NextStart          *time.Time    `json:"nextStart,omitempty"`
	LastEnd            *time.Time    `json:"lastEnd,omitempty"`
	Occurrences        int           `json:"occurrences"`
	DeletedAt          *time.Time    `json:"deletedAt,omitempty"`
	UpdatedAt          time.Time     `json:"updatedAt"`
	HasCover           bool          `json:"hasCover"`
	// FacebookOutOfDate: a Facebook Event may need editing on Facebook.
	FacebookOutOfDate bool `json:"facebookOutOfDate,omitempty"`
	// FacebookDetached: a Facebook Event lost its Occurrence.
	FacebookDetached bool `json:"facebookDetached,omitempty"`
}

// List views.
const (
	ViewUpcoming = "upcoming"
	ViewPast     = "past"
	ViewDrafts   = "drafts"
	ViewBin      = "bin"
)

func summarize(e *domain.Event, loc *time.Location, venues map[string]domain.Venue, now time.Time) EventSummary {
	c := e.Draft
	if e.Published != nil {
		c = *e.Published
	}
	s := EventSummary{
		ID: e.ID, ShortID: e.ShortID, Slug: e.Draft.Slug, Title: e.Draft.Title, Kind: e.Draft.Schedule.Kind,
		Status: e.Status(), UnpublishedChanges: e.HasUnpublishedChanges(), DeletedAt: e.DeletedAt,
		UpdatedAt: e.UpdatedAt, HasCover: e.Draft.Cover != nil,
	}
	s.FacebookOutOfDate, s.FacebookDetached = e.FacebookEventFlags(loc, venues)
	occ := c.Schedule.Occurrences(loc, c.VenueID)
	s.Occurrences = len(occ)
	for i := range occ {
		o := occ[i]
		if s.FirstStart == nil {
			s.FirstStart = &o.Start
		}
		if s.NextStart == nil && !o.Cancelled && !o.End.Before(now) {
			s.NextStart = &o.Start
		}
		s.LastEnd = &o.End
	}
	return s
}

func (a *App) events(ctx context.Context, org string) ([]*domain.Event, error) {
	return loadAll[domain.Event](ctx, a.Docs, eventsPrefix(org))
}

// ListEvents lists an Organization's Events in a view.
func (a *App) ListEvents(ctx context.Context, p domain.Principal, org, view string) ([]EventSummary, error) {
	o, err := a.authorize(p, org)
	if err != nil {
		return nil, err
	}
	all, err := a.events(ctx, org)
	if err != nil {
		return nil, err
	}
	now := a.now()
	venues, err := a.venueMap(ctx, org)
	if err != nil {
		return nil, err
	}
	out := []EventSummary{}
	for _, e := range all {
		s := summarize(e, o.Location(), venues, now)
		var in bool
		switch view {
		case ViewBin:
			in = s.Status == domain.StatusBinned
		case ViewDrafts:
			in = s.Status == domain.StatusDraft
		case ViewUpcoming:
			in = s.Status == domain.StatusPublished && s.NextStart != nil
		case ViewPast:
			in = s.Status == domain.StatusPublished && s.NextStart == nil
		default:
			in = s.Status != domain.StatusBinned
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
		if view == ViewDrafts || view == ViewBin {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return at(out[i]).After(at(out[j]))
	})
	return out, nil
}

// EventView is an Event as shown in the editor.
type EventView struct {
	Event       *domain.Event          `json:"event"`
	ETag        string                 `json:"etag"`
	Status      domain.Status          `json:"status"`
	Occurrences []domain.Occurrence    `json:"occurrences"`
	Problems    []domain.FieldError    `json:"problems"`
	Permalink   map[domain.Lang]string `json:"permalink"`
}

func (a *App) view(ctx context.Context, o *domain.Organization, e *domain.Event, etag string) (EventView, error) {
	v := EventView{Event: e, ETag: etag, Status: e.Status(), Problems: []domain.FieldError{},
		Occurrences: e.Draft.Schedule.Occurrences(o.Location(), e.Draft.VenueID),
		Permalink: map[domain.Lang]string{
			domain.PL: o.Permalink(e.ShortID, e.Draft.Slug, domain.PL),
			domain.EN: o.Permalink(e.ShortID, e.Draft.Slug, domain.EN),
		}}
	if v.Occurrences == nil {
		v.Occurrences = []domain.Occurrence{}
	}
	venues, err := a.venueMap(ctx, o.ID)
	if err != nil {
		return v, err
	}
	var ve *domain.ValidationError
	if err := e.Draft.ValidatePublish(func(id string) bool { _, ok := venues[id]; return ok }); errors.As(err, &ve) {
		v.Problems = ve.Problems
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

// PreviewOccurrences expands a Draft's schedule for the editor.
func (a *App) PreviewOccurrences(p domain.Principal, org string, c domain.Content) ([]domain.Occurrence, []domain.FieldError, error) {
	o, err := a.authorize(p, org)
	if err != nil {
		return nil, nil, err
	}
	var ve *domain.ValidationError
	if err := c.Schedule.Validate(); errors.As(err, &ve) {
		return []domain.Occurrence{}, ve.Problems, nil
	}
	return c.Schedule.Occurrences(o.Location(), c.VenueID), []domain.FieldError{}, nil
}

// slugTaken reports whether another non-binned Event of org uses slug.
func slugTaken(events []*domain.Event, exceptID string) func(string) bool {
	return func(slug string) bool {
		for _, e := range events {
			if e.ID == exceptID || e.DeletedAt != nil {
				continue
			}
			if e.Draft.Slug == slug || (e.Published != nil && e.Published.Slug == slug) {
				return true
			}
		}
		return false
	}
}

func slugConflict(slug string) error {
	return domain.Problem("slug", fmt.Sprintf("adres „%s” jest już używany przez inne wydarzenie", slug),
		fmt.Sprintf("the address “%s” is already used by another event", slug))
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

// CreateEvent creates a Draft Event.
func (a *App) CreateEvent(ctx context.Context, p domain.Principal, org string, c domain.Content) (EventView, error) {
	o, err := a.authorize(p, org)
	if err != nil {
		return EventView{}, err
	}
	all, err := a.events(ctx, org)
	if err != nil {
		return EventView{}, err
	}
	explicit := c.Slug != ""
	c = c.Normalize()
	if err := c.ValidateDraft(); err != nil {
		return EventView{}, err
	}
	taken := slugTaken(all, "")
	if explicit && taken(c.Slug) {
		return EventView{}, slugConflict(c.Slug)
	}
	if !explicit {
		c.Slug = domain.UniqueSlug(c.Slug, taken)
	}
	return a.createEvent(ctx, p, o, c, "", "event.created")
}

func (a *App) createEvent(ctx context.Context, p domain.Principal, o *domain.Organization, c domain.Content, from, action string) (EventView, error) {
	id := a.newID()
	sid, err := a.reserveShortID(ctx, o.ID, id)
	if err != nil {
		return EventView{}, err
	}
	e, err := domain.NewEvent(id, o.ID, sid, c, p.ID(), a.now())
	if err != nil {
		return EventView{}, err
	}
	e.DuplicatedFrom = from
	etag, err := save(ctx, a.Docs, eventKey(o.ID, id), e, "")
	if err != nil {
		return EventView{}, err
	}
	details := map[string]any{"title": e.Draft.Title.PL}
	if from != "" {
		details["from"] = from
	}
	a.audit(ctx, p, action, o.ID, id, details)
	return a.view(ctx, o, e, etag)
}

// UpdateEvent replaces the Draft. etag must be the one the editor loaded,
// so concurrent edits by two Moderators conflict instead of overwriting.
func (a *App) UpdateEvent(ctx context.Context, p domain.Principal, org, id, etag string, c domain.Content) (EventView, error) {
	o, err := a.authorize(p, org)
	if err != nil {
		return EventView{}, err
	}
	if etag == "" {
		return EventView{}, fmt.Errorf("%w: missing version", domain.ErrConflict)
	}
	e, cur, err := load[domain.Event](ctx, a.Docs, eventKey(org, id))
	if err != nil {
		return EventView{}, err
	}
	if cur != etag {
		return EventView{}, fmt.Errorf("%w: the event was changed by someone else", domain.ErrConflict)
	}
	explicit := c.Slug != ""
	c = c.Normalize()
	all, err := a.events(ctx, org)
	if err != nil {
		return EventView{}, err
	}
	taken := slugTaken(all, id)
	if explicit && taken(c.Slug) {
		return EventView{}, slugConflict(c.Slug)
	}
	if !explicit {
		c.Slug = domain.UniqueSlug(c.Slug, taken)
	}
	if err := e.UpdateDraft(c, p.ID(), a.now()); err != nil {
		return EventView{}, err
	}
	newTag, err := save(ctx, a.Docs, eventKey(org, id), e, etag)
	if err != nil {
		return EventView{}, err
	}
	a.audit(ctx, p, "event.updated", org, id, map[string]any{"title": c.Title.PL, "revision": e.DraftRevision})
	return a.view(ctx, o, e, newTag)
}

// transition applies an Event state change and rebuilds the snapshot when
// the Site's content changed.
func (a *App) transition(ctx context.Context, p domain.Principal, org, id, action string, fn func(*domain.Event, map[string]domain.Venue) (rebuild bool, err error)) (EventView, error) {
	o, err := a.authorize(p, org)
	if err != nil {
		return EventView{}, err
	}
	venues, err := a.venueMap(ctx, org)
	if err != nil {
		return EventView{}, err
	}
	rebuild := false
	e, etag, err := mutate(ctx, a.Docs, eventKey(org, id), func(e *domain.Event) error {
		r, err := fn(e, venues)
		rebuild = r
		return err
	})
	if err != nil {
		return EventView{}, err
	}
	a.audit(ctx, p, action, org, id, map[string]any{"title": e.Draft.Title.PL})
	if rebuild {
		if err := a.RebuildSnapshot(ctx, org); err != nil {
			return EventView{}, fmt.Errorf("event saved but snapshot rebuild failed: %w", err)
		}
	}
	return a.view(ctx, o, e, etag)
}

// PublishEvent makes the Draft the version shown on the Site.
func (a *App) PublishEvent(ctx context.Context, p domain.Principal, org, id string) (EventView, error) {
	now := a.now()
	return a.transition(ctx, p, org, id, "event.published", func(e *domain.Event, venues map[string]domain.Venue) (bool, error) {
		return true, e.Publish(func(v string) bool { _, ok := venues[v]; return ok }, p.ID(), now)
	})
}

// UnpublishEvent removes the Event from the Site.
func (a *App) UnpublishEvent(ctx context.Context, p domain.Principal, org, id string) (EventView, error) {
	now := a.now()
	return a.transition(ctx, p, org, id, "event.unpublished", func(e *domain.Event, _ map[string]domain.Venue) (bool, error) {
		return true, e.Unpublish(p.ID(), now)
	})
}

// DeleteEvent moves the Event to the Bin (Unpublishing it first).
func (a *App) DeleteEvent(ctx context.Context, p domain.Principal, org, id string) (EventView, error) {
	now := a.now()
	return a.transition(ctx, p, org, id, "event.deleted", func(e *domain.Event, _ map[string]domain.Venue) (bool, error) {
		return e.Delete(p.ID(), now)
	})
}

// RestoreEvent takes an Event out of the Bin (Super Admin only).
func (a *App) RestoreEvent(ctx context.Context, p domain.Principal, org, id string) (EventView, error) {
	if err := requireSuperAdmin(p); err != nil {
		return EventView{}, err
	}
	all, err := a.events(ctx, org)
	if err != nil {
		return EventView{}, err
	}
	now := a.now()
	return a.transition(ctx, p, org, id, "event.restored", func(e *domain.Event, _ map[string]domain.Venue) (bool, error) {
		if err := e.Restore(p.ID(), now); err != nil {
			return false, err
		}
		taken := slugTaken(all, e.ID)
		if taken(e.Draft.Slug) {
			e.Draft.Slug = domain.UniqueSlug(e.Draft.Slug, taken)
		}
		return false, nil
	})
}

// DuplicateEvent creates a new Draft from an existing Event.
func (a *App) DuplicateEvent(ctx context.Context, p domain.Principal, org, id string) (EventView, error) {
	o, err := a.authorize(p, org)
	if err != nil {
		return EventView{}, err
	}
	src, _, err := load[domain.Event](ctx, a.Docs, eventKey(org, id))
	if err != nil {
		return EventView{}, err
	}
	all, err := a.events(ctx, org)
	if err != nil {
		return EventView{}, err
	}
	d := src.Duplicate("", "", p.ID(), a.now())
	d.Draft.Slug = domain.UniqueSlug(src.Draft.Slug, slugTaken(all, ""))
	return a.createEvent(ctx, p, o, d.Draft, src.ID, "event.duplicated")
}

// PurgeBin removes Events that have been in the Bin for longer than the
// retention, with their Social Post records. The Audit Log keeps their
// history, including Facebook and Instagram URLs. It is idempotent and safe
// to call from anywhere.
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
			details := map[string]any{"title": e.Draft.Title.PL, "shortId": e.ShortID, "deletedAt": e.DeletedAt}
			if len(e.FacebookEvents) > 0 {
				var fb []string
				for _, l := range e.FacebookEvents {
					fb = append(fb, l.URL)
				}
				details["facebookEventUrls"] = fb
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
			// The Event document goes last, so a crash leaves it to be retried.
			for _, k := range []string{postsKey(o.ID, e.ID), shortIDKey(o.ID, e.ShortID)} {
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
