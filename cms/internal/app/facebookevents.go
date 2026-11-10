package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/filip-be/folk-ms/cms/internal/domain"
)

// HandoffTokenTTL is how long a handoff token proves what a Facebook Event
// was created from.
const HandoffTokenTTL = 24 * time.Hour

// handoffToken is the HMAC-signed proof of the values a Facebook Event
// Handoff gave the Moderator. With it, a link recorded later keeps the
// values the Facebook Event was really created from, not the values at the
// time the link is saved.
type handoffToken struct {
	Org        string            `json:"o"`
	Event      string            `json:"e"`
	Key        string            `json:"k"`
	HandedOver domain.HandedOver `json:"h"`
	Expires    int64             `json:"exp"`
}

const handoffPurpose = "facebook-event-handoff"

// SetHandoffKey sets the key handoff tokens are signed with.
func (a *App) SetHandoffKey(k []byte) { a.handoffKey = k }

func (a *App) handoffMAC(payload []byte) []byte {
	m := hmac.New(sha256.New, a.handoffKey)
	m.Write([]byte(handoffPurpose))
	m.Write([]byte{0})
	m.Write(payload)
	return m.Sum(nil)
}

func (a *App) sealHandoff(t handoffToken) (string, error) {
	if len(a.handoffKey) == 0 {
		return "", errors.New("handoff key not set")
	}
	p, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding
	return enc.EncodeToString(p) + "." + enc.EncodeToString(a.handoffMAC(p)), nil
}

// openHandoff returns the token's values when it is valid for this Event;
// an invalid or expired token is ignored (the server then computes the
// values itself).
func (a *App) openHandoff(token, org, eventID string) (*handoffToken, bool) {
	if token == "" || len(a.handoffKey) == 0 {
		return nil, false
	}
	enc := base64.RawURLEncoding
	ps, ss, ok := strings.Cut(token, ".")
	if !ok {
		return nil, false
	}
	p, err1 := enc.DecodeString(ps)
	sig, err2 := enc.DecodeString(ss)
	if err1 != nil || err2 != nil || !hmac.Equal(sig, a.handoffMAC(p)) {
		return nil, false
	}
	var t handoffToken
	if json.Unmarshal(p, &t) != nil || t.Org != org || t.Event != eventID || a.now().Unix() > t.Expires {
		return nil, false
	}
	return &t, true
}

// HandoffView is what the Facebook Event Handoff screen shows.
type HandoffView struct {
	Published   bool                        `json:"published"`
	Connection  ConnectionStatus            `json:"connection"`
	Title       domain.Text                 `json:"title"`
	Fields      *domain.FacebookEventFields `json:"fields,omitempty"`
	CoverURL    string                      `json:"coverUrl,omitempty"`
	CoverName   string                      `json:"coverName,omitempty"`
	Token       string                      `json:"token,omitempty"`
	Links       []domain.FacebookEventState `json:"links"`
	Occurrences []domain.Occurrence         `json:"occurrences"`
	// NextLinked: the next Occurrence already has a Facebook Event.
	NextLinked bool `json:"nextLinked"`
}

// coverPath is the same-origin address of an Event's Cover Image (the CMS
// allows only connect-src 'self').
func coverPath(org, id string) string {
	return "/api/orgs/" + org + "/events/" + id + "/facebook-event/cover"
}

// FacebookEventHandoff prepares a Facebook Event for the next upcoming,
// non-cancelled Occurrence of a Published Event, and lists the links
// already recorded. It needs a Social Account Connection: the Page it
// names hosts the Facebook Event.
func (a *App) FacebookEventHandoff(ctx context.Context, p domain.Principal, org, id string) (HandoffView, error) {
	o, err := a.authorize(p, org)
	if err != nil {
		return HandoffView{}, err
	}
	e, _, err := load[domain.Event](ctx, a.Docs, eventKey(org, id))
	if err != nil {
		return HandoffView{}, err
	}
	conn, err := a.SocialStatus(ctx, p, org)
	if err != nil {
		return HandoffView{}, err
	}
	venues, err := a.venueMap(ctx, org)
	if err != nil {
		return HandoffView{}, err
	}
	loc := o.Location()
	v := HandoffView{
		Published: e.Published != nil && e.DeletedAt == nil, Connection: conn, Title: e.Draft.Title,
		Links: e.FacebookEventStates(loc, venues), Occurrences: []domain.Occurrence{},
	}
	if !v.Published {
		return v, nil
	}
	v.Title = e.Published.Title
	v.Occurrences = e.Published.Schedule.Occurrences(loc, e.Published.VenueID)
	if !conn.Connected {
		return v, nil
	}
	f, ok := domain.GenerateFacebookEventFields(o, e, venues, a.now())
	if !ok {
		return v, nil
	}
	v.Fields = &f
	if f.CoverPath != "" {
		v.CoverURL = coverPath(org, id)
		v.CoverName = e.Published.Slug + ".jpg"
	}
	for _, st := range v.Links {
		if st.Occurrence != nil && st.Occurrence.Key == f.OccurrenceKey {
			v.NextLinked = true
		}
	}
	v.Token, err = a.sealHandoff(handoffToken{Org: org, Event: id, Key: f.OccurrenceKey, HandedOver: f.HandedOver, Expires: a.now().Add(HandoffTokenTTL).Unix()})
	return v, err
}

