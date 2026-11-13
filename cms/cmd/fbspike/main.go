// Command fbspike is the throwaway go/no-go check for "Facebook Events are the
// source of Events" (plan step 1). It is not shipped: the CMS never imports it,
// and it is deleted once the result is on record.
//
// It answers, for one test Facebook page and this Meta app:
//
//  1. Does GET /{page-id}/events return the page's events with a page token?
//  2. Which event fields come back?
//  3. Repeating events: are child dates and their ids returned (event_times)?
//  4. Co-hosted events: listed or not; readable by id; is the host visible?
//  5. What Graph answers for a deleted, private or draft event read by id.
//  6. The long-lived page token: expiry, data access expiry, scopes, and
//     whether a call without appsecret_proof is refused.
//  7. Rate-limit headers.
//
// It prints a Markdown report to stdout. Tokens and the app secret never appear
// in it: every known secret is replaced by [redacted] before printing. Nothing is
// written to disk.
//
// Usage (from cms/):
//
//	META_APP_ID=… META_APP_SECRET=… go run ./cmd/fbspike -page 123 -login > report.md
//	META_APP_ID=… META_APP_SECRET=… FBSPIKE_USER_TOKEN=… go run ./cmd/fbspike -page 123 > report.md
//
// See README.md next to this file.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// eventFields are every Graph Event field the new design might use.
var eventFields = []string{
	"id", "name", "description", "start_time", "end_time", "timezone", "place",
	"cover", "ticket_uri", "is_canceled", "updated_time", "event_times",
	"is_online", "type", "owner", "is_draft", "category",
}

// baseScopes are the ones the CMS asks for today, minus business_management
// (plan T12). -scopes adds more.
var baseScopes = []string{
	"pages_show_list", "pages_read_engagement", "pages_manage_posts",
	"instagram_basic", "instagram_content_publish",
}

// usageHeaders are Graph's rate-limit headers.
var usageHeaders = []string{"X-App-Usage", "X-Page-Usage", "X-Business-Use-Case-Usage", "X-Ad-Account-Usage"}

type config struct {
	AppID, AppSecret string
	Version          string
	GraphURL         string
	DialogURL        string
	PageID           string
	UserToken        string // short-lived user token; empty with Login
	Login            bool
	LoginAddr        string
	ExtraScopes      []string
	Probe            []string // event ids expected gone, private or draft
	CoHosted         []string // event ids co-hosted by the page
	MaxPages         int
}

type listFlag []string

func (l *listFlag) String() string { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error {
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			*l = append(*l, s)
		}
	}
	return nil
}

func main() {
	var cfg config
	var probe, cohosted, scopes listFlag
	flag.StringVar(&cfg.PageID, "page", "", "id of the test Facebook page (required)")
	flag.BoolVar(&cfg.Login, "login", false, "sign in with Facebook through a browser instead of FBSPIKE_USER_TOKEN")
	flag.StringVar(&cfg.LoginAddr, "login-addr", "localhost:8765", "address of the local sign-in callback")
	flag.StringVar(&cfg.Version, "version", "v24.0", "Graph API version")
	flag.Var(&probe, "probe", "event ids to read by id: deleted, private or draft events (comma-separated, repeatable)")
	flag.Var(&cohosted, "cohosted", "event ids the page co-hosts but another page created (comma-separated, repeatable)")
	flag.Var(&scopes, "scopes", "extra permissions to request at sign-in (comma-separated)")
	flag.IntVar(&cfg.MaxPages, "max-pages", 4, "most pages of /events to read per variant")
	flag.Parse()
	cfg.Probe, cfg.CoHosted, cfg.ExtraScopes = probe, cohosted, scopes
	cfg.AppID, cfg.AppSecret = os.Getenv("META_APP_ID"), os.Getenv("META_APP_SECRET")
	cfg.UserToken = os.Getenv("FBSPIKE_USER_TOKEN")
	cfg.GraphURL, cfg.DialogURL = "https://graph.facebook.com", "https://www.facebook.com"
	if cfg.PageID == "" || cfg.AppID == "" || cfg.AppSecret == "" || (cfg.UserToken == "" && !cfg.Login) {
		fmt.Fprintln(os.Stderr, "fbspike: need -page, META_APP_ID, META_APP_SECRET, and FBSPIKE_USER_TOKEN or -login")
		flag.Usage()
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if cfg.Login {
		tok, err := login(ctx, &cfg, os.Stderr)
		if err != nil {
			fmt.Fprintln(os.Stderr, "fbspike: sign-in:", err)
			os.Exit(1)
		}
		cfg.UserToken = tok
	}
	if err := run(ctx, cfg, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "fbspike:", err)
		os.Exit(1)
	}
}

