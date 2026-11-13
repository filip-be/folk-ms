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

// GenerateCaption builds the default Caption of a Social Post from the
// Event's effective values: name, next Occurrence, Location, Price, Summary
// and Permalink. The Moderator edits it before posting.
func GenerateCaption(org *Organization, e *Event, l Lang, now time.Time) string {
	v := e.Effective()
	loc := org.Location()
	var next *Occurrence
	upcoming := 0
	for i := range v.Occurrences {
		if v.Occurrences[i].Cancelled || v.Occurrences[i].End.Before(now) {
			continue
		}
		if next == nil {
			next = &v.Occurrences[i]
		}
		upcoming++
	}
	if next == nil && len(v.Occurrences) > 0 {
		next = &v.Occurrences[len(v.Occurrences)-1]
	}
	label := func(pl, en string) string {
		if l == EN {
			return en
		}
		return pl
	}
	var b strings.Builder
	b.WriteString(v.Name.In(l))
	b.WriteString("\n\n")
	if next != nil {
		start, end := next.Start.In(loc), next.End.In(loc)
		b.WriteString(label("Kiedy: ", "When: ") + FormatDate(start, l))
		if end.YearDay() != start.YearDay() && end.Sub(start) > 24*time.Hour {
			b.WriteString(" – " + FormatDate(end, l))
		}
		if upcoming > 1 {
			b.WriteString(label(fmt.Sprintf(" (i %d kolejnych terminów)", upcoming-1), fmt.Sprintf(" (and %d more dates)", upcoming-1)))
		}
		b.WriteString("\n")
		if lc := v.Location; lc != nil {
			where := lc.Name
			if lc.Address != "" {
				where += ", " + lc.Address
			}
			b.WriteString(label("Gdzie: ", "Where: ") + where + "\n")
		} else if v.Online {
			b.WriteString(label("Gdzie: online", "Where: online") + "\n")
		}
	}
	if p := v.Price.In(l); p != "" {
		b.WriteString(label("Wstęp: ", "Admission: ") + p + "\n")
	}
	if s := v.Summary.In(l); s != "" {
		b.WriteString("\n" + s + "\n")
	}
	b.WriteString("\n" + label("Szczegóły: ", "Details: ") + org.Permalink(e.ShortID, e.Slug(), l))
	out := b.String()
	if r := []rune(out); len(r) > MaxCaptionLen {
		out = string(r[:MaxCaptionLen])
	}
	return out
}
