package aegean

import (
	"math"
	"runtime"
	"sort"
	"sync"
)

// Placement is a p-median answer.
type Placement struct {
	// Facilities are site indices, sorted ascending so two equal answers
	// compare equal.
	Facilities []int
	// Assignment maps every site index to the facility index serving it. The
	// value is an index INTO Facilities, not into the site slice, because that
	// is what a renderer needs to draw a spoke.
	Assignment []int
	// Cost is the weighted sum of distances, in weight·kilometres.
	Cost float64
	// Passes is how many improving interchange passes ran across all restarts.
	Passes int
	// Swaps is how many improving swaps were applied.
	Swaps int
	// Restarts is how many starting sets were tried.
	Restarts int
}

// clone returns a deep copy, so a caller holding a result cannot be surprised
// by a later solve reusing a buffer.
func (p Placement) clone() Placement {
	out := p
	out.Facilities = append([]int(nil), p.Facilities...)
	out.Assignment = append([]int(nil), p.Assignment...)
	return out
}

// PMedian places p facilities to minimize the weighted sum of distances from
// every site to its nearest facility.
//
// TEITZ-BART INTERCHANGE, RESTARTED. One pass evaluates every swap of a chosen
// site for an unchosen one and applies the single best improvement; passes run
// until none improves. That descends to a local optimum, which for this problem
// is usually good and occasionally poor, so the whole descent is repeated from
// Restarts different random starting sets and the best result wins.
//
// THE SWAP NEIGHBORHOOD IS FANNED ACROSS WORKERS AND FANNED BACK IN ON BEST
// GAIN. Each pass is p·(n-p) independent evaluations of a full reassignment,
// which is the entire cost of the algorithm and is embarrassingly parallel. The
// fan-in takes the best gain deterministically — ties break on the lowest
// (out, in) index pair, never on which goroutine finished first — so the result
// does not depend on scheduling. That is what lets the seed alone reproduce a
// run.
func PMedian(sites []Site, p int, cfg Config) (Placement, error) {
	if err := validate(sites, p); err != nil {
		return Placement{}, err
	}
	cfg = cfg.withDefaults()
	m := NewMatrix(sites)
	rng := cfg.rng()

	best := Placement{Cost: math.Inf(1)}
	for r := range cfg.Restarts {
		start := randomChoice(rng, len(sites), p)
		got := interchange(sites, m, start, cfg.MaxPasses)
		got.Restarts = r + 1
		if got.Cost < best.Cost {
			best = got
		} else {
			// The counters accumulate across restarts even when the result is
			// discarded: they describe the WORK done, not the winning descent.
			best.Passes += got.Passes
			best.Swaps += got.Swaps
			best.Restarts = r + 1
		}
	}
	return best.clone(), nil
}

// interchange runs one descent from a starting set to a local optimum.
func interchange(sites []Site, m *Matrix, chosen []int, maxPasses int) Placement {
	n := len(sites)
	cur := append([]int(nil), chosen...)
	sort.Ints(cur)
	cost := assignCost(sites, m, cur)

	passes, swaps := 0, 0
	for pass := 0; pass < maxPasses; pass++ {
		passes++
		inSet := make([]bool, n)
		for _, c := range cur {
			inSet[c] = true
		}

		bestGain, bestOut, bestIn := 0.0, -1, -1
		for _, cand := range evaluateSwaps(sites, m, cur, inSet, cost) {
			// STRICTLY GREATER, and the tie rule is the ordering of the
			// candidate list rather than a comparison here — evaluateSwaps
			// returns them in (out, in) order, so the first best wins and the
			// choice is independent of goroutine timing.
			if cand.gain > bestGain {
				bestGain, bestOut, bestIn = cand.gain, cand.out, cand.in
			}
		}
		if bestOut < 0 {
			break // a local optimum: no swap improves
		}

		for i, c := range cur {
			if c == bestOut {
				cur[i] = bestIn
				break
			}
		}
		sort.Ints(cur)
		cost -= bestGain
		swaps++
	}

	return Placement{
		Facilities: cur,
		Assignment: assignment(sites, m, cur),
		Cost:       assignCost(sites, m, cur),
		Passes:     passes,
		Swaps:      swaps,
	}
}

