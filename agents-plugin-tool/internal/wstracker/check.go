package wstracker

import (
	"strconv"
	"strings"

	"github.com/kang-sw/devenv/internal/wsledger"
)

func itoa(i int) string { return strconv.Itoa(i) }

// rejection carries a flagged reason out of a rule check.
type rejection string

func (s *state) needItem(id, what string) (*item, rejection) {
	if it := s.items[id]; it != nil {
		return it, ""
	}
	return nil, rejection(what + " " + alias(id) + " is not in the state its author had seen")
}

func (s *state) needMember(id string) rejection {
	m := s.members[id]
	if m == nil {
		return rejection(`unknown member "` + id + `"`)
	}
	if m.removed != nil {
		return rejection(id + " was removed (" + m.removed.alias + ")")
	}
	return ""
}

// creatorGone (208, 292): an admin stands in for a removed creator on
// assign, unassign, edit, and hide.
func (s *state) creatorGone(it *item, a string) bool {
	cr := s.members[it.author]
	return s.admins[a] && cr != nil && cr.removed != nil
}

func (s *state) needAdmin(a string) rejection {
	if !s.admins[a] {
		return rejection("roles events are admin-only (" + a + " is not an admin)")
	}
	return ""
}

func (s *state) checkLeads(id string, p obj) rejection {
	var leads []string
	if p.truthy("leads") {
		if _, ok := p.list("leads"); !ok {
			return "leads must be a list"
		}
		leads = p.strList("leads")
	}
	if len(leads) > 1 {
		return rejection("leads must have length at most 1 (got " + itoa(len(leads)) + ")")
	}
	for _, l := range leads {
		if !s.liveMember(l) {
			return rejection(`lead "` + l + `" is not a member`)
		}
		if l == id || s.wouldCycle(id, l) {
			return rejection("cycle: " + l + " is already below " + id)
		}
	}
	return ""
}

func (s *state) checkFields(p obj) rejection {
	if p.has("title") && !p.truthy("title") {
		return "title cannot be empty"
	}
	if !p.null("project") {
		if _, ok := p["project"].(string); !ok {
			return "project is one optional free-text string"
		}
	}
	if !p.null("due") && !validDue(p["due"]) {
		return "due is an optional YYYY-MM-DD date"
	}
	if !p.null("priority") {
		if v, ok := p.intOf("priority"); !ok || v < 0 || v >= len(s.prio.names) {
			return rejection("priority must be a level 0.." + itoa(len(s.prio.names)-1) + " (" + strings.Join(s.prio.names, " / ") + ")")
		}
	}
	return ""
}

func (s *state) notAssignee(it *item, who string) rejection {
	if s.pendingRec(it.id, who) != nil {
		return rejection(who + " has not accepted the pending assignment on " + it.alias)
	}
	if gone := s.lastEndedRec(it.id, who); gone != nil {
		return rejection(who + "'s assignment on " + it.alias + " ended (" + gone.state + ", " + gone.ended.alias + ")")
	}
	if it.author == who {
		return rejection(who + " created " + it.alias + " but is not assigned to it (self-assign first)")
	}
	return rejection(who + " is not an assignee of " + it.alias)
}

func (s *state) needRec(it *item, who string) (*rec, rejection) {
	if r := s.liveRec(it.id, who); r != nil {
		return r, ""
	}
	return nil, s.notAssignee(it, who)
}

