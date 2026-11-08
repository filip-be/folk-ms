package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/domain"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// MediaCacheControl is the Cache-Control of content-hashed media.
const MediaCacheControl = "public, max-age=31536000, immutable"

// UploadImage processes an upload into JPEG variants under immutable,
// content-hashed names in the public store.
func (a *App) UploadImage(ctx context.Context, p domain.Principal, org string, kind ports.ImageKind, data []byte) (*domain.Image, error) {
	if _, err := a.authorize(p, org); err != nil {
		return nil, err
	}
	if kind != ports.ImageCover && kind != ports.ImageSocial {
		return nil, &domain.ValidationError{Problems: []domain.FieldError{{Field: "kind", Message: domain.Text{PL: "nieznany rodzaj obrazu", EN: "unknown image kind"}}}}
	}
	rends, err := a.Images.Process(data, kind)
	if err != nil {
		// Adapters describe a rejected upload with Localized; other errors
		// are shown as they are.
		var ue interface{ Localized() (pl, en string) }
		if errors.As(err, &ue) {
			pl, en := ue.Localized()
			return nil, domain.Problem("image", pl, en)
		}
		return nil, domain.Problem("image", err.Error(), err.Error())
	}
	sum := sha256.Sum256(append([]byte(kind+"\x00"), data...))
	hash := hex.EncodeToString(sum[:8])
	img := &domain.Image{ID: hash}
	for _, r := range rends {
		path := fmt.Sprintf("media/%s/%s-%s.jpg", org, hash, r.Name)
		if _, err := a.Pub.Put(ctx, path, r.Data, "image/jpeg", MediaCacheControl, ""); err != nil && !errors.Is(err, ports.ErrPrecondition) {
			return nil, err // ErrPrecondition: the same upload exists already
		}
		v := domain.ImageVariant{Width: r.Width, Height: r.Height, Path: path}
		if r.Name == "ig" {
			img.Instagram = &v
		} else {
			img.Variants = append(img.Variants, v)
		}
	}
	a.audit(ctx, p, "image.uploaded", org, hash, map[string]any{"kind": kind, "bytes": len(data)})
	return img, nil
}

// ConnectionStatus describes an Organization's Social Account Connection.
type ConnectionStatus struct {
	Connected      bool       `json:"connected"`
	PageName       string     `json:"pageName,omitempty"`
	PageID         string     `json:"pageId,omitempty"`
	IGUsername     string     `json:"igUsername,omitempty"`
	CreatedByEmail string     `json:"createdByEmail,omitempty"`
	CreatedAt      *time.Time `json:"createdAt,omitempty"`
	PendingPages   []PageInfo `json:"pendingPages,omitempty"`
}

// PageInfo is a Facebook Page to choose from.
type PageInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	IGUsername string `json:"igUsername,omitempty"`
}

// pendingPages are the Pages a Moderator may choose from after signing in
// with Meta. They hold no token.
type pendingPages struct {
	ExpiresAt time.Time          `json:"expiresAt"`
	Pages     []ports.SocialPage `json:"pages"`
}

// SocialStatus returns the connection status of an Organization, including
// Pages waiting for the current user to choose one.
func (a *App) SocialStatus(ctx context.Context, p domain.Principal, org string) (ConnectionStatus, error) {
	if _, err := a.authorize(p, org); err != nil {
		return ConnectionStatus{}, err
	}
	st := ConnectionStatus{}
	c, _, err := load[domain.Connection](ctx, a.Docs, connectionKey(org))
	switch {
	case err == nil:
		st.Connected, st.PageName, st.PageID, st.IGUsername = true, c.PageName, c.PageID, c.IGUsername
		st.CreatedByEmail, st.CreatedAt = c.CreatedByEmail, &c.CreatedAt
	case !errors.Is(err, domain.ErrNotFound):
		return st, err
	}
	pages, err := a.pendingPages(ctx, org, p.ID())
	if err != nil {
		return st, err
	}
	for _, pg := range pages {
		st.PendingPages = append(st.PendingPages, PageInfo{ID: pg.ID, Name: pg.Name, IGUsername: pg.IGUsername})
	}
	return st, nil
}