// FacebookEventCover returns the bytes of the Cover Image the Handoff
// offers, from the public store.
func (a *App) FacebookEventCover(ctx context.Context, p domain.Principal, org, id string) ([]byte, error) {
	if _, err := a.authorize(p, org); err != nil {
		return nil, err
	}
	e, _, err := load[domain.Event](ctx, a.Docs, eventKey(org, id))
	if err != nil {
		return nil, err
	}
	if e.Published == nil || e.DeletedAt != nil || e.Published.Cover == nil || len(e.Published.Cover.Variants) == 0 {
		return nil, fmt.Errorf("%w: cover image", domain.ErrNotFound)
	}
	vs := e.Published.Cover.Variants
	d, err := a.Pub.Get(ctx, vs[len(vs)-1].Path)
	if err != nil {
		return nil, storeErr(err)
	}
	return d.Data, nil
}

// LinkInput records or changes a link. Token is the handoff token, when
// the Facebook Event was created from a Handoff. OccurrenceKey chooses the
// Occurrence when there is no valid token (adding), or re-attaches the
// link (changing).
type LinkInput struct {
	URL           string `json:"url"`
	OccurrenceKey string `json:"occurrenceKey,omitempty"`
	Token         string `json:"token,omitempty"`
}

// linkChange runs fn on the Event and rebuilds the snapshot.
func (a *App) linkChange(ctx context.Context, p domain.Principal, org, id, action string, rebuild bool, fn func(*domain.Event, *domain.Organization, map[string]domain.Venue) (map[string]any, error)) (HandoffView, error) {
	o, err := a.authorize(p, org)
	if err != nil {
		return HandoffView{}, err
	}
	venues, err := a.venueMap(ctx, org)
	if err != nil {
		return HandoffView{}, err
	}
	var details map[string]any
	e, _, err := mutate(ctx, a.Docs, eventKey(org, id), func(e *domain.Event) error {
		d, err := fn(e, o, venues)
		details = d
		return err
	})
	if err != nil {
		return HandoffView{}, err
	}
	details["title"] = e.Draft.Title.PL
	a.audit(ctx, p, action, org, id, details)
	if rebuild {
		if err := a.RebuildSnapshot(ctx, org); err != nil {
			return HandoffView{}, fmt.Errorf("link saved but snapshot rebuild failed: %w", err)
		}
	}
	return a.FacebookEventHandoff(ctx, p, org, id)
}

// AddFacebookEventLink records the link to a Facebook Event a Moderator
// created by hand (or with Autofill). The values it was created from come
// from a valid handoff token; otherwise the server takes the current values
// of the chosen Occurrence (by default the next upcoming one).
func (a *App) AddFacebookEventLink(ctx context.Context, p domain.Principal, org, id string, in LinkInput) (HandoffView, error) {
	now := a.now()
	return a.linkChange(ctx, p, org, id, "facebook_event.linked", true, func(e *domain.Event, o *domain.Organization, venues map[string]domain.Venue) (map[string]any, error) {
		loc := o.Location()
		key, handedOver, ok := a.tokenValues(in.Token, org, id)
		if !ok {
			key = in.OccurrenceKey
			if key == "" {
				next, _, found := e.NextFacebookOccurrence(loc, now)
				if !found {
					return nil, domain.Problem("occurrenceKey", "wybierz termin", "choose a date")
				}
				key = next.Key
			}
			if e.Published != nil {
				for _, occ := range e.Published.Schedule.Occurrences(loc, e.Published.VenueID) {
					if occ.Key == key {
						handedOver = e.HandOverValues(occ, venues)
					}
				}
			}
		}
		l, err := e.AddFacebookEvent(a.newID(), in.URL, key, handedOver, loc, p.ID(), now)
		if err != nil {
			return nil, err
		}
		return map[string]any{"link": l.ID, "url": l.URL, "occurrence": l.OccurrenceKey, "fromHandoff": ok}, nil
	})
}

