package aegean

import (
	"container/heap"
	"errors"
	"math"
	"runtime"
	"testing"
)

// The refusals and the edges. Every branch here is one a caller can reach with
// a bad instance, and each returns a sentinel rather than a formatted string so
// the caller can act on it.

func TestRefusals(t *testing.T) {
	ok := []Site{{Lat: 37, Lon: 25, Weight: 1}, {Lat: 38, Lon: 26, Weight: 1}}

	cases := []struct {
		name string
		run  func() error
		want error
	}{
		{"p-median with no sites", func() error { _, e := PMedian(nil, 1, Config{}); return e }, ErrNoSites},
		{"p-median p of zero", func() error { _, e := PMedian(ok, 0, Config{}); return e }, ErrFacilityCount},
		{"p-median p past the site count", func() error { _, e := PMedian(ok, 3, Config{}); return e }, ErrFacilityCount},
		{"brute p-median with no sites", func() error { _, e := PMedianBrute(nil, 1); return e }, ErrNoSites},
		{"brute p-median p past the site count", func() error { _, e := PMedianBrute(ok, 9); return e }, ErrFacilityCount},
		{"max-cover with no sites", func() error { _, e := MaxCover(nil, 1, 10, Config{}); return e }, ErrNoSites},
		{"max-cover k of zero", func() error { _, e := MaxCover(ok, 0, 10, Config{}); return e }, ErrFacilityCount},
		{"max-cover zero radius", func() error { _, e := MaxCover(ok, 1, 0, Config{}); return e }, ErrRadius},
		{"max-cover negative radius", func() error { _, e := MaxCover(ok, 1, -5, Config{}); return e }, ErrRadius},
		{"brute max-cover with no sites", func() error { _, e := MaxCoverBrute(nil, 1, 10); return e }, ErrNoSites},
		{"brute max-cover k past the site count", func() error { _, e := MaxCoverBrute(ok, 4, 10); return e }, ErrFacilityCount},
		{"brute max-cover zero radius", func() error { _, e := MaxCoverBrute(ok, 1, 0); return e }, ErrRadius},
	}
	for _, c := range cases {
		if err := c.run(); !errors.Is(err, c.want) {
			t.Errorf("%s returned %v, want %v", c.name, err, c.want)
		}
	}
}

// TestExhaustiveSearchRefusesWhatItCannotFinish is the guard that keeps the
// brute-force entry points honest. They exist to VERIFY the heuristics, so one
// that quietly became a heuristic itself would defeat their whole purpose.
func TestExhaustiveSearchRefusesWhatItCannotFinish(t *testing.T) {
	big := randomSites(80, 1) // C(80,20) is astronomically past the ceiling

	if _, err := PMedianBrute(big, 20); !errors.Is(err, ErrTooManyCombinations) {
		t.Errorf("p-median brute force accepted an impossible search: %v", err)
	}
	if _, err := MaxCoverBrute(big, 20, 50); !errors.Is(err, ErrTooManyCombinations) {
		t.Errorf("max-cover brute force accepted an impossible search: %v", err)
	}
}

// TestCombinationsSaturatesRatherThanOverflowing pins the one arithmetic
// property the refusal above depends on. An overflow that wrapped to a small
// number would let through exactly the search that cannot finish, so the guard
// has to fail safe.
func TestCombinationsSaturatesRatherThanOverflowing(t *testing.T) {
	for _, c := range []struct{ n, k, want int }{
		{5, 0, 1}, {5, 5, 1}, {5, 1, 5}, {5, 2, 10}, {12, 3, 220}, {50, 3, 19600},
		{5, 6, 0}, {5, -1, 0},
	} {
		if got := combinations(c.n, c.k); got != c.want {
			t.Errorf("combinations(%d,%d) = %d, want %d", c.n, c.k, got, c.want)
		}
	}
	// Well past the ceiling, and past int64 if computed naively.
	if got := combinations(200, 100); got <= MaxBruteCombinations {
		t.Errorf("combinations(200,100) = %d — it must saturate above the ceiling", got)
	}
	if got := combinations(60, 30); got <= MaxBruteCombinations {
		t.Errorf("combinations(60,30) = %d — it must saturate above the ceiling", got)
	}
}

