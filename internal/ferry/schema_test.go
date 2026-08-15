package ferry

import (
	"errors"
	"testing"
	"time"

	"github.com/pigfox/aegean-solver/internal/config"
	"github.com/pigfox/aegean-solver/internal/vocab"
)

// The fixtures below are the SHAPE of a schedule and not a schedule. Every
// coordinate, name and time in this file was chosen to exercise a rule, and no
// part of it is transcribed from any real timetable — the phase that produced
// this package was forbidden from authoring schedule data, and a test fixture
// that looked like a real sailing would be the easiest way for one to escape
// into the repository by being copied somewhere it does not belong.

func fixturePortAlpha() Port {
	return Port{ID: "alpha", PortName: "Alpha Harbor", IslandName: "Alpha", Lat: 37.0, Lon: 25.0}
}

func fixturePortBeta() Port {
	return Port{ID: "beta", PortName: "Beta Harbor", IslandName: "Beta", Lat: 37.5, Lon: 25.0}
}

func fixtureVessel() Vessel {
	return Vessel{
		ID:                "ship-one",
		Name:              "Ship One",
		OperatorID:        "op-one",
		Class:             vocab.ClassConventionalFerry,
		ServiceSpeedKnots: 15,
	}
}

func fixtureTrip() Trip {
	return Trip{
		ID: "trip-one", RouteID: "route-one", VesselID: "ship-one", ServiceID: "svc-one",
		StopTimes: []StopTime{
			{PortID: "alpha", ArrivalSeconds: 8 * config.SecondsPerHour, DepartureSeconds: 8 * config.SecondsPerHour},
			{PortID: "beta", ArrivalSeconds: 12 * config.SecondsPerHour, DepartureSeconds: 12 * config.SecondsPerHour},
		},
	}
}

func fixtureFeed() *Feed {
	return &Feed{
		Ports:     []Port{fixturePortAlpha(), fixturePortBeta()},
		Operators: []Operator{{ID: "op-one", Name: "Operator One"}},
		Vessels:   []Vessel{fixtureVessel()},
		Routes:    []Route{{ID: "route-one", OperatorID: "op-one", ShortName: "A1"}},
		Trips:     []Trip{fixtureTrip()},
		Calendars: []Calendar{{
			ServiceID: "svc-one",
			Days: Weekdays(0).With(time.Sunday, time.Monday, time.Tuesday, time.Wednesday,
				time.Thursday, time.Friday, time.Saturday),
			Start: Date{2026, 1, 1},
			End:   Date{2026, 12, 31},
		}},
	}
}

func TestPortValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*Port)
		want   error
	}{
		{name: "the fixture is valid"},
		{name: "an identifier outside the grammar", mutate: func(p *Port) { p.ID = "Alpha" }, want: ErrID},
		{name: "no port name", mutate: func(p *Port) { p.PortName = "" }, want: ErrName},
		{name: "a port name that is punctuation only", mutate: func(p *Port) { p.PortName = " -- " }, want: ErrName},
		{name: "no island name", mutate: func(p *Port) { p.IslandName = "" }, want: ErrName},
		{name: "latitude above the pole", mutate: func(p *Port) { p.Lat = 90.5 }, want: ErrCoordinate},
		{name: "latitude below the pole", mutate: func(p *Port) { p.Lat = -90.5 }, want: ErrCoordinate},
		{name: "longitude past the meridian", mutate: func(p *Port) { p.Lon = 180.5 }, want: ErrCoordinate},
		{name: "longitude behind the meridian", mutate: func(p *Port) { p.Lon = -180.5 }, want: ErrCoordinate},
		{name: "a negative transfer time", mutate: func(p *Port) { p.MinTransferSeconds = -1 }, want: ErrTransferTime},
		{
			name:   "a transfer time longer than a day",
			mutate: func(p *Port) { p.MinTransferSeconds = MaxMinTransferSeconds + 1 },
			want:   ErrTransferTime,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			p := fixturePortAlpha()
			if c.mutate != nil {
				c.mutate(&p)
			}
			assertErrIs(t, p.Validate(), c.want)
		})
	}
}

// TestPortCarriesBothNamesIndependently is the schema half of the first-class
// divergence between a harbor's name and the island it serves. Neither field
// substitutes for the other, and the two being EQUAL — a mainland port naming
// its own region — is legal rather than suspicious.
func TestPortCarriesBothNamesIndependently(t *testing.T) {
	t.Parallel()

	diverging := Port{ID: "adamas", PortName: "Adamas", IslandName: "Milos", Lat: 36.7, Lon: 24.4}
	if err := diverging.Validate(); err != nil {
		t.Fatalf("a port whose harbor name differs from its island name was refused: %v", err)
	}
	if diverging.PortName == diverging.IslandName {
		t.Fatal("the fixture stopped diverging, so this proves nothing")
	}

	coinciding := Port{ID: "mainland", PortName: "Mainland", IslandName: "Mainland", Lat: 37.9, Lon: 23.6}
	if err := coinciding.Validate(); err != nil {
		t.Fatalf("a port whose two names coincide was refused: %v", err)
	}
}

