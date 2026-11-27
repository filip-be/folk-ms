package domain

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// The Tune Collection: per Organization that opted in (config/sites.json),
// the Tunes played at its sessions, its Sets, and its own lists of Tune
// Types, Instruments and Origins. Everything is one document, so in-use
// checks and taking a removed Tune out of its Sets are atomic. Every Tune,
// Set and list entry carries its own revision: a change names the revision
// it was made on and is refused only when that item changed meanwhile.

// Limits of a Tune Collection.
const (
	MaxTunes                = 1000
	MaxTuneSets             = 500
	MaxTuneTypes            = 50
	MaxInstruments          = 100
	MaxTuneCountries        = 100
	MaxTuneRegions          = 50
	MaxOtherNames           = 10
	MaxRecordings           = 10
	MaxAttachments          = 10
	MaxTuneNameLen          = 200
	MaxComposerLen          = 200
	MaxRecordingLabelLen    = 200
	MaxAttachmentCaptionLen = 300
	MaxSetNameLen           = 200
	MaxTuneDescLen          = 10000
	// MaxABCBytes bounds one Transcription.
	MaxABCBytes = 16 << 10
	MinSetTunes = 2
	MaxSetTunes = 10
	MinTempo    = 20
	MaxTempo    = 400
	// MaxPDFBytes bounds a PDF Attachment.
	MaxPDFBytes = 15 << 20
)

// TranscriptionABC is the only Transcription format for now.
const TranscriptionABC = "abc"

// Attachment kinds.
const (
	AttachmentImage = "image"
	AttachmentPDF   = "pdf"
)

// Bi is bilingual text where either language may be written first; a Site
// shows the other where one is missing.
type Bi struct {
	PL string `json:"pl,omitempty"`
	EN string `json:"en,omitempty"`
}

// IsZero reports whether both languages are empty.
func (b Bi) IsZero() bool { return b.PL == "" && b.EN == "" }

// In returns the text in l, else the other language.
func (b Bi) In(l Lang) string {
	if l == EN && b.EN != "" || b.PL == "" {
		return b.EN
	}
	return b.PL
}

// TuneName is a Tune's name or an Other Name as musicians know it, with an
// optional Polish and English translation.
type TuneName struct {
	Name        string `json:"name"`
	Translation Bi     `json:"translation,omitzero"`
}

// TuneType is an entry of the Tune Types list, with the Time Signature and
// Playback Tempo it proposes.
type TuneType struct {
	ID    string `json:"id"`
	Label Bi     `json:"label"`
	Meter string `json:"meter"`
	Tempo int    `json:"tempo"`
	Rev   int    `json:"rev"`
}

// Instrument is an entry of the Instruments list.
type Instrument struct {
	ID    string `json:"id"`
	Label Bi     `json:"label"`
	Rev   int    `json:"rev"`
}

// TuneRegion is a region of a country of the Tune Collection's Origins.
type TuneRegion struct {
	ID    string `json:"id"`
	Label Bi     `json:"label"`
	Rev   int    `json:"rev"`
}

// TuneCountry is a country of the Tune Collection's Origins, with regions.
type TuneCountry struct {
	ID      string       `json:"id"`
	Label   Bi           `json:"label"`
	Regions []TuneRegion `json:"regions,omitempty"`
	Rev     int          `json:"rev"`
}

// Recording is a link to someone playing a Tune: a YouTube or Vimeo video
// (Video, with Thumbnail for Vimeo) or any other https page (URL).
type Recording struct {
	Video     *VideoLink `json:"video,omitempty"`
	Thumbnail string     `json:"thumbnail,omitempty"`
	URL       string     `json:"url,omitempty"`
	Label     string     `json:"label,omitempty"`
}

// Link is the address of the Recording.
func (r Recording) Link() string {
	if r.Video != nil {
		return r.Video.WatchURL()
	}
	return r.URL
}

// ThumbnailURL is the video's thumbnail ("" for a plain link).
func (r Recording) ThumbnailURL() string {
	if r.Video == nil {
		return ""
	}
	if r.Video.Provider == YouTube {
		return YouTubeThumbnail(r.Video.ID)
	}
	return r.Thumbnail
}

// Attachment is an image or PDF of a Tune, stored in the public store
// under the Tune's own prefix.
type Attachment struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Size    int    `json:"size"`
	Caption Bi     `json:"caption,omitzero"`
	// Image: the original size and the JPEG variants.
	Width    int            `json:"width,omitempty"`
	Height   int            `json:"height,omitempty"`
	Variants []ImageVariant `json:"variants,omitempty"`
	// PDF: the file as uploaded.
	Path    string    `json:"path,omitempty"`
	AddedAt time.Time `json:"addedAt"`
	AddedBy string    `json:"addedBy"`
}

// Paths lists the public store files of the Attachment.
func (a Attachment) Paths() []string {
	var out []string
	if a.Path != "" {
		out = append(out, a.Path)
	}
	for _, v := range a.Variants {
		out = append(out, v.Path)
	}
	return out
}

// Transcription is a Tune's notation.
type Transcription struct {
	Format string `json:"format"`
	Text   string `json:"text"`
}

// Tune is one tune of the Tune Collection.
type Tune struct {
	ID            string         `json:"id"`
	Name          TuneName       `json:"name"`
	OtherNames    []TuneName     `json:"otherNames"`
	TypeID        string         `json:"typeId"`
	Key           Key            `json:"key"`
	Meter         string         `json:"meter"`
	Tempo         int            `json:"tempo,omitempty"` // 0: the Tune Type's
	OriginID      string         `json:"originId,omitempty"`
	Composer      string         `json:"composer,omitempty"`
	Suits         []string       `json:"suits"`
	Tricky        []string       `json:"tricky"`
	Transcription *Transcription `json:"transcription,omitempty"`
	IncipitBars   int            `json:"incipitBars,omitempty"` // 0: DefaultIncipitBars
	Recordings    []Recording    `json:"recordings"`
	// Description is Markdown (RenderMarkdown).
	Description Bi           `json:"description,omitzero"`
	Attachments []Attachment `json:"attachments"`
	Hidden      bool         `json:"hidden,omitempty"`
	Rev         int          `json:"rev"`
	CreatedAt   time.Time    `json:"createdAt"`
	CreatedBy   string       `json:"createdBy"`
	UpdatedAt   time.Time    `json:"updatedAt"`
	UpdatedBy   string       `json:"updatedBy"`
}

// ABC is the Transcription's ABC text ("" without one).
func (t *Tune) ABC() string {
	if t.Transcription == nil || t.Transcription.Format != TranscriptionABC {
		return ""
	}
	return t.Transcription.Text
}

// Slug is the readable part of the Tune's Permalink.
func (t *Tune) Slug() string { return slugOr(t.Name.Name, "tune") }

// TuneSet is a Set: Tunes played back to back, in order.
type TuneSet struct {
	ID        string    `json:"id"`
	Name      Bi        `json:"name,omitzero"`
	TuneIDs   []string  `json:"tuneIds"`
	Hidden    bool      `json:"hidden,omitempty"`
	Rev       int       `json:"rev"`
	CreatedAt time.Time `json:"createdAt"`
	CreatedBy string    `json:"createdBy"`
	UpdatedAt time.Time `json:"updatedAt"`
	UpdatedBy string    `json:"updatedBy"`
}

