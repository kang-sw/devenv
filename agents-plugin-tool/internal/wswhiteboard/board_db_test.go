package wswhiteboard

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func boardDBFixture() *State {
	proof := &BodyTimeProof{Version: 1, At: "2026-11-19T01:02:03Z", Changed: 4, Digest: "opaque-fixture-proof"}
	return &State{
		Counter: 4, EntrySeq: 2, PanelCounter: 1, Seq: 5,
		Topics: []TopicDoc{{
			Name: "work", Body: "body", Changed: 4, ChangedAt: "2026-11-19T01:02:03Z", BodyTimeProof: proof,
			Entries: []Entry{
				{ID: "e1", Kind: EntryPanel, PanelID: "p1", PanelName: "status", Recap: "first", Text: "<b>panel</b>", At: "2026-11-19T01:02:03Z"},
				{ID: "e2", Kind: EntryAgent, Text: "log", At: "2026-11-19T01:02:04Z"},
			},
		}},
		Items:  []Item{{ID: 4, Topic: "work", Class: ClassCheck, Status: StatusOpen, Title: "verify"}},
		Labels: map[string]string{"work": "Work"},
	}
}

func makeLegacyBoard(t *testing.T, dir string, state *State) *State {
	t.Helper()
	got, err := (FolderStore{Dir: dir}).Open(state)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func openTestBoardDB(t *testing.T, dir string, legacy *State, opts BoardDBOptions) *BoardDB {
	t.Helper()
	board, err := OpenBoardDB(dir, legacy, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = board.Close() })
	return board
}

