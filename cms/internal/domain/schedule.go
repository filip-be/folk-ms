package domain

import (
	"fmt"
	"sort"
	"time"
)

// Kind is the kind of Event.
type Kind string

const (
	KindSingle    Kind = "single"
	KindMultiDay  Kind = "multiDay"
	KindRecurring Kind = "recurring"
)

// Freq is the repetition of a Recurrence.
type Freq string

const (
	Weekly   Freq = "weekly"
	Biweekly Freq = "biweekly"
	Monthly  Freq = "monthly"
)

// MaxRecurrenceMonths is the longest allowed Recurrence span.
const MaxRecurrenceMonths = 24

// MaxMultiDayDays is the longest allowed Multi-day Event.
const MaxMultiDayDays = 31

// Schedule says when an Event happens. Dates are "YYYY-MM-DD" and times
// "HH:MM", local to the Organization's timezone.
//
//   - single: StartDate, StartTime, EndTime. An EndTime not after StartTime
//     means the Occurrence ends the next day (a bal ending after midnight).
//   - multiDay: StartDate, StartTime, EndDate (after StartDate), EndTime.
//   - recurring: StartDate (first possible date), StartTime, EndTime and a
//     Recurrence whose EndDate is mandatory.
type Schedule struct {
	Kind       Kind        `json:"kind"`
	StartDate  string      `json:"startDate"`
	StartTime  string      `json:"startTime"`
	EndDate    string      `json:"endDate,omitempty"`
	EndTime    string      `json:"endTime"`
	Recurrence *Recurrence `json:"recurrence,omitempty"`
	Exceptions []Exception `json:"exceptions,omitempty"`
}

// Recurrence is the repetition rule of a Recurring Event.
type Recurrence struct {
	Freq    Freq   `json:"freq"`
	Weekday int    `json:"weekday"` // 0 = Sunday
	Nth     int    `json:"nth,omitempty"`
	EndDate string `json:"endDate"`
}

// Exception cancels or changes one Occurrence, identified by its original
// date.
type Exception struct {
	Date      string `json:"date"`
	Cancelled bool   `json:"cancelled,omitempty"`
	StartTime string `json:"startTime,omitempty"`
	EndTime   string `json:"endTime,omitempty"`
	VenueID   string `json:"venueId,omitempty"`
	Note      Text   `json:"note,omitzero"`
}

// Occurrence is one concrete dated instance of an Event.
type Occurrence struct {
	Key       string    `json:"key"` // original local start date
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	Cancelled bool      `json:"cancelled"`
	VenueID   string    `json:"venueId"`
	Note      Text      `json:"note,omitzero"`
}

func parseDate(s string) (time.Time, bool) {
	t, err := time.Parse(time.DateOnly, s)
	return t, err == nil
}

func parseClock(s string) (h, m int, ok bool) {
	t, err := time.Parse("15:04", s)
	if err != nil || len(s) != 5 {
		return 0, 0, false
	}
	return t.Hour(), t.Minute(), true
}

func at(d time.Time, clock string, loc *time.Location) time.Time {
	h, m, _ := parseClock(clock)
	return time.Date(d.Year(), d.Month(), d.Day(), h, m, 0, 0, loc)
}

// span builds the start and end of an Occurrence on date d. When endDate is
// zero the end is on d, or on the next day if endClock is not after
// startClock. Dates are combined in loc, so DST changes are handled.
func span(d time.Time, startClock, endClock string, endDate time.Time, loc *time.Location) (time.Time, time.Time) {
	start := at(d, startClock, loc)
	if !endDate.IsZero() {
		return start, at(endDate, endClock, loc)
	}
	end := at(d, endClock, loc)
	if !end.After(start) {
		end = at(d.AddDate(0, 0, 1), endClock, loc)
	}
	return start, end
}

// Validate checks the Schedule fully (as required for Publish).
func (s Schedule) Validate() error {
	var p problems
	s.validate(&p)
	return p.err()
}

