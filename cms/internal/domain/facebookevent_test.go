package domain

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/schema"
)

func TestNormalizeFacebookEventURL(t *testing.T) {
	good := map[string]string{
		"https://www.facebook.com/events/1004759319341552/":                                 "https://www.facebook.com/events/1004759319341552/",
		"https://www.facebook.com/events/1004759319341552":                                  "https://www.facebook.com/events/1004759319341552/",
		"https://m.facebook.com/events/1004759319341552/?ref=share":                         "https://www.facebook.com/events/1004759319341552/",
		"facebook.com/events/1004759319341552":                                              "https://www.facebook.com/events/1004759319341552/",
		"  http://WWW.facebook.com/events/1004759319341552/#x ":                             "https://www.facebook.com/events/1004759319341552/",
		"https://www.facebook.com/events/1004759319341552/?acontext=%7B%7D&show_toast=true": "https://www.facebook.com/events/1004759319341552/",
	}
	for in, want := range good {
		got, err := NormalizeFacebookEventURL(in)
		if err != nil || got != want {
			t.Errorf("%q: got %q, %v", in, got, err)
		}
	}
	for _, in := range []string{
		"", "https://evil.example/events/1004759319341552/", "https://www.facebook.com.evil.example/events/1004759319341552/",
		"https://www.facebook.com/events/create/", "https://www.facebook.com/groups/123456/", "https://www.facebook.com/events/12/",
		"https://user@www.facebook.com/events/1004759319341552/", "https://www.facebook.com:8443/events/1004759319341552/",
		"ftp://www.facebook.com/events/1004759319341552/", "https://www.facebook.com/events/1004759319341552/extra/",
	} {
		if _, err := NormalizeFacebookEventURL(in); !IsValidation(err) {
			t.Errorf("%q: want a validation error, got %v", in, err)
		}
	}
}

func venuesFB() map[string]Venue {
	return map[string]Venue{
		"zamek": {ID: "zamek", Name: "Zamek", Address: "Św. Marcin 80"},
		"other": {ID: "other", Name: "Inne", Address: "ul. Inna 2"},
	}
}

func published(t *testing.T, c Content, at time.Time) *Event {
	t.Helper()
	e, err := NewEvent("e1e1e1e1e1e1e1e1", "balfolk-poznan", "abcdefgh", c, "u", at)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Publish(venueOK, "u", at); err != nil {
		t.Fatal(err)
	}
	return e
}

func republish(t *testing.T, e *Event, c Content, at time.Time) {
	t.Helper()
	if err := e.UpdateDraft(c, "u", at); err != nil {
		t.Fatal(err)
	}
	if err := e.Publish(venueOK, "u", at); err != nil {
		t.Fatal(err)
	}
}

func weekly() Content {
	return Content{Title: Text{PL: "Jam"}, VenueID: "zamek", Schedule: Schedule{Kind: KindRecurring, StartDate: "2026-11-01",
		StartTime: "19:00", EndTime: "22:00", Recurrence: &Recurrence{Freq: Weekly, Weekday: 4, EndDate: "2026-12-31"}}}
}

