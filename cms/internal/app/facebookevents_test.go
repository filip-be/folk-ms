package app_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/app"
	"github.com/filip-be/folk-ms/cms/internal/domain"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

func connect(t *testing.T, e *env, org string) {
	t.Helper()
	data, _ := json.Marshal(domain.Connection{OrgID: org, PageID: "p1", PageName: "Folk MS", CreatedAt: e.clock.t})
	if _, err := e.a.Docs.Put(e.ctx, "orgs/"+org+"/social/connection.json", data, ports.AnyETag); err != nil {
		t.Fatal(err)
	}
}

func TestFacebookEventHandoffAndLinks(t *testing.T) {
	e := newEnv(t)
	e.a.SetHandoffKey([]byte(strings.Repeat("k", 32)))
	sa := must[domain.Principal](t)(e.signIn(t, "sa", "admin@example.org"))
	must[*domain.Invitation](t)(e.a.CreateInvitation(e.ctx, sa, "b@example.org", []string{"org-b"}))
	modB := must[domain.Principal](t)(e.signIn(t, "b", "b@example.org"))
	v := must[app.VenueView](t)(e.a.CreateVenue(e.ctx, sa, "org-a", domain.Venue{Name: "Zamek", Address: "Św. Marcin 80"}))
	ev := must[app.EventView](t)(e.a.CreateEvent(e.ctx, sa, "org-a", publishable(v.ID)))
	id := ev.Event.ID

	// A Draft gets no Handoff and no links.
	h := must[app.HandoffView](t)(e.a.FacebookEventHandoff(e.ctx, sa, "org-a", id))
	if h.Published || h.Fields != nil || h.Token != "" {
		t.Fatalf("draft: %+v", h)
	}
	if _, err := e.a.AddFacebookEventLink(e.ctx, sa, "org-a", id, app.LinkInput{URL: "https://www.facebook.com/events/111111/"}); !domain.IsValidation(err) {
		t.Fatalf("draft link: %v", err)
	}
	must[app.EventView](t)(e.a.PublishEvent(e.ctx, sa, "org-a", id))

	// Without a Social Account Connection there is no Facebook page to host it.
	h = must[app.HandoffView](t)(e.a.FacebookEventHandoff(e.ctx, sa, "org-a", id))
	if h.Connection.Connected || h.Fields != nil {
		t.Fatalf("no connection: %+v", h)
	}
	connect(t, e, "org-a")
	h = must[app.HandoffView](t)(e.a.FacebookEventHandoff(e.ctx, sa, "org-a", id))
	if h.Fields == nil || h.Token == "" || h.Fields.OccurrenceKey != "2026-11-05" || h.Connection.PageName != "Folk MS" {
		t.Fatalf("handoff: %+v", h)
	}
	if _, err := e.a.FacebookEventHandoff(e.ctx, modB, "org-a", id); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("other organization's moderator")
	}

	// The Event changes after the handoff; the link saved with the token
	// keeps what the Facebook Event was created from, so it is out of date.
	token := h.Token
	upd := publishable(v.ID)
	upd.Title.PL = "Bal jesienny – zmiana"
	cur := must[app.EventView](t)(e.a.GetEvent(e.ctx, sa, "org-a", id))
	must[app.EventView](t)(e.a.UpdateEvent(e.ctx, sa, "org-a", id, cur.ETag, upd))
	must[app.EventView](t)(e.a.PublishEvent(e.ctx, sa, "org-a", id))
	h = must[app.HandoffView](t)(e.a.AddFacebookEventLink(e.ctx, sa, "org-a", id, app.LinkInput{URL: "https://m.facebook.com/events/111111/?x=1", Token: token}))
	if len(h.Links) != 1 || !h.Links[0].OutOfDate || h.Links[0].Changed[0] != domain.ChangedTitle || h.Links[0].Link.HandedOver.Title != "Bal jesienny" {
		t.Fatalf("token link: %+v", h.Links)
	}
	if !h.NextLinked {
		t.Fatal("the next Occurrence is linked now")
	}
	doc := must[ports.Document](t)(e.pub.Get(e.ctx, "sites/org-a/events.json"))
	if !strings.Contains(string(doc.Data), `"facebookEventUrl":"https://www.facebook.com/events/111111/"`) {
		t.Fatalf("snapshot: %s", doc.Data)
	}

	// Without a valid token the server computes the values itself: a
	// tampered token, one for another Event, or an expired one are ignored.
	occKey := "2026-11-12"
	tampered := token[:len(token)-2] + "xx"
	h = must[app.HandoffView](t)(e.a.AddFacebookEventLink(e.ctx, sa, "org-a", id, app.LinkInput{URL: "https://www.facebook.com/events/222222/", OccurrenceKey: occKey, Token: tampered}))
	if h.Links[1].OutOfDate || h.Links[1].Link.OccurrenceKey != occKey || h.Links[1].Link.HandedOver.Title != "Bal jesienny – zmiana" {
		t.Fatalf("no token: %+v", h.Links[1])
	}
	e.clock.t = e.clock.t.Add(app.HandoffTokenTTL + time.Minute)
	h = must[app.HandoffView](t)(e.a.AddFacebookEventLink(e.ctx, sa, "org-a", id, app.LinkInput{URL: "https://www.facebook.com/events/333333/", OccurrenceKey: "2026-11-19", Token: token}))
	if h.Links[2].Link.OccurrenceKey != "2026-11-19" || h.Links[2].OutOfDate {
		t.Fatalf("expired token: %+v", h.Links[2])
	}

	// Dismiss, change, remove; each is audited.
	l1 := h.Links[0].Link.ID
	h = must[app.HandoffView](t)(e.a.DismissFacebookEventFlag(e.ctx, sa, "org-a", id, l1))
	if h.Links[0].OutOfDate {
		t.Fatal("dismissed")
	}
	h = must[app.HandoffView](t)(e.a.ChangeFacebookEventLink(e.ctx, sa, "org-a", id, l1, app.LinkInput{URL: "https://www.facebook.com/events/444444/"}))
	if h.Links[0].Link.URL != "https://www.facebook.com/events/444444/" {
		t.Fatal("changed")
	}
	if _, err := e.a.ChangeFacebookEventLink(e.ctx, sa, "org-a", id, "nope", app.LinkInput{URL: "https://www.facebook.com/events/555555/"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("unknown link")
	}
	h = must[app.HandoffView](t)(e.a.RemoveFacebookEventLink(e.ctx, sa, "org-a", id, l1))
	if len(h.Links) != 2 {
		t.Fatal("removed")
	}
	if _, err := e.a.RemoveFacebookEventLink(e.ctx, modB, "org-a", id, h.Links[0].Link.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("other organization cannot remove")
	}

	// List flags: unpublishing flags every link.
	must[app.EventView](t)(e.a.UnpublishEvent(e.ctx, sa, "org-a", id))
	list := must[[]app.EventSummary](t)(e.a.ListEvents(e.ctx, sa, "org-a", app.ViewDrafts))
	if len(list) != 1 || !list[0].FacebookOutOfDate {
		t.Fatalf("list flags: %+v", list)
	}

	audit := must[[]app.AuditEntry](t)(e.a.ReadAudit(e.ctx, sa, e.clock.t.Format(time.DateOnly)))
	seen := map[string]bool{}
	for _, a := range audit {
		seen[a.Action] = true
	}
	for _, want := range []string{"facebook_event.dismissed", "facebook_event.changed", "facebook_event.removed"} {
		if !seen[want] {
			t.Errorf("audit lacks %s: %+v", want, audit)
		}
	}
}

