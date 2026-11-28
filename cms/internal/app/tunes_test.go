package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/filip-be/folk-ms/cms/internal/adapters/memory"
	"github.com/filip-be/folk-ms/cms/internal/app"
	"github.com/filip-be/folk-ms/cms/internal/domain"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// optIn gives org-a a Tune Collection.
func (e *env) optIn(t *testing.T) {
	t.Helper()
	o, _ := e.a.Org("org-a")
	o.Site.Tunes = &domain.SiteTunes{TunePermalinkTemplate: "/{lang}/tunes/{shortId}-{slug}", SetPermalinkTemplate: "/{lang}/sets/{shortId}-{slug}"}
}

func (e *env) tuneMods(t *testing.T) (modA, modB domain.Principal) {
	t.Helper()
	sa := must[domain.Principal](t)(e.signIn(t, "sa", "admin@example.org"))
	must[*domain.Invitation](t)(e.a.CreateInvitation(e.ctx, sa, "a@example.org", []string{"org-a"}))
	must[*domain.Invitation](t)(e.a.CreateInvitation(e.ctx, sa, "b@example.org", []string{"org-b"}))
	return must[domain.Principal](t)(e.signIn(t, "a", "a@example.org")), must[domain.Principal](t)(e.signIn(t, "b", "b@example.org"))
}

func typeByEN(v app.TuneCollectionView, en string) app.TuneTypeView {
	for _, t := range v.Types {
		if t.Label.EN == en {
			return t
		}
	}
	panic(en)
}

func tuneIn(v app.TuneCollectionView, name string) domain.TuneInput {
	return domain.TuneInput{Name: domain.TuneName{Name: name}, TypeID: typeByEN(v, "reel").ID, Key: domain.Key{Tonic: "D", Mode: "major"}, Meter: "4/4"}
}

func (e *env) catalogue(t *testing.T) domain.TunesSnapshot {
	t.Helper()
	d := must[ports.Document](t)(e.pub.Get(e.ctx, app.TunesPath("org-a")))
	var s domain.TunesSnapshot
	if err := json.Unmarshal(d.Data, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTuneCollectionAccessAndOptIn(t *testing.T) {
	e := newEnv(t)
	modA, modB := e.tuneMods(t)
	// Without the opt-in every use case answers not found.
	if _, err := e.a.GetTunes(e.ctx, modA, "org-a"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("no opt-in: %v", err)
	}
	if _, err := e.a.PreviewMarkdown(e.ctx, modA, "org-a", "x"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("no opt-in preview: %v", err)
	}
	if m := e.a.Me(modA); len(m.Orgs) != 1 || m.Orgs[0].Tunes {
		t.Fatalf("me without opt-in: %+v", m.Orgs)
	}
	must[struct{}](t)(struct{}{}, e.a.RebuildAll(e.ctx))
	if _, err := e.pub.Get(e.ctx, app.TunesPath("org-a")); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("snapshot without opt-in: %v", err)
	}
	e.optIn(t)
	if m := e.a.Me(modA); !m.Orgs[0].Tunes {
		t.Fatalf("me with opt-in: %+v", m.Orgs)
	}
	v := must[app.TuneCollectionView](t)(e.a.GetTunes(e.ctx, modA, "org-a"))
	if len(v.Types) != 12 || len(v.Instruments) != 13 || len(v.Tunes) != 0 || len(v.Modes) != 7 {
		t.Fatalf("seeded: %+v", v)
	}
	if _, err := e.a.GetTunes(e.ctx, modB, "org-a"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("other moderator: %v", err)
	}
	if _, err := e.a.CreateTune(e.ctx, modB, "org-a", tuneIn(v, "x")); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("other moderator writes: %v", err)
	}
	if _, err := e.a.GetTunes(e.ctx, modB, "org-b"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("org-b has no opt-in: %v", err)
	}
	// Reading never creates the document; the first change does, seeded.
	if _, err := e.a.Docs.Get(e.ctx, "orgs/org-a/tunes.json"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("read created the document: %v", err)
	}
	must[app.TuneCollectionView](t)(e.a.AddInstrument(e.ctx, modA, "org-a", domain.Bi{EN: "Low whistle"}))
	v2 := must[app.TuneCollectionView](t)(e.a.GetTunes(e.ctx, modA, "org-a"))
	if len(v2.Instruments) != 14 || v2.Types[0].ID != v.Types[0].ID {
		t.Fatalf("first change: %+v", v2.Instruments)
	}
	if s := e.catalogue(t); s.SchemaVersion != 1 || len(s.Instruments) != 14 || len(s.Tunes) != 0 {
		t.Fatalf("catalogue: %+v", s)
	}
}

