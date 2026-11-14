// Package meta implements ports.SocialPublisher and ports.FacebookEvents on
// the Meta Graph API: Facebook Login to a short-lived user token, the
// Facebook page token and linked Instagram account of one Facebook page,
// Facebook page photo posts, Instagram image posts, reading the page's
// Facebook Events, and the long-lived page token Autosync keeps (ADR 0010).
// The adapter itself keeps no token.
package meta

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// Scopes requested at Facebook Login.
var Scopes = []string{
	"pages_show_list", "pages_read_engagement", "pages_manage_posts",
	"instagram_basic", "instagram_content_publish",
}

// Client talks to Graph.
type Client struct {
	AppID       string
	AppSecret   string
	Version     string // e.g. v24.0
	GraphURL    string // https://graph.facebook.com
	DialogURL   string // https://www.facebook.com
	HTTP        *http.Client
	PollDelay   time.Duration
	PollRetries int
}

// New returns a client with Meta's endpoints.
func New(appID, appSecret, version string) *Client {
	if version == "" {
		version = "v24.0"
	}
	return &Client{
		AppID: appID, AppSecret: appSecret, Version: version,
		GraphURL: "https://graph.facebook.com", DialogURL: "https://www.facebook.com",
		HTTP: &http.Client{Timeout: 60 * time.Second}, PollDelay: 2 * time.Second, PollRetries: 10,
	}
}

// GraphError is an error answer from Graph.
type GraphError struct {
	Status  int
	Message string
	Code    int
	Subcode int
}

func (e *GraphError) Error() string {
	return fmt.Sprintf("meta: HTTP %d code %d/%d: %s", e.Status, e.Code, e.Subcode, e.Message)
}

// Unwrap maps Graph's answers to the port's errors: code 190 (and 102, the
// older session error) is a rejected token; code 100 subcode 33 is an
// object that does not exist or cannot be loaded.
func (e *GraphError) Unwrap() error {
	switch {
	case e.Code == 190 || e.Code == 102:
		return ports.ErrTokenRejected
	case e.Code == 100 && e.Subcode == 33:
		return ports.ErrEventGone
	}
	return nil
}

func (c *Client) proof(token string) string {
	m := hmac.New(sha256.New, []byte(c.AppSecret))
	m.Write([]byte(token))
	return hex.EncodeToString(m.Sum(nil))
}

