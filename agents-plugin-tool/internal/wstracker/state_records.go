package wstracker

// (295). Unassign, return, withdraw, and an accepted handoff beat any
// concurrent status write on it: the writes are marked, valid but not in
// effect (213).
func (s *state) endRec(r *rec, n *node, st string) {
	r.state, r.ended = st, n
	if ru := s.ruleByID[r.rule]; r.ruleRequest && ru != nil && ru.ended == "" {
		ru.ended = n.id // a withdrawn request ends its rule
	}
	for _, x := range s.byItem[r.item] {
		if x.handoffOf == r.id && x.state == "pending" {
			x.state, x.ended = "withdrawn", n
		}
	}
	for _, id := range s.rorder {
		if q := s.reqs[id]; q.of == r.id && q.state == "pending" {
			q.state, q.ended = "lapsed", n // a take request names one assignment
		}
	}
	if !s.marks {
		return
	}
	label := r.to + "'s assignment on " + alias(r.item)
	for _, w := range r.writes {
		if s.hb(w, n) {
			continue
		}
		mark(w, n, "removed", label, n.id, "", "")
		mark(n, w, "removed", label, n.id, "", "")
	}
	parent := s.items[r.item]
	for _, id := range s.iorder {
		it := s.items[id]
		if it.parent != r.item || it.parentBy.member != r.to || parent == nil || parent.author == r.to || s.hb(it.parentBy, n) {
			continue
		}
		f := "subtask under " + alias(r.item)
		mark(it.parentBy, n, "subtask", f, n.id, "", "")
		mark(n, it.parentBy, "subtask", f, n.id, "", "")
	}
}

// onEnded folds a write onto a record a concurrent event already ended.
func (s *state) onEnded(item, who string, n *node) {
	gone := s.lastEndedRec(item, who)
	if gone == nil {
		return
	}
	gone.writes = append(gone.writes, n)
	if s.marks && !s.hb(gone.ended, n) {
		lb := who + "'s assignment on " + alias(item)
		mark(n, gone.ended, "removed", lb, gone.ended.id, "", "")
		mark(gone.ended, n, "removed", lb, gone.ended.id, "", "")
	}
}

// handedConc reports a record handed on by an accept concurrent with n.
func (s *state) handedConc(r *rec, n *node) bool {
	return r.state == "handed" && r.ended != nil && r.ended.typ == TypeItemAccept && !s.hb(r.ended, n)
}

func (s *state) handedRec(item, who string, n *node) *rec {
	var last *rec
	for _, r := range s.byItem[item] {
		if r.to == who && s.handedConc(r, n) {
			last = r
		}
	}
	return last
}

// reverseHanded (295): an unassign, return, or removal of a record that is
// concurrent with the accept that handed it on wins, as removal beats writes
// (213). The accepted handoff ends withdrawn (the accept and its holder's
// writes are marked, valid but not in effect), and the record comes back
// live for the ending event to end.
func (s *state) reverseHanded(q *rec, n *node) *rec {
	acc := q.ended
	for _, h := range s.byItem[q.item] {
		if h.handoffOf == q.id && h.accepted == acc {
			if h.state == "live" {
				h.writes = append(h.writes, acc)
				s.endRec(h, n, "withdrawn")
			}
			break
		}
	}
	q.state, q.ended = "live", nil
	return q
}

func endState(r *rec) string {
	if r.state == "pending" {
		return "withdrawn"
	}
	return "unassigned"
}
