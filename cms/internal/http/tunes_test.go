package httpapi

import (
	"encoding/json"
	"mime"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/filip-be/folk-ms/cms/internal/app"
	"github.com/filip-be/folk-ms/cms/internal/domain"
)

func TestTuneAPI(t *testing.T) {
	h := newHarnessWith(t, "org-a", "org-b")
	o, _ := h.app.Org("org-a")
	o.Site.Tunes = &domain.SiteTunes{TunePermalinkTemplate: "/{lang}/tunes/{shortId}-{slug}", SetPermalinkTemplate: "/{lang}/sets/{shortId}-{slug}"}
	admin := h.client(t)
	h.signIn(t, admin)
	call := func(method, path, body string, rev int, want int, out any) string {
		t.Helper()
		hdr := h.same()
		if rev > 0 {
			hdr["If-Match"] = strconv.Itoa(rev)
		}
		resp, b := h.do(t, admin, method, path, body, hdr)
		if resp.StatusCode != want {
			t.Fatalf("%s %s: %d %s", method, path, resp.StatusCode, b)
		}
		if out != nil {
			if err := json.Unmarshal([]byte(b), out); err != nil {
				t.Fatalf("%v: %s", err, b)
			}
		}
		return b
	}
	const base = "/api/orgs/org-a/tunes"

	// An Organization without the opt-in has no tune routes.
	if resp, _ := h.do(t, admin, "GET", "/api/orgs/org-b/tunes", "", nil); resp.StatusCode != 404 {
		t.Fatalf("no opt-in: %d", resp.StatusCode)
	}
	if resp, _ := h.do(t, admin, "POST", "/api/orgs/org-b/tunes/markdown-preview", `{"text":"x"}`, h.same()); resp.StatusCode != 404 {
		t.Fatalf("no opt-in preview: %d", resp.StatusCode)
	}
	if resp, _ := h.do(t, admin, "GET", "/api/orgs/nope/tunes", "", nil); resp.StatusCode != 404 {
		t.Fatalf("unknown org: %d", resp.StatusCode)
	}
	var me struct {
		Orgs []struct {
			ID    string
			Tunes bool
		}
	}
	_, b := h.do(t, admin, "GET", "/api/me", "", nil)
	_ = json.Unmarshal([]byte(b), &me)
	for _, mo := range me.Orgs {
		if mo.Tunes != (mo.ID == "org-a") {
			t.Fatalf("me: %s", b)
		}
	}

	var cv app.TuneCollectionView
	call("GET", base, "", 0, 200, &cv)
	reel := ""
	for _, ty := range cv.Types {
		if ty.Label.EN == "reel" {
			reel = ty.ID
		}
	}
	tune := `{"name":{"name":"The Silver Spear"},"typeId":"` + reel + `","key":{"tonic":"D","mode":"major"},"meter":"4/4","abc":"X:1\nK:D\nA|FA AA BAFA|dfed BddB|"}`
	var tv app.TuneView
	call("POST", base, tune, 0, 200, &tv)
	if tv.Rev != 1 || tv.Incipit == "" {
		t.Fatalf("created: %+v", tv)
	}
	call("POST", base, `{"name":{"name":""}}`, 0, 422, nil)
	id := tv.ID
	call("GET", base+"/"+id, "", 0, 200, &tv)

	// Changes name the item's revision in If-Match.
	upd := strings.Replace(tune, "The Silver Spear", "Silver Spear", 1)
	call("PUT", base+"/"+id, upd, 0, 409, nil)
	call("PUT", base+"/"+id, upd, 1, 200, &tv)
	if tv.Rev != 2 || tv.Name.Name != "Silver Spear" {
		t.Fatalf("updated: %+v", tv)
	}
	call("PUT", base+"/"+id, upd, 1, 409, nil)
	call("POST", base+"/"+id+"/hide", "", 0, 200, &tv)
	if !tv.Hidden {
		t.Fatal("hidden")
	}
	call("POST", base+"/"+id+"/show", "", 0, 200, &tv)
	call("POST", base+"/"+id+"/nope", "", 0, 404, nil)

	// Attachments: the raw file is the body.
	resp, b := h.do(t, admin, "POST", base+"/"+id+"/attachments?name=Nuty.pdf", "%PDF-1.4 x", map[string]string{"Origin": h.ts.URL, "Content-Type": "application/pdf"})
	if resp.StatusCode != 200 {
		t.Fatalf("upload: %d %s", resp.StatusCode, b)
	}
	_ = json.Unmarshal([]byte(b), &tv)
	if len(tv.Attachments) != 1 || tv.Attachments[0].Name != "nuty.pdf" {
		t.Fatalf("attachment: %+v", tv.Attachments)
	}
	if resp, _ := h.do(t, admin, "POST", base+"/"+id+"/attachments", "<html>", map[string]string{"Origin": h.ts.URL, "Content-Type": "text/html"}); resp.StatusCode != 400 {
		t.Fatalf("html upload: %d", resp.StatusCode)
	}
	aid := tv.Attachments[0].ID
	call("PUT", base+"/"+id+"/attachments/"+aid, `{"caption":{"en":"Sheet"}}`, 0, 200, &tv)
	call("POST", base+"/"+id+"/attachments/"+aid+"/move", `{"to":0}`, 0, 200, &tv)
	call("DELETE", base+"/"+id+"/attachments/"+aid, "", 0, 200, &tv)
	if len(tv.Attachments) != 0 {
		t.Fatal("attachment removed")
	}

	// Previews.
	var md app.MarkdownPreview
	call("POST", base+"/markdown-preview", `{"text":"<script>"}`, 0, 200, &md)
	if md.HTML != "<p>&lt;script&gt;</p>" {
		t.Fatalf("markdown: %q", md.HTML)
	}
	var ap app.ABCPreview
	call("POST", base+"/abc-preview", `{"abc":"X:1\nM:6/8\nK:Ador\nABc|","bars":1}`, 0, 200, &ap)
	if ap.Proposal.Key == nil || ap.Proposal.Key.Mode != "dorian" || !ap.Valid {
		t.Fatalf("abc preview: %+v", ap)
	}
	var ip app.ImportPreviewView
	call("POST", base+"/import-preview", `{"text":"X:1\nT:Silver Spear\nR:reel\nM:4/4\nK:D\nABcd|\n"}`, 0, 200, &ip)
	if len(ip.Candidates) != 1 || len(ip.Candidates[0].Duplicate) != 1 {
		t.Fatalf("import preview: %+v", ip)
	}
	var rv app.RecordingView
	call("POST", base+"/link-preview", `{"link":"https://youtu.be/dQw4w9WgXcQ"}`, 0, 200, &rv)
	call("POST", base+"/link-preview", `{"link":"http://example.org"}`, 0, 422, nil)
	call("POST", base+"/import", `{"tunes":[{"name":{"name":"Imported"},"typeId":"`+reel+`","key":{"tonic":"G","mode":"major"},"meter":"4/4"}]}`, 0, 200, &cv)
	if len(cv.Tunes) != 2 {
		t.Fatalf("import: %+v", cv.Tunes)
	}
	other := ""
	for _, s := range cv.Tunes {
		if s.Name.Name == "Imported" {
			other = s.ID
			if !s.Hidden {
				t.Fatal("import not hidden")
			}
		}
	}

	// Sets.
	call("POST", base+"/sets", `{"tuneIds":["`+id+`","`+other+`"]}`, 0, 200, &cv)
	set := cv.Sets[0]
	call("PUT", base+"/sets/"+set.ID, `{"name":{"en":"Reels"},"tuneIds":["`+other+`","`+id+`"]}`, set.Rev, 200, &cv)
	call("PUT", base+"/sets/"+set.ID, `{"tuneIds":["`+id+`","`+other+`"]}`, set.Rev, 409, nil)
	call("POST", base+"/sets/"+set.ID+"/hide", "", 0, 200, &cv)
	call("POST", base+"/sets/"+set.ID+"/show", "", 0, 200, &cv)

	// Removing a Tune in a Set needs ?confirm=true.
	call("GET", base+"/"+id, "", 0, 200, &tv)
	b = call("DELETE", base+"/"+id, "", tv.Rev, 409, nil)
	if !strings.Contains(b, "needs_confirmation") {
		t.Fatalf("confirm: %s", b)
	}
	call("DELETE", base+"/"+id+"?confirm=true", "", tv.Rev, 200, &cv)
	if len(cv.Tunes) != 1 || len(cv.Sets[0].TuneIDs) != 1 {
		t.Fatalf("removed: %+v", cv)
	}
	call("DELETE", base+"/sets/"+cv.Sets[0].ID, "", cv.Sets[0].Rev, 200, &cv)

	// Lists.
	call("POST", base+"/types", `{"label":{"en":"fling"},"meter":"4/4","tempo":120}`, 0, 200, &cv)
	fling := cv.Types[len(cv.Types)-1]
	call("PUT", base+"/types/"+fling.ID, `{"label":{"en":"Fling"},"meter":"4/4","tempo":110}`, fling.Rev, 200, &cv)
	call("POST", base+"/types/"+fling.ID+"/move", `{"to":0}`, 0, 200, &cv)
	call("DELETE", base+"/types/"+fling.ID, "", 0, 200, &cv)
	call("DELETE", base+"/types/"+reel, "", 0, 422, nil) // in use: a problem on the label
	call("POST", base+"/instruments", `{"label":{"pl":"lira korbowa"}}`, 0, 200, &cv)
	in := cv.Instruments[len(cv.Instruments)-1]
	call("PUT", base+"/instruments/"+in.ID, `{"label":{"pl":"lira"}}`, in.Rev, 200, &cv)
	call("POST", base+"/instruments/"+in.ID+"/move", `{"to":0}`, 0, 200, &cv)
	call("DELETE", base+"/instruments/"+in.ID, "", 0, 200, &cv)
	call("POST", base+"/origins", `{"label":{"en":"Ireland"}}`, 0, 200, &cv)
	ie := cv.Origins[0]
	call("POST", base+"/origins", `{"label":{"en":"Clare"},"parentId":"`+ie.ID+`"}`, 0, 200, &cv)
	call("PUT", base+"/origins/"+ie.ID, `{"label":{"en":"Éire"}}`, ie.Rev, 200, &cv)
	call("POST", base+"/origins/"+cv.Origins[0].Regions[0].ID+"/move", `{"to":0}`, 0, 200, &cv)
	call("DELETE", base+"/origins/"+cv.Origins[0].Regions[0].ID, "", 0, 200, &cv)
	call("DELETE", base+"/origins/"+ie.ID, "", 0, 200, &cv)

	// Path values are plain names.
	if resp, _ := h.do(t, admin, "DELETE", base+"/"+id+"/attachments/..%2F..%2Fx", "", h.same()); resp.StatusCode != 404 {
		t.Fatalf("crafted attachment id: %d", resp.StatusCode)
	}
}

