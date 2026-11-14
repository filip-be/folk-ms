package domain

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/filip-be/folk-ms/cms/internal/schema"
)

func TestParseVideoLink(t *testing.T) {
	yt := func(id string, start int) VideoLink { return VideoLink{Provider: YouTube, ID: id, StartSeconds: start} }
	vm := func(id, hash string, start int) VideoLink {
		return VideoLink{Provider: Vimeo, ID: id, Hash: hash, StartSeconds: start}
	}
	good := []struct {
		in   string
		want VideoLink
	}{
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", yt("dQw4w9WgXcQ", 0)},
		{"http://youtube.com/watch?v=dQw4w9WgXcQ&list=PL1&index=2", yt("dQw4w9WgXcQ", 0)},
		{"m.youtube.com/watch?v=dQw4w9WgXcQ&t=90", yt("dQw4w9WgXcQ", 90)},
		{"https://music.youtube.com/watch?v=dQw4w9WgXcQ&t=1m30s", yt("dQw4w9WgXcQ", 90)},
		{"https://youtu.be/dQw4w9WgXcQ?t=75", yt("dQw4w9WgXcQ", 75)},
		{"  https://youtu.be/dQw4w9WgXcQ?si=abc  ", yt("dQw4w9WgXcQ", 0)},
		{"https://www.youtube.com/shorts/dQw4w9WgXcQ", yt("dQw4w9WgXcQ", 0)},
		{"https://www.youtube.com/embed/dQw4w9WgXcQ?start=42", yt("dQw4w9WgXcQ", 42)},
		{"https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ", yt("dQw4w9WgXcQ", 0)},
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ#t=1:02:03", yt("dQw4w9WgXcQ", 3723)},
		{"https://www.youtube.com/watch?v=a-b_c-d_e-f&t=1h2m3s", yt("a-b_c-d_e-f", 3723)},
		{"https://vimeo.com/123456789", vm("123456789", "", 0)},
		{"https://vimeo.com/123456789/abcdef1234", vm("123456789", "abcdef1234", 0)},
		{"https://vimeo.com/123456789#t=1m5s", vm("123456789", "", 65)},
		{"https://player.vimeo.com/video/123456789?h=abcdef1234&badge=0", vm("123456789", "abcdef1234", 0)},
		{"https://player.vimeo.com/video/123456789#t=30s", vm("123456789", "", 30)},
	}
	for _, c := range good {
		got, err := ParseVideoLink(c.in)
		if err != nil || got != c.want {
			t.Errorf("%q: got %+v, %v; want %+v", c.in, got, err, c.want)
		}
	}
	bad := []string{
		"", "dQw4w9WgXcQ", "https://evil.example/watch?v=dQw4w9WgXcQ", "javascript:alert(1)//youtube.com/watch?v=dQw4w9WgXcQ",
		"ftp://youtube.com/watch?v=dQw4w9WgXcQ", "https://www.youtube.com/watch?v=short", "https://www.youtube.com/watch?v=dQw4w9WgXcQ0",
		"https://www.youtube.com/watch?v=dQw4w9Wg<cQ", "https://youtube.com.evil.example/watch?v=dQw4w9WgXcQ",
		"https://user@youtube.com/watch?v=dQw4w9WgXcQ", "https://youtube.com:8443/watch?v=dQw4w9WgXcQ",
		"https://www.youtube.com/playlist?list=PL1", "https://www.youtube.com/@channel",
		"https://vimeo.com/abc", "https://vimeo.com/123/NOT-A-HASH", "https://vimeo.com/channels/staffpicks/123",
		"https://player.vimeo.com/video/123?h=zz", "https://vimeo.com/123/abcdef12?h=fedcba21",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=abc", "https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=99h",
	}
	for _, in := range bad {
		if l, err := ParseVideoLink(in); err == nil {
			t.Errorf("%q accepted: %+v", in, l)
		} else if !IsValidation(err) {
			t.Errorf("%q: not a validation error: %v", in, err)
		}
	}
}

