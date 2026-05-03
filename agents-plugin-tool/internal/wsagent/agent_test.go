package wsagent

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 5, 3, 14, 0, 0, 0, time.UTC)

type fakeRunner struct {
	calls []RunnerRequest
}

func (f *fakeRunner) Call(req RunnerRequest) (RunnerResult, error) {
	f.calls = append(f.calls, req)
	session := req.SessionID
	if session == "" {
		session = "thread-1"
	}
	return RunnerResult{
		SessionID: session,
		Text:      "reply: " + req.Prompt + "\n",
	}, nil
}

func TestRegisterCreatesAgentDirectory(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
	})

	agent, layout, err := manager.Register(RegisterOptions{
		Root:             repo,
		Name:             "skeleton writer",
		Backend:          "codex",
		Tier:             "core",
		Model:            "gpt-test",
		PromptRefs:       []string{"skeleton-writer"},
		SystemPromptText: "system prompt\n",
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	if filepath.Base(layout.AgentDir) != "skeleton-writer" {
		t.Fatalf("agent dir = %q", layout.AgentDir)
	}
	if agent.Status != StatusIdle || agent.SessionID != "" {
		t.Fatalf("unexpected agent state: %+v", agent)
	}
	if agent.SystemPromptPath != "system.md" {
		t.Fatalf("system prompt path = %q", agent.SystemPromptPath)
	}
	for _, path := range []string{layout.AgentFile, layout.SystemFile, layout.EventsFile} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
}

func TestCallCreatesAndResumesSession(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	runner := &fakeRunner{}
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
		Runner:    runner,
	})
	if _, _, err := manager.Register(RegisterOptions{Root: repo, Name: "impl", SystemPromptText: "sys"}); err != nil {
		t.Fatal(err)
	}

	agent, text, err := manager.Call(CallOptions{Root: repo, Name: "impl", Prompt: "first"})
	if err != nil {
		t.Fatalf("first Call returned error: %v", err)
	}
	if text != "reply: first\n" || agent.SessionID != "thread-1" || agent.Status != StatusIdle {
		t.Fatalf("first call mismatch: agent=%+v text=%q", agent, text)
	}
	if len(runner.calls) != 1 || runner.calls[0].SessionID != "" || runner.calls[0].SystemPromptPath == "" {
		t.Fatalf("first runner call mismatch: %+v", runner.calls)
	}

	agent, text, err = manager.Call(CallOptions{Root: repo, Name: "impl", Prompt: "second"})
	if err != nil {
		t.Fatalf("second Call returned error: %v", err)
	}
	if text != "reply: second\n" || agent.SessionID != "thread-1" {
		t.Fatalf("second call mismatch: agent=%+v text=%q", agent, text)
	}
	if len(runner.calls) != 2 || runner.calls[1].SessionID != "thread-1" {
		t.Fatalf("resume runner call mismatch: %+v", runner.calls)
	}
	printed, err := manager.Print(repo, "impl")
	if err != nil {
		t.Fatalf("Print returned error: %v", err)
	}
	if printed != "reply: second\n" {
		t.Fatalf("printed = %q", printed)
	}
}

func TestOneShotErasesAgentDirectory(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
		Runner:    &fakeRunner{},
	})
	text, err := manager.OneShot(OneShotOptions{Root: repo, Name: "tmp", Prompt: "hello"})
	if err != nil {
		t.Fatalf("OneShot returned error: %v", err)
	}
	if text != "reply: hello\n" {
		t.Fatalf("oneshot text = %q", text)
	}
	layout, err := manager.layout(repo, "tmp", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(layout.AgentDir); !os.IsNotExist(err) {
		t.Fatalf("agent dir still exists or stat failed differently: %v", err)
	}
}

func TestParseCodexJSONL(t *testing.T) {
	raw := []byte(strings.Join([]string{
		`{"type":"thread.started","thread_id":"019test"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"hello"}}`,
	}, "\n"))
	result, err := parseCodexJSONL(raw)
	if err != nil {
		t.Fatalf("parseCodexJSONL returned error: %v", err)
	}
	if result.SessionID != "019test" || result.Text != "hello" {
		t.Fatalf("result mismatch: %+v", result)
	}
}

func TestAgentJSONRoundTripIncludesContractFields(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{CacheHome: cache, Now: func() time.Time { return testNow }})
	_, layout, err := manager.Register(RegisterOptions{Root: repo, Name: "reviewer", PromptRefs: []string{"code-reviewer"}})
	if err != nil {
		t.Fatal(err)
	}
	var agent Agent
	raw, err := os.ReadFile(layout.AgentFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &agent); err != nil {
		t.Fatal(err)
	}
	if agent.SchemaVersion != 1 || agent.LastOutputPath != "output.md" || !agent.Capabilities["resume"] {
		t.Fatalf("contract fields missing: %+v", agent)
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.invalid")
	runGit(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "init")
	return repo
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}
