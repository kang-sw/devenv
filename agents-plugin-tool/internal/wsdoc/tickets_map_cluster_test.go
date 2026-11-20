package wsdoc

import (
	"bytes"
	"math"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/kang-sw/devenv/internal/wsdoc/wsdoctest"
)

// tmPlantedName reports whether a cluster name term is one of a topic's
// planted title words. CJK titles tokenize to rune bigrams, so for a non-Latin
// term a bigram inside a planted word counts too.
func tmPlantedName(term string, words []string) bool {
	for _, w := range words {
		if term == w || (term[0] >= utf8.RuneSelf && strings.Contains(w, term)) {
			return true
		}
	}
	return false
}

func tmAllRunes(s string, tables ...*unicode.RangeTable) bool {
	for _, r := range s {
		if !unicode.In(r, tables...) {
			return false
		}
	}
	return s != ""
}

// tmFixtureCorpus runs the analysis up to the vectors over root's board.
func tmFixtureCorpus(t *testing.T, root string) *tmCorpus {
	t.Helper()
	load, err := loadTicketMap(root)
	if err != nil {
		t.Fatal(err)
	}
	c := &tmCorpus{nodes: load.nodes, edges: ticketMapEdges(load.nodes), n: len(load.nodes)}
	for _, node := range c.nodes {
		if node.open {
			c.nOpen++
		}
	}
	c.buildVocab()
	c.buildStats()
	c.cutAndCode()
	c.buildVectors()
	return c
}

