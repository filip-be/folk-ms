package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/domain"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// The Tune Collection of an Organization that opted in (config/sites.json).
// One document holds everything; every Tune, Set and list entry carries its
// own revision. A change loads the latest document, applies the change if
// the item it names is still at the revision the screen loaded, and saves
// with the document's ETag, retrying on a concurrent save: a change is
// refused only when the same item changed meanwhile.

func tunesKey(org string) string { return "orgs/" + org + "/tunes.json" }

// TunesPath is where a Site's Tune catalogue lives in the public store.
func TunesPath(siteID string) string { return "sites/" + siteID + "/tunes.json" }

// TunePath is where one published Tune's file lives in the public store.
func TunePath(siteID, tuneID string) string { return "sites/" + siteID + "/tunes/" + tuneID + ".json" }

// --- Views ---------------------------------------------------------------------

// TuneTypeView is a Tune Type with the number of Tunes that use it.
type TuneTypeView struct {
	ID    string    `json:"id"`
	Label domain.Bi `json:"label"`
	Meter string    `json:"meter"`
	Tempo int       `json:"tempo"`
	Rev   int       `json:"rev"`
	Uses  int       `json:"uses"`
}

// TuneListEntryView is an Instrument or an Origin region with its use count.
type TuneListEntryView struct {
	ID    string    `json:"id"`
	Label domain.Bi `json:"label"`
	Rev   int       `json:"rev"`
	Uses  int       `json:"uses"`
}

// TuneCountryView is a country of the Origins with its regions.
type TuneCountryView struct {
	TuneListEntryView
	Regions []TuneListEntryView `json:"regions"`
}

// TuneSummary is a Tune as the list shows it (no Transcription or description).
type TuneSummary struct {
	ID               string            `json:"id"`
	Name             domain.TuneName   `json:"name"`
	OtherNames       []domain.TuneName `json:"otherNames"`
	TypeID           string            `json:"typeId"`
	Key              domain.Key        `json:"key"`
	Meter            string            `json:"meter"`
	Tempo            int               `json:"tempo"` // the Playback Tempo in effect
	OriginID         string            `json:"originId,omitempty"`
	Composer         string            `json:"composer,omitempty"`
	Suits            []string          `json:"suits"`
	Tricky           []string          `json:"tricky"`
	HasTranscription bool              `json:"hasTranscription"`
	Recordings       int               `json:"recordings"`
	Attachments      int               `json:"attachments"`
	SetIDs           []string          `json:"setIds"`
	Hidden           bool              `json:"hidden"`
	Rev              int               `json:"rev"`
	UpdatedAt        time.Time         `json:"updatedAt"`
}

// TuneSetView is a Set as the admin shows it.
type TuneSetView struct {
	ID      string    `json:"id"`
	Name    domain.Bi `json:"name"`
	Title   domain.Bi `json:"title"` // the name, else the Tunes' names
	TuneIDs []string  `json:"tuneIds"`
	Hidden  bool      `json:"hidden"`
	// Published: shown on the Site (not Hidden, at least two visible Tunes).
	Published bool                   `json:"published"`
	Permalink map[domain.Lang]string `json:"permalink,omitempty"`
	Rev       int                    `json:"rev"`
	UpdatedAt time.Time              `json:"updatedAt"`
}

// TuneCollectionView is the Tune Collection as the list screens show it.
type TuneCollectionView struct {
	Types       []TuneTypeView      `json:"types"`
	Instruments []TuneListEntryView `json:"instruments"`
	Origins     []TuneCountryView   `json:"origins"`
	Tunes       []TuneSummary       `json:"tunes"`
	Sets        []TuneSetView       `json:"sets"`
	Modes       []string            `json:"modes"`
}

// RecordingView is a Recording as the edit form shows it.
type RecordingView struct {
	// Link is what the form sends back to keep the Recording.
	Link         string `json:"link"`
	Label        string `json:"label,omitempty"`
	Kind         string `json:"kind"` // "video" or "link"
	Provider     string `json:"provider,omitempty"`
	VideoID      string `json:"videoId,omitempty"`
	Start        string `json:"start,omitempty"` // m:ss
	URL          string `json:"url"`
	EmbedURL     string `json:"embedUrl,omitempty"`
	ThumbnailURL string `json:"thumbnailUrl,omitempty"`
	Host         string `json:"host,omitempty"`
}

// AttachmentVariantView is one JPEG width of an image Attachment.
type AttachmentVariantView struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	URL    string `json:"url"`
}

// AttachmentView is an Attachment with its public URLs.
type AttachmentView struct {
	ID      string    `json:"id"`
	Kind    string    `json:"kind"`
	Name    string    `json:"name"`
	Size    int       `json:"size"`
	Caption domain.Bi `json:"caption"`
	// URL is the PDF, or the largest image width.
	URL      string                  `json:"url"`
	Width    int                     `json:"width,omitempty"`
	Height   int                     `json:"height,omitempty"`
	Variants []AttachmentVariantView `json:"variants,omitempty"`
	AddedAt  time.Time               `json:"addedAt"`
	AddedBy  string                  `json:"addedBy"`
}

// TuneSetRef is a Set a Tune is in; EmptiedOnRemove: removing the Tune
// removes the Set too (it is the Set's only Tune).
type TuneSetRef struct {
	ID              string    `json:"id"`
	Title           domain.Bi `json:"title"`
	Tunes           int       `json:"tunes"`
	Hidden          bool      `json:"hidden"`
	EmptiedOnRemove bool      `json:"emptiedOnRemove"`
}

// TuneView is one Tune in full, as the editor shows it.
type TuneView struct {
	ID         string            `json:"id"`
	Name       domain.TuneName   `json:"name"`
	OtherNames []domain.TuneName `json:"otherNames"`
	TypeID     string            `json:"typeId"`
	Key        domain.Key        `json:"key"`
	Meter      string            `json:"meter"`
	// Tempo is the Tune's own Playback Tempo (0: the Tune Type's);
	// EffectiveTempo the one the Site plays, counted in TempoBeat notes.
	Tempo          int      `json:"tempo"`
	EffectiveTempo int      `json:"effectiveTempo"`
	TempoBeat      string   `json:"tempoBeat"`
	OriginID       string   `json:"originId,omitempty"`
	Composer       string   `json:"composer,omitempty"`
	Suits          []string `json:"suits"`
	Tricky         []string `json:"tricky"`
	ABC            string   `json:"abc"`
	IncipitBars    int      `json:"incipitBars"`
	Incipit        string   `json:"incipit,omitempty"`
	// ABCProposal is what the Transcription proposes; KeyDiffers and
	// MeterDiffers ask for the admin's non-blocking notice.
	ABCProposal  *domain.ABCProposal    `json:"abcProposal,omitempty"`
	KeyDiffers   bool                   `json:"keyDiffers"`
	MeterDiffers bool                   `json:"meterDiffers"`
	Recordings   []RecordingView        `json:"recordings"`
	Description  domain.Bi              `json:"description"`
	Attachments  []AttachmentView       `json:"attachments"`
	Sets         []TuneSetRef           `json:"sets"`
	Hidden       bool                   `json:"hidden"`
	Permalink    map[domain.Lang]string `json:"permalink,omitempty"`
	Rev          int                    `json:"rev"`
	CreatedAt    time.Time              `json:"createdAt"`
	CreatedBy    string                 `json:"createdBy"`
	UpdatedAt    time.Time              `json:"updatedAt"`
	UpdatedBy    string                 `json:"updatedBy"`
}