func TestForEachCombinationRefusesAnImpossibleK(t *testing.T) {
	calls := 0
	forEachCombination(3, 5, func([]int) { calls++ })
	forEachCombination(3, -1, func([]int) { calls++ })
	if calls != 0 {
		t.Errorf("an impossible k produced %d calls", calls)
	}

	// And it enumerates in lexicographic order, which PMedianBrute's determinism
	// rests on — first-best-wins is only stable if the order is.
	var got [][]int
	forEachCombination(4, 2, func(pick []int) {
		got = append(got, append([]int(nil), pick...))
	})
	want := [][]int{{0, 1}, {0, 2}, {0, 3}, {1, 2}, {1, 3}, {2, 3}}
	if len(got) != len(want) {
		t.Fatalf("enumerated %d combinations, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i][0] != want[i][0] || got[i][1] != want[i][1] {
			t.Errorf("combination %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestDistanceIsSaneAndSymmetric checks the model against a figure that can be
// looked up, rather than against itself.
func TestDistanceIsSaneAndSymmetric(t *testing.T) {
	athens := Site{Name: "Piraeus", Lat: 37.9420, Lon: 23.6465}
	rhodes := Site{Name: "Rhodes", Lat: 36.4349, Lon: 28.2176}

	d := DistanceKm(athens, rhodes)
	// Piraeus to Rhodes is about 425 km great-circle. A generous window: the
	// assertion is that the model is a real distance, not that it is precise to
	// the metre.
	if d < 400 || d > 450 {
		t.Errorf("Piraeus to Rhodes = %.1f km, expected roughly 425", d)
	}
	if back := DistanceKm(rhodes, athens); math.Abs(back-d) > 1e-9 {
		t.Errorf("distance is not symmetric: %.6f against %.6f", d, back)
	}
	if self := DistanceKm(athens, athens); self != 0 {
		t.Errorf("distance from a site to itself = %v, want 0", self)
	}

	// The clamp. Antipodal points accumulate rounding that can push the
	// haversine term a hair above 1, where Asin is NaN rather than pi/2.
	anti := DistanceKm(Site{Lat: 0, Lon: 0}, Site{Lat: 0, Lon: 180})
	if math.IsNaN(anti) {
		t.Error("an antipodal pair produced NaN — the clamp is not working")
	}
	if anti < 20000 || anti > 20040 {
		t.Errorf("an antipodal pair = %.1f km, expected about half the circumference", anti)
	}
}

func TestMatrixIsSymmetricAndSized(t *testing.T) {
	sites := randomSites(6, 5)
	m := NewMatrix(sites)
	if m.N() != 6 {
		t.Fatalf("N = %d, want 6", m.N())
	}
	for i := range 6 {
		if m.At(i, i) != 0 {
			t.Errorf("diagonal at %d = %v, want 0", i, m.At(i, i))
		}
		for j := range 6 {
			if m.At(i, j) != m.At(j, i) {
				t.Errorf("matrix is asymmetric at (%d,%d)", i, j)
			}
		}
	}
}

// TestZeroWeightSitesStillCompeteAsFacilities pins the documented behavior: a
// site with no demand contributes nothing to the objective but is still a
// legal place to put a facility. An uninhabited islet with a good harbor is
// exactly that.
func TestZeroWeightSitesStillCompeteAsFacilities(t *testing.T) {
	// A TRIANGLE, not a line. Three equal weights strung out collinearly make
	// every candidate between the ends tie — the sum of distances along a line
	// is constant — so the first version of this test asserted a preference
	// that does not exist and failed on a correct answer. Three demand points
	// around a central uninhabited site give the middle a strict advantage.
	sites := []Site{
		{ID: "a", Lat: 37.00, Lon: 25.00, Weight: 1000},
		{ID: "b", Lat: 37.00, Lon: 25.40, Weight: 1000},
		{ID: "c", Lat: 37.40, Lon: 25.20, Weight: 1000},
		{ID: "islet", Lat: 37.13, Lon: 25.20, Weight: 0}, // no demand, good harbor
	}

	got, err := PMedian(sites, 1, Config{Seed: 1})
	if err != nil {
		t.Fatalf("pmedian: %v", err)
	}
	if len(got.Facilities) != 1 {
		t.Fatalf("got %d facilities, want 1", len(got.Facilities))
	}

	// Compared against the exhaustive optimum rather than against a hard-coded
	// index, so the assertion is "it found the best site" rather than "it found
	// the site I expected".
	opt, err := PMedianBrute(sites, 1)
	if err != nil {
		t.Fatalf("brute: %v", err)
	}
	if math.Abs(got.Cost-opt.Cost) > 1e-9 {
		t.Errorf("cost %.6f against an optimal %.6f", got.Cost, opt.Cost)
	}
	if got.Facilities[0] != 3 {
		t.Errorf("the zero-weight islet is central and should win; got index %d at cost %.3f "+
			"(optimum %.3f)", got.Facilities[0], got.Cost, opt.Cost)
	}
}

// TestEveryFacilityCountIsSolvable walks p from 1 to n so no count is left
// untried, and pins that the cost falls as facilities are added.
func TestEveryFacilityCountIsSolvable(t *testing.T) {
	sites := randomSites(9, 21)
	prev := math.Inf(1)
	for p := 1; p <= len(sites); p++ {
		got, err := PMedian(sites, p, Config{Seed: 4})
		if err != nil {
			t.Fatalf("p %d: %v", p, err)
		}
		if len(got.Facilities) != p {
			t.Errorf("p %d: got %d facilities", p, len(got.Facilities))
		}
		if got.Cost > prev+1e-9 {
			t.Errorf("p %d: cost rose to %.3f from %.3f — more facilities cannot be worse",
				p, got.Cost, prev)
		}
		prev = got.Cost
		if len(got.Assignment) != len(sites) {
			t.Errorf("p %d: assignment covers %d of %d sites", p, len(got.Assignment), len(sites))
		}
		for i, a := range got.Assignment {
			if a < 0 || a >= p {
				t.Errorf("p %d: site %d assigned to facility position %d", p, i, a)
			}
		}
	}
}

// TestCoverageStopsWhenNothingIsLeftToCover pins the early exit. Padding an
// answer with facilities that cover nobody would report k placements and mean
// fewer.
func TestCoverageStopsWhenNothingIsLeftToCover(t *testing.T) {
	// Three sites inside one radius of each other: one facility covers all.
	sites := []Site{
		{ID: "a", Lat: 37.00, Lon: 25.00, Weight: 10},
		{ID: "b", Lat: 37.01, Lon: 25.01, Weight: 10},
		{ID: "c", Lat: 37.02, Lon: 25.02, Weight: 10},
	}
	got, err := MaxCover(sites, 3, 50, Config{Seed: 1})
	if err != nil {
		t.Fatalf("maxcover: %v", err)
	}
	if len(got.Facilities) != 1 {
		t.Errorf("got %d facilities for a fully covered instance, want 1", len(got.Facilities))
	}
	if got.Covered != 30 {
		t.Errorf("covered %.0f, want 30", got.Covered)
	}
	for i, by := range got.CoveredBy {
		if by != 0 {
			t.Errorf("site %d reports facility %d, want 0", i, by)
		}
	}
}

// TestUncoveredSitesReportMinusOne pins the sentinel a renderer keys on.
func TestUncoveredSitesReportMinusOne(t *testing.T) {
	sites := []Site{
		{ID: "near", Lat: 37.00, Lon: 25.00, Weight: 10},
		{ID: "far", Lat: 40.00, Lon: 28.00, Weight: 10},
	}
	got, err := MaxCover(sites, 1, 20, Config{Seed: 1})
	if err != nil {
		t.Fatalf("maxcover: %v", err)
	}
	uncovered := 0
	for _, by := range got.CoveredBy {
		if by == -1 {
			uncovered++
		}
	}
	if uncovered != 1 {
		t.Errorf("%d sites reported uncovered, want 1", uncovered)
	}
}

// TestConfigDefaults pins that a zero Config is usable and that explicit values
// survive.
func TestConfigDefaults(t *testing.T) {
	got := Config{}.withDefaults()
	if got.Restarts != DefaultRestarts {
		t.Errorf("Restarts = %d, want %d", got.Restarts, DefaultRestarts)
	}
	if got.MaxPasses != DefaultMaxPasses {
		t.Errorf("MaxPasses = %d, want %d", got.MaxPasses, DefaultMaxPasses)
	}
	explicit := Config{Restarts: 3, MaxPasses: 5}.withDefaults()
	if explicit.Restarts != 3 || explicit.MaxPasses != 5 {
		t.Errorf("explicit values were overwritten: %+v", explicit)
	}
	// Assigned first: a composite literal in an if-condition is a parse error,
	// because the brace is read as the start of the block.
	cfg := Config{Seed: 9}
	if cfg.rng() == nil {
		t.Error("rng returned nil")
	}
}

// TestRandomChoiceDrawsDistinctIndices pins the sampler both ways: the right
// count, no repeats, and the same draw for the same seed.
func TestRandomChoiceDrawsDistinctIndices(t *testing.T) {
	for _, k := range []int{1, 5, 20} {
		rng := Config{Seed: 3}.rng()
		got := randomChoice(rng, 20, k)
		if len(got) != k {
			t.Fatalf("k %d: drew %d", k, len(got))
		}
		seen := map[int]bool{}
		for _, v := range got {
			if v < 0 || v >= 20 {
				t.Errorf("k %d: index %d out of range", k, v)
			}
			if seen[v] {
				t.Errorf("k %d: index %d drawn twice", k, v)
			}
			seen[v] = true
		}
		again := randomChoice(Config{Seed: 3}.rng(), 20, k)
		for i := range got {
			if got[i] != again[i] {
				t.Errorf("k %d: one seed produced two different draws", k)
				break
			}
		}
	}
}

// --- the defensive edges, as contracts ------------------------------------------
//
// Each of the four below guards a case no real input reaches: a haversine term
// above 1, a combination count that overflows, an exhausted candidate heap, a
// zero worker count. Written inline they were unreachable branches that could
// only be verified by reading them, which is the same as not verifying them.
// Each is now a named function with a contract, and these are the contracts.

func TestClampUnitHoldsTheAsinDomain(t *testing.T) {
	for _, c := range []struct{ in, want float64 }{
		{0, 0}, {0.5, 0.5}, {1, 1},
		{1.0000000002, 1}, // the near-antipodal rounding case
		{2, 1},
		{-1e-17, 0}, // the other end, from the same rounding
		{-3, 0},
	} {
		if got := clampUnit(c.in); got != c.want {
			t.Errorf("clampUnit(%v) = %v, want %v", c.in, got, c.want)
		}
	}
	// The reason it exists: unclamped, this is NaN.
	if math.IsNaN(math.Asin(math.Sqrt(clampUnit(1.0000000002)))) {
		t.Error("a clamped term still produced NaN through Asin")
	}
	if !math.IsNaN(math.Asin(math.Sqrt(1.0000000002))) {
		t.Skip("Asin no longer returns NaN above 1 — the clamp's premise has changed")
	}
}

// TestTheCeilingKeepsTheProductInRange is the invariant that let the overflow
// check come out of combinations. If someone raises the ceiling past the safe
// point this fails, rather than a refusal silently wrapping into an acceptance.
func TestTheCeilingKeepsTheProductInRange(t *testing.T) {
	const maxInt = int(^uint(0) >> 1)
	if MaxBruteCombinations < 1 {
		t.Fatalf("the ceiling is %d — a non-positive ceiling refuses everything",
			MaxBruteCombinations)
	}
	if MaxBruteCombinations > maxInt/MaxBruteCombinations {
		t.Fatalf("the ceiling %d is too large: its square overflows an int, so "+
			"combinations() can wrap and let through a search that cannot finish",
			MaxBruteCombinations)
	}
}

func TestPopBestReportsAnExhaustedQueue(t *testing.T) {
	empty := gainQueue{}
	got, evaluations := popBest(&empty, 0, func(int) float64 {
		t.Error("an empty queue must not evaluate anything")
		return 0
	})
	if got != nil {
		t.Errorf("an empty queue returned %+v, want nil", got)
	}
	if evaluations != 0 {
		t.Errorf("an empty queue reported %d evaluations, want 0", evaluations)
	}

	// And it drains: k pops of a k-item queue leave it empty and the next
	// returns nil rather than panicking.
	pq := gainQueue{
		{site: 0, gain: 5, round: -1},
		{site: 1, gain: 9, round: -1},
	}
	heap.Init(&pq)
	gains := map[int]float64{0: 5, 1: 9}
	first, _ := popBest(&pq, 0, func(s int) float64 { return gains[s] })
	if first == nil || first.site != 1 {
		t.Fatalf("first pop = %+v, want the site-1 item", first)
	}
	second, _ := popBest(&pq, 0, func(s int) float64 { return gains[s] })
	if second == nil || second.site != 0 {
		t.Fatalf("second pop = %+v, want the site-0 item", second)
	}
	if third, _ := popBest(&pq, 0, func(s int) float64 { return gains[s] }); third != nil {
		t.Errorf("a drained queue returned %+v, want nil", third)
	}
}

func TestWorkerCountNeverFallsBelowOne(t *testing.T) {
	if got := workerCount(0); got != 1 {
		t.Errorf("workerCount(0) = %d, want 1 — zero workers would leave every "+
			"result slot at its zero value and report that no swap improves", got)
	}
	if got := workerCount(-5); got != 1 {
		t.Errorf("workerCount(-5) = %d, want 1", got)
	}
	if got := workerCount(1); got != 1 {
		t.Errorf("workerCount(1) = %d, want 1", got)
	}
	// Never more workers than there is work for.
	if got := workerCount(2); got > 2 {
		t.Errorf("workerCount(2) = %d — more workers than tasks", got)
	}
	// And it does use the cores when there is work for them.
	if procs := runtime.GOMAXPROCS(0); procs > 1 {
		if got := workerCount(1 << 20); got != procs {
			t.Errorf("workerCount(large) = %d, want GOMAXPROCS %d", got, procs)
		}
	}
}

// TestMaxPassesBoundsTheLoop pins the backstop. One pass cannot reach a local
// optimum from a bad start on a real instance, so a capped run must be no
// better than an uncapped one.
func TestMaxPassesBoundsTheLoop(t *testing.T) {
	sites := randomSites(30, 6)
	capped, err := PMedian(sites, 5, Config{Seed: 2, Restarts: 1, MaxPasses: 1})
	if err != nil {
		t.Fatalf("pmedian: %v", err)
	}
	full, err := PMedian(sites, 5, Config{Seed: 2, Restarts: 1})
	if err != nil {
		t.Fatalf("pmedian: %v", err)
	}
	if capped.Passes != 1 {
		t.Errorf("capped run made %d passes, want 1", capped.Passes)
	}
	if full.Cost > capped.Cost+1e-9 {
		t.Errorf("the uncapped run was worse (%.3f) than the one-pass run (%.3f)",
			full.Cost, capped.Cost)
	}
}
