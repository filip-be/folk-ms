package domain

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// MaxFacebookEventNameLen is Facebook's limit on an event name (its form counts to 100, spike of 2026-11-10).
const MaxFacebookEventNameLen = 100

// MaxFacebookEventLinks bounds the links kept on one Event.
const MaxFacebookEventLinks = 200

// HandedOver are the source values a Facebook Event was created from: what
// the CMS compares with the Published version to tell whether the Facebook
// Event may be out of date. It never holds the Moderator's edited text, nor
// the further dates of a Recurring Event.
type HandedOver struct {
	Title        string    `json:"title"`
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	VenueName    string    `json:"venueName"`
	VenueAddress string    `json:"venueAddress"`
	Cancelled    bool      `json:"cancelled"`
}

// Fields a Facebook Event can be out of date in.
const (
	ChangedTitle     = "title"
	ChangedStart     = "start"
	ChangedEnd       = "end"
	ChangedVenue     = "venue"
	ChangedCancelled = "cancelled"
)

// Changes lists the fields where cur differs from h.
func (h HandedOver) Changes(cur HandedOver) []string {
	out := []string{}
	if h.Title != cur.Title {
		out = append(out, ChangedTitle)
	}
	if !h.Start.Equal(cur.Start) {
		out = append(out, ChangedStart)
	}
	if !h.End.Equal(cur.End) {
		out = append(out, ChangedEnd)
	}
	if h.VenueName != cur.VenueName || h.VenueAddress != cur.VenueAddress {
		out = append(out, ChangedVenue)
	}
	if h.Cancelled != cur.Cancelled {
		out = append(out, ChangedCancelled)
	}
	return out
}

// FacebookEventLink is the link to a Facebook Event, kept against the
// Occurrence it announces.
type FacebookEventLink struct {
	ID            string     `json:"id"`
	URL           string     `json:"url"`
	OccurrenceKey string     `json:"occurrenceKey"`
	HandedOver    HandedOver `json:"handedOver"`
	RecordedBy    string     `json:"recordedBy"`
	RecordedAt    time.Time  `json:"recordedAt"`
}

var facebookEventPath = regexp.MustCompile(`^/events/([0-9]{5,25})/?$`)

// NormalizeFacebookEventURL validates the address of a Facebook Event and
// returns it as https://www.facebook.com/events/{id}/. It accepts the www.,
// m. and bare facebook.com hosts, with or without a trailing slash, query
// or fragment.
func NormalizeFacebookEventURL(s string) (string, error) {
	bad := Problem("url", "podaj adres wydarzenia na Facebooku, np. https://www.facebook.com/events/123456789/",
		"enter the address of a Facebook event, e.g. https://www.facebook.com/events/123456789/")
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Port() != "" {
		return "", bad
	}
	switch strings.ToLower(u.Hostname()) {
	case "facebook.com", "www.facebook.com", "m.facebook.com":
	default:
		return "", bad
	}
	m := facebookEventPath.FindStringSubmatch(u.Path)
	if m == nil {
		return "", bad
	}
	return "https://www.facebook.com/events/" + m[1] + "/", nil
}

// FacebookEventState is a link resolved against the current Event.
type FacebookEventState struct {
	Link FacebookEventLink `json:"link"`
	// Occurrence is the Published Occurrence the link belongs to; nil when
	// it is detached or the Event is not Published.
	Occurrence *Occurrence `json:"occurrence,omitempty"`
	// Detached: on a Recurring Event, the link's Occurrence no longer
	// exists. The Site hides it until it is re-attached or removed.
	Detached bool `json:"detached"`
	// Unpublished: the Event is not on the Site (Unpublished or Binned).
	Unpublished bool `json:"unpublished"`
	// OutOfDate: the Facebook Event may need editing on Facebook.
	OutOfDate bool `json:"outOfDate"`
	// Changed lists the fields that differ from what was handed over.
	Changed []string `json:"changed"`
	// Current are the values the Facebook Event should now show.
	Current *HandedOver `json:"current,omitempty"`
}

// publishedOccurrences expands the Published version, or nil when the Event
// is not on the Site.
func (e *Event) publishedOccurrences(loc *time.Location) []Occurrence {
	if e.Published == nil || e.DeletedAt != nil {
		return nil
	}
	return e.Published.Schedule.Occurrences(loc, e.Published.VenueID)
}

// resolveLink finds the Occurrence a link belongs to: on a Single or
// Multi-day Event the only Occurrence whatever the key; on a Recurring
// Event the one with the link's key.
func resolveLink(kind Kind, occ []Occurrence, key string) (Occurrence, bool) {
	if kind != KindRecurring {
		if len(occ) == 1 {
			return occ[0], true
		}
		return Occurrence{}, false
	}
	for _, o := range occ {
		if o.Key == key {
			return o, true
		}
	}
	return Occurrence{}, false
}

