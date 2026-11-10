package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/adapters/img"
	"github.com/filip-be/folk-ms/cms/internal/adapters/memory"
	"github.com/filip-be/folk-ms/cms/internal/adapters/meta"
	"github.com/filip-be/folk-ms/cms/internal/app"
	"github.com/filip-be/folk-ms/cms/internal/domain"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

type clock struct{ t time.Time }

func (c *clock) Now() time.Time { return c.t }

type env struct {
	a      *app.App
	clock  *clock
	pub    *memory.PublicStore
	social *meta.Recorder
	ctx    context.Context
}

func orgs() []*domain.Organization {
	mk := func(id string) *domain.Organization {
		o := &domain.Organization{ID: id, Name: domain.Text{PL: id}, Timezone: "Europe/Warsaw",
			Site: domain.Site{ID: id, BaseURL: "https://" + id + ".example", PermalinkTemplate: "/{lang}/events/{shortId}-{slug}"}}
		loc, _ := time.LoadLocation("Europe/Warsaw")
		o.SetLocation(loc)
		return o
	}
	return []*domain.Organization{mk("org-a"), mk("org-b")}
}

func newEnv(t *testing.T) *env {
	t.Helper()
	a := app.New(orgs())
	e := &env{a: a, clock: &clock{time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}, pub: memory.NewPublicStore("https://store.example/pub"), social: &meta.Recorder{}, ctx: context.Background()}
	a.Docs, a.Pub, a.Audit, a.Social, a.Images, a.Clock = memory.NewDocumentStore(), e.pub, memory.NewAppendLog(), e.social, img.Processor{}, e.clock
	a.SuperAdminEmail = "Admin@Example.org"
	return e
}

func (e *env) signIn(t *testing.T, sub, email string) (domain.Principal, error) {
	t.Helper()
	u, err := e.a.SignIn(e.ctx, ports.Identity{Issuer: "https://accounts.google.com", Subject: sub, Email: email, EmailVerified: true})
	if err != nil {
		return domain.Principal{}, err
	}
	return e.a.Principal(e.ctx, u.ID, e.clock.t)
}

func must[T any](t *testing.T) func(T, error) T {
	return func(v T, err error) T {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
}

func TestSignInAndInvitations(t *testing.T) {
	e := newEnv(t)
	if _, err := e.signIn(t, "x", "stranger@example.org"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("stranger: %v", err)
	}
	sa := must[domain.Principal](t)(e.signIn(t, "sa-sub", "admin@example.org"))
	if !sa.SuperAdmin || !sa.CanManageOrg("org-b") {
		t.Fatal("super admin bound on first sign-in")
	}
	if _, err := e.signIn(t, "other-sub", "admin@example.org"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("super admin email from another identity must be refused")
	}
	inv := must[*domain.Invitation](t)(e.a.CreateInvitation(e.ctx, sa, "ola@example.org", []string{"org-a"}))
	ola := must[domain.Principal](t)(e.signIn(t, "ola-sub", "Ola@Example.org"))
	if ola.SuperAdmin || !ola.CanManageOrg("org-a") || ola.CanManageOrg("org-b") {
		t.Fatalf("moderator scoped to org-a: %+v", ola.User)
	}
	if _, err := e.a.CreateInvitation(e.ctx, ola, "x@y.org", []string{"org-a"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("only super admin invites")
	}
	invs := must[[]app.InvitationView](t)(e.a.ListInvitations(e.ctx, sa))
	if invs[0].ID != inv.ID || invs[0].EffectiveStatus != domain.InvitationBound {
		t.Fatalf("%+v", invs)
	}
	// Expired invitation.
	must[*domain.Invitation](t)(e.a.CreateInvitation(e.ctx, sa, "late@example.org", []string{"org-b"}))
	e.clock.t = e.clock.t.Add(15 * 24 * time.Hour)
	if _, err := e.signIn(t, "late-sub", "late@example.org"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("expired invitation")
	}
	// Revocation takes effect on the next request.
	must[*domain.User](t)(e.a.RevokeUser(e.ctx, sa, ola.User.ID))
	if _, err := e.a.Principal(e.ctx, ola.User.ID, e.clock.t.Add(-time.Hour)); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("revoked session")
	}
}

func publishable(venue string) domain.Content {
	return domain.Content{
		Title:    domain.Text{PL: "Bal jesienny", EN: "Autumn bal"},
		VenueID:  venue,
		Schedule: domain.Schedule{Kind: domain.KindRecurring, StartDate: "2026-10-01", StartTime: "19:00", EndTime: "22:00", Recurrence: &domain.Recurrence{Freq: domain.Weekly, Weekday: 4, EndDate: "2026-12-31"}},
	}
}

func pngImage(w, h int) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h)))
	return b.Bytes()
}

