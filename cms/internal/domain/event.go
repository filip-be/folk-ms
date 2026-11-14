package domain

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
)

// BinRetention is how long an Event stays in the Bin before it is purged.
const BinRetention = 30 * 24 * time.Hour

// FollowFor is how long after its last Occurrence ends an Event is still
// synced; after that it stays as it was.
const FollowFor = 24 * time.Hour

// Text length limits.
const (
	MaxTitleLen       = 200
	MaxSummaryLen     = 500
	MaxDescriptionLen = 10000
	MaxPriceLen       = 500
	MaxLocationName   = 200
	MaxLocationAddr   = 500
	MaxOccurrences    = 100
)

// Source is where an Event comes from.
type Source string

const (
	SourceFacebook Source = "facebook" // Sync
	SourceManual   Source = "manual"   // a Manual Event
)

// Bin reasons.
const (
	BinGone    = "gone"    // Facebook confirmed the Facebook Event is gone
	BinRemoved = "removed" // a Moderator removed a Manual Event
)

// ImageVariant is one rendition of an image in the public store.
type ImageVariant struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Path   string `json:"path"` // relative to the public store root
}

// Image is a Cover Image or Social Image with its JPEG variants. Instagram
// is the variant cropped to an aspect ratio Instagram accepts.
type Image struct {
	ID        string         `json:"id"`
	Alt       Text           `json:"alt,omitzero"`
	Variants  []ImageVariant `json:"variants"`
	Instagram *ImageVariant  `json:"instagram,omitempty"`
}

// Location is where an Occurrence happens, as free text.
type Location struct {
	Name    string `json:"name"`
	Address string `json:"address,omitempty"`
	MapURL  string `json:"mapUrl,omitempty"`
}

// Occurrence is one date of an Event. FacebookID is the Facebook Event of
// that date (the Event's own one for a Manual Event).
type Occurrence struct {
	FacebookID string    `json:"facebookId,omitempty"`
	Start      time.Time `json:"start"`
	// End equals Start when the end is not known.
	End       time.Time `json:"end"`
	Cancelled bool      `json:"cancelled,omitempty"`
}

