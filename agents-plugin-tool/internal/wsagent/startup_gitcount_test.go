package wsagent

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Exercise the real lazy recovery path, not MCP intake's separate repo reader.
// Every PATH-routed git attempt is recorded and refused, so an accidental new
// discovery/status call cannot be silently credited as a recovery saving.
func TestColdRecoveryTwelveConversationsRunsNoGit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX git-attempt trap; verification runs on Linux")
	}
	bin, log := t.TempDir(), filepath.Join(t.TempDir(), "git-attempts")
	trap := "#!/bin/sh\nprintf '%s\\n' \"$*\" >>\"$PHASE3_GIT_LOG\"\nexit 97\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(trap), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PHASE3_GIT_LOG", log)
	h := newHarness(t, map[string]string{"chat": chatProfile})
	for range 12 {
		h.create("chat")
	}
	h.runner.Close()
	h.start()
	if states := h.runner.List(); len(states) != 12 {
		t.Fatalf("recovered %d conversations", len(states))
	}
	for _, state := range h.runner.List() {
		if state.Process {
			t.Fatal("cold recovery started a process")
		}
	}
	h.runner.Close()
	if raw, err := os.ReadFile(log); !os.IsNotExist(err) {
		t.Fatalf("unexpected startup Git attempts: %q (%v)", raw, err)
	}
	if h.backend.startCount() != 0 {
		t.Fatal("recovery started a provider")
	}
	t.Log("12 seeded conversations, cold recovery and shutdown, no consumers: 0 Git attempts (already 0 on base; not a new saving)")
}
