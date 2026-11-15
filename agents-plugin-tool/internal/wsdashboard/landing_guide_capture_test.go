package wsdashboard

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// guideAgents is the capture fixture's agent host: it makes the rail offer
// the new-session button and answers the spawn popover's Start with the
// fixture's fresh session. It starts no harness.
type guideAgents struct{}

func (a *guideAgents) Start(func(string, string)) {}
func (a *guideAgents) InboxChanged(string)        {}
func (a *guideAgents) Close()                     {}
func (a *guideAgents) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/agents/leads" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"` + guideFresh + `"}`))
			return
		}
		http.NotFound(w, r)
	})
}

// guideWorktrees is the capture fixture's worktree list: no git runs.
type guideWorktrees []RepoWorktree

func (w guideWorktrees) ReadWorktrees(Registration) (WorktreePool, error) { return WorktreePool{}, nil }
func (w guideWorktrees) ReadRepoWorktrees(string, []Registration) (RepoWorktrees, error) {
	return RepoWorktrees{Worktrees: w}, nil
}

// The capture fixture's sessions: the one the guide works in, a fresh one
// the spawn popover opens, and a neighbour so the rail is not a single row.
const (
	guideMain  = "tidyotter"
	guideFresh = "brightfinch"
	guideOther = "calmheron"
)

// guideText is the fixture's agent-authored content in one language.
type guideText struct {
	ask, reply, topic, label, body, title, why, optA, optB, rec, other, otherMsg string
}

var guideTexts = map[string]guideText{
	"ko": {
		ask: "화이트보드 사용해주세요", reply: "네, 로그인 화면 개선 계획을 화이트보드에 정리했습니다. 결정이 필요한 항목 하나에 답해 주세요.",
		topic: "login", label: "로그인 화면 개선", body: "## 목표\n\n- 첫 화면에서 바로 로그인\n- 오류 메시지를 입력칸 옆에 표시\n\n## 진행\n\n1. 현재 흐름 정리 (완료)\n2. 화면 시안\n3. 구현과 테스트",
		title: "비밀번호 찾기 링크를 어디에 둘까요?", why: "지금은 화면 맨 아래에 있어 잘 보이지 않습니다.",
		optA: "비밀번호 입력칸 바로 아래", optB: "로그인 버튼 옆", rec: "a",
		other: "문서 정리", otherMsg: "README 업데이트했습니다",
	},
	"en": {
		ask: "Please use the whiteboard.", reply: "Sure. I laid out the login screen plan on the whiteboard. One item needs your answer.",
		topic: "login", label: "Login screen", body: "## Goal\n\n- Sign in straight from the first screen\n- Show errors next to the field\n\n## Progress\n\n1. Map the current flow (done)\n2. Screen draft\n3. Build and test",
		title: "Where should the forgot-password link go?", why: "It sits at the very bottom today and is easy to miss.",
		optA: "Right under the password field", optB: "Next to the sign-in button", rec: "a",
		other: "Docs tidy-up", otherMsg: "Updated the README",
	},
}

// TestLandingGuideCapture regenerates the landing guide's screenshots
// (assets/guide) from the real dashboard over disposable fixture sessions;
// testdata/landing-guide.capture.cjs documents the command. It never runs
// by default: it writes into the source tree.
func TestLandingGuideCapture(t *testing.T) {
	out := os.Getenv("WS_GUIDE_CAPTURE")
	if out == "" || os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_GUIDE_CAPTURE (absolute output directory) and WS_PLAYWRIGHT_MODULE to capture the landing guide")
	}
	if !filepath.IsAbs(out) {
		t.Fatal("WS_GUIDE_CAPTURE must be absolute")
	}
	for _, lang := range []string{"ko", "en"} {
		t.Run(lang, func(t *testing.T) { captureGuide(t, lang, out) })
	}
}