// FacebookPart is what Sync last read from Facebook. Only Sync writes it.
type FacebookPart struct {
	PageID      string       `json:"pageId"`
	EventID     string       `json:"eventId"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Location    *Location    `json:"location,omitempty"`
	Online      bool         `json:"online,omitempty"`
	Cancelled   bool         `json:"cancelled,omitempty"`
	TicketURL   string       `json:"ticketUrl,omitempty"`
	CoverID     string       `json:"coverId,omitempty"` // Facebook's id of the cover photo
	Cover       *Image       `json:"cover,omitempty"`   // the imported copy
	Occurrences []Occurrence `json:"occurrences"`
	UpdatedTime time.Time    `json:"updatedTime"`
	SyncedAt    time.Time    `json:"syncedAt"`
}

// TextOverride replaces a Facebook text. Against is the Facebook value it was
// set or last confirmed against.
type TextOverride struct {
	Value   string `json:"value"`
	Against string `json:"against"`
}

// LocationOverride replaces the Facebook location.
type LocationOverride struct {
	Value   *Location `json:"value,omitempty"`
	Against *Location `json:"against,omitempty"`
}

// CoverOverride replaces the Facebook cover; Against is the Facebook cover id.
type CoverOverride struct {
	Value   *Image `json:"value,omitempty"`
	Against string `json:"against"`
}

// Overrides are a Moderator's replacements of Facebook-sourced fields.
type Overrides struct {
	Name        *TextOverride     `json:"name,omitempty"`
	Description *TextOverride     `json:"description,omitempty"`
	Location    *LocationOverride `json:"location,omitempty"`
	Cover       *CoverOverride    `json:"cover,omitempty"`
	TicketURL   *TextOverride     `json:"ticketUrl,omitempty"`
}

// Override field names.
const (
	FieldName        = "name"
	FieldDescription = "description"
	FieldLocation    = "location"
	FieldCover       = "cover"
	FieldTicketURL   = "ticketUrl"
)

// OverrideFields lists the fields a Moderator may override.
var OverrideFields = []string{FieldName, FieldDescription, FieldLocation, FieldCover, FieldTicketURL}

// Extras are what the CMS adds to an Event (decision 6).
type Extras struct {
	NameEN        string `json:"nameEn,omitempty"`
	DescriptionEN string `json:"descriptionEn,omitempty"`
	Summary       Text   `json:"summary,omitzero"`
	Price         Text   `json:"price,omitzero"`
	SocialImage   *Image `json:"socialImage,omitempty"`
	// WrittenAgainst is the Facebook name and description the English texts
	// and the Summary were written (or last confirmed) against.
	WrittenAgainst *WrittenAgainst `json:"writtenAgainst,omitempty"`
}

// WrittenAgainst is a snapshot of the Facebook texts.
type WrittenAgainst struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (x Extras) hasTranslatedText() bool {
	return x.NameEN != "" || x.DescriptionEN != "" || !x.Summary.IsZero()
}

// ManualPart is a Manual Event's content, entered by a Moderator.
type ManualPart struct {
	Name            Text         `json:"name"`
	Description     Text         `json:"description,omitzero"`
	Location        *Location    `json:"location,omitempty"`
	Online          bool         `json:"online,omitempty"`
	Cover           *Image       `json:"cover,omitempty"`
	TicketURL       string       `json:"ticketUrl,omitempty"`
	FacebookEventID string       `json:"facebookEventId"`
	Occurrences     []Occurrence `json:"occurrences"`
}

// Event is the aggregate: one Facebook Event of an Organization (synced) or
// a Manual Event.
type Event struct {
	ID       string        `json:"id"`
	OrgID    string        `json:"orgId"`
	ShortID  string        `json:"shortId"`
	Source   Source        `json:"source"`
	Facebook *FacebookPart `json:"facebook,omitempty"`
	Manual   *ManualPart   `json:"manual,omitempty"`
	Override Overrides     `json:"overrides,omitzero"`
	Extras   Extras        `json:"extras,omitzero"`
	Hidden   bool          `json:"hidden,omitempty"`
	// SyncConflict marks a Manual Event whose Facebook Event Sync found on
	// the connected page: it stays manual and the Event list flags it.
	SyncConflict bool       `json:"syncConflict,omitempty"`
	DeletedAt    *time.Time `json:"deletedAt,omitempty"`
	DeletedBy    string     `json:"deletedBy,omitempty"`
	BinReason    string     `json:"binReason,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	CreatedBy    string     `json:"createdBy"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	UpdatedBy    string     `json:"updatedBy"`
}

// FacebookID is the id of the Event's (first) Facebook Event.
func (e *Event) FacebookID() string {
	if e.Manual != nil {
		return e.Manual.FacebookEventID
	}
	if e.Facebook != nil {
		return e.Facebook.EventID
	}
	return ""
}

// Effective are the values the Site shows: an Override, else Facebook; the
// Moderator's values for a Manual Event; English and the Summary from Extras.
type Effective struct {
	Name        Text         `json:"name"`
	Description Text         `json:"description"`
	Summary     Text         `json:"summary"`
	Price       Text         `json:"price"`
	Location    *Location    `json:"location,omitempty"`
	Online      bool         `json:"online"`
	Cancelled   bool         `json:"cancelled"`
	TicketURL   string       `json:"ticketUrl,omitempty"`
	Cover       *Image       `json:"cover,omitempty"`
	SocialImage *Image       `json:"socialImage,omitempty"`
	Occurrences []Occurrence `json:"occurrences"`
}

