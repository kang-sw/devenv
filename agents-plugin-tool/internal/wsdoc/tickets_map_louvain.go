package wsdoc

import (
	"math"
	"slices"
)

// Louvain community detection and the ticket map's γ sweep
// (tickets_map_cluster.go): deterministic local moving, aggregation,
// splitOversized, absorbTiny, and the partition score.

const (
	lvMaxLevels   = 10    // aggregation levels per Louvain run
	lvMaxPasses   = 256   // local-moving passes per level (a safety bound)
	lvMoveEpsilon = 1e-12 // a move needs gain > best + lvMoveEpsilon
	lvSplitRounds = 6
	lvSplitShare  = 0.85 // a split's largest part must be under this share
	lvAbsorbIters = 10
	lvAbsorbSim   = 0.5 // weight of similarity in absorbTiny's connection
)

// lvSplitFactors scale γ for the re-runs that split an oversized cluster.
var lvSplitFactors = [...]float64{1.3, 1.7, 2.2, 3.0}

// lvGraph is an undirected weighted graph in CSR form. Adjacency lists hold
// no self-loop and are sorted by neighbor; self holds each node's self-loop
// weight (an aggregated community's internal weight), counted twice in deg.
type lvGraph struct {
	off  []int32
	nbr  []int32
	wt   []float64
	self []float64
	deg  []float64
	m2   float64 // 2m: the sum of deg
}

func (g *lvGraph) n() int { return len(g.deg) }

// lvPair is one undirected edge u < v.
type lvPair struct {
	u, v int32
	w    float64
}

// newLvGraph builds a graph of n nodes from undirected pairs sorted by (u, v)
// with u < v, so every adjacency list comes out sorted.
func newLvGraph(n int, pairs []lvPair) *lvGraph {
	g := &lvGraph{
		off:  make([]int32, n+1),
		nbr:  make([]int32, 2*len(pairs)),
		wt:   make([]float64, 2*len(pairs)),
		self: make([]float64, n),
		deg:  make([]float64, n),
	}
	for _, p := range pairs {
		g.off[p.u+1]++
		g.off[p.v+1]++
	}
	for i := 0; i < n; i++ {
		g.off[i+1] += g.off[i]
	}
	fill := slices.Clone(g.off[:n])
	for _, p := range pairs {
		g.nbr[fill[p.u]], g.wt[fill[p.u]] = p.v, p.w
		fill[p.u]++
		g.nbr[fill[p.v]], g.wt[fill[p.v]] = p.u, p.w
		fill[p.v]++
	}
	g.finish()
	return g
}

// finish computes deg and m2 from the adjacency and self-loops.
func (g *lvGraph) finish() {
	g.m2 = 0
	for u := range g.deg {
		d := 0.0
		for e := g.off[u]; e < g.off[u+1]; e++ {
			d += g.wt[e]
		}
		g.deg[u] = d + 2*g.self[u]
		g.m2 += g.deg[u]
	}
}

// induced returns the subgraph on members (ascending node indices).
func (g *lvGraph) induced(members []int32) *lvGraph {
	local := make(map[int32]int32, len(members))
	for i, u := range members {
		local[u] = int32(i)
	}
	var pairs []lvPair
	for i, u := range members {
		for e := g.off[u]; e < g.off[u+1]; e++ {
			if j, ok := local[g.nbr[e]]; ok && j > int32(i) {
				pairs = append(pairs, lvPair{u: int32(i), v: j, w: g.wt[e]})
			}
		}
	}
	return newLvGraph(len(members), pairs)
}

// louvain partitions g at resolution gamma and returns each node's community,
// relabeled by size descending, then smallest member index. Nodes are
// visited in index order and candidate communities in ascending id order.
func louvain(g *lvGraph, gamma float64) []int32 {
	final := make([]int32, g.n())
	for i := range final {
		final[i] = int32(i)
	}
	if g.m2 <= 0 {
		return final
	}
	cur := g
	for level := 0; level < lvMaxLevels; level++ {
		comm, moved := lvMove(cur, gamma)
		if !moved {
			break
		}
		nc := lvRelabelFirst(comm)
		for i, c := range final {
			final[i] = comm[c]
		}
		if nc == cur.n() {
			break
		}
		cur = lvAggregate(cur, comm, nc)
	}
	return lvRelabelSize(final)
}