func TestBoardDBImportPendingReplayRoundTripAndSentinel(t *testing.T) {
	dir := t.TempDir()
	fixture := boardDBFixture()
	fixture.MetadataRevision = 9
	fixture.StaleNudgeRevision = "nudge-9"
	want := makeLegacyBoard(t, dir, fixture)
	loadedAt := "2026-11-19T01:02:07.001Z"
	loadFloorPath := filepath.Join(dir, filepath.FromSlash(folderLoadFloorLog))
	if err := os.MkdirAll(filepath.Dir(loadFloorPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(loadFloorPath, folderJSON(folderLoadFloor{Generation: 1, LoadedAt: loadedAt}), 0o644); err != nil {
		t.Fatal(err)
	}
	want.LoadedAt = loadedAt
	crash := FolderStore{Dir: dir, Fault: func(stage string) error {
		if stage == "intent" {
			return folderCrash
		}
		return nil
	}}
	if err := crash.Update(nil, func(state *State) error {
		state.EntrySeq++
		state.Seq++
		state.Topics[0].Entries = append(state.Topics[0].Entries, Entry{ID: fmt.Sprintf("e%d", state.EntrySeq), Kind: EntryAgent, Text: "pending", At: "2026-11-19T01:02:05Z"})
		return nil
	}); !errors.Is(err, folderCrash) {
		t.Fatalf("pending fixture: %v", err)
	}
	want, err := (FolderStore{Dir: dir}).Open(nil)
	if err != nil {
		t.Fatal(err)
	}
	// Recreate the pending fixture: the import itself must perform replay.
	if err := crash.Update(nil, func(state *State) error {
		state.EntrySeq++
		state.Seq++
		state.Topics[0].Entries = append(state.Topics[0].Entries, Entry{ID: fmt.Sprintf("e%d", state.EntrySeq), Kind: EntryAgent, Text: "pending again", At: "2026-11-19T01:02:06Z"})
		return nil
	}); !errors.Is(err, folderCrash) {
		t.Fatalf("second pending fixture: %v", err)
	}
	want.EntrySeq++
	want.Seq++
	want.Topics[0].Entries = append(want.Topics[0].Entries, Entry{ID: fmt.Sprintf("e%d", want.EntrySeq), Kind: EntryAgent, Text: "pending again", At: "2026-11-19T01:02:06Z"})

	board := openTestBoardDB(t, dir, nil, BoardDBOptions{})
	if got := board.State(); !reflect.DeepEqual(got, want) {
		t.Fatalf("import mismatch\nwant %s\ngot  %s", folderJSON(want), folderJSON(got))
	}
	if has, err := boardHasSentinel(dir); err != nil || !has {
		t.Fatalf("sentinel = %v, %v", has, err)
	}
	if _, err := (FolderStore{Dir: dir}).Open(nil); err == nil || !strings.Contains(err.Error(), "newer schema_version 3") {
		t.Fatalf("old folder reader did not refuse sentinel: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, boardDBName+"-shm")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("exclusive board DB created shm: %v", err)
	}
	if err := board.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestBoardDB(t, dir, nil, BoardDBOptions{})
	if got := reopened.State(); !reflect.DeepEqual(got, want) {
		t.Fatalf("reopen mismatch\nwant %s\ngot  %s", folderJSON(want), folderJSON(got))
	}
}

func TestBoardDBImportsLegacyV1Conversion(t *testing.T) {
	dir := t.TempDir()
	legacy := &State{Counter: 1, Seq: 1, Topics: []TopicDoc{{Name: "legacy", Recap: "old recap", RecapAt: "2026-11-01T00:00:00Z"}}}
	board := openTestBoardDB(t, dir, legacy, BoardDBOptions{})
	got := board.State()
	if got.EntrySeq != 1 || len(got.Topics) != 1 || len(got.Topics[0].Entries) != 1 || got.Topics[0].Entries[0].Text != "old recap" || !got.Topics[0].Entries[0].Imported {
		t.Fatalf("legacy conversion = %s", folderJSON(got))
	}
}

func TestBoardDBImportHoldsLegacyLockThroughSentinel(t *testing.T) {
	dir := t.TempDir()
	makeLegacyBoard(t, dir, boardDBFixture())
	originalAttempt := folderLockAttempt
	attempted := make(chan struct{}, 1)
	var attemptMu sync.Mutex
	attemptCount := 0
	folderLockAttempt = func(path string) {
		originalAttempt(path)
		if path == dir {
			attemptMu.Lock()
			attemptCount++
			count := attemptCount
			attemptMu.Unlock()
			if count == 1 { // the importer itself
				return
			}
			select {
			case attempted <- struct{}{}:
			default:
			}
		}
	}
	t.Cleanup(func() { folderLockAttempt = originalAttempt })
	writerDone := make(chan error, 1)
	startedWriter := false
	board, err := OpenBoardDB(dir, nil, BoardDBOptions{Observe: func(stage string) {
		if stage != "import-committed" || startedWriter {
			return
		}
		startedWriter = true
		go func() {
			writerDone <- (FolderStore{Dir: dir}).Update(nil, func(state *State) error {
				state.Seq++
				return nil
			})
		}()
		select {
		case <-attempted:
		case <-time.After(2 * time.Second):
			t.Error("legacy writer did not reach the held lock")
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = board.Close() })
	select {
	case err := <-writerDone:
		if err == nil || !strings.Contains(err.Error(), "newer schema_version 3") {
			t.Fatalf("legacy writer result = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("legacy writer stayed blocked after import")
	}
}

func TestBoardDBSentinellessOpenWaitsForLegacyWriter(t *testing.T) {
	dir := t.TempDir()
	makeLegacyBoard(t, dir, boardDBFixture())
	board := openTestBoardDB(t, dir, nil, BoardDBOptions{Fault: func(stage string) error {
		if stage == "sentinel-write" {
			return folderCrash
		}
		return nil
	}})
	want := board.State()
	if err := board.Close(); err != nil {
		t.Fatal(err)
	}
	originalAttempt := folderLockAttempt
	attempted := make(chan struct{}, 4)
	folderLockAttempt = func(path string) {
		originalAttempt(path)
		if path == dir {
			attempted <- struct{}{}
		}
	}
	t.Cleanup(func() { folderLockAttempt = originalAttempt })
	holding := make(chan struct{})
	release := make(chan struct{})
	writerDone := make(chan error, 1)
	go func() {
		writerDone <- (FolderStore{Dir: dir}).Update(nil, func(state *State) error {
			close(holding)
			<-release
			state.Seq++
			return nil
		})
	}()
	select {
	case <-holding:
	case <-time.After(2 * time.Second):
		t.Fatal("legacy writer did not acquire lock")
	}
	<-attempted // writer's attempt
	openDone := make(chan struct {
		board *BoardDB
		err   error
	}, 1)
	var logs []string
	go func() {
		next, err := OpenBoardDB(dir, nil, BoardDBOptions{Logf: func(format string, args ...any) {
			logs = append(logs, fmt.Sprintf(format, args...))
		}})
		openDone <- struct {
			board *BoardDB
			err   error
		}{next, err}
	}()
	select {
	case <-attempted:
	case <-time.After(2 * time.Second):
		t.Fatal("sentinel retry did not reach legacy lock")
	}
	select {
	case result := <-openDone:
		if result.board != nil {
			_ = result.board.Close()
		}
		t.Fatalf("sentinel retry bypassed held lock: %v", result.err)
	default:
	}
	close(release)
	if err := <-writerDone; err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-openDone:
		if result.err != nil {
			t.Fatal(result.err)
		}
		defer result.board.Close()
		if !reflect.DeepEqual(result.board.State(), want) {
			t.Fatal("sentinel retry reimported the intervening legacy commit")
		}
		joined := strings.Join(logs, "\n")
		for _, text := range []string{dir, "imported generation 1", "legacy committed generation 2"} {
			if !strings.Contains(joined, text) {
				t.Fatalf("waited writer's commit was not logged: %q", joined)
			}
		}
		if has, err := boardHasSentinel(dir); err != nil || !has {
			t.Fatalf("sentinel = %v, %v", has, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sentinel retry stayed blocked")
	}
}

func TestBoardDBSentinelFailureOpensAndRetries(t *testing.T) {
	dir := t.TempDir()
	makeLegacyBoard(t, dir, boardDBFixture())
	var logs []string
	board := openTestBoardDB(t, dir, nil, BoardDBOptions{
		Fault: func(stage string) error {
			if stage == "sentinel-write" {
				return errors.New("sentinel refused")
			}
			return nil
		},
		Logf: func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) },
	})
	if board.State() == nil || len(logs) != 1 || !strings.Contains(logs[0], "sentinel refused") {
		t.Fatalf("open/log = %v, %q", board.State() != nil, logs)
	}
	if has, err := boardHasSentinel(dir); err != nil || has {
		t.Fatalf("failed sentinel unexpectedly present: %v, %v", has, err)
	}
	if err := board.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestBoardDB(t, dir, nil, BoardDBOptions{})
	if reopened.State() == nil {
		t.Fatal("retry lost DB state")
	}
	if has, err := boardHasSentinel(dir); err != nil || !has {
		t.Fatalf("sentinel retry = %v, %v", has, err)
	}
}

func TestBoardDBSentinellessDivergenceKeepsDB(t *testing.T) {
	dir := t.TempDir()
	imported := makeLegacyBoard(t, dir, boardDBFixture())
	board := openTestBoardDB(t, dir, nil, BoardDBOptions{Fault: func(stage string) error {
		if stage == "sentinel-write" {
			return folderCrash
		}
		return nil
	}})
	if err := board.Close(); err != nil {
		t.Fatal(err)
	}
	if err := (FolderStore{Dir: dir}).Update(nil, func(state *State) error {
		state.Seq += 10
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var logs []string
	reopened := openTestBoardDB(t, dir, nil, BoardDBOptions{Logf: func(format string, args ...any) {
		logs = append(logs, fmt.Sprintf(format, args...))
	}})
	if got := reopened.State(); !reflect.DeepEqual(got, imported) {
		t.Fatalf("divergence reimported legacy\nwant %s\ngot  %s", folderJSON(imported), folderJSON(got))
	}
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "imported generation 1") || !strings.Contains(joined, "legacy committed generation 2") {
		t.Fatalf("missing generation divergence log: %q", joined)
	}
}

func TestBoardDBSentinellessPendingIntentLoggedAndOverwritten(t *testing.T) {
	dir := t.TempDir()
	imported := makeLegacyBoard(t, dir, boardDBFixture())
	board := openTestBoardDB(t, dir, nil, BoardDBOptions{Fault: func(stage string) error {
		if stage == "sentinel-write" {
			return folderCrash
		}
		return nil
	}})
	if err := board.Close(); err != nil {
		t.Fatal(err)
	}
	crash := FolderStore{Dir: dir, Fault: func(stage string) error {
		if stage == "intent" {
			return folderCrash
		}
		return nil
	}}
	if err := crash.Update(nil, func(state *State) error { state.Seq += 10; return nil }); !errors.Is(err, folderCrash) {
		t.Fatal(err)
	}
	var logs []string
	reopened := openTestBoardDB(t, dir, nil, BoardDBOptions{Logf: func(format string, args ...any) {
		logs = append(logs, fmt.Sprintf(format, args...))
	}})
	if got := reopened.State(); !reflect.DeepEqual(got, imported) {
		t.Fatal("pending legacy intent was replayed into DB")
	}
	if joined := strings.Join(logs, "\n"); !strings.Contains(joined, "pending intent generation 2 will be overwritten") {
		t.Fatalf("missing pending-intent log: %q", joined)
	}
	if has, err := boardHasSentinel(dir); err != nil || !has {
		t.Fatalf("sentinel = %v, %v", has, err)
	}
}

func TestBoardDBMissingOrCorruptNeverReimports(t *testing.T) {
	for _, tc := range []struct {
		name      string
		sentinel  bool
		corrupt   bool
		directory bool
	}{
		{name: "missing-under-sentinel", sentinel: true},
		{name: "corrupt-under-sentinel", sentinel: true, corrupt: true},
		{name: "corrupt-without-sentinel", corrupt: true},
		{name: "unreadable-without-sentinel", directory: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			makeLegacyBoard(t, dir, boardDBFixture())
			if tc.sentinel {
				if err := (FolderStore{Dir: dir}).atomic("pending.json", folderJSON(boardSentinel{SchemaVersion: boardSentinelSchemaVersion}), "test"); err != nil {
					t.Fatal(err)
				}
			}
			if tc.corrupt {
				if err := os.WriteFile(filepath.Join(dir, boardDBName), []byte("not sqlite"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tc.directory {
				if err := os.Mkdir(filepath.Join(dir, boardDBName), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			before := folderTree(t, dir)
			_, err := OpenBoardDB(dir, nil, BoardDBOptions{})
			if err == nil || !strings.Contains(err.Error(), "manual rollback: delete pending.json and .board.sqlite") {
				t.Fatalf("error = %v", err)
			}
			if got := folderTree(t, dir); !reflect.DeepEqual(got, before) {
				t.Fatal("failed open changed legacy folder")
			}
		})
	}
}

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

func TestBoardDBImportCommitWithoutSentinelRecovers(t *testing.T) {
	dir := t.TempDir()
	want := makeLegacyBoard(t, dir, boardDBFixture())
	_, err := OpenBoardDB(dir, nil, BoardDBOptions{Fault: func(stage string) error {
		if stage == "import-committed" {
			return folderCrash
		}
		return nil
	}})
	if !errors.Is(err, folderCrash) {
		t.Fatalf("fault = %v", err)
	}
	if has, err := boardHasSentinel(dir); err != nil || has {
		t.Fatalf("sentinel after kill = %v, %v", has, err)
	}
	recovered := openTestBoardDB(t, dir, nil, BoardDBOptions{})
	if got := recovered.State(); !reflect.DeepEqual(got, want) {
		t.Fatalf("recovered state mismatch\nwant %s\ngot %s", folderJSON(want), folderJSON(got))
	}
}

func TestBoardDBLegacyArchiveExportBackupAndLoad(t *testing.T) {
	originalNow := archiveLoadNow
	archiveLoadNow = func() time.Time { return time.Date(2026, 11, 20, 12, 1, 0, 0, time.UTC) }
	t.Cleanup(func() { archiveLoadNow = originalNow })
	sourceDir := t.TempDir()
	want := makeLegacyBoard(t, sourceDir, boardDBFixture())
	for revision := uint64(5); revision <= 6; revision++ {
		if err := (FolderStore{Dir: sourceDir}).Update(nil, func(state *State) error {
			state.MetadataRevision = revision
			state.StaleNudgeRevision = fmt.Sprintf("receipt-%d", revision)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	loadFloorPath := filepath.Join(sourceDir, filepath.FromSlash(folderLoadFloorLog))
	if err := os.WriteFile(loadFloorPath, append(folderJSON(folderLoadFloor{Generation: 2, LoadedAt: "2026-11-19T01:00:00.001Z"}), folderJSON(folderLoadFloor{Generation: 3, LoadedAt: "2026-11-19T01:00:00.002Z"})...), 0o644); err != nil {
		t.Fatal(err)
	}
	reopenedSource, err := (FolderStore{Dir: sourceDir}).Open(nil)
	if err != nil {
		t.Fatal(err)
	}
	want = reopenedSource
	board := openTestBoardDB(t, sourceDir, nil, BoardDBOptions{})
	exported, err := board.ExportArchive()
	if err != nil {
		t.Fatal(err)
	}
	assertBoardDBArchiveState(t, exported, board.State())
	if err := board.Update(func(state *State) error {
		state.Items[0].Status = StatusAnswered
		state.Items[0].Answer = "yes"
		state.Items[0].AnswerQuote = "owner's answer"
		state.Items[0].AnsweredAt = "2026-11-20T00:00:00Z"
		state.MetadataRevision++
		state.StaleNudgeRevision = "updated-receipt"
		return folderAdd(state)
	}); err != nil {
		t.Fatal(err)
	}
	want = board.State()
	exported, err = board.ExportArchive()
	if err != nil {
		t.Fatal(err)
	}
	assertBoardDBArchiveState(t, exported, want)

	loadState := boardDBFixture()
	loadState.Seq = 1
	loadDir := t.TempDir()
	loadState = makeLegacyBoard(t, loadDir, loadState)
	loadZip, err := (FolderStore{Dir: loadDir}).ExportArchive()
	if err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(t.TempDir(), "board.zip")
	if err := os.WriteFile(zipPath, loadZip, 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	backup, topics, err := board.LoadArchive(zipPath, now)
	if err != nil {
		t.Fatal(err)
	}
	if topics != len(loadState.Topics) || backup == "" {
		t.Fatalf("load result = %q, %d", backup, topics)
	}
	backupState, err := (FolderStore{Dir: backup}).Open(nil)
	if err != nil || !reflect.DeepEqual(backupState, want) {
		t.Fatalf("legacy backup mismatch: %v\nwant %s\ngot %s", err, folderJSON(want), folderJSON(backupState))
	}
	got := board.State()
	loadState.Seq = want.Seq + 1
	loadState.MetadataRevision = want.MetadataRevision
	loadState.StaleNudgeRevision = want.StaleNudgeRevision
	loadState.LoadedAt = "2026-11-20T12:01:00.001Z"
	if !reflect.DeepEqual(got, loadState) {
		t.Fatalf("loaded state mismatch\nwant %s\ngot %s", folderJSON(loadState), folderJSON(got))
	}
	if err := board.Close(); err != nil {
		t.Fatal(err)
	}
	if got := openTestBoardDB(t, sourceDir, nil, BoardDBOptions{}).State(); !reflect.DeepEqual(got, loadState) {
		t.Fatalf("loaded reopen mismatch\nwant %s\ngot %s", folderJSON(loadState), folderJSON(got))
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

func TestBoardDBCloseCheckpointsAndNoSharedMemory(t *testing.T) {
	dir := t.TempDir()
	makeLegacyBoard(t, dir, boardDBFixture())
	board := openTestBoardDB(t, dir, nil, BoardDBOptions{})
	for pragma, want := range map[string]string{
		"locking_mode":       "exclusive",
		"journal_mode":       "wal",
		"synchronous":        "1",
		"wal_autocheckpoint": "0",
		"cache_size":         "-256",
	} {
		var got string
		if err := board.db.QueryRow(`PRAGMA ` + pragma).Scan(&got); err != nil || got != want {
			t.Fatalf("PRAGMA %s = %q, %v; want %q", pragma, got, err, want)
		}
	}
	if err := board.Update(func(state *State) error { state.Seq++; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, boardDBName+"-shm")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("exclusive mode created shm: %v", err)
	}
	if err := board.Checkpoint(); err != nil {
		t.Fatal(err)
	}
	if err := board.Close(); err != nil {
		t.Fatal(err)
	}
	openTestBoardDB(t, dir, nil, BoardDBOptions{})
}

func TestBoardDBOpenRetriesExclusiveHandoff(t *testing.T) {
	dir := t.TempDir()
	makeLegacyBoard(t, dir, boardDBFixture())
	first := openTestBoardDB(t, dir, nil, BoardDBOptions{})
	type result struct {
		board *BoardDB
		err   error
	}
	done := make(chan result, 1)
	go func() {
		board, err := OpenBoardDB(dir, nil, BoardDBOptions{})
		done <- result{board: board, err: err}
	}()
	select {
	case got := <-done:
		if got.board != nil {
			_ = got.board.Close()
		}
		t.Fatalf("second opener did not wait: %v", got.err)
	case <-time.After(150 * time.Millisecond):
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if got.err != nil {
			t.Fatal(got.err)
		}
		defer got.board.Close()
	case <-time.After(BoardDBOpenRetryBound + time.Second):
		t.Fatal("second opener did not acquire DB after drain")
	}
}

func zipFiles(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, file := range r.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(rc)
		if closeErr := rc.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			t.Fatal(err)
		}
		out[file.Name] = body
	}
	return out
}

func slicesContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func assertBoardDBArchiveState(t *testing.T, archive []byte, want *State) {
	t.Helper()
	for name := range zipFiles(t, archive) {
		if strings.Contains(name, boardDBName) || name == "pending.json" {
			t.Fatalf("archive contains migration file %q", name)
		}
	}
	zipPath := filepath.Join(t.TempDir(), "board.zip")
	if err := os.WriteFile(zipPath, archive, 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := extractArchive(zipPath, dir); err != nil {
		t.Fatal(err)
	}
	old, err := (FolderStore{Dir: dir}).Open(nil)
	if err != nil || !reflect.DeepEqual(old, want) {
		t.Fatalf("legacy archive mismatch: %v\nwant %s\ngot %s", err, folderJSON(want), folderJSON(old))
	}
	board := openTestBoardDB(t, dir, nil, BoardDBOptions{})
	if got := board.State(); !reflect.DeepEqual(got, want) {
		t.Fatalf("reimport mismatch\nwant %s\ngot %s", folderJSON(want), folderJSON(got))
	}
	if err := board.Close(); err != nil {
		t.Fatal(err)
	}
	if got := openTestBoardDB(t, dir, nil, BoardDBOptions{}).State(); !reflect.DeepEqual(got, want) {
		t.Fatalf("reimport reopen mismatch\nwant %s\ngot %s", folderJSON(want), folderJSON(got))
	}
}

func TestBoardDBImportOnlyChangesLegacyThroughFolderOpen(t *testing.T) {
	for _, scenario := range []string{"unchanged", "content-and-close-migration", "pending-replay", "v1"} {
		t.Run(scenario, func(t *testing.T) {
			dir, control := t.TempDir(), t.TempDir()
			var legacy *State
			if scenario == "v1" {
				legacy = folderFixture()
			} else {
				fixture := boardDBFixture()
				if scenario == "content-and-close-migration" {
					fixture.ClosedTopics = []string{"work"}
					fixture.CloseResults = map[string]string{"work": "Completed legacy work."}
					// Commit pre-migration files directly, bypassing Open.
					f := FolderStore{Dir: dir}
					intent, err := f.plan(nil, nil, fixture)
					if err != nil {
						t.Fatal(err)
					}
					if err := f.commit(intent); err != nil {
						t.Fatal(err)
					}
				} else {
					makeLegacyBoard(t, dir, fixture)
				}
				if scenario == "pending-replay" {
					f := FolderStore{Dir: dir, Fault: func(stage string) error {
						if stage == "intent" {
							return folderCrash
						}
						return nil
					}}
					if err := f.Update(nil, folderAdd); !errors.Is(err, folderCrash) {
						t.Fatalf("pending fixture: %v", err)
					}
				}
			}
			for name, data := range folderTree(t, dir) {
				path := filepath.Join(control, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want := folderOpen(t, control, legacy)
			wantFiles := folderTree(t, control)
			board := openTestBoardDB(t, dir, legacy, BoardDBOptions{})
			if got := board.State(); !reflect.DeepEqual(got, want) {
				t.Fatalf("import mismatch\nwant %s\ngot %s", folderJSON(want), folderJSON(got))
			}
			assertLegacy := func() {
				t.Helper()
				gotFiles := folderTree(t, dir)
				delete(gotFiles, "pending.json")
				for name := range gotFiles {
					if strings.HasPrefix(name, boardDBName) {
						delete(gotFiles, name)
					}
				}
				if !reflect.DeepEqual(gotFiles, wantFiles) {
					t.Fatal("legacy files differ from FolderStore.Open's replay/migration")
				}
			}
			assertLegacy()
			if err := board.Update(folderAdd); err != nil {
				t.Fatal(err)
			}
			want = board.State()
			if err := board.Close(); err != nil {
				t.Fatal(err)
			}
			assertLegacy()
			reopened := openTestBoardDB(t, dir, nil, BoardDBOptions{})
			if got := reopened.State(); !reflect.DeepEqual(got, want) {
				t.Fatalf("updated reopen mismatch\nwant %s\ngot %s", folderJSON(want), folderJSON(got))
			}
			assertLegacy()
		})
	}
}

func TestBoardDBProcessKillRecovery(t *testing.T) {
	if stage := os.Getenv("WS_BOARD_DB_KILL_STAGE"); stage != "" {
		board, err := OpenBoardDB(os.Getenv("WS_BOARD_DB_KILL_DIR"), nil, BoardDBOptions{Observe: func(got string) {
			if got == stage {
				fmt.Println("board-kill-ready")
				select {} // Parent kills this process without transaction/handle cleanup.
			}
		}})
		if err != nil {
			t.Fatal(err)
		}
		defer board.Close()
		if err := board.Update(func(state *State) error {
			state.Seq += 1000
			state.Topics[0].BodyTimeProof.Digest = "uncommitted-proof"
			state.EntrySeq++
			state.PanelCounter++
			state.Topics[0].Entries = append(state.Topics[0].Entries, Entry{
				ID: fmt.Sprintf("e%d", state.EntrySeq), Kind: EntryPanel,
				PanelID: fmt.Sprintf("p%d", state.PanelCounter), Text: strings.Repeat("uncommitted", 100000),
			})
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		t.Fatal("child did not reach kill stage")
	}
	for _, stage := range []string{"import-committed", "transaction-before-commit"} {
		t.Run(stage, func(t *testing.T) {
			dir := t.TempDir()
			want := makeLegacyBoard(t, dir, boardDBFixture())
			if stage == "transaction-before-commit" {
				board := openTestBoardDB(t, dir, nil, BoardDBOptions{})
				if err := board.Update(folderAdd); err != nil {
					t.Fatal(err)
				}
				want = board.State()
				if err := board.Close(); err != nil {
					t.Fatal(err)
				}
			}
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, executable, "-test.run=^TestBoardDBProcessKillRecovery$")
			cmd.Env = append(os.Environ(), "WS_BOARD_DB_KILL_STAGE="+stage, "WS_BOARD_DB_KILL_DIR="+dir)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			ready := false
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				if scanner.Text() == "board-kill-ready" {
					ready = true
					break
				}
			}
			killErr := cmd.Process.Kill()
			waitErr := cmd.Wait()
			if !ready || killErr != nil || waitErr == nil {
				t.Fatalf("child ready=%v kill=%v wait=%v stderr=%s", ready, killErr, waitErr, stderr.String())
			}
			if stage == "import-committed" {
				if has, err := boardHasSentinel(dir); err != nil || has {
					t.Fatalf("pre-recovery sentinel = %v, %v", has, err)
				}
			}
			reopened := openTestBoardDB(t, dir, nil, BoardDBOptions{})
			if got := reopened.State(); !reflect.DeepEqual(got, want) {
				t.Fatalf("killed transaction escaped\nwant %s\ngot %s", folderJSON(want), folderJSON(got))
			}
			if has, err := boardHasSentinel(dir); err != nil || !has {
				t.Fatalf("post-recovery sentinel = %v, %v", has, err)
			}
		})
	}
}

func TestBoardDBBackgroundCheckpointWaitsForIdleAndWALThreshold(t *testing.T) {
	dir := t.TempDir()
	makeLegacyBoard(t, dir, boardDBFixture())
	board := openTestBoardDB(t, dir, nil, BoardDBOptions{})
	if board.checkpointTimer == nil {
		t.Fatal("import did not schedule an idle checkpoint")
	}
	before := folderRead(t, board.path)
	board.mu.Lock()
	board.checkpointDue = time.Now().Add(-time.Second)
	board.mu.Unlock()
	board.backgroundCheckpoint()
	if after := folderRead(t, board.path); !bytes.Equal(before, after) {
		t.Fatal("background checkpoint ran below WAL threshold")
	}
	if err := board.Update(func(state *State) error {
		state.EntrySeq++
		state.PanelCounter++
		state.Topics[0].Entries = append(state.Topics[0].Entries, Entry{
			ID: fmt.Sprintf("e%d", state.EntrySeq), Kind: EntryPanel,
			PanelID: fmt.Sprintf("p%d", state.PanelCounter), Text: strings.Repeat("x", BoardDBCheckpointWALBytes+1),
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(board.path + "-wal")
	if err != nil || info.Size() < BoardDBCheckpointWALBytes {
		t.Fatalf("large WAL fixture: %v, %v", info, err)
	}
	before = folderRead(t, board.path)
	board.backgroundCheckpoint() // A stale timer callback after the new write.
	if after := folderRead(t, board.path); !bytes.Equal(before, after) {
		t.Fatal("background checkpoint ignored the latest write's idle deadline")
	}
	board.mu.Lock()
	board.checkpointDue = time.Now().Add(-time.Second)
	board.mu.Unlock()
	board.backgroundCheckpoint()
	if after := folderRead(t, board.path); bytes.Equal(before, after) {
		t.Fatal("idle large WAL was not checkpointed")
	}
	want := board.State()
	if err := board.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestBoardDB(t, dir, nil, BoardDBOptions{})
	if reopened.checkpointTimer == nil || !reflect.DeepEqual(reopened.State(), want) {
		t.Fatal("reopen lost state or did not schedule idle checkpoint")
	}
}

func TestBoardDBBusyOpenIsBoundedWithoutRollbackAdvice(t *testing.T) {
	dir := t.TempDir()
	makeLegacyBoard(t, dir, boardDBFixture())
	first := openTestBoardDB(t, dir, nil, BoardDBOptions{})
	want := first.State()
	started := time.Now()
	second, err := OpenBoardDB(dir, nil, BoardDBOptions{})
	if second != nil {
		defer second.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "owning daemon") || strings.Contains(err.Error(), "manual rollback") {
		t.Fatalf("busy open diagnostic = %v", err)
	}
	if elapsed := time.Since(started); elapsed < BoardDBOpenRetryBound-100*time.Millisecond || elapsed > BoardDBOpenRetryBound+3*time.Second {
		t.Fatalf("busy open elapsed %s, bound %s", elapsed, BoardDBOpenRetryBound)
	}
	if !reflect.DeepEqual(first.State(), want) {
		t.Fatal("busy open changed the owning board")
	}
}

func TestBoardDBConcurrentFirstImportRechecksAfterLegacyLock(t *testing.T) {
	dir := t.TempDir()
	want := makeLegacyBoard(t, dir, boardDBFixture())
	holding, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	lockerDone := make(chan error, 1)
	go func() {
		lockerDone <- (FolderStore{Dir: dir}).locked(func() error {
			close(holding)
			<-release
			return nil
		})
	}()
	<-holding
	originalAttempt := folderLockAttempt
	attempted := make(chan struct{}, 2)
	folderLockAttempt = func(path string) {
		originalAttempt(path)
		if path == dir {
			select {
			case attempted <- struct{}{}:
			default:
			}
		}
	}
	t.Cleanup(func() { folderLockAttempt = originalAttempt })
	type result struct {
		board *BoardDB
		err   error
	}
	done := make(chan result, 2)
	for range 2 {
		go func() {
			board, err := OpenBoardDB(dir, nil, BoardDBOptions{})
			done <- result{board, err}
		}()
	}
	for range 2 {
		select {
		case <-attempted:
		case <-time.After(2 * time.Second):
			t.Fatal("both first importers did not reach the legacy lock")
		}
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-lockerDone; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		select {
		case got := <-done:
			if got.err != nil {
				t.Fatal(got.err)
			}
			if !reflect.DeepEqual(got.board.State(), want) {
				t.Fatal("concurrent import changed state")
			}
			if err := got.board.Close(); err != nil {
				t.Fatal(err)
			}
		case <-time.After(BoardDBOpenRetryBound + time.Second):
			t.Fatal("concurrent opener did not finish after owner close")
		}
	}
}

func TestBoardDBLoadArchiveRejectsRecoveryFiles(t *testing.T) {
	legacyDir := t.TempDir()
	makeLegacyBoard(t, legacyDir, boardDBFixture())
	legacyZip, err := (FolderStore{Dir: legacyDir}).ExportArchive()
	if err != nil {
		t.Fatal(err)
	}
	foreignDir := t.TempDir()
	foreign := boardDBFixture()
	foreign.Seq = 999
	foreignDB := openTestBoardDB(t, foreignDir, foreign, BoardDBOptions{})
	if err := foreignDB.Close(); err != nil {
		t.Fatal(err)
	}
	dbBytes := folderRead(t, filepath.Join(foreignDir, boardDBName))
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		t.Run("recovery"+suffix, func(t *testing.T) {
			files := zipFiles(t, legacyZip)
			files[boardDBName+suffix] = dbBytes
			var archive bytes.Buffer
			zw := zip.NewWriter(&archive)
			for name, data := range files {
				writer, err := zw.Create(name)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := writer.Write(data); err != nil {
					t.Fatal(err)
				}
			}
			if err := zw.Close(); err != nil {
				t.Fatal(err)
			}
			zipPath := filepath.Join(t.TempDir(), "board.zip")
			if err := os.WriteFile(zipPath, archive.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
			board := openTestBoardDB(t, t.TempDir(), boardDBFixture(), BoardDBOptions{})
			want := board.State()
			backup, _, err := board.LoadArchive(zipPath, time.Now())
			if err == nil || !strings.Contains(err.Error(), "SQLite recovery files") || backup != "" {
				t.Fatalf("recovery archive accepted or backed up: backup=%q err=%v", backup, err)
			}
			if !reflect.DeepEqual(board.State(), want) {
				t.Fatal("rejected archive changed destination")
			}
		})
	}
}

func TestBoardDBImportDuplicateLoadFloorsMatchesLegacy(t *testing.T) {
	dir := t.TempDir()
	makeLegacyBoard(t, dir, boardDBFixture())
	path := filepath.Join(dir, filepath.FromSlash(folderLoadFloorLog))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	first := folderLoadFloor{Generation: 1, LoadedAt: "2026-11-19T00:00:00.001Z"}
	duplicate := folderLoadFloor{Generation: 1, LoadedAt: "2026-11-19T00:00:00.002Z"}
	if err := os.WriteFile(path, append(folderJSON(first), folderJSON(duplicate)...), 0o644); err != nil {
		t.Fatal(err)
	}
	want := folderOpen(t, dir, nil)
	if want.LoadedAt != first.LoadedAt {
		t.Fatalf("legacy duplicate fixture floor = %q", want.LoadedAt)
	}
	board := openTestBoardDB(t, dir, nil, BoardDBOptions{})
	if !reflect.DeepEqual(board.State(), want) {
		t.Fatal("duplicate floor import disagrees with FolderStore")
	}
	if err := board.Close(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(openTestBoardDB(t, dir, nil, BoardDBOptions{}).State(), want) {
		t.Fatal("duplicate floor reopen disagrees with FolderStore")
	}
}

func TestBoardDBFreshPrimitives(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "board")
	if _, err := OpenBoardDB(dir, nil, BoardDBOptions{}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("absent: %v", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("open created directory: %v", err)
	}
	initial := boardDBFixture()
	b, err := CreateBoardDB(dir, initial, BoardDBOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if _, err := CreateBoardDB(dir, initial, BoardDBOptions{}); !errors.Is(err, os.ErrExist) {
		t.Fatalf("create existing: %v", err)
	}
	for _, name := range []string{"meta.json", "committed.json", "t1"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("unexpected legacy artifact %s: %v", name, err)
		}
	}
	if has, err := boardHasSentinel(dir); err != nil || !has {
		t.Fatalf("sentinel: %v %v", has, err)
	}
	snap, rev := b.Snapshot()
	if rev != 1 || !reflect.DeepEqual(snap, initial) {
		t.Fatalf("snapshot %d %s", rev, folderJSON(snap))
	}
	snap.Seq++
	if b.Revision() != rev || b.State().Seq == snap.Seq {
		t.Fatal("snapshot aliases state")
	}
	if err := b.RecordDashboardAnswer("id", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := b.RecordDashboardAnswer("id", time.Now()); err != nil {
		t.Fatal(err)
	}
	if b.Revision() != rev+1 {
		t.Fatalf("answer revision %d", b.Revision())
	}
	if unlock, ok := LockBoardIncarnation(dir, initial.LoadedAt); !ok {
		t.Fatal("open did not register incarnation")
	} else {
		unlock()
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	calls := 0
	reopened, err := OpenBoardDBLazy(dir, func() (*State, error) { calls++; return nil, errors.New("bad legacy") }, BoardDBOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if calls != 0 {
		t.Fatal("read obsolete snapshot")
	}
}

func TestBoardDBNewLoadAndFailedReplacementIncarnation(t *testing.T) {
	root := t.TempDir()
	src, err := CreateBoardDB(filepath.Join(root, "src"), boardDBFixture(), BoardDBOptions{})
	if err != nil {
		t.Fatal(err)
	}
	archive, err := src.ExportArchive()
	if err != nil {
		t.Fatal(err)
	}
	_ = src.Close()
	zipPath := filepath.Join(root, "board.zip")
	if err := os.WriteFile(zipPath, archive, 0600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "dst")
	fail := false
	b, n, err := LoadNewBoardDB(dir, zipPath, BoardDBOptions{Fault: func(stage string) error {
		if fail && stage == "transaction-before-commit" {
			return errors.New("injected")
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	state, rev := b.Snapshot()
	if n != len(state.Topics) || state.LoadedAt == "" {
		t.Fatal("missing load floor")
	}
	if matches, _ := filepath.Glob(dir + ".bak-*"); len(matches) != 0 {
		t.Fatal("new load made backup")
	}
	fail = true
	if _, _, err := b.LoadArchive(zipPath, time.Now()); err == nil {
		t.Fatal("expected replacement failure")
	}
	after, afterRev := b.Snapshot()
	if !reflect.DeepEqual(after, state) || afterRev != rev {
		t.Fatal("failed replacement changed board")
	}
	if unlock, ok := LockBoardIncarnation(dir, state.LoadedAt); !ok {
		t.Fatal("failed load lost admission")
	} else {
		unlock()
	}
	if _, ok := LockBoardIncarnation(dir, "stale"); ok {
		t.Fatal("stale token admitted")
	}
}

func TestBoardDBLoadBlocksStaleAdmission(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "board")
	reached, release := make(chan struct{}), make(chan struct{})
	block := false
	b, err := CreateBoardDB(dir, boardDBFixture(), BoardDBOptions{Observe: func(stage string) {
		if block && stage == "transaction-before-commit" {
			close(reached)
			<-release
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	archive, err := b.ExportArchive()
	if err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(root, "archive.zip")
	if err := os.WriteFile(zipPath, archive, 0600); err != nil {
		t.Fatal(err)
	}
	old := b.State().LoadedAt
	block = true
	done := make(chan error, 1)
	go func() { _, _, err := b.LoadArchive(zipPath, time.Now()); done <- err }()
	select {
	case <-reached:
	case <-time.After(5 * time.Second):
		t.Fatal("load did not reach commit")
	}
	if unlock, ok := LockBoardIncarnation(dir, old); ok {
		unlock()
		close(release)
		t.Fatal("old token admitted during load")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if unlock, ok := LockBoardIncarnation(dir, old); ok {
		unlock()
		t.Fatal("old token admitted after load")
	}
	if unlock, ok := LockBoardIncarnation(dir, b.State().LoadedAt); !ok {
		t.Fatal("new token rejected")
	} else {
		unlock()
	}
}

func TestBoardDBLazyAbsentAndConversionError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "board")
	marker := errors.New("parse failure")
	calls := 0
	_, err := OpenBoardDBLazy(dir, func() (*State, error) { calls++; return nil, marker }, BoardDBOptions{})
	if !errors.Is(err, marker) || calls != 1 {
		t.Fatalf("callback %d error %v", calls, err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed open created artifacts: %v", err)
	}
}