func biTitle(c *domain.TuneCollection, s *domain.TuneSet) domain.Bi {
	return domain.Bi{PL: c.SetTitle(s, domain.PL), EN: c.SetTitle(s, domain.EN)}
}

func tuneCollectionView(o *domain.Organization, c *domain.TuneCollection) TuneCollectionView {
	out := TuneCollectionView{Types: []TuneTypeView{}, Instruments: []TuneListEntryView{}, Origins: []TuneCountryView{},
		Tunes: []TuneSummary{}, Sets: []TuneSetView{}, Modes: domain.Modes}
	for _, t := range c.Types {
		out.Types = append(out.Types, TuneTypeView{ID: t.ID, Label: t.Label, Meter: t.Meter, Tempo: t.Tempo, Rev: t.Rev, Uses: c.TypeUse(t.ID)})
	}
	for _, in := range c.Instruments {
		out.Instruments = append(out.Instruments, TuneListEntryView{ID: in.ID, Label: in.Label, Rev: in.Rev, Uses: c.InstrumentUse(in.ID)})
	}
	for _, co := range c.Origins {
		cv := TuneCountryView{TuneListEntryView: TuneListEntryView{ID: co.ID, Label: co.Label, Rev: co.Rev, Uses: c.OriginUse(co.ID)}, Regions: []TuneListEntryView{}}
		for _, r := range co.Regions {
			cv.Regions = append(cv.Regions, TuneListEntryView{ID: r.ID, Label: r.Label, Rev: r.Rev, Uses: c.OriginUse(r.ID)})
		}
		out.Origins = append(out.Origins, cv)
	}
	tunes := make([]*domain.Tune, len(c.Tunes))
	for i := range c.Tunes {
		tunes[i] = &c.Tunes[i]
	}
	slices.SortStableFunc(tunes, func(a, b *domain.Tune) int {
		return strings.Compare(domain.SortKey(a.Name.Name), domain.SortKey(b.Name.Name))
	})
	for _, t := range tunes {
		s := TuneSummary{ID: t.ID, Name: t.Name, OtherNames: t.OtherNames, TypeID: t.TypeID, Key: t.Key, Meter: t.Meter, Tempo: c.TempoOf(t),
			OriginID: t.OriginID, Composer: t.Composer, Suits: t.Suits, Tricky: t.Tricky, HasTranscription: t.ABC() != "",
			Recordings: len(t.Recordings), Attachments: len(t.Attachments), SetIDs: []string{}, Hidden: t.Hidden, Rev: t.Rev, UpdatedAt: t.UpdatedAt}
		for _, st := range c.SetsWith(t.ID) {
			s.SetIDs = append(s.SetIDs, st.ID)
		}
		out.Tunes = append(out.Tunes, s)
	}
	pub := c.Published()
	for i := range c.Sets {
		s := &c.Sets[i]
		v := TuneSetView{ID: s.ID, Name: s.Name, Title: biTitle(c, s), TuneIDs: s.TuneIDs, Hidden: s.Hidden, Rev: s.Rev, UpdatedAt: s.UpdatedAt}
		if ids, ok := pub.SetTunes[s.ID]; ok {
			v.Published = true
			slug := c.SetSlug(s, ids)
			v.Permalink = map[domain.Lang]string{domain.PL: o.SetPermalink(s.ID, slug, domain.PL), domain.EN: o.SetPermalink(s.ID, slug, domain.EN)}
		}
		out.Sets = append(out.Sets, v)
	}
	return out
}

func recordingView(r domain.Recording) RecordingView {
	if r.Video != nil {
		return RecordingView{Link: r.Video.WatchURL(), Label: r.Label, Kind: "video", Provider: r.Video.Provider, VideoID: r.Video.ID,
			Start: domain.FormatStart(r.Video.StartSeconds), URL: r.Video.WatchURL(), EmbedURL: r.Video.EmbedURL(), ThumbnailURL: r.ThumbnailURL()}
	}
	host := r.URL
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	return RecordingView{Link: r.URL, Label: r.Label, Kind: "link", URL: r.URL, Host: strings.TrimPrefix(strings.ToLower(host), "www.")}
}

func (a *App) attachmentView(at domain.Attachment) AttachmentView {
	v := AttachmentView{ID: at.ID, Kind: at.Kind, Name: at.Name, Size: at.Size, Caption: at.Caption, Width: at.Width, Height: at.Height,
		AddedAt: at.AddedAt, AddedBy: at.AddedBy}
	if at.Path != "" {
		v.URL = a.Pub.URL(at.Path)
	}
	for _, x := range at.Variants {
		v.Variants = append(v.Variants, AttachmentVariantView{Width: x.Width, Height: x.Height, URL: a.Pub.URL(x.Path)})
		v.URL = a.Pub.URL(x.Path)
	}
	return v
}

