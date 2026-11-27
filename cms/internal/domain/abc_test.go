package domain

import (
	"slices"
	"strings"
	"testing"
)

func TestParseABCKey(t *testing.T) {
	cases := map[string]Key{
		"G":                   {"G", "major"},
		"Gmaj":                {"G", "major"},
		"Em":                  {"E", "minor"},
		"E min":               {"E", "minor"},
		"Eaeolian":            {"E", "minor"},
		"Ador":                {"A", "dorian"},
		"A Dorian":            {"A", "dorian"},
		"Dmix":                {"D", "mixolydian"},
		"D Mixolydian % trad": {"D", "mixolydian"},
		"F#m":                 {"F#", "minor"},
		"Bb":                  {"Bb", "major"},
		"Cion":                {"C", "major"},
		"Flyd":                {"F", "lydian"},
		"Ephr":                {"E", "phrygian"},
		"Bloc":                {"B", "locrian"},
		"G clef=bass":         {"G", "major"},
		"D exp ^f ^c":         {"D", "major"},
		"D ^g":                {"D", "major"},
	}
	for in, want := range cases {
		got, ok := ParseABCKey(in)
		if !ok || got != want {
			t.Errorf("%q: got %+v %v, want %+v", in, got, ok, want)
		}
		if !got.Valid() {
			t.Errorf("%q: %+v not valid", in, got)
		}
	}
	for _, in := range []string{"", "none", "HP", "Hp", "x", "%G"} {
		if k, ok := ParseABCKey(in); ok {
			t.Errorf("%q: want no key, got %+v", in, k)
		}
	}
	if (Key{"G", "major"}).String() != "G" || (Key{"E", "minor"}).String() != "Em" || (Key{"A", "dorian"}).String() != "Ador" {
		t.Error("Key.String")
	}
	if (Key{"H", "major"}).Valid() || (Key{"G", "blues"}).Valid() {
		t.Error("invalid keys pass")
	}
}

func TestParseABCMeter(t *testing.T) {
	for in, want := range map[string]string{"6/8": "6/8", " 4/4 ": "4/4", "C": "4/4", "C|": "2/2", "12/8": "12/8", "9/8 % slip": "9/8", "3 / 4": "3/4"} {
		if got, ok := ParseABCMeter(in); !ok || got != want {
			t.Errorf("%q: %q %v", in, got, ok)
		}
	}
	for _, in := range []string{"none", "", "2+3/8", "6/7", "0/4", "33/4", "4"} {
		if got, ok := ParseABCMeter(in); ok {
			t.Errorf("%q: want none, got %q", in, got)
		}
	}
	for m, beat := range map[string]string{"4/4": "1/4", "6/8": "3/8", "9/8": "3/8", "12/8": "3/8", "2/2": "1/2", "2/4": "1/4", "3/4": "1/4", "7/8": "1/8", "3/8": "3/8"} {
		if TempoBeat(m) != beat {
			t.Errorf("tempo beat of %s: %s", m, TempoBeat(m))
		}
	}
}

const kesh = `X:1
T:The Kesh
T:Kesh Jig
T:An Cheis
R:jig
O:Ireland
C:trad.
M:6/8
L:1/8
K:G
|:GAG GAB|ABA ABd|edd gdd|edB dBA|
GAG GAB|ABA ABd|edd gdB|AGF G3:|
|:BAB dBd|ege dBA|BAB dBG|ABA AGA|
BAB dBd|ege dBd|gfg aga|bgf g3:|`

func TestProposeFromABC(t *testing.T) {
	h := ParseABCHeader(kesh)
	if !slices.Equal(h.Titles, []string{"The Kesh", "Kesh Jig", "An Cheis"}) || h.Rhythm != "jig" || h.Origin != "Ireland" || h.Composer != "trad." || h.Key != "G" || h.Meter != "6/8" || h.Unit != "1/8" {
		t.Fatalf("header %+v", h)
	}
	p := ProposeFromABC(kesh)
	if p.Key == nil || *p.Key != (Key{"G", "major"}) || p.Meter != "6/8" {
		t.Fatalf("proposal %+v", p)
	}
	// A body K: after the header is a key change, not the Key.
	p = ProposeFromABC("X:1\nT:x\nM:C|\nK:Ador\nABcd|\nK:G\nGABc|")
	if p.Key == nil || *p.Key != (Key{"A", "dorian"}) || p.Meter != "2/2" {
		t.Fatalf("proposal %+v", p)
	}
	if p := ProposeFromABC("X:1\nT:x\nM:none\nK:none\nabc|"); p.Key != nil || p.Meter != "" {
		t.Fatalf("no proposal expected: %+v", p)
	}
}

func TestSplitABCTunes(t *testing.T) {
	file := "%abc-2.1\n%%title Session tunes\n\nX:1\nT:One\nK:G\nGABc|\n\nX:2\nT:Two\nK:D\nDEFG|\n  X:3\nT:Three\nK:A\n"
	got := SplitABCTunes(file)
	if len(got) != 3 || !strings.HasPrefix(got[0], "X:1\nT:One") || !strings.HasSuffix(got[1], "DEFG|") || !strings.HasPrefix(got[2], "X:3") {
		t.Fatalf("split %q", got)
	}
	if got := SplitABCTunes("T:No X\nK:G\nGABc|"); len(got) != 1 {
		t.Fatalf("no X: %q", got)
	}
	if got := SplitABCTunes(" \n "); len(got) != 0 {
		t.Fatalf("empty: %q", got)
	}
}

