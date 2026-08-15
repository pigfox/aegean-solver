// Package tdsp answers one question: leaving a port at a given moment, what is
// the earliest a traveler can arrive at another one, and by which sailings.
//
// # Why the straight-line model from phase one is not enough
//
// The placement work in the root package measures distance as a great circle,
// which is the right model for siting a depot and the wrong one for travel. A
// vessel cannot sail through an island, and — far more decisive in this basin —
// a route served twice a week has a WAITING time that dwarfs its sailing time.
// A traveler who misses Tuesday's boat does not arrive four hours late; they
// arrive on Friday. No distance metric can express that, because the cost of an
// edge here depends on when you arrive at it.
//
// # The time-expanded model
//
// Rather than give each edge a travel-time function, every departure and every
// arrival becomes its own node at its own absolute instant, and an edge is a
// concrete thing a traveler can do: ride this sailing, stay aboard through this
// call, step ashore, wait, board. Waiting for the boat stops being a function
// and becomes a walk along a chain of nodes at one port.
//
// # FIFO does not hold here, and does not need to
//
// A time-dependent network is called FIFO, or non-overtaking, when leaving
// later can never get you there earlier. A SCHEDULE GRAPH VIOLATES THAT BY
// CONSTRUCTION: a fast catamaran departing at ten can berth before a
// conventional ferry that sailed at nine, and any basin with two vessel classes
// on one leg will show it. It is stated here rather than assumed away because
// the assumption is common and quiet, and a solver that needed it would be
// wrong on exactly the instances this module exists for.
//
// It is not needed. FIFO is what makes Dijkstra safe on the COMPRESSED
// formulation, where an arc carries a travel-time function and the search
// evaluates that function at whatever time it happens to reach the arc. Nothing
// is compressed here. Every edge weight is a non-negative elapsed time between
// two fixed instants, so the search key never decreases along an edge, which is
// the only property Dijkstra actually requires. Overtaking is representable and
// is simply two different nodes.
//
// # Why the cost is a pair
//
// Earliest arrival ALONE is degenerate on this graph. Every edge weight is the
// difference of its endpoints' instants, so every path reaching a node costs
// exactly the same — the node's own time minus the start — and the search
// collapses into reachability in time order. It would still be correct and it
// would still be Dijkstra; it just would not be doing any work, and it could
// never prefer one way of arriving at ten o'clock over another.
//
// So the cost is lexicographic: arrival instant first, then NUMBER OF LEGS.
// Only boarding adds a leg; staying aboard through an intermediate call does
// not. Both components are non-negative and additive, so the search is
// unchanged in its guarantees, and the answer becomes a single well-defined
// itinerary rather than an arbitrary one among many.
package tdsp

import (
	"errors"
	"time"

	"github.com/pigfox/aegean-solver/internal/config"
	"github.com/pigfox/aegean-solver/internal/ferry"
	"github.com/pigfox/aegean-solver/internal/vocab"
)

// The errors a caller can act on.
var (
	// ErrNoFeed is a query with no schedule attached.
	ErrNoFeed = errors.New("tdsp: query has no feed")
	// ErrUnknownPort is an origin or destination the feed does not declare.
	ErrUnknownPort = errors.New("tdsp: unknown port")
	// ErrHorizon is a search horizon that is negative or past the ceiling.
	ErrHorizon = errors.New("tdsp: search horizon out of range")
	// ErrNoConnection means no itinerary exists inside the horizon. It is a
	// real answer about the schedule, not a failure — see the note on Query
	// about how narrowly it should be read.
	ErrNoConnection = errors.New("tdsp: no connection within the horizon")
	// ErrTooManyStates refuses an exhaustive enumeration that would not finish.
	// Only the verifier returns it; the solver has no such ceiling.
	ErrTooManyStates = errors.New("tdsp: too many states for an exhaustive search")
)

// Query is one earliest-arrival question.
type Query struct {
	// Feed is the schedule to search. Required.
	Feed *ferry.Feed
	// Origin and Destination are canonical port IDs. Resolving a typed name
	// onto one is the caller's job, through vocab.AliasIndex.
	Origin      vocab.ID
	Destination vocab.ID
	// Depart is the instant the traveler is first available at Origin.
	Depart time.Time
	// HorizonDays is how many days past Depart to search. Zero means
	// config.DefaultSearchHorizonDays.
	//
	// ErrNoConnection IS RELATIVE TO THIS NUMBER and should be read that way. A
	// weekly route missed by a day is unreachable inside a horizon of one and
	// reachable inside a horizon of seven, and the solver cannot tell the
	// difference between "these ports are not connected" and "you did not look
	// far enough". The default is a week for that reason.
	HorizonDays int
}

// Leg is one continuous ride on one sailing, from the port where the traveler
// boarded to the port where they stepped off. A sailing that calls at three
// ports in between is still ONE leg — staying aboard is not a transfer, and
// counting it as one would make the cost function prefer routes for the wrong
// reason.
type Leg struct {
	TripID   vocab.ID
	RouteID  vocab.ID
	VesselID vocab.ID
	// ServiceDate is the service day the sailing belongs to, which is not
	// always the calendar date of its departure — see ferry.MaxStopTimeSeconds.
	ServiceDate ferry.Date
	FromPort    vocab.ID
	ToPort      vocab.ID
	Depart      time.Time
	Arrive      time.Time
}

