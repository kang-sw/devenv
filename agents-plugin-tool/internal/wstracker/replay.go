package wstracker

import (
	"container/heap"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
)

// node is one event as the domain folds it.
type node struct {
	ev                    wsledger.Event
	id, alias             string
	member, clone, stream string
	seq                   int
	ts                    time.Time
	typ, origin           string
	p                     obj
	broken                bool
	synthetic             bool // a Dispatch candidate, after every held event
	pos                   int  // fold order position; -1 outside it
	preds                 []*node
	indeg                 int
	succs                 []*node

	status, reason       string // applied | flagged | waiting
	ignored              bool
	markers              []marker
	superseded           []supersession
	thread, parentThread string
}

func newNode(ev wsledger.Event) *node {
	n := &node{ev: ev, id: ev.ID.String(), member: ev.ID.Member, clone: ev.ID.Clone, stream: ev.ID.Stream(),
		seq: ev.ID.Seq, ts: ev.TS, typ: ev.Type, origin: ev.Origin, p: parsePayload(ev.Payload),
		broken: ev.ID.Broken != "", pos: -1}
	n.alias = alias(n.id)
	if n.origin == "" {
		n.origin = OriginHuman
	}
	return n
}

// later is the display-winner order for concurrent writes and the fold
// tie-break: later ts, then member id, clone id, seq.
func later(x, y *node) bool {
	if !x.ts.Equal(y.ts) {
		return x.ts.After(y.ts)
	}
	if x.member != y.member {
		return x.member > y.member
	}
	if x.clone != y.clone {
		return x.clone > y.clone
	}
	return x.seq > y.seq
}

// nodeLess is wsledger's order: earlier ts, then member, clone, seq.
func nodeLess(a, b *node) bool {
	if !a.ts.Equal(b.ts) {
		return a.ts.Before(b.ts)
	}
	if a.member != b.member {
		return a.member < b.member
	}
	if a.clone != b.clone {
		return a.clone < b.clone
	}
	if a.seq != b.seq {
		return a.seq < b.seq
	}
	return a.ev.ID.Broken < b.ev.ID.Broken
}

type nodeHeap []*node

func (h nodeHeap) Len() int           { return len(h) }
func (h nodeHeap) Less(i, j int) bool { return nodeLess(h[i], h[j]) }
func (h nodeHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *nodeHeap) Push(x any)        { *h = append(*h, x.(*node)) }
func (h *nodeHeap) Pop() any {
	old := *h
	n := old[len(old)-1]
	*h = old[:len(old)-1]
	return n
}

// replayed is the folded store as one clone holds it.
type replayed struct {
	g     *genesis
	nodes []*node // display order
	order []*node // fold order
	byID  map[string]*node
	S     *state // final fold
	hb    func(x, y *node) bool
	heads map[string]int

	rolesPos, assignPos   []int
	threadPos, commentPos map[string][]int
	rolesOnly             bool // judge and fold only roles and identity events
}

// replay validates and folds the events one clone holds. conc is the
// source's Concurrent.
func replay(events []wsledger.Event, genesisRaw json.RawMessage, conc func(a, b wsledger.EventID) bool) *replayed {
	return replayWith(events, genesisRaw, conc, false)
}

// replayRoles is replay folding only the org: members, admins, and leads.
// Every event still takes its place in the fold order.
func replayRoles(events []wsledger.Event, genesisRaw json.RawMessage, conc func(a, b wsledger.EventID) bool) *replayed {
	return replayWith(events, genesisRaw, conc, true)
}

