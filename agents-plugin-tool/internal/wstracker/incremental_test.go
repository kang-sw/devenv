package wstracker

import (
	"encoding/json"
	"math/rand"
	"slices"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
)

func assertIncrementalFold(t *testing.T, tr *Tracker, src *memSource) {
	t.Helper()
	tr.Refresh()
	full := replay(src.events, src.genesis, src.Concurrent)
	ids := func(ns []*node) []string {
		out := make([]string, len(ns))
		for i, n := range ns {
			out[i] = n.id
		}
		return out
	}
	if !slices.Equal(ids(tr.r.order), ids(full.order)) {
		t.Fatal("incremental causal order differs from Kahn full replay")
	}
	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	got, err := json.Marshal(tr.r.view(src.stream, now, 0))
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(full.view(src.stream, now, 0))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("checkpoint fold differs from zero fold\ngot: %s\nwant: %s", got, want)
	}
	if len(tr.r.memo.checkpoints) > foldCheckpointLimit {
		t.Fatal("unbounded checkpoints")
	}
}

// Random arrival permutations create tail appends, mid-order insertions and
// filled dependency gaps, including timestamp priorities that are not causal
// order. The zero-fold oracle also checks annotations on PREFIX events.
func TestIncrementalFoldRandomArrivals(t *testing.T) {
	genesis, events := scaleStoreWith(1200, true)
	for seed := int64(0); seed < 8; seed++ {
		t.Run(itoa(int(seed)), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			src := newMemSource(genesis, "root.pc", nil)
			tr := New(src, src.stream)
			perm := rng.Perm(len(events))
			for len(perm) > 0 {
				n := min(len(perm), 5+rng.Intn(60))
				batch := make([]wsledger.Event, n)
				for i, idx := range perm[:n] {
					batch[i] = events[idx]
				}
				src.add(batch...)
				assertIncrementalFold(t, tr, src)
				perm = perm[n:]
			}
			for i := 0; i < 30; i++ {
				e := events[rng.Intn(len(events))]
				verdict := []wsledger.Verdict{wsledger.Valid, wsledger.Waiting, wsledger.Ignored}[rng.Intn(3)]
				src.setVerdict(e.ID, verdict, "changed protocol reason")
				assertIncrementalFold(t, tr, src)
			}
		})
	}
}

func TestIncrementalFoldCheckpointPrefixAndAnnotations(t *testing.T) {
	g, events := scaleStoreWith(1500, true)
	src := newMemSource(g, "root.pc", events[:1300])
	tr := New(src, src.stream)
	src.add(events[1300:]...)
	assertIncrementalFold(t, tr, src)
	if tr.r.memo.replayedFrom != 1300 {
		t.Fatalf("tail replay from %d, want terminal checkpoint 1300", tr.r.memo.replayedFrom)
	}
	// Moving an early verdict rechecks the suffix and removes any markings
	// it wrote back into an earlier register head.
	for _, idx := range []int{1200, 4, 800, 1499} {
		src.setVerdict(events[idx].ID, wsledger.Ignored, "revoked")
		assertIncrementalFold(t, tr, src)
		src.setVerdict(events[idx].ID, wsledger.Valid, "")
		assertIncrementalFold(t, tr, src)
	}
}

func TestIncrementalFoldConcurrentLabelsAndReason(t *testing.T) {
	g := json.RawMessage(`{"admins":["a","b"],"members":[{"id":"a","keys":["a"]},{"id":"b","keys":["b"]}]}`)
	mk := func(member string, seq int, typ, payload string, sec int64) wsledger.Event {
		return wsledger.Event{ID: wsledger.EventID{Member: member, Clone: "pc", Seq: seq}, TS: time.Unix(sec, 0), Type: typ, Payload: json.RawMessage(payload), Verdict: wsledger.Valid}
	}
	a := mk("a", 1, TypeAddLabel, `{"name":"same","color":"red"}`, 2)
	b := mk("b", 1, TypeAddLabel, `{"name":"same","color":"blue"}`, 1)
	src := newMemSource(g, "a.pc", []wsledger.Event{a})
	tr := New(src, src.stream)
	src.add(b)
	assertIncrementalFold(t, tr, src)
	src.setVerdict(b.ID, wsledger.Ignored, "first reason")
	assertIncrementalFold(t, tr, src)
	src.setVerdict(b.ID, wsledger.Ignored, "second reason")
	assertIncrementalFold(t, tr, src)
}
