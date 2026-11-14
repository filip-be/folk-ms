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

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, warsaw)
	if err != nil {
		panic(err)
	}
	return t
}

func fbPart() FacebookPart {
	return FacebookPart{
		PageID: "1001", EventID: "500", Name: "Bal listopadowy", Description: "Opis",
		Location: &Location{Name: "Zamek", Address: "Św. Marcin 80"}, TicketURL: "https://bilety.example/1",
		CoverID: "c1", Occurrences: []Occurrence{{FacebookID: "500", Start: at("2026-11-21 19:00"), End: at("2026-11-22 01:00")}},
	}
}

func synced() *Event {
	return NewSyncedEvent("aaaaaaaaaaaaaaaa", "balfolk-poznan", "abcdefgh", fbPart(), at("2026-11-01 12:00"))
}

func TestEffectiveAndOverrides(t *testing.T) {
	now := at("2026-11-02 10:00")
	e := synced()
	if v := e.Effective(); v.Name.PL != "Bal listopadowy" || v.Location.Name != "Zamek" || v.Cancelled {
		t.Fatalf("%+v", v)
	}
	if err := e.SetOverride(OverrideInput{Field: FieldName, Text: "Bal!"}, "u", now); err != nil {
		t.Fatal(err)
	}
	if err := e.SetOverride(OverrideInput{Field: FieldTicketURL, Text: "javascript:alert(1)"}, "u", now); !IsValidation(err) {
		t.Fatalf("ticket link must be https: %v", err)
	}
	if err := e.SetOverride(OverrideInput{Field: "start", Text: "x"}, "u", now); !IsValidation(err) {
		t.Fatal("dates are never overridable")
	}
	if e.Effective().Name.PL != "Bal!" || len(e.OverrideChanged()) != 0 {
		t.Fatal("override wins, nothing changed yet")
	}
	// Facebook changes the overridden name and the description.
	f := fbPart()
	f.Name, f.Description = "Bal listopadowy 2026", "Nowy opis"
	if changed, _ := e.ApplyFacebook(f, now); !changed {
		t.Fatal("changed")
	}
	if e.Effective().Name.PL != "Bal!" {
		t.Fatal("the override stays")
	}
	if got := e.OverrideChanged(); len(got) != 1 || got[0] != FieldName {
		t.Fatalf("flagged %v", got)
	}
	if err := e.SetOverride(OverrideInput{Field: FieldName, Keep: true}, "u", now); err != nil || len(e.OverrideChanged()) != 0 {
		t.Fatal("keep confirms against the new Facebook value")
	}
	_ = e.SetOverride(OverrideInput{Field: FieldName, Follow: true}, "u", now)
	if e.Effective().Name.PL != "Bal listopadowy 2026" {
		t.Fatal("follow Facebook again")
	}
	if e.Flags("1001").FormerPage || !e.Flags("2002").FormerPage {
		t.Fatal("former page flag")
	}
}

func TestTextOutOfDate(t *testing.T) {
	now := at("2026-11-02 10:00")
	e := synced()
	if err := e.SetExtras(ExtrasInput{NameEN: "November bal", Summary: Text{PL: "Krótko"}, Price: Text{PL: "40 zł"}}, "u", now); err != nil {
		t.Fatal(err)
	}
	if e.TextOutOfDate() || e.Effective().Name.EN != "November bal" {
		t.Fatal("fresh")
	}
	f := fbPart()
	f.Description = "Zmieniony"
	_, _ = e.ApplyFacebook(f, now)
	if !e.TextOutOfDate() {
		t.Fatal("decision 16: flagged when the Facebook text changes")
	}
	if err := e.DismissTextFlag("u", now); err != nil || e.TextOutOfDate() {
		t.Fatal("dismissed")
	}
	if err := e.SetExtras(ExtrasInput{Summary: Text{EN: "only english"}}, "u", now); !IsValidation(err) {
		t.Fatal("English requires Polish")
	}
}

