package wsagent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// ClaudeBackend runs Claude Code headless: print mode with stream-json in
// and out and stdin held open, one process per conversation. It keeps
// Claude Code's own system prompt and appends the composed instruction file
// (--append-system-prompt-file); it never passes --bare (which drops the
// subscription login) or --system-prompt.
type ClaudeBackend struct {
	// Command is the claude executable; empty looks up "claude" on PATH.
	Command string
}

// claudeDisabledPlugins are Claude Code's built-in plugins that settings can
// turn off; the remaining built-ins (telemetry, privacy) cannot be.
var claudeDisabledPlugins = []string{"cc-plugin-agents-md@builtin", "cc-plugin-plugin-authoring@builtin"}

// claudeKeptEnv are CLAUDE* variables a child keeps: the user's config
// directory and credentials. Every other CLAUDE* variable belongs to the
// parent harness session that started the daemon.
var claudeKeptEnv = map[string]bool{
	"CLAUDE_CONFIG_DIR":       true,
	"CLAUDE_CODE_OAUTH_TOKEN": true,
	"CLAUDE_CODE_USE_BEDROCK": true,
	"CLAUDE_CODE_USE_VERTEX":  true,
}

// claudeArgs is the claude command line for spec, without the executable.
func claudeArgs(spec StartSpec) ([]string, error) {
	p := spec.Profile
	mcpConfig, err := claudeMCPConfig(spec)
	if err != nil {
		return nil, err
	}
	disabled := map[string]bool{}
	for _, id := range claudeDisabledPlugins {
		disabled[id] = false
	}
	settings, _ := json.Marshal(map[string]any{"enabledPlugins": disabled})
	var bases []string
	seen := map[string]bool{}
	for _, entry := range p.Tools {
		if base := ToolBase(entry); !seen[base] {
			seen[base] = true
			bases = append(bases, base)
		}
	}
	args := []string{
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		// No settings file: user, project, and local settings (their
		// permission rules, hooks, plugins, and memory files) stay out.
		"--setting-sources", "",
		"--settings", string(settings),
		"--strict-mcp-config", "--mcp-config", mcpConfig,
		"--tools", strings.Join(bases, ","),
		"--permission-mode", "dontAsk",
	}
	if !p.Skills {
		args = append(args, "--disable-slash-commands")
	}
	for _, dir := range p.PluginPaths(spec.Root) {
		args = append(args, "--plugin-dir", dir)
	}
	if spec.Instructions != "" {
		args = append(args, "--append-system-prompt-file", spec.Instructions)
	}
	if spec.Model != "" {
		args = append(args, "--model", spec.Model)
	}
	if spec.Effort != "" {
		args = append(args, "--effort", spec.Effort)
	}
	if spec.SessionID != "" {
		args = append(args, "--resume", spec.SessionID)
	}
	// The allowlist goes last: the option takes every following argument.
	if allowed := claudeAllowedRules(spec); len(allowed) > 0 {
		args = append(args, "--allowedTools")
		args = append(args, allowed...)
	}
	return args, nil
}

// claudeAllowedRules are the permission rules the child runs without a
// prompt: the profile's tool entries, its ws tools, and every tool of its
// own MCP servers. dontAsk denies everything else.
func claudeAllowedRules(spec StartSpec) []string {
	rules := append([]string{}, spec.Profile.Tools...)
	if spec.WSMCP != nil {
		for _, name := range spec.Profile.WSTools {
			rules = append(rules, claudeMCPToolName(ReservedMCPServer, name))
		}
	}
	for _, name := range spec.Profile.MCPServerNames() {
		rules = append(rules, "mcp__"+claudeNamePart(name))
	}
	return rules
}

func claudeMCPConfig(spec StartSpec) (string, error) {
	servers := map[string]any{}
	for name, raw := range spec.Profile.MCPServers {
		servers[name] = raw
	}
	if spec.WSMCP != nil {
		servers[ReservedMCPServer] = map[string]any{"type": "stdio", "command": spec.WSMCP.Command, "args": spec.WSMCP.Args, "env": spec.WSMCP.Env}
	}
	raw, err := json.Marshal(map[string]any{"mcpServers": servers})
	return string(raw), err
}

