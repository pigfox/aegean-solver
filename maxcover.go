package aegean

import (
	"container/heap"
	"math"
)

// Coverage is a max-coverage answer.
type Coverage struct {
	// Facilities are site indices, in the order greedy chose them. The order is
	// meaningful here in a way it is not for p-median: it is the marginal
	// ranking, and a page can show the k-th facility's contribution.
	Facilities []int
	// Gains is the weight each facility added when it was chosen. Non-increasing
	// by submodularity, which TestGainsAreNonIncreasing asserts.
	Gains []float64
	// Covered is the total weight within RadiusKm of some facility.
	Covered float64
	// CoveredBy maps a site index to the position in Facilities covering it, or
	// -1 when the site is outside every radius.
	CoveredBy []int
	// RadiusKm is the radius the answer was computed at, carried so a renderer
	// draws the circle the solve actually used.
	RadiusKm float64
	// Evaluations counts marginal-gain computations. Lazy greedy exists to make
	// this much smaller than the naive k·n, and TestLazyGreedySkipsWork asserts
	// it does.
	Evaluations int
}

func (c Coverage) clone() Coverage {
	out := c
	out.Facilities = append([]int(nil), c.Facilities...)
	out.Gains = append([]float64(nil), c.Gains...)
	out.CoveredBy = append([]int(nil), c.CoveredBy...)
	return out
}

// MaxCover places k facilities to maximize the weight within RadiusKm of one.
//
// LAZY GREEDY (Minoux). Plain greedy recomputes every candidate's marginal gain
// on every round, which is k·n evaluations. Submodularity means a candidate's
// gain can only SHRINK as facilities are added, so a stale gain is an upper
// bound: pop the candidate with the largest stale gain, recompute only that
// one, and if it still tops the heap it is genuinely the best and can be taken
// without touching the rest. In practice that skips most of the work, and it
// returns exactly what plain greedy returns — this is an acceleration, not an
// approximation of an approximation.
//
// THE GUARANTEE. Because coverage is monotone and submodular, greedy is within
// (1 - 1/e) of the optimum. That is a worst-case bound and it is proven, but
// this package checks it anyway against an exhaustive optimum at small k; see
// TestGreedyMeetsTheOneMinusOneOverE.
func MaxCover(sites []Site, k int, radiusKm float64, cfg Config) (Coverage, error) {
	if err := validate(sites, k); err != nil {
		return Coverage{}, err
	}
	if radiusKm <= 0 {
		return Coverage{}, ErrRadius
	}
	_ = cfg.withDefaults() // no randomness in greedy; kept for a uniform signature

	m := NewMatrix(sites)
	n := len(sites)

	// within[f] is the set of sites facility f covers.
	within := make([][]int, n)
	for f := range n {
		for i := range n {
			if m.At(i, f) <= radiusKm {
				within[f] = append(within[f], i)
			}
		}
	}

	covered := make([]bool, n)
	out := Coverage{RadiusKm: radiusKm, CoveredBy: make([]int, n)}
	for i := range out.CoveredBy {
		out.CoveredBy[i] = -1
	}

	gain := func(f int) float64 {
		g := 0.0
		for _, i := range within[f] {
			if !covered[i] {
				g += sites[i].Weight
			}
		}
		return g
	}

	// Seed the heap with every candidate's true first-round gain.
	pq := make(gainQueue, 0, n)
	for f := range n {
		pq = append(pq, &gainItem{site: f, gain: gain(f), round: 0})
		out.Evaluations++
	}
	heap.Init(&pq)

	// THE LENGTH IS IN THE LOOP CONDITION rather than a nil check on the result.
	// popBest returns nil only for an empty queue, so testing the queue here is
	// the same guarantee stated where it can be seen, and it leaves no branch
	// below that no input can reach. Each round consumes exactly one candidate
	// and validate already refused a k above the site count, so the queue cannot
	// actually run dry — but the loop is total either way.
	for round := 0; round < k && pq.Len() > 0; round++ {
		chosen, evaluations := popBest(&pq, round, gain)
		out.Evaluations += evaluations

		// A zero marginal gain means every remaining facility adds nothing.
		// Stopping is right: padding the answer with facilities that cover
		// nobody would report k placements and mean fewer.
		if chosen.gain <= 0 && round > 0 {
			break
		}

		out.Facilities = append(out.Facilities, chosen.site)
		out.Gains = append(out.Gains, chosen.gain)
		out.Covered += chosen.gain
		pos := len(out.Facilities) - 1
		for _, i := range within[chosen.site] {
			if !covered[i] {
				covered[i] = true
				out.CoveredBy[i] = pos
			}
		}
	}

	return out.clone(), nil
}

