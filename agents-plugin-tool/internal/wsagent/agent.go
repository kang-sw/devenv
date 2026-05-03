package wsagent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsprompt"
	"github.com/kang-sw/devenv/internal/wsstate"
)

const (
	schemaVersion = 1

	StatusIdle    = "idle"
	StatusRunning = "running"
	StatusBlocked = "blocked"
	StatusFailed  = "failed"
	StatusErased  = "erased"
)

const (
	CallStatusQueued    = "queued"
	CallStatusRunning   = "running"
	CallStatusCompleted = "completed"
	CallStatusFailed    = "failed"
	CallStatusCancelled = "cancelled"

	defaultSubqueryTimeout = 90 * time.Second
)

var unsafeNameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

type Clock func() time.Time

type Options struct {
	CacheHome     string
	Now           Clock
	Runner        Runner
	WorkerStarter WorkerStarter
	ProcessAlive  ProcessAliveFunc
	ProcessCancel ProcessCancelFunc
}

type RegisterOptions struct {
	Root             string
	Name             string
	Backend          string
	Tier             string
	Model            string
	Prompts          []string
	PromptRefs       []string
	SystemPromptText string
}

type CallOptions struct {
	Root   string
	Name   string
	Prompt string
}

type CallResult struct {
	AgentName  string
	Status     string
	PID        int
	SessionID  string
	PromptPath string
	StdoutPath string
	StderrPath string
}

type WaitOptions struct {
	Root    string
	Name    string
	Timeout time.Duration
	Poll    time.Duration
	Context context.Context
}

type TailOptions struct {
	Root  string
	Name  string
	Lines int
}

type DiagnosticStreamOptions struct {
	Root   string
	Name   string
	Stream string
	Lines  int
}

type AsyncWorkerRequest struct {
	Root       string
	Name       string
	PromptPath string
	StdoutPath string
	StderrPath string
}

type WorkerStarter interface {
	StartAsyncCall(AsyncWorkerRequest) (int, error)
}

type ProcessAliveFunc func(pid int) (bool, error)
type ProcessCancelFunc func(pid int) error

type syncCallOptions struct {
	Root    string
	Name    string
	Prompt  string
	Timeout time.Duration
}

type oneShotOptions struct {
	Root             string
	Name             string
	Backend          string
	Tier             string
	Model            string
	Prompts          []string
	PromptRefs       []string
	SystemPromptText string
	Prompt           string
	Timeout          time.Duration
}

type SubqueryOptions struct {
	Root         string
	Question     string
	DeepResearch bool
	Timeout      time.Duration
}

type SelfWorkerStarter struct{}

func (SelfWorkerStarter) StartAsyncCall(req AsyncWorkerRequest) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("locate ws-mcp executable: %w", err)
	}
	cmd := exec.Command(exe, "agents", "run-current", "--root", req.Root, "--name", req.Name)
	configureAsyncCommand(cmd)
	if req.StdoutPath != "" {
		stdout, err := os.OpenFile(req.StdoutPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return 0, fmt.Errorf("open async worker stdout: %w", err)
		}
		defer stdout.Close()
		cmd.Stdout = stdout
	}
	if req.StderrPath != "" {
		stderr, err := os.OpenFile(req.StderrPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return 0, fmt.Errorf("open async worker stderr: %w", err)
		}
		defer stderr.Close()
		cmd.Stderr = stderr
	}
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start async worker: %w", err)
	}
	go func() {
		_ = cmd.Wait()
	}()
	return cmd.Process.Pid, nil
}

type Agent struct {
	SchemaVersion    int             `json:"schema_version"`
	Name             string          `json:"name"`
	Backend          string          `json:"backend"`
	Tier             string          `json:"tier"`
	Model            string          `json:"model"`
	SessionID        string          `json:"session_id"`
	Status           string          `json:"status"`
	CreatedAt        string          `json:"created_at"`
	LastSeenAt       string          `json:"last_seen_at"`
	LastCallAt       string          `json:"last_call_at"`
	LastOutputPath   string          `json:"last_output_path"`
	PromptRefs       []string        `json:"prompt_refs"`
	SystemPromptPath string          `json:"system_prompt_path"`
	Capabilities     map[string]bool `json:"capabilities"`
}

type Message struct {
	SchemaVersion int    `json:"schema_version"`
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	CreatedAt     string `json:"created_at"`
	Status        string `json:"status"`
	Text          string `json:"text"`
}

type CurrentCall struct {
	SchemaVersion int    `json:"schema_version"`
	AgentName     string `json:"agent_name"`
	CallSeq       int    `json:"call_seq,omitempty"`
	ExecutionID   string `json:"execution_id,omitempty"`
	Status        string `json:"status"`
	PID           int    `json:"pid,omitempty"`
	StartedAt     string `json:"started_at,omitempty"`
	UpdatedAt     string `json:"updated_at"`
	FinishedAt    string `json:"finished_at,omitempty"`
	PromptPath    string `json:"prompt_path,omitempty"`
	StdoutPath    string `json:"stdout_path"`
	StderrPath    string `json:"stderr_path"`
	ExitCode      *int   `json:"exit_code,omitempty"`
	SessionID     string `json:"session_id,omitempty"`
	Error         string `json:"error,omitempty"`
	CleanupNeeded bool   `json:"cleanup_needed,omitempty"`
	CancelPID     int    `json:"cancel_pid,omitempty"`
}

type Layout struct {
	Root              string
	AgentDir          string
	AgentFile         string
	InboxDir          string
	OutboxDir         string
	CurrentDir        string
	CurrentStateFile  string
	CurrentStdout     string
	CurrentStderr     string
	CurrentRuntimeLog string
	OutputFile        string
	EventsFile        string
	SystemFile        string
}

