package wsdashboard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/kang-sw/devenv/internal/wsholder"
)

// holderOutput is a wsholder.Sink that keeps each holder's output bytes.
type holderOutput struct {
	mu  sync.Mutex
	out map[string]*bytes.Buffer
}

func (h *holderOutput) HolderEvent(id string, ev wsholder.Event) {
	if ev.Event != "output" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.out == nil {
		h.out = map[string]*bytes.Buffer{}
	}
	if h.out[id] == nil {
		h.out[id] = &bytes.Buffer{}
	}
	h.out[id].Write(ev.Data)
}

func (h *holderOutput) text(id string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.out[id] == nil {
		return ""
	}
	return h.out[id].String()
}

// holderPython is an interpreter that can run a holder; the test is skipped
// without one.
func holderPython(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			if out, err := exec.Command(path, "-c", "import sys; print(sys.version_info >= (3, 10))").Output(); err == nil && strings.TrimSpace(string(out)) == "True" {
				return path
			}
		}
	}
	t.Skip("no python3 (3.10 or newer) on PATH")
	return ""
}

// TestDaemonRestartReattachesToARunningHolder runs real holders under two
// daemons in turn: the first starts a holder with a ticking child and
// closes, the second finds the holder from its descriptor, lists it
// attached with the same child still running, receives the output the child
// wrote while no daemon was up, and ends it through the stop route.
func TestDaemonRestartReattachesToARunningHolder(t *testing.T) {
	python := holderPython(t)
	holdersDir := filepath.Join(t.TempDir(), wsholder.DirName)
	// Whatever the test leaves running is stopped by a last manager.
	t.Cleanup(func() {
		last := wsholder.New(wsholder.Options{Dir: holdersDir, Python: python})
		defer last.Close()
		last.Scan()
		for _, h := range last.List().Holders {
			_ = last.Stop(h.ID)
		}
	})
	const ticker = "import time\nfor n in range(1, 1200):\n    print('tick %d' % n, flush=True)\n    time.sleep(0.05)\n"

	first := wsholder.New(wsholder.Options{Dir: holdersDir, Python: python})
	f1 := startPageFixture(t, withHolders(first))
	id, err := first.Launch(json.RawMessage(`{"label":"restart check"}`))
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	child, err := first.Client(id).Spawn(wsholder.SpawnSpec{Argv: []string{python, "-c", ticker}, Mode: "pipe"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	// Whether the holder could leave the test's job object is the
	// environment's (a GitHub Windows runner's job forbids it, and the
	// holder then starts marked not restart-safe); the second daemon must
	// list what the holder was started with.
	started := first.List().Holders
	if len(started) != 1 || started[0].ID != id {
		t.Fatalf("first daemon's holders = %+v, want %s", started, id)
	}
	f1.d.Close()
	if first.Client(id) != nil {
		t.Fatal("the closed daemon still holds the holder's attach")
	}

	sink := &holderOutput{}
	second := wsholder.New(wsholder.Options{Dir: holdersDir, Python: python, Sink: sink})
	f2 := startPageFixture(t, withHolders(second))
	local := viewer{peer: "127.0.0.1:50000", host: "127.0.0.1:" + strconv.Itoa(f2.d.Endpoint().Port)}
	var listed wsholder.Info
	waitFor(t, "the second daemon to attach to the holder", func() bool {
		list := decodeHolderList(t, serve(f2.d, local, http.MethodGet, "/holders", ""))
		if len(list.Holders) != 1 || !list.Holders[0].Attached {
			return false
		}
		listed = list.Holders[0]
		return true
	})
	if listed.ID != id || listed.Child == nil || listed.Child.State != "running" || listed.Child.PID != child.PID || listed.RestartSafe != started[0].RestartSafe {
		t.Fatalf("listed holder = %+v (child %+v), want %s with child %d still running, restart-safe %v", listed, listed.Child, id, child.PID, started[0].RestartSafe)
	}
	if !strings.Contains(string(listed.Session), "restart check") {
		t.Errorf("session = %s, want the descriptor the holder was started with", listed.Session)
	}
	// The whole ring is replayed to a new daemon: the first tick, written
	// under the first daemon, and ticks written since.
	waitFor(t, "the child's output on the second daemon", func() bool {
		out := sink.text(id)
		return strings.Contains(out, "tick 1\n") || strings.Contains(out, "tick 1\r\n")
	})
	waitFor(t, "output written after the first daemon closed", func() bool { return strings.Contains(sink.text(id), "tick 8") })

	if rec := serve(f2.d, local, http.MethodPost, "/holders/"+id+"/stop", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("POST stop = %d (%s), want 204", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if list := decodeHolderList(t, serve(f2.d, local, http.MethodGet, "/holders", "")); len(list.Holders) != 0 {
		t.Errorf("holders after the stop = %+v, want none", list.Holders)
	}
	if _, err := os.Stat(filepath.Join(holdersDir, id)); !os.IsNotExist(err) {
		t.Errorf("the holder's directory still exists after the stop: %v", err)
	}
}
