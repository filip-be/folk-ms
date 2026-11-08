package domain

import (
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

// Begin records a pending attempt, enforcing the confirmed-retry rule: when
// the platform already has a post (pending with an unknown outcome, failed,
// or posted), the Moderator must confirm that posting again is intended.
func (ep *EventPosts) Begin(id string, p Platform, caption, imagePath, by string, confirmed bool, now time.Time) (*SocialPost, error) {
	if !ValidPlatform(p) {
		return nil, fmt.Errorf("%w: unknown platform", ErrInvalidState)
	}
	caption = strings.TrimSpace(caption)
	var pr problems
	if caption == "" {
		pr.add("caption", "treść posta jest wymagana", "the post text is required")
	} else if len([]rune(caption)) > MaxCaptionLen {
		pr.add("caption", "treść posta może mieć najwyżej %d znaków", "the post text can have at most %d characters", MaxCaptionLen)
	}
	if err := pr.err(); err != nil {
		return nil, err
	}
	if last := ep.Latest(p); last != nil && !confirmed {
		return nil, fmt.Errorf("%w: %s already has a %s post", ErrNeedsConfirm, p, last.Status)
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

// Connection is an Organization's Social Account Connection. The Page token
// is stored encrypted; the domain never sees it in clear.
type Connection struct {
	OrgID          string    `json:"orgId"`
	PageID         string    `json:"pageId"`
	PageName       string    `json:"pageName"`
	IGUserID       string    `json:"igUserId,omitempty"`
	IGUsername     string    `json:"igUsername,omitempty"`
	TokenEnc       string    `json:"tokenEnc"`
	CreatedBy      string    `json:"createdBy"`
	CreatedByEmail string    `json:"createdByEmail"`
	CreatedAt      time.Time `json:"createdAt"`
	NeedsReconnect bool      `json:"needsReconnect,omitempty"`
	// ReconnectBy is the email of the creator who lost access.
	ReconnectBy string `json:"reconnectBy,omitempty"`
	// ReconnectNote is the Polish-only note of connections flagged before
	// ReconnectBy existed; it is read, never written.
	ReconnectNote string `json:"reconnectNote,omitempty"`
}

// ReconnectMessage explains to Moderators why the connection needs
// reconnecting.
func (c Connection) ReconnectMessage() Text {
	switch {
	case !c.NeedsReconnect:
		return Text{}
	case c.ReconnectBy != "":
		return Text{
			PL: "Osoba, która połączyła konto (" + c.ReconnectBy + "), straciła dostęp. Połącz konto ponownie.",
			EN: "The person who connected the account (" + c.ReconnectBy + ") lost access. Reconnect the account.",
		}
	case c.ReconnectNote != "":
		return Text{PL: c.ReconnectNote}
	}
	return Text{PL: "Połącz konto ponownie.", EN: "Reconnect the account."}
}

// FlagIfCreatedBy marks the connection for reconnection when its creator
// lost access. It reports whether it changed.
func (c *Connection) FlagIfCreatedBy(userID, email string) bool {
	if c.CreatedBy != userID || c.NeedsReconnect {
		return false
	}
	c.NeedsReconnect = true
	c.ReconnectBy = email
	return true
}
