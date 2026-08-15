package tdsp

import (
	"errors"
	"testing"
	"time"

	"github.com/pigfox/aegean-solver/internal/config"
	"github.com/pigfox/aegean-solver/internal/ferry"
	"github.com/pigfox/aegean-solver/internal/vocab"
)

// Everything below is a FIXTURE and not a schedule. The ports are called Alpha
// through Delta, they sit on a straight line of invented coordinates, and every
// sailing exists to exercise one rule. None of it is transcribed from any real
// timetable, and it is named so that nobody could mistake it for one and copy
// it somewhere it would be read as data.

const (
	transferAtHub = 30 * config.SecondsPerMinute
	// The fixture vessel is fast and the ports are close, so no fixture leg
	// comes anywhere near the speed envelope. That keeps the physics check out
	// of the way of tests that are about routing.
	fixtureSpeedKnots = 30
)

func clock(hour, minute int) int {
	return hour*config.SecondsPerHour + minute*config.SecondsPerMinute
}

type stop struct {
	port vocab.ID
	arr  int
	dep  int
}

func call(port vocab.ID, at int) stop { return stop{port: port, arr: at, dep: at} }

func dwell(port vocab.ID, arr, dep int) stop { return stop{port: port, arr: arr, dep: dep} }

func fixtureTrip(id, service vocab.ID, stops ...stop) ferry.Trip {
	t := ferry.Trip{ID: id, RouteID: "route-one", VesselID: "ship-one", ServiceID: service}
	for _, s := range stops {
		t.StopTimes = append(t.StopTimes, ferry.StopTime{
			PortID: s.port, ArrivalSeconds: s.arr, DepartureSeconds: s.dep,
		})
	}
	return t
}

func everyDay() ferry.Weekdays {
	return ferry.Weekdays(0).With(time.Sunday, time.Monday, time.Tuesday, time.Wednesday,
		time.Thursday, time.Friday, time.Saturday)
}

// fixtureFeed assembles a feed around whatever trips and calendars a test
// supplies. Ports, operator, vessel and route are constant across every
// scenario, so a scenario reads as the sailings it is about.
func fixtureFeed(t *testing.T, calendars []ferry.Calendar, trips ...ferry.Trip) *ferry.Feed {
	t.Helper()
	f := &ferry.Feed{
		Location: time.UTC,
		Ports: []ferry.Port{
			{ID: "alpha", PortName: "Alpha Harbor", IslandName: "Alpha", Lat: 37.0, Lon: 25.0},
			{ID: "hub", PortName: "Hub Harbor", IslandName: "Hub", Lat: 37.3, Lon: 25.0, MinTransferSeconds: transferAtHub},
			{ID: "beta", PortName: "Beta Harbor", IslandName: "Beta", Lat: 37.6, Lon: 25.0},
			{ID: "delta", PortName: "Delta Harbor", IslandName: "Delta", Lat: 37.9, Lon: 25.0},
		},
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
		t.Fatalf("the fixture feed is not valid, so nothing below would mean anything: %v", err)
	}
	return f
}

func allYear(service vocab.ID) []ferry.Calendar {
	return []ferry.Calendar{{
		ServiceID: service, Days: everyDay(),
		Start: ferry.Date{Year: 2026, Month: 1, Day: 1},
		End:   ferry.Date{Year: 2026, Month: 12, Day: 31},
	}}
}

func departAt(hour, minute int) time.Time {
	return time.Date(2026, time.August, 15, hour, minute, 0, 0, time.UTC)
}

// wantArrival asserts the itinerary's arrival instant and leg count, which is
// the cost pair the solver actually minimizes.
func wantArrival(t *testing.T, got Itinerary, hour, minute, legs int) {
	t.Helper()
	want := departAt(hour, minute)
	if !got.Arrive.Equal(want) {
		t.Fatalf("arrived %v, want %v", got.Arrive, want)
	}
	if len(got.Legs) != legs {
		t.Fatalf("itinerary has %d legs, want %d: %+v", len(got.Legs), legs, got.Legs)
	}
}

