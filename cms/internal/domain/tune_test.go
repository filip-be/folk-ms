package domain

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

var tuneNow = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func tuneOrg() *Organization {
	return &Organization{ID: "org", Name: Text{PL: "Org"}, Timezone: "Europe/Warsaw", Site: Site{ID: "site", BaseURL: "https://site.example",
		PermalinkTemplate: "/{lang}/events/{shortId}-{slug}",
		Tunes:             &SiteTunes{TunePermalinkTemplate: "/{lang}/tunes/{shortId}-{slug}", SetPermalinkTemplate: "/{lang}/sets/{shortId}-{slug}"}}}
}

func typeID(c *TuneCollection, en string) string {
	for _, t := range c.Types {
		if t.Label.EN == en {
			return t.ID
		}
	}
	panic(en)
}

func instrumentID(c *TuneCollection, en string) string {
	for _, t := range c.Instruments {
		if t.Label.EN == en {
			return t.ID
		}
	}
	panic(en)
}

func fields(err error) []string {
	var ve *ValidationError
	if !errors.As(err, &ve) {
		return nil
	}
	var out []string
	for _, p := range ve.Problems {
		out = append(out, p.Field)
	}
	return out
}

func reel(c *TuneCollection, name string) TuneInput {
	return TuneInput{Name: TuneName{Name: name}, TypeID: typeID(c, "reel"), Key: Key{"D", "major"}, Meter: "4/4"}
}

