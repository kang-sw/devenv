package wstracker

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
)

var (
	kinds    = []string{"task", "todo", "topic"}
	settable = []string{"idea", "todo", "doing", "blocked"}
	origins  = []string{OriginHuman, OriginAgent, OriginAgentAuto}
	// itemFields are the item.edit fields, in the engine's order. stage is
	// retired: only older events carry it (see apply), so editFields, the
	// list a refusal names, leaves it out.
	itemFields = []string{"title", "body", "project", "due", "priority", "labels", "stage"}
	editFields = itemFields[:6]
)

// DefaultStaleDays is the stale threshold (284): a live assignment in doing
// or blocked with no status write for this many days by the viewer's clock.
const DefaultStaleDays = 3

// defaultPriority applies when genesis has no priority (272 c, 277 c).
var defaultPriority = priority{names: []string{"Notice", "Task", "Urgent"}, def: 1}

// DefaultAttachmentMax is attachments.max_bytes when genesis sets none.
const DefaultAttachmentMax = 10 << 20

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
	id      string
	profile Profile
	keys    []memberKey
	revoked []revokedKey
	leads   []string
	removed *node
	joined  *node
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
	// after is the sibling anchor (528) when afterSet: "" = first. Unset
	// (every item created before the field) = last.
	after    string
	afterSet bool
	// placedBy is the event of the current place (the create, or the
	// item.move whose register write is shown); parentBy the one that set
	// the current parent.
	placedBy, parentBy *node
	ts                 time.Time
	hidden             bool
	edited             *node
	node               *node
	mentions           []string
	attachments        []attachment
	refs               []string
	labels             labelSet // label ids as written; the view drops removed ones
	open               *opening // the latest item.open; active while its ended is nil
}

// opening is an item.open: the creator (or the admin standing in for a
// removed one) opens a task nobody else holds to a scope, team (the
// opener's org subtree) or store, and one member in it claims it. It ends
// when the item is claimed or assigned, when anyone else comes to hold it,
// or when it is withdrawn (item.unopen); ended is that event.
type opening struct {
	node      *node
	by, scope string
	ended     *node
}

// takeReq is an item.request: by asks for the live assignment of (a record
// id) that holder holds. state: pending -> approved | denied | withdrawn
// (item.unrequest, or the requester removed) | lapsed (the assignment
// ended first); ended is that event.
type takeReq struct {
	node                 *node
	id, item, by, holder string
	of, note, answerNote string
	state                string
	ended                *node
}

// placement is one sibling placement (528) in fold order: a create, or an
// item.move that changed what the item's position register shows. The view
// replays them to order siblings.
type placement struct {
	id, parent, after string
	afterSet          bool
}

// placeVal is an item.move's position register value.
type placeVal struct {
	parent, after string
	afterSet      bool
}

// labelSet is an item's labels register value: label ids, in the order
// written. It is its own type so a concurrency marker shows it as labels.
type labelSet []string

// label is one entry of the store's label list; its id is its
// roles.add_label event's id. A removed label stays here (removed set) so
// the items that carry it keep its id; every reader skips it.
type label struct {
	id, name, color string
	removed         *node
}

// attachment is attachment metadata as folded; oid is the content's sha256.
type attachment struct {
	oid, name, mime string
	size            float64 // the engine's number; never converted to int (see posIntOf)
}

// rec is one assignment record: "<assign or handoff event id>><member>",
// or "<rule id>><member>" for a rule's records (rule set).
// state: pending -> live (accept) | declined | withdrawn;
// live -> unassigned | returned | handed (a handoff was accepted).
// A rule request (ruleRequest, id = the rule id) is the pending record a
// rule rooted outside its author's reach sends to the root: pending ->
// accepted (the rule goes live) | declined | withdrawn; accepted ->
// returned | unassigned. It never holds the item, so openRec skips it.
// A fallback (fallback set) is the holding the fold derives for a task
// nobody else holds (see settle): live -> displaced (someone else came to
// hold it) | unassigned (its holder was removed). openRec and openRecs skip
// it, so it never blocks an assign; liveRec finds it, so its holder sets its
// status and closes it.
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
	rule                                      string
	ruleRequest                               bool
	fallback                                  bool
	start                                     *node // a fallback's: the event that started its period
}