type Manager struct {
	opts Options
}

func NewManager(opts Options) Manager {
	return Manager{opts: opts}
}

func (m Manager) Register(opts RegisterOptions) (Agent, Layout, error) {
	if strings.TrimSpace(opts.Root) == "" {
		opts.Root = "."
	}
	if strings.TrimSpace(opts.Backend) == "" {
		opts.Backend = "codex"
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		return Agent{}, Layout{}, errors.New("agent name is required")
	}
	promptSpecs := promptSpecs(opts.Prompts, opts.PromptRefs)
	resolved, err := wsprompt.Resolve(promptSpecs, opts.SystemPromptText, opts.Tier, opts.Model)
	if err != nil {
		return Agent{}, Layout{}, err
	}
	if strings.TrimSpace(opts.Tier) == "" {
		opts.Tier = resolved.Tier
	}
	if strings.TrimSpace(opts.Model) == "" {
		opts.Model = resolved.Model
	}
	if strings.TrimSpace(opts.Tier) == "" {
		opts.Tier = "core"
	}
	existingLayout, err := m.layout(opts.Root, name, false)
	if err != nil {
		return Agent{}, Layout{}, err
	}
	existingCall, err := readCurrentCall(existingLayout.CurrentStateFile)
	if err == nil && isActiveCallStatus(existingCall.Status) {
		return Agent{}, Layout{}, fmt.Errorf("agent %q has active call status %q", name, existingCall.Status)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Agent{}, Layout{}, err
	}
	if err := os.RemoveAll(existingLayout.AgentDir); err != nil {
		return Agent{}, Layout{}, fmt.Errorf("reset agent directory: %w", err)
	}
	layout, err := m.layout(opts.Root, name, true)
	if err != nil {
		return Agent{}, Layout{}, err
	}

	now := m.now().UTC().Format(time.RFC3339)
	agent := Agent{
		SchemaVersion:    schemaVersion,
		Name:             name,
		Backend:          opts.Backend,
		Tier:             opts.Tier,
		Model:            opts.Model,
		Status:           StatusIdle,
		CreatedAt:        now,
		LastSeenAt:       now,
		LastOutputPath:   "output.md",
		PromptRefs:       append([]string(nil), promptSpecs...),
		SystemPromptPath: "",
		Capabilities: map[string]bool{
			"resume":      true,
			"interrupt":   false,
			"compression": false,
		},
	}
	if strings.TrimSpace(resolved.Text) != "" {
		if err := os.WriteFile(layout.SystemFile, []byte(resolved.Text), 0o644); err != nil {
			return Agent{}, Layout{}, fmt.Errorf("write system prompt: %w", err)
		}
		agent.SystemPromptPath = "system.md"
	}
	if err := writeAgent(layout.AgentFile, agent); err != nil {
		return Agent{}, Layout{}, err
	}
	if err := appendEvent(layout.EventsFile, m.now(), "registered", map[string]any{
		"backend": agent.Backend,
		"tier":    agent.Tier,
		"model":   agent.Model,
	}); err != nil {
		return Agent{}, Layout{}, err
	}
	return agent, layout, nil
}

func (m Manager) syncCall(opts syncCallOptions) (Agent, string, error) {
	if strings.TrimSpace(opts.Root) == "" {
		opts.Root = "."
	}
	layout, err := m.layout(opts.Root, opts.Name, false)
	if err != nil {
		return Agent{}, "", err
	}
	agent, err := readAgent(layout.AgentFile)
	if err != nil {
		return Agent{}, "", err
	}
	if agent.Backend != "codex" {
		return agent, "", fmt.Errorf("unsupported agent backend %q", agent.Backend)
	}
	if strings.TrimSpace(opts.Prompt) == "" {
		return agent, "", errors.New("prompt is required")
	}

	text, agent, err := m.executeCall(layout, agent, executeCallOptions{
		Prompt:         opts.Prompt,
		CaptureStreams: false,
		Timeout:        opts.Timeout,
		ToolProfile:    "delegate",
	})
	return agent, text, err
}

type executeCallOptions struct {
	Prompt         string
	CaptureStreams bool
	Timeout        time.Duration
	ToolProfile    string
}

