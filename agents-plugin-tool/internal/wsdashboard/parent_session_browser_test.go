package wsdashboard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// The head's parent jump on real pages, in Chromium: on a child's page the
// pill sits left of the identity inside the shell's 34px strip, at wide and
// at the desktop shell's 760px minimum, in both themes; a click switches the
// shell to the parent in place (no reload) and a root parent shows no pill;
// a pruned parent's pill is inert. WS_PARENT_SESSION_SHOTS (absolute) keeps
// the screenshots.
func TestParentSessionBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to use cached Chromium")
	}
	shots := os.Getenv("WS_PARENT_SESSION_SHOTS")
	if shots != "" && !filepath.IsAbs(shots) {
		t.Fatal("WS_PARENT_SESSION_SHOTS must be absolute")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	t.Setenv("WS_CONFIG_HOME", t.TempDir())
	SetDevAssets(nil)
	f := startPageFixture(t, func(o *DaemonOptions) {
		o.Port = 0
		o.LAN = false
		o.bindHost = "127.0.0.1"
	})
	now := time.Now()
	add := func(id, parent, label string) {
		reg := register(t, f.dir, id, f.project, now)
		reg.ParentReplyID = parent
		if err := WriteRegistration(f.dir, reg); err != nil {
			t.Fatal(err)
		}
		if label != "" {
			if err := writeSessionLabel(SessionDir(f.dir, id), label); err != nil {
				t.Fatal(err)
			}
		}
		f.src.setPage(id, PageContent{HasBoard: true, Seq: 1, Board: boardRegions(t, id)})
	}
	add("parentsession", "", "Parent Label")
	add("kidsession", "parentsession", "Kid Label")
	add("deepsession", "kidsession", "A Rather Long Descendant Session Label")
	add("orphankid", "gonesession", "Orphan Kid")
	f.d.Index().Refresh()
	for _, id := range []string{"parentsession", "kidsession", "deepsession", "orphankid"} {
		id := id
		waitFor(t, id+" to be indexed", func() bool { _, ok := f.d.Index().Session(id); return ok })
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "node", "testdata/parent-session.browser.cjs", f.base, shots).CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("Chromium parent session jump: %v", err)
	}
}