// Itinerary is the answer: when the traveler set out, when they arrive, and
// what they ride.
//
// A journey from a port to itself is the empty itinerary arriving at the
// departure instant. That is not a special case bolted on; it is what the model
// says, and returning ErrNoConnection for it would be wrong.
type Itinerary struct {
	Depart time.Time
	Arrive time.Time
	Legs   []Leg
}

// instance is one trip on one service day, with every stop time resolved to an
// absolute instant.
//
// The resolution happens HERE, once, rather than inside the search. A stop time
// is seconds from the start of a service day and a service day starts at a
// different instant depending on the zone and on whether a clock shifted that
// week; doing that arithmetic in the inner loop would mean doing it thousands
// of times and getting it wrong in one of them.
type instance struct {
	tripIdx int
	date    ferry.Date
	arrive  []int64
	depart  []int64
}

// prepared is the shared setup both the solver and the verifier run from.
//
// They share it on purpose. If each built its own view of which sailings exist,
// an agreement test between them would be comparing two answers to two slightly
// different questions, and would pass or fail for reasons that had nothing to
// do with the search.
type prepared struct {
	feed        *ferry.Feed
	loc         *time.Location
	insts       []instance
	origin      vocab.ID
	dest        vocab.ID
	departAt    int64
	minTransfer map[vocab.ID]int
}

// prepare validates the query and expands the schedule into dated instances.
func prepare(q Query) (prepared, error) {
	if q.Feed == nil {
		return prepared{}, ErrNoFeed
	}
	horizon := q.HorizonDays
	if horizon == 0 {
		horizon = config.DefaultSearchHorizonDays
	}
	if horizon < 0 || horizon > config.MaxSearchHorizonDays {
		return prepared{}, ErrHorizon
	}
	if _, ok := q.Feed.PortByID(q.Origin); !ok {
		return prepared{}, ErrUnknownPort
	}
	if _, ok := q.Feed.PortByID(q.Destination); !ok {
		return prepared{}, ErrUnknownPort
	}

	loc := q.Feed.Loc()
	departAt := q.Depart.Unix()
	minTransfer := make(map[vocab.ID]int, len(q.Feed.Ports))
	for _, p := range q.Feed.Ports {
		minTransfer[p.ID] = p.MinTransfer()
	}

	queryDate := ferry.DateIn(q.Depart, loc)
	first := queryDate.AddDays(-PriorServiceDays)
	last := queryDate.AddDays(horizon)

	return prepared{
		feed:        q.Feed,
		loc:         loc,
		insts:       expandInstances(q.Feed, first, last, departAt),
		origin:      q.Origin,
		dest:        q.Destination,
		departAt:    departAt,
		minTransfer: minTransfer,
	}, nil
}

// expandInstances turns every trip that runs on a service day in the window
// into a dated instance with absolute times.
//
// A sailing whose LAST arrival is already in the past at departAt is dropped.
// That is not an optimization heuristic — no part of any such sailing can be
// boarded after departAt, because a trip's times only advance, so it cannot
// appear in any itinerary. Dropping it keeps the exhaustive verifier's search
// space finite in the useful sense rather than merely bounded.
//
// The iteration order is date, then trip index, and both are deterministic. The
// search's tie-breaking eventually reaches node numbering, so a schedule that
// produced different node numbers on different runs would produce different
// itineraries of identical cost, and nothing would look wrong.
func expandInstances(f *ferry.Feed, first, last ferry.Date, departAt int64) []instance {
	var out []instance
	for d := first; d.Compare(last) <= 0; d = d.AddDays(1) {
		dayStart := d.StartOfServiceDay(f.Loc()).Unix()
		for ti := range f.Trips {
			t := &f.Trips[ti]
			if !f.ServiceActiveOn(t.ServiceID, d) {
				continue
			}
			inst := instance{
				tripIdx: ti,
				date:    d,
				arrive:  make([]int64, len(t.StopTimes)),
				depart:  make([]int64, len(t.StopTimes)),
			}
			for i, st := range t.StopTimes {
				inst.arrive[i] = dayStart + int64(st.ArrivalSeconds)
				inst.depart[i] = dayStart + int64(st.DepartureSeconds)
			}
			if inst.arrive[len(inst.arrive)-1] < departAt {
				continue
			}
			out = append(out, inst)
		}
	}
	return out
}

// rawLeg is a leg as the search and the verifier both find it, before it is
// dressed up into the exported form: which instance, boarded at which stop
// index, stepped off at which.
type rawLeg struct {
	inst   int
	board  int
	alight int
}

// itinerary converts raw legs into the exported answer.
//
// BOTH the solver and the verifier go through it, so a disagreement between
// them can never be an artifact of two different renderings of the same
// journey.
func (p prepared) itinerary(raw []rawLeg) Itinerary {
	out := Itinerary{
		Depart: time.Unix(p.departAt, 0).In(p.loc),
		Arrive: time.Unix(p.departAt, 0).In(p.loc),
	}
	for _, r := range raw {
		inst := p.insts[r.inst]
		trip := &p.feed.Trips[inst.tripIdx]
		out.Legs = append(out.Legs, Leg{
			TripID:      trip.ID,
			RouteID:     trip.RouteID,
			VesselID:    trip.VesselID,
			ServiceDate: inst.date,
			FromPort:    trip.StopTimes[r.board].PortID,
			ToPort:      trip.StopTimes[r.alight].PortID,
			Depart:      time.Unix(inst.depart[r.board], 0).In(p.loc),
			Arrive:      time.Unix(inst.arrive[r.alight], 0).In(p.loc),
		})
	}
	if n := len(out.Legs); n > 0 {
		out.Arrive = out.Legs[n-1].Arrive
	}
	return out
}