func (m Manager) executeCall(layout Layout, agent Agent, opts executeCallOptions) (string, Agent, error) {
	now := m.now().UTC().Format(time.RFC3339)
	agent.Status = StatusRunning
	agent.LastSeenAt = now
	agent.LastCallAt = now
	if err := writeAgent(layout.AgentFile, agent); err != nil {
		return "", agent, err
	}
	if err := appendEvent(layout.EventsFile, m.now(), "call.started", map[string]any{
		"backend": agent.Backend,
		"resume":  agent.SessionID != "",
	}); err != nil {
		return "", agent, err
	}
	_ = appendRuntimeLog(layout, m.now(), "backend.call.start", map[string]any{
		"backend":         agent.Backend,
		"resume":          agent.SessionID != "",
		"capture_streams": opts.CaptureStreams,
	})

	var stdoutFile *os.File
	var stderrFile *os.File
	var stdoutWriter io.Writer
	var stderrWriter io.Writer
	if opts.CaptureStreams {
		var err error
		stdoutFile, err = os.OpenFile(layout.CurrentStdout, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return "", agent, fmt.Errorf("open current stdout: %w", err)
		}
		defer stdoutFile.Close()
		stderrFile, err = os.OpenFile(layout.CurrentStderr, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return "", agent, fmt.Errorf("open current stderr: %w", err)
		}
		defer stderrFile.Close()
		stdoutWriter = stdoutFile
		stderrWriter = stderrFile
	}
	runner := m.opts.Runner
	if runner == nil {
		runner = CodexRunner{}
	}
	var onSessionID func(string) error
	if opts.CaptureStreams {
		onSessionID = func(sessionID string) error {
			if strings.TrimSpace(sessionID) == "" {
				return nil
			}
			agent.SessionID = sessionID
			agent.LastSeenAt = m.now().UTC().Format(time.RFC3339)
			if err := writeAgent(layout.AgentFile, agent); err != nil {
				return err
			}
			if _, err := m.MarkCurrentCallRunning(layout, os.Getpid(), sessionID); err != nil {
				return err
			}
			_ = appendRuntimeLog(layout, m.now(), "backend.session_started", map[string]any{
				"session_id": sessionID,
			})
			return appendEvent(layout.EventsFile, m.now(), "call.session_started", map[string]any{
				"session_id": sessionID,
			})
		}
	}
	result, err := runner.Call(RunnerRequest{
		Root:                layout.Root,
		Prompt:              opts.Prompt,
		Model:               agent.Model,
		SessionID:           agent.SessionID,
		SystemPromptPath:    absOptional(layout.AgentDir, agent.SystemPromptPath),
		Stdout:              stdoutWriter,
		Stderr:              stderrWriter,
		OnSessionID:         onSessionID,
		Timeout:             opts.Timeout,
		InheritProcessGroup: opts.CaptureStreams,
		ToolProfile:         opts.ToolProfile,
	})
	if err != nil {
		agent.Status = StatusFailed
		agent.LastSeenAt = m.now().UTC().Format(time.RFC3339)
		_ = writeAgent(layout.AgentFile, agent)
		_ = appendEvent(layout.EventsFile, m.now(), "call.failed", map[string]any{"error": err.Error()})
		_ = appendRuntimeLog(layout, m.now(), "backend.call.error", map[string]any{"error": err.Error()})
		return "", agent, err
	}
	_ = appendRuntimeLog(layout, m.now(), "backend.call.complete", map[string]any{
		"session_id": result.SessionID,
	})
	if result.SessionID != "" {
		agent.SessionID = result.SessionID
	}
	agent.Status = StatusIdle
	agent.LastSeenAt = m.now().UTC().Format(time.RFC3339)
	if err := os.WriteFile(layout.OutputFile, []byte(result.Text), 0o644); err != nil {
		agent.Status = StatusFailed
		_ = writeAgent(layout.AgentFile, agent)
		return "", agent, fmt.Errorf("write output: %w", err)
	}
	if err := writeAgent(layout.AgentFile, agent); err != nil {
		return "", agent, err
	}
	if err := appendEvent(layout.EventsFile, m.now(), "call.completed", map[string]any{
		"session_id": agent.SessionID,
	}); err != nil {
		return "", agent, err
	}
	_ = appendRuntimeLog(layout, m.now(), "state.output.write.ok", map[string]any{
		"output_path": "output.md",
	})
	return result.Text, agent, nil
}

func (m Manager) Call(opts CallOptions) (CallResult, error) {
	if strings.TrimSpace(opts.Root) == "" {
		opts.Root = "."
	}
	layout, err := m.layout(opts.Root, opts.Name, false)
	if err != nil {
		return CallResult{}, err
	}
	agent, err := readAgent(layout.AgentFile)
	if err != nil {
		return CallResult{}, err
	}
	if agent.Backend != "codex" {
		return CallResult{}, fmt.Errorf("unsupported agent backend %q", agent.Backend)
	}
	if strings.TrimSpace(opts.Prompt) == "" {
		return CallResult{}, errors.New("prompt is required")
	}

	call, err := m.BeginCurrentCall(layout, agent)
	if err != nil {
		return CallResult{}, err
	}
	promptPath := filepath.Join(layout.CurrentDir, "prompt.md")
	if err := os.WriteFile(promptPath, []byte(opts.Prompt), 0o644); err != nil {
		_, _ = m.FailCurrentCall(layout, fmt.Sprintf("write prompt snapshot: %v", err), nil)
		return CallResult{}, fmt.Errorf("write prompt snapshot: %w", err)
	}
	call.PromptPath = "current/prompt.md"
	if err := writeCurrentCall(layout.CurrentStateFile, call); err != nil {
		_, _ = m.FailCurrentCall(layout, fmt.Sprintf("record prompt snapshot: %v", err), nil)
		return CallResult{}, err
	}

	now := m.now().UTC().Format(time.RFC3339)
	agent.Status = StatusRunning
	agent.LastSeenAt = now
	agent.LastCallAt = now
	if err := writeAgent(layout.AgentFile, agent); err != nil {
		_, _ = m.FailCurrentCall(layout, fmt.Sprintf("mark agent running: %v", err), nil)
		return CallResult{}, err
	}
	if err := appendEvent(layout.EventsFile, m.now(), "call.queued", map[string]any{
		"backend":      agent.Backend,
		"resume":       agent.SessionID != "",
		"execution_id": call.ExecutionID,
	}); err != nil {
		_, _ = m.FailCurrentCall(layout, fmt.Sprintf("append async queue event: %v", err), nil)
		return CallResult{}, err
	}

	starter := m.opts.WorkerStarter
	if starter == nil {
		starter = SelfWorkerStarter{}
	}
	pid, err := starter.StartAsyncCall(AsyncWorkerRequest{
		Root:       opts.Root,
		Name:       agent.Name,
		PromptPath: promptPath,
		StdoutPath: layout.CurrentStdout,
		StderrPath: layout.CurrentStderr,
	})
	if err != nil {
		agent.Status = StatusFailed
		agent.LastSeenAt = m.now().UTC().Format(time.RFC3339)
		_ = writeAgent(layout.AgentFile, agent)
		_, _ = m.FailCurrentCall(layout, err.Error(), nil)
		_ = appendEvent(layout.EventsFile, m.now(), "call.failed", map[string]any{"error": err.Error()})
		return CallResult{}, err
	}
	call, err = m.MarkCurrentCallRunning(layout, pid, "")
	if err != nil {
		return CallResult{}, err
	}
	if err := appendEvent(layout.EventsFile, m.now(), "call.started_async", map[string]any{
		"pid":          pid,
		"execution_id": call.ExecutionID,
	}); err != nil {
		return CallResult{}, err
	}
	return CallResult{
		AgentName:  agent.Name,
		Status:     call.Status,
		PID:        call.PID,
		SessionID:  call.SessionID,
		PromptPath: call.PromptPath,
		StdoutPath: call.StdoutPath,
		StderrPath: call.StderrPath,
	}, nil
}