// Effective computes the values the Site shows.
func (e *Event) Effective() Effective {
	v := Effective{Summary: e.Extras.Summary, Price: e.Extras.Price, SocialImage: e.Extras.SocialImage}
	switch {
	case e.Manual != nil:
		m := e.Manual
		v.Name, v.Description = m.Name, m.Description
		v.Location, v.Online, v.TicketURL, v.Cover = m.Location, m.Online, m.TicketURL, m.Cover
		v.Occurrences = slices.Clone(m.Occurrences)
	case e.Facebook != nil:
		f, o := e.Facebook, e.Override
		v.Name = Text{PL: f.Name, EN: e.Extras.NameEN}
		v.Description = Text{PL: f.Description, EN: e.Extras.DescriptionEN}
		v.Location, v.Online, v.Cancelled, v.TicketURL, v.Cover = f.Location, f.Online, f.Cancelled, f.TicketURL, f.Cover
		if o.Name != nil {
			v.Name.PL = o.Name.Value
		}
		if o.Description != nil {
			v.Description.PL = o.Description.Value
		}
		if o.Location != nil {
			v.Location = o.Location.Value
		}
		if o.TicketURL != nil {
			v.TicketURL = o.TicketURL.Value
		}
		if o.Cover != nil {
			v.Cover = o.Cover.Value
		}
		v.Occurrences = slices.Clone(f.Occurrences)
		if f.Cancelled {
			for i := range v.Occurrences {
				v.Occurrences[i].Cancelled = true
			}
		}
	}
	if v.Description.PL == "" && v.Description.EN != "" {
		v.Description.PL = v.Description.EN // never English without Polish
	}
	if v.Name.PL == "" {
		v.Name.PL = v.Name.EN
	}
	slices.SortStableFunc(v.Occurrences, func(a, b Occurrence) int { return a.Start.Compare(b.Start) })
	if len(v.Occurrences) > 0 {
		all := true
		for _, oc := range v.Occurrences {
			all = all && oc.Cancelled
		}
		v.Cancelled = v.Cancelled || all
	}
	return v
}

// Slug is the readable part of the Permalink, from the effective Polish name.
// It may change; the shortId keeps the Permalink stable.
func (e *Event) Slug() string {
	s := Slugify(e.Effective().Name.PL)
	if s == "" {
		s = "wydarzenie"
	}
	return s
}

// LastEnd returns the end of the last Occurrence (zero when none).
func (e *Event) LastEnd() time.Time {
	var t time.Time
	for _, o := range e.Effective().Occurrences {
		if o.End.After(t) {
			t = o.End
		}
	}
	return t
}

// Followed reports whether Sync still updates the Event: synced from the
// connected Facebook page and not over for longer than FollowFor. (Sync
// skips Events in the Bin separately.)
func (e *Event) Followed(pageID string, now time.Time) bool {
	if e.Source != SourceFacebook || e.Facebook == nil || e.Facebook.PageID != pageID {
		return false
	}
	end := e.LastEnd()
	return end.IsZero() || now.Before(end.Add(FollowFor))
}

// OnSite reports whether the Site shows the Event.
func (e *Event) OnSite() bool {
	return e.DeletedAt == nil && !e.Hidden && len(e.Effective().Occurrences) > 0
}

// Flags are what the Event list marks.
type Flags struct {
	Hidden    bool `json:"hidden,omitempty"`
	Cancelled bool `json:"cancelled,omitempty"`
	Manual    bool `json:"manual,omitempty"`
	// OverrideChanged lists overridden fields Facebook changed since.
	OverrideChanged []string `json:"overrideChanged,omitempty"`
	// TextOutOfDate: English or the Summary were written against Facebook
	// text that has changed since (decision 16).
	TextOutOfDate bool `json:"textOutOfDate,omitempty"`
	// FormerPage: synced from a Facebook page no longer connected.
	FormerPage   bool `json:"formerPage,omitempty"`
	SyncConflict bool `json:"syncConflict,omitempty"`
	Binned       bool `json:"binned,omitempty"`
}

