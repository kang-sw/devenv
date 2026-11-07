package wstracker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
)

// Tracker is one clone's tracker over a Source: it caches the events read,
// refreshes them incrementally through the Events cursor, derives the view
// state, and writes through Dispatch.
type Tracker struct {
	src     Source
	stream  string // the viewer and writer clone, "<member>.<clone>"
	genesis json.RawMessage

	cur    wsledger.Cursor
	events map[wsledger.EventID]wsledger.Event
	order  []wsledger.EventID // first-read order
	r      *replayed
}

// New returns the tracker of clone stream ("<member>.<clone>") over src,
// with everything src holds read. stream must be src's own writer clone
// (for a *wsledger.Ledger, MemberID() + "." + CloneID()): Source cannot
// report it, and NextID and Dispatch's judged ids are computed from it.
func New(src Source, stream string) *Tracker {
	t := &Tracker{src: src, stream: stream, genesis: src.Identity().Genesis}
	t.Refresh()
	return t
}

// Refresh reads what changed since the last read: new events and events
// whose verdict or pending state changed behind the cursor (a revocation
// cut, a filled gap). A cursor of another epoch reads everything, which
// replaces the cache. It re-folds when anything changed and reports whether
// it did.
func (t *Tracker) Refresh() bool {
	evs, cur := t.src.Events(t.cur)
	reset := t.events == nil || cur.Epoch != t.cur.Epoch
	t.cur = cur
	if reset {
		t.events, t.order = map[wsledger.EventID]wsledger.Event{}, nil
	}
	for _, e := range evs {
		if _, ok := t.events[e.ID]; !ok {
			t.order = append(t.order, e.ID)
		}
		t.events[e.ID] = e
	}
	if !reset && len(evs) == 0 && t.r != nil {
		return false
	}
	list := make([]wsledger.Event, len(t.order))
	for i, id := range t.order {
		list[i] = t.events[id]
	}
	t.r = replay(list, t.genesis, t.src.Concurrent)
	return true
}

// View refreshes and returns the view state at now; staleDays <= 0 means
// DefaultStaleDays, a zero now turns stale off.
func (t *Tracker) View(now time.Time, staleDays int) *View {
	t.Refresh()
	return t.r.view(t.stream, now, staleDays)
}

// Refusal is Dispatch's error when an event would be flagged.
type Refusal struct {
	Index  int // the event's index in the Dispatch call
	Type   string
	Reason string // the flagged reason it would get
}

func (e *Refusal) Error() string {
	return fmt.Sprintf("event %d (%s) would be flagged: %s", e.Index, e.Type, e.Reason)
}

// Dispatch validates events against this clone's current state and appends
// them in one Append (294 a). A call is all-or-nothing: each event is judged
// against the state including the call's earlier events, and the first one
// that would be flagged returns a *Refusal with its reason and nothing is
// appended. The call's events get consecutive ids from NextID, so a later
// event of the call may name an item an earlier one creates.
//
// Identity events other than member.join (which wsledger.Join writes) are
// judged by the protocol's rules plus (209) against the identity state the
// domain folds; a domain event by the domain rules after the protocol's
// author check.
//
// The state judged is everything this clone holds, which is exactly the new
// events' causal past, so the verdict is the one every client will reach,
// with one exception: when the clone holds a waiting event, the new commit's
// seen covers it, so the new events wait too and are judged again once the
// gap fills, against a past that then includes the filled events. Such an
// event can still end up flagged.
func (t *Tracker) Dispatch(ctx context.Context, events []wsledger.NewEvent) ([]wsledger.EventID, error) {
	t.Refresh()
	if err := t.validate(events); err != nil {
		return nil, err
	}
	ids, err := t.src.Append(ctx, events)
	t.Refresh()
	return ids, err
}

// NextID is the id the next event this clone appends gets: the next seq of
// its stream after every event read (pending ones included). Dispatch
// validates with it, so a call can create an item and name it in a later
// event.
func (t *Tracker) NextID() wsledger.EventID {
	member, clone, _ := strings.Cut(t.stream, ".")
	seq := 0
	for id := range t.events {
		if id.Stream() == t.stream && id.Broken == "" && id.Seq > seq {
			seq = id.Seq
		}
	}
	return wsledger.EventID{Member: member, Clone: clone, Seq: seq + 1}
}