// interferingDocs runs before once, just before the next Put of key: a
// concurrent change saved while a use case was working.
type interferingDocs struct {
	ports.DocumentStore
	key    string
	before func()
}

func (d *interferingDocs) Put(ctx context.Context, key string, data []byte, etag string) (string, error) {
	if key == d.key && d.before != nil {
		f := d.before
		d.before = nil
		f()
	}
	return d.DocumentStore.Put(ctx, key, data, etag)
}

func TestTuneRevisionsAndConcurrentChanges(t *testing.T) {
	e := newEnv(t)
	e.optIn(t)
	modA, _ := e.tuneMods(t)
	v := must[app.TuneCollectionView](t)(e.a.GetTunes(e.ctx, modA, "org-a"))
	spear := must[app.TuneView](t)(e.a.CreateTune(e.ctx, modA, "org-a", tuneIn(v, "Silver Spear")))
	maggie := must[app.TuneView](t)(e.a.CreateTune(e.ctx, modA, "org-a", tuneIn(v, "Drowsy Maggie")))
	if spear.Rev != 1 || !domain.IsShortID(spear.ID) || spear.IncipitBars != 2 || spear.Permalink[domain.PL] == "" {
		t.Fatalf("created: %+v", spear)
	}

	// While one Moderator edits Silver Spear, another changes Drowsy Maggie
	// and a list: the save applies to the latest document.
	docs := &interferingDocs{DocumentStore: e.a.Docs, key: "orgs/org-a/tunes.json"}
	e.a.Docs = docs
	docs.before = func() {
		must[app.TuneView](t)(e.a.UpdateTune(e.ctx, modA, "org-a", maggie.ID, 1, tuneIn(v, "The Drowsy Maggie")))
	}
	in := tuneIn(v, "The Silver Spear")
	in.Composer = "trad."
	got := must[app.TuneView](t)(e.a.UpdateTune(e.ctx, modA, "org-a", spear.ID, 1, in))
	if got.Rev != 2 || got.Name.Name != "The Silver Spear" {
		t.Fatalf("updated: %+v", got)
	}
	m := must[app.TuneView](t)(e.a.GetTune(e.ctx, modA, "org-a", maggie.ID))
	if m.Name.Name != "The Drowsy Maggie" || m.Rev != 2 {
		t.Fatalf("concurrent change lost: %+v", m)
	}
	// The same Tune changed meanwhile: refused, not retried.
	if _, err := e.a.UpdateTune(e.ctx, modA, "org-a", spear.ID, 1, in); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale: %v", err)
	}
	if _, err := e.a.UpdateTune(e.ctx, modA, "org-a", spear.ID, 0, in); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("no revision: %v", err)
	}
	// An import concurrent with an edit never conflicts either.
	docs.before = func() {
		must[app.TuneCollectionView](t)(e.a.ImportTunes(e.ctx, modA, "org-a", []domain.TuneInput{tuneIn(v, "Imported")}))
	}
	must[app.TuneView](t)(e.a.UpdateTune(e.ctx, modA, "org-a", spear.ID, 2, in))
	all := must[app.TuneCollectionView](t)(e.a.GetTunes(e.ctx, modA, "org-a"))
	if len(all.Tunes) != 3 {
		t.Fatalf("tunes: %+v", all.Tunes)
	}
	acts := e.auditActions(t)
	if !slices.Contains(acts, "tune.add") || !slices.Contains(acts, "tune.update") || !slices.Contains(acts, "tune.import") {
		t.Fatalf("audit: %v", acts)
	}
}

