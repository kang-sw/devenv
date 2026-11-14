package wsdashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// This synthetic agent-host boundary holds a canned creation response. It
// starts no harness/provider. The browser still uses the real served modal,
// routing, completion callback and shell navigation, not an extracted script.
type effectsBrowserAgents struct {
	mu      sync.Mutex
	pending chan struct{}
	count   int
}

func (a *effectsBrowserAgents) Start(func(string, string)) {}
func (a *effectsBrowserAgents) InboxChanged(string)        {}
func (a *effectsBrowserAgents) Close()                     { a.complete() }
func (a *effectsBrowserAgents) complete() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.pending != nil {
		close(a.pending)
		a.pending = nil
	}
}
func (a *effectsBrowserAgents) requests() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.count
}
func (a *effectsBrowserAgents) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/agents/leads" {
			http.NotFound(w, r)
			return
		}
		a.mu.Lock()
		if a.pending != nil {
			a.mu.Unlock()
			http.Error(w, "duplicate fixture creation", http.StatusConflict)
			return
		}
		ready := make(chan struct{})
		a.pending = ready
		a.count++
		a.mu.Unlock()
		select {
		case <-ready:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"createdsession"}`))
		case <-r.Context().Done():
		}
	})
}

// Opt in with already-installed Playwright/Chromium and a durable artifact
// directory. Merely compiling/default package tests never launch this fixture.
// All page/stream/history handlers and rendering are production code. Only
// synthetic store writes and canned agent-host choices/completion are fixtures.
func TestWhiteboardEffectsBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run isolated Chromium acceptance")
	}
	artifacts := os.Getenv("WS_WHITEBOARD_EFFECTS_ARTIFACTS")
	if !filepath.IsAbs(artifacts) {
		t.Fatal("set WS_WHITEBOARD_EFFECTS_ARTIFACTS to an absolute retained output directory")
	}
	if err := os.MkdirAll(artifacts, 0o700); err != nil {
		t.Fatal(err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	t.Setenv("WS_CONFIG_HOME", t.TempDir())
	t.Setenv("WS_MCP_DASHBOARD_DEV_SOURCE", "")
	f := &pageFixture{dir: t.TempDir(), project: t.TempDir(), src: newPageFake()}
	// Rail repository recognition needs no Git process or real repository.
	if err := os.Mkdir(filepath.Join(f.project, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"effectsession", "othersession", "createdsession"} {
		register(t, f.dir, id, f.project, time.Now())
	}
	boardState := &wswhiteboard.State{}
	body := "[Jump to target](#p1)\n\n" + strings.Repeat("Navigation context above the target.\n\n", 40)
	_, problems := wswhiteboard.Apply(boardState, `<wb:topic name="navigation" label="Navigation"><wb:body>`+body+`</wb:body><wb:panel recap="Jump target">Target panel content.</wb:panel></wb:topic>`, wswhiteboard.Env{Now: time.Now()})
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	board, err := wswhiteboard.RenderRegions(wswhiteboard.BuildView(boardState, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull}))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"effectsession", "othersession", "createdsession"} {
		f.src.setPage(id, PageContent{HasBoard: true, Seq: 1, Board: board})
	}
	agents := &effectsBrowserAgents{}
	f.d = startIndexedDaemon(t, f.dir, f.src, time.Hour, func(o *DaemonOptions) {
		o.noGitBadge = true
		o.Agents = agents
		o.Support = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/support/choices" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"harnesses":[{"harness":"claude","available":true,"models":[{"id":"fixture-model"}],"efforts":["default"]},{"harness":"pi","available":true,"models":[{"id":"fixture-model"}],"efforts":["default"]}]}`))
		})
	})
	t.Cleanup(agents.Close) // Release a held synthetic response before d.Close.
	f.base = endpointURL(f.d.Endpoint(), "")
	if u, err := url.Parse(f.base); err != nil || u.Port() == "29417" {
		t.Fatal("fixture must use an ephemeral port, never the shared daemon port")
	}
	waitFor(t, "effects browser sessions to be indexed", func() bool {
		_, ok := f.d.Index().Session("effectsession")
		return ok
	})
	ringDir := wslive.RingDir(SessionDir(f.dir, "effectsession"))
	setAdapter := func(mode string) error {
		path := wslive.SnapshotPath(SessionDir(f.dir, "effectsession"))
		if mode == "absent" {
			err := os.Remove(path)
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		updated := time.Now()
		if mode == "stale" {
			updated = updated.Add(-time.Hour)
		}
		snap := wslive.Snapshot{SchemaVersion: 1, ReplyID: "effectsession", Adapter: "pi", UpdatedAt: updated.UnixMilli(), HeartbeatMs: 30000, Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseIdle}}}
		raw, err := json.Marshal(snap)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path+".effects.tmp", raw, 0o600); err != nil {
			return err
		}
		return wsstate.AtomicReplaceFile(path+".effects.tmp", path)
	}
	var mu sync.Mutex
	seq := 1
	publish := func() {
		seq++
		f.src.setPage("effectsession", PageContent{HasBoard: true, Seq: seq, Board: board})
		f.d.Index().Refresh()
		f.d.broker.wake(ringDep("effectsession"))
	}
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		result := map[string]any{}
		var err error
		switch r.URL.Query().Get("action") {
		case "reset":
			err = os.RemoveAll(ringDir)
			if err == nil {
				err = setAdapter("absent")
			}
			valid, _ := wslive.NewWhiteboardEffects([]wswhiteboard.Effect{{Kind: "panel", Topic: "effects", EntryID: "e1", PanelID: "p1", Field: "payload", After: `<img src=x onerror="window.effectsAttack=1"><script>window.effectsAttack=1</script> hostile-effect-marker **plain text**`}})
			entries := []wslive.RingEntry{{Kind: wslive.RingUser, Text: "old-history-marker"}}
			for i := 0; i < 64; i++ {
				entries = append(entries, wslive.RingEntry{Kind: wslive.RingReply, Text: strings.Repeat(fmt.Sprintf("History %02d has enough text to create real scroll height. ", i), 8)})
			}
			entries = append(entries, valid, wslive.RingEntry{Kind: wslive.RingWhiteboardEffects, Caption: "Unsupported", Text: `{"v":99,"effects":[{"kind":"panel","topic":"effects","entryId":"e2","panelId":"p2","field":"payload","after":"<img src=x onerror=attack()> unsupported-effect-marker"}]}`})
			for i := 0; i < 8; i++ {
				entries = append(entries, wslive.RingEntry{Kind: wslive.RingReply, Text: fmt.Sprintf("Tail %d", i)})
			}
			if err == nil {
				err = wslive.AppendRing(ringDir, entries)
			}
			if err == nil {
				var names []string
				names, err = wslive.RingNames(ringDir)
				if err == nil {
					result["focus"], result["fallback"] = names[65], names[66]
				}
			}
		case "append":
			marker := fmt.Sprintf("refresh-marker-%d", seq)
			payload := `<img src=x onerror="window.effectsAttack=1"><script>window.effectsAttack=1</script> ` + marker + ` **plain text**`
			entryID := fmt.Sprintf("e%d", seq+1)
			// Model a newly projected post-commit carrier through the real
			// builder and ring writer; this does not simulate an MCP commit.
			entry, _ := wslive.NewWhiteboardEffects([]wswhiteboard.Effect{
				{Kind: "panel", Topic: "effects", EntryID: entryID, PanelID: "p1", Field: "payload", After: payload},
				{Kind: "panel", Topic: "effects", EntryID: entryID, PanelID: "p1", Field: "recap", After: marker},
			})
			err = wslive.AppendRing(ringDir, []wslive.RingEntry{entry})
			if err == nil {
				var names []string
				names, err = wslive.RingNames(ringDir)
				if err == nil && len(names) == 0 {
					err = fmt.Errorf("appended effects carrier has no ring key")
				}
				if err == nil {
					result["key"] = names[len(names)-1]
				}
			}
			result["marker"], result["payload"] = marker, payload
		case "live", "stale", "absent":
			err = setAdapter(r.URL.Query().Get("action"))
		case "complete":
			agents.complete()
		case "requests":
			result["count"] = agents.requests()
		default:
			http.Error(w, "unknown fixture action", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if action := r.URL.Query().Get("action"); action != "requests" && action != "complete" {
			publish()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}))
	t.Cleanup(control.Close)
	if u, err := url.Parse(control.URL); err != nil || u.Port() == "29417" {
		t.Fatal("fixture control must never use the shared daemon port")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/whiteboard-effects-browser.cjs", f.base, control.URL, artifacts)
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if writeErr := os.WriteFile(filepath.Join(artifacts, "browser-process.log"), out, 0o600); writeErr != nil {
		t.Error(writeErr)
	}
	if err != nil {
		t.Fatalf("isolated effects browser acceptance: %v", err)
	}
}
