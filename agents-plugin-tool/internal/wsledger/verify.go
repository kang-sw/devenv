package wsledger

import (
	"container/heap"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"
)

// This file is the pure verification core: parsed commits in, verdicts out.
// It never touches git, so every rule is testable without a repository.
//
// Pipeline: (1) envelope: per member ref, each commit and line is either
// inside causality (right ref, good signature, continuous seq) or broken,
// with a synthetic id and no seq; (2) structure: each causal event's causal
// past is its stream's earlier events plus what its commit's seen names,
// closed transitively, linearized with ties by ts, member, clone, seq, with
// waiting and cyclic events left out; (3) verdicts: each event is judged
// against the identity state folded from its own causal past, then the
// revocation cutoff may ignore it.

type sigState int

const (
	sigUnsigned sigState = iota
	sigGood
	sigBad
)

// commitRec is one parsed commit of a member ref, or the virtual commit of
// this clone's open batch (Pending).
type commitRec struct {
	OID    string
	Stream string // stream the header claims
	Seen   map[string]int
	TS     time.Time
	Sig    sigState
	FP     string // signer fingerprint when Sig is sigGood
	// Bad is a commit-level malformation; "" when the commit is well formed.
	Bad     string
	Root    bool // the commit has no parent
	Lines   []lineRec
	Pending bool
}

// lineRec is one event line of a commit.
type lineRec struct {
	Seq     int
	TS      time.Time
	Type    string
	Origin  string
	Payload json.RawMessage
	Bad     string // malformed line
}

// refCommits is one member ref's commits, oldest first.
type refCommits struct {
	Owner   string // the stream the ref name gives
	Commits []*commitRec
}

type verifyInput struct {
	Roles      Roles
	GenesisRaw json.RawMessage
	Refs       []refCommits // sorted by Owner
	// Claims maps a member id to its claim ref's commit.
	Claims map[string]*commitRec
	Veto   VetoFunc
}

type structState int

const (
	stBroken structState = iota // outside causality
	stResolved
	stWaiting
	stCycle
)

type node struct {
	id     EventID
	stream string
	commit *commitRec
	line   *lineRec
	state  structState
	first  bool  // first causal node of its commit
	prev   *node // previous causal node of the stream
	preds  []*node
	succs  []*node
	indeg  int
	// past maps streams to the highest seq in the causal past. It is shared
	// by every node of one commit; for a node's own stream use seq order.
	past   map[string]int
	pos    int // index in view.order; -1 unless resolved
	waitOn string

	idp   *idPayload
	idErr error

	verdict Verdict
	reason  string
}

func (n *node) typ() string { return n.line.Type }

func (n *node) isRevoke() bool { return n.state != stBroken && n.line.Type == TypeRevokeKey }

// hb reports whether x is in y's causal past.
func hb(x, y *node) bool {
	if x == y || x.state != stResolved || y.state != stResolved {
		return false
	}
	if x.stream == y.stream {
		return x.id.Seq < y.id.Seq
	}
	return y.past[x.stream] >= x.id.Seq
}

// pastSeq is the highest seq of stream s in n's causal past.
func pastSeq(n *node, s string) int {
	if n.stream == s {
		return n.id.Seq - 1
	}
	return n.past[s]
}

type result struct {
	verdict Verdict
	reason  string
}

var valid = result{verdict: Valid}

// view is one verification of everything received.
type view struct {
	nodes   []*node // display order
	order   []*node // resolved nodes, causal order
	byID    map[EventID]*node
	streams map[string][]*node // causal nodes by stream, index seq-1
	heads   map[string]int     // stream -> highest resolved seq
	ident   *idState
	// commitRes is each resolved commit's signer verdict in the final pass.
	commitRes map[*commitRec]result
	revs      []revCut
}

