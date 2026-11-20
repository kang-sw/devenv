package wstracker

import (
	"strings"

	"github.com/kang-sw/devenv/internal/wsledger"
)

// ---------------------------------------------------------------- apply

// apply folds one valid event leniently: it is applied even when a
// concurrent event has since changed the state under it.
func (s *state) apply(n *node) {
	p, a := n.p, n.member
	switch n.typ {
	case TypeItemCreate:
		// (154 a) task and todo are one kind: a todo create folds as a task.
		kind := p.str("kind")
		if kind == "todo" {
			kind = "task"
		}
		it := &item{id: n.id, alias: n.alias, kind: kind, title: jsString(p["title"]),
			parent: p.str("parent"), author: a, ts: n.ts, node: n, priority: s.prio.def, ccExcluded: map[string]bool{}}
		it.project = p.str("project")
		if d, ok := p["due"].(string); ok && d != "" {
			it.due = &d
		}
		if d, ok := p["start"].(string); ok && d != "" {
			it.start = &d
		}
		if v, ok := p.intOf("priority"); ok {
			it.priority = v
		}
		// The create is the item's first body write: the register's first
		// head, and every line's source until an edit replaces it.
		f := &bodyFold{writes: []*node{n}, val: replaceVal(n), base: map[string]bodyVal{}}
		s.bodies[n.id] = f
		s.reg["it:"+n.id+":body"] = []head{{n, bodyText(n)}}
		s.bodyExtras(it, f)
		it.labels = labelsOf(p)
		it.after, it.afterSet = p.str("after"), p.has("after")
		it.placedBy, it.parentBy = n, n
		if s.items[n.id] == nil {
			s.iorder = append(s.iorder, n.id)
			s.place = append(s.place, placement{n.id, it.parent, it.after, it.afterSet})
		}
		s.items[n.id] = it
	case TypeItemEdit:
		it := s.items[p.str("item")]
		if it == nil {
			break
		}
		for _, f := range itemFields {
			if !p.has(f) {
				continue
			}
			key, label := "it:"+it.id+":"+f, f+" of "+it.alias
			switch f {
			case "title":
				it.title = s.write(key, label, n, jsString(p[f])).(string)
			case "body":
				// The running fold value, not the register's winner (merge.go).
				s.applyBody(it, n)
			case "project":
				v := ""
				if !p.null(f) {
					v = jsString(p[f])
				}
				it.project = s.write(key, label, n, v).(string)
			case "due", "start":
				var v any
				if !p.null(f) {
					v = jsString(p[f])
				}
				var d *string
				if w, ok := s.write(key, label, n, v).(string); ok {
					d = &w
				}
				if f == "due" {
					it.due = d
				} else {
					it.start = d
				}
			case "priority":
				v, _ := p.intOf(f)
				it.priority = s.write(key, label, n, v).(int)
			case "labels":
				it.labels = s.write(key, label, n, labelsOf(p)).(labelSet)
			case "stage":
				// An older event: review is the fallback holder's idea status,
				// null (todo) its todo. Held by anyone else, the item is not in
				// review, and the assignment wins.
				fb := s.liveFallback(it.id)
				if fb == nil {
					continue
				}
				v := "todo"
				if p.str(f) == "review" {
					v = "idea"
				}
				fb.writes = append(fb.writes, n)
				w := s.write("st:"+fb.id, "status of "+fb.to+" on "+alias(it.id), n, statusVal{v, "", a, n}).(statusVal)
				fb.status, fb.note, fb.noteBy, fb.last = w.status, w.note, w.by, w.n
			}
		}
		it.edited = n
	case TypeItemHide:
		if it := s.items[p.str("item")]; it != nil {
			it.hidden = s.write("it:"+it.id+":hidden", it.alias+" hidden", n, true).(bool)
		}
	case TypeItemMove:
		s.applyMove(n)
	case TypeItemCC, TypeItemUncc:
		s.applyCC(n)
	case TypeItemCCOrg, TypeItemUnccOrg:
		s.applyCCOrg(n)
	case TypeItemAssign:
		s.applyAssign(n)
	case TypeItemHandoff:
		from := p.str("assignee")
		if from == "" {
			from = a
		}
		r := s.liveRec(p.str("item"), from)
		to := p.str("to")
		if r == nil || r.fallback || s.members[to] == nil || s.openRec(p.str("item"), to) != nil {
			break
		}
		s.addRec(n.id, n.ts, n, r.item, to, r.by, r.completion, func(x *rec) {
			x.forced, x.state, x.sender, x.handoffOf, x.handoffNote = false, "pending", a, r.id, p.str("note")
		})
	case TypeItemAccept:
		// answers every pending assignment of the author on the item
		var acc []*rec
		for _, r := range s.byItem[p.str("item")] {
			if r.to == a && r.state == "pending" {
				acc = append(acc, r)
			}
		}
		if len(acc) == 0 {
			s.onEnded(p.str("item"), a, n)
			break
		}
		// a handoff whose original an earlier event in fold order wrapped up
		// is flagged, and the accept takes no effect
		if r := s.handoffWrapped(p.str("item"), a); r != "" {
			s.foldFlag(n, r+concurrently)
			break
		}
		for _, r := range acc {
			r.accepted, r.answerNote, r.last = n, p.str("note"), n
			if m := s.members[a]; m != nil {
				r.leadAtAccept = m.lead()
			}
			if r.ruleRequest { // (491) the rule goes live, forced from here on
				r.state = "accepted"
				if ru := s.ruleByID[r.rule]; ru != nil && ru.ended == "" {
					ru.state = "live"
					s.extendRule(ru, n, false)
				}
				continue
			}
			r.state = "live"
			if q := s.asg[r.handoffOf]; r.handoffOf != "" && q != nil && q.state == "live" {
				// (290) the new assignment starts with the original's status and note
				s.endRec(q, n, "handed")
				r.status, r.note, r.noteBy = q.status, q.note, q.noteBy
			}
		}
	case TypeItemDecline:
		for _, r := range s.byItem[p.str("item")] {
			if r.to != a || r.state != "pending" {
				continue
			}
			r.state, r.ended, r.answerNote = "declined", n, p.str("note")
			if ru := s.ruleByID[r.rule]; r.ruleRequest && ru != nil && ru.ended == "" {
				ru.ended = n.id
			}
		}
	case TypeItemReturn:
		item, who := p.str("item"), p.str("assignee")
		rq := s.acceptedReq(item, who)
		r := s.liveRec(item, who)
		if r != nil && r.fallback {
			r = nil // a fallback is not returned; with no other holding nothing happens
		}
		if r == nil {
			if q := s.handedRec(item, who, n); q != nil {
				r = s.reverseHanded(q, n)
			}
		}
		if r != nil && (r.accepted != nil || rq == nil) {
			s.endRec(r, n, "returned")
			r.answerNote = p.str("note")
			// (291) returning an accepted handoff restores the original
			// assignment with its prior status, unless its holder was removed or
			// holds another one now.
			if q := s.asg[r.handoffOf]; r.handoffOf != "" && q != nil && q.state == "handed" && q.ended == r.accepted &&
				s.liveMember(q.to) && s.openRec(q.item, q.to) == nil {
				q.state, q.ended, q.restored, r.restores = "live", nil, n, q.id
			}
		}
		if rq != nil {
			rq.state, rq.ended, rq.answerNote = "returned", n, p.str("note")
			s.endRule(s.ruleByID[rq.rule], n)
		}
	case TypeItemUnassign:
		item := p.str("item")
		if !p.null("rule") {
			if ru := s.ruleByID[p.str("rule")]; ru != nil && ru.item == item && ru.ended == "" && ru.by == a {
				s.unassignRule(ru, n)
			}
		} else if !p.null("root") {
			refresh := false
			for _, ru := range s.openRules(item) {
				if ru.by == a && ru.assignRoot == p.str("root") {
					refresh = s.unassignRuleRecords(ru, n) || refresh
				}
			}
			if refresh {
				s.refreshItemRules(item, n)
			}
		} else if p.truthy("assignee") {
			who := p.str("assignee")
			r := s.openRec(item, who)
			if r == nil {
				if q := s.handedRec(item, who, n); q != nil {
					r = s.reverseHanded(q, n)
				}
			}
			by := a
			if r != nil {
				by = r.by
			}
			for _, ru := range s.openRules(item) {
				if ru.by == by {
					if ru.excluded == nil {
						ru.excluded = map[string]bool{}
					}
					ru.excluded[who] = true
				}
			}
			if r != nil {
				s.endRec(r, n, endState(r))
			}
		} else {
			refresh := false
			for _, ru := range s.openRules(item) {
				if ru.by != a {
					continue
				}
				if ru.org != "" {
					refresh = s.unassignRuleRecords(ru, n) || refresh
				} else {
					ru.ended = n.id
					if q := s.asg[ru.request]; ru.request != "" && q != nil && q.state == "accepted" {
						q.state, q.ended = "unassigned", n
					}
				}
			}
			for _, x := range append([]*rec(nil), s.byItem[item]...) {
				if x.by == a && s.handedConc(x, n) {
					s.reverseHanded(x, n)
				}
			}
			for _, x := range s.openRecs(item) {
				if x.by == a || (x.state == "pending" && x.sender == a) {
					s.endRec(x, n, endState(x))
				}
			}
			if refresh {
				s.refreshItemRules(item, n)
			}
		}
	case TypeItemStatus, TypeItemClose, TypeItemConfirm, TypeItemReject, TypeItemReopen:
		who := a
		if n.typ != TypeItemClose && p.truthy("assignee") {
			who = p.str("assignee") // item.status: an org superior's write on who's assignment
		}
		item := p.str("item")
		r := s.liveRec(item, who)
		if r != nil && r.fallback && r.start != n && !s.hb(r.start, n) {
			r = nil // n had not seen the fallback start: it was meant for the record that ended
		}
		if r == nil {
			// folded onto an assignment a concurrent event ended
			s.onEnded(item, who, n)
			break
		}
		r.writes = append(r.writes, n)
		var v string
		switch n.typ {
		case TypeItemStatus:
			v = p.str("status")
		case TypeItemClose:
			v = "done"
			if r.completion == "assigner" {
				v = "in-review"
			}
		case TypeItemConfirm:
			v = "done"
		default:
			v = "doing"
		}
		w := s.write("st:"+r.id, "status of "+who+" on "+alias(item), n, statusVal{v, p.str("note"), a, n}).(statusVal)
		r.status, r.note, r.noteBy, r.last = w.status, w.note, w.by, w.n
	case TypeItemOpen:
		s.applyOpen(n)
	case TypeItemUnopen:
		if it := s.items[p.str("item")]; it != nil && it.open != nil && it.open.ended == nil {
			it.open.ended = n // after a claim or an assign folded first, a no-op
		}
	case TypeItemClaim:
		s.applyClaim(n)
	case TypeItemRequest:
		s.applyRequest(n)
	case TypeItemApprove:
		s.applyApprove(n)
	case TypeItemDeny, TypeItemUnrequest:
		// after the request was answered or lapsed, a no-op
		if q := s.reqs[p.str("request")]; q != nil && q.item == p.str("item") && q.state == "pending" {
			q.state, q.ended = "withdrawn", n
			if n.typ == TypeItemDeny {
				q.state, q.answerNote = "denied", p.str("note")
			}
		}
	case TypeCommentAdd:
		c := &comment{id: n.id, alias: n.alias, item: p.str("item"), author: a, body: jsString(p["body"]),
			replyTo: p.str("reply_to"), ts: n.ts, mentions: mentionsOf(p), attachments: attachmentsOf(p), refs: refsOf(p)}
		if s.comments[n.id] == nil {
			s.corder = append(s.corder, n.id)
		}
		s.comments[n.id] = c
	case TypeCommentEdit:
		if c := s.comments[p.str("comment")]; c != nil {
			c.body = s.write("cm:"+c.id, "text of comment "+c.alias, n, jsString(p["body"])).(string)
			c.edited = n
			w := s.regWinner("cm:" + c.id)
			c.mentions, c.attachments, c.refs = mentionsOf(w.p), attachmentsOf(w.p), refsOf(w.p)
		}
	case TypeCommentHide:
		if c := s.comments[p.str("comment")]; c != nil {
			c.hidden, c.hiddenBy = true, n
		}
	case wsledger.TypeMemberJoin:
		// A joined member is a root with no lead. A join declaring a key the
		// id already holds is a no-op.
		id := p.str("member")
		if s.members[id] == nil {
			// The profile rides in the join payload; a bare name (what wsledger.Join
			// writes today) becomes the given name.
			s.members[id] = &member{id: id, profile: profileOf(p, id), keys: []memberKey{keyOf(p.str("key"))}, joined: n}
			s.morder = append(s.morder, id)
		}
	case TypeMemberProfile:
		// (392) each field is its own register: concurrent changes merge by (203)
		id := a
		if !p.null("member") {
			id = p.disp("member")
		}
		m := s.members[id]
		if m == nil {
			break
		}
		pr, _ := p["profile"].(map[string]any)
		for _, f := range profileFields {
			if v, ok := pr[f]; ok {
				m.profile.set(f, pstr(s.write("prof:"+id+":"+f, profileLabel[f]+" of "+id, n, pstr(v))))
			}
		}
	case TypeSetLead:
		m := s.members[p.str("member")]
		if m == nil {
			break
		}
		leads := p.strList("leads")
		cycle := false
		for _, l := range leads {
			cycle = cycle || l == m.id || s.wouldCycle(m.id, l) // concurrent edits closing a cycle
		}
		if !cycle {
			m.leads = s.write("lead:"+m.id, "lead of "+m.id, n, leads).([]string)
		}
	case TypeOrgSet:
		id := p.str("org")
		o := s.orgs[id]
		if o == nil {
			o = &org{id: id}
			s.orgs[id] = o
			s.oorder = append(s.oorder, id)
		}
		o.name, o.head = p.str("name"), p.str("head")
	case TypeOrgRemove:
		if o := s.orgs[p.str("org")]; o != nil && o.removed == nil {
			o.removed, o.head = n, ""
		}
	case wsledger.TypeAddAdmin:
		s.addAdmin(p.str("member"))
	case wsledger.TypeRemoveAdmin:
		s.removeAdmin(p.str("member"))
	case wsledger.TypeAddKey:
		if m := s.members[p.str("member")]; m != nil {
			m.keys = append(m.keys, keyOf(p.str("key")))
		}
	case wsledger.TypeRevokeKey:
		if m := s.members[p.str("member")]; m != nil {
			k := keyOf(p.str("key"))
			kept := m.keys[:0:0]
			for _, x := range m.keys {
				if x.cmp != k.cmp {
					kept = append(kept, x)
				}
			}
			m.keys = kept
			m.revoked = append(m.revoked, revokedKey{key: k, by: n})
		}
	case wsledger.TypeRemoveMember:
		// (287) the member's live assignments end as unassigned, pending ones
		// as withdrawn; (295) one handed on by a concurrent accept comes back
		// first.
		id := p.str("member")
		m := s.members[id]
		if m == nil {
			break
		}
		m.removed = n
		s.removeAdmin(id)
		for _, oid := range s.oorder {
			if o := s.liveOrg(oid); o != nil && o.head == id {
				o.head = ""
			}
		}
		for _, x := range append([]*rec(nil), s.recs...) {
			if x.to == id && s.handedConc(x, n) {
				s.reverseHanded(x, n)
			}
		}
		for _, x := range append([]*rec(nil), s.recs...) {
			if x.to == id && isOpen(x) {
				s.endRec(x, n, endState(x))
			}
		}
		for _, rid := range s.rorder {
			if q := s.reqs[rid]; q.by == id && q.state == "pending" {
				q.state, q.ended = "withdrawn", n
			}
		}
	case TypeSetPriorityLevels:
		d, _ := n.p.intOf("default")
		s.prio = s.write("prio", "priority levels", n, priority{names: p.strList("names"), def: d}).(priority)
	case TypeSetAttachmentLimit:
		v, _ := p.posIntOf("max_bytes")
		s.attMax = s.write("att", "attachment limit", n, v).(float64)
	case TypeAddLabel:
		if s.labels[n.id] == nil {
			s.lorder = append(s.lorder, n.id)
		}
		s.labels[n.id] = &label{id: n.id, name: p.str("name"), color: p.str("color")}
	case TypeEditLabel:
		// name and color are field writes, merged like priority levels
		l := s.labels[p.str("label")]
		if l == nil {
			break
		}
		if p.has("name") {
			l.name = s.write("lb:"+l.id+":name", "name of label "+alias(l.id), n, p.str("name")).(string)
		}
		if p.has("color") {
			l.color = s.write("lb:"+l.id+":color", "color of label "+alias(l.id), n, p.str("color")).(string)
		}
	case TypeRemoveLabel:
		if l := s.labels[p.str("label")]; l != nil && l.removed == nil {
			l.removed = n
		}
	}
	switch {
	case strings.HasPrefix(n.typ, "roles.") || n.typ == TypeItemAssign || n.typ == wsledger.TypeMemberJoin:
		if strings.HasPrefix(n.typ, "roles.") || n.typ == wsledger.TypeMemberJoin {
			s.refreshOrgCC(n)
			s.refreshOrgRules()
		}
		s.refreshLive(n)
		s.settleAll(n)
	case n.typ == TypeItemCreate:
		s.settle(n.id, n)
	case strings.HasPrefix(n.typ, "item."):
		s.settle(p.str("item"), n)
	}
}