func TestVideoURLs(t *testing.T) {
	y := VideoLink{Provider: YouTube, ID: "dQw4w9WgXcQ", StartSeconds: 90}
	if y.WatchURL() != "https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=90s" || y.LinkURL() != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" ||
		y.EmbedURL() != "https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ?start=90" {
		t.Fatalf("youtube: %s %s %s", y.WatchURL(), y.LinkURL(), y.EmbedURL())
	}
	v := VideoLink{Provider: Vimeo, ID: "123", Hash: "abcdef12", StartSeconds: 5}
	if v.WatchURL() != "https://vimeo.com/123/abcdef12#t=5s" || v.EmbedURL() != "https://player.vimeo.com/video/123?h=abcdef12#t=5s" {
		t.Fatalf("vimeo: %s %s", v.WatchURL(), v.EmbedURL())
	}
	// The canonical URL parses back to the same link.
	for _, l := range []VideoLink{y, v, {Provider: Vimeo, ID: "9"}} {
		if back, err := ParseVideoLink(l.WatchURL()); err != nil || back != l {
			t.Fatalf("round trip %+v: %+v %v", l, back, err)
		}
	}
	for secs, want := range map[int]string{0: "", 5: "0:05", 90: "1:30", 3723: "1:02:03"} {
		if got := FormatStart(secs); got != want {
			t.Errorf("FormatStart(%d) = %q", secs, got)
		}
		if back, ok := ParseStart(want); !ok || back != secs {
			t.Errorf("ParseStart(%q) = %d %v", want, back, ok)
		}
	}
	for _, bad := range []string{"1:60", "abc", "-5", "25:00:00", "1.5"} {
		if _, ok := ParseStart(bad); ok {
			t.Errorf("ParseStart(%q) accepted", bad)
		}
	}
}

// ids hands out shortId-shaped ids in order.
type ids struct{ n int }

func (g *ids) next() string {
	g.n++
	return strings.Repeat(string(rune('a'+g.n%26)), 4) + strings.Repeat(string(rune('a'+g.n/26)), 4)
}

func testCollection(t *testing.T) (*VideoCollection, map[string]string) {
	t.Helper()
	c := NewVideoCollection("balfolk-poznan")
	now := at("2026-11-01 12:00")
	g := &ids{}
	m := map[string]string{}
	add := func(key string, fn func(id string) error) {
		t.Helper()
		id := g.next()
		if err := fn(id); err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		m[key] = id
	}
	add("pair", func(id string) error { return c.AddForm(id, Text{PL: "Para", EN: "Pair"}, "u", now) })
	add("chain", func(id string) error { return c.AddForm(id, Text{PL: "Łańcuch"}, "u", now) })
	add("france", func(id string) error { return c.AddOrigin(id, "", Text{PL: "Francja", EN: "France"}, "u", now) })
	add("brittany", func(id string) error { return c.AddOrigin(id, m["france"], Text{PL: "Bretania"}, "u", now) })
	add("sweden", func(id string) error { return c.AddOrigin(id, "", Text{PL: "Szwecja"}, "u", now) })
	video := func(title, link, origin string, forms ...string) func(string) error {
		return func(id string) error {
			v, err := c.PrepareVideo(VideoInput{Link: link, Title: Text{PL: title}, OriginID: origin, FormIDs: forms})
			if err != nil {
				return err
			}
			return c.AddVideo(v, id, "u", now)
		}
	}
	add("andro", video("An dro", "https://youtu.be/dQw4w9WgXcQ", m["brittany"], m["chain"]))
	add("polska", video("Polska", "https://www.youtube.com/watch?v=aBcDeFgHiJk", m["sweden"], m["pair"]))
	add("mazurka", video("Mazurek", "https://www.youtube.com/watch?v=Zz_9-8y7x6W&t=30", "", m["pair"], m["chain"]))
	return c, m
}