func (a *App) tuneView(o *domain.Organization, c *domain.TuneCollection, t *domain.Tune) TuneView {
	v := TuneView{ID: t.ID, Name: t.Name, OtherNames: t.OtherNames, TypeID: t.TypeID, Key: t.Key, Meter: t.Meter,
		Tempo: t.Tempo, EffectiveTempo: c.TempoOf(t), TempoBeat: domain.TempoBeat(t.Meter), OriginID: t.OriginID, Composer: t.Composer,
		Suits: t.Suits, Tricky: t.Tricky, ABC: t.ABC(), IncipitBars: t.IncipitBars, Recordings: []RecordingView{}, Description: t.Description,
		Attachments: []AttachmentView{}, Sets: []TuneSetRef{}, Hidden: t.Hidden, Rev: t.Rev,
		CreatedAt: t.CreatedAt, CreatedBy: t.CreatedBy, UpdatedAt: t.UpdatedAt, UpdatedBy: t.UpdatedBy}
	if v.IncipitBars == 0 {
		v.IncipitBars = domain.DefaultIncipitBars
	}
	if v.ABC != "" {
		p := domain.ProposeFromABC(v.ABC)
		v.ABCProposal = &p
		v.KeyDiffers = p.Key != nil && *p.Key != t.Key
		v.MeterDiffers = p.Meter != "" && p.Meter != t.Meter
		v.Incipit, _ = domain.Incipit(v.ABC, v.IncipitBars)
	}
	for _, r := range t.Recordings {
		v.Recordings = append(v.Recordings, recordingView(r))
	}
	for _, at := range t.Attachments {
		v.Attachments = append(v.Attachments, a.attachmentView(at))
	}
	for _, s := range c.SetsWith(t.ID) {
		v.Sets = append(v.Sets, TuneSetRef{ID: s.ID, Title: biTitle(c, s), Tunes: len(s.TuneIDs), Hidden: s.Hidden, EmptiedOnRemove: len(s.TuneIDs) == 1})
	}
	if !t.Hidden {
		slug := t.Slug()
		v.Permalink = map[domain.Lang]string{domain.PL: o.TunePermalink(t.ID, slug, domain.PL), domain.EN: o.TunePermalink(t.ID, slug, domain.EN)}
	}
	return v
}

// --- Loading and changing ------------------------------------------------------

// authorizeTunes checks access like authorize; an Organization without the
// opt-in has no Tune Collection (not found).
func (a *App) authorizeTunes(p domain.Principal, org string) (*domain.Organization, error) {
	o, err := a.authorize(p, org)
	if err != nil {
		return nil, err
	}
	if !o.HasTunes() {
		return nil, fmt.Errorf("%w: tune collection", domain.ErrNotFound)
	}
	return o, nil
}

// loadTunes loads an Organization's Tune Collection. A collection never
// saved reads as the seeded one with etag "" (the first save creates it).
func (a *App) loadTunes(ctx context.Context, org string) (*domain.TuneCollection, string, error) {
	c, etag, err := load[domain.TuneCollection](ctx, a.Docs, tunesKey(org))
	if errors.Is(err, domain.ErrNotFound) {
		return domain.NewTuneCollection(org), "", nil
	}
	if err != nil {
		return nil, "", err
	}
	if c.OrgID != org {
		return nil, "", fmt.Errorf("%w: tune collection of %s holds %s", domain.ErrNotFound, org, c.OrgID)
	}
	c.Normalize()
	return c, etag, nil
}

// tuneChange is what a change did: the audit target and details, and the
// public files it left unreferenced (deleted once the document is saved).
type tuneChange struct {
	target  string
	details map[string]any
	orphans []string
}

// changeTunes applies fn to the latest Tune Collection and saves it with
// the document's ETag, running fn again on a concurrent save. fn checks the
// revision of the item it changes (a conflict there is final) and must be
// safe to run more than once. The change is recorded in the Audit Log, the
// files it left unreferenced are deleted and the Site's tunes are rebuilt.
func (a *App) changeTunes(ctx context.Context, p domain.Principal, org, action string,
	fn func(c *domain.TuneCollection) (tuneChange, error)) (*domain.Organization, *domain.TuneCollection, error) {
	o, err := a.authorizeTunes(p, org)
	if err != nil {
		return nil, nil, err
	}
	for range 8 {
		c, etag, err := a.loadTunes(ctx, org)
		if err != nil {
			return nil, nil, err
		}
		ch, err := fn(c)
		if err != nil {
			return nil, nil, err
		}
		if _, err := save(ctx, a.Docs, tunesKey(org), c, etag); errors.Is(err, domain.ErrConflict) {
			continue // another change was saved meanwhile: apply to it
		} else if err != nil {
			return nil, nil, err
		}
		a.audit(ctx, p, action, org, ch.target, ch.details)
		a.deleteUnreferenced(ctx, c, ch.orphans)
		if err := a.RebuildTuneSnapshot(ctx, org); err != nil {
			return nil, nil, fmt.Errorf("tune collection saved but snapshot rebuild failed: %w", err)
		}
		return o, c, nil
	}
	return nil, nil, fmt.Errorf("%w: too many concurrent changes to the tune collection", domain.ErrConflict)
}

// referencedFiles lists every public file the collection's Attachments use.
func referencedFiles(c *domain.TuneCollection) map[string]bool {
	out := map[string]bool{}
	for _, t := range c.Tunes {
		for _, at := range t.Attachments {
			for _, p := range at.Paths() {
				out[p] = true
			}
		}
	}
	return out
}

// deleteUnreferenced deletes public files no Attachment of c uses. A failed
// delete leaves an orphan, which is logged, not an error.
func (a *App) deleteUnreferenced(ctx context.Context, c *domain.TuneCollection, paths []string) {
	if len(paths) == 0 {
		return
	}
	ref := referencedFiles(c)
	for _, p := range paths {
		if ref[p] {
			continue
		}
		if err := a.Pub.Delete(ctx, p); err != nil && !errors.Is(err, ports.ErrNotFound) {
			a.Log.Warn("tune attachment file not deleted (orphan)", "path", p, "err", err)
		}
	}
}

func (a *App) tuneResult(o *domain.Organization, c *domain.TuneCollection, id string, err error) (TuneView, error) {
	if err != nil {
		return TuneView{}, err
	}
	t, ok := c.Tune(id)
	if !ok {
		return TuneView{}, fmt.Errorf("%w: tune", domain.ErrNotFound)
	}
	return a.tuneView(o, c, t), nil
}

func collectionResult(o *domain.Organization, c *domain.TuneCollection, err error) (TuneCollectionView, error) {
	if err != nil {
		return TuneCollectionView{}, err
	}
	return tuneCollectionView(o, c), nil
}

// GetTunes returns the Tune Collection's lists, Tune summaries and Sets.
func (a *App) GetTunes(ctx context.Context, p domain.Principal, org string) (TuneCollectionView, error) {
	o, err := a.authorizeTunes(p, org)
	if err != nil {
		return TuneCollectionView{}, err
	}
	c, _, err := a.loadTunes(ctx, org)
	return collectionResult(o, c, err)
}

