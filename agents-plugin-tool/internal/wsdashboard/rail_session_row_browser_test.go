package wsdashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// Only the agent-host boundary is synthetic: creation registers an idle
// session without starting a harness or provider. The browser uses the real
// spawn popover, label/layout routes, shell, and streamed rail renders.
type sessionRowAgents struct {
	mu     sync.Mutex
	create func() (string, error)
}

func (*sessionRowAgents) Start(func(string, string)) {}
func (*sessionRowAgents) InboxChanged(string)        {}
func (*sessionRowAgents) Close()                     {}
func (a *sessionRowAgents) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"backend":"pi","status":"running","can_control":true,"process":false}`))
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/agents/leads" {
			http.NotFound(w, r)
			return
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		id, err := a.create()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
	})
}

func TestRailSessionRowBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run isolated Chromium acceptance")
	}
	shots := os.Getenv("WS_SESSION_ROW_SHOTS")
	if shots != "" && !filepath.IsAbs(shots) {
		t.Fatal("WS_SESSION_ROW_SHOTS must be absolute")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	t.Setenv("WS_CONFIG_HOME", t.TempDir())
	SetDevAssets(nil)
	dir, project := t.TempDir(), filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	src := newPageFake()
	seed := func(id, label, harness string, count int, at time.Time) error {
		if err := WriteRegistration(dir, Registration{ReplyID: id, Project: project, Harness: harness, Hosted: true, RegisteredAt: stampFor(at), HeartbeatAt: stampFor(time.Now())}); err != nil {
			return err
		}
		if err := writeSessionLabel(SessionDir(dir, id), label); err != nil {
			return err
		}
		snap := &wslive.Snapshot{SchemaVersion: 1, ReplyID: id, Adapter: harness, UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 3600000, Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseIdle}}}
		for i := 0; i < count; i++ {
			snap.Agents = append(snap.Agents, wslive.Agent{ID: fmt.Sprintf("sub-%d", i), Status: wslive.StatusRunning})
		}
		raw, err := json.Marshal(snap)
		if err != nil {
			return err
		}
		if err := os.WriteFile(wslive.SnapshotPath(SessionDir(dir, id)), raw, 0o600); err != nil {
			return err
		}
		src.set(id, SessionData{Fingerprint: at.String(), SnapshotUpdatedAt: at})
		return nil
	}
	for i, s := range []struct {
		id, label, harness string
		count              int
	}{
		{"plainrow", "메인", "pi", 0},
		{"agentsrow", "리뷰", "codex", 3},
		{"dotrow", "문서 정리", "claude", 12},
		{"kidrow", "하위 세션", "pi", 0},
		{"archivedrow", "보관된 세션", "claude", 0},
	} {
		if err := seed(s.id, s.label, s.harness, s.count, time.Now().Add(time.Duration(i-10)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	if err := WriteSessionHidden(dir, "archivedrow", true); err != nil {
		t.Fatal(err)
	}
	agents := &sessionRowAgents{}
	d := startIndexedDaemon(t, dir, src, time.Hour, func(o *DaemonOptions) {
		o.Port, o.LAN, o.bindHost = 0, false, "127.0.0.1"
		o.noGitBadge = true
		o.Agents = agents
		o.Worktrees = guideWorktrees{{Path: project, Branch: "develop"}}
		o.Support = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"harnesses":[{"harness":"pi","lead":true,"available":true,"models":[],"efforts":["off"]}]}`))
		})
	})
	n := 0
	agents.create = func() (string, error) {
		n++
		id := fmt.Sprintf("freshsession%d", n)
		if err := seed(id, "", "pi", 0, time.Now()); err != nil {
			return "", err
		}
		d.Index().Refresh()
		return id, nil
	}
	base := endpointURL(d.Endpoint(), "")
	waitFor(t, "session row fixture indexed", func() bool { _, ok := d.Index().Session("plainrow"); return ok && d.Index().Loaded() })
	if status, _, body := postJSON(t, base+railLayoutRoute, base, `{"op":"attach","rid":"kidrow","parent":"plainrow"}`); status != http.StatusOK {
		t.Fatalf("attach fixture child: %d %s", status, body)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "node", "testdata/rail-session-row.browser.cjs", base, shots).CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("Chromium session row acceptance: %v", err)
	}
}
