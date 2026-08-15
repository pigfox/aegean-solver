package aegean

import "math/rand/v2"

// combinations is C(n, k), saturating rather than overflowing.
//
// It is used as a REFUSAL TEST before an exhaustive search, so an overflow that
// wrapped to a small number would be the one failure that matters: it would let
// through exactly the search that cannot finish. Saturation makes the guard
// fail safe.
func combinations(n, k int) int {
	if k < 0 || k > n {
		return 0
	}
	if k > n-k {
		k = n - k
	}
	result := 1
	for i := range k {
		result = result * (n - i) / (i + 1)
		if result > MaxBruteCombinations {
			return MaxBruteCombinations + 1 // saturate: too big to enumerate
		}
	}
	return result
}

// THE MULTIPLICATION CANNOT OVERFLOW, and the reason is worth writing down
// because an earlier draft carried an overflow check here that no input could
// ever reach.
//
// The first iteration computes result = n exactly (i is 0, so it is 1·n/1, and
// one times an int cannot overflow). If n exceeds the ceiling the function has
// already returned. So every later iteration multiplies two values that are
// both at most MaxBruteCombinations, and that product fits in an int as long as
// the ceiling squared does. TestTheCeilingKeepsTheProductInRange pins exactly
// that, so raising the ceiling past the safe point fails a test instead of
// silently wrapping a refusal into an acceptance.

// forEachCombination calls fn with every k-subset of [0,n), in lexicographic
// order. The slice is REUSED between calls — callers that keep it must copy,
// and both callers in this package do.
func forEachCombination(n, k int, fn func(pick []int)) {
	if k < 0 || k > n {
		return
	}
	pick := make([]int, k)
	var rec func(start, depth int)
	rec = func(start, depth int) {
		if depth == k {
			fn(pick)
			return
		}
		// The upper bound leaves room for the remaining picks, so the recursion
		// never walks a branch that cannot complete.
		for i := start; i <= n-(k-depth); i++ {
			pick[depth] = i
			rec(i+1, depth+1)
		}
	}
	rec(0, 0)
}

// randomChoice draws k distinct indices from [0,n) using the supplied
// generator.
//
// PARTIAL FISHER-YATES over an explicit permutation buffer, not repeated
// rejection sampling. Rejection would consume a number of draws that depends on
// the collisions it happened to hit, so two runs at one seed could diverge the
// moment k approached n. This consumes exactly k draws for any k, which is what
// makes the sequence a function of the seed alone.
func randomChoice(rng *rand.Rand, n, k int) []int {
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	for i := range k {
		j := i + rng.IntN(n-i)
		idx[i], idx[j] = idx[j], idx[i]
	}
	out := append([]int(nil), idx[:k]...)
	return out
}