// addTune prepares and adds a Tune, failing the test on any error.
func addTune(t *testing.T, c *TuneCollection, in TuneInput) string {
	t.Helper()
	tu, err := c.PrepareTune(in)
	if err != nil {
		t.Fatal(err)
	}
	id, err := c.NewItemID(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.AddTune(tu, id, false, "u", tuneNow); err != nil {
		t.Fatal(err)
	}
	return id
}

func newID(t *testing.T, c *TuneCollection) string {
	t.Helper()
	id, err := c.NewItemID(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestNewTuneCollectionIsSeededDeterministically(t *testing.T) {
	a, b := NewTuneCollection("org"), NewTuneCollection("org")
	if len(a.Types) != 12 || len(a.Instruments) != 13 || len(a.Tunes) != 0 || a.Sets == nil || a.Origins == nil {
		t.Fatalf("seed: %+v", a)
	}
	for i := range a.Types {
		if a.Types[i].ID != b.Types[i].ID || !IsShortID(a.Types[i].ID) || a.Types[i].Rev != 1 || !ValidMeter(a.Types[i].Meter) {
			t.Fatalf("type %d: %+v / %+v", i, a.Types[i], b.Types[i])
		}
	}
	if a.Instruments[0].ID != b.Instruments[0].ID {
		t.Fatal("instrument ids differ between loads")
	}
}

func TestPrepareTuneValidation(t *testing.T) {
	c := NewTuneCollection("org")
	long := strings.Repeat("x", MaxTuneNameLen+1)
	whistle, flute := instrumentID(c, "whistle"), instrumentID(c, "flute")
	in := TuneInput{
		Name:        TuneName{Name: " ", Translation: Bi{PL: long}},
		OtherNames:  []TuneName{{Name: "ok"}, {Translation: Bi{EN: "no name"}}, {Name: long}},
		TypeID:      "nosuchid",
		Key:         Key{"H", "major"},
		Meter:       "6/7",
		Tempo:       5,
		OriginID:    "nosuchid",
		Composer:    strings.Repeat("c", MaxComposerLen+1),
		Suits:       []string{whistle, flute},
		Tricky:      []string{flute},
		ABC:         "X:1\nT:no key\nGABc|",
		IncipitBars: 9,
		Recordings:  []RecordingInput{{Link: "http://example.org/x"}, {Link: "https://soundcloud.com/a", Label: strings.Repeat("l", MaxRecordingLabelLen+1)}, {}},
		Description: Bi{PL: strings.Repeat("d", MaxTuneDescLen+1), EN: strings.Repeat("d", MaxTuneDescLen+1)},
	}
	_, err := c.PrepareTune(in)
	want := []string{"abc", "composer", "description.en", "description.pl", "incipitBars", "key", "meter", "name.name", "name.translation.pl",
		"originId", "otherNames.1.name", "otherNames.2.name", "recordings.0.link", "recordings.1.label", "tempo", "tricky", "typeId"}
	if got := fields(err); !slices.Equal(got, want) {
		t.Fatalf("fields:\n got %v\nwant %v", got, want)
	}
	if _, err := c.PrepareTune(TuneInput{}); !slices.Equal(fields(err), []string{"key", "meter", "name.name", "typeId"}) {
		t.Fatalf("empty: %v", fields(err))
	}
	if _, err := c.PrepareTune(TuneInput{Name: TuneName{Name: "x"}, TypeID: typeID(c, "jig"), Key: Key{"G", "major"}, Meter: "6/8", Suits: []string{"nope"}}); !slices.Equal(fields(err), []string{"suits"}) {
		t.Fatalf("unknown instrument: %v", fields(err))
	}
	many := TuneInput{Name: TuneName{Name: "x"}, TypeID: typeID(c, "jig"), Key: Key{"G", "major"}, Meter: "6/8"}
	for range MaxOtherNames + 1 {
		many.OtherNames = append(many.OtherNames, TuneName{Name: "o"})
	}
	for range MaxRecordings + 1 {
		many.Recordings = append(many.Recordings, RecordingInput{Link: "https://example.org"})
	}
	many.ABC = "X:1\nK:G\n" + strings.Repeat("G", MaxABCBytes)
	if !slices.Equal(fields(func() error { _, err := c.PrepareTune(many); return err }()), []string{"abc", "otherNames", "recordings"}) {
		t.Fatal("limits")
	}

	// A valid Tune: cleaned, instruments in list order, default bars stored as 0.
	ok := TuneInput{Name: TuneName{Name: "  The Kesh ", Translation: Bi{PL: "Kesz"}}, OtherNames: []TuneName{{Name: "An Cheis"}, {}},
		TypeID: typeID(c, "jig"), Key: Key{"G", "major"}, Meter: " 6/8 ", Composer: "trad.",
		Suits: []string{flute, whistle, flute}, ABC: kesh + "\r\n", IncipitBars: DefaultIncipitBars,
		Recordings:  []RecordingInput{{Link: "https://youtu.be/dQw4w9WgXcQ?t=75", Label: "Ensemble"}, {Link: "vimeo.com/123456"}, {Link: "https://soundcloud.com/x/y"}},
		Description: Bi{EN: "**Lovely** tune"}}
	tu, err := c.PrepareTune(ok)
	if err != nil {
		t.Fatal(err)
	}
	if tu.Name.Name != "The Kesh" || tu.Meter != "6/8" || len(tu.OtherNames) != 1 || !slices.Equal(tu.Suits, []string{whistle, flute}) ||
		tu.IncipitBars != 0 || tu.ABC() != kesh || tu.Transcription.Format != TranscriptionABC || tu.Description.EN != "**Lovely** tune" {
		t.Fatalf("prepared: %+v", tu)
	}
	if r := tu.Recordings; len(r) != 3 || r[0].Video == nil || r[0].Video.Provider != YouTube || r[0].Video.StartSeconds != 75 || r[0].Label != "Ensemble" ||
		r[1].Video == nil || r[1].Video.Provider != Vimeo || r[2].Video != nil || r[2].URL != "https://soundcloud.com/x/y" {
		t.Fatalf("recordings: %+v", r)
	}
}

func TestParseRecordingLink(t *testing.T) {
	for _, bad := range []string{"", "http://soundcloud.com/x", "ftp://x.org", "https://x.org:8443/a", "javascript:alert(1)", "https://youtube.com/watch?v=bad", "https://vimeo.com/abc"} {
		if _, err := ParseRecordingLink(bad); !IsValidation(err) {
			t.Errorf("%q accepted", bad)
		}
	}
	if r, err := ParseRecordingLink("http://www.youtube.com/watch?v=dQw4w9WgXcQ"); err != nil || r.Video == nil || r.Video.ID != "dQw4w9WgXcQ" {
		t.Errorf("youtube over http: %+v %v", r, err)
	}
	if r, err := ParseRecordingLink("https://bandcamp.com/track"); err != nil || r.URL != "https://bandcamp.com/track" {
		t.Errorf("other https: %+v %v", r, err)
	}
}

func TestTuneRevisions(t *testing.T) {
	c := NewTuneCollection("org")
	id := addTune(t, c, reel(c, "Silver Spear"))
	other := addTune(t, c, reel(c, "Drowsy Maggie"))
	tu, _ := c.PrepareTune(reel(c, "The Silver Spear"))
	if err := c.UpdateTune(id, 1, tu, "u", tuneNow); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.Tune(id); got.Rev != 2 || got.Name.Name != "The Silver Spear" {
		t.Fatalf("updated: %+v", got)
	}
	// A save on the old revision of the same Tune is refused …
	if err := c.UpdateTune(id, 1, tu, "u", tuneNow); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale: %v", err)
	}
	// … one on another Tune is not.
	if err := c.UpdateTune(other, 1, tu, "u", tuneNow); err != nil {
		t.Fatalf("unrelated: %v", err)
	}
	if err := c.UpdateTune("nosuchid", 1, tu, "u", tuneNow); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	// Hide keeps the revision; Attachments survive an update.
	if err := c.SetTuneHidden(id, true, "u", tuneNow); err != nil {
		t.Fatal(err)
	}
	if err := c.AddAttachment(id, Attachment{ID: newID(t, c), Kind: AttachmentPDF, Path: "media/org/tunes/" + id + "/a.pdf"}, "u", tuneNow); err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateTune(id, 2, tu, "u", tuneNow); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.Tune(id); !got.Hidden || len(got.Attachments) != 1 || got.Rev != 3 {
		t.Fatalf("hidden and attachments kept: %+v", got)
	}

	// List entries and Sets carry their own revisions.
	jig := typeID(c, "jig")
	if err := c.UpdateTuneType(jig, 1, TuneTypeInput{Label: Bi{EN: "jig"}, Meter: "6/8", Tempo: 120}, "u", tuneNow); err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateTuneType(jig, 1, TuneTypeInput{Label: Bi{EN: "jig"}, Meter: "6/8", Tempo: 100}, "u", tuneNow); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale type: %v", err)
	}
	fl := instrumentID(c, "flute")
	if err := c.UpdateInstrument(fl, 1, Bi{PL: "flet poprzeczny"}, "u", tuneNow); err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateInstrument(fl, 1, Bi{PL: "flet"}, "u", tuneNow); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale instrument: %v", err)
	}
	sid := newID(t, c)
	if err := c.AddSet(sid, SetInput{TuneIDs: []string{id, other}}, "u", tuneNow); err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateSet(sid, 1, SetInput{TuneIDs: []string{other, id}}, "u", tuneNow); err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateSet(sid, 1, SetInput{TuneIDs: []string{id, other}}, "u", tuneNow); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale set: %v", err)
	}
	if _, err := c.RemoveSet(sid, 1, "u", tuneNow); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale set removal: %v", err)
	}
	if _, err := c.RemoveTune(other, 1, true, "u", tuneNow); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale tune removal: %v", err)
	}
}