func (m Manager) RunCurrent(root, name string) (err error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	layout, err := m.layout(root, name, false)
	if err != nil {
		return err
	}
	agent, err := readAgent(layout.AgentFile)
	if err != nil {
		return err
	}
	call, err := readCurrentCall(layout.CurrentStateFile)
	if err != nil {
		return err
	}
	_ = appendRuntimeLog(layout, m.now(), "worker.entry", map[string]any{
		"pid":          os.Getpid(),
		"execution_id": call.ExecutionID,
	})
	defer func() {
		if recovered := recover(); recovered != nil {
			errText := fmt.Sprintf("panic: %v", recovered)
			_ = appendRuntimeLog(layout, m.now(), "worker.panic", map[string]any{"error": errText})
			_, _ = m.markAgentFailed(layout, errText)
			_, _ = m.FailCurrentCall(layout, errText, nil)
			err = errors.New(errText)
		}
	}()
	if !isActiveCallStatus(call.Status) {
		return fmt.Errorf("agent %q current call is not active: %s", agent.Name, call.Status)
	}
	promptPath := absOptional(layout.AgentDir, call.PromptPath)
	if promptPath == "" {
		return errors.New("current call missing prompt_path")
	}
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		_ = appendRuntimeLog(layout, m.now(), "prompt.read.error", map[string]any{"error": err.Error()})
		_, _ = m.markAgentFailed(layout, err.Error())
		_, _ = m.FailCurrentCall(layout, fmt.Sprintf("read prompt snapshot: %v", err), nil)
		return fmt.Errorf("read prompt snapshot: %w", err)
	}
	_ = appendRuntimeLog(layout, m.now(), "prompt.read.ok", map[string]any{"path": call.PromptPath})

	pid := os.Getpid()
	if _, err := m.MarkCurrentCallRunning(layout, pid, ""); err != nil {
		return err
	}
	if err := appendEvent(layout.EventsFile, m.now(), "call.worker_started", map[string]any{
		"pid":          pid,
		"execution_id": call.ExecutionID,
	}); err != nil {
		return err
	}
	text, resultAgent, runErr := m.executeCall(layout, agent, executeCallOptions{
		Prompt:         string(prompt),
		CaptureStreams: true,
		ToolProfile:    "leaf",
	})
	if runErr != nil {
		_ = appendRuntimeLog(layout, m.now(), "state.finalize.begin", map[string]any{"status": CallStatusFailed})
		_, _ = m.FailCurrentCall(layout, runErr.Error(), nil)
		_ = appendRuntimeLog(layout, m.now(), "state.finalize.end", map[string]any{"status": CallStatusFailed})
		return runErr
	}
	_ = appendRuntimeLog(layout, m.now(), "state.finalize.begin", map[string]any{"status": CallStatusCompleted})
	if _, err := m.CompleteCurrentCall(layout, resultAgent.SessionID, 0); err != nil {
		return err
	}
	_ = appendRuntimeLog(layout, m.now(), "state.finalize.end", map[string]any{"status": CallStatusCompleted})
	_ = text
	return nil
}

func (m Manager) oneShot(opts oneShotOptions) (string, error) {
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = fmt.Sprintf("oneshot-%d", m.now().UTC().UnixNano())
	}
	_, _, err := m.Register(RegisterOptions{
		Root:             opts.Root,
		Name:             name,
		Backend:          opts.Backend,
		Tier:             opts.Tier,
		Model:            opts.Model,
		Prompts:          opts.Prompts,
		PromptRefs:       opts.PromptRefs,
		SystemPromptText: opts.SystemPromptText,
	})
	if err != nil {
		return "", err
	}
	_, text, callErr := m.syncCall(syncCallOptions{
		Root:    opts.Root,
		Name:    name,
		Prompt:  opts.Prompt,
		Timeout: opts.Timeout,
	})
	eraseErr := m.Erase(opts.Root, name)
	if callErr != nil {
		return "", callErr
	}
	return text, eraseErr
}

func promptSpecs(prompts, promptRefs []string) []string {
	if len(prompts) > 0 {
		return append([]string(nil), prompts...)
	}
	return append([]string(nil), promptRefs...)
}

func (m Manager) Subquery(opts SubqueryOptions) (string, error) {
	tier := "light"
	if opts.DeepResearch {
		tier = "deep"
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultSubqueryTimeout
	}
	return m.oneShot(oneShotOptions{
		Root:             opts.Root,
		Backend:          "codex",
		Tier:             tier,
		SystemPromptText: SubquerySystemPrompt,
		Prompt:           opts.Question,
		Timeout:          timeout,
	})
}

