package domain

import (
	"fmt"
	"strings"
	"time"
)

var (
	plMonthsGen = [...]string{"stycznia", "lutego", "marca", "kwietnia", "maja", "czerwca", "lipca", "sierpnia", "września", "października", "listopada", "grudnia"}
	plWeekdays  = [...]string{"niedziela", "poniedziałek", "wtorek", "środa", "czwartek", "piątek", "sobota"}
)

// FormatDate formats a local instant for people, e.g. "sobota, 21 listopada
// 2026, 19:00" or "Saturday, 21 November 2026, 19:00".
func FormatDate(t time.Time, l Lang) string {
	if l == EN {
		return t.Format("Monday, 2 January 2006, 15:04")
	}
	return fmt.Sprintf("%s, %d %s %d, %s", plWeekdays[t.Weekday()], t.Day(), plMonthsGen[t.Month()-1], t.Year(), t.Format("15:04"))
}

// GenerateCaption builds the default Caption of a Social Post for the
// Event's Published version: title, next Occurrence, Venue, Price, summary
// and Permalink. The Moderator edits it before posting.
func GenerateCaption(org *Organization, e *Event, venues map[string]Venue, l Lang, now time.Time) string {
	c := e.Published
	if c == nil {
		c = &e.Draft
	}
	loc := org.Location()
	occ := c.Schedule.Occurrences(loc, c.VenueID)
	var next *Occurrence
	upcoming := 0
	for i := range occ {
		if occ[i].Cancelled || occ[i].End.Before(now) {
			continue
		}
		if next == nil {
			next = &occ[i]
		}
		upcoming++
	}
	if next == nil && len(occ) > 0 {
		next = &occ[len(occ)-1]
	}
	label := func(pl, en string) string {
		if l == EN {
			return en
		}
		return pl
	}
	var b strings.Builder
	b.WriteString(c.Title.In(l))
	b.WriteString("\n\n")
	if next != nil {
		start := next.Start.In(loc)
		b.WriteString(label("Kiedy: ", "When: ") + FormatDate(start, l))
		if c.Schedule.Kind == KindMultiDay {
			b.WriteString(" – " + FormatDate(next.End.In(loc), l))
		}
		if upcoming > 1 {
			b.WriteString(label(fmt.Sprintf(" (i %d kolejnych terminów)", upcoming-1), fmt.Sprintf(" (and %d more dates)", upcoming-1)))
		}
		b.WriteString("\n")
		if v, ok := venues[next.VenueID]; ok {
			b.WriteString(label("Gdzie: ", "Where: ") + v.Name + ", " + v.Address + "\n")
		}
	}
	if p := c.Price.Text.In(l); p != "" {
		b.WriteString(label("Wstęp: ", "Admission: ") + p + "\n")
	}
	if s := c.Summary.In(l); s != "" {
		b.WriteString("\n" + s + "\n")
	}
	b.WriteString("\n" + label("Szczegóły: ", "Details: ") + org.Permalink(e.ShortID, c.Slug, l))
	out := b.String()
	if r := []rune(out); len(r) > MaxCaptionLen {
		out = string(r[:MaxCaptionLen])
	}
	return out
}