func replayWith(events []wsledger.Event, genesisRaw json.RawMessage, conc func(a, b wsledger.EventID) bool, rolesOnly bool) *replayed {
	r := &replayed{g: parseGenesis(genesisRaw), byID: map[string]*node{}, heads: map[string]int{},
		threadPos: map[string][]int{}, commentPos: map[string][]int{}, rolesOnly: rolesOnly}
	r.hb = func(x, y *node) bool {
		if y.synthetic {
			return !x.synthetic || x.pos < y.pos
		}
		return x != y && !x.synthetic && !conc(x.ev.ID, y.ev.ID)
	}
	all := make([]*node, 0, len(events))
	streams := map[string]map[int]*node{}
	for _, ev := range events {
		n := newNode(ev)
		if r.byID[n.id] != nil {
			continue
		}
		r.byID[n.id] = n
		all = append(all, n)
		if !n.broken {
			if streams[n.stream] == nil {
				streams[n.stream] = map[int]*node{}
			}
			streams[n.stream][n.seq] = n
			if n.seq > r.heads[n.stream] {
				r.heads[n.stream] = n.seq
			}
		}
	}
	r.linearize(all, streams)
	r.verdicts(all)
	r.S = newState(r.g, r.hb, true)
	for _, n := range r.order {
		if n.status == "applied" {
			r.S.apply(n)
		}
	}
	r.threads()
	return r
}

// linearize puts the resolved events in one causal order (Kahn's algorithm,
// ready ties by ts, member, clone, seq) and places the rest (broken,
// waiting, cyclic) by ts, as wsledger does.
func (r *replayed) linearize(all []*node, streams map[string]map[int]*node) {
	inCausal := func(n *node) bool { return n != nil && !n.broken && n.ev.Verdict != wsledger.Waiting }
	ready := &nodeHeap{}
	for _, n := range all {
		if !inCausal(n) {
			continue
		}
		blocked := false
		add := func(p *node) {
			if !inCausal(p) {
				blocked = true
				return
			}
			for _, x := range n.preds {
				if x == p {
					return
				}
			}
			n.preds = append(n.preds, p)
		}
		if n.seq > 1 {
			add(streams[n.stream][n.seq-1])
		}
		for s, q := range n.ev.Seen {
			if s != n.stream && q > 0 {
				add(streams[s][q])
			}
		}
		if blocked {
			n.indeg = -1
			continue
		}
		n.indeg = len(n.preds)
	}
	for _, n := range all {
		if n.indeg < 0 {
			continue
		}
		for _, p := range n.preds {
			p.succs = append(p.succs, n)
		}
		if inCausal(n) && n.indeg == 0 {
			heap.Push(ready, n)
		}
	}
	for ready.Len() > 0 {
		n := heap.Pop(ready).(*node)
		n.pos = len(r.order)
		r.order = append(r.order, n)
		for _, s := range n.succs {
			s.indeg--
			if s.indeg == 0 {
				heap.Push(ready, s)
			}
		}
	}
	var others []*node
	for _, n := range all {
		if n.pos < 0 {
			others = append(others, n)
		}
	}
	sort.SliceStable(others, func(i, j int) bool { return nodeLess(others[i], others[j]) })
	i, j := 0, 0
	for i < len(r.order) || j < len(others) {
		if j < len(others) && (i == len(r.order) || others[j].ts.Before(r.order[i].ts)) {
			r.nodes = append(r.nodes, others[j])
			j++
		} else {
			r.nodes = append(r.nodes, r.order[i])
			i++
		}
	}
	for _, n := range r.order {
		switch {
		case isRoles(n.typ):
			r.rolesPos = append(r.rolesPos, n.pos)
		case n.typ == TypeItemAssign:
			r.assignPos = append(r.assignPos, n.pos)
		}
		if n.typ == TypeItemCreate {
			r.threadPos[n.id] = append(r.threadPos[n.id], n.pos)
		} else if it := n.p.str("item"); it != "" {
			r.threadPos[it] = append(r.threadPos[it], n.pos)
		}
		if n.typ == TypeCommentAdd {
			r.commentPos[n.id] = append(r.commentPos[n.id], n.pos)
		} else if c := n.p.str("comment"); c != "" {
			r.commentPos[c] = append(r.commentPos[c], n.pos)
		}
	}
}