func (m Manager) Print(root, name string) (string, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	layout, err := m.layout(root, name, false)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(layout.OutputFile)
	if err != nil {
		return "", fmt.Errorf("read output: %w", err)
	}
	return string(raw), nil
}

func (m Manager) Wait(opts WaitOptions) (string, error) {
	if strings.TrimSpace(opts.Root) == "" {
		opts.Root = "."
	}
	if opts.Poll <= 0 {
		opts.Poll = 200 * time.Millisecond
	}
	layout, err := m.layout(opts.Root, opts.Name, false)
	if err != nil {
		return "", err
	}
	deadline := time.Time{}
	if opts.Timeout > 0 {
		deadline = time.Now().Add(opts.Timeout)
	}
	for {
		if opts.Context != nil {
			select {
			case <-opts.Context.Done():
				_ = appendRuntimeLog(layout, m.now(), "wait.cancelled", map[string]any{
					"error": opts.Context.Err().Error(),
				})
				return m.Status(opts.Root, opts.Name)
			default:
			}
		}
		call, err := m.reconcileActiveCall(layout)
		if err != nil {
			return "", err
		}
		switch call.Status {
		case CallStatusCompleted:
			text, err := m.Print(opts.Root, opts.Name)
			if err != nil {
				return "", err
			}
			return text, nil
		case CallStatusFailed, CallStatusCancelled:
			return m.Status(opts.Root, opts.Name)
		}
		if opts.Timeout <= 0 {
			status, err := m.Status(opts.Root, opts.Name)
			if err != nil {
				return "", err
			}
			return "wait_pending: true\n" +
				"timeout_status: call may still be running\n" +
				"follow_up: agents.wait --timeout <duration> | agents.status | agents.cancel | agents.tail\n" +
				status, nil
		}
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			_ = appendRuntimeLog(layout, m.now(), "wait.timeout", map[string]any{
				"timeout_seconds": opts.Timeout.Seconds(),
				"call_status":     call.Status,
				"pid":             call.PID,
			})
			status, err := m.Status(opts.Root, opts.Name)
			if err != nil {
				return "", err
			}
			return "wait_timeout: true\n" +
				"timeout_status: call may still be running\n" +
				"follow_up: agents.wait | agents.status | agents.cancel | agents.tail\n" +
				status, nil
		}
		if opts.Context != nil {
			timer := time.NewTimer(opts.Poll)
			select {
			case <-opts.Context.Done():
				timer.Stop()
				_ = appendRuntimeLog(layout, m.now(), "wait.cancelled", map[string]any{
					"error": opts.Context.Err().Error(),
				})
				return m.Status(opts.Root, opts.Name)
			case <-timer.C:
			}
		} else {
			time.Sleep(opts.Poll)
		}
	}
}

func (m Manager) Status(root, name string) (string, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	layout, err := m.layout(root, name, false)
	if err != nil {
		return "", err
	}
	if _, err := m.reconcileActiveCall(layout); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	agent, err := readAgent(layout.AgentFile)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "agent: %s\n", agent.Name)
	fmt.Fprintf(&b, "agent_status: %s\n", agent.Status)
	fmt.Fprintf(&b, "backend: %s\n", agent.Backend)
	fmt.Fprintf(&b, "tier: %s\n", agent.Tier)
	if agent.Model != "" {
		fmt.Fprintf(&b, "model: %s\n", agent.Model)
	}
	if agent.SessionID != "" {
		fmt.Fprintf(&b, "session_id: %s\n", agent.SessionID)
	}
	if agent.LastCallAt != "" {
		fmt.Fprintf(&b, "last_call_at: %s\n", agent.LastCallAt)
	}
	call, err := readCurrentCall(layout.CurrentStateFile)
	if errors.Is(err, os.ErrNotExist) {
		b.WriteString("call_status: none\n")
		b.WriteString("active: false\n")
		return b.String(), nil
	}
	if err != nil {
		return "", err
	}
	fmt.Fprintf(&b, "call_status: %s\n", call.Status)
	fmt.Fprintf(&b, "active: %t\n", isActiveCallStatus(call.Status))
	if call.ExecutionID != "" {
		fmt.Fprintf(&b, "execution_id: %s\n", call.ExecutionID)
	}
	if call.PID != 0 {
		fmt.Fprintf(&b, "pid: %d\n", call.PID)
	}
	if call.SessionID != "" && call.SessionID != agent.SessionID {
		fmt.Fprintf(&b, "call_session_id: %s\n", call.SessionID)
	}
	if call.StartedAt != "" {
		fmt.Fprintf(&b, "started_at: %s\n", call.StartedAt)
	}
	if call.UpdatedAt != "" {
		fmt.Fprintf(&b, "updated_at: %s\n", call.UpdatedAt)
	}
	if call.FinishedAt != "" {
		fmt.Fprintf(&b, "finished_at: %s\n", call.FinishedAt)
	}
	if call.ExitCode != nil {
		fmt.Fprintf(&b, "exit_code: %d\n", *call.ExitCode)
	}
	if call.Error != "" {
		fmt.Fprintf(&b, "error: %s\n", call.Error)
	}
	if call.CancelPID != 0 {
		fmt.Fprintf(&b, "cancel_pid: %d\n", call.CancelPID)
	}
	fmt.Fprintf(&b, "cleanup_needed: %t\n", call.CleanupNeeded)
	if call.StdoutPath != "" {
		fmt.Fprintf(&b, "stdout_path: %s\n", call.StdoutPath)
	}
	if call.StderrPath != "" {
		fmt.Fprintf(&b, "stderr_path: %s\n", call.StderrPath)
	}
	fmt.Fprintf(&b, "runtime_log_path: %s\n", "current/runtime.jsonl")
	if call.Status == CallStatusCompleted {
		fmt.Fprintf(&b, "output_path: %s\n", agent.LastOutputPath)
	}
	fmt.Fprintf(&b, "follow_up: %s\n", followUpForCall(call))
	return b.String(), nil
}

