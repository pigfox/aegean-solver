package tdsp

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"testing"
	"time"

	aegean "github.com/pigfox/aegean-solver"
	"github.com/pigfox/aegean-solver/ferry"
	"github.com/pigfox/aegean-solver/internal/config"
	"github.com/pigfox/aegean-solver/vocab"
)

// The generator below produces SCHEDULE-SHAPED NOISE and nothing else. Its
// ports are called p0..p4 and sit at random points inside a small box of
// invented coordinates; its sailings are random walks at a made-up cruising
// speed. It exists entirely inside this _test.go file, nothing it produces is
// written to disk, and no fixture derived from it reaches a committed artifact.
// A generated graph is a fixture; it is not data, and it is not a timetable.

const (
	genStream = 0x2545F4914F6CDD1D

	genLatBase     = 37.0
	genLonBase     = 25.0
	genSpanDegrees = 0.4

	// genCruiseKnots is the pace every generated leg is timed at. It sits well
	// below the fixture vessel's own service speed, so a generated feed can
	// never be refused by the physics check — which would turn a routing test
	// into a fixture-tuning exercise.
	genCruiseKnots = 20

	genMinPorts   = 3
	genPortSpread = 3
	genMinTrips   = 4
	genTripSpread = 5
	genMinStops   = 2
	genStopSpread = 3
	genServices   = 2

	// genRunningDayOdds is the reciprocal chance that a weekday is LEFT OUT of
	// a generated service pattern. Two days in three are included, which is
	// denser than a coin flip on purpose: a sparse schedule makes almost every
	// query unreachable, and a run where nearly everything is unreachable
	// compares two refusals and establishes nothing about routing.
	genRunningDayOdds = 3

	// genSameSpotOdds is the reciprocal chance that a generated query asks for
	// a journey to the port it starts at. It is kept deliberately small but
	// non-zero: the case has to be exercised and it is worth almost nothing
	// repeated.
	genSameSpotOdds = 20

	genFirstHour        = 5
	genLastHour         = 20
	genSlackSeconds     = 40 * config.SecondsPerMinute
	genMaxDwellSeconds  = 15 * config.SecondsPerMinute
	genMinTravelSeconds = 20 * config.SecondsPerMinute

	// A narrow horizon on purpose. It keeps the exhaustive verifier finite and,
	// just as usefully, it makes "no connection inside the horizon" a common
	// outcome rather than a rare one, so the two searches are compared on the
	// negative answer as often as the positive.
	genHorizonDays = 2

	// genInstances is how many independent generated schedules the agreement
	// test runs. See the VERIFIED line it logs for exactly what that number
	// does and does not establish.
	genInstances = 2000

	// genNarrowWindowOdds is the reciprocal chance that a generated service
	// gets a short seasonal window instead of a year-long one.
	genNarrowWindowOdds = 4
)

var (
	genWindowStart = ferry.Date{Year: 2026, Month: 1, Day: 1}
	genWindowEnd   = ferry.Date{Year: 2026, Month: 12, Day: 31}
	genQueryDate   = ferry.Date{Year: 2026, Month: 8, Day: 15}

	genTransferChoices = []int{
		10 * config.SecondsPerMinute,
		20 * config.SecondsPerMinute,
		30 * config.SecondsPerMinute,
		45 * config.SecondsPerMinute,
	}
)

func newGenRNG(seed uint64) *rand.Rand { return rand.New(rand.NewPCG(seed, genStream)) }

// minimumTravelSeconds is how long a generated leg must take for its implied
// speed to stay at the generator's cruising pace or below.
func minimumTravelSeconds(from, to ferry.Port) int {
	km := aegean.DistanceKm(
		aegean.Site{Lat: from.Lat, Lon: from.Lon},
		aegean.Site{Lat: to.Lat, Lon: to.Lon},
	)
	kmh := genCruiseKnots * config.KilometersPerNauticalMile
	seconds := int(math.Ceil(km / kmh * float64(config.SecondsPerHour)))
	if seconds < genMinTravelSeconds {
		return genMinTravelSeconds
	}
	return seconds
}

