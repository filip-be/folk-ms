package app_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/filip-be/folk-ms/cms/internal/adapters/memory"
	"github.com/filip-be/folk-ms/cms/internal/app"
	"github.com/filip-be/folk-ms/cms/internal/domain"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

func (e *env) videoSnapshot(t *testing.T, org string) domain.VideoSnapshot {
	t.Helper()
	d := must[ports.Document](t)(e.pub.Get(e.ctx, app.VideosPath(org)))
	var s domain.VideoSnapshot
	if err := json.Unmarshal(d.Data, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func (e *env) auditActions(t *testing.T) []string {
	t.Helper()
	data := must[[]byte](t)(e.a.Audit.Read(e.ctx, e.clock.t.Format("2006-01-02")))
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var a app.AuditEntry
		_ = json.Unmarshal([]byte(line), &a)
		out = append(out, a.Action)
	}
	return out
}

func TestVideoCollection(t *testing.T) {
	e := newEnv(t)
	info := &memory.VideoInfo{Unavailable: map[string]bool{"404": true}}
	e.a.VideoInfo = info
	sa := must[domain.Principal](t)(e.signIn(t, "sa", "admin@example.org"))
	must[*domain.Invitation](t)(e.a.CreateInvitation(e.ctx, sa, "a@example.org", []string{"org-a"}))
	must[*domain.Invitation](t)(e.a.CreateInvitation(e.ctx, sa, "b@example.org", []string{"org-b"}))
	modA := must[domain.Principal](t)(e.signIn(t, "a", "a@example.org"))
	modB := must[domain.Principal](t)(e.signIn(t, "b", "b@example.org"))

	// Every Organization has a (possibly empty) collection.
	v := must[app.VideoCollectionView](t)(e.a.GetVideos(e.ctx, modA, "org-a"))
	if v.ETag != app.NewVideosETag || len(v.Videos) != 0 || v.Forms == nil {
		t.Fatalf("empty: %+v", v)
	}
	if _, err := e.a.GetVideos(e.ctx, modB, "org-a"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("other organization's moderator: %v", err)
	}
	if _, err := e.a.AddForm(e.ctx, modB, "org-a", v.ETag, domain.Text{PL: "Para"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("other organization's moderator writes: %v", err)
	}
	// Writes need the version the screen loaded.
	if _, err := e.a.AddForm(e.ctx, modA, "org-a", "", domain.Text{PL: "Para"}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("no version: %v", err)
	}
	v = must[app.VideoCollectionView](t)(e.a.AddForm(e.ctx, modA, "org-a", v.ETag, domain.Text{PL: "Para", EN: "Pair"}))
	pair := v.Forms[0].ID
	if !domain.IsShortID(pair) {
		t.Fatalf("form id %q", pair)
	}
	// A second screen still holding "new" conflicts instead of overwriting.
	if _, err := e.a.AddForm(e.ctx, modA, "org-a", app.NewVideosETag, domain.Text{PL: "Koło"}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale new: %v", err)
	}
	stale := v.ETag
	v = must[app.VideoCollectionView](t)(e.a.AddOrigin(e.ctx, modA, "org-a", v.ETag, "", domain.Text{PL: "Francja"}))
	france := v.Origins[0].ID
	v = must[app.VideoCollectionView](t)(e.a.AddOrigin(e.ctx, modA, "org-a", v.ETag, france, domain.Text{PL: "Bretania"}))
	brittany := v.Origins[0].Regions[0].ID
	if _, err := e.a.AddForm(e.ctx, modA, "org-a", stale, domain.Text{PL: "Koło"}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale version: %v", err)
	}

	// Videos: YouTube needs no lookup; Vimeo is asked for its thumbnail.
	yt := domain.VideoInput{Link: "https://youtu.be/dQw4w9WgXcQ?t=90", Title: domain.Text{PL: "An dro"}, OriginID: brittany, FormIDs: []string{pair}}
	v = must[app.VideoCollectionView](t)(e.a.AddVideo(e.ctx, modA, "org-a", v.ETag, yt))
	if info.Calls != 0 || v.Videos[0].ThumbnailURL != "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg" || v.Videos[0].Start != "1:30" ||
		v.Videos[0].LinkURL != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" || v.Forms[0].Uses != 1 || v.Origins[0].Regions[0].Uses != 1 {
		t.Fatalf("youtube video: %+v", v)
	}
	andro := v.Videos[0].ID
	if _, err := e.a.AddVideo(e.ctx, modA, "org-a", v.ETag, domain.VideoInput{Link: "https://vimeo.com/404", Title: domain.Text{PL: "x"}}); !hasProblem(err, "link") {
		t.Fatalf("vimeo not found: %v", err)
	}
	info.Err = errors.New("network down")
	if _, err := e.a.AddVideo(e.ctx, modA, "org-a", v.ETag, domain.VideoInput{Link: "https://vimeo.com/5", Title: domain.Text{PL: "x"}}); !hasProblem(err, "link") {
		t.Fatalf("vimeo down: %v", err)
	}
	info.Err = nil
	vm := domain.VideoInput{Link: "https://vimeo.com/5/abcdef12", Title: domain.Text{PL: "Szot"}}
	v = must[app.VideoCollectionView](t)(e.a.AddVideo(e.ctx, modA, "org-a", v.ETag, vm))
	schot := v.Videos[1].ID
	if v.Videos[1].Thumbnail != "https://i.vimeocdn.com/video/5-d_640" || info.Calls != 3 {
		t.Fatalf("vimeo video: %+v (%d calls)", v.Videos[1], info.Calls)
	}
	// Editing without changing the video does not ask Vimeo again; a new video does.
	vm.Title.PL, vm.Start = "Szot (schottische)", "0:10"
	v = must[app.VideoCollectionView](t)(e.a.UpdateVideo(e.ctx, modA, "org-a", schot, v.ETag, vm))
	if info.Calls != 3 || v.Videos[1].Title.PL != "Szot (schottische)" || v.Videos[1].Link.StartSeconds != 10 {
		t.Fatalf("update: %+v (%d calls)", v.Videos[1], info.Calls)
	}
	vm.Link = "https://vimeo.com/404"
	if _, err := e.a.UpdateVideo(e.ctx, modA, "org-a", schot, v.ETag, vm); !hasProblem(err, "link") || info.Calls != 4 {
		t.Fatalf("update to an unavailable video: %v", err)
	}

	// The Site's videos.json follows every change.
	s := e.videoSnapshot(t, "org-a")
	if len(s.Videos) != 2 || s.Videos[0].ID != andro || s.Videos[0].Start != 90 || s.Videos[1].Thumbnail != "https://i.vimeocdn.com/video/5-d_640" || len(s.Forms) != 1 {
		t.Fatalf("snapshot: %+v", s)
	}
	v = must[app.VideoCollectionView](t)(e.a.MoveVideo(e.ctx, modA, "org-a", schot, v.ETag, 0))
	v = must[app.VideoCollectionView](t)(e.a.SetVideoHidden(e.ctx, modA, "org-a", andro, v.ETag, true))
	if s := e.videoSnapshot(t, "org-a"); len(s.Videos) != 1 || s.Videos[0].ID != schot {
		t.Fatalf("hidden and moved: %+v", s.Videos)
	}

	// Forms and Origins in use cannot be removed.
	if _, err := e.a.RemoveForm(e.ctx, modA, "org-a", pair, v.ETag); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("form in use: %v", err)
	}
	if _, err := e.a.RemoveOrigin(e.ctx, modA, "org-a", france, v.ETag); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("country with regions: %v", err)
	}
	v = must[app.VideoCollectionView](t)(e.a.DeleteVideo(e.ctx, modA, "org-a", andro, v.ETag))
	v = must[app.VideoCollectionView](t)(e.a.RemoveOrigin(e.ctx, modA, "org-a", brittany, v.ETag))
	v = must[app.VideoCollectionView](t)(e.a.RemoveForm(e.ctx, modA, "org-a", pair, v.ETag))
	v = must[app.VideoCollectionView](t)(e.a.RenameOrigin(e.ctx, modA, "org-a", france, v.ETag, domain.Text{PL: "Francja", EN: "France"}))
	if len(v.Forms) != 0 || len(v.Origins[0].Regions) != 0 || len(v.Videos) != 1 || v.Origins[0].Label.EN != "France" {
		t.Fatalf("after removals: %+v", v)
	}
	if _, err := e.a.DeleteVideo(e.ctx, modA, "org-a", andro, v.ETag); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted twice: %v", err)
	}

	// The Audit Log records each change; a deleted Video keeps its title and link there.
	actions := strings.Join(e.auditActions(t), " ")
	for _, want := range []string{"form.add", "origin.add", "video.add", "video.update", "video.move", "video.hide", "video.delete", "origin.remove", "form.remove", "origin.rename"} {
		if !strings.Contains(actions, want) {
			t.Errorf("audit has no %s: %s", want, actions)
		}
	}
	data := must[[]byte](t)(e.a.Audit.Read(e.ctx, e.clock.t.Format("2006-01-02")))
	if !strings.Contains(string(data), `"action":"video.delete","org":"org-a","target":"`+andro+`","details":{"title":"An dro","url":"https://www.youtube.com/watch?v=dQw4w9WgXcQ`+"\\u0026"+`t=90s"}`) {
		t.Fatalf("delete audit: %s", data)
	}

	// Link preview parses without saving.
	pv := must[app.LinkPreview](t)(e.a.PreviewVideoLink(e.ctx, modA, "org-a", domain.VideoInput{Link: "player.vimeo.com/video/77?h=abcdef12#t=1m"}))
	if pv.Provider != "vimeo" || pv.VideoID != "77" || pv.Hash != "abcdef12" || pv.Start != "1:00" || pv.LinkURL != "https://vimeo.com/77/abcdef12" || pv.ThumbnailURL == "" {
		t.Fatalf("preview: %+v", pv)
	}
	if _, err := e.a.PreviewVideoLink(e.ctx, modA, "org-a", domain.VideoInput{Link: "https://evil.example/x"}); !hasProblem(err, "link") {
		t.Fatalf("preview of a bad link: %v", err)
	}
	if _, err := e.a.PreviewVideoLink(e.ctx, modB, "org-a", domain.VideoInput{Link: "https://youtu.be/dQw4w9WgXcQ"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("preview by another organization: %v", err)
	}
	if after := must[app.VideoCollectionView](t)(e.a.GetVideos(e.ctx, modA, "org-a")); after.ETag != v.ETag {
		t.Fatal("preview saved something")
	}

	// RebuildAll writes both snapshots of every Organization, also an empty one.
	if err := e.a.RebuildAll(e.ctx); err != nil {
		t.Fatal(err)
	}
	if s := e.videoSnapshot(t, "org-b"); s.SchemaVersion != 1 || len(s.Videos) != 0 || s.Organization.ID != "org-b" {
		t.Fatalf("org-b: %+v", s)
	}
}

// A collection document that names another Organization is not read.
func TestVideoCollectionOwnership(t *testing.T) {
	e := newEnv(t)
	sa := must[domain.Principal](t)(e.signIn(t, "sa", "admin@example.org"))
	data, _ := json.Marshal(domain.NewVideoCollection("org-b"))
	if _, err := e.a.Docs.Put(e.ctx, "orgs/org-a/videos.json", data, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.a.GetVideos(e.ctx, sa, "org-a"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign document: %v", err)
	}
}
