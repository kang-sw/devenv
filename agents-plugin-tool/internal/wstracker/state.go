package wstracker

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
)

var (
	kinds    = []string{"task", "todo", "topic"}
	settable = []string{"todo", "doing", "blocked"}
	origins  = []string{OriginHuman, OriginAgent, OriginAgentAuto}
	// itemFields are the item.edit fields, in the engine's order.
	itemFields = []string{"title", "body", "project", "due", "priority"}
)

// DefaultStaleDays is the stale threshold (284): a live assignment in doing
// or blocked with no status write for this many days by the viewer's clock.
const DefaultStaleDays = 3

// defaultPriority applies when genesis has no priority (272 c, 277 c).
var defaultPriority = priority{names: []string{"알림", "업무", "긴급"}, def: 1}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

type priority struct {
	names []string
	def   int
}

// level is the display level of a stored priority: a value outside the
// current range displays as the top level.
func (p priority) level(v int) int {
	top := len(p.names) - 1
	if v >= 0 && v <= top {
		return v
	}
	return top
}

type member struct {
	id, name string
	keys     []memberKey
	revoked  []revokedKey
	leads    []string
	removed  *node
	joined   *node
}

// memberKey is a key in display form plus the form keys compare by: the
// fingerprint when it parses as an SSH key, else the raw text.
type memberKey struct{ text, cmp string }

type revokedKey struct {
	key memberKey
	by  *node
}

func keyOf(s string) memberKey {
	if norm, fp, err := wsledger.KeyFingerprint(s); err == nil {
		return memberKey{norm, fp}
	}
	return memberKey{s, s}
}

func (m *member) lead() string {
	if len(m.leads) == 0 {
		return ""
	}
	return m.leads[0]
}

func (m *member) holds(k memberKey) bool {
	for _, x := range m.keys {
		if x.cmp == k.cmp {
			return true
		}
	}
	return false
}

func (m *member) revokedBy(k memberKey) *node {
	for _, r := range m.revoked {
		if r.key.cmp == k.cmp {
			return r.by
		}
	}
	return nil
}

type item struct {
	id, alias, kind      string
	title, body, project string
	due                  *string
	priority             int
	parent, author       string
	ts                   time.Time
	hidden               bool
	edited               *node
	node                 *node
}

// rec is one assignment record: "<assign or handoff event id>><member>".
// state: pending -> live (accept) | declined | withdrawn;
// live -> unassigned | returned | handed (a handoff was accepted).
type rec struct {
	id, item, to, by, sender, completion, via string
	ts                                        time.Time
	state                                     string
	forced                                    bool
	accepted, ended, restored, last           *node
	leadAtAccept                              string
	handoffOf, restores                       string
	answerNote, handoffNote                   string
	status, note, noteBy                      string
	writes                                    []*node
	inherited                                 string
}

type rule struct {
	id, item, by, completion string
	levels                   float64
	excluded                 map[string]bool
	ended                    string
}

type comment struct {
	id, alias, item, author, body, replyTo string
	ts                                     time.Time
	hidden                                 bool
	hiddenBy, edited                       *node
}

// head is one value of a multi-value register.
type head struct {
	n *node
	v any
}

type statusVal struct {
	status, note, by string
	n                *node
}

// state is the folded store. Maps hold the records; the order slices keep
// insertion order, which the derived lists follow.
type state struct {
	hb    func(x, y *node) bool // x is in y's causal past (x earlier in fold order)
	marks bool                  // final fold: record concurrent-change markers

	members  map[string]*member
	morder   []string
	admins   map[string]bool
	aorder   []string
	items    map[string]*item
	iorder   []string
	asg      map[string]*rec
	recs     []*rec
	byItem   map[string][]*rec
	rules    []*rule
	comments map[string]*comment
	corder   []string
	reg      map[string][]head
	prio     priority
}

// genesis is the parsed roles.json the fold starts from.
type genesis struct {
	raw     obj
	members []*member
	admins  []string
	prio    priority
}