func TestPortMinTransferAppliesTheDefaultInOnePlace(t *testing.T) {
	t.Parallel()

	unset := fixturePortAlpha()
	if got, want := unset.MinTransfer(), config.DefaultMinTransferSeconds; got != want {
		t.Fatalf("an unset transfer time resolved to %ds, want the default %ds", got, want)
	}

	set := fixturePortAlpha()
	set.MinTransferSeconds = 45 * config.SecondsPerMinute
	if got, want := set.MinTransfer(), 45*config.SecondsPerMinute; got != want {
		t.Fatalf("a declared transfer time resolved to %ds, want %ds", got, want)
	}
}

func TestOperatorValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*Operator)
		want   error
	}{
		{name: "the fixture is valid"},
		{name: "an identifier outside the grammar", mutate: func(o *Operator) { o.ID = "OP" }, want: ErrID},
		{name: "no name", mutate: func(o *Operator) { o.Name = "" }, want: ErrName},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			o := Operator{ID: "op-one", Name: "Operator One"}
			if c.mutate != nil {
				c.mutate(&o)
			}
			assertErrIs(t, o.Validate(), c.want)
		})
	}
}

func TestVesselValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*Vessel)
		want   error
	}{
		{name: "the fixture is valid"},
		{name: "an identifier outside the grammar", mutate: func(v *Vessel) { v.ID = "Ship" }, want: ErrID},
		{name: "no name", mutate: func(v *Vessel) { v.Name = "" }, want: ErrName},
		{name: "an operator reference outside the grammar", mutate: func(v *Vessel) { v.OperatorID = "OP" }, want: ErrID},
		{name: "a class nobody declared", mutate: func(v *Vessel) { v.Class = "submarine" }, want: ErrVesselClass},
		{name: "no class at all", mutate: func(v *Vessel) { v.Class = "" }, want: ErrVesselClass},
		{name: "slower than its class can be", mutate: func(v *Vessel) { v.ServiceSpeedKnots = 1 }, want: ErrVesselSpeed},
		{name: "faster than its class can be", mutate: func(v *Vessel) { v.ServiceSpeedKnots = 99 }, want: ErrVesselSpeed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			v := fixtureVessel()
			if c.mutate != nil {
				c.mutate(&v)
			}
			assertErrIs(t, v.Validate(), c.want)
		})
	}
}

func TestRouteValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*Route)
		want   error
	}{
		{name: "the fixture is valid"},
		{name: "a long name alone is enough", mutate: func(r *Route) { r.ShortName = ""; r.LongName = "Alpha to Beta" }},
		{name: "an identifier outside the grammar", mutate: func(r *Route) { r.ID = "R1" }, want: ErrID},
		{name: "an operator reference outside the grammar", mutate: func(r *Route) { r.OperatorID = "OP" }, want: ErrID},
		{name: "neither name", mutate: func(r *Route) { r.ShortName = "" }, want: ErrRouteName},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := Route{ID: "route-one", OperatorID: "op-one", ShortName: "A1"}
			if c.mutate != nil {
				c.mutate(&r)
			}
			assertErrIs(t, r.Validate(), c.want)
		})
	}
}

func TestStopTimeValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*StopTime)
		want   error
	}{
		{name: "the fixture is valid"},
		{name: "arrival equal to departure is normal", mutate: func(s *StopTime) { s.DepartureSeconds = s.ArrivalSeconds }},
		{
			name: "past midnight is normal and is how an overnight sailing is written",
			mutate: func(s *StopTime) {
				s.ArrivalSeconds = 25 * config.SecondsPerHour
				s.DepartureSeconds = 26 * config.SecondsPerHour
			},
		},
		{name: "a port reference outside the grammar", mutate: func(s *StopTime) { s.PortID = "Alpha" }, want: ErrID},
		{name: "a negative arrival", mutate: func(s *StopTime) { s.ArrivalSeconds = -1 }, want: ErrStopTimeRange},
		{
			name: "an arrival past the ceiling",
			mutate: func(s *StopTime) {
				s.ArrivalSeconds = MaxStopTimeSeconds + 1
				s.DepartureSeconds = MaxStopTimeSeconds + 1
			},
			want: ErrStopTimeRange,
		},
		{name: "departing before arriving", mutate: func(s *StopTime) { s.DepartureSeconds = s.ArrivalSeconds - 1 }, want: ErrStopTimeRange},
		{name: "a departure past the ceiling", mutate: func(s *StopTime) { s.DepartureSeconds = MaxStopTimeSeconds + 1 }, want: ErrStopTimeRange},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			s := StopTime{PortID: "alpha", ArrivalSeconds: 3600, DepartureSeconds: 4200}
			if c.mutate != nil {
				c.mutate(&s)
			}
			assertErrIs(t, s.Validate(), c.want)
		})
	}
}

func TestTripValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*Trip)
		want   error
	}{
		{name: "the fixture is valid"},
		{
			name: "calling twice at one port is legal, because rotations exist",
			mutate: func(tr *Trip) {
				tr.StopTimes = append(tr.StopTimes, StopTime{
					PortID:           "alpha",
					ArrivalSeconds:   16 * config.SecondsPerHour,
					DepartureSeconds: 16 * config.SecondsPerHour,
				})
			},
		},
		{name: "an identifier outside the grammar", mutate: func(tr *Trip) { tr.ID = "T1" }, want: ErrID},
		{name: "a route reference outside the grammar", mutate: func(tr *Trip) { tr.RouteID = "R1" }, want: ErrID},
		{name: "a vessel reference outside the grammar", mutate: func(tr *Trip) { tr.VesselID = "S1" }, want: ErrID},
		{name: "a service reference outside the grammar", mutate: func(tr *Trip) { tr.ServiceID = "SVC" }, want: ErrID},
		{name: "one stop is not a journey", mutate: func(tr *Trip) { tr.StopTimes = tr.StopTimes[:1] }, want: ErrTooFewStops},
		{name: "no stops at all", mutate: func(tr *Trip) { tr.StopTimes = nil }, want: ErrTooFewStops},
		{name: "a malformed stop inside", mutate: func(tr *Trip) { tr.StopTimes[1].ArrivalSeconds = -1 }, want: ErrStopTimeRange},
		{
			name:   "a leg that takes no time at all",
			mutate: func(tr *Trip) { tr.StopTimes[1].ArrivalSeconds = tr.StopTimes[0].DepartureSeconds },
			want:   ErrStopSequence,
		},
		{
			name: "a leg that goes backward",
			mutate: func(tr *Trip) {
				tr.StopTimes[1].ArrivalSeconds = tr.StopTimes[0].DepartureSeconds - 1
				tr.StopTimes[1].DepartureSeconds = tr.StopTimes[1].ArrivalSeconds
			},
			want: ErrStopSequence,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			tr := fixtureTrip()
			if c.mutate != nil {
				c.mutate(&tr)
			}
			assertErrIs(t, tr.Validate(), c.want)
		})
	}
}

func TestCalendarValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*Calendar)
		want   error
	}{
		{name: "the fixture is valid"},
		{name: "a single-day window is valid", mutate: func(c *Calendar) { c.End = c.Start }},
		{name: "a service reference outside the grammar", mutate: func(c *Calendar) { c.ServiceID = "SVC" }, want: ErrID},
		{name: "naming no day at all", mutate: func(c *Calendar) { c.Days = 0 }, want: ErrWeekdays},
		{name: "a bit that is not a day", mutate: func(c *Calendar) { c.Days = Weekdays(1 << 7) }, want: ErrWeekdays},
		{name: "a start that is not a date", mutate: func(c *Calendar) { c.Start = Date{2026, 2, 30} }, want: ErrDate},
		{name: "an end that is not a date", mutate: func(c *Calendar) { c.End = Date{2026, 13, 1} }, want: ErrDate},
		{name: "a window that ends before it starts", mutate: func(c *Calendar) { c.End = c.Start.AddDays(-1) }, want: ErrDateWindow},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			cal := Calendar{
				ServiceID: "svc-one",
				Days:      Weekdays(0).With(time.Tuesday),
				Start:     Date{2026, 7, 1},
				End:       Date{2026, 7, 31},
			}
			if c.mutate != nil {
				c.mutate(&cal)
			}
			assertErrIs(t, cal.Validate(), c.want)
		})
	}
}

func TestCalendarDateValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*CalendarDate)
		want   error
	}{
		{name: "the fixture is valid"},
		{name: "removal is valid too", mutate: func(cd *CalendarDate) { cd.Exception = vocab.ServiceRemoved }},
		{name: "a service reference outside the grammar", mutate: func(cd *CalendarDate) { cd.ServiceID = "SVC" }, want: ErrID},
		{name: "a date that is not a date", mutate: func(cd *CalendarDate) { cd.Date = Date{2026, 4, 31} }, want: ErrDate},
		{name: "the zero exception is not a default", mutate: func(cd *CalendarDate) { cd.Exception = 0 }, want: ErrExceptionType},
		{name: "an unknown exception code", mutate: func(cd *CalendarDate) { cd.Exception = 7 }, want: ErrExceptionType},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			cd := CalendarDate{ServiceID: "svc-one", Date: Date{2026, 8, 15}, Exception: vocab.ServiceAdded}
			if c.mutate != nil {
				c.mutate(&cd)
			}
			assertErrIs(t, cd.Validate(), c.want)
		})
	}
}

// assertErrIs is the shared check every table above ends on: nil when want is
// nil, and wrapping want otherwise. It is one helper rather than a repeated
// four lines so that a table row says only what it is testing.
func assertErrIs(t *testing.T, got, want error) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Fatalf("got %v, want no error", got)
		}
		return
	}
	if !errors.Is(got, want) {
		t.Fatalf("got %v, want an error wrapping %v", got, want)
	}
}
