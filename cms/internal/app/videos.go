package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/filip-be/folk-ms/cms/internal/domain"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// NewVideosETag is the version of a Video Collection that has never been
// saved: a change sent with it creates the document, and fails if another
// Moderator created it meanwhile.
const NewVideosETag = "new"

func videosKey(org string) string { return "orgs/" + org + "/videos.json" }

// VideosPath is where a Site's Video Collection snapshot lives in the public store.
func VideosPath(siteID string) string { return "sites/" + siteID + "/videos.json" }

// VideoView is a Video as the admin shows it.
type VideoView struct {
	domain.Video
	// URL is the canonical address with the start; LinkURL without it.
	URL          string `json:"url"`
	LinkURL      string `json:"linkUrl"`
	Start        string `json:"start,omitempty"` // m:ss
	ThumbnailURL string `json:"thumbnailUrl"`
}

// LabeledView is a Form or a region with the number of Videos that use it.
type LabeledView struct {
	ID    string      `json:"id"`
	Label domain.Text `json:"label"`
	Uses  int         `json:"uses"`
}

// CountryView is a country with its regions and use counts.
type CountryView struct {
	LabeledView
	Regions []LabeledView `json:"regions"`
}

// VideoCollectionView is the Video Collection with its version. Every
// change sends the version back (If-Match).
type VideoCollectionView struct {
	ETag    string        `json:"etag"`
	Forms   []LabeledView `json:"forms"`
	Origins []CountryView `json:"origins"`
	Videos  []VideoView   `json:"videos"`
}

func videoView(v domain.Video) VideoView {
	return VideoView{Video: v, URL: v.Link.WatchURL(), LinkURL: v.Link.LinkURL(), Start: domain.FormatStart(v.Link.StartSeconds), ThumbnailURL: v.ThumbnailURL()}
}

func collectionView(c *domain.VideoCollection, etag string) VideoCollectionView {
	out := VideoCollectionView{ETag: etag, Forms: []LabeledView{}, Origins: []CountryView{}, Videos: []VideoView{}}
	for _, f := range c.Forms {
		out.Forms = append(out.Forms, LabeledView{ID: f.ID, Label: f.Label, Uses: c.FormUse(f.ID)})
	}
	for _, co := range c.Origins {
		cv := CountryView{LabeledView: LabeledView{ID: co.ID, Label: co.Label, Uses: c.OriginUse(co.ID)}, Regions: []LabeledView{}}
		for _, r := range co.Regions {
			cv.Regions = append(cv.Regions, LabeledView{ID: r.ID, Label: r.Label, Uses: c.OriginUse(r.ID)})
		}
		out.Origins = append(out.Origins, cv)
	}
	for _, v := range c.Videos {
		out.Videos = append(out.Videos, videoView(v))
	}
	return out
}

// loadVideos loads an Organization's Video Collection: an empty one with
// NewVideosETag when it was never saved.
func (a *App) loadVideos(ctx context.Context, org string) (*domain.VideoCollection, string, error) {
	c, etag, err := load[domain.VideoCollection](ctx, a.Docs, videosKey(org))
	if errors.Is(err, domain.ErrNotFound) {
		return domain.NewVideoCollection(org), NewVideosETag, nil
	}
	if err != nil {
		return nil, "", err
	}
	if c.OrgID != org {
		return nil, "", fmt.Errorf("%w: video collection of %s holds %s", domain.ErrNotFound, org, c.OrgID)
	}
	return c, etag, nil
}

// GetVideos returns the Video Collection of an Organization.
func (a *App) GetVideos(ctx context.Context, p domain.Principal, org string) (VideoCollectionView, error) {
	if _, err := a.authorize(p, org); err != nil {
		return VideoCollectionView{}, err
	}
	c, etag, err := a.loadVideos(ctx, org)
	if err != nil {
		return VideoCollectionView{}, err
	}
	return collectionView(c, etag), nil
}

