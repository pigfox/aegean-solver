package ferry

import (
	"testing"
	"time"

	"github.com/pigfox/aegean-solver/internal/config"
	"github.com/pigfox/aegean-solver/vocab"
)

// TestEmptyFeedIsValid is a REQUIREMENT and not a curiosity. The source
// boundary ships a null origin whose whole purpose is to hand back an empty
// schedule that passes here, so that everything downstream can be built and
// exercised before a single real row exists. A validator that refused the empty
// case would push whoever came next into populating something merely to get the
// pipeline to run, which is precisely how invented data gets into a repository
// that swore it would have none.
func TestEmptyFeedIsValid(t *testing.T) {
	t.Parallel()

	var f Feed
	if err := f.Validate(); err != nil {
		t.Fatalf("the empty feed was refused: %v", err)
	}
	if got := f.Loc(); got != time.UTC {
		t.Fatalf("Loc() on a feed with no zone = %v, want UTC", got)
	}
}

func TestFeedLocHonorsADeclaredZone(t *testing.T) {
	t.Parallel()

	zone := time.FixedZone("plus-three", 3*config.SecondsPerHour)
	f := Feed{Location: zone}
	if got := f.Loc(); got != zone {
		t.Fatalf("Loc() = %v, want the declared zone", got)
	}
}

func TestFeedPortByID(t *testing.T) {
	t.Parallel()

	f := fixtureFeed()
	got, ok := f.PortByID("beta")
	if !ok || got.ID != "beta" {
		t.Fatalf("PortByID(beta) = %+v, %v", got, ok)
	}
	if _, ok := f.PortByID("nowhere"); ok {
		t.Fatal("PortByID reported a hit for a port the feed does not declare")
	}
}

func TestFeedValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*Feed)
		want   error
	}{
		{name: "the fixture is valid"},
		{
			name:   "a malformed port is reported",
			mutate: func(f *Feed) { f.Ports[0].PortName = "" },
			want:   ErrName,
		},
		{
			name:   "two ports with one identifier",
			mutate: func(f *Feed) { f.Ports[1].ID = f.Ports[0].ID },
			want:   ErrDuplicateID,
		},
		{
			name:   "a malformed operator is reported",
			mutate: func(f *Feed) { f.Operators[0].Name = "" },
			want:   ErrName,
		},
		{
			name:   "two operators with one identifier",
			mutate: func(f *Feed) { f.Operators = append(f.Operators, f.Operators[0]) },
			want:   ErrDuplicateID,
		},
		{
			name:   "a malformed vessel is reported",
			mutate: func(f *Feed) { f.Vessels[0].Class = "submarine" },
			want:   ErrVesselClass,
		},
		{
			name:   "two vessels with one identifier",
			mutate: func(f *Feed) { f.Vessels = append(f.Vessels, f.Vessels[0]) },
			want:   ErrDuplicateID,
		},
		{
			name:   "a vessel whose operator nobody declared",
			mutate: func(f *Feed) { f.Vessels[0].OperatorID = "op-missing" },
			want:   ErrDanglingRef,
		},
		{
			name:   "a malformed route is reported",
			mutate: func(f *Feed) { f.Routes[0].ShortName = "" },
			want:   ErrRouteName,
		},
		{
			name:   "two routes with one identifier",
			mutate: func(f *Feed) { f.Routes = append(f.Routes, f.Routes[0]) },
			want:   ErrDuplicateID,
		},
		{
			name:   "a route whose operator nobody declared",
			mutate: func(f *Feed) { f.Routes[0].OperatorID = "op-missing" },
			want:   ErrDanglingRef,
		},
		{
			name:   "a malformed calendar is reported",
			mutate: func(f *Feed) { f.Calendars[0].Days = 0 },
			want:   ErrWeekdays,
		},
		{
			name: "a malformed calendar override is reported",
			mutate: func(f *Feed) {
				f.CalendarDates = append(f.CalendarDates, CalendarDate{ServiceID: "svc-one", Date: Date{2026, 8, 15}})
			},
			want: ErrExceptionType,
		},
		{
			name: "one service given two overrides on one date",
			mutate: func(f *Feed) {
				f.CalendarDates = append(f.CalendarDates,
					CalendarDate{ServiceID: "svc-one", Date: Date{2026, 8, 15}, Exception: vocab.ServiceAdded},
					CalendarDate{ServiceID: "svc-one", Date: Date{2026, 8, 15}, Exception: vocab.ServiceRemoved},
				)
			},
			want: ErrConflictingException,
		},
		{
			name:   "a malformed trip is reported",
			mutate: func(f *Feed) { f.Trips[0].StopTimes = f.Trips[0].StopTimes[:1] },
			want:   ErrTooFewStops,
		},
		{
			name:   "two trips with one identifier",
			mutate: func(f *Feed) { f.Trips = append(f.Trips, f.Trips[0]) },
			want:   ErrDuplicateID,
		},
		{
			name:   "a trip whose route nobody declared",
			mutate: func(f *Feed) { f.Trips[0].RouteID = "route-missing" },
			want:   ErrDanglingRef,
		},
		{
			name:   "a trip whose vessel nobody declared",
			mutate: func(f *Feed) { f.Trips[0].VesselID = "ship-missing" },
			want:   ErrDanglingRef,
		},
		{
			name:   "a trip whose service nobody declared",
			mutate: func(f *Feed) { f.Trips[0].ServiceID = "svc-missing" },
			want:   ErrDanglingRef,
		},
		{
			name:   "a trip calling at a port nobody declared",
			mutate: func(f *Feed) { f.Trips[0].StopTimes[1].PortID = "port-missing" },
			want:   ErrDanglingRef,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := fixtureFeed()
			if c.mutate != nil {
				c.mutate(f)
			}
			assertErrIs(t, f.Validate(), c.want)
		})
	}
}

