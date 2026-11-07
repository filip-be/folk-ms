package domain

import (
	"sort"
	"strings"
	"time"
)

// SnapshotSchemaVersion is the contract version the builder writes.
const SnapshotSchemaVersion = 1

// Organization is an Organization with its Site, from config/sites.json.
type Organization struct {
	ID       string `json:"id"`
	Name     Text   `json:"name"`
	Timezone string `json:"timezone"`
	Site     Site   `json:"site"`

	loc *time.Location
}

// Site is the static website of an Organization.
type Site struct {
	ID                string   `json:"id"`
	BaseURL           string   `json:"baseUrl"`
	Origins           []string `json:"origins"`
	PermalinkTemplate string   `json:"permalinkTemplate"`
}

// Location returns the Organization's time zone.
func (o *Organization) Location() *time.Location {
	if o.loc == nil {
		loc, err := time.LoadLocation(o.Timezone)
		if err != nil {
			loc = time.UTC
		}
		o.loc = loc
	}
	return o.loc
}

// SetLocation sets the loaded time zone (used by config loading).
func (o *Organization) SetLocation(l *time.Location) { o.loc = l }

// Permalink builds the absolute Permalink of an Event for a language.
func (o *Organization) Permalink(shortID, slug string, l Lang) string {
	p := strings.NewReplacer("{lang}", string(l), "{shortId}", shortID, "{slug}", slug).Replace(o.Site.PermalinkTemplate)
	return strings.TrimRight(o.Site.BaseURL, "/") + p
}

// Snapshot types mirror contract/events.schema.json.
type (
	Snapshot struct {
		SchemaVersion int                  `json:"schemaVersion"`
		GeneratedAt   string               `json:"generatedAt"`
		Organization  SnapshotOrg          `json:"organization"`
		Venues        map[string]SnapVenue `json:"venues"`
		Events        []SnapshotEvent      `json:"events"`
	}
	SnapshotOrg struct {
		ID       string `json:"id"`
		Name     Text   `json:"name"`
		SiteID   string `json:"siteId"`
		BaseURL  string `json:"baseUrl"`
		Timezone string `json:"timezone"`
	}
	SnapVenue struct {
		Name    string `json:"name"`
		Address string `json:"address"`
		MapURL  string `json:"mapUrl,omitempty"`
	}
	SnapPrice struct {
		Text Text   `json:"text"`
		URL  string `json:"url,omitempty"`
	}
	SnapVariant struct {
		Width  int    `json:"width"`
		Height int    `json:"height"`
		URL    string `json:"url"`
	}
	SnapImage struct {
		Alt      *Text         `json:"alt,omitempty"`
		Variants []SnapVariant `json:"variants"`
	}
	SnapRecurrence struct {
		Freq      Freq   `json:"freq"`
		Weekday   int    `json:"weekday"`
		Nth       int    `json:"nth,omitempty"`
		StartDate string `json:"startDate"`
		EndDate   string `json:"endDate"`
	}
	SnapOccurrence struct {
		Start     string `json:"start"`
		End       string `json:"end"`
		Cancelled bool   `json:"cancelled"`
		VenueID   string `json:"venueId"`
		Note      *Text  `json:"note,omitempty"`
	}
	SnapshotEvent struct {
		ID          string           `json:"id"`
		ShortID     string           `json:"shortId"`
		Slug        string           `json:"slug"`
		Permalink   map[Lang]string  `json:"permalink"`
		Kind        Kind             `json:"kind"`
		Title       Text             `json:"title"`
		Summary     *Text            `json:"summary,omitempty"`
		Description *Text            `json:"description,omitempty"`
		Price       *SnapPrice       `json:"price,omitempty"`
		Cover       *SnapImage       `json:"cover,omitempty"`
		Recurrence  *SnapRecurrence  `json:"recurrence,omitempty"`
		Occurrences []SnapOccurrence `json:"occurrences"`
		PublishedAt string           `json:"publishedAt"`
	}
)

func optText(t Text) *Text {
	if t.PL == "" {
		return nil
	}
	return &t
}

// BuildSnapshot builds the Site snapshot of an Organization from its Events
// (only Published, not Binned ones are included) and Venues. publicURL turns
// a public store path into an absolute URL.
func BuildSnapshot(org *Organization, events []*Event, venues map[string]Venue, publicURL func(path string) string, now time.Time) Snapshot {
	loc := org.Location()
	s := Snapshot{
		SchemaVersion: SnapshotSchemaVersion,
		GeneratedAt:   now.UTC().Format(time.RFC3339),
		Organization: SnapshotOrg{
			ID: org.ID, Name: org.Name, SiteID: org.Site.ID, BaseURL: org.Site.BaseURL, Timezone: org.Timezone,
		},
		Venues: map[string]SnapVenue{},
		Events: []SnapshotEvent{},
	}
	addVenue := func(key string, v Venue) {
		s.Venues[key] = SnapVenue{Name: v.Name, Address: v.Address, MapURL: v.MapURL}
	}
	for _, e := range events {
		if e.Published == nil || e.DeletedAt != nil || e.OrgID != org.ID {
			continue
		}
		c := e.Published
		occ := c.Schedule.Occurrences(loc, c.VenueID)
		if len(occ) == 0 {
			continue // cannot happen for a validly Published Event
		}
		se := SnapshotEvent{
			ID: e.ID, ShortID: e.ShortID, Slug: c.Slug,
			Permalink: map[Lang]string{
				PL: org.Permalink(e.ShortID, c.Slug, PL),
				EN: org.Permalink(e.ShortID, c.Slug, EN),
			},
			Kind: c.Schedule.Kind, Title: c.Title,
			Summary: optText(c.Summary), Description: optText(c.Description),
			PublishedAt: e.PublishedAt.UTC().Format(time.RFC3339),
		}
		if c.Price.Text.PL != "" {
			se.Price = &SnapPrice{Text: c.Price.Text, URL: c.Price.URL}
		}
		if c.Cover != nil && len(c.Cover.Variants) > 0 {
			img := &SnapImage{Alt: optText(c.Cover.Alt)}
			for _, v := range c.Cover.Variants {
				img.Variants = append(img.Variants, SnapVariant{Width: v.Width, Height: v.Height, URL: publicURL(v.Path)})
			}
			se.Cover = img
		}
		if r := c.Schedule.Recurrence; c.Schedule.Kind == KindRecurring && r != nil {
			se.Recurrence = &SnapRecurrence{Freq: r.Freq, Weekday: r.Weekday, Nth: r.Nth, StartDate: c.Schedule.StartDate, EndDate: r.EndDate}
		}
		for _, o := range occ {
			venueKey := o.VenueID
			if frozen, ok := e.PastVenues[o.Key]; ok && frozen.ID == o.VenueID {
				venueKey = frozen.frozenKey()
				addVenue(venueKey, frozen)
			} else if v, ok := venues[o.VenueID]; ok {
				addVenue(venueKey, v)
			}
			se.Occurrences = append(se.Occurrences, SnapOccurrence{
				Start: o.Start.Format(time.RFC3339), End: o.End.Format(time.RFC3339),
				Cancelled: o.Cancelled, VenueID: venueKey, Note: optText(o.Note),
			})
		}
		s.Events = append(s.Events, se)
	}
	sort.Slice(s.Events, func(i, j int) bool {
		a, b := s.Events[i].Occurrences[0].Start, s.Events[j].Occurrences[0].Start
		if a != b {
			return a < b
		}
		return s.Events[i].ID < s.Events[j].ID
	})
	return s
}