func parseGenesis(raw json.RawMessage) *genesis {
	g := &genesis{raw: parsePayload(raw), prio: normPriority(nil)}
	if p, ok := g.raw["priority"].(map[string]any); ok {
		g.prio = normPriority(obj(p))
	}
	seen := map[string]bool{}
	ml, _ := g.raw.list("members")
	for _, x := range ml {
		mo, ok := x.(map[string]any)
		if !ok {
			continue
		}
		m := obj(mo)
		id := m.str("id")
		// Membership follows the protocol: an invalid or repeated id is not a member.
		if !wsledger.ValidID(id) || seen[id] {
			continue
		}
		seen[id] = true
		mem := &member{id: id, name: m.str("name")}
		if mem.name == "" {
			mem.name = id
		}
		for _, k := range m.strList("keys") {
			mem.keys = append(mem.keys, keyOf(k))
		}
		mem.leads = m.strList("leads")
		g.members = append(g.members, mem)
	}
	for _, a := range g.raw.strList("admins") {
		if seen[a] && !contains(g.admins, a) {
			g.admins = append(g.admins, a)
		}
	}
	return g
}

func normPriority(p obj) priority {
	names, ok := p.list("names")
	if p == nil || !ok || len(names) == 0 {
		return priority{names: append([]string(nil), defaultPriority.names...), def: defaultPriority.def}
	}
	out := priority{names: p.strList("names")}
	if d, ok := p.intOf("default"); ok && d >= 0 && d < len(names) {
		out.def = d
	} else {
		out.def = min(1, len(names)-1)
	}
	return out
}

// rolesProblems lists problems in the roles file itself.
func rolesProblems(r obj) []string {
	out := []string{}
	ids := map[string]bool{}
	ml, _ := r.list("members")
	var members []obj
	for _, x := range ml {
		if mo, ok := x.(map[string]any); ok {
			members = append(members, obj(mo))
			ids[obj(mo).disp("id")] = true
		}
	}
	admins := r.strList("admins")
	for _, a := range admins {
		if !ids[a] {
			out = append(out, `admin "`+a+`" is not a member`)
		}
	}
	if len(admins) == 0 {
		out = append(out, "no admins")
	}
	for _, m := range members {
		id := m.disp("id")
		leads := m.strList("leads")
		if strings.Contains(id, ".") {
			out = append(out, id+`: member ids cannot contain "."`)
		}
		if len(leads) > 1 {
			out = append(out, id+": leads must have length at most 1 (got "+itoa(len(leads))+")")
		}
		for _, l := range leads {
			if !ids[l] {
				out = append(out, id+`: lead "`+l+`" is not a member`)
			}
		}
		if keys, _ := m.list("keys"); len(keys) == 0 {
			out = append(out, id+": no keys")
		}
	}
	if p, ok := r["priority"]; ok && p != nil {
		po, _ := p.(map[string]any)
		if names, ok := obj(po).list("names"); !ok || len(names) == 0 {
			out = append(out, "priority.names must be a non-empty list")
		}
	}
	return out
}

func newState(g *genesis, hb func(x, y *node) bool, marks bool) *state {
	s := &state{
		hb: hb, marks: marks,
		members: map[string]*member{}, admins: map[string]bool{},
		items: map[string]*item{}, asg: map[string]*rec{}, byItem: map[string][]*rec{},
		comments: map[string]*comment{}, reg: map[string][]head{},
		prio: priority{names: append([]string(nil), g.prio.names...), def: g.prio.def},
	}
	for _, m := range g.members {
		cp := *m
		cp.keys = append([]memberKey(nil), m.keys...)
		cp.leads = append([]string(nil), m.leads...)
		s.members[m.id] = &cp
		s.morder = append(s.morder, m.id)
	}
	for _, a := range g.admins {
		s.addAdmin(a)
	}
	return s
}