// call sends one Graph request. A user or page token travels in the
// Authorization header, never in the URL, so a URL that reaches a log, an
// error or a proxy carries no token (T9); the appsecret_proof stays a
// parameter, as Graph requires. Errors never quote the URL, and Graph's
// message is scrubbed of the token and the app secret.
func (c *Client) call(ctx context.Context, method, path string, params url.Values, token string, out any) error {
	if params == nil {
		params = url.Values{}
	}
	if token != "" {
		params.Set("appsecret_proof", c.proof(token))
	}
	u := c.GraphURL + "/" + c.Version + path
	var body io.Reader
	if method == http.MethodGet || method == http.MethodDelete {
		u += "?" + params.Encode()
	} else {
		body = strings.NewReader(params.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		// The error quotes the URL, whose query may carry the app secret.
		return fmt.Errorf("meta: %s %s: invalid request", method, path)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		// A *url.Error quotes the full URL: keep only the cause.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("meta: %s %s: %s", method, path, c.scrub(err.Error(), token))
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		var ge struct {
			Error struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
				Subcode int    `json:"error_subcode"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &ge)
		return &GraphError{Status: resp.StatusCode, Message: c.scrub(ge.Error.Message, token, params.Get("fb_exchange_token"), params.Get("input_token")), Code: ge.Error.Code, Subcode: ge.Error.Subcode}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

// scrub removes secrets from a message that may be stored or logged, and
// bounds its length.
func (c *Client) scrub(msg string, secrets ...string) string {
	for _, s := range append(secrets, c.AppSecret) {
		if len(s) >= 8 {
			msg = strings.ReplaceAll(msg, s, "[redacted]")
		}
	}
	if len(msg) > 300 {
		msg = msg[:300] + "…"
	}
	return msg
}

// LoginURL implements ports.SocialPublisher. auth_type=rerequest asks
// again for permissions the user declined before.
func (c *Client) LoginURL(state, redirectURI string) string {
	q := url.Values{
		"client_id": {c.AppID}, "redirect_uri": {redirectURI}, "state": {state},
		"response_type": {"code"}, "scope": {strings.Join(Scopes, ",")}, "auth_type": {"rerequest"},
	}
	return c.DialogURL + "/" + c.Version + "/dialog/oauth?" + q.Encode()
}

// UserToken implements ports.SocialPublisher: the login code becomes a
// short-lived user token (about an hour). It is never exchanged for a
// long-lived one.
func (c *Client) UserToken(ctx context.Context, code, redirectURI string) (string, error) {
	var r struct {
		AccessToken string `json:"access_token"`
	}
	if err := c.call(ctx, http.MethodGet, "/oauth/access_token", url.Values{
		"client_id": {c.AppID}, "client_secret": {c.AppSecret}, "redirect_uri": {redirectURI}, "code": {code},
	}, "", &r); err != nil {
		return "", err
	}
	if r.AccessToken == "" {
		return "", errors.New("meta: no access token in the login answer")
	}
	return r.AccessToken, nil
}

type graphPage struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	AccessToken string `json:"access_token"`
	IG          *struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"instagram_business_account"`
}

func (g graphPage) page() ports.SocialPage {
	p := ports.SocialPage{ID: g.ID, Name: g.Name}
	if g.IG != nil {
		p.IGUserID, p.IGUsername = g.IG.ID, g.IG.Username
	}
	return p
}

// Pages implements ports.SocialPublisher. Facebook page tokens are not requested.
func (c *Client) Pages(ctx context.Context, userToken string) ([]ports.SocialPage, error) {
	var accounts struct {
		Data []graphPage `json:"data"`
	}
	if err := c.call(ctx, http.MethodGet, "/me/accounts", url.Values{
		"fields": {"id,name,instagram_business_account{id,username}"}, "limit": {"100"},
	}, userToken, &accounts); err != nil {
		return nil, err
	}
	pages := []ports.SocialPage{}
	for _, a := range accounts.Data {
		pages = append(pages, a.page())
	}
	return pages, nil
}

// PageToken implements ports.SocialPublisher. Graph answers a Facebook page the user
// has no role on without a token, or with a permission error.
func (c *Client) PageToken(ctx context.Context, userToken, pageID string) (string, ports.SocialPage, error) {
	var g graphPage
	err := c.call(ctx, http.MethodGet, "/"+url.PathEscape(pageID), url.Values{
		"fields": {"id,name,access_token,instagram_business_account{id,username}"},
	}, userToken, &g)
	var ge *GraphError
	if errors.As(err, &ge) && (ge.Code == 10 || ge.Code == 100 || ge.Code == 200 || ge.Status == http.StatusForbidden) {
		return "", ports.SocialPage{}, fmt.Errorf("%w: %v", ports.ErrNoPageAccess, err)
	}
	if err != nil {
		return "", ports.SocialPage{}, err
	}
	if g.AccessToken == "" {
		return "", ports.SocialPage{}, ports.ErrNoPageAccess
	}
	return g.AccessToken, g.page(), nil
}

// PostFacebookPhoto implements ports.SocialPublisher.
func (c *Client) PostFacebookPhoto(ctx context.Context, pageID, token, imageURL, caption string) (ports.PostResult, error) {
	var r struct {
		ID     string `json:"id"`
		PostID string `json:"post_id"`
	}
	if err := c.call(ctx, http.MethodPost, "/"+url.PathEscape(pageID)+"/photos", url.Values{
		"url": {imageURL}, "message": {caption}, "published": {"true"},
	}, token, &r); err != nil {
		return ports.PostResult{}, err
	}
	id := r.PostID
	if id == "" {
		id = r.ID
	}
	return ports.PostResult{ID: id, URL: "https://www.facebook.com/" + id}, nil
}

// PostInstagramImage implements ports.SocialPublisher: create a media
// container, wait until it is ready, publish it, then read its permalink.
func (c *Client) PostInstagramImage(ctx context.Context, igUserID, token, imageURL, caption string) (ports.PostResult, error) {
	var container struct {
		ID string `json:"id"`
	}
	if err := c.call(ctx, http.MethodPost, "/"+url.PathEscape(igUserID)+"/media", url.Values{
		"image_url": {imageURL}, "caption": {caption},
	}, token, &container); err != nil {
		return ports.PostResult{}, err
	}
	for i := 0; ; i++ {
		var st struct {
			StatusCode string `json:"status_code"`
		}
		if err := c.call(ctx, http.MethodGet, "/"+url.PathEscape(container.ID), url.Values{"fields": {"status_code"}}, token, &st); err != nil {
			return ports.PostResult{}, err
		}
		if st.StatusCode == "FINISHED" || st.StatusCode == "" {
			break
		}
		if st.StatusCode == "ERROR" || st.StatusCode == "EXPIRED" || i >= c.PollRetries {
			return ports.PostResult{}, fmt.Errorf("meta: Instagram container %s", strings.ToLower(st.StatusCode))
		}
		select {
		case <-ctx.Done():
			return ports.PostResult{}, ctx.Err()
		case <-time.After(c.PollDelay):
		}
	}
	var pub struct {
		ID string `json:"id"`
	}
	if err := c.call(ctx, http.MethodPost, "/"+url.PathEscape(igUserID)+"/media_publish", url.Values{
		"creation_id": {container.ID},
	}, token, &pub); err != nil {
		return ports.PostResult{}, err
	}
	res := ports.PostResult{ID: pub.ID}
	var media struct {
		Permalink string `json:"permalink"`
	}
	if err := c.call(ctx, http.MethodGet, "/"+url.PathEscape(pub.ID), url.Values{"fields": {"permalink"}}, token, &media); err == nil {
		res.URL = media.Permalink
	}
	return res, nil
}

// InstagramQuota implements ports.SocialPublisher.
func (c *Client) InstagramQuota(ctx context.Context, igUserID, token string) (int, int, error) {
	var r struct {
		Data []struct {
			QuotaUsage int `json:"quota_usage"`
			Config     struct {
				QuotaTotal int `json:"quota_total"`
			} `json:"config"`
		} `json:"data"`
	}
	if err := c.call(ctx, http.MethodGet, "/"+url.PathEscape(igUserID)+"/content_publishing_limit", url.Values{
		"fields": {"quota_usage,config"},
	}, token, &r); err != nil {
		return 0, 0, err
	}
	if len(r.Data) == 0 {
		return 0, 0, errors.New("meta: no quota data")
	}
	return r.Data[0].QuotaUsage, r.Data[0].Config.QuotaTotal, nil
}

// Recorder is a fake SocialPublisher for tests and local development: it
// records posts instead of sending them.
type Recorder struct {
	Posts   []RecordedPost
	PagesFn func() []ports.SocialPage
	// Tokens counts the user tokens issued, one per Meta sign-in.
	Tokens int
	// NoAccess makes PageToken refuse every Facebook page.
	NoAccess bool
	FailFB   error
	FailIG   error
	// Facebook Events: EventsFn replaces the canned events; Calls records
	// list and read calls with the token used.
	EventsFn    func() []ports.FacebookEvent
	Calls       []string
	RejectToken bool
	FailList    error
	FailRead    error
	Cover       []byte
	// Tokens used for posts.
	PostTokens []string
}

// RecordedPost is one call to the Recorder.
type RecordedPost struct {
	Platform, Target, ImageURL, Caption string
}

func (r *Recorder) LoginURL(state, redirectURI string) string {
	return redirectURI + "?" + url.Values{"code": {"fake-code"}, "state": {state}}.Encode()
}

func (r *Recorder) UserToken(context.Context, string, string) (string, error) {
	r.Tokens++
	return fmt.Sprintf("user-token-%d", r.Tokens), nil
}

func (r *Recorder) pages() []ports.SocialPage {
	if r.PagesFn != nil {
		return r.PagesFn()
	}
	return []ports.SocialPage{{ID: "1001", Name: "Strona testowa", IGUserID: "2002", IGUsername: "test_ig"}}
}

func (r *Recorder) Pages(context.Context, string) ([]ports.SocialPage, error) {
	return r.pages(), nil
}

func (r *Recorder) PageToken(_ context.Context, _, pageID string) (string, ports.SocialPage, error) {
	if !r.NoAccess {
		for _, p := range r.pages() {
			if p.ID == pageID {
				return "page-token-" + p.ID, p, nil
			}
		}
	}
	return "", ports.SocialPage{}, ports.ErrNoPageAccess
}

func (r *Recorder) PostFacebookPhoto(_ context.Context, pageID, token, imageURL, caption string) (ports.PostResult, error) {
	r.PostTokens = append(r.PostTokens, token)
	if r.FailFB != nil {
		return ports.PostResult{}, r.FailFB
	}
	r.Posts = append(r.Posts, RecordedPost{"facebook", pageID, imageURL, caption})
	id := fmt.Sprintf("%s_%d", pageID, len(r.Posts))
	return ports.PostResult{ID: id, URL: "https://www.facebook.com/" + id}, nil
}

func (r *Recorder) PostInstagramImage(_ context.Context, ig, token, imageURL, caption string) (ports.PostResult, error) {
	r.PostTokens = append(r.PostTokens, token)
	if r.FailIG != nil {
		return ports.PostResult{}, r.FailIG
	}
	r.Posts = append(r.Posts, RecordedPost{"instagram", ig, imageURL, caption})
	return ports.PostResult{ID: fmt.Sprint(len(r.Posts)), URL: fmt.Sprintf("https://www.instagram.com/p/fake%d/", len(r.Posts))}, nil
}

func (r *Recorder) InstagramQuota(context.Context, string, string) (int, int, error) {
	return 0, 100, nil
}

// RecorderEvents are the canned Facebook Events of the Recorder's page 1001
// for local runs: a single bal, a repeating jam and a cancelled concert.
func RecorderEvents(now time.Time) []ports.FacebookEvent {
	loc, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		loc = time.UTC
	}
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	at := func(days, hour int) time.Time { return day.AddDate(0, 0, days).Add(time.Duration(hour) * time.Hour) }
	return []ports.FacebookEvent{
		{ID: "9001", Name: "Bal folkowy", Description: "Muzyka na żywo i warsztaty dla początkujących.",
			Start: at(10, 19), End: at(11, 1), Place: &ports.FacebookPlace{Name: "Zamek", Street: "Św. Marcin 80/82", City: "Poznań"},
			TicketURL: "https://bilety.example/bal", Updated: day},
		{ID: "9002", Name: "Jam czwartkowy", Description: "Otwarte granie i tańce.", Start: at(3, 19), End: at(3, 22),
			Place: &ports.FacebookPlace{Name: "Klub Dragon"}, Updated: day,
			Times: []ports.FacebookEventTime{
				{ID: "9003", Start: at(3, 19), End: at(3, 22)},
				{ID: "9004", Start: at(17, 19), End: at(17, 22)},
			}},
		{ID: "9005", Name: "Koncert odwołany", Start: at(20, 18), Online: true, Cancelled: true, Updated: day},
	}
}

func (r *Recorder) events() []ports.FacebookEvent {
	if r.EventsFn != nil {
		return r.EventsFn()
	}
	return RecorderEvents(time.Now())
}

// PageEvents implements ports.FacebookEvents.
func (r *Recorder) PageEvents(_ context.Context, pageID, token string) ([]ports.FacebookEvent, error) {
	r.Calls = append(r.Calls, "list "+pageID+" "+token)
	if r.RejectToken {
		return nil, ports.ErrTokenRejected
	}
	if r.FailList != nil {
		return nil, r.FailList
	}
	return r.events(), nil
}

// Event implements ports.FacebookEvents: an id not among the events is gone.
func (r *Recorder) Event(_ context.Context, id, token string) (ports.FacebookEvent, error) {
	r.Calls = append(r.Calls, "read "+id+" "+token)
	if r.RejectToken {
		return ports.FacebookEvent{}, ports.ErrTokenRejected
	}
	if r.FailRead != nil {
		return ports.FacebookEvent{}, r.FailRead
	}
	for _, e := range r.events() {
		if e.ID == id {
			return e, nil
		}
	}
	return ports.FacebookEvent{}, ports.ErrEventGone
}

// CoverImage implements ports.FacebookEvents.
func (r *Recorder) CoverImage(context.Context, string) ([]byte, error) {
	if r.Cover != nil {
		return r.Cover, nil
	}
	return nil, errors.New("recorder: no cover")
}

// LongLivedPageToken implements ports.FacebookEvents.
func (r *Recorder) LongLivedPageToken(_ context.Context, _, pageID string) (string, ports.SocialPage, time.Time, error) {
	if r.NoAccess {
		return "", ports.SocialPage{}, time.Time{}, ports.ErrNoPageAccess
	}
	for _, p := range r.pages() {
		if p.ID == pageID {
			r.Tokens++
			return fmt.Sprintf("long-page-token-%s-%d", p.ID, r.Tokens), p, time.Now().Add(90 * 24 * time.Hour).UTC(), nil
		}
	}
	return "", ports.SocialPage{}, time.Time{}, ports.ErrNoPageAccess
}
