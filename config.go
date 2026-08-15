package aegean

import "math/rand/v2"

// The tuning constants. They live here rather than at a call site so the figure
// a page prints and the figure a solve uses are the same one, and so changing a
// default is one edit with one place to argue about it.
const (
	// DefaultRestarts is how many independent starting sets Teitz-Bart tries
	// before returning its best. Interchange descends to a LOCAL optimum and
	// stops; restarting from elsewhere is the cheapest way to find a better
	// one. Eight is where the gain flattened on the Aegean instance — see
	// TestRestartsEarnTheirKeep, which measures it rather than asserting it.
	DefaultRestarts = 8

	// DefaultMaxPasses bounds the interchange loop. Each pass is O(p·(n-p)·n)
	// and the loop exits on its own when no swap improves, so this is a
	// backstop against a cycle rather than a schedule. It has never been the
	// binding constraint on any instance in the tests.
	DefaultMaxPasses = 64

	// MaxBruteCombinations refuses an exhaustive search that would not finish.
	// C(50,5) is 2.1 million and takes seconds; C(50,8) is 536 million and does
	// not. The brute-force entry points exist to VERIFY the heuristics at small
	// n, so a hard refusal above this is correct — an exhaustive search that is
	// quietly a heuristic would defeat their whole purpose.
	MaxBruteCombinations = 5_000_000

	// pcgStream is the second PCG word. Fixed, so a seed alone determines the
	// stream — the same reasoning internal/reconcilelab uses in pigfox2.
	pcgStream = 0x9E3779B97F4A7C15
)

// Config carries what a solve needs beyond the instance itself.
type Config struct {
	// Seed determines every random choice. It is EXPLICIT and never defaulted
	// from the clock: a page that prints its seed is making a promise that the
	// run can be reproduced, and a clock-seeded default would quietly break it.
	Seed uint64

	// Restarts is the number of Teitz-Bart starting sets. Zero means
	// DefaultRestarts.
	Restarts int

	// MaxPasses bounds the interchange loop. Zero means DefaultMaxPasses.
	MaxPasses int
}

// withDefaults returns a copy with the zero values filled in.
func (c Config) withDefaults() Config {
	if c.Restarts <= 0 {
		c.Restarts = DefaultRestarts
	}
	if c.MaxPasses <= 0 {
		c.MaxPasses = DefaultMaxPasses
	}
	return c
}

// rng builds this config's generator. A fresh one per call, so the sequence a
// solve sees depends on the seed and nothing else — not on how many solves ran
// before it.
func (c Config) rng() *rand.Rand {
	return rand.New(rand.NewPCG(c.Seed, pcgStream))
}
