package wsdoc

import (
	"math"
	"slices"
	"strings"
)

// The ticket map's analysis: corpus lemmatization, TF-IDF vectors,
// similarity, the open-ticket graph, clustering (tickets_map_louvain.go),
// closed-ref placement, then cluster order, names and the Keyword Soup
// (tickets_map_naming.go). Every constant below is the ticket map contract.
const (
	tmDFCutoff       = 0.32 // cut terms with df_open/N_open above this
	tmTopK           = 70   // vector terms kept per ticket
	tmSimNeighbor    = 0.08 // least score of a listed neighbor
	tmSimGraph       = 0.16 // least score of a semantic graph edge
	tmSimMisc        = 0.12 // an edgeless ticket under this best score is Misc
	tmMentionBoost   = 0.18
	tmMentionCap     = 0.98
	tmNeighborKeep   = 25
	tmGraphNeighbors = 4
	tmSemScale       = 3.5
	tmSharedTerms    = 4
	tmKWTerms        = 6
	tmDFCutTopMax    = 10
	tmSmallBoard     = 8 // under this many open tickets Louvain is skipped
	tmRefNeighbors   = 8
	tmAbsorbNbrs     = 6
	tmNameCands      = 8
	tmNameTerms      = 3
	tmNameCDF        = 0.60
	tmSoupCands      = 40
	tmSoupCDF        = 0.50
	tmSoupMax        = 10
	tmSoupCodeMax    = 2
	tmHueOffset      = 205
)

// tmRelWeight is an explicit relation's weight, in the graph and in a closed
// ref's cluster score.
func tmRelWeight(rel string) float64 {
	switch rel {
	case "parent":
		return 4.0
	case "blocked-by":
		return 3.2
	}
	return 2.2 // related
}

// tmStatusWeight is an open ticket's active weight for cluster order.
func tmStatusWeight(status string) float64 {
	switch status {
	case "ready":
		return 3
	case "todo":
		return 1.5
	case "idea":
		return 0.6
	}
	return 0
}

// mapAnalysis is analyzeTicketMap's output, indexed like its nodes argument.
type mapAnalysis struct {
	cluster      []int                         // per node: its cluster id
	kw           [][]TicketMapTerm             // per node: top-6 vector terms
	clusters     []TicketMapCluster            // ids 0..K-1, Misc last
	keywords     map[string][]TicketMapKeyword // cluster id (decimal) -> soup
	sim          []TicketMapSim                // sorted by (I, J)
	mentionPairs int
	gamma        float64 // raw; the assembler rounds to 1 decimal
	modularity   float64 // raw; the assembler rounds to 3 decimals
	dfCut        int
	dfCutTop     []string
	params       TicketMapParams
}

// tmTermEntry is one ticket's lemma-merged stat of a term id.
type tmTermEntry struct {
	id int32
	st termStat
}

type tmVecEntry struct {
	id int32
	w  float64
}

// tmCorpus carries the analysis state. Term ids index the sorted raw
// vocabulary, so id order is lexicographic term order.
type tmCorpus struct {
	nodes []mapNode
	edges []mapEdge
	n     int
	nOpen int

	vocab []string
	index map[string]int32
	lemma []int32 // raw id -> its lemma's id (itself when none)
	code  []bool  // per id: isCodeTerm
	kept  []bool  // per id: survives the df cut

	stats [][]tmTermEntry // per node, lemma-merged, sorted by id
	vecs  [][]tmVecEntry  // per node, sorted by id, L2-normalized
	kw    [][]TicketMapTerm

	sim      []float64 // dense n×n, mention boost applied
	mention  map[[2]int32]bool
	explicit map[[2]int32]bool
	nbrs     [][]int32 // per node: top-25 neighbors
}

func (c *tmCorpus) s(i, j int32) float64 { return c.sim[int(i)*c.n+int(j)] }

func tmPairKey(i, j int32) [2]int32 {
	if i > j {
		i, j = j, i
	}
	return [2]int32{i, j}
}