// MaxCoverBrute returns the true optimum by enumeration. Used to verify the
// bound, not to serve a page.
func MaxCoverBrute(sites []Site, k int, radiusKm float64) (Coverage, error) {
	if err := validate(sites, k); err != nil {
		return Coverage{}, err
	}
	if radiusKm <= 0 {
		return Coverage{}, ErrRadius
	}
	if combinations(len(sites), k) > MaxBruteCombinations {
		return Coverage{}, ErrTooManyCombinations
	}
	m := NewMatrix(sites)
	n := len(sites)

	best := Coverage{RadiusKm: radiusKm, Covered: math.Inf(-1)}
	forEachCombination(n, k, func(pick []int) {
		total := 0.0
		for i := range n {
			for _, f := range pick {
				if m.At(i, f) <= radiusKm {
					total += sites[i].Weight
					break
				}
			}
		}
		if total > best.Covered {
			best.Facilities = append([]int(nil), pick...)
			best.Covered = total
		}
	})

	best.CoveredBy = make([]int, n)
	for i := range best.CoveredBy {
		best.CoveredBy[i] = -1
		for fi, f := range best.Facilities {
			if m.At(i, f) <= radiusKm {
				best.CoveredBy[i] = fi
				break
			}
		}
	}
	return best, nil
}

// popBest returns the candidate with the true largest marginal gain for this
// round, along with how many gains it had to recompute to be sure. It returns
// nil only when the queue is empty.
//
// THE LAZY STEP LIVES HERE. Pop the largest stale gain, recompute just that
// one, and if it still outranks the next stale upper bound then no other
// candidate can beat it — submodularity means every other stale gain is an
// upper bound that can only shrink. Everything recomputed and not chosen goes
// back on the heap carrying its fresh value, so the work is never repeated
// within a round.
//
// THE COMPARISON IS THE HEAP'S OWN ORDER, not `>=` on gain alone. A plain `>=`
// accepts the popped item on an EQUAL gain even when the tie rule says a lower
// site index should win, so lazy greedy and plain greedy chose different
// facilities of identical value. Reusing the comparator is what makes the
// acceleration return exactly what plain greedy returns.
// TestLazyGreedyMatchesPlainGreedy caught that and now pins it.
//
// IT IS A FUNCTION RATHER THAN AN INLINE LOOP so the empty-queue case can be
// tested. MaxCover refuses k above the site count, and each round consumes
// exactly one candidate, so the queue provably cannot run dry inside a real
// solve — as an inline branch that `break` was unreachable code guarding a nil
// dereference. Extracted, the contract is checkable directly.
func popBest(pq *gainQueue, round int, gain func(int) float64) (*gainItem, int) {
	evaluations := 0
	for pq.Len() > 0 {
		top := heap.Pop(pq).(*gainItem)
		if top.round == round {
			// Already recomputed this round: its gain is exact and it is the
			// largest, so it wins.
			return top, evaluations
		}
		top.gain = gain(top.site)
		top.round = round
		evaluations++
		if pq.Len() == 0 || outranks(top, (*pq)[0]) {
			return top, evaluations
		}
		heap.Push(pq, top)
	}
	return nil, evaluations
}

// --- the lazy-greedy priority queue -------------------------------------------

type gainItem struct {
	site  int
	gain  float64
	round int
}

type gainQueue []*gainItem

func (q gainQueue) Len() int { return len(q) }

// Less is a MAX-heap on gain, and ties break on the lower site index. The tie
// rule is not cosmetic: without it two candidates with equal gain would be
// ordered by their position in the heap array, which depends on insertion
// history, and the answer would stop being a function of the input.
func (q gainQueue) Less(i, j int) bool { return outranks(q[i], q[j]) }

// outranks is the single total order over candidates: larger gain first, lower
// site index on a tie. The heap and the lazy step both read it, which is what
// stops them from disagreeing about which of two equal candidates wins.
func outranks(a, b *gainItem) bool {
	if a.gain != b.gain {
		return a.gain > b.gain
	}
	return a.site <= b.site
}

func (q gainQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }

func (q *gainQueue) Push(x any) { *q = append(*q, x.(*gainItem)) }

func (q *gainQueue) Pop() any {
	old := *q
	n := len(old)
	it := old[n-1]
	old[n-1] = nil
	*q = old[:n-1]
	return it
}