// GetTune returns one Tune in full.
func (a *App) GetTune(ctx context.Context, p domain.Principal, org, id string) (TuneView, error) {
	o, err := a.authorizeTunes(p, org)
	if err != nil {
		return TuneView{}, err
	}
	c, _, err := a.loadTunes(ctx, org)
	return a.tuneResult(o, c, id, err)
}

// --- Tunes ---------------------------------------------------------------------

// thumbnails fills the Vimeo thumbnails of a prepared Tune's Recordings:
// kept from prev when the Recording points to the same video, else asked
// of Vimeo once per use case (cache survives the retries of changeTunes).
func (a *App) thumbnails(ctx context.Context, t *domain.Tune, prev []domain.Recording, cache map[string]string, field string) error {
	for i := range t.Recordings {
		r := &t.Recordings[i]
		if r.Video == nil || r.Video.Provider != domain.Vimeo {
			continue
		}
		key := r.Video.ID + "/" + r.Video.Hash
		if th, ok := cache[key]; ok {
			r.Thumbnail = th
			continue
		}
		for _, o := range prev {
			if o.Video != nil && o.Thumbnail != "" && o.Video.SameVideo(*r.Video) {
				r.Thumbnail = o.Thumbnail
				break
			}
		}
		if r.Thumbnail == "" {
			th, err := a.vimeoThumbnail(ctx, *r.Video)
			if err != nil {
				return prefixProblems(err, fmt.Sprintf("%srecordings.%d.", field, i))
			}
			r.Thumbnail = th
		}
		cache[key] = r.Thumbnail
	}
	return nil
}

// prefixProblems puts prefix before every field of a ValidationError.
func prefixProblems(err error, prefix string) error {
	var ve *domain.ValidationError
	if !errors.As(err, &ve) {
		return err
	}
	out := &domain.ValidationError{}
	for _, p := range ve.Problems {
		out.Problems = append(out.Problems, domain.FieldError{Field: prefix + p.Field, Message: p.Message})
	}
	return out
}

func tuneDetails(t *domain.Tune) map[string]any { return map[string]any{"name": t.Name.Name} }

// CreateTune adds a Tune (visible).
func (a *App) CreateTune(ctx context.Context, p domain.Principal, org string, in domain.TuneInput) (TuneView, error) {
	cache := map[string]string{}
	var id string
	o, c, err := a.changeTunes(ctx, p, org, "tune.add", func(c *domain.TuneCollection) (tuneChange, error) {
		t, err := c.PrepareTune(in)
		if err != nil {
			return tuneChange{}, err
		}
		if err := a.thumbnails(ctx, &t, nil, cache, ""); err != nil {
			return tuneChange{}, err
		}
		if id, err = c.NewItemID(a.Rand); err != nil {
			return tuneChange{}, err
		}
		if err := c.AddTune(t, id, false, p.ID(), a.now()); err != nil {
			return tuneChange{}, err
		}
		return tuneChange{target: id, details: tuneDetails(&t)}, nil
	})
	return a.tuneResult(o, c, id, err)
}

// UpdateTune replaces a Tune's content if it is still at rev.
func (a *App) UpdateTune(ctx context.Context, p domain.Principal, org, id string, rev int, in domain.TuneInput) (TuneView, error) {
	cache := map[string]string{}
	o, c, err := a.changeTunes(ctx, p, org, "tune.update", func(c *domain.TuneCollection) (tuneChange, error) {
		cur, ok := c.Tune(id)
		if !ok {
			return tuneChange{}, fmt.Errorf("%w: tune", domain.ErrNotFound)
		}
		if cur.Rev != rev {
			return tuneChange{}, fmt.Errorf("%w: the tune was changed meanwhile", domain.ErrConflict)
		}
		t, err := c.PrepareTune(in)
		if err != nil {
			return tuneChange{}, err
		}
		if err := a.thumbnails(ctx, &t, cur.Recordings, cache, ""); err != nil {
			return tuneChange{}, err
		}
		if err := c.UpdateTune(id, rev, t, p.ID(), a.now()); err != nil {
			return tuneChange{}, err
		}
		return tuneChange{target: id, details: tuneDetails(&t)}, nil
	})
	return a.tuneResult(o, c, id, err)
}

// SetTuneHidden switches a Tune's Hide (no revision: it changes no content).
func (a *App) SetTuneHidden(ctx context.Context, p domain.Principal, org, id string, hidden bool) (TuneView, error) {
	action := "tune.show"
	if hidden {
		action = "tune.hide"
	}
	o, c, err := a.changeTunes(ctx, p, org, action, func(c *domain.TuneCollection) (tuneChange, error) {
		if err := c.SetTuneHidden(id, hidden, p.ID(), a.now()); err != nil {
			return tuneChange{}, err
		}
		t, _ := c.Tune(id)
		return tuneChange{target: id, details: tuneDetails(t)}, nil
	})
	return a.tuneResult(o, c, id, err)
}

// RemoveTune deletes a Tune at rev; when it is in Sets, confirm must be true
// and it is taken out of them (a Set left empty is removed). Its
// Attachments' files are deleted after the write; the Audit Log keeps its name.
func (a *App) RemoveTune(ctx context.Context, p domain.Principal, org, id string, rev int, confirm bool) (TuneCollectionView, error) {
	o, c, err := a.changeTunes(ctx, p, org, "tune.remove", func(c *domain.TuneCollection) (tuneChange, error) {
		r, err := c.RemoveTune(id, rev, confirm, p.ID(), a.now())
		if err != nil {
			return tuneChange{}, err
		}
		var files []string
		for _, at := range r.Tune.Attachments {
			files = append(files, at.Paths()...)
		}
		d := tuneDetails(&r.Tune)
		if len(r.LeftSets) > 0 {
			d["leftSets"] = r.LeftSets
		}
		if len(r.RemovedSets) > 0 {
			d["removedSets"] = r.RemovedSets
		}
		return tuneChange{target: id, details: d, orphans: files}, nil
	})
	return collectionResult(o, c, err)
}