// lvMove runs local moving to convergence from singletons and reports whether
// any node changed community.
func lvMove(g *lvGraph, gamma float64) ([]int32, bool) {
	n := g.n()
	comm := make([]int32, n)
	tot := make([]float64, n)
	link := make([]float64, n)
	seen := make([]bool, n)
	touched := make([]int32, 0, 16)
	for i := range comm {
		comm[i] = int32(i)
		tot[i] = g.deg[i]
	}
	inv := 2 / g.m2 // 1/m: gains are modularity deltas
	movedAny := false
	for pass := 0; pass < lvMaxPasses; pass++ {
		moved := false
		for u := 0; u < n; u++ {
			cu, ku := comm[u], g.deg[u]
			touched = touched[:0]
			for e := g.off[u]; e < g.off[u+1]; e++ {
				c := comm[g.nbr[e]]
				if !seen[c] {
					seen[c] = true
					touched = append(touched, c)
				}
				link[c] += g.wt[e]
			}
			tot[cu] -= ku
			best := (link[cu] - gamma*tot[cu]*ku/g.m2) * inv
			bestC := cu
			slices.Sort(touched)
			for _, c := range touched {
				if c == cu {
					continue
				}
				if gain := (link[c] - gamma*tot[c]*ku/g.m2) * inv; gain > best+lvMoveEpsilon {
					best, bestC = gain, c
				}
			}
			tot[bestC] += ku
			comm[u] = bestC
			for _, c := range touched {
				link[c], seen[c] = 0, false
			}
			if bestC != cu {
				moved = true
			}
		}
		if !moved {
			break
		}
		movedAny = true
	}
	return comm, movedAny
}

// lvRelabelFirst relabels communities 0..k-1 by first appearance in node
// order, in place, and returns k.
func lvRelabelFirst(comm []int32) int {
	ids := make([]int32, len(comm))
	for i := range ids {
		ids[i] = -1
	}
	k := int32(0)
	for i, c := range comm {
		if ids[c] < 0 {
			ids[c] = k
			k++
		}
		comm[i] = ids[c]
	}
	return int(k)
}

// lvAggregate collapses each community of g into one node: inter-community
// weights summed, internal weight (each undirected edge once) as a self-loop.
func lvAggregate(g *lvGraph, comm []int32, nc int) *lvGraph {
	n := g.n()
	start := make([]int32, nc+1)
	for _, c := range comm {
		start[c+1]++
	}
	for c := 0; c < nc; c++ {
		start[c+1] += start[c]
	}
	members := make([]int32, n)
	fill := slices.Clone(start[:nc])
	for u, c := range comm {
		members[fill[c]] = int32(u)
		fill[c]++
	}
	out := &lvGraph{off: make([]int32, nc+1), self: make([]float64, nc), deg: make([]float64, nc)}
	acc := make([]float64, nc)
	seen := make([]bool, nc)
	touched := make([]int32, 0, 16)
	for c := int32(0); int(c) < nc; c++ {
		touched = touched[:0]
		for _, u := range members[start[c]:start[c+1]] {
			out.self[c] += g.self[u]
			for e := g.off[u]; e < g.off[u+1]; e++ {
				v := g.nbr[e]
				cv := comm[v]
				if cv == c {
					if u < v {
						out.self[c] += g.wt[e]
					}
					continue
				}
				if !seen[cv] {
					seen[cv] = true
					touched = append(touched, cv)
				}
				acc[cv] += g.wt[e]
			}
		}
		slices.Sort(touched)
		for _, cv := range touched {
			out.nbr = append(out.nbr, cv)
			out.wt = append(out.wt, acc[cv])
			acc[cv], seen[cv] = 0, false
		}
		out.off[c+1] = int32(len(out.nbr))
	}
	out.finish()
	return out
}

// lvRelabelSize relabels a partition (ids in [0, len)) by size descending,
// then smallest member index, and returns the new labels.
func lvRelabelSize(part []int32) []int32 {
	n := len(part)
	size := make([]int32, n)
	first := make([]int32, n)
	for i := range first {
		first[i] = -1
	}
	for i, c := range part {
		size[c]++
		if first[c] < 0 {
			first[c] = int32(i)
		}
	}
	ids := make([]int32, 0, 16)
	for c := range size {
		if size[c] > 0 {
			ids = append(ids, int32(c))
		}
	}
	slices.SortFunc(ids, func(a, b int32) int {
		if size[a] != size[b] {
			return int(size[b] - size[a])
		}
		return int(first[a] - first[b])
	})
	newID := make([]int32, n)
	for k, c := range ids {
		newID[c] = int32(k)
	}
	out := make([]int32, n)
	for i, c := range part {
		out[i] = newID[c]
	}
	return out
}

// lvCount returns the number of communities and each one's size for a
// partition labeled 0..k-1.
func lvCount(part []int32) (int, []int) {
	k := 0
	for _, c := range part {
		if int(c)+1 > k {
			k = int(c) + 1
		}
	}
	sizes := make([]int, k)
	for _, c := range part {
		sizes[c]++
	}
	return k, sizes
}

// lvModularity is the standard (γ = 1) modularity of part on g.
func lvModularity(g *lvGraph, part []int32) float64 {
	if g.m2 <= 0 {
		return 0
	}
	k, _ := lvCount(part)
	in := make([]float64, k)
	tot := make([]float64, k)
	for u := range g.deg {
		c := part[u]
		tot[c] += g.deg[u]
		in[c] += 2 * g.self[u]
		for e := g.off[u]; e < g.off[u+1]; e++ {
			if part[g.nbr[e]] == c {
				in[c] += g.wt[e]
			}
		}
	}
	q := 0.0
	for c := 0; c < k; c++ {
		q += in[c]/g.m2 - (tot[c]/g.m2)*(tot[c]/g.m2)
	}
	return q
}

