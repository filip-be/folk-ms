// Package meta implements ports.SocialPublisher on the Meta Graph API:
// Facebook Login to a short-lived user token, the Page token and linked
// Instagram account of one Page, Page photo posts and Instagram image posts.
// No token outlives the request that obtained it.
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
	"instagram_basic", "instagram_content_publish", "business_management",
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
}

func (e *GraphError) Error() string {
	return fmt.Sprintf("meta: HTTP %d code %d: %s", e.Status, e.Code, e.Message)
}

func (c *Client) proof(token string) string {
	m := hmac.New(sha256.New, []byte(c.AppSecret))
	m.Write([]byte(token))
	return hex.EncodeToString(m.Sum(nil))
}

func (c *Client) call(ctx context.Context, method, path string, params url.Values, token string, out any) error {
	if params == nil {
		params = url.Values{}
	}
	if token != "" {
		params.Set("access_token", token)
		params.Set("appsecret_proof", c.proof(token))
	}
	u := c.GraphURL + "/" + c.Version + path
	var body io.Reader
	if method == http.MethodGet {
		u += "?" + params.Encode()
	} else {
		body = strings.NewReader(params.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		// A *url.Error quotes the full URL, whose query carries tokens and
		// the app secret: never let it reach logs or stored records.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("meta: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		var ge struct {
			Error struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &ge)
		return &GraphError{Status: resp.StatusCode, Message: ge.Error.Message, Code: ge.Error.Code}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
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

// Pages implements ports.SocialPublisher. Page tokens are not requested.
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

// PageToken implements ports.SocialPublisher. Graph answers a Page the user
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
		if err := c.call(ctx, http.MethodGet, "/"+container.ID, url.Values{"fields": {"status_code"}}, token, &st); err != nil {
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
	if err := c.call(ctx, http.MethodGet, "/"+pub.ID, url.Values{"fields": {"permalink"}}, token, &media); err == nil {
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
	// NoAccess makes PageToken refuse every Page.
	NoAccess bool
	FailFB   error
	FailIG   error
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

func (r *Recorder) PostFacebookPhoto(_ context.Context, pageID, _, imageURL, caption string) (ports.PostResult, error) {
	if r.FailFB != nil {
		return ports.PostResult{}, r.FailFB
	}
	r.Posts = append(r.Posts, RecordedPost{"facebook", pageID, imageURL, caption})
	id := fmt.Sprintf("%s_%d", pageID, len(r.Posts))
	return ports.PostResult{ID: id, URL: "https://www.facebook.com/" + id}, nil
}

func (r *Recorder) PostInstagramImage(_ context.Context, ig, _, imageURL, caption string) (ports.PostResult, error) {
	if r.FailIG != nil {
		return ports.PostResult{}, r.FailIG
	}
	r.Posts = append(r.Posts, RecordedPost{"instagram", ig, imageURL, caption})
	return ports.PostResult{ID: fmt.Sprint(len(r.Posts)), URL: fmt.Sprintf("https://www.instagram.com/p/fake%d/", len(r.Posts))}, nil
}

func (r *Recorder) InstagramQuota(context.Context, string, string) (int, int, error) {
	return 0, 100, nil
}