func TestTuneListsInUseAndDuplicates(t *testing.T) {
	c := NewTuneCollection("org")
	ie, munster := newID(t, c), newID(t, c)
	if err := c.AddTuneOrigin(ie, "", Bi{EN: "Ireland"}, "u", tuneNow); err != nil {
		t.Fatal(err)
	}
	if err := c.AddTuneOrigin(munster, ie, Bi{PL: "Munster"}, "u", tuneNow); err != nil {
		t.Fatal(err)
	}
	if err := c.AddTuneOrigin(newID(t, c), "", Bi{EN: "ireland"}, "u", tuneNow); !slices.Equal(fields(err), []string{"label.pl"}) {
		t.Fatalf("duplicate country: %v", err)
	}
	if err := c.AddTuneOrigin(newID(t, c), munster, Bi{PL: "x"}, "u", tuneNow); !slices.Equal(fields(err), []string{"parentId"}) {
		t.Fatalf("region of a region: %v", err)
	}
	if err := c.AddTuneOrigin(newID(t, c), "", Bi{}, "u", tuneNow); !slices.Equal(fields(err), []string{"label.pl"}) {
		t.Fatalf("no label: %v", err)
	}
	if err := c.AddInstrument(newID(t, c), Bi{EN: "Fiddle"}, "u", tuneNow); !slices.Equal(fields(err), []string{"label.pl"}) {
		t.Fatalf("duplicate instrument: %v", err)
	}
	if err := c.AddTuneType(newID(t, c), TuneTypeInput{Label: Bi{PL: "Reel"}, Meter: "4/4", Tempo: 100}, "u", tuneNow); !slices.Equal(fields(err), []string{"label.pl"}) {
		t.Fatalf("duplicate type: %v", err)
	}
	if err := c.AddTuneType(newID(t, c), TuneTypeInput{Label: Bi{EN: "fling"}, Meter: "x", Tempo: 1}, "u", tuneNow); !slices.Equal(fields(err), []string{"meter", "tempo"}) {
		t.Fatalf("bad type: %v", err)
	}

	in := reel(c, "Tune")
	in.OriginID = munster
	in.Tricky = []string{instrumentID(c, "harp")}
	addTune(t, c, in)
	for name, err := range map[string]error{
		"type":       func() error { _, err := c.RemoveTuneType(in.TypeID, "u", tuneNow); return err }(),
		"instrument": func() error { _, err := c.RemoveInstrument(in.Tricky[0], "u", tuneNow); return err }(),
		"region":     c.RemoveTuneOrigin(munster, "u", tuneNow),
		"country":    c.RemoveTuneOrigin(ie, "u", tuneNow), // still has its region
	} {
		if !errors.Is(err, ErrInvalidState) || !slices.Equal(fields(err), []string{"label.pl"}) {
			t.Errorf("%s in use: %v", name, err)
		}
	}
	if c.TypeUse(in.TypeID) != 1 || c.InstrumentUse(in.Tricky[0]) != 1 || c.OriginUse(munster) != 1 || c.OriginUse(ie) != 0 {
		t.Fatal("use counts")
	}
	// Unused entries go; their ids are never reused.
	slip := typeID(c, "slip jig")
	if _, err := c.RemoveTuneType(slip, "u", tuneNow); err != nil {
		t.Fatal(err)
	}
	if !c.taken(slip) {
		t.Fatal("removed id may be reused")
	}
	if err := c.MoveTuneType(typeID(c, "air"), 0, "u", tuneNow); err != nil || c.Types[0].Label.EN != "air" {
		t.Fatalf("move: %v", err)
	}
}

