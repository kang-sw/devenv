package wsagent

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

var testNow = time.Date(2026, 5, 3, 14, 0, 0, 0, time.UTC)

type fakeRunner struct {
	calls         []RunnerRequest
	systemPrompts []string
}

func (f *fakeRunner) Call(req RunnerRequest) (RunnerResult, error) {
	f.calls = append(f.calls, req)
	if req.SystemPromptPath != "" {
		raw, err := os.ReadFile(req.SystemPromptPath)
		if err == nil {
			f.systemPrompts = append(f.systemPrompts, string(raw))
		}
	}
	session := req.SessionID
	if session == "" {
		session = "thread-1"
	}
	return RunnerResult{
		SessionID: session,
		Text:      "reply: " + req.Prompt + "\n",
	}, nil
}

type errorRunner struct {
	err error
}

func (r errorRunner) Call(RunnerRequest) (RunnerResult, error) {
	if r.err != nil {
		return RunnerResult{}, r.err
	}
	return RunnerResult{}, errors.New("runner failed")
}

type panicRunner struct{}

func (panicRunner) Call(RunnerRequest) (RunnerResult, error) {
	panic("runner panic")
}

func writeBackendShim(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if runtime.GOOS == "windows" {
		path += ".exe"
	}
	if err := os.WriteFile(path, []byte(""), 0o755); err != nil {
		t.Fatalf("write backend shim: %v", err)
	}
	return path
}

func writeFakeClaudeExecutable(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "claude")
	if runtime.GOOS == "windows" {
		path += ".cmd"
		body := `@echo off
echo %*>>"%CLAUDE_FAKE_LOG%"
if "%CLAUDE_FAKE_FAIL%"=="1" (
  echo login required 1>&2
  exit /b 7
)
echo {^"result^":^"claude reply^",^"is_error^":false}
`
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatalf("write fake claude cmd: %v", err)
		}
		return path
	}
	body := `#!/bin/sh
printf '%s\n' "$@" >> "$CLAUDE_FAKE_LOG"
if [ "$CLAUDE_FAKE_FAIL" = "1" ]; then
  echo "login required" >&2
  exit 7
fi
printf '{"result":"claude reply","is_error":false}\n'
`
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}
	return path
}

type sessionPersistRunner struct {
	t         *testing.T
	agentFile string
}

func (r sessionPersistRunner) Call(req RunnerRequest) (RunnerResult, error) {
	if req.OnSessionID != nil {
		r.t.Fatal("OnSessionID should be nil for synchronous calls")
	}
	return RunnerResult{SessionID: "thread-streamed", Text: "done\n"}, nil
}

type asyncSessionPersistRunner struct {
	t         *testing.T
	agentFile string
}

func (r asyncSessionPersistRunner) Call(req RunnerRequest) (RunnerResult, error) {
	if req.OnSessionID == nil {
		r.t.Fatal("OnSessionID is nil")
	}
	if err := req.OnSessionID("thread-streamed"); err != nil {
		r.t.Fatalf("OnSessionID returned error: %v", err)
	}
	agent, err := readAgent(r.agentFile)
	if err != nil {
		r.t.Fatalf("read streamed agent: %v", err)
	}
	if agent.SessionID != "thread-streamed" || agent.Status != StatusRunning {
		r.t.Fatalf("session not persisted during call: %+v", agent)
	}
	return RunnerResult{SessionID: "thread-streamed", Text: "done\n"}, nil
}

type fakeWorkerStarter struct {
	requests []AsyncWorkerRequest
	pid      int
	err      error
}

func (f *fakeWorkerStarter) StartAsyncCall(req AsyncWorkerRequest) (int, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return 0, f.err
	}
	if f.pid == 0 {
		f.pid = 4321
	}
	return f.pid, nil
}

type blockingWorkerStarter struct {
	started chan struct{}
	release chan struct{}
}

func (f blockingWorkerStarter) StartAsyncCall(AsyncWorkerRequest) (int, error) {
	close(f.started)
	<-f.release
	return 8765, nil
}

type interruptingRunner struct {
	calls   []RunnerRequest
	onFirst func() error
}

func (r *interruptingRunner) Call(req RunnerRequest) (RunnerResult, error) {
	r.calls = append(r.calls, req)
	if len(r.calls) == 1 {
		if r.onFirst != nil {
			if err := r.onFirst(); err != nil {
				return RunnerResult{}, err
			}
		}
		if req.OnSessionID != nil {
			if err := req.OnSessionID("thread-interrupt"); err != nil {
				return RunnerResult{}, err
			}
		}
		return RunnerResult{SessionID: "thread-interrupt", Interrupted: true}, nil
	}
	return RunnerResult{SessionID: "thread-interrupt", Text: "resumed\n"}, nil
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
		PromptRefs:       []string{"code-reviewer"},
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

func TestRegisterResolvesPromptChain(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
	})

	agent, layout, err := manager.Register(RegisterOptions{
		Root:    repo,
		Name:    "reviewer",
		Prompts: []string{"code-reviewer", "code-review-correctness", "code-review-fit"},
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if agent.Tier != "core" || agent.Model != "gpt-5.5" {
		t.Fatalf("tier/model = %q/%q", agent.Tier, agent.Model)
	}
	if agent.SystemPromptPath != "system.md" {
		t.Fatalf("system prompt path = %q", agent.SystemPromptPath)
	}
	raw, err := os.ReadFile(layout.SystemFile)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "model: sonnet") {
		t.Fatalf("frontmatter was not stripped:\n%s", text)
	}
	if !strings.Contains(text, "You are a delegated worker") ||
		!strings.Contains(text, "You are a code reviewer.") ||
		!strings.Contains(text, "Correctness Partition") ||
		!strings.Contains(text, "Fit Partition") {
		t.Fatalf("materialized prompt missing expected sections:\n%s", text)
	}
	if len(agent.PromptRefs) != 4 || agent.PromptRefs[0] != "delegate-orientation" || agent.PromptRefs[1] != "code-reviewer" {
		t.Fatalf("prompt refs = %+v", agent.PromptRefs)
	}
}

func TestRegisterInjectsDelegateOrientationForInlineSystemPrompt(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
	})

	agent, layout, err := manager.Register(RegisterOptions{
		Root:             repo,
		Name:             "inline",
		SystemPromptText: "inline system",
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if len(agent.PromptRefs) != 1 || agent.PromptRefs[0] != "delegate-orientation" {
		t.Fatalf("prompt refs = %+v", agent.PromptRefs)
	}
	raw, err := os.ReadFile(layout.SystemFile)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "You are a delegated worker") || !strings.Contains(text, "inline system") {
		t.Fatalf("missing delegate orientation or inline system prompt:\n%s", text)
	}
	if strings.Index(text, "You are a delegated worker") > strings.Index(text, "inline system") {
		t.Fatalf("delegate orientation did not precede inline system prompt:\n%s", text)
	}
}