// OverrideChanged lists overridden fields whose Facebook value changed since
// the Override was set or confirmed.
func (e *Event) OverrideChanged() []string {
	f, o := e.Facebook, e.Override
	if f == nil {
		return nil
	}
	var out []string
	if o.Name != nil && o.Name.Against != f.Name {
		out = append(out, FieldName)
	}
	if o.Description != nil && o.Description.Against != f.Description {
		out = append(out, FieldDescription)
	}
	if o.Location != nil && !sameLocation(o.Location.Against, f.Location) {
		out = append(out, FieldLocation)
	}
	if o.Cover != nil && o.Cover.Against != f.CoverID {
		out = append(out, FieldCover)
	}
	if o.TicketURL != nil && o.TicketURL.Against != f.TicketURL {
		out = append(out, FieldTicketURL)
	}
	return out
}

// TextOutOfDate reports decision 16's flag.
func (e *Event) TextOutOfDate() bool {
	f, x := e.Facebook, e.Extras
	if f == nil || x.WrittenAgainst == nil || !x.hasTranslatedText() {
		return false
	}
	return x.WrittenAgainst.Name != f.Name || x.WrittenAgainst.Description != f.Description
}

// Flags computes the list flags; pageID is the connected Facebook page ("" if none).
func (e *Event) Flags(pageID string) Flags {
	return Flags{
		Hidden: e.Hidden, Cancelled: e.Effective().Cancelled, Manual: e.Source == SourceManual,
		OverrideChanged: e.OverrideChanged(), TextOutOfDate: e.TextOutOfDate(),
		FormerPage:   e.Source == SourceFacebook && e.Facebook != nil && e.Facebook.PageID != pageID,
		SyncConflict: e.SyncConflict, Binned: e.DeletedAt != nil,
	}
}

func sameLocation(a, b *Location) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func (e *Event) touch(by string, now time.Time) { e.UpdatedAt, e.UpdatedBy = now, by }

// --- Moderator edits ------------------------------------------------------

// OverrideInput sets or clears one Override. Follow true clears it ("Follow
// Facebook again"); Keep true confirms it against the current Facebook value.
type OverrideInput struct {
	Field    string    `json:"field"`
	Follow   bool      `json:"follow,omitempty"`
	Keep     bool      `json:"keep,omitempty"`
	Text     string    `json:"text,omitempty"`
	Location *Location `json:"location,omitempty"`
	Cover    *Image    `json:"cover,omitempty"`
}

// SetOverride applies one OverrideInput to a synced Event.
func (e *Event) SetOverride(in OverrideInput, by string, now time.Time) error {
	if e.Source != SourceFacebook || e.Facebook == nil {
		return fmt.Errorf("%w: only synced events have overrides", ErrInvalidState)
	}
	if e.DeletedAt != nil {
		return fmt.Errorf("%w: event is in the Bin", ErrInvalidState)
	}
	f, o := e.Facebook, &e.Override
	var p problems
	switch in.Field {
	case FieldName:
		switch {
		case in.Follow:
			o.Name = nil
		case in.Keep && o.Name != nil:
			o.Name.Against = f.Name
		default:
			v := cleanText(in.Text, MaxTitleLen+1)
			if v == "" {
				p.add("name", "nazwa jest wymagana", "the name is required")
			} else if len([]rune(v)) > MaxTitleLen {
				p.add("name", "tekst jest za długi (maks. %d znaków)", "the text is too long (max. %d characters)", MaxTitleLen)
			}
			o.Name = &TextOverride{Value: v, Against: f.Name}
		}
	case FieldDescription:
		switch {
		case in.Follow:
			o.Description = nil
		case in.Keep && o.Description != nil:
			o.Description.Against = f.Description
		default:
			v := cleanText(in.Text, MaxDescriptionLen+1)
			if len([]rune(v)) > MaxDescriptionLen {
				p.add("description", "tekst jest za długi (maks. %d znaków)", "the text is too long (max. %d characters)", MaxDescriptionLen)
			}
			o.Description = &TextOverride{Value: v, Against: f.Description}
		}
	case FieldLocation:
		switch {
		case in.Follow:
			o.Location = nil
		case in.Keep && o.Location != nil:
			o.Location.Against = f.Location
		default:
			l, ok := validLocation(in.Location, "location", &p)
			if ok {
				o.Location = &LocationOverride{Value: l, Against: f.Location}
			}
		}
	case FieldCover:
		switch {
		case in.Follow:
			o.Cover = nil
		case in.Keep && o.Cover != nil:
			o.Cover.Against = f.CoverID
		default:
			o.Cover = &CoverOverride{Value: in.Cover, Against: f.CoverID}
		}
	case FieldTicketURL:
		switch {
		case in.Follow:
			o.TicketURL = nil
		case in.Keep && o.TicketURL != nil:
			o.TicketURL.Against = f.TicketURL
		default:
			v := cleanText(in.Text, 2049)
			if v != "" && !IsHTTPSURL(v) {
				p.add("ticketUrl", "podaj adres zaczynający się od https://", "enter an address starting with https://")
			}
			o.TicketURL = &TextOverride{Value: v, Against: f.TicketURL}
		}
	default:
		p.add("field", "nieznane pole", "unknown field")
	}
	if err := p.err(); err != nil {
		return err
	}
	e.touch(by, now)
	return nil
}

