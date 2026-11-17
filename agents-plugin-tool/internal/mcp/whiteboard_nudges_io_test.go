package mcp

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

func TestWhiteboardPokedDiscoveryIsConstantIO(t *testing.T) {
	f := newNudgeFixture(t)
	for i := 0; i < 200; i++ {
		nudgeWriteJSON(t, f.n.dirs().partPath(fmt.Sprintf("unrelated-%d", i), partKey), f.record)
	}
	path := f.n.dirs().partPath(f.target.key, partKey)
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	f.n.discover()
	before := wsstate.FileIOCounters()
	if _, ok := f.n.discoverTarget(f.target); !ok {
		t.Fatal("poked board missing")
	}
	after := wsstate.FileIOCounters()
	if got := after["file_io_stat"] - before["file_io_stat"]; got != 2 {
		t.Fatalf("poked discovery used %d stats, want identity + board only", got)
	}
	if got := after["file_io_read_dir"] - before["file_io_read_dir"]; got != 0 {
		t.Fatalf("poke enumerated keys: %d directory reads", got)
	}
}