// SocialLoginURL returns where to send the Moderator to sign in with Meta,
// to connect a Page or to complete a Post Request.
func (a *App) SocialLoginURL(p domain.Principal, org, state, redirectURI string) (string, error) {
	if _, err := a.authorize(p, org); err != nil {
		return "", err
	}
	return a.Social.LoginURL(state, redirectURI), nil
}

// CompleteSocialLogin lists the Pages the Moderator has a role on and keeps
// that list (no tokens) for 15 minutes until they choose one. The user
// token is used for this one call and dropped.
func (a *App) CompleteSocialLogin(ctx context.Context, p domain.Principal, org, code, redirectURI string) ([]PageInfo, error) {
	if _, err := a.authorize(p, org); err != nil {
		return nil, err
	}
	token, err := a.Social.UserToken(ctx, code, redirectURI)
	if err != nil {
		return nil, fmt.Errorf("meta login: %w", err)
	}
	pages, err := a.Social.Pages(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("meta pages: %w", err)
	}
	if _, err := save(ctx, a.Docs, pendingPagesKey(org, p.ID()), pendingPages{ExpiresAt: a.now().Add(15 * time.Minute), Pages: pages}, ports.AnyETag); err != nil {
		return nil, err
	}
	out := []PageInfo{}
	for _, pg := range pages {
		out = append(out, PageInfo{ID: pg.ID, Name: pg.Name, IGUsername: pg.IGUsername})
	}
	return out, nil
}

func (a *App) pendingPages(ctx context.Context, org, user string) ([]ports.SocialPage, error) {
	key := pendingPagesKey(org, user)
	pp, _, err := load[pendingPages](ctx, a.Docs, key)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if a.now().After(pp.ExpiresAt) {
		_ = a.Docs.Delete(ctx, key, "")
		return nil, nil
	}
	return pp.Pages, nil
}

// SelectPage makes a chosen Page the Organization's connection, replacing
// any previous one.
func (a *App) SelectPage(ctx context.Context, p domain.Principal, org, pageID string) (ConnectionStatus, error) {
	if _, err := a.authorize(p, org); err != nil {
		return ConnectionStatus{}, err
	}
	pages, err := a.pendingPages(ctx, org, p.ID())
	if err != nil {
		return ConnectionStatus{}, err
	}
	var page *ports.SocialPage
	for i := range pages {
		if pages[i].ID == pageID {
			page = &pages[i]
		}
	}
	if page == nil {
		return ConnectionStatus{}, fmt.Errorf("%w: page not available, sign in with Meta again", domain.ErrNotFound)
	}
	c := domain.Connection{
		OrgID: org, PageID: page.ID, PageName: page.Name, IGUserID: page.IGUserID, IGUsername: page.IGUsername,
		CreatedBy: p.ID(), CreatedByEmail: p.User.Email, CreatedAt: a.now(),
	}
	if _, err := save(ctx, a.Docs, connectionKey(org), c, ports.AnyETag); err != nil {
		return ConnectionStatus{}, err
	}
	_ = a.Docs.Delete(ctx, pendingPagesKey(org, p.ID()), "")
	a.audit(ctx, p, "social.connected", org, page.ID, map[string]any{"page": page.Name, "instagram": page.IGUsername})
	return a.SocialStatus(ctx, p, org)
}

// Disconnect removes the Organization's connection. Any of its Moderators
// (and the Super Admin) may do it.
func (a *App) Disconnect(ctx context.Context, p domain.Principal, org string) error {
	if _, err := a.authorize(p, org); err != nil {
		return err
	}
	if err := a.Docs.Delete(ctx, connectionKey(org), ""); err != nil {
		return storeErr(err)
	}
	a.audit(ctx, p, "social.disconnected", org, "", nil)
	return nil
}