func brokenTag(c *commitRec, line int) string {
	oid := c.OID
	if c.Pending {
		oid = "pending"
	} else if len(oid) > 12 {
		oid = oid[:12]
	}
	if line < 0 {
		return oid + ":-"
	}
	return oid + ":" + strconv.Itoa(line)
}

func buildView(in *verifyInput) *view {
	v := &view{
		byID:      map[EventID]*node{},
		streams:   map[string][]*node{},
		heads:     map[string]int{},
		commitRes: map[*commitRec]result{},
	}
	var all []*node
	for _, r := range in.Refs {
		member, clone, _ := splitStream(r.Owner)
		expected := 1
		for _, c := range r.Commits {
			bad := c.Bad
			switch {
			case bad != "":
			case c.Stream != r.Owner:
				bad = fmt.Sprintf("commit author %q does not own the ref of %q (an event sits only in its author's ref)", c.Stream, r.Owner)
			case c.Sig == sigUnsigned:
				bad = "commit is unsigned"
			case c.Sig == sigBad || c.FP == "":
				bad = "commit signature does not verify"
			}
			if len(c.Lines) == 0 && bad != "" {
				n := &node{
					id:      EventID{Member: member, Clone: clone, Broken: brokenTag(c, -1)},
					stream:  r.Owner,
					commit:  c,
					line:    &lineRec{TS: c.TS},
					pos:     -1,
					verdict: Flagged,
					reason:  bad,
				}
				all = append(all, n)
				continue
			}
			first := true
			for i := range c.Lines {
				ln := &c.Lines[i]
				n := &node{stream: r.Owner, commit: c, line: ln, pos: -1}
				reason := bad
				if reason == "" {
					reason = ln.Bad
				}
				if reason == "" && ln.Seq != expected {
					if ln.Seq < expected {
						reason = fmt.Sprintf("seq %d duplicates an earlier event (expected %d; a broken event consumes no seq)", ln.Seq, expected)
					} else {
						reason = fmt.Sprintf("seq %d leaves a gap (expected %d; a broken event consumes no seq)", ln.Seq, expected)
					}
				}
				if reason != "" {
					n.id = EventID{Member: member, Clone: clone, Seq: ln.Seq, Broken: brokenTag(c, i)}
					n.verdict, n.reason = Flagged, reason
				} else {
					n.id = EventID{Member: member, Clone: clone, Seq: ln.Seq}
					n.state = stWaiting // provisional until placed
					n.first = first
					first = false
					expected++
					v.streams[r.Owner] = append(v.streams[r.Owner], n)
					if isIdentityType(ln.Type) {
						n.idp, n.idErr = parseIDPayload(ln.Type, ln.Payload)
					}
				}
				all = append(all, n)
			}
		}
	}
	for _, n := range all {
		v.byID[n.id] = n
	}

	// Causal predecessors.
	for _, list := range v.streams {
		for _, n := range list {
			if n.id.Seq > 1 {
				n.prev = list[n.id.Seq-2]
				n.preds = append(n.preds, n.prev)
			}
			if !n.first {
				continue
			}
			keys := make([]string, 0, len(n.commit.Seen))
			for s := range n.commit.Seen {
				keys = append(keys, s)
			}
			sort.Strings(keys)
			for _, s := range keys {
				q := n.commit.Seen[s]
				if s == n.stream || q <= 0 {
					continue
				}
				other := v.streams[s]
				if q > len(other) {
					if n.waitOn == "" {
						n.waitOn = s + "#" + strconv.Itoa(q)
					}
					continue
				}
				p := other[q-1]
				dup := false
				for _, x := range n.preds {
					dup = dup || x == p
				}
				if !dup {
					n.preds = append(n.preds, p)
				}
			}
		}
	}
	var causal []*node
	for _, n := range all {
		if n.state == stBroken {
			continue
		}
		causal = append(causal, n)
		n.indeg = len(n.preds)
		for _, p := range n.preds {
			p.succs = append(p.succs, n)
		}
	}

	// Linearize: Kahn's algorithm, ready ties broken by ts, member, clone, seq.
	ready := &nodeHeap{}
	for _, n := range causal {
		if n.indeg == 0 && n.waitOn == "" {
			heap.Push(ready, n)
		}
	}
	for ready.Len() > 0 {
		n := heap.Pop(ready).(*node)
		n.state = stResolved
		n.pos = len(v.order)
		v.order = append(v.order, n)
		if n.first {
			past := map[string]int{}
			for _, p := range n.preds {
				for s, q := range p.past {
					if past[s] < q {
						past[s] = q
					}
				}
				if past[p.stream] < p.id.Seq {
					past[p.stream] = p.id.Seq
				}
			}
			n.past = past
		} else {
			n.past = n.prev.past
		}
		for _, s := range n.succs {
			s.indeg--
			if s.indeg == 0 && s.waitOn == "" {
				heap.Push(ready, s)
			}
		}
	}

	// Unplaced events wait on a missing event, or sit in or after a cycle.
	var queue []*node
	for _, n := range causal {
		if n.state != stResolved && n.waitOn != "" {
			queue = append(queue, n)
		}
	}
	reached := map[*node]bool{}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if reached[n] {
			continue
		}
		reached[n] = true
		for _, s := range n.succs {
			if s.state != stResolved && !reached[s] {
				if s.waitOn == "" {
					s.waitOn = n.waitOn
				}
				queue = append(queue, s)
			}
		}
	}
	for _, n := range causal {
		if n.state == stResolved {
			continue
		}
		if reached[n] {
			n.state = stWaiting
			n.verdict = Waiting
			n.reason = fmt.Sprintf("waiting for %s, which its causal past names but this client has not received", n.waitOn)
		} else {
			n.state = stCycle
			n.verdict = Flagged
			n.reason = "causal cycle: its seen names an event that transitively saw it"
		}
	}
	for s, list := range v.streams {
		h := 0
		for h < len(list) && list[h].state == stResolved {
			h++
		}
		if h > 0 {
			v.heads[s] = h
		}
	}

	ev := &evaluator{v: v, in: in, genesis: initialIdentity(in.Roles), revMemo: map[*node]result{}}
	ev.run()

	// Display order: causal order, with events outside it placed by ts.
	var others []*node
	for _, n := range all {
		if n.state != stResolved {
			others = append(others, n)
		}
	}
	sort.SliceStable(others, func(i, j int) bool { return nodeLess(others[i], others[j]) })
	i, j := 0, 0
	for i < len(v.order) || j < len(others) {
		if j < len(others) && (i == len(v.order) || others[j].line.TS.Before(v.order[i].line.TS)) {
			v.nodes = append(v.nodes, others[j])
			j++
		} else {
			v.nodes = append(v.nodes, v.order[i])
			i++
		}
	}
	return v
}

