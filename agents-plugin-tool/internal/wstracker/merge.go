package wstracker

import "strings"

// The body merge: an item.edit body carries the full new text and, from the
// editor, base: the body revision it saw (the body write heads of the value
// it loaded). The engine mirrors every function here line for line
// (ai-docs/ref/tracker-mockup/engine.js, "body merge"); the golden cases pin
// the two to one result, so a change here is a change there.

// bline is one line of a folded body and the body write it came from.
type bline struct {
	text string
	src  *node
}

// bodyFold is one item's body as folded: its applied body writes in fold
// order (the item.create first), its value, and each merging write's base
// text, which depends only on the write and the causal structure.
type bodyFold struct {
	writes []*node
	lines  []bline
	base   map[string][]bline
}

// bodyText is the body a write carries: item.create's when truthy, an
// item.edit's unless null.
func bodyText(n *node) string {
	if n.typ == TypeItemCreate {
		if n.p.truthy("body") {
			return jsString(n.p["body"])
		}
		return ""
	}
	if n.p.null("body") {
		return ""
	}
	return jsString(n.p["body"])
}

func splitLines(text string) []string { return strings.Split(text, "\n") }

func linesFrom(text string, src *node) []bline {
	l := splitLines(text)
	out := make([]bline, len(l))
	for i, t := range l {
		out[i] = bline{t, src}
	}
	return out
}

func joinLines(l []bline) string {
	t := make([]string, len(l))
	for i, x := range l {
		t[i] = x.text
	}
	return strings.Join(t, "\n")
}

// hunk is one change of a line diff: base lines [a0, a1) became the other
// side's lines [b0, b1).
type hunk struct{ a0, a1, b0, b1 int }

// diffLines is Myers' greedy line diff of a and b, after the common prefix
// and suffix are set aside, as maximal hunks between the matched lines. A
// step goes down (an insertion) when V[k-1] < V[k+1], else right (a
// deletion), as the engine's diffLines does.
func diffLines(a, b []string) []hunk {
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	a, b = a[pre:len(a)-suf], b[pre:len(b)-suf]
	n, m := len(a), len(b)
	max := n + m
	off := max + 1
	v := make([]int, 2*max+3)
	// trace[d] is V before step d, kept for k in [-d-1, d+1].
	var trace [][]int
	d := 0
found:
	for ; d <= max; d++ {
		trace = append(trace, append([]int(nil), v[off-d-1:off+d+2]...))
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[off+k-1] < v[off+k+1]) {
				x = v[off+k+1]
			} else {
				x = v[off+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[off+k] = x
			if x >= n && y >= m {
				break found
			}
		}
	}
	// Backtrack the matched pairs, last first.
	var mx, my []int
	x, y := n, m
	for ; d > 0; d-- {
		pv, po := trace[d], d+1
		k := x - y
		pk := k - 1
		if k == -d || (k != d && pv[po+k-1] < pv[po+k+1]) {
			pk = k + 1
		}
		px := pv[po+pk]
		py := px - pk
		for x > px && y > py {
			x, y = x-1, y-1
			mx, my = append(mx, x), append(my, y)
		}
		x, y = px, py
	}
	for x > 0 && y > 0 {
		x, y = x-1, y-1
		mx, my = append(mx, x), append(my, y)
	}
	var out []hunk
	pa, pb := 0, 0
	for i := len(mx) - 1; i >= -1; i-- {
		ma, mb := n, m
		if i >= 0 {
			ma, mb = mx[i], my[i]
		}
		if ma > pa || mb > pb {
			out = append(out, hunk{pre + pa, pre + ma, pre + pb, pre + mb})
		}
		pa, pb = ma+1, mb+1
	}
	return out
}

func textsOf(l []bline) []string {
	t := make([]string, len(l))
	for i, x := range l {
		t[i] = x.text
	}
	return t
}

func sameTexts(a, b []bline) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].text != b[i].text {
			return false
		}
	}
	return true
}