// PostsView is what the Post dialog shows.
type PostsView struct {
	Published    bool                       `json:"published"`
	Captions     map[domain.Lang]string     `json:"captions"`
	Posts        []domain.SocialPost        `json:"posts"`
	Images       map[domain.Platform]string `json:"images"`
	Connection   ConnectionStatus           `json:"connection"`
	CanInstagram bool                       `json:"canInstagram"`
}

func imageFor(c *domain.Content, pl domain.Platform) string {
	if pl == domain.Instagram {
		if c.SocialImage != nil && c.SocialImage.Instagram != nil {
			return c.SocialImage.Instagram.Path
		}
		if c.Cover != nil && c.Cover.Instagram != nil {
			return c.Cover.Instagram.Path
		}
		return ""
	}
	if c.Cover == nil || len(c.Cover.Variants) == 0 {
		return ""
	}
	return c.Cover.Variants[len(c.Cover.Variants)-1].Path // the largest
}

// EventPosts returns an Event's Social Posts and generated Captions.
func (a *App) EventPosts(ctx context.Context, p domain.Principal, org, id string) (PostsView, error) {
	o, err := a.authorize(p, org)
	if err != nil {
		return PostsView{}, err
	}
	e, _, err := load[domain.Event](ctx, a.Docs, eventKey(org, id))
	if err != nil {
		return PostsView{}, err
	}
	venues, err := a.venueMap(ctx, org)
	if err != nil {
		return PostsView{}, err
	}
	conn, err := a.SocialStatus(ctx, p, org)
	if err != nil {
		return PostsView{}, err
	}
	v := PostsView{
		Published: e.Published != nil && e.DeletedAt == nil, Posts: []domain.SocialPost{}, Connection: conn,
		Captions: map[domain.Lang]string{
			domain.PL: domain.GenerateCaption(o, e, venues, domain.PL, a.now()),
			domain.EN: domain.GenerateCaption(o, e, venues, domain.EN, a.now()),
		},
		Images:       map[domain.Platform]string{},
		CanInstagram: conn.IGUsername != "",
	}
	if c := e.Published; c != nil {
		for _, pl := range []domain.Platform{domain.Facebook, domain.Instagram} {
			if path := imageFor(c, pl); path != "" {
				v.Images[pl] = a.Pub.URL(path)
			}
		}
	}
	if posts, _, err := load[domain.EventPosts](ctx, a.Docs, postsKey(org, id)); err == nil {
		v.Posts = posts.Posts
	} else if !errors.Is(err, domain.ErrNotFound) {
		return v, err
	}
	return v, nil
}

func problem(field, pl, en string) error {
	return domain.Problem(field, pl, en)
}

// postable loads a Published Event and the Organization's connection, and
// checks each platform can be posted: Instagram needs a linked account, and
// each platform needs its image.
func (a *App) postable(ctx context.Context, org, id string, platforms []domain.Platform) (*domain.Event, *domain.Connection, error) {
	e, _, err := load[domain.Event](ctx, a.Docs, eventKey(org, id))
	if err != nil {
		return nil, nil, err
	}
	if e.Published == nil || e.DeletedAt != nil {
		return nil, nil, problem("event", "tylko opublikowane wydarzenia można ogłaszać", "only published events can be announced")
	}
	conn, _, err := load[domain.Connection](ctx, a.Docs, connectionKey(org))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil, problem("connection", "połącz najpierw stronę na Facebooku", "connect the Facebook Page first")
	}
	if err != nil {
		return nil, nil, err
	}
	for _, pl := range platforms {
		if pl == domain.Instagram && conn.IGUserID == "" {
			return nil, nil, problem("connection", "strona nie ma połączonego konta Instagram", "the Page has no connected Instagram account")
		}
		if imageFor(e.Published, pl) == "" {
			return nil, nil, problem("image", "dodaj zdjęcie główne i opublikuj wydarzenie ponownie", "add a cover image and publish the event again")
		}
	}
	return e, conn, nil
}