var claudeNameUnsafe = regexp.MustCompile(`[^A-Za-z0-9_-]`)

func claudeNamePart(s string) string { return claudeNameUnsafe.ReplaceAllString(s, "_") }

// claudeMCPToolName is Claude Code's name for an MCP server's tool:
// mcp__<server>__<tool>, with characters outside [A-Za-z0-9_-] (a ws tool
// name's dots) as underscores.
func claudeMCPToolName(server, tool string) string {
	return "mcp__" + claudeNamePart(server) + "__" + claudeNamePart(tool)
}

// claudeInit is the part of the system/init message the init check reads.
type claudeInit struct {
	SessionID      string   `json:"session_id"`
	Tools          []string `json:"tools"`
	PermissionMode string   `json:"permissionMode"`
	MCPServers     []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"mcp_servers"`
	Plugins []struct {
		Name   string `json:"name"`
		Path   string `json:"path"`
		Source string `json:"source"`
	} `json:"plugins"`
}

// checkClaudeInit verifies that the child sees only what spec permits: its
// tools are the profile's built-in tools, ws tools, and own servers' tools;
// its MCP servers are ws (when attached) and the profile's; every plugin
// is a built-in or one of the profile's plugin directories; and permissions
// run in dontAsk mode.
func checkClaudeInit(init claudeInit, spec StartSpec) error {
	allowed := map[string]bool{}
	for _, entry := range spec.Profile.Tools {
		allowed[ToolBase(entry)] = true
	}
	servers := map[string]bool{}
	var prefixes []string
	if spec.WSMCP != nil {
		servers[ReservedMCPServer] = true
		for _, name := range spec.Profile.WSTools {
			allowed[claudeMCPToolName(ReservedMCPServer, name)] = true
		}
	}
	for _, name := range spec.Profile.MCPServerNames() {
		servers[name] = true
		prefixes = append(prefixes, "mcp__"+claudeNamePart(name)+"__")
	}
	var extra []string
	for _, tool := range init.Tools {
		ok := allowed[tool]
		for _, prefix := range prefixes {
			ok = ok || strings.HasPrefix(tool, prefix)
		}
		if !ok {
			extra = append(extra, tool)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		return fmt.Errorf("launch refused: claude reports tools outside the profile: %s", strings.Join(extra, ", "))
	}
	for _, s := range init.MCPServers {
		if !servers[s.Name] {
			return fmt.Errorf("launch refused: claude reports MCP server %q outside the profile", s.Name)
		}
	}
	dirs := map[string]bool{}
	for _, dir := range spec.Profile.PluginPaths(spec.Root) {
		dirs[filepath.Clean(dir)] = true
	}
	for _, pl := range init.Plugins {
		if strings.HasSuffix(pl.Source, "@builtin") || dirs[filepath.Clean(pl.Path)] {
			continue
		}
		return fmt.Errorf("launch refused: claude loaded plugin %q (%s) outside the profile", pl.Name, pl.Path)
	}
	if init.PermissionMode != "dontAsk" {
		return fmt.Errorf("launch refused: claude runs in permission mode %q, not dontAsk", init.PermissionMode)
	}
	return nil
}

// Start launches the claude process for spec.
func (b ClaudeBackend) Start(ctx context.Context, spec StartSpec) (Process, error) {
	command := b.Command
	if command == "" {
		path, err := lookPath("claude")
		if err != nil {
			return nil, fmt.Errorf("claude backend: %w", err)
		}
		command = path
	}
	args, err := claudeArgs(spec)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(command, args...)
	cmd.Dir = spec.Root
	cmd.Env = harnessEnv(spec.Env, []string{"CLAUDE"}, claudeKeptEnv, nil)
	return startStreamProcess(cmd, newClaudeMapper(spec), claudeControl{})
}

// claudeControl writes Claude Code's stream-json input lines.
type claudeControl struct{}

func (claudeControl) userLine(text string) any {
	return map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": text}}
}

func (claudeControl) controlLine(id string, request map[string]any) any {
	return map[string]any{"type": "control_request", "request_id": id, "request": request}
}

// streamProcess is a child speaking line-delimited JSON on stdin and
// stdout, parsed by a lineMapper.
type streamProcess struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	mapper  lineMapper
	control claudeControl
	events  chan Event
	closing chan struct{}
	exited  chan struct{}
	read    chan struct{}

	writeMu sync.Mutex
	ctlMu   sync.Mutex
	ctlSeq  int
	waiters map[string]chan controlAnswer
	once    sync.Once
}

// lineMapper turns one output line into events; a control response is
// answered through answer.
type lineMapper interface {
	mapLine(line []byte, answer func(id string, a controlAnswer)) []Event
}

type controlAnswer struct {
	ok  bool
	err string
}

func startStreamProcess(cmd *exec.Cmd, mapper lineMapper, control claudeControl) (*streamProcess, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr := &tailBuffer{max: 4 << 10}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", filepath.Base(cmd.Path), err)
	}
	p := &streamProcess{
		cmd: cmd, stdin: stdin, mapper: mapper, control: control,
		events:  make(chan Event, 256),
		closing: make(chan struct{}),
		exited:  make(chan struct{}),
		read:    make(chan struct{}),
		waiters: map[string]chan controlAnswer{},
	}
	go func() {
		defer close(p.read)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64<<10), 64<<20)
		for sc.Scan() {
			for _, ev := range mapper.mapLine(sc.Bytes(), p.answer) {
				if ev.Fatal != nil {
					// A refused launch never gets to run a tool call.
					_ = cmd.Process.Kill()
				}
				select {
				case p.events <- ev:
				case <-p.closing:
				}
			}
		}
	}()
	go func() {
		<-p.read
		err := cmd.Wait()
		close(p.exited)
		select {
		case <-p.closing:
		default:
			if err != nil {
				msg := strings.TrimSpace(stderr.String())
				if msg == "" {
					msg = err.Error()
				}
				select {
				case p.events <- Event{Fatal: fmt.Errorf("%s exited: %s", filepath.Base(cmd.Path), msg)}:
				case <-p.closing:
				}
			}
		}
		close(p.events)
	}()
	return p, nil
}

func (p *streamProcess) Events() <-chan Event { return p.events }

func (p *streamProcess) writeLine(v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	_, err = p.stdin.Write(append(raw, '\n'))
	return err
}

func (p *streamProcess) Send(text string) error { return p.writeLine(p.control.userLine(text)) }

// controlTimeout bounds the wait for a control response.
var controlTimeout = 10 * time.Second

// request sends a control request and waits for its response.
func (p *streamProcess) request(req map[string]any) error {
	p.ctlMu.Lock()
	p.ctlSeq++
	id := "ws-" + strconv.Itoa(p.ctlSeq)
	ch := make(chan controlAnswer, 1)
	p.waiters[id] = ch
	p.ctlMu.Unlock()
	defer func() {
		p.ctlMu.Lock()
		delete(p.waiters, id)
		p.ctlMu.Unlock()
	}()
	if err := p.writeLine(p.control.controlLine(id, req)); err != nil {
		return err
	}
	select {
	case a := <-ch:
		if !a.ok {
			if strings.Contains(strings.ToLower(a.err), "unsupported") {
				return fmt.Errorf("%w: %s", ErrUnsupported, a.err)
			}
			return errors.New(a.err)
		}
		return nil
	case <-p.exited:
		return errors.New("the process exited")
	case <-time.After(controlTimeout):
		return fmt.Errorf("no answer to %v within %s", req["subtype"], controlTimeout)
	}
}

func (p *streamProcess) answer(id string, a controlAnswer) {
	p.ctlMu.Lock()
	ch := p.waiters[id]
	p.ctlMu.Unlock()
	if ch != nil {
		ch <- a
	}
}

func (p *streamProcess) Interrupt() error {
	return p.request(map[string]any{"subtype": "interrupt"})
}

// SetModel uses Claude Code's set_model control, which applies from the
// next turn. Returning to the default model has no in-stream form.
func (p *streamProcess) SetModel(model string) error {
	if model == "" {
		return ErrUnsupported
	}
	return p.request(map[string]any{"subtype": "set_model", "model": model})
}

// SetEffort has no in-stream form in Claude Code (2.1.319 answers
// set_effort with "Unsupported control request subtype"), so an effort
// change restarts the process with --effort on the same session.
func (p *streamProcess) SetEffort(string) error { return ErrUnsupported }

func (p *streamProcess) Close() error {
	p.once.Do(func() {
		close(p.closing)
		stopChild(p.cmd, p.stdin.Close, p.exited)
	})
	for range p.events {
	}
	return nil
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (t *tailBuffer) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, b...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(b), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

// claudeMapper maps Claude Code's stream-json output to runner events: the
// init message to the session id (after the init check), assistant text to
// final replies, tool calls to the lead's batch (one per assistant
// message), rate-limit and result figures to usage, and the result to the
// turn's end.
type claudeMapper struct {
	spec  StartSpec
	now   func() time.Time
	epoch string
	seq   int64

	batch   *claudeBatch
	context *wslive.UsageContext
	limits  map[string]wslive.UsageLimit
	cost    *wslive.UsageCost
	window  int64
}

type claudeBatch struct {
	messageID string
	seq       int64
	startedAt int64
	calls     []*claudeCall
}

type claudeCall struct {
	id        string
	name      string
	args      string
	startedAt int64
	endedAt   int64
	failed    bool
}

func newClaudeMapper(spec StartSpec) *claudeMapper {
	return &claudeMapper{spec: spec, now: time.Now, epoch: "c" + strconv.FormatInt(time.Now().UnixMilli(), 36), limits: map[string]wslive.UsageLimit{}}
}

type claudeLine struct {
	Type     string          `json:"type"`
	Subtype  string          `json:"subtype"`
	Message  json.RawMessage `json:"message"`
	Response struct {
		Subtype   string `json:"subtype"`
		RequestID string `json:"request_id"`
		Error     string `json:"error"`
	} `json:"response"`
	RateLimit *struct {
		RateLimitType  string `json:"rateLimitType"`
		ResetsAt       int64  `json:"resetsAt"`
		UnifiedWindows map[string]struct {
			Utilization *float64 `json:"utilization"`
			ResetsAt    int64    `json:"resetsAt"`
		} `json:"unifiedWindows"`
	} `json:"rate_limit_info"`
	IsError       bool     `json:"is_error"`
	Result        string   `json:"result"`
	Errors        []string `json:"errors"`
	TerminalState string   `json:"terminal_reason"`
	TotalCostUSD  *float64 `json:"total_cost_usd"`
	ModelUsage    map[string]struct {
		ContextWindow int64 `json:"contextWindow"`
	} `json:"modelUsage"`
	CompactMetadata *struct {
		Trigger string `json:"trigger"`
	} `json:"compact_metadata"`
}

type claudeMessage struct {
	ID      string          `json:"id"`
	Content json.RawMessage `json:"content"`
	Usage   *struct {
		InputTokens              int64 `json:"input_tokens"`
		CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		OutputTokens             int64 `json:"output_tokens"`
	} `json:"usage"`
}

type claudeBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
}

func (m *claudeMapper) mapLine(line []byte, answer func(string, controlAnswer)) []Event {
	var l claudeLine
	if json.Unmarshal(line, &l) != nil {
		return nil
	}
	now := m.now().UnixMilli()
	switch l.Type {
	case "control_response":
		answer(l.Response.RequestID, controlAnswer{ok: l.Response.Subtype == "success", err: l.Response.Error})
	case "system":
		switch l.Subtype {
		case "init":
			var init claudeInit
			if json.Unmarshal(line, &init) != nil {
				return []Event{{Fatal: errors.New("launch refused: unreadable claude init message")}}
			}
			if err := checkClaudeInit(init, m.spec); err != nil {
				return []Event{{Fatal: err}}
			}
			if init.SessionID != "" {
				return []Event{{SessionID: init.SessionID}}
			}
		case "compact_boundary":
			trigger := ""
			if l.CompactMetadata != nil && (l.CompactMetadata.Trigger == wslive.TriggerAuto || l.CompactMetadata.Trigger == wslive.TriggerManual) {
				trigger = l.CompactMetadata.Trigger
			}
			return []Event{{Live: &wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactEnd, Trigger: trigger}}}
		}
	case "assistant":
		return m.assistant(l.Message, now)
	case "user":
		return m.toolResults(l.Message, now)
	case "rate_limit_event":
		if l.RateLimit != nil {
			for kind, w := range l.RateLimit.UnifiedWindows {
				if (kind == wslive.LimitFiveHour || kind == wslive.LimitSevenDay) && w.Utilization != nil {
					m.limits[kind] = wslive.UsageLimit{Kind: kind, Percent: *w.Utilization * 100, ResetsAt: w.ResetsAt * 1000}
				}
			}
			return []Event{m.status()}
		}
	case "result":
		var out []Event
		if b := m.finishBatch(now, true); b != nil {
			out = append(out, *b)
		}
		for _, mu := range l.ModelUsage {
			if mu.ContextWindow > m.window {
				m.window = mu.ContextWindow
			}
		}
		if l.TotalCostUSD != nil {
			m.cost = &wslive.UsageCost{USD: *l.TotalCostUSD, Estimate: true}
		}
		out = append(out, m.status())
		end := Event{TurnEnd: true}
		// An interrupted turn ends as error_during_execution with an
		// aborted_* terminal reason (aborted_streaming, aborted_tools); that
		// is the interrupt asked for, not a failure.
		interrupted := strings.HasPrefix(l.TerminalState, "aborted")
		if (l.IsError || l.Subtype != "success") && !interrupted {
			msg := strings.TrimSpace(l.Result)
			if msg == "" {
				msg = strings.Join(l.Errors, "; ")
			}
			if msg == "" {
				msg = "turn failed: " + l.Subtype
			}
			end.TurnError = msg
		}
		return append(out, end)
	}
	return nil
}

func (m *claudeMapper) assistant(raw json.RawMessage, now int64) []Event {
	var msg claudeMessage
	if json.Unmarshal(raw, &msg) != nil {
		return nil
	}
	if msg.Usage != nil {
		tokens := msg.Usage.InputTokens + msg.Usage.CacheReadInputTokens + msg.Usage.CacheCreationInputTokens + msg.Usage.OutputTokens
		m.context = &wslive.UsageContext{Tokens: &tokens}
	}
	var blocks []claudeBlock
	if json.Unmarshal(msg.Content, &blocks) != nil {
		return nil
	}
	var out []Event
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) != "" {
				out = append(out, Event{Live: &wslive.Event{Kind: wslive.KindReply, Text: b.Text, Final: true}})
			}
		case "tool_use":
			if m.batch != nil && m.batch.messageID != msg.ID {
				if ev := m.finishBatch(now, true); ev != nil {
					out = append(out, *ev)
				}
			}
			if m.batch == nil {
				m.seq++
				m.batch = &claudeBatch{messageID: msg.ID, seq: m.seq, startedAt: now}
			}
			m.batch.calls = append(m.batch.calls, &claudeCall{id: b.ID, name: b.Name, args: argsPreview(b.Input), startedAt: now})
			out = append(out, m.batchEvent())
		}
	}
	return out
}

func (m *claudeMapper) toolResults(raw json.RawMessage, now int64) []Event {
	var msg claudeMessage
	if m.batch == nil || json.Unmarshal(raw, &msg) != nil {
		return nil
	}
	var blocks []claudeBlock
	if json.Unmarshal(msg.Content, &blocks) != nil {
		return nil
	}
	changed := false
	for _, b := range blocks {
		if b.Type != "tool_result" {
			continue
		}
		for _, c := range m.batch.calls {
			if c.id == b.ToolUseID && c.endedAt == 0 {
				c.endedAt, c.failed, changed = now, b.IsError, true
			}
		}
	}
	if !changed {
		return nil
	}
	for _, c := range m.batch.calls {
		if c.endedAt == 0 {
			return []Event{m.batchEvent()}
		}
	}
	if ev := m.finishBatch(now, false); ev != nil {
		return []Event{*ev}
	}
	return nil
}

// finishBatch ends the current batch (unfinished calls fail when
// cutShort) and returns its final event.
func (m *claudeMapper) finishBatch(now int64, cutShort bool) *Event {
	if m.batch == nil {
		return nil
	}
	for _, c := range m.batch.calls {
		if c.endedAt == 0 {
			c.endedAt, c.failed = now, cutShort
		}
	}
	ev := m.batchEvent()
	m.batch = nil
	return &ev
}

type wireBatch struct {
	Epoch     string              `json:"epoch"`
	Seq       int64               `json:"seq"`
	ToolNames []string            `json:"toolNames"`
	CallCount int                 `json:"callCount"`
	Failed    int                 `json:"failed"`
	StartedAt int64               `json:"startedAt"`
	EndedAt   *int64              `json:"endedAt,omitempty"`
	Running   *wslive.RunningView `json:"running,omitempty"`
	Calls     []wireCall          `json:"calls,omitempty"`
}

type wireCall struct {
	Name      string `json:"name"`
	StartedAt int64  `json:"startedAt"`
	EndedAt   int64  `json:"endedAt,omitempty"`
	Failed    bool   `json:"failed,omitempty"`
}

// batchEvent is the lead's batch list holding the current batch: running
// while a call is unfinished, else ended.
func (m *claudeMapper) batchEvent() Event {
	b := m.batch
	w := wireBatch{Epoch: m.epoch, Seq: b.seq, ToolNames: []string{}, CallCount: len(b.calls), StartedAt: b.startedAt}
	var running *claudeCall
	done, open := 0, 0
	var ended int64
	for _, c := range b.calls {
		if !contains(w.ToolNames, c.name) {
			w.ToolNames = append(w.ToolNames, c.name)
		}
		w.Calls = append(w.Calls, wireCall{Name: c.name, StartedAt: c.startedAt, EndedAt: c.endedAt, Failed: c.failed})
		switch {
		case c.endedAt == 0:
			open++
			if running == nil {
				running = c
			}
		default:
			done++
			if c.failed {
				w.Failed++
			}
			ended = max(ended, c.endedAt)
		}
	}
	if running != nil {
		w.Calls = nil
		w.Running = &wslive.RunningView{ToolName: running.name, ArgsPreview: running.args, StartedAt: running.startedAt, Done: done, OthersRunning: open - 1}
	} else {
		w.EndedAt = &ended
	}
	raw, _ := json.Marshal(w)
	return Event{Live: &wslive.Event{Kind: wslive.KindAgent, Batches: []json.RawMessage{raw}}}
}

func (m *claudeMapper) status() Event {
	ev := wslive.Event{Kind: wslive.KindStatus, Cost: m.cost}
	if m.context != nil {
		c := *m.context
		c.Window = m.window
		ev.Context = &c
	}
	for _, kind := range []string{wslive.LimitFiveHour, wslive.LimitSevenDay} {
		if l, ok := m.limits[kind]; ok {
			ev.Limits = append(ev.Limits, l)
		}
	}
	return Event{Live: &ev}
}

// argsPreview is a tool call's arguments as one line: the first string
// value of a small object, else the compact JSON.
func argsPreview(raw json.RawMessage) string {
	var obj map[string]any
	if json.Unmarshal(raw, &obj) == nil {
		for _, key := range []string{"command", "file_path", "path", "pattern", "url", "query", "description"} {
			if s, ok := obj[key].(string); ok && s != "" {
				return s
			}
		}
	}
	return string(raw)
}

func contains(list []string, s string) bool {
	for _, have := range list {
		if have == s {
			return true
		}
	}
	return false
}