func TestRegisterPromptRefsAliasAndExplicitTierWins(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
	})

	agent, _, err := manager.Register(RegisterOptions{
		Root:       repo,
		Name:       "reviewer",
		Tier:       "deep",
		PromptRefs: []string{"code-reviewer"},
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if agent.Tier != "deep" {
		t.Fatalf("tier = %q", agent.Tier)
	}
}

func TestRegisterConditionalPromptRefPresent(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	binDir := t.TempDir()
	toolName := "ws-test-tool"
	bin := filepath.Join(binDir, toolName)
	script := "#!/bin/sh\nexit 0\n"
	if runtime.GOOS == "windows" {
		bin += ".cmd"
		script = "@echo off\r\nexit /b 0\r\n"
	}
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
	})

	agent, layout, err := manager.Register(RegisterOptions{
		Root:    repo,
		Name:    "conditional",
		Prompts: []string{"code-reviewer"},
		ConditionalPromptRefs: []ConditionalPromptRef{
			{Binary: toolName, PromptRef: "code-review-fit"},
		},
		SuppressOrientation: true,
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if len(agent.PromptRefs) != 2 || agent.PromptRefs[0] != "code-reviewer" || agent.PromptRefs[1] != "code-review-fit" {
		t.Fatalf("prompt refs = %+v", agent.PromptRefs)
	}
	raw, err := os.ReadFile(layout.SystemFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Fit Partition") {
		t.Fatalf("conditional prompt was not materialized:\n%s", raw)
	}
}

func TestRegisterConditionalPromptRefAbsent(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
	})

	agent, layout, err := manager.Register(RegisterOptions{
		Root:    repo,
		Name:    "conditional",
		Prompts: []string{"code-reviewer"},
		ConditionalPromptRefs: []ConditionalPromptRef{
			{Binary: "ws-test-tool-definitely-missing", PromptRef: "code-review-fit"},
		},
		SuppressOrientation: true,
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if len(agent.PromptRefs) != 1 || agent.PromptRefs[0] != "code-reviewer" {
		t.Fatalf("prompt refs = %+v", agent.PromptRefs)
	}
	raw, err := os.ReadFile(layout.SystemFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "Fit Partition") {
		t.Fatalf("absent conditional prompt was materialized:\n%s", raw)
	}
}

func TestRegisterAppliesConfiguredTierModel(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	if _, err := wsconfig.SetAgentsTier(wsconfig.Options{CacheHome: cache}, "core", "", "gemini-3-1-pro"); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
	})

	agent, _, err := manager.Register(RegisterOptions{
		Root:    repo,
		Name:    "reviewer",
		Prompts: []string{"code-reviewer"},
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if agent.Tier != "core" || agent.Backend != "gemini" || agent.Model != "gemini-3-1-pro" {
		t.Fatalf("tier/backend/model = %q/%q/%q", agent.Tier, agent.Backend, agent.Model)
	}
}

func TestRegisterExplicitModelBypassesTierConfig(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	if _, err := wsconfig.SetAgentsTier(wsconfig.Options{CacheHome: cache}, "core", "gemini", "gemini-3-1-pro"); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
	})

	agent, _, err := manager.Register(RegisterOptions{
		Root:  repo,
		Name:  "reviewer",
		Tier:  "core",
		Model: "gpt-5.2",
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if agent.Backend != "codex" || agent.Model != "gpt-5.2" {
		t.Fatalf("backend/model = %q/%q", agent.Backend, agent.Model)
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

	agent, text, err := manager.syncCall(syncCallOptions{Root: repo, Name: "impl", Prompt: "first"})
	if err != nil {
		t.Fatalf("first Call returned error: %v", err)
	}
	if text != "reply: first\n" || agent.SessionID != "thread-1" || agent.Status != StatusIdle {
		t.Fatalf("first call mismatch: agent=%+v text=%q", agent, text)
	}
	if len(runner.calls) != 1 || runner.calls[0].SessionID != "" || runner.calls[0].SystemPromptPath == "" {
		t.Fatalf("first runner call mismatch: %+v", runner.calls)
	}
	if runner.calls[0].Stdout != nil || runner.calls[0].Stderr != nil {
		t.Fatalf("sync call passed stream writers: stdout=%T stderr=%T", runner.calls[0].Stdout, runner.calls[0].Stderr)
	}

	agent, text, err = manager.syncCall(syncCallOptions{Root: repo, Name: "impl", Prompt: "second"})
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

func TestCallStoresSessionIDAfterSynchronousCompletion(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	baseManager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
	})
	_, layout, err := baseManager.Register(RegisterOptions{Root: repo, Name: "impl"})
	if err != nil {
		t.Fatal(err)
	}

	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
		Runner:    sessionPersistRunner{t: t, agentFile: layout.AgentFile},
	})
	agent, text, err := manager.syncCall(syncCallOptions{Root: repo, Name: "impl", Prompt: "work"})
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if agent.SessionID != "thread-streamed" || text != "done\n" || agent.Status != StatusIdle {
		t.Fatalf("final call mismatch: agent=%+v text=%q", agent, text)
	}
}

func TestClaudeRunnerCreatesSessionAndParsesJSON(t *testing.T) {
	repo := initRepo(t)
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "claude.log")
	writeFakeClaudeExecutable(t, binDir)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CLAUDE_FAKE_LOG", logPath)

	var capturedSession string
	var stdout bytes.Buffer
	result, err := ClaudeRunner{}.Call(RunnerRequest{
		Root:    repo,
		Prompt:  "hello claude",
		Model:   "claude",
		Stdout:  &stdout,
		Timeout: time.Minute,
		OnSessionID: func(sessionID string) error {
			capturedSession = sessionID
			return nil
		},
	})
	if err != nil {
		t.Fatalf("ClaudeRunner.Call returned error: %v", err)
	}
	if capturedSession == "" || result.SessionID != capturedSession {
		t.Fatalf("session not captured: result=%q captured=%q", result.SessionID, capturedSession)
	}
	if result.Text != "claude reply" || !strings.Contains(stdout.String(), "claude reply") {
		t.Fatalf("result/stdout = %q/%q", result.Text, stdout.String())
	}
	logRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(logRaw)
	if !strings.Contains(log, "--session-id") ||
		!strings.Contains(log, capturedSession) ||
		!strings.Contains(log, "hello claude") ||
		strings.Contains(log, "--model") {
		t.Fatalf("unexpected first-call args:\n%s", log)
	}
}

