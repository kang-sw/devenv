package wsdashboard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsconfigservice"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// Run with WS_PLAYWRIGHT_MODULE pointing at an installed Playwright module and
// its Chromium downloaded. WS_CONFIG_SETTINGS_CAPTURES optionally keeps the
// global and repo pairs at 1440×960; the default is a disposable directory.
func RunSettingsVisualBrowser(t *testing.T, backend *ConfigBackend) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run the settings Chromium acceptance")
	}
	base, oldRoot := worktreeRepo(t)
	root := filepath.Join(base, "devenv")
	if err := os.Rename(oldRoot, root); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "branch", "-m", "develop")
	linked := filepath.Join(base, "config-git-sync")
	gitRun(t, root, "worktree", "add", "-q", "-b", "feat/config-git-sync", linked)
	second := filepath.Join(base, "my-app")
	gitRun(t, base, "init", "-q", "-b", "main", second)
	gitCommit(t, second, "a")

	opts := wsconfig.Options{ConfigHome: t.TempDir(), CacheHome: t.TempDir(), RepoRoot: root}
	global, err := wsconfig.GlobalPath(opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(global, []byte(`{"overrides":{"workflow.lang":"Korean","workflow.profile.all":"direct","workflow.profile.claude":"orchestrate","workflow.prefer_subagent":"on","workflow.tracker_enabled.all":"off","claude.compaction_advisory_percent":"60%"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	repo, _ := wsconfig.RepoPath(opts)
	if err := os.MkdirAll(filepath.Dir(repo), 0o700); err != nil {
		t.Fatal(err)
	}
	repoContents := `{"overrides":{"workflow.lang":"Korean","workflow.whiteboard_enabled.all":"on"},"structured_overrides":{"agent.profile.reviewer-readonly":{"description":"Read-only code reviewer","backend":"claude","model":"sonnet","mcp_servers":{"test":{"type":"stdio","command":"/private/bin/node","args":["--api-key=secret-123"],"url":"https://user:secret@example.com/mcp?token=abc","env":{"PASSWORD":"secret-env"}}}}}}`
	if err := os.WriteFile(repo, []byte(repoContents), 0o600); err != nil {
		t.Fatal(err)
	}
	opts.RepoRoot = ""
	resolver := wsconfig.NewResolver(opts, nil, nil, nil)
	if err := resolver.Set("workflow.whiteboard_enabled.claude", "on", wsconfig.SetOptions{ExplicitScope: wsconfig.ScopeProject}); err != nil {
		t.Fatal(err)
	}
	backend.Options = opts
	backend.IsLead = nil // Session rows below reproduce the mockup's data.
	backend.Session = func(Registration) (wsconfigservice.SessionLayer, wsconfig.SessionWriter, string, bool) {
		return wsconfigservice.SessionLayer{Effective: map[string]string{"workflow.lang": "Korean"}}, nil, "never-return-this-session-key", true
	}
	dir := filepath.Join(t.TempDir(), "dashboard")
	src := newPageFake()
	now := time.Now()
	reg := register(t, dir, "scarinessclerkenergy", root, now)
	src.setPage(reg.ReplyID, PageContent{})
	secondReg := register(t, dir, "otherlead", second, now)
	src.setPage(secondReg.ReplyID, PageContent{})
	if err := writeSessionLabel(SessionDir(dir, reg.ReplyID), "scariness-clerk-energy"); err != nil {
		t.Fatal(err)
	}
	if err := wslive.AppendRing(wslive.RingDir(SessionDir(dir, reg.ReplyID)), []wslive.RingEntry{
		{Kind: wslive.RingUser, Origin: wslive.OriginTerminal, Text: "config-settings-ui 목업 설계 중"},
	}); err != nil {
		t.Fatal(err)
	}
	d := startIndexedDaemon(t, dir, src, time.Hour, withWorkspaces(t), func(o *DaemonOptions) {
		o.Config, o.Worktrees, o.noGitBadge = backend, gitWorktreeSource{}, true
	})
	for _, path := range []string{root, second} {
		if _, err := d.workspaces.add(path, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.recordUserWorktree(root, linked, "feat/config-git-sync"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "settings test index", func() bool {
		_, ok := d.Index().Session(reg.ReplyID)
		return d.Index().Loaded() && ok
	})
	captures := os.Getenv("WS_CONFIG_SETTINGS_CAPTURES")
	if captures == "" {
		captures = t.TempDir()
	}
	if err := os.MkdirAll(captures, 0o755); err != nil {
		t.Fatal(err)
	}
	mockup, err := filepath.Abs("../../../ai-docs/assets/mockups/261121/config-settings-ui/mockup.html")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/settings.browser.cjs", endpointURL(d.Endpoint(), ""), captures, mockup,
		settingsID("workspace", root), settingsID("worktree", root), settingsID("worktree", linked), reg.ReplyID, mustProjectConfigPath(t, opts))
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("settings Chromium acceptance: %v", err)
	}
	data, err := os.ReadFile(repo)
	if err != nil || string(data) != repoContents {
		t.Fatalf("proposal changed the repo file: %s, %v", data, err)
	}
	linkedRepo, _ := wsconfig.RepoPath(wsconfig.Options{RepoRoot: linked})
	if _, err := os.Stat(linkedRepo); !os.IsNotExist(err) {
		t.Fatalf("proposal created the linked repo config: %v", err)
	}
	store := (wsmailbox.Paths{State: wsstate.Options{CacheHome: filepath.Dir(dir)}}).Store()
	mail, err := store.ReplyQueue(reg.ReplyID)
	if err != nil || len(mail) != 1 || !strings.Contains(mail[0].Content, "workflow.lang") {
		t.Fatalf("expected one patch for the active lead: %#v, %v", mail, err)
	}
}
