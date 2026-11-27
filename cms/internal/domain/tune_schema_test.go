package domain

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/filip-be/folk-ms/cms/internal/schema"
)

func compileContract(t *testing.T, name string) *schema.Schema {
	t.Helper()
	raw, err := os.ReadFile("../../../contract/" + name)
	if err != nil {
		t.Fatal(err)
	}
	sch, err := schema.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	return sch
}

// richTunes is a collection using every field the snapshots carry.
func richTunes(t *testing.T) *TuneCollection {
	t.Helper()
	c := NewTuneCollection("org")
	ie, clare := newID(t, c), newID(t, c)
	_ = c.AddTuneOrigin(ie, "", Bi{PL: "Irlandia", EN: "Ireland"}, "u", tuneNow)
	_ = c.AddTuneOrigin(clare, ie, Bi{EN: "Clare"}, "u", tuneNow)
	in := TuneInput{Name: TuneName{Name: "The Kesh", Translation: Bi{PL: "Kesz", EN: "The Kesh"}},
		OtherNames: []TuneName{{Name: "An Cheis", Translation: Bi{EN: "The Kesh"}}, {Name: "Kesh Jig"}},
		TypeID:     typeID(c, "jig"), Key: Key{"G", "major"}, Meter: "6/8", Tempo: 120, OriginID: clare, Composer: "trad.",
		Suits: []string{instrumentID(c, "flute")}, Tricky: []string{instrumentID(c, "fiddle")},
		ABC: kesh, IncipitBars: 3,
		Recordings: []RecordingInput{{Link: "https://youtu.be/dQw4w9WgXcQ?t=75", Label: "Ensemble"}, {Link: "https://vimeo.com/123456/abcdef12"},
			{Link: "https://soundcloud.com/x/kesh", Label: "Live"}},
		Description: Bi{PL: "**Ładna** [link](https://ex.org)", EN: "- one\n- two"}}
	kesh := addTune(t, c, in)
	tu, _ := c.Tune(kesh)
	tu.Recordings[1].Thumbnail = "https://i.vimeocdn.com/video/123456-d_640"
	pre := AttachmentPrefix("org", kesh)
	_ = c.AddAttachment(kesh, Attachment{ID: newID(t, c), Kind: AttachmentImage, Name: "scan.jpg", Size: 1234, Caption: Bi{EN: "Old scan"},
		Width: 960, Height: 480, Variants: []ImageVariant{{480, 240, pre + "h-480.jpg"}, {960, 480, pre + "h-960.jpg"}}}, "u", tuneNow)
	_ = c.AddAttachment(kesh, Attachment{ID: newID(t, c), Kind: AttachmentPDF, Name: "sheet.pdf", Size: 999, Path: pre + "p.pdf"}, "u", tuneNow)
	plain := addTune(t, c, TuneInput{Name: TuneName{Name: "Plain"}, TypeID: typeID(c, "reel"), Key: Key{"E", "dorian"}, Meter: "4/4"})
	hidden := addTune(t, c, TuneInput{Name: TuneName{Name: "Hidden"}, TypeID: typeID(c, "reel"), Key: Key{"D", "major"}, Meter: "4/4"})
	_ = c.SetTuneHidden(hidden, true, "u", tuneNow)
	_ = c.AddSet(newID(t, c), SetInput{Name: Bi{EN: "Jig set"}, TuneIDs: []string{kesh, plain, hidden}}, "u", tuneNow)
	_ = c.AddSet(newID(t, c), SetInput{TuneIDs: []string{plain, kesh}}, "u", tuneNow)
	_ = c.AddSet(newID(t, c), SetInput{TuneIDs: []string{plain, hidden}}, "u", tuneNow) // one visible Tune: not published
	return c
}

func TestTuneSnapshotsMatchContract(t *testing.T) {
	cat, one := compileContract(t, "tunes.schema.json"), compileContract(t, "tune.schema.json")
	url := func(p string) string { return "https://pub.example/" + p }
	rich := richTunes(t)
	for name, c := range map[string]*TuneCollection{"rich": rich, "seeded": NewTuneCollection("org"), "empty": {OrgID: "org"}} {
		snap, files, err := BuildTuneSnapshots(tuneOrg(), c, url)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(snap)
		if err := cat.Validate(data); err != nil {
			t.Fatalf("%s catalogue does not match contract: %v\n%s", name, err, data)
		}
		for _, f := range files {
			if err := one.Validate(f.Data); err != nil {
				t.Fatalf("%s tune file does not match contract: %v\n%s", name, err, f.Data)
			}
		}
		if name == "rich" && (len(snap.Tunes) != 2 || len(snap.Sets) != 2 || len(files) != 2) {
			t.Fatalf("rich: %d tunes, %d sets", len(snap.Tunes), len(snap.Sets))
		}
	}
}
