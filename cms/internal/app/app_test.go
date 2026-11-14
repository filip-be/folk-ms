package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"slices"
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
	a       *app.App
	clock   *clock
	pub     *memory.PublicStore
	social  *meta.Recorder
	secrets *memory.Secrets
	ctx     context.Context
	// fb is what the Recorder's Facebook page lists.
	fb     []ports.FacebookEvent
	nonces int
	// linkAs is the user of the browser's current session at sign-in.
	linkAs string
}

const cb = "https://cms/auth/meta/callback"

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
	e := &env{a: a, clock: &clock{time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}, pub: memory.NewPublicStore("https://store.example/pub"),
		social: &meta.Recorder{}, secrets: memory.NewSecrets(), ctx: context.Background()}
	e.social.EventsFn = func() []ports.FacebookEvent { return slices.Clone(e.fb) }
	e.social.Cover = pngImage(1200, 600)
	a.Docs, a.Pub, a.Audit, a.Social, a.Events, a.Secrets, a.Images, a.Clock =
		memory.NewDocumentStore(), e.pub, memory.NewAppendLog(), e.social, e.social, e.secrets, img.Processor{}, e.clock
	a.SuperAdminEmail = "Admin@Example.org"
	return e
}

func (e *env) signInWith(t *testing.T, id ports.Identity) (domain.Principal, error) {
	t.Helper()
	e.nonces++
	nonce := fmt.Sprintf("nonce-%d", e.nonces)
	u, err := e.a.SignIn(e.ctx, id, nonce, e.linkAs)
	if err != nil {
		return domain.Principal{}, err
	}
	return e.a.Principal(e.ctx, u.ID, e.clock.t, nonce)
}

// signIn signs in with Google; the Principal has a Recent sign-in.
func (e *env) signIn(t *testing.T, sub, email string) (domain.Principal, error) {
	t.Helper()
	return e.signInWith(t, ports.Identity{Issuer: "https://accounts.google.com", Subject: sub, Email: email, EmailVerified: true})
}

// stale is the same session without a verified Recent sign-in (a session
// whose nonce is unknown, or one that signed in long ago).
func stale(p domain.Principal) domain.Principal {
	return domain.Principal{User: p.User, SuperAdmin: p.SuperAdmin}
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

func pngImage(w, h int) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h)))
	return b.Bytes()
}

func (e *env) advance(d time.Duration) { e.clock.t = e.clock.t.Add(d) }

// fbEvent is a single Facebook Event starting days from now.
func (e *env) fbEvent(id, name string, days int) ports.FacebookEvent {
	start := e.clock.t.Add(time.Duration(days) * 24 * time.Hour).Truncate(time.Hour)
	return ports.FacebookEvent{ID: id, Name: name, Description: "Opis " + name, Start: start, End: start.Add(3 * time.Hour),
		Place: &ports.FacebookPlace{Name: "Zamek", Street: "Św. Marcin 80", City: "Poznań"}, Updated: e.clock.t}
}

func (e *env) connect(t *testing.T, p domain.Principal, org, page string, confirm bool) app.ConnectionStatus {
	t.Helper()
	must[[]app.PageInfo](t)(e.a.CompleteSocialLogin(e.ctx, p, org, "code", cb))
	return must[app.ConnectionStatus](t)(e.a.SelectPage(e.ctx, p, org, page, confirm))
}

// syncBySignIn runs "Sync now" with Autosync off, a minute after the last.
func (e *env) syncBySignIn(t *testing.T, p domain.Principal, org string) domain.SyncResult {
	t.Helper()
	e.advance(2 * time.Minute)
	if _, err := e.a.SyncNow(e.ctx, p, org); !errors.Is(err, app.ErrNeedsMetaLogin) {
		t.Fatalf("sync now with autosync off: %v", err)
	}
	return must[domain.SyncResult](t)(e.a.CompleteSyncLogin(e.ctx, p, org, "code", cb))
}

func (e *env) autosyncOn(t *testing.T, p domain.Principal, org string) string {
	t.Helper()
	if err := e.a.StartAutosync(e.ctx, p, org); err != nil {
		t.Fatal(err)
	}
	if err := e.a.CompleteAutosync(e.ctx, p, org, "code", cb); err != nil {
		t.Fatal(err)
	}
	return e.conn(t, org).Autosync.SecretName
}

func (e *env) conn(t *testing.T, org string) *domain.Connection {
	t.Helper()
	d, err := e.a.Docs.Get(e.ctx, "orgs/"+org+"/social/connection.json")
	if err != nil {
		t.Fatal(err)
	}
	var c domain.Connection
	if err := json.Unmarshal(d.Data, &c); err != nil {
		t.Fatal(err)
	}
	return &c
}

func (e *env) secretGone(t *testing.T, name string) {
	t.Helper()
	if _, err := e.secrets.Get(e.ctx, name); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("secret %s still stored: %v", name, err)
	}
}

// noTokenStored fails if any stored document holds a Meta token.
func (e *env) noTokenStored(t *testing.T, tokens ...string) {
	t.Helper()
	keys := must[[]string](t)(e.a.Docs.List(e.ctx, ""))
	for _, k := range keys {
		d := must[ports.Document](t)(e.a.Docs.Get(e.ctx, k))
		for _, tok := range append(tokens, "page-token-", "user-token-") {
			if bytes.Contains(d.Data, []byte(tok)) {
				t.Fatalf("%s holds a token: %s", k, d.Data)
			}
		}
	}
}

// event finds an Event by its Facebook Event id, in or out of the Bin.
func (e *env) event(t *testing.T, p domain.Principal, org, fbID string) app.EventView {
	t.Helper()
	for _, view := range []string{"", app.ViewBin} {
		for _, s := range must[[]app.EventSummary](t)(e.a.ListEvents(e.ctx, p, org, view)) {
			if s.FacebookURL == domain.FacebookEventURL(fbID) {
				return must[app.EventView](t)(e.a.GetEvent(e.ctx, p, org, s.ID))
			}
		}
	}
	t.Fatalf("no event for Facebook Event %s", fbID)
	return app.EventView{}
}