func generateFeed(t *testing.T, r *rand.Rand) *ferry.Feed {
	t.Helper()

	nPorts := genMinPorts + r.IntN(genPortSpread)
	ports := make([]ferry.Port, nPorts)
	for i := range ports {
		ports[i] = ferry.Port{
			ID:                 vocab.ID(fmt.Sprintf("p%d", i)),
			PortName:           fmt.Sprintf("Port %d", i),
			IslandName:         fmt.Sprintf("Island %d", i),
			Lat:                genLatBase + r.Float64()*genSpanDegrees,
			Lon:                genLonBase + r.Float64()*genSpanDegrees,
			MinTransferSeconds: genTransferChoices[r.IntN(len(genTransferChoices))],
		}
	}

	calendars := make([]ferry.Calendar, 0, genServices)
	for s := range genServices {
		days := ferry.Weekdays(0)
		for d := time.Sunday; d <= time.Saturday; d++ {
			if r.IntN(genRunningDayOdds) != 0 {
				days = days.With(d)
			}
		}
		if days.Empty() {
			days = days.With(time.Wednesday)
		}
		start, end := genWindowStart, genWindowEnd
		if r.IntN(genNarrowWindowOdds) == 0 {
			start = genQueryDate.AddDays(-1 - r.IntN(genPortSpread))
			end = start.AddDays(1 + r.IntN(genPortSpread))
		}
		calendars = append(calendars, ferry.Calendar{
			ServiceID: vocab.ID(fmt.Sprintf("svc-%d", s)), Days: days, Start: start, End: end,
		})
	}

	nTrips := genMinTrips + r.IntN(genTripSpread)
	trips := make([]ferry.Trip, 0, nTrips)
	for i := range nTrips {
		nStops := genMinStops + r.IntN(genStopSpread)
		route := []int{r.IntN(nPorts)}
		for len(route) < nStops {
			next := r.IntN(nPorts)
			// Consecutive calls at one port would be a leg between a place and
			// itself. A trip may still RETURN to a port later, which is a
			// rotation and is legal.
			if next == route[len(route)-1] {
				continue
			}
			route = append(route, next)
		}

		stops := make([]ferry.StopTime, 0, nStops)
		now := (genFirstHour + r.IntN(genLastHour-genFirstHour)) * config.SecondsPerHour
		for j, pi := range route {
			if j == 0 {
				stops = append(stops, ferry.StopTime{PortID: ports[pi].ID, ArrivalSeconds: now, DepartureSeconds: now})
				continue
			}
			arrive := now + minimumTravelSeconds(ports[route[j-1]], ports[pi]) + r.IntN(genSlackSeconds)
			depart := arrive + r.IntN(genMaxDwellSeconds)
			stops = append(stops, ferry.StopTime{PortID: ports[pi].ID, ArrivalSeconds: arrive, DepartureSeconds: depart})
			now = depart
		}

		trips = append(trips, ferry.Trip{
			ID:        vocab.ID(fmt.Sprintf("trip-%d", i)),
			RouteID:   "route-one",
			VesselID:  "ship-one",
			ServiceID: calendars[r.IntN(len(calendars))].ServiceID,
			StopTimes: stops,
		})
	}

	f := &ferry.Feed{
		Location:  time.UTC,
		Ports:     ports,
		Operators: []ferry.Operator{{ID: "op-one", Name: "Operator One"}},
		Vessels: []ferry.Vessel{{
			ID: "ship-one", Name: "Ship One", OperatorID: "op-one",
			Class: vocab.ClassHighSpeedCatamaran, ServiceSpeedKnots: fixtureSpeedKnots,
		}},
		Routes:    []ferry.Route{{ID: "route-one", OperatorID: "op-one", ShortName: "R1"}},
		Trips:     trips,
		Calendars: calendars,
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("the generator produced a feed the schema refuses, which is a defect in the generator: %v", err)
	}
	return f
}

func generateQuery(r *rand.Rand, f *ferry.Feed) Query {
	origin := f.Ports[r.IntN(len(f.Ports))].ID
	destination := origin
	// Drawing both ends independently made a quarter of every run a journey to
	// the port the traveler was already standing in, which is answered before
	// the graph is even built. The case still has to be exercised, so it is
	// kept at a low fixed rate rather than removed.
	if r.IntN(genSameSpotOdds) != 0 {
		for destination == origin {
			destination = f.Ports[r.IntN(len(f.Ports))].ID
		}
	}
	return Query{
		Feed:        f,
		Origin:      origin,
		Destination: destination,
		Depart: time.Date(genQueryDate.Year, time.Month(genQueryDate.Month), genQueryDate.Day,
			r.IntN(24), 0, 0, 0, time.UTC),
		HorizonDays: genHorizonDays,
	}
}