func TestTicketMapClusterFixture(t *testing.T) {
	root := t.TempDir()
	layout := writeTicketMapFixture(t, root, wsdoctest.TicketMapDefaultOpen)
	resetTicketMapState(t)
	r := buildTicketMap(t, root)
	idx := func(stem string) int { return mapNodeIndex(t, r, stem) }

	// Clusters: ids in order, Misc exactly once and last, K within TARGET_K.
	k := 0
	for i, cl := range r.Clusters {
		if cl.ID != i {
			t.Fatalf("cluster %d has id %d", i, cl.ID)
		}
		if cl.Misc && i != len(r.Clusters)-1 {
			t.Fatalf("Misc cluster %d is not last", i)
		}
		if !cl.Misc {
			k++
		}
		if want := int(math.Round(math.Mod(float64(i)*360/float64(len(r.Clusters))+205, 360))); cl.Hue != want {
			t.Errorf("cluster %d hue %d, want %d", i, cl.Hue, want)
		}
	}
	if tk := r.Meta.Params.TargetK; k < tk[0] || k > tk[1] {
		t.Errorf("K = %d non-Misc clusters, outside TARGET_K %v", k, tk)
	}
	if r.Meta.Params.TargetK != [2]int{7, 12} || r.Meta.Params.MaxComm != 36 || r.Meta.Params.MinComm != 4 {
		t.Errorf("params %+v", r.Meta.Params)
	}
	misc := r.Clusters[len(r.Clusters)-1]
	if !misc.Misc || misc.Label != "" || len(misc.NameTerms) != 0 {
		t.Fatalf("last cluster %+v is not an unnamed Misc", misc)
	}
	for _, stem := range layout.Isolated {
		if c := r.Nodes[idx(stem)].Cluster; c != misc.ID {
			t.Errorf("isolated %s in cluster %d, want Misc %d", stem, c, misc.ID)
		}
	}
	if misc.Open != len(layout.Isolated) {
		t.Errorf("Misc holds %d open tickets, want the %d isolated", misc.Open, len(layout.Isolated))
	}

	// Every planted topic dominates one named cluster.
	for _, topic := range layout.Topics {
		count := map[int]int{}
		for _, stem := range topic.Stems {
			count[r.Nodes[idx(stem)].Cluster]++
		}
		best, bestN := -1, 0
		for c, n := range count {
			if n > bestN || (n == bestN && c < best) {
				best, bestN = c, n
			}
		}
		if float64(bestN) < 0.8*float64(len(topic.Stems)) {
			t.Errorf("topic %s: largest cluster share %d/%d (%v)", topic.Name, bestN, len(topic.Stems), count)
			continue
		}
		cl := r.Clusters[best]
		planted := false
		for _, term := range cl.NameTerms {
			planted = planted || tmPlantedName(term, topic.TitleWords)
		}
		if !planted {
			t.Errorf("topic %s: cluster %d names %v carry no planted title word", topic.Name, best, cl.NameTerms)
		}
		switch topic.Script {
		case wsdoctest.TicketMapScriptHangul:
			if !strings.ContainsFunc(cl.Label, func(r rune) bool { return unicode.Is(unicode.Hangul, r) }) {
				t.Errorf("Korean topic label %q has no Hangul term", cl.Label)
			}
		case wsdoctest.TicketMapScriptJapanese:
			ok := false
			for _, term := range cl.NameTerms {
				ok = ok || (utf8.RuneCountInString(term) == 2 && strings.Contains(cl.Label, term) &&
					(tmAllRunes(term, unicode.Han) || tmAllRunes(term, unicode.Katakana)))
			}
			if !ok {
				t.Errorf("Japanese topic label %q has no Kanji or Katakana bigram", cl.Label)
			}
		}
		if len(cl.NameTerms) == 0 || len(cl.NameTerms) > 3 || cl.Label == "" {
			t.Errorf("topic %s: cluster %d label %q names %v", topic.Name, best, cl.Label, cl.NameTerms)
		}
	}

	// The every-ticket term is cut and reaches no vector, kw, chip or name.
	every := wsdoctest.TicketMapEveryTicketTerm
	if !slices.Contains(r.Meta.DFCutTop, every) || r.Meta.DFCutTop[0] != every || r.Meta.DFCut < 1 {
		t.Errorf("dfCut %d dfCutTop %v lacks %q first", r.Meta.DFCut, r.Meta.DFCutTop, every)
	}
	for _, n := range r.Nodes {
		for _, kw := range n.KW {
			if kw.Term == every {
				t.Errorf("%s kw carries %q", n.Stem, every)
			}
		}
		if len(n.KW) > 6 {
			t.Errorf("%s has %d kw", n.Stem, len(n.KW))
		}
	}
	for _, row := range r.Sim {
		for _, term := range row.Shared {
			if term.Term == every {
				t.Errorf("sim row %d-%d shares %q", row.I, row.J, every)
			}
		}
	}
	corpus := tmFixtureCorpus(t, root)
	if id, ok := corpus.index[every]; !ok || corpus.kept[corpus.lemma[id]] {
		t.Errorf("%q is in the vocabulary %v and kept %v", every, ok, ok && corpus.kept[corpus.lemma[id]])
	}
	for i, vec := range corpus.vecs {
		for _, e := range vec {
			if corpus.vocab[e.id] == every {
				t.Errorf("node %d vector carries %q", i, every)
			}
		}
	}

	// Body-only mention pairs are boosted and flagged.
	if r.Meta.MentionPairs != len(layout.MentionPairs) {
		t.Errorf("meta.mentionPairs %d, want %d", r.Meta.MentionPairs, len(layout.MentionPairs))
	}
	for _, pair := range layout.MentionPairs {
		i, j := idx(pair[0]), idx(pair[1])
		if i > j {
			i, j = j, i
		}
		found := false
		for _, row := range r.Sim {
			if row.I == i && row.J == j {
				found = true
				if !row.Mention || row.Score < 0.18 {
					t.Errorf("mention pair %v: row %+v", pair, row)
				}
			}
		}
		if !found {
			t.Errorf("mention pair %v has no sim row", pair)
		}
	}
	mentions := 0
	for x, row := range r.Sim {
		if row.I >= row.J || (x > 0 && (r.Sim[x-1].I > row.I || (r.Sim[x-1].I == row.I && r.Sim[x-1].J >= row.J))) {
			t.Fatalf("sim rows unsorted or not i<j at %d: %+v", x, row)
		}
		if row.Mention {
			mentions++
		}
		if len(row.Shared) > 4 {
			t.Errorf("sim row %+v has %d shared terms", row, len(row.Shared))
		}
	}
	if mentions != r.Meta.MentionPairs {
		t.Errorf("%d mention rows, meta says %d", mentions, r.Meta.MentionPairs)
	}

	// Keyword Soup: no name term, at most 2 code terms and 10 terms, weights
	// normalized to the first, carriers in the cluster.
	for key, soup := range r.ClusterKeywords {
		id, err := strconv.Atoi(key)
		if err != nil || id < 0 || id >= len(r.Clusters) || r.Clusters[id].Misc {
			t.Fatalf("clusterKeywords key %q", key)
		}
		cl := r.Clusters[id]
		code := 0
		for x, kwd := range soup {
			if slices.Contains(cl.NameTerms, kwd.Term) {
				t.Errorf("cluster %d soup term %q is a name term %v", id, kwd.Term, cl.NameTerms)
			}
			code += kwd.Code
			if (x == 0 && kwd.Weight != 1) || kwd.Weight > 1 || kwd.Weight <= 0 {
				t.Errorf("cluster %d soup %q weight %v", id, kwd.Term, kwd.Weight)
			}
			if len(kwd.TicketIDs) < 2 || len(kwd.TicketIDs) != len(kwd.TW) {
				t.Errorf("cluster %d soup %q ids %v tw %v", id, kwd.Term, kwd.TicketIDs, kwd.TW)
			}
			open := 0
			for _, i := range kwd.TicketIDs {
				if r.Nodes[i].Cluster != id {
					t.Errorf("cluster %d soup %q carrier %d is in cluster %d", id, kwd.Term, i, r.Nodes[i].Cluster)
				}
				if s := r.Nodes[i].Status; s == "ready" || s == "todo" || s == "idea" {
					open++
				}
			}
			if open != kwd.Open {
				t.Errorf("cluster %d soup %q open %d, want %d", id, kwd.Term, kwd.Open, open)
			}
		}
		if code > 2 || len(soup) > 10 {
			t.Errorf("cluster %d soup has %d code terms, %d terms", id, code, len(soup))
		}
	}
	if len(r.ClusterKeywords) < k-1 {
		t.Errorf("only %d of %d clusters have a soup", len(r.ClusterKeywords), k)
	}

	// Closed refs join a topic cluster, never Misc; cluster counts add up.
	for _, n := range r.Nodes[layout.Open:] {
		if r.Clusters[n.Cluster].Misc {
			t.Errorf("ref %s joined Misc", n.Stem)
		}
	}
	open, refs := 0, 0
	for _, cl := range r.Clusters {
		open += cl.Open
		refs += cl.Ref
	}
	if open != r.Meta.Open || refs != r.Meta.Refs {
		t.Errorf("cluster counts open %d refs %d, meta %d %d", open, refs, r.Meta.Open, r.Meta.Refs)
	}
	if r.Meta.Gamma < 0.4 || r.Meta.Gamma > 3.0 || r.Meta.Modularity <= 0 {
		t.Errorf("gamma %v modularity %v", r.Meta.Gamma, r.Meta.Modularity)
	}

	// GOMAXPROCS does not reach the bytes.
	prev := runtime.GOMAXPROCS(1)
	t.Cleanup(func() { runtime.GOMAXPROCS(prev) })
	resetTicketMapState(t)
	one := bytes.Clone(buildTicketMapJSON(t, root))
	runtime.GOMAXPROCS(8)
	resetTicketMapState(t)
	if eight := buildTicketMapJSON(t, root); !bytes.Equal(one, eight) {
		t.Fatal("GOMAXPROCS 1 and 8 builds differ")
	}
}