func TestSuitsAndTrickyExclusive(t *testing.T) {
	c := NewTuneCollection("org")
	in := reel(c, "x")
	in.Suits = []string{instrumentID(c, "fiddle")}
	in.Tricky = []string{instrumentID(c, "fiddle")}
	if _, err := c.PrepareTune(in); !slices.Equal(fields(err), []string{"tricky"}) {
		t.Fatalf("suits and tricky: %v", err)
	}
}

func TestRemoveTuneInSets(t *testing.T) {
	c := NewTuneCollection("org")
	a, b, d := addTune(t, c, reel(c, "A")), addTune(t, c, reel(c, "B")), addTune(t, c, reel(c, "D"))
	keep, empty := newID(t, c), newID(t, c)
	if err := c.AddSet(keep, SetInput{TuneIDs: []string{a, b, d}}, "u", tuneNow); err != nil {
		t.Fatal(err)
	}
	if err := c.AddSet(empty, SetInput{TuneIDs: []string{a, b}}, "u", tuneNow); err != nil {
		t.Fatal(err)
	}
	if err := c.AddSet(newID(t, c), SetInput{TuneIDs: []string{a}}, "u", tuneNow); !slices.Equal(fields(err), []string{"tuneIds"}) {
		t.Fatalf("one-tune set: %v", err)
	}
	if err := c.AddSet(newID(t, c), SetInput{TuneIDs: []string{a, a}}, "u", tuneNow); !slices.Equal(fields(err), []string{"tuneIds"}) {
		t.Fatalf("repeated tune: %v", err)
	}
	// Take b out of "empty" first by removing it with confirmation, then a.
	if _, err := c.RemoveTune(b, 1, false, "u", tuneNow); !errors.Is(err, ErrNeedsConfirm) {
		t.Fatalf("needs confirm: %v", err)
	}
	r, err := c.RemoveTune(b, 1, true, "u", tuneNow)
	if err != nil || !slices.Equal(r.LeftSets, []string{keep, empty}) || len(r.RemovedSets) != 0 || r.Tune.Name.Name != "B" {
		t.Fatalf("remove b: %+v %v", r, err)
	}
	if s, _ := c.Set(keep); !slices.Equal(s.TuneIDs, []string{a, d}) || s.Rev != 2 {
		t.Fatalf("detached: %+v", s)
	}
	r, err = c.RemoveTune(a, 1, true, "u", tuneNow)
	if err != nil || !slices.Equal(r.RemovedSets, []string{empty}) || !slices.Equal(r.LeftSets, []string{keep}) {
		t.Fatalf("remove a: %+v %v", r, err)
	}
	if _, ok := c.Set(empty); ok || !c.taken(empty) || !slices.Equal(c.RemovedTunes, []string{b, a}) {
		t.Fatal("emptied set removed, ids retired")
	}
	// A Tune in no Set needs no confirmation.
	e := addTune(t, c, reel(c, "E"))
	if _, err := c.RemoveTune(e, 1, false, "u", tuneNow); err != nil {
		t.Fatal(err)
	}
}