// tmParams scales the clustering parameters to nOpen.
func tmParams(nOpen int) TicketMapParams {
	minComm := 2
	if nOpen >= 40 {
		minComm = 4
	} else if nOpen >= 16 {
		minComm = 3
	}
	root := math.Sqrt(float64(nOpen))
	return TicketMapParams{
		WTitle: tmWTitle, WCode: tmWCode, WProse: tmWProse,
		DFCutoff: tmDFCutoff, TopK: tmTopK, SimNeighbor: tmSimNeighbor, SimGraph: tmSimGraph,
		MinComm: minComm,
		MaxComm: max(12, int(math.Ceil(0.21*float64(nOpen)))),
		TargetK: [2]int{
			min(max(int(math.Round(root/2)), 2), 7),
			min(max(int(math.Round(0.9*root)), 3), 12),
		},
	}
}

// analyzeTicketMap runs vectors, similarity, the graph, Louvain, naming and
// the Keyword Soup over the included nodes (open first, then closed refs) and
// their explicit edges. It is deterministic in its inputs: no map iteration
// order reaches the output, and every float sum runs in a fixed order.
func analyzeTicketMap(nodes []mapNode, edges []mapEdge) mapAnalysis {
	c := &tmCorpus{nodes: nodes, edges: edges, n: len(nodes)}
	for _, node := range nodes {
		if node.open {
			c.nOpen++
		}
	}
	out := mapAnalysis{
		cluster:  make([]int, len(nodes)),
		clusters: []TicketMapCluster{},
		keywords: map[string][]TicketMapKeyword{},
		sim:      []TicketMapSim{},
		dfCutTop: []string{},
		params:   tmParams(c.nOpen),
	}
	if c.n == 0 {
		return out
	}
	c.buildVocab()
	c.buildStats()
	out.dfCut, out.dfCutTop = c.cutAndCode()
	c.buildVectors()
	out.kw = c.kw
	out.mentionPairs = c.buildSimilarity()
	c.buildNeighbors()
	out.sim = c.simRows()

	part, misc := c.cluster(&out)
	c.finishClusters(&out, part, misc)
	return out
}

// buildVocab collects every term key of every document, sorted, and the
// corpus-driven lemma of each pure [a-z]+ term.
func (c *tmCorpus) buildVocab() {
	seen := map[string]struct{}{}
	for _, node := range c.nodes {
		for k := range node.doc.Terms {
			seen[k] = struct{}{}
		}
	}
	c.vocab = make([]string, 0, len(seen))
	for k := range seen {
		c.vocab = append(c.vocab, k)
	}
	slices.Sort(c.vocab)
	c.index = make(map[string]int32, len(c.vocab))
	for i, k := range c.vocab {
		c.index[k] = int32(i)
	}
	c.lemma = tmBuildLemma(c.vocab, c.index)
}

// tmBuildLemma maps each term id to its lemma's id. Iterating the vocabulary
// in lexicographic order, a pure [a-z]+ word takes the first candidate
// (tmLemmaCandidates) present in the vocabulary and not a stopword; chains
// resolve at most 3 hops.
func tmBuildLemma(vocab []string, index map[string]int32) []int32 {
	next := make([]int32, len(vocab))
	var buf [8]string
	for id, w := range vocab {
		next[id] = int32(id)
		if !tmPureLower(w) {
			continue
		}
		for _, cand := range tmLemmaCandidates(w, buf[:0]) {
			if _, stop := enStop[cand]; stop {
				continue
			}
			if cid, ok := index[cand]; ok {
				next[id] = cid
				break
			}
		}
	}
	lemma := make([]int32, len(vocab))
	for id := range vocab {
		l := int32(id)
		for hop := 0; hop < 3 && next[l] != l; hop++ {
			l = next[l]
		}
		lemma[id] = l
	}
	return lemma
}

func tmPureLower(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 'a' || s[i] > 'z' {
			return false
		}
	}
	return true
}