func (a *App) tokenValues(token, org, id string) (string, domain.HandedOver, bool) {
	t, ok := a.openHandoff(token, org, id)
	if !ok {
		return "", domain.HandedOver{}, false
	}
	return t.Key, t.HandedOver, true
}

// ChangeFacebookEventLink changes a link's address or re-attaches it to
// another Occurrence. A valid handoff token for the same Occurrence
// replaces the values it was created from.
func (a *App) ChangeFacebookEventLink(ctx context.Context, p domain.Principal, org, id, linkID string, in LinkInput) (HandoffView, error) {
	now := a.now()
	return a.linkChange(ctx, p, org, id, "facebook_event.changed", true, func(e *domain.Event, o *domain.Organization, _ map[string]domain.Venue) (map[string]any, error) {
		var handedOver *domain.HandedOver
		key := in.OccurrenceKey
		if tk, hv, ok := a.tokenValues(in.Token, org, id); ok && (key == "" || key == tk) {
			key, handedOver = tk, &hv
		}
		l, err := e.ChangeFacebookEvent(linkID, in.URL, key, handedOver, o.Location(), p.ID(), now)
		if err != nil {
			return nil, err
		}
		return map[string]any{"link": l.ID, "url": l.URL, "occurrence": l.OccurrenceKey}, nil
	})
}

// RemoveFacebookEventLink forgets a link; the Facebook Event stays on
// Facebook.
func (a *App) RemoveFacebookEventLink(ctx context.Context, p domain.Principal, org, id, linkID string) (HandoffView, error) {
	return a.linkChange(ctx, p, org, id, "facebook_event.removed", true, func(e *domain.Event, _ *domain.Organization, _ map[string]domain.Venue) (map[string]any, error) {
		l, err := e.RemoveFacebookEvent(linkID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"link": l.ID, "url": l.URL, "occurrence": l.OccurrenceKey}, nil
	})
}

// DismissFacebookEventFlag records that the Moderator brought the Facebook
// Event up to date by hand.
func (a *App) DismissFacebookEventFlag(ctx context.Context, p domain.Principal, org, id, linkID string) (HandoffView, error) {
	return a.linkChange(ctx, p, org, id, "facebook_event.dismissed", false, func(e *domain.Event, o *domain.Organization, venues map[string]domain.Venue) (map[string]any, error) {
		l, err := e.DismissFacebookEvent(linkID, o.Location(), venues)
		if err != nil {
			return nil, err
		}
		return map[string]any{"link": l.ID, "url": l.URL, "occurrence": l.OccurrenceKey}, nil
	})
}

// AutofillReport is what Autofill reports after filling Facebook's form,
// relayed by the admin SPA. It is recorded in the Audit Log.
type AutofillReport struct {
	EventID  string            `json:"eventId"`
	Version  string            `json:"version"`
	Channel  string            `json:"channel"`
	Browser  string            `json:"browser"`
	UILang   string            `json:"uiLanguage"`
	Fields   map[string]string `json:"fields"`
	Errors   []string          `json:"errors,omitempty"`
	URL      string            `json:"url,omitempty"`
	FilledAt *time.Time        `json:"filledAt,omitempty"`
}

// Limits on what an Autofill report may hold.
const (
	maxReportFields = 32
	maxReportString = 300
)

func clip(s string) string {
	if utf8.RuneCountInString(s) > maxReportString {
		return string([]rune(s)[:maxReportString])
	}
	return s
}

// ReportAutofill appends an Autofill report to the Audit Log.
func (a *App) ReportAutofill(ctx context.Context, p domain.Principal, org string, r AutofillReport) error {
	if _, err := a.authorize(p, org); err != nil {
		return err
	}
	if len(r.Fields) > maxReportFields || len(r.Errors) > maxReportFields {
		return domain.Problem("fields", "zbyt duży raport", "the report is too large")
	}
	fields := map[string]string{}
	failed := 0
	for k, v := range r.Fields {
		fields[clip(k)] = clip(v)
		if v == "failed" {
			failed++
		}
	}
	errs := make([]string, len(r.Errors))
	for i, e := range r.Errors {
		errs[i] = clip(e)
	}
	details := map[string]any{
		"version": clip(r.Version), "channel": clip(r.Channel), "browser": clip(r.Browser),
		"uiLanguage": clip(r.UILang), "fields": fields, "failed": failed,
	}
	if len(errs) > 0 {
		details["errors"] = errs
	}
	if r.FilledAt != nil {
		details["filledAt"] = r.FilledAt.UTC()
	}
	a.audit(ctx, p, "autofill.reported", org, clip(r.EventID), details)
	return nil
}