// ExtrasInput is the CMS-only fields as the Moderator edits them.
type ExtrasInput struct {
	NameEN        string `json:"nameEn"`
	DescriptionEN string `json:"descriptionEn"`
	Summary       Text   `json:"summary"`
	Price         Text   `json:"price"`
	SocialImage   *Image `json:"socialImage"`
}

// SetExtras replaces the CMS-only fields. When the texts change, they are
// recorded as written against the current Facebook texts.
func (e *Event) SetExtras(in ExtrasInput, by string, now time.Time) error {
	if e.DeletedAt != nil {
		return fmt.Errorf("%w: event is in the Bin", ErrInvalidState)
	}
	var p problems
	x := Extras{
		NameEN: cleanText(in.NameEN, MaxTitleLen+1), DescriptionEN: cleanText(in.DescriptionEN, MaxDescriptionLen+1),
		Summary: cleanBi(in.Summary, MaxSummaryLen+1), Price: cleanBi(in.Price, MaxPriceLen+1),
		SocialImage: in.SocialImage, WrittenAgainst: e.Extras.WrittenAgainst,
	}
	checkLen(&p, "nameEn", Text{PL: x.NameEN}, MaxTitleLen)
	checkLen(&p, "descriptionEn", Text{PL: x.DescriptionEN}, MaxDescriptionLen)
	checkLen(&p, "summary", x.Summary, MaxSummaryLen)
	checkLen(&p, "price", x.Price, MaxPriceLen)
	for _, f := range []struct {
		field string
		t     Text
	}{{"summary", x.Summary}, {"price", x.Price}} {
		if f.t.PL == "" && f.t.EN != "" {
			p.add(f.field+".pl", "tekst angielski wymaga polskiego", "English text requires Polish text")
		}
	}
	if e.Manual != nil {
		// A Manual Event's English name and description live in ManualPart.
		x.NameEN, x.DescriptionEN = "", ""
	}
	if err := p.err(); err != nil {
		return err
	}
	old := e.Extras
	textChanged := old.NameEN != x.NameEN || old.DescriptionEN != x.DescriptionEN || old.Summary != x.Summary
	if e.Facebook != nil && (textChanged || x.WrittenAgainst == nil) {
		x.WrittenAgainst = &WrittenAgainst{Name: e.Facebook.Name, Description: e.Facebook.Description}
	}
	e.Extras = x
	e.touch(by, now)
	return nil
}