// tmLemmaCandidates lists w's lemma candidates in contract order: `ies`→`y`
// (length > 5); `es` stripped (length > 5); `s` stripped (not ss/us/is,
// length > 4); then for `ing` and `ed` with a base of at least 4: the base,
// base+`e`, the de-doubled base, and a final `i`→`y`.
func tmLemmaCandidates(w string, out []string) []string {
	n := len(w)
	if n > 5 && strings.HasSuffix(w, "ies") {
		out = append(out, w[:n-3]+"y")
	}
	if n > 5 && strings.HasSuffix(w, "es") {
		out = append(out, w[:n-2])
	}
	if n > 4 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") &&
		!strings.HasSuffix(w, "us") && !strings.HasSuffix(w, "is") {
		out = append(out, w[:n-1])
	}
	for _, suf := range [...]string{"ing", "ed"} {
		if !strings.HasSuffix(w, suf) {
			continue
		}
		base := w[:n-len(suf)]
		if len(base) < 4 {
			continue
		}
		out = append(out, base, base+"e")
		if b := len(base); base[b-1] == base[b-2] {
			out = append(out, base[:b-1])
		}
		if b := len(base); base[b-1] == 'i' {
			out = append(out, base[:b-1]+"y")
		}
	}
	return out
}

// buildStats merges each document's term stats by lemma into fresh entries
// (the shared document stats are never written), summing in raw id order.
func (c *tmCorpus) buildStats() {
	scratch := make([]termStat, len(c.vocab))
	used := make([]bool, len(c.vocab))
	var ids, touched []int32
	c.stats = make([][]tmTermEntry, c.n)
	for i, node := range c.nodes {
		ids = ids[:0]
		for k := range node.doc.Terms {
			ids = append(ids, c.index[k])
		}
		slices.Sort(ids)
		touched = touched[:0]
		for _, id := range ids {
			st := node.doc.Terms[c.vocab[id]]
			l := c.lemma[id]
			if !used[l] {
				used[l] = true
				touched = append(touched, l)
			}
			acc := &scratch[l]
			for ch := 0; ch < 4; ch++ {
				acc.w[ch] += st.w[ch]
				acc.n[ch] += st.n[ch]
			}
		}
		slices.Sort(touched)
		entries := make([]tmTermEntry, len(touched))
		for k, l := range touched {
			entries[k] = tmTermEntry{id: l, st: scratch[l]}
			scratch[l], used[l] = termStat{}, false
		}
		c.stats[i] = entries
	}
}

// cutAndCode classifies every lemma term: isCodeTerm, and the df cut
// (df_open/N_open > 0.32, or df_all < 2). It returns the high-df cut count and
// its top terms by df_open.
func (c *tmCorpus) cutAndCode() (int, []string) {
	V := len(c.vocab)
	dfOpen := make([]int32, V)
	dfAll := make([]int32, V)
	codeN := make([]int64, V)
	otherN := make([]int64, V)
	for i, entries := range c.stats {
		for _, e := range entries {
			dfAll[e.id]++
			if c.nodes[i].open {
				dfOpen[e.id]++
			}
			codeN[e.id] += int64(e.st.n[tmChCode])
			otherN[e.id] += int64(e.st.n[tmChTitle]) + int64(e.st.n[tmChHeading]) + int64(e.st.n[tmChProse])
		}
	}
	c.code = make([]bool, V)
	c.kept = make([]bool, V)
	var cut []int32
	for id := range c.vocab {
		if dfAll[id] == 0 {
			continue
		}
		c.code[id] = strings.ContainsAny(c.vocab[id], "./_#") || 2*codeN[id] >= otherN[id]
		high := c.nOpen > 0 && float64(dfOpen[id])/float64(c.nOpen) > tmDFCutoff
		if high {
			cut = append(cut, int32(id))
		}
		c.kept[id] = !high && dfAll[id] >= 2
	}
	slices.SortStableFunc(cut, func(a, b int32) int { return int(dfOpen[b] - dfOpen[a]) })
	top := make([]string, 0, tmDFCutTopMax)
	for _, id := range cut[:min(len(cut), tmDFCutTopMax)] {
		top = append(top, c.vocab[id])
	}
	return len(cut), top
}