// TuneCollection is the aggregate: one per Organization with the opt-in.
type TuneCollection struct {
	OrgID       string        `json:"orgId"`
	Types       []TuneType    `json:"types"`
	Instruments []Instrument  `json:"instruments"`
	Origins     []TuneCountry `json:"origins"`
	Tunes       []Tune        `json:"tunes"`
	Sets        []TuneSet     `json:"sets"`
	// Retired holds the ids of removed items, so an id is never reused.
	Retired   []string  `json:"retired,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitzero"`
	UpdatedBy string    `json:"updatedBy,omitempty"`
}

// seedID is the id of a seeded list entry: the same on every load of a
// collection never saved, so a screen can refer to it before the first change.
func seedID(kind, key string) string {
	sum := sha256.Sum256([]byte("folk-ms tune seed\x00" + kind + "\x00" + key))
	return shortIDEncoding.EncodeToString(sum[:5])
}

var seedTypes = []struct {
	key, pl, en, meter string
	tempo              int
}{
	{"reel", "reel", "reel", "4/4", 180},
	{"jig", "jig", "jig", "6/8", 110},
	{"slip-jig", "slip jig", "slip jig", "9/8", 110},
	{"hornpipe", "hornpipe", "hornpipe", "4/4", 140},
	{"polka", "polka", "polka", "2/4", 200},
	{"slide", "slide", "slide", "12/8", 130},
	{"waltz", "walc", "waltz", "3/4", 140},
	{"mazurka", "mazurek", "mazurka", "3/4", 130},
	{"barndance", "barndance", "barndance", "4/4", 150},
	{"march", "marsz", "march", "2/4", 110},
	{"strathspey", "strathspey", "strathspey", "4/4", 120},
	{"air", "melodia (air)", "air", "3/4", 70},
}

var seedInstruments = []struct{ key, pl, en string }{
	{"whistle", "whistle", "whistle"},
	{"flute", "flet", "flute"},
	{"fiddle", "skrzypce", "fiddle"},
	{"button-accordion", "akordeon guzikowy", "button accordion"},
	{"concertina", "koncertyna", "concertina"},
	{"uilleann-pipes", "dudy irlandzkie (uilleann)", "uilleann pipes"},
	{"banjo", "banjo", "banjo"},
	{"mandolin", "mandolina", "mandolin"},
	{"guitar", "gitara", "guitar"},
	{"bouzouki", "buzuki", "bouzouki"},
	{"bodhran", "bodhrán", "bodhrán"},
	{"harp", "harfa", "harp"},
	{"piano", "pianino", "piano"},
}

// NewTuneCollection is the collection of an Organization before its first
// change: seeded with the common Irish Tune Types and session Instruments.
func NewTuneCollection(org string) *TuneCollection {
	c := &TuneCollection{OrgID: org, Types: []TuneType{}, Instruments: []Instrument{}, Origins: []TuneCountry{}, Tunes: []Tune{}, Sets: []TuneSet{}}
	for _, s := range seedTypes {
		c.Types = append(c.Types, TuneType{ID: seedID("type", s.key), Label: Bi{PL: s.pl, EN: s.en}, Meter: s.meter, Tempo: s.tempo, Rev: 1})
	}
	for _, s := range seedInstruments {
		c.Instruments = append(c.Instruments, Instrument{ID: seedID("instrument", s.key), Label: Bi{PL: s.pl, EN: s.en}, Rev: 1})
	}
	return c
}

// Normalize fills nil lists after decoding, so every list encodes as [].
func (c *TuneCollection) Normalize() {
	for _, p := range []*[]TuneType{&c.Types} {
		if *p == nil {
			*p = []TuneType{}
		}
	}
	if c.Instruments == nil {
		c.Instruments = []Instrument{}
	}
	if c.Origins == nil {
		c.Origins = []TuneCountry{}
	}
	if c.Tunes == nil {
		c.Tunes = []Tune{}
	}
	if c.Sets == nil {
		c.Sets = []TuneSet{}
	}
	for i := range c.Tunes {
		t := &c.Tunes[i]
		for _, p := range []*[]string{&t.Suits, &t.Tricky} {
			if *p == nil {
				*p = []string{}
			}
		}
		if t.OtherNames == nil {
			t.OtherNames = []TuneName{}
		}
		if t.Recordings == nil {
			t.Recordings = []Recording{}
		}
		if t.Attachments == nil {
			t.Attachments = []Attachment{}
		}
	}
}

func (c *TuneCollection) touch(by string, now time.Time) { c.UpdatedAt, c.UpdatedBy = now, by }
func (c *TuneCollection) retire(id string)               { c.Retired = append(c.Retired, id) }

func (c *TuneCollection) taken(id string) bool {
	if slices.Contains(c.Retired, id) || c.TuneIndex(id) >= 0 || c.SetIndex(id) >= 0 || c.typeIndex(id) >= 0 || c.instrumentIndex(id) >= 0 {
		return true
	}
	ci, _ := c.origin(id)
	if ci >= 0 {
		return true
	}
	for _, t := range c.Tunes {
		for _, a := range t.Attachments {
			if a.ID == id {
				return true
			}
		}
	}
	return false
}

// NewItemID draws a shortId-shaped id no item of the collection has had.
func (c *TuneCollection) NewItemID(r io.Reader) (string, error) {
	return GenerateShortID(r, c.taken)
}

// TuneIndex finds a Tune.
func (c *TuneCollection) TuneIndex(id string) int {
	return slices.IndexFunc(c.Tunes, func(t Tune) bool { return t.ID == id })
}

// SetIndex finds a Set.
func (c *TuneCollection) SetIndex(id string) int {
	return slices.IndexFunc(c.Sets, func(s TuneSet) bool { return s.ID == id })
}

func (c *TuneCollection) typeIndex(id string) int {
	return slices.IndexFunc(c.Types, func(t TuneType) bool { return t.ID == id })
}

func (c *TuneCollection) instrumentIndex(id string) int {
	return slices.IndexFunc(c.Instruments, func(t Instrument) bool { return t.ID == id })
}

// origin finds a country (region index -1) or a region.
func (c *TuneCollection) origin(id string) (country, region int) {
	for i, co := range c.Origins {
		if co.ID == id {
			return i, -1
		}
		for j, r := range co.Regions {
			if r.ID == id {
				return i, j
			}
		}
	}
	return -1, -1
}

// Tune returns a Tune by id.
func (c *TuneCollection) Tune(id string) (*Tune, bool) {
	i := c.TuneIndex(id)
	if i < 0 {
		return nil, false
	}
	return &c.Tunes[i], true
}

// Set returns a Set by id.
func (c *TuneCollection) Set(id string) (*TuneSet, bool) {
	i := c.SetIndex(id)
	if i < 0 {
		return nil, false
	}
	return &c.Sets[i], true
}

// TuneType returns a Tune Type by id.
func (c *TuneCollection) TuneType(id string) (*TuneType, bool) {
	i := c.typeIndex(id)
	if i < 0 {
		return nil, false
	}
	return &c.Types[i], true
}