func TestEventFlowAndSnapshot(t *testing.T) {
	e := newEnv(t)
	sa := must[domain.Principal](t)(e.signIn(t, "sa", "admin@example.org"))
	must[*domain.Invitation](t)(e.a.CreateInvitation(e.ctx, sa, "b@example.org", []string{"org-b"}))
	modB := must[domain.Principal](t)(e.signIn(t, "b", "b@example.org"))

	v := must[app.VenueView](t)(e.a.CreateVenue(e.ctx, sa, "org-a", domain.Venue{Name: "Zamek", Address: "Św. Marcin 80"}))
	ev := must[app.EventView](t)(e.a.CreateEvent(e.ctx, sa, "org-a", publishable(v.ID)))
	if ev.Event.Draft.Slug != "bal-jesienny" || !domain.IsShortID(ev.Event.ShortID) || len(ev.Problems) != 0 {
		t.Fatalf("%+v", ev)
	}
	if _, err := e.a.GetEvent(e.ctx, modB, "org-a", ev.Event.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("moderator of org-b cannot read org-a")
	}
	// Second Event with the same title gets a unique slug; explicit clash fails.
	ev2 := must[app.EventView](t)(e.a.CreateEvent(e.ctx, sa, "org-a", publishable(v.ID)))
	if ev2.Event.Draft.Slug != "bal-jesienny-2" {
		t.Fatal(ev2.Event.Draft.Slug)
	}
	clash := publishable(v.ID)
	clash.Slug = "bal-jesienny"
	if _, err := e.a.UpdateEvent(e.ctx, sa, "org-a", ev2.Event.ID, ev2.ETag, clash); !domain.IsValidation(err) {
		t.Fatalf("slug clash: %v", err)
	}
	// Stale editor version conflicts.
	upd := publishable(v.ID)
	upd.Title.PL = "Bal jesienny 2026"
	ev = must[app.EventView](t)(e.a.UpdateEvent(e.ctx, sa, "org-a", ev.Event.ID, ev.ETag, upd))
	if _, err := e.a.UpdateEvent(e.ctx, sa, "org-a", ev.Event.ID, `"stale"`, upd); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("stale etag")
	}

	must[app.EventView](t)(e.a.PublishEvent(e.ctx, sa, "org-a", ev.Event.ID))
	doc := must[ports.Document](t)(e.pub.Get(e.ctx, "sites/org-a/events.json"))
	var snap domain.Snapshot
	_ = json.Unmarshal(doc.Data, &snap)
	if len(snap.Events) != 1 || snap.Events[0].Title.PL != "Bal jesienny 2026" {
		t.Fatalf("snapshot: %s", doc.Data)
	}
	if ct, cc := e.pub.Meta("sites/org-a/events.json"); ct != "application/json" || cc != "public, max-age=60" {
		t.Fatal(ct, cc)
	}

	// Venue edit freezes past Occurrences and rebuilds the snapshot.
	v2 := must[app.VenueView](t)(e.a.UpdateVenue(e.ctx, sa, "org-a", v.ID, v.ETag, domain.Venue{Name: "Nowy Zamek", Address: "ul. Nowa 1"}))
	doc = must[ports.Document](t)(e.pub.Get(e.ctx, "sites/org-a/events.json"))
	_ = json.Unmarshal(doc.Data, &snap)
	occ := snap.Events[0].Occurrences
	if snap.Venues[occ[0].VenueID].Name != "Zamek" || snap.Venues[occ[len(occ)-1].VenueID].Name != "Nowy Zamek" {
		t.Fatalf("venues: %s", doc.Data)
	}
	if _, err := e.a.UpdateVenue(e.ctx, sa, "org-a", v.ID, v.ETag, domain.Venue{Name: "X", Address: "Y"}); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("stale venue etag")
	}
	_ = v2

	// Lists.
	up := must[[]app.EventSummary](t)(e.a.ListEvents(e.ctx, sa, "org-a", app.ViewUpcoming))
	drafts := must[[]app.EventSummary](t)(e.a.ListEvents(e.ctx, sa, "org-a", app.ViewDrafts))
	if len(up) != 1 || len(drafts) != 1 {
		t.Fatalf("upcoming %d drafts %d", len(up), len(drafts))
	}

	// Duplicate copies content without dates.
	dup := must[app.EventView](t)(e.a.DuplicateEvent(e.ctx, sa, "org-a", ev.Event.ID))
	if dup.Event.Draft.Schedule.StartDate != "" || dup.Event.ShortID == ev.Event.ShortID || dup.Event.Draft.Slug == ev.Event.Draft.Slug {
		t.Fatalf("duplicate %+v", dup.Event)
	}

	// Delete of a Published Event unpublishes it; restore is Super Admin only.
	must[app.EventView](t)(e.a.DeleteEvent(e.ctx, sa, "org-a", ev.Event.ID))
	doc = must[ports.Document](t)(e.pub.Get(e.ctx, "sites/org-a/events.json"))
	if !strings.Contains(string(doc.Data), `"events":[]`) {
		t.Fatalf("delete must unpublish: %s", doc.Data)
	}
	must[*domain.Invitation](t)(e.a.CreateInvitation(e.ctx, sa, "a@example.org", []string{"org-a"}))
	modA := must[domain.Principal](t)(e.signIn(t, "a", "a@example.org"))
	if _, err := e.a.RestoreEvent(e.ctx, modA, "org-a", ev.Event.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("restore is super admin only")
	}

	// Purge after 30 days.
	e.clock.t = e.clock.t.Add(31 * 24 * time.Hour)
	n := must[int](t)(e.a.PurgeBin(e.ctx))
	if n != 1 {
		t.Fatalf("purged %d", n)
	}
	if _, err := e.a.GetEvent(e.ctx, sa, "org-a", ev.Event.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("purged event gone")
	}
	audit := must[[]app.AuditEntry](t)(e.a.ReadAudit(e.ctx, sa, e.clock.t.Format(time.DateOnly)))
	if len(audit) == 0 || audit[0].Action != "event.purged" {
		t.Fatalf("audit: %+v", audit)
	}
}

