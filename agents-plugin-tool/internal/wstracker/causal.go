package wstracker

import "github.com/kang-sw/devenv/internal/wsledger"

// causal is the causality of a set of events computed from their streams
// and seen alone, as the protocol computes it: an event's causal past is its
// stream predecessor and the event each seen entry names, closed
// transitively. An event whose past is not all in the set (a gap), or that
// is broken or waiting, is unresolved and concurrent with nothing.
type causal struct {
	past map[wsledger.EventID]map[string]int
}

func newCausal(events []wsledger.Event) *causal {
	c := &causal{past: map[wsledger.EventID]map[string]int{}}
	streams := map[string]map[int]wsledger.Event{}
	for _, e := range events {
		if e.ID.Broken == "" {
			if streams[e.ID.Stream()] == nil {
				streams[e.ID.Stream()] = map[int]wsledger.Event{}
			}
			streams[e.ID.Stream()][e.ID.Seq] = e
		}
	}
	busy := map[wsledger.EventID]bool{}
	failed := map[wsledger.EventID]bool{}
	var visit func(e wsledger.Event) (map[string]int, bool)
	visit = func(e wsledger.Event) (map[string]int, bool) {
		if p, ok := c.past[e.ID]; ok {
			return p, true
		}
		if failed[e.ID] || busy[e.ID] || e.ID.Broken != "" || e.Verdict == wsledger.Waiting {
			return nil, false
		}
		busy[e.ID] = true
		defer delete(busy, e.ID)
		s := e.ID.Stream()
		var preds []wsledger.Event
		if e.ID.Seq > 1 {
			p, ok := streams[s][e.ID.Seq-1]
			if !ok {
				failed[e.ID] = true
				return nil, false
			}
			preds = append(preds, p)
		}
		for os, q := range e.Seen {
			if os == s || q <= 0 {
				continue
			}
			p, ok := streams[os][q]
			if !ok {
				failed[e.ID] = true
				return nil, false
			}
			preds = append(preds, p)
		}
		past := map[string]int{}
		for _, p := range preds {
			pp, ok := visit(p)
			if !ok {
				failed[e.ID] = true
				return nil, false
			}
			for k, q := range pp {
				if past[k] < q {
					past[k] = q
				}
			}
			if past[p.ID.Stream()] < p.ID.Seq {
				past[p.ID.Stream()] = p.ID.Seq
			}
		}
		c.past[e.ID] = past
		return past, true
	}
	for _, e := range events {
		visit(e)
	}
	return c
}

func (c *causal) hb(a, b wsledger.EventID) bool {
	if a.Stream() == b.Stream() {
		return a.Seq < b.Seq
	}
	return c.past[b][a.Stream()] >= a.Seq
}

// Concurrent has the Source method's meaning over the set.
func (c *causal) Concurrent(a, b wsledger.EventID) bool {
	_, okA := c.past[a]
	_, okB := c.past[b]
	if !okA || !okB || a == b {
		return false
	}
	return !c.hb(a, b) && !c.hb(b, a)
}