// check judges a domain event against the state folded from its causal
// past. It returns the flagged reason in the reference engine's words, or ""
// when the event is valid. The envelope, signer, and membership rules are
// the protocol's and are not repeated here.
func (s *state) check(n *node) rejection {
	p, a := n.p, n.member
	if !contains(origins, n.origin) {
		return "origin must be human, agent or agent-auto"
	}
	if n.origin == OriginAgentAuto {
		switch n.typ {
		case TypeItemAccept, TypeItemDecline, TypeItemReturn, TypeItemHandoff:
			return "an agent never accepts, declines, returns or hands off an assignment on its own (agent-auto is invalid here)"
		case TypeItemStatus, TypeItemClose:
		default:
			return "agent-auto is limited to item.status and item.close on the author's own assignment"
		}
	}
	needIt := func() (*item, rejection) { return s.needItem(p.disp("item"), "item") }

	switch n.typ {
	case TypeItemCreate:
		if !p.truthy("title") {
			return "title required"
		}
		if k, ok := p["kind"].(string); !ok || !contains(kinds, k) {
			return "kind must be task, todo or topic"
		}
		if r := s.checkFields(p); r != "" {
			return r
		}
		if p.truthy("parent") {
			par, r := s.needItem(p.disp("parent"), "parent")
			if r != "" {
				return r
			}
			if par.hidden {
				return rejection("parent " + par.alias + " is hidden")
			}
			if par.author != a && s.liveRec(par.id, a) == nil {
				return rejection("parent " + par.alias + " is neither assigned to nor created by " + a)
			}
		}

	case TypeItemEdit:
		it, r := needIt()
		if r != "" {
			return r
		}
		if it.author != a && !s.creatorGone(it, a) {
			return rejection("only the creator (" + it.author + ") edits " + it.alias)
		}
		if it.hidden {
			return rejection(it.alias + " is hidden")
		}
		any := false
		for _, f := range itemFields {
			any = any || p.has(f)
		}
		if !any {
			return rejection("nothing to edit (" + strings.Join(itemFields, ", ") + ")")
		}
		if p.has("priority") && p.null("priority") {
			return "priority cannot be cleared; set a level"
		}
		return s.checkFields(p)

	case TypeItemHide:
		it, r := needIt()
		if r != "" {
			return r
		}
		if it.author != a && !s.creatorGone(it, a) {
			return rejection("only the creator (" + it.author + ") hides " + it.alias)
		}
		if it.hidden {
			return rejection(it.alias + " is already hidden")
		}
		var open []string
		for _, x := range s.openRecs(it.id) {
			if x.to != a && x.status != "done" {
				open = append(open, x.to)
			}
		}
		if len(open) > 0 {
			return rejection("unassign the open assignments first (" + strings.Join(open, ", ") + ")")
		}

	case TypeItemAssign:
		it, r := needIt()
		if r != "" {
			return r
		}
		if it.hidden {
			return rejection(it.alias + " is hidden")
		}
		if it.kind == "topic" {
			return "topics have no assignment"
		}
		if it.author != a && !s.creatorGone(it, a) {
			return rejection("only the creator (" + it.author + ") assigns " + it.alias + "; an assignee hands off their own assignment with item.handoff")
		}
		completion := "assignee"
		if p.truthy("completion") {
			completion = jsString(p["completion"])
		}
		if completion != "assignee" && completion != "assigner" {
			return "completion must be assignee or assigner"
		}
		switch p["mode"] {
		case "all": // live and forced-only: everyone within N levels below the author
			levels, _ := p["levels"].(float64)
			if !(levels >= 1) {
				return "levels must be >= 1"
			}
			if len(s.reach(a, levels)) == 0 {
				return rejection("nobody is within " + jsNum(levels) + " levels below " + a)
			}
			for _, x := range s.liveRules(it.id) {
				if x.by == a && x.levels >= levels {
					return rejection("already assigned to everyone within " + jsNum(x.levels) + " levels (" + alias(x.id) + ")")
				}
			}
		case "chosen": // anyone: below the author = forced (live), anyone else = pending
			ppl := p.strList("people")
			if len(ppl) == 0 {
				return "choose at least one person"
			}
			free := false
			for _, x := range ppl {
				if x != a && !s.liveMember(x) {
					return rejection(x + " is not a member")
				}
			}
			for _, x := range ppl {
				free = free || s.openRec(it.id, x) == nil
			}
			if !free {
				return "everyone chosen already holds a live or pending assignment"
			}
		default:
			return "mode must be all or chosen"
		}

	case TypeItemHandoff:
		it, r := needIt()
		if r != "" {
			return r
		}
		if it.hidden {
			return rejection(it.alias + " is hidden")
		}
		from := a
		if p.truthy("assignee") {
			from = p.disp("assignee")
		}
		if from != a && !s.admins[a] { // (286) an admin may hand off any member's live assignment
			return rejection("only the assignee (" + from + ") hands off their assignment; an admin may hand off any member's")
		}
		rc, r := s.needRec(it, from)
		if r != "" {
			return r
		}
		if rc.status == "done" {
			return "assignment is done; nothing to hand off"
		}
		if !p.truthy("to") {
			return "to required"
		}
		to := p.disp("to")
		if to == from {
			return "hand off to someone else"
		}
		if r := s.needMember(to); r != "" {
			return r
		}
		if s.openRec(it.id, to) != nil {
			return rejection(to + " already holds a live or pending assignment on " + it.alias)
		}
		for _, x := range s.byItem[it.id] {
			if x.handoffOf == rc.id && x.state == "pending" {
				return rejection("a handoff of this assignment to " + x.to + " is already pending (" + alias(x.via) + ")")
			}
		}

	case TypeItemAccept, TypeItemDecline:
		it, r := needIt()
		if r != "" {
			return r
		}
		if s.pendingRec(it.id, a) == nil {
			if s.liveRec(it.id, a) != nil {
				return rejection(a + "'s assignment on " + it.alias + " is already live")
			}
			return rejection(a + " has no pending assignment on " + it.alias)
		}

	case TypeItemReturn:
		it, r := needIt()
		if r != "" {
			return r
		}
		who := p.disp("assignee")
		rc := s.liveRec(it.id, who)
		if rc == nil {
			if !p.truthy("assignee") {
				who = "?"
			}
			return rejection(who + " has no live assignment on " + it.alias)
		}
		lead := ""
		if m := s.members[rc.to]; m != nil {
			lead = m.lead()
		}
		if lead != a {
			l := lead
			if l == "" {
				l = "none: " + rc.to + " is a root"
			}
			return rejection("only " + rc.to + "'s direct lead (" + l + ") returns an accepted assignment")
		}
		if rc.accepted == nil {
			return rejection(rc.to + "'s assignment was forced by " + rc.by + " (their lead chain); only an accepted one is returnable")
		}

	case TypeItemUnassign:
		it, r := needIt()
		if r != "" {
			return r
		}
		if p.truthy("assignee") {
			who := p.disp("assignee")
			rc := s.openRec(it.id, who)
			if rc == nil {
				return rejection(who + " has no live or pending assignment on " + it.alias)
			}
			departed := (s.members[rc.to] != nil && s.members[rc.to].removed != nil) || (s.members[rc.by] != nil && s.members[rc.by].removed != nil)
			if rc.by != a && !(rc.state == "pending" && rc.sender == a) && !(s.admins[a] && departed) {
				extra := ""
				if rc.state == "pending" && rc.sender != rc.by {
					extra = " (or " + rc.sender + " withdraws)"
				}
				return rejection("only the assigner (" + rc.by + ") unassigns" + extra)
			}
		} else {
			has := false
			for _, x := range s.openRecs(it.id) {
				has = has || x.by == a || (x.state == "pending" && x.sender == a)
			}
			for _, x := range s.liveRules(it.id) {
				has = has || x.by == a
			}
			if !has {
				return rejection(a + " has no assignments to withdraw on " + it.alias)
			}
		}

	case TypeItemStatus:
		it, r := needIt()
		if r != "" {
			return r
		}
		if st, ok := p["status"].(string); !ok || !contains(settable, st) {
			return "status must be todo, doing or blocked (done goes through item.close)"
		}
		rc, r := s.needRec(it, a)
		if r != "" {
			return r
		}
		if rc.status == "done" {
			return "assignment is done; reopen it first"
		}

	case TypeItemClose:
		it, r := needIt()
		if r != "" {
			return r
		}
		rc, r := s.needRec(it, a)
		if r != "" {
			return r
		}
		if rc.status == "done" {
			return "assignment is already done"
		}
		if rc.status == "awaiting" {
			return rejection("already awaiting confirmation by " + rc.by)
		}

	case TypeItemConfirm, TypeItemReject:
		it, r := needIt()
		if r != "" {
			return r
		}
		who := p.disp("assignee")
		rc := s.liveRec(it.id, who)
		if rc == nil {
			return rejection("no live assignment of " + it.alias + " to " + who)
		}
		if rc.by != a {
			verb := "rejects"
			if n.typ == TypeItemConfirm {
				verb = "confirms"
			}
			return rejection("only the assigner (" + rc.by + ") " + verb)
		}
		if rc.completion != "assigner" {
			return "this assignment does not require confirmation"
		}
		if rc.status != "awaiting" {
			return rejection("assignment is " + rc.status + ", not awaiting confirmation")
		}

	case TypeItemReopen:
		it, r := needIt()
		if r != "" {
			return r
		}
		who := a
		if p.truthy("assignee") {
			who = p.disp("assignee")
		}
		rc := s.liveRec(it.id, who)
		if rc == nil {
			return s.notAssignee(it, who)
		}
		if rc.by != a && rc.to != a {
			return rejection("only the assigner (" + rc.by + ") or the assignee (" + rc.to + ") reopens")
		}
		if rc.status != "done" {
			return rejection("assignment is " + rc.status + ", not done")
		}

	case TypeCommentAdd:
		it, r := needIt()
		if r != "" {
			return r
		}
		if it.hidden {
			return rejection("thread " + it.alias + " is hidden")
		}
		if !p.truthy("body") {
			return "body required"
		}
		if p.truthy("reply_to") {
			q := s.comments[p.disp("reply_to")]
			if q == nil {
				return rejection("reply_to " + alias(p.disp("reply_to")) + " is not a comment its author had seen")
			}
			if q.item != p.disp("item") {
				return "reply_to must be in the same thread"
			}
			if q.hidden {
				return "cannot reply to a hidden comment"
			}
		}

	case TypeCommentEdit, TypeCommentHide:
		q := s.comments[p.disp("comment")]
		if q == nil {
			return rejection("comment " + alias(p.disp("comment")) + " is not in the state its author had seen")
		}
		if q.author != a {
			verb := "hides"
			if n.typ == TypeCommentEdit {
				verb = "edits"
			}
			return rejection("only the comment author (" + q.author + ") " + verb + " it")
		}
		if q.hidden {
			return "comment is already hidden"
		}
		if n.typ == TypeCommentEdit && !p.truthy("body") {
			return "body required"
		}

	case TypeSetLead:
		if r := s.needAdmin(a); r != "" {
			return r
		}
		if r := s.needMember(p.disp("member")); r != "" {
			return r
		}
		return s.checkLeads(p.disp("member"), p)

	case TypeSetPriorityLevels:
		if r := s.needAdmin(a); r != "" {
			return r
		}
		names, ok := p.list("names")
		if !ok || len(names) == 0 {
			return "names must be a non-empty list (lowest to highest)"
		}
		if len(names) > 9 {
			return "at most 9 priority levels"
		}
		for _, x := range names {
			if str, ok := x.(string); !ok || strings.TrimSpace(str) == "" {
				return "every level needs a display name"
			}
		}
		if d, ok := p.intOf("default"); !ok || d < 0 || d >= len(names) {
			return rejection("default must be a level 0.." + itoa(len(names)-1))
		}

	default:
		return rejection(`unknown event type "` + n.typ + `"`)
	}
	return ""
}

// isIdentity reports a protocol-owned identity event type.
func isIdentity(t string) bool {
	switch t {
	case wsledger.TypeMemberJoin, wsledger.TypeAddAdmin, wsledger.TypeRemoveAdmin,
		wsledger.TypeAddKey, wsledger.TypeRevokeKey, wsledger.TypeRemoveMember:
		return true
	}
	return false
}
