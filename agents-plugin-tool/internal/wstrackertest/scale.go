// Package wstrackertest supplies opt-in benchmark fixtures. Production code
// must not import it. Nothing is generated at package initialization.
package wstrackertest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
)

const (
	MillionEvents = 1_000_000
	Stream        = "root.pc"
	Tail          = "tail-append"
	NearTail      = "near-tail-remote"
	FarBack       = "far-back-remote"
)

// Scale has exactly EventCount events: ItemCount creates followed by status
// writes round-robin over the items. There are no moves, assigns, role changes
// or live rules that would make stateBefore read a store-wide quadratic past.
// Each task's implicit fallback holder is root. Repeated writes still exercise
// real per-thread stateBefore checks, register writes and checkpoint copies.
// Payloads are shared per item/status, not allocated once per million writes.
// Events and their payloads are immutable; each Source owns only its arrival.
type Scale struct {
	Events    []wsledger.Event
	Genesis   json.RawMessage
	ItemCount int
	Now       time.Time
}

func NewScale(items, eventCount int) *Scale {
	if items < 1 || eventCount < items+1024 {
		panic("scale fixture needs items and at least 1024 status writes")
	}
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	s := &Scale{
		Events: make([]wsledger.Event, eventCount), ItemCount: items,
		Genesis: json.RawMessage(`{"admins":["root"],"members":[{"id":"root","name":"Root","keys":["root-key"],"leads":[]}]}`),
		Now:     start.Add(time.Duration(eventCount+60) * time.Second),
	}
	body := strings.Repeat("A **benchmark** body with `code` and a short paragraph.\n\n", 8)
	statuses := []string{"todo", "doing", "blocked"}
	writes := make([][3]json.RawMessage, items)
	for i := range items {
		id := wsledger.EventID{Member: "root", Clone: "pc", Seq: i + 1}
		s.Events[i] = wsledger.Event{ID: id, TS: start.Add(time.Duration(i+1) * time.Second),
			Type: "item.create", Verdict: wsledger.Valid,
			Payload: mustJSON(struct {
				Kind  string `json:"kind"`
				Title string `json:"title"`
				Body  string `json:"body"`
			}{"task", fmt.Sprintf("Benchmark item %d", i+1), body})}
		for j, status := range statuses {
			writes[i][j] = mustJSON(struct {
				Item   string `json:"item"`
				Status string `json:"status"`
				Note   string `json:"note"`
			}{id.String(), status, "distributed status write"})
		}
	}
	for i := items; i < eventCount; i++ {
		item, round := (i-items)%items, (i-items)/items
		s.Events[i] = wsledger.Event{
			ID: wsledger.EventID{Member: "root", Clone: "pc", Seq: i + 1},
			TS: start.Add(time.Duration(i+1) * time.Second), Type: "item.status",
			Payload: writes[item][round%len(statuses)], Verdict: wsledger.Valid,
		}
	}
	return s
}

func mustJSON(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}

// Change targets the very first create. Remote seq 1 explicitly sees the
// local prefix ending at insertion, so it sees its target create but remains
// concurrent with the local suffix. Half-second timestamps place it BETWEEN
// existing events (not just in arrival order). FarBack inserts after create 1;
// NearTail inserts 512 events before the end. A far-back status may lose to a
// later local status, but its thread/detail version must still change.
func (s *Scale) Change(kind string) wsledger.Event {
	seq := len(s.Events)
	ev := wsledger.Event{ID: wsledger.EventID{Member: "root", Clone: "pc", Seq: seq + 1},
		TS: s.Events[seq-1].TS.Add(time.Second), Type: "item.status", Verdict: wsledger.Valid,
		Payload: json.RawMessage(`{"item":"root.pc#1","status":"doing","note":"one arriving change"}`)}
	switch kind {
	case Tail:
		return ev
	case NearTail:
		seq -= 512
	case FarBack:
		seq = 1
	default:
		panic("unknown scale change: " + kind)
	}
	ev.ID.Clone, ev.ID.Seq = "remote", 1
	ev.Seen = map[string]int{Stream: seq}
	ev.TS = s.Events[seq-1].TS.Add(time.Second / 2)
	return ev
}

// Source is deliberately restricted to this fixture: an immutable local
// prefix and at most one new event. Cursor reads of an unchanged source and
// of a single arrival are O(1); a zero-cursor read after arrival allocates a
// combined slice (included in the old full-read benchmark). No newCausal or
// million-entry map is rebuilt on arrival. This models fold cost, not ledger
// verification, journal I/O, gap filling or protocol-verdict changes.
type Source struct {
	scale   *Scale
	arrival []wsledger.Event
}

func (s *Scale) Source() *Source { return &Source{scale: s} }

func (s *Source) Arrive(ev wsledger.Event) {
	if len(s.arrival) != 0 {
		panic("scale source allows exactly one arrival")
	}
	s.arrival = []wsledger.Event{ev}
}

func (s *Source) Events(after wsledger.Cursor) ([]wsledger.Event, wsledger.Cursor) {
	cur := wsledger.Cursor{Epoch: "scale", Gen: uint64(1 + len(s.arrival))}
	if after.Epoch != cur.Epoch || after.Gen > cur.Gen || after.Gen == 0 {
		if len(s.arrival) == 0 {
			return s.scale.Events, cur
		}
		all := make([]wsledger.Event, len(s.scale.Events)+1)
		copy(all, s.scale.Events)
		all[len(s.scale.Events)] = s.arrival[0]
		return all, cur
	}
	if after.Gen < cur.Gen {
		return s.arrival, cur
	}
	return nil, cur
}

func (s *Source) Identity() wsledger.IdentityState {
	return wsledger.IdentityState{Genesis: s.scale.Genesis}
}

func (s *Source) Append(context.Context, []wsledger.NewEvent) ([]wsledger.EventID, error) {
	return nil, errors.New("scale source: use Arrive, not Dispatch")
}

// Concurrent is the exact transitive relation for the generated two-stream
// graph: all local events form a chain, remote#1 sees a local prefix, and no
// local event sees remote#1. Unknown events are unresolved, hence concurrent
// with nothing, matching the Source contract. Same-member remote clones avoid
// needing membership/role setup just to exercise a mid-order arrival.
func (s *Source) Concurrent(a, b wsledger.EventID) bool {
	localMax := len(s.scale.Events)
	remoteSeen := 0
	if len(s.arrival) != 0 {
		ev := s.arrival[0]
		if ev.ID.Clone == "pc" {
			localMax++
		} else {
			remoteSeen = ev.Seen[Stream]
		}
	}
	known := func(id wsledger.EventID) bool {
		return id.Broken == "" && id.Member == "root" &&
			((id.Clone == "pc" && id.Seq > 0 && id.Seq <= localMax) ||
				(id.Clone == "remote" && id.Seq == 1 && remoteSeen > 0))
	}
	if a == b || !known(a) || !known(b) || a.Clone == b.Clone {
		return false
	}
	if a.Clone == "pc" {
		return a.Seq > remoteSeen
	}
	return b.Seq > remoteSeen
}