// changeVideos applies a Moderator's change to the Video Collection with
// If-Match on etag (the version the screen loaded), records it in the Audit
// Log and rebuilds the Site's videos.json. fn returns the audit target and
// details.
func (a *App) changeVideos(ctx context.Context, p domain.Principal, org, etag, action string,
	fn func(c *domain.VideoCollection) (target string, details map[string]any, err error)) (VideoCollectionView, error) {
	if _, err := a.authorize(p, org); err != nil {
		return VideoCollectionView{}, err
	}
	if etag == "" {
		return VideoCollectionView{}, fmt.Errorf("%w: missing version", domain.ErrConflict)
	}
	c, cur, err := a.loadVideos(ctx, org)
	if err != nil {
		return VideoCollectionView{}, err
	}
	if cur != etag {
		return VideoCollectionView{}, fmt.Errorf("%w: the video collection was changed meanwhile", domain.ErrConflict)
	}
	target, details, err := fn(c)
	if err != nil {
		return VideoCollectionView{}, err
	}
	put := etag
	if put == NewVideosETag {
		put = "" // create-only
	}
	tag, err := save(ctx, a.Docs, videosKey(org), c, put)
	if err != nil {
		return VideoCollectionView{}, err
	}
	a.audit(ctx, p, action, org, target, details)
	if err := a.RebuildVideoSnapshot(ctx, org); err != nil {
		return VideoCollectionView{}, fmt.Errorf("video collection saved but snapshot rebuild failed: %w", err)
	}
	return collectionView(c, tag), nil
}

// vimeoThumbnail asks Vimeo for a video's thumbnail; a video Vimeo does not
// know or will not embed, or Vimeo not answering, is a problem on the link.
func (a *App) vimeoThumbnail(ctx context.Context, l domain.VideoLink) (string, error) {
	if a.VideoInfo == nil {
		return "", errors.New("no video info adapter")
	}
	th, err := a.VideoInfo.Vimeo(ctx, l.ID, l.Hash)
	switch {
	case errors.Is(err, ports.ErrVideoUnavailable):
		return "", domain.Problem("link", "Vimeo nie zna tego filmu albo nie pozwala go osadzać na stronach (sprawdź ustawienia prywatności filmu)",
			"Vimeo does not know this video or does not allow embedding it (check the video's privacy settings)")
	case err != nil:
		a.Log.Warn("vimeo oembed", "id", l.ID, "err", err)
		return "", domain.Problem("link", "nie udało się sprawdzić filmu na Vimeo – spróbuj za chwilę", "the video could not be checked on Vimeo – try again in a moment")
	}
	return th, nil
}

// LinkPreview is a video link as the CMS understands it.
type LinkPreview struct {
	Provider     string `json:"provider"`
	VideoID      string `json:"videoId"`
	Hash         string `json:"hash,omitempty"`
	StartSeconds int    `json:"startSeconds,omitempty"`
	Start        string `json:"start,omitempty"`
	URL          string `json:"url"`
	LinkURL      string `json:"linkUrl"`
	ThumbnailURL string `json:"thumbnailUrl"`
}

// PreviewVideoLink parses a link (and asks Vimeo for a Vimeo video) without
// saving anything, so the edit form shows what will be saved.
func (a *App) PreviewVideoLink(ctx context.Context, p domain.Principal, org string, in domain.VideoInput) (LinkPreview, error) {
	if _, err := a.authorize(p, org); err != nil {
		return LinkPreview{}, err
	}
	l, err := domain.ParseVideoInputLink(in)
	if err != nil {
		return LinkPreview{}, err
	}
	out := LinkPreview{Provider: l.Provider, VideoID: l.ID, Hash: l.Hash, StartSeconds: l.StartSeconds, Start: domain.FormatStart(l.StartSeconds),
		URL: l.WatchURL(), LinkURL: l.LinkURL()}
	if l.Provider == domain.Vimeo {
		if out.ThumbnailURL, err = a.vimeoThumbnail(ctx, l); err != nil {
			return LinkPreview{}, err
		}
	} else {
		out.ThumbnailURL = domain.YouTubeThumbnail(l.ID)
	}
	return out, nil
}

func videoDetails(v domain.Video) map[string]any {
	return map[string]any{"title": v.Title.PL, "url": v.Link.WatchURL()}
}

// AddVideo adds a Video at the end of the list.
func (a *App) AddVideo(ctx context.Context, p domain.Principal, org, etag string, in domain.VideoInput) (VideoCollectionView, error) {
	return a.changeVideos(ctx, p, org, etag, "video.add", func(c *domain.VideoCollection) (string, map[string]any, error) {
		v, err := c.PrepareVideo(in)
		if err != nil {
			return "", nil, err
		}
		if v.Link.Provider == domain.Vimeo {
			if v.Thumbnail, err = a.vimeoThumbnail(ctx, v.Link); err != nil {
				return "", nil, err
			}
		}
		id, err := c.NewItemID(a.Rand)
		if err != nil {
			return "", nil, err
		}
		if err := c.AddVideo(v, id, p.ID(), a.now()); err != nil {
			return "", nil, err
		}
		return id, videoDetails(v), nil
	})
}

