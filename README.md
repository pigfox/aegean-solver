# aegean-solver

Two different questions about the same Aegean island ports, in one module with
no dependencies outside the standard library, no server and no I/O.

**Placement** — where to put `p` supply depots among a few dozen ports, solved
two ways, with the difference between the two ways as the point. A caller hands
it sites and gets placements back. This is the solver core behind the
[Aegean placement demo](https://pigfox.com/demos/aegean-placement).

**Routing** — leaving one port at a given moment, what is the earliest you can
reach another, and by which sailings. This half ships the schema, the source
boundary and the solver, and **ships no schedule**: see
[Routing over a schedule](#routing-over-a-schedule).

## Placement: two problems, and why both are here

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

## Routing over a schedule

The placement half measures distance as a great circle, which is right for
siting a depot and wrong for travel. A vessel cannot sail through an island,
and — far more decisive here — a route served twice a week has a *waiting* time
that dwarfs its sailing time. Miss Tuesday's boat and you do not arrive four
hours late; you arrive on Friday. No distance metric expresses that, because the
cost of an edge depends on when you reach it.

`tdsp` answers earliest arrival over a timetable. `ferry` is the GTFS-shaped
schema it reads, `vocab` is the identity and vessel vocabulary, and `source` is
the boundary a schedule arrives through. All four are importable; only the
shared tuning constants stay in `internal/config`, because nothing in an
exported signature mentions them.

```go
feed := &ferry.Feed{Ports: …, Trips: …, Calendars: …}
it, err := tdsp.EarliestArrival(tdsp.Query{
    Feed: feed, Origin: "piraeus", Destination: "naxos", Depart: t,
})
```

### It ships no schedule data, deliberately

There is not one port, one operator and not one sailing anywhere in these
packages, and the empty feed is *valid* — `source.Empty` returns it and it
passes `Validate()`. That is a requirement rather than an omission: building the
consumer before any producer exists is what keeps invented rows out, because
nobody ever needs to populate something merely to get the pipeline to run.

Where real Aegean sailing times may legitimately be obtained from is an open
question, and answering it by writing plausible-looking rows would have been the
one unrecoverable mistake available here.

### The model

Every departure and every arrival is its own node at its own absolute instant,
and an edge is a concrete thing a traveler can do: ride this sailing, stay
aboard through this call, step ashore, wait, board. Waiting stops being a
function and becomes a walk along a per-port chain.

**FIFO does not hold, and does not need to.** A time-dependent network is FIFO
when leaving later can never get you there earlier. A schedule graph violates
that by construction — a fast catamaran departing at ten berths before a
conventional ferry that sailed at nine — and it is stated rather than assumed
away, because the assumption is common and quiet. It is also not needed: FIFO is
what makes Dijkstra safe on the *compressed* formulation, where an arc carries a
travel-time function. Nothing is compressed here. Every edge weight is a
non-negative elapsed time between two fixed instants, which is the only property
Dijkstra actually requires. Overtaking is simply two different nodes.

**The cost is a pair.** Earliest arrival alone is degenerate on this graph:
every edge weight is the difference of its endpoints' instants, so every path to
a node costs the same and the search collapses into reachability in time order.
Correct, still Dijkstra, and doing no work — and unable to prefer one way of
arriving at ten o'clock over another. So the cost is lexicographic: arrival
instant, then **number of legs**, where only boarding adds a leg and staying
aboard through an intermediate call does not.

**Departure nodes are not in the wait chain.** Staying aboard is a free edge
from an arrival node straight to the departure node of the same sailing, because
a traveler who never gets off needs no connection time. Put departure nodes in
the per-port chain as well and that free edge lands them *in* the chain, from
which any later departure is reachable without ever paying the minimum transfer
— a vessel change with no time in between. Nothing crashes, every itinerary is
still well formed, and the only symptom is answers that are slightly too good.
`TestStayingAboardDoesNotBuyAFreeTransfer` plants exactly that arrangement.

### What the routing tests measure

```
VERIFIED: 2000 generated schedules, 2000 compared against exhaustive enumeration
(420 unreachable, 1154 single-sailing, 320 requiring a transfer, 106 origin
equals destination, deepest itinerary 3 legs, 0 refused above the work ceiling).
VERIFIED OVER: randomly generated schedules of 3-5 ports and 4-8 trips of 2-4
calls, inside a 2-day horizon, in UTC. This is agreement on THESE instances and
is a property of them, not of the method.
```

`BruteEarliestArrival` enumerates every itinerary and is ground truth for the
Dijkstra result. The two are compared on the **cost pair**, not on the
itinerary: where several journeys share a cost, both may return different ones
and both are right, and asserting identical leg lists would assert that two
tie-breaking rules coincide. The itinerary the solver returns is checked
separately against the feed, by re-deriving each leg's instants from its service
day.

The outcome classes are **asserted, not just counted** — a run in which nothing
was unreachable, or nothing needed a transfer, fails, because agreement between
two searches that both refused everything establishes nothing about routing.

## What this does not do

**No LP or MIP solver, and no dependency that is one.** Both placement modes are
combinatorial heuristics over an explicit distance matrix. The exhaustive
searches (`PMedianBrute`, `MaxCoverBrute`) exist to *verify* the heuristics at
small sizes, and refuse with `ErrTooManyCombinations` rather than starting a
search they cannot finish.

**No chain, no network, no files, no credentials.** Sites in, placements out;
a schedule in, an itinerary out. Nothing in this module opens a connection or
reads a key, and an origin that talks to the outside world would implement
`source.Source` from its own package, where its dependencies are visible at its
own import list.

## Coverage

100% of statements across every package, and there is no exclusion list. The
`make cover` target **fails** below 100% and names each shortfall, rather than
printing a percentage and exiting zero. Where a defensive edge could
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
