package ferry

import (
	"testing"
	"time"

	// The zone database is embedded into the TEST binary only, so the DST
	// assertions below can never degrade into a skip on a machine with no
	// system zoneinfo. It is standard library, so it costs no dependency, and
	// it does not reach the library itself — nothing outside a _test.go file
	// imports it.
	_ "time/tzdata"
)

func TestDateValid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   Date
		want bool
	}{
		{name: "an ordinary date", in: Date{Year: 2026, Month: 8, Day: 15}, want: true},
		{name: "the first day of a leap February", in: Date{Year: 2024, Month: 2, Day: 29}, want: true},
		{name: "February 29 in a common year", in: Date{Year: 2026, Month: 2, Day: 29}, want: false},
		{name: "February 30 never exists", in: Date{Year: 2024, Month: 2, Day: 30}, want: false},
		{name: "the 31st of a 30-day month", in: Date{Year: 2026, Month: 4, Day: 31}, want: false},
		{name: "month zero", in: Date{Year: 2026, Month: 0, Day: 1}, want: false},
		{name: "month thirteen", in: Date{Year: 2026, Month: 13, Day: 1}, want: false},
		{name: "day zero", in: Date{Year: 2026, Month: 1, Day: 0}, want: false},
		{name: "before the year floor", in: Date{Year: MinYear - 1, Month: 1, Day: 1}, want: false},
		{name: "at the year floor", in: Date{Year: MinYear, Month: 1, Day: 1}, want: true},
		{name: "past the year ceiling", in: Date{Year: MaxYear + 1, Month: 1, Day: 1}, want: false},
		{name: "at the year ceiling", in: Date{Year: MaxYear, Month: 12, Day: 31}, want: true},
		{name: "the zero value is not a date", in: Date{}, want: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := c.in.Valid(); got != c.want {
				t.Fatalf("Date%+v.Valid() = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// TestDateValidRefusesWhatTimeDateNormalizes is the reason Valid round-trips
// instead of counting days per month. The standard library does not reject the
// 30th of February; it quietly returns the 1st or 2nd of March. Without the
// round-trip that silence becomes a valid date nobody wrote.
func TestDateValidRefusesWhatTimeDateNormalizes(t *testing.T) {
	t.Parallel()

	normalized := time.Date(2026, time.February, 30, 0, 0, 0, 0, time.UTC)
	if normalized.Month() != time.March {
		t.Fatalf("premise failed: the standard library no longer normalizes February 30, it produced %v", normalized)
	}
	if (Date{Year: 2026, Month: 2, Day: 30}).Valid() {
		t.Fatal("February 30 was accepted, so the round-trip check is not doing its job")
	}
}

func TestDateCompare(t *testing.T) {
	t.Parallel()

	base := Date{Year: 2026, Month: 8, Day: 15}
	cases := []struct {
		name  string
		other Date
		want  int
	}{
		{name: "equal", other: base, want: 0},
		{name: "an earlier year", other: Date{Year: 2025, Month: 12, Day: 31}, want: 1},
		{name: "a later year", other: Date{Year: 2027, Month: 1, Day: 1}, want: -1},
		{name: "an earlier month", other: Date{Year: 2026, Month: 7, Day: 31}, want: 1},
		{name: "a later month", other: Date{Year: 2026, Month: 9, Day: 1}, want: -1},
		{name: "an earlier day", other: Date{Year: 2026, Month: 8, Day: 14}, want: 1},
		{name: "a later day", other: Date{Year: 2026, Month: 8, Day: 16}, want: -1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := base.Compare(c.other)
			if (got > 0) != (c.want > 0) || (got < 0) != (c.want < 0) {
				t.Fatalf("Date%+v.Compare(%+v) = %d, want sign %d", base, c.other, got, c.want)
			}
		})
	}
}

func TestDateAddDays(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   Date
		n    int
		want Date
	}{
		{name: "within a month", in: Date{2026, 8, 15}, n: 3, want: Date{2026, 8, 18}},
		{name: "across a month", in: Date{2026, 8, 31}, n: 1, want: Date{2026, 9, 1}},
		{name: "across a year", in: Date{2026, 12, 31}, n: 1, want: Date{2027, 1, 1}},
		{name: "backward across a year", in: Date{2027, 1, 1}, n: -1, want: Date{2026, 12, 31}},
		{name: "across a leap day", in: Date{2024, 2, 28}, n: 1, want: Date{2024, 2, 29}},
		{name: "zero is identity", in: Date{2026, 8, 15}, n: 0, want: Date{2026, 8, 15}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := c.in.AddDays(c.n); got != c.want {
				t.Fatalf("Date%+v.AddDays(%d) = %+v, want %+v", c.in, c.n, got, c.want)
			}
		})
	}
}

func TestDateWeekdayAndString(t *testing.T) {
	t.Parallel()

	d := Date{Year: 2026, Month: 8, Day: 15}
	if got, want := d.Weekday(), time.Saturday; got != want {
		t.Fatalf("Weekday() = %v, want %v", got, want)
	}
	if got, want := d.String(), "2026-08-15"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if got, want := (Date{Year: 999, Month: 1, Day: 2}).String(), "0999-01-02"; got != want {
		t.Fatalf("String() = %q, want %q — the pad width is part of the format", got, want)
	}
}

func TestDateIn(t *testing.T) {
	t.Parallel()

	east := time.FixedZone("east", 14*60*60)
	west := time.FixedZone("west", -11*60*60)
	instant := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)

	// The SAME instant is three different calendar dates depending on where the
	// question is asked from. That is the reason DateIn takes a location and
	// the reason a service date is not a time.Time.
	if got, want := DateIn(instant, east), (Date{2026, 8, 16}); got != want {
		t.Fatalf("DateIn east = %+v, want %+v", got, want)
	}
	if got, want := DateIn(instant, time.UTC), (Date{2026, 8, 15}); got != want {
		t.Fatalf("DateIn UTC = %+v, want %+v", got, want)
	}
	if got, want := DateIn(instant, west), (Date{2026, 8, 15}); got != want {
		t.Fatalf("DateIn west = %+v, want %+v", got, want)
	}
}