func TestVideoValidation(t *testing.T) {
	c, m := testCollection(t)
	_, err := c.PrepareVideo(VideoInput{Link: "https://evil.example/x", Title: Text{EN: "x", PL: ""}, Description: Text{EN: "y"},
		Credit: strings.Repeat("c", MaxCreditLen+1), OriginID: "nosuchid", FormIDs: []string{m["pair"], "nosuchid"}})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want validation: %v", err)
	}
	fields := map[string]bool{}
	for _, p := range ve.Problems {
		fields[p.Field] = true
	}
	for _, f := range []string{"link", "title.pl", "description.pl", "credit", "originId", "formIds"} {
		if !fields[f] {
			t.Errorf("no problem on %s: %v", f, ve.Problems)
		}
	}
	if _, err := c.PrepareVideo(VideoInput{Link: "https://youtu.be/dQw4w9WgXcQ", Start: "x", Title: Text{PL: "a"}}); !hasField(err, "start") {
		t.Fatalf("start: %v", err)
	}
	// Text is cleaned; Forms are kept unique in list order; start overrides the link's.
	v, err := c.PrepareVideo(VideoInput{Link: "https://youtu.be/dQw4w9WgXcQ?t=10", Start: "1:05", Title: Text{PL: " An‮dro\x00 "},
		Credit: "Anna\nPiotr", FormIDs: []string{m["chain"], m["pair"], m["chain"]}})
	if err != nil {
		t.Fatal(err)
	}
	if v.Title.PL != "Andro" || v.Credit != "Anna Piotr" || v.Link.StartSeconds != 65 || strings.Join(v.FormIDs, ",") != m["pair"]+","+m["chain"] {
		t.Fatalf("%+v", v)
	}
	if err := c.AddForm("zzzzzzzz", Text{PL: "para"}, "u", at("2026-11-01 12:00")); !hasField(err, "label.pl") {
		t.Fatalf("duplicate form: %v", err)
	}
	if err := c.AddForm("zzzzzzzz", Text{PL: strings.Repeat("x", MaxLabelLen+1)}, "u", at("2026-11-01 12:00")); !hasField(err, "label.pl") {
		t.Fatalf("long label: %v", err)
	}
	if err := c.AddOrigin("zzzzzzzz", m["brittany"], Text{PL: "Region regionu"}, "u", at("2026-11-01 12:00")); !hasField(err, "parentId") {
		t.Fatalf("region of a region: %v", err)
	}
}

func hasField(err error, field string) bool {
	var ve *ValidationError
	if !errors.As(err, &ve) {
		return false
	}
	for _, p := range ve.Problems {
		if p.Field == field {
			return true
		}
	}
	return false
}

func TestVideoCollectionOperations(t *testing.T) {
	c, m := testCollection(t)
	now := at("2026-11-02 12:00")
	order := func() string {
		var s []string
		for _, v := range c.Videos {
			s = append(s, v.Title.PL)
		}
		return strings.Join(s, ",")
	}
	if err := c.MoveVideo(m["mazurka"], 0, "u", now); err != nil || order() != "Mazurek,An dro,Polska" {
		t.Fatalf("move to top: %v %s", err, order())
	}
	if err := c.MoveVideo(m["mazurka"], 99, "u", now); err != nil || order() != "An dro,Polska,Mazurek" {
		t.Fatalf("move past the end clamps: %v %s", err, order())
	}
	// A Form or Origin in use cannot be removed; the error says how many use it.
	err := c.RemoveForm(m["pair"], "u", now)
	if !errors.Is(err, ErrInvalidState) || !strings.Contains(err.Error(), "2 video") || !hasField(err, "label.pl") {
		t.Fatalf("form in use: %v", err)
	}
	if err := c.RemoveOrigin(m["france"], "u", now); !errors.Is(err, ErrInvalidState) || !strings.Contains(err.Error(), "regions") {
		t.Fatalf("country with regions: %v", err)
	}
	if err := c.RemoveOrigin(m["brittany"], "u", now); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("region in use: %v", err)
	}
	// Moving Forms keeps every Video's Forms in list order.
	if err := c.MoveForm(m["chain"], 0, "u", now); err != nil {
		t.Fatal(err)
	}
	if v, _ := c.Video(m["mazurka"]); v.FormIDs[0] != m["chain"] {
		t.Fatalf("forms reordered: %v", v.FormIDs)
	}
	// Free a region and a country, then remove them; their ids are never reused.
	if _, err := c.DeleteVideo(m["andro"], "u", now); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveOrigin(m["brittany"], "u", now); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveOrigin(m["france"], "u", now); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"andro", "brittany", "france"} {
		if !c.taken(m[k]) {
			t.Fatalf("%s id can be reused", k)
		}
	}
	if err := c.AddForm(m["andro"], Text{PL: "Nowa"}, "u", now); !errors.Is(err, ErrConflict) {
		t.Fatalf("reused id: %v", err)
	}
	if err := c.SetVideoHidden(m["polska"], true, "u", now); err != nil {
		t.Fatal(err)
	}
	// Update keeps id, Hidden and place.
	v, _ := c.PrepareVideo(VideoInput{Link: "https://vimeo.com/123", Title: Text{PL: "Polska 2"}})
	v.Thumbnail = "https://i.vimeocdn.com/video/1"
	if err := c.UpdateVideo(m["polska"], v, "u2", now); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.Video(m["polska"]); !got.Hidden || got.Title.PL != "Polska 2" || got.UpdatedBy != "u2" || got.CreatedBy != "u" || c.Videos[0].ID != m["polska"] {
		t.Fatalf("update: %+v", got)
	}
	for _, err := range []error{c.MoveVideo("nosuchid", 0, "u", now), c.SetVideoHidden("nosuchid", true, "u", now), c.RenameForm("nosuchid", Text{PL: "x"}, "u", now)} {
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("missing: %v", err)
		}
	}
	// Regions move only within their country.
	g := &ids{n: 100}
	pl, mz, ma := g.next(), g.next(), g.next()
	_ = c.AddOrigin(pl, "", Text{PL: "Polska"}, "u", now)
	_ = c.AddOrigin(mz, pl, Text{PL: "Mazowsze"}, "u", now)
	_ = c.AddOrigin(ma, pl, Text{PL: "Małopolska"}, "u", now)
	if err := c.MoveOrigin(ma, 0, "u", now); err != nil || c.Origins[1].Regions[0].ID != ma || len(c.Origins) != 2 {
		t.Fatalf("region move: %v %+v", err, c.Origins)
	}
}