// --- Graph -----------------------------------------------------------------

type graphErr struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    int    `json:"code"`
	Subcode int    `json:"error_subcode"`
	Trace   string `json:"fbtrace_id"`
}

type answer struct {
	Status int
	Body   json.RawMessage
	Err    *graphErr
	Usage  map[string]string
	NetErr error
}

func (a answer) ok() bool { return a.NetErr == nil && a.Status == http.StatusOK && a.Err == nil }

func (a answer) describe() string {
	switch {
	case a.NetErr != nil:
		return "network error: " + a.NetErr.Error()
	case a.Err != nil:
		return fmt.Sprintf("HTTP %d, code %d, subcode %d, type %s: %s", a.Status, a.Err.Code, a.Err.Subcode, a.Err.Type, a.Err.Message)
	default:
		return fmt.Sprintf("HTTP %d", a.Status)
	}
}

type spike struct {
	cfg     config
	http    *http.Client
	secrets []string
	usage   map[string][]string
	out     io.Writer
}

func (s *spike) remember(secret string) {
	if secret != "" {
		s.secrets = append(s.secrets, secret)
	}
}

// redact removes every known secret, and the HMAC of each, from text.
func (s *spike) redact(text string) string {
	for _, sec := range s.secrets {
		text = strings.ReplaceAll(text, sec, "[redacted]")
		text = strings.ReplaceAll(text, url.QueryEscape(sec), "[redacted]")
	}
	return text
}

func (s *spike) printf(format string, args ...any) {
	fmt.Fprint(s.out, s.redact(fmt.Sprintf(format, args...)))
}

func (s *spike) proof(token string) string {
	m := hmac.New(sha256.New, []byte(s.cfg.AppSecret))
	m.Write([]byte(token))
	return hex.EncodeToString(m.Sum(nil))
}

// get calls Graph. withProof false leaves appsecret_proof out, to test
// "Require App Secret".
func (s *spike) get(ctx context.Context, path string, q url.Values, token string, withProof bool) answer {
	if q == nil {
		q = url.Values{}
	}
	if token != "" {
		q.Set("access_token", token)
		if withProof {
			q.Set("appsecret_proof", s.proof(token))
		}
	}
	return s.getURL(ctx, s.cfg.GraphURL+"/"+s.cfg.Version+path+"?"+q.Encode(), path)
}

func (s *spike) getURL(ctx context.Context, u, label string) answer {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return answer{NetErr: errors.New("bad request")}
	}
	resp, err := s.http.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // the URL carries the token
		}
		return answer{NetErr: err}
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	a := answer{Status: resp.StatusCode, Body: data, Usage: map[string]string{}}
	for _, h := range usageHeaders {
		if v := resp.Header.Get(h); v != "" {
			a.Usage[h] = v
			s.usage[h] = append(s.usage[h], label+": "+v)
		}
	}
	var e struct {
		Error *graphErr `json:"error"`
	}
	if json.Unmarshal(data, &e) == nil && e.Error != nil {
		a.Err = e.Error
	}
	return a
}

// --- Sign-in ---------------------------------------------------------------

func login(ctx context.Context, cfg *config, log io.Writer) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	state := hex.EncodeToString(b)
	redirect := "http://" + cfg.LoginAddr + "/callback"
	scopes := append(append([]string{}, baseScopes...), cfg.ExtraScopes...)
	q := url.Values{
		"client_id": {cfg.AppID}, "redirect_uri": {redirect}, "state": {state},
		"response_type": {"code"}, "scope": {strings.Join(scopes, ",")}, "auth_type": {"rerequest"},
	}
	ln, err := net.Listen("tcp", cfg.LoginAddr)
	if err != nil {
		return "", err
	}
	codes := make(chan string, 1)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			msg := "no code: " + r.URL.Query().Get("error_description")
			http.Error(w, msg, http.StatusBadRequest)
			code = "\x00" + msg
		} else {
			fmt.Fprintln(w, "Signed in. Return to the terminal.")
		}
		select {
		case codes <- code:
		default:
		}
	})}
	go srv.Serve(ln)
	defer srv.Close()
	fmt.Fprintf(log, "Add %s to the Meta app's Valid OAuth Redirect URIs if Facebook refuses it, then open:\n\n%s\n\n",
		redirect, cfg.DialogURL+"/"+cfg.Version+"/dialog/oauth?"+q.Encode())
	var code string
	select {
	case code = <-codes:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	if msg, failed := strings.CutPrefix(code, "\x00"); failed {
		return "", errors.New(msg)
	}
	s := &spike{cfg: *cfg, http: &http.Client{Timeout: 60 * time.Second}, usage: map[string][]string{}}
	a := s.get(ctx, "/oauth/access_token", url.Values{
		"client_id": {cfg.AppID}, "client_secret": {cfg.AppSecret}, "redirect_uri": {redirect}, "code": {code},
	}, "", false)
	var r struct {
		AccessToken string `json:"access_token"`
	}
	if !a.ok() || json.Unmarshal(a.Body, &r) != nil || r.AccessToken == "" {
		s.remember(cfg.AppSecret)
		s.remember(code)
		return "", errors.New(s.redact(a.describe()))
	}
	return r.AccessToken, nil
}