func TestClaudeRunnerResumesWithSystemPromptModelAndHook(t *testing.T) {
	repo := initRepo(t)
	binDir := t.TempDir()
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "claude.log")
	systemPromptPath := filepath.Join(tmp, "system.md")
	if err := os.WriteFile(systemPromptPath, []byte("system text"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeFakeClaudeExecutable(t, binDir)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CLAUDE_FAKE_LOG", logPath)

	result, err := ClaudeRunner{}.Call(RunnerRequest{
		Root:                 repo,
		Prompt:               "resume prompt",
		Model:                "claude-sonnet-4",
		SessionID:            "session-123",
		SystemPromptPath:     systemPromptPath,
		InterruptHookCommand: "ws hook",
		Timeout:              time.Minute,
	})
	if err != nil {
		t.Fatalf("ClaudeRunner.Call returned error: %v", err)
	}
	if result.SessionID != "session-123" || result.Text != "claude reply" {
		t.Fatalf("result = %+v", result)
	}
	logRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(logRaw)
	for _, want := range []string{
		"--resume",
		"session-123",
		"--model",
		"claude-sonnet-4",
		"--system-prompt",
		"system text",
		"--settings",
		"PostToolBatch",
		"ws hook",
		"resume prompt",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("claude args missing %q:\n%s", want, log)
		}
	}
}

func TestClaudeRunnerNonZeroExitPreservesStderr(t *testing.T) {
	repo := initRepo(t)
	binDir := t.TempDir()
	writeFakeClaudeExecutable(t, binDir)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CLAUDE_FAKE_LOG", filepath.Join(t.TempDir(), "claude.log"))
	t.Setenv("CLAUDE_FAKE_FAIL", "1")

	_, err := ClaudeRunner{}.Call(RunnerRequest{Root: repo, Prompt: "fail", Timeout: time.Minute})
	if err == nil {
		t.Fatal("ClaudeRunner.Call returned nil error")
	}
	if !strings.Contains(err.Error(), "claude failed") || !strings.Contains(err.Error(), "login required") {
		t.Fatalf("error did not preserve stderr: %v", err)
	}
}

func TestCallStartsWorkerAndRejectsBusyAgent(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	starter := &fakeWorkerStarter{pid: 4567}
	manager := NewManager(Options{
		CacheHome:     cache,
		Now:           func() time.Time { return testNow },
		WorkerStarter: starter,
	})
	if _, _, err := manager.Register(RegisterOptions{Root: repo, Name: "impl"}); err != nil {
		t.Fatal(err)
	}

	result, err := manager.Call(CallOptions{Root: repo, Name: "impl", Prompt: "do work"})
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if result.AgentName != "impl" || result.Status != CallStatusRunning || result.PID != 4567 {
		t.Fatalf("async result mismatch: %+v", result)
	}
	if len(starter.requests) != 1 || starter.requests[0].Name != "impl" || starter.requests[0].PromptPath == "" {
		t.Fatalf("worker request mismatch: %+v", starter.requests)
	}
	rawPrompt, err := os.ReadFile(starter.requests[0].PromptPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(rawPrompt) != "do work" {
		t.Fatalf("prompt snapshot = %q", rawPrompt)
	}
	layout, err := manager.layout(repo, "impl", false)
	if err != nil {
		t.Fatal(err)
	}
	call, err := readCurrentCall(layout.CurrentStateFile)
	if err != nil {
		t.Fatal(err)
	}
	if call.Status != CallStatusRunning || call.PID != 4567 || call.PromptPath != "current/prompt.md" {
		t.Fatalf("current call mismatch: %+v", call)
	}
	if _, err := manager.Call(CallOptions{Root: repo, Name: "impl", Prompt: "again"}); err == nil || !strings.Contains(err.Error(), "active call") {
		t.Fatalf("expected busy rejection, got %v", err)
	}
}

func TestCallRejectsConcurrentSetupWithCurrentCallLock(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	starter := blockingWorkerStarter{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	manager := NewManager(Options{
		CacheHome:     cache,
		Now:           func() time.Time { return testNow },
		WorkerStarter: starter,
	})
	if _, _, err := manager.Register(RegisterOptions{Root: repo, Name: "impl"}); err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := manager.Call(CallOptions{Root: repo, Name: "impl", Prompt: "first"})
		errCh <- err
	}()
	<-starter.started
	if _, err := manager.Call(CallOptions{Root: repo, Name: "impl", Prompt: "second"}); err == nil ||
		!strings.Contains(err.Error(), "setup is already in progress") {
		t.Fatalf("expected setup lock rejection, got %v", err)
	}
	close(starter.release)
	if err := <-errCh; err != nil {
		t.Fatalf("first Call returned error: %v", err)
	}
	layout, err := manager.layout(repo, "impl", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(layout.CurrentLockFile); !os.IsNotExist(err) {
		t.Fatalf("current setup lock still exists or stat failed differently: %v", err)
	}
}

func TestRunCurrentCompletesAsyncCallAndCapturesStreams(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	starter := &fakeWorkerStarter{pid: 4567}
	base := NewManager(Options{
		CacheHome:     cache,
		Now:           func() time.Time { return testNow },
		WorkerStarter: starter,
	})
	if _, _, err := base.Register(RegisterOptions{Root: repo, Name: "impl"}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.Call(CallOptions{Root: repo, Name: "impl", Prompt: "async prompt"}); err != nil {
		t.Fatal(err)
	}

	runner := &streamingFakeRunner{}
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
		Runner:    runner,
	})
	if err := manager.RunCurrent(repo, "impl"); err != nil {
		t.Fatalf("RunCurrent returned error: %v", err)
	}
	layout, err := manager.layout(repo, "impl", false)
	if err != nil {
		t.Fatal(err)
	}
	call, err := readCurrentCall(layout.CurrentStateFile)
	if err != nil {
		t.Fatal(err)
	}
	if call.Status != CallStatusCompleted || call.SessionID != "thread-async" || call.ExitCode == nil || *call.ExitCode != 0 {
		t.Fatalf("completed current call mismatch: %+v", call)
	}
	printed, err := manager.Print(repo, "impl")
	if err != nil {
		t.Fatal(err)
	}
	if printed != "async reply\n" {
		t.Fatalf("printed = %q", printed)
	}
	stdout, err := os.ReadFile(layout.CurrentStdout)
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := os.ReadFile(layout.CurrentStderr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stdout), "jsonl") || !strings.Contains(string(stderr), "diagnostic") {
		t.Fatalf("streams not captured: stdout=%q stderr=%q", stdout, stderr)
	}
	if len(runner.calls) != 1 || runner.calls[0].Prompt != "async prompt" || !runner.calls[0].InheritProcessGroup {
		t.Fatalf("runner prompt mismatch: %+v", runner.calls)
	}

	waited, err := manager.Wait(WaitOptions{Root: repo, Name: "impl", Timeout: time.Second})
	if err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if !strings.Contains(waited, "agent: impl") ||
		!strings.Contains(waited, "call_status: completed") ||
		!strings.Contains(waited, "result_available: true") ||
		strings.Contains(waited, "async reply") {
		t.Fatalf("waited = %q", waited)
	}
	result, err := manager.Result(ResultOptions{Root: repo, Name: "impl"})
	if err != nil {
		t.Fatalf("Result returned error: %v", err)
	}
	if result != "async reply\n" {
		t.Fatalf("result = %q", result)
	}
	status, err := manager.Status(repo, "impl")
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if !strings.Contains(status, "call_status: completed") || !strings.Contains(status, "session_id: thread-async") {
		t.Fatalf("status mismatch:\n%s", status)
	}
	tail, err := manager.Tail(TailOptions{Root: repo, Name: "impl", Lines: 20})
	if err != nil {
		t.Fatalf("Tail returned error: %v", err)
	}
	if !strings.Contains(tail, "== events ==") ||
		!strings.Contains(tail, "== runtime ==") ||
		!strings.Contains(tail, "backend.call.start") ||
		!strings.Contains(tail, "jsonl") ||
		!strings.Contains(tail, "async reply") {
		t.Fatalf("tail mismatch:\n%s", tail)
	}
}

