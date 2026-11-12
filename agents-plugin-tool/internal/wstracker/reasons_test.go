package wstracker

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
)

// step is one event of a test history: [stream, type, payload, origin?,
// seen?]. An empty origin is none; seen, when given, replaces the event's
// seen (a write concurrent with what it leaves out).
type step []any

// history builds the events of a linear history: each event is its own
// commit and has seen every earlier event unless its step names a seen; ts
// advances a minute per event from 2026-11-01T00:00Z. All are
// protocol-Valid.
func history(steps []step) []wsledger.Event {
	heads := map[string]int{}
	t := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	var out []wsledger.Event
	for _, st := range steps {
		stream := st[0].(string)
		member, clone := splitTestStream(stream)
		seq := heads[stream] + 1
		seen := map[string]int{}
		for s, q := range heads {
			if s != stream {
				seen[s] = q
			}
		}
		payload, err := json.Marshal(st[2])
		if err != nil {
			panic(err)
		}
		t = t.Add(time.Minute)
		ev := wsledger.Event{ID: wsledger.EventID{Member: member, Clone: clone, Seq: seq}, TS: t, Type: st[1].(string),
			Payload: payload, Seen: seen, Verdict: wsledger.Valid}
		if len(st) > 3 {
			ev.Origin, _ = st[3].(string)
		}
		if len(st) > 4 {
			ev.Seen = map[string]int{}
			switch x := st[4].(type) {
			case map[string]int:
				ev.Seen = x
			case map[string]any:
				for s, q := range x {
					ev.Seen[s] = int(q.(float64))
				}
			}
		}
		heads[stream] = seq
		out = append(out, ev)
	}
	return out
}

type reasonCases struct {
	Genesis json.RawMessage `json:"genesis"`
	Base    []step          `json:"base"`
	Cases   []struct {
		Name   string `json:"name"`
		GoOnly bool   `json:"goOnly"`
		// EngineOnly cases use the roles.add_member stand-in, which Go does not port.
		EngineOnly bool `json:"engineOnly"`
		// Write cases are a write-time rule (checkWrite): the last step is
		// refused when dispatched and applied when replayed.
		Write  bool   `json:"write"`
		Steps  []step `json:"steps"`
		Reason string `json:"reason"`
	} `json:"cases"`
}

func loadReasonCases(t *testing.T) reasonCases {
	t.Helper()
	raw, err := os.ReadFile("testdata/reasons.json")
	if err != nil {
		t.Fatal(err)
	}
	var rc reasonCases
	if err := json.Unmarshal(raw, &rc); err != nil {
		t.Fatal(err)
	}
	return rc
}

// TestFlaggedReasons covers every flagged reason the domain words, in the
// reference engine's words (gen-golden.js checks the same cases against it).
// A write case's reason is the refusal of its last step dispatched on the
// history before it, and the replay applies that step.
func TestFlaggedReasons(t *testing.T) {
	rc := loadReasonCases(t)
	for _, c := range rc.Cases {
		if c.EngineOnly {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			evs := history(append(append([]step(nil), rc.Base...), c.Steps...))
			if c.Write {
				last := evs[len(evs)-1]
				trk := New(newMemSource(rc.Genesis, last.ID.Stream(), evs[:len(evs)-1]), last.ID.Stream())
				err := trk.validate([]wsledger.NewEvent{{Type: last.Type, Origin: last.Origin, Payload: last.Payload}})
				var ref *Refusal
				if !errors.As(err, &ref) || ref.Reason != c.Reason {
					t.Fatalf("dispatch: got %v, want the refusal %q", err, c.Reason)
				}
			}
			v := Derive(newMemSource(rc.Genesis, "ada.pc", evs), "ada.pc", time.Time{}, 0)
			byID := map[string]EventView{}
			for _, e := range v.Events {
				byID[e.ID] = e
			}
			for i, ev := range evs {
				e := byID[ev.ID.String()]
				if i < len(evs)-1 {
					if e.Status != "applied" {
						t.Fatalf("setup %s is %s: %v", e.ID, e.Status, deref(e.Reason))
					}
					continue
				}
				if c.Write {
					if e.Status != "applied" {
						t.Fatalf("replayed: got %s %q, want applied", e.Status, deref(e.Reason))
					}
				} else if e.Status != "flagged" || deref(e.Reason) != c.Reason {
					t.Fatalf("got %s %q, want flagged %q", e.Status, deref(e.Reason), c.Reason)
				}
			}
		})
	}
}

// TestAssignBelowTheWriterTakesAssigner pins what the write-time rule
// (checkWrite) lets through; its refusals are reasons.json write cases. On
// the reasons.json genesis ada leads maya, raj, and bo; maya leads lena and
// omar; raj leads sven. The base's maya.pc#2 gave lena, below maya, an
// assignment with completion absent (assignee) before the rule: it still
// folds that way, and lena wraps it up herself.
func TestAssignBelowTheWriterTakesAssigner(t *testing.T) {
	rc := loadReasonCases(t)
	evs := history(rc.Base)
	for _, c := range []struct {
		what, stream string
		payload      map[string]any
	}{
		{"below with completion assigner", "maya.pc", map[string]any{"item": "maya.pc#1", "targets": []any{"omar"}, "completion": "assigner"}},
		{"a peer with completion assignee", "maya.pc", map[string]any{"item": "maya.pc#1", "targets": []any{"raj"}, "completion": "assignee"}},
		{"above without completion", "maya.pc", map[string]any{"item": "maya.pc#1", "mode": "chosen", "people": []any{"ada"}}},
		{"the writer with completion assignee", "maya.pc", map[string]any{"item": "maya.pc#1", "targets": []any{"maya"}, "completion": "assignee"}},
		{"a rule rooted at a peer with completion assignee", "maya.pc", map[string]any{"item": "maya.pc#1", "targets": []any{map[string]any{"root": "raj"}}, "completion": "assignee"}},
		{"a topic to a member below", "ada.pc", map[string]any{"item": "ada.pc#3", "targets": []any{"raj"}, "completion": "assignee"}},
	} {
		raw, err := json.Marshal(c.payload)
		if err != nil {
			t.Fatal(err)
		}
		trk := New(newMemSource(rc.Genesis, c.stream, evs), c.stream)
		if err := trk.validate([]wsledger.NewEvent{{Type: TypeItemAssign, Payload: raw}}); err != nil {
			t.Errorf("%s: %v", c.what, err)
		}
	}

	v := stepsView(t, "maya.pc", append(append([]step(nil), rc.Base...), step{"lena.pc", "item.close", map[string]any{"item": "maya.pc#1"}}))
	wantEq(t, "the close applies", lastVerdict(v), "applied ")
	old := asg(t, v, "maya.pc#2>lena")
	wantEq(t, "the older record", old.Completion+" "+old.Status, "assignee done")
}