// ImportTunes creates the chosen import candidates, all Hidden, in one
// write. It changes no existing item, so it never conflicts.
func (a *App) ImportTunes(ctx context.Context, p domain.Principal, org string, in []domain.TuneInput) (TuneCollectionView, error) {
	if len(in) == 0 {
		return TuneCollectionView{}, domain.Problem("tunes", "zaznacz melodie do dodania", "tick the tunes to add")
	}
	if len(in) > domain.MaxTunes {
		return TuneCollectionView{}, domain.Problem("tunes", fmt.Sprintf("najwyżej %d melodii naraz", domain.MaxTunes), fmt.Sprintf("at most %d tunes at once", domain.MaxTunes))
	}
	cache := map[string]string{}
	o, c, err := a.changeTunes(ctx, p, org, "tune.import", func(c *domain.TuneCollection) (tuneChange, error) {
		var problems []domain.FieldError
		var prepared []domain.Tune
		for i, ti := range in {
			t, err := c.PrepareTune(ti)
			if err == nil {
				err = a.thumbnails(ctx, &t, nil, cache, "")
			}
			if err != nil {
				var ve *domain.ValidationError
				if !errors.As(prefixProblems(err, fmt.Sprintf("tunes.%d.", i)), &ve) {
					return tuneChange{}, err
				}
				problems = append(problems, ve.Problems...)
				continue
			}
			prepared = append(prepared, t)
		}
		if len(problems) > 0 {
			return tuneChange{}, &domain.ValidationError{Problems: problems}
		}
		if len(c.Tunes)+len(prepared) > domain.MaxTunes {
			return tuneChange{}, domain.Problem("tunes", fmt.Sprintf("kolekcja może mieć najwyżej %d melodii", domain.MaxTunes), fmt.Sprintf("the collection can hold at most %d tunes", domain.MaxTunes))
		}
		var ids, names []string
		for _, t := range prepared {
			id, err := c.NewItemID(a.Rand)
			if err != nil {
				return tuneChange{}, err
			}
			if err := c.AddTune(t, id, true, p.ID(), a.now()); err != nil {
				return tuneChange{}, err
			}
			ids, names = append(ids, id), append(names, t.Name.Name)
		}
		return tuneChange{details: map[string]any{"ids": ids, "names": names}}, nil
	})
	return collectionResult(o, c, err)
}

// --- Attachments -----------------------------------------------------------------

const pdfDisposition = `inline; filename="%s"`

// UploadTuneAttachment stores an image (re-encoded to JPEG widths) or a PDF
// (kept as uploaded) under the Tune's own prefix and adds it to the Tune.
func (a *App) UploadTuneAttachment(ctx context.Context, p domain.Principal, org, tuneID, fileName, contentType string, data []byte) (TuneView, error) {
	o, err := a.authorizeTunes(p, org)
	if err != nil {
		return TuneView{}, err
	}
	c, _, err := a.loadTunes(ctx, org)
	if err != nil {
		return TuneView{}, err
	}
	if err := c.CanAddAttachment(tuneID); err != nil {
		return TuneView{}, err
	}
	prefix := domain.AttachmentPrefix(org, tuneID)
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:8])
	at := domain.Attachment{}
	ct := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch {
	case ct == "application/pdf":
		if len(data) > domain.MaxPDFBytes {
			return TuneView{}, domain.Problem("file", fmt.Sprintf("plik PDF jest za duży (maks. %d MB)", domain.MaxPDFBytes>>20), fmt.Sprintf("the PDF is too large (max. %d MB)", domain.MaxPDFBytes>>20))
		}
		if !bytes.HasPrefix(data, []byte("%PDF-")) {
			return TuneView{}, domain.Problem("file", "to nie jest plik PDF", "this is not a PDF file")
		}
		at = domain.Attachment{Kind: domain.AttachmentPDF, Name: domain.SafeFileName(fileName, "pdf"), Size: len(data), Path: prefix + hash + ".pdf"}
		if _, err := a.Pub.Put(ctx, at.Path, data, "application/pdf", MediaCacheControl, "",
			ports.WithContentDisposition(fmt.Sprintf(pdfDisposition, at.Name))); err != nil && !errors.Is(err, ports.ErrPrecondition) {
			return TuneView{}, err // ErrPrecondition: the same file is stored already
		}
	case strings.HasPrefix(ct, "image/"):
		rends, err := a.Images.Process(data, ports.ImageAttachment)
		if err != nil {
			var ue interface{ Localized() (pl, en string) }
			if errors.As(err, &ue) {
				pl, en := ue.Localized()
				return TuneView{}, domain.Problem("file", pl, en)
			}
			return TuneView{}, domain.Problem("file", err.Error(), err.Error())
		}
		base := fileName // re-encoded: the name gets .jpg instead of its own extension
		if i := strings.LastIndexByte(base, '.'); i > 0 {
			base = base[:i]
		}
		at = domain.Attachment{Kind: domain.AttachmentImage, Name: domain.SafeFileName(base, "jpg")}
		disp := ports.WithContentDisposition(fmt.Sprintf(pdfDisposition, at.Name))
		for _, r := range rends {
			path := prefix + hash + "-" + r.Name + ".jpg"
			if _, err := a.Pub.Put(ctx, path, r.Data, "image/jpeg", MediaCacheControl, "", disp); err != nil && !errors.Is(err, ports.ErrPrecondition) {
				a.deleteUnreferencedLatest(ctx, org, at.Paths())
				return TuneView{}, err
			}
			at.Variants = append(at.Variants, domain.ImageVariant{Width: r.Width, Height: r.Height, Path: path})
			at.Width, at.Height, at.Size = r.Width, r.Height, len(r.Data)
		}
	default:
		return TuneView{}, domain.Problem("file", "dodaj obraz (JPEG, PNG, WebP) albo plik PDF", "add an image (JPEG, PNG, WebP) or a PDF file")
	}
	var id string
	o, c, err = a.changeTunes(ctx, p, org, "tune.attachment.add", func(c *domain.TuneCollection) (tuneChange, error) {
		var err error
		if id, err = c.NewItemID(a.Rand); err != nil {
			return tuneChange{}, err
		}
		x := at
		x.ID = id
		if err := c.AddAttachment(tuneID, x, p.ID(), a.now()); err != nil {
			return tuneChange{}, err
		}
		t, _ := c.Tune(tuneID)
		return tuneChange{target: tuneID, details: map[string]any{"name": t.Name.Name, "attachment": id, "file": at.Name, "kind": at.Kind, "bytes": len(data)}}, nil
	})
	if err != nil {
		// The Tune was removed or filled up meanwhile: its files go.
		a.deleteUnreferencedLatest(ctx, org, at.Paths())
		return TuneView{}, err
	}
	return a.tuneResult(o, c, tuneID, nil)
}

// deleteUnreferencedLatest deletes the files the latest saved collection
// does not use (cleanup after a failed upload).
func (a *App) deleteUnreferencedLatest(ctx context.Context, org string, paths []string) {
	c, _, err := a.loadTunes(ctx, org)
	if err != nil {
		a.Log.Warn("tune attachment files not cleaned up", "paths", paths, "err", err)
		return
	}
	a.deleteUnreferenced(ctx, c, paths)
}