func TestRunCurrentUsesClaudeBackendRunner(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "claude.log")
	writeFakeClaudeExecutable(t, binDir)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CLAUDE_FAKE_LOG", logPath)

	starter := &fakeWorkerStarter{pid: 4567}
	base := NewManager(Options{
		CacheHome:     cache,
		Now:           func() time.Time { return testNow },
		WorkerStarter: starter,
	})
	if _, _, err := base.Register(RegisterOptions{
		Root:             repo,
		Name:             "impl",
		Backend:          "claude",
		Model:            "claude",
		SystemPromptText: "sys",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.Call(CallOptions{Root: repo, Name: "impl", Prompt: "async prompt"}); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
	})
	if err := manager.RunCurrent(repo, "impl"); err != nil {
		t.Fatalf("RunCurrent returned error: %v", err)
	}
	layout, err := manager.layout(repo, "impl", false)
	if err != nil {
		t.Fatal(err)
	}
	agent, err := readAgent(layout.AgentFile)
	if err != nil {
		t.Fatal(err)
	}
	call, err := readCurrentCall(layout.CurrentStateFile)
	if err != nil {
		t.Fatal(err)
	}
	if agent.Backend != "claude" || agent.SessionID == "" || call.SessionID != agent.SessionID || call.Status != CallStatusCompleted {
		t.Fatalf("claude call state mismatch: agent=%+v call=%+v", agent, call)
	}
	result, err := manager.Result(ResultOptions{Root: repo, Name: "impl"})
	if err != nil {
		t.Fatalf("Result returned error: %v", err)
	}
	if result != "claude reply" {
		t.Fatalf("result = %q", result)
	}
	logRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(logRaw)
	if !strings.Contains(log, "--session-id") || !strings.Contains(log, agent.SessionID) || strings.Contains(log, "--model") {
		t.Fatalf("unexpected claude args:\n%s", log)
	}

	if _, err := base.Call(CallOptions{Root: repo, Name: "impl", Prompt: "resume prompt"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.RunCurrent(repo, "impl"); err != nil {
		t.Fatalf("resume RunCurrent returned error: %v", err)
	}
	logRaw, err = os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log = string(logRaw)
	if !strings.Contains(log, "--resume") || !strings.Contains(log, agent.SessionID) || !strings.Contains(log, "resume prompt") {
		t.Fatalf("resume did not use stored session:\n%s", log)
	}
}

func TestInterruptQueuesInboxAndHookDeliversMessages(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	starter := &fakeWorkerStarter{pid: 4567}
	base := NewManager(Options{
		CacheHome:     cache,
		Now:           func() time.Time { return testNow },
		WorkerStarter: starter,
	})
	if _, _, err := base.Register(RegisterOptions{Root: repo, Name: "impl"}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.Call(CallOptions{Root: repo, Name: "impl", Prompt: "async prompt"}); err != nil {
		t.Fatal(err)
	}

	queued, err := base.Interrupt(InterruptOptions{Root: repo, Name: "impl", Message: "Switch to tests only."})
	if err != nil {
		t.Fatal(err)
	}
	if queued.MessageID != "000001" || !queued.Queued {
		t.Fatalf("queued interrupt mismatch: %+v", queued)
	}
	messages, err := base.DeliverPendingInbox(repo, "impl", "hook")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Text != "Switch to tests only." {
		t.Fatalf("delivered messages mismatch: %+v", messages)
	}
	feedback := ComposeLeadMessageFeedback(messages)
	if !strings.Contains(feedback, "Switch to tests only.") ||
		!strings.Contains(feedback, "Lead messages queued") {
		t.Fatalf("feedback did not include interrupt:\n%s", feedback)
	}

	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
		Runner:    &fakeRunner{},
	})
	if err := manager.RunCurrent(repo, "impl"); err != nil {
		t.Fatalf("RunCurrent returned error: %v", err)
	}
	layout, err := manager.layout(repo, "impl", false)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := readMessage(filepath.Join(layout.InboxDir, "000001.json"))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Status != "delivered" {
		t.Fatalf("message status = %q, want delivered", msg.Status)
	}
	printed, err := manager.Print(repo, "impl")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(printed, "Switch to tests only.") {
		t.Fatalf("hook-delivered interrupt was unexpectedly re-delivered via resume:\n%s", printed)
	}
	if !strings.Contains(printed, "reply: async prompt") {
		t.Fatalf("printed = %q", printed)
	}
	tail, err := manager.Tail(TailOptions{Root: repo, Name: "impl", Lines: 40})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tail, "inbox.delivered_via_hook") || strings.Contains(tail, "call.interrupted") {
		t.Fatalf("tail missing interrupt diagnostics:\n%s", tail)
	}
}