func platformsOf(drafts []domain.PostDraft) []domain.Platform {
	out := make([]domain.Platform, len(drafts))
	for i, d := range drafts {
		out[i] = d.Platform
	}
	return out
}

// RequestPost starts posting a Published Event on one or both platforms. It
// checks everything that can be checked without Meta, including the
// confirmed-retry rule, and keeps the request while the Moderator signs in
// with Meta; CompletePost finishes it.
func (a *App) RequestPost(ctx context.Context, p domain.Principal, org, id string, drafts []domain.PostDraft, confirmed bool) (*domain.PostRequest, error) {
	if _, err := a.authorize(p, org); err != nil {
		return nil, err
	}
	req, err := domain.NewPostRequest(a.newID(), org, id, p.ID(), drafts, confirmed, a.now())
	if err != nil {
		return nil, err
	}
	if _, _, err := a.postable(ctx, org, id, platformsOf(req.Drafts)); err != nil {
		return nil, err
	}
	ep, _, err := load[domain.EventPosts](ctx, a.Docs, postsKey(org, id))
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	if ep != nil {
		for _, d := range req.Drafts {
			if err := ep.CheckRetry(d.Platform, confirmed); err != nil {
				return nil, err
			}
		}
	}
	if _, err := save(ctx, a.Docs, postRequestKey(org, req.ID), req, ""); err != nil {
		return nil, err
	}
	return req, nil
}

// Reasons why a Post Request, or one platform of it, was not sent to Meta.
const (
	RefusedCancelled = "cancelled" // the Moderator cancelled the Meta sign-in
	RefusedExpired   = "expired"   // the request is gone, expired or not theirs
	RefusedLogin     = "login"     // the Meta sign-in could not be completed
	RefusedPage      = "page"      // their Meta account has no role on the Page
	RefusedInvalid   = "invalid"   // the Event or connection changed meanwhile
	RefusedInstagram = "instagram" // their Meta account cannot reach the Instagram account
	RefusedQuota     = "quota"     // the daily Instagram publishing limit is used up
	RefusedConfirm   = "confirm"   // a post appeared meanwhile: confirm to post again
	RefusedError     = "error"     // the CMS could not record the attempt
)

// PlatformResult is what happened on one platform: "posted", "failed" or
// "refused" (with the reason).
type PlatformResult struct {
	Platform domain.Platform `json:"platform"`
	Status   string          `json:"status"`
	Reason   string          `json:"reason,omitempty"`
}

// PostOutcome is the result of a Post Request. Refused is set when nothing
// was sent.
type PostOutcome struct {
	Refused string           `json:"refused,omitempty"`
	Results []PlatformResult `json:"results,omitempty"`
}

// CancelPost drops a Post Request whose Meta sign-in was cancelled.
func (a *App) CancelPost(ctx context.Context, p domain.Principal, org, requestID string) {
	if _, err := a.authorize(p, org); err != nil {
		return
	}
	if req, _, err := load[domain.PostRequest](ctx, a.Docs, postRequestKey(org, requestID)); err == nil && req.UserID == p.ID() {
		_ = a.Docs.Delete(ctx, postRequestKey(org, requestID), "")
	}
}