// swapResult is one evaluated interchange.
type swapResult struct {
	out, in int
	gain    float64
}

// evaluateSwaps scores every (chosen, unchosen) pair, in parallel, and returns
// them in a deterministic order.
//
// THE ORDER IS THE POINT. Results are written into a preallocated slice at an
// index derived from the pair, never appended, so the returned slice is in
// (out, in) order however the goroutines interleave. Appending under a mutex
// would be simpler and would make the tie-break depend on scheduling, which is
// exactly the nondeterminism the seed is supposed to remove.
func evaluateSwaps(sites []Site, m *Matrix, chosen []int, inSet []bool, base float64) []swapResult {
	n := len(sites)
	var candidates []int
	for i := range n {
		if !inSet[i] {
			candidates = append(candidates, i)
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	out := make([]swapResult, len(chosen)*len(candidates))
	workers := workerCount(len(chosen))

	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			trial := make([]int, len(chosen))
			for oi := w; oi < len(chosen); oi += workers {
				for ci, cand := range candidates {
					copy(trial, chosen)
					trial[oi] = cand
					cost := assignCost(sites, m, trial)
					out[oi*len(candidates)+ci] = swapResult{
						out: chosen[oi], in: cand, gain: base - cost,
					}
				}
			}
		}(w)
	}
	wg.Wait()
	return out
}

// workerCount is how many goroutines to fan a pass across: one per core, but
// never more than there is work for, and never fewer than one.
//
// The floor is the part worth naming. Fanning across zero workers would leave
// every result slot at its zero value and the pass would report that no swap
// improves anything — a silent wrong answer rather than a crash, which is the
// worse of the two. GOMAXPROCS is documented to return at least 1 and a solve
// always has at least one chosen facility, so no real call reaches the floor;
// it is here as a named contract that can be tested rather than an inline
// branch that could only be read.
func workerCount(tasks int) int {
	n := runtime.GOMAXPROCS(0)
	if n > tasks {
		n = tasks
	}
	if n < 1 {
		n = 1
	}
	return n
}

// assignCost is the objective: every site served by its nearest facility,
// weighted by demand.
func assignCost(sites []Site, m *Matrix, facilities []int) float64 {
	total := 0.0
	for i := range sites {
		if sites[i].Weight == 0 {
			continue
		}
		nearest := math.Inf(1)
		for _, f := range facilities {
			if d := m.At(i, f); d < nearest {
				nearest = d
			}
		}
		total += sites[i].Weight * nearest
	}
	return total
}

// assignment maps each site to the position in facilities that serves it.
func assignment(sites []Site, m *Matrix, facilities []int) []int {
	out := make([]int, len(sites))
	for i := range sites {
		best, bestIdx := math.Inf(1), 0
		for fi, f := range facilities {
			if d := m.At(i, f); d < best {
				best, bestIdx = d, fi
			}
		}
		out[i] = bestIdx
	}
	return out
}

// PMedianBrute returns the true optimum by enumerating every combination.
//
// It exists to MEASURE the heuristic rather than to be used. Nothing on a page
// calls it; TestInterchangeStaysNearTheOptimum does, at a size where exhaustive
// search is honest, and reports the gap.
func PMedianBrute(sites []Site, p int) (Placement, error) {
	if err := validate(sites, p); err != nil {
		return Placement{}, err
	}
	if combinations(len(sites), p) > MaxBruteCombinations {
		return Placement{}, ErrTooManyCombinations
	}
	m := NewMatrix(sites)

	best := Placement{Cost: math.Inf(1)}
	forEachCombination(len(sites), p, func(pick []int) {
		if c := assignCost(sites, m, pick); c < best.Cost {
			best.Facilities = append([]int(nil), pick...)
			best.Cost = c
		}
	})
	best.Assignment = assignment(sites, m, best.Facilities)
	return best, nil
}
