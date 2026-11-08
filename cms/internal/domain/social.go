package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Platform is a social network.
type Platform string

const (
	Facebook  Platform = "facebook"
	Instagram Platform = "instagram"
)

// ValidPlatform reports whether p is known.
func ValidPlatform(p Platform) bool { return p == Facebook || p == Instagram }

// PostStatus is the outcome of a Social Post.
type PostStatus string

const (
	PostPending PostStatus = "pending"
	PostPosted  PostStatus = "posted"
	PostFailed  PostStatus = "failed"
)

// MaxCaptionLen is Instagram's caption limit (Facebook allows more).
const MaxCaptionLen = 2200

// SocialPost is one attempt to post an Event on one platform.
type SocialPost struct {
	ID          string     `json:"id"`
	Platform    Platform   `json:"platform"`
	Status      PostStatus `json:"status"`
	ExternalID  string     `json:"externalId,omitempty"`
	URL         string     `json:"url,omitempty"`
	Caption     string     `json:"caption"`
	ImagePath   string     `json:"imagePath,omitempty"`
	Error       string     `json:"error,omitempty"`
	AttemptedBy string     `json:"attemptedBy"`
	AttemptedAt time.Time  `json:"attemptedAt"`
	FinishedAt  *time.Time `json:"finishedAt,omitempty"`
}

// EventPosts holds every Social Post of one Event, newest last per platform.
type EventPosts struct {
	EventID string       `json:"eventId"`
	OrgID   string       `json:"orgId"`
	Posts   []SocialPost `json:"posts"`
}

// Latest returns the newest post on a platform.
func (ep *EventPosts) Latest(p Platform) *SocialPost {
	for i := len(ep.Posts) - 1; i >= 0; i-- {
		if ep.Posts[i].Platform == p {
			return &ep.Posts[i]
		}
	}
	return nil
}

// Find returns a post by id.
func (ep *EventPosts) Find(id string) *SocialPost {
	for i := range ep.Posts {
		if ep.Posts[i].ID == id {
			return &ep.Posts[i]
		}
	}
	return nil
}

// validCaption trims a Caption and checks its length.
func validCaption(caption string) (string, error) {
	caption = strings.TrimSpace(caption)
	var pr problems
	if caption == "" {
		pr.add("caption", "treść posta jest wymagana", "the post text is required")
	} else if len([]rune(caption)) > MaxCaptionLen {
		pr.add("caption", "treść posta może mieć najwyżej %d znaków", "the post text can have at most %d characters", MaxCaptionLen)
	}
	return caption, pr.err()
}

// CheckRetry enforces the confirmed-retry rule: when the platform already
// has a post (pending with an unknown outcome, failed, or posted), the
// Moderator must confirm that posting again is intended.
func (ep *EventPosts) CheckRetry(p Platform, confirmed bool) error {
	if last := ep.Latest(p); last != nil && !confirmed {
		return fmt.Errorf("%w: %s already has a %s post", ErrNeedsConfirm, p, last.Status)
	}
	return nil
}

// Begin records a pending attempt, enforcing the confirmed-retry rule.
func (ep *EventPosts) Begin(id string, p Platform, caption, imagePath, by string, confirmed bool, now time.Time) (*SocialPost, error) {
	if !ValidPlatform(p) {
		return nil, fmt.Errorf("%w: unknown platform", ErrInvalidState)
	}
	caption, err := validCaption(caption)
	if err != nil {
		return nil, err
	}
	if err := ep.CheckRetry(p, confirmed); err != nil {
		return nil, err
	}
	ep.Posts = append(ep.Posts, SocialPost{
		ID: id, Platform: p, Status: PostPending, Caption: caption, ImagePath: imagePath,
		AttemptedBy: by, AttemptedAt: now,
	})
	return &ep.Posts[len(ep.Posts)-1], nil
}

// Succeed records the outcome of a post.
func (sp *SocialPost) Succeed(externalID, url string, now time.Time) {
	sp.Status, sp.ExternalID, sp.URL, sp.Error, sp.FinishedAt = PostPosted, externalID, url, "", &now
}

// Fail records a failed post.
func (sp *SocialPost) Fail(msg string, now time.Time) {
	sp.Status, sp.Error, sp.FinishedAt = PostFailed, msg, &now
}

// Connection is an Organization's Social Account Connection: which Facebook
// Page and Instagram account it posts to. It holds no token: every Social
// Post is sent with the posting Moderator's own Meta sign-in.
type Connection struct {
	OrgID          string    `json:"orgId"`
	PageID         string    `json:"pageId"`
	PageName       string    `json:"pageName"`
	IGUserID       string    `json:"igUserId,omitempty"`
	IGUsername     string    `json:"igUsername,omitempty"`
	CreatedBy      string    `json:"createdBy"`
	CreatedByEmail string    `json:"createdByEmail"`
	CreatedAt      time.Time `json:"createdAt"`
}

// PostDraft is the Caption a Moderator chose for one platform.
type PostDraft struct {
	Platform Platform `json:"platform"`
	Caption  string   `json:"caption"`
}

// PostRequestTTL is how long a Post Request waits for the Meta sign-in.
const PostRequestTTL = 15 * time.Minute

// PostRequest is a Moderator's request to post an Event on one or both
// platforms, kept while they sign in with Meta. It holds no secret.
type PostRequest struct {
	ID        string      `json:"id"`
	OrgID     string      `json:"orgId"`
	EventID   string      `json:"eventId"`
	UserID    string      `json:"userId"`
	Drafts    []PostDraft `json:"drafts"`
	Confirmed bool        `json:"confirmed"`
	ExpiresAt time.Time   `json:"expiresAt"`
}

// NewPostRequest validates the drafts: at least one, each platform once,
// each Caption valid.
func NewPostRequest(id, org, eventID, userID string, drafts []PostDraft, confirmed bool, now time.Time) (*PostRequest, error) {
	var pr problems
	if len(drafts) == 0 {
		pr.add("platform", "wybierz Facebook lub Instagram", "choose Facebook or Instagram")
	}
	seen := map[Platform]bool{}
	out := make([]PostDraft, 0, len(drafts))
	for _, d := range drafts {
		if !ValidPlatform(d.Platform) || seen[d.Platform] {
			pr.add("platform", "nieznana lub powtórzona platforma", "unknown or repeated platform")
			continue
		}
		seen[d.Platform] = true
		c, err := validCaption(d.Caption)
		if err != nil {
			var ve *ValidationError
			if errors.As(err, &ve) {
				pr = append(pr, ve.Problems...)
			}
			continue
		}
		out = append(out, PostDraft{Platform: d.Platform, Caption: c})
	}
	if err := pr.err(); err != nil {
		return nil, err
	}
	return &PostRequest{ID: id, OrgID: org, EventID: eventID, UserID: userID, Drafts: out, Confirmed: confirmed, ExpiresAt: now.Add(PostRequestTTL)}, nil
}

// Expired reports whether the request can no longer be completed.
func (r *PostRequest) Expired(now time.Time) bool { return now.After(r.ExpiresAt) }
