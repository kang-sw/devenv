package wsagent

import (
	"encoding/json"
	"io"
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

type sessionPersistRunner struct {
	t         *testing.T
	agentFile string
}

func (r sessionPersistRunner) Call(req RunnerRequest) (RunnerResult, error) {
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

func TestCallPersistsStreamedSessionIDBeforeCompletion(t *testing.T) {
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
	agent, text, err := manager.Call(CallOptions{Root: repo, Name: "impl", Prompt: "work"})
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if agent.SessionID != "thread-streamed" || text != "done\n" || agent.Status != StatusIdle {
		t.Fatalf("final call mismatch: agent=%+v text=%q", agent, text)
	}
}

func TestCallAsyncStartsWorkerAndRejectsBusyAgent(t *testing.T) {
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

	result, err := manager.CallAsync(CallAsyncOptions{Root: repo, Name: "impl", Prompt: "do work"})
	if err != nil {
		t.Fatalf("CallAsync returned error: %v", err)
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
	if _, err := manager.CallAsync(CallAsyncOptions{Root: repo, Name: "impl", Prompt: "again"}); err == nil || !strings.Contains(err.Error(), "active call") {
		t.Fatalf("expected busy rejection, got %v", err)
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
	if _, err := base.CallAsync(CallAsyncOptions{Root: repo, Name: "impl", Prompt: "async prompt"}); err != nil {
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
	if len(runner.calls) != 1 || runner.calls[0].Prompt != "async prompt" {
		t.Fatalf("runner prompt mismatch: %+v", runner.calls)
	}

	waited, err := manager.Wait(WaitOptions{Root: repo, Name: "impl", Timeout: time.Second})
	if err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if waited != "async reply\n" {
		t.Fatalf("waited = %q", waited)
	}
	status, err := manager.Status(repo, "impl")
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if !strings.Contains(status, "call_status: completed") || !strings.Contains(status, "session_id: thread-async") {
		t.Fatalf("status mismatch:\n%s", status)
	}
	tail, err := manager.Tail(TailOptions{Root: repo, Name: "impl", Lines: 5})
	if err != nil {
		t.Fatalf("Tail returned error: %v", err)
	}
	if !strings.Contains(tail, "== events ==") || !strings.Contains(tail, "jsonl") || !strings.Contains(tail, "async reply") {
		t.Fatalf("tail mismatch:\n%s", tail)
	}
}

func TestWaitTimeoutAndCancelCurrentCall(t *testing.T) {
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
	timeoutText, err := manager.Wait(WaitOptions{
		Root:    repo,
		Name:    "impl",
		Timeout: time.Millisecond,
		Poll:    time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Wait timeout returned error: %v", err)
	}
	if !strings.Contains(timeoutText, "timeout") || !strings.Contains(timeoutText, "call_status: queued") {
		t.Fatalf("timeout text mismatch:\n%s", timeoutText)
	}
	cancelled, err := manager.Cancel(repo, "impl")
	if err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}
	if !strings.Contains(cancelled, "call_status: cancelled") {
		t.Fatalf("cancel status mismatch:\n%s", cancelled)
	}
	call, err := readCurrentCall(layout.CurrentStateFile)
	if err != nil {
		t.Fatal(err)
	}
	if call.Status != CallStatusCancelled || call.FinishedAt == "" {
		t.Fatalf("cancelled call mismatch: %+v", call)
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
	if _, _, err := manager.Call(CallOptions{Root: repo, Name: "impl", Prompt: "first"}); err != nil {
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
	if string(raw) != "new" {
		t.Fatalf("system prompt after reset = %q", raw)
	}

	if _, err := manager.BeginCurrentCall(layout, agent); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Register(RegisterOptions{Root: repo, Name: "impl"}); err == nil || !strings.Contains(err.Error(), "active call") {
		t.Fatalf("expected active-call register rejection, got %v", err)
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

func TestSubqueryUsesOneShotLightOrDeepTier(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	runner := &fakeRunner{}
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return testNow },
		Runner:    runner,
	})

	text, err := manager.Subquery(SubqueryOptions{Root: repo, Question: "Where is workflow?"})
	if err != nil {
		t.Fatalf("Subquery returned error: %v", err)
	}
	if text != "reply: Where is workflow?\n" {
		t.Fatalf("subquery text = %q", text)
	}
	if len(runner.calls) != 1 || !strings.Contains(runner.calls[0].SystemPromptPath, "system.md") {
		t.Fatalf("subquery runner call mismatch: %+v", runner.calls)
	}

	if _, err := manager.Subquery(SubqueryOptions{Root: repo, Question: "Trace history", DeepResearch: true}); err != nil {
		t.Fatalf("deep Subquery returned error: %v", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("runner calls = %d", len(runner.calls))
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