// CompletePost finishes a Post Request after the Meta sign-in: it redeems
// the code for a short-lived user token, gets the token of the
// Organization's Page (which proves the Moderator has a role on it), posts
// each draft and drops both tokens. Every post is recorded as pending
// before its Graph call, so a crash leaves an unknown outcome that is
// retried only after the Moderator confirms.
func (a *App) CompletePost(ctx context.Context, p domain.Principal, org, requestID, code, redirectURI string) (PostOutcome, error) {
	if _, err := a.authorize(p, org); err != nil {
		return PostOutcome{}, err
	}
	key := postRequestKey(org, requestID)
	req, _, err := load[domain.PostRequest](ctx, a.Docs, key)
	if errors.Is(err, domain.ErrNotFound) {
		return PostOutcome{Refused: RefusedExpired}, nil
	}
	if err != nil {
		return PostOutcome{}, err
	}
	if req.UserID != p.ID() || req.OrgID != org {
		return PostOutcome{Refused: RefusedExpired}, nil
	}
	// A request is used once: whoever deletes it goes on.
	if err := a.Docs.Delete(ctx, key, ""); err != nil {
		if errors.Is(storeErr(err), domain.ErrNotFound) {
			return PostOutcome{Refused: RefusedExpired}, nil
		}
		return PostOutcome{}, err
	}
	if req.Expired(a.now()) {
		return PostOutcome{Refused: RefusedExpired}, nil
	}
	// From here on, the work runs to completion even if the Moderator's
	// request is cancelled: a cancelled Graph call would be recorded as
	// failed although Meta may have published it, inviting a duplicate.
	// The Graph client has its own timeout.
	ctx = context.WithoutCancel(ctx)
	userToken, err := a.Social.UserToken(ctx, code, redirectURI)
	if err != nil {
		a.Log.Warn("meta login", "err", err)
		return PostOutcome{Refused: RefusedLogin}, nil
	}
	e, conn, err := a.postable(ctx, org, req.EventID, platformsOf(req.Drafts))
	if domain.IsValidation(err) || errors.Is(err, domain.ErrNotFound) {
		return PostOutcome{Refused: RefusedInvalid}, nil
	}
	if err != nil {
		return PostOutcome{}, err
	}
	pageToken, page, err := a.Social.PageToken(ctx, userToken, conn.PageID)
	if errors.Is(err, ports.ErrNoPageAccess) {
		a.audit(ctx, p, "social.post_refused", org, req.EventID, map[string]any{"reason": RefusedPage, "page": conn.PageID})
		return PostOutcome{Refused: RefusedPage}, nil
	}
	if err != nil {
		a.Log.Warn("meta page token", "err", err)
		return PostOutcome{Refused: RefusedLogin}, nil
	}
	out := PostOutcome{}
	for _, d := range req.Drafts {
		out.Results = append(out.Results, a.postOne(ctx, p, org, e, conn, page, pageToken, d, req.Confirmed))
	}
	return out, nil
}