func TestStartOfServiceDayIsNoonMinusTwelveHours(t *testing.T) {
	t.Parallel()

	d := Date{Year: 2026, Month: 8, Day: 15}
	zones := []*time.Location{time.UTC, time.FixedZone("plus-three", 3*60*60)}
	for _, loc := range zones {
		start := d.StartOfServiceDay(loc)
		noon := time.Date(d.Year, time.Month(d.Month), d.Day, ServiceDayAnchorHour, 0, 0, 0, loc)
		if !start.Add(ServiceDayAnchorHour * time.Hour).Equal(noon) {
			t.Fatalf("in %v, start %v plus twelve hours is not local noon %v", loc, start, noon)
		}
	}
}

// TestStartOfServiceDayDivergesFromMidnightExactlyOnClockShiftDays is the
// evidence for ServiceDayAnchorHour existing at all.
//
// It does not hardcode a transition date, because a hardcoded one is a fact
// about a zone database version rather than about the code. It sweeps a whole
// year in a zone that shifts twice, counts the days on which the service day
// does NOT begin at local midnight, and asserts that count is exactly two.
// Anchoring on midnight instead would move every sailing on those days by an
// hour, in opposite directions.
func TestStartOfServiceDayDivergesFromMidnightExactlyOnClockShiftDays(t *testing.T) {
	t.Parallel()

	loc, err := time.LoadLocation("Europe/Athens")
	if err != nil {
		t.Fatalf("the embedded zone database did not load Europe/Athens: %v", err)
	}

	diverged := 0
	for d := (Date{Year: 2026, Month: 1, Day: 1}); d.Year == 2026; d = d.AddDays(1) {
		midnight := time.Date(d.Year, time.Month(d.Month), d.Day, 0, 0, 0, 0, loc)
		if !d.StartOfServiceDay(loc).Equal(midnight) {
			diverged++
		}
	}
	if diverged != 2 {
		t.Fatalf("the service day diverged from local midnight on %d days in 2026, want exactly 2 "+
			"(one clock shift forward, one back)", diverged)
	}
}

func TestWeekdays(t *testing.T) {
	t.Parallel()

	var w Weekdays
	if !w.Empty() || w.Valid() {
		t.Fatal("the zero weekday set must be empty and must not be valid")
	}

	w = w.With(time.Tuesday, time.Friday)
	if !w.Has(time.Tuesday) || !w.Has(time.Friday) {
		t.Fatal("With did not record the days it was given")
	}
	if w.Has(time.Monday) || w.Has(time.Saturday) {
		t.Fatal("With recorded a day it was not given")
	}
	if w.Empty() || !w.Valid() {
		t.Fatal("a set naming two days must be non-empty and valid")
	}
}

// TestWeekdaysIgnoresDaysThatAreNotDays pins the branch that keeps a stray bit
// out of the mask. time.Weekday is an int underneath, so a caller can cast any
// integer to it; shifting by 9 would set a bit Has can never read back, which
// on the page looks like a service that simply never runs.
func TestWeekdaysIgnoresDaysThatAreNotDays(t *testing.T) {
	t.Parallel()

	for _, bad := range []time.Weekday{-1, 7, 9} {
		if got := Weekdays(0).With(bad); got != 0 {
			t.Fatalf("With(%d) produced %08b, want an unchanged set", int(bad), uint8(got))
		}
		if Weekdays(allWeekdaysMask).Has(bad) {
			t.Fatalf("Has(%d) reported a hit against a full mask", int(bad))
		}
	}
}

// TestWeekdaysValidRefusesAStrayHighBit covers the half of Valid that Empty
// does not: a value built by casting rather than by With.
func TestWeekdaysValidRefusesAStrayHighBit(t *testing.T) {
	t.Parallel()

	stray := Weekdays(1 << 7)
	if !stray.Empty() {
		t.Fatal("a bit outside the seven days must not count as naming a day")
	}
	if stray.Valid() {
		t.Fatal("a set whose only bit is not a day was accepted")
	}
	withReal := stray.With(time.Monday)
	if withReal.Valid() {
		t.Fatal("a real day alongside a stray bit was accepted")
	}
}