func nodeLess(a, b *node) bool {
	if !a.line.TS.Equal(b.line.TS) {
		return a.line.TS.Before(b.line.TS)
	}
	if a.id.Member != b.id.Member {
		return a.id.Member < b.id.Member
	}
	if a.id.Clone != b.id.Clone {
		return a.id.Clone < b.id.Clone
	}
	if a.id.Seq != b.id.Seq {
		return a.id.Seq < b.id.Seq
	}
	return a.id.Broken < b.id.Broken
}

type nodeHeap []*node

func (h nodeHeap) Len() int           { return len(h) }
func (h nodeHeap) Less(i, j int) bool { return nodeLess(h[i], h[j]) }
func (h nodeHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *nodeHeap) Push(x any)        { *h = append(*h, x.(*node)) }
func (h *nodeHeap) Pop() any {
	old := *h
	n := old[len(old)-1]
	*h = old[:len(old)-1]
	return n
}

// Identity state.

type idKey struct{ norm, fp string }

type idMember struct {
	id      string
	name    string
	keys    []idKey
	revoked []idRevoked
	leads   json.RawMessage
	joined  *node
	removed *node
}

type idRevoked struct {
	key idKey
	by  *node
}

func (m *idMember) holds(fp string) bool {
	for _, k := range m.keys {
		if k.fp == fp {
			return true
		}
	}
	return false
}

