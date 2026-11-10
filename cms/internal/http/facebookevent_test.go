package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/app"
	"github.com/filip-be/folk-ms/cms/internal/domain"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

func TestFacebookEventRoutes(t *testing.T) {
	h := newHarness(t)
	c := h.client(t)
	h.signIn(t, c)
	ctx := context.Background()
	u, err := h.app.SignIn(ctx, ports.Identity{Issuer: "test", Subject: "admin@example.org", Email: "admin@example.org", EmailVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	p, err := h.app.Principal(ctx, u.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	v, err := h.app.CreateVenue(ctx, p, "org-a", domain.Venue{Name: "Zamek", Address: "Św. Marcin 80"})
	if err != nil {
		t.Fatal(err)
	}
	var png bytes.Buffer
	_ = pngEncode(&png, 1200, 600)
	cover, err := h.app.UploadImage(ctx, p, "org-a", ports.ImageCover, png.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	ev, err := h.app.CreateEvent(ctx, p, "org-a", domain.Content{Title: domain.Text{PL: "Bal"}, VenueID: v.ID, Cover: cover,
		Schedule: domain.Schedule{Kind: domain.KindSingle, StartDate: "2030-10-01", StartTime: "19:00", EndTime: "22:00"}})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/orgs/org-a/events/" + ev.Event.ID + "/facebook-event"
	same := map[string]string{"Origin": h.ts.URL}

	// A Draft is not handed over and gets no cover.
	resp, body := h.do(t, c, "GET", base, "", nil)
	if resp.StatusCode != 200 || !strings.Contains(body, `"published":false`) || strings.Contains(body, `"token"`) {
		t.Fatalf("draft handoff: %d %s", resp.StatusCode, body)
	}
	if resp, _ := h.do(t, c, "GET", base+"/cover", "", nil); resp.StatusCode != 404 {
		t.Fatalf("draft cover: %d", resp.StatusCode)
	}

	if _, err := h.app.PublishEvent(ctx, p, "org-a", ev.Event.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.app.CompleteSocialLogin(ctx, p, "org-a", "code", h.ts.URL+"/auth/meta/callback"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.app.SelectPage(ctx, p, "org-a", "1001"); err != nil {
		t.Fatal(err)
	}

	var hv app.HandoffView
	resp, body = h.do(t, c, "GET", base, "", nil)
	if resp.StatusCode != 200 || json.Unmarshal([]byte(body), &hv) != nil || hv.Fields == nil || hv.Token == "" || hv.CoverURL != base+"/cover" {
		t.Fatalf("handoff: %d %s", resp.StatusCode, body)
	}
	resp, body = h.do(t, c, "GET", hv.CoverURL, "", nil)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/jpeg" || !strings.HasPrefix(body, "\xff\xd8") {
		t.Fatalf("cover: %d %v", resp.StatusCode, resp.Header)
	}

	// Writes need the CMS's own Origin (CSRF).
	add := `{"url":"https://m.facebook.com/events/123456789/","token":"` + hv.Token + `"}`
	if resp, _ := h.do(t, c, "POST", base, add, nil); resp.StatusCode != 403 {
		t.Fatalf("cross-site add: %d", resp.StatusCode)
	}
	if resp, body := h.do(t, c, "POST", base, `{"url":"https://example.org/events/1/"}`, same); resp.StatusCode != 422 || !strings.Contains(body, "url") {
		t.Fatalf("bad url: %d %s", resp.StatusCode, body)
	}
	if resp, _ := h.do(t, c, "POST", base, `{"url":"x","handedOver":{"title":"forged"}}`, same); resp.StatusCode != 400 {
		t.Fatalf("client-supplied values are refused: %d", resp.StatusCode)
	}
	resp, body = h.do(t, c, "POST", base, add, same)
	if resp.StatusCode != 200 || json.Unmarshal([]byte(body), &hv) != nil || len(hv.Links) != 1 || hv.Links[0].Link.URL != "https://www.facebook.com/events/123456789/" {
		t.Fatalf("add: %d %s", resp.StatusCode, body)
	}
	link := base + "/" + hv.Links[0].Link.ID
	doc, err := h.app.Pub.Get(ctx, "sites/org-a/events.json")
	if err != nil || !strings.Contains(string(doc.Data), `"facebookEventUrl":"https://www.facebook.com/events/123456789/"`) {
		t.Fatalf("snapshot: %v %s", err, doc.Data)
	}

	resp, body = h.do(t, c, "PUT", link, `{"url":"https://facebook.com/events/987654321"}`, same)
	if resp.StatusCode != 200 || !strings.Contains(body, "https://www.facebook.com/events/987654321/") {
		t.Fatalf("change: %d %s", resp.StatusCode, body)
	}
	if resp, body := h.do(t, c, "POST", link+"/dismiss", "", same); resp.StatusCode != 200 || !strings.Contains(body, `"outOfDate":false`) {
		t.Fatalf("dismiss: %d %s", resp.StatusCode, body)
	}
	if resp, _ := h.do(t, c, "PUT", base+"/nope", `{"url":"https://facebook.com/events/987654321"}`, same); resp.StatusCode != 404 {
		t.Fatalf("unknown link: %d", resp.StatusCode)
	}
	if resp, body := h.do(t, c, "DELETE", link, "", same); resp.StatusCode != 200 || !strings.Contains(body, `"links":[]`) {
		t.Fatalf("remove: %d %s", resp.StatusCode, body)
	}

	resp, body = h.do(t, c, "POST", "/api/orgs/org-a/autofill-reports", `{"eventId":"`+ev.Event.ID+`","version":"0.1.0","fields":{"name":"filled","location":"failed"}}`, same)
	if resp.StatusCode != 200 {
		t.Fatalf("report: %d %s", resp.StatusCode, body)
	}
	audit, err := h.app.ReadAudit(ctx, p, time.Now().UTC().Format(time.DateOnly))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, a := range audit {
		seen[a.Action] = true
	}
	for _, want := range []string{"facebook_event.linked", "facebook_event.changed", "facebook_event.dismissed", "facebook_event.removed", "autofill.reported"} {
		if !seen[want] {
			t.Errorf("audit lacks %s", want)
		}
	}

	// Signed out, nothing is reachable.
	if resp, _ := h.do(t, h.client(t), "GET", base, "", nil); resp.StatusCode != 401 {
		t.Fatalf("anonymous handoff: %d", resp.StatusCode)
	}
}

func TestUserscriptCarriesTheCMSOrigin(t *testing.T) {
	h := newHarness(t)
	resp, body := h.do(t, h.client(t), "GET", "/app/autofill.user.js", "", nil)
	if resp.StatusCode != 200 || body != "// @match "+h.ts.URL+"/app/*" || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/javascript") {
		t.Fatalf("userscript: %d %q %v", resp.StatusCode, body, resp.Header)
	}
}

func TestAutofillConstantsMatch(t *testing.T) {
	lib, err := os.ReadFile("../../../autofill/lib.ts")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(lib), `ORIGIN_PLACEHOLDER = "`+userscriptPlaceholder+`";`) {
		t.Errorf("autofill/lib.ts does not use the placeholder %q", userscriptPlaceholder)
	}
	view, err := os.ReadFile("../../admin/src/views/facebookEvent.ts")
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("const MAX_NAME = %d;", domain.MaxFacebookEventNameLen); !strings.Contains(string(view), want) {
		t.Errorf("cms/admin/src/views/facebookEvent.ts lacks %q", want)
	}
}