func TestSocialPosting(t *testing.T) {
	e := newEnv(t)
	sa := must[domain.Principal](t)(e.signIn(t, "sa", "admin@example.org"))
	must[*domain.Invitation](t)(e.a.CreateInvitation(e.ctx, sa, "m@example.org", []string{"org-a"}))
	mod := must[domain.Principal](t)(e.signIn(t, "m", "m@example.org"))
	const cb = "https://cms/auth/meta/callback"
	fb := func(caption string) []domain.PostDraft {
		return []domain.PostDraft{{Platform: domain.Facebook, Caption: caption}}
	}
	ig := func(caption string) []domain.PostDraft {
		return []domain.PostDraft{{Platform: domain.Instagram, Caption: caption}}
	}

	v := must[app.VenueView](t)(e.a.CreateVenue(e.ctx, mod, "org-a", domain.Venue{Name: "Zamek", Address: "Św. Marcin 80"}))
	c := publishable(v.ID)
	c.Cover = must[*domain.Image](t)(e.a.UploadImage(e.ctx, mod, "org-a", ports.ImageCover, pngImage(1200, 600)))
	ev := must[app.EventView](t)(e.a.CreateEvent(e.ctx, mod, "org-a", c))
	id := ev.Event.ID

	if _, err := e.a.RequestPost(e.ctx, mod, "org-a", id, fb("Hi"), false); !domain.IsValidation(err) {
		t.Fatal("unpublished events cannot be posted")
	}
	must[app.EventView](t)(e.a.PublishEvent(e.ctx, mod, "org-a", id))
	if _, err := e.a.RequestPost(e.ctx, mod, "org-a", id, fb("Hi"), false); !domain.IsValidation(err) {
		t.Fatal("needs a connection")
	}
	pages := must[[]app.PageInfo](t)(e.a.CompleteSocialLogin(e.ctx, mod, "org-a", "code", cb))
	st := must[app.ConnectionStatus](t)(e.a.SelectPage(e.ctx, mod, "org-a", pages[0].ID))
	if !st.Connected || st.IGUsername != "test_ig" {
		t.Fatalf("%+v", st)
	}
	if _, err := e.a.RequestPost(e.ctx, mod, "org-a", id, nil, false); !domain.IsValidation(err) {
		t.Fatal("at least one platform")
	}
	if _, err := e.a.RequestPost(e.ctx, mod, "org-a", id, fb("  "), false); !domain.IsValidation(err) {
		t.Fatal("a caption is required")
	}

	// One Meta sign-in posts on both platforms.
	pv := must[app.PostsView](t)(e.a.EventPosts(e.ctx, mod, "org-a", id))
	if !strings.Contains(pv.Captions[domain.PL], "Bal jesienny") || pv.Images[domain.Instagram] == "" {
		t.Fatalf("%+v", pv)
	}
	both := []domain.PostDraft{{Platform: domain.Facebook, Caption: pv.Captions[domain.PL]}, {Platform: domain.Instagram, Caption: "IG"}}
	req := must[*domain.PostRequest](t)(e.a.RequestPost(e.ctx, mod, "org-a", id, both, false))
	tokens := e.social.Tokens
	out := must[app.PostOutcome](t)(e.a.CompletePost(e.ctx, mod, "org-a", req.ID, "code", cb))
	if out.Refused != "" || len(out.Results) != 2 || out.Results[0].Status != "posted" || out.Results[1].Status != "posted" {
		t.Fatalf("%+v", out)
	}
	if e.social.Tokens != tokens+1 {
		t.Fatalf("one sign-in per Post Request, got %d", e.social.Tokens-tokens)
	}
	if !strings.HasSuffix(e.social.Posts[0].ImageURL, "-1600.jpg") && !strings.HasSuffix(e.social.Posts[0].ImageURL, "-960.jpg") {
		t.Fatalf("fb image %s", e.social.Posts[0].ImageURL)
	}
	if e.social.Posts[1].Platform != "instagram" || !strings.HasSuffix(e.social.Posts[1].ImageURL, "-ig.jpg") {
		t.Fatalf("%+v", e.social.Posts)
	}
	// A Post Request is used once.
	if out := must[app.PostOutcome](t)(e.a.CompletePost(e.ctx, mod, "org-a", req.ID, "code", cb)); out.Refused != app.RefusedExpired {
		t.Fatalf("replayed request: %+v", out)
	}

	// The confirmed-retry rule applies before the sign-in.
	if _, err := e.a.RequestPost(e.ctx, mod, "org-a", id, fb("again"), false); !errors.Is(err, domain.ErrNeedsConfirm) {
		t.Fatalf("re-post without confirmation: %v", err)
	}

	// A Meta account without a role on the Facebook page posts nothing.
	e.social.NoAccess = true
	req = must[*domain.PostRequest](t)(e.a.RequestPost(e.ctx, mod, "org-a", id, fb("again"), true))
	if out := must[app.PostOutcome](t)(e.a.CompletePost(e.ctx, mod, "org-a", req.ID, "code", cb)); out.Refused != app.RefusedPage || len(e.social.Posts) != 2 {
		t.Fatalf("%+v %d", out, len(e.social.Posts))
	}
	e.social.NoAccess = false

	// A Meta account that cannot reach the connected Instagram account.
	e.social.PagesFn = func() []ports.SocialPage { return []ports.SocialPage{{ID: "1001", Name: "Strona testowa"}} }
	req = must[*domain.PostRequest](t)(e.a.RequestPost(e.ctx, mod, "org-a", id, ig("IG"), true))
	out = must[app.PostOutcome](t)(e.a.CompletePost(e.ctx, mod, "org-a", req.ID, "code", cb))
	if len(out.Results) != 1 || out.Results[0].Status != "refused" || out.Results[0].Reason != app.RefusedInstagram {
		t.Fatalf("%+v", out)
	}
	e.social.PagesFn = nil

	// A Graph error is recorded as failed.
	e.social.FailIG = errors.New("boom")
	req = must[*domain.PostRequest](t)(e.a.RequestPost(e.ctx, mod, "org-a", id, ig("IG"), true))
	out = must[app.PostOutcome](t)(e.a.CompletePost(e.ctx, mod, "org-a", req.ID, "code", cb))
	if out.Results[0].Status != "failed" {
		t.Fatalf("%+v", out)
	}
	pv = must[app.PostsView](t)(e.a.EventPosts(e.ctx, mod, "org-a", id))
	if last := pv.Posts[len(pv.Posts)-1]; last.Status != domain.PostFailed || last.Error == "" {
		t.Fatalf("%+v", last)
	}
	e.social.FailIG = nil

	// Only the Moderator who made a request can complete it, within its TTL.
	req = must[*domain.PostRequest](t)(e.a.RequestPost(e.ctx, mod, "org-a", id, fb("x"), true))
	if out := must[app.PostOutcome](t)(e.a.CompletePost(e.ctx, sa, "org-a", req.ID, "code", cb)); out.Refused != app.RefusedExpired {
		t.Fatalf("another user's request: %+v", out)
	}
	e.clock.t = e.clock.t.Add(domain.PostRequestTTL + time.Minute)
	if out := must[app.PostOutcome](t)(e.a.CompletePost(e.ctx, mod, "org-a", req.ID, "code", cb)); out.Refused != app.RefusedExpired {
		t.Fatalf("expired request: %+v", out)
	}

	// A connection saved by an earlier version loses its stored token.
	legacy := `{"orgId":"org-a","pageId":"1001","pageName":"Strona testowa","igUserId":"2002","igUsername":"test_ig","tokenEnc":"c2VjcmV0","createdBy":"x","createdByEmail":"x@example.org","createdAt":"2026-11-01T12:00:00Z","needsReconnect":true}`
	if _, err := e.a.Docs.Put(e.ctx, "orgs/org-a/social/connection.json", []byte(legacy), ports.AnyETag); err != nil {
		t.Fatal(err)
	}

	// Abandoned requests and Facebook page lists are purged.
	must[*domain.PostRequest](t)(e.a.RequestPost(e.ctx, mod, "org-a", id, fb("x"), true))
	must[[]app.PageInfo](t)(e.a.CompleteSocialLogin(e.ctx, mod, "org-a", "code", cb))
	e.clock.t = e.clock.t.Add(time.Hour)
	if _, err := e.a.PurgeBin(e.ctx); err != nil {
		t.Fatal(err)
	}
	if keys, _ := e.a.Docs.List(e.ctx, "orgs/org-a/social/"); len(keys) != 1 || !strings.HasSuffix(keys[0], "/connection.json") {
		t.Fatalf("left over: %v", keys)
	}
	if st := must[app.ConnectionStatus](t)(e.a.SocialStatus(e.ctx, mod, "org-a")); !st.Connected || st.PageID != "1001" {
		t.Fatalf("legacy connection kept: %+v", st)
	}

	// Nothing the CMS stores holds a Meta token.
	keys, err := e.a.Docs.List(e.ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		d, err := e.a.Docs.Get(e.ctx, k)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(d.Data, []byte("token")) {
			t.Fatalf("%s holds a token: %s", k, d.Data)
		}
	}

	if err := e.a.Disconnect(e.ctx, sa, "org-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.a.RequestPost(e.ctx, mod, "org-a", id, fb("x"), true); !domain.IsValidation(err) {
		t.Fatal("disconnected")
	}
}