func (s Schedule) validate(p *problems) {
	before := len(*p)
	start, okStart := parseDate(s.StartDate)
	if !okStart {
		p.add("schedule.startDate", "podaj datę rozpoczęcia", "enter the start date")
	}
	if _, _, ok := parseClock(s.StartTime); !ok {
		p.add("schedule.startTime", "podaj godzinę rozpoczęcia (GG:MM)", "enter the start time (HH:MM)")
	}
	if _, _, ok := parseClock(s.EndTime); !ok {
		p.add("schedule.endTime", "podaj godzinę zakończenia (GG:MM)", "enter the end time (HH:MM)")
	}
	switch s.Kind {
	case KindSingle:
		if s.Recurrence != nil {
			p.add("schedule.recurrence", "wydarzenie jednorazowe nie ma powtarzania", "a single event has no recurrence")
		}
		if len(s.Exceptions) > 0 {
			p.add("schedule.exceptions", "wyjątki dotyczą tylko wydarzeń cyklicznych", "exceptions apply only to recurring events")
		}
	case KindMultiDay:
		end, ok := parseDate(s.EndDate)
		switch {
		case !ok:
			p.add("schedule.endDate", "podaj datę zakończenia", "enter the end date")
		case okStart && !end.After(start):
			p.add("schedule.endDate", "data zakończenia musi być późniejsza niż data rozpoczęcia", "the end date must be after the start date")
		case okStart && end.Sub(start) > MaxMultiDayDays*24*time.Hour:
			p.add("schedule.endDate", "wydarzenie wielodniowe może trwać najwyżej %d dni", "a multi-day event can last at most %d days", MaxMultiDayDays)
		}
		if s.Recurrence != nil {
			p.add("schedule.recurrence", "wydarzenie wielodniowe nie ma powtarzania", "a multi-day event has no recurrence")
		}
		if len(s.Exceptions) > 0 {
			p.add("schedule.exceptions", "wyjątki dotyczą tylko wydarzeń cyklicznych", "exceptions apply only to recurring events")
		}
	case KindRecurring:
		r := s.Recurrence
		if r == nil {
			p.add("schedule.recurrence", "podaj regułę powtarzania", "enter the recurrence rule")
			return
		}
		switch r.Freq {
		case Weekly, Biweekly:
			if r.Nth != 0 {
				p.add("schedule.recurrence.nth", "„który w miesiącu” dotyczy tylko powtarzania miesięcznego", "“which in the month” applies only to monthly recurrence")
			}
		case Monthly:
			if r.Nth != -1 && (r.Nth < 1 || r.Nth > 4) {
				p.add("schedule.recurrence.nth", "wybierz który dzień tygodnia w miesiącu (1–4 lub ostatni)", "choose which weekday of the month (1–4 or last)")
			}
		default:
			p.add("schedule.recurrence.freq", "wybierz częstotliwość", "choose the frequency")
		}
		if r.Weekday < 0 || r.Weekday > 6 {
			p.add("schedule.recurrence.weekday", "wybierz dzień tygodnia", "choose the weekday")
		}
		end, ok := parseDate(r.EndDate)
		switch {
		case !ok:
			p.add("schedule.recurrence.endDate", "podaj datę końca powtarzania", "enter the date the recurrence ends")
		case okStart && end.Before(start):
			p.add("schedule.recurrence.endDate", "koniec powtarzania musi być po dacie rozpoczęcia", "the recurrence must end after the start date")
		case okStart && end.After(start.AddDate(0, MaxRecurrenceMonths, 0)):
			p.add("schedule.recurrence.endDate", "powtarzanie może trwać najwyżej %d miesiące", "the recurrence can last at most %d months", MaxRecurrenceMonths)
		}
		if len(*p) == before {
			dates := s.recurringDates()
			if len(dates) == 0 {
				p.add("schedule.recurrence", "reguła nie daje żadnego terminu", "the rule yields no dates")
			}
			known := map[string]bool{}
			for _, d := range dates {
				known[d.Format(time.DateOnly)] = true
			}
			seen := map[string]bool{}
			for i, e := range s.Exceptions {
				f := fmt.Sprintf("schedule.exceptions[%d]", i)
				if !known[e.Date] {
					p.add(f+".date", "%s nie jest terminem tego wydarzenia", "%s is not a date of this event", e.Date)
				}
				if seen[e.Date] {
					p.add(f+".date", "termin %s ma już wyjątek", "the date %s already has an exception", e.Date)
				}
				seen[e.Date] = true
				if e.StartTime != "" {
					if _, _, ok := parseClock(e.StartTime); !ok {
						p.add(f+".startTime", "nieprawidłowa godzina", "invalid time")
					}
				}
				if e.EndTime != "" {
					if _, _, ok := parseClock(e.EndTime); !ok {
						p.add(f+".endTime", "nieprawidłowa godzina", "invalid time")
					}
				}
			}
		}
	default:
		p.add("schedule.kind", "wybierz rodzaj wydarzenia", "choose the event type")
	}
}