func TestADirectSailingIsOneLeg(t *testing.T) {
	t.Parallel()

	f := fixtureFeed(t, allYear("svc-all"),
		fixtureTrip("trip-a", "svc-all", call("alpha", clock(8, 0)), call("beta", clock(10, 0))),
	)
	got, err := EarliestArrival(Query{Feed: f, Origin: "alpha", Destination: "beta", Depart: departAt(7, 0)})
	if err != nil {
		t.Fatalf("EarliestArrival: %v", err)
	}
	wantArrival(t, got, 10, 0, 1)
	assertItineraryIsFeasible(t, f, got, departAt(7, 0))
}

// TestNoConnectionExists is one of the four cases the directive names. The
// schedule is real and internally sound; there is simply no way through.
func TestNoConnectionExists(t *testing.T) {
	t.Parallel()

	f := fixtureFeed(t, allYear("svc-all"),
		fixtureTrip("trip-a", "svc-all", call("alpha", clock(8, 0)), call("hub", clock(10, 0))),
	)
	_, err := EarliestArrival(Query{Feed: f, Origin: "alpha", Destination: "beta", Depart: departAt(7, 0)})
	if !errors.Is(err, ErrNoConnection) {
		t.Fatalf("got %v, want ErrNoConnection", err)
	}
	if _, err := BruteEarliestArrival(Query{Feed: f, Origin: "alpha", Destination: "beta", Depart: departAt(7, 0)}); !errors.Is(err, ErrNoConnection) {
		t.Fatalf("the exhaustive search got %v, want ErrNoConnection", err)
	}
}

// TestAConnectionRequiringAHubTransfer is the second named case: two legs and a
// change of vessel, which no single sailing offers.
func TestAConnectionRequiringAHubTransfer(t *testing.T) {
	t.Parallel()

	f := fixtureFeed(t, allYear("svc-all"),
		fixtureTrip("trip-a", "svc-all", call("alpha", clock(8, 0)), call("hub", clock(10, 0))),
		fixtureTrip("trip-b", "svc-all", call("hub", clock(11, 0)), call("beta", clock(13, 0))),
	)
	q := Query{Feed: f, Origin: "alpha", Destination: "beta", Depart: departAt(7, 0)}
	got, err := EarliestArrival(q)
	if err != nil {
		t.Fatalf("EarliestArrival: %v", err)
	}
	wantArrival(t, got, 13, 0, 2)
	assertItineraryIsFeasible(t, f, got, q.Depart)
	assertAgreesWithExhaustiveSearch(t, q)
}

// TestTheMinimumTransferTimeIsPaid is the third named case. The early
// connection is refused because thirty minutes is what changing vessels takes
// at that port, and the answer is the later one.
func TestTheMinimumTransferTimeIsPaid(t *testing.T) {
	t.Parallel()

	f := fixtureFeed(t, allYear("svc-all"),
		fixtureTrip("trip-a", "svc-all", call("alpha", clock(8, 0)), call("hub", clock(10, 0))),
		// Twenty minutes after the first vessel berths, which is less than the
		// thirty this port needs.
		fixtureTrip("trip-early", "svc-all", call("hub", clock(10, 20)), call("beta", clock(12, 20))),
		fixtureTrip("trip-late", "svc-all", call("hub", clock(11, 0)), call("beta", clock(13, 0))),
	)
	q := Query{Feed: f, Origin: "alpha", Destination: "beta", Depart: departAt(7, 0)}
	got, err := EarliestArrival(q)
	if err != nil {
		t.Fatalf("EarliestArrival: %v", err)
	}
	wantArrival(t, got, 13, 0, 2)
	assertDoesNotUse(t, got, "trip-early")
	assertItineraryIsFeasible(t, f, got, q.Depart)
	assertAgreesWithExhaustiveSearch(t, q)
}

