// Tests for board db import.
package wswhiteboard

import (
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
