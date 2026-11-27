package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"time"
)

// TuneSnapshotSchemaVersion is the version of contract/tunes.schema.json
// (the catalogue) and contract/tune.schema.json (one Tune's file).
const TuneSnapshotSchemaVersion = 1

// Tune snapshot types mirror contract/tunes.schema.json and contract/tune.schema.json.
type (
	SnapTuneType struct {
		ID        string `json:"id"`
		Label     Bi     `json:"label"`
		Meter     string `json:"meter"`
		Tempo     int    `json:"tempo"`
		TempoBeat string `json:"tempoBeat"`
	}
	SnapBiLabeled struct {
		ID    string `json:"id"`
		Label Bi     `json:"label"`
	}
	SnapTuneCountry struct {
		ID      string          `json:"id"`
		Label   Bi              `json:"label"`
		Regions []SnapBiLabeled `json:"regions"`
	}
	SnapTuneName struct {
		Name        string `json:"name"`
		Translation *Bi    `json:"translation,omitempty"`
	}
	// SnapRecording is a Recording: kind "video" plays on the Site, "link"
	// is a page elsewhere (Host names it).
	SnapRecording struct {
		Kind      string `json:"kind"`
		URL       string `json:"url"`
		Provider  string `json:"provider,omitempty"`
		EmbedURL  string `json:"embedUrl,omitempty"`
		Thumbnail string `json:"thumbnail,omitempty"`
		Start     int    `json:"start,omitempty"`
		Host      string `json:"host,omitempty"`
		Label     string `json:"label,omitempty"`
	}
	// SnapTuneBase holds what the catalogue and a Tune's file both carry.
	SnapTuneBase struct {
		ID         string          `json:"id"`
		Slug       string          `json:"slug"`
		Permalink  map[Lang]string `json:"permalink"`
		Name       SnapTuneName    `json:"name"`
		OtherNames []SnapTuneName  `json:"otherNames"`
		TypeID     string          `json:"typeId"`
		Key        Key             `json:"key"`
		Meter      string          `json:"meter"`
		Tempo      int             `json:"tempo"`
		TempoBeat  string          `json:"tempoBeat"`
		OriginID   string          `json:"originId,omitempty"`
		Composer   string          `json:"composer,omitempty"`
		Suits      []string        `json:"suits"`
		Tricky     []string        `json:"tricky"`
		// Incipit is the opening bars as ABC (none without a Transcription).
		Incipit string `json:"incipit,omitempty"`
	}
	SnapCatalogueTune struct {
		SnapTuneBase
		HasTranscription bool           `json:"hasTranscription"`
		FirstRecording   *SnapRecording `json:"firstRecording,omitempty"`
		// Rev is a content hash of the Tune's own file.
		Rev string `json:"rev"`
	}
	SnapSet struct {
		ID        string          `json:"id"`
		Slug      string          `json:"slug"`
		Permalink map[Lang]string `json:"permalink"`
		Name      *Bi             `json:"name,omitempty"`
		TuneIDs   []string        `json:"tuneIds"`
	}
	// TunesSnapshot is a Site's tunes.json: the catalogue.
	TunesSnapshot struct {
		SchemaVersion int                 `json:"schemaVersion"`
		Organization  SnapshotOrg         `json:"organization"`
		Types         []SnapTuneType      `json:"types"`
		Instruments   []SnapBiLabeled     `json:"instruments"`
		Origins       []SnapTuneCountry   `json:"origins"`
		Tunes         []SnapCatalogueTune `json:"tunes"`
		Sets          []SnapSet           `json:"sets"`
		UpdatedAt     string              `json:"updatedAt,omitempty"`
	}
	SnapAttachmentVariant struct {
		Width  int    `json:"width"`
		Height int    `json:"height"`
		URL    string `json:"url"`
	}
	SnapAttachment struct {
		ID       string                  `json:"id"`
		Kind     string                  `json:"kind"`
		Name     string                  `json:"name"`
		Size     int                     `json:"size"`
		Caption  *Bi                     `json:"caption,omitempty"`
		URL      string                  `json:"url,omitempty"`
		Width    int                     `json:"width,omitempty"`
		Height   int                     `json:"height,omitempty"`
		Variants []SnapAttachmentVariant `json:"variants,omitempty"`
	}
	SnapTranscription struct {
		Format string `json:"format"`
		Text   string `json:"text"`
	}
	// TuneFileSnapshot is a Site's tunes/<id>.json: one Tune in full.
	TuneFileSnapshot struct {
		SchemaVersion int `json:"schemaVersion"`
		SnapTuneBase
		Transcription   *SnapTranscription `json:"transcription,omitempty"`
		DescriptionHTML *Bi                `json:"descriptionHtml,omitempty"`
		Recordings      []SnapRecording    `json:"recordings"`
		Attachments     []SnapAttachment   `json:"attachments"`
		Sets            []SnapSet          `json:"sets"`
	}
)