func TestWaitFinalizesDeadAsyncWorker(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	starter := &fakeWorkerStarter{pid: 987654}
	manager := NewManager(Options{
		CacheHome:     cache,
		Now:           func() time.Time { return testNow },
		WorkerStarter: starter,
		ProcessAlive:  func(int) (bool, error) { return false, nil },
	})
	if _, _, err := manager.Register(RegisterOptions{Root: repo, Name: "impl"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Call(CallOptions{Root: repo, Name: "impl", Prompt: "async prompt"}); err != nil {
		t.Fatal(err)
	}

	status, err := manager.Wait(WaitOptions{
		Root:    repo,
		Name:    "impl",
		Timeout: time.Second,
		Poll:    time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if !strings.Contains(status, "agent: impl") ||
		!strings.Contains(status, "call_status: failed") ||
		!strings.Contains(status, "result_available: false") ||
		!strings.Contains(status, "async worker process 987654 is not running") {
		t.Fatalf("dead worker status mismatch:\n%s", status)
	}
	tail, err := manager.Tail(TailOptions{Root: repo, Name: "impl", Lines: 20})
	if err != nil {
		t.Fatalf("Tail returned error: %v", err)
	}
	if !strings.Contains(tail, "== runtime ==") || !strings.Contains(tail, "worker.dead") {
		t.Fatalf("tail missing runtime dead-worker log:\n%s", tail)
	}
}

func TestWaitReturnsWhenAnyNamedAgentIsReady(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
	})
	for _, name := range []string{"ready", "pending"} {
		if _, _, err := manager.Register(RegisterOptions{Root: repo, Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	readyLayout, err := manager.layout(repo, "ready", false)
	if err != nil {
		t.Fatal(err)
	}
	pendingLayout, err := manager.layout(repo, "pending", false)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := manager.BeginCurrentCall(readyLayout, Agent{Name: "ready"})
	if err != nil {
		t.Fatal(err)
	}
	completed.Status = CallStatusCompleted
	completed.PID = 1234
	if err := writeCurrentCall(readyLayout.CurrentStateFile, completed); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(readyLayout.OutputFile, []byte("ready output\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	running, err := manager.BeginCurrentCall(pendingLayout, Agent{Name: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	running.Status = CallStatusRunning
	running.PID = os.Getpid()
	if err := writeCurrentCall(pendingLayout.CurrentStateFile, running); err != nil {
		t.Fatal(err)
	}

	text, err := manager.Wait(WaitOptions{
		Root:    repo,
		Names:   []string{"ready", "pending"},
		Timeout: time.Minute,
		Poll:    time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if !strings.Contains(text, "agent: ready") ||
		!strings.Contains(text, "call_status: completed") ||
		!strings.Contains(text, "result_available: true") ||
		!strings.Contains(text, "agent: pending") ||
		!strings.Contains(text, "call_status: running") ||
		!strings.Contains(text, "active: true") ||
		strings.Contains(text, "ready output") {
		t.Fatalf("multi-agent readiness mismatch:\n%s", text)
	}
}

func TestResultConsumesCompletedEphemeralAgentOnly(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
	})
	_, ephemeralLayout, err := manager.Register(RegisterOptions{Root: repo, Name: "tmp", Ephemeral: true})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := manager.BeginCurrentCall(ephemeralLayout, Agent{Name: "tmp"})
	if err != nil {
		t.Fatal(err)
	}
	completed.Status = CallStatusCompleted
	if err := writeCurrentCall(ephemeralLayout.CurrentStateFile, completed); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ephemeralLayout.OutputFile, []byte("temporary result\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	text, err := manager.Result(ResultOptions{Root: repo, Name: "tmp"})
	if err != nil {
		t.Fatalf("Result returned error: %v", err)
	}
	if text != "temporary result\n" {
		t.Fatalf("result = %q", text)
	}
	if _, err := os.Stat(ephemeralLayout.AgentDir); !os.IsNotExist(err) {
		t.Fatalf("ephemeral agent dir still exists after result: %v", err)
	}

	_, failedLayout, err := manager.Register(RegisterOptions{Root: repo, Name: "failed-tmp", Ephemeral: true})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := manager.BeginCurrentCall(failedLayout, Agent{Name: "failed-tmp"})
	if err != nil {
		t.Fatal(err)
	}
	failed.Status = CallStatusFailed
	failed.Error = "boom"
	if err := writeCurrentCall(failedLayout.CurrentStateFile, failed); err != nil {
		t.Fatal(err)
	}

	status, err := manager.Result(ResultOptions{Root: repo, Name: "failed-tmp"})
	if err != nil {
		t.Fatalf("failed Result returned error: %v", err)
	}
	if !strings.Contains(status, "result_available: false") || !strings.Contains(status, "boom") {
		t.Fatalf("failed result status mismatch:\n%s", status)
	}
	if _, err := os.Stat(failedLayout.AgentDir); err != nil {
		t.Fatalf("failed ephemeral agent was erased: %v", err)
	}
}

func TestRunCurrentFailureAndPanicDiagnostics(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	binDir := t.TempDir()
	claudePath := writeBackendShim(t, binDir, "claude")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	starter := &fakeWorkerStarter{pid: 4567}
	base := NewManager(Options{
		CacheHome:     cache,
		Now:           func() time.Time { return testNow },
		WorkerStarter: starter,
	})
	if _, _, err := base.Register(RegisterOptions{Root: repo, Name: "impl"}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.Call(CallOptions{Root: repo, Name: "impl", Prompt: "async prompt"}); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
		Runner:    errorRunner{err: errors.New("backend exploded")},
	})
	if err := manager.RunCurrent(repo, "impl"); err == nil ||
		!strings.Contains(err.Error(), "backend exploded") ||
		!strings.Contains(err.Error(), "backend invocation failed") ||
		!strings.Contains(err.Error(), "- claude: "+claudePath) ||
		!strings.Contains(err.Error(), "re-run agents.register") ||
		!strings.Contains(err.Error(), "config.agents_tier") {
		t.Fatalf("RunCurrent error = %v", err)
	}
	status, err := manager.Status(repo, "impl")
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if !strings.Contains(status, "agent_status: failed") ||
		!strings.Contains(status, "error: backend invocation failed") ||
		!strings.Contains(status, "raw_error:") ||
		!strings.Contains(status, "backend exploded") ||
		!strings.Contains(status, "- claude: "+claudePath) {
		t.Fatalf("failure status mismatch:\n%s", status)
	}
	tail, err := manager.Tail(TailOptions{Root: repo, Name: "impl", Lines: 20})
	if err != nil {
		t.Fatalf("Tail returned error: %v", err)
	}
	if !strings.Contains(tail, "backend.call.error") || !strings.Contains(tail, "state.finalize.end") {
		t.Fatalf("tail missing failure runtime diagnostics:\n%s", tail)
	}

	if _, _, err := base.Register(RegisterOptions{Root: repo, Name: "panic-impl"}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.Call(CallOptions{Root: repo, Name: "panic-impl", Prompt: "panic prompt"}); err != nil {
		t.Fatal(err)
	}
	panicManager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
		Runner:    panicRunner{},
	})
	if err := panicManager.RunCurrent(repo, "panic-impl"); err == nil || !strings.Contains(err.Error(), "panic") {
		t.Fatalf("panic RunCurrent error = %v", err)
	}
	panicTail, err := panicManager.Tail(TailOptions{Root: repo, Name: "panic-impl", Lines: 20})
	if err != nil {
		t.Fatalf("panic Tail returned error: %v", err)
	}
	if !strings.Contains(panicTail, "worker.panic") {
		t.Fatalf("tail missing panic runtime diagnostics:\n%s", panicTail)
	}
}

func TestRunCurrentUnsupportedBackendIncludesRecoveryHint(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	binDir := t.TempDir()
	geminiPath := writeBackendShim(t, binDir, "gemini")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	starter := &fakeWorkerStarter{pid: 4567}
	base := NewManager(Options{
		CacheHome:     cache,
		Now:           func() time.Time { return testNow },
		WorkerStarter: starter,
	})
	if _, _, err := base.Register(RegisterOptions{Root: repo, Name: "impl", Backend: "gemini", Model: "gemini-3-1-pro"}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.Call(CallOptions{Root: repo, Name: "impl", Prompt: "async prompt"}); err != nil {
		t.Fatalf("Call should queue unsupported backend for worker diagnostics: %v", err)
	}

	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
	})
	err := manager.RunCurrent(repo, "impl")
	if err == nil {
		t.Fatal("RunCurrent returned nil error")
	}
	for _, want := range []string{
		`unsupported agent backend "gemini"`,
		"backend: gemini",
		"model: gemini-3-1-pro",
		"- gemini: " + geminiPath,
		"re-run agents.register",
		"config.agents_tier",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("RunCurrent error missing %q:\n%v", want, err)
		}
	}
	status, err := manager.Status(repo, "impl")
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if !strings.Contains(status, "call_status: failed") || !strings.Contains(status, `unsupported agent backend "gemini"`) {
		t.Fatalf("status missing unsupported backend diagnostic:\n%s", status)
	}
}

func TestWaitTimeoutAndCancelCurrentCall(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	var cancelledPID int
	processAlive := true
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
		ProcessCancel: func(pid int) error {
			cancelledPID = pid
			processAlive = false
			return nil
		},
		ProcessAlive: func(int) (bool, error) { return processAlive, nil },
	})
	agent, layout, err := manager.Register(RegisterOptions{Root: repo, Name: "impl"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.BeginCurrentCall(layout, agent); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.MarkCurrentCallRunning(layout, 2468, ""); err != nil {
		t.Fatal(err)
	}
	timeoutText, err := manager.Wait(WaitOptions{
		Root:    repo,
		Name:    "impl",
		Timeout: time.Millisecond,
		Poll:    time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Wait timeout returned error: %v", err)
	}
	if !strings.Contains(timeoutText, "wait_timeout: true") ||
		!strings.Contains(timeoutText, "call_status: running") ||
		!strings.Contains(timeoutText, "active: true") ||
		!strings.Contains(timeoutText, "follow_up: agents.wait --timeout 10m | agents.status | agents.cancel | agents.tail") {
		t.Fatalf("timeout text mismatch:\n%s", timeoutText)
	}
	tail, err := manager.Tail(TailOptions{Root: repo, Name: "impl", Lines: 20})
	if err != nil {
		t.Fatalf("Tail returned error: %v", err)
	}
	if !strings.Contains(tail, "wait.timeout") {
		t.Fatalf("tail missing wait timeout diagnostic:\n%s", tail)
	}
	cancelled, err := manager.Cancel(repo, "impl")
	if err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}
	if cancelledPID != 2468 {
		t.Fatalf("cancelled pid = %d, want 2468", cancelledPID)
	}
	if !strings.Contains(cancelled, "call_status: cancelled") ||
		!strings.Contains(cancelled, "active: false") ||
		!strings.Contains(cancelled, "cancel_pid: 2468") ||
		!strings.Contains(cancelled, "cleanup_needed: false") {
		t.Fatalf("cancel status mismatch:\n%s", cancelled)
	}
	call, err := readCurrentCall(layout.CurrentStateFile)
	if err != nil {
		t.Fatal(err)
	}
	if call.Status != CallStatusCancelled || call.FinishedAt == "" || call.CancelPID != 2468 || call.CleanupNeeded {
		t.Fatalf("cancelled call mismatch: %+v", call)
	}
	tail, err = manager.Tail(TailOptions{Root: repo, Name: "impl", Lines: 20})
	if err != nil {
		t.Fatalf("Tail returned error: %v", err)
	}
	if !strings.Contains(tail, "cancel.begin") || !strings.Contains(tail, "cancel.end") {
		t.Fatalf("tail missing cancel diagnostics:\n%s", tail)
	}
}

func TestAgentTimeoutDefaultsAreTenMinutes(t *testing.T) {
	if defaultAgentWaitTimeout != 10*time.Minute {
		t.Fatalf("defaultAgentWaitTimeout = %s", defaultAgentWaitTimeout)
	}
}

func TestCancelReportsCleanupNeededWhenOwnedProcessSurvives(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{
		CacheHome:     cache,
		Now:           func() time.Time { return testNow },
		ProcessCancel: func(int) error { return nil },
		ProcessAlive:  func(int) (bool, error) { return true, nil },
		WorkerStarter: &fakeWorkerStarter{pid: 1357},
	})
	if _, _, err := manager.Register(RegisterOptions{Root: repo, Name: "impl"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Call(CallOptions{Root: repo, Name: "impl", Prompt: "long review"}); err != nil {
		t.Fatal(err)
	}
	status, err := manager.Cancel(repo, "impl")
	if err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}
	if !strings.Contains(status, "call_status: cancelled") ||
		!strings.Contains(status, "cancel_pid: 1357") ||
		!strings.Contains(status, "cleanup_needed: true") ||
		!strings.Contains(status, "follow_up: inspect runtime log | manual cleanup | agents.erase") {
		t.Fatalf("cleanup-needed status mismatch:\n%s", status)
	}
	tail, err := manager.Tail(TailOptions{Root: repo, Name: "impl", Lines: 20})
	if err != nil {
		t.Fatalf("Tail returned error: %v", err)
	}
	if !strings.Contains(tail, "cancel.cleanup_needed") {
		t.Fatalf("tail missing cleanup-needed diagnostic:\n%s", tail)
	}
}

type streamingFakeRunner struct {
	calls []RunnerRequest
}

func (f *streamingFakeRunner) Call(req RunnerRequest) (RunnerResult, error) {
	f.calls = append(f.calls, req)
	if req.Stdout != nil {
		_, _ = req.Stdout.Write([]byte("jsonl\n"))
	}
	if req.Stderr != nil {
		_, _ = req.Stderr.Write([]byte("diagnostic\n"))
	}
	if req.OnSessionID != nil {
		if err := req.OnSessionID("thread-async"); err != nil {
			return RunnerResult{}, err
		}
	}
	return RunnerResult{SessionID: "thread-async", Text: "async reply\n"}, nil
}

func TestCurrentCallLifecycleAndBusyRejection(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
	})
	agent, layout, err := manager.Register(RegisterOptions{Root: repo, Name: "impl"})
	if err != nil {
		t.Fatal(err)
	}

	call, err := manager.BeginCurrentCall(layout, agent)
	if err != nil {
		t.Fatalf("BeginCurrentCall returned error: %v", err)
	}
	if call.Status != CallStatusQueued || call.CallSeq != 1 || call.ExecutionID != "000001" {
		t.Fatalf("initial call mismatch: %+v", call)
	}
	for _, path := range []string{layout.CurrentStateFile, layout.CurrentStdout, layout.CurrentStderr} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected current call file %s: %v", path, err)
		}
	}
	if _, err := manager.BeginCurrentCall(layout, agent); err == nil || !strings.Contains(err.Error(), "active call") {
		t.Fatalf("expected active-call rejection, got %v", err)
	}

	call, err = manager.MarkCurrentCallRunning(layout, 1234, "thread-current")
	if err != nil {
		t.Fatalf("MarkCurrentCallRunning returned error: %v", err)
	}
	if call.Status != CallStatusRunning || call.PID != 1234 || call.SessionID != "thread-current" {
		t.Fatalf("running call mismatch: %+v", call)
	}

	call, err = manager.CompleteCurrentCall(layout, "thread-current", 0)
	if err != nil {
		t.Fatalf("CompleteCurrentCall returned error: %v", err)
	}
	if call.Status != CallStatusCompleted || call.ExitCode == nil || *call.ExitCode != 0 || call.FinishedAt == "" {
		t.Fatalf("completed call mismatch: %+v", call)
	}

	call, err = manager.BeginCurrentCall(layout, agent)
	if err != nil {
		t.Fatalf("second BeginCurrentCall returned error: %v", err)
	}
	if call.CallSeq != 2 || call.ExecutionID != "000002" {
		t.Fatalf("second call sequence mismatch: %+v", call)
	}
}

