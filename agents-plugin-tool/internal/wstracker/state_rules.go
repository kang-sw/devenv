package wstracker

import "time"

// addRec adds a record "<viaID>><to>" unless one with the same id exists.
// It starts live and forced; set adjusts it before it is stored.
func (s *state) addRec(viaID string, viaTS time.Time, last *node, item, to, by, completion string, set func(*rec)) *rec {
	return s.addRecID(viaID+">"+to, viaID, viaTS, last, item, to, by, completion, set)
}

func (s *state) addRecID(id, viaID string, viaTS time.Time, last *node, item, to, by, completion string, set func(*rec)) *rec {
	if s.asg[id] != nil {
		return nil
	}
	r := &rec{id: id, item: item, to: to, by: by, sender: by, completion: completion, via: viaID, ts: viaTS,
		state: "live", forced: true, status: "todo", last: last}
	if set != nil {
		set(r)
	}
	s.asg[id] = r
	s.recs = append(s.recs, r)
	s.byItem[item] = append(s.byItem[item], r)
	return r
}

// extendRule adds ru's records, "<rule id>><member>" via the assign event,
// for everyone it covers who holds nothing on the item and was not
// unassigned from it.
func (s *state) extendRule(ru *rule, n *node, inherited bool) {
	for _, t := range s.coveredBy(ru) {
		if ru.excluded[t] || s.openRec(ru.item, t) != nil || s.asg[ru.id+">"+t] != nil {
			continue
		}
		s.addRecID(ru.id+">"+t, ru.via, n.ts, n, ru.item, t, ru.by, ru.completion, func(r *rec) {
			r.rule = ru.id
			if inherited {
				r.inherited = n.id
			}
		})
	}
}

// refreshLive re-applies the live rules: whoever entered a coverage since
// inherits the assignment.
func (s *state) refreshLive(n *node) {
	for _, ru := range s.rules {
		if ru.ended == "" && ru.state == "live" {
			s.extendRule(ru, n, true)
		}
	}
}

func (s *state) refreshOrgRules() {
	for _, ru := range s.rules {
		if ru.ended != "" || ru.org == "" {
			continue
		}
		if o := s.liveOrg(ru.org); o != nil && o.head != "" {
			ru.root = o.head
		}
	}
}

func (s *state) refreshItemRules(item string, n *node) {
	for _, ru := range s.liveRules(item) {
		s.extendRule(ru, n, true)
	}
}

// endRuleRecords ends ru with the live records it made; a handoff its holder
// accepted stays. It reports whether an org rule ended, so callers that end a
// batch of rules can defer re-extension until every rule and record is closed.
func (s *state) endRuleRecords(ru *rule, n *node) bool {
	if ru == nil {
		return false
	}
	if ru.ended == "" {
		ru.ended = n.id
	}
	for _, x := range append([]*rec(nil), s.byItem[ru.item]...) {
		if x.rule == ru.id && !x.ruleRequest && x.state == "live" {
			s.endRec(x, n, "unassigned")
		}
	}
	return ru.org != ""
}

func (s *state) endRule(ru *rule, n *node) {
	if s.endRuleRecords(ru, n) {
		s.refreshItemRules(ru.item, n)
	}
}

// unassignRule ends ru for item.unassign {root} or {rule}: a pending request
// is withdrawn, an accepted one unassigned, then the rule ends with its
// records.
func (s *state) unassignRule(ru *rule, n *node) {
	if s.unassignRuleRecords(ru, n) {
		s.refreshItemRules(ru.item, n)
	}
}

func (s *state) unassignRuleRecords(ru *rule, n *node) bool {
	if q := s.asg[ru.request]; ru.request != "" && q != nil {
		switch q.state {
		case "pending":
			s.endRec(q, n, "withdrawn")
		case "accepted":
			q.state, q.ended = "unassigned", n
		}
	}
	return s.endRuleRecords(ru, n)
}

// fallbackHolder is who holds a task nobody else holds: its creator, or
// when the creator was removed the first admin in the roles admin order.
func (s *state) fallbackHolder(it *item) string {
	if s.liveMember(it.author) {
		return it.author
	}
	for _, a := range s.aorder {
		if s.liveMember(a) {
			return a
		}
	}
	return ""
}

// liveFallback is item's live fallback record, if any.
func (s *state) liveFallback(item string) *rec {
	for _, r := range s.byItem[item] {
		if r.fallback && r.state == "live" {
			return r
		}
	}
	return nil
}

