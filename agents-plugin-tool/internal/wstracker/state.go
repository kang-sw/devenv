package wstracker

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
)

var (
	kinds = []string{"task", "todo", "topic"}
	// settable are the statuses an assignee writes on their own assignment
	// (done goes through item.close); superiorSettable those an org superior
	// of the assignee writes on it, the superior's wrap-up: done is final, no
	// review step.
	settable         = []string{"idea", "todo", "doing", "blocked", "in-review", "dropped"}
	superiorSettable = []string{"blocked", "done", "dropped"}
	origins          = []string{OriginHuman, OriginAgent, OriginAgentAuto}
	// itemFields are the item.edit fields, in the engine's order. stage is
	// retired: only older events carry it (see apply), so editFields, the
	// list a refusal names, leaves it out.
	itemFields = []string{"title", "body", "project", "due", "start", "priority", "labels", "stage"}
	editFields = itemFields[:7]
)

// DefaultStaleDays is the stale threshold (284): a live assignment in doing
// or blocked with no status write for this many days by the viewer's clock.
const DefaultStaleDays = 3

// defaultPriority applies when genesis has no priority (272 c, 277 c).
var defaultPriority = priority{names: []string{"Notice", "Task", "Urgent"}, def: 1}

// DefaultAttachmentMax is attachments.max_bytes when genesis sets none.
const DefaultAttachmentMax = 10 << 20

// wrapped reports a done or dropped assignment status: the assignment is
// wrapped up, so it takes no status write, handoff, or take request until it
// is reopened, and it no longer counts as work in anyone's hands.
func wrapped(st string) bool { return st == "done" || st == "dropped" }

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
	due, start           *string
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
	labels             labelSet        // label ids as written; the view drops removed ones
	open               *opening        // the latest item.open; active while its ended is nil
	cc                 []ccEntry       // the CC'd members, in the order they came to be CC'd
	ccExcluded         map[string]bool // personal uncc exclusions shared by every org registration
	mentionsAt         *node           // the body write mentions come from; nil while none
}

// ccEntry is one CC'd member of an item; since is the add whose register
// write made the member CC'd (a repeated add moves nothing).
type ccEntry struct {
	member   string
	since    *node
	personal bool
	orgs     map[string]bool
}

// ccVal is a CC register value (one register per item and member): true
// while the member is CC'd. Its own type so a marker shows it as a CC.
type ccVal bool

// ccOrgVal is the registration value written for one item and org.
type ccOrgVal bool

// ccOrgReg is one item.cc_org registration. added is the provenance set that
// remains after members leave the org; excluded is written by personal uncc
// and prevents future automatic re-adds.
type ccOrgReg struct {
	item, org       string
	active          bool
	since           *node
	added, excluded map[string]bool
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

// org is a first-class organization. Removed orgs remain as tombstones so
// registrations can keep their display identity.
type org struct {
	id, name, head string
	removed        *node
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
	id, via, item, by, root, assignRoot, org, completion string
	includeRoot                                          bool
	levels                                               float64
	excluded                                             map[string]bool
	ended                                                string
	state                                                string
	request                                              string
	ts                                                   time.Time
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

	members      map[string]*member
	morder       []string
	admins       map[string]bool
	aorder       []string
	items        map[string]*item
	iorder       []string
	place        []placement
	asg          map[string]*rec
	recs         []*rec
	byItem       map[string][]*rec
	rules        []*rule
	ruleByID     map[string]*rule
	comments     map[string]*comment
	corder       []string
	reqs         map[string]*takeReq
	rorder       []string // take request ids in fold order
	reg          map[string][]head
	bodies       map[string]*bodyFold // item id -> its body as folded (merge.go)
	labels       map[string]*label
	lorder       []string // label ids in fold order of their adds
	orgs         map[string]*org
	oorder       []string // org ids in fold order of their creation
	ccOrgs       map[string]*ccOrgReg
	ccOrgsByItem map[string][]*ccOrgReg
	ccOrder      []string // item/org keys in first-registration order
	prio         priority
	attMax       float64 // attachments.max_bytes, the engine's number (posIntOf)
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
		comments: map[string]*comment{}, reg: map[string][]head{}, bodies: map[string]*bodyFold{}, labels: map[string]*label{}, orgs: map[string]*org{}, ccOrgs: map[string]*ccOrgReg{}, ccOrgsByItem: map[string][]*ccOrgReg{}, reqs: map[string]*takeReq{},
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

func (s *state) liveOrg(id string) *org {
	o := s.orgs[id]
	if o == nil || o.removed != nil {
		return nil
	}
	return o
}

func (s *state) orgNamed(name, except string) *org {
	for _, id := range s.oorder {
		o := s.liveOrg(id)
		if o != nil && id != except && strings.EqualFold(o.name, name) {
			return o
		}
	}
	return nil
}

func (s *state) orgHeadedBy(member, except string) *org {
	if member == "" {
		return nil
	}
	for _, id := range s.oorder {
		o := s.liveOrg(id)
		if o != nil && id != except && o.head == member {
			return o
		}
	}
	return nil
}

// nearestOrg is the member's display org: the closest live org head at or
// above the member in the lead chain. Org registration scope intentionally
// uses orgMembers instead and includes nested orgs.
func (s *state) nearestOrg(member string) *org {
	for cur, n := member, 0; cur != "" && n <= len(s.members); n++ {
		if o := s.orgHeadedBy(cur, ""); o != nil {
			return o
		}
		m := s.members[cur]
		if m == nil {
			return nil
		}
		cur = m.lead()
	}
	return nil
}

// orgMembers is the whole live subtree rooted at the org's head, including
// that head and members whose nearest org is a nested child org.
func (s *state) orgMembers(o *org) []string {
	if o == nil || o.removed != nil || !s.liveMember(o.head) {
		return nil
	}
	out := []string{o.head}
	for _, r := range s.reach(o.head, wholeSubtree) {
		out = append(out, r.id)
	}
	return out
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
	if ru.org != "" {
		o := s.liveOrg(ru.org)
		if o == nil || o.head == "" {
			return nil
		}
	}
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
