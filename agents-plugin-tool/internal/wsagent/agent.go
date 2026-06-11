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
	"sort"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsprompt"
	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wsstore"
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

	defaultAgentWaitTimeout = 10 * time.Minute
)

const (
	tailMaxFieldRunes = 2000
	tailMaxLineRunes  = 6000
	backendErrorRunes = 4000
)

const defaultRecallPrompt = "Continue from the previous session. This is a recovery retry after the lead observed a result timeout and no useful activity in diagnostics; resume the assigned task from the last useful state and report progress."
const cancelRecoveryTip = "If this was cancelled because the agent did not respond and no result is available, call the same registered agent again with a recovery prompt; ws will resume the stored session when the backend supports it."

var tailLargeFieldKeys = map[string]struct{}{
	"aggregated_output": {},
	"content":           {},
	"output":            {},
	"stderr":            {},
	"stdout":            {},
	"text":              {},
}

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
	Root                  string
	Name                  string
	Backend               string
	Harness               string
	Tier                  string
	Model                 string
	Prompts               []string
	PromptRefs            []string
	ConditionalPromptRefs []ConditionalPromptRef
	SystemPromptText      string
	SuppressOrientation   bool
	Ephemeral             bool
}

type ConditionalPromptRef struct {
	Binary    string
	PromptRef string
}

type CallOptions struct {
	Root   string
	Name   string
	Prompt string
}

type RecallOptions struct {
	Root   string
	Name   string
	Prompt string
}

