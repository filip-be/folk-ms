package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/filip-be/folk-ms/cms/internal/domain"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

const (
	maxJSON  = 1 << 20
	maxImage = 16 << 20
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
	m.Handle("POST /api/orgs/{org}/events", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		var c domain.Content
		if err := decode(w, r, &c); err != nil {
			return err
		}
		v, err := s.App.CreateEvent(r.Context(), p, r.PathValue("org"), c)
		return ok(w, v, err)
	}))
	m.Handle("POST /api/orgs/{org}/occurrences/preview", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		var c domain.Content
		if err := decode(w, r, &c); err != nil {
			return err
		}
		occ, probs, err := s.App.PreviewOccurrences(p, r.PathValue("org"), c)
		return ok(w, map[string]any{"occurrences": occ, "problems": probs}, err)
	}))
	m.Handle("GET /api/orgs/{org}/events/{id}", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		v, err := s.App.GetEvent(r.Context(), p, r.PathValue("org"), r.PathValue("id"))
		return ok(w, v, err)
	}))
	m.Handle("PUT /api/orgs/{org}/events/{id}", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		var c domain.Content
		if err := decode(w, r, &c); err != nil {
			return err
		}
		v, err := s.App.UpdateEvent(r.Context(), p, r.PathValue("org"), r.PathValue("id"), r.Header.Get("If-Match"), c)
		return ok(w, v, err)
	}))
	m.Handle("POST /api/orgs/{org}/events/{id}/{action}", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		org, id, ctx := r.PathValue("org"), r.PathValue("id"), r.Context()
		switch r.PathValue("action") {
		case "publish":
			v, err := s.App.PublishEvent(ctx, p, org, id)
			return ok(w, v, err)
		case "unpublish":
			v, err := s.App.UnpublishEvent(ctx, p, org, id)
			return ok(w, v, err)
		case "delete":
			v, err := s.App.DeleteEvent(ctx, p, org, id)
			return ok(w, v, err)
		case "restore":
			v, err := s.App.RestoreEvent(ctx, p, org, id)
			return ok(w, v, err)
		case "duplicate":
			v, err := s.App.DuplicateEvent(ctx, p, org, id)
			return ok(w, v, err)
		}
		return domain.ErrNotFound
	}))
	m.Handle("GET /api/orgs/{org}/events/{id}/posts", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		v, err := s.App.EventPosts(r.Context(), p, r.PathValue("org"), r.PathValue("id"))
		return ok(w, v, err)
	}))
	m.Handle("POST /api/orgs/{org}/events/{id}/posts", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		var in struct {
			Platform domain.Platform `json:"platform"`
			Caption  string          `json:"caption"`
			Confirm  bool            `json:"confirm"`
		}
		if err := decode(w, r, &in); err != nil {
			return err
		}
		v, err := s.App.Post(r.Context(), p, r.PathValue("org"), r.PathValue("id"), in.Platform, in.Caption, in.Confirm)
		return ok(w, v, err)
	}))

	// Venues.
	m.Handle("GET /api/orgs/{org}/venues", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		v, err := s.App.ListVenues(r.Context(), p, r.PathValue("org"))
		return ok(w, v, err)
	}))
	m.Handle("POST /api/orgs/{org}/venues", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		var in domain.Venue
		if err := decode(w, r, &in); err != nil {
			return err
		}
		v, err := s.App.CreateVenue(r.Context(), p, r.PathValue("org"), in)
		return ok(w, v, err)
	}))
	m.Handle("PUT /api/orgs/{org}/venues/{id}", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		var in domain.Venue
		if err := decode(w, r, &in); err != nil {
			return err
		}
		v, err := s.App.UpdateVenue(r.Context(), p, r.PathValue("org"), r.PathValue("id"), r.Header.Get("If-Match"), in)
		return ok(w, v, err)
	}))
	m.Handle("POST /api/orgs/{org}/venues/{id}/archive", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		var in struct {
			Archived bool `json:"archived"`
		}
		if err := decode(w, r, &in); err != nil {
			return err
		}
		v, err := s.App.SetVenueArchived(r.Context(), p, r.PathValue("org"), r.PathValue("id"), in.Archived)
		return ok(w, v, err)
	}))

	// Images: the raw file is the body.
	m.Handle("POST /api/orgs/{org}/images", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "image/") {
			return &badRequest{msg: msg("nieprawidłowe żądanie: oczekiwano pliku obrazu", "bad request: image file expected")}
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
	m.Handle("POST /api/orgs/{org}/social/connect", s.h(s.startMetaLogin))
	m.Handle("POST /api/orgs/{org}/social/select", s.h(func(w http.ResponseWriter, r *http.Request, p domain.Principal) error {
		var in struct {
			PageID string `json:"pageId"`
		}
		if err := decode(w, r, &in); err != nil {
			return err
		}
		v, err := s.App.SelectPage(r.Context(), p, r.PathValue("org"), in.PageID)
		return ok(w, v, err)
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