func TestCurrentCallFailureAndRecoveryFromExistingState(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
	})
	agent, layout, err := manager.Register(RegisterOptions{Root: repo, Name: "impl"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.BeginCurrentCall(layout, agent); err != nil {
		t.Fatal(err)
	}
	exitCode := 7
	call, err := manager.FailCurrentCall(layout, "boom", &exitCode)
	if err != nil {
		t.Fatalf("FailCurrentCall returned error: %v", err)
	}
	if call.Status != CallStatusFailed || call.Error != "boom" || call.ExitCode == nil || *call.ExitCode != 7 {
		t.Fatalf("failed call mismatch: %+v", call)
	}
	recovered, err := readCurrentCall(layout.CurrentStateFile)
	if err != nil {
		t.Fatalf("readCurrentCall returned error: %v", err)
	}
	if recovered.Status != CallStatusFailed || recovered.CallSeq != 1 {
		t.Fatalf("recovered call mismatch: %+v", recovered)
	}
	if err := ResetCurrentCall(layout); err != nil {
		t.Fatalf("ResetCurrentCall returned error: %v", err)
	}
	if _, err := os.Stat(layout.CurrentStateFile); !os.IsNotExist(err) {
		t.Fatalf("state file still exists or stat failed differently: %v", err)
	}
}

func TestRegisterResetsExistingAgentUnlessCurrentCallActive(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
		Runner:    &fakeRunner{},
	})
	agent, layout, err := manager.Register(RegisterOptions{Root: repo, Name: "impl", SystemPromptText: "old"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.syncCall(syncCallOptions{Root: repo, Name: "impl", Prompt: "first"}); err != nil {
		t.Fatal(err)
	}

	agent, layout, err = manager.Register(RegisterOptions{Root: repo, Name: "impl", SystemPromptText: "new"})
	if err != nil {
		t.Fatalf("Register reset returned error: %v", err)
	}
	if agent.SessionID != "" {
		t.Fatalf("reset register kept session id: %+v", agent)
	}
	if _, err := os.Stat(layout.OutputFile); !os.IsNotExist(err) {
		t.Fatalf("output survived reset or stat failed differently: %v", err)
	}
	raw, err := os.ReadFile(layout.SystemFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "You are a delegated worker") || !strings.Contains(string(raw), "new") {
		t.Fatalf("system prompt after reset = %q", raw)
	}

	if _, err := manager.BeginCurrentCall(layout, agent); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Register(RegisterOptions{Root: repo, Name: "impl"}); err == nil || !strings.Contains(err.Error(), "active call") {
		t.Fatalf("expected active-call register rejection, got %v", err)
	}
}

func TestInternalOneShotErasesAgentDirectory(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
		Runner:    &fakeRunner{},
	})
	text, err := manager.oneShot(oneShotOptions{Root: repo, Name: "tmp", Prompt: "hello"})
	if err != nil {
		t.Fatalf("oneShot returned error: %v", err)
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

func TestSubqueryUsesOneShotLightOrDeepTier(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	starter := &fakeWorkerStarter{pid: 2468}
	manager := NewManager(Options{
		CacheHome:     cache,
		Now:           func() time.Time { return testNow },
		WorkerStarter: starter,
	})

	text, err := manager.Subquery(SubqueryOptions{Root: repo, Question: "Where is workflow?"})
	if err != nil {
		t.Fatalf("Subquery returned error: %v", err)
	}
	key := extractFieldLine(t, text, "subquery_key")
	if !strings.HasPrefix(key, "subquery-tmpdi93gj02ha80-") ||
		!strings.Contains(text, "agent_name: "+key) ||
		!strings.Contains(text, "status: running") ||
		!strings.Contains(text, "pid: 2468") ||
		!strings.Contains(text, `agents.result(name: "`+key+`", timeout_seconds: 600)`) {
		t.Fatalf("subquery start text mismatch:\n%s", text)
	}
	if len(starter.requests) != 1 || starter.requests[0].Name != key {
		t.Fatalf("worker starter requests = %+v", starter.requests)
	}
	layout, err := manager.layout(repo, key, false)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(layout.SystemFile)
	if err != nil {
		t.Fatal(err)
	}
	system := string(raw)
	if strings.Contains(system, "You are a delegated worker") {
		t.Fatalf("subquery prompt included delegate orientation:\n%s", system)
	}
	if !strings.Contains(system, "You are a scoped sub-query worker") {
		t.Fatalf("subquery prompt missing scoped worker prompt:\n%s", system)
	}
	agent, err := readAgent(layout.AgentFile)
	if err != nil {
		t.Fatal(err)
	}
	if agent.Tier != "light" || !agent.Ephemeral {
		t.Fatalf("subquery metadata mismatch: tier=%q ephemeral=%t", agent.Tier, agent.Ephemeral)
	}

	deepStarter := &fakeWorkerStarter{pid: 3579}
	deepManager := NewManager(Options{
		CacheHome:     filepath.Join(t.TempDir(), "cache"),
		Now:           func() time.Time { return testNow.Add(time.Second) },
		WorkerStarter: deepStarter,
	})
	deepText, err := deepManager.Subquery(SubqueryOptions{Root: repo, Question: "Trace history", DeepResearch: true})
	if err != nil {
		t.Fatalf("deep Subquery returned error: %v", err)
	}
	deepKey := extractFieldLine(t, deepText, "subquery_key")
	if !strings.HasPrefix(deepKey, "subquery-tmpdi93gjglur5s-") || deepKey == key {
		t.Fatalf("deep subquery key = %q, first key = %q", deepKey, key)
	}
	deepLayout, err := deepManager.layout(repo, deepKey, false)
	if err != nil {
		t.Fatal(err)
	}
	deepAgent, err := readAgent(deepLayout.AgentFile)
	if err != nil {
		t.Fatal(err)
	}
	if deepAgent.Tier != "deep" {
		t.Fatalf("deep subquery tier = %q", deepAgent.Tier)
	}
}

func extractFieldLine(t *testing.T, text, field string) string {
	t.Helper()
	prefix := field + ": "
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	t.Fatalf("missing %s in:\n%s", field, text)
	return ""
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

func TestParseCodexJSONLAllowsLargeToolEventLine(t *testing.T) {
	raw := []byte(strings.Join([]string{
		`{"type":"thread.started","thread_id":"019test"}`,
		`{"type":"item.completed","item":{"type":"command_execution","aggregated_output":"` + strings.Repeat("x", 256*1024) + `"}}`,
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

func TestParseCodexJSONLToleratesTrailingNonJSONAfterFinalMessage(t *testing.T) {
	raw := []byte(strings.Join([]string{
		`{"type":"thread.started","thread_id":"019test"}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"hello"}}`,
		`����: PID 22896�� ���μ���(PID 25328�� �ڽ� ���μ���)�� �����Ǿ����ϴ�.`,
	}, "\n"))
	result, err := parseCodexJSONL(raw)
	if err != nil {
		t.Fatalf("parseCodexJSONL returned error: %v", err)
	}
	if result.SessionID != "019test" || result.Text != "hello" {
		t.Fatalf("result mismatch: %+v", result)
	}
}

func TestParseCodexJSONLRejectsNonJSONBeforeFinalMessage(t *testing.T) {
	raw := []byte(strings.Join([]string{
		`{"type":"thread.started","thread_id":"019test"}`,
		`not-json`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"hello"}}`,
	}, "\n"))
	if _, err := parseCodexJSONL(raw); err == nil {
		t.Fatalf("parseCodexJSONL returned nil error")
	}
}

func TestParseCodexJSONLStreamNotifiesSessionBeforeFinalMessage(t *testing.T) {
	sessionSeen := false
	reader := &chunkReader{
		t:    t,
		seen: &sessionSeen,
		chunks: [][]byte{
			[]byte(`{"type":"thread.started","thread_id":"019test"}` + "\n"),
			[]byte(`{"type":"item.completed","item":{"type":"agent_message","text":"hello"}}` + "\n"),
			[]byte(`{"type":"turn.completed"}` + "\n"),
		},
	}
	result, err := parseCodexJSONLStream(reader, func(sessionID string) error {
		if sessionID != "019test" {
			t.Fatalf("session id = %q", sessionID)
		}
		sessionSeen = true
		return nil
	})
	if err != nil {
		t.Fatalf("parseCodexJSONLStream returned error: %v", err)
	}
	if result.SessionID != "019test" || result.Text != "hello" {
		t.Fatalf("result mismatch: %+v", result)
	}
}

type chunkReader struct {
	t      *testing.T
	seen   *bool
	chunks [][]byte
	index  int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if r.index >= len(r.chunks) {
		return 0, io.EOF
	}
	if r.index > 0 && !*r.seen {
		r.t.Fatal("requested later JSONL before session callback")
	}
	chunk := r.chunks[r.index]
	r.index++
	return copy(p, chunk), nil
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

func TestDiagnosticStreamSelectsRawFilesAndBoundsLines(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
	})
	_, layout, err := manager.Register(RegisterOptions{Root: repo, Name: "impl"})
	if err != nil {
		t.Fatal(err)
	}
	mustWriteAgentTest(t, layout.CurrentStdout, "old stdout\nnew stdout\n")
	mustWriteAgentTest(t, layout.CurrentStderr, "stderr one\nstderr two\n")
	mustWriteAgentTest(t, layout.CurrentRuntimeLog, "runtime one\nruntime two\n")
	mustWriteAgentTest(t, layout.EventsFile, "event one\nevent two\n")

	stdout, err := manager.DiagnosticStream(DiagnosticStreamOptions{Root: repo, Name: "impl", Stream: "stdout", Lines: 1})
	if err != nil {
		t.Fatalf("DiagnosticStream stdout returned error: %v", err)
	}
	if stdout != "new stdout\n" {
		t.Fatalf("stdout stream = %q", stdout)
	}

	for _, tc := range []struct {
		stream string
		want   string
	}{
		{stream: "stderr", want: "stderr two\n"},
		{stream: "runtime_log", want: "runtime two\n"},
		{stream: "events", want: "event two\n"},
	} {
		t.Run(tc.stream, func(t *testing.T) {
			got, err := manager.DiagnosticStream(DiagnosticStreamOptions{Root: repo, Name: "impl", Stream: tc.stream, Lines: 1})
			if err != nil {
				t.Fatalf("DiagnosticStream returned error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("%s stream = %q, want %q", tc.stream, got, tc.want)
			}
		})
	}
}

func TestTailSanitizesLargeJSONFieldsAndRawTailPreservesThem(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
	})
	_, layout, err := manager.Register(RegisterOptions{Root: repo, Name: "impl"})
	if err != nil {
		t.Fatal(err)
	}
	largeOutput := strings.Repeat("x", tailMaxFieldRunes+200)
	line := `{"type":"event","aggregated_output":"` + largeOutput + `","small":"keep"}`
	mustWriteAgentTest(t, layout.CurrentStdout, line+"\n")

	tail, err := manager.Tail(TailOptions{Root: repo, Name: "impl", Lines: 1})
	if err != nil {
		t.Fatalf("Tail returned error: %v", err)
	}
	if !strings.Contains(tail, "ws-tail truncated field aggregated_output") {
		t.Fatalf("tail missing truncation marker:\n%s", tail)
	}
	if strings.Contains(tail, largeOutput) {
		t.Fatalf("tail included the full large field")
	}
	if !strings.Contains(tail, `"small":"keep"`) {
		t.Fatalf("tail lost unrelated JSON fields:\n%s", tail)
	}

	rawTail, err := manager.Tail(TailOptions{Root: repo, Name: "impl", Lines: 1, Raw: true})
	if err != nil {
		t.Fatalf("raw Tail returned error: %v", err)
	}
	if !strings.Contains(rawTail, largeOutput) || strings.Contains(rawTail, "ws-tail truncated") {
		t.Fatalf("raw tail did not preserve large field:\n%s", rawTail)
	}
}

func mustWriteAgentTest(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
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