// DismissTextFlag confirms the English texts and Summary against the
// current Facebook texts (decision 16).
func (e *Event) DismissTextFlag(by string, now time.Time) error {
	if e.Facebook == nil {
		return fmt.Errorf("%w: not a synced event", ErrInvalidState)
	}
	e.Extras.WrittenAgainst = &WrittenAgainst{Name: e.Facebook.Name, Description: e.Facebook.Description}
	e.touch(by, now)
	return nil
}

// SetHidden switches Hide. Sync never changes it.
func (e *Event) SetHidden(hidden bool, by string, now time.Time) error {
	if e.DeletedAt != nil {
		return fmt.Errorf("%w: event is in the Bin", ErrInvalidState)
	}
	e.Hidden = hidden
	e.touch(by, now)
	return nil
}

// --- Manual Events ----------------------------------------------------------

// ManualInput is a Manual Event as the Moderator enters it.
type ManualInput struct {
	Name        Text              `json:"name"`
	Description Text              `json:"description"`
	Location    *Location         `json:"location"`
	Online      bool              `json:"online"`
	Cover       *Image            `json:"cover"`
	TicketURL   string            `json:"ticketUrl"`
	FacebookURL string            `json:"facebookUrl"`
	Occurrences []OccurrenceInput `json:"occurrences"`
	Summary     Text              `json:"summary"`
	Price       Text              `json:"price"`
	SocialImage *Image            `json:"socialImage"`
}

// OccurrenceInput is one date as entered: local date and times.
type OccurrenceInput struct {
	StartDate string `json:"startDate"` // YYYY-MM-DD
	StartTime string `json:"startTime"` // HH:MM
	EndDate   string `json:"endDate"`   // empty: same day, or the next day when EndTime is not after StartTime
	EndTime   string `json:"endTime"`
	Cancelled bool   `json:"cancelled"`
}

// ValidateManual checks a Manual Event and returns its content.
func ValidateManual(in ManualInput, loc *time.Location) (ManualPart, Extras, error) {
	var p problems
	m := ManualPart{
		Name:        cleanBi(in.Name, MaxTitleLen+1),
		Description: cleanBi(in.Description, MaxDescriptionLen+1),
		Online:      in.Online, Cover: in.Cover, TicketURL: cleanText(in.TicketURL, 2049),
	}
	if m.Name.PL == "" {
		p.add("name.pl", "nazwa po polsku jest wymagana", "a Polish name is required")
	}
	checkLen(&p, "name", m.Name, MaxTitleLen)
	checkLen(&p, "description", m.Description, MaxDescriptionLen)
	if m.Description.PL == "" && m.Description.EN != "" {
		p.add("description.pl", "tekst angielski wymaga polskiego", "English text requires Polish text")
	}
	if m.TicketURL != "" && !IsHTTPSURL(m.TicketURL) {
		p.add("ticketUrl", "podaj adres zaczynający się od https://", "enter an address starting with https://")
	}
	if l, ok := validLocation(in.Location, "location", &p); ok {
		m.Location = l
	}
	if id, ok := ParseFacebookEventLink(in.FacebookURL); ok {
		m.FacebookEventID = id
	} else {
		p.add("facebookUrl", "podaj adres wydarzenia na Facebooku, np. https://www.facebook.com/events/123456789/",
			"enter the address of the Facebook event, e.g. https://www.facebook.com/events/123456789/")
	}
	if len(in.Occurrences) == 0 {
		p.add("occurrences", "dodaj co najmniej jeden termin", "add at least one date")
	}
	if len(in.Occurrences) > MaxOccurrences {
		p.add("occurrences", "najwyżej %d terminów", "at most %d dates", MaxOccurrences)
	}
	for i, oi := range in.Occurrences {
		o, ok := oi.resolve(loc, fmt.Sprintf("occurrences[%d]", i), &p)
		if ok {
			o.FacebookID = m.FacebookEventID
			m.Occurrences = append(m.Occurrences, o)
		}
	}
	slices.SortStableFunc(m.Occurrences, func(a, b Occurrence) int { return a.Start.Compare(b.Start) })
	x := Extras{Summary: cleanBi(in.Summary, MaxSummaryLen+1), Price: cleanBi(in.Price, MaxPriceLen+1), SocialImage: in.SocialImage}
	checkLen(&p, "summary", x.Summary, MaxSummaryLen)
	checkLen(&p, "price", x.Price, MaxPriceLen)
	for _, f := range []struct {
		field string
		t     Text
	}{{"summary", x.Summary}, {"price", x.Price}} {
		if f.t.PL == "" && f.t.EN != "" {
			p.add(f.field+".pl", "tekst angielski wymaga polskiego", "English text requires Polish text")
		}
	}
	return m, x, p.err()
}

