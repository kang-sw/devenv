package wsagent

import (
	"fmt"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsstate"
)

func TestChildEventWith200ConversationsDoesNoFileIO(t *testing.T) {
	r := NewRunner(Options{})
	dir := t.TempDir()
	parent := &conv{r: r, dir: dir, rec: Record{ReplyID: "parent"}, spawnDirty: true, spawnC: make(chan struct{}, 1),
		live: newLiveWriter(dir, "parent-key", "pi", r.opts.Now, io.Discard)}
	r.convs["parent"] = parent
	var child *conv
	for i := 0; i < 200; i++ {
		id := fmt.Sprintf("child-%d", i)
		c := &conv{r: r, rec: Record{ReplyID: id}, inbox: filepath.Join(dir, id, "inbox")}
		if i == 0 {
			c.rec.Parent = &ParentRef{Kind: ParentConversation, ReplyID: "parent"}
			child = c
		}
		r.convs[id] = c
	}
	before := wsstate.FileIOCounters()["file_io_total"]
	r.wakeSpawnParents(child.memoryState())
	parent.seedSpawnRows()
	if got := len(r.List()); got != 201 {
		t.Fatalf("List returned %d rows", got)
	}
	if got := wsstate.FileIOCounters()["file_io_total"] - before; got != 0 {
		t.Fatalf("child event/list made %d file operations with 200 conversations, want 0", got)
	}
}

func TestSpawnBurstInboxScanCountsAndTrailingUpdate(t *testing.T) {
	now := time.Unix(1000, 0)
	scans, lists := 0, 0
	r := NewRunner(Options{
		Now:            func() time.Time { return now },
		stateInboxRead: func(string) { scans++ },
		spawnListed:    func(string) { lists++ },
	})
	dir := t.TempDir()
	parent := &conv{r: r, rec: Record{ReplyID: "parent"}, inbox: filepath.Join(dir, "parent", "inbox"), spawnDirty: true,
		live: newLiveWriter(filepath.Join(dir, "parent"), "parent-key", "pi", r.opts.Now, io.Discard)}
	child := &conv{r: r, rec: Record{ReplyID: "child", WSKey: "child-key", Label: "initial", Parent: &ParentRef{Kind: ParentConversation, ReplyID: "parent"}},
		inbox: filepath.Join(dir, "child", "inbox"), turn: true}
	r.convs["parent"], r.convs["child"] = parent, child
	defer func() {
		if parent.spawnTimer != nil {
			parent.spawnTimer.Stop()
		}
	}()
	// Even forced reseeds now use memory-only indexed descendants.
	for range 20 {
		parent.spawnDirty, parent.spawnNext = true, time.Time{}
		parent.seedSpawnRows()
	}
	control := scans
	if control != 0 || lists != 20 {
		t.Fatalf("control: inbox scans=%d listings=%d", control, lists)
	}
	scans, lists = 0, 0
	parent.spawnDirty, parent.spawnNext = true, time.Time{}
	parent.seedSpawnRows()
	if scans != 0 || lists != 1 {
		t.Fatalf("initial tree: inbox scans=%d listings=%d", scans, lists)
	}
	before := scans
	for range 20 {
		now = now.Add(time.Millisecond)
		child.rec.Label = "latest"
		child.turn, child.lastErr = false, "failed"
		parent.spawnDirty = true
		parent.seedSpawnRows()
	}
	if scans != before || lists != 1 || !parent.spawnDirty || parent.spawnTimer == nil {
		t.Fatalf("burst: scans=%d listings=%d dirty=%v timer=%v", scans-before, lists, parent.spawnDirty, parent.spawnTimer != nil)
	}
	t.Logf("20 invalidations, 2 conversations: forced=%d inbox ReadDir calls, burst=%d", control, scans-before)
	// No later child event is needed: the retained dirty bit and trailing
	// deadline refresh the latest state, rather than the first queued event.
	now = time.Unix(1001, 0)
	parent.seedSpawnRows()
	if scans != 0 || lists != 2 || parent.spawnDirty || parent.spawnTimer != nil {
		t.Fatalf("trailing update: scans=%d listings=%d dirty=%v", scans, lists, parent.spawnDirty)
	}
	rows := parent.live.st.Snapshot(now.UnixMilli()).Agents
	if len(rows) != 1 || rows[0].Label != "latest" || rows[0].Status != wslive.StatusFailed {
		t.Fatalf("trailing projection lost latest child state: %+v", rows)
	}
}