func (e *env) snapshot(t *testing.T, org string) domain.Snapshot {
	t.Helper()
	d := must[ports.Document](t)(e.pub.Get(e.ctx, app.SnapshotPath(org)))
	var s domain.Snapshot
	if err := json.Unmarshal(d.Data, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func onSite(s domain.Snapshot, id string) *domain.SnapshotEvent {
	for i := range s.Events {
		if s.Events[i].ID == id {
			return &s.Events[i]
		}
	}
	return nil
}

func (e *env) calls(since int) []string { return slices.Clone(e.social.Calls[since:]) }

func TestSignInAndInvitations(t *testing.T) {
	e := newEnv(t)
	if _, err := e.signIn(t, "x", "stranger@example.org"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("stranger: %v", err)
	}
	sa := must[domain.Principal](t)(e.signIn(t, "sa-sub", "admin@example.org"))
	if !sa.SuperAdmin || !sa.CanManageOrg("org-b") || !sa.RecentlySignedIn(e.clock.t) {
		t.Fatal("super admin bound on first sign-in, recently signed in")
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
	// A session is valid only with a nonce recorded on the user document
	// (a cookie forged with the session key alone is refused), and its
	// Recent sign-in lasts 15 minutes.
	for _, nonce := range []string{"forged", ""} {
		if _, err := e.a.Principal(e.ctx, ola.User.ID, e.clock.t, nonce); !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("nonce %q: %v", nonce, err)
		}
	}
	// Access changes need the Super Admin's Recent sign-in.
	must[*domain.Invitation](t)(e.a.CreateInvitation(e.ctx, sa, "late@example.org", []string{"org-b"}))
	e.advance(16 * time.Minute)
	if ola.RecentlySignedIn(e.clock.t) {
		t.Fatal("a sign-in 16 minutes ago is not recent")
	}
	if _, err := e.a.CreateInvitation(e.ctx, sa, "mallory@example.org", []string{"org-a"}); !errors.Is(err, domain.ErrRecentSignIn) {
		t.Fatalf("invitation without a recent sign-in: %v", err)
	}
	if _, err := e.a.RevokeUser(e.ctx, sa, ola.User.ID); !errors.Is(err, domain.ErrRecentSignIn) {
		t.Fatalf("revoke without a recent sign-in: %v", err)
	}
	if _, err := e.a.SetUserOrgs(e.ctx, sa, ola.User.ID, nil); !errors.Is(err, domain.ErrRecentSignIn) {
		t.Fatalf("set orgs without a recent sign-in: %v", err)
	}
	// Expired invitation.
	e.advance(15 * 24 * time.Hour)
	sa = must[domain.Principal](t)(e.signIn(t, "sa-sub", "admin@example.org"))
	if _, err := e.signIn(t, "late-sub", "late@example.org"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("expired invitation")
	}
	// Revocation takes effect on the next request.
	must[*domain.User](t)(e.a.RevokeUser(e.ctx, sa, ola.User.ID))
	if _, err := e.a.Principal(e.ctx, ola.User.ID, e.clock.t.Add(-time.Hour), "ola-nonce"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("revoked session")
	}
}

// Sync imports upcoming Facebook Events only, one Event per Facebook Event,
// and later updates only the Facebook part.
func TestSyncImportsAndUpdates(t *testing.T) {
	e := newEnv(t)
	sa := must[domain.Principal](t)(e.signIn(t, "sa", "admin@example.org"))
	bal := e.fbEvent("100", "Bal folkowy", 10)
	bal.CoverID, bal.CoverURL, bal.TicketURL = "c1", "https://scontent.xx.fbcdn.net/c1.jpg", "https://bilety.example/bal"
	jam := e.fbEvent("200", "Jam czwartkowy", 3)
	jam.Times = []ports.FacebookEventTime{
		{ID: "201", Start: jam.Start, End: jam.End},
		{ID: "202", Start: jam.Start.Add(14 * 24 * time.Hour), End: jam.End.Add(14 * 24 * time.Hour)},
	}
	cancelled := e.fbEvent("300", "Koncert odwołany", 20)
	cancelled.Cancelled = true
	past := e.fbEvent("400", "Bal wiosenny", -10)
	e.fb = []ports.FacebookEvent{bal, jam, cancelled, past}

	if _, err := e.a.SyncNow(e.ctx, sa, "org-a"); !domain.IsValidation(err) {
		t.Fatalf("sync without a connection: %v", err)
	}
	e.connect(t, sa, "org-a", "1001", false)
	before := len(e.social.Calls)
	r := e.syncBySignIn(t, sa, "org-a")
	if r.Listed != 4 || r.Added != 3 || len(r.Errors) != 0 {
		t.Fatalf("first sync: %+v", r)
	}
	if got := e.calls(before); !slices.Equal(got, []string{"list 1001 page-token-1001"}) {
		t.Fatalf("calls: %v", got)
	}
	if all := must[[]app.EventSummary](t)(e.a.ListEvents(e.ctx, sa, "org-a", "")); len(all) != 3 {
		t.Fatalf("no history backfill: %+v", all)
	}

	// A repeating Facebook Event is one Event; each Occurrence links to its own Facebook Event.
	jv := e.event(t, sa, "org-a", "200")
	if occ := jv.Event.Facebook.Occurrences; len(occ) != 2 || occ[0].FacebookID != "201" || occ[1].FacebookID != "202" {
		t.Fatalf("occurrences: %+v", occ)
	}
	cv := e.event(t, sa, "org-a", "300")
	if !cv.Flags.Cancelled || !cv.Effective.Cancelled {
		t.Fatalf("cancelled: %+v", cv.Flags)
	}

	// The cover is imported as re-encoded variants in the public store.
	bv := e.event(t, sa, "org-a", "100")
	cover := bv.Event.Facebook.Cover
	if cover == nil || len(cover.Variants) == 0 || cover.Instagram == nil {
		t.Fatalf("cover not imported: %+v", bv.Event.Facebook)
	}
	for _, v := range append(slices.Clone(cover.Variants), *cover.Instagram) {
		if _, err := e.pub.Get(e.ctx, v.Path); err != nil || !strings.HasPrefix(v.Path, "media/org-a/") {
			t.Fatalf("variant %s: %v", v.Path, err)
		}
	}

	snap := e.snapshot(t, "org-a")
	if len(snap.Events) != 3 {
		t.Fatalf("snapshot: %+v", snap.Events)
	}
	if se := onSite(snap, jv.Event.ID); se == nil || len(se.Occurrences) != 2 ||
		se.Occurrences[1].FacebookEventURL != "https://www.facebook.com/events/202/" {
		t.Fatalf("repeating on site: %+v", se)
	}
	if se := onSite(snap, cv.Event.ID); se == nil || !se.Occurrences[0].Cancelled {
		t.Fatalf("cancelled on site: %+v", se)
	}
	if se := onSite(snap, bv.Event.ID); se == nil || se.Cover == nil || se.Price == nil || se.Price.URL != "https://bilety.example/bal" {
		t.Fatalf("bal on site: %+v", se)
	}

	// A Moderator's Override, Extras and Hide.
	bv = must[app.EventView](t)(e.a.SetOverride(e.ctx, sa, "org-a", bv.Event.ID, domain.OverrideInput{Field: domain.FieldName, Text: "Bal (nasz)"}))
	bv = must[app.EventView](t)(e.a.SetExtras(e.ctx, sa, "org-a", bv.Event.ID, bv.ETag, domain.ExtrasInput{NameEN: "Folk bal", Summary: domain.Text{PL: "Krótko"}}))
	if _, err := e.a.SetExtras(e.ctx, sa, "org-a", bv.Event.ID, `"stale"`, domain.ExtrasInput{NameEN: "x"}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale extras: %v", err)
	}
	must[app.EventView](t)(e.a.SetHidden(e.ctx, sa, "org-a", cv.Event.ID, true))
	if bv.Flags.TextOutOfDate || len(bv.Flags.OverrideChanged) != 0 {
		t.Fatalf("fresh flags: %+v", bv.Flags)
	}

	// Facebook changes the name: Sync updates the Facebook part only and flags the rest (decision 16).
	e.fb[0].Name, e.fb[0].Description = "Bal folkowy 2026", "Nowy opis"
	r = e.syncBySignIn(t, sa, "org-a")
	if r.Updated != 1 || r.Added != 0 {
		t.Fatalf("second sync: %+v", r)
	}
	bv = e.event(t, sa, "org-a", "100")
	ev := bv.Event
	if ev.Facebook.Name != "Bal folkowy 2026" || ev.Override.Name == nil || ev.Override.Name.Value != "Bal (nasz)" ||
		bv.Effective.Name.PL != "Bal (nasz)" || ev.Extras.NameEN != "Folk bal" || bv.Effective.Description.PL != "Nowy opis" {
		t.Fatalf("after second sync: %+v %+v", ev, bv.Effective)
	}
	if !slices.Equal(bv.Flags.OverrideChanged, []string{domain.FieldName}) || !bv.Flags.TextOutOfDate {
		t.Fatalf("flags: %+v", bv.Flags)
	}
	if ev.Facebook.Cover == nil || ev.Facebook.Cover.ID != cover.ID {
		t.Fatal("the same cover is kept")
	}
	if cv = e.event(t, sa, "org-a", "300"); !cv.Event.Hidden {
		t.Fatal("sync never undoes Hide")
	}
	if onSite(e.snapshot(t, "org-a"), cv.Event.ID) != nil {
		t.Fatal("hidden event on site")
	}
	// Follow Facebook again; dismiss the text flag.
	must[app.EventView](t)(e.a.SetOverride(e.ctx, sa, "org-a", ev.ID, domain.OverrideInput{Field: domain.FieldName, Follow: true}))
	bv = must[app.EventView](t)(e.a.DismissTextFlag(e.ctx, sa, "org-a", ev.ID))
	if bv.Effective.Name.PL != "Bal folkowy 2026" || bv.Flags.TextOutOfDate || len(bv.Flags.OverrideChanged) != 0 {
		t.Fatalf("follow again: %+v %+v", bv.Effective.Name, bv.Flags)
	}
	// Nothing changed on Facebook: nothing is updated.
	if r = e.syncBySignIn(t, sa, "org-a"); r.Updated != 0 || r.Added != 0 {
		t.Fatalf("idle sync: %+v", r)
	}
	e.noTokenStored(t)
}

// Decision 11: an Event is gone only when the list succeeded and the read
// by id says so.
func TestGoneRule(t *testing.T) {
	e := newEnv(t)
	sa := must[domain.Principal](t)(e.signIn(t, "sa", "admin@example.org"))
	e.fb = []ports.FacebookEvent{e.fbEvent("100", "Bal", 5), e.fbEvent("200", "Jam", 6)}
	e.connect(t, sa, "org-a", "1001", false)
	e.syncBySignIn(t, sa, "org-a")
	id := e.event(t, sa, "org-a", "100").Event.ID
	keep := e.fb[0]
	e.fb = e.fb[1:]

	// The list fails: nothing is read by id and nothing changes.
	e.social.FailList = errors.New("network down")
	before := len(e.social.Calls)
	e.advance(2 * time.Minute)
	if _, err := e.a.CompleteSyncLogin(e.ctx, sa, "org-a", "code", cb); err == nil {
		t.Fatal("a failed list fails the sync")
	}
	if got := e.calls(before); len(got) != 1 || !strings.HasPrefix(got[0], "list ") {
		t.Fatalf("calls after a failed list: %v", got)
	}
	if v := e.event(t, sa, "org-a", "100"); v.Event.DeletedAt != nil {
		t.Fatal("binned after a failed list")
	}
	st := must[app.SyncStatusView](t)(e.a.SyncStatus(e.ctx, sa, "org-a"))
	if st.LastError == "" || st.Running || st.LeaseHolder != "" {
		t.Fatalf("status after failure: %+v", st)
	}
	e.social.FailList = nil

	// The read by id fails otherwise: nothing changes.
	e.social.FailRead = errors.New("HTTP 500")
	if r := e.syncBySignIn(t, sa, "org-a"); r.Binned != 0 || len(r.Errors) != 1 {
		t.Fatalf("read error: %+v", r)
	}
	if v := e.event(t, sa, "org-a", "100"); v.Event.DeletedAt != nil {
		t.Fatal("binned after a read error")
	}
	e.social.FailRead = nil

	// Listed fine and read as gone: to the Bin.
	before = len(e.social.Calls)
	if r := e.syncBySignIn(t, sa, "org-a"); r.Binned != 1 {
		t.Fatalf("gone: %+v", r)
	}
	if got := e.calls(before); !slices.Equal(got, []string{"list 1001 page-token-1001", "read 100 page-token-1001"}) {
		t.Fatalf("calls: %v", got)
	}
	v := e.event(t, sa, "org-a", "100")
	if v.Event.DeletedAt == nil || v.Event.BinReason != domain.BinGone || !v.Flags.Binned {
		t.Fatalf("not binned: %+v", v.Event)
	}
	if onSite(e.snapshot(t, "org-a"), id) != nil {
		t.Fatal("gone event on site")
	}
	if bin := must[[]app.EventSummary](t)(e.a.ListEvents(e.ctx, sa, "org-a", app.ViewBin)); len(bin) != 1 || bin[0].ID != id {
		t.Fatalf("bin: %+v", bin)
	}
	// An Event in the Bin is not read again.
	before = len(e.social.Calls)
	e.syncBySignIn(t, sa, "org-a")
	if got := e.calls(before); len(got) != 1 {
		t.Fatalf("binned event read again: %v", got)
	}

	// Listed again: back out of the Bin.
	e.fb = append(e.fb, keep)
	if r := e.syncBySignIn(t, sa, "org-a"); r.Updated != 1 {
		t.Fatalf("back: %+v", r)
	}
	if v := e.event(t, sa, "org-a", "100"); v.Event.DeletedAt != nil || v.Event.BinReason != "" || v.Event.ID != id {
		t.Fatalf("still binned: %+v", v.Event)
	}
	if onSite(e.snapshot(t, "org-a"), id) == nil {
		t.Fatal("restored event not on site")
	}
}

// Decision 17: after the connection changes to another page, the old page's
// Events are neither read nor changed.
func TestFormerPage(t *testing.T) {
	e := newEnv(t)
	sa := must[domain.Principal](t)(e.signIn(t, "sa", "admin@example.org"))
	e.social.PagesFn = func() []ports.SocialPage {
		return []ports.SocialPage{{ID: "1001", Name: "Stara"}, {ID: "1002", Name: "Nowa"}}
	}
	e.fb = []ports.FacebookEvent{e.fbEvent("100", "Bal", 5)}
	e.connect(t, sa, "org-a", "1001", false)
	e.syncBySignIn(t, sa, "org-a")
	old := e.event(t, sa, "org-a", "100")

	must[[]app.PageInfo](t)(e.a.CompleteSocialLogin(e.ctx, sa, "org-a", "code", cb))
	if _, err := e.a.SelectPage(e.ctx, sa, "org-a", "1002", false); !errors.Is(err, domain.ErrNeedsConfirm) {
		t.Fatalf("replacing the page needs confirmation: %v", err)
	}
	if st := must[app.ConnectionStatus](t)(e.a.SelectPage(e.ctx, sa, "org-a", "1002", true)); st.PageID != "1002" {
		t.Fatalf("%+v", st)
	}

	e.fb = []ports.FacebookEvent{e.fbEvent("500", "Nowy bal", 7)}
	before := len(e.social.Calls)
	if r := e.syncBySignIn(t, sa, "org-a"); r.Added != 1 || r.Binned != 0 {
		t.Fatalf("sync of the new page: %+v", r)
	}
	if got := e.calls(before); !slices.Equal(got, []string{"list 1002 page-token-1002"}) {
		t.Fatalf("the old page's events were read: %v", got)
	}
	v := e.event(t, sa, "org-a", "100")
	if v.Event.DeletedAt != nil || v.ETag != old.ETag || v.Event.Facebook.PageID != "1001" || !v.Flags.FormerPage {
		t.Fatalf("old page's event changed: %+v %+v", v.Event, v.Flags)
	}
	if onSite(e.snapshot(t, "org-a"), v.Event.ID) == nil {
		t.Fatal("the old page's events stay on the site")
	}
	if nv := e.event(t, sa, "org-a", "500"); nv.Flags.FormerPage {
		t.Fatal("the new page's event is not from a former page")
	}
}

func manual(link string) domain.ManualInput {
	return domain.ManualInput{
		Name: domain.Text{PL: "Bal współorganizowany", EN: "Co-hosted bal"}, FacebookURL: link,
		Location:    &domain.Location{Name: "Dom Kultury"},
		Occurrences: []domain.OccurrenceInput{{StartDate: "2026-12-05", StartTime: "19:00", EndTime: "01:00"}, {StartDate: "2026-11-20", StartTime: "18:00"}},
	}
}

func TestManualEvents(t *testing.T) {
	e := newEnv(t)
	sa := must[domain.Principal](t)(e.signIn(t, "sa", "admin@example.org"))
	must[*domain.Invitation](t)(e.a.CreateInvitation(e.ctx, sa, "a@example.org", []string{"org-a"}))
	must[*domain.Invitation](t)(e.a.CreateInvitation(e.ctx, sa, "b@example.org", []string{"org-b"}))
	modA := must[domain.Principal](t)(e.signIn(t, "a", "a@example.org"))
	modB := must[domain.Principal](t)(e.signIn(t, "b", "b@example.org"))

	// The Facebook link is required and normalised.
	in := manual("")
	if _, err := e.a.CreateManualEvent(e.ctx, modA, "org-a", in); !hasProblem(err, "facebookUrl") {
		t.Fatalf("link required: %v", err)
	}
	for _, bad := range []string{"https://evil.example/events/777/", "https://www.facebook.com/groups/777/", "javascript:alert(1)"} {
		if _, err := e.a.CreateManualEvent(e.ctx, modA, "org-a", manual(bad)); !hasProblem(err, "facebookUrl") {
			t.Fatalf("%s: %v", bad, err)
		}
	}
	mv := must[app.EventView](t)(e.a.CreateManualEvent(e.ctx, modA, "org-a", manual("m.facebook.com/events/777?ref=share")))
	if mv.Event.Manual.FacebookEventID != "777" || mv.FacebookURL != "https://www.facebook.com/events/777/" || !mv.Flags.Manual {
		t.Fatalf("manual: %+v", mv)
	}
	if occ := mv.Effective.Occurrences; len(occ) != 2 || !occ[0].Start.Before(occ[1].Start) || occ[1].End.Sub(occ[1].Start) != 6*time.Hour {
		t.Fatalf("occurrences: %+v", occ)
	}
	if !domain.IsShortID(mv.Event.ShortID) || mv.Permalink[domain.PL] == "" {
		t.Fatalf("permalink: %+v", mv)
	}
	if _, err := e.a.GetEvent(e.ctx, modB, "org-a", mv.Event.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("a moderator of org-b cannot read org-a")
	}
	if se := onSite(e.snapshot(t, "org-a"), mv.Event.ID); se == nil || se.Occurrences[0].FacebookEventURL != "https://www.facebook.com/events/777/" {
		t.Fatalf("manual on site: %+v", se)
	}
	// The same Facebook Event cannot be added twice.
	if _, err := e.a.CreateManualEvent(e.ctx, modA, "org-a", manual("https://www.facebook.com/events/777/")); !hasProblem(err, "facebookUrl") {
		t.Fatalf("duplicate: %v", err)
	}
	// Another Organization may hold the same Facebook Event independently.
	must[app.EventView](t)(e.a.CreateManualEvent(e.ctx, modB, "org-b", manual("https://www.facebook.com/events/777/")))

	// Nor one Sync already imported.
	e.fb = []ports.FacebookEvent{e.fbEvent("100", "Bal", 5)}
	e.connect(t, modA, "org-a", "1001", false)
	e.syncBySignIn(t, modA, "org-a")
	if _, err := e.a.CreateManualEvent(e.ctx, modA, "org-a", manual("https://www.facebook.com/events/100/")); !hasProblem(err, "facebookUrl") {
		t.Fatalf("held by a synced event: %v", err)
	}

	// Update with If-Match; the link cannot change.
	upd := manual("https://www.facebook.com/events/777/")
	upd.Name.PL = "Bal współorganizowany 2026"
	if _, err := e.a.UpdateManualEvent(e.ctx, modA, "org-a", mv.Event.ID, `"stale"`, upd); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale: %v", err)
	}
	mv = must[app.EventView](t)(e.a.UpdateManualEvent(e.ctx, modA, "org-a", mv.Event.ID, mv.ETag, upd))
	if mv.Effective.Name.PL != "Bal współorganizowany 2026" {
		t.Fatal(mv.Effective.Name)
	}
	if _, err := e.a.UpdateManualEvent(e.ctx, modA, "org-a", mv.Event.ID, mv.ETag, manual("https://www.facebook.com/events/778/")); !hasProblem(err, "facebookUrl") {
		t.Fatalf("link changed: %v", err)
	}

	// Sync meeting the Manual Event's id leaves it alone and flags it.
	e.fb = append(e.fb, e.fbEvent("777", "Bal z Facebooka", 8))
	if r := e.syncBySignIn(t, modA, "org-a"); r.Conflicts != 1 || r.Added != 0 {
		t.Fatalf("conflict: %+v", r)
	}
	got := must[app.EventView](t)(e.a.GetEvent(e.ctx, modA, "org-a", mv.Event.ID))
	if got.Event.Source != domain.SourceManual || got.Effective.Name.PL != "Bal współorganizowany 2026" || got.Event.Facebook != nil ||
		!got.Event.SyncConflict || !got.Flags.SyncConflict {
		t.Fatalf("manual event after sync: %+v", got.Event)
	}
	if all := must[[]app.EventSummary](t)(e.a.ListEvents(e.ctx, modA, "org-a", "")); len(all) != 2 {
		t.Fatalf("one event per Facebook Event: %+v", all)
	}
	// Sync never reads a Manual Event by id.
	e.fb = e.fb[:1]
	before := len(e.social.Calls)
	e.syncBySignIn(t, modA, "org-a")
	if got := e.calls(before); len(got) != 1 {
		t.Fatalf("manual event read: %v", got)
	}

	// Remove to the Bin; synced Events cannot be removed; restore is the Super Admin's.
	synced := e.event(t, modA, "org-a", "100")
	if _, err := e.a.RemoveManualEvent(e.ctx, modA, "org-a", synced.Event.ID); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("remove synced: %v", err)
	}
	mv = must[app.EventView](t)(e.a.RemoveManualEvent(e.ctx, modA, "org-a", mv.Event.ID))
	if mv.Event.DeletedAt == nil || mv.Event.BinReason != domain.BinRemoved {
		t.Fatalf("%+v", mv.Event)
	}
	if onSite(e.snapshot(t, "org-a"), mv.Event.ID) != nil {
		t.Fatal("removed event on site")
	}
	if _, err := e.a.RestoreEvent(e.ctx, modA, "org-a", mv.Event.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("restore is super admin only")
	}
	must[app.EventView](t)(e.a.RestoreEvent(e.ctx, sa, "org-a", mv.Event.ID))
	must[app.EventView](t)(e.a.RemoveManualEvent(e.ctx, modA, "org-a", mv.Event.ID))

	// Purged after 30 days, with its index: the link can be added again.
	e.advance(31 * 24 * time.Hour)
	if n := must[int](t)(e.a.PurgeBin(e.ctx)); n != 1 {
		t.Fatalf("purged %d", n)
	}
	if _, err := e.a.GetEvent(e.ctx, sa, "org-a", mv.Event.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("purged event gone")
	}
	audit := must[[]app.AuditEntry](t)(e.a.ReadAudit(e.ctx, sa, e.clock.t.Format(time.DateOnly)))
	if len(audit) == 0 || audit[0].Action != "event.purged" {
		t.Fatalf("audit: %+v", audit)
	}
	modA = must[domain.Principal](t)(e.signIn(t, "a", "a@example.org"))
	must[app.EventView](t)(e.a.CreateManualEvent(e.ctx, modA, "org-a", manual("https://www.facebook.com/events/777/")))
}

