package mcp

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsrsrc"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// loggingGitDir writes a POSIX git shim into a fresh directory: it appends
// its arguments to logPath, then runs the real git. A hosted server whose
// environment PATH names the directory runs git through it, so the log
// says which git commands ran on the connection's environment.
func loggingGitDir(t *testing.T, logPath string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the git shim is a POSIX shell script")
	}
	real, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\necho \"$*\" >> \"$WS_TEST_GIT_LOG\"\nexec \"" + real + "\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func readGitLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(data)
}

func truncateFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

// hostedServerEnv is a hosted server's environment: this process's, with
// the ws keys dropped, its own config and cache homes, and set applied.
func hostedServerEnv(t *testing.T, name string, set map[string]string) []string {
	t.Helper()
	base := map[string]string{
		wsconfig.EnvConfigHome: filepath.Join(t.TempDir(), name+"-config"),
		wsstate.EnvCacheHome:   filepath.Join(t.TempDir(), name+"-cache"),
		wsrsrc.EnvRsrcRoot:     absRsrcRoot(t),
		envNoAgent:             "1",
		"PATH":                 os.Getenv("PATH"),
	}
	for k, v := range set {
		base[k] = v
	}
	return hostedEnv(base)
}

// TestHostedServerCanonicalizesRootsThroughItsOwnGit pins canonicalGitRoot
// on the connection's git runner: the ferrule login, a worker mint under
// it, and git.merge each resolve the root with the git on the hosted
// server's PATH, not the daemon process's.
func TestHostedServerCanonicalizesRootsThroughItsOwnGit(t *testing.T) {
	processEnvFixture(t)
	logPath := filepath.Join(t.TempDir(), "git.log")
	gitDir := loggingGitDir(t, logPath)
	root, branch := mergeFixture(t, "main")
	env := hostedServerEnv(t, "hosted", map[string]string{"PATH": gitDir, "WS_TEST_GIT_LOG": logPath})
	s := NewServerWithOptions(ServerOptions{Root: root, Version: "test", Env: env, OwnerPID: os.Getppid()})

	key, _ := parseLoginResponse(t, callLogin(t, s, 1, root, nil))
	if log := readGitLog(t, logPath); !strings.Contains(log, "rev-parse --show-toplevel") {
		t.Fatalf("ferrule login did not resolve the root through the connection's git:\n%s", log)
	}
	truncateFile(t, logPath)

	resp := callLogin(t, s, 2, root, map[string]any{"parent_session_key": key, "capability": "delegate"})
	if toolIsError(t, resp) {
		t.Fatalf("worker mint: %s", resp)
	}
	if log := readGitLog(t, logPath); !strings.Contains(log, "rev-parse --show-toplevel") {
		t.Fatalf("worker mint did not resolve the root through the connection's git:\n%s", log)
	}
	truncateFile(t, logPath)

	args := map[string]any{"session_key": key, "branch": branch, "title": "merge(test): release", "ai_context": []string{"Test explicit acknowledgement."}, "format": "json"}
	first := toolText(t, keyedCall(t, s, 3, key, "git.merge", args))
	if !strings.Contains(first, `"policy_blocked"`) {
		t.Fatalf("first git.merge = %s", first)
	}
	var r implMergeResult
	if err := json.Unmarshal([]byte(first), &r); err != nil {
		t.Fatal(err)
	}
	args["release_target_override"], args["expected_source_oid"], args["expected_target_oid"], args["format"] = true, r.SourceOID, r.TargetOID, "text"
	if out := toolText(t, keyedCall(t, s, 4, key, "git.merge", args)); !strings.Contains(out, "status: merged") {
		t.Fatalf("git.merge retry: %s", out)
	}
	log := readGitLog(t, logPath)
	if !strings.Contains(log, "rev-parse --show-toplevel") || !strings.Contains(log, "merge --no-ff") {
		t.Fatalf("git.merge did not run through the connection's git:\n%s", log)
	}
}

