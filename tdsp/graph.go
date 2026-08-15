package tdsp

import (
	"container/heap"
	"math"
	"slices"

	"github.com/pigfox/aegean-solver/vocab"
)

// nodeKind is what a node represents. The three kinds are not decoration —
// which edges may leave a node depends entirely on which kind it is, and that
// is what keeps the minimum transfer time honest.
type nodeKind uint8

const (
	// kindWait is a traveler standing at a port at a given instant, ready to
	// board. These are the only nodes joined into a per-port chain.
	kindWait nodeKind = iota
	// kindDepart is a specific sailing leaving a specific call.
	kindDepart
	// kindArrive is a specific sailing berthing at a specific call.
	kindArrive
)

type node struct {
	kind nodeKind
	at   int64
	port vocab.ID
	// inst and stop locate a departure or an arrival within the expanded
	// schedule. They are meaningless on a wait node.
	inst int
	stop int
}

// arc is an edge. legs is 1 on a boarding edge and 0 on every other, which is
// the entire definition of the second cost component.
type arc struct {
	to   int
	legs int
}

type graph struct {
	nodes []node
	arcs  [][]arc
}

func (g *graph) addNode(n node) int {
	g.nodes = append(g.nodes, n)
	g.arcs = append(g.arcs, nil)
	return len(g.nodes) - 1
}

func (g *graph) addArc(from, to, legs int) {
	g.arcs[from] = append(g.arcs[from], arc{to: to, legs: legs})
}

// portTime keys the lookup from a port and an instant onto its wait node.
type portTime struct {
	port vocab.ID
	at   int64
}

// buildGraph expands the prepared schedule into the time-expanded graph and
// returns it with the index of the node the traveler starts at.
//
// # The shape, and the one mistake that is invisible if you make it
//
// Per port there is a chain of WAIT nodes, one per instant at which somebody
// could be standing there ready to board: every arrival instant plus that
// port's minimum transfer time, every departure instant, and — at the origin —
// the query's own departure instant. Consecutive wait nodes are joined, which
// is how waiting is modeled.
//
// DEPARTURE NODES ARE NOT IN THAT CHAIN, and keeping them out is the whole
// correctness of the transfer rule. Staying aboard through an intermediate call
// is a free edge from the arrival node straight to the departure node of the
// same sailing, because a traveler who never gets off does not need connection
// time. If departure nodes were also links in the wait chain, that free edge
// would land the traveler inside the chain, and from there they could walk
// forward to ANY later departure at that port without ever paying the minimum
// transfer — stepping off one vessel and onto another with no time in between,
// as long as they had been aboard something that happened to call there.
//
// The failure is invisible in the ordinary sense: nothing crashes, no
// invariant trips, every itinerary is well-formed, and the only symptom is that
// the answers are slightly too good. TestStayingAboardDoesNotBuyAFreeTransfer
// plants exactly that arrangement — a sailing that calls at a hub, and a second
// sailing leaving that hub inside the transfer window — and fails if the
// connection is offered.
//
// A traveler DISEMBARKS by an edge from the arrival node to the wait node at
// arrival-plus-transfer, which is how the transfer time is paid exactly once
// and only by those who actually get off.
func (p prepared) buildGraph() (*graph, int) {
	waitAt := make(map[vocab.ID][]int64)
	addWait := func(port vocab.ID, at int64) { waitAt[port] = append(waitAt[port], at) }

	for _, inst := range p.insts {
		trip := &p.feed.Trips[inst.tripIdx]
		last := len(trip.StopTimes) - 1
		for i, st := range trip.StopTimes {
			// No arrival node at the first call and no departure node at the
			// last: nobody can step off a sailing before it has gone anywhere,
			// and nobody can board one that is finished. Creating them would
			// add nodes no edge can reach, which is harmless until somebody
			// tries to reason about the node count.
			if i > 0 {
				addWait(st.PortID, inst.arrive[i]+int64(p.minTransfer[st.PortID]))
			}
			if i < last {
				addWait(st.PortID, inst.depart[i])
			}
		}
	}
	addWait(p.origin, p.departAt)

	g := &graph{}
	waitIdx := make(map[portTime]int)

	// Ports are visited in sorted order and instants in sorted order, so node
	// numbering is a function of the input alone. The search breaks its final
	// tie on node index, so unstable numbering would silently produce different
	// itineraries of identical cost between two runs on the same schedule.
	ports := make([]vocab.ID, 0, len(waitAt))
	for id := range waitAt {
		ports = append(ports, id)
	}
	slices.Sort(ports)

	for _, port := range ports {
		times := waitAt[port]
		slices.Sort(times)
		times = slices.Compact(times)
		prev := noNode
		for _, at := range times {
			idx := g.addNode(node{kind: kindWait, at: at, port: port, inst: noNode, stop: noNode})
			waitIdx[portTime{port: port, at: at}] = idx
			if prev != noNode {
				g.addArc(prev, idx, 0)
			}
			prev = idx
		}
	}

	for ii, inst := range p.insts {
		trip := &p.feed.Trips[inst.tripIdx]
		last := len(trip.StopTimes) - 1
		depNode := make([]int, len(trip.StopTimes))
		arrNode := make([]int, len(trip.StopTimes))
		for i := range trip.StopTimes {
			depNode[i], arrNode[i] = noNode, noNode
		}
		for i, st := range trip.StopTimes {
			if i < last {
				depNode[i] = g.addNode(node{kind: kindDepart, at: inst.depart[i], port: st.PortID, inst: ii, stop: i})
			}
			if i > 0 {
				arrNode[i] = g.addNode(node{kind: kindArrive, at: inst.arrive[i], port: st.PortID, inst: ii, stop: i})
			}
		}
		for i, st := range trip.StopTimes {
			if i < last {
				// Boarding: the only edge that costs a leg.
				g.addArc(waitIdx[portTime{port: st.PortID, at: inst.depart[i]}], depNode[i], 1)
				// Riding.
				g.addArc(depNode[i], arrNode[i+1], 0)
			}
			if i > 0 {
				// Disembarking, which is what pays the transfer time.
				ready := inst.arrive[i] + int64(p.minTransfer[st.PortID])
				g.addArc(arrNode[i], waitIdx[portTime{port: st.PortID, at: ready}], 0)
				// Staying aboard through an intermediate call. Free, and
				// deliberately NOT via the wait chain.
				if i < last {
					g.addArc(arrNode[i], depNode[i], 0)
				}
			}
		}
	}

	return g, waitIdx[portTime{port: p.origin, at: p.departAt}]
}