func hasProblem(err error, field string) bool {
	var ve *domain.ValidationError
	if !errors.As(err, &ve) {
		return false
	}
	for _, p := range ve.Problems {
		if p.Field == field {
			return true
		}
	}
	return false
}

// T10: the lease and the intervals bound how often Graph is called.
func TestSyncLeaseAndIntervals(t *testing.T) {
	e := newEnv(t)
	sa := must[domain.Principal](t)(e.signIn(t, "sa", "admin@example.org"))
	e.fb = []ports.FacebookEvent{e.fbEvent("100", "Bal", 5)}
	e.connect(t, sa, "org-a", "1001", false)
	e.connect(t, sa, "org-b", "1001", false)
	secret := e.autosyncOn(t, sa, "org-a")
	token := must[string](t)(e.secrets.Get(e.ctx, secret))

	// Autosync on: "Sync now" runs at once with the stored token.
	before := len(e.social.Calls)
	if r := must[domain.SyncResult](t)(e.a.SyncNow(e.ctx, sa, "org-a")); r.Added != 1 {
		t.Fatalf("%+v", r)
	}
	if got := e.calls(before); !slices.Equal(got, []string{"list 1001 " + token}) {
		t.Fatalf("calls: %v", got)
	}
	e.advance(30 * time.Second)
	if _, err := e.a.SyncNow(e.ctx, sa, "org-a"); !errors.Is(err, domain.ErrTooSoon) {
		t.Fatalf("twice within a minute: %v", err)
	}

	// A held lease: ErrConflict, and the timer skips the Organization.
	e.advance(2 * time.Minute)
	key := "orgs/org-a/sync/status.json"
	d := must[ports.Document](t)(e.a.Docs.Get(e.ctx, key))
	var st domain.SyncStatus
	_ = json.Unmarshal(d.Data, &st)
	st.LeaseUntil, st.LeaseHolder = e.clock.t.Add(time.Minute), "other"
	data, _ := json.Marshal(st)
	must[string](t)(e.a.Docs.Put(e.ctx, key, data, d.ETag))
	if _, err := e.a.SyncNow(e.ctx, sa, "org-a"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("held lease: %v", err)
	}
	if v := must[app.SyncStatusView](t)(e.a.SyncStatus(e.ctx, sa, "org-a")); !v.Running || v.LeaseHolder != "" || !v.Autosync {
		t.Fatalf("status: %+v", v)
	}
	before = len(e.social.Calls)
	if n := must[int](t)(e.a.AutosyncAll(e.ctx)); n != 0 || len(e.social.Calls) != before {
		t.Fatalf("timer during a lease: %d %v", n, e.calls(before))
	}

	// The timer: only Organizations with Autosync on, at most every 50 minutes.
	e.advance(10 * time.Minute)
	if n := must[int](t)(e.a.AutosyncAll(e.ctx)); n != 0 || len(e.social.Calls) != before {
		t.Fatalf("timer within 50 minutes: %d %v", n, e.calls(before))
	}
	e.advance(40 * time.Minute)
	if n := must[int](t)(e.a.AutosyncAll(e.ctx)); n != 1 {
		t.Fatalf("timer after 50 minutes: %d", n)
	}
	if got := e.calls(before); !slices.Equal(got, []string{"list 1001 " + token}) {
		t.Fatalf("calls: %v", got)
	}
	if v := must[app.SyncStatusView](t)(e.a.SyncStatus(e.ctx, sa, "org-a")); v.LastRunBy != app.SyncByAutosync || v.LastSuccessAt == nil {
		t.Fatalf("status: %+v", v)
	}
	if n := must[int](t)(e.a.AutosyncAll(e.ctx)); n != 0 {
		t.Fatalf("timer right again: %d", n)
	}

	// Autosync off (org-b): "Sync now" needs a Meta sign-in, whose page token runs it.
	before = len(e.social.Calls)
	if _, err := e.a.SyncNow(e.ctx, sa, "org-b"); !errors.Is(err, app.ErrNeedsMetaLogin) {
		t.Fatalf("org-b: %v", err)
	}
	if len(e.social.Calls) != before {
		t.Fatal("nothing is read before the sign-in")
	}
	must[domain.SyncResult](t)(e.a.CompleteSyncLogin(e.ctx, sa, "org-b", "code", cb))
	if got := e.calls(before); !slices.Equal(got, []string{"list 1001 page-token-1001"}) {
		t.Fatalf("calls: %v", got)
	}
	if v := must[app.SyncStatusView](t)(e.a.SyncStatus(e.ctx, sa, "org-b")); v.LastRunBy != sa.ID() || v.Autosync {
		t.Fatalf("status: %+v", v)
	}
	if _, err := e.a.SyncNow(e.ctx, sa, "org-b"); !errors.Is(err, domain.ErrTooSoon) {
		t.Fatalf("too soon before the sign-in: %v", err)
	}
	if _, err := e.a.CompleteSyncLogin(e.ctx, sa, "org-b", "code", cb); !errors.Is(err, domain.ErrTooSoon) {
		t.Fatalf("too soon after the sign-in: %v", err)
	}
	e.noTokenStored(t, token)
}

