package wstracker

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
)

// step is one event of a test history: [stream, type, payload, origin?].
type step []any

// history builds the events of a linear history: each event is its own
// commit and has seen every earlier event; ts advances a minute per event
// from 2026-11-01T00:00Z. All are protocol-Valid.
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
			ev.Origin = st[3].(string)
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
func TestFlaggedReasons(t *testing.T) {
	rc := loadReasonCases(t)
	for _, c := range rc.Cases {
		t.Run(c.Name, func(t *testing.T) {
			evs := history(append(append([]step(nil), rc.Base...), c.Steps...))
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
				if e.Status != "flagged" || deref(e.Reason) != c.Reason {
					t.Fatalf("got %s %q, want flagged %q", e.Status, deref(e.Reason), c.Reason)
				}
			}
		})
	}
}
