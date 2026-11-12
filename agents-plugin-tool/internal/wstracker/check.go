package wstracker

import (
	"math"
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

// needCreatorOrAssignee is the creator-or-live-assignee rule: a holds it
// when a created it or holds a live assignment on it (the fallback
// included). what prefixes the item in the reason ("parent " on a
// destination, "" on the item itself).
func (s *state) needCreatorOrAssignee(it *item, a, what string) rejection {
	if it.author == a || s.liveRec(it.id, a) != nil {
		return ""
	}
	return rejection(what + it.alias + " is neither assigned to nor created by " + a)
}

// underItem reports that id is it or sits in its subtree, walking parents
// up from id.
func (s *state) underItem(id string, it *item) bool {
	for cur, n := id, 0; cur != "" && n <= len(s.items); n++ {
		if cur == it.id {
			return true
		}
		x := s.items[cur]
		if x == nil {
			return false
		}
		cur = x.parent
	}
	return false
}

// checkMove judges item.move: the item rule (creator or live assignee, the
// creator-gone fallback included), then, only when the parent changes, the
// destination rule (item.create's parent rules; a topic takes children like
// any item), the subtree rule, and the after anchor.
func (s *state) checkMove(p obj, a string) rejection {
	it, r := s.needItem(p.disp("item"), "item")
	if r != "" {
		return r
	}
	if r := s.needCreatorOrAssignee(it, a, ""); r != "" && !s.creatorGone(it, a) {
		return r
	}
	if it.hidden {
		return rejection(it.alias + " is hidden")
	}
	if !p.has("parent") {
		return "parent required (an item id, or null for top level)"
	}
	if _, ok := p["parent"].(string); !ok && !p.null("parent") {
		return "parent must be an item id, or null for top level"
	}
	parent := p.str("parent")
	if parent != it.parent && parent != "" {
		if parent == it.id {
			return rejection(it.alias + " cannot move under itself")
		}
		par, r := s.needItem(parent, "parent")
		if r != "" {
			return r
		}
		if par.hidden {
			return rejection("parent " + par.alias + " is hidden")
		}
		if r := s.needCreatorOrAssignee(par, a, "parent "); r != "" {
			return r
		}
		if s.underItem(par.id, it) {
			return rejection("parent " + par.alias + " is in " + it.alias + "'s own subtree")
		}
	}
	if _, ok := p["after"].(string); !ok && !p.null("after") {
		return "after must be a sibling item id"
	}
	if after := p.str("after"); after != "" {
		if after == it.id {
			return rejection(it.alias + " cannot follow itself")
		}
		sib, r := s.needItem(after, "after")
		if r != "" {
			return r
		}
		if sib.parent != parent {
			at := "the top level"
			if parent != "" {
				at = alias(parent)
			}
			return rejection("after " + sib.alias + " is not a child of " + at)
		}
	}
	return ""
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
	if !p.null("labels") {
		l, ok := p.list("labels")
		if !ok {
			return "labels must be a list of label ids"
		}
		seen := map[string]bool{}
		for _, x := range l {
			id := jsString(x)
			if _, ok := x.(string); !ok || s.labels[id] == nil {
				return rejection(`label "` + id + `" is not in the store's label list`)
			}
			if s.labels[id].removed != nil {
				return rejection("label " + alias(id) + " (" + s.labels[id].name + ") was removed")
			}
			if seen[id] {
				return rejection("label " + alias(id) + " is listed twice")
			}
			seen[id] = true
		}
	}
	return ""
}

// isColor mirrors /^#[0-9a-f]{6}$/: a label colour.
func isColor(v any) bool {
	c, ok := v.(string)
	if !ok || len(c) != 7 || c[0] != '#' {
		return false
	}
	for _, r := range c[1:] {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// checkLabelName: a label name is a non-blank string without surrounding
// spaces, unique among the live labels other than except, ignoring case.
func (s *state) checkLabelName(p obj, except string) rejection {
	name, ok := p["name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return "name must be a non-blank string"
	}
	if strings.TrimSpace(name) != name {
		return "name has leading or trailing spaces"
	}
	if l := s.labelNamed(name, except); l != nil {
		return rejection(`label "` + name + `" already exists (` + alias(l.id) + ")")
	}
	return ""
}

// needLabel is the live label p.label names.
func (s *state) needLabel(p obj) (*label, rejection) {
	id := p.disp("label")
	l := s.labels[id]
	if l == nil {
		return nil, rejection(`label "` + id + `" is not in the store's label list`)
	}
	if l.removed != nil {
		return nil, rejection("label " + alias(id) + " (" + l.name + ") was removed")
	}
	return l, ""
}

func hasOID(l []attachment, oid string) bool {
	for _, x := range l {
		if x.oid == oid {
			return true
		}
	}
	return false
}

// isOID mirrors /^[0-9a-f]{64}$/: the sha256 of an attachment's content.
func isOID(v any) bool {
	o, ok := v.(string)
	if !ok || len(o) != 64 {
		return false
	}
	for _, c := range o {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// checkExtras (335, 338, 531): mentions is an explicit list of member ids
// (the text keeps @name for display); attachments is metadata only, each
// within attachments.max_bytes; refs is the item ids the text references
// with #ID. All three ride with a body. prior is the target's shown
// attachments in the author's seen state (nil on create and comment.add): an
// oid already there is exempt from the size check, so only an attachment the
// write adds is held to a limit an admin may since have lowered.
func (s *state) checkExtras(p obj, prior []attachment) rejection {
	if !p.null("mentions") {
		l, ok := p.list("mentions")
		if !ok {
			return "mentions must be a list of member ids"
		}
		for _, x := range l {
			id, ok := x.(string)
			if !ok {
				return "mentions must be a list of member ids"
			}
			m := s.members[id]
			if m == nil {
				return rejection(`mentioned "` + id + `" is not a member`)
			}
			if m.removed != nil {
				return rejection("mentioned " + id + " was removed (" + m.removed.alias + ")")
			}
		}
	}
	if !p.null("attachments") {
		const shape = "attachments must be a list of {oid, name, size, mime}"
		l, ok := p.list("attachments")
		if !ok {
			return shape
		}
		for _, x := range l {
			m, ok := x.(map[string]any)
			if !ok {
				return shape
			}
			a := obj(m)
			if !isOID(a["oid"]) {
				return "attachment oid must be the sha256 of the content (64 lowercase hex digits)"
			}
			if !a.truthy("name") || a.str("name") == "" {
				return "attachment name required"
			}
			size, ok := a["size"].(float64)
			if !ok || size != math.Trunc(size) || math.IsInf(size, 0) || size < 0 {
				return "attachment size must be a byte count"
			}
			if !a.truthy("mime") || a.str("mime") == "" {
				return "attachment mime type required"
			}
			if size > s.attMax && !hasOID(prior, a.str("oid")) {
				return rejection(`attachment "` + a.str("name") + `" is ` + jsNum(size) + " bytes, above the store limit of " +
					jsNum(s.attMax) + " bytes (attachments.max_bytes)")
			}
		}
	}
	if !p.null("refs") {
		l, ok := p.list("refs")
		if !ok {
			return "refs must be a list of item ids"
		}
		for _, x := range l {
			id, ok := x.(string)
			if !ok {
				return "refs must be a list of item ids"
			}
			if s.items[id] == nil {
				return rejection("referenced " + alias(id) + " is not an item its author had seen")
			}
		}
	}
	return ""
}

const baseShape = "base must be a list of event ids (the body revision the editor saw)"

const stageShape = "stage must be review (null or absent: todo)"

// retiredStage refuses a new write carrying stage. The review stage is
// retired for the idea status; older events with it still fold (apply).
const retiredStage = "stage is retired; a task before todo is its holder's idea status (item.status)"

// checkStage checks an older item.edit stage change as it was checked when
// stages were written: tasks and todos only, review or null (todo). Moving
// to review needs an item with no live or pending assignment (the fallback
// is neither).
func (s *state) checkStage(it *item, p obj) rejection {
	if it.kind == "topic" {
		return "topics have no stage"
	}
	if p.null("stage") {
		return ""
	}
	if p["stage"] != "review" {
		return stageShape
	}
	var held []string
	for _, x := range s.openRecs(it.id) {
		if !contains(held, x.to) {
			held = append(held, x.to)
		}
	}
	if len(held) > 0 {
		return rejection(it.alias + " has live or pending assignments (" + strings.Join(held, ", ") + "); unassign them before moving it to review")
	}
	return ""
}

const targetShape = "a target is a member id or a rule {root, include_root?, levels?}"

// checkTargets checks item.assign targets (490)-(492): member ids and rules
// {root, include_root?, levels?}, mixed, one rule per root. A rule is
// checked like the older "everyone within N": someone must be in it, and
// the author's live rules must not already cover it.
func (s *state) checkTargets(it *item, a string, p obj) rejection {
	targets, ok := p.list("targets")
	if !ok || len(targets) == 0 {
		return "choose at least one person or rule"
	}
	roots := map[string]bool{}
	rules := 0
	for _, x := range targets {
		if id, ok := x.(string); ok {
			if id != a && !s.liveMember(id) {
				return rejection(id + " is not a member")
			}
			continue
		}
		m, ok := x.(map[string]any)
		root, _ := m["root"].(string)
		if !ok || root == "" {
			return targetShape
		}
		t := obj(m)
		if _, ok := t["include_root"].(bool); !ok && !t.null("include_root") {
			return targetShape
		}
		ru := &rule{root: root, includeRoot: t["include_root"] == true}
		if !t.null("levels") {
			l, ok := t["levels"].(float64)
			if !ok || l != math.Trunc(l) || l < 1 || math.IsInf(l, 0) {
				return "rule levels must be a whole number >= 1"
			}
			ru.levels = l
		}
		if !s.liveMember(root) {
			return rejection(root + " is not a member")
		}
		if roots[root] {
			return rejection("one rule per root in an assign (" + root + ")")
		}
		roots[root] = true
		rules++
		if len(s.coveredBy(ru)) == 0 {
			within := ""
			if ru.levels != 0 {
				within = "within " + jsNum(ru.levels) + " levels "
			}
			return rejection("nobody is " + within + "below " + root)
		}
		for _, y := range s.liveRules(it.id) {
			if y.by == a && s.ruleCovers(y, ru) {
				return rejection("the rule rooted at " + root + " is already covered by " + alias(y.id))
			}
		}
	}
	if rules == 0 {
		for _, x := range targets {
			if s.openRec(it.id, jsString(x)) == nil {
				return ""
			}
		}
		return "everyone chosen already holds a live or pending assignment"
	}
	return ""
}

const topicOne = "a topic has one holder: assign it to exactly one person"

// checkTopicAssign (153 b): a topic is held by one person, for ownership
// only. An assign names exactly one member, as targets [id] or mode chosen
// with one person, and only while the topic has no live or pending record
// (a pending handoff included). Its completion is accepted and not stored.
func (s *state) checkTopicAssign(it *item, p obj) rejection {
	one := false
	if !p.null("targets") {
		l, ok := p.list("targets")
		if ok && p.null("mode") && len(l) == 1 {
			_, one = l[0].(string)
		}
	} else if l, ok := p.list("people"); ok && p["mode"] == "chosen" && len(l) == 1 {
		_, one = l[0].(string)
	}
	if !one {
		return topicOne
	}
	var held []string
	for _, x := range s.openRecs(it.id) {
		if !contains(held, x.to) {
			held = append(held, x.to)
		}
	}
	if len(held) > 0 {
		return rejection("topic " + it.alias + " is already held by " + strings.Join(held, ", ") + "; unassign " + held[0] + " first")
	}
	return ""
}

func (s *state) notAssignee(it *item, who string) rejection {
	if r := s.pendingRec(it.id, who); r != nil && s.openRec(it.id, who) == nil {
		// who holds only a rule request, which never holds the item itself.
		if ru := s.ruleByID[r.rule]; ru != nil && !ru.includeRoot {
			return rejection(who + " holds only a pending rule request on " + it.alias + "; accepting it assigns the members below " + who + ", not " + who)
		}
		return rejection(who + " has not accepted the pending rule request on " + it.alias)
	}
	if s.pendingRec(it.id, who) != nil {
		return rejection(who + " has not accepted the pending assignment on " + it.alias)
	}
	var gone *rec // the last ended holding of who's, a fallback left out
	for _, x := range s.byItem[it.id] {
		if x.to == who && x.ended != nil && !x.fallback {
			gone = x
		}
	}
	if gone != nil {
		return rejection(who + "'s assignment on " + it.alias + " ended (" + gone.state + ", " + gone.ended.alias + ")")
	}
	if it.author == who {
		return rejection(who + " created " + it.alias + " but is not assigned to it (self-assign first)")
	}
	return rejection(who + " is not an assignee of " + it.alias)
}

// notOpen is why it cannot be claimed: never opened, or the event that
// ended its opening.
func (s *state) notOpen(it *item) rejection {
	o := it.open
	if o == nil || o.ended == nil && s.liveFallback(it.id) == nil {
		return rejection(it.alias + " is not open for claiming")
	}
	e := o.ended
	switch e.typ {
	case TypeItemClaim:
		return rejection(e.member + " already claimed " + it.alias + " (" + e.alias + ")")
	case TypeItemUnopen:
		return rejection(e.member + " withdrew the opening of " + it.alias + " (" + e.alias + ")")
	case TypeItemAssign:
		return rejection(e.member + " assigned " + it.alias + " (" + e.alias + "), which ended its opening")
	}
	return rejection("the opening of " + it.alias + " ended (" + e.alias + ")")
}

// needReq is the take request p.request names on it.
func (s *state) needReq(it *item, p obj) (*takeReq, rejection) {
	q := s.reqs[p.disp("request")]
	if q == nil || q.item != it.id {
		return nil, rejection(alias(p.disp("request")) + " is not a take request on " + it.alias)
	}
	return q, ""
}

// reqEnded is why q is no longer pending.
func (s *state) reqEnded(q *takeReq) rejection {
	e, rq := q.ended, alias(q.id)
	switch q.state {
	case "approved":
		return rejection(e.member + " already approved " + rq + " (" + e.alias + ")")
	case "denied":
		return rejection(e.member + " denied " + rq + " (" + e.alias + ")")
	case "withdrawn":
		return rejection(rq + " was withdrawn (" + e.alias + ")")
	}
	return rejection(rq + " lapsed: " + q.holder + "'s assignment on " + alias(q.item) + " ended (" + e.alias + ")")
}

// takeWrapped refuses the approval of pending q when the assignment it
// names is done or dropped.
func (s *state) takeWrapped(q *takeReq) rejection {
	if old := s.asg[q.of]; old != nil && old.state == "live" && wrapped(old.status) {
		return rejection(q.holder + "'s assignment on " + alias(q.item) + " is " + old.status + "; nothing to take")
	}
	return ""
}

// handoffWrapped refuses who's accepting their pending assignments on item
// when one is a handoff of an assignment that is done or dropped.
func (s *state) handoffWrapped(item, who string) rejection {
	for _, r := range s.byItem[item] {
		if r.to != who || r.state != "pending" || r.handoffOf == "" {
			continue
		}
		if old := s.asg[r.handoffOf]; old != nil && old.state == "live" && wrapped(old.status) {
			return rejection(old.to + "'s assignment on " + alias(item) + " is " + old.status + "; nothing to accept")
		}
	}
	return ""
}

func (s *state) needRec(it *item, who string) (*rec, rejection) {
	if r := s.liveRec(it.id, who); r != nil {
		return r, ""
	}
	return nil, s.notAssignee(it, who)
}

// checkStatusOf judges item.status on another member's assignment (Decisions
// of the superior wrap-up): an org superior of who sets blocked, done (final,
// no review step), or dropped; the assigner, a peer included, only drops it.
// The record must be live (the fallback included), not pending, and not
// already wrapped up.
func (s *state) checkStatusOf(it *item, a, who, st string) rejection {
	if !contains(superiorSettable, st) {
		return "status on another member's assignment must be blocked, done or dropped"
	}
	rc, r := s.needRec(it, who)
	if r != "" {
		return r
	}
	if !s.below(a, who) {
		if rc.by != a {
			return rejection("only " + who + " or an org superior of " + who + " sets the status of " + who + "'s assignment")
		}
		if st != "dropped" {
			return rejection("the assigner (" + a + ") only drops " + who + "'s assignment; " + st + " is for an org superior of " + who)
		}
	}
	if wrapped(rc.status) {
		return rejection("assignment is " + rc.status + "; reopen it first")
	}
	return ""
}

// checkWrite judges a new write by the rules a replay never applies:
// validate runs it after check, so an event written before such a rule
// existed keeps folding as it did (no fold flag, no migration). The
// reference engine's store.dispatch runs the same rules (writeRefusal).
func (s *state) checkWrite(n *node) rejection {
	if n.typ == TypeItemAssign {
		return s.assignBelowConfirmed(n.member, n.p)
	}
	return ""
}

// assignBelowConfirmed refuses completion assignee on an item.assign that
// reaches below its writer a: a member below a, a rule rooted at a or below
// a, or mode all. a confirms the wrap-up of such work (its holder sends it
// to review). A topic keeps no completion and is left out.
func (s *state) assignBelowConfirmed(a string, p obj) rejection {
	if p.truthy("completion") && p["completion"] != "assignee" {
		return ""
	}
	if it := s.items[p.disp("item")]; it == nil || it.kind == "topic" {
		return ""
	}
	var low []string
	person := func(t string) {
		if t != a && s.below(a, t) {
			low = append(low, t)
		}
	}
	rule := func(root string) {
		if root == a || s.below(a, root) {
			low = append(low, "the rule rooted at "+root)
		}
	}
	switch {
	case !p.null("targets"):
		l, _ := p.list("targets")
		for _, x := range l {
			switch t := x.(type) {
			case string:
				person(t)
			case map[string]any:
				if root, ok := t["root"].(string); ok {
					rule(root)
				}
			}
		}
	case p["mode"] == "all":
		rule(a)
	default:
		for _, t := range p.strList("people") {
			person(t)
		}
	}
	if len(low) == 0 {
		return ""
	}
	return rejection("completion must be assigner for an assignment below " + a + " (" + strings.Join(low, ", ") + "): " + a + " confirms its wrap-up")
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
			if n.typ == TypeItemStatus && p.truthy("assignee") && p.disp("assignee") != a {
				return "agent-auto is limited to item.status and item.close on the author's own assignment"
			}
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
		if n.synthetic && p.has("stage") {
			return retiredStage
		}
		if !p.null("stage") {
			if p["kind"] == "topic" {
				return "topics have no stage"
			}
			if p["stage"] != "review" {
				return stageShape
			}
		}
		if r := s.checkExtras(p, nil); r != "" {
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
			if r := s.needCreatorOrAssignee(par, a, "parent "); r != "" {
				return r
			}
		}
		// (528) the sibling it follows; null or "" = first. An anchor that is
		// not a sibling when the view is built falls back to the end.
		if _, ok := p["after"].(string); !ok && !p.null("after") {
			return "after must be a sibling item id"
		}

	case TypeItemEdit:
		it, r := needIt()
		if r != "" {
			return r
		}
		// The creator or a live assignee edits every field.
		if r := s.needCreatorOrAssignee(it, a, ""); r != "" && !s.creatorGone(it, a) {
			return r
		}
		if it.hidden {
			return rejection(it.alias + " is hidden")
		}
		any := false
		for _, f := range itemFields {
			any = any || p.has(f)
		}
		if !any {
			return rejection("nothing to edit (" + strings.Join(editFields, ", ") + ")")
		}
		if p.has("priority") && p.null("priority") {
			return "priority cannot be cleared; set a level"
		}
		if r := s.checkFields(p); r != "" {
			return r
		}
		if n.synthetic && p.has("stage") {
			return retiredStage
		}
		if p.has("stage") {
			if r := s.checkStage(it, p); r != "" {
				return r
			}
		}
		if (!p.null("mentions") || !p.null("attachments")) && !p.has("body") {
			return "mentions and attachments go with a body edit"
		}
		if !p.null("refs") && !p.has("body") {
			return "refs go with a body edit"
		}
		// base is the body revision the editor saw; an id naming no body
		// write folds as a whole replace, so only its shape is checked.
		if !p.null("base") {
			if !p.has("body") {
				return "base goes with a body edit"
			}
			l, ok := p.list("base")
			if !ok {
				return baseShape
			}
			for _, x := range l {
				if _, ok := x.(string); !ok {
					return baseShape
				}
			}
		}
		return s.checkExtras(p, it.attachments)

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
			if x.to != a && !wrapped(x.status) {
				open = append(open, x.to)
			}
		}
		if len(open) > 0 {
			return rejection("unassign the open assignments first (" + strings.Join(open, ", ") + ")")
		}

	case TypeItemMove:
		return s.checkMove(p, a)

	case TypeItemAssign:
		it, r := needIt()
		if r != "" {
			return r
		}
		if it.hidden {
			return rejection(it.alias + " is hidden")
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
		if it.kind == "topic" {
			if r := s.checkTopicAssign(it, p); r != "" {
				return r
			}
		}
		if !p.null("targets") {
			if !p.null("mode") {
				return "item.assign takes targets or mode, not both"
			}
			return s.checkTargets(it, a, p)
		}
		switch p["mode"] {
		case "all": // the older form of a rule rooted at the author
			levels, _ := p["levels"].(float64)
			if !(levels >= 1) {
				return "levels must be >= 1"
			}
			if len(s.reach(a, levels)) == 0 {
				return rejection("nobody is within " + jsNum(levels) + " levels below " + a)
			}
			for _, x := range s.liveRules(it.id) {
				if x.by != a || !s.ruleCovers(x, &rule{root: a, levels: levels}) {
					continue
				}
				if x.root == a && x.levels != 0 {
					return rejection("already assigned to everyone within " + jsNum(x.levels) + " levels (" + alias(x.id) + ")")
				}
				return rejection("the rule rooted at " + a + " is already covered by " + alias(x.id))
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
		if rc.fallback {
			return rejection(from + " holds " + it.alias + " only because nobody else does; assign it instead")
		}
		if wrapped(rc.status) {
			return rejection("assignment is " + rc.status + "; nothing to hand off")
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

	case TypeItemOpen:
		it, r := needIt()
		if r != "" {
			return r
		}
		if it.hidden {
			return rejection(it.alias + " is hidden")
		}
		if it.author != a && !s.creatorGone(it, a) {
			return rejection("only the creator (" + it.author + ") opens " + it.alias + " for claiming")
		}
		if it.kind == "topic" {
			return "a topic is not opened for claiming; its creator assigns its one holder"
		}
		if sc := p["scope"]; sc != ScopeTeam && sc != ScopeStore {
			return "scope must be team (the opener's org subtree) or store (every member)"
		}
		if o := s.activeOpen(it); o != nil {
			return rejection(it.alias + " is already open (" + o.node.alias + ")")
		}
		if held := s.holders(it); len(held) > 0 {
			return rejection(it.alias + " has live or pending assignments (" + strings.Join(held, ", ") + "); only a task nobody else holds is opened")
		}
		if fb := s.liveFallback(it.id); fb != nil && wrapped(fb.status) {
			return rejection(it.alias + " is " + fb.status + "; nothing to open")
		}

	case TypeItemUnopen:
		it, r := needIt()
		if r != "" {
			return r
		}
		if it.author != a && !s.creatorGone(it, a) {
			return rejection("only the creator (" + it.author + ") withdraws the opening of " + it.alias)
		}
		if s.activeOpen(it) == nil {
			return s.notOpen(it)
		}

	case TypeItemClaim:
		it, r := needIt()
		if r != "" {
			return r
		}
		if it.hidden {
			return rejection(it.alias + " is hidden")
		}
		o := s.activeOpen(it)
		if o == nil {
			return s.notOpen(it)
		}
		if fb := s.liveFallback(it.id); fb.to == a {
			return rejection(a + " already holds " + it.alias)
		}
		if !s.inScope(o, a) {
			return rejection(a + " is not in " + o.by + "'s org subtree; " + it.alias + " is open to that team only")
		}

	case TypeItemRequest:
		it, r := needIt()
		if r != "" {
			return r
		}
		if it.hidden {
			return rejection(it.alias + " is hidden")
		}
		if !p.truthy("assignee") {
			return "assignee required (the holder of the assignment to take)"
		}
		who := p.disp("assignee")
		if who == a {
			return rejection(a + " cannot request their own assignment")
		}
		rc, r := s.needRec(it, who)
		if r != "" {
			return r
		}
		if rc.fallback {
			return rejection(who + " holds " + it.alias + " only because nobody else does; it is taken by a claim once its creator opens it")
		}
		if wrapped(rc.status) {
			return rejection("assignment is " + rc.status + "; nothing to take")
		}
		if s.openRec(it.id, a) != nil {
			return rejection(a + " already holds a live or pending assignment on " + it.alias)
		}
		for _, id := range s.rorder {
			if q := s.reqs[id]; q.by == a && q.of == rc.id && q.state == "pending" {
				return rejection(a + "'s request for " + who + "'s assignment on " + it.alias + " is already pending (" + alias(q.id) + ")")
			}
		}

	case TypeItemApprove, TypeItemDeny:
		it, r := needIt()
		if r != "" {
			return r
		}
		q, r := s.needReq(it, p)
		if r != "" {
			return r
		}
		if q.state != "pending" {
			return s.reqEnded(q)
		}
		if a != q.holder && it.author != a && !s.creatorGone(it, a) {
			verb := "approves"
			if n.typ == TypeItemDeny {
				verb = "denies"
			}
			return rejection("only " + q.holder + " (the assignment's holder) or the creator (" + it.author + ") " + verb + " " + alias(q.id))
		}
		// A wrapped-up record stays live, so its pending requests never
		// lapse; approving one would hand over finished work. A denial
		// stays open.
		if n.typ == TypeItemApprove {
			if r := s.takeWrapped(q); r != "" {
				return r
			}
		}

	case TypeItemUnrequest:
		it, r := needIt()
		if r != "" {
			return r
		}
		q, r := s.needReq(it, p)
		if r != "" {
			return r
		}
		if q.state != "pending" {
			return s.reqEnded(q)
		}
		if q.by != a {
			return rejection("only the requester (" + q.by + ") withdraws " + alias(q.id))
		}

	case TypeItemCC, TypeItemUncc:
		// Open to every member, whoever may edit the item: a CC carries no
		// duty. Adding a member already CC'd, or removing one who is not, is
		// a no-op. A removed member's CC may still be removed.
		it, r := needIt()
		if r != "" {
			return r
		}
		if it.hidden {
			return rejection(it.alias + " is hidden")
		}
		m, ok := p["member"].(string)
		if !ok || m == "" {
			return "member must be a member id"
		}
		if n.typ == TypeItemCC {
			return s.needMember(m)
		}
		if s.members[m] == nil {
			return rejection(`unknown member "` + m + `"`)
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
		// A wrapped-up record stays live, so a pending handoff of it never
		// lapses; accepting one would take over finished work. A decline
		// stays open.
		if n.typ == TypeItemAccept {
			if r := s.handoffWrapped(it.id, a); r != "" {
				return r
			}
		}

	case TypeItemReturn:
		it, r := needIt()
		if r != "" {
			return r
		}
		// an accepted assignment, and an accepted rule rooted at the assignee
		who := p.disp("assignee")
		rc := s.liveRec(it.id, who)
		if rc != nil && rc.fallback {
			rc = nil // nobody else holds it: nothing to return
		}
		rq := s.acceptedReq(it.id, who)
		if rc == nil && rq == nil {
			if !p.truthy("assignee") {
				who = "?"
			}
			return rejection(who + " has no live assignment on " + it.alias)
		}
		to := who
		if rc != nil {
			to = rc.to
		}
		lead := ""
		if m := s.members[to]; m != nil {
			lead = m.lead()
		}
		if lead != a {
			l := lead
			if l == "" {
				l = "none: " + to + " is a root"
			}
			return rejection("only " + to + "'s direct lead (" + l + ") returns an accepted assignment")
		}
		if rq == nil && rc.accepted == nil {
			return rejection(rc.to + "'s assignment was forced by " + rc.by + " (their lead chain); only an accepted one is returnable")
		}

	case TypeItemUnassign:
		it, r := needIt()
		if r != "" {
			return r
		}
		if !p.null("rule") { // one rule of the author's, with its request and records
			if p.truthy("assignee") {
				return "item.unassign takes assignee or rule, not both"
			}
			id, ok := p["rule"].(string)
			if !ok {
				return "rule must be a rule id"
			}
			ru := s.ruleByID[id]
			if ru == nil || ru.item != it.id || ru.ended != "" {
				return rejection(alias(id) + " is not an open rule on " + it.alias)
			}
			if ru.by != a {
				return rejection("only the rule's author (" + ru.by + ") ends " + alias(id))
			}
			if !p.null("root") && p["root"] != ru.root {
				return rejection(alias(id) + " is rooted at " + ru.root + ", not " + p.disp("root"))
			}
		} else if !p.null("root") { // the author's rules rooted at root, with their requests and records
			if p.truthy("assignee") {
				return "item.unassign takes assignee or root, not both"
			}
			has := false
			for _, x := range s.openRules(it.id) {
				has = has || x.by == a && x.root == p.str("root")
			}
			if !has {
				return rejection(a + " has no rule rooted at " + p.disp("root") + " on " + it.alias)
			}
		} else if p.truthy("assignee") {
			who := p.disp("assignee")
			rc := s.openRec(it.id, who)
			if rc == nil {
				return rejection(who + " has no live or pending assignment on " + it.alias)
			}
			departed := (s.members[rc.to] != nil && s.members[rc.to].removed != nil) || (s.members[rc.by] != nil && s.members[rc.by].removed != nil)
			holder := it.kind == "topic" && rc.to == a && rc.state == "live" // (153 b) the holder releases a topic
			if rc.by != a && !(rc.state == "pending" && rc.sender == a) && !(s.admins[a] && departed) && !holder {
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
			for _, x := range s.openRules(it.id) {
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
		if it.kind == "topic" {
			return "topics have no status"
		}
		st, _ := p["status"].(string)
		who := a
		if p.truthy("assignee") {
			who = p.disp("assignee")
		}
		if who != a {
			return s.checkStatusOf(it, a, who, st)
		}
		if !contains(settable, st) {
			return "status must be idea, todo, doing, blocked, in-review or dropped (done goes through item.close)"
		}
		rc, r := s.needRec(it, a)
		if r != "" {
			return r
		}
		if wrapped(rc.status) {
			return rejection("assignment is " + rc.status + "; reopen it first")
		}
		if st == "dropped" && rc.completion == "assigner" && rc.by != a {
			return rejection("the assigner (" + rc.by + ") confirms this assignment; only they or an org superior of " + a + " drop it")
		}

	case TypeItemClose:
		it, r := needIt()
		if r != "" {
			return r
		}
		if it.kind == "topic" {
			return "topics have no status"
		}
		rc, r := s.needRec(it, a)
		if r != "" {
			return r
		}
		if rc.status == "done" {
			return "assignment is already done"
		}
		if rc.status == "dropped" {
			return "assignment is dropped; reopen it first"
		}
		if rc.status == "in-review" && rc.completion == "assigner" {
			return rejection("already in review by " + rc.by)
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
		if rc.by != a && !s.below(a, rc.to) {
			verb := "rejects"
			if n.typ == TypeItemConfirm {
				verb = "confirms"
			}
			return rejection("only the assigner (" + rc.by + ") or an org superior of " + rc.to + " " + verb)
		}
		if rc.status != "in-review" {
			return rejection("assignment is " + rc.status + ", not in review")
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
		if !wrapped(rc.status) {
			return rejection("assignment is " + rc.status + ", not done or dropped")
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
		if r := s.checkExtras(p, nil); r != "" {
			return r
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
		if n.typ == TypeCommentEdit {
			return s.checkExtras(p, q.attachments)
		}

	case TypeMemberProfile:
		who := a
		if !p.null("member") {
			who = p.disp("member")
		}
		if r := checkProfile(p["profile"], false); r != "" {
			return r
		}
		if r := s.needMember(who); r != "" {
			return r
		}
		if who != a && !s.admins[a] {
			return rejection("only an admin changes another member's profile (" + a + " is not an admin)")
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

	case TypeSetAttachmentLimit:
		if r := s.needAdmin(a); r != "" {
			return r
		}
		if _, ok := p.posIntOf("max_bytes"); !ok {
			return "max_bytes must be a positive integer (bytes)"
		}

	case TypeAddLabel:
		if r := s.needAdmin(a); r != "" {
			return r
		}
		if r := s.checkLabelName(p, ""); r != "" {
			return r
		}
		if !isColor(p["color"]) {
			return "color must be #rrggbb in lowercase hex"
		}

	case TypeEditLabel:
		if r := s.needAdmin(a); r != "" {
			return r
		}
		l, r := s.needLabel(p)
		if r != "" {
			return r
		}
		if !p.has("name") && !p.has("color") {
			return "nothing to edit (name, color)"
		}
		if p.has("name") {
			if r := s.checkLabelName(p, l.id); r != "" {
				return r
			}
		}
		if p.has("color") && !isColor(p["color"]) {
			return "color must be #rrggbb in lowercase hex"
		}

	case TypeRemoveLabel:
		if r := s.needAdmin(a); r != "" {
			return r
		}
		if _, r := s.needLabel(p); r != "" {
			return r
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