// label is one entry in the search frontier. The ordering of these values IS
// the lexicographic cost function.
type label struct {
	at   int64
	legs int
	node int
}

type labelHeap []label

func (h labelHeap) Len() int { return len(h) }

// Less orders by arrival instant, then leg count, then node index.
//
// The third component decides nothing about the COST of the answer — two
// candidates that reach it are already equal on both real components. It is
// there so that "equal" resolves the same way every run, which turns an
// arbitrary choice among tied itineraries into a reproducible one.
func (h labelHeap) Less(i, j int) bool {
	if h[i].at != h[j].at {
		return h[i].at < h[j].at
	}
	if h[i].legs != h[j].legs {
		return h[i].legs < h[j].legs
	}
	return h[i].node < h[j].node
}

func (h labelHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *labelHeap) Push(x any) { *h = append(*h, x.(label)) }

func (h *labelHeap) Pop() any {
	old := *h
	n := len(old)
	last := old[n-1]
	*h = old[:n-1]
	return last
}

// search is Dijkstra over the expanded graph, stopping at the first goal node
// it settles.
//
// # Why stopping there is right
//
// The frontier is ordered by the full cost pair, so the first goal node popped
// is the lexicographically smallest goal node reachable at all. Every edge here
// moves forward in time by a non-negative amount and adds a non-negative number
// of legs, so a node's key never decreases along a path, which is exactly the
// condition Dijkstra needs — and, as the package comment sets out, is a
// property of the expansion rather than of FIFO, which this network does not
// have.
//
// Only the leg count is stored per node, because the arrival instant is a
// property of the node itself: every path that reaches a given node reaches it
// at the same moment. That is the degeneracy the second cost component exists
// to break, and it is also what makes the relaxation test a comparison of leg
// counts alone.
func (g *graph) search(src int, isGoal func(node) bool) (int, []int, bool) {
	legs := make([]int, len(g.nodes))
	parent := make([]int, len(g.nodes))
	for i := range legs {
		legs[i] = math.MaxInt
		parent[i] = noNode
	}
	legs[src] = 0

	settled := make([]bool, len(g.nodes))
	frontier := &labelHeap{{at: g.nodes[src].at, legs: 0, node: src}}
	heap.Init(frontier)

	for frontier.Len() > 0 {
		// A direct assertion rather than the comma-ok form, deliberately.
		// labelHeap holds nothing but labels and Push is the only way in, so
		// the failing arm cannot be reached — and an unreachable arm is a
		// branch no test can cover, which is how coverage exclusion lists get
		// started. The root package makes the same trade in the same direction.
		cur := heap.Pop(frontier).(label)
		if settled[cur.node] {
			continue
		}
		settled[cur.node] = true
		if isGoal(g.nodes[cur.node]) {
			return cur.node, parent, true
		}
		for _, a := range g.arcs[cur.node] {
			next := cur.legs + a.legs
			if next >= legs[a.to] {
				continue
			}
			legs[a.to] = next
			parent[a.to] = cur.node
			heap.Push(frontier, label{at: g.nodes[a.to].at, legs: next, node: a.to})
		}
	}
	return noNode, parent, false
}

// pathTo walks the parent array back from a settled node and returns the node
// sequence in travel order.
func pathTo(parent []int, goal int) []int {
	var reversed []int
	for n := goal; n != noNode; n = parent[n] {
		reversed = append(reversed, n)
	}
	slices.Reverse(reversed)
	return reversed
}

// legsAlong collapses a node path into rides.
//
// A leg opens at a departure node entered from a WAIT node, which is the only
// way of boarding, and closes at an arrival node the traveler leaves for a wait
// node — or at the end of the path. Everything between is the same vessel, so a
// sailing that calls at four ports on the way is one leg and not four.
func (g *graph) legsAlong(path []int) []rawLeg {
	var out []rawLeg
	board := noNode
	for i, idx := range path {
		n := g.nodes[idx]
		switch n.kind {
		case kindDepart:
			if g.nodes[path[i-1]].kind == kindWait {
				board = idx
			}
		case kindArrive:
			if i == len(path)-1 || g.nodes[path[i+1]].kind == kindWait {
				out = append(out, rawLeg{inst: n.inst, board: g.nodes[board].stop, alight: n.stop})
			}
		}
	}
	return out
}

// EarliestArrival answers the query: the earliest instant the traveler can
// reach the destination, and — among the ways of arriving then — the one with
// the fewest legs.
func EarliestArrival(q Query) (Itinerary, error) {
	p, err := prepare(q)
	if err != nil {
		return Itinerary{}, err
	}
	if p.origin == p.dest {
		return p.itinerary(nil), nil
	}
	g, src := p.buildGraph()
	goal, parent, found := g.search(src, func(n node) bool {
		return n.kind == kindArrive && n.port == p.dest
	})
	if !found {
		return Itinerary{}, ErrNoConnection
	}
	return p.itinerary(g.legsAlong(pathTo(parent, goal))), nil
}