func (m *idMember) revokedBy(fp string) *node {
	for _, r := range m.revoked {
		if r.key.fp == fp {
			return r.by
		}
	}
	return nil
}

type idState struct {
	admins  map[string]bool
	members map[string]*idMember
}

func initialIdentity(roles Roles) *idState {
	s := &idState{admins: map[string]bool{}, members: map[string]*idMember{}}
	for _, rm := range roles.Members {
		if !ValidID(rm.ID) || s.members[rm.ID] != nil {
			continue
		}
		m := &idMember{id: rm.ID, name: rm.Name, leads: rm.Leads}
		for _, k := range rm.Keys {
			if norm, fp, err := parseKey(k); err == nil && !s.keyInUse(fp) {
				m.keys = append(m.keys, idKey{norm, fp})
			}
		}
		s.members[rm.ID] = m
	}
	for _, a := range roles.Admins {
		if s.members[a] != nil {
			s.admins[a] = true
		}
	}
	return s
}

func (s *idState) clone() *idState {
	c := &idState{admins: make(map[string]bool, len(s.admins)), members: make(map[string]*idMember, len(s.members))}
	for a := range s.admins {
		c.admins[a] = true
	}
	for id, m := range s.members {
		cp := *m
		cp.keys = append([]idKey(nil), m.keys...)
		cp.revoked = append([]idRevoked(nil), m.revoked...)
		c.members[id] = &cp
	}
	return c
}

func (s *idState) keyInUse(fp string) bool {
	for _, m := range s.members {
		if m.holds(fp) || m.revokedBy(fp) != nil {
			return true
		}
	}
	return false
}

// apply folds one valid identity event.
func (s *idState) apply(n *node) {
	p := n.idp
	m := s.members[p.member]
	switch n.typ() {
	case TypeMemberJoin:
		if m == nil {
			s.members[p.member] = &idMember{id: p.member, name: p.name, keys: []idKey{{p.keyNorm, p.keyFP}}, joined: n}
		}
	case TypeAddAdmin:
		s.admins[p.member] = true
	case TypeRemoveAdmin:
		delete(s.admins, p.member)
	case TypeAddKey:
		if m != nil {
			m.keys = append(m.keys, idKey{p.keyNorm, p.keyFP})
		}
	case TypeRevokeKey:
		if m != nil {
			kept := m.keys[:0]
			for _, k := range m.keys {
				if k.fp != p.keyFP {
					kept = append(kept, k)
				}
			}
			m.keys = kept
			m.revoked = append(m.revoked, idRevoked{key: idKey{p.keyNorm, p.keyFP}, by: n})
		}
	case TypeRemoveMember:
		if m != nil {
			m.removed = n
		}
		delete(s.admins, p.member)
	}
}

// Verdicts.

// revCut is one valid revocation's cut: events of member signed with keyFP
// above the revoker's view of their stream are ignored.
type revCut struct {
	node   *node
	member string
	keyFP  string
}

func (r revCut) cutoff(stream string) int { return pastSeq(r.node, stream) }

type evaluator struct {
	v       *view
	in      *verifyInput
	genesis *idState
	// revMemo holds each revocation's verdict, judged against its own causal
	// past only: a concurrent revocation never cuts it, directly or through
	// an event in its past.
	revMemo map[*node]result
}