func TestFacebookEventCoverAndReports(t *testing.T) {
	e := newEnv(t)
	e.a.SetHandoffKey([]byte(strings.Repeat("k", 32)))
	sa := must[domain.Principal](t)(e.signIn(t, "sa", "admin@example.org"))
	v := must[app.VenueView](t)(e.a.CreateVenue(e.ctx, sa, "org-a", domain.Venue{Name: "Zamek", Address: "Św. Marcin 80"}))
	img := must[*domain.Image](t)(e.a.UploadImage(e.ctx, sa, "org-a", ports.ImageCover, pngImage(1600, 900)))
	c := publishable(v.ID)
	c.Cover = img
	ev := must[app.EventView](t)(e.a.CreateEvent(e.ctx, sa, "org-a", c))
	if _, err := e.a.FacebookEventCover(e.ctx, sa, "org-a", ev.Event.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("no cover before Publish")
	}
	must[app.EventView](t)(e.a.PublishEvent(e.ctx, sa, "org-a", ev.Event.ID))
	data := must[[]byte](t)(e.a.FacebookEventCover(e.ctx, sa, "org-a", ev.Event.ID))
	if len(data) < 100 || data[0] != 0xFF || data[1] != 0xD8 {
		t.Fatal("cover is the largest JPEG variant")
	}

	if err := e.a.ReportAutofill(e.ctx, sa, "org-a", app.AutofillReport{EventID: ev.Event.ID, Version: "0.1.0", Fields: map[string]string{"name": "filled", "location": "failed"}, Errors: []string{strings.Repeat("x", 1000)}}); err != nil {
		t.Fatal(err)
	}
	big := map[string]string{}
	for i := range 40 {
		big[strings.Repeat("f", i+1)] = "failed"
	}
	if err := e.a.ReportAutofill(e.ctx, sa, "org-a", app.AutofillReport{Fields: big}); !domain.IsValidation(err) {
		t.Fatal("oversized report")
	}
	audit := must[[]app.AuditEntry](t)(e.a.ReadAudit(e.ctx, sa, e.clock.t.Format(time.DateOnly)))
	var last app.AuditEntry
	for _, a := range audit {
		if a.Action == "autofill.reported" {
			last = a
		}
	}
	if last.Details["failed"] != float64(1) {
		t.Fatalf("report audit: %+v", last)
	}
	if errs := last.Details["errors"].([]any); len(errs[0].(string)) > 300 {
		t.Fatal("report strings are clipped")
	}
}
