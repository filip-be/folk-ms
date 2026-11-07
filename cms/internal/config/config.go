// Package config loads config/sites.json: the fixed list of Organizations
// and their Sites.
package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/domain"
)

type file struct {
	Organizations []*domain.Organization `json:"organizations"`
}

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// LoadSites reads and validates the sites file.
func LoadSites(path string) ([]*domain.Organization, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseSites(data)
}

// ParseSites validates the sites file content.
func ParseSites(data []byte) ([]*domain.Organization, error) {
	var f file
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("sites: %w", err)
	}
	if len(f.Organizations) == 0 {
		return nil, fmt.Errorf("sites: no organizations")
	}
	seen := map[string]bool{}
	for _, o := range f.Organizations {
		if !idRe.MatchString(o.ID) || !idRe.MatchString(o.Site.ID) {
			return nil, fmt.Errorf("sites: bad id %q/%q", o.ID, o.Site.ID)
		}
		if seen[o.ID] || seen["site:"+o.Site.ID] {
			return nil, fmt.Errorf("sites: duplicate id %q", o.ID)
		}
		seen[o.ID], seen["site:"+o.Site.ID] = true, true
		if o.Name.PL == "" {
			return nil, fmt.Errorf("sites: %s: Polish name required", o.ID)
		}
		loc, err := time.LoadLocation(o.Timezone)
		if err != nil {
			return nil, fmt.Errorf("sites: %s: timezone: %w", o.ID, err)
		}
		o.SetLocation(loc)
		if u, err := url.Parse(o.Site.BaseURL); err != nil || u.Scheme != "https" && u.Scheme != "http" || u.Host == "" || u.Path != "" {
			return nil, fmt.Errorf("sites: %s: baseUrl must be an origin", o.ID)
		}
		t := o.Site.PermalinkTemplate
		if !strings.HasPrefix(t, "/{lang}/") || !strings.Contains(t, "{shortId}") {
			return nil, fmt.Errorf("sites: %s: permalinkTemplate must start with /{lang}/ and contain {shortId}", o.ID)
		}
		for _, origin := range o.Site.Origins {
			if u, err := url.Parse(origin); err != nil || u.Host == "" || u.Path != "" {
				return nil, fmt.Errorf("sites: %s: bad origin %q", o.ID, origin)
			}
		}
	}
	return f.Organizations, nil
}