func TestAutosync(t *testing.T) {
	e := newEnv(t)
	sa := must[domain.Principal](t)(e.signIn(t, "sa", "admin@example.org"))
	must[*domain.Invitation](t)(e.a.CreateInvitation(e.ctx, sa, "m@example.org", []string{"org-a", "org-b"}))
	m := must[domain.Principal](t)(e.signIn(t, "m", "m@example.org"))
	e.social.PagesFn = func() []ports.SocialPage {
		return []ports.SocialPage{{ID: "1001", Name: "Stara", IGUserID: "2002", IGUsername: "ig"}, {ID: "1002", Name: "Nowa"}}
	}
	e.fb = []ports.FacebookEvent{e.fbEvent("100", "Bal", 5)}

	if err := e.a.StartAutosync(e.ctx, m, "org-a"); !domain.IsValidation(err) {
		t.Fatalf("needs a connection: %v", err)
	}
	e.connect(t, m, "org-a", "1001", false)

	// A Recent sign-in is required: not without the session's nonce, not after 15 minutes.
	if err := e.a.StartAutosync(e.ctx, stale(m), "org-a"); !errors.Is(err, domain.ErrRecentSignIn) {
		t.Fatalf("start without a recent sign-in: %v", err)
	}
	if err := e.a.CompleteAutosync(e.ctx, stale(m), "org-a", "code", cb); !errors.Is(err, domain.ErrRecentSignIn) {
		t.Fatalf("complete without a recent sign-in: %v", err)
	}
	e.advance(16 * time.Minute)
	if err := e.a.StartAutosync(e.ctx, m, "org-a"); !errors.Is(err, domain.ErrRecentSignIn) {
		t.Fatalf("a sign-in 16 minutes ago: %v", err)
	}
	if st := must[app.ConnectionStatus](t)(e.a.SocialStatus(e.ctx, m, "org-a")); st.RecentSignIn || st.Autosync != nil {
		t.Fatalf("%+v", st)
	}
	m = must[domain.Principal](t)(e.signIn(t, "m", "m@example.org"))

	// On: the token goes to the secret store; the connection names it and never holds it.
	name1 := e.autosyncOn(t, m, "org-a")
	if !strings.HasPrefix(name1, "fbpage-org-a-") {
		t.Fatal(name1)
	}
	token1 := must[string](t)(e.secrets.Get(e.ctx, name1))
	c := e.conn(t, "org-a")
	if c.Autosync.By != m.ID() || c.Autosync.DataAccessExpiresAt == nil || c.Autosync.NeedsReconnect {
		t.Fatalf("%+v", c.Autosync)
	}
	e.noTokenStored(t, token1)
	if st := must[app.ConnectionStatus](t)(e.a.SocialStatus(e.ctx, m, "org-a")); st.Autosync == nil || st.Autosync.ByEmail != "m@example.org" ||
		st.Autosync.DataAccessExpiresAt == nil || !st.RecentSignIn {
		t.Fatalf("%+v", st)
	} else if b, _ := json.Marshal(st); bytes.Contains(b, []byte(token1)) || bytes.Contains(b, []byte(name1)) {
		t.Fatalf("status exposes the token or its name: %s", b)
	}

	// Off needs a Recent sign-in and deletes the secret.
	if err := e.a.DisableAutosync(e.ctx, stale(m), "org-a"); !errors.Is(err, domain.ErrRecentSignIn) {
		t.Fatalf("off without a recent sign-in: %v", err)
	}
	if err := e.a.DisableAutosync(e.ctx, m, "org-a"); err != nil {
		t.Fatal(err)
	}
	e.secretGone(t, name1)
	if c := e.conn(t, "org-a"); c.Autosync != nil || c.AutosyncLost != nil {
		t.Fatalf("%+v", c)
	}
	if _, err := e.a.SyncNow(e.ctx, m, "org-a"); !errors.Is(err, app.ErrNeedsMetaLogin) {
		t.Fatalf("off: sync needs a Meta sign-in: %v", err)
	}

	// Off then on: a new secret name, the old one never reused.
	name2 := e.autosyncOn(t, m, "org-a")
	if name2 == name1 {
		t.Fatal("secret name reused")
	}
	e.secretGone(t, name1)
	// On again (a fresh token before the data access expires): replaced, the previous deleted.
	name3 := e.autosyncOn(t, m, "org-a")
	if name3 == name2 || must[string](t)(e.secrets.Get(e.ctx, name3)) == "" {
		t.Fatal("not replaced")
	}
	e.secretGone(t, name2)

	// Replacing the page needs a Recent sign-in and confirmation, and turns Autosync off first.
	must[[]app.PageInfo](t)(e.a.CompleteSocialLogin(e.ctx, m, "org-a", "code", cb))
	if _, err := e.a.SelectPage(e.ctx, stale(m), "org-a", "1002", true); !errors.Is(err, domain.ErrRecentSignIn) {
		t.Fatalf("replace without a recent sign-in: %v", err)
	}
	if _, err := e.a.SelectPage(e.ctx, m, "org-a", "1002", false); !errors.Is(err, domain.ErrNeedsConfirm) {
		t.Fatalf("replace without confirmation: %v", err)
	}
	if e.conn(t, "org-a").Autosync == nil {
		t.Fatal("an unconfirmed replace changes nothing")
	}
	if st := must[app.ConnectionStatus](t)(e.a.SelectPage(e.ctx, m, "org-a", "1002", true)); st.PageID != "1002" || st.Autosync != nil {
		t.Fatalf("%+v", st)
	}
	e.secretGone(t, name3)

	// Disconnect deletes the secret.
	name4 := e.autosyncOn(t, m, "org-a")
	if err := e.a.Disconnect(e.ctx, stale(m), "org-a"); !errors.Is(err, domain.ErrRecentSignIn) {
		t.Fatalf("disconnect without a recent sign-in: %v", err)
	}
	if err := e.a.Disconnect(e.ctx, m, "org-a"); err != nil {
		t.Fatal(err)
	}
	e.secretGone(t, name4)
	if st := must[app.ConnectionStatus](t)(e.a.SocialStatus(e.ctx, m, "org-a")); st.Connected {
		t.Fatal("still connected")
	}

	// Decision 19: removing the Organization from the person who turned Autosync on.
	e.connect(t, m, "org-a", "1001", false)
	name5 := e.autosyncOn(t, m, "org-a")
	e.connect(t, m, "org-b", "1001", false)
	sa = must[domain.Principal](t)(e.signIn(t, "sa", "admin@example.org"))
	nameB := e.autosyncOn(t, sa, "org-b") // turned on by someone else: unaffected
	must[*domain.User](t)(e.a.SetUserOrgs(e.ctx, sa, m.ID(), []string{"org-b"}))
	e.secretGone(t, name5)
	c = e.conn(t, "org-a")
	if c.Autosync != nil || c.AutosyncLost == nil || c.AutosyncLost.Reason != "access" || c.AutosyncLost.ByEmail != "m@example.org" {
		t.Fatalf("%+v", c)
	}
	if st := must[app.ConnectionStatus](t)(e.a.SocialStatus(e.ctx, sa, "org-a")); st.AutosyncLost == nil || !st.Connected {
		t.Fatalf("banner: %+v", st)
	}
	if e.conn(t, "org-b").Autosync == nil || must[string](t)(e.secrets.Get(e.ctx, nameB)) == "" {
		t.Fatal("org-b's Autosync, turned on by the super admin, stays")
	}

	// RevokeUser does the same.
	must[*domain.User](t)(e.a.SetUserOrgs(e.ctx, sa, m.ID(), []string{"org-a", "org-b"}))
	e.advance(time.Minute)
	m = must[domain.Principal](t)(e.signIn(t, "m", "m@example.org"))
	name6 := e.autosyncOn(t, m, "org-a")
	if c := e.conn(t, "org-a"); c.AutosyncLost != nil {
		t.Fatal("turning Autosync on clears the banner")
	}
	must[*domain.User](t)(e.a.RevokeUser(e.ctx, sa, m.ID()))
	e.secretGone(t, name6)
	if c := e.conn(t, "org-a"); c.Autosync != nil || c.AutosyncLost == nil {
		t.Fatalf("%+v", c)
	}

	// A stored token Graph rejects marks the connection "needs reconnect".
	sa = must[domain.Principal](t)(e.signIn(t, "sa", "admin@example.org"))
	e.social.RejectToken = true
	if _, err := e.a.SyncNow(e.ctx, sa, "org-b"); !errors.Is(err, ports.ErrTokenRejected) {
		t.Fatalf("rejected token: %v", err)
	}
	if st := must[app.ConnectionStatus](t)(e.a.SocialStatus(e.ctx, sa, "org-b")); st.Autosync == nil || !st.Autosync.NeedsReconnect {
		t.Fatalf("%+v", st.Autosync)
	}
	e.social.RejectToken = false
	e.advance(time.Hour)
	before := len(e.social.Calls)
	if n := must[int](t)(e.a.AutosyncAll(e.ctx)); n != 0 || len(e.social.Calls) != before {
		t.Fatal("the timer skips a connection that needs reconnecting")
	}
	// Turning Autosync on again fixes it.
	sa = must[domain.Principal](t)(e.signIn(t, "sa", "admin@example.org"))
	e.autosyncOn(t, sa, "org-b")
	if n := must[int](t)(e.a.AutosyncAll(e.ctx)); n != 1 {
		t.Fatalf("after reconnect: %d", n)
	}
	e.noTokenStored(t, token1)
}

