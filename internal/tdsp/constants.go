package tdsp

import (
	"github.com/pigfox/aegean-solver/internal/config"
	"github.com/pigfox/aegean-solver/internal/ferry"
)

// PriorServiceDays is how far BEFORE the query's own date the expansion reaches
// for service days.
//
// It is DERIVED rather than chosen, and the derivation is the reason it is not
// simply 1. A stop time is measured from the start of its service day and may
// run past midnight — that is how an overnight sailing is written — so a vessel
// that is at sea when a query is asked may belong to a service day already
// finished on the calendar. Missing it does not produce a wrong number; it
// produces "no connection exists", stated with total confidence, for the exact
// journeys an overnight route is for.
//
// ferry.MaxStopTimeSeconds is the ceiling on how far past its own service day a
// stop time may sit, so looking back that many whole days is sufficient by
// construction. TestPriorServiceDaysCoversTheStopTimeCeiling holds the two
// together, so raising the ceiling without widening the look-back fails the
// build instead of quietly losing sailings.
const PriorServiceDays = ferry.MaxStopTimeSeconds / config.SecondsPerDay

// MaxExhaustiveStates bounds the verifier, not the solver.
//
// The exhaustive search enumerates every itinerary rather than the best one, so
// its cost is combinatorial in the number of trip instances the horizon admits.
// It exists to be GROUND TRUTH for the Dijkstra result on small instances, so
// refusing outright above this ceiling is correct: an exhaustive search that
// quietly became a sampled one would agree with the solver for reasons nobody
// could check, which is worse than no verification at all. This is the same
// reasoning as the root package's MaxBruteCombinations.
const MaxExhaustiveStates = 2_000_000

// noNode marks an absent predecessor in the search's parent array.
const noNode = -1
