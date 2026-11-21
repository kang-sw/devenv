// Tests for board db archive.
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
	"testing"
	"time"
)

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