// abcjs draws with inline styles; scripts stay 'self'; samples load from 'self'.
func TestAdminCSPForNotation(t *testing.T) {
	h := newHarness(t)
	resp, _ := h.do(t, h.client(t), "GET", "/app/", "", nil)
	csp := resp.Header.Get("Content-Security-Policy")
	dirs := map[string]string{}
	for _, d := range strings.Split(csp, ";") {
		d = strings.TrimSpace(d)
		name, _, _ := strings.Cut(d, " ")
		dirs[name] = d
	}
	if dirs["style-src"] != "style-src 'self' 'unsafe-inline'" || dirs["script-src"] != "script-src 'self'" ||
		dirs["media-src"] != "media-src 'self'" || dirs["connect-src"] != "connect-src 'self'" {
		t.Fatalf("csp: %s", csp)
	}
}

// Locally, Tune Attachments are served inline like Storage serves them, and
// the admin's piano samples have their MP3 type.
func TestLocalTuneAttachmentsAndSamples(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "media/org-a/tunes/abcdefgh"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "media/org-a/tunes/abcdefgh/h.pdf"), []byte("%PDF-1.4"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "media/org-a/c.jpg"), []byte("x"), 0o644)
	s := &Server{App: app.New(nil), PublicDir: dir}
	for path, want := range map[string]string{"/pub/media/org-a/tunes/abcdefgh/h.pdf": "inline", "/pub/media/org-a/c.jpg": ""} {
		rec := httptest.NewRecorder()
		s.publicFiles().ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 || rec.Header().Get("Content-Disposition") != want {
			t.Errorf("%s: %d %q", path, rec.Code, rec.Header().Get("Content-Disposition"))
		}
	}
	if ct := mime.TypeByExtension(".mp3"); ct != "audio/mpeg" {
		t.Errorf("mp3: %q", ct)
	}
	st, err := newStaticFS(fstest.MapFS{"chunks/abcjs.js": {Data: []byte("x")}, "sounds/piano/A4.mp3": {Data: []byte("x")}})
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"/app/chunks/abcjs.js": "text/javascript; charset=utf-8", "/app/sounds/piano/A4.mp3": "audio/mpeg"} {
		rec := httptest.NewRecorder()
		st.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 || rec.Header().Get("Content-Type") != want {
			t.Errorf("%s: %d %q", path, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
}