func (s *state) addAdmin(a string) {
	if !s.admins[a] {
		s.admins[a] = true
		s.aorder = append(s.aorder, a)
	}
}

func (s *state) removeAdmin(a string) {
	if !s.admins[a] {
		return
	}
	delete(s.admins, a)
	for i, x := range s.aorder {
		if x == a {
			s.aorder = append(s.aorder[:i:i], s.aorder[i+1:]...)
			break
		}
	}
}

func (s *state) liveMember(id string) bool {
	m := s.members[id]
	return m != nil && m.removed == nil
}

// ---------------------------------------------------------------- org tree

// children maps each lead to their followers, in member order, removed
// members excluded.
func (s *state) children() map[string][]string {
	c := map[string][]string{}
	for _, id := range s.morder {
		m := s.members[id]
		if m.removed != nil {
			continue
		}
		for _, l := range m.leads {
			c[l] = append(c[l], id)
		}
	}
	return c
}

type reached struct {
	id    string
	depth int
}

// reach lists everyone within levels below from (depth 1 = direct
// follower), breadth first in member order.
func (s *state) reach(from string, levels float64) []reached {
	c := s.children()
	var out []reached
	in := map[string]bool{}
	frontier := []string{from}
	for d := 1; float64(d) <= levels; d++ {
		var next []string
		for _, f := range frontier {
			for _, k := range c[f] {
				if !in[k] && k != from {
					in[k] = true
					out = append(out, reached{k, d})
					next = append(next, k)
				}
			}
		}
		frontier = next
	}
	return out
}

func reachMap(r []reached) map[string]int {
	m := make(map[string]int, len(r))
	for _, x := range r {
		m[x.id] = x.depth
	}
	return m
}

// below is the forced-assignment rule: by reaches to when by is in to's
// lead chain.
func (s *state) below(by, to string) bool {
	_, ok := reachMap(s.reach(by, 99))[to]
	return ok
}

func (s *state) wouldCycle(id, lead string) bool {
	cur, n := lead, 0
	for cur != "" && n <= len(s.members)+1 {
		n++
		if cur == id {
			return true
		}
		m := s.members[cur]
		if m == nil {
			return false
		}
		cur = m.lead()
	}
	return false
}

// ---------------------------------------------------------------- records

func (s *state) recsOn(item string) []*rec { return s.byItem[item] }

func (s *state) liveRec(item, who string) *rec {
	for _, r := range s.byItem[item] {
		if r.state == "live" && r.to == who {
			return r
		}
	}
	return nil
}

func (s *state) pendingRec(item, who string) *rec {
	for _, r := range s.byItem[item] {
		if r.state == "pending" && r.to == who {
			return r
		}
	}
	return nil
}

func isOpen(r *rec) bool { return r.state == "live" || r.state == "pending" }

func (s *state) openRec(item, who string) *rec {
	for _, r := range s.byItem[item] {
		if isOpen(r) && r.to == who {
			return r
		}
	}
	return nil
}

func (s *state) openRecs(item string) []*rec {
	var out []*rec
	for _, r := range s.byItem[item] {
		if isOpen(r) {
			out = append(out, r)
		}
	}
	return out
}

func (s *state) lastEndedRec(item, who string) *rec {
	var last *rec
	for _, r := range s.byItem[item] {
		if r.to == who && r.ended != nil {
			last = r
		}
	}
	return last
}

func (s *state) liveRules(item string) []*rule {
	var out []*rule
	for _, r := range s.rules {
		if r.item == item && r.ended == "" {
			out = append(out, r)
		}
	}
	return out
}

func (s *state) hasLiveRule() bool {
	for _, r := range s.rules {
		if r.ended == "" {
			return true
		}
	}
	return false
}