// TestALegInSeasonAndOutOfSeason is the fourth named case. The same query, the
// same ports and the same sailing; only the date moves.
func TestALegInSeasonAndOutOfSeason(t *testing.T) {
	t.Parallel()

	july := []ferry.Calendar{{
		ServiceID: "svc-summer", Days: everyDay(),
		Start: ferry.Date{Year: 2026, Month: 7, Day: 1},
		End:   ferry.Date{Year: 2026, Month: 7, Day: 31},
	}}
	f := fixtureFeed(t, july,
		fixtureTrip("trip-a", "svc-summer", call("alpha", clock(8, 0)), call("beta", clock(10, 0))),
	)

	inSeason := Query{
		Feed: f, Origin: "alpha", Destination: "beta",
		Depart: time.Date(2026, time.July, 15, 7, 0, 0, 0, time.UTC),
	}
	got, err := EarliestArrival(inSeason)
	if err != nil {
		t.Fatalf("in season: %v", err)
	}
	if want := time.Date(2026, time.July, 15, 10, 0, 0, 0, time.UTC); !got.Arrive.Equal(want) {
		t.Fatalf("in season arrived %v, want %v", got.Arrive, want)
	}
	assertAgreesWithExhaustiveSearch(t, inSeason)

	outOfSeason := inSeason
	outOfSeason.Depart = time.Date(2026, time.October, 15, 7, 0, 0, 0, time.UTC)
	if _, err := EarliestArrival(outOfSeason); !errors.Is(err, ErrNoConnection) {
		t.Fatalf("out of season got %v, want ErrNoConnection", err)
	}
	assertAgreesWithExhaustiveSearch(t, outOfSeason)
}

// TestStayingAboardDoesNotBuyAFreeTransfer is the regression the graph's own
// comment names, planted exactly as described.
//
// THE ARRANGEMENT: trip-through calls at the hub at 10:00, sits there ten
// minutes, and carries on to Delta. trip-early leaves the hub for Beta at
// 10:15. The hub needs thirty minutes to change vessels, so a traveler off
// trip-through is not ready until 10:30 and cannot make it. trip-late leaves at
// 11:00 and can be made.
//
// If departure nodes were links in the per-port wait chain, staying aboard
// through the hub would land the traveler in that chain for free, and from
// there trip-early is simply the next node along — a connection made with no
// transfer time at all. Nothing would crash and every itinerary would still be
// well formed. The only symptom is an answer that is forty-five minutes too
// good, which is why this is asserted rather than reasoned about.
func TestStayingAboardDoesNotBuyAFreeTransfer(t *testing.T) {
	t.Parallel()

	f := fixtureFeed(t, allYear("svc-all"),
		fixtureTrip("trip-through", "svc-all",
			call("alpha", clock(8, 0)),
			dwell("hub", clock(10, 0), clock(10, 10)),
			call("delta", clock(12, 0)),
		),
		fixtureTrip("trip-early", "svc-all", call("hub", clock(10, 15)), call("beta", clock(12, 15))),
		fixtureTrip("trip-late", "svc-all", call("hub", clock(11, 0)), call("beta", clock(13, 0))),
	)
	q := Query{Feed: f, Origin: "alpha", Destination: "beta", Depart: departAt(7, 0)}
	got, err := EarliestArrival(q)
	if err != nil {
		t.Fatalf("EarliestArrival: %v", err)
	}
	assertDoesNotUse(t, got, "trip-early")
	wantArrival(t, got, 13, 0, 2)
	assertItineraryIsFeasible(t, f, got, q.Depart)
	assertAgreesWithExhaustiveSearch(t, q)
}

// TestStayingAboardIsStillFreeWhenItShouldBe is the other direction of the same
// rule, and without it the test above could be satisfied by a solver that
// simply charged the transfer to everybody. A traveler who never gets off pays
// nothing, so a sailing that calls at the hub and continues to Delta reaches
// Delta at its own scheduled time and counts as ONE leg, not two.
func TestStayingAboardIsStillFreeWhenItShouldBe(t *testing.T) {
	t.Parallel()

	f := fixtureFeed(t, allYear("svc-all"),
		fixtureTrip("trip-through", "svc-all",
			call("alpha", clock(8, 0)),
			dwell("hub", clock(10, 0), clock(10, 10)),
			call("delta", clock(12, 0)),
		),
	)
	q := Query{Feed: f, Origin: "alpha", Destination: "delta", Depart: departAt(7, 0)}
	got, err := EarliestArrival(q)
	if err != nil {
		t.Fatalf("EarliestArrival: %v", err)
	}
	wantArrival(t, got, 12, 0, 1)
	assertItineraryIsFeasible(t, f, got, q.Depart)
	assertAgreesWithExhaustiveSearch(t, q)
}

