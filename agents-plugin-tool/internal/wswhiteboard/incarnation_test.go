package wswhiteboard

import (
	"path/filepath"
	"testing"
	"time"
)

func TestArchiveLoadIncarnationChangesOnlyOnLoad(t *testing.T) {
	root := t.TempDir()
	source := archiveSource(t, filepath.Join(root, "source"))
	data, err := source.ExportArchive()
	if err != nil {
		t.Fatal(err)
	}
	zipPath := writeArchive(t, filepath.Join(root, "board.zip"), data)
	dest := archiveDestination(t, filepath.Join(root, "dest"))
	clock := archiveLoadNow
	archiveLoadNow = func() time.Time { return archiveNow }
	t.Cleanup(func() { archiveLoadNow = clock })
	var previous string
	for i := 0; i < 2; i++ {
		if _, _, err := dest.LoadArchive(zipPath, archiveNow); err != nil {
			t.Fatal(err)
		}
		state := folderOpen(t, dest.Dir, nil)
		if state.LoadedAt == previous {
			t.Fatal("same-millisecond loads reused an incarnation")
		}
		if _, ok := dest.LockIncarnation(previous); ok {
			t.Fatal("old incarnation admitted")
		}
		unlock, ok := dest.LockIncarnation(state.LoadedAt)
		if !ok {
			t.Fatal("current incarnation refused")
		}
		unlock()
		previous = state.LoadedAt
		archiveUpdate(t, dest, archiveApply(`<wb:topic name="handoff"><wb:log>ordinary edit</wb:log></wb:topic>`))
		if after := folderOpen(t, dest.Dir, nil); after.LoadedAt != previous {
			t.Fatal("ordinary edit changed incarnation")
		}
	}
}