// rule is an assignment rule (490)-(492): it covers root (with
// includeRoot) and everyone up to levels below it, levels 0 meaning the
// whole subtree. The older "everyone within N" is a rule rooted at its
// author, its id the assign event's; a newer rule's id is
// "<event id>~<root>". state: live (the author reaches the root, or the
// root accepted request) | pending; ended is the event that ended it.
type rule struct {
	id, via, item, by, root, completion string
	includeRoot                         bool
	levels                              float64
	excluded                            map[string]bool
	ended                               string
	state                               string
	request                             string
	ts                                  time.Time
}

// wholeSubtree is the reach depth of a rule with no levels.
const wholeSubtree = 99

func (ru *rule) depth() float64 {
	if ru.levels == 0 {
		return wholeSubtree
	}
	return ru.levels
}

type comment struct {
	id, alias, item, author, body, replyTo string
	ts                                     time.Time
	hidden                                 bool
	hiddenBy, edited                       *node
	mentions                               []string
	attachments                            []attachment
	refs                                   []string
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
	place    []placement
	asg      map[string]*rec
	recs     []*rec
	byItem   map[string][]*rec
	rules    []*rule
	ruleByID map[string]*rule
	comments map[string]*comment
	corder   []string
	reqs     map[string]*takeReq
	rorder   []string // take request ids in fold order
	reg      map[string][]head
	labels   map[string]*label
	lorder   []string // label ids in fold order of their adds
	prio     priority
	attMax   float64 // attachments.max_bytes, the engine's number (posIntOf)
}

// genesis is the parsed roles.json the fold starts from.
type genesis struct {
	raw     obj
	members []*member
	admins  []string
	prio    priority
	attMax  float64
}

func parseGenesis(raw json.RawMessage) *genesis {
	g := &genesis{raw: parsePayload(raw), prio: normPriority(nil), attMax: DefaultAttachmentMax}
	if p, ok := g.raw["priority"].(map[string]any); ok {
		g.prio = normPriority(obj(p))
	}
	if a, ok := g.raw["attachments"].(map[string]any); ok {
		if v, ok := obj(a).posIntOf("max_bytes"); ok {
			g.attMax = v
		}
	}
	seen, used := map[string]bool{}, map[string]bool{}
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
		mem := &member{id: id, profile: profileOf(m, id)}
		for _, k := range m.strList("keys") {
			// As in the protocol, an SSH key an earlier member holds is not held again.
			if norm, fp, err := wsledger.KeyFingerprint(k); err == nil {
				if !used[fp] {
					used[fp] = true
					mem.keys = append(mem.keys, memberKey{norm, fp})
				}
				continue
			}
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
		if pr := m["profile"]; truthy(pr) {
			po, _ := pr.(map[string]any)
			if pstr(po["given"]) == "" {
				out = append(out, id+": profile.given required")
			}
		}
	}
	if p, ok := r["priority"]; ok && p != nil {
		po, _ := p.(map[string]any)
		if names, ok := obj(po).list("names"); !ok || len(names) == 0 {
			out = append(out, "priority.names must be a non-empty list")
		}
	}
	if a := r["attachments"]; truthy(a) {
		ao, _ := a.(map[string]any)
		if _, ok := obj(ao).posIntOf("max_bytes"); !ok {
			out = append(out, "attachments.max_bytes must be a positive integer")
		}
	}
	return out
}

