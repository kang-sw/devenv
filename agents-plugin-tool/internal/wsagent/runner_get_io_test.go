package wsagent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// TestLeadGetRereadsNothingWhileUnchanged pins the hosted page's header
// fetch (Runner.Get): an unchanged lead lists its inbox and stats its
// permission selection but opens no file, while an entry another process
// adds and a selection another process rewrites are both picked up.
func TestLeadGetRereadsNothingWhileUnchanged(t *testing.T) {
	r := NewRunner(Options{})
	dir := t.TempDir()
	c := &conv{r: r, dir: dir, rec: Record{ReplyID: "lead", Lead: true}, inbox: wsinbox.Dir(dir)}
	r.convs["lead"] = c
	now := time.Now()
	for _, text := range []string{"one", "two"} {
		if _, _, err := wsinbox.Send(c.inbox, wsinbox.Entry{Parts: []string{text}}, now); err != nil {
			t.Fatal(err)
		}
	}
	permission := filepath.Join(dir, "permission.json")
	if err := os.WriteFile(permission, []byte(`{"permission_level": "read-only"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-time.Hour)
	if err := os.Chtimes(permission, old, old); err != nil {
		t.Fatal(err)
	}
	reads := func() int64 { return wsstate.FileIOCounters()["file_io_read_file"] }

	st, err := r.Get("lead")
	if err != nil || st.Held != 2 {
		t.Fatalf("first Get: held=%d err=%v", st.Held, err)
	}
	before := reads()
	for range 10 {
		if st, _ = r.Get("lead"); st.Held != 2 {
			t.Fatalf("unchanged Get: held=%d", st.Held)
		}
	}
	if got := reads() - before; got != 0 {
		t.Fatalf("10 unchanged Gets read %d files, want 0", got)
	}

	// Another process adds an entry: only the new name is opened.
	if _, _, err := wsinbox.Send(c.inbox, wsinbox.Entry{Parts: []string{"three"}}, now); err != nil {
		t.Fatal(err)
	}
	before = reads()
	if st, _ = r.Get("lead"); st.Held != 3 {
		t.Fatalf("after an external send: held=%d", st.Held)
	}
	if got := reads() - before; got != 1 {
		t.Fatalf("external send: read %d files, want 1", got)
	}

	// Another process drains the inbox: the count follows the listing.
	if _, err := wsinbox.Drain(c.inbox); err != nil {
		t.Fatal(err)
	}
	if st, _ = r.Get("lead"); st.Held != 0 {
		t.Fatalf("after an external drain: held=%d", st.Held)
	}

	// Another process rewrites the selection, keeping its old mtime: the
	// size moved, so the next Get re-reads it.
	if err := os.WriteFile(permission, []byte(`{"permission_level": "workdir-write"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(permission, old, old); err != nil {
		t.Fatal(err)
	}
	r.Get("lead")
	if level, _, _ := cachedPermissionSelection(dir); level != PermissionWorkdirWrite {
		t.Fatalf("external selection edit missed: level=%q", level)
	}
	// A selection removed by another process reads as absent.
	if err := os.Remove(permission); err != nil {
		t.Fatal(err)
	}
	r.Get("lead")
	if level, explicit, err := cachedPermissionSelection(dir); level != "" || explicit || err != nil {
		t.Fatalf("removed selection still cached: level=%q explicit=%v err=%v", level, explicit, err)
	}
}