func TestApplyFacebookKeepsCoverAndModeratorChoices(t *testing.T) {
	now := at("2026-11-02 10:00")
	e := synced()
	e.Facebook.Cover = &Image{ID: "img"}
	_ = e.SetHidden(true, "u", now)
	if changed, _ := e.ApplyFacebook(fbPart(), now); changed {
		t.Fatal("nothing changed")
	}
	if e.Facebook.Cover == nil || !e.Hidden {
		t.Fatal("imported cover and Hide kept")
	}
	f := fbPart()
	f.CoverID = "c2"
	_, _ = e.ApplyFacebook(f, now)
	if e.Facebook.Cover != nil {
		t.Fatal("a new cover id needs a new import")
	}
	f.Cancelled = true
	_, _ = e.ApplyFacebook(f, now)
	if !e.Effective().Cancelled || !e.Effective().Occurrences[0].Cancelled {
		t.Fatal("cancelled")
	}
	if _, err := NewManualEvent("m", "o", "s", ManualPart{}, Extras{}, "u", now).ApplyFacebook(f, now); err == nil {
		t.Fatal("Sync never touches a Manual Event")
	}
}

func TestSanitizeFacebook(t *testing.T) {
	f := SanitizeFacebook(FacebookPart{
		Name: "  <b>Bal</b>\u202e\x00 ", Description: strings.Repeat("ą", MaxDescriptionLen+10), TicketURL: "javascript:alert(1)",
		Location:    &Location{Name: "", Address: "ul. X 1", MapURL: "http://maps.example"},
		Occurrences: []Occurrence{{FacebookID: "2", Start: at("2026-12-02 19:00"), End: at("2026-12-01 19:00")}, {FacebookID: "x1", Start: at("2026-12-01 19:00")}, {FacebookID: "1", Start: at("2026-12-01 19:00")}},
	})
	if f.Name != "<b>Bal</b>" || len([]rune(f.Description)) != MaxDescriptionLen || f.TicketURL != "" {
		t.Fatalf("%q %d %q", f.Name, len([]rune(f.Description)), f.TicketURL)
	}
	if f.Location.Name != "ul. X 1" || f.Location.MapURL != "" {
		t.Fatalf("%+v", f.Location)
	}
	if len(f.Occurrences) != 2 || f.Occurrences[0].FacebookID != "1" || f.Occurrences[1].End != f.Occurrences[1].Start {
		t.Fatalf("%+v", f.Occurrences)
	}
}

func TestFollowed(t *testing.T) {
	e := synced()
	if !e.Followed("1001", at("2026-11-22 12:00")) || e.Followed("1001", at("2026-11-23 02:00")) {
		t.Fatal("followed until one day after the last end")
	}
	if e.Followed("2002", at("2026-11-02 00:00")) {
		t.Fatal("not followed from a former page")
	}
}

func TestManualEvent(t *testing.T) {
	in := ManualInput{
		Name: Text{PL: "Bal u sąsiadów"}, FacebookURL: "https://m.facebook.com/events/123456/?ref=x",
		Location: &Location{Name: "Dom kultury"},
		Occurrences: []OccurrenceInput{
			{StartDate: "2026-12-05", StartTime: "19:00", EndTime: "01:00"},
			{StartDate: "2026-12-01", StartTime: "18:00", EndDate: "2026-12-03", EndTime: "16:00"},
		},
	}
	m, x, err := ValidateManual(in, warsaw)
	if err != nil {
		t.Fatal(err)
	}
	if m.FacebookEventID != "123456" || len(m.Occurrences) != 2 || !m.Occurrences[0].Start.Equal(at("2026-12-01 18:00")) {
		t.Fatalf("%+v", m)
	}
	if !m.Occurrences[1].End.Equal(at("2026-12-06 01:00")) {
		t.Fatal("ends after midnight")
	}
	e := NewManualEvent("m1", "o", "abcdefgh", m, x, "u", at("2026-11-01 10:00"))
	if e.FacebookID() != "123456" || e.Effective().Occurrences[0].FacebookID != "123456" {
		t.Fatal("link")
	}
	m2 := m
	m2.FacebookEventID = "999"
	if err := e.UpdateManual(m2, x, "u", at("2026-11-01 10:00")); !IsValidation(err) {
		t.Fatal("the link cannot change")
	}
	for _, bad := range []string{"", "https://evil.example/events/1/", "https://www.facebook.com/groups/1", "javascript:alert(1)", "https://www.facebook.com.evil.example/events/1/", "https://user@www.facebook.com/events/1/"} {
		in.FacebookURL = bad
		if _, _, err := ValidateManual(in, warsaw); !IsValidation(err) {
			t.Errorf("accepted %q", bad)
		}
	}
	in.FacebookURL, in.Occurrences = "123", nil
	if _, _, err := ValidateManual(in, warsaw); !IsValidation(err) {
		t.Fatal("at least one date")
	}
}