// setupPostable connects org-a and syncs one Event with a cover.
func setupPostable(t *testing.T, e *env, p domain.Principal) string {
	t.Helper()
	bal := e.fbEvent("100", "Bal jesienny", 10)
	bal.CoverID, bal.CoverURL = "c1", "https://scontent.xx.fbcdn.net/c1.jpg"
	e.fb = []ports.FacebookEvent{bal}
	e.connect(t, p, "org-a", "1001", false)
	e.syncBySignIn(t, p, "org-a")
	return e.event(t, p, "org-a", "100").Event.ID
}

func fbDraft(caption string) []domain.PostDraft {
	return []domain.PostDraft{{Platform: domain.Facebook, Caption: caption}}
}

func (e *env) postBySignIn(t *testing.T, p domain.Principal, id string, drafts []domain.PostDraft, confirm bool) app.PostOutcome {
	t.Helper()
	res := must[app.PostResultOrRequest](t)(e.a.Post(e.ctx, p, "org-a", id, drafts, confirm))
	if res.Request == nil || res.Outcome != nil {
		t.Fatalf("autosync off: a Post Request is expected: %+v", res)
	}
	return must[app.PostOutcome](t)(e.a.CompletePost(e.ctx, p, "org-a", res.Request.ID, "code", cb))
}

