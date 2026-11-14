// Package app holds the use cases. Every use case takes the acting
// Principal and checks its access to the Organization it touches.
package app

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/domain"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// App wires the use cases to their ports.
type App struct {
	Docs   ports.DocumentStore
	Pub    ports.PublicStore
	Audit  ports.AppendLog
	Social ports.SocialPublisher
	// Events reads Facebook Events and issues Autosync's page token.
	Events ports.FacebookEvents
	// Secrets keeps the Facebook page tokens of Organizations with Autosync on.
	Secrets ports.SecretStore
	Images  ports.ImageProcessor
	// VideoInfo reads Vimeo thumbnails when a Video is saved.
	VideoInfo ports.VideoInfo
	Clock     ports.Clock
	Rand      io.Reader
	Log       *slog.Logger

	SuperAdminEmail string

	orgs    []*domain.Organization
	orgByID map[string]*domain.Organization
}

// New builds an App. orgs come from config/sites.json.
func New(orgs []*domain.Organization) *App {
	a := &App{orgs: orgs, orgByID: map[string]*domain.Organization{}, Clock: ports.SystemClock{}, Rand: rand.Reader, Log: slog.Default()}
	for _, o := range orgs {
		a.orgByID[o.ID] = o
	}
	return a
}

// Orgs lists the configured Organizations.
func (a *App) Orgs() []*domain.Organization { return a.orgs }

// Org returns an Organization by id.
func (a *App) Org(id string) (*domain.Organization, bool) {
	o, ok := a.orgByID[id]
	return o, ok
}

func (a *App) now() time.Time { return a.Clock.Now().UTC() }

func (a *App) newID() string {
	id, err := domain.NewID(a.Rand)
	if err != nil {
		panic(err) // crypto/rand failing is not recoverable
	}
	return id
}

// authorize checks the principal may manage org and returns it.
func (a *App) authorize(p domain.Principal, org string) (*domain.Organization, error) {
	o, ok := a.orgByID[org]
	if !ok {
		return nil, fmt.Errorf("%w: organization", domain.ErrNotFound)
	}
	if !p.CanManageOrg(org) {
		return nil, domain.ErrForbidden
	}
	return o, nil
}

func requireSuperAdmin(p domain.Principal) error {
	if p.User == nil || p.User.Revoked || !p.SuperAdmin {
		return domain.ErrForbidden
	}
	return nil
}

// storeErr maps port errors to domain errors.
func storeErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ports.ErrNotFound), errors.Is(err, ports.ErrInvalidKey):
		return fmt.Errorf("%w: %v", domain.ErrNotFound, err)
	case errors.Is(err, ports.ErrPrecondition):
		return fmt.Errorf("%w: %v", domain.ErrConflict, err)
	}
	return err
}

func load[T any](ctx context.Context, s ports.DocumentStore, key string) (*T, string, error) {
	d, err := s.Get(ctx, key)
	if err != nil {
		return nil, "", storeErr(err)
	}
	var v T
	if err := json.Unmarshal(d.Data, &v); err != nil {
		return nil, "", fmt.Errorf("decode %s: %w", key, err)
	}
	return &v, d.ETag, nil
}

func save(ctx context.Context, s ports.DocumentStore, key string, v any, etag string) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	tag, err := s.Put(ctx, key, data, etag)
	return tag, storeErr(err)
}

// mutate loads a document, applies fn and saves it with If-Match, retrying
// on concurrent changes. fn must be safe to run more than once.
func mutate[T any](ctx context.Context, s ports.DocumentStore, key string, fn func(*T) error) (*T, string, error) {
	for range 5 {
		v, etag, err := load[T](ctx, s, key)
		if err != nil {
			return nil, "", err
		}
		if err := fn(v); err != nil {
			return nil, "", err
		}
		tag, err := save(ctx, s, key, v, etag)
		if errors.Is(err, domain.ErrConflict) {
			continue
		}
		return v, tag, err
	}
	return nil, "", fmt.Errorf("%w: too many concurrent changes to %s", domain.ErrConflict, key)
}

// loadAll loads every document under prefix in parallel.
func loadAll[T any](ctx context.Context, s ports.DocumentStore, prefix string) ([]*T, error) {
	keys, err := s.List(ctx, prefix)
	if err != nil {
		return nil, err
	}
	out := make([]*T, len(keys))
	errs := make([]error, len(keys))
	sem := make(chan struct{}, 16)
	var wg sync.WaitGroup
	for i, k := range keys {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			v, _, err := load[T](ctx, s, k)
			if errors.Is(err, domain.ErrNotFound) {
				return // deleted meanwhile
			}
			out[i], errs[i] = v, err
		}()
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return slices.DeleteFunc(out, func(v *T) bool { return v == nil }), nil
}

