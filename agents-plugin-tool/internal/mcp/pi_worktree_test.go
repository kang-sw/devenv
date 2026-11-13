package mcp

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsgit"
)

// Exercise both pi.spawn paths with real keys and Git worktrees, without a
// Pi installation: cwd must control the key's root, not just the process cwd.
func TestPiSpawnBindsKeysToCwdWorktree(t *testing.T) {
	setPiVersion(t, "1.0.4", nil)
	setupMailboxTestEnv(t)
	t.Setenv(envConfigManifests, wsManifests())
	cfg := piTierOptions()
	for _, tier := range []string{"small", "medium"} {
		if _, err := wsconfig.SetGlobalAgentsTierForHarness(cfg, tier, "", "prov/model", wsagent.PiTierHarness, "low"); err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	initGit(t, root)
	runGit(t, root, "checkout", "-b", "lead")
	runGit(t, root, "commit", "--allow-empty", "-m", "initial")
	root, err := canonicalGitRoot(wsgit.ExecRunner{}, root)
	if err != nil {
		t.Fatal(err)
	}
	worktree := func(branch string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), branch)
		runGit(t, root, "worktree", "add", "-b", branch, path)
		canonical, err := canonicalGitRoot(wsgit.ExecRunner{}, path)
		if err != nil {
			t.Fatal(err)
		}
		return canonical
	}
	workerRoot, nestedRoot := worktree("worker"), worktree("nested")
	for _, dir := range []string{root, workerRoot, nestedRoot} {
		if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	backend := &routeBackend{}
	dir := t.TempDir()
	keys := NewAgentKeyStore()
	runner := wsagent.NewRunner(wsagent.Options{
		Dir: dir, Backends: map[string]wsagent.Backend{wsagent.BackendPi: backend}, Keys: keys, Config: cfg,
		WSMCP: func(root string) (wsagent.MCPCommand, error) {
			return wsagent.MCPCommand{Command: "/bin/ws-mcp", Args: []string{"serve", "--stdio", "--root", root}}, nil
		},
	})
	daemon := httptest.NewServer(runner.Handler())
	defer daemon.Close()
	runner.SetEndpoint(daemon.URL)
	runner.Start(nil)
	defer runner.Close()
	s := NewServerWithOptions(ServerOptions{Root: root, Version: "test", Agents: runner})
	defer s.Close()
	leadKey := mailboxLogin(t, s, 1, root)

	start := 0
	check := func(child piChildOut, wantRoot, wantCwd, branch string, parent wsagent.ParentRef) wsagent.StartSpec {
		t.Helper()
		spec, proc := backend.waitStart(t, start)
		start++
		proc.nextSent(t)
		rec, err := wsagent.ReadRecord(dir, child.ID)
		if err != nil {
			t.Fatal(err)
		}
		entry, ok := s.sessions.lookup(rec.WSKey)
		if !ok || entry.root != wantRoot || entry.scope != roleDelegate || entry.parent != "" {
			t.Fatalf("child key = %+v, found=%v; want parentless delegate at %s", entry, ok, wantRoot)
		}
		cwd := spec.Cwd
		if cwd == "" {
			cwd = spec.Root
		}
		if rec.Root != wantRoot || rec.Parent == nil || *rec.Parent != parent || spec.Root != wantRoot || cwd != wantCwd || child.Cwd != wantCwd {
			t.Fatalf("root/cwd/ownership mismatch: record=%+v start=%+v child=%+v", rec, spec, child)
		}
		if spec.WSMCP == nil || spec.WSMCP.Args[len(spec.WSMCP.Args)-1] != wantRoot || spec.WSMCP.Env[wsagent.EnvPiToken] == "" {
			t.Fatalf("attached MCP root or capability token = %+v", spec.WSMCP)
		}
		instructions, err := os.ReadFile(spec.Instructions)
		if err != nil || !strings.Contains(string(instructions), rec.WSKey) {
			t.Fatalf("the harness must receive its worktree-bound key: %s, %v", instructions, err)
		}
		if err := keys.Ensure(wantRoot, rec.WSKey); err != nil {
			t.Fatalf("Ensure must agree with Mint: %v", err)
		}
		var status wsgit.StatusResult
		raw := callToolWithKey(t, s, 2, rec.WSKey, "git.status", map[string]any{"format": "json"})
		if err := json.Unmarshal([]byte(raw), &status); err != nil || status.Branch.Head != branch {
			t.Fatalf("child's git.status = %s, %v; want branch %s", raw, err, branch)
		}
		return spec
	}
	parent := wsagent.ParentRef{Kind: wsagent.ParentLead, ReplyID: s.mailboxReplyID(leadKey)}
	var worker piChildOut
	var workerSpec wsagent.StartSpec
	for _, tc := range []struct{ name, cwd, root, branch string }{
		{"default", "", root, "lead"},
		{"lead root", root, root, "lead"},
		{"lead subdir", "sub", root, "lead"},
		{"worktree root", workerRoot, workerRoot, "worker"},
		{"worktree subdir", filepath.Join(workerRoot, "sub"), workerRoot, "worker"},
		{"plain folder", t.TempDir(), root, "lead"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := callToolWithKey(t, s, 3, leadKey, "pi.spawn", map[string]any{"prompt": "work", "tier": "medium", "cwd": tc.cwd, "format": "json"})
			var child piChildOut
			if err := json.Unmarshal([]byte(raw), &child); err != nil {
				t.Fatalf("pi.spawn = %s: %v", raw, err)
			}
			cwd := tc.cwd
			if !filepath.IsAbs(cwd) {
				cwd = filepath.Join(root, cwd)
			}
			spec := check(child, tc.root, cwd, tc.branch, parent)
			if tc.name == "worktree subdir" {
				worker, workerSpec = child, spec
			}
		})
	}

	// The worker's real delegate key and capability token forward a nested
	// spawn: defaults stay in its cwd; another worktree gets its own key root.
	t.Setenv(wsagent.EnvPiToken, workerSpec.WSMCP.Env[wsagent.EnvPiToken])
	t.Setenv(wsagent.EnvPiEndpoint, daemon.URL)
	forward := NewServer(root, "test")
	defer forward.Close()
	rec, err := wsagent.ReadRecord(dir, worker.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, cwd := range []string{"", filepath.Join(nestedRoot, "sub")} {
		raw := callToolWithKey(t, forward, 4, rec.WSKey, "pi.spawn", map[string]any{"prompt": "nested work", "tier": "small", "cwd": cwd, "format": "json"})
		var child piChildOut
		if err := json.Unmarshal([]byte(raw), &child); err != nil {
			t.Fatalf("nested pi.spawn = %s: %v", raw, err)
		}
		wantRoot, branch := nestedRoot, "nested"
		if cwd == "" {
			wantRoot, branch, cwd = workerRoot, "worker", filepath.Join(workerRoot, "sub")
		}
		check(child, wantRoot, cwd, branch, wsagent.ParentRef{Kind: wsagent.ParentConversation, ReplyID: worker.ID})
	}
}