func TestVideoLimits(t *testing.T) {
	c := NewVideoCollection("o")
	now := at("2026-11-01 12:00")
	g := &ids{}
	for i := range MaxForms {
		if err := c.AddForm(g.next(), Text{PL: "F" + string(rune('A'+i%26)) + string(rune('A'+i/26))}, "u", now); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.AddForm(g.next(), Text{PL: "one more"}, "u", now); !IsValidation(err) {
		t.Fatalf("form limit: %v", err)
	}
	c.Videos = make([]Video, MaxVideos)
	if err := c.AddVideo(Video{}, "zzzzzzzz", "u", now); !IsValidation(err) {
		t.Fatalf("video limit: %v", err)
	}
}

func TestVideoSnapshot(t *testing.T) {
	c, m := testCollection(t)
	now := at("2026-11-02 12:00")
	_ = c.SetVideoHidden(m["polska"], true, "u", now)
	g := &ids{n: 200}
	vid := g.next()
	v, _ := c.PrepareVideo(VideoInput{Link: "https://vimeo.com/123456/abcdef12#t=5", Title: Text{PL: "Szot", EN: "Schottische"},
		Description: Text{PL: "Opis"}, Credit: "Anna", OriginID: m["france"], FormIDs: []string{m["pair"]}})
	v.Thumbnail = "https://i.vimeocdn.com/video/1-d_640"
	if err := c.AddVideo(v, vid, "u", now); err != nil {
		t.Fatal(err)
	}
	s := BuildVideoSnapshot(testOrg(), c)
	if len(s.Videos) != 3 || s.Videos[0].ID != m["andro"] || s.Videos[1].ID != m["mazurka"] || s.Videos[2].ID != vid {
		t.Fatalf("hidden left out, order kept: %+v", s.Videos)
	}
	if len(s.Forms) != 2 || len(s.Origins) != 2 || len(s.Origins[0].Regions) != 1 || s.Origins[1].Regions == nil {
		t.Fatalf("lists: %+v %+v", s.Forms, s.Origins)
	}
	y, vm := s.Videos[1], s.Videos[2]
	if y.Thumbnail != "https://i.ytimg.com/vi/Zz_9-8y7x6W/hqdefault.jpg" || y.Start != 30 || y.EmbedURL != "https://www.youtube-nocookie.com/embed/Zz_9-8y7x6W?start=30" || y.Description != nil {
		t.Fatalf("youtube: %+v", y)
	}
	if vm.Hash != "abcdef12" || vm.URL != "https://vimeo.com/123456/abcdef12#t=5s" || vm.EmbedURL != "https://player.vimeo.com/video/123456?h=abcdef12#t=5s" || vm.Credit != "Anna" {
		t.Fatalf("vimeo: %+v", vm)
	}
	raw, err := os.ReadFile("../../../contract/videos.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	sch, err := schema.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, snap := range []VideoSnapshot{s, BuildVideoSnapshot(testOrg(), nil), BuildVideoSnapshot(testOrg(), NewVideoCollection("x"))} {
		data, _ := json.Marshal(snap)
		if err := sch.Validate(data); err != nil {
			t.Fatalf("video snapshot does not match contract: %v\n%s", err, data)
		}
	}
}
