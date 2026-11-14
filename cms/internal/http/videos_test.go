package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/filip-be/folk-ms/cms/internal/adapters/memory"
	"github.com/filip-be/folk-ms/cms/internal/app"
)

func TestVideoAPI(t *testing.T) {
	h := newHarnessWith(t, "org-a", "org-b")
	h.app.VideoInfo = &memory.VideoInfo{Unavailable: map[string]bool{"404": true}}
	admin := h.client(t)
	h.signIn(t, admin)
	var v app.VideoCollectionView
	read := func(body string) {
		t.Helper()
		v = app.VideoCollectionView{}
		if err := json.Unmarshal([]byte(body), &v); err != nil {
			t.Fatalf("%v: %s", err, body)
		}
	}
	call := func(method, path, body string, want int) string {
		t.Helper()
		resp, b := h.do(t, admin, method, path, body, map[string]string{"Origin": h.ts.URL, "If-Match": v.ETag})
		if resp.StatusCode != want {
			t.Fatalf("%s %s: %d %s", method, path, resp.StatusCode, b)
		}
		if want == 200 {
			read(b)
		}
		return b
	}
	const base = "/api/orgs/org-a/videos"
	resp, body := h.do(t, admin, "GET", base, "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("get: %d %s", resp.StatusCode, body)
	}
	read(body)
	if v.ETag != app.NewVideosETag {
		t.Fatalf("etag %q", v.ETag)
	}

	// Writes need If-Match.
	if resp, _ := h.do(t, admin, "POST", base+"/forms", `{"label":{"pl":"Para"}}`, h.same()); resp.StatusCode != 409 {
		t.Fatalf("no If-Match: %d", resp.StatusCode)
	}
	call("POST", base+"/forms", `{"label":{"pl":"Para","en":"Pair"}}`, 200)
	form := v.Forms[0].ID
	call("POST", base+"/forms", `{"label":{"pl":"Koło"}}`, 200)
	call("POST", base+"/forms/"+form+"/move", `{"to":1}`, 200)
	if v.Forms[1].ID != form {
		t.Fatalf("form move: %+v", v.Forms)
	}
	call("POST", base+"/forms/"+form+"/move", `{}`, 400)
	call("PUT", base+"/forms/"+form, `{"label":{"pl":""}}`, 422)
	call("PUT", base+"/forms/"+form, `{"label":{"pl":"Pary"}}`, 200)
	call("POST", base+"/origins", `{"label":{"pl":"Francja"}}`, 200)
	country := v.Origins[0].ID
	call("POST", base+"/origins", `{"label":{"pl":"Bretania"},"parentId":"`+country+`"}`, 200)
	region := v.Origins[0].Regions[0].ID
	call("PUT", base+"/origins/"+region, `{"label":{"pl":"Bretania","en":"Brittany"}}`, 200)
	call("POST", base+"/origins/"+region+"/move", `{"to":0}`, 200)

	// Link preview: the server parses; nothing is saved.
	etag := v.ETag
	resp, body = h.do(t, admin, "POST", base+"/link-preview", `{"link":"youtu.be/dQw4w9WgXcQ?t=1m30s"}`, h.same())
	if resp.StatusCode != 200 || !strings.Contains(body, `"start":"1:30"`) || !strings.Contains(body, `"thumbnailUrl":"https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg"`) {
		t.Fatalf("preview: %d %s", resp.StatusCode, body)
	}
	for _, link := range []string{"https://evil.example/v", "https://vimeo.com/404"} {
		resp, body = h.do(t, admin, "POST", base+"/link-preview", `{"link":"`+link+`"}`, h.same())
		if resp.StatusCode != 422 || !strings.Contains(body, `"field":"link"`) {
			t.Fatalf("preview %s: %d %s", link, resp.StatusCode, body)
		}
	}

	// Videos.
	video := `{"link":"https://youtu.be/dQw4w9WgXcQ","start":"0:30","title":{"pl":"An dro"},"originId":"` + region + `","formIds":["` + form + `"]}`
	if v.ETag != etag {
		t.Fatal("preview changed the collection")
	}
	call("POST", base, `{"link":"https://vimeo.com/1","title":{"pl":""}}`, 422)
	call("POST", base, video, 200)
	id := v.Videos[0].ID
	if v.Videos[0].Link.StartSeconds != 30 {
		t.Fatalf("video: %+v", v.Videos[0])
	}
	call("PUT", base+"/"+id, strings.Replace(video, "An dro", "An dro 2", 1), 200)
	call("POST", base+"/"+id+"/hide", "", 200)
	if !v.Videos[0].Hidden {
		t.Fatal("not hidden")
	}
	call("POST", base+"/"+id+"/show", "", 200)
	call("POST", base+"/"+id+"/move", `{"to":0}`, 200)
	call("POST", base+"/"+id+"/publish", "", 404)
	call("DELETE", base+"/forms/"+form, "", 422) // in use: the problem names how many
	call("DELETE", base+"/origins/"+country, "", 422)

	// A stale version conflicts.
	stale := v.ETag
	call("POST", base+"/"+id+"/hide", "", 200)
	if resp, _ := h.do(t, admin, "POST", base+"/"+id+"/show", "", map[string]string{"Origin": h.ts.URL, "If-Match": stale}); resp.StatusCode != 409 {
		t.Fatalf("stale: %d", resp.StatusCode)
	}
	call("DELETE", base+"/"+id, "", 200)
	call("DELETE", base+"/"+id, "", 404)
	call("DELETE", base+"/origins/"+region, "", 200)
	call("DELETE", base+"/forms/"+form, "", 200)

	// The Site's videos.json is public and follows the changes.
	snap, err := h.app.Pub.Get(t.Context(), app.VideosPath("org-a"))
	if err != nil || !strings.Contains(string(snap.Data), `"schemaVersion":1`) || !strings.Contains(string(snap.Data), `"videos":[]`) {
		t.Fatalf("snapshot: %v %s", err, snap.Data)
	}

	// Path values are checked like every other id.
	if resp, _ := h.do(t, admin, "PUT", base+"/..%2F..%2Fx", video, h.same()); resp.StatusCode != 404 {
		t.Fatalf("traversal: %d", resp.StatusCode)
	}

	// A Moderator of another Organization gets 403.
	if resp, body := h.do(t, admin, "POST", "/api/invitations", `{"email":"bob@example.org","orgs":["org-b"]}`, h.same()); resp.StatusCode != 200 {
		t.Fatalf("invite: %d %s", resp.StatusCode, body)
	}
	h.idp.email = "bob@example.org"
	bob := h.client(t)
	h.signIn(t, bob)
	for _, c := range [][3]string{{"GET", base, ""}, {"POST", base + "/forms", `{"label":{"pl":"x"}}`}, {"POST", base + "/link-preview", `{"link":"https://youtu.be/dQw4w9WgXcQ"}`}} {
		if resp, body := h.do(t, bob, c[0], c[1], c[2], map[string]string{"Origin": h.ts.URL, "If-Match": v.ETag}); resp.StatusCode != 403 {
			t.Fatalf("%s %s by another organization: %d %s", c[0], c[1], resp.StatusCode, body)
		}
	}
	if resp, _ := h.do(t, bob, "GET", "/api/orgs/org-b/videos", "", nil); resp.StatusCode != 200 {
		t.Fatalf("own organization: %d", resp.StatusCode)
	}
}

// The admin may show YouTube and Vimeo thumbnails, and nothing else from them (D22).
func TestAdminCSPAllowsVideoThumbnails(t *testing.T) {
	h := newHarness(t)
	resp, _ := h.do(t, h.client(t), "GET", "/app/", "", nil)
	csp := resp.Header.Get("Content-Security-Policy")
	var img, def string
	for _, d := range strings.Split(csp, ";") {
		d = strings.TrimSpace(d)
		if strings.HasPrefix(d, "img-src ") {
			img = d
		}
		if strings.HasPrefix(d, "default-src ") {
			def = d
		}
	}
	for _, host := range []string{"https://i.ytimg.com", "https://i.vimeocdn.com"} {
		if !strings.Contains(img, " "+host) {
			t.Errorf("img-src lacks %s: %s", host, csp)
		}
		if strings.Count(csp, host) != 1 {
			t.Errorf("%s allowed beyond images: %s", host, csp)
		}
	}
	if def != "default-src 'self'" || strings.Contains(csp, "frame-src") {
		t.Fatalf("csp: %s", csp)
	}
}