// SetsWith lists the Sets a Tune is in.
func (c *TuneCollection) SetsWith(tuneID string) []*TuneSet {
	var out []*TuneSet
	for i := range c.Sets {
		if slices.Contains(c.Sets[i].TuneIDs, tuneID) {
			out = append(out, &c.Sets[i])
		}
	}
	return out
}

// TypeUse, InstrumentUse and OriginUse count the Tunes that use an entry
// (OriginUse: exactly that country or region).
func (c *TuneCollection) TypeUse(id string) int {
	n := 0
	for _, t := range c.Tunes {
		if t.TypeID == id {
			n++
		}
	}
	return n
}

func (c *TuneCollection) InstrumentUse(id string) int {
	n := 0
	for _, t := range c.Tunes {
		if slices.Contains(t.Suits, id) || slices.Contains(t.Tricky, id) {
			n++
		}
	}
	return n
}

func (c *TuneCollection) OriginUse(id string) int {
	n := 0
	for _, t := range c.Tunes {
		if t.OriginID == id {
			n++
		}
	}
	return n
}

// TempoOf is the Playback Tempo of a Tune: its own, else its Tune Type's.
func (c *TuneCollection) TempoOf(t *Tune) int {
	if t.Tempo > 0 {
		return t.Tempo
	}
	if tt, ok := c.TuneType(t.TypeID); ok {
		return tt.Tempo
	}
	return 120
}

// TempoBeat is the note a Playback Tempo counts for a Time Signature:
// 3/8 (a dotted quarter) in compound metres, 1/2 in 2/2, else one note of
// the signature's denominator.
func TempoBeat(meter string) string {
	n, d, ok := strings.Cut(meter, "/")
	if !ok {
		return "1/4"
	}
	switch {
	case d == "8" && (n == "3" || n == "6" || n == "9" || n == "12"):
		return "3/8"
	case d == "2":
		return "1/2"
	}
	return "1/" + d
}

func itemConflict(what string) error {
	return fmt.Errorf("%w: the %s was changed meanwhile", ErrConflict, what)
}

// --- Text helpers --------------------------------------------------------

var foldTable = func() map[rune]string {
	m := map[rune]string{'ß': "ss", 'æ': "ae", 'œ': "oe", 'ø': "o", 'ł': "l", 'đ': "d", 'ð': "d", 'þ': "th"}
	for base, chars := range map[string]string{
		"a": "àáâãäåāăą", "c": "çćĉċč", "d": "ď", "e": "èéêëēĕėęě", "g": "ĝğġģ", "h": "ĥħ",
		"i": "ìíîïĩīĭįı", "j": "ĵ", "k": "ķ", "l": "ĺļľŀ", "n": "ñńņňŉ", "o": "òóôõöōŏő",
		"r": "ŕŗř", "s": "śŝşšș", "t": "ţťŧț", "u": "ùúûüũūŭůűų", "w": "ŵ", "y": "ýÿŷ", "z": "źżž",
	} {
		for _, r := range chars {
			m[r] = base
		}
	}
	return m
}()