// merge3 merges mine (w's new text) and cur (the folded value) from base.
// Hunks of the two diffs whose base ranges intersect or touch form one
// group; a group changed on one side takes that side; a group both sides
// changed to the same lines takes cur's; any other group both changed is
// an overlap, which settle judges from cur's lines in it: mine when it
// returns true, else cur's. A kept line keeps its source: cur's for lines
// outside mine's changes, base's for lines mine kept inside a group, w for
// the lines mine wrote.
func merge3(base, cur []bline, mine []string, w *node, settle func(vr []bline) bool) (out []bline) {
	bt := textsOf(base)
	hc, hm := diffLines(bt, textsOf(cur)), diffLines(bt, mine)
	// mine with sources: lines outside its hunks are base's.
	tl := make([]bline, 0, len(mine))
	bi := 0
	for _, h := range hm {
		tl = append(tl, base[bi:h.a0]...)
		for _, t := range mine[h.b0:h.b1] {
			tl = append(tl, bline{t, w})
		}
		bi = h.a1
	}
	tl = append(tl, base[bi:]...)

	bi, vi, ti := 0, 0, 0
	i, j := 0, 0
	for i < len(hc) || j < len(hm) {
		// A group starts at the earliest remaining hunk and takes every hunk
		// starting at or before its end.
		gs := 0
		if j >= len(hm) || (i < len(hc) && hc[i].a0 <= hm[j].a0) {
			gs = hc[i].a0
		} else {
			gs = hm[j].a0
		}
		ge, inA, inB, vLen, tLen := gs, false, false, 0, 0
		for {
			var h hunk
			if i < len(hc) && hc[i].a0 <= ge {
				h, inA = hc[i], true
				i++
				vLen += (h.b1 - h.b0) - (h.a1 - h.a0)
			} else if j < len(hm) && hm[j].a0 <= ge {
				h, inB = hm[j], true
				j++
				tLen += (h.b1 - h.b0) - (h.a1 - h.a0)
			} else {
				break
			}
			if h.a1 > ge {
				ge = h.a1
			}
		}
		same := gs - bi
		out = append(out, cur[vi:vi+same]...)
		vi, ti = vi+same, ti+same
		vLen += ge - gs
		tLen += ge - gs
		vr, tr := cur[vi:vi+vLen], tl[ti:ti+tLen]
		switch {
		case !inB:
			out = append(out, vr...)
		case !inA:
			out = append(out, tr...)
		case sameTexts(vr, tr):
			out = append(out, vr...)
		default:
			if settle(vr) {
				out = append(out, tr...)
			} else {
				out = append(out, vr...)
			}
		}
		bi, vi, ti = ge, vi+vLen, ti+tLen
	}
	return append(out, cur[vi:]...)
}

// inClosure reports that write x is in the causal closure of heads: a head,
// or before one in fold order and in its causal past (hb assumes its first
// argument folds first).
func (s *state) inClosure(x *node, heads []*node) bool {
	for _, h := range heads {
		if x == h || (x.pos < h.pos && s.hb(x, h)) {
			return true
		}
	}
	return false
}

// baseHeads is the writes of prior that w's base names and that are in w's
// causal past; none means w is a whole replace.
func (s *state) baseHeads(prior []*node, w *node) []*node {
	if w.typ != TypeItemEdit || w.p.null("base") {
		return nil
	}
	ids := w.p.strList("base")
	var heads []*node
	for _, x := range prior {
		if contains(ids, x.id) && s.hb(x, w) {
			heads = append(heads, x)
		}
	}
	return heads
}

// rival is a prior write that a body write overlapped, with the write
// whose lines the overlap kept.
type rival struct{ x, win *node }

