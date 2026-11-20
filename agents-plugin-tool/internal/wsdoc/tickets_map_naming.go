package wsdoc

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Cluster order, hues, c-TF-IDF names and the Keyword Soup of the ticket map
// (tickets_map_cluster.go).

// tmCTerm is one term's c-TF-IDF aggregate in a cluster.
type tmCTerm struct {
	id          int32
	s           float64 // summed presence score
	tick, title int     // open members carrying it; in their title
	score       float64
}

// finishClusters orders and renumbers the clusters (Misc last), assigns hues,
// names each cluster by c-TF-IDF, and builds its Keyword Soup.
func (c *tmCorpus) finishClusters(out *mapAnalysis, part []int32, misc bool) {
	k := 0
	for _, p := range part {
		k = max(k, int(p)+1)
	}
	weight := make([]float64, k)
	for i := 0; i < c.n; i++ {
		if p := part[i]; p >= 0 && c.nodes[i].open {
			weight[p] += tmStatusWeight(c.nodes[i].info.Status)
		}
	}
	order := make([]int, k)
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		if weight[a] != weight[b] {
			if weight[a] > weight[b] {
				return -1
			}
			return 1
		}
		return a - b
	})
	newID := make([]int, k)
	for pos, old := range order {
		newID[old] = pos
	}
	total := k
	if misc {
		total++
	}
	for i, p := range part {
		if p >= 0 {
			out.cluster[i] = newID[p]
		} else {
			out.cluster[i] = k
		}
	}
	members := make([][]int, total)
	for i, id := range out.cluster {
		members[id] = append(members[id], i)
	}
	out.clusters = make([]TicketMapCluster, total)
	for id := 0; id < total; id++ {
		cl := TicketMapCluster{
			ID:        id,
			NameTerms: []string{},
			Terms:     []TicketMapTerm{},
			Hue:       int(math.Round(math.Mod(float64(id)*360/float64(total)+tmHueOffset, 360))),
			Misc:      id == k,
		}
		for _, i := range members[id] {
			switch {
			case !c.nodes[i].open:
				cl.Ref++
			case c.nodes[i].info.Status == "ready":
				cl.Open++
				cl.Ready++
			default:
				cl.Open++
			}
		}
		out.clusters[id] = cl
	}
	if k > 0 {
		c.nameClusters(out, members[:k])
	}
}

// tmPresence is a ticket's c-TF-IDF presence score of a term: title 3.0,
// heading 1.0, code 1.0 (prose carries none).
func tmPresence(st *termStat) float64 {
	p := 0.0
	if st.n[tmChTitle] > 0 {
		p += 3
	}
	if st.n[tmChHeading] > 0 {
		p++
	}
	if st.n[tmChCode] > 0 {
		p++
	}
	return p
}

// nameClusters scores every kept term per non-Misc cluster by c-TF-IDF over
// its open members, then picks names, labels and the Keyword Soup.
func (c *tmCorpus) nameClusters(out *mapAnalysis, members [][]int) {
	K := len(members)
	V := len(c.vocab)
	acc := make([]tmCTerm, V)
	used := make([]bool, V)
	var touched []int32
	cterms := make([][]tmCTerm, K)
	sizes := make([]int, K)
	totals := make([]float64, K)
	ftot := make([]float64, V)
	cdf := make([]int, V)
	for cid, list := range members {
		touched = touched[:0]
		for _, i := range list {
			if !c.nodes[i].open {
				continue
			}
			sizes[cid]++
			for x := range c.stats[i] {
				e := &c.stats[i][x]
				p := tmPresence(&e.st)
				if !c.kept[e.id] || p == 0 {
					continue
				}
				if !used[e.id] {
					used[e.id] = true
					touched = append(touched, e.id)
					acc[e.id] = tmCTerm{id: e.id}
				}
				a := &acc[e.id]
				a.s += p
				a.tick++
				if e.st.n[tmChTitle] > 0 {
					a.title++
				}
			}
		}
		slices.Sort(touched)
		terms := make([]tmCTerm, len(touched))
		for x, id := range touched {
			terms[x] = acc[id]
			used[id] = false
			totals[cid] += acc[id].s
			ftot[id] += acc[id].s
			if acc[id].s >= 2.0 {
				cdf[id]++
			}
		}
		cterms[cid] = terms
	}
	meanTotal := 0.0
	for _, t := range totals {
		meanTotal += t
	}
	meanTotal /= float64(K)

	for cid := range members {
		terms := cterms[cid]
		size := float64(sizes[cid])
		for x := range terms {
			t := &terms[x]
			t.score = (t.s / totals[cid]) * math.Log(1+meanTotal/ftot[t.id]) *
				(1 + float64(t.tick)/size) * (1 + 1.5*float64(t.title)/size)
		}
		slices.SortFunc(terms, func(a, b tmCTerm) int {
			if a.score != b.score {
				if a.score > b.score {
					return -1
				}
				return 1
			}
			return int(a.id - b.id)
		})

		// Names: up to 8 candidates; title-carried non-code terms first.
		minTick := max(2, int(math.Ceil(0.12*size)))
		cands := c.pickTerms(terms, cdf, K, minTick, tmNameCDF, tmNameCands, true)
		minTitle := 2
		if sizes[cid] < 6 {
			minTitle = 1
		}
		var names []tmCTerm
		for _, pass := range [...]bool{true, false} {
			for _, t := range cands {
				if len(names) == tmNameTerms {
					break
				}
				titled := t.title >= minTitle && !c.code[t.id]
				if titled == pass {
					names = append(names, t)
				}
			}
		}
		cl := &out.clusters[cid]
		labels := make([]string, 0, len(names))
		for _, t := range names {
			term := c.vocab[t.id]
			cl.NameTerms = append(cl.NameTerms, term)
			labels = append(labels, tmLabelTerm(term))
		}
		cl.Label = strings.Join(labels, " · ")
		for _, t := range cands {
			cl.Terms = append(cl.Terms, TicketMapTerm{Term: c.vocab[t.id], Code: c.code[t.id]})
		}

		if soup := c.soup(terms, cdf, K, members[cid], cl.NameTerms); len(soup) > 0 {
			out.keywords[strconv.Itoa(cid)] = soup
		}
	}
}