// occurrenceVenue resolves an Occurrence's Venue as BuildSnapshot does: a
// frozen copy for a past Occurrence wins over the current Venue.
func (e *Event) occurrenceVenue(o Occurrence, venues map[string]Venue) Venue {
	if frozen, ok := e.PastVenues[o.Key]; ok && frozen.ID == o.VenueID {
		return frozen
	}
	return venues[o.VenueID]
}

// HandOverValues are the source values of an Occurrence of the Published
// version.
func (e *Event) HandOverValues(o Occurrence, venues map[string]Venue) HandedOver {
	v := e.occurrenceVenue(o, venues)
	title := ""
	if e.Published != nil {
		title = e.Published.Title.PL
	}
	return HandedOver{Title: title, Start: o.Start, End: o.End, VenueName: v.Name, VenueAddress: v.Address, Cancelled: o.Cancelled}
}

// FacebookEventStates resolves every link against the Published version.
func (e *Event) FacebookEventStates(loc *time.Location, venues map[string]Venue) []FacebookEventState {
	out := make([]FacebookEventState, 0, len(e.FacebookEvents))
	occ := e.publishedOccurrences(loc)
	for _, l := range e.FacebookEvents {
		st := FacebookEventState{Link: l, Changed: []string{}}
		switch {
		case occ == nil:
			st.Unpublished, st.OutOfDate = true, true
		default:
			o, ok := resolveLink(e.Published.Schedule.Kind, occ, l.OccurrenceKey)
			if !ok {
				st.Detached = true
				break
			}
			cur := e.HandOverValues(o, venues)
			st.Occurrence, st.Current = &o, &cur
			st.Changed = l.HandedOver.Changes(cur)
			st.OutOfDate = len(st.Changed) > 0
		}
		out = append(out, st)
	}
	return out
}

// FacebookEventFlags summarizes the states for the Event list.
func (e *Event) FacebookEventFlags(loc *time.Location, venues map[string]Venue) (outOfDate, detached bool) {
	for _, st := range e.FacebookEventStates(loc, venues) {
		outOfDate = outOfDate || st.OutOfDate
		detached = detached || st.Detached
	}
	return outOfDate, detached
}

// FacebookEventURLs maps each Published Occurrence key to the link the Site
// shows: the most recently recorded one that resolves to it. Detached links
// are left out.
func (e *Event) FacebookEventURLs(occ []Occurrence) map[string]string {
	out := map[string]string{}
	if e.Published == nil {
		return out
	}
	at := map[string]time.Time{}
	for _, l := range e.FacebookEvents {
		o, ok := resolveLink(e.Published.Schedule.Kind, occ, l.OccurrenceKey)
		if !ok {
			continue
		}
		if prev, seen := at[o.Key]; !seen || !l.RecordedAt.Before(prev) {
			out[o.Key], at[o.Key] = l.URL, l.RecordedAt
		}
	}
	return out
}

// FindFacebookEvent returns a link by id.
func (e *Event) FindFacebookEvent(id string) *FacebookEventLink {
	for i := range e.FacebookEvents {
		if e.FacebookEvents[i].ID == id {
			return &e.FacebookEvents[i]
		}
	}
	return nil
}

func (e *Event) checkFacebookURL(url, exceptID string) error {
	for _, l := range e.FacebookEvents {
		if l.URL == url && l.ID != exceptID {
			return Problem("url", "ten link jest już zapisany przy tym wydarzeniu", "this link is already recorded for this event")
		}
	}
	return nil
}

// checkOccurrenceFree refuses a second link on an Occurrence that already
// has one: the Moderator changes or removes the existing link instead.
func (e *Event) checkOccurrenceFree(occ []Occurrence, key, exceptID string) error {
	for _, l := range e.FacebookEvents {
		if l.ID == exceptID {
			continue
		}
		if o, ok := resolveLink(e.Published.Schedule.Kind, occ, l.OccurrenceKey); ok && o.Key == key {
			return Problem("occurrenceKey", "ten termin ma już wydarzenie na Facebooku – zmień lub usuń istniejący link",
				"this date already has a Facebook event – change or remove the existing link")
		}
	}
	return nil
}

// occurrenceFor returns the Published Occurrence with key.
func (e *Event) occurrenceFor(occ []Occurrence, key string) (Occurrence, error) {
	for _, o := range occ {
		if o.Key == key {
			return o, nil
		}
	}
	return Occurrence{}, Problem("occurrenceKey", "wybrany termin nie istnieje", "the chosen date does not exist")
}

func requirePublished(e *Event) error {
	if e.Published == nil || e.DeletedAt != nil {
		return Problem("event", "tylko opublikowane wydarzenia mogą mieć wydarzenie na Facebooku", "only published events can have a Facebook event")
	}
	return nil
}

