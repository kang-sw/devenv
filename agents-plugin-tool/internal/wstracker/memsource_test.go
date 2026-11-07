package wstracker

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
)

// memSource is an in-memory Source: one clone's held events with their
// protocol verdicts, causality from each event's seen. Appended events get
// the next seqs of the clone's stream, the seen of everything held, and a
// Valid verdict (the domain pre-validates; the protocol rules beyond
// membership are not simulated here). Events honors the cursor: it returns
// the events added or changed since it.
type memSource struct {
	genesis json.RawMessage
	stream  string
	events  []wsledger.Event
	clock   func() time.Time

	byID map[wsledger.EventID]int
	gens map[wsledger.EventID]uint64 // generation of each event's last add or change
	gen  uint64
	c    *causal
}

func newMemSource(genesis json.RawMessage, stream string, events []wsledger.Event) *memSource {
	m := &memSource{genesis: genesis, stream: stream, byID: map[wsledger.EventID]int{}, gens: map[wsledger.EventID]uint64{}}
	m.add(events...)
	return m
}

// add holds events (an event already held is replaced, as a verdict change).
func (m *memSource) add(events ...wsledger.Event) {
	m.gen++
	for _, e := range events {
		if i, ok := m.byID[e.ID]; ok {
			m.events[i] = e
		} else {
			m.byID[e.ID] = len(m.events)
			m.events = append(m.events, e)
		}
		m.gens[e.ID] = m.gen
	}
	m.c = newCausal(m.events)
}

// setVerdict changes a held event's protocol verdict.
func (m *memSource) setVerdict(id wsledger.EventID, v wsledger.Verdict, reason string) {
	e := m.events[m.byID[id]]
	e.Verdict, e.Reason = v, reason
	m.add(e)
}

const memEpoch = "mem"

func (m *memSource) Events(after wsledger.Cursor) ([]wsledger.Event, wsledger.Cursor) {
	var out []wsledger.Event
	for _, e := range m.events {
		if after.Epoch != memEpoch || m.gens[e.ID] > after.Gen {
			out = append(out, e)
		}
	}
	return out, wsledger.Cursor{Gen: m.gen, Epoch: memEpoch}
}

func (m *memSource) Concurrent(a, b wsledger.EventID) bool { return m.c.Concurrent(a, b) }

func (m *memSource) Identity() wsledger.IdentityState {
	return wsledger.IdentityState{Genesis: m.genesis}
}

// heads is the seen a write carries now: the highest resolved seq per
// other stream.
func (m *memSource) heads() map[string]int {
	out := map[string]int{}
	for id := range m.c.past {
		if id.Stream() != m.stream && id.Seq > out[id.Stream()] {
			out[id.Stream()] = id.Seq
		}
	}
	return out
}

func (m *memSource) Append(ctx context.Context, evs []wsledger.NewEvent) ([]wsledger.EventID, error) {
	if m.stream == "" {
		return nil, errors.New("memSource: no own stream")
	}
	member, clone := splitTestStream(m.stream)
	seq := 0
	for _, e := range m.events {
		if e.ID.Stream() == m.stream && e.ID.Broken == "" && e.ID.Seq > seq {
			seq = e.ID.Seq
		}
	}
	now := time.Now().UTC()
	if m.clock != nil {
		now = m.clock()
	}
	seen := m.heads()
	var out []wsledger.EventID
	var add []wsledger.Event
	for _, e := range evs {
		seq++
		id := wsledger.EventID{Member: member, Clone: clone, Seq: seq}
		payload := e.Payload
		if len(payload) == 0 {
			payload = json.RawMessage("{}")
		}
		add = append(add, wsledger.Event{ID: id, TS: now, Origin: e.Origin, Type: e.Type,
			Payload: payload, Seen: seen, Verdict: wsledger.Valid})
		out = append(out, id)
	}
	m.add(add...)
	return out, nil
}

func splitTestStream(s string) (string, string) {
	member, clone, _ := strings.Cut(s, ".")
	return member, clone
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
