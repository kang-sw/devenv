package wsledger

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// EventID names one event: "<member>.<clone>#<seq>".
type EventID struct {
	Member string
	Clone  string
	Seq    int
	// Broken is empty for an event inside causality. An event that breaks an
	// envelope rule (wrong ref, seq gap or duplicate, bad or missing
	// signature, malformed commit) consumes no seq; it keeps the seq it
	// claimed for display and gets Broken = "<commit>:<line>", so EventID
	// stays unique and stable across refreshes.
	Broken string
}

// Stream is the clone stream the event belongs to: "<member>.<clone>".
func (id EventID) Stream() string { return id.Member + "." + id.Clone }

func (id EventID) String() string {
	s := id.Stream() + "#" + strconv.Itoa(id.Seq)
	if id.Broken != "" {
		s += "!" + id.Broken
	}
	return s
}

// ParseEventID parses the String form of an EventID.
func ParseEventID(s string) (EventID, error) {
	var id EventID
	rest := s
	if i := strings.IndexByte(rest, '!'); i >= 0 {
		id.Broken = rest[i+1:]
		rest = rest[:i]
	}
	hash := strings.LastIndexByte(rest, '#')
	if hash < 0 {
		return EventID{}, fmt.Errorf("event id %q: missing #<seq>", s)
	}
	seq, err := strconv.Atoi(rest[hash+1:])
	if err != nil || seq < 0 {
		return EventID{}, fmt.Errorf("event id %q: bad seq", s)
	}
	member, clone, ok := splitStream(rest[:hash])
	if !ok {
		return EventID{}, fmt.Errorf("event id %q: bad stream", s)
	}
	id.Member, id.Clone, id.Seq = member, clone, seq
	return id, nil
}

// Verdict is what verification concluded about one event.
type Verdict int

const (
	// Valid: the event passed every protocol rule against its causal past.
	Valid Verdict = iota
	// Waiting: the event's causal past names an event this client has not
	// received (or a join whose claim ref has not arrived); no verdict yet.
	Waiting
	// Flagged: the event breaks a protocol rule. Reason says which.
	Flagged
	// Ignored: the event was signed with a key a valid revocation cut, above
	// the revoker's cutoff. This is the one verdict a late arrival can give
	// to an already shown event.
	Ignored
)

func (v Verdict) String() string {
	switch v {
	case Valid:
		return "valid"
	case Waiting:
		return "waiting"
	case Flagged:
		return "flagged"
	case Ignored:
		return "ignored"
	}
	return "verdict(" + strconv.Itoa(int(v)) + ")"
}

// Event is one verified event as the domain sees it.
type Event struct {
	ID EventID
	// TS is the event line's own write time.
	TS time.Time
	// Origin is carried for the domain ("human", "agent", "agent-auto");
	// the protocol does not judge it.
	Origin string
	Type   string
	// Payload is opaque to the protocol except for the identity events it
	// owns.
	Payload json.RawMessage
	// Seen is the commit's seen: "<member>.<clone>" -> last seq received.
	Seen    map[string]int
	Verdict Verdict
	// Reason is set when Verdict is Waiting, Flagged, or Ignored.
	Reason string
	// Pending marks an event of this clone still in its journal, not yet in
	// a commit. Its verdict is provisional until the batch closes.
	Pending bool
}

// NewEvent is one event the domain appends.
type NewEvent struct {
	Type    string
	Origin  string
	Payload json.RawMessage
}

// Cursor is a read position for Events: a per-stream watermark of the
// causal events already delivered, plus the verdict generation the caller
// has seen. The zero Cursor reads everything. A cursor from another Ledger
// instance (another Epoch) also reads everything, because generations are
// per instance.
type Cursor struct {
	Watermark map[string]int
	Gen       uint64
	Epoch     string
}