func TestSyncLease(t *testing.T) {
	now := at("2026-11-02 10:00")
	var s SyncStatus
	if err := s.Claim("a", SyncNowInterval, now); err != nil {
		t.Fatal(err)
	}
	if err := s.Claim("b", 0, now.Add(time.Minute)); !errors.Is(err, ErrConflict) {
		t.Fatal("held")
	}
	s.Finish("a", "u", SyncResult{}, nil, now.Add(10*time.Second))
	if err := s.Claim("b", SyncNowInterval, now.Add(30*time.Second)); !errors.Is(err, ErrTooSoon) {
		t.Fatal("too soon")
	}
	if err := s.Claim("b", SyncNowInterval, now.Add(2*time.Minute)); err != nil || s.FirstSyncAt == nil {
		t.Fatal(err)
	}
	if err := s.Claim("c", 0, now.Add(2*time.Minute+SyncLease+time.Second)); err != nil {
		t.Fatal("an abandoned lease expires")
	}
}

func TestRecentSignIn(t *testing.T) {
	now := at("2026-11-02 10:00")
	u := &User{ID: "u"}
	u.RecordSignIn("n1", now)
	p := Principal{User: u, SignedInAt: u.SignedInAt("n1")}
	if !p.RecentlySignedIn(now.Add(14*time.Minute)) || p.RecentlySignedIn(now.Add(16*time.Minute)) {
		t.Fatal("15 minutes")
	}
	if u.SignedInAt("forged") != nil || u.SignedInAt("") != nil {
		t.Fatal("unknown nonce")
	}
	for i := range maxSignIns + 2 {
		u.RecordSignIn(string(rune('a'+i)), now)
	}
	if len(u.SignIns) != maxSignIns || u.SignedInAt("n1") != nil {
		t.Fatal("bounded")
	}
	if !u.ForgetSignIn("c") || u.SignedInAt("c") != nil || u.ForgetSignIn("c") || len(u.SignIns) != maxSignIns-1 {
		t.Fatal("sign-out forgets the session")
	}
}