// applyMove folds item.move into the item's position register: concurrent
// moves of one item keep the register's winner, the other marked. A move
// whose shown place would sit in the item's own subtree, which only a
// concurrent move can cause, is flagged and ignored, the later in fold
// order losing. A parent change by an assignee concurrent with the end of
// their assignment on the new parent is marked as a subtask, as endRec
// marks a create.
func (s *state) applyMove(n *node) {
	p := n.p
	it := s.items[p.str("item")]
	if it == nil {
		return
	}
	key := "it:" + it.id + ":place"
	v := placeVal{parent: p.str("parent"), after: p.str("after"), afterSet: p.has("after")}
	if shown := s.peek(key, n, v).(placeVal); s.underItem(shown.parent, it) {
		s.foldFlag(n, rejection("moving "+it.alias+" under "+alias(shown.parent)+" would close a cycle with a concurrent move; ignored"))
		return
	}
	w := s.write(key, "position of "+it.alias, n, v).(placeVal)
	wn := s.regWinner(key)
	if wn == it.placedBy {
		return
	}
	if w.parent != it.parent {
		it.parentBy = wn
	}
	it.parent, it.after, it.afterSet, it.placedBy = w.parent, w.after, w.afterSet, wn
	s.place = append(s.place, placement{it.id, w.parent, w.after, w.afterSet})
	par := s.items[it.parent]
	if !s.marks || wn != n || it.parentBy != n || par == nil || par.author == n.member {
		return
	}
	for _, r := range s.byItem[par.id] {
		if r.to == n.member && r.ended != nil && !r.fallback && !s.hb(r.ended, n) {
			f := "subtask under " + alias(par.id)
			mark(n, r.ended, "subtask", f, r.ended.id, "", "")
			mark(r.ended, n, "subtask", f, r.ended.id, "", "")
		}
	}
}
