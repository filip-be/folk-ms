package domain

import (
	"fmt"
	"io"
	"time"
)

// BinRetention is how long an Event stays in the Bin before it is purged.
const BinRetention = 30 * 24 * time.Hour

// Text length limits.
const (
	MaxTitleLen       = 200
	MaxSummaryLen     = 500
	MaxDescriptionLen = 10000
	MaxPriceLen       = 500
)

// Price is the free-text cost of attending with an optional tickets link.
type Price struct {
	Text Text   `json:"text,omitzero"`
	URL  string `json:"url,omitempty"`
}

// ImageVariant is one rendition of an image in the public store.
type ImageVariant struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Path   string `json:"path"` // relative to the public store root
}

// Image is an uploaded Cover Image or Social Image with its JPEG variants.
// Instagram is the variant cropped to an aspect ratio Instagram accepts.
type Image struct {
	ID        string         `json:"id"`
	Alt       Text           `json:"alt,omitzero"`
	Variants  []ImageVariant `json:"variants"`
	Instagram *ImageVariant  `json:"instagram,omitempty"`
}

// Content is everything a Moderator authors about an Event.
type Content struct {
	Slug        string   `json:"slug"`
	Title       Text     `json:"title"`
	Summary     Text     `json:"summary,omitzero"`
	Description Text     `json:"description,omitzero"`
	Schedule    Schedule `json:"schedule"`
	VenueID     string   `json:"venueId,omitempty"`
	Price       Price    `json:"price,omitzero"`
	Cover       *Image   `json:"cover,omitempty"`
	SocialImage *Image   `json:"socialImage,omitempty"`
}

// Normalize trims texts and fills an empty slug from the Polish title.
func (c Content) Normalize() Content {
	c.Title = c.Title.trimmed()
	c.Summary = c.Summary.trimmed()
	c.Description = c.Description.trimmed()
	c.Price.Text = c.Price.Text.trimmed()
	if c.Slug == "" {
		c.Slug = Slugify(c.Title.PL)
	}
	return c
}

// ValidateDraft checks only what must hold even for an unfinished Draft.
func (c Content) ValidateDraft() error {
	var p problems
	c.validateShape(&p)
	return p.err()
}

func (c Content) validateShape(p *problems) {
	if c.Slug != "" && !IsSlug(c.Slug) {
		p.add("slug", "adres może zawierać tylko małe litery, cyfry i pojedyncze myślniki", "the address may contain only lowercase letters, digits and single hyphens")
	}
	checkLen := func(field string, t Text, max int) {
		if len([]rune(t.PL)) > max || len([]rune(t.EN)) > max {
			p.add(field, "tekst jest za długi (maks. %d znaków)", "the text is too long (max. %d characters)", max)
		}
	}
	checkLen("title", c.Title, MaxTitleLen)
	checkLen("summary", c.Summary, MaxSummaryLen)
	checkLen("description", c.Description, MaxDescriptionLen)
	checkLen("price.text", c.Price.Text, MaxPriceLen)
	if c.Price.URL != "" && !isHTTPURL(c.Price.URL) {
		p.add("price.url", "podaj adres zaczynający się od https://", "enter an address starting with https://")
	}
	switch c.Schedule.Kind {
	case KindSingle, KindMultiDay, KindRecurring, "":
	default:
		p.add("schedule.kind", "nieznany rodzaj wydarzenia", "unknown event type")
	}
	for _, f := range []struct {
		field string
		t     Text
	}{{"title", c.Title}, {"summary", c.Summary}, {"description", c.Description}, {"price.text", c.Price.Text}} {
		if f.t.PL == "" && f.t.EN != "" {
			p.add(f.field+".pl", "tekst angielski wymaga polskiego", "English text requires Polish text")
		}
	}
}