// AddFacebookEvent records a link to a Facebook Event for the Published
// Occurrence with key. handedOver is what the Facebook Event was created
// from: the handoff token's values, or the current values when there is
// no valid token.
func (e *Event) AddFacebookEvent(id, rawURL, key string, handedOver HandedOver, loc *time.Location, by string, now time.Time) (*FacebookEventLink, error) {
	if err := requirePublished(e); err != nil {
		return nil, err
	}
	u, err := NormalizeFacebookEventURL(rawURL)
	if err != nil {
		return nil, err
	}
	if len(e.FacebookEvents) >= MaxFacebookEventLinks {
		return nil, Problem("url", "to wydarzenie ma już zbyt wiele linków do Facebooka", "this event has too many Facebook links")
	}
	if err := e.checkFacebookURL(u, ""); err != nil {
		return nil, err
	}
	occ := e.publishedOccurrences(loc)
	if _, err := e.occurrenceFor(occ, key); err != nil {
		return nil, err
	}
	if err := e.checkOccurrenceFree(occ, key, ""); err != nil {
		return nil, err
	}
	e.FacebookEvents = append(e.FacebookEvents, FacebookEventLink{
		ID: id, URL: u, OccurrenceKey: key, HandedOver: handedOver, RecordedBy: by, RecordedAt: now,
	})
	return &e.FacebookEvents[len(e.FacebookEvents)-1], nil
}

// ChangeFacebookEvent changes a link's address and, when key is not empty,
// (re-)attaches it to that Published Occurrence. handedOver, when not nil,
// replaces the recorded source values (a new Facebook Event was created
// from a handoff).
func (e *Event) ChangeFacebookEvent(linkID, rawURL, key string, handedOver *HandedOver, loc *time.Location, by string, now time.Time) (*FacebookEventLink, error) {
	l := e.FindFacebookEvent(linkID)
	if l == nil {
		return nil, fmt.Errorf("%w: facebook event link", ErrNotFound)
	}
	if err := requirePublished(e); err != nil {
		return nil, err
	}
	u := l.URL
	if rawURL != "" {
		var err error
		if u, err = NormalizeFacebookEventURL(rawURL); err != nil {
			return nil, err
		}
		if err := e.checkFacebookURL(u, linkID); err != nil {
			return nil, err
		}
	}
	if key != "" {
		occ := e.publishedOccurrences(loc)
		if _, err := e.occurrenceFor(occ, key); err != nil {
			return nil, err
		}
		if err := e.checkOccurrenceFree(occ, key, linkID); err != nil {
			return nil, err
		}
		l.OccurrenceKey = key
	}
	l.URL = u
	if handedOver != nil {
		l.HandedOver = *handedOver
	}
	l.RecordedBy, l.RecordedAt = by, now
	return l, nil
}

// RemoveFacebookEvent forgets a link. The Facebook Event itself stays on
// Facebook.
func (e *Event) RemoveFacebookEvent(linkID string) (FacebookEventLink, error) {
	for i, l := range e.FacebookEvents {
		if l.ID == linkID {
			e.FacebookEvents = append(e.FacebookEvents[:i], e.FacebookEvents[i+1:]...)
			return l, nil
		}
	}
	return FacebookEventLink{}, fmt.Errorf("%w: facebook event link", ErrNotFound)
}

// DismissFacebookEvent records that the Moderator brought the Facebook
// Event up to date: the source values become the current ones.
func (e *Event) DismissFacebookEvent(linkID string, loc *time.Location, venues map[string]Venue) (*FacebookEventLink, error) {
	l := e.FindFacebookEvent(linkID)
	if l == nil {
		return nil, fmt.Errorf("%w: facebook event link", ErrNotFound)
	}
	if err := requirePublished(e); err != nil {
		return nil, err
	}
	o, ok := resolveLink(e.Published.Schedule.Kind, e.publishedOccurrences(loc), l.OccurrenceKey)
	if !ok {
		return nil, Problem("occurrenceKey", "ten termin już nie istnieje – przypisz link do innego terminu albo go usuń",
			"this date no longer exists – attach the link to another date or remove it")
	}
	l.HandedOver = e.HandOverValues(o, venues)
	if e.Published.Schedule.Kind != KindRecurring {
		l.OccurrenceKey = o.Key // follow a moved Single or Multi-day Event
	}
	return l, nil
}