// Store is the protocol surface the domain builds on.
type Store interface {
	// Append writes events durably to this clone's journal and returns
	// their ids. They join the open batch.
	Append(ctx context.Context, events []NewEvent) ([]EventID, error)
	// Sync is the apply path: close the open batch, push this clone's ref,
	// fetch, and verify. With no remote it closes the batch and verifies
	// local refs.
	Sync(ctx context.Context) (SyncReport, error)
	// Fetch receives only: fetch and verify, never push. When new events
	// arrive it first closes the open batch into a local commit.
	Fetch(ctx context.Context) (SyncReport, error)
	// Events returns the events new since after, plus every event whose
	// verdict (or pending state) changed since it, in display order: causal
	// order with ties by ts, member, clone, seq; events outside causality
	// (waiting, broken) placed by ts.
	Events(after Cursor) ([]Event, Cursor)
	// Concurrent reports whether neither event is in the other's causal
	// past. Unknown, waiting, or broken events are never concurrent.
	Concurrent(a, b EventID) bool
	// Identity returns the identity state folded from genesis and every
	// valid identity event.
	Identity() IdentityState
}

// PendingReport describes this clone's own writes that have not reached
// the remote yet.
type PendingReport struct {
	// Count is the number of own writes not yet pushed. With no remote it
	// counts writes not yet committed.
	Count int
	// Oldest is the write time of the oldest of them; zero when Count is 0.
	Oldest time.Time
	// Uncommitted is the number of writes still in the open batch.
	Uncommitted int
	// KeyNotRegistered: this clone's signing key is not a live key of its
	// member in the store's identity state, so the batch stays open until
	// an admin adds the key.
	KeyNotRegistered bool
	// IDTaken: this clone's member.join lost its id to another join's
	// claim. Nothing of this clone is pushed; join under another id.
	IDTaken bool
	// ForeignAdvance: the remote copy of this clone's own ref holds commits
	// this clone did not write, so it cannot push. MoveToFreshClone moves
	// the unpushed events to a fresh clone id.
	ForeignAdvance bool
	// PushDue is when the idle push delay fires; zero when nothing is
	// pending or the store has no remote.
	PushDue time.Time
}

// Warning reports a ref anomaly. Kind is one of the Warn* constants.
type Warning struct {
	Ref    string
	Kind   string
	Detail string
}

const (
	// WarnDeleted: a ref this clone holds was deleted on the remote.
	WarnDeleted = "deleted"
	// WarnRewound: a remote ref moved to a non-descendant of the local copy,
	// or a create-once ref (genesis, claim) moved at all.
	WarnRewound = "rewound"
	// WarnForeignAdvance: the remote advanced this clone's own ref with
	// commits this clone did not write.
	WarnForeignAdvance = "foreign-advance"
	// WarnBadRef: a ref under the ledger namespace whose name is not a
	// ledger ref; it is not read.
	WarnBadRef = "bad-ref"
)

// SyncReport is the outcome of one Sync or Fetch.
type SyncReport struct {
	// Closed is true when the open batch was closed into a commit.
	Closed bool
	// Received counts events that appeared in this view during the call.
	Received int
	// Pushed is true when this clone's ref was pushed.
	Pushed bool
	// Reissued counts events re-issued under a newly registered key.
	Reissued int
	Warnings []Warning
	Pending  PendingReport
}

// IdentityState is the protocol's identity view: genesis plus every valid
// identity event, in causal order.
type IdentityState struct {
	// Genesis is the raw roles.json of the genesis ref, for the domain's own
	// fields (leads).
	Genesis json.RawMessage
	// Admins lists admin member ids, sorted.
	Admins []string
	// Members lists every member, removed ones included, sorted by id.
	Members []Member
}

// Member is one member of the identity state.
type Member struct {
	ID   string
	Name string
	// Keys are the live public keys, "<type> <base64>".
	Keys []string
	// Revoked lists revoked keys with their cutoffs.
	Revoked []RevokedKey
	// Leads is the member's genesis "leads" value, passed through untouched;
	// nil for members who joined later.
	Leads json.RawMessage
	// Joined is the member.join that added the member; zero for genesis
	// members.
	Joined EventID
	// Removed is the roles.remove_member that removed the member, if any.
	Removed *EventID
}

// RevokedKey is one revocation.
type RevokedKey struct {
	Key string
	By  EventID
	// Cutoff maps each of the member's streams to the last seq the revoking
	// admin had seen. Events signed with Key above it are ignored.
	Cutoff map[string]int
}

// VetoFunc is the domain veto hook for identity events. It receives an
// identity event that passed the protocol rules plus the verified events of
// its causal past with their verdicts, in causal order. A non-nil result
// flags the event with err.Error() as the reason. It must be a pure function
// of its arguments: every client must reach the same answer.
type VetoFunc func(ev Event, past []Event) error