// TestFewerLegsBreaksATieOnArrivalTime is what the second cost component is
// for. Two ways to reach Beta at 13:00 exist — one vessel straight through, or
// a change at the hub — and the answer is the one with fewer legs. With arrival
// time alone the search could return either and nothing would be wrong.
func TestFewerLegsBreaksATieOnArrivalTime(t *testing.T) {
	t.Parallel()

	f := fixtureFeed(t, allYear("svc-all"),
		fixtureTrip("trip-direct", "svc-all", call("alpha", clock(8, 0)), call("beta", clock(13, 0))),
		fixtureTrip("trip-first", "svc-all", call("alpha", clock(8, 30)), call("hub", clock(10, 0))),
		fixtureTrip("trip-second", "svc-all", call("hub", clock(11, 0)), call("beta", clock(13, 0))),
	)
	q := Query{Feed: f, Origin: "alpha", Destination: "beta", Depart: departAt(7, 0)}
	got, err := EarliestArrival(q)
	if err != nil {
		t.Fatalf("EarliestArrival: %v", err)
	}
	wantArrival(t, got, 13, 0, 1)
	if got.Legs[0].TripID != "trip-direct" {
		t.Fatalf("the single-leg answer used %q, want trip-direct", string(got.Legs[0].TripID))
	}
	assertAgreesWithExhaustiveSearch(t, q)
}

// TestAnOvernightSailingIsReachable proves the look-back is not decoration. The
// sailing belongs to the previous service day and is still at sea when the
// query is asked; a search that only expanded the query's own date would report
// no connection with complete confidence.
func TestAnOvernightSailingIsReachable(t *testing.T) {
	t.Parallel()

	f := fixtureFeed(t, allYear("svc-all"),
		// Departs 23:00 on its service day and berths at 02:00 the next
		// morning, written as 26:00 against the day it sailed.
		fixtureTrip("trip-night", "svc-all", call("alpha", clock(23, 0)), call("beta", clock(26, 0))),
	)
	q := Query{
		Feed: f, Origin: "alpha", Destination: "beta",
		Depart: time.Date(2026, time.August, 14, 22, 0, 0, 0, time.UTC),
	}
	got, err := EarliestArrival(q)
	if err != nil {
		t.Fatalf("EarliestArrival: %v", err)
	}
	want := time.Date(2026, time.August, 15, 2, 0, 0, 0, time.UTC)
	if !got.Arrive.Equal(want) {
		t.Fatalf("arrived %v, want %v", got.Arrive, want)
	}
	if got.Legs[0].ServiceDate != (ferry.Date{Year: 2026, Month: 8, Day: 14}) {
		t.Fatalf("the leg reports service date %s, want 2026-08-14: the sailing belongs to the day it left",
			got.Legs[0].ServiceDate)
	}
	assertAgreesWithExhaustiveSearch(t, q)
}

// TestPriorServiceDaysCoversTheStopTimeCeiling is the guard the constant's own
// comment promises. Raising the ceiling on how far a stop time may run past its
// service day without widening the look-back would silently lose sailings, and
// the symptom would be a confident "no connection".
func TestPriorServiceDaysCoversTheStopTimeCeiling(t *testing.T) {
	t.Parallel()

	if PriorServiceDays*config.SecondsPerDay < ferry.MaxStopTimeSeconds {
		t.Fatalf("the look-back is %d days but a stop time may run %d seconds past its service day",
			PriorServiceDays, ferry.MaxStopTimeSeconds)
	}
}