// buildVectors computes each node's sublinear TF-IDF vector over the kept
// terms (f is the channel-weighted frequency), keeps the top 70 by weight
// (ties by term), L2-normalizes it, and records the top 6 terms as kw.
func (c *tmCorpus) buildVectors() {
	dfAll := make([]int32, len(c.vocab))
	for _, entries := range c.stats {
		for _, e := range entries {
			dfAll[e.id]++
		}
	}
	nAll := float64(c.n)
	c.vecs = make([][]tmVecEntry, c.n)
	c.kw = make([][]TicketMapTerm, c.n)
	var buf []tmVecEntry
	for i, entries := range c.stats {
		buf = buf[:0]
		for _, e := range entries {
			if !c.kept[e.id] {
				continue
			}
			f := e.st.w[0] + e.st.w[1] + e.st.w[2] + e.st.w[3]
			tfw := f
			if f >= 1 {
				tfw = 1 + math.Log(f)
			}
			idf := math.Log((nAll+1)/(float64(dfAll[e.id])+1)) + 1
			buf = append(buf, tmVecEntry{id: e.id, w: tfw * idf})
		}
		slices.SortFunc(buf, func(a, b tmVecEntry) int {
			if a.w != b.w {
				if a.w > b.w {
					return -1
				}
				return 1
			}
			return int(a.id - b.id)
		})
		buf = buf[:min(len(buf), tmTopK)]
		kw := make([]TicketMapTerm, 0, tmKWTerms)
		for _, e := range buf[:min(len(buf), tmKWTerms)] {
			kw = append(kw, TicketMapTerm{Term: c.vocab[e.id], Code: c.code[e.id]})
		}
		c.kw[i] = kw
		vec := slices.Clone(buf)
		slices.SortFunc(vec, func(a, b tmVecEntry) int { return int(a.id - b.id) })
		norm := 0.0
		for _, e := range vec {
			norm += e.w * e.w
		}
		if norm > 0 {
			norm = math.Sqrt(norm)
			for k := range vec {
				vec[k].w /= norm
			}
		}
		c.vecs[i] = vec
	}
}

// buildSimilarity computes cosine similarity through an inverted index
// (terms in sorted order, postings in node order) and applies the body-mention
// boost; it returns the number of boosted pairs.
func (c *tmCorpus) buildSimilarity() int {
	V := len(c.vocab)
	off := make([]int32, V+1)
	for _, vec := range c.vecs {
		for _, e := range vec {
			off[e.id+1]++
		}
	}
	for t := 0; t < V; t++ {
		off[t+1] += off[t]
	}
	postNode := make([]int32, off[V])
	postW := make([]float64, off[V])
	fill := slices.Clone(off[:V])
	for i, vec := range c.vecs {
		for _, e := range vec {
			postNode[fill[e.id]], postW[fill[e.id]] = int32(i), e.w
			fill[e.id]++
		}
	}
	n := c.n
	c.sim = make([]float64, n*n)
	for i, vec := range c.vecs {
		row := c.sim[i*n : (i+1)*n]
		for _, e := range vec {
			for p := off[e.id]; p < off[e.id+1]; p++ {
				if j := postNode[p]; int(j) > i {
					row[j] += e.w * postW[p]
				}
			}
		}
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			c.sim[j*n+i] = c.sim[i*n+j]
		}
	}

	c.explicit = map[[2]int32]bool{}
	for _, e := range c.edges {
		c.explicit[tmPairKey(int32(e.from), int32(e.to))] = true
	}
	index := make(map[string]int32, n)
	for i, node := range c.nodes {
		index[node.info.Stem] = int32(i)
	}
	c.mention = map[[2]int32]bool{}
	var pairs [][2]int32
	for i, node := range c.nodes {
		for _, stem := range node.doc.Mentions {
			j, ok := index[stem]
			if !ok || int(j) == i {
				continue
			}
			key := tmPairKey(int32(i), j)
			if c.explicit[key] || c.mention[key] {
				continue
			}
			c.mention[key] = true
			pairs = append(pairs, key)
		}
	}
	for _, key := range pairs {
		s := min(tmMentionCap, c.s(key[0], key[1])+tmMentionBoost)
		c.sim[int(key[0])*n+int(key[1])] = s
		c.sim[int(key[1])*n+int(key[0])] = s
	}
	return len(pairs)
}

