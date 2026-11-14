package domain

import (
	"fmt"
	"slices"
	"time"
)

// SanitizeFacebook treats what Graph returned as untrusted input (T7):
// plain text within length limits, https URLs only, a bounded number of
// Occurrences with End never before Start.
func SanitizeFacebook(f FacebookPart) FacebookPart {
	f.Name = cleanText(f.Name, MaxTitleLen)
	if f.Name == "" {
		f.Name = "(bez nazwy)"
	}
	f.Description = cleanText(f.Description, MaxDescriptionLen)
	if f.TicketURL = cleanText(f.TicketURL, 2048); !IsHTTPSURL(f.TicketURL) {
		f.TicketURL = ""
	}
	if f.Location != nil {
		l := &Location{
			Name:    cleanText(f.Location.Name, MaxLocationName),
			Address: cleanText(f.Location.Address, MaxLocationAddr),
			MapURL:  cleanText(f.Location.MapURL, 2048),
		}
		if !IsHTTPSURL(l.MapURL) {
			l.MapURL = ""
		}
		if l.Name == "" {
			l.Name, l.Address = l.Address, ""
		}
		f.Location = l
		if l.Name == "" {
			f.Location = nil
		}
	}
	f.CoverID = cleanText(f.CoverID, 64)
	occ := make([]Occurrence, 0, len(f.Occurrences))
	for _, o := range f.Occurrences {
		if o.Start.IsZero() || !isDigits(o.FacebookID) {
			continue
		}
		if o.End.Before(o.Start) {
			o.End = o.Start
		}
		occ = append(occ, o)
		if len(occ) == MaxOccurrences {
			break
		}
	}
	slices.SortStableFunc(occ, func(a, b Occurrence) int { return a.Start.Compare(b.Start) })
	f.Occurrences = occ
	return f
}

// NewSyncedEvent creates an Event from a Facebook Event Sync saw first.
func NewSyncedEvent(id, orgID, shortID string, f FacebookPart, now time.Time) *Event {
	return &Event{ID: id, OrgID: orgID, ShortID: shortID, Source: SourceFacebook, Facebook: &f,
		CreatedAt: now, CreatedBy: "sync", UpdatedAt: now, UpdatedBy: "sync"}
}

// ApplyFacebook replaces the Facebook part with what Sync read and reports
// whether anything the Site or a Moderator sees changed. It keeps the
// imported cover while the Facebook cover id is the same, and never
// touches Overrides, Extras or Hidden.
func (e *Event) ApplyFacebook(f FacebookPart, now time.Time) (bool, error) {
	if e.Source != SourceFacebook || e.Facebook == nil {
		return false, fmt.Errorf("%w: not a synced event", ErrInvalidState)
	}
	old := e.Facebook
	if f.CoverID == old.CoverID && f.Cover == nil {
		f.Cover = old.Cover
	}
	f.SyncedAt = now
	changed := old.Name != f.Name || old.Description != f.Description || !sameLocation(old.Location, f.Location) ||
		old.Online != f.Online || old.Cancelled != f.Cancelled || old.TicketURL != f.TicketURL ||
		old.CoverID != f.CoverID || !slices.Equal(old.Occurrences, f.Occurrences) || old.PageID != f.PageID ||
		(old.Cover == nil) != (f.Cover == nil)
	e.Facebook = &f
	if changed {
		e.touch("sync", now)
	}
	return changed, nil
}

// SyncLease is how long one Sync run holds an Organization.
const SyncLease = 5 * time.Minute

// SyncNowInterval is the least time between two Syncs a Moderator starts.
const SyncNowInterval = time.Minute

// AutosyncInterval is the least time between two Syncs the timer starts.
const AutosyncInterval = 50 * time.Minute

// SyncResult counts what one Sync run did.
type SyncResult struct {
	Listed    int      `json:"listed"`
	Added     int      `json:"added"`
	Updated   int      `json:"updated"`
	Binned    int      `json:"binned"`
	Conflicts int      `json:"conflicts"`
	Errors    []string `json:"errors,omitempty"`
}

// SyncStatus is an Organization's sync status document; it doubles as the
// lease that keeps two runs from overlapping (T10).
type SyncStatus struct {
	OrgID          string      `json:"orgId"`
	LeaseUntil     time.Time   `json:"leaseUntil"`
	LeaseHolder    string      `json:"leaseHolder,omitempty"`
	FirstSyncAt    *time.Time  `json:"firstSyncAt,omitempty"`
	LastRunAt      *time.Time  `json:"lastRunAt,omitempty"`
	LastRunBy      string      `json:"lastRunBy,omitempty"` // a user id, or "autosync"
	LastRunByEmail string      `json:"lastRunByEmail,omitempty"`
	LastSuccessAt  *time.Time  `json:"lastSuccessAt,omitempty"`
	LastError      string      `json:"lastError,omitempty"`
	LastResult     *SyncResult `json:"lastResult,omitempty"`
}

// Claim takes the lease for a run, or reports why it cannot: ErrConflict
// while another run holds it, ErrTooSoon when the last run started less than
// minInterval ago.
func (s *SyncStatus) Claim(holder string, minInterval time.Duration, now time.Time) error {
	if now.Before(s.LeaseUntil) {
		return fmt.Errorf("%w: a sync is running", ErrConflict)
	}
	if s.LastRunAt != nil && now.Sub(*s.LastRunAt) < minInterval {
		return fmt.Errorf("%w: last sync at %s", ErrTooSoon, s.LastRunAt.Format(time.RFC3339))
	}
	s.LeaseUntil, s.LeaseHolder, s.LastRunAt = now.Add(SyncLease), holder, &now
	return nil
}

// Finish releases the lease and records the outcome.
func (s *SyncStatus) Finish(holder, by string, r SyncResult, runErr error, now time.Time) {
	if s.LeaseHolder == holder {
		s.LeaseUntil, s.LeaseHolder = time.Time{}, ""
	}
	s.LastRunBy, s.LastResult = by, &r
	if runErr != nil {
		s.LastError = cleanText(runErr.Error(), 500)
		return
	}
	s.LastError = ""
	s.LastSuccessAt = &now
	if s.FirstSyncAt == nil {
		s.FirstSyncAt = &now
	}
}
