package meta

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// eventFields are the Graph Event fields Sync reads (spike, 2026-11-13).
const eventFields = "id,name,description,start_time,end_time,place,cover,ticket_uri,is_canceled,updated_time,event_times,is_online"

// maxEventPages bounds paging through /{page-id}/events.
const maxEventPages = 20

// MaxCoverBytes caps a downloaded cover.
const MaxCoverBytes = 10 << 20

type graphPlace struct {
	Name     string `json:"name"`
	Location *struct {
		Street    string   `json:"street"`
		City      string   `json:"city"`
		Zip       string   `json:"zip"`
		Country   string   `json:"country"`
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
	} `json:"location"`
}

type graphEvent struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	StartTime   string      `json:"start_time"`
	EndTime     string      `json:"end_time"`
	Place       *graphPlace `json:"place"`
	Cover       *struct {
		ID     string `json:"id"`
		Source string `json:"source"`
	} `json:"cover"`
	TicketURI   string `json:"ticket_uri"`
	IsCanceled  bool   `json:"is_canceled"`
	IsOnline    bool   `json:"is_online"`
	UpdatedTime string `json:"updated_time"`
	EventTimes  []struct {
		ID        string `json:"id"`
		StartTime string `json:"start_time"`
		EndTime   string `json:"end_time"`
	} `json:"event_times"`
}

// graphTime parses Graph's "2026-12-01T19:00:00+0100" (and RFC 3339).
func graphTime(s string) time.Time {
	for _, layout := range []string{"2006-01-02T15:04:05-0700", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func (g graphEvent) event() ports.FacebookEvent {
	e := ports.FacebookEvent{
		ID: g.ID, Name: g.Name, Description: g.Description,
		Start: graphTime(g.StartTime), End: graphTime(g.EndTime),
		Online: g.IsOnline, Cancelled: g.IsCanceled, TicketURL: g.TicketURI, Updated: graphTime(g.UpdatedTime),
	}
	if g.Cover != nil {
		e.CoverID, e.CoverURL = g.Cover.ID, g.Cover.Source
	}
	if g.Place != nil {
		p := &ports.FacebookPlace{Name: g.Place.Name}
		if l := g.Place.Location; l != nil {
			p.Street, p.City, p.Zip, p.Country = l.Street, l.City, l.Zip, l.Country
			if l.Latitude != nil && l.Longitude != nil {
				p.Latitude, p.Longitude, p.HasCoordinates = *l.Latitude, *l.Longitude, true
			}
		}
		e.Place = p
	}
	for _, t := range g.EventTimes {
		e.Times = append(e.Times, ports.FacebookEventTime{ID: t.ID, Start: graphTime(t.StartTime), End: graphTime(t.EndTime)})
	}
	return e
}

// PageEvents implements ports.FacebookEvents. Cancelled events are left out
// unless include_canceled is set (spike).
func (c *Client) PageEvents(ctx context.Context, pageID, token string) ([]ports.FacebookEvent, error) {
	params := url.Values{"fields": {eventFields}, "include_canceled": {"true"}, "limit": {"50"}}
	var out []ports.FacebookEvent
	after := ""
	for range maxEventPages {
		if after != "" {
			params.Set("after", after)
		}
		var page struct {
			Data   []graphEvent `json:"data"`
			Paging struct {
				Cursors struct {
					After string `json:"after"`
				} `json:"cursors"`
				Next string `json:"next"`
			} `json:"paging"`
		}
		if err := c.call(ctx, http.MethodGet, "/"+url.PathEscape(pageID)+"/events", params, token, &page); err != nil {
			return nil, err
		}
		for _, g := range page.Data {
			out = append(out, g.event())
		}
		if page.Paging.Next == "" || page.Paging.Cursors.After == "" || len(page.Data) == 0 {
			return out, nil
		}
		after = page.Paging.Cursors.After
	}
	return out, nil
}

// Event implements ports.FacebookEvents.
func (c *Client) Event(ctx context.Context, eventID, token string) (ports.FacebookEvent, error) {
	var g graphEvent
	if err := c.call(ctx, http.MethodGet, "/"+url.PathEscape(eventID), url.Values{"fields": {eventFields}}, token, &g); err != nil {
		return ports.FacebookEvent{}, err
	}
	return g.event(), nil
}

// coverHostAllowed reports whether a cover URL points to Facebook's CDN.
func coverHostAllowed(u *url.URL) bool {
	h := strings.ToLower(u.Hostname())
	return u.Scheme == "https" && u.User == nil && u.Port() == "" &&
		(h == "fbcdn.net" || strings.HasSuffix(h, ".fbcdn.net") || h == "scontent.xx.fbcdn.net")
}

// CoverImage implements ports.FacebookEvents: https from a Facebook CDN host
// only, no redirects elsewhere, at most MaxCoverBytes (T7).
func (c *Client) CoverImage(ctx context.Context, raw string) ([]byte, error) {
	u, err := url.Parse(raw)
	if err != nil || !coverHostAllowed(u) {
		return nil, errors.New("meta: cover address is not a Facebook CDN https address")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	hc := *c.HTTP
	hc.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) > 3 || !coverHostAllowed(r.URL) {
			return errors.New("meta: cover redirected off the Facebook CDN")
		}
		return nil
	}
	resp, err := hc.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // the signed CDN address is not logged
		}
		return nil, fmt.Errorf("meta: cover: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("meta: cover: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxCoverBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxCoverBytes {
		return nil, errors.New("meta: cover too large")
	}
	return data, nil
}

// LongLivedPageToken implements ports.FacebookEvents.
func (c *Client) LongLivedPageToken(ctx context.Context, userToken, pageID string) (string, ports.SocialPage, time.Time, error) {
	var ll struct {
		AccessToken string `json:"access_token"`
	}
	if err := c.call(ctx, http.MethodGet, "/oauth/access_token", url.Values{
		"grant_type": {"fb_exchange_token"}, "client_id": {c.AppID}, "client_secret": {c.AppSecret}, "fb_exchange_token": {userToken},
	}, "", &ll); err != nil {
		return "", ports.SocialPage{}, time.Time{}, err
	}
	if ll.AccessToken == "" {
		return "", ports.SocialPage{}, time.Time{}, errors.New("meta: no long-lived token in the answer")
	}
	// A page token derived from a long-lived user token does not expire.
	token, page, err := c.PageToken(ctx, ll.AccessToken, pageID)
	if err != nil {
		return "", ports.SocialPage{}, time.Time{}, err
	}
	var dbg struct {
		Data struct {
			IsValid           bool  `json:"is_valid"`
			DataAccessExpires int64 `json:"data_access_expires_at"`
		} `json:"data"`
	}
	if err := c.call(ctx, http.MethodGet, "/debug_token", url.Values{
		"input_token": {token}, "access_token": {c.AppID + "|" + c.AppSecret},
	}, "", &dbg); err != nil {
		return "", ports.SocialPage{}, time.Time{}, err
	}
	if !dbg.Data.IsValid {
		return "", ports.SocialPage{}, time.Time{}, ports.ErrTokenRejected
	}
	var expires time.Time
	if dbg.Data.DataAccessExpires > 0 {
		expires = time.Unix(dbg.Data.DataAccessExpires, 0).UTC()
	}
	return token, page, expires, nil
}