// Fold lowercases s and drops diacritics, for search, sorting and slugs.
func Fold(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if f, ok := foldTable[r]; ok {
			b.WriteString(f)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SortKey orders Tunes: by name, ignoring a leading article (The, An, A,
// Na) and diacritics.
func SortKey(name string) string {
	k := strings.TrimSpace(Fold(name))
	for _, a := range []string{"the ", "an ", "a ", "na "} {
		if rest, ok := strings.CutPrefix(k, a); ok && strings.TrimSpace(rest) != "" {
			return strings.TrimSpace(rest)
		}
	}
	return k
}

func slugOr(s, fallback string) string {
	if sl := Slugify(Fold(s)); sl != "" {
		return sl
	}
	return fallback
}

func oneLine(s string, max int) string { return cleanText(strings.ReplaceAll(s, "\n", " "), max) }

// cleanBiLine trims a one-line bilingual text and checks its length.
func cleanBiLine(p *problems, field string, b Bi, max int) Bi {
	out := Bi{PL: oneLine(b.PL, max+1), EN: oneLine(b.EN, max+1)}
	for _, x := range []struct{ l, v string }{{"pl", out.PL}, {"en", out.EN}} {
		if utf8.RuneCountInString(x.v) > max {
			p.add(field+"."+x.l, "tekst jest za długi (maks. %d znaków)", "the text is too long (max. %d characters)", max)
		}
	}
	return out
}

func cleanTuneLabel(p *problems, b Bi) Bi {
	l := cleanBiLine(p, "label", b, MaxLabelLen)
	if l.IsZero() {
		p.add("label.pl", "podaj nazwę po polsku lub po angielsku", "enter a name in Polish or English")
	}
	return l
}

func sameBi(a, b Bi) bool {
	return a.PL != "" && strings.EqualFold(a.PL, b.PL) || a.EN != "" && strings.EqualFold(a.EN, b.EN)
}

// --- Tunes ------------------------------------------------------------------

// RecordingInput is a Recording as the Moderator enters it.
type RecordingInput struct {
	Link  string `json:"link"`
	Label string `json:"label"`
}

// TuneInput is a Tune as the Moderator enters it.
type TuneInput struct {
	Name        TuneName         `json:"name"`
	OtherNames  []TuneName       `json:"otherNames"`
	TypeID      string           `json:"typeId"`
	Key         Key              `json:"key"`
	Meter       string           `json:"meter"`
	Tempo       int              `json:"tempo"`
	OriginID    string           `json:"originId"`
	Composer    string           `json:"composer"`
	Suits       []string         `json:"suits"`
	Tricky      []string         `json:"tricky"`
	ABC         string           `json:"abc"`
	IncipitBars int              `json:"incipitBars"`
	Recordings  []RecordingInput `json:"recordings"`
	Description Bi               `json:"description"`
}

// videoHosts are the hosts ParseVideoLink reads.
var videoHosts = map[string]bool{
	"youtube.com": true, "www.youtube.com": true, "m.youtube.com": true, "music.youtube.com": true,
	"youtube-nocookie.com": true, "www.youtube-nocookie.com": true, "youtu.be": true, "www.youtu.be": true,
	"vimeo.com": true, "www.vimeo.com": true, "player.vimeo.com": true,
}

// ParseRecordingLink reads a Recording's link: a YouTube or Vimeo video
// (with its start time), or any other https address, kept as it is.
func ParseRecordingLink(raw string) (Recording, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return Recording{}, Problem("link", "link jest wymagany", "the link is required")
	}
	probe := s
	if !strings.Contains(probe, "://") {
		probe = "https://" + probe
	}
	if u, err := url.Parse(probe); err == nil && videoHosts[strings.ToLower(u.Hostname())] {
		l, err := ParseVideoLink(s)
		if err != nil {
			return Recording{}, err
		}
		return Recording{Video: &l}, nil
	}
	if !IsHTTPSURL(s) {
		return Recording{}, Problem("link", "podaj link zaczynający się od https://", "enter a link starting with https://")
	}
	u, _ := url.Parse(s)
	if u.Port() != "" {
		return Recording{}, Problem("link", "podaj link zaczynający się od https://", "enter a link starting with https://")
	}
	return Recording{URL: s}, nil
}

func (c *TuneCollection) cleanName(p *problems, field string, n TuneName, required bool) TuneName {
	out := TuneName{Name: oneLine(n.Name, MaxTuneNameLen+1)}
	out.Translation = cleanBiLine(p, field+".translation", n.Translation, MaxTuneNameLen)
	if out.Name == "" && required {
		p.add(field+".name", "nazwa jest wymagana", "the name is required")
	}
	if utf8.RuneCountInString(out.Name) > MaxTuneNameLen {
		p.add(field+".name", "tekst jest za długi (maks. %d znaków)", "the text is too long (max. %d characters)", MaxTuneNameLen)
	}
	return out
}

// PrepareTune validates an input against the collection's lists and
// returns the Tune's content (no id, revision, times, Hidden or
// Attachments). Recordings are parsed; Vimeo thumbnails are the caller's.
func (c *TuneCollection) PrepareTune(in TuneInput) (Tune, error) {
	var p problems
	t := Tune{
		Name:       c.cleanName(&p, "name", in.Name, true),
		OtherNames: []TuneName{},
		TypeID:     strings.TrimSpace(in.TypeID),
		Key:        Key{Tonic: strings.TrimSpace(in.Key.Tonic), Mode: strings.TrimSpace(in.Key.Mode)},
		Meter:      strings.ReplaceAll(strings.TrimSpace(in.Meter), " ", ""),
		Tempo:      in.Tempo,
		OriginID:   strings.TrimSpace(in.OriginID),
		Composer:   oneLine(in.Composer, MaxComposerLen+1),
		Suits:      []string{}, Tricky: []string{},
		IncipitBars: in.IncipitBars,
		Recordings:  []Recording{},
		Attachments: []Attachment{},
	}
	if len(in.OtherNames) > MaxOtherNames {
		p.add("otherNames", "najwyżej %d innych nazw", "at most %d other names", MaxOtherNames)
	}
	for i, n := range in.OtherNames[:min(len(in.OtherNames), MaxOtherNames)] {
		on := c.cleanName(&p, fmt.Sprintf("otherNames.%d", i), n, false)
		if on.Name == "" {
			if !on.Translation.IsZero() {
				p.add(fmt.Sprintf("otherNames.%d.name", i), "nazwa jest wymagana", "the name is required")
			}
			continue
		}
		t.OtherNames = append(t.OtherNames, on)
	}
	if t.TypeID == "" {
		p.add("typeId", "wybierz rodzaj melodii", "choose a tune type")
	} else if c.typeIndex(t.TypeID) < 0 {
		p.add("typeId", "wybierz rodzaj melodii z listy", "choose a tune type from the list")
	}
	if !t.Key.Valid() {
		p.add("key", "wybierz tonację: dźwięk i skalę", "choose a key: tonic and mode")
	}
	if !ValidMeter(t.Meter) {
		p.add("meter", "podaj metrum jako liczba/liczba, np. 4/4 lub 6/8", "enter the time signature as number/number, e.g. 4/4 or 6/8")
	}
	if t.Tempo != 0 && (t.Tempo < MinTempo || t.Tempo > MaxTempo) {
		p.add("tempo", "tempo od %d do %d na minutę", "a tempo from %d to %d a minute", MinTempo, MaxTempo)
	}
	if t.OriginID != "" {
		if ci, _ := c.origin(t.OriginID); ci < 0 {
			p.add("originId", "wybierz pochodzenie z listy", "choose an origin from the list")
		}
	}
	if utf8.RuneCountInString(t.Composer) > MaxComposerLen {
		p.add("composer", "tekst jest za długi (maks. %d znaków)", "the text is too long (max. %d characters)", MaxComposerLen)
	}
	pick := func(field string, ids []string, dst *[]string) {
		for _, id := range ids {
			if c.instrumentIndex(id) < 0 {
				p.add(field, "wybierz instrumenty z listy", "choose instruments from the list")
				return
			}
			if !slices.Contains(*dst, id) {
				*dst = append(*dst, id)
			}
		}
		slices.SortStableFunc(*dst, func(a, b string) int { return c.instrumentIndex(a) - c.instrumentIndex(b) })
	}
	pick("suits", in.Suits, &t.Suits)
	pick("tricky", in.Tricky, &t.Tricky)
	for _, id := range t.Suits {
		if slices.Contains(t.Tricky, id) {
			p.add("tricky", "instrument nie może być jednocześnie wygodny i trudny", "an instrument cannot be both suited and tricky")
			break
		}
	}
	abc := strings.TrimSpace(strings.ToValidUTF8(strings.ReplaceAll(in.ABC, "\r\n", "\n"), ""))
	switch {
	case len(abc) > MaxABCBytes:
		p.add("abc", "zapis ABC jest za długi (maks. %d KB)", "the ABC is too long (max. %d KB)", MaxABCBytes>>10)
	case strings.ContainsRune(abc, 0):
		p.add("abc", "zapis ABC zawiera niedozwolone znaki", "the ABC contains characters that are not allowed")
	case abc != "":
		t.Transcription = &Transcription{Format: TranscriptionABC, Text: abc}
		if _, ok := Incipit(abc, 1); !ok {
			p.add("abc", "zapis ABC musi mieć linię K: i nuty", "the ABC needs a K: line and notes")
		}
	}
	if t.IncipitBars < 0 || t.IncipitBars > MaxIncipitBars {
		p.add("incipitBars", "od 1 do %d taktów", "from 1 to %d bars", MaxIncipitBars)
	}
	if t.IncipitBars == DefaultIncipitBars {
		t.IncipitBars = 0
	}
	if len(in.Recordings) > MaxRecordings {
		p.add("recordings", "najwyżej %d nagrań", "at most %d recordings", MaxRecordings)
	}
	for i, r := range in.Recordings[:min(len(in.Recordings), MaxRecordings)] {
		field := fmt.Sprintf("recordings.%d", i)
		if strings.TrimSpace(r.Link) == "" && strings.TrimSpace(r.Label) == "" {
			continue
		}
		rec, err := ParseRecordingLink(r.Link)
		if ve, ok := err.(*ValidationError); ok {
			for _, pr := range ve.Problems {
				p = append(p, FieldError{Field: field + "." + pr.Field, Message: pr.Message})
			}
		} else if err != nil {
			return Tune{}, err
		}
		rec.Label = oneLine(r.Label, MaxRecordingLabelLen+1)
		if utf8.RuneCountInString(rec.Label) > MaxRecordingLabelLen {
			p.add(field+".label", "tekst jest za długi (maks. %d znaków)", "the text is too long (max. %d characters)", MaxRecordingLabelLen)
		}
		t.Recordings = append(t.Recordings, rec)
	}
	t.Description = Bi{PL: cleanText(in.Description.PL, MaxTuneDescLen+1), EN: cleanText(in.Description.EN, MaxTuneDescLen+1)}
	for _, x := range []struct{ l, v string }{{"pl", t.Description.PL}, {"en", t.Description.EN}} {
		if utf8.RuneCountInString(x.v) > MaxTuneDescLen {
			p.add("description."+x.l, "tekst jest za długi (maks. %d znaków)", "the text is too long (max. %d characters)", MaxTuneDescLen)
		}
	}
	return t, p.err()
}

// AddTune adds a prepared Tune (Hidden as given: an import adds Hidden Tunes).
func (c *TuneCollection) AddTune(t Tune, id string, hidden bool, by string, now time.Time) error {
	if len(c.Tunes) >= MaxTunes {
		return Problem("tunes", fmt.Sprintf("kolekcja może mieć najwyżej %d melodii", MaxTunes), fmt.Sprintf("the collection can hold at most %d tunes", MaxTunes))
	}
	if c.taken(id) {
		return fmt.Errorf("%w: id %s is taken", ErrConflict, id)
	}
	t.ID, t.Hidden, t.Rev = id, hidden, 1
	t.Attachments = []Attachment{}
	t.CreatedAt, t.CreatedBy, t.UpdatedAt, t.UpdatedBy = now, by, now, by
	c.Tunes = append(c.Tunes, t)
	c.touch(by, now)
	return nil
}

// UpdateTune replaces a Tune's content if it is still at rev; its id,
// Hidden, Attachments and creation stay.
func (c *TuneCollection) UpdateTune(id string, rev int, t Tune, by string, now time.Time) error {
	cur, ok := c.Tune(id)
	if !ok {
		return notFound("tune")
	}
	if cur.Rev != rev {
		return itemConflict("tune")
	}
	t.ID, t.Hidden, t.Attachments, t.CreatedAt, t.CreatedBy = cur.ID, cur.Hidden, cur.Attachments, cur.CreatedAt, cur.CreatedBy
	t.Rev = cur.Rev + 1
	t.UpdatedAt, t.UpdatedBy = now, by
	*cur = t
	c.touch(by, now)
	return nil
}

// SetTuneHidden switches Hide. It changes no content, so it needs no revision.
func (c *TuneCollection) SetTuneHidden(id string, hidden bool, by string, now time.Time) error {
	t, ok := c.Tune(id)
	if !ok {
		return notFound("tune")
	}
	t.Hidden = hidden
	t.UpdatedAt, t.UpdatedBy = now, by
	c.touch(by, now)
	return nil
}

// RemovedTune is what removing a Tune did: the Tune (for the Audit Log and
// its Attachments' files), the Sets it was taken out of and the Sets
// removed because no Tune was left in them.
type RemovedTune struct {
	Tune        Tune
	LeftSets    []string
	RemovedSets []string
}

// RemoveTune deletes a Tune at rev. When it is in Sets, confirm must be
// true: it is then taken out of them in the same change, and a Set left
// empty is removed.
func (c *TuneCollection) RemoveTune(id string, rev int, confirm bool, by string, now time.Time) (RemovedTune, error) {
	i := c.TuneIndex(id)
	if i < 0 {
		return RemovedTune{}, notFound("tune")
	}
	if c.Tunes[i].Rev != rev {
		return RemovedTune{}, itemConflict("tune")
	}
	if len(c.SetsWith(id)) > 0 && !confirm {
		return RemovedTune{}, fmt.Errorf("%w: the tune is in sets", ErrNeedsConfirm)
	}
	out := RemovedTune{Tune: c.Tunes[i]}
	c.Tunes = slices.Delete(c.Tunes, i, i+1)
	c.retire(id)
	for j := 0; j < len(c.Sets); j++ {
		s := &c.Sets[j]
		k := slices.Index(s.TuneIDs, id)
		if k < 0 {
			continue
		}
		s.TuneIDs = slices.DeleteFunc(s.TuneIDs, func(x string) bool { return x == id })
		if len(s.TuneIDs) == 0 {
			out.RemovedSets = append(out.RemovedSets, s.ID)
			c.retire(s.ID)
			c.Sets = slices.Delete(c.Sets, j, j+1)
			j--
			continue
		}
		s.Rev++
		s.UpdatedAt, s.UpdatedBy = now, by
		out.LeftSets = append(out.LeftSets, s.ID)
	}
	c.touch(by, now)
	return out, nil
}

// --- Attachments ------------------------------------------------------------

// AttachmentPrefix is the public store prefix of a Tune's Attachments.
func AttachmentPrefix(org, tuneID string) string { return "media/" + org + "/tunes/" + tuneID + "/" }

func (t *Tune) attachmentIndex(id string) int {
	return slices.IndexFunc(t.Attachments, func(a Attachment) bool { return a.ID == id })
}

// CanAddAttachment reports whether the Tune can take one more Attachment.
func (c *TuneCollection) CanAddAttachment(tuneID string) error {
	t, ok := c.Tune(tuneID)
	if !ok {
		return notFound("tune")
	}
	if len(t.Attachments) >= MaxAttachments {
		return Problem("attachments", fmt.Sprintf("melodia może mieć najwyżej %d załączników", MaxAttachments), fmt.Sprintf("a tune can have at most %d attachments", MaxAttachments))
	}
	return nil
}

// AddAttachment appends a stored Attachment to a Tune.
func (c *TuneCollection) AddAttachment(tuneID string, a Attachment, by string, now time.Time) error {
	if err := c.CanAddAttachment(tuneID); err != nil {
		return err
	}
	if c.taken(a.ID) {
		return fmt.Errorf("%w: id %s is taken", ErrConflict, a.ID)
	}
	t, _ := c.Tune(tuneID)
	var p problems
	a.Caption = cleanBiLine(&p, "caption", a.Caption, MaxAttachmentCaptionLen)
	if err := p.err(); err != nil {
		return err
	}
	a.AddedAt, a.AddedBy = now, by
	t.Attachments = append(t.Attachments, a)
	t.UpdatedAt, t.UpdatedBy = now, by
	c.touch(by, now)
	return nil
}

// SetAttachmentCaption changes an Attachment's caption (alt text of an image).
func (c *TuneCollection) SetAttachmentCaption(tuneID, id string, caption Bi, by string, now time.Time) error {
	t, ok := c.Tune(tuneID)
	if !ok {
		return notFound("tune")
	}
	i := t.attachmentIndex(id)
	if i < 0 {
		return notFound("attachment")
	}
	var p problems
	cl := cleanBiLine(&p, "caption", caption, MaxAttachmentCaptionLen)
	if err := p.err(); err != nil {
		return err
	}
	t.Attachments[i].Caption = cl
	t.UpdatedAt, t.UpdatedBy = now, by
	c.touch(by, now)
	return nil
}

// MoveAttachment moves an Attachment within its Tune.
func (c *TuneCollection) MoveAttachment(tuneID, id string, to int, by string, now time.Time) error {
	t, ok := c.Tune(tuneID)
	if !ok {
		return notFound("tune")
	}
	i := t.attachmentIndex(id)
	if i < 0 {
		return notFound("attachment")
	}
	t.Attachments = move(t.Attachments, i, to)
	t.UpdatedAt, t.UpdatedBy = now, by
	c.touch(by, now)
	return nil
}

// RemoveAttachment removes an Attachment and returns it (its files are the
// caller's to delete once the document is saved).
func (c *TuneCollection) RemoveAttachment(tuneID, id string, by string, now time.Time) (Attachment, error) {
	t, ok := c.Tune(tuneID)
	if !ok {
		return Attachment{}, notFound("tune")
	}
	i := t.attachmentIndex(id)
	if i < 0 {
		return Attachment{}, notFound("attachment")
	}
	a := t.Attachments[i]
	t.Attachments = slices.Delete(t.Attachments, i, i+1)
	c.retire(id)
	t.UpdatedAt, t.UpdatedBy = now, by
	c.touch(by, now)
	return a, nil
}

// SafeFileName keeps the readable part of an uploaded file's name for
// Content-Disposition and the Site: letters, digits, spaces, dots, hyphens
// and underscores, at most 100 characters, with the given extension.
func SafeFileName(name, ext string) string {
	name = strings.TrimSuffix(strings.TrimSpace(name), "."+ext)
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	var b strings.Builder
	for _, r := range Fold(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	s := strings.Trim(b.String(), ".-_")
	if len(s) > 100 {
		s = s[:100]
	}
	if s == "" {
		s = "attachment"
	}
	return s + "." + ext
}

// --- Sets -------------------------------------------------------------------

// SetInput is a Set as the Moderator enters it.
type SetInput struct {
	Name    Bi       `json:"name"`
	TuneIDs []string `json:"tuneIds"`
}

func (c *TuneCollection) prepareSet(in SetInput) (TuneSet, error) {
	var p problems
	s := TuneSet{Name: cleanBiLine(&p, "name", in.Name, MaxSetNameLen), TuneIDs: []string{}}
	for _, id := range in.TuneIDs {
		if c.TuneIndex(id) < 0 {
			p.add("tuneIds", "wybierz melodie z listy", "choose tunes from the list")
			break
		}
		if slices.Contains(s.TuneIDs, id) {
			p.add("tuneIds", "melodia może być w zestawie tylko raz", "a tune can be in a set only once")
			break
		}
		s.TuneIDs = append(s.TuneIDs, id)
	}
	if len(in.TuneIDs) < MinSetTunes || len(in.TuneIDs) > MaxSetTunes {
		p.add("tuneIds", "zestaw ma od %d do %d melodii", "a set has %d to %d tunes", MinSetTunes, MaxSetTunes)
	}
	return s, p.err()
}

// AddSet adds a Set.
func (c *TuneCollection) AddSet(id string, in SetInput, by string, now time.Time) error {
	s, err := c.prepareSet(in)
	if err != nil {
		return err
	}
	if len(c.Sets) >= MaxTuneSets {
		return Problem("tuneIds", fmt.Sprintf("kolekcja może mieć najwyżej %d zestawów", MaxTuneSets), fmt.Sprintf("the collection can hold at most %d sets", MaxTuneSets))
	}
	if c.taken(id) {
		return fmt.Errorf("%w: id %s is taken", ErrConflict, id)
	}
	s.ID, s.Rev = id, 1
	s.CreatedAt, s.CreatedBy, s.UpdatedAt, s.UpdatedBy = now, by, now, by
	c.Sets = append(c.Sets, s)
	c.touch(by, now)
	return nil
}

// UpdateSet replaces a Set's name and Tunes if it is still at rev.
func (c *TuneCollection) UpdateSet(id string, rev int, in SetInput, by string, now time.Time) error {
	cur, ok := c.Set(id)
	if !ok {
		return notFound("set")
	}
	if cur.Rev != rev {
		return itemConflict("set")
	}
	s, err := c.prepareSet(in)
	if err != nil {
		return err
	}
	cur.Name, cur.TuneIDs = s.Name, s.TuneIDs
	cur.Rev++
	cur.UpdatedAt, cur.UpdatedBy = now, by
	c.touch(by, now)
	return nil
}

// RemoveSet deletes a Set at rev.
func (c *TuneCollection) RemoveSet(id string, rev int, by string, now time.Time) (TuneSet, error) {
	i := c.SetIndex(id)
	if i < 0 {
		return TuneSet{}, notFound("set")
	}
	if c.Sets[i].Rev != rev {
		return TuneSet{}, itemConflict("set")
	}
	s := c.Sets[i]
	c.Sets = slices.Delete(c.Sets, i, i+1)
	c.retire(id)
	c.touch(by, now)
	return s, nil
}

// SetSetHidden switches a Set's Hide.
func (c *TuneCollection) SetSetHidden(id string, hidden bool, by string, now time.Time) error {
	s, ok := c.Set(id)
	if !ok {
		return notFound("set")
	}
	s.Hidden = hidden
	s.UpdatedAt, s.UpdatedBy = now, by
	c.touch(by, now)
	return nil
}

// SetTitle is a Set's name in l, else its Tunes' names joined with " / ".
func (c *TuneCollection) SetTitle(s *TuneSet, l Lang) string {
	if !s.Name.IsZero() {
		return s.Name.In(l)
	}
	var names []string
	for _, id := range s.TuneIDs {
		if t, ok := c.Tune(id); ok {
			names = append(names, t.Name.Name)
		}
	}
	return strings.Join(names, " / ")
}

// --- Lists ------------------------------------------------------------------

func tuneInUse(n int) error {
	return fmt.Errorf("%w: %w", ErrInvalidState, Problem("label.pl",
		fmt.Sprintf("nie można usunąć: używa tego %d melodii", n), fmt.Sprintf("cannot remove: %d tune(s) use it", n)))
}

// TuneTypeInput is a Tune Type as the Moderator enters it.
type TuneTypeInput struct {
	Label Bi     `json:"label"`
	Meter string `json:"meter"`
	Tempo int    `json:"tempo"`
}

func (c *TuneCollection) prepareType(in TuneTypeInput, self int) (TuneType, error) {
	var p problems
	t := TuneType{Label: cleanTuneLabel(&p, in.Label), Meter: strings.ReplaceAll(strings.TrimSpace(in.Meter), " ", ""), Tempo: in.Tempo}
	if !ValidMeter(t.Meter) {
		p.add("meter", "podaj metrum jako liczba/liczba, np. 4/4 lub 6/8", "enter the time signature as number/number, e.g. 4/4 or 6/8")
	}
	if t.Tempo < MinTempo || t.Tempo > MaxTempo {
		p.add("tempo", "tempo od %d do %d na minutę", "a tempo from %d to %d a minute", MinTempo, MaxTempo)
	}
	for j, o := range c.Types {
		if j != self && sameBi(o.Label, t.Label) {
			p.add("label.pl", "taka nazwa już jest na liście", "this name is already on the list")
			break
		}
	}
	return t, p.err()
}

// AddTuneType appends a Tune Type.
func (c *TuneCollection) AddTuneType(id string, in TuneTypeInput, by string, now time.Time) error {
	t, err := c.prepareType(in, -1)
	if err != nil {
		return err
	}
	if len(c.Types) >= MaxTuneTypes {
		return Problem("label.pl", fmt.Sprintf("lista może mieć najwyżej %d rodzajów", MaxTuneTypes), fmt.Sprintf("the list can hold at most %d tune types", MaxTuneTypes))
	}
	if c.taken(id) {
		return fmt.Errorf("%w: id %s is taken", ErrConflict, id)
	}
	t.ID, t.Rev = id, 1
	c.Types = append(c.Types, t)
	c.touch(by, now)
	return nil
}

// UpdateTuneType changes a Tune Type at rev.
func (c *TuneCollection) UpdateTuneType(id string, rev int, in TuneTypeInput, by string, now time.Time) error {
	i := c.typeIndex(id)
	if i < 0 {
		return notFound("tune type")
	}
	if c.Types[i].Rev != rev {
		return itemConflict("tune type")
	}
	t, err := c.prepareType(in, i)
	if err != nil {
		return err
	}
	t.ID, t.Rev = id, rev+1
	c.Types[i] = t
	c.touch(by, now)
	return nil
}

// RemoveTuneType removes a Tune Type no Tune uses.
func (c *TuneCollection) RemoveTuneType(id string, by string, now time.Time) (TuneType, error) {
	i := c.typeIndex(id)
	if i < 0 {
		return TuneType{}, notFound("tune type")
	}
	if n := c.TypeUse(id); n > 0 {
		return TuneType{}, tuneInUse(n)
	}
	t := c.Types[i]
	c.Types = slices.Delete(c.Types, i, i+1)
	c.retire(id)
	c.touch(by, now)
	return t, nil
}

// MoveTuneType moves a Tune Type to an index of the list.
func (c *TuneCollection) MoveTuneType(id string, to int, by string, now time.Time) error {
	i := c.typeIndex(id)
	if i < 0 {
		return notFound("tune type")
	}
	c.Types = move(c.Types, i, to)
	c.touch(by, now)
	return nil
}

func (c *TuneCollection) prepareInstrument(label Bi, self int) (Bi, error) {
	var p problems
	l := cleanTuneLabel(&p, label)
	for j, o := range c.Instruments {
		if j != self && sameBi(o.Label, l) {
			p.add("label.pl", "taka nazwa już jest na liście", "this name is already on the list")
			break
		}
	}
	return l, p.err()
}

// AddInstrument appends an Instrument.
func (c *TuneCollection) AddInstrument(id string, label Bi, by string, now time.Time) error {
	l, err := c.prepareInstrument(label, -1)
	if err != nil {
		return err
	}
	if len(c.Instruments) >= MaxInstruments {
		return Problem("label.pl", fmt.Sprintf("lista może mieć najwyżej %d instrumentów", MaxInstruments), fmt.Sprintf("the list can hold at most %d instruments", MaxInstruments))
	}
	if c.taken(id) {
		return fmt.Errorf("%w: id %s is taken", ErrConflict, id)
	}
	c.Instruments = append(c.Instruments, Instrument{ID: id, Label: l, Rev: 1})
	c.touch(by, now)
	return nil
}

// UpdateInstrument renames an Instrument at rev.
func (c *TuneCollection) UpdateInstrument(id string, rev int, label Bi, by string, now time.Time) error {
	i := c.instrumentIndex(id)
	if i < 0 {
		return notFound("instrument")
	}
	if c.Instruments[i].Rev != rev {
		return itemConflict("instrument")
	}
	l, err := c.prepareInstrument(label, i)
	if err != nil {
		return err
	}
	c.Instruments[i].Label = l
	c.Instruments[i].Rev++
	c.touch(by, now)
	return nil
}

// RemoveInstrument removes an Instrument no Tune lists.
func (c *TuneCollection) RemoveInstrument(id string, by string, now time.Time) (Instrument, error) {
	i := c.instrumentIndex(id)
	if i < 0 {
		return Instrument{}, notFound("instrument")
	}
	if n := c.InstrumentUse(id); n > 0 {
		return Instrument{}, tuneInUse(n)
	}
	in := c.Instruments[i]
	c.Instruments = slices.Delete(c.Instruments, i, i+1)
	c.retire(id)
	c.touch(by, now)
	return in, nil
}

// MoveInstrument moves an Instrument; Tunes list their Instruments in the
// list's order.
func (c *TuneCollection) MoveInstrument(id string, to int, by string, now time.Time) error {
	i := c.instrumentIndex(id)
	if i < 0 {
		return notFound("instrument")
	}
	c.Instruments = move(c.Instruments, i, to)
	order := func(a, b string) int { return c.instrumentIndex(a) - c.instrumentIndex(b) }
	for k := range c.Tunes {
		slices.SortStableFunc(c.Tunes[k].Suits, order)
		slices.SortStableFunc(c.Tunes[k].Tricky, order)
	}
	c.touch(by, now)
	return nil
}

// AddTuneOrigin appends a country (parentID "") or a region of a country.
func (c *TuneCollection) AddTuneOrigin(id, parentID string, label Bi, by string, now time.Time) error {
	var p problems
	l := cleanTuneLabel(&p, label)
	if err := p.err(); err != nil {
		return err
	}
	if c.taken(id) {
		return fmt.Errorf("%w: id %s is taken", ErrConflict, id)
	}
	if parentID == "" {
		if len(c.Origins) >= MaxTuneCountries {
			return Problem("label.pl", fmt.Sprintf("lista może mieć najwyżej %d krajów", MaxTuneCountries), fmt.Sprintf("the list can hold at most %d countries", MaxTuneCountries))
		}
		if slices.ContainsFunc(c.Origins, func(co TuneCountry) bool { return sameBi(co.Label, l) }) {
			return duplicateLabel()
		}
		c.Origins = append(c.Origins, TuneCountry{ID: id, Label: l, Rev: 1})
		c.touch(by, now)
		return nil
	}
	ci, ri := c.origin(parentID)
	if ci < 0 || ri >= 0 {
		return Problem("parentId", "wybierz kraj z listy", "choose a country from the list")
	}
	co := &c.Origins[ci]
	if len(co.Regions) >= MaxTuneRegions {
		return Problem("label.pl", fmt.Sprintf("kraj może mieć najwyżej %d regionów", MaxTuneRegions), fmt.Sprintf("a country can have at most %d regions", MaxTuneRegions))
	}
	if slices.ContainsFunc(co.Regions, func(r TuneRegion) bool { return sameBi(r.Label, l) }) {
		return duplicateLabel()
	}
	co.Regions = append(co.Regions, TuneRegion{ID: id, Label: l, Rev: 1})
	c.touch(by, now)
	return nil
}

// RenameTuneOrigin changes a country's or region's label at rev.
func (c *TuneCollection) RenameTuneOrigin(id string, rev int, label Bi, by string, now time.Time) error {
	ci, ri := c.origin(id)
	if ci < 0 {
		return notFound("origin")
	}
	var p problems
	l := cleanTuneLabel(&p, label)
	if err := p.err(); err != nil {
		return err
	}
	if ri < 0 {
		if c.Origins[ci].Rev != rev {
			return itemConflict("origin")
		}
		for j, co := range c.Origins {
			if j != ci && sameBi(co.Label, l) {
				return duplicateLabel()
			}
		}
		c.Origins[ci].Label = l
		c.Origins[ci].Rev++
	} else {
		r := &c.Origins[ci].Regions[ri]
		if r.Rev != rev {
			return itemConflict("origin")
		}
		for j, o := range c.Origins[ci].Regions {
			if j != ri && sameBi(o.Label, l) {
				return duplicateLabel()
			}
		}
		r.Label = l
		r.Rev++
	}
	c.touch(by, now)
	return nil
}

// OriginLabel is a country's or region's label ("Region, Country").
func (c *TuneCollection) OriginLabel(id string) Bi {
	ci, ri := c.origin(id)
	if ci < 0 {
		return Bi{}
	}
	if ri < 0 {
		return c.Origins[ci].Label
	}
	return c.Origins[ci].Regions[ri].Label
}

// RemoveTuneOrigin removes a country without regions or a region, if no
// Tune has it as its Origin.
func (c *TuneCollection) RemoveTuneOrigin(id string, by string, now time.Time) error {
	ci, ri := c.origin(id)
	if ci < 0 {
		return notFound("origin")
	}
	if ri < 0 && len(c.Origins[ci].Regions) > 0 {
		return fmt.Errorf("%w: %w", ErrInvalidState, Problem("label.pl",
			"nie można usunąć kraju, który ma regiony – najpierw usuń regiony", "cannot remove a country that has regions – remove its regions first"))
	}
	if n := c.OriginUse(id); n > 0 {
		return tuneInUse(n)
	}
	if ri < 0 {
		c.Origins = slices.Delete(c.Origins, ci, ci+1)
	} else {
		c.Origins[ci].Regions = slices.Delete(c.Origins[ci].Regions, ri, ri+1)
	}
	c.retire(id)
	c.touch(by, now)
	return nil
}

// MoveTuneOrigin moves a country among the countries or a region within
// its country.
func (c *TuneCollection) MoveTuneOrigin(id string, to int, by string, now time.Time) error {
	ci, ri := c.origin(id)
	if ci < 0 {
		return notFound("origin")
	}
	if ri < 0 {
		c.Origins = move(c.Origins, ci, to)
	} else {
		c.Origins[ci].Regions = move(c.Origins[ci].Regions, ri, to)
	}
	c.touch(by, now)
	return nil
}

// --- Import -------------------------------------------------------------------

// ImportCandidate is one tune of an ABC file as the import proposes it.
type ImportCandidate struct {
	Index      int        `json:"index"`
	Name       TuneName   `json:"name"`
	OtherNames []TuneName `json:"otherNames"`
	// TypeID is the Tune Type whose label matches R: ("" when none does).
	TypeID string `json:"typeId,omitempty"`
	Rhythm string `json:"rhythm,omitempty"`
	Key    *Key   `json:"key,omitempty"`
	Meter  string `json:"meter,omitempty"`
	// OriginID matches O: by label; Origin is the O: text either way.
	OriginID string `json:"originId,omitempty"`
	Origin   string `json:"origin,omitempty"`
	Composer string `json:"composer,omitempty"`
	ABC      string `json:"abc"`
	// Duplicate names the Tunes of the collection (or earlier candidates)
	// with the same name, ignoring case and diacritics.
	Duplicate []string `json:"duplicate,omitempty"`
	// Problems lists what the Moderator must still decide or fix.
	Problems []FieldError `json:"problems,omitempty"`
}

// MaxImportBytes bounds an imported ABC file.
const MaxImportBytes = 1 << 20

func (c *TuneCollection) matchType(r string) string {
	f := Fold(strings.TrimSpace(r))
	if f == "" {
		return ""
	}
	for _, t := range c.Types {
		if Fold(t.Label.PL) == f || Fold(t.Label.EN) == f {
			return t.ID
		}
	}
	return ""
}

func (c *TuneCollection) matchOrigin(o string) string {
	f := Fold(strings.TrimSpace(o))
	if f == "" {
		return ""
	}
	for _, co := range c.Origins {
		for _, r := range co.Regions {
			if Fold(r.Label.PL) == f || Fold(r.Label.EN) == f {
				return r.ID
			}
		}
		if Fold(co.Label.PL) == f || Fold(co.Label.EN) == f {
			return co.ID
		}
	}
	return ""
}

// ImportPreview splits an ABC file into tunes and proposes each one's
// fields; nothing is saved.
func (c *TuneCollection) ImportPreview(text string) ([]ImportCandidate, error) {
	if len(text) > MaxImportBytes {
		return nil, Problem("abc", "plik ABC jest za duży (maks. 1 MB)", "the ABC file is too large (max. 1 MB)")
	}
	text = strings.ToValidUTF8(text, "")
	tunes := SplitABCTunes(text)
	if len(tunes) == 0 {
		return nil, Problem("abc", "nie znaleziono melodii (każda zaczyna się od X:)", "no tunes found (each starts with X:)")
	}
	if len(tunes) > MaxTunes {
		return nil, Problem("abc", fmt.Sprintf("najwyżej %d melodii naraz", MaxTunes), fmt.Sprintf("at most %d tunes at once", MaxTunes))
	}
	names := map[string][]string{}
	for _, t := range c.Tunes {
		for _, n := range append([]TuneName{t.Name}, t.OtherNames...) {
			names[Fold(n.Name)] = append(names[Fold(n.Name)], t.Name.Name)
		}
	}
	out := []ImportCandidate{}
	for i, abc := range tunes {
		h := ParseABCHeader(abc)
		ic := ImportCandidate{Index: i, OtherNames: []TuneName{}, ABC: abc, Rhythm: h.Rhythm, Origin: h.Origin, Composer: oneLine(h.Composer, MaxComposerLen)}
		if len(h.Titles) > 0 {
			ic.Name = TuneName{Name: oneLine(h.Titles[0], MaxTuneNameLen)}
			for _, o := range h.Titles[1:min(len(h.Titles), MaxOtherNames+1)] {
				ic.OtherNames = append(ic.OtherNames, TuneName{Name: oneLine(o, MaxTuneNameLen)})
			}
		}
		ic.TypeID = c.matchType(h.Rhythm)
		ic.OriginID = c.matchOrigin(h.Origin)
		if k, ok := ParseABCKey(h.Key); ok {
			ic.Key = &k
		}
		if m, ok := ParseABCMeter(h.Meter); ok {
			ic.Meter = m
		} else if tt, ok := c.TuneType(ic.TypeID); ok {
			ic.Meter = tt.Meter
		}
		var p problems
		if ic.Name.Name == "" {
			p.add("name.name", "brak tytułu (T:)", "no title (T:)")
		}
		if ic.TypeID == "" {
			p.add("typeId", "wybierz rodzaj melodii", "choose a tune type")
		}
		if ic.Key == nil {
			p.add("key", "brak tonacji (K:)", "no key (K:)")
		}
		if ic.Meter == "" {
			p.add("meter", "brak metrum (M:)", "no time signature (M:)")
		}
		if len(abc) > MaxABCBytes {
			p.add("abc", "zapis ABC jest za długi (maks. %d KB)", "the ABC is too long (max. %d KB)", MaxABCBytes>>10)
		}
		if e := p.err(); e != nil {
			ic.Problems = e.(*ValidationError).Problems
		}
		key := Fold(ic.Name.Name)
		if key != "" {
			ic.Duplicate = slices.Compact(slices.Sorted(slices.Values(names[key])))
			names[key] = append(names[key], ic.Name.Name)
		}
		out = append(out, ic)
	}
	return out, nil
}