func TestAJourneyToWhereYouAlreadyAreIsEmpty(t *testing.T) {
	t.Parallel()

	f := fixtureFeed(t, allYear("svc-all"),
		fixtureTrip("trip-a", "svc-all", call("alpha", clock(8, 0)), call("beta", clock(10, 0))),
	)
	q := Query{Feed: f, Origin: "alpha", Destination: "alpha", Depart: departAt(7, 0)}
	got, err := EarliestArrival(q)
	if err != nil {
		t.Fatalf("EarliestArrival: %v", err)
	}
	if len(got.Legs) != 0 {
		t.Fatalf("a journey to the port you are standing in produced %d legs", len(got.Legs))
	}
	if !got.Arrive.Equal(q.Depart) || !got.Depart.Equal(q.Depart) {
		t.Fatalf("departed %v and arrived %v, want both to be %v", got.Depart, got.Arrive, q.Depart)
	}
	assertAgreesWithExhaustiveSearch(t, q)
}

func TestQueryRefusals(t *testing.T) {
	t.Parallel()

	good := fixtureFeed(t, allYear("svc-all"),
		fixtureTrip("trip-a", "svc-all", call("alpha", clock(8, 0)), call("beta", clock(10, 0))),
	)

	cases := []struct {
		name string
		q    Query
		want error
	}{
		{
			name: "no schedule at all",
			q:    Query{Origin: "alpha", Destination: "beta", Depart: departAt(7, 0)},
			want: ErrNoFeed,
		},
		{
			name: "an origin the schedule does not declare",
			q:    Query{Feed: good, Origin: "nowhere", Destination: "beta", Depart: departAt(7, 0)},
			want: ErrUnknownPort,
		},
		{
			name: "a destination the schedule does not declare",
			q:    Query{Feed: good, Origin: "alpha", Destination: "nowhere", Depart: departAt(7, 0)},
			want: ErrUnknownPort,
		},
		{
			name: "a negative horizon",
			q:    Query{Feed: good, Origin: "alpha", Destination: "beta", Depart: departAt(7, 0), HorizonDays: -1},
			want: ErrHorizon,
		},
		{
			name: "a horizon past the ceiling",
			q: Query{Feed: good, Origin: "alpha", Destination: "beta", Depart: departAt(7, 0),
				HorizonDays: config.MaxSearchHorizonDays + 1},
			want: ErrHorizon,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if _, err := EarliestArrival(c.q); !errors.Is(err, c.want) {
				t.Fatalf("EarliestArrival got %v, want %v", err, c.want)
			}
			if _, err := BruteEarliestArrival(c.q); !errors.Is(err, c.want) {
				t.Fatalf("BruteEarliestArrival got %v, want %v", err, c.want)
			}
		})
	}
}

// TestTheHorizonDecidesWhetherAWeeklySailingExists is the concrete reason
// ErrNoConnection has to be read relative to the horizon. Nothing about the
// schedule changes between the two halves of this test.
func TestTheHorizonDecidesWhetherAWeeklySailingExists(t *testing.T) {
	t.Parallel()

	// 2026-08-15 is a Saturday, so a Wednesday-only sailing is four days out.
	weekly := []ferry.Calendar{{
		ServiceID: "svc-weekly", Days: ferry.Weekdays(0).With(time.Wednesday),
		Start: ferry.Date{Year: 2026, Month: 1, Day: 1},
		End:   ferry.Date{Year: 2026, Month: 12, Day: 31},
	}}
	f := fixtureFeed(t, weekly,
		fixtureTrip("trip-weekly", "svc-weekly", call("alpha", clock(8, 0)), call("beta", clock(10, 0))),
	)

	narrow := Query{Feed: f, Origin: "alpha", Destination: "beta", Depart: departAt(7, 0), HorizonDays: 1}
	if _, err := EarliestArrival(narrow); !errors.Is(err, ErrNoConnection) {
		t.Fatalf("inside a one-day horizon got %v, want ErrNoConnection", err)
	}

	wide := narrow
	wide.HorizonDays = config.DefaultSearchHorizonDays
	got, err := EarliestArrival(wide)
	if err != nil {
		t.Fatalf("inside the default horizon: %v", err)
	}
	want := time.Date(2026, time.August, 19, 10, 0, 0, 0, time.UTC)
	if !got.Arrive.Equal(want) {
		t.Fatalf("arrived %v, want %v", got.Arrive, want)
	}
	assertAgreesWithExhaustiveSearch(t, wide)

	// The ceiling itself is accepted; only past it is refused.
	atCeiling := narrow
	atCeiling.HorizonDays = config.MaxSearchHorizonDays
	if _, err := EarliestArrival(atCeiling); err != nil {
		t.Fatalf("a horizon exactly at the ceiling was refused: %v", err)
	}
}