// tmClusterer runs the γ sweep over the core graph: the open tickets outside
// Misc, indexed in node order.
type tmClusterer struct {
	g       *lvGraph
	minComm int
	maxComm int
	targetK [2]int
	// absorb lists each core node's top open neighbors (by core index) with
	// their similarity, for absorbTiny's connection.
	absorb    [][]int32
	absorbSim [][]float64
}

// sweep runs every γ from 0.4 to 3.0 in steps of 0.1 and keeps the partition
// with the highest (score, modularity); a tie keeps the lower γ.
func (cl *tmClusterer) sweep() (part []int32, gamma, modularity float64) {
	bestScore := math.Inf(-1)
	for i := 0; i <= 26; i++ {
		gm := float64(4+i) / 10
		p := cl.run(gm)
		s := cl.score(p)
		q := lvModularity(cl.g, p)
		if s > bestScore || (s == bestScore && q > modularity) {
			bestScore, part, gamma, modularity = s, p, gm, q
		}
	}
	return part, gamma, modularity
}

// run is louvain → splitOversized → absorbTiny → splitOversized → absorbTiny.
func (cl *tmClusterer) run(gamma float64) []int32 {
	part := louvain(cl.g, gamma)
	part = cl.splitOversized(part, gamma)
	part = cl.absorbTiny(part)
	part = cl.splitOversized(part, gamma)
	return cl.absorbTiny(part)
}

// splitOversized re-runs Louvain on each cluster larger than maxComm at
// γ×{1.3, 1.7, 2.2, 3.0}, accepting the first result with at least two
// communities whose largest is under 0.85 of the cluster.
func (cl *tmClusterer) splitOversized(part []int32, gamma float64) []int32 {
	for round := 0; round < lvSplitRounds; round++ {
		k, sizes := lvCount(part)
		next := int32(k)
		split := false
		for c := 0; c < k; c++ {
			if sizes[c] <= cl.maxComm {
				continue
			}
			members := make([]int32, 0, sizes[c])
			for u, pc := range part {
				if int(pc) == c {
					members = append(members, int32(u))
				}
			}
			sub := cl.g.induced(members)
			for _, f := range lvSplitFactors {
				r := louvain(sub, gamma*f)
				rk, rs := lvCount(r)
				if rk < 2 || float64(rs[0]) >= lvSplitShare*float64(len(members)) {
					continue
				}
				for i, u := range members {
					if r[i] > 0 {
						part[u] = next + r[i] - 1
					}
				}
				next += int32(rk - 1)
				split = true
				break
			}
		}
		part = lvRelabelSize(part)
		if !split {
			break
		}
	}
	return part
}

// absorbTiny merges each cluster under minComm into the neighbor cluster with
// the highest connection: graph edge weight plus 0.5 × the similarity to each
// member's top open neighbors. Ties go to the lowest cluster id.
func (cl *tmClusterer) absorbTiny(part []int32) []int32 {
	part = slices.Clone(part)
	for round := 0; round < lvAbsorbIters; round++ {
		k, sizes := lvCount(part)
		conn := make([]float64, k)
		merged := false
		for c := int32(0); int(c) < k; c++ {
			if sizes[c] == 0 || sizes[c] >= cl.minComm {
				continue
			}
			clear(conn)
			for u, pc := range part {
				if pc != c {
					continue
				}
				for e := cl.g.off[u]; e < cl.g.off[u+1]; e++ {
					if d := part[cl.g.nbr[e]]; d != c {
						conn[d] += cl.g.wt[e]
					}
				}
				for x, v := range cl.absorb[u] {
					if d := part[v]; d != c {
						conn[d] += lvAbsorbSim * cl.absorbSim[u][x]
					}
				}
			}
			best, target := 0.0, int32(-1)
			for d, w := range conn {
				if w > best {
					best, target = w, int32(d)
				}
			}
			if target < 0 {
				continue
			}
			for u, pc := range part {
				if pc == c {
					part[u] = target
				}
			}
			sizes[target] += sizes[c]
			sizes[c] = 0
			merged = true
		}
		part = lvRelabelSize(part)
		if !merged {
			break
		}
	}
	return part
}

// score is (size entropy / ln k) − 2·(distance of k outside targetK) −
// max(0, largest − maxComm)/6.
func (cl *tmClusterer) score(part []int32) float64 {
	k, sizes := lvCount(part)
	if k == 0 {
		return math.Inf(-1)
	}
	n := float64(len(part))
	h, largest := 0.0, 0
	for _, s := range sizes {
		p := float64(s) / n
		h -= p * math.Log(p)
		largest = max(largest, s)
	}
	ent := 0.0
	if k > 1 {
		ent = h / math.Log(float64(k))
	}
	dist := 0
	if k < cl.targetK[0] {
		dist = cl.targetK[0] - k
	} else if k > cl.targetK[1] {
		dist = k - cl.targetK[1]
	}
	return ent - 2*float64(dist) - float64(max(0, largest-cl.maxComm))/6
}