func TestPublished(t *testing.T) {
	c := NewTuneCollection("org")
	names := []string{"The Silver Spear", "Ástor", "An Phis Fhliuch", "a Zebra", "Na Ceannabháin Bhána", "Banish Misfortune", "The"}
	ids := map[string]string{}
	for _, n := range names {
		ids[n] = addTune(t, c, reel(c, n))
	}
	hidden := addTune(t, c, reel(c, "Hidden"))
	_ = c.SetTuneHidden(hidden, true, "u", tuneNow)
	s1, s2, s3 := newID(t, c), newID(t, c), newID(t, c)
	_ = c.AddSet(s1, SetInput{TuneIDs: []string{ids["Ástor"], hidden, ids["The"]}}, "u", tuneNow)
	_ = c.AddSet(s2, SetInput{TuneIDs: []string{ids["Ástor"], hidden}}, "u", tuneNow) // one visible Tune
	_ = c.AddSet(s3, SetInput{TuneIDs: []string{ids["Ástor"], ids["The"]}}, "u", tuneNow)
	_ = c.SetSetHidden(s3, true, "u", tuneNow)
	p := c.Published()
	var got []string
	for _, tu := range p.Tunes {
		got = append(got, tu.Name.Name)
	}
	want := []string{"Ástor", "Banish Misfortune", "Na Ceannabháin Bhána", "An Phis Fhliuch", "The Silver Spear", "The", "a Zebra"}
	if !slices.Equal(got, want) {
		t.Fatalf("order:\n got %v\nwant %v", got, want)
	}
	if len(p.Sets) != 1 || p.Sets[0].ID != s1 || !slices.Equal(p.SetTunes[s1], []string{ids["Ástor"], ids["The"]}) {
		t.Fatalf("sets: %+v %v", p.Sets, p.SetTunes)
	}
	if SortKey("The Kesh") != "kesh" || SortKey("Żółć") != "zolc" || SortKey("A") != "a" {
		t.Fatal("sort key")
	}
}

func TestImportPreview(t *testing.T) {
	c := NewTuneCollection("org")
	ie := newID(t, c)
	_ = c.AddTuneOrigin(ie, "", Bi{PL: "Irlandia", EN: "Ireland"}, "u", tuneNow)
	clare := newID(t, c)
	_ = c.AddTuneOrigin(clare, ie, Bi{EN: "Clare"}, "u", tuneNow)
	addTune(t, c, reel(c, "The Kesh"))
	file := "%abc\nX:1\nT:The Kesh\nT:Kesh Jig\nR:Jig\nO:ireland\nC:trad.\nM:6/8\nK:G\nGAG GAB|\n\n" +
		"X:2\nT:The Késh\nR:hop jig\nO:Clare\nK:Ador\nABcd|\n\n" +
		"X:3\nR:reel\nO:Mars\nM:none\nK:none\nabc|\n"
	cs, err := c.ImportPreview(file)
	if err != nil || len(cs) != 3 {
		t.Fatalf("%v %+v", err, cs)
	}
	a := cs[0]
	if a.Name.Name != "The Kesh" || len(a.OtherNames) != 1 || a.TypeID != typeID(c, "jig") || a.OriginID != ie || a.Composer != "trad." ||
		a.Key == nil || *a.Key != (Key{"G", "major"}) || a.Meter != "6/8" || !slices.Equal(a.Duplicate, []string{"The Kesh"}) || len(a.Problems) != 0 {
		t.Fatalf("first: %+v", a)
	}
	b := cs[1]
	if b.TypeID != "" || b.Rhythm != "hop jig" || b.OriginID != clare || b.Meter != "" || !slices.Equal(b.Duplicate, []string{"The Kesh"}) {
		t.Fatalf("second: %+v", b)
	}
	if p := fields(&ValidationError{Problems: b.Problems}); !slices.Equal(p, []string{"meter", "typeId"}) {
		t.Fatalf("second problems: %v", p)
	}
	d := cs[2]
	if d.OriginID != "" || d.Origin != "Mars" || d.Meter != "4/4" || d.Key != nil || len(d.Duplicate) != 0 {
		t.Fatalf("third: %+v", d)
	}
	if p := fields(&ValidationError{Problems: d.Problems}); !slices.Equal(p, []string{"key", "name.name"}) {
		t.Fatalf("third problems: %v", p)
	}
	if _, err := c.ImportPreview(strings.Repeat("x", MaxImportBytes+1)); !slices.Equal(fields(err), []string{"abc"}) {
		t.Fatalf("too large: %v", err)
	}
	if _, err := c.ImportPreview("  "); !slices.Equal(fields(err), []string{"abc"}) {
		t.Fatalf("empty: %v", err)
	}
}