// addRec adds a record unless one with the same id exists. It starts live
// and forced; set adjusts it before it is stored.
func (s *state) addRec(viaID string, viaTS time.Time, last *node, item, to, by, completion string, set func(*rec)) *rec {
	id := viaID + ">" + to
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

// refreshLive re-applies the live "everyone within N" rules: whoever entered
// the reach since inherits the assignment.
func (s *state) refreshLive(n *node) {
	for _, ru := range s.rules {
		if ru.ended != "" {
			continue
		}
		for _, t := range s.reach(ru.by, ru.levels) {
			if ru.excluded[t.id] || s.openRec(ru.item, t.id) != nil || s.asg[ru.id+">"+t.id] != nil {
				continue
			}
			s.addRec(ru.id, n.ts, n, ru.item, t.id, ru.by, ru.completion, func(r *rec) { r.inherited = n.id })
		}
	}
}

// ---------------------------------------------------------------- markers

type marker struct {
	kind, field, other, winner string
	mine, theirs               string
}

type supersession struct {
	field, by, why string
}

func mark(n, other *node, kind, field, winner, mine, theirs string) {
	for _, x := range n.markers {
		if x.other == other.id && x.field == field {
			return
		}
	}
	n.markers = append(n.markers, marker{kind: kind, field: field, other: other.id, winner: winner, mine: mine, theirs: theirs})
}

func supersede(n *node, field string, by *node, why string) {
	kept := n.superseded[:0:0]
	for _, x := range n.superseded {
		if x.field != field {
			kept = append(kept, x)
		}
	}
	if by != nil {
		kept = append(kept, supersession{field: field, by: by.id, why: why})
	}
	n.superseded = kept
}

// show renders a register value for a marker.
func show(v any) string {
	switch x := v.(type) {
	case nil:
		return "(none)"
	case priority:
		return strings.Join(x.names, " / ") + " (default " + x.names[x.def] + ")"
	case statusVal:
		if x.note != "" {
			return x.status + ` ("` + x.note + `")`
		}
		return x.status
	case []string:
		if len(x) == 0 {
			return "nobody"
		}
		return strings.Join(x, ", ")
	case bool:
		if x {
			return "hidden"
		}
		return "false"
	case int:
		return itoa(x)
	case string:
		return truncate60(x)
	}
	return ""
}

// winner picks the display value among pairwise concurrent heads: an agent
// write with a same-member human write beside it drops out (288), then the
// later ts, member, clone, seq wins (203, 271).
func winner(heads []head) head {
	var w *head
	for i := range heads {
		h := &heads[i]
		if h.n.origin != OriginHuman {
			dropped := false
			for _, o := range heads {
				if o.n.member == h.n.member && o.n.origin == OriginHuman {
					dropped = true
					break
				}
			}
			if dropped {
				continue
			}
		}
		if w == nil || later(h.n, w.n) {
			w = h
		}
	}
	return *w
}

// write folds one write into a multi-value register and returns the shown
// value. Writes by two members get a "concurrent change" marker; one
// member's losing write from another clone gets a quiet "superseded" note.
func (s *state) write(key, label string, n *node, v any) any {
	var heads []head
	for _, h := range s.reg[key] {
		if !s.hb(h.n, n) {
			heads = append(heads, h)
		}
	}
	conc := heads
	heads = append(append([]head(nil), heads...), head{n, v})
	s.reg[key] = heads
	w := winner(heads)
	if s.marks {
		for _, h := range conc {
			if h.n.member == n.member {
				continue
			}
			mark(h.n, n, "field", label, w.n.id, show(h.v), show(v))
			mark(n, h.n, "field", label, w.n.id, show(v), show(h.v))
		}
		for _, h := range heads {
			if h.n == w.n || h.n.member != w.n.member {
				supersede(h.n, label, nil, "")
			} else if later(h.n, w.n) {
				supersede(h.n, label, w.n, "human")
			} else {
				supersede(h.n, label, w.n, "later")
			}
		}
	}
	return w.v
}

// ---------------------------------------------------------------- endings

// endRec ends a record. A pending handoff of it ends with it, as withdrawn
// (295). Unassign, return, withdraw, and an accepted handoff beat any
// concurrent status write on it: the writes are marked, valid but not in
// effect (213).
func (s *state) endRec(r *rec, n *node, st string) {
	r.state, r.ended = st, n
	for _, x := range s.byItem[r.item] {
		if x.handoffOf == r.id && x.state == "pending" {
			x.state, x.ended = "withdrawn", n
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
		if it.parent != r.item || it.author != r.to || parent == nil || parent.author == r.to || s.hb(it.node, n) {
			continue
		}
		f := "subtask under " + alias(r.item)
		mark(it.node, n, "subtask", f, n.id, "", "")
		mark(n, it.node, "subtask", f, n.id, "", "")
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

// ---------------------------------------------------------------- apply

// apply folds one valid event leniently: it is applied even when a
// concurrent event has since changed the state under it.
func (s *state) apply(n *node) {
	p, a := n.p, n.member
	switch n.typ {
	case TypeItemCreate:
		it := &item{id: n.id, alias: n.alias, kind: p.str("kind"), title: jsString(p["title"]),
			parent: p.str("parent"), author: a, ts: n.ts, node: n, priority: s.prio.def}
		if p.truthy("body") {
			it.body = jsString(p["body"])
		}
		it.project = p.str("project")
		if d, ok := p["due"].(string); ok && d != "" {
			it.due = &d
		}
		if v, ok := p.intOf("priority"); ok {
			it.priority = v
		}
		if s.items[n.id] == nil {
			s.iorder = append(s.iorder, n.id)
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
			case "body", "project":
				v := ""
				if !p.null(f) {
					v = jsString(p[f])
				}
				if f == "body" {
					it.body = s.write(key, label, n, v).(string)
				} else {
					it.project = s.write(key, label, n, v).(string)
				}
			case "due":
				var v any
				if !p.null(f) {
					v = jsString(p[f])
				}
				if w, ok := s.write(key, label, n, v).(string); ok {
					it.due = &w
				} else {
					it.due = nil
				}
			case "priority":
				v, _ := p.intOf(f)
				it.priority = s.write(key, label, n, v).(int)
			}
		}
		it.edited = n
	case TypeItemHide:
		if it := s.items[p.str("item")]; it != nil {
			it.hidden = s.write("it:"+it.id+":hidden", it.alias+" hidden", n, true).(bool)
		}
	case TypeItemAssign:
		s.applyAssign(n)
	case TypeItemHandoff:
		from := p.str("assignee")
		if from == "" {
			from = a
		}
		r := s.liveRec(p.str("item"), from)
		to := p.str("to")
		if r == nil || s.members[to] == nil || s.openRec(p.str("item"), to) != nil {
			break
		}
		s.addRec(n.id, n.ts, n, r.item, to, r.by, r.completion, func(x *rec) {
			x.forced, x.state, x.sender, x.handoffOf, x.handoffNote = false, "pending", a, r.id, p.str("note")
		})
	case TypeItemAccept:
		r := s.pendingRec(p.str("item"), a)
		if r == nil {
			s.onEnded(p.str("item"), a, n)
			break
		}
		r.state, r.accepted, r.answerNote, r.last = "live", n, p.str("note"), n
		if m := s.members[a]; m != nil {
			r.leadAtAccept = m.lead()
		}
		if q := s.asg[r.handoffOf]; r.handoffOf != "" && q != nil && q.state == "live" {
			// (290) the new assignment starts with the original's status and note
			s.endRec(q, n, "handed")
			r.status, r.note, r.noteBy = q.status, q.note, q.noteBy
		}
	case TypeItemDecline:
		if r := s.pendingRec(p.str("item"), a); r != nil {
			r.state, r.ended, r.answerNote = "declined", n, p.str("note")
		}
	case TypeItemReturn:
		item, who := p.str("item"), p.str("assignee")
		r := s.liveRec(item, who)
		if r == nil {
			if q := s.handedRec(item, who, n); q != nil {
				r = s.reverseHanded(q, n)
			}
		}
		if r == nil {
			break
		}
		s.endRec(r, n, "returned")
		r.answerNote = p.str("note")
		// (291) returning an accepted handoff restores the original assignment
		// with its prior status, unless its holder was removed or holds another
		// one now.
		if q := s.asg[r.handoffOf]; r.handoffOf != "" && q != nil && q.state == "handed" && q.ended == r.accepted &&
			s.liveMember(q.to) && s.openRec(q.item, q.to) == nil {
			q.state, q.ended, q.restored, r.restores = "live", nil, n, q.id
		}
	case TypeItemUnassign:
		item := p.str("item")
		if p.truthy("assignee") {
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
			for _, ru := range s.liveRules(item) {
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
			for _, ru := range s.liveRules(item) {
				if ru.by == a {
					ru.ended = n.id
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
		}
	case TypeItemStatus, TypeItemClose, TypeItemConfirm, TypeItemReject, TypeItemReopen:
		who := a
		if n.typ != TypeItemStatus && n.typ != TypeItemClose && p.truthy("assignee") {
			who = p.str("assignee")
		}
		item := p.str("item")
		r := s.liveRec(item, who)
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
				v = "awaiting"
			}
		case TypeItemConfirm:
			v = "done"
		default:
			v = "doing"
		}
		w := s.write("st:"+r.id, "status of "+who+" on "+alias(item), n, statusVal{v, p.str("note"), a, n}).(statusVal)
		r.status, r.note, r.noteBy, r.last = w.status, w.note, w.by, w.n
	case TypeCommentAdd:
		c := &comment{id: n.id, alias: n.alias, item: p.str("item"), author: a, body: jsString(p["body"]),
			replyTo: p.str("reply_to"), ts: n.ts}
		if s.comments[n.id] == nil {
			s.corder = append(s.corder, n.id)
		}
		s.comments[n.id] = c
	case TypeCommentEdit:
		if c := s.comments[p.str("comment")]; c != nil {
			c.body = s.write("cm:"+c.id, "text of comment "+c.alias, n, jsString(p["body"])).(string)
			c.edited = n
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
			s.members[id] = &member{id: id, name: p.str("name"), keys: []memberKey{keyOf(p.str("key"))}, joined: n}
			if s.members[id].name == "" {
				s.members[id].name = id
			}
			s.morder = append(s.morder, id)
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
	case TypeSetPriorityLevels:
		d, _ := n.p.intOf("default")
		s.prio = s.write("prio", "priority levels", n, priority{names: p.strList("names"), def: d}).(priority)
	}
	if strings.HasPrefix(n.typ, "roles.") || n.typ == TypeItemAssign || n.typ == wsledger.TypeMemberJoin {
		s.refreshLive(n)
	}
}

func (s *state) applyAssign(n *node) {
	p, a := n.p, n.member
	item := p.str("item")
	if s.items[item] == nil {
		return
	}
	completion := p.str("completion")
	if !p.truthy("completion") {
		completion = "assignee"
	}
	if p.str("mode") == "all" {
		levels, _ := p["levels"].(float64)
		for _, t := range s.reach(a, levels) {
			if s.openRec(item, t.id) == nil {
				s.addRec(n.id, n.ts, n, item, t.id, a, completion, nil)
			}
		}
		s.rules = append(s.rules, &rule{id: n.id, item: item, by: a, levels: levels, completion: completion, excluded: map[string]bool{}})
		return
	}
	for _, t := range p.strList("people") {
		if s.members[t] == nil || s.openRec(item, t) != nil {
			continue
		}
		forced := t == a || s.below(a, t)
		s.addRec(n.id, n.ts, n, item, t, a, completion, func(r *rec) {
			r.forced = forced
			if !forced {
				r.state = "pending"
			}
		})
	}
}
