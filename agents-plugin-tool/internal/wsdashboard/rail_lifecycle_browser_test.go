package wsdashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// fourStateAgents is a canned agent host for the four-state rail browser
// test: GET /agents/<id> answers a controllable lead, and POST
// /agents/<id>/hide|unhide writes the dashboard's real visibility sidecar
// (WriteSessionHidden) and re-registers the row asleep, as the runner's
// SetHidden does. The id busyID answers hide with the runner's retryable
// 409. The runner's own gates are covered in internal/wsagent.
type fourStateAgents struct {
	dir    string
	busyID string
	mu     sync.Mutex
	notify func(replyID, project string)
	posts  []string
}

func (a *fourStateAgents) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, AgentRoutePrefix+"/")
		id, action, _ := strings.Cut(rest, "/")
		if !ValidReplyID(id) {
			http.NotFound(w, r)
			return
		}
		state := func() {
			hidden, _ := ReadSessionHidden(a.dir, id)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "backend": "claude", "status": "running", "can_control": true, "process": false, "hidden": hidden, "model": "claude-opus-5-5", "permission_mode": "auto"})
		}
		if r.Method == http.MethodGet && action == "" {
			state()
			return
		}
		if r.Method != http.MethodPost || (action != "hide" && action != "unhide") {
			http.Error(w, "unknown agent route", http.StatusNotFound)
			return
		}
		a.mu.Lock()
		a.posts = append(a.posts, id+"/"+action)
		notify := a.notify
		a.mu.Unlock()
		if action == "hide" && id == a.busyID {
			http.Error(w, "the session holder is still being scanned; try again", http.StatusConflict)
			return
		}
		if err := WriteSessionHidden(a.dir, id, action == "hide"); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		reg, err := ReadRegistration(a.dir, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		reg.HostStopped, reg.HostExitReason, reg.HeartbeatAt = true, "stop", stampFor(time.Now())
		if err := WriteRegistration(a.dir, reg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if notify != nil {
			notify(id, reg.Project)
		}
		state()
	})
}

func (a *fourStateAgents) Start(notify func(replyID, project string)) {
	a.mu.Lock()
	a.notify = notify
	a.mu.Unlock()
}
func (a *fourStateAgents) InboxChanged(string) {}
func (a *fourStateAgents) Close()              {}

// fourStateSession is one seeded rail row.
type fourStateSession struct {
	id, project, label, harness string
	hosted, asleep, hidden      bool
	// endedAgo ends the session that long ago (zero: live).
	endedAgo time.Duration
	reply    string
	user     bool
	snap     *wslive.Snapshot
}

// seedFourStateRail registers the mockup's rows: two repositories, active,
// asleep, archived, and ended (pending-deletion) sessions.
func seedFourStateRail(t *testing.T, dir, devenv, game string) []fourStateSession {
	t.Helper()
	day := 24 * time.Hour
	left := func(d time.Duration) time.Duration { return RegistryRetention - d - 30*time.Minute }
	now := time.Now()
	working := &wslive.Snapshot{SchemaVersion: 1, Adapter: "claude", UpdatedAt: now.UnixMilli(), HeartbeatMs: 30000,
		Lead:   wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseThinking}},
		Agents: []wslive.Agent{{ID: "w1", Status: wslive.StatusRunning}, {ID: "w2", Status: wslive.StatusRunning}}}
	approval := &wslive.Snapshot{SchemaVersion: 1, Adapter: "claude", UpdatedAt: now.UnixMilli(), HeartbeatMs: 30000,
		Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhasePermission, Permission: &wslive.Permission{ToolName: "Bash", ArgsPreview: "cargo build --release -p renderer"}}}}
	rows := []fourStateSession{
		{id: "mainlead", project: devenv, label: "메인", harness: "claude", hosted: true, snap: working, reply: "Whiteboard: 네 가지 상태 목업을 만들었습니다. 레일 화면 6장과 범례 카드를 붙였고, 승인 후 구현 티켓으로 옮기겠습니다."},
		{id: "reviewer", project: devenv, label: "리뷰어", harness: "claude", hosted: true, asleep: true},
		{id: "sosterm", project: devenv, label: "비상복구", harness: "pi", reply: "플러그인을 다시 불러온 뒤에도 ws MCP가 응답하는 것을 확인했습니다."},
		{id: "docaudit", project: devenv, label: "문서 감사", harness: "claude", hosted: true, asleep: true, hidden: true},
		{id: "csidesign", project: devenv, label: "CSI 설계", harness: "pi", hosted: true, asleep: true, hidden: true},
		{id: "brandmock", project: devenv, label: "브랜드 목업", harness: "claude", hosted: true, asleep: true, hidden: true},
		{id: "release49", project: devenv, label: "릴리스 0.49", harness: "claude", hosted: true, endedAgo: left(6 * day)},
		{id: "mk936tidy", project: devenv, label: "mk936 정리", harness: "claude", hosted: true, endedAgo: left(4 * day)},
		{id: "execsecond", project: devenv, label: "EXEC 2", harness: "pi", hosted: true, endedAgo: left(2 * day)},
		{id: "termtrial", project: devenv, label: "터미널 실험", harness: "claude", endedAgo: left(day)},
		{id: "hotfixprobe", project: devenv, label: "핫픽스 조사", harness: "pi", hosted: true, endedAgo: left(9 * time.Hour)},
		{id: "renderer", project: game, label: "렌더러", harness: "claude", hosted: true, snap: approval},
		{id: "assettidy", project: game, label: "에셋 정리", harness: "claude", hosted: true, asleep: true},
		{id: "shaderlab", project: game, label: "셰이더 실험", harness: "pi", reply: "normal map 경로가 바뀐 이유를 확인해 줘", user: true},
		{id: "rendererone", project: game, label: "렌더러 v1", harness: "claude", hosted: true, endedAgo: left(5 * day)},
		{id: "perfmeasure", project: game, label: "perf 측정", harness: "pi", endedAgo: left(3 * day)},
	}
	for i, s := range rows {
		at := now.Add(-time.Duration(len(rows)-i) * time.Minute)
		reg := Registration{ReplyID: s.id, Project: s.project, Harness: s.harness, Hosted: s.hosted, RegisteredAt: stampFor(at), HeartbeatAt: stampFor(now)}
		if s.asleep {
			reg.HostStopped, reg.HostExitReason = true, "stop"
		}
		if s.endedAgo > 0 {
			ended := stampFor(now.Add(-s.endedAgo))
			reg.RegisteredAt, reg.HeartbeatAt, reg.EndedAt = ended, ended, ended
		}
		if err := WriteRegistration(dir, reg); err != nil {
			t.Fatal(err)
		}
		if err := writeSessionLabel(SessionDir(dir, s.id), s.label); err != nil {
			t.Fatal(err)
		}
		if s.hidden {
			if err := WriteSessionHidden(dir, s.id, true); err != nil {
				t.Fatal(err)
			}
		}
		if s.snap != nil {
			writeSnapshot(t, dir, s.id, s.snap)
		}
		if s.reply != "" {
			kind := wslive.RingReply
			if s.user {
				kind = wslive.RingUser
			}
			if err := wslive.AppendRing(wslive.RingDir(SessionDir(dir, s.id)), []wslive.RingEntry{{Kind: kind, Text: s.reply}}); err != nil {
				t.Fatal(err)
			}
		}
	}
	return rows
}