func (e *evaluator) run() {
	var cuts []revCut
	for _, r := range e.v.order {
		if !r.isRevoke() {
			continue
		}
		var scoped []revCut
		for _, c := range cuts {
			if hb(c.node, r) {
				scoped = append(scoped, c)
			}
		}
		p := e.newPass(scoped)
		res := p.run(func(n *node) bool { return n == r || hb(n, r) })
		e.revMemo[r] = res[r.pos]
		if res[r.pos].verdict == Valid {
			cuts = append(cuts, revCut{node: r, member: r.idp.member, keyFP: r.idp.keyFP})
		}
	}
	p := e.newPass(cuts)
	res := p.run(nil)
	for _, n := range e.v.order {
		n.verdict, n.reason = res[n.pos].verdict, res[n.pos].reason
	}
	e.v.commitRes = p.commitRes
	e.v.revs = cuts
	st := e.genesis.clone()
	for _, n := range e.v.order {
		if n.verdict == Valid && n.idp != nil && n.idErr == nil {
			st.apply(n)
		}
	}
	e.v.ident = st
}

type pass struct {
	e         *evaluator
	cuts      []revCut
	res       []result
	validIDs  []*node // identity nodes judged valid so far, causal order
	waiting   []*node // nodes judged waiting so far
	commitRes map[*commitRec]result
	joinRes   map[*node]result
}

func (e *evaluator) newPass(cuts []revCut) *pass {
	return &pass{
		e:         e,
		cuts:      cuts,
		res:       make([]result, len(e.v.order)),
		commitRes: map[*commitRec]result{},
		joinRes:   map[*node]result{},
	}
}

// run judges the resolved nodes in scope (nil: all) in causal order and
// returns the results by position.
func (p *pass) run(scope func(*node) bool) []result {
	for _, n := range p.e.v.order {
		if scope != nil && !scope(n) {
			continue
		}
		r := p.judge(n)
		p.res[n.pos] = r
		switch {
		case r.verdict == Waiting:
			p.waiting = append(p.waiting, n)
		case r.verdict == Valid && n.idp != nil:
			p.validIDs = append(p.validIDs, n)
		}
	}
	return p.res
}

// stateBefore folds genesis and the valid identity events in n's causal past.
func (p *pass) stateBefore(n *node) *idState {
	st := p.e.genesis.clone()
	for _, i := range p.validIDs {
		if hb(i, n) {
			st.apply(i)
		}
	}
	return st
}

func (p *pass) judge(n *node) result {
	for _, w := range p.waiting {
		if hb(w, n) {
			return result{Waiting, fmt.Sprintf("waiting: its causal past holds %s, which is waiting", w.id)}
		}
	}
	st := p.stateBefore(n)
	if n.first {
		p.commitRes[n.commit] = p.signer(n, st)
	}
	// A memoized revocation still sets its commit's signer result above:
	// the zero result is Valid, so the events after it in the commit would
	// otherwise skip the signer check.
	if m, ok := p.e.revMemo[n]; ok {
		return m
	}
	if r := p.commitRes[n.commit]; r.verdict != Valid {
		return r
	}
	author := n.id.Member
	if n.typ() == TypeMemberJoin {
		if r := p.join(n, st); r.verdict != Valid {
			return r
		}
	} else {
		m := st.members[author]
		if m == nil {
			return result{Flagged, fmt.Sprintf("author %q is not a member in its causal past", author)}
		}
		if m.removed != nil {
			return result{Flagged, fmt.Sprintf("author %q was removed from the store (%s)", author, m.removed.id)}
		}
	}
	if n.idp != nil || n.idErr != nil {
		if r := p.roles(n, st); r.verdict != Valid {
			return r
		}
		if veto := p.e.in.Veto; veto != nil {
			if err := veto(p.event(n), p.pastEvents(n)); err != nil {
				return result{Flagged, err.Error()}
			}
		}
	}
	if !n.isRevoke() {
		for _, c := range p.cuts {
			if c.member == author && c.keyFP == n.commit.FP && n.id.Seq > c.cutoff(n.stream) {
				return result{Ignored, fmt.Sprintf("signed with a key that %s revoked having seen %s only up to seq %d; events from a revoked key that the revoker had not seen are ignored, whatever their seen claims",
					c.node.id, n.stream, c.cutoff(n.stream))}
			}
		}
	}
	return valid
}