// buildNeighbors keeps, per node, the nodes scoring at least 0.08, sorted by
// score descending then stem, up to 25.
func (c *tmCorpus) buildNeighbors() {
	c.nbrs = make([][]int32, c.n)
	var buf []int32
	for i := 0; i < c.n; i++ {
		buf = buf[:0]
		row := c.sim[i*c.n : (i+1)*c.n]
		for j, s := range row {
			if j != i && s >= tmSimNeighbor {
				buf = append(buf, int32(j))
			}
		}
		slices.SortFunc(buf, func(a, b int32) int {
			if row[a] != row[b] {
				if row[a] > row[b] {
					return -1
				}
				return 1
			}
			return strings.Compare(c.nodes[a].info.Stem, c.nodes[b].info.Stem)
		})
		c.nbrs[i] = slices.Clone(buf[:min(len(buf), tmNeighborKeep)])
	}
}

// simRows lists every pair where either side is in the other's top 25, sorted
// by (i, j), with its rounded score, shared terms and mention flag.
func (c *tmCorpus) simRows() []TicketMapSim {
	var keys [][2]int32
	for i, list := range c.nbrs {
		for _, j := range list {
			keys = append(keys, tmPairKey(int32(i), j))
		}
	}
	slices.SortFunc(keys, func(a, b [2]int32) int {
		if a[0] != b[0] {
			return int(a[0] - b[0])
		}
		return int(a[1] - b[1])
	})
	keys = slices.Compact(keys)
	rows := make([]TicketMapSim, 0, len(keys))
	for _, k := range keys {
		rows = append(rows, TicketMapSim{
			I: int(k[0]), J: int(k[1]),
			Score:   MapFloat(roundTo(c.s(k[0], k[1]), 3)),
			Shared:  c.sharedTerms(k[0], k[1]),
			Mention: c.mention[k],
		})
	}
	return rows
}

// sharedTerms returns the top 4 terms of two vectors by product (ties by
// term), de-duplicated by base, each with its code flag.
func (c *tmCorpus) sharedTerms(i, j int32) []TicketMapTerm {
	a, b := c.vecs[i], c.vecs[j]
	var both []tmVecEntry
	for x, y := 0, 0; x < len(a) && y < len(b); {
		switch {
		case a[x].id < b[y].id:
			x++
		case a[x].id > b[y].id:
			y++
		default:
			both = append(both, tmVecEntry{id: a[x].id, w: a[x].w * b[y].w})
			x++
			y++
		}
	}
	slices.SortFunc(both, func(p, q tmVecEntry) int {
		if p.w != q.w {
			if p.w > q.w {
				return -1
			}
			return 1
		}
		return int(p.id - q.id)
	})
	out := []TicketMapTerm{}
	var bases []string
	for _, e := range both {
		if len(out) == tmSharedTerms {
			break
		}
		base := tmTermBase(c.vocab[e.id])
		if slices.Contains(bases, base) {
			continue
		}
		bases = append(bases, base)
		out = append(out, TicketMapTerm{Term: c.vocab[e.id], Code: c.code[e.id]})
	}
	return out
}

// tmTermBase strips a leading `#`/`.` marker and a trailing `.ext`.
func tmTermBase(term string) string {
	base := strings.TrimLeft(term, "#.")
	if dot := strings.LastIndexByte(base, '.'); dot > 0 {
		base = base[:dot]
	}
	return base
}

