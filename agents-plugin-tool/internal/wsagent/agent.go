package wsagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

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

var unsafeNameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

type Clock func() time.Time

type Options struct {
	CacheHome string
	Now       Clock
	Runner    Runner
}

type RegisterOptions struct {
	Root             string
	Name             string
	Backend          string
	Tier             string
	Model            string
	PromptRefs       []string
	SystemPromptText string
}

type CallOptions struct {
	Root   string
	Name   string
	Prompt string
}

type OneShotOptions struct {
	Root             string
	Name             string
	Backend          string
	Tier             string
	Model            string
	PromptRefs       []string
	SystemPromptText string
	Prompt           string
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

type Layout struct {
	Root       string
	AgentDir   string
	AgentFile  string
	InboxDir   string
	OutboxDir  string
	OutputFile string
	EventsFile string
	SystemFile string
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
	if strings.TrimSpace(opts.Tier) == "" {
		opts.Tier = "core"
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		return Agent{}, Layout{}, errors.New("agent name is required")
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
		PromptRefs:       append([]string(nil), opts.PromptRefs...),
		SystemPromptPath: "",
		Capabilities: map[string]bool{
			"resume":      true,
			"interrupt":   false,
			"compression": false,
		},
	}
	if strings.TrimSpace(opts.SystemPromptText) != "" {
		if err := os.WriteFile(layout.SystemFile, []byte(opts.SystemPromptText), 0o644); err != nil {
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

func (m Manager) Call(opts CallOptions) (Agent, string, error) {
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

	now := m.now().UTC().Format(time.RFC3339)
	agent.Status = StatusRunning
	agent.LastSeenAt = now
	agent.LastCallAt = now
	if err := writeAgent(layout.AgentFile, agent); err != nil {
		return agent, "", err
	}
	if err := appendEvent(layout.EventsFile, m.now(), "call.started", map[string]any{
		"backend": agent.Backend,
		"resume":  agent.SessionID != "",
	}); err != nil {
		return agent, "", err
	}

	runner := m.opts.Runner
	if runner == nil {
		runner = CodexRunner{}
	}
	result, err := runner.Call(RunnerRequest{
		Root:             opts.Root,
		Prompt:           opts.Prompt,
		Model:            agent.Model,
		SessionID:        agent.SessionID,
		SystemPromptPath: absOptional(layout.AgentDir, agent.SystemPromptPath),
	})
	if err != nil {
		agent.Status = StatusFailed
		agent.LastSeenAt = m.now().UTC().Format(time.RFC3339)
		_ = writeAgent(layout.AgentFile, agent)
		_ = appendEvent(layout.EventsFile, m.now(), "call.failed", map[string]any{"error": err.Error()})
		return agent, "", err
	}
	if result.SessionID != "" {
		agent.SessionID = result.SessionID
	}
	agent.Status = StatusIdle
	agent.LastSeenAt = m.now().UTC().Format(time.RFC3339)
	if err := os.WriteFile(layout.OutputFile, []byte(result.Text), 0o644); err != nil {
		agent.Status = StatusFailed
		_ = writeAgent(layout.AgentFile, agent)
		return agent, "", fmt.Errorf("write output: %w", err)
	}
	if err := writeAgent(layout.AgentFile, agent); err != nil {
		return agent, "", err
	}
	if err := appendEvent(layout.EventsFile, m.now(), "call.completed", map[string]any{
		"session_id": agent.SessionID,
	}); err != nil {
		return agent, "", err
	}
	return agent, result.Text, nil
}

func (m Manager) OneShot(opts OneShotOptions) (string, error) {
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
		PromptRefs:       opts.PromptRefs,
		SystemPromptText: opts.SystemPromptText,
	})
	if err != nil {
		return "", err
	}
	_, text, callErr := m.Call(CallOptions{Root: opts.Root, Name: name, Prompt: opts.Prompt})
	eraseErr := m.Erase(opts.Root, name)
	if callErr != nil {
		return "", callErr
	}
	return text, eraseErr
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
		Root:       root,
		AgentDir:   dir,
		AgentFile:  filepath.Join(dir, "agent.json"),
		InboxDir:   filepath.Join(dir, "inbox"),
		OutboxDir:  filepath.Join(dir, "outbox"),
		OutputFile: filepath.Join(dir, "output.md"),
		EventsFile: filepath.Join(dir, "events.jsonl"),
		SystemFile: filepath.Join(dir, "system.md"),
	}
	if create {
		for _, path := range []string{layout.InboxDir, layout.OutboxDir} {
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
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("write agent: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace agent: %w", err)
	}
	return nil
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

func absOptional(base, rel string) string {
	if rel == "" {
		return ""
	}
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(base, rel)
}