// FacebookEventFields is what the Facebook Event Handoff prepares for one
// Occurrence. Times are local to the Organization.
type FacebookEventFields struct {
	OccurrenceKey string     `json:"occurrenceKey"`
	Name          string     `json:"name"`
	StartDate     string     `json:"startDate"`
	StartTime     string     `json:"startTime"`
	EndDate       string     `json:"endDate"`
	EndTime       string     `json:"endTime"`
	Start         time.Time  `json:"start"`
	End           time.Time  `json:"end"`
	Timezone      string     `json:"timezone"`
	Location      string     `json:"location"`
	VenueName     string     `json:"venueName"`
	VenueAddress  string     `json:"venueAddress"`
	Description   string     `json:"description"`
	TicketURL     string     `json:"ticketUrl,omitempty"`
	CoverPath     string     `json:"-"`
	HandedOver    HandedOver `json:"-"`
}

// maxFurtherDates bounds the dates listed in a description.
const maxFurtherDates = 12

// NextFacebookOccurrence is the Occurrence a new Facebook Event announces:
// the first Published one that is not cancelled and has not started.
func (e *Event) NextFacebookOccurrence(loc *time.Location, now time.Time) (Occurrence, []Occurrence, bool) {
	occ := e.publishedOccurrences(loc)
	for i, o := range occ {
		if !o.Cancelled && o.Start.After(now) {
			var further []Occurrence
			for _, f := range occ[i+1:] {
				if !f.Cancelled {
					further = append(further, f)
				}
			}
			return o, further, true
		}
	}
	return Occurrence{}, nil, false
}

// TruncateRunes cuts s to at most n characters.
func TruncateRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return strings.TrimSpace(string(r[:n]))
	}
	return s
}

// GenerateFacebookEventFields prepares a Facebook Event Handoff for the next
// upcoming Occurrence of a Published Event. ok is false when there is none.
func GenerateFacebookEventFields(org *Organization, e *Event, venues map[string]Venue, now time.Time) (FacebookEventFields, bool) {
	loc := org.Location()
	o, further, ok := e.NextFacebookOccurrence(loc, now)
	if !ok {
		return FacebookEventFields{}, false
	}
	c := e.Published
	v := e.occurrenceVenue(o, venues)
	start, end := o.Start.In(loc), o.End.In(loc)
	f := FacebookEventFields{
		OccurrenceKey: o.Key,
		Name:          TruncateRunes(c.Title.PL, MaxFacebookEventNameLen),
		StartDate:     start.Format(time.DateOnly), StartTime: start.Format("15:04"),
		EndDate: end.Format(time.DateOnly), EndTime: end.Format("15:04"),
		Start: start, End: end, Timezone: org.Timezone,
		VenueName: v.Name, VenueAddress: v.Address,
		TicketURL:  c.Price.URL,
		HandedOver: e.HandOverValues(o, venues),
	}
	f.Location = strings.TrimSuffix(strings.TrimSpace(v.Name+", "+v.Address), ",")
	if c.Cover != nil && len(c.Cover.Variants) > 0 {
		f.CoverPath = c.Cover.Variants[len(c.Cover.Variants)-1].Path // the largest
	}
	f.Description = facebookDescription(org, e, o, further, loc)
	return f, true
}

// facebookDescription is the Polish block, then the English block when the
// Event has English text: summary, description, price, further dates and
// the Permalink in each.
func facebookDescription(org *Organization, e *Event, o Occurrence, further []Occurrence, loc *time.Location) string {
	c := e.Published
	block := func(l Lang) string {
		var b []string
		pick := func(t Text) string {
			if l == EN {
				return t.EN
			}
			return t.PL
		}
		if s := pick(c.Summary); s != "" {
			b = append(b, s)
		}
		if d := pick(c.Description); d != "" {
			b = append(b, d)
		}
		if n := pick(o.Note); n != "" {
			b = append(b, n)
		}
		label := func(pl, en string) string {
			if l == EN {
				return en
			}
			return pl
		}
		if p := c.Price.Text.In(l); p != "" {
			b = append(b, label("Wstęp: ", "Admission: ")+p)
		}
		if len(further) > 0 {
			lines := []string{label("Kolejne terminy:", "Further dates:")}
			for i, f := range further {
				if i == maxFurtherDates {
					lines = append(lines, label(fmt.Sprintf("… i %d kolejnych", len(further)-i), fmt.Sprintf("… and %d more", len(further)-i)))
					break
				}
				lines = append(lines, "• "+FormatDate(f.Start.In(loc), l))
			}
			b = append(b, strings.Join(lines, "\n"))
		}
		b = append(b, label("Szczegóły: ", "Details: ")+org.Permalink(e.ShortID, c.Slug, l))
		return strings.Join(b, "\n\n")
	}
	out := block(PL)
	if c.Title.EN != "" || c.Summary.EN != "" || c.Description.EN != "" {
		head := ""
		if c.Title.EN != "" {
			head = c.Title.EN + "\n\n"
		}
		out += "\n\n— English —\n\n" + head + block(EN)
	}
	return out
}
