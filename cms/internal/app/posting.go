package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/domain"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// PostsView is what the Post dialog shows.
type PostsView struct {
	Postable     bool                       `json:"postable"`
	Captions     map[domain.Lang]string     `json:"captions"`
	Posts        []domain.SocialPost        `json:"posts"`
	Images       map[domain.Platform]string `json:"images"`
	Connection   ConnectionStatus           `json:"connection"`
	CanInstagram bool                       `json:"canInstagram"`
	// StoredToken: Autosync is on, so posting uses the stored token and
	// needs a Recent sign-in instead of a Meta sign-in.
	StoredToken bool `json:"storedToken"`
	PostsToday  int  `json:"postsToday"`
	DailyLimit  int  `json:"dailyLimit"`
}

func imageFor(v domain.Effective, pl domain.Platform) string {
	if pl == domain.Instagram {
		if v.SocialImage != nil && v.SocialImage.Instagram != nil {
			return v.SocialImage.Instagram.Path
		}
		if v.Cover != nil && v.Cover.Instagram != nil {
			return v.Cover.Instagram.Path
		}
		return ""
	}
	if v.Cover == nil || len(v.Cover.Variants) == 0 {
		return ""
	}
	return v.Cover.Variants[len(v.Cover.Variants)-1].Path // the largest
}

func today(now time.Time) string { return now.UTC().Format(time.DateOnly) }

// EventPosts returns an Event's Social Posts and generated Captions.
func (a *App) EventPosts(ctx context.Context, p domain.Principal, org, id string) (PostsView, error) {
	o, err := a.authorize(p, org)
	if err != nil {
		return PostsView{}, err
	}
	e, _, err := a.loadEvent(ctx, org, id)
	if err != nil {
		return PostsView{}, err
	}
	conn, err := a.SocialStatus(ctx, p, org)
	if err != nil {
		return PostsView{}, err
	}
	v := PostsView{
		Postable: e.OnSite(), Posts: []domain.SocialPost{}, Connection: conn,
		Captions: map[domain.Lang]string{
			domain.PL: domain.GenerateCaption(o, e, domain.PL, a.now()),
			domain.EN: domain.GenerateCaption(o, e, domain.EN, a.now()),
		},
		Images: map[domain.Platform]string{}, CanInstagram: conn.IGUsername != "",
		StoredToken: conn.Autosync != nil, DailyLimit: domain.DailyPostLimit,
	}
	eff := e.Effective()
	for _, pl := range []domain.Platform{domain.Facebook, domain.Instagram} {
		if path := imageFor(eff, pl); path != "" {
			v.Images[pl] = a.Pub.URL(path)
		}
	}
	if posts, _, err := load[domain.EventPosts](ctx, a.Docs, postsKey(org, id)); err == nil {
		v.Posts = posts.Posts
	} else if !errors.Is(err, domain.ErrNotFound) {
		return v, err
	}
	if pc, _, err := load[domain.PostCounter](ctx, a.Docs, postCounterKey(org, today(a.now()))); err == nil {
		v.PostsToday = pc.Count
	}
	return v, nil
}

func problem(field, pl, en string) error {
	return domain.Problem(field, pl, en)
}