func TestTicketMapLemmaChains(t *testing.T) {
	vocab := []string{"map", "mapped", "mapping", "mappings", "maps.go", "other", "others", "queries", "query", "status", "studied", "study"}
	index := map[string]int32{}
	for i, w := range vocab {
		index[w] = int32(i)
	}
	lemma := tmBuildLemma(vocab, index)
	for w, want := range map[string]string{
		"queries":  "query",
		"mapped":   "map",
		"mapping":  "map",
		"mappings": "map", // mappings → mapping → map: two hops
		"maps.go":  "maps.go",
		"others":   "others", // "other" is a stopword
		"status":   "status", // `us` is never stripped
		"studied":  "study",  // `ed` with i→y
		"map":      "map",
	} {
		if got := vocab[lemma[index[w]]]; got != want {
			t.Errorf("lemma(%s) = %s, want %s", w, got, want)
		}
	}

	// Merging sums into fresh stats and leaves the shared documents alone.
	a := buildTicketDoc("260101-feat-a", "# A\n\n## Background\n\nThe queries hit the query planner.\n", false)
	before := *a.Terms["queries"]
	c := &tmCorpus{nodes: []mapNode{{doc: a, open: true}}, n: 1, nOpen: 1}
	c.buildVocab()
	c.buildStats()
	if *a.Terms["queries"] != before {
		t.Fatalf("document stats mutated: %+v, was %+v", *a.Terms["queries"], before)
	}
	st := c.stat(0, c.index["query"])
	if st == nil || st.n[tmChProse] != a.Terms["queries"].n[tmChProse]+a.Terms["query"].n[tmChProse] {
		t.Fatalf("merged query stat %+v", st)
	}
	if c.stat(0, c.index["queries"]) != nil {
		t.Fatalf("queries kept its own entry after merging")
	}
}