// TestRailFourStateBrowser drives the real rail in Chromium against a
// disposable daemon on an ephemeral port: the fold row's tri-state radio,
// archive moving a row into the archived fold and staying there across a
// reload, unarchive returning it asleep, a 409 archive leaving the row put
// with its message, and the row menu's archive entry. With
// WS_FOUR_STATE_SHOTS set to an absolute directory it also saves the
// mockup-matched screenshots there (1440px and 700px, dark and light).
func TestRailFourStateBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	shots := os.Getenv("WS_FOUR_STATE_SHOTS")
	if shots != "" && !filepath.IsAbs(shots) {
		t.Fatal("WS_FOUR_STATE_SHOTS must be an absolute directory")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	t.Setenv("WS_CONFIG_HOME", t.TempDir())
	SetDevAssets(nil)
	dir, root := t.TempDir(), t.TempDir()
	devenv, game := filepath.Join(root, "devenv"), filepath.Join(root, "gunpowder-odyssey")
	for _, p := range []string{devenv, game} {
		// Rail repository recognition needs no Git process.
		if err := os.MkdirAll(filepath.Join(p, ".git"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	seedFourStateRail(t, dir, devenv, game)
	agents := &fourStateAgents{dir: dir, busyID: "assettidy"}
	d := startIndexedDaemon(t, dir, newPageFake(), time.Hour, func(o *DaemonOptions) {
		o.Port = 0
		o.LAN = false
		o.bindHost = "127.0.0.1"
		o.noGitBadge = true
		o.Agents = agents
	})
	base := endpointURL(d.Endpoint(), "")
	if u, err := url.Parse(base); err != nil || u.Port() == "29417" {
		t.Fatal("fixture must use an ephemeral port, never the shared daemon port")
	}
	waitFor(t, "the four-state sessions to be indexed", func() bool {
		s, ok := d.Index().Session("docaudit")
		return ok && s.Hidden
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/rail-four-state.browser.cjs", base, shots)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("Chromium four-state rail acceptance: %v", err)
	}
	hidden, err := ReadSessionHidden(dir, "reviewer")
	if err != nil || hidden {
		t.Errorf("reviewer sidecar hidden = %v, %v; want unarchived at the end", hidden, err)
	}
	if hidden, _ := ReadSessionHidden(dir, "assettidy"); hidden {
		t.Error("a refused (409) archive wrote the sidecar")
	}
	agents.mu.Lock()
	posts := strings.Join(agents.posts, ",")
	agents.mu.Unlock()
	if strings.Count(posts, "assettidy/hide") != 1 {
		t.Errorf("posts = %s; want exactly one refused archive and no retry", posts)
	}
}
