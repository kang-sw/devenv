package wstracker

import "strings"

func (s *state) applyAssign(n *node) {
	p, a := n.p, n.member
	item := p.str("item")
	if s.items[item] == nil {
		return
	}
	if o := s.items[item].open; o != nil && o.ended == nil {
		o.ended = n // the creator delegated it another way
	}
	completion := p.str("completion")
	if !p.truthy("completion") {
		completion = "assignee"
	}
	if s.items[item].kind == "topic" {
		completion = "" // a topic record carries no status, so no completion mode either
	}
	if !p.null("targets") {
		l, _ := p.list("targets")
		for _, x := range l {
			switch t := x.(type) {
			case string:
				s.assignPerson(n, item, t, completion)
			case map[string]any:
				if _, ok := t["root"].(string); ok {
					s.addRule(n, item, obj(t), completion)
				}
			}
		}
		return
	}
	if p.str("mode") == "all" {
		// the older form: a rule rooted at the author, its id the event's
		levels, _ := p["levels"].(float64)
		ru := &rule{id: n.id, via: n.id, item: item, by: a, root: a, assignRoot: a, levels: levels, completion: completion,
			excluded: map[string]bool{}, state: "live", ts: n.ts}
		s.addRuleRec(ru)
		s.extendRule(ru, n, false)
		return
	}
	for _, t := range p.strList("people") {
		s.assignPerson(n, item, t, completion)
	}
}

// applyOpen opens a task to its scope. A creator's assign or another
// opening concurrent with it and earlier in fold order already holds or
// opened the item: the opening is flagged and takes no effect.
func (s *state) applyOpen(n *node) {
	it := s.items[n.p.str("item")]
	if it == nil {
		return
	}
	if held := s.holders(it); len(held) > 0 {
		s.foldFlag(n, rejection(it.alias+" has live or pending assignments ("+strings.Join(held, ", ")+"); only a task nobody else holds is opened"+concurrently))
		return
	}
	if o := s.activeOpen(it); o != nil {
		s.foldFlag(n, rejection(it.alias+" is already open ("+o.node.alias+")"+concurrently))
		return
	}
	it.open = &opening{node: n, by: n.member, scope: n.p.str("scope")}
}

// holders are the members holding a live or pending record on it, the
// fallback and rule requests left out, each once.
func (s *state) holders(it *item) []string {
	var held []string
	for _, x := range s.openRecs(it.id) {
		if !x.ruleRequest && !contains(held, x.to) {
			held = append(held, x.to)
		}
	}
	return held
}

// applyClaim gives the claimer a live record by the opener, which displaces
// the fallback and ends the opening (settle). A claim whose opening an
// earlier event in fold order ended (another claim, an assign, a
// withdrawal) is flagged and takes no effect.
func (s *state) applyClaim(n *node) {
	it := s.items[n.p.str("item")]
	if it == nil || it.open == nil {
		return
	}
	o := s.activeOpen(it)
	if o == nil {
		s.foldFlag(n, s.notOpen(it)+concurrently)
		return
	}
	a := n.member
	if !s.liveMember(a) {
		return
	}
	s.addRec(n.id, n.ts, n, it.id, a, o.by, "assignee", func(r *rec) {
		r.forced, r.sender, r.accepted = false, a, n // no accepted notice: nobody asked
	})
	o.ended = n
}

// applyRequest records a take request for the live assignment its assignee
// holds. When that assignment ended earlier in fold order, the request
// lapses at once.
func (s *state) applyRequest(n *node) {
	p := n.p
	it := s.items[p.str("item")]
	if it == nil || s.reqs[n.id] != nil {
		return
	}
	who := p.str("assignee")
	q := &takeReq{node: n, id: n.id, item: it.id, by: n.member, holder: who, note: p.str("note"), state: "pending"}
	if r := s.liveRec(it.id, who); r != nil && !r.fallback {
		q.of = r.id
	} else {
		q.state, q.ended = "lapsed", n
		if r := s.lastEndedRec(it.id, who); r != nil && !r.fallback {
			q.of, q.ended = r.id, r.ended
		}
	}
	s.reqs[n.id] = q
	s.rorder = append(s.rorder, n.id)
}

// applyApprove moves the named assignment to the requester: it ends handed,
// and the requester's new record starts live with its assigner, completion,
// status, and note, as an accepted handoff does (290). A requester who holds
// a record on the item by then gets no second one. An approval of a request
// that an earlier event in fold order answered, withdrew, or let lapse, or
// whose assignment such an event wrapped up, is flagged and takes no effect.
func (s *state) applyApprove(n *node) {
	p := n.p
	q := s.reqs[p.str("request")]
	if q == nil || q.item != p.str("item") {
		return
	}
	if q.state != "pending" {
		s.foldFlag(n, s.reqEnded(q)+concurrently)
		return
	}
	if r := s.takeWrapped(q); r != "" {
		s.foldFlag(n, r+concurrently)
		return
	}
	q.state, q.ended, q.answerNote = "approved", n, p.str("note")
	old := s.asg[q.of]
	if old == nil || old.state != "live" {
		return
	}
	s.endRec(old, n, "handed")
	if s.openRec(q.item, q.by) != nil || !s.liveMember(q.by) {
		return
	}
	s.addRec(n.id, n.ts, n, q.item, q.by, old.by, old.completion, func(r *rec) {
		r.forced, r.sender, r.handoffOf, r.accepted = false, n.member, old.id, n
		r.status, r.note, r.noteBy = old.status, old.note, old.noteBy
	})
}
