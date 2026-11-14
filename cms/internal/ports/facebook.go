package ports

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrEventGone is Graph's answer for a deleted event read by id (code
	// 100, subcode 33). Graph gives the same answer for a missing
	// permission, so it means "gone" only after a successful list with the
	// same token (decision 11).
	ErrEventGone = errors.New("social: Facebook Event does not exist or cannot be loaded")
	// ErrTokenRejected means Graph refused the token (expired, revoked,
	// role lost, data access expired).
	ErrTokenRejected = errors.New("social: token rejected")
)

// FacebookEvent is a Facebook Event as Graph returns it. Its text is
// untrusted (T7).
type FacebookEvent struct {
	ID          string
	Name        string
	Description string
	Start       time.Time
	End         time.Time // zero when Facebook has no end
	Place       *FacebookPlace
	Online      bool
	Cancelled   bool
	CoverID     string
	CoverURL    string
	TicketURL   string
	Updated     time.Time
	// Times are the dates of a repeating Facebook Event (event_times),
	// each with its own Facebook Event id; empty for a single one.
	Times []FacebookEventTime
}

// FacebookPlace is a Facebook Event's place.
type FacebookPlace struct {
	Name, Street, City, Zip, Country string
	Latitude, Longitude              float64
	HasCoordinates                   bool
}

// FacebookEventTime is one date of a repeating Facebook Event.
type FacebookEventTime struct {
	ID         string
	Start, End time.Time
}

// FacebookEvents reads Facebook Events with a Facebook page token, and
// issues the long-lived page token Autosync stores.
type FacebookEvents interface {
	// PageEvents lists the events the page created, cancelled ones included.
	PageEvents(ctx context.Context, pageID, pageToken string) ([]FacebookEvent, error)
	// Event reads one event by id; ErrEventGone as described above.
	Event(ctx context.Context, eventID, pageToken string) (FacebookEvent, error)
	// CoverImage downloads a cover over https from a Facebook CDN host, size-capped.
	CoverImage(ctx context.Context, url string) ([]byte, error)
	// LongLivedPageToken exchanges a short-lived user token for a long-lived
	// one, derives the page's token from it (it does not expire), and
	// returns it with the page as the user sees it and the date Meta stops
	// letting it read data. Both user tokens are dropped.
	LongLivedPageToken(ctx context.Context, userToken, pageID string) (token string, page SocialPage, dataAccessExpires time.Time, err error)
}