// postOne posts one draft with a Page token held in memory only.
func (a *App) postOne(ctx context.Context, p domain.Principal, org string, e *domain.Event, conn *domain.Connection, page ports.SocialPage, token string, d domain.PostDraft, confirmed bool) PlatformResult {
	refuse := func(reason string) PlatformResult {
		a.audit(ctx, p, "social.post_refused", org, e.ID, map[string]any{"platform": d.Platform, "reason": reason})
		return PlatformResult{Platform: d.Platform, Status: "refused", Reason: reason}
	}
	target := conn.PageID
	if d.Platform == domain.Instagram {
		// The Moderator's Meta account must reach the connected Instagram
		// account through the Page.
		if page.IGUserID == "" || page.IGUserID != conn.IGUserID {
			return refuse(RefusedInstagram)
		}
		target = conn.IGUserID
		used, limit, err := a.Social.InstagramQuota(ctx, conn.IGUserID, token)
		if err == nil && limit > 0 && used >= limit {
			return refuse(RefusedQuota)
		}
	}
	imgPath := imageFor(e.Published, d.Platform)

	postID := a.newID()
	key := postsKey(org, e.ID)
	begin := func(ep *domain.EventPosts) error {
		_, err := ep.Begin(postID, d.Platform, d.Caption, imgPath, p.ID(), confirmed, a.now())
		return err
	}
	_, _, err := mutate(ctx, a.Docs, key, begin)
	if errors.Is(err, domain.ErrNotFound) {
		ep := &domain.EventPosts{EventID: e.ID, OrgID: org}
		if err = begin(ep); err == nil {
			_, err = save(ctx, a.Docs, key, ep, "")
		}
	}
	if errors.Is(err, domain.ErrNeedsConfirm) || errors.Is(err, domain.ErrConflict) {
		return refuse(RefusedConfirm) // another post appeared meanwhile
	}
	if err != nil {
		a.Log.Error("social post record", "org", org, "event", e.ID, "err", err)
		return refuse(RefusedError)
	}

	imageURL := a.Pub.URL(imgPath)
	var res ports.PostResult
	var postErr error
	if d.Platform == domain.Facebook {
		res, postErr = a.Social.PostFacebookPhoto(ctx, conn.PageID, token, imageURL, d.Caption)
	} else {
		res, postErr = a.Social.PostInstagramImage(ctx, conn.IGUserID, token, imageURL, d.Caption)
	}
	_, _, err = mutate(ctx, a.Docs, key, func(ep *domain.EventPosts) error {
		sp := ep.Find(postID)
		if sp == nil {
			return fmt.Errorf("%w: post record vanished", domain.ErrNotFound)
		}
		if postErr != nil {
			sp.Fail(postErr.Error(), a.now())
		} else {
			sp.Succeed(res.ID, res.URL, a.now())
		}
		return nil
	})
	if err != nil {
		// The record stays pending: the Moderator confirms before a retry.
		a.Log.Error("social post outcome not recorded", "org", org, "event", e.ID, "posted", postErr == nil, "err", err)
	}
	details := map[string]any{"platform": d.Platform, "target": target, "url": res.URL}
	if postErr != nil {
		details["error"] = postErr.Error()
		a.audit(ctx, p, "social.post_failed", org, e.ID, details)
		return PlatformResult{Platform: d.Platform, Status: string(domain.PostFailed)}
	}
	a.audit(ctx, p, "social.posted", org, e.ID, details)
	return PlatformResult{Platform: d.Platform, Status: string(domain.PostPosted)}
}

// purgeSocial removes expired Post Requests and Page lists of an
// Organization; neither holds a secret. It also rewrites a connection saved
// by an earlier version, which kept an encrypted Page token, without it.
func (a *App) purgeSocial(ctx context.Context, org string, now time.Time) error {
	var errs []error
	if d, err := a.Docs.Get(ctx, connectionKey(org)); err == nil && bytes.Contains(d.Data, []byte(`"tokenEnc"`)) {
		var c domain.Connection
		if err := json.Unmarshal(d.Data, &c); err != nil {
			errs = append(errs, err)
		} else if _, err := save(ctx, a.Docs, connectionKey(org), c, d.ETag); err != nil {
			errs = append(errs, err)
		} else {
			a.audit(ctx, system, "social.token_removed", org, c.PageID, nil)
		}
	} else if err != nil && !errors.Is(err, ports.ErrNotFound) {
		errs = append(errs, err)
	}
	drop := func(key string, expired bool, err error) {
		if err == nil && expired {
			err = a.Docs.Delete(ctx, key, "")
		}
		if err != nil && !errors.Is(storeErr(err), domain.ErrNotFound) {
			errs = append(errs, err)
		}
	}
	keys, err := a.Docs.List(ctx, postRequestsPrefix(org))
	errs = append(errs, err)
	for _, k := range keys {
		r, _, err := load[domain.PostRequest](ctx, a.Docs, k)
		drop(k, err == nil && r.Expired(now), err)
	}
	keys, err = a.Docs.List(ctx, pendingPagesPrefix(org))
	errs = append(errs, err)
	for _, k := range keys {
		pp, _, err := load[pendingPages](ctx, a.Docs, k)
		drop(k, err == nil && now.After(pp.ExpiresAt), err)
	}
	return errors.Join(errs...)
}