// bodyStep folds body write w onto cur, the value of prior (the writes
// folded before it). It returns the new value; the concurrent writes w
// overlapped, in fold order, each with the overlap's winner; and lostTo,
// the human write of w's own member it lost an overlap to, if any.
//
// A base whose closure holds every prior write is a plain replace. A
// base-less (or unknown-base) write is a whole replace, which overlaps
// every concurrent write. Otherwise the write merges three-way from its
// base text, and each overlapping group names its rivals: the concurrent
// writes whose lines cur holds there, or, when cur only dropped base lines
// there (no line tells who), every concurrent write. An overlap goes to w,
// the later in fold order, except that an agent write loses a group to a
// concurrent human write of its own member among that group's rivals.
func (s *state) bodyStep(f *bodyFold, prior []*node, cur []bline, w *node) (out []bline, rivals []rival, lostTo *node) {
	if w.typ == TypeItemCreate {
		return linesFrom(bodyText(w), w), nil, nil
	}
	var conc []*node
	for _, x := range prior {
		if !s.hb(x, w) {
			conc = append(conc, x)
		}
	}
	var human *node
	if w.origin != OriginHuman {
		for _, x := range conc {
			if x.member == w.member && x.origin == OriginHuman {
				human = x
			}
		}
	}
	heads := s.baseHeads(prior, w)
	if len(heads) == 0 {
		out = linesFrom(bodyText(w), w)
		if human != nil {
			out, lostTo = cur, human
		}
		for _, x := range conc {
			win := w
			if human != nil {
				win = x
			}
			rivals = append(rivals, rival{x, win})
		}
		return out, rivals, lostTo
	}
	var sub []*node
	inBase := map[*node]bool{}
	for _, x := range prior {
		if s.inClosure(x, heads) {
			sub = append(sub, x)
			inBase[x] = true
		}
	}
	if len(sub) == len(prior) {
		return linesFrom(bodyText(w), w), nil, nil
	}
	base, ok := f.base[w.id]
	if !ok {
		base = s.foldBody(f, sub)
		f.base[w.id] = base
	}
	hit := map[*node]bool{}
	settle := func(vr []bline) bool {
		changed := false
		for _, l := range vr {
			changed = changed || !inBase[l.src]
		}
		wins := true
		for _, x := range conc {
			if changed && !hasSrc(vr, x) {
				continue
			}
			hit[x] = true
			if x == human {
				wins = false
			}
		}
		if !wins {
			lostTo = human
		}
		return wins
	}
	out = merge3(base, cur, splitLines(bodyText(w)), w, settle)
	for _, x := range conc {
		if hit[x] {
			win := w
			if x == human {
				win = x
			}
			rivals = append(rivals, rival{x, win})
		}
	}
	return out, rivals, lostTo
}

func hasSrc(l []bline, x *node) bool {
	for _, b := range l {
		if b.src == x {
			return true
		}
	}
	return false
}

// foldBody is the value of writes, a causally closed run in fold order.
func (s *state) foldBody(f *bodyFold, writes []*node) []bline {
	var cur []bline
	for i, w := range writes {
		cur, _, _ = s.bodyStep(f, writes[:i], cur, w)
	}
	return cur
}

// applyBody folds item.edit n's body: the merged value, the body revision
// (the register's heads), the markers of an overlap, and the attachments
// and refs (the union over the writes whose lines survive) and mentions
// (the latest such write's).
func (s *state) applyBody(it *item, n *node) {
	f := s.bodies[it.id]
	key, label := "it:"+it.id+":body", "body of "+it.alias
	out, rivals, lostTo := s.bodyStep(f, f.writes, f.lines, n)
	f.writes = append(f.writes, n)
	f.lines = out
	var heads []head
	for _, h := range s.reg[key] {
		if !s.hb(h.n, n) {
			heads = append(heads, h)
		}
	}
	s.reg[key] = append(heads, head{n, bodyText(n)})
	if s.marks {
		for _, r := range rivals {
			x := r.x
			if x.member != n.member {
				mark(x, n, "field", label, r.win.id, show(bodyText(x)), show(bodyText(n)))
				mark(n, x, "field", label, r.win.id, show(bodyText(n)), show(bodyText(x)))
			} else if r.win == n {
				why := "later"
				if x.origin != OriginHuman && n.origin == OriginHuman {
					why = "human"
				}
				supersede(x, label, n, why)
			}
		}
		if lostTo != nil {
			supersede(n, label, lostTo, "human")
		}
	}
	s.bodyExtras(it, f)
}

// bodyExtras sets the item's body and the lists that follow it from the
// folded lines.
func (s *state) bodyExtras(it *item, f *bodyFold) {
	it.body = joinLines(f.lines)
	in := map[*node]bool{}
	for _, l := range f.lines {
		in[l.src] = true
	}
	it.mentions, it.attachments, it.refs = []string{}, []attachment{}, []string{}
	var last *node
	defer func() { it.mentionsAt = last }()
	for _, w := range f.writes {
		if !in[w] {
			continue
		}
		last = w
		for _, a := range attachmentsOf(w.p) {
			if !hasOID(it.attachments, a.oid) {
				it.attachments = append(it.attachments, a)
			}
		}
		for _, r := range refsOf(w.p) {
			if !contains(it.refs, r) {
				it.refs = append(it.refs, r)
			}
		}
	}
	if last != nil {
		it.mentions = mentionsOf(last.p)
	}
}

// bodyRev is item id's body revision: its body register's head ids.
func (s *state) bodyRev(id string) []string {
	out := []string{}
	for _, h := range s.reg["it:"+id+":body"] {
		out = append(out, h.n.id)
	}
	return out
}