// Incipits of real session tunes (traditional; these transcriptions are
// the CMS's own): pickups, tuplets, broken rhythm, repeats, endings,
// inline fields, chord symbols, decorations and grace notes.
func TestIncipit(t *testing.T) {
	cases := []struct {
		name, abc string
		bars      int
		want      string
	}{
		{"jig without pickup, leading repeat", kesh, 0,
			"X:1\nM:6/8\nL:1/8\nK:G\n|:GAG GAB|ABA ABd|]\n"},
		{"one bar", kesh, 1,
			"X:1\nM:6/8\nL:1/8\nK:G\n|:GAG GAB|]\n"},
		{"bars across lines", kesh, 5,
			"X:1\nM:6/8\nL:1/8\nK:G\n|:GAG GAB|ABA ABd|edd gdd|edB dBA|\nGAG GAB|]\n"},
		{"reel with pickup and triplet", "X:1\nT:The Silver Spear\nR:reel\nM:4/4\nL:1/8\nK:D\nA|FA (3AAA BAFA|dfed BddB|AF (3FFF AFdF|EFGE FDEG|]\n", 2,
			"X:1\nM:4/4\nL:1/8\nK:D\nA|FA (3AAA BAFA|dfed BddB|]\n"},
		{"hornpipe: triplet pickup and dotted rhythm", "X:1\nT:Hornpipe\nM:4/4\nL:1/8\nK:D\n(3ABc|d>cd>e f>ed>c|B>AB>c d2 (3ABc|d>cd>e f2 a2|\n", 2,
			"X:1\nM:4/4\nL:1/8\nK:D\n(3ABc|d>cd>e f>ed>c|B>AB>c d2 (3ABc|]\n"},
		{"first and second endings", "X:1\nT:Endings\nM:6/8\nL:1/8\nK:D\n|:A|d2d f2d|e2e g2e|1 f2f e2d:|2 f2e d2||\n", 3,
			"X:1\nM:6/8\nL:1/8\nK:D\n|:A|d2d f2d|e2e g2e|1 f2f e2d|]\n"},
		{"second ending counted as written", "X:1\nT:Endings\nM:6/8\nL:1/8\nK:D\n|:A|d2d f2d|e2e g2e|1 f2f e2d:|2 f2e d2||\n", 4,
			"X:1\nM:6/8\nL:1/8\nK:D\n|:A|d2d f2d|e2e g2e|1 f2f e2d:|2 f2e d2|]\n"},
		{"[1 endings and :: double repeat", "X:1\nT:Endings\nM:3/4\nL:1/8\nK:G\nG2 B2 d2|g4 d2::[1 e2 d2 B2:|[2 c2 B2 A2|]\n", 3,
			"X:1\nM:3/4\nL:1/8\nK:G\nG2 B2 d2|g4 d2::[1 e2 d2 B2|]\n"},
		{"chords, grace notes, decorations, inline meter, comments", "X:1\nT:Mix\nM:C|\nL:1/8\nK:Ador % comment\n% a comment line\n\"Am\"~A3B {g}AGEG|!trill!A2 [ce]2 ed cB % here\nw: la la\n[M:3/4] e2 d2 c2|A6|\n", 3,
			"X:1\nM:C|\nL:1/8\nK:Ador\n\"Am\"~A3B {g}AGEG|!trill!A2 [ce]2 ed cB\n[M:3/4] e2 d2 c2|]\n"},
		{"default unit length 1/16 in 2/4", "X:1\nT:Polka\nM:2/4\nK:D\nAB|d2d2 e2f2|g2f2 e2d2|\n", 1,
			"X:1\nM:2/4\nK:D\nAB|d2d2 e2f2|]\n"},
		{"slip jig with a chord at the bar line", "X:1\nT:Slip\nM:9/8\nL:1/8\nK:G\n[GB]2A B2d g2e|dBG A2G FGA|\n", 1,
			"X:1\nM:9/8\nL:1/8\nK:G\n[GB]2A B2d g2e|]\n"},
		{"shorter than asked", "X:1\nT:Short\nM:4/4\nL:1/8\nK:G\nGABc dBAG|\n", 4,
			"X:1\nM:4/4\nL:1/8\nK:G\nGABc dBAG|]\n"},
		{"body key change kept", "X:1\nT:Change\nM:4/4\nL:1/4\nK:G\nGABc|\nK:D\nDEFG|ABcd|\n", 2,
			"X:1\nM:4/4\nL:1/4\nK:G\nGABc|\nK:D\nDEFG|]\n"},
		{"more than eight is eight", "X:1\nM:2/4\nL:1/8\nK:G\nGA|B2 B2|c2 c2|d2 d2|e2 e2|f2 f2|g2 g2|a2 a2|b2 b2|c'2 c'2|\n", 20,
			"X:1\nM:2/4\nL:1/8\nK:G\nGA|B2 B2|c2 c2|d2 d2|e2 e2|f2 f2|g2 g2|a2 a2|b2 b2|]\n"},
	}
	for _, c := range cases {
		got, ok := Incipit(c.abc, c.bars)
		if !ok || got != c.want {
			t.Errorf("%s:\n got %q %v\nwant %q", c.name, got, ok, c.want)
		}
	}
	for _, bad := range []string{"", "X:1\nT:No key\nGABc|", "X:1\nK:G\n", "X:1\nK:G\n% only a comment"} {
		if got, ok := Incipit(bad, 2); ok {
			t.Errorf("%q: want no incipit, got %q", bad, got)
		}
	}
}
