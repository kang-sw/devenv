package wswhiteboard

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// v0.50.7 decodes State into a fixed struct without LoadedAt, then marshals
// the entire folderSession again (folders.go:746,1018). This reproduces that
// field loss without retaining the new field in a raw JSON map.
func rewriteWithoutLoadTime(t *testing.T, dir string) {
	t.Helper()
	var meta folderSession
	if _, err := folderDecode(filepath.Join(dir, "meta.json"), &meta); err != nil {
		t.Fatal(err)
	}
	meta.State.LoadedAt = ""
	data := folderJSON(meta)
	if bytes.Contains(data, []byte(`"loaded_at"`)) {
		t.Fatal("downgrade fixture retained loaded_at")
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertOldAnswerExcluded(t *testing.T, state *State, sentAt int64) {
	t.Helper()
	if len(state.Items) == 0 {
		t.Fatal("fixture needs a reused item ID")
	}
	for _, item := range state.Items {
		// The dashboard sentMarks and ackBatch both exclude a chip exactly
		// when chip.SentAt < ReaskTimes()[id]. Missing floors must fail too.
		if floor, ok := state.ReaskTimes()[item.ID]; !ok || sentAt >= floor {
			t.Fatalf("old answer at %d would attach to reused item %d (floor %d, present %v)", sentAt, item.ID, floor, ok)
		}
	}
}

func TestArchiveLoadFloorSurvivesDowngradeRewrite(t *testing.T) {
	root := t.TempDir()
	source := archiveSource(t, filepath.Join(root, "source"))
	data, err := source.ExportArchive()
	if err != nil {
		t.Fatal(err)
	}
	zipPath := writeArchive(t, filepath.Join(root, "b.zip"), data)
	clock := archiveLoadNow
	archiveLoadNow = func() time.Time { return archiveNow }
	t.Cleanup(func() { archiveLoadNow = clock })
	for _, stage := range []string{"", "intent", "log-partial", "session-meta", "complete"} {
		t.Run("crash-"+stage, func(t *testing.T) {
			dest := archiveDestination(t, filepath.Join(t.TempDir(), "dest"))
			crashing := dest
			crashing.Fault = func(at string) error {
				if stage != "" && at == stage {
					return folderCrash
				}
				return nil
			}
			_, _, err := crashing.LoadArchive(zipPath, archiveNow)
			if stage == "" && err != nil || stage != "" && !errors.Is(err, folderCrash) {
				t.Fatalf("load at %q: %v", stage, err)
			}
			// Recover the intent before the simulated old writer opens it.
			// Its log append carries raw bytes even in v0.50.7 replay.
			before := folderOpen(t, dest.Dir, nil)
			want := archiveNow.Add(time.Millisecond).UTC().Format(reaskLayout)
			if before.LoadedAt != want {
				t.Fatalf("LoadedAt = %q, want %q", before.LoadedAt, want)
			}
			logPath := filepath.Join(dest.Dir, filepath.FromSlash(folderLoadFloorLog))
			record := folderRead(t, logPath)
			if bytes.Count(record, []byte{'\n'}) != 1 {
				t.Fatalf("one load must append one record: %q", record)
			}
			rewriteWithoutLoadTime(t, dest.Dir)
			var cache FolderCache
			got, err := cache.Open(dest, nil)
			if err != nil || got.LoadedAt != want {
				t.Fatalf("reopen after downgrade: state=%+v err=%v", got, err)
			}
			assertOldAnswerExcluded(t, got, archiveNow.UnixMilli())

			// Warm reads neither reopen the sidecar nor take the folder lock.
			reads := countFolderReads(t)
			for i := 0; i < 3; i++ {
				if _, err := cache.Open(dest, nil); err != nil {
					t.Fatal(err)
				}
			}
			if *reads != 0 {
				t.Fatalf("warm reads did %d file reads", *reads)
			}
			archiveUpdate(t, dest, archiveApply(`<wb:topic name="handoff"><wb:log>ordinary update</wb:log></wb:topic>`))
			if !bytes.Equal(record, folderRead(t, logPath)) {
				t.Fatal("ordinary update rewrote the load floor")
			}
			rewriteWithoutLoadTime(t, dest.Dir)
			assertOldAnswerExcluded(t, folderOpen(t, dest.Dir, nil), archiveNow.UnixMilli())

			// Export/backup retain the recovery bytes even with no meta field.
			exported, err := dest.ExportArchive()
			if err != nil || !bytes.Equal(unzip(t, exported)[folderLoadFloorLog], record) {
				t.Fatalf("export lost the load floor: %v", err)
			}
			backup, _, err := dest.LoadArchive(zipPath, archiveNow)
			if err != nil {
				t.Fatal(err)
			}
			assertOldAnswerExcluded(t, folderOpen(t, backup, nil), archiveNow.UnixMilli())
			if got := bytes.Count(folderRead(t, logPath), []byte{'\n'}); got != 2 {
				t.Fatalf("two loads appended %d records", got)
			}
		})
	}
}

func TestArchiveLoadFloorCapturedAfterLockWait(t *testing.T) {
	root := t.TempDir()
	source := archiveSource(t, filepath.Join(root, "source"))
	data, err := source.ExportArchive()
	if err != nil {
		t.Fatal(err)
	}
	zipPath := writeArchive(t, filepath.Join(root, "b.zip"), data)
	dest := archiveDestination(t, filepath.Join(root, "dest"))
	var now atomic.Int64
	now.Store(archiveNow.UnixMilli())
	clock := archiveLoadNow
	archiveLoadNow = func() time.Time { return time.UnixMilli(now.Load()) }
	t.Cleanup(func() { archiveLoadNow = clock })

	locked, release, holderDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		holderDone <- dest.locked(func() error {
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked
	attempt := make(chan struct{}, 1)
	oldAttempt := folderLockAttempt
	folderLockAttempt = func(dir string) {
		if dir == dest.Dir {
			attempt <- struct{}{}
		}
	}
	t.Cleanup(func() { folderLockAttempt = oldAttempt })
	done := make(chan error, 1)
	go func() {
		_, _, err := dest.LoadArchive(zipPath, archiveNow)
		done <- err
	}()
	select {
	case <-attempt:
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("load never reached the destination lock")
	}
	// The old board remains visible while load waits. An answer now is newer
	// than the call's timestamp and must still be excluded after the swap.
	answerAt := archiveNow.Add(time.Minute).UnixMilli()
	now.Store(answerAt)
	close(release)
	if err := <-holderDone; err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	folderLockAttempt = oldAttempt
	got := folderOpen(t, dest.Dir, nil)
	if want := time.UnixMilli(answerAt + 1).UTC().Format(reaskLayout); got.LoadedAt != want {
		t.Fatalf("LoadedAt = %q, want locked swap time %q", got.LoadedAt, want)
	}
	assertOldAnswerExcluded(t, got, answerAt)
}