func attachmentDetails(c *domain.TuneCollection, tuneID, id string) map[string]any {
	d := map[string]any{"attachment": id}
	if t, ok := c.Tune(tuneID); ok {
		d["name"] = t.Name.Name
	}
	return d
}

// SetAttachmentCaption changes an Attachment's caption (an image's alt text).
func (a *App) SetAttachmentCaption(ctx context.Context, p domain.Principal, org, tuneID, id string, caption domain.Bi) (TuneView, error) {
	o, c, err := a.changeTunes(ctx, p, org, "tune.attachment.caption", func(c *domain.TuneCollection) (tuneChange, error) {
		return tuneChange{target: tuneID, details: attachmentDetails(c, tuneID, id)}, c.SetAttachmentCaption(tuneID, id, caption, p.ID(), a.now())
	})
	return a.tuneResult(o, c, tuneID, err)
}

// MoveAttachment moves an Attachment within its Tune.
func (a *App) MoveAttachment(ctx context.Context, p domain.Principal, org, tuneID, id string, to int) (TuneView, error) {
	o, c, err := a.changeTunes(ctx, p, org, "tune.attachment.move", func(c *domain.TuneCollection) (tuneChange, error) {
		d := attachmentDetails(c, tuneID, id)
		d["to"] = to
		return tuneChange{target: tuneID, details: d}, c.MoveAttachment(tuneID, id, to, p.ID(), a.now())
	})
	return a.tuneResult(o, c, tuneID, err)
}

// RemoveAttachment removes an Attachment; its files are deleted after the write.
func (a *App) RemoveAttachment(ctx context.Context, p domain.Principal, org, tuneID, id string) (TuneView, error) {
	o, c, err := a.changeTunes(ctx, p, org, "tune.attachment.remove", func(c *domain.TuneCollection) (tuneChange, error) {
		d := attachmentDetails(c, tuneID, id)
		at, err := c.RemoveAttachment(tuneID, id, p.ID(), a.now())
		if err != nil {
			return tuneChange{}, err
		}
		d["file"] = at.Name
		return tuneChange{target: tuneID, details: d, orphans: at.Paths()}, nil
	})
	return a.tuneResult(o, c, tuneID, err)
}

// --- Sets ------------------------------------------------------------------------

func setDetails(c *domain.TuneCollection, id string) map[string]any {
	if s, ok := c.Set(id); ok {
		return map[string]any{"title": c.SetTitle(s, domain.PL), "tuneIds": s.TuneIDs}
	}
	return nil
}

// AddTuneSet adds a Set.
func (a *App) AddTuneSet(ctx context.Context, p domain.Principal, org string, in domain.SetInput) (TuneCollectionView, error) {
	o, c, err := a.changeTunes(ctx, p, org, "tuneset.add", func(c *domain.TuneCollection) (tuneChange, error) {
		id, err := c.NewItemID(a.Rand)
		if err != nil {
			return tuneChange{}, err
		}
		if err := c.AddSet(id, in, p.ID(), a.now()); err != nil {
			return tuneChange{}, err
		}
		return tuneChange{target: id, details: setDetails(c, id)}, nil
	})
	return collectionResult(o, c, err)
}

// UpdateTuneSet replaces a Set's name and Tunes if it is still at rev.
func (a *App) UpdateTuneSet(ctx context.Context, p domain.Principal, org, id string, rev int, in domain.SetInput) (TuneCollectionView, error) {
	o, c, err := a.changeTunes(ctx, p, org, "tuneset.update", func(c *domain.TuneCollection) (tuneChange, error) {
		if err := c.UpdateSet(id, rev, in, p.ID(), a.now()); err != nil {
			return tuneChange{}, err
		}
		return tuneChange{target: id, details: setDetails(c, id)}, nil
	})
	return collectionResult(o, c, err)
}

// RemoveTuneSet deletes a Set at rev (its Tunes stay).
func (a *App) RemoveTuneSet(ctx context.Context, p domain.Principal, org, id string, rev int) (TuneCollectionView, error) {
	o, c, err := a.changeTunes(ctx, p, org, "tuneset.remove", func(c *domain.TuneCollection) (tuneChange, error) {
		d := setDetails(c, id)
		_, err := c.RemoveSet(id, rev, p.ID(), a.now())
		return tuneChange{target: id, details: d}, err
	})
	return collectionResult(o, c, err)
}

// SetTuneSetHidden switches a Set's Hide.
func (a *App) SetTuneSetHidden(ctx context.Context, p domain.Principal, org, id string, hidden bool) (TuneCollectionView, error) {
	action := "tuneset.show"
	if hidden {
		action = "tuneset.hide"
	}
	o, c, err := a.changeTunes(ctx, p, org, action, func(c *domain.TuneCollection) (tuneChange, error) {
		if err := c.SetSetHidden(id, hidden, p.ID(), a.now()); err != nil {
			return tuneChange{}, err
		}
		return tuneChange{target: id, details: setDetails(c, id)}, nil
	})
	return collectionResult(o, c, err)
}

// --- Lists -------------------------------------------------------------------------

func labelDetails(l domain.Bi) map[string]any { return map[string]any{"label": l} }

// AddTuneType appends a Tune Type.
func (a *App) AddTuneType(ctx context.Context, p domain.Principal, org string, in domain.TuneTypeInput) (TuneCollectionView, error) {
	o, c, err := a.changeTunes(ctx, p, org, "tunetype.add", func(c *domain.TuneCollection) (tuneChange, error) {
		id, err := c.NewItemID(a.Rand)
		if err != nil {
			return tuneChange{}, err
		}
		return tuneChange{target: id, details: labelDetails(in.Label)}, c.AddTuneType(id, in, p.ID(), a.now())
	})
	return collectionResult(o, c, err)
}

// UpdateTuneType changes a Tune Type at rev.
func (a *App) UpdateTuneType(ctx context.Context, p domain.Principal, org, id string, rev int, in domain.TuneTypeInput) (TuneCollectionView, error) {
	o, c, err := a.changeTunes(ctx, p, org, "tunetype.update", func(c *domain.TuneCollection) (tuneChange, error) {
		return tuneChange{target: id, details: labelDetails(in.Label)}, c.UpdateTuneType(id, rev, in, p.ID(), a.now())
	})
	return collectionResult(o, c, err)
}

