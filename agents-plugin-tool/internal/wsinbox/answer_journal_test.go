package wsinbox

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAnswerJournalSurvivesCancelAndRetry(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	name, _, err := SendJournaled(dir, Entry{Chips: []Chip{{ID: "03", Answer: "a"}}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Cancel(dir, name); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("board unavailable")
	if err := ReplayAnswerJournal(dir, func(string, Entry) error { return failure }); !errors.Is(err, failure) {
		t.Fatalf("got %v", err)
	}
	calls := 0
	if err := ReplayAnswerJournal(dir, func(id string, e Entry) error {
		calls++
		if id != (Record{Name: name}).ID() || len(e.Chips) != 1 || e.SentAt != now.UnixMilli() {
			t.Fatalf("wrong journal: %s %+v", id, e)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := ReplayAnswerJournal(dir, func(string, Entry) error { t.Fatal("replayed twice"); return nil }); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls: %d", calls)
	}
}

func TestAnswerJournalSkipsMalformedRemnant(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := SendJournaled(dir, Entry{Parts: []string{"committed answer"}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "answer-pending", "000-invalid")
	if err := os.WriteFile(bad, []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	if err := ReplayAnswerJournal(dir, func(string, Entry) error { calls++; return nil }); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("malformed remnant starved committed answer: calls=%d", calls)
	}
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Fatalf("malformed remnant remains: %v", err)
	}
}

func TestAnswerJournalSurvivesSteer(t *testing.T) {
	dir := t.TempDir()
	name, _, err := SendJournaled(dir, Entry{Chips: []Chip{{ID: "03", Answer: "a"}}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Steer(dir, name); err != nil {
		t.Fatal(err)
	}
	if err := ReplayAnswerJournal(dir, func(id string, _ Entry) error {
		if id != (Record{Name: name}).ID() {
			t.Fatalf("changed ID: %s", id)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAnswerJournalDiscardsUncommittedStage(t *testing.T) {
	dir := t.TempDir()
	stage, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		t.Fatal(err)
	}
	stage.Close()
	journal := filepath.Join(dir, "answer-pending")
	if err := os.Mkdir(journal, 0700); err != nil {
		t.Fatal(err)
	}
	name := "20261117T000000.000000000Z-12345678.json--" + filepath.Base(stage.Name())
	if err := os.WriteFile(filepath.Join(journal, name), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ReplayAnswerJournal(dir, func(string, Entry) error { t.Fatal("uncommitted send replayed"); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stage.Name()); !os.IsNotExist(err) {
		t.Fatalf("staging file remains: %v", err)
	}
}
