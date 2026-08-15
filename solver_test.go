package aegean

import (
	"math"
	"math/rand/v2"
	"reflect"
	"testing"
)

// randomSites builds a reproducible instance in a box roughly the size of the
// Aegean, so the distances the tests exercise are the distances the real
// instance has.
func randomSites(n int, seed uint64) []Site {
	rng := rand.New(rand.NewPCG(seed, 0xA5A5A5A5))
	out := make([]Site, n)
	for i := range out {
		out[i] = Site{
			ID:     string(rune('a'+i%26)) + string(rune('0'+i/26)),
			Name:   "site",
			Lat:    35.0 + rng.Float64()*6.0, // 35N..41N
			Lon:    23.0 + rng.Float64()*5.0, // 23E..28E
			Weight: math.Round(rng.Float64() * 20000),
		}
	}
	return out
}

// --- the two verifications this package exists to make ------------------------

// TestGreedyMeetsTheOneMinusOneOverE is the claim the max-coverage mode is here
// to demonstrate, checked rather than cited.
//
// Coverage is monotone and submodular, so greedy is proven to return at least
// (1 - 1/e) of the optimum. This enumerates the TRUE optimum at a size where
// exhaustive search is honest and asserts the ratio never falls below the bound,
// across many independent instances and every k in range. A proof that is only
// quoted is a claim; this is evidence.
func TestGreedyMeetsTheOneMinusOneOverE(t *testing.T) {
	const bound = 1 - 1/math.E // ≈ 0.6321

	worst := math.Inf(1)
	instances := 0
	for seed := uint64(1); seed <= 40; seed++ {
		sites := randomSites(12, seed)
		for k := 1; k <= 4; k++ {
			for _, radius := range []float64{40, 90, 160} {
				got, err := MaxCover(sites, k, radius, Config{Seed: seed})
				if err != nil {
					t.Fatalf("greedy: %v", err)
				}
				opt, err := MaxCoverBrute(sites, k, radius)
				if err != nil {
					t.Fatalf("brute: %v", err)
				}
				if opt.Covered <= 0 {
					continue // nothing to cover at this radius; ratio undefined
				}
				instances++

				ratio := got.Covered / opt.Covered
				if ratio < worst {
					worst = ratio
				}
				if ratio < bound-1e-9 {
					t.Errorf("seed %d k %d radius %.0f: greedy covered %.0f of an optimal %.0f "+
						"= %.4f, BELOW the proven (1-1/e) = %.4f bound",
						seed, k, radius, got.Covered, opt.Covered, ratio, bound)
				}
				if got.Covered > opt.Covered+1e-9 {
					t.Errorf("seed %d k %d radius %.0f: greedy covered MORE than the exhaustive "+
						"optimum (%.0f > %.0f) — the brute-force search is wrong",
						seed, k, radius, got.Covered, opt.Covered)
				}
			}
		}
	}

	if instances < 100 {
		t.Fatalf("only %d instances compared — too few to mean anything", instances)
	}
	t.Logf("VERIFIED over %d instances: worst greedy/optimum ratio %.4f against the proven bound %.4f",
		instances, worst, bound)
}