// verdicts gives every event its status. A protocol verdict other than
// Valid stands; a protocol-Valid domain event is judged against the state
// folded from its causal past.
func (r *replayed) verdicts(all []*node) {
	for _, n := range r.order {
		if r.rolesOnly && !isRoles(n.typ) {
			n.status = "skipped"
			continue
		}
		switch n.ev.Verdict {
		case wsledger.Valid:
			n.status = "applied"
			if !isIdentity(n.typ) {
				if rej := r.stateBefore(n).check(n); rej != "" {
					n.status, n.reason = "flagged", string(rej)
				}
			}
		case wsledger.Ignored:
			n.status, n.reason, n.ignored = "flagged", n.ev.Reason, true
		default:
			n.status, n.reason = "flagged", n.ev.Reason
		}
	}
	for _, n := range all {
		if n.pos >= 0 {
			continue
		}
		switch n.ev.Verdict {
		case wsledger.Waiting:
			n.status, n.reason = "waiting", n.ev.Reason
		case wsledger.Valid:
			// The protocol resolved it but its causal past is not all here.
			n.status, n.reason = "waiting", "waiting: its causal past is not complete in this read"
		default:
			n.status, n.reason = "flagged", n.ev.Reason
			n.ignored = n.ev.Verdict == wsledger.Ignored
		}
	}
}

// stateBefore folds the part of n's causal past that n's rules read: the
// roles and identity events, the threads and comments n names, and every
// item.assign as a trigger of the live rules on those threads. Nothing else
// can change what check reads, so this equals folding the whole past.
func (r *replayed) stateBefore(n *node) *state {
	rel := map[string]bool{}
	var lists [][]int
	lists = append(lists, r.rolesPos, r.assignPos)
	for _, k := range []string{"item", "parent"} {
		if id := n.p.str(k); id != "" && !rel[id] {
			rel[id] = true
			lists = append(lists, r.threadPos[id])
		}
	}
	for _, k := range []string{"comment", "reply_to"} {
		if id := n.p.str(k); id != "" {
			lists = append(lists, r.commentPos[id])
		}
	}
	limit := len(r.order)
	if !n.synthetic {
		limit = n.pos
	}
	var cands []int
	for _, l := range lists {
		for _, pos := range l {
			if pos >= limit {
				break
			}
			cands = append(cands, pos)
		}
	}
	sort.Ints(cands)
	S := newState(r.g, r.hb, false)
	last := -1
	for _, pos := range cands {
		if pos == last {
			continue
		}
		last = pos
		x := r.order[pos]
		if x.status != "applied" {
			continue
		}
		if x.typ == TypeItemAssign && !rel[x.p.str("item")] {
			// Another thread's assign only re-applies the live rules here.
			if S.hasLiveRule() && r.hb(x, n) {
				S.refreshLive(x)
			}
			continue
		}
		if r.hb(x, n) {
			S.apply(x)
		}
	}
	return S
}

// isRoles reports a roles.* or member.join event: the events the org (and
// every rule reading it) folds from.
func isRoles(t string) bool {
	return strings.HasPrefix(t, "roles.") || t == wsledger.TypeMemberJoin
}

// threads assigns each event its thread: the item it names, and for a child
// create also the parent's thread.
func (r *replayed) threads() {
	cmItem := map[string]string{}
	for _, n := range r.nodes {
		if n.typ == TypeCommentAdd && n.p.str("item") != "" {
			cmItem[n.id] = n.p.str("item")
		}
	}
	for _, n := range r.nodes {
		switch {
		case n.typ == TypeItemCreate:
			n.thread = n.id
			n.parentThread = n.p.str("parent")
		case n.p.str("item") != "":
			n.thread = n.p.str("item")
		case n.p.str("comment") != "":
			n.thread = cmItem[n.p.str("comment")]
		}
	}
}