func (oi OccurrenceInput) resolve(loc *time.Location, field string, p *problems) (Occurrence, bool) {
	sd, err1 := time.ParseInLocation("2006-01-02 15:04", oi.StartDate+" "+oi.StartTime, loc)
	if err1 != nil {
		p.add(field+".start", "podaj datę i godzinę rozpoczęcia", "enter the start date and time")
		return Occurrence{}, false
	}
	endDate := oi.EndDate
	if endDate == "" {
		endDate = oi.StartDate
	}
	if oi.EndTime == "" {
		if oi.EndDate != "" {
			p.add(field+".end", "podaj godzinę zakończenia", "enter the end time")
			return Occurrence{}, false
		}
		return Occurrence{Start: sd, End: sd, Cancelled: oi.Cancelled}, true
	}
	ed, err := time.ParseInLocation("2006-01-02 15:04", endDate+" "+oi.EndTime, loc)
	if err != nil {
		p.add(field+".end", "podaj poprawną datę i godzinę zakończenia", "enter a valid end date and time")
		return Occurrence{}, false
	}
	if oi.EndDate == "" && !ed.After(sd) {
		ed = ed.AddDate(0, 0, 1) // a bal ending after midnight
	}
	if !ed.After(sd) {
		p.add(field+".end", "koniec musi być po początku", "the end must be after the start")
		return Occurrence{}, false
	}
	if ed.Sub(sd) > 31*24*time.Hour {
		p.add(field+".end", "termin może trwać najwyżej 31 dni", "a date can last at most 31 days")
		return Occurrence{}, false
	}
	return Occurrence{Start: sd, End: ed, Cancelled: oi.Cancelled}, true
}

// NewManualEvent creates a Manual Event.
func NewManualEvent(id, orgID, shortID string, m ManualPart, x Extras, by string, now time.Time) *Event {
	return &Event{ID: id, OrgID: orgID, ShortID: shortID, Source: SourceManual, Manual: &m, Extras: x,
		CreatedAt: now, CreatedBy: by, UpdatedAt: now, UpdatedBy: by}
}

// UpdateManual replaces a Manual Event's content. The Facebook link cannot
// change: remove the Manual Event and add another.
func (e *Event) UpdateManual(m ManualPart, x Extras, by string, now time.Time) error {
	if e.Manual == nil {
		return fmt.Errorf("%w: not a manual event", ErrInvalidState)
	}
	if e.DeletedAt != nil {
		return fmt.Errorf("%w: event is in the Bin", ErrInvalidState)
	}
	if m.FacebookEventID != e.Manual.FacebookEventID {
		return Problem("facebookUrl", "adresu wydarzenia na Facebooku nie można zmienić; usuń to wydarzenie i dodaj nowe",
			"the Facebook event address cannot change; remove this event and add a new one")
	}
	e.Manual = &m
	e.Extras = x
	e.touch(by, now)
	return nil
}

// --- Bin ---------------------------------------------------------------------

// Bin moves the Event to the Bin with a reason.
func (e *Event) Bin(reason, by string, now time.Time) error {
	if e.DeletedAt != nil {
		return fmt.Errorf("%w: already in the Bin", ErrInvalidState)
	}
	e.DeletedAt, e.DeletedBy, e.BinReason = &now, by, reason
	e.touch(by, now)
	return nil
}

