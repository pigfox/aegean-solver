package ferry

import (
	"fmt"
	"time"
)

// Date is a calendar date with no time of day and no zone.
//
// IT IS NOT A time.Time, and that is the point. A service date is the label a
// timetable hangs a day's sailings on; it is not an instant, and giving it an
// instant's type invites two mistakes that are hard to see afterwards. The
// first is comparing two dates that carry different zones and getting an
// ordering that depends on the zone rather than the calendar. The second is
// arithmetic: adding 24 hours to an instant crosses a clock shift twice a year
// and lands on the same date it started from, or skips one. A date plus one day
// is always the next date, and that is what AddDays does.
//
// An instant is produced exactly once, in StartOfServiceDay, at the boundary
// where a service day is pinned to a location.
type Date struct {
	Year  int
	Month int
	Day   int
}

// Valid reports whether the fields name a real calendar date inside the year
// bounds.
//
// It ROUND-TRIPS through the standard library rather than counting days per
// month, because time.Date normalizes out-of-range input instead of rejecting
// it: the 30th of February becomes the 1st or 2nd of March and no error is
// returned anywhere. Building the date and checking that all three fields came
// back unchanged is what turns that silent normalization into a refusal.
func (d Date) Valid() bool {
	if d.Year < MinYear || d.Year > MaxYear {
		return false
	}
	if d.Month < 1 || d.Month > 12 || d.Day < 1 {
		return false
	}
	t := time.Date(d.Year, time.Month(d.Month), d.Day, 0, 0, 0, 0, time.UTC)
	return t.Year() == d.Year && int(t.Month()) == d.Month && t.Day() == d.Day
}

// Compare orders two dates: negative if d is earlier, zero if equal, positive
// if later.
func (d Date) Compare(o Date) int {
	if d.Year != o.Year {
		return d.Year - o.Year
	}
	if d.Month != o.Month {
		return d.Month - o.Month
	}
	return d.Day - o.Day
}

// AddDays returns the date n days later, or earlier for a negative n.
func (d Date) AddDays(n int) Date {
	t := time.Date(d.Year, time.Month(d.Month), d.Day, 0, 0, 0, 0, time.UTC).AddDate(0, 0, n)
	return Date{Year: t.Year(), Month: int(t.Month()), Day: t.Day()}
}

// Weekday is the day of the week the date falls on.
//
// No location is taken, and none is needed: which weekday a given calendar date
// falls on is the same everywhere on earth. The instant that date begins is not,
// which is why StartOfServiceDay does take one.
func (d Date) Weekday() time.Weekday {
	return time.Date(d.Year, time.Month(d.Month), d.Day, 0, 0, 0, 0, time.UTC).Weekday()
}

// StartOfServiceDay is the instant that stop times on this date are measured
// from, in loc. See ServiceDayAnchorHour for why it is derived from noon rather
// than taken as midnight.
func (d Date) StartOfServiceDay(loc *time.Location) time.Time {
	noon := time.Date(d.Year, time.Month(d.Month), d.Day, ServiceDayAnchorHour, 0, 0, 0, loc)
	return noon.Add(-ServiceDayAnchorHour * time.Hour)
}

// String renders the date as YYYY-MM-DD.
func (d Date) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

// DateIn is the calendar date that instant t falls on, as seen from loc.
func DateIn(t time.Time, loc *time.Location) Date {
	y, m, day := t.In(loc).Date()
	return Date{Year: y, Month: int(m), Day: day}
}

// Weekdays is a set of days of the week, one bit per time.Weekday.
//
// A BITMASK RATHER THAN SEVEN BOOL FIELDS, which is what GTFS uses on the wire.
// Seven fields make "does this service run on the query's weekday" a seven-arm
// branch at every call site, and the directive that produced this package asked
// specifically for a calendar that does not force branching. One masked test
// answers it.
type Weekdays uint8

// With returns the set with each named day added.
//
// A day outside Sunday..Saturday is IGNORED rather than shifted into the mask.
// time.Weekday is an int underneath, so a caller can cast any integer to it,
// and 1<<9 would set a bit that Has can never read back — a day silently added
// to the set and permanently invisible.
func (w Weekdays) With(days ...time.Weekday) Weekdays {
	for _, d := range days {
		if d < time.Sunday || d > time.Saturday {
			continue
		}
		w |= 1 << uint(d)
	}
	return w
}

// Has reports whether the set contains the day.
func (w Weekdays) Has(d time.Weekday) bool {
	if d < time.Sunday || d > time.Saturday {
		return false
	}
	return w&(1<<uint(d)) != 0
}

// Empty reports whether the set names no day at all.
func (w Weekdays) Empty() bool { return w&allWeekdaysMask == 0 }

// Valid reports whether the set names at least one day and carries no bit
// outside the seven.
func (w Weekdays) Valid() bool {
	return !w.Empty() && w&^allWeekdaysMask == 0
}