type InterruptOptions struct {
	Root    string
	Name    string
	Message string
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

type InterruptResult struct {
	AgentName string
	MessageID string
	Queued    bool
}

type WaitOptions struct {
	Root    string
	Name    string
	Names   []string
	Timeout time.Duration
	Poll    time.Duration
	Context context.Context
}

type ResultOptions struct {
	Root              string
	Name              string
	Timeout           time.Duration
	Poll              time.Duration
	Context           context.Context
	OnEphemeralErased func(Agent)
}

type TailOptions struct {
	Root  string
	Name  string
	Lines int
	Raw   bool
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

type asyncWorkerCommand struct {
	Path string
	Args []string
}

type syncCallOptions struct {
	Root    string
	Name    string
	Prompt  string
	Timeout time.Duration
}

type SelfWorkerStarter struct{}

func (SelfWorkerStarter) StartAsyncCall(req AsyncWorkerRequest) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("locate ws-mcp executable: %w", err)
	}
	worker, err := asyncWorkerCommandFor(exe)
	if err != nil {
		return 0, err
	}
	args := asyncWorkerArgs(worker, req)
	cmd := exec.Command(worker.Path, args...)
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

func asyncWorkerArgs(worker asyncWorkerCommand, req AsyncWorkerRequest) []string {
	args := append([]string{}, worker.Args...)
	args = append(args, "agents", "run-current", "--root", req.Root, "--name", req.Name)
	return args
}

func asyncWorkerCommandFor(exe string) (asyncWorkerCommand, error) {
	candidates := []asyncWorkerCommand{}
	if env := strings.TrimSpace(os.Getenv("WS_MCP_RUNTIME_BINARY")); env != "" {
		candidates = append(candidates, asyncWorkerCommand{Path: env})
	}
	if strings.TrimSpace(exe) != "" {
		candidates = append(candidates, asyncWorkerCommand{Path: exe})
	}
	for _, candidate := range candidates {
		if regularFileExists(candidate.Path) {
			return candidate, nil
		}
	}
	if command, ok := cacheLauncherCommand(exe); ok {
		return command, nil
	}
	if exe != "" {
		if path, err := exec.LookPath(filepath.Base(exe)); err == nil && regularFileExists(path) {
			return asyncWorkerCommand{Path: path}, nil
		}
	}
	return asyncWorkerCommand{}, fmt.Errorf("locate async worker executable: current executable %q is unavailable and no repaired launcher was found", exe)
}

func cacheLauncherCommand(exe string) (asyncWorkerCommand, bool) {
	cacheRoot, ok := codexPluginCacheRoot(exe)
	if !ok {
		return asyncWorkerCommand{}, false
	}
	pluginDirs, err := filepath.Glob(filepath.Join(cacheRoot, "ws", "*"))
	if err != nil || len(pluginDirs) == 0 {
		return asyncWorkerCommand{}, false
	}
	sort.Strings(pluginDirs)
	for i := len(pluginDirs) - 1; i >= 0; i-- {
		shim := filepath.Join(pluginDirs[i], "bin", "ws-mcp-launcher")
		if regularFileExists(shim) {
			return asyncWorkerCommand{Path: shim}, true
		}
		py := filepath.Join(pluginDirs[i], "bin", "ws-mcp-launcher.py")
		if regularFileExists(py) {
			if python, err := exec.LookPath("python3"); err == nil {
				return asyncWorkerCommand{Path: python, Args: []string{py}}, true
			}
			if python, err := exec.LookPath("python"); err == nil {
				return asyncWorkerCommand{Path: python, Args: []string{py}}, true
			}
		}
	}
	return asyncWorkerCommand{}, false
}

func codexPluginCacheRoot(path string) (string, bool) {
	clean := filepath.Clean(path)
	marker := filepath.Join(".codex", "plugins", "cache", "kang-sw-devenv")
	index := strings.Index(clean, marker)
	if index < 0 {
		return "", false
	}
	root := clean[:index+len(marker)]
	if root == "" {
		return "", false
	}
	return root, true
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

type Agent struct {
	SchemaVersion    int             `json:"schema_version"`
	Name             string          `json:"name"`
	Backend          string          `json:"backend"`
	Harness          string          `json:"harness,omitempty"`
	Tier             string          `json:"tier"`
	Model            string          `json:"model"`
	Effort           string          `json:"effort,omitempty"`
	SessionID        string          `json:"session_id"`
	Status           string          `json:"status"`
	CreatedAt        string          `json:"created_at"`
	LastSeenAt       string          `json:"last_seen_at"`
	LastCallAt       string          `json:"last_call_at"`
	LastOutputPath   string          `json:"last_output_path"`
	PromptRefs       []string        `json:"prompt_refs"`
	SystemPromptPath string          `json:"system_prompt_path"`
	Capabilities     map[string]bool `json:"capabilities"`
	Ephemeral        bool            `json:"ephemeral,omitempty"`
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
	Name              string
	AgentDir          string
	AgentFile         string
	InboxDir          string
	InboxLockFile     string
	OutboxDir         string
	CurrentDir        string
	CurrentLockFile   string
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

func (m Manager) registryStore(root string) (*wsstore.Store, error) {
	return wsstore.NewManager(wsstore.Options{CacheHome: m.opts.CacheHome, Now: wsstore.Clock(m.now)}).Open(root)
}

func agentDefinitionFromAgent(key, statePath string, agent Agent) wsstore.AgentDefinition {
	return wsstore.AgentDefinition{
		AgentKey:         key,
		PublicName:       agent.Name,
		StatePath:        statePath,
		SchemaVersion:    agent.SchemaVersion,
		Backend:          agent.Backend,
		Harness:          agent.Harness,
		Tier:             agent.Tier,
		Model:            agent.Model,
		Effort:           agent.Effort,
		SessionID:        agent.SessionID,
		Status:           agent.Status,
		CreatedAt:        agent.CreatedAt,
		LastSeenAt:       agent.LastSeenAt,
		LastCallAt:       agent.LastCallAt,
		LastOutputPath:   agent.LastOutputPath,
		PromptRefs:       append([]string(nil), agent.PromptRefs...),
		SystemPromptPath: agent.SystemPromptPath,
		Capabilities:     copyCapabilities(agent.Capabilities),
		Ephemeral:        agent.Ephemeral,
	}
}

func agentFromDefinition(def wsstore.AgentDefinition) Agent {
	return Agent{
		SchemaVersion:    def.SchemaVersion,
		Name:             def.PublicName,
		Backend:          def.Backend,
		Harness:          def.Harness,
		Tier:             def.Tier,
		Model:            def.Model,
		Effort:           def.Effort,
		SessionID:        def.SessionID,
		Status:           def.Status,
		CreatedAt:        def.CreatedAt,
		LastSeenAt:       def.LastSeenAt,
		LastCallAt:       def.LastCallAt,
		LastOutputPath:   def.LastOutputPath,
		PromptRefs:       append([]string(nil), def.PromptRefs...),
		SystemPromptPath: def.SystemPromptPath,
		Capabilities:     copyCapabilities(def.Capabilities),
		Ephemeral:        def.Ephemeral,
	}
}

func copyCapabilities(in map[string]bool) map[string]bool {
	if in == nil {
		return nil
	}
	out := make(map[string]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (m Manager) registryKey(name string) (string, error) {
	return wsstore.AgentInternalKey(name)
}

func (m Manager) readAgentMetadata(layout Layout, name string) (Agent, error) {
	store, err := m.registryStore(layout.Root)
	if err != nil {
		return Agent{}, err
	}
	defer store.Close()
	key, err := m.registryKey(name)
	if err != nil {
		return Agent{}, err
	}
	if def, ok, err := store.AgentDefinition(context.Background(), key); err != nil {
		return Agent{}, err
	} else if ok {
		return agentFromDefinition(def), nil
	}
	legacy, err := readAgent(layout.AgentFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Agent{}, err
		}
		return Agent{}, fmt.Errorf("legacy agent.json recovery required for %q: %w", name, err)
	}
	def := agentDefinitionFromAgent(key, AgentKey(name), legacy)
	if err := store.UpsertAgentDefinition(context.Background(), def); err != nil {
		return Agent{}, fmt.Errorf("import legacy agent.json for %q: %w", name, err)
	}
	if err := os.Remove(layout.AgentFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Agent{}, fmt.Errorf("retire imported agent.json for %q: %w", name, err)
	}
	return legacy, nil
}

func (m Manager) writeAgentMetadata(layout Layout, agent Agent) error {
	store, err := m.registryStore(layout.Root)
	if err != nil {
		return err
	}
	defer store.Close()
	key, err := m.registryKey(agent.Name)
	if err != nil {
		return err
	}
	statePath := filepath.Base(layout.AgentDir)
	return store.UpsertAgentDefinition(context.Background(), agentDefinitionFromAgent(key, statePath, agent))
}

func (m Manager) deleteAgentMetadata(root, name string) error {
	store, err := m.registryStore(root)
	if err != nil {
		return err
	}
	defer store.Close()
	key, err := m.registryKey(name)
	if err != nil {
		return err
	}
	return store.DeleteAgentDefinition(context.Background(), key)
}

func (m Manager) Register(opts RegisterOptions) (Agent, Layout, error) {
	if strings.TrimSpace(opts.Root) == "" {
		opts.Root = "."
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		return Agent{}, Layout{}, errors.New("agent name is required")
	}
	explicitBackend := strings.TrimSpace(opts.Backend)
	promptSpecs := promptSpecs(opts.Prompts, opts.PromptRefs)
	if !opts.SuppressOrientation && (len(promptSpecs) == 0 || promptSpecs[0] != "delegate-orientation") {
		promptSpecs = append([]string{"delegate-orientation"}, promptSpecs...)
	}
	conditionalSpecs, err := m.resolveConditionalPromptRefs(opts.ConditionalPromptRefs)
	if err != nil {
		return Agent{}, Layout{}, err
	}
	promptSpecs = append(promptSpecs, conditionalSpecs...)
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
	if alias := wsconfig.ModelAlias(opts.Model); alias != "" {
		opts.Tier = alias
	}
	if strings.TrimSpace(opts.Tier) == "" {
		opts.Tier = "core"
	}
	resolvedBackend, resolvedModel, resolvedEffort, err := wsconfig.ResolveAgentForHarnessConfig(wsconfig.Options{CacheHome: m.opts.CacheHome}, opts.Tier, explicitBackend, opts.Model, opts.Harness)
	if err != nil {
		return Agent{}, Layout{}, err
	}
	opts.Backend = resolvedBackend
	opts.Model = resolvedModel
	existingLayout, err := m.scopedLayout(opts.Root, name, false)
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
	layout, err := m.scopedLayout(opts.Root, name, true)
	if err != nil {
		return Agent{}, Layout{}, err
	}

	now := m.now().UTC().Format(time.RFC3339)
	agent := Agent{
		SchemaVersion:    schemaVersion,
		Name:             name,
		Backend:          opts.Backend,
		Harness:          opts.Harness,
		Tier:             opts.Tier,
		Model:            opts.Model,
		Effort:           resolvedEffort,
		Status:           StatusIdle,
		CreatedAt:        now,
		LastSeenAt:       now,
		LastOutputPath:   "output.md",
		PromptRefs:       append([]string(nil), promptSpecs...),
		SystemPromptPath: "",
		Ephemeral:        opts.Ephemeral,
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
	if err := appendEvent(layout.EventsFile, m.now(), "registered", map[string]any{
		"backend": agent.Backend,
		"tier":    agent.Tier,
		"model":   agent.Model,
		"effort":  agent.Effort,
	}); err != nil {
		return Agent{}, Layout{}, err
	}
	if err := m.writeAgentMetadata(layout, agent); err != nil {
		return Agent{}, Layout{}, err
	}
	return agent, layout, nil
}

func (m Manager) Agent(root, name string) (Agent, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	layout, err := m.scopedLayout(root, name, false)
	if err != nil {
		return Agent{}, err
	}
	return m.readAgentMetadata(layout, name)
}

func (m Manager) resolveConditionalPromptRefs(refs []ConditionalPromptRef) ([]string, error) {
	var specs []string
	for _, ref := range refs {
		binary := strings.TrimSpace(ref.Binary)
		if binary == "" {
			return nil, errors.New("conditional prompt binary is required")
		}
		if _, err := exec.LookPath(binary); err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				continue
			}
			return nil, fmt.Errorf("resolve conditional prompt binary %q: %w", binary, err)
		}
		promptRef := strings.TrimSpace(ref.PromptRef)
		if promptRef == "" {
			promptRef = binary
		}
		specs = append(specs, promptRef)
	}
	return specs, nil
}

func (m Manager) syncCall(opts syncCallOptions) (Agent, string, error) {
	if strings.TrimSpace(opts.Root) == "" {
		opts.Root = "."
	}
	layout, err := m.scopedLayout(opts.Root, opts.Name, false)
	if err != nil {
		return Agent{}, "", err
	}
	agent, err := m.readAgentMetadata(layout, opts.Name)
	if err != nil {
		return Agent{}, "", err
	}
	if strings.TrimSpace(opts.Prompt) == "" {
		return agent, "", errors.New("prompt is required")
	}

	text, agent, err := m.executeCall(layout, agent, executeCallOptions{
		Prompt:         opts.Prompt,
		CaptureStreams: false,
		Timeout:        opts.Timeout,
	})
	return agent, text, err
}

type executeCallOptions struct {
	Prompt         string
	CaptureStreams bool
	Timeout        time.Duration
}

func (m Manager) executeCall(layout Layout, agent Agent, opts executeCallOptions) (string, Agent, error) {
	now := m.now().UTC().Format(time.RFC3339)
	agent.Status = StatusRunning
	agent.LastSeenAt = now
	agent.LastCallAt = now
	if err := m.writeAgentMetadata(layout, agent); err != nil {
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
		var err error
		runner, err = runnerForBackend(agent.Backend)
		if err != nil {
			diagnostic := backendInvocationError(agent, err)
			agent.Status = StatusFailed
			agent.LastSeenAt = m.now().UTC().Format(time.RFC3339)
			_ = m.writeAgentMetadata(layout, agent)
			_ = appendEvent(layout.EventsFile, m.now(), "call.failed", map[string]any{"error": diagnostic.Error()})
			_ = appendRuntimeLog(layout, m.now(), "backend.call.error", map[string]any{"error": diagnostic.Error()})
			return "", agent, diagnostic
		}
	}
	hookCommand := ""
	if opts.CaptureStreams {
		hookCommand = interruptHookCommand(layout.Root, agent.Name)
	}
	var onSessionID func(string) error
	if opts.CaptureStreams {
		onSessionID = func(sessionID string) error {
			if strings.TrimSpace(sessionID) == "" {
				return nil
			}
			agent.SessionID = sessionID
			agent.LastSeenAt = m.now().UTC().Format(time.RFC3339)
			if err := m.writeAgentMetadata(layout, agent); err != nil {
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
	prompt := opts.Prompt
	var result RunnerResult
	var err error
	messages, err := m.deliverPendingInbox(layout, "resume")
	if err != nil {
		return "", agent, err
	}
	if len(messages) > 0 {
		prompt = composeLeadMessagePrompt(messages, prompt)
	}
	_ = appendRuntimeLog(layout, m.now(), "backend.prompt.delivery", map[string]any{
		"backend":          agent.Backend,
		"resume":           agent.SessionID != "",
		"prompt_byte_size": len([]byte(prompt)),
	})
	result, err = runner.Call(RunnerRequest{
		Root:                 layout.Root,
		Prompt:               prompt,
		Model:                agent.Model,
		Effort:               agent.Effort,
		SessionID:            agent.SessionID,
		SystemPromptPath:     absOptional(layout.AgentDir, agent.SystemPromptPath),
		InterruptHookCommand: hookCommand,
		Stdout:               stdoutWriter,
		Stderr:               stderrWriter,
		OnSessionID:          onSessionID,
		Timeout:              opts.Timeout,
		InheritProcessGroup:  opts.CaptureStreams,
	})
	if err != nil {
		diagnostic := backendInvocationError(agent, err)
		agent.Status = StatusFailed
		agent.LastSeenAt = m.now().UTC().Format(time.RFC3339)
		_ = m.writeAgentMetadata(layout, agent)
		_ = appendEvent(layout.EventsFile, m.now(), "call.failed", map[string]any{"error": diagnostic.Error()})
		_ = appendRuntimeLog(layout, m.now(), "backend.call.error", map[string]any{"error": diagnostic.Error()})
		return "", agent, diagnostic
	}
	if result.SessionID != "" {
		agent.SessionID = result.SessionID
	}
	completeFields := map[string]any{
		"session_id": result.SessionID,
	}
	if result.BackendVersion != "" {
		completeFields["backend_version"] = result.BackendVersion
	}
	if result.PromptDelivery != "" {
		completeFields["prompt_delivery"] = result.PromptDelivery
	}
	if result.FinalEventShape != "" {
		completeFields["final_event_shape"] = result.FinalEventShape
	}
	_ = appendRuntimeLog(layout, m.now(), "backend.call.complete", completeFields)
	if result.SessionID != "" {
		agent.SessionID = result.SessionID
	}
	agent.Status = StatusIdle
	agent.LastSeenAt = m.now().UTC().Format(time.RFC3339)
	if err := os.WriteFile(layout.OutputFile, []byte(result.Text), 0o644); err != nil {
		agent.Status = StatusFailed
		_ = m.writeAgentMetadata(layout, agent)
		return "", agent, fmt.Errorf("write output: %w", err)
	}
	if err := m.writeAgentMetadata(layout, agent); err != nil {
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
	layout, err := m.scopedLayout(opts.Root, opts.Name, false)
	if err != nil {
		return CallResult{}, err
	}
	agent, err := m.readAgentMetadata(layout, opts.Name)
	if err != nil {
		return CallResult{}, err
	}
	if strings.TrimSpace(opts.Prompt) == "" {
		return CallResult{}, errors.New("prompt is required")
	}

	unlock, err := m.acquireCurrentCallLock(layout)
	if err != nil {
		return CallResult{}, err
	}
	defer unlock()

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
	if err := m.writeAgentMetadata(layout, agent); err != nil {
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
		_ = m.writeAgentMetadata(layout, agent)
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

func (m Manager) Recall(opts RecallOptions) (string, error) {
	if strings.TrimSpace(opts.Root) == "" {
		opts.Root = "."
	}
	layout, err := m.scopedLayout(opts.Root, opts.Name, false)
	if err != nil {
		return "", err
	}
	if _, err := m.readAgentMetadata(layout, opts.Name); err != nil {
		return "", err
	}
	cancelled := false
	if call, err := m.reconcileActiveCall(layout); err == nil && isActiveCallStatus(call.Status) {
		cancelled = true
		if _, err := m.cancelScoped(opts.Root, opts.Name); err != nil {
			return "", err
		}
		cancelledCall, err := readCurrentCall(layout.CurrentStateFile)
		if err != nil {
			return "", err
		}
		if cancelledCall.CleanupNeeded {
			status, statusErr := m.statusScoped(opts.Root, opts.Name)
			if statusErr != nil {
				return "", statusErr
			}
			return status + "recall_recovery_only: true\nrecall_aborted: cleanup_needed\nrecall_guidance: manual cleanup is required before retrying.\n", nil
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	prompt := strings.TrimSpace(opts.Prompt)
	if prompt == "" {
		prompt = defaultRecallPrompt
	}
	result, err := m.Call(CallOptions{
		Root:   opts.Root,
		Name:   opts.Name,
		Prompt: prompt,
	})
	if err != nil {
		return "", err
	}
	if err := appendEvent(layout.EventsFile, m.now(), "recall.started", map[string]any{
		"cancelled_active_call": cancelled,
		"pid":                   result.PID,
	}); err != nil {
		return "", err
	}
	_ = appendRuntimeLog(layout, m.now(), "recall.started", map[string]any{
		"cancelled_active_call": cancelled,
		"pid":                   result.PID,
	})
	return fmt.Sprintf("recall_recovery_only: true\nrecall_cancelled_active_call: %t\n%s\t%s\tpid=%d\nfollow_up: agents.result --timeout 10m | agents.tail | agents.status | agents.cancel\n", cancelled, result.AgentName, result.Status, result.PID), nil
}

func (m Manager) Interrupt(opts InterruptOptions) (InterruptResult, error) {
	if strings.TrimSpace(opts.Root) == "" {
		opts.Root = "."
	}
	layout, err := m.scopedLayout(opts.Root, opts.Name, false)
	if err != nil {
		return InterruptResult{}, err
	}
	agent, err := m.readAgentMetadata(layout, opts.Name)
	if err != nil {
		return InterruptResult{}, err
	}
	message := strings.TrimSpace(opts.Message)
	if message == "" {
		return InterruptResult{}, errors.New("interrupt message is required")
	}
	msg, err := m.enqueueInboxMessage(layout, "interrupt", message)
	if err != nil {
		return InterruptResult{}, err
	}
	if err := appendEvent(layout.EventsFile, m.now(), "inbox.queued", map[string]any{
		"message_id": msg.ID,
		"kind":       msg.Kind,
	}); err != nil {
		return InterruptResult{}, err
	}
	_ = appendRuntimeLog(layout, m.now(), "inbox.queued", map[string]any{
		"message_id": msg.ID,
		"kind":       msg.Kind,
	})
	return InterruptResult{AgentName: agent.Name, MessageID: msg.ID, Queued: true}, nil
}

func (m Manager) RunCurrent(root, name string) (err error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	layout, err := m.scopedLayout(root, name, false)
	if err != nil {
		return err
	}
	agent, err := m.readAgentMetadata(layout, name)
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

func promptSpecs(prompts, promptRefs []string) []string {
	if len(prompts) > 0 {
		return append([]string(nil), prompts...)
	}
	return append([]string(nil), promptRefs...)
}

func (m Manager) Print(root, name string) (string, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	layout, err := m.scopedLayout(root, name, false)
	if err != nil {
		return "", err
	}
	if _, err := m.readAgentMetadata(layout, name); err != nil {
		return "", err
	}
	raw, err := os.ReadFile(layout.OutputFile)
	if err != nil {
		return "", fmt.Errorf("read output: %w", err)
	}
	return string(raw), nil
}

func (m Manager) Result(opts ResultOptions) (string, error) {
	if strings.TrimSpace(opts.Root) == "" {
		opts.Root = "."
	}
	if opts.Poll <= 0 {
		opts.Poll = 200 * time.Millisecond
	}
	layout, err := m.scopedLayout(opts.Root, opts.Name, false)
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
				return m.resultStatusText(layout, opts.Name, "result_cancelled: true\n")
			default:
			}
		}
		call, err := m.reconcileActiveCall(layout)
		if errors.Is(err, os.ErrNotExist) {
			return m.resultStatusText(layout, opts.Name, "result_ready: false\n")
		}
		if err != nil {
			return "", err
		}
		switch call.Status {
		case CallStatusCompleted:
			agent, err := m.readAgentMetadata(layout, opts.Name)
			if err != nil {
				return "", err
			}
			raw, err := os.ReadFile(layout.OutputFile)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return m.resultStatusText(layout, opts.Name, "payload_consistency: missing_file_backed_payload_recoverable\nmissing_payload_path: output.md\n")
				}
				return "", fmt.Errorf("read output: %w", err)
			}
			text := string(raw)
			if agent.Ephemeral {
				if err := m.Erase(opts.Root, opts.Name); err != nil {
					return "", err
				}
				if opts.OnEphemeralErased != nil {
					opts.OnEphemeralErased(agent)
				}
			}
			return text, nil
		case CallStatusFailed, CallStatusCancelled:
			return m.resultStatusText(layout, opts.Name, "")
		}
		if opts.Timeout <= 0 {
			return m.resultStatusText(layout, opts.Name, "result_ready: false\n")
		}
		if !time.Now().Before(deadline) {
			_ = appendRuntimeLog(layout, m.now(), "result.timeout", map[string]any{
				"timeout_seconds": opts.Timeout.Seconds(),
				"call_status":     call.Status,
				"pid":             call.PID,
			})
			return m.resultStatusText(layout, opts.Name, "result_timeout: true\n")
		}
		if opts.Context != nil {
			timer := time.NewTimer(opts.Poll)
			select {
			case <-opts.Context.Done():
				timer.Stop()
				return m.resultStatusText(layout, opts.Name, "result_cancelled: true\n")
			case <-timer.C:
			}
		} else {
			time.Sleep(opts.Poll)
		}
	}
}

func (m Manager) resultStatusText(layout Layout, name, prefix string) (string, error) {
	status, err := m.statusScoped(layout.Root, name)
	if err != nil {
		return "", err
	}
	return prefix +
		"result_available: false\n" +
		"follow_up: agents.result --timeout 10m | agents.status | agents.tail | agents.cancel\n" +
		status, nil
}

func (m Manager) Wait(opts WaitOptions) (string, error) {
	if strings.TrimSpace(opts.Root) == "" {
		opts.Root = "."
	}
	if opts.Poll <= 0 {
		opts.Poll = 200 * time.Millisecond
	}
	if opts.Timeout <= 0 {
		opts.Timeout = defaultAgentWaitTimeout
	}
	names := append([]string(nil), opts.Names...)
	if strings.TrimSpace(opts.Name) != "" {
		names = append([]string{opts.Name}, names...)
	}
	if len(names) == 0 {
		return "", errors.New("agent name is required")
	}
	layouts := make(map[string]Layout, len(names))
	for _, name := range names {
		layout, err := m.scopedLayout(opts.Root, name, false)
		if err != nil {
			return "", err
		}
		layouts[name] = layout
	}
	deadline := time.Now().Add(opts.Timeout)
	for {
		if opts.Context != nil {
			select {
			case <-opts.Context.Done():
				return m.readinessText(opts.Root, names, layouts, "wait_cancelled: true\n")
			default:
			}
		}
		anyReady := false
		for _, name := range names {
			call, err := m.reconcileActiveCall(layouts[name])
			if errors.Is(err, os.ErrNotExist) {
				anyReady = true
				continue
			}
			if err != nil {
				return "", err
			}
			if !isActiveCallStatus(call.Status) {
				anyReady = true
			}
		}
		if anyReady {
			return m.readinessText(opts.Root, names, layouts, "")
		}
		if !time.Now().Before(deadline) {
			for _, name := range names {
				call, err := readCurrentCall(layouts[name].CurrentStateFile)
				if err == nil {
					_ = appendRuntimeLog(layouts[name], m.now(), "wait.timeout", map[string]any{
						"timeout_seconds": opts.Timeout.Seconds(),
						"call_status":     call.Status,
						"pid":             call.PID,
					})
				}
			}
			return m.readinessText(opts.Root, names, layouts, "wait_timeout: true\n")
		}
		if opts.Context != nil {
			timer := time.NewTimer(opts.Poll)
			select {
			case <-opts.Context.Done():
				timer.Stop()
				return m.readinessText(opts.Root, names, layouts, "wait_cancelled: true\n")
			case <-timer.C:
			}
		} else {
			time.Sleep(opts.Poll)
		}
	}
}

func (m Manager) readinessText(root string, names []string, layouts map[string]Layout, prefix string) (string, error) {
	var b strings.Builder
	b.WriteString(prefix)
	for i, name := range names {
		if i > 0 {
			b.WriteByte('\n')
		}
		text, err := m.readinessBlock(name, layouts[name])
		if err != nil {
			return "", err
		}
		b.WriteString(text)
	}
	return b.String(), nil
}

func (m Manager) readinessBlock(name string, layout Layout) (string, error) {
	agent, err := m.readAgentMetadata(layout, name)
	if err != nil {
		return "", err
	}
	call, err := readCurrentCall(layout.CurrentStateFile)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Sprintf("agent: %s\ncall_status: none\nready: false\nterminal: false\nresult_available: false\nactive: false\nfollow_up: agents.status | agents.tail | agents.cancel\n", agent.Name), nil
	}
	if err != nil {
		return "", err
	}
	resultAvailable := call.Status == CallStatusCompleted
	var b strings.Builder
	fmt.Fprintf(&b, "agent: %s\n", agent.Name)
	fmt.Fprintf(&b, "call_status: %s\n", call.Status)
	fmt.Fprintf(&b, "ready: %t\n", !isActiveCallStatus(call.Status))
	fmt.Fprintf(&b, "terminal: %t\n", !isActiveCallStatus(call.Status))
	fmt.Fprintf(&b, "result_available: %t\n", resultAvailable)
	fmt.Fprintf(&b, "active: %t\n", isActiveCallStatus(call.Status))
	if call.PID != 0 {
		fmt.Fprintf(&b, "pid: %d\n", call.PID)
	}
	if call.Error != "" {
		fmt.Fprintf(&b, "error: %s\n", call.Error)
	}
	fmt.Fprintf(&b, "follow_up: %s\n", followUpForCall(call))
	return b.String(), nil
}

func (m Manager) Status(root, name string) (string, error) {
	return m.statusScoped(root, name)
}

func (m Manager) statusScoped(root, name string) (string, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	layout, err := m.scopedLayout(root, name, false)
	if err != nil {
		return "", err
	}
	if _, err := m.reconcileActiveCall(layout); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	agent, err := m.readAgentMetadata(layout, name)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "agent: %s\n", agent.Name)
	fmt.Fprintf(&b, "agent_status: %s\n", agent.Status)
	fmt.Fprintf(&b, "backend: %s\n", agent.Backend)
	if agent.Harness != "" {
		fmt.Fprintf(&b, "harness: %s\n", agent.Harness)
	}
	fmt.Fprintf(&b, "tier: %s\n", agent.Tier)
	if agent.Model != "" {
		fmt.Fprintf(&b, "model: %s\n", agent.Model)
	}
	if agent.Effort != "" {
		fmt.Fprintf(&b, "effort: %s\n", agent.Effort)
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
	if call.Status == CallStatusCancelled && !call.CleanupNeeded {
		fmt.Fprintf(&b, "cancel_recovery_tip: %s\n", cancelRecoveryTip)
	}
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

func (m Manager) Inspect(root, name string) (Agent, bool, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	layout, err := m.layout(root, name, false)
	if err != nil {
		return Agent{}, false, err
	}
	if _, err := m.reconcileActiveCall(layout); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Agent{}, false, err
	}
	agent, err := m.readAgentMetadata(layout, name)
	if err != nil {
		return Agent{}, false, err
	}
	call, err := readCurrentCall(layout.CurrentStateFile)
	if errors.Is(err, os.ErrNotExist) {
		return agent, false, nil
	}
	if err != nil {
		return Agent{}, false, err
	}
	return agent, isActiveCallStatus(call.Status), nil
}

func (m Manager) Tail(opts TailOptions) (string, error) {
	if strings.TrimSpace(opts.Root) == "" {
		opts.Root = "."
	}
	if opts.Lines <= 0 {
		opts.Lines = 40
	}
	layout, err := m.scopedLayout(opts.Root, opts.Name, false)
	if err != nil {
		return "", err
	}
	if _, err := m.readAgentMetadata(layout, opts.Name); err != nil {
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
		if !opts.Raw {
			lines = sanitizeTailLines(lines)
		}
		b.WriteString(strings.Join(lines, "\n"))
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func sanitizeTailLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = sanitizeTailLine(line)
	}
	return out
}

func sanitizeTailLine(line string) string {
	var payload any
	if err := json.Unmarshal([]byte(line), &payload); err == nil {
		payload = sanitizeTailJSONValue(payload, "")
		if raw, err := json.Marshal(payload); err == nil {
			line = string(raw)
		}
	}
	return truncateTailString(line, tailMaxLineRunes, "line")
}

func sanitizeTailJSONValue(value any, key string) any {
	switch typed := value.(type) {
	case map[string]any:
		for k, v := range typed {
			typed[k] = sanitizeTailJSONValue(v, k)
		}
		return typed
	case []any:
		for i, v := range typed {
			typed[i] = sanitizeTailJSONValue(v, key)
		}
		return typed
	case string:
		if _, ok := tailLargeFieldKeys[key]; ok {
			return truncateTailString(typed, tailMaxFieldRunes, "field "+key)
		}
		return typed
	default:
		return typed
	}
}

func truncateTailString(value string, limit int, label string) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	omitted := len(runes) - limit
	return string(runes[:limit]) + fmt.Sprintf("\n[ws-tail truncated %s: omitted %d chars]", label, omitted)
}

func backendInvocationError(agent Agent, err error) error {
	if err == nil {
		return nil
	}
	var b strings.Builder
	b.WriteString("backend invocation failed\n")
	fmt.Fprintf(&b, "agent: %s\n", agent.Name)
	if agent.Tier != "" {
		fmt.Fprintf(&b, "tier: %s\n", agent.Tier)
	}
	if agent.Harness != "" {
		fmt.Fprintf(&b, "harness: %s\n", agent.Harness)
	}
	if agent.Backend != "" {
		fmt.Fprintf(&b, "backend: %s\n", agent.Backend)
	}
	if agent.Model != "" {
		fmt.Fprintf(&b, "model: %s\n", agent.Model)
	}
	b.WriteString("\nraw_error:\n")
	b.WriteString(truncateBackendError(err.Error()))
	b.WriteString("\n\nhint:\n")
	b.WriteString("If the configured backend is unavailable on this machine, fix that backend and retry, or switch explicitly.\n")
	b.WriteString("PATH-detected backend binaries:\n")
	for _, backend := range []string{"codex", "claude"} {
		if path, lookErr := exec.LookPath(backend); lookErr == nil {
			fmt.Fprintf(&b, "- %s: %s\n", backend, path)
		} else {
			fmt.Fprintf(&b, "- %s: not found\n", backend)
		}
	}
	b.WriteString("Existing agents keep stored backend/model; re-run agents.register with backend/model to switch an existing agent.\n")
	b.WriteString("Future registrations can change tier defaults with config.agents_tier.\n")
	return errors.New(b.String())
}

func truncateBackendError(value string) string {
	runes := []rune(value)
	if len(runes) <= backendErrorRunes {
		return value
	}
	omitted := len(runes) - backendErrorRunes
	return string(runes[:backendErrorRunes]) + fmt.Sprintf("\n[ws-backend-error truncated: omitted %d chars]", omitted)
}

func (m Manager) DiagnosticStream(opts DiagnosticStreamOptions) (string, error) {
	if strings.TrimSpace(opts.Root) == "" {
		opts.Root = "."
	}
	if opts.Lines <= 0 {
		opts.Lines = 40
	}
	layout, err := m.scopedLayout(opts.Root, opts.Name, false)
	if err != nil {
		return "", err
	}
	if _, err := m.readAgentMetadata(layout, opts.Name); err != nil {
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
	return m.cancelScoped(root, name)
}

func (m Manager) cancelScoped(root, name string) (string, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	layout, err := m.scopedLayout(root, name, false)
	if err != nil {
		return "", err
	}
	agent, err := m.readAgentMetadata(layout, name)
	if err != nil {
		return "", err
	}
	call, err := readCurrentCall(layout.CurrentStateFile)
	if err != nil {
		return "", err
	}
	if !isActiveCallStatus(call.Status) {
		return m.statusScoped(root, name)
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
	if err := m.writeAgentMetadata(layout, agent); err != nil {
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
	return m.statusScoped(root, name)
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

func (m Manager) acquireCurrentCallLock(layout Layout) (func(), error) {
	if err := os.MkdirAll(layout.CurrentDir, 0o755); err != nil {
		return nil, fmt.Errorf("create current call dir: %w", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		file, err := os.OpenFile(layout.CurrentLockFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			payload := map[string]any{
				"schema_version": schemaVersion,
				"pid":            os.Getpid(),
				"started_at":     m.now().UTC().Format(time.RFC3339),
			}
			raw, _ := json.Marshal(payload)
			_, _ = file.Write(append(raw, '\n'))
			_ = file.Close()
			return func() { _ = os.Remove(layout.CurrentLockFile) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("create current call lock: %w", err)
		}
		stale, staleErr := m.currentCallLockStale(layout.CurrentLockFile)
		if staleErr != nil {
			return nil, staleErr
		}
		if !stale {
			return nil, fmt.Errorf("agent current-call setup is already in progress")
		}
		_ = os.Remove(layout.CurrentLockFile)
	}
	return nil, fmt.Errorf("agent current-call setup is already in progress")
}

func (m Manager) currentCallLockStale(path string) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, fmt.Errorf("read current call lock: %w", err)
	}
	var payload struct {
		PID int `json:"pid"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.PID <= 0 {
		return false, fmt.Errorf("current call lock exists but cannot be checked safely")
	}
	alive, err := m.processAlive(payload.PID)
	if err != nil {
		return false, fmt.Errorf("check current call lock owner: %w", err)
	}
	return !alive, nil
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
	agent, err := m.readAgentMetadata(layout, layout.Name)
	if err != nil {
		return agent, err
	}
	agent.Status = StatusFailed
	agent.LastSeenAt = m.now().UTC().Format(time.RFC3339)
	if err := m.writeAgentMetadata(layout, agent); err != nil {
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

func (m Manager) enqueueInboxMessage(layout Layout, kind, text string) (Message, error) {
	if err := os.MkdirAll(layout.InboxDir, 0o755); err != nil {
		return Message{}, fmt.Errorf("create inbox dir: %w", err)
	}
	now := m.now().UTC().Format(time.RFC3339)
	for seq := nextInboxSeq(layout.InboxDir); seq < 1000000; seq++ {
		id := fmt.Sprintf("%06d", seq)
		path := filepath.Join(layout.InboxDir, id+".json")
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return Message{}, fmt.Errorf("create inbox message: %w", err)
		}
		msg := Message{
			SchemaVersion: schemaVersion,
			ID:            id,
			Kind:          kind,
			CreatedAt:     now,
			Status:        "pending",
			Text:          text,
		}
		raw, err := json.MarshalIndent(msg, "", "  ")
		if err != nil {
			_ = file.Close()
			_ = os.Remove(path)
			return Message{}, fmt.Errorf("marshal inbox message: %w", err)
		}
		if _, err := file.Write(append(raw, '\n')); err != nil {
			_ = file.Close()
			_ = os.Remove(path)
			return Message{}, fmt.Errorf("write inbox message: %w", err)
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(path)
			return Message{}, fmt.Errorf("close inbox message: %w", err)
		}
		return msg, nil
	}
	return Message{}, fmt.Errorf("no inbox message id available")
}

func nextInboxSeq(dir string) int {
	matches, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	maxSeq := 0
	for _, path := range matches {
		base := strings.TrimSuffix(filepath.Base(path), ".json")
		var seq int
		if _, err := fmt.Sscanf(base, "%d", &seq); err == nil && seq > maxSeq {
			maxSeq = seq
		}
	}
	return maxSeq + 1
}

func (m Manager) deliverPendingInbox(layout Layout, route string) ([]Message, error) {
	unlock, err := m.acquireInboxDeliveryLock(layout)
	if err != nil {
		return nil, err
	}
	defer unlock()

	matches, err := filepath.Glob(filepath.Join(layout.InboxDir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("list inbox: %w", err)
	}
	sort.Strings(matches)
	var messages []Message
	for _, path := range matches {
		msg, err := readMessage(path)
		if err != nil {
			return nil, err
		}
		if msg.Status != "pending" {
			continue
		}
		msg.Status = "delivered"
		if err := writeMessage(path, msg); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	if len(messages) > 0 {
		fields := map[string]any{
			"count":       len(messages),
			"message_ids": messageIDs(messages),
		}
		_ = appendRuntimeLog(layout, m.now(), "inbox.delivered_via_"+route, fields)
		_ = appendEvent(layout.EventsFile, m.now(), "inbox.delivered_via_"+route, fields)
	}
	return messages, nil
}

func (m Manager) DeliverPendingInbox(root, name, route string) ([]Message, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	if route == "" {
		route = "manual"
	}
	layout, err := m.scopedLayout(root, name, false)
	if err != nil {
		return nil, err
	}
	return m.deliverPendingInbox(layout, route)
}

func (m Manager) acquireInboxDeliveryLock(layout Layout) (func(), error) {
	if err := os.MkdirAll(layout.InboxDir, 0o755); err != nil {
		return nil, fmt.Errorf("create inbox dir: %w", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		file, err := os.OpenFile(layout.InboxLockFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			payload := map[string]any{
				"schema_version": schemaVersion,
				"pid":            os.Getpid(),
				"started_at":     m.now().UTC().Format(time.RFC3339),
			}
			raw, _ := json.Marshal(payload)
			_, _ = file.Write(append(raw, '\n'))
			_ = file.Close()
			return func() { _ = os.Remove(layout.InboxLockFile) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("create inbox delivery lock: %w", err)
		}
		stale, staleErr := m.currentCallLockStale(layout.InboxLockFile)
		if staleErr != nil {
			return nil, staleErr
		}
		if !stale {
			return nil, fmt.Errorf("agent inbox delivery is already in progress")
		}
		_ = os.Remove(layout.InboxLockFile)
	}
	return nil, fmt.Errorf("agent inbox delivery is already in progress")
}

func messageIDs(messages []Message) []string {
	ids := make([]string, 0, len(messages))
	for _, msg := range messages {
		ids = append(ids, msg.ID)
	}
	return ids
}

func ComposeLeadMessageFeedback(messages []Message) string {
	if len(messages) == 0 {
		return ""
	}
	return composeLeadMessagePrompt(messages, "")
}

func composeLeadMessagePrompt(messages []Message, prompt string) string {
	var b strings.Builder
	b.WriteString("Lead messages queued for this agent:\n\n")
	for _, msg := range messages {
		fmt.Fprintf(&b, "[%s %s]\n%s\n\n", msg.Kind, msg.ID, strings.TrimSpace(msg.Text))
	}
	b.WriteString("Apply these lead messages immediately. If an interrupt redirects or narrows the task, stop the previous line of work and continue from the latest lead instruction.")
	if strings.TrimSpace(prompt) != "" {
		b.WriteString("\n\n---\n\nOriginal prompt:\n")
		b.WriteString(prompt)
	}
	return b.String()
}

func (m Manager) Erase(root, name string) error {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	if err := m.deleteAgentMetadata(root, name); err != nil {
		return err
	}
	return nil
}

func (m Manager) layout(root, name string, create bool) (Layout, error) {
	return m.scopedLayout(root, name, create)
}

func (m Manager) scopedLayout(root, name string, create bool) (Layout, error) {
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
	internalKey, err := m.registryKey(name)
	if err != nil {
		return Layout{}, err
	}
	dirKey := key
	hasCurrentRole := false
	if store, err := m.registryStore(root); err == nil {
		if def, ok, defErr := store.AgentDefinition(context.Background(), internalKey); defErr == nil && ok && strings.TrimSpace(def.StatePath) != "" {
			hasCurrentRole = true
			if !create {
				dirKey = def.StatePath
			}
		}
		_ = store.Close()
	}
	if create && hasCurrentRole {
		stamp := m.now().UTC().Format("20060102T150405.000000000Z")
		dirKey = key + "-" + stamp
		for i := 0; pathExists(filepath.Join(state.AgentsDir, dirKey)) && i < 1000; i++ {
			dirKey = fmt.Sprintf("%s-%03d", dirKey, i+1)
		}
	}
	dir := filepath.Join(state.AgentsDir, dirKey)
	layout := Layout{
		Root:              root,
		Name:              name,
		AgentDir:          dir,
		AgentFile:         filepath.Join(dir, "agent.json"),
		InboxDir:          filepath.Join(dir, "inbox"),
		InboxLockFile:     filepath.Join(dir, "inbox", "delivery.lock"),
		OutboxDir:         filepath.Join(dir, "outbox"),
		CurrentDir:        filepath.Join(dir, "current"),
		CurrentLockFile:   filepath.Join(dir, "current", "setup.lock"),
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

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
		return "agents.wait --timeout 10m | agents.status | agents.tail | agents.cancel"
	case CallStatusCompleted:
		return "agents.result | agents.tail"
	case CallStatusFailed:
		return "agents.tail | agents.erase"
	case CallStatusCancelled:
		if call.CleanupNeeded {
			return "inspect runtime log | manual cleanup | agents.erase"
		}
		return "agents.call | agents.tail | agents.erase"
	default:
		return "agents.status"
	}
}

func interruptHookCommand(root, name string) string {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		exe = "ws-mcp"
	}
	cmd := shellQuote(exe) + " agents check-inbox --root " + shellQuote(root) + " --name " + shellQuote(name)
	return cmd
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
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
	if err := replaceFile(tmp, path); err != nil {
		return fmt.Errorf("replace agent: %w", err)
	}
	return nil
}

func readMessage(path string) (Message, error) {
	var msg Message
	raw, err := os.ReadFile(path)
	if err != nil {
		return msg, fmt.Errorf("read inbox message: %w", err)
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return msg, fmt.Errorf("parse inbox message: %w", err)
	}
	return msg, nil
}

func writeMessage(path string, msg Message) error {
	raw, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal inbox message: %w", err)
	}
	tmp := uniqueTempPath(path)
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("write inbox message: %w", err)
	}
	if err := replaceFile(tmp, path); err != nil {
		return fmt.Errorf("replace inbox message: %w", err)
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
	if err := replaceFile(tmp, path); err != nil {
		return fmt.Errorf("replace current call: %w", err)
	}
	return nil
}

func replaceFile(tmp, path string) error {
	if err := os.Rename(tmp, path); err == nil {
		return nil
	} else if _, statErr := os.Stat(path); statErr != nil {
		return err
	}
	if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return removeErr
	}
	return os.Rename(tmp, path)
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