func (m Manager) Tail(opts TailOptions) (string, error) {
	if strings.TrimSpace(opts.Root) == "" {
		opts.Root = "."
	}
	if opts.Lines <= 0 {
		opts.Lines = 40
	}
	layout, err := m.layout(opts.Root, opts.Name, false)
	if err != nil {
		return "", err
	}
	if _, err := readAgent(layout.AgentFile); err != nil {
		return "", err
	}
	sections := []struct {
		name string
		path string
	}{
		{name: "events", path: layout.EventsFile},
		{name: "runtime", path: layout.CurrentRuntimeLog},
		{name: "stdout", path: layout.CurrentStdout},
		{name: "stderr", path: layout.CurrentStderr},
		{name: "output", path: layout.OutputFile},
	}
	var b strings.Builder
	for _, section := range sections {
		fmt.Fprintf(&b, "== %s ==\n", section.name)
		lines, err := tailLines(section.path, opts.Lines)
		if errors.Is(err, os.ErrNotExist) {
			b.WriteString("(missing)\n")
			continue
		}
		if err != nil {
			return "", err
		}
		if len(lines) == 0 {
			b.WriteString("(empty)\n")
			continue
		}
		b.WriteString(strings.Join(lines, "\n"))
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func (m Manager) DiagnosticStream(opts DiagnosticStreamOptions) (string, error) {
	if strings.TrimSpace(opts.Root) == "" {
		opts.Root = "."
	}
	if opts.Lines <= 0 {
		opts.Lines = 40
	}
	layout, err := m.layout(opts.Root, opts.Name, false)
	if err != nil {
		return "", err
	}
	if _, err := readAgent(layout.AgentFile); err != nil {
		return "", err
	}
	path, err := diagnosticStreamPath(layout, opts.Stream)
	if err != nil {
		return "", err
	}
	lines, err := tailLines(path, opts.Lines)
	if errors.Is(err, os.ErrNotExist) {
		return "(missing)\n", nil
	}
	if err != nil {
		return "", err
	}
	if len(lines) == 0 {
		return "(empty)\n", nil
	}
	return strings.Join(lines, "\n") + "\n", nil
}

func diagnosticStreamPath(layout Layout, stream string) (string, error) {
	switch stream {
	case "stdout":
		return layout.CurrentStdout, nil
	case "stderr":
		return layout.CurrentStderr, nil
	case "runtime_log":
		return layout.CurrentRuntimeLog, nil
	case "events":
		return layout.EventsFile, nil
	default:
		return "", fmt.Errorf("unknown diagnostic stream %q", stream)
	}
}

func (m Manager) Cancel(root, name string) (string, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	layout, err := m.layout(root, name, false)
	if err != nil {
		return "", err
	}
	agent, err := readAgent(layout.AgentFile)
	if err != nil {
		return "", err
	}
	call, err := readCurrentCall(layout.CurrentStateFile)
	if err != nil {
		return "", err
	}
	if !isActiveCallStatus(call.Status) {
		return m.Status(root, name)
	}
	cancelledPID := call.PID
	_ = appendRuntimeLog(layout, m.now(), "cancel.begin", map[string]any{
		"pid":          cancelledPID,
		"execution_id": call.ExecutionID,
	})
	cancelErr := m.cancelProcessTree(cancelledPID)
	cleanupNeeded := false
	if cancelledPID != 0 {
		if alive, err := m.processAliveAfterCancel(cancelledPID); err != nil {
			cleanupNeeded = true
			_ = appendRuntimeLog(layout, m.now(), "cancel.liveness.error", map[string]any{
				"pid":   cancelledPID,
				"error": err.Error(),
			})
		} else if alive {
			cleanupNeeded = true
			_ = appendRuntimeLog(layout, m.now(), "cancel.cleanup_needed", map[string]any{
				"pid": cancelledPID,
			})
		}
	}
	now := m.now().UTC().Format(time.RFC3339)
	agent.Status = StatusIdle
	agent.LastSeenAt = now
	if err := writeAgent(layout.AgentFile, agent); err != nil {
		return "", err
	}
	errText := ""
	if cancelErr != nil {
		errText = cancelErr.Error()
	}
	call, err = m.CancelCurrentCall(layout, errText, cancelledPID, cleanupNeeded)
	if err != nil {
		return "", err
	}
	if err := appendEvent(layout.EventsFile, m.now(), "call.cancelled", map[string]any{
		"pid":            cancelledPID,
		"error":          errText,
		"cleanup_needed": cleanupNeeded,
	}); err != nil {
		return "", err
	}
	_ = appendRuntimeLog(layout, m.now(), "cancel.end", map[string]any{
		"pid":            cancelledPID,
		"error":          errText,
		"cleanup_needed": cleanupNeeded,
	})
	return m.Status(root, name)
}

func (m Manager) BeginCurrentCall(layout Layout, agent Agent) (CurrentCall, error) {
	existing, err := readCurrentCall(layout.CurrentStateFile)
	if err == nil && isActiveCallStatus(existing.Status) {
		return CurrentCall{}, fmt.Errorf("agent %q has active call status %q", agent.Name, existing.Status)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return CurrentCall{}, err
	}
	now := m.now().UTC().Format(time.RFC3339)
	call := CurrentCall{
		SchemaVersion: schemaVersion,
		AgentName:     agent.Name,
		CallSeq:       existing.CallSeq + 1,
		ExecutionID:   fmt.Sprintf("%06d", existing.CallSeq+1),
		Status:        CallStatusQueued,
		StartedAt:     now,
		UpdatedAt:     now,
		StdoutPath:    "current/stdout",
		StderrPath:    "current/stderr",
		SessionID:     agent.SessionID,
	}
	if err := os.MkdirAll(layout.CurrentDir, 0o755); err != nil {
		return CurrentCall{}, fmt.Errorf("create current call dir: %w", err)
	}
	for _, path := range []string{layout.CurrentStdout, layout.CurrentStderr, layout.CurrentRuntimeLog} {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			return CurrentCall{}, fmt.Errorf("reset current call stream: %w", err)
		}
	}
	if err := writeCurrentCall(layout.CurrentStateFile, call); err != nil {
		return CurrentCall{}, err
	}
	return call, nil
}

func (m Manager) MarkCurrentCallRunning(layout Layout, pid int, sessionID string) (CurrentCall, error) {
	call, err := readCurrentCall(layout.CurrentStateFile)
	if err != nil {
		return CurrentCall{}, err
	}
	if !isActiveCallStatus(call.Status) {
		return call, nil
	}
	call.Status = CallStatusRunning
	call.PID = pid
	call.UpdatedAt = m.now().UTC().Format(time.RFC3339)
	if strings.TrimSpace(sessionID) != "" {
		call.SessionID = sessionID
	}
	if err := writeCurrentCall(layout.CurrentStateFile, call); err != nil {
		return CurrentCall{}, err
	}
	return call, nil
}

func (m Manager) CompleteCurrentCall(layout Layout, sessionID string, exitCode int) (CurrentCall, error) {
	call, err := readCurrentCall(layout.CurrentStateFile)
	if err != nil {
		return CurrentCall{}, err
	}
	now := m.now().UTC().Format(time.RFC3339)
	call.Status = CallStatusCompleted
	call.UpdatedAt = now
	call.FinishedAt = now
	call.PID = 0
	call.ExitCode = &exitCode
	if strings.TrimSpace(sessionID) != "" {
		call.SessionID = sessionID
	}
	if err := writeCurrentCall(layout.CurrentStateFile, call); err != nil {
		return CurrentCall{}, err
	}
	return call, nil
}

func (m Manager) FailCurrentCall(layout Layout, errText string, exitCode *int) (CurrentCall, error) {
	call, err := readCurrentCall(layout.CurrentStateFile)
	if err != nil {
		return CurrentCall{}, err
	}
	now := m.now().UTC().Format(time.RFC3339)
	call.Status = CallStatusFailed
	call.UpdatedAt = now
	call.FinishedAt = now
	call.PID = 0
	call.ExitCode = exitCode
	call.Error = errText
	if err := writeCurrentCall(layout.CurrentStateFile, call); err != nil {
		return CurrentCall{}, err
	}
	return call, nil
}

func (m Manager) reconcileActiveCall(layout Layout) (CurrentCall, error) {
	call, err := readCurrentCall(layout.CurrentStateFile)
	if err != nil {
		return call, err
	}
	if call.Status != CallStatusRunning || call.PID == 0 {
		return call, nil
	}
	alive, err := m.processAlive(call.PID)
	if err != nil {
		_ = appendRuntimeLog(layout, m.now(), "worker.liveness.error", map[string]any{
			"pid":   call.PID,
			"error": err.Error(),
		})
		return call, nil
	}
	if alive {
		return call, nil
	}
	errText := fmt.Sprintf("async worker process %d is not running", call.PID)
	_ = appendRuntimeLog(layout, m.now(), "worker.dead", map[string]any{
		"pid":   call.PID,
		"error": errText,
	})
	_, _ = m.markAgentFailed(layout, errText)
	return m.FailCurrentCall(layout, errText, nil)
}

func (m Manager) markAgentFailed(layout Layout, errText string) (Agent, error) {
	agent, err := readAgent(layout.AgentFile)
	if err != nil {
		return agent, err
	}
	agent.Status = StatusFailed
	agent.LastSeenAt = m.now().UTC().Format(time.RFC3339)
	if err := writeAgent(layout.AgentFile, agent); err != nil {
		return agent, err
	}
	_ = appendEvent(layout.EventsFile, m.now(), "agent.failed", map[string]any{"error": errText})
	return agent, nil
}

func (m Manager) CancelCurrentCall(layout Layout, errText string, cancelPID int, cleanupNeeded bool) (CurrentCall, error) {
	call, err := readCurrentCall(layout.CurrentStateFile)
	if err != nil {
		return CurrentCall{}, err
	}
	now := m.now().UTC().Format(time.RFC3339)
	call.Status = CallStatusCancelled
	call.UpdatedAt = now
	call.FinishedAt = now
	call.PID = 0
	call.Error = errText
	call.CancelPID = cancelPID
	call.CleanupNeeded = cleanupNeeded
	if err := writeCurrentCall(layout.CurrentStateFile, call); err != nil {
		return CurrentCall{}, err
	}
	return call, nil
}

func ResetCurrentCall(layout Layout) error {
	if err := os.RemoveAll(layout.CurrentDir); err != nil {
		return fmt.Errorf("reset current call: %w", err)
	}
	if err := os.MkdirAll(layout.CurrentDir, 0o755); err != nil {
		return fmt.Errorf("create current call dir: %w", err)
	}
	return nil
}

func (m Manager) Erase(root, name string) error {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	layout, err := m.layout(root, name, false)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(layout.AgentDir); err != nil {
		return fmt.Errorf("erase agent: %w", err)
	}
	return nil
}

func (m Manager) layout(root, name string, create bool) (Layout, error) {
	state, _, _, err := wsstate.NewManager(wsstate.Options{
		CacheHome: m.opts.CacheHome,
		Now:       wsstate.Clock(m.now),
	}).Ensure(root)
	if err != nil {
		return Layout{}, err
	}
	key := AgentKey(name)
	if key == "" {
		return Layout{}, errors.New("agent name resolves to empty path key")
	}
	dir := filepath.Join(state.AgentsDir, key)
	layout := Layout{
		Root:              root,
		AgentDir:          dir,
		AgentFile:         filepath.Join(dir, "agent.json"),
		InboxDir:          filepath.Join(dir, "inbox"),
		OutboxDir:         filepath.Join(dir, "outbox"),
		CurrentDir:        filepath.Join(dir, "current"),
		CurrentStateFile:  filepath.Join(dir, "current", "state.json"),
		CurrentStdout:     filepath.Join(dir, "current", "stdout"),
		CurrentStderr:     filepath.Join(dir, "current", "stderr"),
		CurrentRuntimeLog: filepath.Join(dir, "current", "runtime.jsonl"),
		OutputFile:        filepath.Join(dir, "output.md"),
		EventsFile:        filepath.Join(dir, "events.jsonl"),
		SystemFile:        filepath.Join(dir, "system.md"),
	}
	if create {
		for _, path := range []string{layout.InboxDir, layout.OutboxDir, layout.CurrentDir} {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return Layout{}, fmt.Errorf("create %s: %w", path, err)
			}
		}
	}
	return layout, nil
}