func captureGuide(t *testing.T, lang, out string) {
	text := guideTexts[lang]
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	t.Setenv("WS_CONFIG_HOME", t.TempDir())
	SetDevAssets(nil)
	// A repository with a second worktree on a feature branch, where the
	// guide's sessions run; the rail reads the list from guideWorktrees.
	project := filepath.Join(home, "projects", "my-app")
	login := filepath.Join(home, "projects", "my-app-login")
	for name, body := range map[string]string{
		filepath.Join(project, ".git", "HEAD"): "ref: refs/heads/main\n", filepath.Join(project, "README.md"): "# my-app\n",
		filepath.Join(login, "README.md"): "# my-app\n", filepath.Join(login, "go.mod"): "module my-app\n",
		filepath.Join(login, "src", "login.go"): "package src\n", filepath.Join(login, "src", "login_test.go"): "package src\n",
		filepath.Join(login, "docs", "plan.md"): "# Plan\n",
		// The folder the add-workspace step types into the popover.
		filepath.Join(home, "projects", "web-shop", "README.md"): "# web-shop\n",
	} {
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f := &pageFixture{dir: t.TempDir(), project: login, src: newPageFake()}
	now := time.Now()
	for id, root := range map[string]string{guideMain: login, guideFresh: login, guideOther: project} {
		reg := register(t, f.dir, id, root, now)
		reg.GitCommonDir = filepath.Join(project, ".git")
		if err := WriteRegistration(f.dir, reg); err != nil {
			t.Fatal(err)
		}
	}

	state := &wswhiteboard.State{}
	outcome, problems := wswhiteboard.Apply(state, `<wb:topic name="`+text.topic+`" label="`+text.label+`"><wb:body>`+text.body+`</wb:body>`+
		`<wb:add class="policy"><wb:title>`+text.title+`</wb:title><wb:option>`+text.optA+`</wb:option><wb:option>`+text.optB+`</wb:option>`+
		`<wb:recommend>`+text.rec+`</wb:recommend><wb:why>`+text.why+`</wb:why></wb:add></wb:topic>`, wswhiteboard.Env{Now: now})
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	board, err := wswhiteboard.RenderRegions(wswhiteboard.BuildView(state, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull}))
	if err != nil {
		t.Fatal(err)
	}
	f.src.setPage(guideMain, PageContent{HasBoard: true, Seq: 2, Board: board})
	f.src.setPage(guideOther, PageContent{HasBoard: false, Seq: 1})
	f.src.setPage(guideFresh, PageContent{HasBoard: false, Seq: 1})
	effects, ok := wslive.NewWhiteboardEffects(outcome.Effects)
	if !ok {
		t.Fatal("no whiteboard effects")
	}
	if err := wslive.AppendRing(wslive.RingDir(SessionDir(f.dir, guideMain)), []wslive.RingEntry{
		{Kind: wslive.RingUser, Origin: wslive.OriginWhiteboard, Text: text.ask},
		effects,
		{Kind: wslive.RingReply, Text: text.reply},
	}); err != nil {
		t.Fatal(err)
	}
	if err := wslive.AppendRing(wslive.RingDir(SessionDir(f.dir, guideOther)), []wslive.RingEntry{
		{Kind: wslive.RingUser, Text: text.otherMsg},
	}); err != nil {
		t.Fatal(err)
	}
	idle := func(id string) {
		writeSnapshot(t, f.dir, id, &wslive.Snapshot{SchemaVersion: 1, Adapter: "claude", UpdatedAt: now.UnixMilli(), HeartbeatMs: 30000,
			Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseIdle}}})
	}
	idle(guideMain)
	idle(guideOther)
	idle(guideFresh)

	f.d = startIndexedDaemon(t, f.dir, f.src, time.Hour, func(o *DaemonOptions) {
		o.noGitBadge = true
		o.Port = 0
		o.LAN = false
		o.bindHost = "127.0.0.1"
		o.Agents = &guideAgents{}
		// Stored workspaces put the add-workspace + on the rail's header.
		o.WorkspacesFile = filepath.Join(t.TempDir(), "workspaces.json")
		o.Worktrees = guideWorktrees{{Path: project, Branch: "main"}, {Path: login, Branch: "feat/login"}}
		o.Support = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/support/choices" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"harnesses":[{"harness":"claude","available":true,"models":[{"id":"opus"},{"id":"sonnet"}],"efforts":["low","medium","high"]},{"harness":"pi","available":true,"models":[],"efforts":[]}]}`))
		})
	})
	f.base = endpointURL(f.d.Endpoint(), "")
	if u, err := url.Parse(f.base); err != nil || u.Port() == "29417" {
		t.Fatal("the capture must use an ephemeral port, never the shared daemon's")
	}
	waitFor(t, "the capture sessions to be indexed", func() bool {
		for _, id := range []string{guideMain, guideOther, guideFresh} {
			if _, ok := f.d.Index().Session(id); !ok {
				return false
			}
		}
		return true
	})
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/landing-guide.capture.cjs", f.base, lang, out, guideMain, guideFresh, text.ask, home)
	res, err := cmd.CombinedOutput()
	t.Log(string(res))
	if err != nil {
		t.Fatalf("landing guide capture: %v", err)
	}
}