// TestSolverAgreesWithExhaustiveEnumeration is the verification the whole
// package answers to.
//
// # What is compared, and what is not
//
// The two searches are compared on the COST PAIR — arrival instant and leg
// count — and not on the itinerary. Where several journeys share a cost, the
// two are free to return different ones and both are right; asserting identical
// leg lists would be asserting that two tie-breaking rules coincide, which is a
// claim about the implementations rather than about the answer. The itinerary
// the solver returns is checked separately, against the feed, by
// assertItineraryIsFeasible.
//
// # What agreement establishes, stated narrowly
//
// It establishes that on THESE generated instances the two searches give the
// same answer. That is a property of these instances and not of the method: the
// generator makes small schedules inside a narrow horizon, and a whole class of
// schedule it does not produce is a class neither search has been checked on.
// It is evidence, not a proof, and the count logged below is the size of the
// evidence rather than the strength of it.
func TestSolverAgreesWithExhaustiveEnumeration(t *testing.T) {
	t.Parallel()

	var (
		refused    int
		none       int
		direct     int
		connecting int
		sameSpot   int
		maxLegs    int
	)

	for i := range genInstances {
		seed := uint64(i) + 1
		r := newGenRNG(seed)
		f := generateFeed(t, r)
		q := generateQuery(r, f)

		want, wantErr := BruteEarliestArrival(q)
		if errors.Is(wantErr, ErrTooManyStates) {
			// Reported rather than swallowed. A run where this climbs is a run
			// where the evidence is thinner than the instance count suggests.
			refused++
			continue
		}
		got, gotErr := EarliestArrival(q)

		switch {
		case wantErr != nil && gotErr != nil:
			if !errors.Is(gotErr, ErrNoConnection) || !errors.Is(wantErr, ErrNoConnection) {
				t.Fatalf("seed %d: unexpected errors, solver %v, exhaustive %v", seed, gotErr, wantErr)
			}
			none++
			continue
		case wantErr != nil:
			t.Fatalf("seed %d: the solver found an itinerary arriving %v that exhaustive search says does not exist",
				seed, got.Arrive)
		case gotErr != nil:
			t.Fatalf("seed %d: exhaustive search found an itinerary arriving %v that the solver missed (%v)",
				seed, want.Arrive, gotErr)
		}

		if !got.Arrive.Equal(want.Arrive) {
			t.Fatalf("seed %d: solver arrives %v, exhaustive search arrives %v", seed, got.Arrive, want.Arrive)
		}
		if len(got.Legs) != len(want.Legs) {
			t.Fatalf("seed %d: both arrive %v but the solver uses %d legs and exhaustive search uses %d",
				seed, got.Arrive, len(got.Legs), len(want.Legs))
		}
		assertItineraryIsFeasible(t, f, got, q.Depart)

		switch n := len(got.Legs); {
		case n == 0:
			sameSpot++
		case n == 1:
			direct++
		default:
			connecting++
		}
		maxLegs = max(maxLegs, len(got.Legs))
	}

	// THE OUTCOME CLASSES ARE ASSERTED, NOT JUST COUNTED, and this is the shape
	// the seed-variance trap recorded in the ledger calls for. The tempting
	// check on a seeded generator is that different seeds give different
	// answers, which is a claim about the DATA and can be false for perfectly
	// good reasons. What matters is that the run actually exercised the
	// behaviors being verified: if every generated instance came back "no
	// connection", the agreement above would be a comparison of two refusals
	// and would prove nothing about routing at all.
	if none == 0 {
		t.Fatal("no generated instance was unreachable, so the negative answer was never compared")
	}
	if direct == 0 {
		t.Fatal("no generated instance was a single sailing, so the simplest positive answer was never compared")
	}
	if connecting == 0 {
		t.Fatal("no generated instance required a transfer, so the interesting positive answer was never compared")
	}
	if refused*2 > genInstances {
		t.Fatalf("exhaustive search refused %d of %d instances; the evidence is thinner than the count suggests",
			refused, genInstances)
	}

	t.Logf("VERIFIED: %d generated schedules, %d compared against exhaustive enumeration "+
		"(%d unreachable, %d single-sailing, %d requiring a transfer, %d origin equals destination, "+
		"deepest itinerary %d legs, %d refused above the work ceiling). "+
		"VERIFIED OVER: randomly generated schedules of %d-%d ports and %d-%d trips of %d-%d calls, "+
		"inside a %d-day horizon, in UTC. This is agreement on THESE instances and is a property of "+
		"them, not of the method.",
		genInstances, genInstances-refused, none, direct, connecting, sameSpot, maxLegs, refused,
		genMinPorts, genMinPorts+genPortSpread-1, genMinTrips, genMinTrips+genTripSpread-1,
		genMinStops, genMinStops+genStopSpread-1, genHorizonDays)
}