// TestFeedRefusesAPhysicallyImpossibleLeg is the one rule in this package that
// checks a schedule against the sea rather than against the grammar. Every
// identifier below is sound and every time is well ordered; the only thing
// wrong is that no conventional ferry crosses fifty kilometers in ten minutes.
func TestFeedRefusesAPhysicallyImpossibleLeg(t *testing.T) {
	t.Parallel()

	f := fixtureFeed()
	f.Trips[0].StopTimes[1].ArrivalSeconds = f.Trips[0].StopTimes[0].DepartureSeconds + 10*config.SecondsPerMinute
	f.Trips[0].StopTimes[1].DepartureSeconds = f.Trips[0].StopTimes[1].ArrivalSeconds
	assertErrIs(t, f.Validate(), ErrImpliedSpeed)
}

// TestFeedAcceptsALegAtTheVesselsOwnPace is the other direction of the same
// check, and it is what stops the speed rule from being a test that everything
// fails. A schedule inside the envelope must pass.
func TestFeedAcceptsALegAtTheVesselsOwnPace(t *testing.T) {
	t.Parallel()

	f := fixtureFeed()
	if err := f.Validate(); err != nil {
		t.Fatalf("a leg well inside the vessel envelope was refused: %v", err)
	}
}

// TestAServiceMayBeDeclaredByOverridesAlone covers the branch that lets a
// relief sailing exist. It runs on three named dates and has no weekly pattern
// at all, and requiring a Calendar row for it would force whoever populates the
// feed to invent a pattern that is not there.
func TestAServiceMayBeDeclaredByOverridesAlone(t *testing.T) {
	t.Parallel()

	f := fixtureFeed()
	f.Trips[0].ServiceID = "svc-relief"
	f.CalendarDates = []CalendarDate{
		{ServiceID: "svc-relief", Date: Date{2026, 8, 14}, Exception: vocab.ServiceAdded},
		{ServiceID: "svc-relief", Date: Date{2026, 8, 15}, Exception: vocab.ServiceAdded},
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("a service declared only by overrides was refused: %v", err)
	}
	if !f.ServiceActiveOn("svc-relief", Date{2026, 8, 15}) {
		t.Fatal("the relief service did not run on a date it was explicitly added to")
	}
	if f.ServiceActiveOn("svc-relief", Date{2026, 8, 16}) {
		t.Fatal("the relief service ran on a date nothing declared")
	}
}

