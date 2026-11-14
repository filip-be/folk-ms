package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/app"
	"github.com/filip-be/folk-ms/cms/internal/domain"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

const (
	maxJSON  = 1 << 20
	maxImage = 16 << 20
	// uploadWait is how long an upload waits for the upload slot.
	uploadWait = 20 * time.Second
)

func decode(w http.ResponseWriter, r *http.Request, v any) error {
	ct := r.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		return &badRequest{msg: msg("nieprawidłowe żądanie: oczekiwano JSON", "bad request: JSON expected")}
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSON))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if _, ok := err.(*http.MaxBytesError); ok {
			return err
		}
		return &badRequest{msg: msg("nieprawidłowe żądanie: "+err.Error(), "bad request: "+err.Error()), cause: err}
	}
	return nil
}

func ok(w http.ResponseWriter, v any, err error) error {
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, v)
	return nil
}

func (s *Server) routes(m *http.ServeMux) {
	m.Handle("GET /api/me", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		return ok(w, s.App.Me(p), nil)
	}))

	// Events.
	m.Handle("GET /api/orgs/{org}/events", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		v, err := s.App.ListEvents(r.Context(), p, r.PathValue("org"), r.URL.Query().Get("view"))
		return ok(w, v, err)
	}))
	// Adding an Event by hand: a Manual Event (decision 20).
	m.Handle("POST /api/orgs/{org}/events", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		var in domain.ManualInput
		if err := decode(w, r, &in); err != nil {
			return err
		}
		v, err := s.App.CreateManualEvent(r.Context(), p, r.PathValue("org"), in)
		return ok(w, v, err)
	}))
	m.Handle("GET /api/orgs/{org}/events/{id}", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		v, err := s.App.GetEvent(r.Context(), p, r.PathValue("org"), r.PathValue("id"))
		return ok(w, v, err)
	}))
	m.Handle("PUT /api/orgs/{org}/events/{id}/manual", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		var in domain.ManualInput
		if err := decode(w, r, &in); err != nil {
			return err
		}
		v, err := s.App.UpdateManualEvent(r.Context(), p, r.PathValue("org"), r.PathValue("id"), r.Header.Get("If-Match"), in)
		return ok(w, v, err)
	}))
	m.Handle("PUT /api/orgs/{org}/events/{id}/extras", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		var in domain.ExtrasInput
		if err := decode(w, r, &in); err != nil {
			return err
		}
		v, err := s.App.SetExtras(r.Context(), p, r.PathValue("org"), r.PathValue("id"), r.Header.Get("If-Match"), in)
		return ok(w, v, err)
	}))
	m.Handle("POST /api/orgs/{org}/events/{id}/override", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		var in domain.OverrideInput
		if err := decode(w, r, &in); err != nil {
			return err
		}
		v, err := s.App.SetOverride(r.Context(), p, r.PathValue("org"), r.PathValue("id"), in)
		return ok(w, v, err)
	}))
	m.Handle("POST /api/orgs/{org}/events/{id}/{action}", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		org, id, ctx := r.PathValue("org"), r.PathValue("id"), r.Context()
		switch r.PathValue("action") {
		case "hide":
			v, err := s.App.SetHidden(ctx, p, org, id, true)
			return ok(w, v, err)
		case "show":
			v, err := s.App.SetHidden(ctx, p, org, id, false)
			return ok(w, v, err)
		case "dismiss-text":
			v, err := s.App.DismissTextFlag(ctx, p, org, id)
			return ok(w, v, err)
		case "delete":
			v, err := s.App.RemoveManualEvent(ctx, p, org, id)
			return ok(w, v, err)
		case "restore":
			v, err := s.App.RestoreEvent(ctx, p, org, id)
			return ok(w, v, err)
		}
		return domain.ErrNotFound
	}))
	m.Handle("GET /api/orgs/{org}/events/{id}/posts", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		v, err := s.App.EventPosts(r.Context(), p, r.PathValue("org"), r.PathValue("id"))
		return ok(w, v, err)
	}))
	// Posting with Autosync off starts a Post Request and answers the Meta
	// sign-in URL; the posts are sent from /auth/meta/callback. With Autosync
	// on it posts at once and answers the outcome.
	m.Handle("POST /api/orgs/{org}/events/{id}/posts", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		var in struct {
			Posts   []domain.PostDraft `json:"posts"`
			Confirm bool               `json:"confirm"`
		}
		if err := decode(w, r, &in); err != nil {
			return err
		}
		org, id := r.PathValue("org"), r.PathValue("id")
		res, err := s.App.Post(r.Context(), p, org, id, in.Posts, in.Confirm)
		if err != nil {
			return err
		}
		if res.Outcome != nil {
			return ok(w, map[string]any{"outcome": res.Outcome}, nil)
		}
		return s.startMetaLogin(w, p, metaState{Org: org, Request: res.Request.ID, Event: id})
	}))

	// Sync.
	m.Handle("GET /api/orgs/{org}/sync", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		v, err := s.App.SyncStatus(r.Context(), p, r.PathValue("org"))
		return ok(w, v, err)
	}))
	// "Sync now": with Autosync on it runs and answers the result; with it
	// off it answers the Meta sign-in URL, whose callback runs the Sync.
	m.Handle("POST /api/orgs/{org}/sync", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		org := r.PathValue("org")
		res, err := s.App.SyncNow(r.Context(), p, org)
		if errors.Is(err, app.ErrNeedsMetaLogin) {
			return s.startMetaLogin(w, p, metaState{Org: org, Sync: true})
		}
		return ok(w, map[string]any{"result": res}, err)
	}))

	// Images: the raw file is the body.
	m.Handle("POST /api/orgs/{org}/images", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "image/") {
			return &badRequest{msg: msg("nieprawidłowe żądanie: oczekiwano pliku obrazu", "bad request: image file expected")}
		}
		// One upload at a time per instance, taken before the body is read,
		// so waiting uploads hold no image in memory (T15).
		select {
		case s.uploads <- struct{}{}:
			defer func() { <-s.uploads }()
		case <-r.Context().Done():
			return r.Context().Err()
		case <-time.After(uploadWait):
			w.Header().Set("Retry-After", "10")
			writeJSON(w, http.StatusTooManyRequests, errorBody{Error: "rate_limited", Message: msg("trwa przesyłanie innego obrazu – spróbuj za chwilę", "another image is being uploaded – try again in a moment")})
			return nil
		}
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxImage))
		if err != nil {
			return err
		}
		v, err := s.App.UploadImage(r.Context(), p, r.PathValue("org"), ports.ImageKind(r.URL.Query().Get("kind")), data)
		return ok(w, v, err)
	}))

	// Social Account Connection.
	m.Handle("GET /api/orgs/{org}/social", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		v, err := s.App.SocialStatus(r.Context(), p, r.PathValue("org"))
		return ok(w, v, err)
	}))
	m.Handle("POST /api/orgs/{org}/social/connect", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		return s.startMetaLogin(w, p, metaState{Org: r.PathValue("org")})
	}))
	m.Handle("POST /api/orgs/{org}/social/select", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		var in struct {
			PageID  string `json:"pageId"`
			Confirm bool   `json:"confirm"`
		}
		if err := decode(w, r, &in); err != nil {
			return err
		}
		v, err := s.App.SelectPage(r.Context(), p, r.PathValue("org"), in.PageID, in.Confirm)
		return ok(w, v, err)
	}))
	// Autosync on needs a Meta sign-in that issues the token; off deletes it.
	m.Handle("POST /api/orgs/{org}/social/autosync", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		var in struct {
			On bool `json:"on"`
		}
		if err := decode(w, r, &in); err != nil {
			return err
		}
		org := r.PathValue("org")
		if !in.On {
			return ok(w, map[string]bool{"ok": true}, s.App.DisableAutosync(r.Context(), p, org))
		}
		if err := s.App.StartAutosync(r.Context(), p, org); err != nil {
			return err
		}
		return s.startMetaLogin(w, p, metaState{Org: org, Autosync: true})
	}))
	m.Handle("POST /api/orgs/{org}/social/disconnect", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		return ok(w, map[string]bool{"ok": true}, s.App.Disconnect(r.Context(), p, r.PathValue("org")))
	}))

	// Super Admin.
	m.Handle("GET /api/users", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		v, err := s.App.ListUsers(r.Context(), p)
		return ok(w, v, err)
	}))
	m.Handle("PUT /api/users/{id}/orgs", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		var in struct {
			Orgs []string `json:"orgs"`
		}
		if err := decode(w, r, &in); err != nil {
			return err
		}
		v, err := s.App.SetUserOrgs(r.Context(), p, r.PathValue("id"), in.Orgs)
		return ok(w, v, err)
	}))
	m.Handle("POST /api/users/{id}/revoke", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		v, err := s.App.RevokeUser(r.Context(), p, r.PathValue("id"))
		return ok(w, v, err)
	}))
	m.Handle("GET /api/invitations", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		v, err := s.App.ListInvitations(r.Context(), p)
		return ok(w, v, err)
	}))
	m.Handle("POST /api/invitations", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		var in struct {
			Email string   `json:"email"`
			Orgs  []string `json:"orgs"`
		}
		if err := decode(w, r, &in); err != nil {
			return err
		}
		v, err := s.App.CreateInvitation(r.Context(), p, in.Email, in.Orgs)
		return ok(w, v, err)
	}))
	m.Handle("POST /api/invitations/{id}/revoke", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		v, err := s.App.RevokeInvitation(r.Context(), p, r.PathValue("id"))
		return ok(w, v, err)
	}))
	m.Handle("GET /api/audit", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		v, err := s.App.AuditDays(r.Context(), p)
		return ok(w, v, err)
	}))
	m.Handle("GET /api/audit/{day}", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		v, err := s.App.ReadAudit(r.Context(), p, r.PathValue("day"))
		return ok(w, v, err)
	}))
	m.Handle("POST /api/admin/purge", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		if !p.SuperAdmin {
			return domain.ErrForbidden
		}
		n, err := s.App.PurgeBin(r.Context())
		return ok(w, map[string]int{"purged": n}, err)
	}))
	m.Handle("POST /api/admin/autosync", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		if !p.SuperAdmin {
			return domain.ErrForbidden
		}
		n, err := s.App.AutosyncAll(r.Context())
		return ok(w, map[string]int{"synced": n}, err)
	}))
	m.Handle("POST /api/admin/rebuild", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		if !p.SuperAdmin {
			return domain.ErrForbidden
		}
		return ok(w, map[string]bool{"ok": true}, s.App.RebuildAll(r.Context()))
	}))
	m.Handle("/api/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, s.Log, domain.ErrNotFound)
	}))
}
