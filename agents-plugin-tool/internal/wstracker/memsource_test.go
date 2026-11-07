package wstracker

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
)

// memSource is an in-memory Source: one clone's held events with their
// protocol verdicts, causality from each event's seen. Appended events get
// the next seqs of the clone's stream, the seen of everything held, and a
// Valid verdict (the domain pre-validates; the protocol rules beyond
// membership are not simulated here).
type memSource struct {
	genesis json.RawMessage
	stream  string
	events  []wsledger.Event
	clock   func() time.Time

	byID map[wsledger.EventID]int
	past map[wsledger.EventID]map[string]int // closed causal past of resolved events
}

func newMemSource(genesis json.RawMessage, stream string, events []wsledger.Event) *memSource {
	m := &memSource{genesis: genesis, stream: stream}
	m.events = append(m.events, events...)
	m.resolve()
	return m
}

func (m *memSource) resolve() {
	m.byID = map[wsledger.EventID]int{}
	m.past = map[wsledger.EventID]map[string]int{}
	streams := map[string]map[int]wsledger.Event{}
	for i, e := range m.events {
		m.byID[e.ID] = i
		if e.ID.Broken == "" {
			if streams[e.ID.Stream()] == nil {
				streams[e.ID.Stream()] = map[int]wsledger.Event{}
			}
			streams[e.ID.Stream()][e.ID.Seq] = e
		}
	}
	busy := map[wsledger.EventID]bool{}
	var visit func(e wsledger.Event) (map[string]int, bool)
	visit = func(e wsledger.Event) (map[string]int, bool) {
		if p, ok := m.past[e.ID]; ok {
			return p, true
		}
		if busy[e.ID] || e.ID.Broken != "" || e.Verdict == wsledger.Waiting {
			return nil, false
		}
		busy[e.ID] = true
		defer delete(busy, e.ID)
		past := map[string]int{}
		var preds []wsledger.Event
		s := e.ID.Stream()
		if e.ID.Seq > 1 {
			p, ok := streams[s][e.ID.Seq-1]
			if !ok {
				return nil, false
			}
			preds = append(preds, p)
		}
		for os, q := range e.Seen {
			if os == s || q <= 0 {
				continue
			}
			p, ok := streams[os][q]
			if !ok {
				return nil, false
			}
			preds = append(preds, p)
		}
		for _, p := range preds {
			pp, ok := visit(p)
			if !ok {
				return nil, false
			}
			for k, q := range pp {
				if past[k] < q {
					past[k] = q
				}
			}
			if past[p.ID.Stream()] < p.ID.Seq {
				past[p.ID.Stream()] = p.ID.Seq
			}
		}
		m.past[e.ID] = past
		return past, true
	}
	for _, e := range m.events {
		visit(e)
	}
}

func (m *memSource) Events(after wsledger.Cursor) ([]wsledger.Event, wsledger.Cursor) {
	return append([]wsledger.Event(nil), m.events...), wsledger.Cursor{}
}

func (m *memSource) hb(a, b wsledger.EventID) bool {
	pb := m.past[b]
	if a.Stream() == b.Stream() {
		return a.Seq < b.Seq
	}
	return pb[a.Stream()] >= a.Seq
}

func (m *memSource) Concurrent(a, b wsledger.EventID) bool {
	_, okA := m.past[a]
	_, okB := m.past[b]
	if !okA || !okB || a == b {
		return false
	}
	return !m.hb(a, b) && !m.hb(b, a)
}

func (m *memSource) Identity() wsledger.IdentityState {
	return wsledger.IdentityState{Genesis: m.genesis}
}

// heads is the seen a write carries now: the highest resolved seq per
// other stream.
func (m *memSource) heads() map[string]int {
	out := map[string]int{}
	for id := range m.past {
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
	for _, e := range evs {
		seq++
		id := wsledger.EventID{Member: member, Clone: clone, Seq: seq}
		m.events = append(m.events, wsledger.Event{ID: id, TS: now, Origin: e.Origin, Type: e.Type,
			Payload: e.Payload, Seen: seen, Verdict: wsledger.Valid})
		out = append(out, id)
	}
	m.resolve()
	return out, nil
}

func splitTestStream(s string) (string, string) {
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			return s[:i], s[i+1:]
		}
	}
	return s, ""
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
