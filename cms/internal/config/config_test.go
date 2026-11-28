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
		if !o.HasTunes() {
			continue
		}
		if p := o.TunePermalink("abcdefgh", "the-kesh", domain.PL); p != o.Site.BaseURL+"/pl/tunes/abcdefgh-the-kesh" {
			t.Errorf("%s: tune permalink %s does not match the Sites' route /<lang>/tunes/<shortId>-<slug>", o.ID, p)
		}
		if p := o.SetPermalink("abcdefgh", "reels", domain.EN); p != o.Site.BaseURL+"/en/sets/abcdefgh-reels" {
			t.Errorf("%s: set permalink %s does not match the Sites' route /<lang>/sets/<shortId>-<slug>", o.ID, p)
		}
	}
	// Only Celtycki Wrocław keeps a Tune Collection for now.
	for _, o := range orgs {
		if o.HasTunes() != (o.ID == "celtycki-wroclaw") {
			t.Errorf("%s: tunes opt-in %v", o.ID, o.HasTunes())
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
		`{"organizations":[{"id":"a","name":{"pl":"x"},"timezone":"Europe/Warsaw","site":{"id":"x","baseUrl":"https://x","permalinkTemplate":"/{lang}/e/{shortId}","tunes":{"tunePermalinkTemplate":"/t/{shortId}","setPermalinkTemplate":"/{lang}/s/{shortId}"}}}]}`,
		`{"organizations":[{"id":"a","name":{"pl":"x"},"timezone":"Europe/Warsaw","site":{"id":"x","baseUrl":"https://x","permalinkTemplate":"/{lang}/e/{shortId}","tunes":{"tunePermalinkTemplate":"/{lang}/t/{shortId}","setPermalinkTemplate":"/{lang}/t/{shortId}"}}}]}`,
	}
	for _, b := range bad {
		if _, err := ParseSites([]byte(b)); err == nil {
			t.Errorf("want error for %s", b)
		}
	}
}