func TestTuneUseCases(t *testing.T) {
	e := newEnv(t)
	e.optIn(t)
	info := &memory.VideoInfo{Unavailable: map[string]bool{"404": true}}
	e.a.VideoInfo = info
	modA, _ := e.tuneMods(t)
	v := must[app.TuneCollectionView](t)(e.a.GetTunes(e.ctx, modA, "org-a"))

	// Recordings: Vimeo is asked once, and not again when the video stays.
	in := tuneIn(v, "The Kesh")
	in.TypeID, in.Key, in.Meter = typeByEN(v, "jig").ID, domain.Key{Tonic: "A", Mode: "major"}, "6/8"
	in.ABC = "X:1\nT:The Kesh\nM:6/8\nL:1/8\nK:G\n|:GAG GAB|ABA ABd|edd gdd|\n"
	in.Recordings = []domain.RecordingInput{{Link: "https://vimeo.com/123456#t=1m5s", Label: "Ensemble"}, {Link: "https://soundcloud.com/x/kesh"}}
	kesh := must[app.TuneView](t)(e.a.CreateTune(e.ctx, modA, "org-a", in))
	if info.Calls != 1 || kesh.Recordings[0].ThumbnailURL == "" || kesh.Recordings[0].Kind != "video" || kesh.Recordings[0].Start != "1:05" ||
		kesh.Recordings[1].Host != "soundcloud.com" {
		t.Fatalf("recordings: %d %+v", info.Calls, kesh.Recordings)
	}
	// The ABC proposes G major, the Moderator chose A major: a notice.
	if kesh.ABCProposal == nil || kesh.ABCProposal.Key.Tonic != "G" || !kesh.KeyDiffers || kesh.MeterDiffers ||
		kesh.Incipit != "X:1\nM:6/8\nL:1/8\nK:G\n|:GAG GAB|ABA ABd|]\n" {
		t.Fatalf("abc: %+v", kesh)
	}
	in.Recordings[0].Link = "https://vimeo.com/123456" // start changed, same video
	kesh = must[app.TuneView](t)(e.a.UpdateTune(e.ctx, modA, "org-a", kesh.ID, kesh.Rev, in))
	if info.Calls != 1 {
		t.Fatalf("vimeo asked again for the same video: %d", info.Calls)
	}
	in.Recordings[0].Link = "https://vimeo.com/404"
	if _, err := e.a.UpdateTune(e.ctx, modA, "org-a", kesh.ID, kesh.Rev, in); !slices.Equal(problemFields(err), []string{"recordings.0.link"}) {
		t.Fatalf("unknown vimeo: %v", err)
	}
	if rv := must[app.RecordingView](t)(e.a.PreviewRecordingLink(e.ctx, modA, "org-a", "https://youtu.be/dQw4w9WgXcQ?t=90")); rv.Kind != "video" || rv.Start != "1:30" || rv.ThumbnailURL == "" {
		t.Fatalf("link preview: %+v", rv)
	}
	if _, err := e.a.PreviewRecordingLink(e.ctx, modA, "org-a", "http://example.org"); !domain.IsValidation(err) {
		t.Fatalf("http link preview: %v", err)
	}

	// Previews.
	if p := must[app.MarkdownPreview](t)(e.a.PreviewMarkdown(e.ctx, modA, "org-a", "**a** <b>")); p.HTML != "<p><strong>a</strong> &lt;b&gt;</p>" {
		t.Fatalf("markdown: %q", p.HTML)
	}
	ap := must[app.ABCPreview](t)(e.a.PreviewABC(e.ctx, modA, "org-a", in.ABC, 1))
	if !ap.Valid || ap.Bars != 1 || ap.Proposal.Meter != "6/8" || ap.Incipit != "X:1\nM:6/8\nL:1/8\nK:G\n|:GAG GAB|]\n" || ap.Header.Titles[0] != "The Kesh" {
		t.Fatalf("abc preview: %+v", ap)
	}
	if _, err := e.a.PreviewABC(e.ctx, modA, "org-a", in.ABC, 9); !slices.Equal(problemFields(err), []string{"incipitBars"}) {
		t.Fatalf("bars: %v", err)
	}

	// Import: preview, then the chosen ones are created Hidden.
	ip := must[app.ImportPreviewView](t)(e.a.PreviewImport(e.ctx, modA, "org-a", "X:1\nT:The Kesh\nR:jig\nM:6/8\nK:G\nGAG|\nX:2\nT:Morrison's\nR:jig\nM:6/8\nK:Edor\nEBE|\n"))
	if len(ip.Candidates) != 2 || len(ip.Candidates[0].Duplicate) != 1 || ip.Candidates[1].TypeID != typeByEN(v, "jig").ID {
		t.Fatalf("import preview: %+v", ip)
	}
	c := ip.Candidates[1]
	bad := domain.TuneInput{Name: c.Name}
	if _, err := e.a.ImportTunes(e.ctx, modA, "org-a", []domain.TuneInput{tuneIn(v, "ok"), bad}); !slices.Equal(problemFields(err), []string{"tunes.1.key", "tunes.1.meter", "tunes.1.typeId"}) {
		t.Fatalf("import problems: %v", err)
	}
	iv := must[app.TuneCollectionView](t)(e.a.ImportTunes(e.ctx, modA, "org-a", []domain.TuneInput{{Name: c.Name, TypeID: c.TypeID, Key: *c.Key, Meter: c.Meter, ABC: c.ABC}}))
	var imported app.TuneSummary
	for _, s := range iv.Tunes {
		if s.Name.Name == "Morrison's" {
			imported = s
		}
	}
	if !imported.Hidden || !imported.HasTranscription {
		t.Fatalf("imported: %+v", imported)
	}
	if s := e.catalogue(t); len(s.Tunes) != 1 || s.Tunes[0].ID != kesh.ID {
		t.Fatalf("hidden import published: %+v", s.Tunes)
	}

	// Lists: in-use removal refused, type with its default meter.
	jig := typeByEN(iv, "jig")
	if jig.Uses != 2 {
		t.Fatalf("jig uses: %+v", jig)
	}
	if _, err := e.a.RemoveTuneType(e.ctx, modA, "org-a", jig.ID); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("type in use: %v", err)
	}
	iv = must[app.TuneCollectionView](t)(e.a.AddTuneOrigin(e.ctx, modA, "org-a", "", domain.Bi{EN: "Ireland"}))
	ie := iv.Origins[0]
	iv = must[app.TuneCollectionView](t)(e.a.AddTuneOrigin(e.ctx, modA, "org-a", ie.ID, domain.Bi{EN: "Sligo"}))
	if len(iv.Origins[0].Regions) != 1 {
		t.Fatalf("region: %+v", iv.Origins)
	}
	if _, err := e.a.RemoveTuneOrigin(e.ctx, modA, "org-a", ie.ID); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("country with regions: %v", err)
	}
	iv = must[app.TuneCollectionView](t)(e.a.RenameTuneOrigin(e.ctx, modA, "org-a", ie.ID, ie.Rev, domain.Bi{EN: "Ireland", PL: "Irlandia"}))
	if _, err := e.a.RenameTuneOrigin(e.ctx, modA, "org-a", ie.ID, ie.Rev, domain.Bi{EN: "Eire"}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale origin: %v", err)
	}
	must[app.TuneCollectionView](t)(e.a.MoveTuneType(e.ctx, modA, "org-a", jig.ID, 0))
	must[app.TuneCollectionView](t)(e.a.UpdateTuneType(e.ctx, modA, "org-a", jig.ID, jig.Rev, domain.TuneTypeInput{Label: jig.Label, Meter: "6/8", Tempo: 100}))
	acts := e.auditActions(t)
	for _, a := range []string{"tune.add", "tune.update", "tune.import", "tuneorigin.add", "tuneorigin.rename", "tunetype.move", "tunetype.update"} {
		if !slices.Contains(acts, a) {
			t.Errorf("audit lacks %s: %v", a, acts)
		}
	}
}

