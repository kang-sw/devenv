package wstracker

import (
	"container/heap"
	"slices"
	"sort"

	"github.com/kang-sw/devenv/internal/wsledger"
)

// Checkpoints are process-local, bounded, and deliberately denser near the
// tail. Keeping the terminal checkpoint makes a single append replay one event.
const foldCheckpointStride = 1024
const foldCheckpointLimit = 16

type nodeMarks struct {
	markers    []marker
	superseded []supersession
}

type foldCheckpoint struct {
	pos   int // state immediately before order[pos]
	state *state
	marks map[*node]nodeMarks
}

type foldMemo struct {
	all         []*node
	deps        map[string][]string
	succ        map[string]map[string]bool
	checkpoints []foldCheckpoint
	// replayedFrom is observable in package tests/benchmarks, not a wire field.
	replayedFrom int
}

func dependencies(n *node) []string {
	if n.broken {
		return nil
	}
	ids := map[string]bool{}
	if n.seq > 1 {
		id := n.ev.ID
		id.Seq--
		ids[id.String()] = true
	}
	for stream, seq := range n.ev.Seen {
		if stream == n.stream || seq <= 0 {
			continue
		}
		id := wsledger.EventID{Seq: seq}
		// Seen keys are validated member.clone stream names by the source.
		for i := range stream {
			if stream[i] == '.' {
				id.Member, id.Clone = stream[:i], stream[i+1:]
				break
			}
		}
		ids[id.String()] = true
	}
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	return out
}

func newFoldMemo(r *replayed) *foldMemo {
	m := &foldMemo{deps: map[string][]string{}, succ: map[string]map[string]bool{}}
	for _, n := range r.nodes {
		m.add(n)
	}
	return m
}

func (m *foldMemo) add(n *node) {
	m.all = append(m.all, n)
	m.deps[n.id] = dependencies(n)
	for _, id := range m.deps[n.id] {
		if m.succ[id] == nil {
			m.succ[id] = map[string]bool{}
		}
		m.succ[id][n.id] = true
	}
}

// update splices the changed dependency closure into the old causal order.
// Unaffected nodes retain their relative order: a removed node's successors
// are in the closure too. Kahn's ready heap therefore needs only the closure,
// merged against the unchanged order, not a new sort of the whole history.
func (r *replayed) update(events []wsledger.Event) {
	m := r.memo
	affected := map[string]bool{}
	queue := make([]string, 0, len(events))
	earliest := len(r.order)
	for _, ev := range events {
		id := ev.ID.String()
		n := r.byID[id]
		if n == nil {
			n = newNode(ev)
			r.byID[id] = n
			m.add(n)
		} else {
			// Source changes existing events only for protocol verdict/reason.
			if n.pos >= 0 {
				earliest = min(earliest, n.pos)
			}
			n.ev = ev
		}
		if !affected[id] {
			affected[id] = true
			queue = append(queue, id)
		}
		if !n.broken {
			r.heads[n.stream] = max(r.heads[n.stream], n.seq)
		}
	}
	for i := 0; i < len(queue); i++ {
		for id := range m.succ[queue[i]] {
			if !affected[id] {
				affected[id] = true
				queue = append(queue, id)
			}
		}
	}
	old := r.order
	kept := make([]*node, 0, len(old))
	for _, n := range old {
		if !affected[n.id] {
			kept = append(kept, n)
		}
	}
	degree := map[string]int{}
	ready := &nodeHeap{}
	for id := range affected {
		n := r.byID[id]
		if n == nil || n.broken || n.ev.Verdict == wsledger.Waiting {
			continue
		}
		d := 0
		for _, dep := range m.deps[id] {
			p := r.byID[dep]
			if p == nil || p.broken || p.ev.Verdict == wsledger.Waiting || (!affected[dep] && p.pos < 0) {
				d = -1
				break
			}
			d++
		}
		degree[id] = d
		if d == 0 {
			heap.Push(ready, n)
		}
	}
	order := make([]*node, 0, len(old)+len(events))
	emit := func(n *node) {
		order = append(order, n)
		for id := range m.succ[n.id] {
			if d, ok := degree[id]; ok && d > 0 {
				degree[id] = d - 1
				if d == 1 {
					heap.Push(ready, r.byID[id])
				}
			}
		}
	}
	for i := 0; i < len(kept) || ready.Len() > 0; {
		if ready.Len() > 0 && (i == len(kept) || nodeLess((*ready)[0], kept[i])) {
			emit(heap.Pop(ready).(*node))
		} else {
			emit(kept[i])
			i++
		}
	}
	for i := 0; i < min(len(old), len(order)); i++ {
		if old[i] != order[i] {
			earliest = min(earliest, i)
			break
		}
	}
	earliest = min(earliest, len(order))
	for _, n := range m.all {
		n.pos = -1
	}
	r.order = order
	for i, n := range order {
		n.pos = i
	}
	r.indexOrder()
	r.displayOrder()

	var saved *foldCheckpoint
	keep := 0
	for i := range m.checkpoints {
		if m.checkpoints[i].pos > earliest {
			break
		}
		saved = &m.checkpoints[i]
		keep = i + 1
	}
	start := 0
	var state *state
	// Restore event annotations too: a later write can supersede or mark a
	// PREFIX event, and removing/rejudging that write must undo that effect.
	for _, n := range m.all {
		n.markers, n.superseded = nil, nil
	}
	if saved != nil {
		start, state = saved.pos, saved.state.checkpoint()
		for n, marks := range saved.marks {
			n.markers, n.superseded = slices.Clone(marks.markers), slices.Clone(marks.superseded)
		}
	}
	m.checkpoints = m.checkpoints[:keep]
	r.verdictsFrom(m.all, start)
	r.foldFrom(start, state)
	r.threads()
}