// UpdateVideo replaces a Video's content. Vimeo is asked again only when the
// link points to another video.
func (a *App) UpdateVideo(ctx context.Context, p domain.Principal, org, id, etag string, in domain.VideoInput) (VideoCollectionView, error) {
	return a.changeVideos(ctx, p, org, etag, "video.update", func(c *domain.VideoCollection) (string, map[string]any, error) {
		cur, ok := c.Video(id)
		if !ok {
			return "", nil, fmt.Errorf("%w: video", domain.ErrNotFound)
		}
		v, err := c.PrepareVideo(in)
		if err != nil {
			return "", nil, err
		}
		if v.Link.Provider == domain.Vimeo {
			if v.Link.SameVideo(cur.Link) && cur.Thumbnail != "" {
				v.Thumbnail = cur.Thumbnail
			} else if v.Thumbnail, err = a.vimeoThumbnail(ctx, v.Link); err != nil {
				return "", nil, err
			}
		}
		if err := c.UpdateVideo(id, v, p.ID(), a.now()); err != nil {
			return "", nil, err
		}
		return id, videoDetails(v), nil
	})
}

// DeleteVideo removes a Video for good; the Audit Log keeps its title and link.
func (a *App) DeleteVideo(ctx context.Context, p domain.Principal, org, id, etag string) (VideoCollectionView, error) {
	return a.changeVideos(ctx, p, org, etag, "video.delete", func(c *domain.VideoCollection) (string, map[string]any, error) {
		v, err := c.DeleteVideo(id, p.ID(), a.now())
		if err != nil {
			return "", nil, err
		}
		return id, videoDetails(v), nil
	})
}

// SetVideoHidden switches a Video's Hide.
func (a *App) SetVideoHidden(ctx context.Context, p domain.Principal, org, id, etag string, hidden bool) (VideoCollectionView, error) {
	action := "video.show"
	if hidden {
		action = "video.hide"
	}
	return a.changeVideos(ctx, p, org, etag, action, func(c *domain.VideoCollection) (string, map[string]any, error) {
		if err := c.SetVideoHidden(id, hidden, p.ID(), a.now()); err != nil {
			return "", nil, err
		}
		v, _ := c.Video(id)
		return id, map[string]any{"title": v.Title.PL}, nil
	})
}

// MoveVideo moves a Video to an index of the list.
func (a *App) MoveVideo(ctx context.Context, p domain.Principal, org, id, etag string, to int) (VideoCollectionView, error) {
	return a.changeVideos(ctx, p, org, etag, "video.move", func(c *domain.VideoCollection) (string, map[string]any, error) {
		if err := c.MoveVideo(id, to, p.ID(), a.now()); err != nil {
			return "", nil, err
		}
		v, _ := c.Video(id)
		return id, map[string]any{"title": v.Title.PL, "to": to}, nil
	})
}

// AddForm appends a Form to the list.
func (a *App) AddForm(ctx context.Context, p domain.Principal, org, etag string, label domain.Text) (VideoCollectionView, error) {
	return a.changeVideos(ctx, p, org, etag, "form.add", func(c *domain.VideoCollection) (string, map[string]any, error) {
		id, err := c.NewItemID(a.Rand)
		if err != nil {
			return "", nil, err
		}
		if err := c.AddForm(id, label, p.ID(), a.now()); err != nil {
			return "", nil, err
		}
		return id, map[string]any{"label": c.Forms[len(c.Forms)-1].Label.PL}, nil
	})
}

// RenameForm changes a Form's label.
func (a *App) RenameForm(ctx context.Context, p domain.Principal, org, id, etag string, label domain.Text) (VideoCollectionView, error) {
	return a.changeVideos(ctx, p, org, etag, "form.rename", func(c *domain.VideoCollection) (string, map[string]any, error) {
		if err := c.RenameForm(id, label, p.ID(), a.now()); err != nil {
			return "", nil, err
		}
		return id, map[string]any{"label": label.PL}, nil
	})
}