func TestSnapshotMatchesSchema(t *testing.T) {
	org := testOrg()
	now := at("2026-11-02 10:00")
	e := synced()
	e.Facebook.Cover = &Image{ID: "c", Variants: []ImageVariant{{Width: 960, Height: 540, Path: "media/x-960.jpg"}}}
	_ = e.SetExtras(ExtrasInput{NameEN: "November bal", Price: Text{PL: "40 zł"}}, "u", now)
	rep := NewSyncedEvent("bbbbbbbbbbbbbbbb", org.ID, "bcdefgh2", FacebookPart{PageID: "1001", EventID: "600", Name: "Jam", Online: true,
		Occurrences: []Occurrence{{FacebookID: "602", Start: at("2026-12-03 19:00"), End: at("2026-12-03 22:00")}, {FacebookID: "601", Start: at("2026-11-05 19:00"), End: at("2026-11-05 22:00")}}}, now)
	hidden := NewSyncedEvent("cccccccccccccccc", org.ID, "cdefgh23", fbPart(), now)
	_ = hidden.SetHidden(true, "u", now)
	binned := NewSyncedEvent("dddddddddddddddd", org.ID, "defgh234", fbPart(), now)
	_ = binned.Bin(BinGone, "sync", now)
	s := BuildSnapshot(org, []*Event{e, rep, hidden, binned}, func(p string) string { return "https://store.example/pub/" + p }, now)
	if len(s.Events) != 2 || s.Events[0].ID != rep.ID {
		t.Fatalf("hidden and binned left out, ordered by first date: %+v", s.Events)
	}
	if o := s.Events[0].Occurrences; o[0].FacebookEventURL != "https://www.facebook.com/events/601/" || !o[0].Online {
		t.Fatalf("%+v", o)
	}
	ev := s.Events[1]
	if ev.Permalink[EN] != "https://bfp.example/en/events/abcdefgh-bal-listopadowy" || ev.Title.EN != "November bal" || ev.Price.URL != "https://bilety.example/1" {
		t.Fatalf("%+v", ev)
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
	empty, _ := json.Marshal(BuildSnapshot(org, nil, nil, now))
	if err := sch.Validate(empty); err != nil || !bytes.Contains(empty, []byte(`"events":[]`)) {
		t.Fatalf("empty snapshot: %v %s", err, empty)
	}
}

func TestCaption(t *testing.T) {
	org := testOrg()
	now := at("2026-11-01 12:00")
	e := synced()
	_ = e.SetExtras(ExtrasInput{NameEN: "November bal", Price: Text{PL: "40 zł"}}, "u", now)
	pl := GenerateCaption(org, e, PL, now)
	for _, want := range []string{"Bal listopadowy", "sobota, 21 listopada 2026, 19:00", "Gdzie: Zamek, Św. Marcin 80", "Wstęp: 40 zł", "https://bfp.example/pl/events/abcdefgh-bal-listopadowy"} {
		if !strings.Contains(pl, want) {
			t.Errorf("PL caption lacks %q:\n%s", want, pl)
		}
	}
	en := GenerateCaption(org, e, EN, now)
	if !strings.Contains(en, "November bal") || !strings.Contains(en, "Saturday, 21 November 2026") || !strings.Contains(en, "/en/events/") {
		t.Errorf("EN caption:\n%s", en)
	}
}

func TestShortIDAndLinks(t *testing.T) {
	id, err := GenerateShortID(nil, func(string) bool { return false })
	if err != nil || !IsShortID(id) {
		t.Fatalf("%q %v", id, err)
	}
	if !IsHTTPSURL("https://a.example/x?y=1") || IsHTTPSURL("http://a.example") || IsHTTPSURL("https://a.example/\"><") || IsHTTPSURL("https:///x") {
		t.Fatal("https check")
	}
}

func TestDailyPostLimit(t *testing.T) {
	c := PostCounter{}
	if err := c.Take(DailyPostLimit); err != nil {
		t.Fatal(err)
	}
	if err := c.Take(1); !errors.Is(err, ErrLimit) {
		t.Fatal("limit")
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

func TestCheckImage(t *testing.T) {
	good := &Image{ID: "0123456789abcdef", Alt: Text{PL: " opis\x00 "}, Variants: []ImageVariant{{Width: 960, Height: 540, Path: "media/org-a/0123456789abcdef-960.jpg"}},
		Instagram: &ImageVariant{Width: 1080, Height: 1350, Path: "media/org-a/0123456789abcdef-ig.jpg"}}
	img, err := CheckImage("org-a", good, "cover")
	if err != nil || img.Alt.PL != "opis" {
		t.Fatalf("%v %+v", err, img)
	}
	for _, path := range []string{"media/org-b/0123456789abcdef-960.jpg", "media/org-a/0123456789abcdef-960.png", "https://evil.example/x.jpg", "media/org-a/0123456789abcdef-../../x.jpg"} {
		bad := *good
		bad.Variants = []ImageVariant{{Width: 1, Height: 1, Path: path}}
		if _, err := CheckImage("org-a", &bad, "cover"); !IsValidation(err) {
			t.Errorf("accepted %q", path)
		}
	}
	if img, err := CheckImage("org-a", nil, "cover"); img != nil || err != nil {
		t.Fatal("nil is no image")
	}
}
