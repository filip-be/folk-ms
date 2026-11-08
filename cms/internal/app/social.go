package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
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
	Connected      bool         `json:"connected"`
	PageName       string       `json:"pageName,omitempty"`
	PageID         string       `json:"pageId,omitempty"`
	IGUsername     string       `json:"igUsername,omitempty"`
	CreatedByEmail string       `json:"createdByEmail,omitempty"`
	CreatedAt      *time.Time   `json:"createdAt,omitempty"`
	NeedsReconnect bool         `json:"needsReconnect"`
	ReconnectNote  *domain.Text `json:"reconnectNote,omitempty"`
	PendingPages   []PageInfo   `json:"pendingPages,omitempty"`
}

// PageInfo is a Facebook Page to choose from, without its token.
type PageInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	IGUsername string `json:"igUsername,omitempty"`
}

type pendingPages struct {
	ExpiresAt time.Time `json:"expiresAt"`
	Enc       string    `json:"enc"`
}

func (a *App) encrypt(v any, aad string) (string, error) {
	pt, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	ct, err := a.Cipher.Encrypt(pt, []byte(aad))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ct), nil
}

func (a *App) decrypt(s, aad string, v any) error {
	ct, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	pt, err := a.Cipher.Decrypt(ct, []byte(aad))
	if err != nil {
		return fmt.Errorf("decrypt: %w", err)
	}
	return json.Unmarshal(pt, v)
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
		st.NeedsReconnect = c.NeedsReconnect
		if c.NeedsReconnect {
			note := c.ReconnectMessage()
			st.ReconnectNote = &note
		}
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

// SocialLoginURL returns where to send the Moderator to connect.
func (a *App) SocialLoginURL(p domain.Principal, org, state, redirectURI string) (string, error) {
	if _, err := a.authorize(p, org); err != nil {
		return "", err
	}
	return a.Social.LoginURL(state, redirectURI), nil
}

// CompleteSocialLogin exchanges the Facebook Login code and keeps the
// Moderator's Pages (encrypted, for 15 minutes) until they choose one.
func (a *App) CompleteSocialLogin(ctx context.Context, p domain.Principal, org, code, redirectURI string) ([]PageInfo, error) {
	if _, err := a.authorize(p, org); err != nil {
		return nil, err
	}
	pages, err := a.Social.Pages(ctx, code, redirectURI)
	if err != nil {
		return nil, fmt.Errorf("facebook login: %w", err)
	}
	key := pendingPagesKey(org, p.ID())
	enc, err := a.encrypt(pages, key)
	if err != nil {
		return nil, err
	}
	if _, err := save(ctx, a.Docs, key, pendingPages{ExpiresAt: a.now().Add(15 * time.Minute), Enc: enc}, ports.AnyETag); err != nil {
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
	var pages []ports.SocialPage
	if err := a.decrypt(pp.Enc, key, &pages); err != nil {
		return nil, err
	}
	return pages, nil
}

// SelectPage turns a chosen Page into the Organization's connection,
// replacing any previous one.
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
		return ConnectionStatus{}, fmt.Errorf("%w: page not available, sign in to Facebook again", domain.ErrNotFound)
	}
	enc, err := a.encrypt(page.AccessToken, connectionKey(org))
	if err != nil {
		return ConnectionStatus{}, err
	}
	c := domain.Connection{
		OrgID: org, PageID: page.ID, PageName: page.Name, IGUserID: page.IGUserID, IGUsername: page.IGUsername,
		TokenEnc: enc, CreatedBy: p.ID(), CreatedByEmail: p.User.Email, CreatedAt: a.now(),
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

// Post publishes a Social Post of a Published Event on one platform. The
// record is written as pending before the Graph call, so a crash leaves an
// unknown outcome that is retried only after the Moderator confirms.
func (a *App) Post(ctx context.Context, p domain.Principal, org, id string, platform domain.Platform, caption string, confirmed bool) (*domain.SocialPost, error) {
	if _, err := a.authorize(p, org); err != nil {
		return nil, err
	}
	if !domain.ValidPlatform(platform) {
		return nil, problem("platform", "nieznana platforma", "unknown platform")
	}
	e, _, err := load[domain.Event](ctx, a.Docs, eventKey(org, id))
	if err != nil {
		return nil, err
	}
	if e.Published == nil || e.DeletedAt != nil {
		return nil, problem("event", "tylko opublikowane wydarzenia można ogłaszać", "only published events can be announced")
	}
	conn, _, err := load[domain.Connection](ctx, a.Docs, connectionKey(org))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, problem("connection", "połącz najpierw konto Facebook/Instagram", "connect the Facebook/Instagram account first")
	}
	if err != nil {
		return nil, err
	}
	target := conn.PageID
	if platform == domain.Instagram {
		if conn.IGUserID == "" {
			return nil, problem("connection", "strona nie ma połączonego konta Instagram", "the Page has no connected Instagram account")
		}
		target = conn.IGUserID
	}
	imgPath := imageFor(e.Published, platform)
	if imgPath == "" {
		return nil, problem("image", "dodaj zdjęcie główne i opublikuj wydarzenie ponownie", "add a cover image and publish the event again")
	}
	var token string
	if err := a.decrypt(conn.TokenEnc, connectionKey(org), &token); err != nil {
		return nil, err
	}
	if platform == domain.Instagram {
		used, limit, err := a.Social.InstagramQuota(ctx, conn.IGUserID, token)
		if err == nil && limit > 0 && used >= limit {
			return nil, problem("instagram", fmt.Sprintf("wyczerpano dzienny limit publikacji na Instagramie (%d/%d)", used, limit),
				fmt.Sprintf("the daily Instagram publishing limit is used up (%d/%d)", used, limit))
		}
	}

	postID := a.newID()
	key := postsKey(org, id)
	begin := func(ep *domain.EventPosts) error {
		_, err := ep.Begin(postID, platform, caption, imgPath, p.ID(), confirmed, a.now())
		return err
	}
	if _, _, err := mutate(ctx, a.Docs, key, begin); errors.Is(err, domain.ErrNotFound) {
		ep := &domain.EventPosts{EventID: id, OrgID: org}
		if err := begin(ep); err != nil {
			return nil, err
		}
		if _, err := save(ctx, a.Docs, key, ep, ""); err != nil {
			return nil, err // a concurrent first post: the Moderator retries
		}
	} else if err != nil {
		return nil, err
	}

	imageURL := a.Pub.URL(imgPath)
	var res ports.PostResult
	var postErr error
	// Once the pending record exists, the Graph call and the recording of its
	// outcome run to completion even if the Moderator's request is cancelled:
	// a cancelled call would be recorded as failed although Meta may have
	// published it, inviting a duplicate. The Graph client has its own timeout.
	rctx := context.WithoutCancel(ctx)
	if platform == domain.Facebook {
		res, postErr = a.Social.PostFacebookPhoto(rctx, conn.PageID, token, imageURL, caption)
	} else {
		res, postErr = a.Social.PostInstagramImage(rctx, conn.IGUserID, token, imageURL, caption)
	}
	ep, _, err := mutate(rctx, a.Docs, key, func(ep *domain.EventPosts) error {
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
	details := map[string]any{"platform": platform, "target": target, "url": res.URL}
	if postErr != nil {
		details["error"] = postErr.Error()
		a.audit(rctx, p, "social.post_failed", org, id, details)
	} else {
		a.audit(rctx, p, "social.posted", org, id, details)
	}
	if err != nil {
		return nil, fmt.Errorf("posted=%v but recording the outcome failed: %w", postErr == nil, err)
	}
	sp := ep.Find(postID)
	return sp, nil
}