func TestFacebookEventLinkLifecycle(t *testing.T) {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	org := testOrg()
	vs := venuesFB()
	e := published(t, publishable(), now)
	f, ok := GenerateFacebookEventFields(org, e, vs, now)
	if !ok {
		t.Fatal("fields")
	}
	l, err := e.AddFacebookEvent("l1", "https://m.facebook.com/events/111111/", f.OccurrenceKey, f.HandedOver, warsaw, "u", now)
	if err != nil || l.URL != "https://www.facebook.com/events/111111/" {
		t.Fatalf("add: %v %+v", err, l)
	}
	if _, err := e.AddFacebookEvent("l2", "https://www.facebook.com/events/111111", f.OccurrenceKey, f.HandedOver, warsaw, "u", now); !IsValidation(err) {
		t.Fatal("the same link twice must be refused")
	}
	if _, err := e.AddFacebookEvent("l2", "https://www.facebook.com/events/222222/", f.OccurrenceKey, f.HandedOver, warsaw, "u", now); !IsValidation(err) {
		t.Fatal("a second link on one Occurrence must be refused")
	}
	st := e.FacebookEventStates(warsaw, vs)
	if len(st) != 1 || st[0].OutOfDate || st[0].Detached || st[0].Unpublished {
		t.Fatalf("fresh link: %+v", st)
	}

	// Single Event moved and renamed: the link follows its only Occurrence
	// and is flagged out of date in exactly the changed fields.
	c := publishable()
	c.Title.PL = "Bal listopadowy – nowa data"
	c.Schedule.StartDate = "2026-11-28"
	republish(t, e, c, now)
	st = e.FacebookEventStates(warsaw, vs)
	if st[0].Detached || !st[0].OutOfDate || !slices.Equal(st[0].Changed, []string{ChangedTitle, ChangedStart, ChangedEnd}) {
		t.Fatalf("moved single: %+v", st[0])
	}
	if urls := e.FacebookEventURLs(e.Published.Schedule.Occurrences(warsaw, "zamek")); urls["2026-11-28"] != l.URL {
		t.Fatalf("site keeps the link on the moved Occurrence: %v", urls)
	}
	if _, err := e.DismissFacebookEvent("l1", warsaw, vs); err != nil {
		t.Fatal(err)
	}
	st = e.FacebookEventStates(warsaw, vs)
	if st[0].OutOfDate || e.FacebookEvents[0].OccurrenceKey != "2026-11-28" {
		t.Fatalf("dismissed: %+v", st[0])
	}

	// Unpublished: flagged, cannot be dismissed, hidden from the Site.
	_ = e.Unpublish("u", now)
	st = e.FacebookEventStates(warsaw, vs)
	if !st[0].Unpublished || !st[0].OutOfDate {
		t.Fatalf("unpublished: %+v", st[0])
	}
	if _, err := e.DismissFacebookEvent("l1", warsaw, vs); !IsValidation(err) {
		t.Fatal("dismiss needs a Published Event")
	}
	if _, err := e.RemoveFacebookEvent("l1"); err != nil || len(e.FacebookEvents) != 0 {
		t.Fatal("remove")
	}
	if _, err := e.RemoveFacebookEvent("l1"); err == nil {
		t.Fatal("remove twice")
	}
}