// TestInterchangeStaysNearTheOptimum measures the p-median heuristic against
// the true optimum and reports the gap.
//
// There is NO guarantee to assert here — that asymmetry is exactly why both
// modes are in the package — so this measures instead of proving, and fails
// only if the heuristic is worse than a stated tolerance it has never
// approached.
func TestInterchangeStaysNearTheOptimum(t *testing.T) {
	worstGap, sumGap, exact, instances := 0.0, 0.0, 0, 0

	for seed := uint64(1); seed <= 30; seed++ {
		sites := randomSites(14, seed)
		for p := 1; p <= 4; p++ {
			got, err := PMedian(sites, p, Config{Seed: seed})
			if err != nil {
				t.Fatalf("pmedian: %v", err)
			}
			opt, err := PMedianBrute(sites, p)
			if err != nil {
				t.Fatalf("brute: %v", err)
			}
			if opt.Cost <= 0 {
				continue
			}
			instances++

			gap := (got.Cost - opt.Cost) / opt.Cost
			if gap < -1e-9 {
				t.Errorf("seed %d p %d: heuristic beat the exhaustive optimum (%.3f < %.3f) — "+
					"the brute-force search is wrong", seed, p, got.Cost, opt.Cost)
			}
			if gap < 1e-9 {
				exact++
			}
			sumGap += gap
			if gap > worstGap {
				worstGap = gap
			}
		}
	}

	if instances < 50 {
		t.Fatalf("only %d instances compared", instances)
	}
	t.Logf("MEASURED over %d instances: worst gap %.4f%%, mean gap %.4f%%, optimal on %d of %d",
		instances, worstGap*100, (sumGap/float64(instances))*100, exact, instances)

	// A tolerance far above anything observed. It is a regression tripwire, not
	// a claim about the algorithm.
	if worstGap > 0.05 {
		t.Errorf("worst gap %.4f%% exceeds the 5%% tripwire", worstGap*100)
	}
}

// --- determinism ---------------------------------------------------------------

// TestOneSeedReproducesARun is the promise the page makes when it prints a seed.
func TestOneSeedReproducesARun(t *testing.T) {
	sites := randomSites(40, 99)

	for _, seed := range []uint64{0, 1, 7, 1 << 40} {
		a, err := PMedian(sites, 5, Config{Seed: seed})
		if err != nil {
			t.Fatalf("pmedian: %v", err)
		}
		b, err := PMedian(sites, 5, Config{Seed: seed})
		if err != nil {
			t.Fatalf("pmedian: %v", err)
		}
		if !reflect.DeepEqual(a, b) {
			t.Errorf("seed %d: two runs differ.\n a=%v cost %.6f\n b=%v cost %.6f",
				seed, a.Facilities, a.Cost, b.Facilities, b.Cost)
		}
	}
}

// TestDifferentSeedsExploreDifferently is the other half. If every seed gave
// the same answer the seed would be decorative, and the reproducibility test
// above would pass just as well against a constant.
func TestDifferentSeedsExploreDifferently(t *testing.T) {
	sites := randomSites(40, 4242)

	seen := map[float64]bool{}
	for seed := uint64(1); seed <= 20; seed++ {
		got, err := PMedian(sites, 6, Config{Seed: seed, Restarts: 1})
		if err != nil {
			t.Fatalf("pmedian: %v", err)
		}
		seen[math.Round(got.Cost*1000)] = true
	}
	if len(seen) < 2 {
		t.Errorf("20 single-restart seeds produced %d distinct costs — the seed is not "+
			"reaching the search", len(seen))
	}
}

// TestGreedyIsSeedIndependent pins that max-coverage is NOT randomized. Greedy
// is deterministic by construction, so a seed must not change its answer — and
// if one ever does, something has picked up an unseeded source.
func TestGreedyIsSeedIndependent(t *testing.T) {
	sites := randomSites(30, 11)
	first, err := MaxCover(sites, 4, 80, Config{Seed: 1})
	if err != nil {
		t.Fatalf("maxcover: %v", err)
	}
	for seed := uint64(2); seed <= 6; seed++ {
		got, err := MaxCover(sites, 4, 80, Config{Seed: seed})
		if err != nil {
			t.Fatalf("maxcover: %v", err)
		}
		if !reflect.DeepEqual(first, got) {
			t.Fatalf("seed %d changed a deterministic algorithm's answer", seed)
		}
	}
}

// --- properties -----------------------------------------------------------------

// TestGainsAreNonIncreasing is submodularity, observed. If a later facility
// ever added more than an earlier one, the lazy heap's central assumption —
// that a stale gain is an upper bound — would be false and the answer would
// silently stop matching plain greedy.
func TestGainsAreNonIncreasing(t *testing.T) {
	for seed := uint64(1); seed <= 15; seed++ {
		sites := randomSites(35, seed)
		got, err := MaxCover(sites, 8, 70, Config{Seed: seed})
		if err != nil {
			t.Fatalf("maxcover: %v", err)
		}
		for i := 1; i < len(got.Gains); i++ {
			if got.Gains[i] > got.Gains[i-1]+1e-9 {
				t.Errorf("seed %d: gain rose from %.0f to %.0f at step %d — not submodular",
					seed, got.Gains[i-1], got.Gains[i], i)
			}
		}
	}
}