func TestPostingWithMetaSignIn(t *testing.T) {
	e := newEnv(t)
	sa := must[domain.Principal](t)(e.signIn(t, "sa", "admin@example.org"))
	must[*domain.Invitation](t)(e.a.CreateInvitation(e.ctx, sa, "m@example.org", []string{"org-a"}))
	mod := must[domain.Principal](t)(e.signIn(t, "m", "m@example.org"))
	id := setupPostable(t, e, mod)
	e.advance(time.Hour) // Autosync off: no Recent sign-in is needed.

	if _, err := e.a.Post(e.ctx, mod, "org-a", id, nil, false); !domain.IsValidation(err) {
		t.Fatal("at least one platform")
	}
	if _, err := e.a.Post(e.ctx, mod, "org-a", id, fbDraft("  "), false); !domain.IsValidation(err) {
		t.Fatal("a caption is required")
	}
	pv := must[app.PostsView](t)(e.a.EventPosts(e.ctx, mod, "org-a", id))
	if !pv.Postable || pv.StoredToken || !strings.Contains(pv.Captions[domain.PL], "Bal jesienny") || pv.Images[domain.Instagram] == "" {
		t.Fatalf("%+v", pv)
	}

	// One Meta sign-in posts on both platforms with its page token.
	both := []domain.PostDraft{{Platform: domain.Facebook, Caption: pv.Captions[domain.PL]}, {Platform: domain.Instagram, Caption: "IG"}}
	tokens := e.social.Tokens
	out := e.postBySignIn(t, mod, id, both, false)
	if out.Refused != "" || len(out.Results) != 2 || out.Results[0].Status != "posted" || out.Results[1].Status != "posted" {
		t.Fatalf("%+v", out)
	}
	if e.social.Tokens != tokens+1 || !slices.Equal(e.social.PostTokens, []string{"page-token-1001", "page-token-1001"}) {
		t.Fatalf("tokens: %d %v", e.social.Tokens-tokens, e.social.PostTokens)
	}
	if e.social.Posts[1].Platform != "instagram" || !strings.HasSuffix(e.social.Posts[1].ImageURL, "-ig.jpg") {
		t.Fatalf("%+v", e.social.Posts)
	}

	// The confirmed-retry rule applies before the sign-in; a Post Request is used once.
	if _, err := e.a.Post(e.ctx, mod, "org-a", id, fbDraft("again"), false); !errors.Is(err, domain.ErrNeedsConfirm) {
		t.Fatalf("re-post without confirmation: %v", err)
	}
	res := must[app.PostResultOrRequest](t)(e.a.Post(e.ctx, mod, "org-a", id, fbDraft("again"), true))
	must[app.PostOutcome](t)(e.a.CompletePost(e.ctx, mod, "org-a", res.Request.ID, "code", cb))
	if out := must[app.PostOutcome](t)(e.a.CompletePost(e.ctx, mod, "org-a", res.Request.ID, "code", cb)); out.Refused != app.RefusedExpired {
		t.Fatalf("replayed request: %+v", out)
	}

	// A Meta account without a role on the page posts nothing.
	e.social.NoAccess = true
	n := len(e.social.Posts)
	if out := e.postBySignIn(t, mod, id, fbDraft("x"), true); out.Refused != app.RefusedPage || len(e.social.Posts) != n {
		t.Fatalf("%+v", out)
	}
	e.social.NoAccess = false

	// Hidden Events cannot be posted.
	must[app.EventView](t)(e.a.SetHidden(e.ctx, mod, "org-a", id, true))
	if _, err := e.a.Post(e.ctx, mod, "org-a", id, fbDraft("x"), true); !hasProblem(err, "event") {
		t.Fatalf("hidden: %v", err)
	}
	if pv := must[app.PostsView](t)(e.a.EventPosts(e.ctx, mod, "org-a", id)); pv.Postable {
		t.Fatal("hidden is not postable")
	}
	must[app.EventView](t)(e.a.SetHidden(e.ctx, mod, "org-a", id, false))

	// The daily limit of the Organization.
	for range 20 {
		if pv := must[app.PostsView](t)(e.a.EventPosts(e.ctx, mod, "org-a", id)); pv.PostsToday >= domain.DailyPostLimit {
			break
		}
		e.postBySignIn(t, mod, id, fbDraft("x"), true)
	}
	n = len(e.social.Posts)
	if out := e.postBySignIn(t, mod, id, fbDraft("x"), true); out.Refused != app.RefusedLimit || len(e.social.Posts) != n {
		t.Fatalf("over the limit: %+v", out)
	}
	if pv := must[app.PostsView](t)(e.a.EventPosts(e.ctx, mod, "org-a", id)); pv.PostsToday != domain.DailyPostLimit {
		t.Fatalf("posts today %d", pv.PostsToday)
	}

	// Only the Moderator who made a request can complete it, within its TTL.
	e.advance(24 * time.Hour)
	res = must[app.PostResultOrRequest](t)(e.a.Post(e.ctx, mod, "org-a", id, fbDraft("x"), true))
	if out := must[app.PostOutcome](t)(e.a.CompletePost(e.ctx, sa, "org-a", res.Request.ID, "code", cb)); out.Refused != app.RefusedExpired {
		t.Fatalf("another user's request: %+v", out)
	}
	e.advance(domain.PostRequestTTL + time.Minute)
	if out := must[app.PostOutcome](t)(e.a.CompletePost(e.ctx, mod, "org-a", res.Request.ID, "code", cb)); out.Refused != app.RefusedExpired {
		t.Fatalf("expired request: %+v", out)
	}

	// A connection saved by an early version loses its encrypted token;
	// abandoned requests and page lists are purged.
	legacy := `{"orgId":"org-a","pageId":"1001","pageName":"Strona testowa","igUserId":"2002","igUsername":"test_ig","tokenEnc":"c2VjcmV0","createdBy":"x","createdByEmail":"x@example.org","createdAt":"2026-11-01T12:00:00Z"}`
	must[string](t)(e.a.Docs.Put(e.ctx, "orgs/org-a/social/connection.json", []byte(legacy), ports.AnyETag))
	must[app.PostResultOrRequest](t)(e.a.Post(e.ctx, mod, "org-a", id, fbDraft("x"), true))
	must[[]app.PageInfo](t)(e.a.CompleteSocialLogin(e.ctx, mod, "org-a", "code", cb))
	e.advance(3 * 24 * time.Hour)
	must[int](t)(e.a.PurgeBin(e.ctx))
	if keys, _ := e.a.Docs.List(e.ctx, "orgs/org-a/social/"); len(keys) != 1 || !strings.HasSuffix(keys[0], "/connection.json") {
		t.Fatalf("left over: %v", keys)
	}
	if d := must[ports.Document](t)(e.a.Docs.Get(e.ctx, "orgs/org-a/social/connection.json")); bytes.Contains(d.Data, []byte("tokenEnc")) {
		t.Fatalf("legacy token kept: %s", d.Data)
	}
	e.noTokenStored(t)

	if err := e.a.Disconnect(e.ctx, mod, "org-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.a.Post(e.ctx, mod, "org-a", id, fbDraft("x"), true); !domain.IsValidation(err) {
		t.Fatal("disconnected")
	}
}