func problemFields(err error) []string {
	var ve *domain.ValidationError
	if !errors.As(err, &ve) {
		return nil
	}
	var out []string
	for _, p := range ve.Problems {
		out = append(out, p.Field)
	}
	return out
}

func TestTuneAttachmentsAndRemoval(t *testing.T) {
	e := newEnv(t)
	e.optIn(t)
	modA, _ := e.tuneMods(t)
	v := must[app.TuneCollectionView](t)(e.a.GetTunes(e.ctx, modA, "org-a"))
	a := must[app.TuneView](t)(e.a.CreateTune(e.ctx, modA, "org-a", tuneIn(v, "A")))
	b := must[app.TuneView](t)(e.a.CreateTune(e.ctx, modA, "org-a", tuneIn(v, "B")))
	prefix := "media/org-a/tunes/" + a.ID + "/"

	pdf := []byte("%PDF-1.4\n%fake\n")
	tv := must[app.TuneView](t)(e.a.UploadTuneAttachment(e.ctx, modA, "org-a", a.ID, "Nuty Kesh.pdf", "application/pdf", pdf))
	tv = must[app.TuneView](t)(e.a.UploadTuneAttachment(e.ctx, modA, "org-a", a.ID, "scan.png", "image/png", pngImage(1000, 500)))
	if len(tv.Attachments) != 2 {
		t.Fatalf("attachments: %+v", tv.Attachments)
	}
	p, im := tv.Attachments[0], tv.Attachments[1]
	if p.Kind != "pdf" || p.Name != "nuty-kesh.pdf" || p.Size != len(pdf) || !strings.HasPrefix(p.URL, "https://store.example/pub/"+prefix) {
		t.Fatalf("pdf: %+v", p)
	}
	if im.Kind != "image" || len(im.Variants) != 2 || im.Variants[1].Width != 960 || im.Name != "scan.jpg" {
		t.Fatalf("image: %+v", im)
	}
	paths := e.pub.Paths(prefix)
	if len(paths) != 3 {
		t.Fatalf("stored: %v", paths)
	}
	for _, path := range paths {
		if !strings.HasSuffix(path, ".pdf") && !strings.HasSuffix(path, "-480.jpg") && !strings.HasSuffix(path, "-960.jpg") {
			t.Errorf("name %s", path)
		}
	}
	if d := e.pub.Disposition(strings.TrimPrefix(p.URL, "https://store.example/pub/")); d != `inline; filename="nuty-kesh.pdf"` {
		t.Fatalf("disposition %q", d)
	}
	for name, err := range map[string]error{
		"not a pdf": func() error {
			_, err := e.a.UploadTuneAttachment(e.ctx, modA, "org-a", a.ID, "x.pdf", "application/pdf", []byte("<html>"))
			return err
		}(),
		"not an image": func() error {
			_, err := e.a.UploadTuneAttachment(e.ctx, modA, "org-a", a.ID, "x.png", "image/png", []byte("nope"))
			return err
		}(),
		"other type": func() error {
			_, err := e.a.UploadTuneAttachment(e.ctx, modA, "org-a", a.ID, "x.html", "text/html", []byte("<html>"))
			return err
		}(),
	} {
		if !slices.Equal(problemFields(err), []string{"file"}) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := e.a.UploadTuneAttachment(e.ctx, modA, "org-a", "nosuchid", "x.pdf", "application/pdf", pdf); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing tune: %v", err)
	}
	tv = must[app.TuneView](t)(e.a.SetAttachmentCaption(e.ctx, modA, "org-a", a.ID, im.ID, domain.Bi{EN: "Old scan"}))
	tv = must[app.TuneView](t)(e.a.MoveAttachment(e.ctx, modA, "org-a", a.ID, im.ID, 0))
	if tv.Attachments[0].ID != im.ID || tv.Attachments[0].Caption.EN != "Old scan" || tv.Rev != 1 {
		t.Fatalf("caption, move: %+v", tv)
	}
	// Up to ten per Tune.
	for i := range 8 {
		must[app.TuneView](t)(e.a.UploadTuneAttachment(e.ctx, modA, "org-a", a.ID, "p.pdf", "application/pdf", append(pdf, byte(i))))
	}
	if _, err := e.a.UploadTuneAttachment(e.ctx, modA, "org-a", a.ID, "p.pdf", "application/pdf", append(pdf, 'x')); !slices.Equal(problemFields(err), []string{"attachments"}) {
		t.Fatalf("eleventh: %v", err)
	}
	// Removing an Attachment deletes its files after the write.
	tv = must[app.TuneView](t)(e.a.RemoveAttachment(e.ctx, modA, "org-a", a.ID, im.ID))
	if n := len(e.pub.Paths(prefix)); n != 9 {
		t.Fatalf("after removal %d files: %v", n, e.pub.Paths(prefix))
	}

	// The published Tune file lists the Attachments.
	var f domain.TuneFileSnapshot
	_ = json.Unmarshal(must[ports.Document](t)(e.pub.Get(e.ctx, app.TunePath("org-a", a.ID))).Data, &f)
	if len(f.Attachments) != 9 {
		t.Fatalf("published attachments: %d", len(f.Attachments))
	}

	// Removing a Tune in Sets needs confirmation and takes it out of them.
	cv := must[app.TuneCollectionView](t)(e.a.AddTuneSet(e.ctx, modA, "org-a", domain.SetInput{TuneIDs: []string{a.ID, b.ID}}))
	set := cv.Sets[0]
	if !set.Published || set.Title.PL != "A / B" || set.Permalink[domain.EN] == "" {
		t.Fatalf("set: %+v", set)
	}
	tv = must[app.TuneView](t)(e.a.GetTune(e.ctx, modA, "org-a", a.ID))
	if len(tv.Sets) != 1 || tv.Sets[0].EmptiedOnRemove {
		t.Fatalf("tune's sets: %+v", tv.Sets)
	}
	if _, err := e.a.RemoveTune(e.ctx, modA, "org-a", a.ID, tv.Rev, false); !errors.Is(err, domain.ErrNeedsConfirm) {
		t.Fatalf("needs confirm: %v", err)
	}
	cv = must[app.TuneCollectionView](t)(e.a.RemoveTune(e.ctx, modA, "org-a", a.ID, tv.Rev, true))
	if len(cv.Tunes) != 1 || len(cv.Sets) != 1 || !slices.Equal(cv.Sets[0].TuneIDs, []string{b.ID}) || cv.Sets[0].Published {
		t.Fatalf("after removal: %+v", cv)
	}
	if n := len(e.pub.Paths(prefix)); n != 0 {
		t.Fatalf("tune's files left: %v", e.pub.Paths(prefix))
	}
	if _, err := e.pub.Get(e.ctx, app.TunePath("org-a", a.ID)); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("removed tune's file: %v", err)
	}
	if s := e.catalogue(t); len(s.Sets) != 0 || len(s.Tunes) != 1 {
		t.Fatalf("catalogue: %+v", s)
	}
	cv = must[app.TuneCollectionView](t)(e.a.SetTuneSetHidden(e.ctx, modA, "org-a", cv.Sets[0].ID, true))
	if !cv.Sets[0].Hidden {
		t.Fatal("set hidden")
	}
	acts := e.auditActions(t)
	for _, x := range []string{"tune.attachment.add", "tune.attachment.caption", "tune.attachment.move", "tune.attachment.remove", "tuneset.add", "tune.remove", "tuneset.hide"} {
		if !slices.Contains(acts, x) {
			t.Errorf("audit lacks %s", x)
		}
	}
}