// tmTwoCliques is two 4-cliques (0-3, 4-7) joined by the bridge 3-4.
func tmTwoCliques() *lvGraph {
	var pairs []lvPair
	for _, base := range []int32{0, 4} {
		for u := base; u < base+4; u++ {
			for v := u + 1; v < base+4; v++ {
				pairs = append(pairs, lvPair{u: u, v: v, w: 1})
			}
		}
	}
	pairs = append(pairs, lvPair{u: 3, v: 4, w: 1})
	slices.SortFunc(pairs, func(a, b lvPair) int {
		if a.u != b.u {
			return int(a.u - b.u)
		}
		return int(a.v - b.v)
	})
	return newLvGraph(8, pairs)
}

func TestTicketMapLouvainDeterministic(t *testing.T) {
	g := tmTwoCliques()
	want := []int32{0, 0, 0, 0, 1, 1, 1, 1}
	for run := 0; run < 5; run++ {
		if got := louvain(g, 1.0); !slices.Equal(got, want) {
			t.Fatalf("run %d: %v, want %v", run, got, want)
		}
	}
	// Q = 2 × (6/13 − (13/26)²).
	if q := lvModularity(g, want); math.Abs(q-(2*(6.0/13-0.25))) > 1e-12 {
		t.Fatalf("modularity %v", q)
	}

	// A 5-clique and a 3-clique: the larger community gets id 0 even though
	// the smaller one holds the lowest node index.
	var pairs []lvPair
	for u := int32(0); u < 3; u++ {
		for v := u + 1; v < 3; v++ {
			pairs = append(pairs, lvPair{u: u, v: v, w: 1})
		}
	}
	pairs = append(pairs, lvPair{u: 2, v: 3, w: 0.1})
	for u := int32(3); u < 8; u++ {
		for v := u + 1; v < 8; v++ {
			pairs = append(pairs, lvPair{u: u, v: v, w: 1})
		}
	}
	if got := louvain(newLvGraph(8, pairs), 1.0); !slices.Equal(got, []int32{1, 1, 1, 0, 0, 0, 0, 0}) {
		t.Fatalf("size relabel: %v", got)
	}

	// An edgeless graph stays singletons.
	if got := louvain(newLvGraph(3, nil), 1.0); !slices.Equal(got, []int32{0, 1, 2}) {
		t.Fatalf("edgeless: %v", got)
	}
}