func TestPostingWithStoredToken(t *testing.T) {
	e := newEnv(t)
	sa := must[domain.Principal](t)(e.signIn(t, "sa", "admin@example.org"))
	must[*domain.Invitation](t)(e.a.CreateInvitation(e.ctx, sa, "m@example.org", []string{"org-a"}))
	mod := must[domain.Principal](t)(e.signIn(t, "m", "m@example.org"))
	id := setupPostable(t, e, mod)
	name := e.autosyncOn(t, mod, "org-a")
	token := must[string](t)(e.secrets.Get(e.ctx, name))

	if pv := must[app.PostsView](t)(e.a.EventPosts(e.ctx, mod, "org-a", id)); !pv.StoredToken || pv.DailyLimit != domain.DailyPostLimit {
		t.Fatalf("%+v", pv)
	}
	both := []domain.PostDraft{{Platform: domain.Facebook, Caption: "FB"}, {Platform: domain.Instagram, Caption: "IG"}}
	if _, err := e.a.Post(e.ctx, stale(mod), "org-a", id, both, false); !errors.Is(err, domain.ErrRecentSignIn) {
		t.Fatalf("without a recent sign-in: %v", err)
	}
	signIns := e.social.Tokens
	res := must[app.PostResultOrRequest](t)(e.a.Post(e.ctx, mod, "org-a", id, both, false))
	if res.Request != nil || res.Outcome == nil || len(res.Outcome.Results) != 2 || res.Outcome.Results[0].Status != "posted" || res.Outcome.Results[1].Status != "posted" {
		t.Fatalf("%+v", res)
	}
	if !slices.Equal(e.social.PostTokens, []string{token, token}) || e.social.Tokens != signIns {
		t.Fatalf("posted with %v, %d Meta sign-ins", e.social.PostTokens, e.social.Tokens-signIns)
	}
	if keys, _ := e.a.Docs.List(e.ctx, "orgs/org-a/social/requests/"); len(keys) != 0 {
		t.Fatalf("no Post Request with Autosync on: %v", keys)
	}
	if _, err := e.a.Post(e.ctx, mod, "org-a", id, fbDraft("again"), false); !errors.Is(err, domain.ErrNeedsConfirm) {
		t.Fatalf("confirmed-retry rule: %v", err)
	}

	// Hidden or binned Events cannot be posted.
	cover := must[*domain.Image](t)(e.a.UploadImage(e.ctx, mod, "org-a", ports.ImageCover, pngImage(1200, 600)))
	in := manual("https://www.facebook.com/events/777/")
	in.Cover = cover
	mv := must[app.EventView](t)(e.a.CreateManualEvent(e.ctx, mod, "org-a", in))
	must[app.PostResultOrRequest](t)(e.a.Post(e.ctx, mod, "org-a", mv.Event.ID, fbDraft("manual"), false))
	must[app.EventView](t)(e.a.RemoveManualEvent(e.ctx, mod, "org-a", mv.Event.ID))
	if _, err := e.a.Post(e.ctx, mod, "org-a", mv.Event.ID, fbDraft("x"), true); !hasProblem(err, "event") {
		t.Fatalf("binned: %v", err)
	}
	must[app.EventView](t)(e.a.SetHidden(e.ctx, mod, "org-a", id, true))
	if _, err := e.a.Post(e.ctx, mod, "org-a", id, fbDraft("x"), true); !hasProblem(err, "event") {
		t.Fatalf("hidden: %v", err)
	}
	must[app.EventView](t)(e.a.SetHidden(e.ctx, mod, "org-a", id, false))

	// A token Graph rejects while posting: failed, and "needs reconnect".
	e.social.FailFB = fmt.Errorf("meta: %w", ports.ErrTokenRejected)
	res = must[app.PostResultOrRequest](t)(e.a.Post(e.ctx, mod, "org-a", id, fbDraft("x"), true))
	if res.Outcome.Results[0].Status != "failed" || !e.conn(t, "org-a").Autosync.NeedsReconnect {
		t.Fatalf("%+v %+v", res.Outcome, e.conn(t, "org-a").Autosync)
	}
	e.social.FailFB = nil

	// The daily limit applies with the stored token too.
	for range 20 {
		if pv := must[app.PostsView](t)(e.a.EventPosts(e.ctx, mod, "org-a", id)); pv.PostsToday >= domain.DailyPostLimit {
			break
		}
		must[app.PostResultOrRequest](t)(e.a.Post(e.ctx, mod, "org-a", id, fbDraft("x"), true))
	}
	n := len(e.social.PostTokens)
	if _, err := e.a.Post(e.ctx, mod, "org-a", id, fbDraft("x"), true); !hasProblem(err, "platform") || len(e.social.PostTokens) != n {
		t.Fatalf("over the limit: %v", err)
	}
	e.noTokenStored(t, token)
}

func fb(sub, email string) ports.Identity {
	return ports.Identity{Issuer: meta.Issuer, Subject: sub, Email: email, EmailVerified: email != "", NoSuperAdmin: true}
}

func TestSignInMatchesUsersByEmail(t *testing.T) {
	e := newEnv(t)
	sa := must[domain.Principal](t)(e.signIn(t, "sa-sub", "admin@example.org"))
	must[*domain.Invitation](t)(e.a.CreateInvitation(e.ctx, sa, "ola@example.org", []string{"org-a"}))
	ola := must[domain.Principal](t)(e.signIn(t, "ola-sub", "ola@example.org"))

	// Facebook with the same email is the same user, with the same access,
	// once linked from a session of that user: an email alone is not enough.
	if _, err := e.signInWith(t, fb("fb-ola", "Ola@Example.org")); !errors.Is(err, app.ErrLinkRequired) {
		t.Fatalf("unlinked facebook identity: %v", err)
	}
	e.linkAs = sa.User.ID
	if _, err := e.signInWith(t, fb("fb-ola", "Ola@Example.org")); !errors.Is(err, app.ErrLinkRequired) {
		t.Fatalf("linked from another user's session: %v", err)
	}
	e.linkAs = ola.User.ID
	viaFB := must[domain.Principal](t)(e.signInWith(t, fb("fb-ola", "Ola@Example.org")))
	e.linkAs = ""
	if viaFB.User.ID != ola.User.ID || !viaFB.CanManageOrg("org-a") || !viaFB.RecentlySignedIn(e.clock.t) {
		t.Fatalf("matched %+v", viaFB.User)
	}
	if len(viaFB.User.Identities) != 1 || viaFB.User.Identities[0].Subject != "fb-ola" {
		t.Fatalf("identities %+v", viaFB.User.Identities)
	}
	must[domain.Principal](t)(e.signInWith(t, fb("fb-ola", "ola@example.org")))
	day := e.clock.t.Format("2006-01-02")
	matched := 0
	for _, a := range must[[]app.AuditEntry](t)(e.a.ReadAudit(e.ctx, sa, day)) {
		if a.Action == "identity.matched" {
			matched++
		}
	}
	if matched != 1 {
		t.Fatalf("identity.matched logged %d times", matched)
	}

	// No email, or an email nobody has: refused.
	if _, err := e.signInWith(t, fb("fb-x", "")); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("no email: %v", err)
	}
	if _, err := e.signInWith(t, fb("fb-y", "nobody@example.org")); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("stranger: %v", err)
	}

	// An Invitation can be redeemed through Facebook; Google then matches.
	must[*domain.Invitation](t)(e.a.CreateInvitation(e.ctx, sa, "jan@example.org", []string{"org-b"}))
	jan := must[domain.Principal](t)(e.signInWith(t, fb("fb-jan", "jan@example.org")))
	if !jan.CanManageOrg("org-b") {
		t.Fatal("invitation via facebook")
	}
	if g := must[domain.Principal](t)(e.signIn(t, "jan-google", "jan@example.org")); g.User.ID != jan.User.ID {
		t.Fatal("google matches the facebook user")
	}

	// A revoked user matched by email needs a new Invitation, bound to them.
	must[*domain.User](t)(e.a.RevokeUser(e.ctx, sa, ola.User.ID))
	if _, err := e.signInWith(t, fb("fb-ola", "ola@example.org")); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("revoked: %v", err)
	}
	must[*domain.Invitation](t)(e.a.CreateInvitation(e.ctx, sa, "ola@example.org", []string{"org-b"}))
	back := must[domain.Principal](t)(e.signInWith(t, fb("fb-ola2", "ola@example.org")))
	if back.User.ID != ola.User.ID || !back.CanManageOrg("org-b") || back.CanManageOrg("org-a") {
		t.Fatalf("re-invited %+v", back.User)
	}
}

func TestFacebookNeverSuperAdmin(t *testing.T) {
	e := newEnv(t)
	// Not even before the Super Admin is bound.
	if _, err := e.signInWith(t, fb("fb-sa", "admin@example.org")); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("unbound: %v", err)
	}
	sa := must[domain.Principal](t)(e.signIn(t, "sa-sub", "admin@example.org"))
	if _, err := e.signInWith(t, fb("fb-sa", "admin@example.org")); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("bound: %v", err)
	}
	// The Super Admin's Google email changes; the binding is by user id.
	moved := must[domain.Principal](t)(e.signIn(t, "sa-sub", "new@example.org"))
	if moved.User.ID != sa.User.ID || !moved.SuperAdmin {
		t.Fatal("still the super admin")
	}
	if _, err := e.signInWith(t, fb("fb-new", "new@example.org")); !errors.Is(err, app.ErrSuperAdminIdentity) {
		t.Fatalf("changed email: %v", err)
	}
	if _, err := e.signIn(t, "other-google", "new@example.org"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("another google identity: %v", err)
	}
}