// pickTerms walks score-ordered terms keeping those with tick ≥ minTick and
// cdf/K ≤ maxCDF, de-duplicated by substring of the base or an equal lemma,
// up to limit. short drops Latin, digit and code terms under 3 runes and
// Hangul/CJK terms under 2.
func (c *tmCorpus) pickTerms(terms []tmCTerm, cdf []int, K, minTick int, maxCDF float64, limit int, short bool) []tmCTerm {
	var out []tmCTerm
	var bases, lemmas []string
	for _, t := range terms {
		if len(out) == limit {
			break
		}
		if t.tick < minTick || float64(cdf[t.id])/float64(K) > maxCDF {
			continue
		}
		term := c.vocab[t.id]
		if short {
			need := 3
			if term[0] >= utf8.RuneSelf {
				need = 2
			}
			if utf8.RuneCountInString(term) < need {
				continue
			}
		}
		base := tmTermBase(term)
		lemma := base
		if id, ok := c.index[base]; ok {
			lemma = c.vocab[c.lemma[id]]
		}
		dup := false
		for x, b := range bases {
			if strings.Contains(b, base) || strings.Contains(base, b) || lemmas[x] == lemma {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		bases = append(bases, base)
		lemmas = append(lemmas, lemma)
		out = append(out, t)
	}
	return out
}

// tmLabelTerm uppercases a Latin term's first rune; code terms (any of
// `./_#`) and Hangul/CJK terms stay as they are.
func tmLabelTerm(term string) string {
	if term == "" || strings.ContainsAny(term, "./_#") || term[0] < 'a' || term[0] > 'z' {
		return term
	}
	return string(term[0]-'a'+'A') + term[1:]
}

// soup builds one cluster's Keyword Soup: candidates with tick ≥ 2 and
// cdf/K ≤ 0.50 (de-duplicated, up to 40), the cluster's name terms removed,
// at most 2 code terms and 10 terms, each carried by at least 2 members.
func (c *tmCorpus) soup(terms []tmCTerm, cdf []int, K int, members []int, names []string) []TicketMapKeyword {
	cands := c.pickTerms(terms, cdf, K, 2, tmSoupCDF, tmSoupCands, false)
	var picked []tmCTerm
	var carriers [][]int
	codes := 0
	for _, t := range cands {
		if len(picked) == tmSoupMax {
			break
		}
		if slices.Contains(names, c.vocab[t.id]) || (c.code[t.id] && codes == tmSoupCodeMax) {
			continue
		}
		var ids []int
		for _, i := range members {
			if st := c.stat(i, t.id); st != nil && tmPresence(st) > 0 {
				ids = append(ids, i)
			}
		}
		if len(ids) < 2 {
			continue
		}
		if c.code[t.id] {
			codes++
		}
		picked = append(picked, t)
		carriers = append(carriers, ids)
	}
	if len(picked) == 0 {
		return nil
	}
	top := picked[0].score
	out := make([]TicketMapKeyword, 0, len(picked))
	for x, t := range picked {
		kwd := TicketMapKeyword{
			Term:      c.vocab[t.id],
			Weight:    MapFloat(roundTo(t.score/top, 3)),
			Code:      boolInt(c.code[t.id]),
			TicketIDs: carriers[x],
			TW:        make([]MapFloat, 0, len(carriers[x])),
		}
		for _, i := range carriers[x] {
			tw := max(c.vecWeight(i, t.id), 0.02)
			if c.stat(i, t.id).n[tmChTitle] > 0 {
				tw += 0.15
			}
			kwd.TW = append(kwd.TW, MapFloat(roundTo(tw, 3)))
			if c.nodes[i].open {
				kwd.Open++
			}
		}
		out = append(out, kwd)
	}
	return out
}

// stat returns node i's merged stat of term id, or nil.
func (c *tmCorpus) stat(i int, id int32) *termStat {
	entries := c.stats[i]
	if x, ok := slices.BinarySearchFunc(entries, id, func(e tmTermEntry, id int32) int { return int(e.id - id) }); ok {
		return &entries[x].st
	}
	return nil
}

// vecWeight returns node i's normalized vector weight of term id, or 0.
func (c *tmCorpus) vecWeight(i int, id int32) float64 {
	vec := c.vecs[i]
	if x, ok := slices.BinarySearchFunc(vec, id, func(e tmVecEntry, id int32) int { return int(e.id - id) }); ok {
		return vec[x].w
	}
	return 0
}
