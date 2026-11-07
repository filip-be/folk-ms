package config

import (
	"testing"

	"github.com/filip-be/folk-ms/cms/internal/domain"
)

func TestRepositorySitesFile(t *testing.T) {
	orgs, err := LoadSites("../../../config/sites.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(orgs) < 2 {
		t.Fatalf("want the configured organizations, got %d", len(orgs))
	}
	for _, o := range orgs {
		if p := o.Permalink("abcdefgh", "bal", domain.EN); p != o.Site.BaseURL+"/en/events/abcdefgh-bal" {
			t.Errorf("%s: permalink %s does not match the Sites' route /<lang>/events/<shortId>-<slug>", o.ID, p)
		}
	}
}

func TestRejects(t *testing.T) {
	bad := []string{
		`{"organizations":[]}`,
		`{"organizations":[{"id":"Bad","name":{"pl":"x"},"timezone":"Europe/Warsaw","site":{"id":"x","baseUrl":"https://x","permalinkTemplate":"/{lang}/e/{shortId}"}}]}`,
		`{"organizations":[{"id":"a","name":{"pl":"x"},"timezone":"Mars/Base","site":{"id":"x","baseUrl":"https://x","permalinkTemplate":"/{lang}/e/{shortId}"}}]}`,
		`{"organizations":[{"id":"a","name":{"pl":"x"},"timezone":"Europe/Warsaw","site":{"id":"x","baseUrl":"https://x","permalinkTemplate":"/e/{slug}"}}]}`,
		`{"organizations":[{"id":"a","extra":1}]}`,
	}
	for _, b := range bad {
		if _, err := ParseSites([]byte(b)); err == nil {
			t.Errorf("want error for %s", b)
		}
	}
}
