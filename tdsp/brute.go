package tdsp

import "github.com/pigfox/aegean-solver/vocab"

// bruteSearch enumerates itineraries exhaustively and keeps the best one.
//
// It exists to be GROUND TRUTH for EarliestArrival, and it is written to be
// obviously right rather than fast: it tries every sailing the traveler could
// board, every port they could step off at, and every continuation from there.
// It shares prepare and itinerary with the solver, so the two are answering the
// same question over the same sailings and a disagreement between them can only
// be about the search.
type bruteSearch struct {
	p prepared
	// pairs counts every (board, alight) combination considered, and ceiling is
	// where the enumeration refuses rather than continuing. The ceiling is a
	// FIELD rather than the constant read inline so that the refusal path can
	// be driven directly from a test with a ceiling of one or two, instead of
	// only by building an instance large enough to reach two million. A branch
	// that can only be exercised by an expensive fixture is a branch that ends
	// up on an exclusion list.
	pairs   int
	ceiling int

	found      bool
	bestArrive int64
	bestLegs   int
	best       []rawLeg
}

// walk explores every itinerary that continues from a port at an instant.
//
// # Termination is structural, not a depth limit
//
// A leg can only be boarded at or after availableFrom, and its arrival is
// strictly later than its departure because ferry.Trip.Validate refuses a leg
// that takes no time. So availableFrom strictly increases down the recursion,
// only finitely many departures lie at or after any instant, and the expansion
// window is finite. There is no depth cap because none is needed; the ceiling
// that does exist bounds WORK, not depth, and refuses outright rather than
// truncating.
//
// # The single prune, and why it does not cost ground truth
//
// Reaching the destination ends the itinerary rather than continuing through
// it. Any journey that arrives, carries on and comes back arrives the second
// time strictly later AND with more legs, so it loses on both components of the
// cost and can never be the answer. That is a dominance argument, not a
// heuristic — nothing that could win is discarded. It is the only thing not
// enumerated.
func (b *bruteSearch) walk(port vocab.ID, availableFrom int64, path []rawLeg) error {
	for ii := range b.p.insts {
		inst := &b.p.insts[ii]
		trip := &b.p.feed.Trips[inst.tripIdx]
		last := len(trip.StopTimes) - 1
		for board := 0; board < last; board++ {
			if trip.StopTimes[board].PortID != port || inst.depart[board] < availableFrom {
				continue
			}
			for alight := board + 1; alight <= last; alight++ {
				b.pairs++
				if b.pairs > b.ceiling {
					return ErrTooManyStates
				}
				toPort := trip.StopTimes[alight].PortID
				arrive := inst.arrive[alight]
				next := make([]rawLeg, len(path), len(path)+1)
				copy(next, path)
				next = append(next, rawLeg{inst: ii, board: board, alight: alight})
				if toPort == b.p.dest {
					b.consider(arrive, next)
					continue
				}
				if err := b.walk(toPort, arrive+int64(b.p.minTransfer[toPort]), next); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// consider keeps a candidate if it beats the incumbent on the same
// lexicographic cost the solver minimizes: arrival instant first, leg count
// second.
func (b *bruteSearch) consider(arrive int64, path []rawLeg) {
	better := !b.found ||
		arrive < b.bestArrive ||
		(arrive == b.bestArrive && len(path) < b.bestLegs)
	if !better {
		return
	}
	b.found = true
	b.bestArrive = arrive
	b.bestLegs = len(path)
	b.best = path
}

// BruteEarliestArrival answers the same query as EarliestArrival by exhaustive
// enumeration.
//
// IT IS A VERIFIER AND NOT AN ALTERNATIVE SOLVER. On anything larger than a toy
// schedule it is far too slow to use, and above MaxExhaustiveStates it refuses
// rather than sampling — a search that quietly stopped being exhaustive would
// still agree with the solver most of the time, and that agreement would mean
// nothing.
//
// WHAT AGREEMENT BETWEEN THE TWO DOES AND DOES NOT ESTABLISH: they are compared
// on the COST PAIR, arrival instant and leg count, not on the itinerary itself.
// Where several distinct journeys share the same cost, the two are free to
// return different ones and both are correct. Asserting identical leg lists
// would be asserting that two different tie-breaking rules coincide, which is a
// claim about the implementations rather than about the answer.
func BruteEarliestArrival(q Query) (Itinerary, error) {
	return bruteWithCeiling(q, MaxExhaustiveStates)
}

// bruteWithCeiling is BruteEarliestArrival with the work ceiling supplied.
//
// Splitting it out is what lets the refusal be tested at a ceiling of one
// rather than at two million. The exported entry point supplies the real
// ceiling and nothing else can, so the constant is not weakened by the seam.
func bruteWithCeiling(q Query, ceiling int) (Itinerary, error) {
	p, err := prepare(q)
	if err != nil {
		return Itinerary{}, err
	}
	if p.origin == p.dest {
		return p.itinerary(nil), nil
	}
	b := &bruteSearch{p: p, ceiling: ceiling}
	if err := b.walk(p.origin, p.departAt, nil); err != nil {
		return Itinerary{}, err
	}
	if !b.found {
		return Itinerary{}, ErrNoConnection
	}
	return p.itinerary(b.best), nil
}
