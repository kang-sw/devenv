package wslive

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func summaryTurnFixture() RingEntry {
	s := summaryFixture()
	return RingEntry{Kind: RingTurn, Origin: OriginTerminal, DurationMs: 73, Done: 2, Failed: 1, Reason: "error", SummaryHistory: &SummaryHistory{V: 1, Commit: SummaryCommit("reply", "producer/1"), Requests: s.Requests}}
}

func TestSummaryCommitFreezesRetriesAndNeverRecounts(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ring")
	store := &SummaryCommits{closed: map[string]bool{}}
	fail := true
	var name string
	var raw []byte
	store.write = func(dir, n string, data []byte) error {
		if name == "" {
			name = n
			raw = append([]byte(nil), data...)
		} else if name != n || !bytes.Equal(raw, data) {
			t.Fatal("retry changed frozen candidate")
		}
		if fail {
			return errors.New("disk unavailable")
		}
		return writeFrozenRing(dir, n, data)
	}
	entry := summaryTurnFixture()
	if err := store.Register(entry); err != nil {
		t.Fatal(err)
	}
	entry.SummaryHistory.Requests[0].Parts[0].Text = "caller mutation"
	if n, err := store.Retry(dir); err == nil || n != 0 {
		t.Fatalf("failed append: %d %v", n, err)
	}
	if store.Available() == nil {
		t.Fatal("persistent storage failure not surfaced")
	}
	if names, _ := RingNames(dir); len(names) != 0 {
		t.Fatal("saved before rename")
	}
	fail = false
	if n, err := store.Retry(dir); err != nil || n != 1 {
		t.Fatalf("retry: %d %v", n, err)
	}
	if err := store.Register(summaryTurnFixture()); err != nil {
		t.Fatal(err)
	}
	if n, err := store.Retry(dir); err != nil || n != 0 {
		t.Fatalf("duplicate: %d %v", n, err)
	}
	page, err := ReadRing(dir)
	if err != nil || len(page) != 1 {
		t.Fatalf("ring: %v %v", page, err)
	}
	got := page[0]
	if got.Done != 2 || got.Failed != 1 || got.DurationMs != 73 || got.SummaryHistory.Requests[0].Parts[0].Text != "é" {
		t.Fatalf("candidate recounted/mutated: %+v", got)
	}
	// Pruning/absence never grants a previously accepted commit permission.
	if err := os.Remove(filepath.Join(dir, ringSegmentName(1))); err != nil {
		t.Fatal(err)
	}
	if err := store.Register(summaryTurnFixture()); err != nil {
		t.Fatal(err)
	}
	if n, _ := store.Retry(dir); n != 0 {
		t.Fatal("resurrected pruned turn")
	}
}

func TestSummaryCommitCapacityNeverEvictsAcceptedCandidate(t *testing.T) {
	store := &SummaryCommits{closed: map[string]bool{}, write: func(string, string, []byte) error { return errors.New("full") }}
	for i := 0; i < summaryPendingCap; i++ {
		e := summaryTurnFixture()
		e.SummaryHistory.Commit = SummaryCommit("reply", string(rune(i+1)))
		if err := store.Register(e); err != nil {
			t.Fatal(err)
		}
	}
	if store.Available() == nil {
		t.Fatal("capacity silently accepted")
	}
	if err := store.Register(summaryTurnFixture()); err == nil {
		t.Fatal("overflow accepted")
	}
	if len(store.pending) != summaryPendingCap {
		t.Fatal("accepted pending candidate evicted")
	}
}

func TestSummaryImmutableCommitRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	name := "20261114T000000.000000000Z-abcd.json"
	if err := writeFrozenRing(dir, name, []byte(`{"kind":"turn"}`)); err != nil {
		t.Fatal(err)
	}
	if err := writeFrozenRing(dir, name, []byte(`{"kind":"turn","done":1}`)); err == nil {
		t.Fatal("overwrote immutable prior bytes")
	}
}