// TestTuesdaysInJulyAndNotInOctoberNeedsNoBranching is the seasonality case the
// directive named, asserted rather than described.
//
// One row expresses it: a weekday pattern and an inclusive window. Nothing in
// the caller has to test a month, and no date exception is written. The October
// half of the assertion is the half that matters — a window that failed to
// close would look identical on every July date.
func TestTuesdaysInJulyAndNotInOctoberNeedsNoBranching(t *testing.T) {
	t.Parallel()

	f := &Feed{Calendars: []Calendar{{
		ServiceID: "svc-summer",
		Days:      Weekdays(0).With(time.Tuesday),
		Start:     Date{2026, 7, 1},
		End:       Date{2026, 7, 31},
	}}}
	if err := f.Validate(); err != nil {
		t.Fatalf("the one-row season was refused: %v", err)
	}

	tuesdays := 0
	for d := (Date{2026, 7, 1}); d.Compare(Date{2026, 7, 31}) <= 0; d = d.AddDays(1) {
		active := f.ServiceActiveOn("svc-summer", d)
		wantActive := d.Weekday() == time.Tuesday
		if active != wantActive {
			t.Fatalf("%s: active = %v, want %v", d, active, wantActive)
		}
		if active {
			tuesdays++
		}
	}
	if tuesdays == 0 {
		t.Fatal("no Tuesday in July was active, so the July half proves nothing")
	}
	for d := (Date{2026, 10, 1}); d.Compare(Date{2026, 10, 31}) <= 0; d = d.AddDays(1) {
		if f.ServiceActiveOn("svc-summer", d) {
			t.Fatalf("%s in October was active, so the window never closed", d)
		}
	}
}

// TestSeveralWindowsPerServiceUnion is the deliberate divergence from GTFS,
// pinned. Two seasons, one service, no date exceptions — which is the whole
// reason a service is allowed more than one Calendar row here.
func TestSeveralWindowsPerServiceUnion(t *testing.T) {
	t.Parallel()

	f := &Feed{Calendars: []Calendar{
		{ServiceID: "svc-two-seasons", Days: Weekdays(0).With(time.Tuesday), Start: Date{2026, 7, 1}, End: Date{2026, 7, 31}},
		{ServiceID: "svc-two-seasons", Days: Weekdays(0).With(time.Tuesday), Start: Date{2026, 12, 1}, End: Date{2026, 12, 31}},
	}}
	if err := f.Validate(); err != nil {
		t.Fatalf("two windows for one service were refused: %v", err)
	}
	for _, c := range []struct {
		d    Date
		want bool
	}{
		{d: Date{2026, 7, 7}, want: true},
		{d: Date{2026, 12, 1}, want: true},
		{d: Date{2026, 9, 1}, want: false},
	} {
		if c.d.Weekday() != time.Tuesday {
			t.Fatalf("fixture drift: %s is a %v, not a Tuesday", c.d, c.d.Weekday())
		}
		if got := f.ServiceActiveOn("svc-two-seasons", c.d); got != c.want {
			t.Fatalf("%s: active = %v, want %v", c.d, got, c.want)
		}
	}
}

// TestAnOverrideBeatsThePattern pins the precedence rule in both directions: a
// date the pattern includes can be taken away, and a date it excludes can be
// given back.
func TestAnOverrideBeatsThePattern(t *testing.T) {
	t.Parallel()

	// 2026-07-07 is a Tuesday and 2026-07-08 is a Wednesday; the pattern names
	// Tuesdays only.
	f := &Feed{
		Calendars: []Calendar{{
			ServiceID: "svc-summer",
			Days:      Weekdays(0).With(time.Tuesday),
			Start:     Date{2026, 7, 1},
			End:       Date{2026, 7, 31},
		}},
		CalendarDates: []CalendarDate{
			{ServiceID: "svc-summer", Date: Date{2026, 7, 7}, Exception: vocab.ServiceRemoved},
			{ServiceID: "svc-summer", Date: Date{2026, 7, 8}, Exception: vocab.ServiceAdded},
		},
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("the fixture was refused: %v", err)
	}
	if f.ServiceActiveOn("svc-summer", Date{2026, 7, 7}) {
		t.Fatal("a removal did not take a Tuesday out of the pattern")
	}
	if !f.ServiceActiveOn("svc-summer", Date{2026, 7, 8}) {
		t.Fatal("an addition did not put a Wednesday into the service")
	}
	if !f.ServiceActiveOn("svc-summer", Date{2026, 7, 14}) {
		t.Fatal("an untouched Tuesday stopped running")
	}
	if f.ServiceActiveOn("svc-other", Date{2026, 7, 14}) {
		t.Fatal("a service nothing declares reported itself active")
	}
}