// signer checks the commit's signer against the keys its author held in the
// commit's causal past. A commit whose first event is a valid member.join
// verifies against the key that join declares.
func (p *pass) signer(n *node, st *idState) result {
	fp := n.commit.FP
	m := st.members[n.id.Member]
	if m != nil && m.holds(fp) {
		return valid
	}
	if n.typ() == TypeMemberJoin {
		j := p.join(n, st)
		if j.verdict == Waiting {
			return j
		}
		if j.verdict == Valid && n.idp.keyFP == fp {
			return valid
		}
	}
	if m != nil {
		if by := m.revokedBy(fp); by != nil {
			return result{Flagged, fmt.Sprintf("signed with a key of %s already revoked in its own causal past (%s)", n.id.Member, by.id)}
		}
	}
	return result{Flagged, fmt.Sprintf("signer key %s is not a key of %s in its causal past", fp, n.id.Member)}
}

// join judges a member.join. It is valid when it matches its id's claim ref
// and the id is free in its causal past, or as a no-op when the id already
// holds the declared key.
func (p *pass) join(n *node, st *idState) result {
	if r, ok := p.joinRes[n]; ok {
		return r
	}
	r := p.joinUncached(n, st)
	p.joinRes[n] = r
	return r
}

func (p *pass) joinUncached(n *node, st *idState) result {
	if n.idErr != nil {
		return result{Flagged, "member.join payload: " + n.idErr.Error()}
	}
	jp := n.idp
	if jp.member != n.id.Member {
		return result{Flagged, fmt.Sprintf("member.join names %q but sits in a stream of %q", jp.member, n.id.Member)}
	}
	if m := st.members[jp.member]; m != nil {
		if m.removed != nil {
			return result{Flagged, fmt.Sprintf("member id %q was removed (%s); member ids are not reused", jp.member, m.removed.id)}
		}
		if m.holds(jp.keyFP) {
			return valid // no-op: the id already holds this key
		}
		return result{Flagged, fmt.Sprintf("member id %q is already held in its causal past", jp.member)}
	}
	if st.keyInUse(jp.keyFP) {
		return result{Flagged, "the declared key is already in use by another member"}
	}
	claim := p.e.in.Claims[jp.member]
	if claim == nil {
		return result{Waiting, fmt.Sprintf("waiting for the claim ref of %q", jp.member)}
	}
	if !claimMatches(claim, n) {
		return result{Flagged, fmt.Sprintf("member id %q was claimed by another join", jp.member)}
	}
	return valid
}

// claimMatches reports whether the claim commit carries exactly this join
// (same event id and declared key) and is signed by the declared key.
func claimMatches(c *commitRec, n *node) bool {
	if c.Bad != "" || !c.Root || c.Sig != sigGood || c.FP != n.idp.keyFP || c.Stream != n.stream || len(c.Lines) != 1 {
		return false
	}
	ln := c.Lines[0]
	if ln.Bad != "" || ln.Type != TypeMemberJoin || ln.Seq != n.id.Seq || ln.Seq != 1 {
		return false
	}
	cp, err := parseIDPayload(ln.Type, ln.Payload)
	return err == nil && cp.member == n.idp.member && cp.keyFP == n.idp.keyFP
}