func TestTicketMapSplitAndAbsorb(t *testing.T) {
	g := tmTwoCliques()
	cl := &tmClusterer{g: g, minComm: 2, maxComm: 5, targetK: [2]int{2, 3},
		absorb: make([][]int32, 8), absorbSim: make([][]float64, 8)}
	whole := make([]int32, 8)
	if got := cl.splitOversized(slices.Clone(whole), 0.4); !slices.Equal(got, []int32{0, 0, 0, 0, 1, 1, 1, 1}) {
		t.Fatalf("split of an oversized cluster: %v", got)
	}
	cl.maxComm = 8
	if got := cl.splitOversized(slices.Clone(whole), 0.4); !slices.Equal(got, whole) {
		t.Fatalf("a cluster within maxComm was split: %v", got)
	}

	// Node 7 alone; its only edge goes to the second clique.
	cl.minComm = 4
	part := []int32{0, 0, 0, 0, 1, 1, 1, 2}
	if got := cl.absorbTiny(part); !slices.Equal(got, []int32{0, 0, 0, 0, 1, 1, 1, 1}) {
		t.Fatalf("absorb: %v", got)
	}
	if !slices.Equal(part, []int32{0, 0, 0, 0, 1, 1, 1, 2}) {
		t.Fatalf("absorbTiny mutated its input: %v", part)
	}

	// An unconnected tiny cluster merges through similarity alone.
	iso := newLvGraph(5, []lvPair{{0, 1, 1}, {0, 2, 1}, {1, 2, 1}, {2, 3, 1}})
	cl = &tmClusterer{g: iso, minComm: 2, maxComm: 10, targetK: [2]int{1, 2},
		absorb: [][]int32{nil, nil, nil, nil, {1}}, absorbSim: [][]float64{nil, nil, nil, nil, {0.3}}}
	if got := cl.absorbTiny([]int32{0, 0, 0, 0, 1}); !slices.Equal(got, []int32{0, 0, 0, 0, 0}) {
		t.Fatalf("similarity absorb: %v", got)
	}

	// score: balanced sizes inside TARGET_K beat an out-of-range k.
	cl = &tmClusterer{g: g, minComm: 2, maxComm: 4, targetK: [2]int{2, 3}}
	if a, b := cl.score([]int32{0, 0, 0, 0, 1, 1, 1, 1}), cl.score(make([]int32, 8)); !(a == 1 && b == -2-float64(4)/6) {
		t.Fatalf("scores %v %v", a, b)
	}
}

func TestTicketMapSmallBoardSingleCluster(t *testing.T) {
	resetTicketMapState(t)
	root := t.TempDir()
	files := map[string]string{
		"idea/260109-research-lone-comet": mapTicket("Lone comet"),
	}
	stems := []string{"260101-feat-relay-a", "260102-feat-relay-b", "260103-feat-relay-c", "260104-feat-relay-d"}
	for i, stem := range stems {
		var fm []string
		if i > 0 {
			fm = append(fm, "related:", "  "+stems[i-1]+": shares the relay")
		}
		files[[]string{"ready", "todo", "todo", "idea"}[i]+"/"+stem] = mapTicket("Relay beacon", fm...)
	}
	writeMapBoard(t, root, files)
	r := buildTicketMap(t, root)
	if len(r.Clusters) != 2 || r.Clusters[0].Misc || !r.Clusters[1].Misc {
		t.Fatalf("clusters %+v", r.Clusters)
	}
	if r.Clusters[0].Open != 4 || r.Clusters[0].Ready != 1 || r.Clusters[1].Open != 1 {
		t.Fatalf("cluster counts %+v", r.Clusters)
	}
	if r.Meta.Gamma != 0 || r.Meta.Modularity != 0 {
		t.Fatalf("Louvain ran on a small board: gamma %v modularity %v", r.Meta.Gamma, r.Meta.Modularity)
	}
	if r.Clusters[0].Hue != 205 || r.Clusters[1].Hue != 25 {
		t.Fatalf("hues %d %d", r.Clusters[0].Hue, r.Clusters[1].Hue)
	}
	if c := r.Nodes[mapNodeIndex(t, r, "260109-research-lone-comet")].Cluster; c != 1 {
		t.Fatalf("lone ticket in cluster %d", c)
	}
}