func (m Manager) now() time.Time {
	if m.opts.Now != nil {
		return m.opts.Now()
	}
	return time.Now()
}

func (m Manager) processAlive(pid int) (bool, error) {
	if m.opts.ProcessAlive != nil {
		return m.opts.ProcessAlive(pid)
	}
	return processAlive(pid)
}

func (m Manager) cancelProcessTree(pid int) error {
	if pid <= 0 {
		return nil
	}
	if m.opts.ProcessCancel != nil {
		return m.opts.ProcessCancel(pid)
	}
	return cancelAsyncProcessTree(pid)
}

func (m Manager) processAliveAfterCancel(pid int) (bool, error) {
	if m.opts.ProcessAlive != nil {
		return m.opts.ProcessAlive(pid)
	}
	var lastErr error
	for attempt := 0; attempt < 6; attempt++ {
		alive, err := m.processAlive(pid)
		if err != nil {
			lastErr = err
		}
		if err == nil && !alive {
			return false, nil
		}
		if attempt < 5 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if lastErr != nil {
		return false, lastErr
	}
	return true, nil
}

func followUpForCall(call CurrentCall) string {
	switch call.Status {
	case CallStatusQueued, CallStatusRunning:
		return "agents.wait | agents.status | agents.cancel | agents.tail"
	case CallStatusCompleted:
		return "agents.print | agents.tail"
	case CallStatusFailed:
		return "agents.tail | agents.erase"
	case CallStatusCancelled:
		if call.CleanupNeeded {
			return "inspect runtime log | manual cleanup | agents.erase"
		}
		return "agents.tail | agents.erase"
	default:
		return "agents.status"
	}
}

func AgentKey(name string) string {
	name = strings.TrimSpace(name)
	name = unsafeNameChars.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	return name
}

func readAgent(path string) (Agent, error) {
	var agent Agent
	raw, err := os.ReadFile(path)
	if err != nil {
		return agent, fmt.Errorf("read agent: %w", err)
	}
	if err := json.Unmarshal(raw, &agent); err != nil {
		return agent, fmt.Errorf("parse agent: %w", err)
	}
	return agent, nil
}

func writeAgent(path string, agent Agent) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create agent dir: %w", err)
	}
	raw, err := json.MarshalIndent(agent, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal agent: %w", err)
	}
	tmp := uniqueTempPath(path)
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("write agent: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace agent: %w", err)
	}
	return nil
}