// RemoveTuneType removes a Tune Type no Tune uses.
func (a *App) RemoveTuneType(ctx context.Context, p domain.Principal, org, id string) (TuneCollectionView, error) {
	o, c, err := a.changeTunes(ctx, p, org, "tunetype.remove", func(c *domain.TuneCollection) (tuneChange, error) {
		t, err := c.RemoveTuneType(id, p.ID(), a.now())
		return tuneChange{target: id, details: labelDetails(t.Label)}, err
	})
	return collectionResult(o, c, err)
}

// MoveTuneType moves a Tune Type to an index of the list.
func (a *App) MoveTuneType(ctx context.Context, p domain.Principal, org, id string, to int) (TuneCollectionView, error) {
	o, c, err := a.changeTunes(ctx, p, org, "tunetype.move", func(c *domain.TuneCollection) (tuneChange, error) {
		return tuneChange{target: id, details: map[string]any{"to": to}}, c.MoveTuneType(id, to, p.ID(), a.now())
	})
	return collectionResult(o, c, err)
}

// AddInstrument appends an Instrument.
func (a *App) AddInstrument(ctx context.Context, p domain.Principal, org string, label domain.Bi) (TuneCollectionView, error) {
	o, c, err := a.changeTunes(ctx, p, org, "instrument.add", func(c *domain.TuneCollection) (tuneChange, error) {
		id, err := c.NewItemID(a.Rand)
		if err != nil {
			return tuneChange{}, err
		}
		return tuneChange{target: id, details: labelDetails(label)}, c.AddInstrument(id, label, p.ID(), a.now())
	})
	return collectionResult(o, c, err)
}

// UpdateInstrument renames an Instrument at rev.
func (a *App) UpdateInstrument(ctx context.Context, p domain.Principal, org, id string, rev int, label domain.Bi) (TuneCollectionView, error) {
	o, c, err := a.changeTunes(ctx, p, org, "instrument.update", func(c *domain.TuneCollection) (tuneChange, error) {
		return tuneChange{target: id, details: labelDetails(label)}, c.UpdateInstrument(id, rev, label, p.ID(), a.now())
	})
	return collectionResult(o, c, err)
}

// RemoveInstrument removes an Instrument no Tune lists.
func (a *App) RemoveInstrument(ctx context.Context, p domain.Principal, org, id string) (TuneCollectionView, error) {
	o, c, err := a.changeTunes(ctx, p, org, "instrument.remove", func(c *domain.TuneCollection) (tuneChange, error) {
		in, err := c.RemoveInstrument(id, p.ID(), a.now())
		return tuneChange{target: id, details: labelDetails(in.Label)}, err
	})
	return collectionResult(o, c, err)
}

// MoveInstrument moves an Instrument to an index of the list.
func (a *App) MoveInstrument(ctx context.Context, p domain.Principal, org, id string, to int) (TuneCollectionView, error) {
	o, c, err := a.changeTunes(ctx, p, org, "instrument.move", func(c *domain.TuneCollection) (tuneChange, error) {
		return tuneChange{target: id, details: map[string]any{"to": to}}, c.MoveInstrument(id, to, p.ID(), a.now())
	})
	return collectionResult(o, c, err)
}

// AddTuneOrigin appends a country (parentID "") or a region of a country.
func (a *App) AddTuneOrigin(ctx context.Context, p domain.Principal, org, parentID string, label domain.Bi) (TuneCollectionView, error) {
	o, c, err := a.changeTunes(ctx, p, org, "tuneorigin.add", func(c *domain.TuneCollection) (tuneChange, error) {
		id, err := c.NewItemID(a.Rand)
		if err != nil {
			return tuneChange{}, err
		}
		d := labelDetails(label)
		if parentID != "" {
			d["parentId"] = parentID
		}
		return tuneChange{target: id, details: d}, c.AddTuneOrigin(id, parentID, label, p.ID(), a.now())
	})
	return collectionResult(o, c, err)
}

// RenameTuneOrigin changes a country's or region's label at rev.
func (a *App) RenameTuneOrigin(ctx context.Context, p domain.Principal, org, id string, rev int, label domain.Bi) (TuneCollectionView, error) {
	o, c, err := a.changeTunes(ctx, p, org, "tuneorigin.rename", func(c *domain.TuneCollection) (tuneChange, error) {
		return tuneChange{target: id, details: labelDetails(label)}, c.RenameTuneOrigin(id, rev, label, p.ID(), a.now())
	})
	return collectionResult(o, c, err)
}

// RemoveTuneOrigin removes a country without regions or a region no Tune has.
func (a *App) RemoveTuneOrigin(ctx context.Context, p domain.Principal, org, id string) (TuneCollectionView, error) {
	o, c, err := a.changeTunes(ctx, p, org, "tuneorigin.remove", func(c *domain.TuneCollection) (tuneChange, error) {
		d := labelDetails(c.OriginLabel(id))
		return tuneChange{target: id, details: d}, c.RemoveTuneOrigin(id, p.ID(), a.now())
	})
	return collectionResult(o, c, err)
}

// MoveTuneOrigin moves a country among the countries or a region within its country.
func (a *App) MoveTuneOrigin(ctx context.Context, p domain.Principal, org, id string, to int) (TuneCollectionView, error) {
	o, c, err := a.changeTunes(ctx, p, org, "tuneorigin.move", func(c *domain.TuneCollection) (tuneChange, error) {
		return tuneChange{target: id, details: map[string]any{"to": to}}, c.MoveTuneOrigin(id, to, p.ID(), a.now())
	})
	return collectionResult(o, c, err)
}

// --- Previews ------------------------------------------------------------------------

// MarkdownPreview renders a description as the Site will show it.
type MarkdownPreview struct {
	HTML string `json:"html"`
}

// PreviewMarkdown renders a description with the snapshot's renderer.
func (a *App) PreviewMarkdown(ctx context.Context, p domain.Principal, org, text string) (MarkdownPreview, error) {
	if _, err := a.authorizeTunes(p, org); err != nil {
		return MarkdownPreview{}, err
	}
	if len([]rune(text)) > domain.MaxTuneDescLen {
		return MarkdownPreview{}, domain.Problem("description", fmt.Sprintf("tekst jest za długi (maks. %d znaków)", domain.MaxTuneDescLen), fmt.Sprintf("the text is too long (max. %d characters)", domain.MaxTuneDescLen))
	}
	return MarkdownPreview{HTML: domain.RenderMarkdown(text)}, nil
}

// ABCPreview is what a Transcription gives: its header, the Key and Time
// Signature it proposes and the Incipit for the bars asked.
type ABCPreview struct {
	Header   domain.ABCHeader   `json:"header"`
	Proposal domain.ABCProposal `json:"proposal"`
	Bars     int                `json:"bars"`
	Incipit  string             `json:"incipit,omitempty"`
	// Valid: the text has a K: line and notes (it can be saved).
	Valid bool `json:"valid"`
}

