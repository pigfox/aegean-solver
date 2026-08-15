# aegean-solver

Facility placement on the Aegean: where to put `p` supply depots among a few
dozen island ports, solved two ways, with the difference between the two ways as
the point.

This is the solver core behind an interactive demo on pigfox.com. It has no
dependencies outside the standard library, no server, and no I/O — a caller
hands it sites and gets placements back.

> The demo page is being built. This README will carry its URL once the page is
> live rather than before, because a link that 404s reads exactly like a link
> that works until someone clicks it.

## Two problems, and why both are here

| mode | question | objective | guarantee |
|---|---|---|---|
| **p-median** | where do `p` depots go so travel is cheapest overall? | minimize Σ weight × distance-to-nearest | **none** — a heuristic with a measured gap |
| **max-coverage** | where do `k` depots go to serve the most people within `r` km? | maximize weight within `r` of some depot | **1 − 1/e ≈ 0.6321 of optimum, proven** |

The asymmetry is the reason both modes exist. Coverage is monotone and
submodular, so plain greedy carries a worst-case bound that is *proven* — you
can put a number on how bad the answer is allowed to be before you run it.
P-median has no such property and no such bound: the interchange heuristic here
is very good in practice and is guaranteed nothing.

Presenting a heuristic with a proof next to one without is more honest than
presenting either alone, and it is the thing worth showing about this class of
problem.

## What the tests measure

These are `t.Logf` lines from the suite, not claims typed into a README.

```
VERIFIED over 480 instances: worst greedy/optimum ratio 0.7934 against the proven bound 0.6321
MEASURED over 120 instances: worst gap 0.0000%, mean gap 0.0000%, optimal on 120 of 120
lazy greedy: 68 evaluations against a naive 400 (83% saved)
restarts: mean cost 29687390 at 1 restart, 29420973 at 8 (0.90% better)
```

**The bound is checked, not cited.** `TestGreedyMeetsTheOneMinusOneOverE`
enumerates the true optimum at a size where exhaustive search is honest and
asserts the ratio never falls below 1 − 1/e, across 480 instances. It also
asserts greedy never *beats* the exhaustive optimum, which would mean the
verifier was wrong rather than the heuristic being brilliant.

**The p-median gap is measured, not asserted.** There is no bound to assert, so
`TestInterchangeStaysNearTheOptimum` reports the gap against exhaustive search
and fails only against a 5% tripwire it has never approached. On the instances
tested it finds the optimum every time — which is a measurement of these
instances, not a property of the algorithm.

## The algorithms

**Teitz-Bart interchange, restarted.** A pass evaluates every swap of a chosen
site for an unchosen one and applies the single best improvement; passes run
until none improves. That descends to a *local* optimum, which is occasionally
poor, so the whole descent repeats from 8 random starting sets and the best
wins.

The swap neighborhood is `p·(n−p)` independent full reassignments — the entire
cost of the algorithm, and embarrassingly parallel. It is fanned across
`GOMAXPROCS` workers and **fanned back in deterministically**: results are
written into a preallocated slice at an index derived from the pair, never
appended under a mutex, so the candidate order is `(out, in)` however the
goroutines interleave. Ties break on the lowest pair, never on which goroutine
finished first.

**Lazy greedy (Minoux) for coverage.** Plain greedy recomputes every candidate's
marginal gain every round: `k·n` evaluations. Submodularity means a gain can
only *shrink* as facilities are added, so a stale gain is an upper bound — pop
the largest stale gain, recompute only that one, and if it still tops the heap
it is genuinely best. Same answer as plain greedy, most of the work skipped, and
`TestLazyGreedyMatchesPlainGreedy` asserts the first half against a reference
implementation of the second.

**Haversine, not a plane.** The basin spans 35°N to 41°N, where a degree of
longitude is about 20% shorter than a degree of latitude. Treating the plate
carrée plane as metric would stretch every east-west leg by that much and bias
every placement northward.

## A seed reproduces a run

P-median is randomized — the restarts have to start somewhere — so the page
prints a seed and the seed is the whole input to the randomness. Two runs at one
seed are asserted byte-identical, and, in the other direction, twenty different
seeds are asserted to produce more than one answer. A reproducibility test alone
would pass just as well against a constant.

Max-coverage is deterministic by construction and takes a seed only for a
uniform signature. `TestGreedyIsSeedIndependent` asserts the seed changes
nothing there, so an unseeded source creeping in shows up as a failure.

## What this does not do

**No LP or MIP solver, and no dependency that is one.** Both modes are
combinatorial heuristics over an explicit distance matrix. The exhaustive
searches (`PMedianBrute`, `MaxCoverBrute`) exist to *verify* the heuristics at
small sizes, and refuse with `ErrTooManyCombinations` rather than starting a
search they cannot finish.

**No chain, no network, no files.** Sites in, placements out.

## Coverage

100% of statements, and there is no exclusion list. Where a defensive edge could
not be reached by any input — a haversine term above 1, an exhausted candidate
heap, a zero worker count — the edge was pulled out into a named function with
its own contract test rather than left inline as a branch that could only be
verified by reading it. `clampUnit`, `popBest` and `workerCount` are each there
for that reason, and each says so in its comment.

## Running it

```
make test    # go test -race -shuffle=on, go vet, gofmt check
make cover   # 100% of statements
```
