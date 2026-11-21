package wswhiteboard

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
