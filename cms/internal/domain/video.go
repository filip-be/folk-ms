package domain

import (
	"fmt"
	"io"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The Video Collection: per Organization, an ordered list of instructional
// Videos of dances, each with any number of Forms and at most one Origin
// (a country or a region of a country). Forms and Origins are lists the
// Organization manages. Everything is one document, so order changes and
// in-use checks are atomic.

// Limits of a Video Collection.
const (
	MaxVideos            = 1000
	MaxForms             = 50
	MaxCountries         = 100
	MaxRegionsPerCountry = 50
	MaxVideoTitleLen     = 200
	MaxVideoDescLen      = 2000
	MaxCreditLen         = 200
	MaxLabelLen          = 60
	// MaxStartSeconds is the latest start time accepted (24 hours).
	MaxStartSeconds = 24 * 60 * 60
)

// Video providers.
const (
	YouTube = "youtube"
	Vimeo   = "vimeo"
)

// VideoLink is a video link reduced to its parts; the CMS never keeps the
// pasted string.
type VideoLink struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	// Hash is the hash of an unlisted Vimeo video.
	Hash         string `json:"hash,omitempty"`
	StartSeconds int    `json:"startSeconds,omitempty"`
}

// SameVideo reports whether two links point to the same video (start aside).
func (l VideoLink) SameVideo(o VideoLink) bool {
	return l.Provider == o.Provider && l.ID == o.ID && l.Hash == o.Hash
}

// WatchURL is the canonical address of the video, with its start.
func (l VideoLink) WatchURL() string {
	switch l.Provider {
	case YouTube:
		u := "https://www.youtube.com/watch?v=" + l.ID
		if l.StartSeconds > 0 {
			u += "&t=" + strconv.Itoa(l.StartSeconds) + "s"
		}
		return u
	case Vimeo:
		u := "https://vimeo.com/" + l.ID
		if l.Hash != "" {
			u += "/" + l.Hash
		}
		if l.StartSeconds > 0 {
			u += "#t=" + strconv.Itoa(l.StartSeconds) + "s"
		}
		return u
	}
	return ""
}

// LinkURL is the canonical address without the start, as the edit form shows it.
func (l VideoLink) LinkURL() string {
	l.StartSeconds = 0
	return l.WatchURL()
}

// EmbedURL is the player address: youtube-nocookie.com or player.vimeo.com.
func (l VideoLink) EmbedURL() string {
	switch l.Provider {
	case YouTube:
		u := "https://www.youtube-nocookie.com/embed/" + l.ID
		if l.StartSeconds > 0 {
			u += "?start=" + strconv.Itoa(l.StartSeconds)
		}
		return u
	case Vimeo:
		u := "https://player.vimeo.com/video/" + l.ID
		if l.Hash != "" {
			u += "?h=" + l.Hash
		}
		if l.StartSeconds > 0 {
			u += "#t=" + strconv.Itoa(l.StartSeconds) + "s"
		}
		return u
	}
	return ""
}

// YouTubeThumbnail is the thumbnail YouTube serves for a video id.
func YouTubeThumbnail(id string) string { return "https://i.ytimg.com/vi/" + id + "/hqdefault.jpg" }

var (
	youTubeID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	vimeoID   = regexp.MustCompile(`^[0-9]{1,20}$`)
	vimeoHash = regexp.MustCompile(`^[0-9a-f]{6,32}$`)
	// 90, 90s, 1m30s, 1h2m3s
	startUnits = regexp.MustCompile(`^(?:(\d{1,2})h)?(?:(\d{1,4})m)?(?:(\d{1,5})s?)?$`)
	// 1:30, 1:02:03
	startClock = regexp.MustCompile(`^(?:(\d{1,2}):)?(\d{1,4}):([0-5]\d)$`)
)

// ParseStart reads a start time: seconds ("90", "90s"), units ("1m30s",
// "1h2m3s") or a clock ("1:30", "1:02:03"). Empty is 0.
func ParseStart(s string) (int, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0, true
	}
	num := func(v string) int {
		n, _ := strconv.Atoi(v)
		return n
	}
	var secs int
	if m := startClock.FindStringSubmatch(s); m != nil {
		secs = num(m[1])*3600 + num(m[2])*60 + num(m[3])
	} else if m := startUnits.FindStringSubmatch(s); m != nil && s != "" {
		secs = num(m[1])*3600 + num(m[2])*60 + num(m[3])
	} else {
		return 0, false
	}
	return secs, secs <= MaxStartSeconds
}

