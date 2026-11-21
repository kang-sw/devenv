// Tests for board db write.
package wswhiteboard

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBoardDBUpdateDeltaUnchangedAndTornTransaction(t *testing.T) {
	dir := t.TempDir()
	makeLegacyBoard(t, dir, boardDBFixture())
	var mu sync.Mutex
	var stages []string
	board := openTestBoardDB(t, dir, nil, BoardDBOptions{Observe: func(stage string) {
		mu.Lock()
		stages = append(stages, stage)
		mu.Unlock()
	}})
	walBefore := folderRead(t, filepath.Join(dir, boardDBName+"-wal"))
	if err := board.Update(func(*State) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if walAfter := folderRead(t, filepath.Join(dir, boardDBName+"-wal")); !bytes.Equal(walBefore, walAfter) {
		t.Fatal("unchanged update wrote WAL frames")
	}
	mu.Lock()
	gotStages := append([]string(nil), stages...)
	mu.Unlock()
	if slicesContains(gotStages, "transaction-begin") {
		t.Fatalf("unchanged update opened transaction: %v", gotStages)
	}
	if err := board.Update(func(state *State) error {
		state.EntrySeq++
		state.Topics[0].Entries = append(state.Topics[0].Entries, Entry{ID: fmt.Sprintf("e%d", state.EntrySeq), Kind: EntryAgent, Text: "delta", At: "2026-11-19T01:03:00Z"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := board.Close(); err != nil {
		t.Fatal(err)
	}
	var crashed = errors.New("before commit")
	reopened := openTestBoardDB(t, dir, nil, BoardDBOptions{Fault: func(stage string) error {
		if stage == "transaction-before-commit" {
			return crashed
		}
		return nil
	}})
	before := reopened.State()
	if err := reopened.Update(func(state *State) error { state.Seq += 100; return nil }); !errors.Is(err, crashed) {
		t.Fatalf("fault = %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	last := openTestBoardDB(t, dir, nil, BoardDBOptions{})
	if got := last.State(); !reflect.DeepEqual(got, before) {
		t.Fatalf("torn transaction escaped\nwant %s\ngot  %s", folderJSON(before), folderJSON(got))
	}
}

func TestBoardDBRejectsEntryRewriteOrRemoval(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*State)
	}{
		{name: "rewrite", mutate: func(state *State) { state.Topics[0].Entries[0].Text = "changed" }},
		{name: "remove", mutate: func(state *State) { state.Topics[0].Entries = state.Topics[0].Entries[1:] }},
		{name: "move", mutate: func(state *State) {
			moved := state.Topics[0].Entries[1]
			state.Topics[0].Entries = state.Topics[0].Entries[:1]
			state.Topics = append(state.Topics, TopicDoc{Name: "other", Entries: []Entry{moved}})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			makeLegacyBoard(t, dir, boardDBFixture())
			board := openTestBoardDB(t, dir, nil, BoardDBOptions{})
			if err := board.Update(func(state *State) error { tc.mutate(state); return nil }); err == nil || !strings.Contains(err.Error(), "append-only") {
				t.Fatalf("mutation accepted: %v", err)
			}
		})
	}
}

func TestBoardDBAdoptionPrefersHistoryDirectoryAndReceiptsDoNotRegress(t *testing.T) {
	dir := t.TempDir()
	fixture := boardDBFixture()
	fixture.ClosedTopics = []string{"destination"}
	makeLegacyBoard(t, dir, fixture)
	board := openTestBoardDB(t, dir, nil, BoardDBOptions{})
	sourceDir := board.topicDirs["work"]
	staleDir := board.topicDirs["destination"]
	if sourceDir == "" || staleDir == "" || sourceDir == staleDir {
		t.Fatalf("fixture mappings = %#v", board.topicDirs)
	}
	if err := board.Update(func(state *State) error {
		state.Topics[0].Name = "destination"
		state.Items[0].Topic = "destination"
		state.ClosedTopics = nil
		delete(state.Labels, "work")
		state.Labels["destination"] = "Work"
		state.MetadataRevision = 8
		state.StaleNudgeRevision = "receipt-8"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := board.topicDirs["destination"]; got != sourceDir {
		t.Fatalf("adoption used %q, want history dir %q (stale %q)", got, sourceDir, staleDir)
	}
	if err := board.Update(func(state *State) error {
		state.MetadataRevision = 1
		state.StaleNudgeRevision = ""
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	got := board.State()
	if got.MetadataRevision != 8 || got.StaleNudgeRevision != "receipt-8" {
		t.Fatalf("receipt regressed: %s", folderJSON(got))
	}
	if err := board.Update(func(state *State) error { state.Seq++; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := board.Close(); err != nil {
		t.Fatal(err)
	}
	board = openTestBoardDB(t, dir, nil, BoardDBOptions{})
	if err := board.Update(func(state *State) error {
		state.MetadataRevision = 9
		state.StaleNudgeRevision = "receipt-9"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := board.db.QueryRow(`SELECT count(*) FROM revision_receipts WHERE metadata_revision IN (8,9)`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("receipts did not alternate across ordinary update/reopen: count=%d, %v", count, err)
	}
}

func TestBoardDBLoadFloorNeverRegresses(t *testing.T) {
	originalNow := archiveLoadNow
	archiveLoadNow = func() time.Time { return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { archiveLoadNow = originalNow })
	dir := t.TempDir()
	fixture := boardDBFixture()
	fixture.LoadedAt = "2026-11-21T12:00:00.000Z"
	makeLegacyBoard(t, dir, fixture)
	board := openTestBoardDB(t, dir, nil, BoardDBOptions{})

	loadDir := t.TempDir()
	makeLegacyBoard(t, loadDir, boardDBFixture())
	archive, err := (FolderStore{Dir: loadDir}).ExportArchive()
	if err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(t.TempDir(), "board.zip")
	if err := os.WriteFile(zipPath, archive, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := board.LoadArchive(zipPath, time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if got := board.State().LoadedAt; got != "2026-11-21T12:00:00.001Z" {
		t.Fatalf("LoadedAt regressed: %q", got)
	}
}