// TestLazyGreedySkipsWork is the reason the heap is there. Plain greedy costs
// k·n evaluations; if lazy greedy is not well below that it is complexity with
// no payoff.
func TestLazyGreedySkipsWork(t *testing.T) {
	sites := randomSites(50, 3)
	const k = 8
	got, err := MaxCover(sites, k, 60, Config{Seed: 3})
	if err != nil {
		t.Fatalf("maxcover: %v", err)
	}
	naive := k * len(sites)
	if got.Evaluations >= naive {
		t.Errorf("lazy greedy used %d evaluations against a naive %d — it saved nothing",
			got.Evaluations, naive)
	}
	t.Logf("lazy greedy: %d evaluations against a naive %d (%.0f%% saved)",
		got.Evaluations, naive, 100*(1-float64(got.Evaluations)/float64(naive)))
}

// TestLazyGreedyMatchesPlainGreedy is the correctness half of the acceleration.
// Skipping work must not change the answer.
func TestLazyGreedyMatchesPlainGreedy(t *testing.T) {
	for seed := uint64(1); seed <= 12; seed++ {
		sites := randomSites(28, seed)
		for k := 1; k <= 5; k++ {
			lazy, err := MaxCover(sites, k, 75, Config{Seed: seed})
			if err != nil {
				t.Fatalf("maxcover: %v", err)
			}
			plain := plainGreedy(sites, k, 75)
			if !reflect.DeepEqual(lazy.Facilities, plain) {
				t.Errorf("seed %d k %d: lazy chose %v, plain greedy chose %v",
					seed, k, lazy.Facilities, plain)
			}
		}
	}
}

// plainGreedy is the unaccelerated reference: recompute every gain every round.
func plainGreedy(sites []Site, k int, radius float64) []int {
	m := NewMatrix(sites)
	n := len(sites)
	covered := make([]bool, n)
	var chosen []int

	for range k {
		bestGain, bestSite := 0.0, -1
		for f := range n {
			g := 0.0
			for i := range n {
				if !covered[i] && m.At(i, f) <= radius {
					g += sites[i].Weight
				}
			}
			// Ties break low, matching the heap's Less.
			if g > bestGain {
				bestGain, bestSite = g, f
			}
		}
		if bestSite < 0 || (bestGain <= 0 && len(chosen) > 0) {
			break
		}
		chosen = append(chosen, bestSite)
		for i := range n {
			if m.At(i, bestSite) <= radius {
				covered[i] = true
			}
		}
	}
	return chosen
}

// TestRestartsEarnTheirKeep measures the DefaultRestarts figure rather than
// asserting it, so the constant's comment stays honest.
func TestRestartsEarnTheirKeep(t *testing.T) {
	sites := randomSites(45, 8)
	const p = 6

	one := 0.0
	many := 0.0
	for seed := uint64(1); seed <= 12; seed++ {
		a, err := PMedian(sites, p, Config{Seed: seed, Restarts: 1})
		if err != nil {
			t.Fatalf("pmedian: %v", err)
		}
		b, err := PMedian(sites, p, Config{Seed: seed, Restarts: DefaultRestarts})
		if err != nil {
			t.Fatalf("pmedian: %v", err)
		}
		one += a.Cost
		many += b.Cost
		if b.Cost > a.Cost+1e-9 {
			t.Errorf("seed %d: more restarts produced a WORSE cost (%.3f > %.3f)", seed, b.Cost, a.Cost)
		}
	}
	t.Logf("restarts: mean cost %.0f at 1 restart, %.0f at %d (%.2f%% better)",
		one/12, many/12, DefaultRestarts, 100*(one-many)/one)
}