// RemoveForm removes a Form no Video uses.
func (a *App) RemoveForm(ctx context.Context, p domain.Principal, org, id, etag string) (VideoCollectionView, error) {
	return a.changeVideos(ctx, p, org, etag, "form.remove", func(c *domain.VideoCollection) (string, map[string]any, error) {
		label := ""
		for _, f := range c.Forms {
			if f.ID == id {
				label = f.Label.PL
			}
		}
		if err := c.RemoveForm(id, p.ID(), a.now()); err != nil {
			return "", nil, err
		}
		return id, map[string]any{"label": label}, nil
	})
}

// MoveForm moves a Form to an index of the list.
func (a *App) MoveForm(ctx context.Context, p domain.Principal, org, id, etag string, to int) (VideoCollectionView, error) {
	return a.changeVideos(ctx, p, org, etag, "form.move", func(c *domain.VideoCollection) (string, map[string]any, error) {
		return id, map[string]any{"to": to}, c.MoveForm(id, to, p.ID(), a.now())
	})
}

// AddOrigin appends a country (parentID "") or a region of a country.
func (a *App) AddOrigin(ctx context.Context, p domain.Principal, org, etag, parentID string, label domain.Text) (VideoCollectionView, error) {
	return a.changeVideos(ctx, p, org, etag, "origin.add", func(c *domain.VideoCollection) (string, map[string]any, error) {
		id, err := c.NewItemID(a.Rand)
		if err != nil {
			return "", nil, err
		}
		if err := c.AddOrigin(id, parentID, label, p.ID(), a.now()); err != nil {
			return "", nil, err
		}
		d := map[string]any{"label": label.PL}
		if parentID != "" {
			d["parentId"] = parentID
		}
		return id, d, nil
	})
}

// RenameOrigin changes a country's or region's label.
func (a *App) RenameOrigin(ctx context.Context, p domain.Principal, org, id, etag string, label domain.Text) (VideoCollectionView, error) {
	return a.changeVideos(ctx, p, org, etag, "origin.rename", func(c *domain.VideoCollection) (string, map[string]any, error) {
		return id, map[string]any{"label": label.PL}, c.RenameOrigin(id, label, p.ID(), a.now())
	})
}

// RemoveOrigin removes a country without regions or a region no Video uses.
func (a *App) RemoveOrigin(ctx context.Context, p domain.Principal, org, id, etag string) (VideoCollectionView, error) {
	return a.changeVideos(ctx, p, org, etag, "origin.remove", func(c *domain.VideoCollection) (string, map[string]any, error) {
		label := ""
		for _, co := range c.Origins {
			if co.ID == id {
				label = co.Label.PL
			}
			for _, r := range co.Regions {
				if r.ID == id {
					label = r.Label.PL
				}
			}
		}
		return id, map[string]any{"label": label}, c.RemoveOrigin(id, p.ID(), a.now())
	})
}

// MoveOrigin moves a country among the countries or a region within its country.
func (a *App) MoveOrigin(ctx context.Context, p domain.Principal, org, id, etag string, to int) (VideoCollectionView, error) {
	return a.changeVideos(ctx, p, org, etag, "origin.move", func(c *domain.VideoCollection) (string, map[string]any, error) {
		return id, map[string]any{"to": to}, c.MoveOrigin(id, to, p.ID(), a.now())
	})
}

// RebuildVideoSnapshot rebuilds an Organization's videos.json, like
// RebuildSnapshot: the snapshot's ETag is read before the collection, so a
// stale build can never overwrite a fresher one.
func (a *App) RebuildVideoSnapshot(ctx context.Context, org string) error {
	o, ok := a.orgByID[org]
	if !ok {
		return fmt.Errorf("%w: organization", domain.ErrNotFound)
	}
	path := VideosPath(o.Site.ID)
	for range 8 {
		cur, err := a.Pub.Get(ctx, path)
		etag := cur.ETag
		if errors.Is(err, ports.ErrNotFound) {
			etag = ""
		} else if err != nil {
			return err
		}
		c, _, err := a.loadVideos(ctx, org)
		if err != nil {
			return err
		}
		data, err := json.Marshal(domain.BuildVideoSnapshot(o, c))
		if err != nil {
			return err
		}
		_, err = a.Pub.Put(ctx, path, data, "application/json", SnapshotCacheControl, etag)
		if errors.Is(err, ports.ErrPrecondition) {
			continue
		}
		return err
	}
	return fmt.Errorf("%w: video snapshot kept changing", domain.ErrConflict)
}