// FormatStart writes seconds as m:ss or h:mm:ss.
func FormatStart(secs int) string {
	if secs <= 0 {
		return ""
	}
	h, m, s := secs/3600, secs/60%60, secs%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func badLink() *ValidationError {
	return Problem("link", "podaj link do filmu na YouTube lub Vimeo, np. https://www.youtube.com/watch?v=… lub https://vimeo.com/…",
		"enter a link to a YouTube or Vimeo video, e.g. https://www.youtube.com/watch?v=… or https://vimeo.com/…")
}

func badStart() *ValidationError {
	return Problem("start", "podaj czas jako m:ss, np. 1:30", "enter the time as m:ss, e.g. 1:30")
}

// ParseVideoLink accepts a YouTube or Vimeo address (http or https; a
// missing scheme means https) and returns its parts, with the start time
// from t=, start= or #t=. Any other host, scheme or a malformed id is refused.
func ParseVideoLink(raw string) (VideoLink, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return VideoLink{}, Problem("link", "link do filmu jest wymagany", "the video link is required")
	}
	if len(s) > 2048 || strings.ContainsAny(s, " \t\r\n\"'<>\\") {
		return VideoLink{}, badLink()
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Port() != "" || u.Opaque != "" {
		return VideoLink{}, badLink()
	}
	q := u.Query()
	frag, _ := url.ParseQuery(u.Fragment)
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	var l VideoLink
	switch strings.ToLower(u.Hostname()) {
	case "youtube.com", "www.youtube.com", "m.youtube.com", "music.youtube.com", "youtube-nocookie.com", "www.youtube-nocookie.com":
		l.Provider = YouTube
		switch {
		case len(parts) == 1 && parts[0] == "watch":
			l.ID = q.Get("v")
		case len(parts) == 2 && (parts[0] == "shorts" || parts[0] == "embed" || parts[0] == "live" || parts[0] == "v"):
			l.ID = parts[1]
		}
	case "youtu.be", "www.youtu.be":
		l.Provider = YouTube
		if len(parts) == 1 {
			l.ID = parts[0]
		}
	case "vimeo.com", "www.vimeo.com", "player.vimeo.com":
		l.Provider = Vimeo
		if strings.EqualFold(u.Hostname(), "player.vimeo.com") {
			if len(parts) == 2 && parts[0] == "video" {
				l.ID = parts[1]
			}
		} else if len(parts) == 1 || len(parts) == 2 {
			l.ID = parts[0]
			if len(parts) == 2 {
				l.Hash = parts[1]
			}
		}
		if h := q.Get("h"); h != "" {
			if l.Hash != "" && l.Hash != h {
				return VideoLink{}, badLink()
			}
			l.Hash = h
		}
	default:
		return VideoLink{}, badLink()
	}
	switch l.Provider {
	case YouTube:
		if !youTubeID.MatchString(l.ID) {
			return VideoLink{}, badLink()
		}
	case Vimeo:
		if !vimeoID.MatchString(l.ID) || (l.Hash != "" && !vimeoHash.MatchString(l.Hash)) {
			return VideoLink{}, badLink()
		}
	}
	for _, v := range []string{q.Get("t"), q.Get("start"), frag.Get("t")} {
		if v == "" {
			continue
		}
		secs, ok := ParseStart(v)
		if !ok {
			return VideoLink{}, badStart()
		}
		l.StartSeconds = secs
		break
	}
	return l, nil
}

// Form is a kind of dance (solo, pair, chain, circle, …), from the
// Organization's list.
type Form struct {
	ID    string `json:"id"`
	Label Text   `json:"label"`
}

// Region is a region of a country.
type Region struct {
	ID    string `json:"id"`
	Label Text   `json:"label"`
}

// Country is an Origin with its optional regions.
type Country struct {
	ID      string   `json:"id"`
	Label   Text     `json:"label"`
	Regions []Region `json:"regions,omitempty"`
}

// Video is one instructional video: one link.
type Video struct {
	ID          string `json:"id"`
	Title       Text   `json:"title"`
	Description Text   `json:"description,omitzero"`
	Credit      string `json:"credit,omitempty"`
	// OriginID is a country or a region id; empty: none.
	OriginID string    `json:"originId,omitempty"`
	FormIDs  []string  `json:"formIds"`
	Link     VideoLink `json:"link"`
	// Thumbnail is the Vimeo thumbnail (https://i.vimeocdn.com/…), read at
	// save; YouTube thumbnails are derived from the id.
	Thumbnail string    `json:"thumbnail,omitempty"`
	Hidden    bool      `json:"hidden,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	CreatedBy string    `json:"createdBy"`
	UpdatedAt time.Time `json:"updatedAt"`
	UpdatedBy string    `json:"updatedBy"`
}

// ThumbnailURL is the image a card shows.
func (v *Video) ThumbnailURL() string {
	if v.Link.Provider == YouTube {
		return YouTubeThumbnail(v.Link.ID)
	}
	return v.Thumbnail
}

// VideoCollection is the aggregate: one per Organization. The order of
// each list is the order Sites show.
type VideoCollection struct {
	OrgID   string    `json:"orgId"`
	Forms   []Form    `json:"forms"`
	Origins []Country `json:"origins"`
	Videos  []Video   `json:"videos"`
	// Retired holds the ids of removed items, so an id is never reused.
	Retired   []string  `json:"retired,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitzero"`
	UpdatedBy string    `json:"updatedBy,omitempty"`
}

// NewVideoCollection is the empty collection of an Organization.
func NewVideoCollection(org string) *VideoCollection {
	return &VideoCollection{OrgID: org, Forms: []Form{}, Origins: []Country{}, Videos: []Video{}}
}

func (c *VideoCollection) touch(by string, now time.Time) { c.UpdatedAt, c.UpdatedBy = now, by }

func (c *VideoCollection) taken(id string) bool {
	if slices.Contains(c.Retired, id) {
		return true
	}
	for _, f := range c.Forms {
		if f.ID == id {
			return true
		}
	}
	for _, co := range c.Origins {
		if co.ID == id {
			return true
		}
		for _, r := range co.Regions {
			if r.ID == id {
				return true
			}
		}
	}
	return c.videoIndex(id) >= 0
}

// NewItemID draws a shortId-shaped id no item of the collection has had.
func (c *VideoCollection) NewItemID(r io.Reader) (string, error) {
	return GenerateShortID(r, c.taken)
}

func (c *VideoCollection) retire(id string) { c.Retired = append(c.Retired, id) }

func (c *VideoCollection) videoIndex(id string) int {
	return slices.IndexFunc(c.Videos, func(v Video) bool { return v.ID == id })
}

func (c *VideoCollection) formIndex(id string) int {
	return slices.IndexFunc(c.Forms, func(f Form) bool { return f.ID == id })
}

// origin finds a country (region index -1) or a region.
func (c *VideoCollection) origin(id string) (country, region int) {
	for i, co := range c.Origins {
		if co.ID == id {
			return i, -1
		}
		for j, r := range co.Regions {
			if r.ID == id {
				return i, j
			}
		}
	}
	return -1, -1
}

// Video returns a Video by id.
func (c *VideoCollection) Video(id string) (*Video, bool) {
	i := c.videoIndex(id)
	if i < 0 {
		return nil, false
	}
	return &c.Videos[i], true
}

// FormUse counts the Videos with a Form.
func (c *VideoCollection) FormUse(id string) int {
	n := 0
	for _, v := range c.Videos {
		if slices.Contains(v.FormIDs, id) {
			n++
		}
	}
	return n
}

// OriginUse counts the Videos whose Origin is exactly id.
func (c *VideoCollection) OriginUse(id string) int {
	n := 0
	for _, v := range c.Videos {
		if v.OriginID == id {
			n++
		}
	}
	return n
}

func notFound(what string) error { return fmt.Errorf("%w: %s", ErrNotFound, what) }

// --- Videos -------------------------------------------------------------

// VideoInput is a Video as the Moderator enters it. Start (m:ss), when
// given, replaces the start time in the link.
type VideoInput struct {
	Link        string   `json:"link"`
	Start       string   `json:"start"`
	Title       Text     `json:"title"`
	Description Text     `json:"description"`
	Credit      string   `json:"credit"`
	OriginID    string   `json:"originId"`
	FormIDs     []string `json:"formIds"`
}

// ParseVideoInputLink reads the link and the start of an input.
func ParseVideoInputLink(in VideoInput) (VideoLink, error) {
	l, err := ParseVideoLink(in.Link)
	if err != nil {
		return VideoLink{}, err
	}
	if strings.TrimSpace(in.Start) != "" {
		secs, ok := ParseStart(in.Start)
		if !ok {
			return VideoLink{}, badStart()
		}
		l.StartSeconds = secs
	}
	return l, nil
}

// PrepareVideo validates an input against the collection's Forms and
// Origins and returns the Video's content (no id, times or thumbnail).
func (c *VideoCollection) PrepareVideo(in VideoInput) (Video, error) {
	var p problems
	v := Video{
		Title:       cleanBi(in.Title, MaxVideoTitleLen+1),
		Description: cleanBi(in.Description, MaxVideoDescLen+1),
		Credit:      cleanText(strings.ReplaceAll(in.Credit, "\n", " "), MaxCreditLen+1),
		OriginID:    strings.TrimSpace(in.OriginID),
		FormIDs:     []string{},
	}
	l, err := ParseVideoInputLink(in)
	if err != nil {
		var ve *ValidationError
		if !asValidation(err, &ve) {
			return Video{}, err
		}
		p = append(p, ve.Problems...)
	}
	v.Link = l
	if v.Title.PL == "" {
		p.add("title.pl", "tytuł po polsku jest wymagany", "a Polish title is required")
	}
	checkLen(&p, "title", v.Title, MaxVideoTitleLen)
	checkLen(&p, "description", v.Description, MaxVideoDescLen)
	if v.Description.PL == "" && v.Description.EN != "" {
		p.add("description.pl", "tekst angielski wymaga polskiego", "English text requires Polish text")
	}
	if len([]rune(v.Credit)) > MaxCreditLen {
		p.add("credit", "tekst jest za długi (maks. %d znaków)", "the text is too long (max. %d characters)", MaxCreditLen)
	}
	if v.OriginID != "" {
		if ci, _ := c.origin(v.OriginID); ci < 0 {
			p.add("originId", "wybierz pochodzenie z listy", "choose an origin from the list")
		}
	}
	for _, id := range in.FormIDs {
		if c.formIndex(id) < 0 {
			p.add("formIds", "wybierz formy z listy", "choose forms from the list")
			break
		}
		if !slices.Contains(v.FormIDs, id) {
			v.FormIDs = append(v.FormIDs, id)
		}
	}
	// Keep the Forms in the list's order.
	slices.SortStableFunc(v.FormIDs, func(a, b string) int { return c.formIndex(a) - c.formIndex(b) })
	return v, p.err()
}

func asValidation(err error, ve **ValidationError) bool {
	v, ok := err.(*ValidationError)
	if ok {
		*ve = v
	}
	return ok
}

// AddVideo appends a prepared Video at the end of the list.
func (c *VideoCollection) AddVideo(v Video, id, by string, now time.Time) error {
	if len(c.Videos) >= MaxVideos {
		return Problem("videos", fmt.Sprintf("kolekcja może mieć najwyżej %d filmów", MaxVideos), fmt.Sprintf("the collection can hold at most %d videos", MaxVideos))
	}
	if c.taken(id) {
		return fmt.Errorf("%w: id %s is taken", ErrConflict, id)
	}
	v.ID, v.Hidden = id, false
	v.CreatedAt, v.CreatedBy, v.UpdatedAt, v.UpdatedBy = now, by, now, by
	c.Videos = append(c.Videos, v)
	c.touch(by, now)
	return nil
}

// UpdateVideo replaces a Video's content; its id, place, Hidden and
// creation stay.
func (c *VideoCollection) UpdateVideo(id string, v Video, by string, now time.Time) error {
	cur, ok := c.Video(id)
	if !ok {
		return notFound("video")
	}
	v.ID, v.Hidden, v.CreatedAt, v.CreatedBy = cur.ID, cur.Hidden, cur.CreatedAt, cur.CreatedBy
	v.UpdatedAt, v.UpdatedBy = now, by
	*cur = v
	c.touch(by, now)
	return nil
}

// DeleteVideo removes a Video for good and returns it.
func (c *VideoCollection) DeleteVideo(id string, by string, now time.Time) (Video, error) {
	i := c.videoIndex(id)
	if i < 0 {
		return Video{}, notFound("video")
	}
	v := c.Videos[i]
	c.Videos = slices.Delete(c.Videos, i, i+1)
	c.retire(id)
	c.touch(by, now)
	return v, nil
}

// SetVideoHidden switches Hide: a Hidden Video is not on the Site.
func (c *VideoCollection) SetVideoHidden(id string, hidden bool, by string, now time.Time) error {
	v, ok := c.Video(id)
	if !ok {
		return notFound("video")
	}
	v.Hidden = hidden
	v.UpdatedAt, v.UpdatedBy = now, by
	c.touch(by, now)
	return nil
}

// move moves item i of s to index to (clamped).
func move[T any](s []T, i, to int) []T {
	to = max(0, min(to, len(s)-1))
	if i == to {
		return s
	}
	item := s[i]
	s = slices.Delete(s, i, i+1)
	return slices.Insert(s, to, item)
}

// MoveVideo moves a Video to an index of the list.
func (c *VideoCollection) MoveVideo(id string, to int, by string, now time.Time) error {
	i := c.videoIndex(id)
	if i < 0 {
		return notFound("video")
	}
	c.Videos = move(c.Videos, i, to)
	c.touch(by, now)
	return nil
}

// --- Forms and Origins ----------------------------------------------------

func cleanLabel(t Text) (Text, error) {
	var p problems
	l := Text{PL: cleanText(strings.ReplaceAll(t.PL, "\n", " "), MaxLabelLen+1), EN: cleanText(strings.ReplaceAll(t.EN, "\n", " "), MaxLabelLen+1)}
	if l.PL == "" {
		p.add("label.pl", "nazwa po polsku jest wymagana", "a Polish name is required")
	}
	if len([]rune(l.PL)) > MaxLabelLen {
		p.add("label.pl", "tekst jest za długi (maks. %d znaków)", "the text is too long (max. %d characters)", MaxLabelLen)
	}
	if len([]rune(l.EN)) > MaxLabelLen {
		p.add("label.en", "tekst jest za długi (maks. %d znaków)", "the text is too long (max. %d characters)", MaxLabelLen)
	}
	return l, p.err()
}

func duplicateLabel() error {
	return Problem("label.pl", "taka nazwa już jest na liście", "this name is already on the list")
}

func sameLabel(a, b Text) bool { return strings.EqualFold(a.PL, b.PL) }

// AddForm appends a Form.
func (c *VideoCollection) AddForm(id string, label Text, by string, now time.Time) error {
	l, err := cleanLabel(label)
	if err != nil {
		return err
	}
	if len(c.Forms) >= MaxForms {
		return Problem("label.pl", fmt.Sprintf("lista może mieć najwyżej %d form", MaxForms), fmt.Sprintf("the list can hold at most %d forms", MaxForms))
	}
	if slices.ContainsFunc(c.Forms, func(f Form) bool { return sameLabel(f.Label, l) }) {
		return duplicateLabel()
	}
	if c.taken(id) {
		return fmt.Errorf("%w: id %s is taken", ErrConflict, id)
	}
	c.Forms = append(c.Forms, Form{ID: id, Label: l})
	c.touch(by, now)
	return nil
}

// RenameForm changes a Form's label.
func (c *VideoCollection) RenameForm(id string, label Text, by string, now time.Time) error {
	i := c.formIndex(id)
	if i < 0 {
		return notFound("form")
	}
	l, err := cleanLabel(label)
	if err != nil {
		return err
	}
	for j, f := range c.Forms {
		if j != i && sameLabel(f.Label, l) {
			return duplicateLabel()
		}
	}
	c.Forms[i].Label = l
	c.touch(by, now)
	return nil
}

func inUse(n int) error {
	return fmt.Errorf("%w: %w", ErrInvalidState, Problem("label.pl",
		fmt.Sprintf("nie można usunąć: używa tego %d film(ów)", n), fmt.Sprintf("cannot remove: %d video(s) use it", n)))
}

// RemoveForm removes a Form no Video uses.
func (c *VideoCollection) RemoveForm(id string, by string, now time.Time) error {
	i := c.formIndex(id)
	if i < 0 {
		return notFound("form")
	}
	if n := c.FormUse(id); n > 0 {
		return inUse(n)
	}
	c.Forms = slices.Delete(c.Forms, i, i+1)
	c.retire(id)
	c.touch(by, now)
	return nil
}

// MoveForm moves a Form to an index of the list.
func (c *VideoCollection) MoveForm(id string, to int, by string, now time.Time) error {
	i := c.formIndex(id)
	if i < 0 {
		return notFound("form")
	}
	c.Forms = move(c.Forms, i, to)
	// Videos list their Forms in the list's order.
	for k := range c.Videos {
		slices.SortStableFunc(c.Videos[k].FormIDs, func(a, b string) int { return c.formIndex(a) - c.formIndex(b) })
	}
	c.touch(by, now)
	return nil
}

// AddOrigin appends a country (parentID empty) or a region of a country.
func (c *VideoCollection) AddOrigin(id, parentID string, label Text, by string, now time.Time) error {
	l, err := cleanLabel(label)
	if err != nil {
		return err
	}
	if c.taken(id) {
		return fmt.Errorf("%w: id %s is taken", ErrConflict, id)
	}
	if parentID == "" {
		if len(c.Origins) >= MaxCountries {
			return Problem("label.pl", fmt.Sprintf("lista może mieć najwyżej %d krajów", MaxCountries), fmt.Sprintf("the list can hold at most %d countries", MaxCountries))
		}
		if slices.ContainsFunc(c.Origins, func(co Country) bool { return sameLabel(co.Label, l) }) {
			return duplicateLabel()
		}
		c.Origins = append(c.Origins, Country{ID: id, Label: l})
		c.touch(by, now)
		return nil
	}
	ci, ri := c.origin(parentID)
	if ci < 0 || ri >= 0 {
		return Problem("parentId", "wybierz kraj z listy", "choose a country from the list")
	}
	co := &c.Origins[ci]
	if len(co.Regions) >= MaxRegionsPerCountry {
		return Problem("label.pl", fmt.Sprintf("kraj może mieć najwyżej %d regionów", MaxRegionsPerCountry), fmt.Sprintf("a country can have at most %d regions", MaxRegionsPerCountry))
	}
	if slices.ContainsFunc(co.Regions, func(r Region) bool { return sameLabel(r.Label, l) }) {
		return duplicateLabel()
	}
	co.Regions = append(co.Regions, Region{ID: id, Label: l})
	c.touch(by, now)
	return nil
}

// RenameOrigin changes a country's or a region's label.
func (c *VideoCollection) RenameOrigin(id string, label Text, by string, now time.Time) error {
	ci, ri := c.origin(id)
	if ci < 0 {
		return notFound("origin")
	}
	l, err := cleanLabel(label)
	if err != nil {
		return err
	}
	if ri < 0 {
		for j, co := range c.Origins {
			if j != ci && sameLabel(co.Label, l) {
				return duplicateLabel()
			}
		}
		c.Origins[ci].Label = l
	} else {
		for j, r := range c.Origins[ci].Regions {
			if j != ri && sameLabel(r.Label, l) {
				return duplicateLabel()
			}
		}
		c.Origins[ci].Regions[ri].Label = l
	}
	c.touch(by, now)
	return nil
}

// RemoveOrigin removes a country without regions or a region, if no
// Video has it as its Origin.
func (c *VideoCollection) RemoveOrigin(id string, by string, now time.Time) error {
	ci, ri := c.origin(id)
	if ci < 0 {
		return notFound("origin")
	}
	if ri < 0 && len(c.Origins[ci].Regions) > 0 {
		return fmt.Errorf("%w: %w", ErrInvalidState, Problem("label.pl",
			"nie można usunąć kraju, który ma regiony – najpierw usuń regiony", "cannot remove a country that has regions – remove its regions first"))
	}
	if n := c.OriginUse(id); n > 0 {
		return inUse(n)
	}
	if ri < 0 {
		c.Origins = slices.Delete(c.Origins, ci, ci+1)
	} else {
		c.Origins[ci].Regions = slices.Delete(c.Origins[ci].Regions, ri, ri+1)
	}
	c.retire(id)
	c.touch(by, now)
	return nil
}

// MoveOrigin moves a country among the countries, or a region among the
// regions of its country.
func (c *VideoCollection) MoveOrigin(id string, to int, by string, now time.Time) error {
	ci, ri := c.origin(id)
	if ci < 0 {
		return notFound("origin")
	}
	if ri < 0 {
		c.Origins = move(c.Origins, ci, to)
	} else {
		c.Origins[ci].Regions = move(c.Origins[ci].Regions, ri, to)
	}
	c.touch(by, now)
	return nil
}

// --- Snapshot ---------------------------------------------------------------

// VideoSnapshotSchemaVersion is the version of contract/videos.schema.json.
const VideoSnapshotSchemaVersion = 1

// Video snapshot types mirror contract/videos.schema.json.
type (
	VideoSnapshot struct {
		SchemaVersion int             `json:"schemaVersion"`
		Organization  SnapshotOrg     `json:"organization"`
		Forms         []SnapLabeled   `json:"forms"`
		Origins       []SnapCountry   `json:"origins"`
		Videos        []SnapshotVideo `json:"videos"`
		UpdatedAt     string          `json:"updatedAt,omitempty"`
	}
	SnapLabeled struct {
		ID    string `json:"id"`
		Label Text   `json:"label"`
	}
	SnapCountry struct {
		ID      string        `json:"id"`
		Label   Text          `json:"label"`
		Regions []SnapLabeled `json:"regions"`
	}
	SnapshotVideo struct {
		ID          string   `json:"id"`
		Title       Text     `json:"title"`
		Description *Text    `json:"description,omitempty"`
		Credit      string   `json:"credit,omitempty"`
		OriginID    string   `json:"originId,omitempty"`
		FormIDs     []string `json:"formIds"`
		Provider    string   `json:"provider"`
		VideoID     string   `json:"videoId"`
		Hash        string   `json:"hash,omitempty"`
		Start       int      `json:"start,omitempty"`
		URL         string   `json:"url"`
		EmbedURL    string   `json:"embedUrl"`
		Thumbnail   string   `json:"thumbnail"`
	}
)

// BuildVideoSnapshot builds a Site's videos.json: every Form and Origin in
// list order (used or not; readers hide empty ones) and the Videos not
// Hidden, in order. coll may be nil (no Video Collection yet).
func BuildVideoSnapshot(org *Organization, coll *VideoCollection) VideoSnapshot {
	s := VideoSnapshot{
		SchemaVersion: VideoSnapshotSchemaVersion,
		Organization:  SnapshotOrg{ID: org.ID, Name: org.Name, SiteID: org.Site.ID, BaseURL: org.Site.BaseURL, Timezone: org.Timezone},
		Forms:         []SnapLabeled{}, Origins: []SnapCountry{}, Videos: []SnapshotVideo{},
	}
	if coll == nil {
		return s
	}
	if !coll.UpdatedAt.IsZero() {
		s.UpdatedAt = coll.UpdatedAt.UTC().Format(time.RFC3339)
	}
	for _, f := range coll.Forms {
		s.Forms = append(s.Forms, SnapLabeled{ID: f.ID, Label: f.Label})
	}
	for _, co := range coll.Origins {
		sc := SnapCountry{ID: co.ID, Label: co.Label, Regions: []SnapLabeled{}}
		for _, r := range co.Regions {
			sc.Regions = append(sc.Regions, SnapLabeled{ID: r.ID, Label: r.Label})
		}
		s.Origins = append(s.Origins, sc)
	}
	for _, v := range coll.Videos {
		if v.Hidden {
			continue
		}
		thumb := v.ThumbnailURL()
		if thumb == "" || !IsHTTPSURL(thumb) {
			continue // a Vimeo Video saved without a thumbnail cannot be shown
		}
		s.Videos = append(s.Videos, SnapshotVideo{
			ID: v.ID, Title: v.Title, Description: optText(v.Description), Credit: v.Credit,
			OriginID: v.OriginID, FormIDs: append([]string{}, v.FormIDs...),
			Provider: v.Link.Provider, VideoID: v.Link.ID, Hash: v.Link.Hash, Start: v.Link.StartSeconds,
			URL: v.Link.WatchURL(), EmbedURL: v.Link.EmbedURL(), Thumbnail: thumb,
		})
	}
	return s
}
