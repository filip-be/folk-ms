package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/schema"
)

var warsaw = func() *time.Location {
	l, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		panic(err)
	}
	return l
}()

func testOrg() *Organization {
	o := &Organization{
		ID: "balfolk-poznan", Name: Text{PL: "Bal Folk Poznań"}, Timezone: "Europe/Warsaw",
		Site: Site{ID: "balfolk-poznan", BaseURL: "https://bfp.example", PermalinkTemplate: "/{lang}/events/{shortId}-{slug}"},
	}
	o.SetLocation(warsaw)
	return o
}

func dates(occ []Occurrence) []string {
	var out []string
	for _, o := range occ {
		out = append(out, o.Start.Format("2006-01-02 15:04 -07:00"))
	}
	return out
}

func TestWeeklyAcrossDST(t *testing.T) {
	s := Schedule{Kind: KindRecurring, StartDate: "2026-10-20", StartTime: "19:00", EndTime: "22:00",
		Recurrence: &Recurrence{Freq: Weekly, Weekday: int(time.Thursday), EndDate: "2026-11-05"}}
	got := dates(s.Occurrences(warsaw, "v"))
	want := []string{"2026-10-22 19:00 +02:00", "2026-10-29 19:00 +01:00", "2026-11-05 19:00 +01:00"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestBiweekly(t *testing.T) {
	s := Schedule{Kind: KindRecurring, StartDate: "2026-01-01", StartTime: "18:00", EndTime: "20:00",
		Recurrence: &Recurrence{Freq: Biweekly, Weekday: int(time.Monday), EndDate: "2026-02-01"}}
	got := dates(s.Occurrences(warsaw, "v"))
	want := []string{"2026-01-05 18:00 +01:00", "2026-01-19 18:00 +01:00"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v", got)
	}
}

func TestMonthlyNthAndLast(t *testing.T) {
	last := Schedule{Kind: KindRecurring, StartDate: "2026-01-01", StartTime: "19:00", EndTime: "22:00",
		Recurrence: &Recurrence{Freq: Monthly, Weekday: int(time.Friday), Nth: -1, EndDate: "2026-04-30"}}
	got := dates(last.Occurrences(warsaw, "v"))
	want := []string{"2026-01-30 19:00 +01:00", "2026-02-27 19:00 +01:00", "2026-03-27 19:00 +01:00", "2026-04-24 19:00 +02:00"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("last: got %v", got)
	}
	second := Schedule{Kind: KindRecurring, StartDate: "2026-03-11", StartTime: "19:00", EndTime: "22:00",
		Recurrence: &Recurrence{Freq: Monthly, Weekday: int(time.Tuesday), Nth: 2, EndDate: "2026-05-31"}}
	got = dates(second.Occurrences(warsaw, "v"))
	// 2nd Tuesday of March is the 10th, before the start date: skipped.
	want = []string{"2026-04-14 19:00 +02:00", "2026-05-12 19:00 +02:00"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("2nd: got %v", got)
	}
}

func TestOvernightAndDSTNight(t *testing.T) {
	// Night of the autumn DST change: 2026-10-25 03:00 CEST -> 02:00 CET.
	s := Schedule{Kind: KindSingle, StartDate: "2026-10-24", StartTime: "20:00", EndTime: "02:00"}
	occ := s.Occurrences(warsaw, "v")
	if len(occ) != 1 {
		t.Fatal("want one occurrence")
	}
	if d := occ[0].End.Sub(occ[0].Start); d != 7*time.Hour {
		t.Fatalf("overnight across DST should last 7h, got %v", d)
	}
}

func TestExceptions(t *testing.T) {
	s := Schedule{Kind: KindRecurring, StartDate: "2026-11-01", StartTime: "19:00", EndTime: "22:00",
		Recurrence: &Recurrence{Freq: Weekly, Weekday: int(time.Thursday), EndDate: "2026-11-30"},
		Exceptions: []Exception{
			{Date: "2026-11-12", Cancelled: true, Note: Text{PL: "Odwołane"}},
			{Date: "2026-11-19", StartTime: "18:00", VenueID: "other"},
		}}
	occ := s.Occurrences(warsaw, "main")
	if len(occ) != 4 {
		t.Fatalf("got %d", len(occ))
	}
	if !occ[1].Cancelled || occ[1].Note.PL != "Odwołane" {
		t.Fatalf("cancel not applied: %+v", occ[1])
	}
	if occ[2].Start.Hour() != 18 || occ[2].VenueID != "other" || occ[0].VenueID != "main" {
		t.Fatalf("override not applied: %+v", occ[2])
	}
	bad := s
	bad.Exceptions = []Exception{{Date: "2026-11-13", Cancelled: true}}
	if err := bad.Validate(); err == nil {
		t.Fatal("exception on a non-occurrence date must fail")
	}
}

func TestScheduleValidation(t *testing.T) {
	cases := map[string]Schedule{
		"too long recurrence": {Kind: KindRecurring, StartDate: "2026-01-01", StartTime: "19:00", EndTime: "20:00",
			Recurrence: &Recurrence{Freq: Weekly, Weekday: 1, EndDate: "2028-01-02"}},
		"missing end date": {Kind: KindRecurring, StartDate: "2026-01-01", StartTime: "19:00", EndTime: "20:00",
			Recurrence: &Recurrence{Freq: Weekly, Weekday: 1}},
		"monthly without nth": {Kind: KindRecurring, StartDate: "2026-01-01", StartTime: "19:00", EndTime: "20:00",
			Recurrence: &Recurrence{Freq: Monthly, Weekday: 1, EndDate: "2026-06-01"}},
		"multi-day backwards": {Kind: KindMultiDay, StartDate: "2026-01-05", EndDate: "2026-01-04", StartTime: "10:00", EndTime: "18:00"},
		"bad time":            {Kind: KindSingle, StartDate: "2026-01-05", StartTime: "25:00", EndTime: "18:00"},
		"no kind":             {StartDate: "2026-01-05", StartTime: "10:00", EndTime: "18:00"},
	}
	for name, s := range cases {
		if err := s.Validate(); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	ok := Schedule{Kind: KindRecurring, StartDate: "2026-01-01", StartTime: "19:00", EndTime: "20:00",
		Recurrence: &Recurrence{Freq: Weekly, Weekday: 1, EndDate: "2028-01-01"}}
	if err := ok.Validate(); err != nil {
		t.Errorf("exactly 24 months must be fine: %v", err)
	}
}

func publishable() Content {
	return Content{
		Title:    Text{PL: "Bal listopadowy", EN: "November bal"},
		Summary:  Text{PL: "Muzyka na żywo"},
		Schedule: Schedule{Kind: KindSingle, StartDate: "2026-11-21", StartTime: "19:00", EndTime: "01:00"},
		VenueID:  "zamek",
		Price:    Price{Text: Text{PL: "40 zł"}, URL: "https://tickets.example/x"},
		Cover: &Image{ID: "abc", Variants: []ImageVariant{{Width: 960, Height: 540, Path: "media/bfp/abc-960.jpg"}},
			Instagram: &ImageVariant{Width: 1080, Height: 1080, Path: "media/bfp/abc-ig.jpg"}},
	}
}

var venueOK = func(id string) bool { return id == "zamek" || id == "other" }

func TestLifecycle(t *testing.T) {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	e, err := NewEvent("e1e1e1e1e1e1e1e1", "balfolk-poznan", "abcdefgh", Content{Title: Text{PL: "Bal Śląski — zażółć"}}, "u1", now)
	if err != nil {
		t.Fatal(err)
	}
	if e.Draft.Slug != "bal-slaski-zazolc" {
		t.Fatalf("slug %q", e.Draft.Slug)
	}
	if err := e.Publish(venueOK, "u1", now); !IsValidation(err) {
		t.Fatalf("incomplete draft must not publish: %v", err)
	}
	c := publishable()
	if err := e.UpdateDraft(c, "u1", now); err != nil {
		t.Fatal(err)
	}
	if err := e.Publish(venueOK, "u1", now); err != nil {
		t.Fatal(err)
	}
	if e.Status() != StatusPublished || e.HasUnpublishedChanges() {
		t.Fatal("should be published without changes")
	}
	c.Title.PL = "Bal zmieniony"
	_ = e.UpdateDraft(c, "u1", now)
	if !e.HasUnpublishedChanges() || e.Published.Title.PL != "Bal listopadowy" {
		t.Fatal("site must keep published version until next Publish")
	}
	was, err := e.Delete("u1", now)
	if err != nil || !was || e.Status() != StatusBinned || e.Published != nil {
		t.Fatalf("delete of published must unpublish: %v %v %v", was, err, e.Status())
	}
	if err := e.Publish(venueOK, "u1", now); !errors.Is(err, ErrInvalidState) {
		t.Fatal("binned event must not publish")
	}
	if e.Purgeable(now.Add(29 * 24 * time.Hour)) {
		t.Fatal("not purgeable before 30 days")
	}
	if !e.Purgeable(now.Add(31 * 24 * time.Hour)) {
		t.Fatal("purgeable after 30 days")
	}
	if err := e.Restore("admin", now.Add(24*time.Hour)); err != nil || e.Status() != StatusDraft {
		t.Fatalf("restore: %v", err)
	}
	d := e.Duplicate("d1d1d1d1d1d1d1d1", "bcdefgh2", "u1", now)
	if d.Draft.Schedule.StartDate != "" || d.Draft.Title.PL != "Bal zmieniony" || d.DuplicatedFrom != e.ID || d.Published != nil {
		t.Fatalf("duplicate: %+v", d.Draft)
	}
}

func TestEnglishRequiresPolish(t *testing.T) {
	c := Content{Title: Text{EN: "Only English"}}
	if err := c.ValidateDraft(); err == nil {
		t.Fatal("want error")
	}
}

func TestUniqueSlugAndShortID(t *testing.T) {
	taken := map[string]bool{"bal": true, "bal-2": true}
	if s := UniqueSlug("bal", func(s string) bool { return taken[s] }); s != "bal-3" {
		t.Fatal(s)
	}
	id, err := GenerateShortID(nil, func(string) bool { return false })
	if err != nil || !IsShortID(id) {
		t.Fatalf("%q %v", id, err)
	}
	if _, err := GenerateShortID(nil, func(string) bool { return true }); err == nil {
		t.Fatal("want collision error")
	}
}

func TestSnapshotMatchesSchemaAndFreezesVenues(t *testing.T) {
	org := testOrg()
	pub := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	single, _ := NewEvent("aaaaaaaaaaaaaaaa", org.ID, "abcdefgh", publishable(), "u", pub)
	if err := single.Publish(venueOK, "u", pub); err != nil {
		t.Fatal(err)
	}
	jam := Content{Title: Text{PL: "Jam"}, VenueID: "zamek", Schedule: Schedule{Kind: KindRecurring, StartDate: "2026-01-01",
		StartTime: "19:00", EndTime: "22:00", Recurrence: &Recurrence{Freq: Monthly, Weekday: 4, Nth: -1, EndDate: "2026-12-31"},
		Exceptions: []Exception{{Date: "2026-10-29", Cancelled: true, Note: Text{PL: "Remont"}}}}}
	rec, _ := NewEvent("bbbbbbbbbbbbbbbb", org.ID, "bcdefgh2", jam, "u", pub)
	if err := rec.Publish(venueOK, "u", pub); err != nil {
		t.Fatal(err)
	}
	draft, _ := NewEvent("cccccccccccccccc", org.ID, "cdefgh23", Content{Title: Text{PL: "Szkic"}}, "u", pub)

	old := Venue{ID: "zamek", Name: "Stara sala", Address: "ul. Stara 1"}
	editAt := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if !rec.FreezePastVenue(old, warsaw, editAt) {
		t.Fatal("past occurrences should be frozen")
	}
	if len(rec.PastVenues) != 2 { // Jan 29, Feb 26
		t.Fatalf("frozen %d", len(rec.PastVenues))
	}
	if rec.FreezePastVenue(Venue{ID: "zamek", Name: "Newer"}, warsaw, editAt) {
		t.Fatal("already frozen occurrences must keep their first copy")
	}
	venues := map[string]Venue{"zamek": {ID: "zamek", Name: "Zamek", Address: "Św. Marcin 80"}}
	s := BuildSnapshot(org, []*Event{rec, single, draft}, venues, func(p string) string { return "https://store.example/pub/" + p }, editAt)
	if len(s.Events) != 2 {
		t.Fatalf("want 2 published events, got %d", len(s.Events))
	}
	if s.Events[0].ID != rec.ID {
		t.Fatal("events ordered by first occurrence")
	}
	first := s.Events[0].Occurrences[0]
	if s.Venues[first.VenueID].Name != "Stara sala" {
		t.Fatalf("frozen venue not used: %v", first.VenueID)
	}
	if s.Venues[s.Events[0].Occurrences[5].VenueID].Name != "Zamek" {
		t.Fatal("future occurrences use the current venue")
	}
	if s.Events[1].Permalink[EN] != "https://bfp.example/en/events/abcdefgh-bal-listopadowy" {
		t.Fatal(s.Events[1].Permalink[EN])
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../../contract/events.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	sch, err := schema.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := sch.Validate(data); err != nil {
		t.Fatalf("snapshot does not match contract: %v\n%s", err, data)
	}
	empty, _ := json.Marshal(BuildSnapshot(org, nil, nil, nil, editAt))
	if err := sch.Validate(empty); err != nil || !bytes.Contains(empty, []byte(`"events":[]`)) {
		t.Fatalf("empty snapshot: %v %s", err, empty)
	}
}

func TestCaption(t *testing.T) {
	org := testOrg()
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	e, _ := NewEvent("aaaaaaaaaaaaaaaa", org.ID, "abcdefgh", publishable(), "u", now)
	_ = e.Publish(venueOK, "u", now)
	venues := map[string]Venue{"zamek": {ID: "zamek", Name: "Zamek", Address: "Św. Marcin 80"}}
	pl := GenerateCaption(org, e, venues, PL, now)
	for _, want := range []string{"Bal listopadowy", "sobota, 21 listopada 2026, 19:00", "Gdzie: Zamek, Św. Marcin 80", "Wstęp: 40 zł", "https://bfp.example/pl/events/abcdefgh-bal-listopadowy"} {
		if !strings.Contains(pl, want) {
			t.Errorf("PL caption lacks %q:\n%s", want, pl)
		}
	}
	en := GenerateCaption(org, e, venues, EN, now)
	if !strings.Contains(en, "November bal") || !strings.Contains(en, "Saturday, 21 November 2026") || !strings.Contains(en, "/en/events/") {
		t.Errorf("EN caption:\n%s", en)
	}
}

func TestInvitationLifecycle(t *testing.T) {
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	known := func(o string) bool { return o == "a" || o == "b" }
	if _, err := NewInvitation("i", "bad", []string{"a"}, known, "sa", now); err == nil {
		t.Fatal("bad email")
	}
	if _, err := NewInvitation("i", "x@y.pl", []string{"zzz"}, known, "sa", now); err == nil {
		t.Fatal("unknown org")
	}
	inv, err := NewInvitation("i", " Ola@Example.PL ", []string{"b", "a", "a"}, known, "sa", now)
	if err != nil {
		t.Fatal(err)
	}
	if inv.Email != "ola@example.pl" || len(inv.Orgs) != 2 {
		t.Fatalf("%+v", inv)
	}
	if !inv.Usable("OLA@example.pl", now.Add(13*24*time.Hour)) {
		t.Fatal("usable within 14 days")
	}
	if inv.Usable("ola@example.pl", now.Add(15*24*time.Hour)) || inv.EffectiveStatus(now.Add(15*24*time.Hour)) != InvitationExpired {
		t.Fatal("expired after 14 days")
	}
	if err := inv.Revoke(now); err != nil || inv.Usable("ola@example.pl", now) {
		t.Fatal("revoked")
	}
	if err := inv.Revoke(now); err == nil {
		t.Fatal("cannot revoke twice")
	}
}

func TestAccess(t *testing.T) {
	u := &User{ID: "u", Orgs: []string{"a"}}
	if !(Principal{User: u}).CanManageOrg("a") || (Principal{User: u}).CanManageOrg("b") {
		t.Fatal("scoped per org")
	}
	if !(Principal{User: u, SuperAdmin: true}).CanManageOrg("b") {
		t.Fatal("super admin manages all")
	}
	now := time.Now()
	removed := u.Revoke(now)
	if len(removed) != 1 || (Principal{User: u}).CanManageOrg("a") {
		t.Fatal("revoked")
	}
	c := &Connection{CreatedBy: "u"}
	if !c.FlagIfCreatedBy("u", "x@y") || !c.NeedsReconnect || c.FlagIfCreatedBy("u", "x@y") {
		t.Fatal("flag once")
	}
}

func TestSocialRetryRule(t *testing.T) {
	now := time.Now()
	ep := &EventPosts{EventID: "e"}
	p, err := ep.Begin("p1", Facebook, "Hello", "img", "u", false, now)
	if err != nil || p.Status != PostPending {
		t.Fatal(err)
	}
	if _, err := ep.Begin("p2", Facebook, "Hello", "img", "u", false, now); !errors.Is(err, ErrNeedsConfirm) {
		t.Fatalf("pending without confirmation must be refused: %v", err)
	}
	if _, err := ep.Begin("p3", Instagram, "Hello", "img", "u", false, now); err != nil {
		t.Fatal("platforms are independent")
	}
	ep.Find("p1").Fail("boom", now)
	if _, err := ep.Begin("p4", Facebook, "Hello", "img", "u", true, now); err != nil {
		t.Fatal(err)
	}
	if _, err := ep.Begin("p5", Instagram, strings.Repeat("x", 2201), "img", "u", true, now); !IsValidation(err) {
		t.Fatal("caption too long")
	}
}
