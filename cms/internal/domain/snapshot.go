package domain

import (
	"sort"
	"strings"
	"time"
)

// SnapshotSchemaVersion is the contract version the builder writes.
const SnapshotSchemaVersion = 2

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
		SchemaVersion int             `json:"schemaVersion"`
		GeneratedAt   string          `json:"generatedAt"`
		Organization  SnapshotOrg     `json:"organization"`
		Events        []SnapshotEvent `json:"events"`
	}
	SnapshotOrg struct {
		ID       string `json:"id"`
		Name     Text   `json:"name"`
		SiteID   string `json:"siteId"`
		BaseURL  string `json:"baseUrl"`
		Timezone string `json:"timezone"`
	}
	SnapLocation struct {
		Name    string `json:"name"`
		Address string `json:"address,omitempty"`
		MapURL  string `json:"mapUrl,omitempty"`
	}
	SnapPrice struct {
		Text *Text  `json:"text,omitempty"`
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
	SnapOccurrence struct {
		Start            string        `json:"start"`
		End              string        `json:"end"`
		Cancelled        bool          `json:"cancelled"`
		Online           bool          `json:"online,omitempty"`
		Location         *SnapLocation `json:"location,omitempty"`
		FacebookEventURL string        `json:"facebookEventUrl"`
	}
	SnapshotEvent struct {
		ID          string           `json:"id"`
		ShortID     string           `json:"shortId"`
		Slug        string           `json:"slug"`
		Permalink   map[Lang]string  `json:"permalink"`
		Title       Text             `json:"title"`
		Summary     *Text            `json:"summary,omitempty"`
		Description *Text            `json:"description,omitempty"`
		Price       *SnapPrice       `json:"price,omitempty"`
		Cover       *SnapImage       `json:"cover,omitempty"`
		Occurrences []SnapOccurrence `json:"occurrences"`
		UpdatedAt   string           `json:"updatedAt"`
	}
)

func optText(t Text) *Text {
	if t.PL == "" {
		return nil
	}
	return &t
}

// BuildSnapshot builds the Site snapshot of an Organization from its Events:
// every Event not Hidden and not in the Bin, with effective values.
// publicURL turns a public store path into an absolute URL.
func BuildSnapshot(org *Organization, events []*Event, publicURL func(path string) string, now time.Time) Snapshot {
	loc := org.Location()
	s := Snapshot{
		SchemaVersion: SnapshotSchemaVersion,
		GeneratedAt:   now.UTC().Format(time.RFC3339),
		Organization: SnapshotOrg{
			ID: org.ID, Name: org.Name, SiteID: org.Site.ID, BaseURL: org.Site.BaseURL, Timezone: org.Timezone,
		},
		Events: []SnapshotEvent{},
	}
	for _, e := range events {
		if e.OrgID != org.ID || !e.OnSite() {
			continue
		}
		v := e.Effective()
		slug := e.Slug()
		se := SnapshotEvent{
			ID: e.ID, ShortID: e.ShortID, Slug: slug,
			Permalink: map[Lang]string{
				PL: org.Permalink(e.ShortID, slug, PL),
				EN: org.Permalink(e.ShortID, slug, EN),
			},
			Title: v.Name, Summary: optText(v.Summary), Description: optText(v.Description),
			UpdatedAt: e.UpdatedAt.UTC().Format(time.RFC3339),
		}
		if v.Price.PL != "" || v.TicketURL != "" {
			se.Price = &SnapPrice{Text: optText(v.Price), URL: v.TicketURL}
		}
		if c := v.Cover; c != nil && len(c.Variants) > 0 {
			img := &SnapImage{Alt: optText(c.Alt)}
			for _, iv := range c.Variants {
				img.Variants = append(img.Variants, SnapVariant{Width: iv.Width, Height: iv.Height, URL: publicURL(iv.Path)})
			}
			se.Cover = img
		}
		var sl *SnapLocation
		if l := v.Location; l != nil && l.Name != "" {
			sl = &SnapLocation{Name: l.Name, Address: l.Address, MapURL: l.MapURL}
		}
		for _, o := range v.Occurrences {
			fb := o.FacebookID
			if fb == "" {
				fb = e.FacebookID()
			}
			se.Occurrences = append(se.Occurrences, SnapOccurrence{
				Start: o.Start.In(loc).Format(time.RFC3339), End: o.End.In(loc).Format(time.RFC3339),
				Cancelled: o.Cancelled, Online: v.Online, Location: sl, FacebookEventURL: FacebookEventURL(fb),
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
