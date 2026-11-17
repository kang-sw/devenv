package wsagent

import (
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

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
	// Replay the old per-invalidation listing policy as a control, using
	// the same real List -> state -> Held -> Names -> ReadDir production path.
	for range 20 {
		parent.spawnDirty, parent.spawnNext = true, time.Time{}
		parent.seedSpawnRows()
	}
	control := scans
	if control != 40 || lists != 20 {
		t.Fatalf("control: inbox scans=%d listings=%d", control, lists)
	}
	scans, lists = 0, 0
	parent.spawnDirty, parent.spawnNext = true, time.Time{}
	parent.seedSpawnRows()
	if scans != 2 || lists != 1 {
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
	t.Logf("20 invalidations, 2 conversations: control=%d inbox ReadDir calls, after=%d in burst + 2 trailing", control, scans-before)
	// No later child event is needed: the retained dirty bit and trailing
	// deadline refresh the latest state, rather than the first queued event.
	now = time.Unix(1001, 0)
	parent.seedSpawnRows()
	if scans != 4 || lists != 2 || parent.spawnDirty || parent.spawnTimer != nil {
		t.Fatalf("trailing update: scans=%d listings=%d dirty=%v", scans, lists, parent.spawnDirty)
	}
	rows := parent.live.st.Snapshot(now.UnixMilli()).Agents
	if len(rows) != 1 || rows[0].Label != "latest" || rows[0].Status != wslive.StatusFailed {
		t.Fatalf("trailing projection lost latest child state: %+v", rows)
	}
}