// PreviewABC reads a Transcription without saving it.
func (a *App) PreviewABC(ctx context.Context, p domain.Principal, org, abc string, bars int) (ABCPreview, error) {
	if _, err := a.authorizeTunes(p, org); err != nil {
		return ABCPreview{}, err
	}
	if len(abc) > domain.MaxABCBytes {
		return ABCPreview{}, domain.Problem("abc", fmt.Sprintf("zapis ABC jest za długi (maks. %d KB)", domain.MaxABCBytes>>10), fmt.Sprintf("the ABC is too long (max. %d KB)", domain.MaxABCBytes>>10))
	}
	if bars < 0 || bars > domain.MaxIncipitBars {
		return ABCPreview{}, domain.Problem("incipitBars", fmt.Sprintf("od 1 do %d taktów", domain.MaxIncipitBars), fmt.Sprintf("from 1 to %d bars", domain.MaxIncipitBars))
	}
	if bars == 0 {
		bars = domain.DefaultIncipitBars
	}
	out := ABCPreview{Header: domain.ParseABCHeader(abc), Proposal: domain.ProposeFromABC(abc), Bars: bars}
	if out.Header.Titles == nil {
		out.Header.Titles = []string{}
	}
	out.Incipit, out.Valid = domain.Incipit(abc, bars)
	return out, nil
}

// ImportPreviewView lists the tunes of an ABC file as the import proposes them.
type ImportPreviewView struct {
	Candidates []domain.ImportCandidate `json:"candidates"`
}

// PreviewImport splits an ABC file (at most 1 MB) and proposes each tune's
// fields; nothing is saved.
func (a *App) PreviewImport(ctx context.Context, p domain.Principal, org, text string) (ImportPreviewView, error) {
	if _, err := a.authorizeTunes(p, org); err != nil {
		return ImportPreviewView{}, err
	}
	c, _, err := a.loadTunes(ctx, org)
	if err != nil {
		return ImportPreviewView{}, err
	}
	cs, err := c.ImportPreview(text)
	return ImportPreviewView{Candidates: cs}, err
}

// PreviewRecordingLink parses a Recording's link (and asks Vimeo for a
// Vimeo video) without saving, so the form shows what will be saved.
func (a *App) PreviewRecordingLink(ctx context.Context, p domain.Principal, org, link string) (RecordingView, error) {
	if _, err := a.authorizeTunes(p, org); err != nil {
		return RecordingView{}, err
	}
	r, err := domain.ParseRecordingLink(link)
	if err != nil {
		return RecordingView{}, err
	}
	if r.Video != nil && r.Video.Provider == domain.Vimeo {
		if r.Thumbnail, err = a.vimeoThumbnail(ctx, *r.Video); err != nil {
			return RecordingView{}, err
		}
	}
	return recordingView(r), nil
}

// --- Snapshot ------------------------------------------------------------------------

// RebuildTuneSnapshot publishes an Organization's Tune Collection: the
// per-Tune files that are new or changed, then the deletion of the files
// of Tunes no longer published (and of every Hidden or removed Tune, in
// case an older rebuild rewrote one), and the catalogue last, with ETag
// compare-and-swap. If a file cannot be written or deleted the catalogue is
// not written, so the next rebuild tries again. Organizations without the
// opt-in have no tunes snapshot.
func (a *App) RebuildTuneSnapshot(ctx context.Context, org string) error {
	o, ok := a.orgByID[org]
	if !ok {
		return fmt.Errorf("%w: organization", domain.ErrNotFound)
	}
	if !o.HasTunes() {
		return nil
	}
	path := TunesPath(o.Site.ID)
	for range 8 {
		cur, err := a.Pub.Get(ctx, path)
		etag := cur.ETag
		prev := map[string]string{} // Tune id → rev of the published catalogue
		if errors.Is(err, ports.ErrNotFound) {
			etag = ""
		} else if err != nil {
			return err
		} else {
			var old struct {
				Tunes []struct {
					ID  string `json:"id"`
					Rev string `json:"rev"`
				} `json:"tunes"`
			}
			if json.Unmarshal(cur.Data, &old) == nil {
				for _, t := range old.Tunes {
					if domain.IsShortID(t.ID) {
						prev[t.ID] = t.Rev
					}
				}
			} // unreadable: every file is written again
		}
		c, _, err := a.loadTunes(ctx, org)
		if err != nil {
			return err
		}
		snap, files, err := domain.BuildTuneSnapshots(o, c, a.Pub.URL)
		if err != nil {
			return err
		}
		var errs []error
		published := map[string]bool{}
		for _, f := range files {
			published[f.ID] = true
			if r, ok := prev[f.ID]; ok && r == f.Rev {
				continue
			}
			if _, err := a.Pub.Put(ctx, TunePath(o.Site.ID, f.ID), f.Data, "application/json", SnapshotCacheControl, ports.AnyETag); err != nil {
				errs = append(errs, err)
			}
		}
		gone := map[string]bool{}
		for id := range prev {
			gone[id] = true
		}
		for _, t := range c.Tunes {
			if t.Hidden {
				gone[t.ID] = true
			}
		}
		for _, id := range c.RemovedTunes {
			gone[id] = true
		}
		stale := make([]string, 0, len(gone))
		for id := range gone {
			if !published[id] {
				stale = append(stale, id)
			}
		}
		slices.Sort(stale)
		for _, id := range stale {
			if err := a.Pub.Delete(ctx, TunePath(o.Site.ID, id)); err != nil && !errors.Is(err, ports.ErrNotFound) {
				errs = append(errs, err)
			}
		}
		if err := errors.Join(errs...); err != nil {
			return fmt.Errorf("tune files: %w", err)
		}
		data, err := json.Marshal(snap)
		if err != nil {
			return err
		}
		_, err = a.Pub.Put(ctx, path, data, "application/json", SnapshotCacheControl, etag)
		if errors.Is(err, ports.ErrPrecondition) {
			continue
		}
		return err
	}
	return fmt.Errorf("%w: tune snapshot kept changing", domain.ErrConflict)
}

// ParseRev reads an item revision sent as If-Match (a decimal, optionally
// quoted); 0 when missing or malformed, which no item has.
func ParseRev(s string) int {
	s = strings.Trim(strings.TrimPrefix(strings.TrimSpace(s), "W/"), `"`)
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