// roles judges the admin identity events. The join has its own check.
func (p *pass) roles(n *node, st *idState) result {
	t := n.typ()
	if t == TypeMemberJoin {
		return valid
	}
	if n.idErr != nil {
		return result{Flagged, t + " payload: " + n.idErr.Error()}
	}
	author, target := n.id.Member, n.idp.member
	if !st.admins[author] {
		return result{Flagged, fmt.Sprintf("%s is admin-only (%s is not an admin in its causal past)", t, author)}
	}
	m := st.members[target]
	if m == nil {
		return result{Flagged, fmt.Sprintf("unknown member %q", target)}
	}
	switch t {
	case TypeAddAdmin:
		if m.removed != nil {
			return result{Flagged, fmt.Sprintf("%s was removed (%s)", target, m.removed.id)}
		}
		if st.admins[target] {
			return result{Flagged, target + " is already an admin"}
		}
	case TypeRemoveAdmin:
		if !st.admins[target] {
			return result{Flagged, target + " is not an admin"}
		}
		if len(st.admins) <= 1 {
			return result{Flagged, "cannot remove the last admin"}
		}
	case TypeAddKey:
		if m.removed != nil {
			return result{Flagged, fmt.Sprintf("%s was removed (%s)", target, m.removed.id)}
		}
		if st.keyInUse(n.idp.keyFP) {
			return result{Flagged, "the key is already in use"}
		}
	case TypeRevokeKey:
		if by := m.revokedBy(n.idp.keyFP); by != nil {
			return result{Flagged, fmt.Sprintf("the key is already revoked (%s)", by.id)}
		}
		if !m.holds(n.idp.keyFP) {
			return result{Flagged, "the key is not a key of " + target}
		}
	case TypeRemoveMember:
		if m.removed != nil {
			return result{Flagged, fmt.Sprintf("%s was already removed (%s)", target, m.removed.id)}
		}
		if st.admins[target] {
			return result{Flagged, "remove the admin role of " + target + " first"}
		}
	}
	return valid
}

func (p *pass) event(n *node) Event {
	e := n.event()
	r := p.res[n.pos]
	e.Verdict, e.Reason = r.verdict, r.reason
	return e
}

// pastEvents lists the events of n's causal past with this pass's verdicts.
func (p *pass) pastEvents(n *node) []Event {
	var out []Event
	for _, x := range p.e.v.order[:n.pos] {
		if hb(x, n) {
			out = append(out, p.event(x))
		}
	}
	return out
}

func (n *node) event() Event {
	var seen map[string]int
	if len(n.commit.Seen) > 0 {
		seen = make(map[string]int, len(n.commit.Seen))
		for k, q := range n.commit.Seen {
			seen[k] = q
		}
	}
	return Event{
		ID:      n.id,
		TS:      n.line.TS,
		Origin:  n.line.Origin,
		Type:    n.line.Type,
		Payload: n.line.Payload,
		Seen:    seen,
		Verdict: n.verdict,
		Reason:  n.reason,
		Pending: n.commit.Pending,
	}
}

// identity renders the folded identity state.
func (v *view) identity(genesisRaw json.RawMessage) IdentityState {
	out := IdentityState{Genesis: genesisRaw}
	if v.ident == nil {
		return out
	}
	for a := range v.ident.admins {
		out.Admins = append(out.Admins, a)
	}
	sort.Strings(out.Admins)
	ids := make([]string, 0, len(v.ident.members))
	for id := range v.ident.members {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		m := v.ident.members[id]
		pm := Member{ID: m.id, Name: m.name, Leads: m.leads}
		for _, k := range m.keys {
			pm.Keys = append(pm.Keys, k.norm)
		}
		for _, r := range m.revoked {
			cut := map[string]int{}
			for s := range v.streams {
				if mem, _, _ := splitStream(s); mem == id {
					cut[s] = pastSeq(r.by, s)
				}
			}
			pm.Revoked = append(pm.Revoked, RevokedKey{Key: r.key.norm, By: r.by.id, Cutoff: cut})
		}
		if m.joined != nil {
			pm.Joined = m.joined.id
		}
		if m.removed != nil {
			id := m.removed.id
			pm.Removed = &id
		}
		out.Members = append(out.Members, pm)
	}
	return out
}

// holdsKey reports whether member holds the key fingerprint in the folded
// identity state.
func (v *view) holdsKey(member, fp string) bool {
	if v.ident == nil {
		return false
	}
	m := v.ident.members[member]
	return m != nil && m.removed == nil && m.holds(fp)
}