// ValidatePublish checks everything required to Publish. venueOK reports
// whether a Venue id exists in the Organization.
func (c Content) ValidatePublish(venueOK func(id string) bool) error {
	var p problems
	c.validateShape(&p)
	if c.Title.PL == "" {
		p.add("title.pl", "tytuł po polsku jest wymagany", "a Polish title is required")
	}
	if c.Slug == "" {
		p.add("slug", "adres jest wymagany", "the address is required")
	}
	if c.VenueID == "" {
		p.add("venueId", "wybierz miejsce", "choose a venue")
	} else if !venueOK(c.VenueID) {
		p.add("venueId", "wybrane miejsce nie istnieje", "the chosen venue does not exist")
	}
	for i, id := range c.Schedule.venueIDs() {
		if !venueOK(id) {
			p.add(fmt.Sprintf("schedule.exceptions[%d].venueId", i), "wybrane miejsce nie istnieje", "the chosen venue does not exist")
		}
	}
	c.Schedule.validate(&p)
	return p.err()
}

// Event is the aggregate. Draft is always editable; Published is the version
// the Site shows (nil when not Published).
type Event struct {
	ID          string     `json:"id"`
	OrgID       string     `json:"orgId"`
	ShortID     string     `json:"shortId"`
	Draft       Content    `json:"draft"`
	Published   *Content   `json:"published,omitempty"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
	DeletedAt   *time.Time `json:"deletedAt,omitempty"`
	DeletedBy   string     `json:"deletedBy,omitempty"`
	// PastVenues holds frozen copies of Venues as they were for Occurrences
	// that had already happened when the Venue was edited, keyed by
	// Occurrence key.
	PastVenues     map[string]Venue `json:"pastVenues,omitempty"`
	DuplicatedFrom string           `json:"duplicatedFrom,omitempty"`
	// FacebookEvents are links to Facebook Events created by hand on
	// Facebook, each kept against the Occurrence it announces.
	FacebookEvents []FacebookEventLink `json:"facebookEvents,omitempty"`
	CreatedAt      time.Time           `json:"createdAt"`
	CreatedBy      string              `json:"createdBy"`
	UpdatedAt      time.Time           `json:"updatedAt"`
	UpdatedBy      string              `json:"updatedBy"`
	// Revisions let the UI tell whether the Draft has unpublished changes.
	DraftRevision     int `json:"draftRevision"`
	PublishedRevision int `json:"publishedRevision,omitempty"`
}

// Status is the Event's lifecycle state.
type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusBinned    Status = "binned"
)

// Status derives the lifecycle state.
func (e *Event) Status() Status {
	switch {
	case e.DeletedAt != nil:
		return StatusBinned
	case e.Published != nil:
		return StatusPublished
	}
	return StatusDraft
}

// HasUnpublishedChanges reports whether a Published Event's Draft differs
// from what the Site shows.
func (e *Event) HasUnpublishedChanges() bool {
	return e.Published != nil && e.DraftRevision != e.PublishedRevision
}

// NewEvent creates a Draft Event.
func NewEvent(id, orgID, shortID string, c Content, by string, now time.Time) (*Event, error) {
	c = c.Normalize()
	if err := c.ValidateDraft(); err != nil {
		return nil, err
	}
	return &Event{
		ID: id, OrgID: orgID, ShortID: shortID, Draft: c,
		CreatedAt: now, CreatedBy: by, UpdatedAt: now, UpdatedBy: by, DraftRevision: 1,
	}, nil
}

// UpdateDraft replaces the Draft. The Site keeps the Published version.
func (e *Event) UpdateDraft(c Content, by string, now time.Time) error {
	if e.DeletedAt != nil {
		return fmt.Errorf("%w: event is in the Bin", ErrInvalidState)
	}
	c = c.Normalize()
	if err := c.ValidateDraft(); err != nil {
		return err
	}
	e.Draft = c
	e.DraftRevision++
	e.UpdatedAt, e.UpdatedBy = now, by
	return nil
}

// Publish makes the current Draft the version the Site shows.
func (e *Event) Publish(venueOK func(string) bool, by string, now time.Time) error {
	if e.DeletedAt != nil {
		return fmt.Errorf("%w: event is in the Bin", ErrInvalidState)
	}
	if err := e.Draft.ValidatePublish(venueOK); err != nil {
		return err
	}
	c := e.Draft
	e.Published = &c
	e.PublishedAt = &now
	e.PublishedRevision = e.DraftRevision
	e.UpdatedAt, e.UpdatedBy = now, by
	return nil
}

// Unpublish removes the Event from the Site; it stays as a Draft.
func (e *Event) Unpublish(by string, now time.Time) error {
	if e.Published == nil {
		return fmt.Errorf("%w: event is not published", ErrInvalidState)
	}
	e.Published = nil
	e.PublishedAt = nil
	e.PublishedRevision = 0
	e.UpdatedAt, e.UpdatedBy = now, by
	return nil
}

// Delete moves the Event to the Bin, Unpublishing it first. It reports
// whether the Event was Published (so the snapshot must be rebuilt).
func (e *Event) Delete(by string, now time.Time) (wasPublished bool, err error) {
	if e.DeletedAt != nil {
		return false, fmt.Errorf("%w: already in the Bin", ErrInvalidState)
	}
	wasPublished = e.Published != nil
	if wasPublished {
		_ = e.Unpublish(by, now)
	}
	e.DeletedAt, e.DeletedBy = &now, by
	e.UpdatedAt, e.UpdatedBy = now, by
	return wasPublished, nil
}

// Restore takes the Event out of the Bin as a Draft.
func (e *Event) Restore(by string, now time.Time) error {
	if e.DeletedAt == nil {
		return fmt.Errorf("%w: event is not in the Bin", ErrInvalidState)
	}
	if now.Sub(*e.DeletedAt) > BinRetention {
		return fmt.Errorf("%w: retention expired", ErrInvalidState)
	}
	e.DeletedAt, e.DeletedBy = nil, ""
	e.UpdatedAt, e.UpdatedBy = now, by
	return nil
}

// Purgeable reports whether the Event has been in the Bin long enough.
func (e *Event) Purgeable(now time.Time) bool {
	return e.DeletedAt != nil && now.Sub(*e.DeletedAt) > BinRetention
}

// Duplicate creates a new Draft from e, copying its content but not its
// dates or Social Posts. It copies the Draft (what the Moderator sees in the
// editor); the caller sets a unique slug.
func (e *Event) Duplicate(id, shortID, by string, now time.Time) *Event {
	c := e.Draft
	c.Schedule = c.Schedule.clearDates()
	return &Event{
		ID: id, OrgID: e.OrgID, ShortID: shortID, Draft: c, DuplicatedFrom: e.ID,
		CreatedAt: now, CreatedBy: by, UpdatedAt: now, UpdatedBy: by, DraftRevision: 1,
	}
}

// UsesVenue reports whether any version of the Event references the Venue.
func (e *Event) UsesVenue(id string) bool {
	for _, c := range []*Content{&e.Draft, e.Published} {
		if c == nil {
			continue
		}
		if c.VenueID == id {
			return true
		}
		for _, v := range c.Schedule.venueIDs() {
			if v == id {
				return true
			}
		}
	}
	return false
}

// FreezePastVenue records old as the Venue of every Published Occurrence
// that uses it and has already ended at now, unless one is recorded already.
// It reports whether anything changed.
func (e *Event) FreezePastVenue(old Venue, loc *time.Location, now time.Time) bool {
	if e.Published == nil {
		return false
	}
	changed := false
	for _, o := range e.Published.Schedule.Occurrences(loc, e.Published.VenueID) {
		if o.VenueID != old.ID || !o.End.Before(now) {
			continue
		}
		if _, ok := e.PastVenues[o.Key]; ok {
			continue
		}
		if e.PastVenues == nil {
			e.PastVenues = map[string]Venue{}
		}
		e.PastVenues[o.Key] = old
		changed = true
	}
	return changed
}

// UniqueSlug returns base, or base with the smallest numeric suffix that is
// not taken.
func UniqueSlug(base string, taken func(string) bool) string {
	if base == "" {
		base = "wydarzenie"
	}
	if !taken(base) {
		return base
	}
	for i := 2; ; i++ {
		suffix := fmt.Sprintf("-%d", i)
		b := base
		if len(b)+len(suffix) > MaxSlugLen {
			b = b[:MaxSlugLen-len(suffix)]
		}
		if s := b + suffix; !taken(s) {
			return s
		}
	}
}

// GenerateShortID draws shortIds until one is not taken.
func GenerateShortID(r io.Reader, taken func(string) bool) (string, error) {
	for range 20 {
		id, err := NewShortID(r)
		if err != nil {
			return "", err
		}
		if !taken(id) {
			return id, nil
		}
	}
	return "", fmt.Errorf("%w: could not find a free shortId", ErrConflict)
}