// activeOpen is item's opening while it is open: not ended, and the item
// still held by its fallback.
func (s *state) activeOpen(it *item) *opening {
	if it.open == nil || it.open.ended != nil || s.liveFallback(it.id) == nil {
		return nil
	}
	return it.open
}

// inScope reports that who may claim under o: a live member, inside the
// opener's org subtree for a team opening.
func (s *state) inScope(o *opening, who string) bool {
	if !s.liveMember(who) {
		return false
	}
	return o.scope == ScopeStore || s.below(o.by, who)
}

// foldFlag flags n in the final fold (marks): a write whose causal-past check
// passed but that an event earlier in fold order, concurrent with it, has
// already overtaken (a second claim of one opening, an approval of a request
// already answered, a move closing a cycle with a concurrent move). It takes
// no effect. A check-time fold skips it by the same test without flagging, so
// later verdicts see what a full replay shows.
func (s *state) foldFlag(n *node, reason rejection) {
	if s.marks {
		n.status, n.reason = "flagged", string(reason)
	}
}

// concurrently is the suffix of a fold-time flag's reason.
const concurrently = "; written concurrently, the first in fold order wins"

// settle keeps every task held after n: a live or pending record other than
// a rule request displaces the fallback, and with none the fallback holder
// gets a fresh record "<n>@<item>><holder>" at todo, so each fallback period
// has its own status register. A topic may be held by nobody.
func (s *state) settle(id string, n *node) {
	it := s.items[id]
	if it == nil || it.kind == "topic" {
		return
	}
	var fb *rec
	held := false
	for _, r := range s.byItem[id] {
		switch {
		case r.fallback:
			if r.state == "live" {
				fb = r
			}
		case isOpen(r) && !r.ruleRequest:
			held = true
		}
	}
	if held {
		if fb != nil {
			s.endRec(fb, n, "displaced")
		}
		if it.open != nil && it.open.ended == nil {
			it.open.ended = n // an opening never outlives the fallback hold
		}
		return
	}
	h := s.fallbackHolder(it)
	if fb != nil || h == "" {
		return
	}
	s.addRecID(n.id+"@"+id+">"+h, n.id, n.ts, n, id, h, h, "assignee", func(r *rec) {
		r.fallback, r.start = true, n
		if n.typ == TypeItemCreate && n.p.str("stage") == "review" {
			r.status = "idea" // an older create in the review stage
		}
	})
}

// settleAll settles every item, in insertion order.
func (s *state) settleAll(n *node) {
	for _, id := range s.iorder {
		s.settle(id, n)
	}
}

// assignPerson (273): below the author is forced (live), anyone else
// pending.
func (s *state) assignPerson(n *node, item, t, completion string) {
	a := n.member
	if s.members[t] == nil || s.openRec(item, t) != nil {
		return
	}
	forced := t == a || s.below(a, t)
	s.addRec(n.id, n.ts, n, item, t, a, completion, func(r *rec) {
		r.forced = forced
		if !forced {
			r.state = "pending"
		}
	})
}

// addRule (491): a rule whose root the author reaches is live at once; one
// rooted outside that reach sends the root a request and waits for its
// accept.
func (s *state) addRule(n *node, item string, x obj, completion string) {
	a, root := n.member, x.str("root")
	id := n.id + "~" + root
	if s.ruleByID[id] != nil || s.members[root] == nil {
		return
	}
	ru := &rule{id: id, via: n.id, item: item, by: a, root: root, assignRoot: root, org: x.str("org"), includeRoot: x["include_root"] == true,
		completion: completion, excluded: map[string]bool{}, state: "live", ts: n.ts}
	ru.levels, _ = x["levels"].(float64)
	// A concurrent head change can fold before an assign that was valid at its
	// causal snapshot. Consent and identity stay on the assign-time root, while
	// coverage starts at the head visible in this fold.
	if o := s.liveOrg(ru.org); o != nil && o.head != "" {
		ru.root = o.head
	}
	s.addRuleRec(ru)
	if root == a || s.below(a, root) {
		s.extendRule(ru, n, false)
		return
	}
	ru.state, ru.request = "pending", id
	s.addRecID(id, n.id, n.ts, n, item, root, a, completion, func(r *rec) {
		r.rule, r.ruleRequest, r.forced, r.state = id, true, false, "pending"
	})
}

func (s *state) addRuleRec(ru *rule) {
	s.rules = append(s.rules, ru)
	s.ruleByID[ru.id] = ru
}
