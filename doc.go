// Package aegean solves two classical facility-siting problems over a set of
// weighted geographic sites, with no dependency beyond the standard library.
//
// # The two problems, and why both are here
//
// P-MEDIAN asks: place p facilities so the weighted sum of distances from every
// demand point to its nearest facility is smallest. It is the "where do we put
// the depots so the average trip is short" question, and it is NP-hard, so what
// runs here is Teitz-Bart interchange — start from a set, try every swap of one
// chosen site for one unchosen site, take the best improvement, repeat until no
// swap helps. Restarts from different starting sets reduce the chance of
// stopping in a poor local optimum.
//
// MAX-COVERAGE asks a different question: place k facilities so the weighted
// population WITHIN A RADIUS of some facility is largest. Distance past the
// radius does not matter at all, which is the right model when service is
// binary — a port is reachable by the daily boat or it is not.
//
// # Why max-coverage is worth having beside p-median
//
// Max-coverage is submodular and monotone, so the greedy algorithm carries a
// PROVEN worst-case guarantee: it returns at least (1 - 1/e) ≈ 63.2% of the
// optimum. P-median has no such bound. That contrast is the reason both modes
// exist in one package rather than one being enough — the same map, the same
// sites, and one question that can be answered with a guarantee while the other
// cannot.
//
// The guarantee is not taken on faith here. TestGreedyMeetsTheOneMinusOneOverE
// enumerates the true optimum by brute force at small k and asserts the greedy
// result is never below the bound, over many random instances and every seed it
// is given. A proof that is only cited is a claim; a proof that is checked
// against an exhaustive search is evidence.
//
// # Determinism
//
// Every randomized step draws from an explicit math/rand/v2 PCG seeded from
// Config.Seed. There is no use of the global source, and nothing is seeded from
// the clock. Two runs with the same seed and the same input produce
// byte-identical results, which is what lets a page print its seed and a reader
// reproduce exactly what they saw.
package aegean