func optBi(b Bi) *Bi {
	if b.IsZero() {
		return nil
	}
	return &b
}

func snapName(n TuneName) SnapTuneName {
	return SnapTuneName{Name: n.Name, Translation: optBi(n.Translation)}
}

func snapRecording(r Recording) SnapRecording {
	if r.Video != nil {
		return SnapRecording{Kind: "video", URL: r.Video.WatchURL(), Provider: r.Video.Provider, EmbedURL: r.Video.EmbedURL(),
			Thumbnail: r.ThumbnailURL(), Start: r.Video.StartSeconds, Label: r.Label}
	}
	host := ""
	if u, err := url.Parse(r.URL); err == nil {
		host = strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	}
	return SnapRecording{Kind: "link", URL: r.URL, Host: host, Label: r.Label}
}

// recordingShown reports whether a Recording can be published (a Vimeo
// video saved without a thumbnail cannot be shown).
func recordingShown(r Recording) bool {
	if r.Video == nil {
		return IsHTTPSURL(r.URL)
	}
	return IsHTTPSURL(r.ThumbnailURL())
}

// PublishedTunes is what a Site shows of a Tune Collection: the Tunes not
// Hidden, by name, and the Sets not Hidden that keep at least two of them.
type PublishedTunes struct {
	Tunes []*Tune
	Sets  []*TuneSet
	// SetTunes are each published Set's visible Tunes, in order.
	SetTunes map[string][]string
}

// Published selects what the Site shows.
func (c *TuneCollection) Published() PublishedTunes {
	p := PublishedTunes{SetTunes: map[string][]string{}}
	visible := map[string]bool{}
	for i := range c.Tunes {
		if !c.Tunes[i].Hidden {
			p.Tunes = append(p.Tunes, &c.Tunes[i])
			visible[c.Tunes[i].ID] = true
		}
	}
	slices.SortStableFunc(p.Tunes, func(a, b *Tune) int {
		if x, y := SortKey(a.Name.Name), SortKey(b.Name.Name); x != y {
			return strings.Compare(x, y)
		}
		return strings.Compare(a.ID, b.ID)
	})
	for i := range c.Sets {
		s := &c.Sets[i]
		if s.Hidden {
			continue
		}
		var ids []string
		for _, id := range s.TuneIDs {
			if visible[id] {
				ids = append(ids, id)
			}
		}
		if len(ids) < MinSetTunes {
			continue
		}
		p.Sets = append(p.Sets, s)
		p.SetTunes[s.ID] = ids
	}
	return p
}

func (c *TuneCollection) snapSet(org *Organization, s *TuneSet, ids []string) SnapSet {
	var names []string
	for _, id := range ids {
		if t, ok := c.Tune(id); ok {
			names = append(names, t.Name.Name)
		}
	}
	title := s.Name.In(PL)
	if title == "" {
		title = strings.Join(names, " / ")
	}
	slug := slugOr(title, "set")
	return SnapSet{ID: s.ID, Slug: slug, Name: optBi(s.Name), TuneIDs: append([]string{}, ids...),
		Permalink: map[Lang]string{PL: org.SetPermalink(s.ID, slug, PL), EN: org.SetPermalink(s.ID, slug, EN)}}
}

func (c *TuneCollection) snapBase(org *Organization, t *Tune) SnapTuneBase {
	slug := t.Slug()
	b := SnapTuneBase{
		ID: t.ID, Slug: slug, Name: snapName(t.Name), OtherNames: []SnapTuneName{},
		Permalink: map[Lang]string{PL: org.TunePermalink(t.ID, slug, PL), EN: org.TunePermalink(t.ID, slug, EN)},
		TypeID:    t.TypeID, Key: t.Key, Meter: t.Meter, Tempo: c.TempoOf(t), TempoBeat: TempoBeat(t.Meter),
		OriginID: t.OriginID, Composer: t.Composer,
		Suits: append([]string{}, t.Suits...), Tricky: append([]string{}, t.Tricky...),
	}
	for _, n := range t.OtherNames {
		b.OtherNames = append(b.OtherNames, snapName(n))
	}
	if abc := t.ABC(); abc != "" {
		if inc, ok := Incipit(abc, t.IncipitBars); ok {
			b.Incipit = inc
		}
	}
	return b
}