// TestHostedConnectionsActOnTheirOwnRoots pins the per-connection root: two
// hosted servers for different projects, each with its own WS_WORKTREE_DIR,
// answer git.status and tickets.query for their own root alone.
func TestHostedConnectionsActOnTheirOwnRoots(t *testing.T) {
	processEnvFixture(t)
	type side struct {
		name, root, key string
		server          *Server
	}
	mk := func(name string) *side {
		root := t.TempDir()
		mustWrite(t, root, "README.md", "# "+name+"\n")
		initGit(t, root)
		runGit(t, root, "add", ".")
		runGit(t, root, "commit", "-q", "-m", "init")
		mustWrite(t, root, name+".txt", "dirty\n")
		mustWrite(t, root, "ai-docs/tickets/idea/260101-feat-"+name+".md", ixTicket(strings.ToUpper(name)))
		env := hostedServerEnv(t, name, map[string]string{"WS_WORKTREE_DIR": filepath.Join(t.TempDir(), name+"-worktrees")})
		s := NewServerWithOptions(ServerOptions{Root: root, Version: "test", Env: env, OwnerPID: os.Getppid()})
		key, canonical := parseLoginResponse(t, callLogin(t, s, 1, root, nil))
		if evaluated, err := filepath.EvalSymlinks(root); err == nil && canonical != evaluated {
			t.Fatalf("%s logged in at %q, want %q", name, canonical, evaluated)
		}
		return &side{name: name, root: root, key: key, server: s}
	}
	alpha, beta := mk("alpha"), mk("beta")
	for _, pair := range []struct{ own, other *side }{{alpha, beta}, {beta, alpha}} {
		status := toolText(t, keyedCall(t, pair.own.server, 2, pair.own.key, "git.status", nil))
		if !strings.Contains(status, "dirty:") || !strings.Contains(status, pair.own.name+".txt") || strings.Contains(status, pair.other.name+".txt") {
			t.Fatalf("%s git.status = %q, want its own dirty file only", pair.own.name, status)
		}
		tickets := toolText(t, keyedCall(t, pair.own.server, 3, pair.own.key, "tickets.query", map[string]any{"statuses": []string{"idea"}}))
		if !strings.Contains(tickets, "260101-feat-"+pair.own.name) || strings.Contains(tickets, "260101-feat-"+pair.other.name) {
			t.Fatalf("%s tickets.query = %q, want its own board only", pair.own.name, tickets)
		}
	}
}

// TestHostedToolsRunGitOnTheConnectionPATH pins the subprocess sites of a
// hosted server's tools: the wsdoc board readers (project_tree's ignore
// matcher, the sparse-checkout ticket scope a tickets.query point resolve
// reads through, tickets.sage_gate's freshness history), wsreview
// (review.marker bootstrap, then the review track and
// checkpoint nudge workflow_manual renders), and wsstate's repository
// identity (path.generate, the worktree note layer note.write, note.query
// and the workflow_manual Notes block resolve, and exec.spawn's job
// directory) run git from the hosted server's environment PATH, not the
// daemon process's. The order matters: the bootstrap marker is what makes
// the checkpoint nudge consult git.
func TestHostedToolsRunGitOnTheConnectionPATH(t *testing.T) {
	processEnvFixture(t)
	logPath := filepath.Join(t.TempDir(), "git.log")
	gitDir := loggingGitDir(t, logPath)
	root := t.TempDir()
	mustWrite(t, root, "README.md", "# Test\n")
	mustWrite(t, root, "ai-docs/tickets/idea/260101-feat-alpha.md", ixTicket("Alpha"))
	// An epic whose design review is recorded without a digest: sage_gate
	// walks the ticket's git history for the baseline.
	mustWrite(t, root, "ai-docs/tickets/todo/260102-epic-beta.md", "---\ntitle: Beta\nsage-review-design: completed\n---\n# Beta\n")
	initGit(t, root)
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "init")
	// A sparse checkout makes every ticket read consult git for the scope.
	runGit(t, root, "sparse-checkout", "set", "ai-docs")
	// Not agentless: the exec tools are agent-backed.
	env := hostedServerEnv(t, "hosted", map[string]string{"PATH": gitDir, "WS_TEST_GIT_LOG": logPath, envNoAgent: ""})
	s := NewServerWithOptions(ServerOptions{Root: root, Version: "test", Env: env, OwnerPID: os.Getppid()})
	key, _ := parseLoginResponse(t, callLogin(t, s, 1, root, nil))

	const sparseScope = "config --type=bool --get core.sparseCheckout"
	for i, c := range []struct {
		tool string
		args map[string]any
		want []string
	}{
		{"project_tree", nil, []string{"rev-parse --is-inside-work-tree"}},
		{"tickets.query", map[string]any{"ticket_stem": "260101-feat-alpha"}, []string{sparseScope}},
		{"tickets.sage_gate", map[string]any{"stem": "260102-epic-beta", "landing": "todo"}, []string{"log --follow"}},
		{"review.marker", map[string]any{"bootstrap": true}, []string{"rev-parse HEAD"}},
		{"workflow_manual", nil, []string{"symbolic-ref --short refs/remotes/origin/HEAD", "rev-list --count", "--git-common-dir"}},
		{"path.generate", map[string]any{"kind": "plan", "stems": []string{"alpha"}}, []string{"--git-common-dir"}},
		{"note.write", map[string]any{"layer": "worktree", "notes": []any{map[string]any{"key": "hosted.note", "value": "v"}}}, []string{"--git-common-dir"}},
		{"note.query", map[string]any{"layer": "worktree"}, []string{"--git-common-dir"}},
		{"exec.spawn", map[string]any{"cmd": "git", "args": []any{"--version"}}, []string{"--git-common-dir"}},
	} {
		truncateFile(t, logPath)
		resp := keyedCall(t, s, 10+i, key, c.tool, c.args)
		if toolIsError(t, resp) {
			t.Fatalf("%s: %s", c.tool, resp)
		}
		log := readGitLog(t, logPath)
		for _, want := range c.want {
			if !strings.Contains(log, want) {
				t.Fatalf("%s did not run %q through the connection's git:\n%s\nresponse: %s", c.tool, want, log, resp)
			}
		}
	}
}