// --- The checks ------------------------------------------------------------

type event struct {
	ID         string                     `json:"id"`
	Name       string                     `json:"name"`
	EventTimes []eventTime                `json:"event_times"`
	Owner      *struct{ ID, Name string } `json:"owner"`
	raw        map[string]json.RawMessage
}

type eventTime struct {
	ID        string `json:"id"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

type listing struct {
	Data   []json.RawMessage `json:"data"`
	Paging struct {
		Next string `json:"next"`
	} `json:"paging"`
}

func parseEvents(raws []json.RawMessage) []event {
	var out []event
	for _, r := range raws {
		var e event
		if json.Unmarshal(r, &e) == nil {
			_ = json.Unmarshal(r, &e.raw)
			out = append(out, e)
		}
	}
	return out
}

// list reads up to MaxPages pages of the events edge.
func (s *spike) list(ctx context.Context, token string, q url.Values) ([]event, answer, int) {
	first := s.get(ctx, "/"+url.PathEscape(s.cfg.PageID)+"/events", q, token, true)
	a := first
	var all []json.RawMessage
	pages := 0
	for {
		if !a.ok() {
			if pages == 0 {
				return nil, a, 0
			}
			break
		}
		var l listing
		if json.Unmarshal(a.Body, &l) != nil {
			break
		}
		all = append(all, l.Data...)
		pages++
		if l.Paging.Next == "" || pages >= s.cfg.MaxPages {
			break
		}
		a = s.getURL(ctx, l.Paging.Next, "/{page-id}/events (next)")
	}
	return parseEvents(all), first, pages
}

func run(ctx context.Context, cfg config, out io.Writer) error {
	s := &spike{cfg: cfg, http: &http.Client{Timeout: 60 * time.Second}, usage: map[string][]string{}, out: out}
	s.remember(cfg.AppSecret)
	s.remember(cfg.UserToken)
	s.remember(cfg.AppID + "|" + cfg.AppSecret)
	page := url.PathEscape(cfg.PageID)

	s.printf("# fbspike report\n\nGraph %s, page %s, run %s.\n\n", cfg.Version, cfg.PageID, time.Now().UTC().Format(time.RFC3339))

	// 6a. Long-lived user token, then the page token derived from it.
	s.printf("## 6. Tokens\n\n")
	a := s.get(ctx, "/oauth/access_token", url.Values{
		"grant_type": {"fb_exchange_token"}, "client_id": {cfg.AppID},
		"client_secret": {cfg.AppSecret}, "fb_exchange_token": {cfg.UserToken},
	}, "", false)
	var ll struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if !a.ok() || json.Unmarshal(a.Body, &ll) != nil || ll.AccessToken == "" {
		s.printf("- Long-lived user token: **failed** (%s). Stopping: no page token can be derived.\n", a.describe())
		return nil
	}
	s.remember(ll.AccessToken)
	s.printf("- Long-lived user token: obtained, expires_in %d s (about %d days).\n", ll.ExpiresIn, ll.ExpiresIn/86400)

	a = s.get(ctx, "/"+page, url.Values{"fields": {"id,name,access_token"}}, ll.AccessToken, true)
	var pg struct {
		ID, Name    string
		AccessToken string `json:"access_token"`
	}
	if !a.ok() || json.Unmarshal(a.Body, &pg) != nil || pg.AccessToken == "" {
		s.printf("- Page token: **failed** (%s). Does the signed-in person have a role on page %s?\n", a.describe(), cfg.PageID)
		return nil
	}
	pageToken := pg.AccessToken
	s.remember(pageToken)
	s.printf("- Page token for **%s** (%s): obtained.\n", pg.Name, pg.ID)

	appToken := cfg.AppID + "|" + cfg.AppSecret
	for _, t := range []struct{ label, tok string }{{"Long-lived user token", ll.AccessToken}, {"Page token", pageToken}} {
		a = s.get(ctx, "/debug_token", url.Values{"input_token": {t.tok}}, appToken, false)
		var d struct {
			Data struct {
				Type              string   `json:"type"`
				IsValid           bool     `json:"is_valid"`
				ExpiresAt         int64    `json:"expires_at"`
				DataAccessExpires int64    `json:"data_access_expires_at"`
				Scopes            []string `json:"scopes"`
				ProfileID         string   `json:"profile_id"`
			} `json:"data"`
		}
		if !a.ok() || json.Unmarshal(a.Body, &d) != nil {
			s.printf("- debug_token (%s): **failed** (%s)\n", t.label, a.describe())
			continue
		}
		s.printf("- debug_token (%s): type %s, valid %v, expires_at %s, data_access_expires_at %s, scopes `%s`\n",
			t.label, d.Data.Type, d.Data.IsValid, unix(d.Data.ExpiresAt), unix(d.Data.DataAccessExpires), strings.Join(d.Data.Scopes, ","))
	}

	a = s.get(ctx, "/"+page, url.Values{"fields": {"id"}}, pageToken, false)
	if a.ok() {
		s.printf("- Call **without** appsecret_proof: **accepted**. \"Require App Secret\" is off; turn it on (App settings, Advanced) and run again.\n")
	} else {
		s.printf("- Call **without** appsecret_proof: refused (%s). Good if \"Require App Secret\" is on.\n", a.describe())
	}
	a = s.get(ctx, "/"+page, url.Values{"fields": {"id"}}, pageToken, true)
	s.printf("- Call **with** appsecret_proof: %s.\n\n", okOr(a))

	// 1. The events edge.
	s.printf("## 1. GET /{page-id}/events\n\n")
	fields := strings.Join(eventFields, ",")
	variants := []struct {
		label string
		token string
		q     url.Values
	}{
		{"page token, all fields", pageToken, url.Values{"fields": {fields}, "limit": {"25"}}},
		{"page token, time_filter=upcoming", pageToken, url.Values{"fields": {fields}, "limit": {"25"}, "time_filter": {"upcoming"}}},
		{"page token, include_canceled=true", pageToken, url.Values{"fields": {fields}, "limit": {"25"}, "include_canceled": {"true"}}},
		{"long-lived user token, all fields", ll.AccessToken, url.Values{"fields": {fields}, "limit": {"25"}}},
	}
	var events []event
	readable := false
	for i, v := range variants {
		evs, first, pages := s.list(ctx, v.token, v.q)
		if !first.ok() {
			s.printf("- %s: **failed** (%s)\n", v.label, first.describe())
			continue
		}
		readable = true
		s.printf("- %s: **%d events** in %d page(s).\n", v.label, len(evs), pages)
		if i == 0 || (len(events) == 0 && len(evs) > 0) {
			events = evs
		}
	}
	if !readable {
		// Tell "edge unavailable" apart from "a field is unknown".
		a = s.get(ctx, "/"+page+"/events", url.Values{"fields": {"id,name"}}, pageToken, true)
		s.printf("- page token, fields id,name only: %s\n", okOr(a))
		if a.ok() {
			readable = true
			var l listing
			_ = json.Unmarshal(a.Body, &l)
			events = parseEvents(l.Data)
		}
	}
	s.printf("\n")

	// 2. Fields.
	s.printf("## 2. Fields\n\n")
	if len(events) == 0 {
		s.printf("No events listed; create an upcoming event on the test page and run again.\n\n")
	} else {
		for _, f := range eventFields {
			n := 0
			for _, e := range events {
				if v, ok := e.raw[f]; ok && string(v) != "null" && string(v) != `""` && string(v) != "[]" {
					n++
				}
			}
			s.printf("- `%s`: present in %d of %d\n", f, n, len(events))
		}
		// Per-field probe for fields Graph refuses by name.
		var unknown []string
		for _, f := range eventFields {
			a = s.get(ctx, "/"+url.PathEscape(events[0].ID), url.Values{"fields": {f}}, pageToken, true)
			if !a.ok() {
				unknown = append(unknown, fmt.Sprintf("`%s` (%s)", f, a.describe()))
			}
		}
		if len(unknown) > 0 {
			s.printf("- Refused when read by id: %s\n", strings.Join(unknown, "; "))
		}
		s.printf("\nFirst event, as read by id:\n\n```json\n%s\n```\n\n", s.sample(ctx, events[0].ID, pageToken))
	}

	// 3. Repeating events.
	s.printf("## 3. Repeating events (event_times)\n\n")
	repeating := 0
	for _, e := range events {
		if len(e.EventTimes) == 0 {
			continue
		}
		repeating++
		s.printf("- **%s** (%s): %d dates\n", e.Name, e.ID, len(e.EventTimes))
		for _, t := range e.EventTimes {
			s.printf("  - %s %s → %s\n", t.ID, t.StartTime, t.EndTime)
		}
		child := e.EventTimes[0].ID
		a = s.get(ctx, "/"+url.PathEscape(child), url.Values{"fields": {"id,name,start_time,end_time,is_canceled,updated_time"}}, pageToken, true)
		s.printf("  - Read child %s by id: %s %s\n", child, okOr(a), compact(a))
	}
	if repeating == 0 {
		s.printf("No listed event has event_times. Create a repeating event (several dates) on the test page and run again.\n")
	}
	s.printf("\n")

	// 4. Co-hosted events.
	s.printf("## 4. Co-hosted events\n\n")
	listed := map[string]bool{}
	for _, e := range events {
		listed[e.ID] = true
		for _, t := range e.EventTimes {
			listed[t.ID] = true
		}
	}
	for _, e := range events {
		if e.Owner != nil && e.Owner.ID != cfg.PageID {
			s.printf("- Listed event %s is owned by %s (%s): co-hosted events appear in the list.\n", e.ID, e.Owner.Name, e.Owner.ID)
		}
	}
	if len(cfg.CoHosted) == 0 {
		s.printf("No -cohosted ids given.\n")
	}
	for _, id := range cfg.CoHosted {
		a = s.get(ctx, "/"+url.PathEscape(id), url.Values{"fields": {"id,name,owner,start_time,updated_time"}}, pageToken, true)
		s.printf("- %s: in the list **%v**; read by id with page token: %s %s\n", id, listed[id], okOr(a), compact(a))
	}
	s.printf("\n")

	// 5. Gone, private, draft.
	s.printf("## 5. Deleted, private or draft events read by id\n\n")
	if len(cfg.Probe) == 0 {
		s.printf("No -probe ids given. Delete one test event, make one private and keep one as draft, then pass their ids.\n")
	}
	for _, id := range cfg.Probe {
		for _, t := range []struct{ label, tok string }{{"page token", pageToken}, {"user token", ll.AccessToken}} {
			a = s.get(ctx, "/"+url.PathEscape(id), url.Values{"fields": {"id,name,is_canceled,is_draft"}}, t.tok, true)
			s.printf("- %s with %s: %s %s\n", id, t.label, okOr(a), compact(a))
		}
	}
	s.printf("\n")

	// 7. Rate limits.
	s.printf("## 7. Rate-limit headers\n\n")
	if len(s.usage) == 0 {
		s.printf("Graph sent no usage headers.\n")
	}
	keys := make([]string, 0, len(s.usage))
	for k := range s.usage {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := s.usage[k]
		s.printf("- `%s`, %d answers; last: %s\n", k, len(v), v[len(v)-1])
	}
	s.printf("\n")

	s.printf("## Verdict\n\n")
	if readable {
		s.printf("**GO**: the page's events are readable with a page token under this app's access.\n")
	} else {
		s.printf("**NO-GO**: the events edge is not readable. Per decision 15, stop and re-plan.\n")
	}
	s.printf("\nThe page token is not stored anywhere. To end it now, remove the app under Facebook, Settings, Business integrations.\n")
	return nil
}

// sample reads one event by id with every field, description cut to 200 characters.
func (s *spike) sample(ctx context.Context, id, token string) string {
	a := s.get(ctx, "/"+url.PathEscape(id), url.Values{"fields": {strings.Join(eventFields, ",")}}, token, true)
	if !a.ok() {
		a = s.get(ctx, "/"+url.PathEscape(id), url.Values{"fields": {"id,name"}}, token, true)
	}
	var m map[string]any
	if json.Unmarshal(a.Body, &m) != nil {
		return a.describe()
	}
	if d, ok := m["description"].(string); ok && len([]rune(d)) > 200 {
		m["description"] = string([]rune(d)[:200]) + "…"
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	return string(b)
}

func okOr(a answer) string {
	if a.ok() {
		return "OK"
	}
	return "**failed** (" + a.describe() + ")"
}

func compact(a answer) string {
	if !a.ok() || len(a.Body) == 0 {
		return ""
	}
	b := string(a.Body)
	if len(b) > 300 {
		b = b[:300] + "…"
	}
	return "`" + strings.ReplaceAll(b, "`", "'") + "`"
}

func unix(t int64) string {
	if t == 0 {
		return "0 (never)"
	}
	return time.Unix(t, 0).UTC().Format("2006-01-02")
}