func readCurrentCall(path string) (CurrentCall, error) {
	var call CurrentCall
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return call, os.ErrNotExist
		}
		return call, fmt.Errorf("read current call: %w", err)
	}
	if err := json.Unmarshal(raw, &call); err != nil {
		return call, fmt.Errorf("parse current call: %w", err)
	}
	return call, nil
}

func writeCurrentCall(path string, call CurrentCall) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create current call dir: %w", err)
	}
	raw, err := json.MarshalIndent(call, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal current call: %w", err)
	}
	tmp := uniqueTempPath(path)
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("write current call: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace current call: %w", err)
	}
	return nil
}

func isActiveCallStatus(status string) bool {
	return status == CallStatusQueued || status == CallStatusRunning
}

func appendEvent(path string, now time.Time, event string, fields map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create event dir: %w", err)
	}
	record := map[string]any{
		"timestamp": now.UTC().Format(time.RFC3339),
		"event":     event,
	}
	for key, value := range fields {
		record[key] = value
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open event log: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	return nil
}

func appendRuntimeLog(layout Layout, now time.Time, event string, fields map[string]any) error {
	return appendEvent(layout.CurrentRuntimeLog, now, event, fields)
}

func tailLines(path string, count int) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > count {
			copy(lines, lines[1:])
			lines = lines[:count]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func uniqueTempPath(path string) string {
	return fmt.Sprintf("%s.%d.%d.tmp", path, os.Getpid(), time.Now().UnixNano())
}

func absOptional(base, rel string) string {
	if rel == "" {
		return ""
	}
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(base, rel)
}