func (t *Tracker) validate(events []wsledger.NewEvent) error {
	r := t.r
	S := newState(r.g, r.hb, false)
	for _, n := range r.order {
		if n.status == "applied" {
			S.apply(n)
		}
	}
	next := t.NextID()
	now := time.Now().UTC()
	for i, e := range events {
		id := next
		id.Seq += i
		n := newNode(wsledger.Event{ID: id,
			TS: now, Origin: e.Origin, Type: e.Type, Payload: e.Payload, Verdict: wsledger.Valid})
		n.synthetic, n.pos = true, len(r.order)+i
		var rej rejection
		switch {
		case e.Type == wsledger.TypeMemberJoin:
			rej = "member.join is written by joining the store, not dispatched"
		case isIdentity(e.Type):
			rej = S.checkIdentity(n)
		default:
			if rej = S.checkAuthor(n); rej == "" {
				rej = S.check(n)
			}
		}
		if rej != "" {
			return &Refusal{Index: i, Type: e.Type, Reason: string(rej)}
		}
		n.status = "applied"
		S.apply(n)
	}
	return nil
}

// checkAuthor is the protocol's membership check on an event's author.
func (s *state) checkAuthor(n *node) rejection {
	m := s.members[n.member]
	if m == nil {
		return rejection(fmt.Sprintf("author %q is not a member in its causal past", n.member))
	}
	if m.removed != nil {
		return rejection(fmt.Sprintf("author %q was removed from the store (%s)", n.member, m.removed.id))
	}
	return ""
}

// checkIdentity mirrors the protocol's rules for the admin identity events
// (wsledger verify.go, roles) on the domain's folded identity state, plus
// the (209) veto, with the protocol's reasons.
func (s *state) checkIdentity(n *node) rejection {
	if r := s.checkAuthor(n); r != "" {
		return r
	}
	t, raw := n.typ, n.ev.Payload
	dec := func(v any) error { return json.NewDecoder(bytes.NewReader(raw)).Decode(v) }
	var target string
	var key memberKey
	switch t {
	case wsledger.TypeAddKey, wsledger.TypeRevokeKey:
		var k wsledger.KeyPayload
		if err := dec(&k); err != nil {
			return rejection(t + " payload: " + err.Error())
		}
		norm, fp, err := wsledger.KeyFingerprint(k.Key)
		if err != nil {
			return rejection(t + " payload: " + err.Error())
		}
		target, key = k.Member, memberKey{norm, fp}
	default:
		var m wsledger.MemberPayload
		if err := dec(&m); err != nil {
			return rejection(t + " payload: " + err.Error())
		}
		target = m.Member
	}
	if !wsledger.ValidID(target) {
		return rejection(fmt.Sprintf("%s payload: member %q is not a valid member id", t, target))
	}
	author := n.member
	if !s.admins[author] {
		return rejection(fmt.Sprintf("%s is admin-only (%s is not an admin in its causal past)", t, author))
	}
	m := s.members[target]
	if m == nil {
		return rejection(fmt.Sprintf("unknown member %q", target))
	}
	switch t {
	case wsledger.TypeAddAdmin:
		if m.removed != nil {
			return rejection(fmt.Sprintf("%s was removed (%s)", target, m.removed.id))
		}
		if s.admins[target] {
			return rejection(target + " is already an admin")
		}
	case wsledger.TypeRemoveAdmin:
		if !s.admins[target] {
			return rejection(target + " is not an admin")
		}
		if len(s.admins) <= 1 {
			return "cannot remove the last admin"
		}
	case wsledger.TypeAddKey:
		if m.removed != nil {
			return rejection(fmt.Sprintf("%s was removed (%s)", target, m.removed.id))
		}
		if s.keyInUse(key) {
			return "the key is already in use"
		}
	case wsledger.TypeRevokeKey:
		if by := m.revokedBy(key); by != nil {
			return rejection(fmt.Sprintf("the key is already revoked (%s)", by.id))
		}
		if !m.holds(key) {
			return rejection("the key is not a key of " + target)
		}
	case wsledger.TypeRemoveMember:
		if m.removed != nil {
			return rejection(fmt.Sprintf("%s was already removed (%s)", target, m.removed.id))
		}
		if s.admins[target] {
			return rejection("remove the admin role of " + target + " first")
		}
		return s.leadsNobody(target)
	}
	return ""
}

func (s *state) keyInUse(k memberKey) bool {
	for _, m := range s.members {
		if m.holds(k) || m.revokedBy(k) != nil {
			return true
		}
	}
	return false
}