// TestTheExhaustiveSearchRefusesRatherThanSampling covers the verifier's work
// ceiling. It is driven through the seam with a ceiling of one and of two, so
// both the refusal itself and its propagation back out of the recursion are
// exercised — rather than by building an instance large enough to reach two
// million, which is the kind of fixture that ends up on an exclusion list.
func TestTheExhaustiveSearchRefusesRatherThanSampling(t *testing.T) {
	t.Parallel()

	f := fixtureFeed(t, allYear("svc-all"),
		fixtureTrip("trip-a", "svc-all", call("alpha", clock(8, 0)), call("hub", clock(10, 0))),
		fixtureTrip("trip-b", "svc-all", call("hub", clock(11, 0)), call("beta", clock(13, 0))),
	)
	q := Query{Feed: f, Origin: "alpha", Destination: "beta", Depart: departAt(7, 0)}

	for _, ceiling := range []int{0, 1} {
		if _, err := bruteWithCeiling(q, ceiling); !errors.Is(err, ErrTooManyStates) {
			t.Fatalf("at a ceiling of %d got %v, want ErrTooManyStates", ceiling, err)
		}
	}
	if _, err := bruteWithCeiling(q, MaxExhaustiveStates); err != nil {
		t.Fatalf("at the real ceiling the same query failed: %v", err)
	}
}

// TestTheSameQueryGivesTheSameItineraryEveryTime is not a formality. The graph
// is assembled from map iteration, and the search's last tie-break is node
// index, so unstable node numbering would produce different itineraries of
// identical cost between runs — every one of them correct, and none of them
// reproducible. A reader who is told the answer must be able to see it again.
func TestTheSameQueryGivesTheSameItineraryEveryTime(t *testing.T) {
	t.Parallel()

	f := fixtureFeed(t, allYear("svc-all"),
		fixtureTrip("trip-a", "svc-all", call("alpha", clock(8, 0)), call("hub", clock(10, 0))),
		fixtureTrip("trip-b", "svc-all", call("alpha", clock(8, 0)), call("hub", clock(10, 0))),
		fixtureTrip("trip-c", "svc-all", call("hub", clock(11, 0)), call("beta", clock(13, 0))),
		fixtureTrip("trip-d", "svc-all", call("hub", clock(11, 0)), call("beta", clock(13, 0))),
	)
	q := Query{Feed: f, Origin: "alpha", Destination: "beta", Depart: departAt(7, 0)}

	first, err := EarliestArrival(q)
	if err != nil {
		t.Fatalf("EarliestArrival: %v", err)
	}
	for i := 0; i < 50; i++ {
		got, err := EarliestArrival(q)
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if len(got.Legs) != len(first.Legs) {
			t.Fatalf("run %d returned %d legs, first run returned %d", i, len(got.Legs), len(first.Legs))
		}
		for j := range got.Legs {
			if got.Legs[j] != first.Legs[j] {
				t.Fatalf("run %d leg %d is %+v, first run had %+v", i, j, got.Legs[j], first.Legs[j])
			}
		}
	}
}

func assertDoesNotUse(t *testing.T, it Itinerary, trip vocab.ID) {
	t.Helper()
	if len(it.Legs) == 0 {
		t.Fatal("the itinerary is empty, so an absence assertion over its legs would pass vacuously")
	}
	for _, leg := range it.Legs {
		if leg.TripID == trip {
			t.Fatalf("the itinerary used %q: %+v", string(trip), it.Legs)
		}
	}
}