func TestFacebookEventRecurringDetachAndKindChange(t *testing.T) {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	vs := venuesFB()
	e := published(t, weekly(), now) // Thursdays from 2026-11-05
	occ := e.Published.Schedule.Occurrences(warsaw, "zamek")
	first, second := occ[0], occ[1]
	if _, err := e.AddFacebookEvent("l1", "https://www.facebook.com/events/111111/", first.Key, e.HandOverValues(first, vs), warsaw, "u", now); err != nil {
		t.Fatal(err)
	}
	if _, err := e.AddFacebookEvent("l2", "https://www.facebook.com/events/222222/", second.Key, e.HandOverValues(second, vs), warsaw, "u", now); err != nil {
		t.Fatal(err)
	}
	if _, err := e.AddFacebookEvent("l3", "https://www.facebook.com/events/333333/", "2026-11-06", HandedOver{}, warsaw, "u", now); !IsValidation(err) {
		t.Fatal("a key that is not an Occurrence must be refused")
	}

	// Move the Recurrence to Wednesdays: both keys vanish, both links detach
	// and the Site drops them.
	c := weekly()
	c.Schedule.Recurrence.Weekday = 3
	republish(t, e, c, now)
	st := e.FacebookEventStates(warsaw, vs)
	if !st[0].Detached || !st[1].Detached || st[0].OutOfDate {
		t.Fatalf("detached: %+v", st)
	}
	if out, det := e.FacebookEventFlags(warsaw, vs); out || !det {
		t.Fatal("list flags")
	}
	newOcc := e.Published.Schedule.Occurrences(warsaw, "zamek")
	if urls := e.FacebookEventURLs(newOcc); len(urls) != 0 {
		t.Fatalf("detached links are hidden: %v", urls)
	}
	if _, err := e.DismissFacebookEvent("l1", warsaw, vs); !IsValidation(err) {
		t.Fatal("a detached link cannot be dismissed")
	}
	// Re-attach to a new Occurrence: it is out of date in its start and end.
	if _, err := e.ChangeFacebookEvent("l1", "", newOcc[0].Key, nil, warsaw, "u", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.ChangeFacebookEvent("l2", "", newOcc[0].Key, nil, warsaw, "u", now); !IsValidation(err) {
		t.Fatal("re-attaching onto a linked Occurrence must be refused")
	}
	st = e.FacebookEventStates(warsaw, vs)
	if st[0].Detached || !slices.Equal(st[0].Changed, []string{ChangedStart, ChangedEnd}) {
		t.Fatalf("re-attached: %+v", st[0])
	}

	// Recurring -> Single: every link resolves to the only Occurrence, and
	// the Site shows the most recently recorded one.
	single := publishable()
	single.Schedule.StartDate = "2026-11-21"
	republish(t, e, single, now)
	st = e.FacebookEventStates(warsaw, vs)
	if st[0].Detached || st[1].Detached {
		t.Fatalf("single resolves all: %+v", st)
	}
	urls := e.FacebookEventURLs(e.Published.Schedule.Occurrences(warsaw, "zamek"))
	if urls["2026-11-21"] != "https://www.facebook.com/events/111111/" { // l1 changed last
		t.Fatalf("most recent link: %v", urls)
	}

	// Single -> Recurring: a link resolves only when its key is a date of the rule.
	r := weekly()
	r.Schedule.Recurrence.Weekday = 3 // Wednesdays: l1's key 2026-11-04 is one
	republish(t, e, r, now)
	st = e.FacebookEventStates(warsaw, vs)
	if st[0].Detached || !st[1].Detached {
		t.Fatalf("back to recurring: %+v", st)
	}
}

func TestFacebookEventOutOfDateVenues(t *testing.T) {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	vs := venuesFB()
	e := published(t, weekly(), now)
	occ := e.Published.Schedule.Occurrences(warsaw, "zamek")
	o := occ[0] // 2026-11-05
	if _, err := e.AddFacebookEvent("l1", "https://www.facebook.com/events/111111/", o.Key, e.HandOverValues(o, vs), warsaw, "u", now); err != nil {
		t.Fatal(err)
	}
	// The Venue is edited after the Occurrence happened: its frozen copy is
	// what the Facebook Event showed, so nothing is flagged.
	old := vs["zamek"]
	edited := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	e.FreezePastVenue(old, warsaw, edited)
	vs["zamek"] = Venue{ID: "zamek", Name: "Nowy Zamek", Address: "ul. Nowa 1"}
	if st := e.FacebookEventStates(warsaw, vs); st[0].OutOfDate {
		t.Fatalf("frozen venue: %+v", st[0])
	}
	// A link on a future Occurrence sees the edit.
	o2 := occ[2]
	if _, err := e.AddFacebookEvent("l2", "https://www.facebook.com/events/222222/", o2.Key, e.HandOverValues(o2, map[string]Venue{"zamek": old}), warsaw, "u", now); err != nil {
		t.Fatal(err)
	}
	st := e.FacebookEventStates(warsaw, vs)
	if !st[1].OutOfDate || !slices.Equal(st[1].Changed, []string{ChangedVenue}) || st[1].Current.VenueName != "Nowy Zamek" {
		t.Fatalf("venue edit: %+v", st[1])
	}
	// Cancelling the Occurrence flags it too.
	c := weekly()
	c.Schedule.Exceptions = []Exception{{Date: o2.Key, Cancelled: true}}
	republish(t, e, c, now)
	st = e.FacebookEventStates(warsaw, vs)
	if !slices.Contains(st[1].Changed, ChangedCancelled) {
		t.Fatalf("cancelled: %+v", st[1])
	}
	// An Exception moving the Occurrence to another Venue.
	c.Schedule.Exceptions = []Exception{{Date: o2.Key, VenueID: "other"}}
	republish(t, e, c, now)
	if st := e.FacebookEventStates(warsaw, vs); !slices.Equal(st[1].Changed, []string{ChangedVenue}) || st[1].Current.VenueName != "Inne" {
		t.Fatalf("exception venue: %+v", st[1])
	}
}

func TestGenerateFacebookEventFields(t *testing.T) {
	org := testOrg()
	vs := venuesFB()
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

	// Single, overnight; English present; long title cut to Facebook's limit.
	c := publishable()
	c.Title.PL = strings.Repeat("Bal ", 40)
	c.Description = Text{PL: "Opis", EN: "Description"}
	e := published(t, c, now)
	f, ok := GenerateFacebookEventFields(org, e, vs, now)
	if !ok {
		t.Fatal("single")
	}
	if len([]rune(f.Name)) > MaxFacebookEventNameLen || f.StartDate != "2026-11-21" || f.StartTime != "19:00" || f.EndDate != "2026-11-22" || f.EndTime != "01:00" {
		t.Fatalf("single: %+v", f)
	}
	if f.Location != "Zamek, Św. Marcin 80" || f.TicketURL != "https://tickets.example/x" || f.CoverPath != "media/bfp/abc-960.jpg" || f.Timezone != "Europe/Warsaw" {
		t.Fatalf("single: %+v", f)
	}
	pl := strings.Index(f.Description, "Opis")
	en := strings.Index(f.Description, "Description")
	if pl < 0 || en < pl || !strings.Contains(f.Description, "Wstęp: 40 zł") || !strings.Contains(f.Description, "https://bfp.example/pl/events/abcdefgh-") ||
		!strings.Contains(f.Description, "https://bfp.example/en/events/abcdefgh-") || strings.Contains(f.Description, "Kolejne terminy") {
		t.Fatalf("description: %s", f.Description)
	}
	if f.HandedOver.Title != e.Published.Title.PL || f.Name == f.HandedOver.Title || f.HandedOver.VenueName != "Zamek" {
		t.Fatal("handed over holds the source title, not the cut name")
	}

	// Multi-day.
	md := publishable()
	md.Schedule = Schedule{Kind: KindMultiDay, StartDate: "2027-01-15", StartTime: "18:00", EndDate: "2027-01-17", EndTime: "16:00"}
	md.Title = Text{PL: "Festiwal"}
	md.Description = Text{}
	e = published(t, md, now)
	f, _ = GenerateFacebookEventFields(org, e, vs, now)
	if f.EndDate != "2027-01-17" || f.EndTime != "16:00" || strings.Contains(f.Description, "English") {
		t.Fatalf("multi-day: %+v", f)
	}

	// Recurring: the next upcoming, non-cancelled Occurrence; further dates
	// listed; none once all have started.
	r := weekly()
	r.Schedule.Exceptions = []Exception{{Date: "2026-11-05", Cancelled: true}}
	e = published(t, r, now)
	f, _ = GenerateFacebookEventFields(org, e, vs, now)
	if f.OccurrenceKey != "2026-11-12" || !strings.Contains(f.Description, "Kolejne terminy:") || !strings.Contains(f.Description, "czwartek, 19 listopada 2026, 19:00") {
		t.Fatalf("recurring: %s / %s", f.OccurrenceKey, f.Description)
	}
	if _, ok := GenerateFacebookEventFields(org, e, vs, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)); ok {
		t.Fatal("no upcoming Occurrence")
	}
}

func TestSnapshotFacebookEventURLMatchesContract(t *testing.T) {
	org := testOrg()
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	vs := venuesFB()
	e := published(t, weekly(), now)
	occ := e.Published.Schedule.Occurrences(warsaw, "zamek")
	if _, err := e.AddFacebookEvent("l1", "https://www.facebook.com/events/111111/", occ[1].Key, e.HandOverValues(occ[1], vs), warsaw, "u", now); err != nil {
		t.Fatal(err)
	}
	s := BuildSnapshot(org, []*Event{e}, vs, func(p string) string { return "https://store.example/pub/" + p }, now)
	if s.Events[0].Occurrences[1].FacebookEventURL != "https://www.facebook.com/events/111111/" || s.Events[0].Occurrences[0].FacebookEventURL != "" {
		t.Fatalf("snapshot: %+v", s.Events[0].Occurrences[:2])
	}
	data, _ := json.Marshal(s)
	raw, err := os.ReadFile("../../../contract/events.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	sch, err := schema.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := sch.Validate(data); err != nil {
		t.Fatalf("contract: %v", err)
	}
}