// recurringDates lists the dates the Recurrence yields, assuming a valid
// Schedule. Dates are civil (UTC midnight), independent of DST.
func (s Schedule) recurringDates() []time.Time {
	r := s.Recurrence
	start, _ := parseDate(s.StartDate)
	end, _ := parseDate(r.EndDate)
	var out []time.Time
	switch r.Freq {
	case Weekly, Biweekly:
		step := 7
		if r.Freq == Biweekly {
			step = 14
		}
		d := start.AddDate(0, 0, (r.Weekday-int(start.Weekday())+7)%7)
		for !d.After(end) {
			out = append(out, d)
			d = d.AddDate(0, 0, step)
		}
	case Monthly:
		for m := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC); !m.After(end); m = m.AddDate(0, 1, 0) {
			d, ok := nthWeekday(m, time.Weekday(r.Weekday), r.Nth)
			if ok && !d.Before(start) && !d.After(end) {
				out = append(out, d)
			}
		}
	}
	return out
}

// nthWeekday returns the nth (1..4, or -1 = last) weekday of the month that
// starts at first.
func nthWeekday(first time.Time, wd time.Weekday, nth int) (time.Time, bool) {
	if nth == -1 {
		last := first.AddDate(0, 1, -1)
		return last.AddDate(0, 0, -((int(last.Weekday()) - int(wd) + 7) % 7)), true
	}
	d := first.AddDate(0, 0, (int(wd)-int(first.Weekday())+7)%7+7*(nth-1))
	return d, d.Month() == first.Month()
}

// Occurrences expands the Schedule in loc. It assumes Validate passed; an
// invalid Schedule yields whatever could be derived, possibly nothing.
// defaultVenue is used where no Exception overrides it.
func (s Schedule) Occurrences(loc *time.Location, defaultVenue string) []Occurrence {
	if s.Validate() != nil {
		return nil
	}
	start, _ := parseDate(s.StartDate)
	switch s.Kind {
	case KindSingle:
		a, b := span(start, s.StartTime, s.EndTime, time.Time{}, loc)
		return []Occurrence{{Key: s.StartDate, Start: a, End: b, VenueID: defaultVenue}}
	case KindMultiDay:
		end, _ := parseDate(s.EndDate)
		a, b := span(start, s.StartTime, s.EndTime, end, loc)
		return []Occurrence{{Key: s.StartDate, Start: a, End: b, VenueID: defaultVenue}}
	}
	exc := map[string]Exception{}
	for _, e := range s.Exceptions {
		exc[e.Date] = e
	}
	var out []Occurrence
	for _, d := range s.recurringDates() {
		key := d.Format(time.DateOnly)
		st, et, venue := s.StartTime, s.EndTime, defaultVenue
		o := Occurrence{Key: key}
		if e, ok := exc[key]; ok {
			if e.StartTime != "" {
				st = e.StartTime
			}
			if e.EndTime != "" {
				et = e.EndTime
			}
			if e.VenueID != "" {
				venue = e.VenueID
			}
			o.Cancelled = e.Cancelled
			o.Note = e.Note
		}
		o.Start, o.End = span(d, st, et, time.Time{}, loc)
		o.VenueID = venue
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out
}

// clearDates removes the dates from a Schedule (used by Duplicate), keeping
// the kind, times and rule shape.
func (s Schedule) clearDates() Schedule {
	s.StartDate = ""
	s.EndDate = ""
	s.Exceptions = nil
	if s.Recurrence != nil {
		r := *s.Recurrence
		r.EndDate = ""
		s.Recurrence = &r
	}
	return s
}

// venueIDs lists every Venue the Schedule's Exceptions reference.
func (s Schedule) venueIDs() []string {
	var ids []string
	for _, e := range s.Exceptions {
		if e.VenueID != "" {
			ids = append(ids, e.VenueID)
		}
	}
	return ids
}