func (r *replayed) displayOrder() {
	var others []*node
	for _, n := range r.memo.all {
		if n.pos < 0 {
			others = append(others, n)
		}
	}
	sort.SliceStable(others, func(i, j int) bool { return nodeLess(others[i], others[j]) })
	r.nodes = make([]*node, 0, len(r.memo.all))
	for i, j := 0, 0; i < len(r.order) || j < len(others); {
		if j < len(others) && (i == len(r.order) || others[j].ts.Before(r.order[i].ts)) {
			r.nodes = append(r.nodes, others[j])
			j++
		} else {
			r.nodes = append(r.nodes, r.order[i])
			i++
		}
	}
}

func (r *replayed) foldFrom(start int, s *state) {
	if s == nil {
		s = newState(r.g, r.hb, true)
	}
	r.S = s
	r.memo.replayedFrom = start
	for i := start; i < len(r.order); i++ {
		if n := r.order[i]; n.status == "applied" {
			s.apply(n)
		}
		if (i+1)%foldCheckpointStride == 0 {
			r.saveCheckpoint(i + 1)
		}
	}
	if len(r.order) > 0 && len(r.order)%foldCheckpointStride != 0 {
		r.saveCheckpoint(len(r.order))
	}
}

func (r *replayed) saveCheckpoint(pos int) {
	m := r.memo
	if len(m.checkpoints) > 0 && m.checkpoints[len(m.checkpoints)-1].pos == pos {
		return
	}
	cp := foldCheckpoint{pos: pos, state: r.S.checkpoint(), marks: map[*node]nodeMarks{}}
	for _, n := range m.all {
		if len(n.markers)+len(n.superseded) > 0 {
			cp.marks[n] = nodeMarks{slices.Clone(n.markers), slices.Clone(n.superseded)}
		}
	}
	m.checkpoints = append(m.checkpoints, cp)
	if len(m.checkpoints) > foldCheckpointLimit {
		// Preserve the newest half at full density; thin the older half by
		// its smallest surrounding gap, keeping a sparse historical ladder.
		remove, gap := 0, m.checkpoints[1].pos
		for i := 1; i < len(m.checkpoints)-foldCheckpointLimit/2; i++ {
			if d := m.checkpoints[i+1].pos - m.checkpoints[i-1].pos; d < gap {
				remove, gap = i, d
			}
		}
		m.checkpoints = slices.Delete(m.checkpoints, remove, remove+1)
	}
}