func newState(g *genesis, hb func(x, y *node) bool, marks bool) *state {
	s := &state{
		hb: hb, marks: marks,
		members: map[string]*member{}, admins: map[string]bool{},
		items: map[string]*item{}, asg: map[string]*rec{}, byItem: map[string][]*rec{}, ruleByID: map[string]*rule{},
		comments: map[string]*comment{}, reg: map[string][]head{}, labels: map[string]*label{}, reqs: map[string]*takeReq{},
		prio:   priority{names: append([]string(nil), g.prio.names...), def: g.prio.def},
		attMax: g.attMax,
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

// openRec is who's live or pending holding on item; a rule request holds
// nothing.
func (s *state) openRec(item, who string) *rec {
	for _, r := range s.byItem[item] {
		if isOpen(r) && r.to == who && !r.ruleRequest && !r.fallback {
			return r
		}
	}
	return nil
}

// acceptedReq is an accepted rule request to who whose rule is still live:
// what item.return ends.
func (s *state) acceptedReq(item, who string) *rec {
	for _, r := range s.byItem[item] {
		if ru := s.ruleByID[r.rule]; r.ruleRequest && r.to == who && r.state == "accepted" && ru != nil && ru.ended == "" {
			return r
		}
	}
	return nil
}

// openRecs are item's live or pending records, the fallback left out.
func (s *state) openRecs(item string) []*rec {
	var out []*rec
	for _, r := range s.byItem[item] {
		if isOpen(r) && !r.fallback {
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

// openRules are item's rules not ended, live or pending.
func (s *state) openRules(item string) []*rule {
	var out []*rule
	for _, r := range s.rules {
		if r.item == item && r.ended == "" {
			out = append(out, r)
		}
	}
	return out
}

func (s *state) liveRules(item string) []*rule {
	var out []*rule
	for _, r := range s.openRules(item) {
		if r.state == "live" {
			out = append(out, r)
		}
	}
	return out
}

func (s *state) hasLiveRule() bool {
	for _, r := range s.rules {
		if r.ended == "" && r.state == "live" {
			return true
		}
	}
	return false
}

// coveredBy lists who ru covers now: the root (with includeRoot, while a
// member), then its reach.
func (s *state) coveredBy(ru *rule) []string {
	var out []string
	if ru.includeRoot && s.liveMember(ru.root) {
		out = append(out, ru.root)
	}
	for _, t := range s.reach(ru.root, ru.depth()) {
		out = append(out, t.id)
	}
	return out
}

// ruleCovers reports that rule y covers rule x by shape: everything x
// reaches, y reaches.
func (s *state) ruleCovers(y, x *rule) bool {
	d, ok := 0, y.root == x.root
	if !ok {
		d, ok = reachMap(s.reach(y.root, wholeSubtree))[x.root]
	}
	if !ok {
		return false
	}
	if x.includeRoot {
		if d == 0 && !y.includeRoot || d > 0 && y.levels != 0 && float64(d) > y.levels {
			return false
		}
	}
	if y.levels == 0 {
		return true
	}
	return x.levels != 0 && float64(d)+x.levels <= y.levels
}

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

// endRule ends ru with the live records it made; a handoff its holder
// accepted stays.
func (s *state) endRule(ru *rule, n *node) {
	if ru == nil {
		return
	}
	if ru.ended == "" {
		ru.ended = n.id
	}
	for _, x := range append([]*rec(nil), s.byItem[ru.item]...) {
		if x.rule == ru.id && !x.ruleRequest && x.state == "live" {
			s.endRec(x, n, "unassigned")
		}
	}
}

// unassignRule ends ru for item.unassign {root} or {rule}: a pending request
// is withdrawn, an accepted one unassigned, then the rule ends with its
// records.
func (s *state) unassignRule(ru *rule, n *node) {
	if q := s.asg[ru.request]; ru.request != "" && q != nil {
		switch q.state {
		case "pending":
			s.endRec(q, n, "withdrawn")
		case "accepted":
			q.state, q.ended = "unassigned", n
		}
	}
	s.endRule(ru, n)
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
	ru := &rule{id: id, via: n.id, item: item, by: a, root: root, includeRoot: x["include_root"] == true,
		completion: completion, excluded: map[string]bool{}, state: "live", ts: n.ts}
	ru.levels, _ = x["levels"].(float64)
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
	case labelSet:
		if len(x) == 0 {
			return "(none)"
		}
		out := make([]string, len(x))
		for i, id := range x {
			out[i] = alias(id)
		}
		return strings.Join(out, ", ")
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
	case placeVal:
		at := "top level"
		if x.parent != "" {
			at = "under " + alias(x.parent)
		}
		switch {
		case !x.afterSet:
			return at + ", last"
		case x.after == "":
			return at + ", first"
		}
		return at + ", after " + alias(x.after)
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

// peek is the value write(key, _, n, v) would show, without writing.
func (s *state) peek(key string, n *node, v any) any {
	heads := []head{{n, v}}
	for _, h := range s.reg[key] {
		if !s.hb(h.n, n) {
			heads = append(heads, h)
		}
	}
	return winner(heads).v
}

// regWinner is the shown write of a register: mentions, attachments, and
// refs follow the body write that won.
func (s *state) regWinner(key string) *node { return winner(s.reg[key]).n }

func mentionsOf(p obj) []string {
	l, _ := p.list("mentions")
	out := make([]string, 0, len(l))
	for _, x := range l {
		out = append(out, jsString(x))
	}
	return out
}

// labelsOf is a payload's labels: absent or null is none.
func labelsOf(p obj) labelSet { return labelSet(p.strList("labels")) }

// liveLabel is the label id names when the list holds it and it is not
// removed, else nil.
func (s *state) liveLabel(id string) *label {
	if l := s.labels[id]; l != nil && l.removed == nil {
		return l
	}
	return nil
}

// liveLabels is ids without the labels the list does not hold live.
func (s *state) liveLabels(ids labelSet) []string {
	out := []string{}
	for _, id := range ids {
		if s.liveLabel(id) != nil {
			out = append(out, id)
		}
	}
	return out
}

// labelNamed is the live label other than except whose name equals name,
// ignoring case (label:<name> is matched lower-cased), else nil.
func (s *state) labelNamed(name, except string) *label {
	k := strings.ToLower(name)
	for _, id := range s.lorder {
		if l := s.liveLabel(id); l != nil && id != except && strings.ToLower(l.name) == k {
			return l
		}
	}
	return nil
}

func refsOf(p obj) []string {
	l, _ := p.list("refs")
	out := make([]string, 0, len(l))
	for _, x := range l {
		out = append(out, jsString(x))
	}
	return out
}

func attachmentsOf(p obj) []attachment {
	l, _ := p.list("attachments")
	out := make([]attachment, 0, len(l))
	for _, x := range l {
		a := obj(nil)
		if m, ok := x.(map[string]any); ok {
			a = obj(m)
		}
		size, _ := a["size"].(float64)
		out = append(out, attachment{oid: a.str("oid"), name: a.str("name"), mime: a.str("mime"), size: size})
	}
	return out
}

// ---------------------------------------------------------------- endings

// endRec ends a record. A pending handoff of it ends with it, as withdrawn
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
		it.mentions, it.attachments, it.refs = mentionsOf(p), attachmentsOf(p), refsOf(p)
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
		if p.has("body") {
			w := s.regWinner("it:" + it.id + ":body")
			it.mentions, it.attachments, it.refs = mentionsOf(w.p), attachmentsOf(w.p), refsOf(w.p)
		}
		it.edited = n
	case TypeItemHide:
		if it := s.items[p.str("item")]; it != nil {
			it.hidden = s.write("it:"+it.id+":hidden", it.alias+" hidden", n, true).(bool)
		}
	case TypeItemMove:
		s.applyMove(n)
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
			for _, ru := range s.openRules(item) {
				if ru.by == a && ru.root == p.str("root") {
					s.unassignRule(ru, n)
				}
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
			for _, ru := range s.openRules(item) {
				if ru.by != a {
					continue
				}
				ru.ended = n.id
				if q := s.asg[ru.request]; ru.request != "" && q != nil && q.state == "accepted" {
					q.state, q.ended = "unassigned", n
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
				v = "awaiting"
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
		ru := &rule{id: n.id, via: n.id, item: item, by: a, root: a, levels: levels, completion: completion,
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
// that an earlier event in fold order answered, withdrew, or let lapse is
// flagged and takes no effect.
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