// postable loads an Event the Site shows and the Organization's connection,
// and checks each platform can be posted: Instagram needs a linked account,
// and each platform needs its image.
func (a *App) postable(ctx context.Context, org, id string, platforms []domain.Platform) (*domain.Event, *domain.Connection, error) {
	e, _, err := a.loadEvent(ctx, org, id)
	if err != nil {
		return nil, nil, err
	}
	if !e.OnSite() {
		return nil, nil, problem("event", "ukrytych wydarzeń i wydarzeń w koszu nie można ogłaszać", "hidden events and events in the Bin cannot be announced")
	}
	conn, err := a.connection(ctx, org)
	if err != nil {
		return nil, nil, err
	}
	eff := e.Effective()
	for _, pl := range platforms {
		if pl == domain.Instagram && conn.IGUserID == "" {
			return nil, nil, problem("connection", "strona nie ma połączonego konta Instagram", "the Facebook page has no connected Instagram account")
		}
		if imageFor(eff, pl) == "" {
			return nil, nil, problem("image", "wydarzenie nie ma zdjęcia głównego", "the event has no cover image")
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

// takePosts counts n attempts against the Organization's daily limit.
func (a *App) takePosts(ctx context.Context, org string, n int) error {
	key := postCounterKey(org, today(a.now()))
	_, _, err := mutate(ctx, a.Docs, key, func(c *domain.PostCounter) error { return c.Take(n) })
	if errors.Is(err, domain.ErrNotFound) {
		c := &domain.PostCounter{Day: today(a.now())}
		if err = c.Take(n); err == nil {
			_, err = save(ctx, a.Docs, key, c, "")
			if errors.Is(err, domain.ErrConflict) {
				_, _, err = mutate(ctx, a.Docs, key, func(c *domain.PostCounter) error { return c.Take(n) })
			}
		}
	}
	if errors.Is(err, domain.ErrLimit) {
		return problem("platform", fmt.Sprintf("dzienny limit %d postów organizacji został wykorzystany", domain.DailyPostLimit),
			fmt.Sprintf("the organization's daily limit of %d posts is used up", domain.DailyPostLimit))
	}
	return err
}

// PostResultOrRequest is what posting answers: with Autosync on, the
// outcome; otherwise a Post Request waiting for the Meta sign-in.
type PostResultOrRequest struct {
	Request *domain.PostRequest
	Outcome *PostOutcome
}

// Post starts posting an Event on one or both platforms. It checks
// everything that can be checked without Meta, including the
// confirmed-retry rule. With Autosync off it keeps a Post Request while the
// Moderator signs in with Meta (CompletePost finishes it); with Autosync on
// it needs a Recent sign-in and posts at once with the stored token.
func (a *App) Post(ctx context.Context, p domain.Principal, org, id string, drafts []domain.PostDraft, confirmed bool) (PostResultOrRequest, error) {
	if _, err := a.authorize(p, org); err != nil {
		return PostResultOrRequest{}, err
	}
	req, err := domain.NewPostRequest(a.newID(), org, id, p.ID(), drafts, confirmed, a.now())
	if err != nil {
		return PostResultOrRequest{}, err
	}
	_, conn, err := a.postable(ctx, org, id, platformsOf(req.Drafts))
	if err != nil {
		return PostResultOrRequest{}, err
	}
	ep, _, err := load[domain.EventPosts](ctx, a.Docs, postsKey(org, id))
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return PostResultOrRequest{}, err
	}
	if ep != nil {
		for _, d := range req.Drafts {
			if err := ep.CheckRetry(d.Platform, confirmed); err != nil {
				return PostResultOrRequest{}, err
			}
		}
	}
	if conn.Autosync != nil {
		if err := a.requireRecentSignIn(p); err != nil {
			return PostResultOrRequest{}, err
		}
		out, err := a.postWithStoredToken(ctx, p, conn, req)
		return PostResultOrRequest{Outcome: &out}, err
	}
	if _, err := save(ctx, a.Docs, postRequestKey(org, req.ID), req, ""); err != nil {
		return PostResultOrRequest{}, err
	}
	return PostResultOrRequest{Request: req}, nil
}

// postWithStoredToken posts with Autosync's token, read for this request only.
func (a *App) postWithStoredToken(ctx context.Context, p domain.Principal, conn *domain.Connection, req *domain.PostRequest) (PostOutcome, error) {
	ctx = context.WithoutCancel(ctx)
	e, conn, err := a.postable(ctx, req.OrgID, req.EventID, platformsOf(req.Drafts))
	if err != nil {
		return PostOutcome{}, err
	}
	// Counted only once nothing else can refuse the request, and before
	// the token is read: the daily limit also bounds reads of the secret.
	if err := a.takePosts(ctx, req.OrgID, len(req.Drafts)); err != nil {
		return PostOutcome{}, err
	}
	token, err := a.storedToken(ctx, p, conn, "post")
	if err != nil {
		return PostOutcome{}, err
	}
	page := ports.SocialPage{ID: conn.PageID, Name: conn.PageName, IGUserID: conn.IGUserID, IGUsername: conn.IGUsername}
	out := PostOutcome{}
	for _, d := range req.Drafts {
		res := a.postOne(ctx, p, req.OrgID, e, conn, page, token, d, req.Confirmed, "stored")
		out.Results = append(out.Results, res)
	}
	return out, nil
}

// Reasons why a Post Request, or one platform of it, was not sent to Meta.
const (
	RefusedCancelled = "cancelled" // the Moderator cancelled the Meta sign-in
	RefusedExpired   = "expired"   // the request is gone, expired or not theirs
	RefusedLogin     = "login"     // the Meta sign-in could not be completed
	RefusedPage      = "page"      // their Meta account has no role on the Facebook page
	RefusedInvalid   = "invalid"   // the Event or connection changed meanwhile
	RefusedInstagram = "instagram" // their Meta account cannot reach the Instagram account
	RefusedQuota     = "quota"     // the daily Instagram publishing limit is used up
	RefusedLimit     = "limit"     // the Organization's daily post limit is used up
	RefusedConfirm   = "confirm"   // a post appeared meanwhile: confirm to post again
	RefusedError     = "error"     // the CMS could not record the attempt
	RefusedToken     = "token"     // Graph rejected the stored token: turn Autosync on again
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
// Organization's Facebook page (which proves the Moderator has a role on it), posts
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
	if err := a.takePosts(ctx, org, len(req.Drafts)); domain.IsValidation(err) {
		a.audit(ctx, p, "social.post_refused", org, req.EventID, map[string]any{"reason": RefusedLimit})
		return PostOutcome{Refused: RefusedLimit}, nil
	} else if err != nil {
		return PostOutcome{}, err
	}
	out := PostOutcome{}
	for _, d := range req.Drafts {
		out.Results = append(out.Results, a.postOne(ctx, p, org, e, conn, page, pageToken, d, req.Confirmed, "sign-in"))
	}
	return out, nil
}

// postOne posts one draft with a Facebook page token held in memory only.
func (a *App) postOne(ctx context.Context, p domain.Principal, org string, e *domain.Event, conn *domain.Connection, page ports.SocialPage, token string, d domain.PostDraft, confirmed bool, tokenKind string) PlatformResult {
	refuse := func(reason string) PlatformResult {
		a.audit(ctx, p, "social.post_refused", org, e.ID, map[string]any{"platform": d.Platform, "reason": reason, "token": tokenKind})
		return PlatformResult{Platform: d.Platform, Status: "refused", Reason: reason}
	}
	target := conn.PageID
	if d.Platform == domain.Instagram {
		// The Meta account behind the token must reach the connected
		// Instagram account through the Facebook page.
		if page.IGUserID == "" || page.IGUserID != conn.IGUserID {
			return refuse(RefusedInstagram)
		}
		target = conn.IGUserID
		used, limit, err := a.Social.InstagramQuota(ctx, conn.IGUserID, token)
		if errors.Is(err, ports.ErrTokenRejected) {
			return refuse(RefusedToken)
		}
		if err == nil && limit > 0 && used >= limit {
			return refuse(RefusedQuota)
		}
	}
	imgPath := imageFor(e.Effective(), d.Platform)

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
	details := map[string]any{"platform": d.Platform, "target": target, "url": res.URL, "token": tokenKind}
	if postErr != nil {
		details["error"] = postErr.Error()
		a.audit(ctx, p, "social.post_failed", org, e.ID, details)
		if errors.Is(postErr, ports.ErrTokenRejected) && tokenKind == "stored" {
			a.markNeedsReconnect(ctx, p, org, "post")
		}
		return PlatformResult{Platform: d.Platform, Status: string(domain.PostFailed)}
	}
	a.audit(ctx, p, "social.posted", org, e.ID, details)
	return PlatformResult{Platform: d.Platform, Status: string(domain.PostPosted)}
}