// Restore takes the Event out of the Bin. A synced Event goes back to the
// Bin if the next Sync confirms its Facebook Event is still gone.
func (e *Event) Restore(by string, now time.Time) error {
	if e.DeletedAt == nil {
		return fmt.Errorf("%w: event is not in the Bin", ErrInvalidState)
	}
	if now.Sub(*e.DeletedAt) > BinRetention {
		return fmt.Errorf("%w: retention expired", ErrInvalidState)
	}
	e.DeletedAt, e.DeletedBy, e.BinReason = nil, "", ""
	e.touch(by, now)
	return nil
}

// Purgeable reports whether the Event has been in the Bin long enough.
func (e *Event) Purgeable(now time.Time) bool {
	return e.DeletedAt != nil && now.Sub(*e.DeletedAt) > BinRetention
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

func checkLen(p *problems, field string, t Text, max int) {
	if len([]rune(t.PL)) > max || len([]rune(t.EN)) > max {
		p.add(field, "tekst jest za długi (maks. %d znaków)", "the text is too long (max. %d characters)", max)
	}
}

func cleanBi(t Text, max int) Text {
	return Text{PL: cleanText(t.PL, max), EN: cleanText(t.EN, max)}
}

func validLocation(l *Location, field string, p *problems) (*Location, bool) {
	if l == nil {
		return nil, true
	}
	out := &Location{
		Name:    cleanText(l.Name, MaxLocationName+1),
		Address: cleanText(l.Address, MaxLocationAddr+1),
		MapURL:  cleanText(l.MapURL, 2049),
	}
	if out.Name == "" && out.Address == "" && out.MapURL == "" {
		return nil, true
	}
	ok := true
	if out.Name == "" {
		p.add(field+".name", "podaj nazwę miejsca", "enter the place name")
		ok = false
	}
	if len([]rune(out.Name)) > MaxLocationName || len([]rune(out.Address)) > MaxLocationAddr {
		p.add(field, "tekst jest za długi", "the text is too long")
		ok = false
	}
	if out.MapURL != "" && !IsHTTPSURL(out.MapURL) {
		p.add(field+".mapUrl", "podaj adres zaczynający się od https://", "enter an address starting with https://")
		ok = false
	}
	return out, ok
}

// MaxAltLen bounds an image's alt text.
const MaxAltLen = 300

// CheckImage accepts an image the client sends back only when it names
// files the CMS stored for this Organization (media/{org}/{id}-*.jpg, as
// UploadImage writes them), and cleans its alt text.
func CheckImage(org string, img *Image, field string) (*Image, error) {
	if img == nil {
		return nil, nil
	}
	bad := Problem(field, "nieprawidłowy obraz; prześlij go ponownie", "invalid image; upload it again")
	if len(img.ID) != 16 || !isHex(img.ID) || len(img.Variants) == 0 || len(img.Variants) > 8 {
		return nil, bad
	}
	prefix := "media/" + org + "/" + img.ID + "-"
	ok := func(v ImageVariant) bool {
		rest, found := strings.CutPrefix(v.Path, prefix)
		return found && strings.HasSuffix(rest, ".jpg") && len(rest) <= 16 && v.Width > 0 && v.Height > 0 &&
			strings.Trim(strings.TrimSuffix(rest, ".jpg"), "abcdefghijklmnopqrstuvwxyz0123456789") == ""
	}
	out := &Image{ID: img.ID, Alt: cleanBi(img.Alt, MaxAltLen)}
	for _, v := range img.Variants {
		if !ok(v) {
			return nil, bad
		}
		out.Variants = append(out.Variants, v)
	}
	if img.Instagram != nil {
		if !ok(*img.Instagram) {
			return nil, bad
		}
		ig := *img.Instagram
		out.Instagram = &ig
	}
	return out, nil
}

func isHex(s string) bool {
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
