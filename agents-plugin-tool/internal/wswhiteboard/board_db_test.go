package wswhiteboard

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
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
	go func() {
		next, err := OpenBoardDB(dir, nil, BoardDBOptions{})
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
		name     string
		sentinel bool
		corrupt  bool
	}{
		{name: "missing-under-sentinel", sentinel: true},
		{name: "corrupt-under-sentinel", sentinel: true, corrupt: true},
		{name: "corrupt-without-sentinel", corrupt: true},
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
	if err := board.Update(func(*State) error { return nil }); err != nil {
		t.Fatal(err)
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
	sourceDir := t.TempDir()
	want := makeLegacyBoard(t, sourceDir, boardDBFixture())
	sourceZip, err := (FolderStore{Dir: sourceDir}).ExportArchive()
	if err != nil {
		t.Fatal(err)
	}
	board := openTestBoardDB(t, sourceDir, nil, BoardDBOptions{})
	exported, err := board.ExportArchive()
	if err != nil {
		t.Fatal(err)
	}
	if got, wantFiles := zipFiles(t, exported), zipFiles(t, sourceZip); !reflect.DeepEqual(got, wantFiles) {
		t.Fatalf("export differs\nwant %#v\ngot  %#v", wantFiles, got)
	}

	loadState := boardDBFixture()
	loadState.Seq = 77
	loadDir := t.TempDir()
	makeLegacyBoard(t, loadDir, loadState)
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
	if _, err := (FolderStore{Dir: backup}).Open(nil); err != nil {
		t.Fatalf("legacy backup unreadable: %v", err)
	}
	got := board.State()
	if got.Seq != loadState.Seq || got.LoadedAt == "" || got.Topics[0].Body != want.Topics[0].Body {
		t.Fatalf("loaded state = %s", folderJSON(got))
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
	if _, err := OpenBoardDB(dir, nil, BoardDBOptions{}); err != nil {
		t.Fatalf("reopen after close checkpoint: %v", err)
	}
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