// countingPub counts writes and deletes of the public store and can fail them.
type countingPub struct {
	*memory.PublicStore
	puts, deletes []string
	failPut       string
}

func (p *countingPub) Put(ctx context.Context, path string, data []byte, ct, cc, etag string, opts ...ports.PutOption) (string, error) {
	if path == p.failPut {
		return "", errors.New("boom")
	}
	p.puts = append(p.puts, path)
	return p.PublicStore.Put(ctx, path, data, ct, cc, etag, opts...)
}

func (p *countingPub) Delete(ctx context.Context, path string) error {
	p.deletes = append(p.deletes, path)
	return p.PublicStore.Delete(ctx, path)
}

func TestRebuildTuneSnapshot(t *testing.T) {
	e := newEnv(t)
	e.optIn(t)
	modA, _ := e.tuneMods(t)
	v := must[app.TuneCollectionView](t)(e.a.GetTunes(e.ctx, modA, "org-a"))
	a := must[app.TuneView](t)(e.a.CreateTune(e.ctx, modA, "org-a", tuneIn(v, "A")))
	b := must[app.TuneView](t)(e.a.CreateTune(e.ctx, modA, "org-a", tuneIn(v, "B")))
	cp := &countingPub{PublicStore: e.pub}
	e.a.Pub = cp
	pa, pb, cat := app.TunePath("org-a", a.ID), app.TunePath("org-a", b.ID), app.TunesPath("org-a")

	// Nothing changed: only the catalogue is written.
	if err := e.a.RebuildTuneSnapshot(e.ctx, "org-a"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cp.puts, []string{cat}) {
		t.Fatalf("unchanged rebuild wrote %v", cp.puts)
	}
	// One Tune changed: its file, then the catalogue last.
	cp.puts, cp.deletes = nil, nil
	must[app.TuneView](t)(e.a.UpdateTune(e.ctx, modA, "org-a", a.ID, 1, tuneIn(v, "A2")))
	if !slices.Equal(cp.puts, []string{pa, cat}) {
		t.Fatalf("changed rebuild wrote %v", cp.puts)
	}
	// Hidden: its file is deleted, the catalogue drops it.
	cp.puts, cp.deletes = nil, nil
	must[app.TuneView](t)(e.a.SetTuneHidden(e.ctx, modA, "org-a", b.ID, true))
	if !slices.Equal(cp.deletes, []string{pb}) || !slices.Equal(cp.puts, []string{cat}) {
		t.Fatalf("hide: puts %v deletes %v", cp.puts, cp.deletes)
	}
	// An older rebuild elsewhere rewrote the hidden Tune's file: the next
	// rebuild deletes it again.
	must[string](t)(e.pub.Put(e.ctx, pb, []byte("{}"), "application/json", "", ports.AnyETag))
	if err := e.a.RebuildTuneSnapshot(e.ctx, "org-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pub.Get(e.ctx, pb); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("stale hidden file: %v", err)
	}
	// A failed file write keeps the old catalogue (retried next time).
	must[app.TuneView](t)(e.a.SetTuneHidden(e.ctx, modA, "org-a", b.ID, false))
	before := must[ports.Document](t)(e.pub.Get(e.ctx, cat))
	cp.failPut = pa
	if _, err := e.a.UpdateTune(e.ctx, modA, "org-a", a.ID, 2, tuneIn(v, "A3")); err == nil || !strings.Contains(err.Error(), "snapshot rebuild failed") {
		t.Fatalf("rebuild with a failed write: %v", err)
	}
	if after := must[ports.Document](t)(e.pub.Get(e.ctx, cat)); after.ETag != before.ETag {
		t.Fatal("catalogue written although a file failed")
	}
	cp.failPut = ""
	if err := e.a.RebuildAll(e.ctx); err != nil {
		t.Fatal(err)
	}
	if s := e.catalogue(t); len(s.Tunes) != 2 || s.Tunes[0].Name.Name != "A3" {
		t.Fatalf("repaired: %+v", s.Tunes)
	}
	// An unreadable catalogue is treated as empty: every file is written.
	must[string](t)(e.pub.Put(e.ctx, cat, []byte("garbage"), "application/json", "", ports.AnyETag))
	cp.puts = nil
	if err := e.a.RebuildTuneSnapshot(e.ctx, "org-a"); err != nil {
		t.Fatal(err)
	}
	if len(cp.puts) != 3 || cp.puts[2] != cat {
		t.Fatalf("after garbage: %v", cp.puts)
	}
}