// assertAgreesWithExhaustiveSearch runs the same comparison over one
// hand-written scenario. The hand-written cases are the ones where a reader can
// see what the right answer is; running them through the verifier as well means
// the verifier itself is exercised on schedules somebody chose rather than only
// on schedules a generator emitted.
func assertAgreesWithExhaustiveSearch(t *testing.T, q Query) {
	t.Helper()

	got, gotErr := EarliestArrival(q)
	want, wantErr := BruteEarliestArrival(q)

	if (gotErr == nil) != (wantErr == nil) {
		t.Fatalf("the two searches disagree on whether a journey exists: solver %v, exhaustive %v", gotErr, wantErr)
	}
	if gotErr != nil {
		if !errors.Is(gotErr, wantErr) && !errors.Is(wantErr, gotErr) {
			t.Fatalf("the two searches refused differently: solver %v, exhaustive %v", gotErr, wantErr)
		}
		return
	}
	if !got.Arrive.Equal(want.Arrive) {
		t.Fatalf("solver arrives %v, exhaustive search arrives %v", got.Arrive, want.Arrive)
	}
	if len(got.Legs) != len(want.Legs) {
		t.Fatalf("both arrive %v but the solver uses %d legs and exhaustive search uses %d",
			got.Arrive, len(got.Legs), len(want.Legs))
	}
}

// assertItineraryIsFeasible re-derives the answer from the feed instead of
// trusting the search that produced it.
//
// It is deliberately independent of both searches: it looks the trip up by
// identifier, confirms the service really runs on the service date the leg
// claims, finds the two calls by port and re-computes their absolute instants
// from the service day. Then it checks the things a traveler would care about —
// you cannot board before you are there, and you cannot change vessels faster
// than the port allows.
func assertItineraryIsFeasible(t *testing.T, f *ferry.Feed, it Itinerary, depart time.Time) {
	t.Helper()

	if len(it.Legs) == 0 {
		if !it.Arrive.Equal(depart) {
			t.Fatalf("an itinerary with no legs arrives %v, want the departure instant %v", it.Arrive, depart)
		}
		return
	}
	if !it.Arrive.Equal(it.Legs[len(it.Legs)-1].Arrive) {
		t.Fatalf("the itinerary arrives %v but its last leg arrives %v", it.Arrive, it.Legs[len(it.Legs)-1].Arrive)
	}

	for i, leg := range it.Legs {
		trip, ok := tripByID(f, leg.TripID)
		if !ok {
			t.Fatalf("leg %d rides %q, which the feed does not declare", i, string(leg.TripID))
		}
		if !f.ServiceActiveOn(trip.ServiceID, leg.ServiceDate) {
			t.Fatalf("leg %d rides %q on %s, when its service does not run", i, string(leg.TripID), leg.ServiceDate)
		}
		dayStart := leg.ServiceDate.StartOfServiceDay(f.Loc()).Unix()
		if !legMatchesTheSchedule(trip, leg, dayStart) {
			t.Fatalf("leg %d claims %s->%s departing %v arriving %v, which is not a pair of calls on %q",
				i, string(leg.FromPort), string(leg.ToPort), leg.Depart, leg.Arrive, string(leg.TripID))
		}

		if i == 0 {
			if leg.Depart.Before(depart) {
				t.Fatalf("the first leg sails at %v, before the traveler is there at %v", leg.Depart, depart)
			}
			continue
		}

		prev := it.Legs[i-1]
		if prev.ToPort != leg.FromPort {
			t.Fatalf("leg %d steps off at %s and leg %d boards at %s", i-1, string(prev.ToPort), i, string(leg.FromPort))
		}
		port, ok := f.PortByID(leg.FromPort)
		if !ok {
			t.Fatalf("leg %d boards at %q, which the feed does not declare", i, string(leg.FromPort))
		}
		ready := prev.Arrive.Add(time.Duration(port.MinTransfer()) * time.Second)
		if leg.Depart.Before(ready) {
			t.Fatalf("leg %d sails at %v, but %s needs %ds to change vessels after berthing at %v",
				i, leg.Depart, string(port.ID), port.MinTransfer(), prev.Arrive)
		}
	}
}

func tripByID(f *ferry.Feed, id vocab.ID) (ferry.Trip, bool) {
	for _, t := range f.Trips {
		if t.ID == id {
			return t, true
		}
	}
	return ferry.Trip{}, false
}

// legMatchesTheSchedule confirms the leg names a real pair of calls on the trip
// — boarded at one, stepped off at a later one — with both instants re-derived
// from the service day rather than taken from the leg.
func legMatchesTheSchedule(trip ferry.Trip, leg Leg, dayStart int64) bool {
	for b, boarded := range trip.StopTimes {
		if boarded.PortID != leg.FromPort || dayStart+int64(boarded.DepartureSeconds) != leg.Depart.Unix() {
			continue
		}
		for a := b + 1; a < len(trip.StopTimes); a++ {
			alighted := trip.StopTimes[a]
			if alighted.PortID == leg.ToPort && dayStart+int64(alighted.ArrivalSeconds) == leg.Arrive.Unix() {
				return true
			}
		}
	}
	return false
}