// loadEvent loads one Event of an Organization. The id comes from a
// request, so it must have the shape of an Event id, and the document must
// say it is that Event of that Organization (T13: a crafted id never reaches
// another Organization's documents, whatever the store does with it).
func (a *App) loadEvent(ctx context.Context, org, id string) (*domain.Event, string, error) {
	if !domain.ValidID(id) {
		return nil, "", fmt.Errorf("%w: event", domain.ErrNotFound)
	}
	e, etag, err := load[domain.Event](ctx, a.Docs, eventKey(org, id))
	if err != nil {
		return nil, "", err
	}
	if err := ownEvent(e, org, id); err != nil {
		return nil, "", err
	}
	return e, etag, nil
}

// ownEvent checks that a loaded Event is the one its key names.
func ownEvent(e *domain.Event, org, id string) error {
	if e.OrgID != org || e.ID != id {
		return fmt.Errorf("%w: event %s/%s holds %s/%s", domain.ErrNotFound, org, id, e.OrgID, e.ID)
	}
	return nil
}

// Keys of the documents in the private store.
func eventKey(org, id string) string    { return "orgs/" + org + "/events/" + id + ".json" }
func eventsPrefix(org string) string    { return "orgs/" + org + "/events/" }
func shortIDKey(org, sid string) string { return "orgs/" + org + "/shortids/" + sid + ".json" }

// fbIndexKey is the create-only index document that makes one Facebook
// Event id map to one Event of the Organization.
func fbIndexKey(org, fbID string) string    { return "orgs/" + org + "/facebook-events/" + fbID + ".json" }
func syncStatusKey(org string) string       { return "orgs/" + org + "/sync/status.json" }
func postCounterKey(org, day string) string { return "orgs/" + org + "/social/daily/" + day + ".json" }
func postsKey(org, eventID string) string   { return "orgs/" + org + "/posts/" + eventID + ".json" }
func connectionKey(org string) string       { return "orgs/" + org + "/social/connection.json" }
func pendingPagesPrefix(org string) string  { return "orgs/" + org + "/social/pending-" }
func pendingPagesKey(org, user string) string {
	return pendingPagesPrefix(org) + user + ".json"
}
func postRequestsPrefix(org string) string { return "orgs/" + org + "/social/requests/" }

func postRequestKey(org, id string) string { return postRequestsPrefix(org) + id + ".json" }
func userKey(id string) string             { return "users/" + id + ".json" }
func invitationKey(id string) string       { return "invitations/" + id + ".json" }

const superAdminKey = "system/superadmin.json"

// SnapshotPath is where a Site's snapshot lives in the public store.
func SnapshotPath(siteID string) string { return "sites/" + siteID + "/events.json" }

// AuditEntry is one line of the Audit Log.
type AuditEntry struct {
	Time       time.Time      `json:"time"`
	Actor      string         `json:"actor"`
	ActorEmail string         `json:"actorEmail,omitempty"`
	Action     string         `json:"action"`
	Org        string         `json:"org,omitempty"`
	Target     string         `json:"target,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
}

// auditStrict appends an Audit Log line and reports failure, for actions
// that must not happen unrecorded (each use of a stored token).
func (a *App) auditStrict(ctx context.Context, p domain.Principal, action, org, target string, details map[string]any) error {
	e := AuditEntry{Time: a.now(), Actor: p.ID(), Action: action, Org: org, Target: target, Details: details}
	if p.User != nil {
		e.ActorEmail = p.User.Email
	}
	if e.Actor == "" {
		e.Actor = "system"
	}
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return a.Audit.Append(ctx, e.Time.Format(time.DateOnly), append(line, '\n'))
}

func (a *App) audit(ctx context.Context, p domain.Principal, action, org, target string, details map[string]any) {
	e := AuditEntry{Time: a.now(), Actor: p.ID(), Action: action, Org: org, Target: target, Details: details}
	if p.User != nil {
		e.ActorEmail = p.User.Email
	}
	if e.Actor == "" {
		e.Actor = "system"
	}
	line, err := json.Marshal(e)
	if err == nil {
		err = a.Audit.Append(ctx, e.Time.Format(time.DateOnly), append(line, '\n'))
	}
	if err != nil {
		// The change itself succeeded; losing an audit line is logged loudly.
		a.Log.Error("audit append failed", "action", action, "org", org, "target", target, "err", err)
	}
}

// system is the principal of timer-triggered work.
var system = domain.Principal{}