func TestBuildTuneSnapshots(t *testing.T) {
	org := tuneOrg()
	c := NewTuneCollection("org")
	in := reel(c, "The Silver Spear")
	in.ABC = "X:1\nT:The Silver Spear\nM:4/4\nL:1/8\nK:D\nA|FA (3AAA BAFA|dfed BddB|AF (3FFF AFdF|\n"
	in.Description = Bi{PL: "**Ładna**"}
	in.Recordings = []RecordingInput{{Link: "https://soundcloud.com/x"}, {Link: "https://youtu.be/dQw4w9WgXcQ"}}
	spear := addTune(t, c, in)
	maggie := addTune(t, c, reel(c, "Drowsy Maggie"))
	hidden := addTune(t, c, reel(c, "Hidden"))
	_ = c.SetTuneHidden(hidden, true, "u", tuneNow)
	set := newID(t, c)
	_ = c.AddSet(set, SetInput{TuneIDs: []string{spear, maggie}}, "u", tuneNow)
	_ = c.AddAttachment(spear, Attachment{ID: newID(t, c), Kind: AttachmentPDF, Name: "a.pdf", Size: 10, Path: "media/org/tunes/" + spear + "/h.pdf"}, "u", tuneNow)
	url := func(p string) string { return "https://pub.example/" + p }

	snap, files, err := BuildTuneSnapshots(org, c, url)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Tunes) != 2 || snap.Tunes[0].ID != maggie || len(files) != 2 || len(snap.Sets) != 1 || len(snap.Types) != 12 {
		t.Fatalf("catalogue: %+v", snap)
	}
	sp := snap.Tunes[1]
	if !sp.HasTranscription || sp.Incipit == "" || sp.FirstRecording == nil || sp.FirstRecording.Kind != "link" || sp.Tempo != 180 || sp.TempoBeat != "1/4" ||
		sp.Permalink[EN] != "https://site.example/en/tunes/"+spear+"-the-silver-spear" || sp.Rev == "" {
		t.Fatalf("spear: %+v", sp)
	}
	var f TuneFileSnapshot
	if err := json.Unmarshal(files[1].Data, &f); err != nil {
		t.Fatal(err)
	}
	if f.ID != spear || f.DescriptionHTML == nil || f.DescriptionHTML.PL != "<p><strong>Ładna</strong></p>" || len(f.Recordings) != 2 ||
		f.Recordings[1].EmbedURL == "" || len(f.Attachments) != 1 || f.Attachments[0].URL != "https://pub.example/media/org/tunes/"+spear+"/h.pdf" ||
		len(f.Sets) != 1 || f.Transcription == nil {
		t.Fatalf("file: %+v", f)
	}

	// The rev is stable, and changes with the Tune's content only.
	_, again, _ := BuildTuneSnapshots(org, c, url)
	if again[0].Rev != files[0].Rev || again[1].Rev != files[1].Rev {
		t.Fatal("rev not stable")
	}
	tu, _ := c.PrepareTune(reel(c, "Drowsy Maggie"))
	tu.Composer = "trad."
	if err := c.UpdateTune(maggie, 1, tu, "u", tuneNow); err != nil {
		t.Fatal(err)
	}
	_, changed, _ := BuildTuneSnapshots(org, c, url)
	if changed[0].Rev == files[0].Rev || changed[1].Rev != files[1].Rev {
		t.Fatalf("rev after change: %s→%s, %s→%s", files[0].Rev, changed[0].Rev, files[1].Rev, changed[1].Rev)
	}
}

func TestSafeFileName(t *testing.T) {
	for in, want := range map[string]string{
		"Nuty – Żółw.PDF":      "nuty-zolw.pdf",
		`C:\scans\kesh.pdf`:    "kesh.pdf",
		`"><script>.pdf`:       "script.pdf",
		"":                     "attachment.pdf",
		"../../etc/passwd":     "passwd.pdf",
		"tune sheet final.pdf": "tune-sheet-final.pdf",
	} {
		if got := SafeFileName(in, "pdf"); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}