// TuneFile is one published Tune's file, with its content hash.
type TuneFile struct {
	ID   string
	Data []byte
	Rev  string
}

// BuildTuneSnapshots builds a Site's catalogue and the file of every
// published Tune. publicURL turns a public store path into an absolute URL.
func BuildTuneSnapshots(org *Organization, c *TuneCollection, publicURL func(string) string) (TunesSnapshot, []TuneFile, error) {
	s := TunesSnapshot{
		SchemaVersion: TuneSnapshotSchemaVersion,
		Organization:  SnapshotOrg{ID: org.ID, Name: org.Name, SiteID: org.Site.ID, BaseURL: org.Site.BaseURL, Timezone: org.Timezone},
		Types:         []SnapTuneType{}, Instruments: []SnapBiLabeled{}, Origins: []SnapTuneCountry{}, Tunes: []SnapCatalogueTune{}, Sets: []SnapSet{},
	}
	if !c.UpdatedAt.IsZero() {
		s.UpdatedAt = c.UpdatedAt.UTC().Format(time.RFC3339)
	}
	for _, t := range c.Types {
		s.Types = append(s.Types, SnapTuneType{ID: t.ID, Label: t.Label, Meter: t.Meter, Tempo: t.Tempo, TempoBeat: TempoBeat(t.Meter)})
	}
	for _, in := range c.Instruments {
		s.Instruments = append(s.Instruments, SnapBiLabeled{ID: in.ID, Label: in.Label})
	}
	for _, co := range c.Origins {
		sc := SnapTuneCountry{ID: co.ID, Label: co.Label, Regions: []SnapBiLabeled{}}
		for _, r := range co.Regions {
			sc.Regions = append(sc.Regions, SnapBiLabeled{ID: r.ID, Label: r.Label})
		}
		s.Origins = append(s.Origins, sc)
	}
	pub := c.Published()
	sets := map[string]SnapSet{}
	for _, st := range pub.Sets {
		ss := c.snapSet(org, st, pub.SetTunes[st.ID])
		sets[st.ID] = ss
		s.Sets = append(s.Sets, ss)
	}
	var files []TuneFile
	for _, t := range pub.Tunes {
		base := c.snapBase(org, t)
		f := TuneFileSnapshot{SchemaVersion: TuneSnapshotSchemaVersion, SnapTuneBase: base, Recordings: []SnapRecording{}, Attachments: []SnapAttachment{}, Sets: []SnapSet{}}
		if abc := t.ABC(); abc != "" {
			f.Transcription = &SnapTranscription{Format: TranscriptionABC, Text: abc}
		}
		if !t.Description.IsZero() {
			f.DescriptionHTML = &Bi{PL: RenderMarkdown(t.Description.PL), EN: RenderMarkdown(t.Description.EN)}
		}
		for _, r := range t.Recordings {
			if recordingShown(r) {
				f.Recordings = append(f.Recordings, snapRecording(r))
			}
		}
		for _, a := range t.Attachments {
			sa := SnapAttachment{ID: a.ID, Kind: a.Kind, Name: a.Name, Size: a.Size, Caption: optBi(a.Caption)}
			if a.Kind == AttachmentPDF {
				sa.URL = publicURL(a.Path)
			} else {
				sa.Width, sa.Height = a.Width, a.Height
				for _, v := range a.Variants {
					sa.Variants = append(sa.Variants, SnapAttachmentVariant{Width: v.Width, Height: v.Height, URL: publicURL(v.Path)})
				}
			}
			f.Attachments = append(f.Attachments, sa)
		}
		for _, st := range pub.Sets {
			if slices.Contains(pub.SetTunes[st.ID], t.ID) {
				f.Sets = append(f.Sets, sets[st.ID])
			}
		}
		data, err := json.Marshal(f)
		if err != nil {
			return TunesSnapshot{}, nil, err
		}
		sum := sha256.Sum256(data)
		rev := hex.EncodeToString(sum[:8])
		files = append(files, TuneFile{ID: t.ID, Data: data, Rev: rev})
		ct := SnapCatalogueTune{SnapTuneBase: base, HasTranscription: f.Transcription != nil, Rev: rev}
		if len(f.Recordings) > 0 {
			r := f.Recordings[0]
			ct.FirstRecording = &r
		}
		s.Tunes = append(s.Tunes, ct)
	}
	return s, files, nil
}