// cluster builds the open-ticket graph, sets Misc aside, partitions the rest
// (a single cluster under 8 open tickets, else the γ sweep), and places the
// closed refs. It returns each node's cluster (-1 for Misc) and whether a
// Misc cluster exists.
func (c *tmCorpus) cluster(out *mapAnalysis) ([]int32, bool) {
	// Open-ticket graph, accumulated per undirected pair in a fixed order:
	// explicit edges in edge order, then semantic edges in node order.
	weights := map[[2]int32]float64{}
	var order [][2]int32
	add := func(i, j int32, w float64) {
		key := tmPairKey(i, j)
		if _, ok := weights[key]; !ok {
			order = append(order, key)
		}
		weights[key] += w
	}
	for _, e := range c.edges {
		if c.nodes[e.from].open && c.nodes[e.to].open {
			add(int32(e.from), int32(e.to), tmRelWeight(e.rel))
		}
	}
	for i := 0; i < c.nOpen; i++ {
		picked := 0
		for _, j := range c.nbrs[i] {
			if picked == tmGraphNeighbors {
				break
			}
			if !c.nodes[j].open || c.s(int32(i), j) < tmSimGraph {
				continue
			}
			add(int32(i), j, c.s(int32(i), j)*tmSemScale/2)
			picked++
		}
	}
	slices.SortFunc(order, func(a, b [2]int32) int {
		if a[0] != b[0] {
			return int(a[0] - b[0])
		}
		return int(a[1] - b[1])
	})
	hasEdge := make([]bool, c.n)
	for _, key := range order {
		hasEdge[key[0]], hasEdge[key[1]] = true, true
	}

	// Misc: open, no graph edge, best neighbor under 0.12. The rest is the core.
	part := make([]int32, c.n)
	coreOf := make([]int32, c.n)
	var core []int32
	misc := false
	for i := 0; i < c.n; i++ {
		part[i], coreOf[i] = -1, -1
		if !c.nodes[i].open {
			continue
		}
		best := 0.0
		if len(c.nbrs[i]) > 0 {
			best = c.s(int32(i), c.nbrs[i][0])
		}
		if !hasEdge[i] && best < tmSimMisc {
			misc = true
			continue
		}
		coreOf[i] = int32(len(core))
		core = append(core, int32(i))
	}

	if len(core) > 0 {
		var corePart []int32
		if c.nOpen < tmSmallBoard {
			corePart = make([]int32, len(core))
		} else {
			pairs := make([]lvPair, 0, len(order))
			for _, key := range order {
				if u, v := coreOf[key[0]], coreOf[key[1]]; u >= 0 && v >= 0 {
					pairs = append(pairs, lvPair{u: u, v: v, w: weights[key]})
				}
			}
			cl := &tmClusterer{
				g:       newLvGraph(len(core), pairs),
				minComm: out.params.MinComm,
				maxComm: out.params.MaxComm,
				targetK: out.params.TargetK,
			}
			cl.absorb = make([][]int32, len(core))
			cl.absorbSim = make([][]float64, len(core))
			for ci, i := range core {
				taken := 0
				for _, j := range c.nbrs[i] {
					if taken == tmAbsorbNbrs {
						break
					}
					if !c.nodes[j].open {
						continue
					}
					taken++
					if coreOf[j] >= 0 {
						cl.absorb[ci] = append(cl.absorb[ci], coreOf[j])
						cl.absorbSim[ci] = append(cl.absorbSim[ci], c.s(i, j))
					}
				}
			}
			corePart, out.gamma, out.modularity = cl.sweep()
		}
		for ci, i := range core {
			part[i] = corePart[ci]
		}
	}

	// Closed refs: relation weights to members plus s × 3.5 over the top 8
	// neighbors; no score joins Misc (or cluster 0 without Misc).
	k := 0
	for _, p := range part {
		k = max(k, int(p)+1)
	}
	refScore := make(map[int][]float64)
	scoreOf := func(r int) []float64 {
		if refScore[r] == nil {
			refScore[r] = make([]float64, k)
		}
		return refScore[r]
	}
	for _, e := range c.edges {
		if !c.nodes[e.from].open && part[e.to] >= 0 {
			scoreOf(e.from)[part[e.to]] += tmRelWeight(e.rel)
		}
		if !c.nodes[e.to].open && part[e.from] >= 0 {
			scoreOf(e.to)[part[e.from]] += tmRelWeight(e.rel)
		}
	}
	for r := c.nOpen; r < c.n; r++ {
		if c.nodes[r].open {
			continue
		}
		list := c.nbrs[r]
		for _, j := range list[:min(len(list), tmRefNeighbors)] {
			if part[j] >= 0 {
				scoreOf(r)[part[j]] += c.s(int32(r), j) * tmSemScale
			}
		}
		best, target := 0.0, int32(-1)
		for d, w := range refScore[r] {
			if w > best {
				best, target = w, int32(d)
			}
		}
		if target < 0 && !misc && k > 0 {
			target = 0
		}
		if target < 0 {
			misc = true
		}
		part[r] = target
	}
	return part, misc
}
