package wsagent

import (
	"testing"

	"github.com/kang-sw/devenv/internal/wsstate"
)

func TestPermissionSelectionCachedAndUpdatedOnWrite(t *testing.T) {
	dir := t.TempDir()
	if _, explicit, err := cachedPermissionSelection(dir); err != nil || explicit {
		t.Fatalf("initial selection: explicit=%v err=%v", explicit, err)
	}
	before := wsstate.FileIOCounters()["file_io_total"]
	for i := 0; i < 200; i++ {
		_, _, _ = cachedPermissionSelection(dir)
	}
	if got := wsstate.FileIOCounters()["file_io_total"] - before; got != 0 {
		t.Fatalf("cached selection made %d file operations", got)
	}
	if err := writePermissionSelection(dir, PermissionReadOnly); err != nil {
		t.Fatal(err)
	}
	if level, explicit, err := cachedPermissionSelection(dir); err != nil || !explicit || level != PermissionReadOnly {
		t.Fatalf("write not visible: level=%s explicit=%v err=%v", level, explicit, err)
	}
}