// Regressions found in review: a refused post with Autosync on uses none
// of the daily limit; choosing the same page again keeps Autosync; an Event
// wrongly thought gone comes back even when it is long over.
func TestReviewRegressions(t *testing.T) {
	e := newEnv(t)
	sa := must[domain.Principal](t)(e.signIn(t, "sa", "admin@example.org"))
	must[*domain.Invitation](t)(e.a.CreateInvitation(e.ctx, sa, "m@example.org", []string{"org-a"}))
	mod := must[domain.Principal](t)(e.signIn(t, "m", "m@example.org"))
	id := setupPostable(t, e, mod)
	name := e.autosyncOn(t, mod, "org-a")

	must[app.EventView](t)(e.a.SetHidden(e.ctx, mod, "org-a", id, true))
	if _, err := e.a.Post(e.ctx, mod, "org-a", id, fbDraft("x"), true); err == nil {
		t.Fatal("hidden")
	}
	if pv := must[app.PostsView](t)(e.a.EventPosts(e.ctx, mod, "org-a", id)); pv.PostsToday != 0 {
		t.Fatalf("a refused post used the limit: %d", pv.PostsToday)
	}

	before := e.conn(t, "org-a").Autosync
	e.connect(t, mod, "org-a", e.conn(t, "org-a").PageID, false)
	after := e.conn(t, "org-a").Autosync
	if after == nil || after.SecretName != name {
		t.Fatalf("same page chosen again turned Autosync off: %+v -> %+v", before, after)
	}
	if _, err := e.secrets.Get(e.ctx, name); err != nil {
		t.Fatalf("secret deleted: %v", err)
	}
}

func TestGoneEventListedAgainAfterItEnded(t *testing.T) {
	e := newEnv(t)
	sa := must[domain.Principal](t)(e.signIn(t, "sa", "admin@example.org"))
	e.fb = []ports.FacebookEvent{e.fbEvent("100", "Bal", 1)}
	e.connect(t, sa, "org-a", "1001", false)
	e.syncBySignIn(t, sa, "org-a")
	fe := e.fb[0]
	e.fb = nil // missing from the list, and gone when read by id
	if r := e.syncBySignIn(t, sa, "org-a"); r.Binned != 1 {
		t.Fatalf("%+v", r)
	}
	e.advance(5 * 24 * time.Hour) // long over
	e.fb = []ports.FacebookEvent{fe}
	e.syncBySignIn(t, sa, "org-a")
	if v := e.event(t, sa, "org-a", "100"); v.Event.DeletedAt != nil {
		t.Fatal("listed again: not gone")
	}
}

// flakySecrets fails the next Delete calls, as Key Vault may.
type flakySecrets struct {
	ports.SecretStore
	failDeletes int
}

func (f *flakySecrets) Delete(ctx context.Context, name string) error {
	if f.failDeletes > 0 {
		f.failDeletes--
		return errors.New("key vault: HTTP 503")
	}
	return f.SecretStore.Delete(ctx, name)
}

// Review finding 1: a failed secret delete must leave Autosync on, naming
// the secret, so turning it off again really deletes it (T11).
func TestAutosyncOffRetriesAFailedDelete(t *testing.T) {
	e := newEnv(t)
	sa := must[domain.Principal](t)(e.signIn(t, "sa", "admin@example.org"))
	e.connect(t, sa, "org-a", "1001", false)
	name := e.autosyncOn(t, sa, "org-a")
	flaky := &flakySecrets{SecretStore: e.a.Secrets, failDeletes: 1}
	e.a.Secrets = flaky
	if err := e.a.DisableAutosync(e.ctx, sa, "org-a"); err == nil {
		t.Fatal("the failed delete is reported")
	}
	if as := e.conn(t, "org-a").Autosync; as == nil || as.SecretName != name {
		t.Fatalf("the connection must keep naming the secret: %+v", as)
	}
	if err := e.a.DisableAutosync(e.ctx, sa, "org-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.a.Secrets.Get(e.ctx, name); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("secret still there: %v", err)
	}
	if e.conn(t, "org-a").Autosync != nil {
		t.Fatal("Autosync off")
	}
}

// failingLog is an Audit Log that refuses every line.
type failingLog struct{ ports.AppendLog }

func (failingLog) Append(context.Context, string, []byte) error {
	return errors.New("append blob full")
}

// The stored token is read only when it will be used, never unrecorded,
// never past Meta's data access, and only under a name of its own
// Organization (T8, T10, T11).
func TestStoredTokenIsGuarded(t *testing.T) {
	e := newEnv(t)
	sa := must[domain.Principal](t)(e.signIn(t, "sa", "admin@example.org"))
	e.connect(t, sa, "org-a", "1001", false)
	e.autosyncOn(t, sa, "org-a")

	// Repeated "Sync now" and timer calls inside the interval read no secret.
	must[domain.SyncResult](t)(e.a.SyncNow(e.ctx, sa, "org-a"))
	if e.secrets.Gets != 1 {
		t.Fatalf("first run: %d reads", e.secrets.Gets)
	}
	for range 20 {
		if _, err := e.a.SyncNow(e.ctx, sa, "org-a"); !errors.Is(err, domain.ErrTooSoon) {
			t.Fatalf("again: %v", err)
		}
		_, _ = e.a.AutosyncAll(e.ctx)
	}
	if e.secrets.Gets != 1 {
		t.Fatalf("calls inside the interval read the secret %d times", e.secrets.Gets)
	}

	// Each use is in the Audit Log before the token is read.
	log := string(must[[]byte](t)(e.a.Audit.Read(e.ctx, e.clock.t.Format(time.DateOnly))))
	if !strings.Contains(log, `"action":"autosync.token_used"`) || !strings.Contains(log, `"purpose":"sync"`) {
		t.Fatalf("audit: %s", log)
	}
	// No Audit Log, no token.
	audit := e.a.Audit
	e.a.Audit = failingLog{audit}
	e.advance(2 * time.Minute)
	if _, err := e.a.SyncNow(e.ctx, sa, "org-a"); err == nil || e.secrets.Gets != 1 {
		t.Fatalf("sync without audit: %v, %d reads", err, e.secrets.Gets)
	}
	e.a.Audit = audit

	// A connection naming another Organization's secret is refused.
	c := e.conn(t, "org-a")
	good := c.Autosync.SecretName
	for _, name := range []string{"fbpage-org-b-0123456789abcdef", "session-key", "fbpage-org-a-x"} {
		c.Autosync.SecretName = name
		must[string](t)(e.a.Docs.Put(e.ctx, "orgs/org-a/social/connection.json", must[[]byte](t)(json.Marshal(c)), ports.AnyETag))
		e.advance(2 * time.Minute)
		if _, err := e.a.SyncNow(e.ctx, sa, "org-a"); !errors.Is(err, domain.ErrInvalidState) || e.secrets.Gets != 1 {
			t.Fatalf("secret %q: %v, %d reads", name, err, e.secrets.Gets)
		}
	}
	c.Autosync.SecretName = good

	// Past Meta's data access the token is not used and a reconnect is asked for.
	past := e.clock.t.Add(-time.Minute)
	c.Autosync.DataAccessExpiresAt = &past
	must[string](t)(e.a.Docs.Put(e.ctx, "orgs/org-a/social/connection.json", must[[]byte](t)(json.Marshal(c)), ports.AnyETag))
	e.advance(2 * time.Minute)
	if _, err := e.a.SyncNow(e.ctx, sa, "org-a"); !errors.Is(err, domain.ErrInvalidState) || e.secrets.Gets != 1 {
		t.Fatalf("expired: %v, %d reads", err, e.secrets.Gets)
	}
	if !e.conn(t, "org-a").Autosync.NeedsReconnect {
		t.Fatal("expired data access asks for a reconnect")
	}
}

// The daily purge deletes token secrets no connection names once they are
// an hour old, and keeps the one in use (T11).
func TestOrphanedSecretsAreSwept(t *testing.T) {
	e := newEnv(t)
	sa := must[domain.Principal](t)(e.signIn(t, "sa", "admin@example.org"))
	e.connect(t, sa, "org-a", "1001", false)
	inUse := e.autosyncOn(t, sa, "org-a")
	for _, n := range []string{"fbpage-org-a-0000000000000001", "fbpage-org-b-0000000000000002"} {
		if err := e.secrets.Set(e.ctx, n, "orphan-token"); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.secrets.Set(e.ctx, "fbpage-org-b-0000000000000003", "fresh-token"); err != nil {
		t.Fatal(err)
	}
	if err := e.secrets.Set(e.ctx, "session-key", "not-a-token"); err != nil {
		t.Fatal(err)
	}
	old := e.clock.t.Add(-2 * time.Hour)
	for _, n := range []string{inUse, "fbpage-org-a-0000000000000001", "fbpage-org-b-0000000000000002", "session-key"} {
		e.secrets.SetCreated(n, old)
	}
	e.secrets.SetCreated("fbpage-org-b-0000000000000003", e.clock.t.Add(-time.Minute))
	must[int](t)(e.a.PurgeBin(e.ctx))
	left := must[[]ports.SecretInfo](t)(e.secrets.List(e.ctx, ""))
	var names []string
	for _, s := range left {
		names = append(names, s.Name)
	}
	slices.Sort(names)
	want := []string{"fbpage-org-b-0000000000000003", inUse, "session-key"}
	slices.Sort(want)
	if !slices.Equal(names, want) {
		t.Fatalf("left %v, want %v", names, want)
	}
}
