package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wswake"
)

// pi_tools.go is the pi.* tools: a lead session spawns Pi subagents in the
// dashboard daemon's agent runner (ServerOptions.Agents), steers them by
// id, and collects each finished turn from a completion mail the daemon
// posts to its reply-id queue (PiTurnReporter), with pi.wait as the
// fallback. The live feed of the session's agents panel is pi_live.go.
//
// A Pi subagent's own attached ws-mcp has no runner: on the capability
// token and the daemon endpoint the runner put in its environment
// (wsagent.EnvPiToken, wsagent.EnvPiEndpoint), it forwards each pi.* call
// to the runner's token route, which spawns and steers that subagent's own
// children and delivers their reports into it (pi_forward.go).

// PiAgents is the dashboard daemon's agent runner as the pi.* tools use it
// (*wsagent.Runner).
type PiAgents interface {
	Spawn(wsagent.SpawnSpec) (wsagent.State, error)
	Send(id, text string) (wsagent.State, error)
	Get(id string) (wsagent.State, error)
	Children(parent wsagent.ParentRef) []wsagent.State
	CloseConversation(id string) (wsagent.State, error)
	Wait(ctx context.Context, id string, timeout time.Duration) (wsagent.TurnResult, error)
	Watch(fn func(wsagent.State)) (stop func())
	Transcript(id string) ([]wslive.RingRecord, error)
}

// DashboardPiSpawnKey lists the pi.* tools (with a daemon handle and a Pi
// of at least piMinMajor.piMinMinor). Its manifest default differs per
// package.
const DashboardPiSpawnKey = "dashboard.pi_spawn_enabled"

// The Pi version the pi.* tools need: the first whose RPC mode the Pi
// backend speaks.
const (
	piMinMajor = 1
	piMinMinor = 0
)

// pi.wait's timeout_s: its default and its cap.
const (
	piWaitDefault = 300 * time.Second
	piWaitMax     = 1800 * time.Second
)

// piTranscriptDefault is how many of the newest transcript entries
// pi.transcript returns without a limit.
const piTranscriptDefault = 40

// piHarness is the agents.tier harness bucket pi.spawn resolves its tier
// from, alone.
const piHarness = wsagent.PiTierHarness

// piProbe runs `pi --version` (a test seam); piVersion caches its answer
// for the process: every server the daemon hosts shares one probe.
var (
	piProbe   = wsagent.PiVersion
	piVersion = new(piVersionCache)
)

type piVersionCache struct {
	once    sync.Once
	version string
	err     error
}

// installedPiVersion is the installed Pi's version, probed once per process.
func installedPiVersion() (string, error) {
	cache := piVersion
	cache.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cache.version, cache.err = piProbe(ctx)
	})
	return cache.version, cache.err
}

// piHost is the server's side of the pi.* tools.
type piHost struct {
	// agents is the daemon's runner; nil on the direct path.
	agents PiAgents
	// stopWatch ends the live feed's runner subscription (Server.Close).
	stopWatch func()

	// listed caches the tools/list gate (piListed) for the server's life:
	// a knob change applies at the next connection.
	listOnce sync.Once
	listErr  error

	// keys caches a child's ws session key by its id, for its agents-panel
	// row (pi_live.go).
	keyMu sync.Mutex
	keys  map[string]string
}

func isPiTool(name string) bool { return strings.HasPrefix(name, "pi.") }

// piUnavailable is why the pi.* tools cannot run in this server, nil when
// they can: no daemon runner, the knob off, or no usable Pi.
func (s *Server) piUnavailable() error {
	if s.piForwards() {
		// The knob passed at the spawn tree's root; the daemon checks the
		// Pi version and its runner on every forwarded call.
		return nil
	}
	if s.pi.agents == nil {
		return errors.New("Pi subagents run in the dashboard daemon's agent runner, and this MCP server is not hosted by the dashboard daemon (it runs as a ws-mcp process of its own, after the daemon refused its connection); restart the session once the dashboard daemon accepts it")
	}
	if !s.piKnobOn() {
		return fmt.Errorf("%s is off; turn it on with config.tune and reconnect the session", DashboardPiSpawnKey)
	}
	return PiReady()
}

// piKnobOn reads dashboard.pi_spawn_enabled without a session, as
// DashboardSettings does: undeclared reads off.
func (s *Server) piKnobOn() bool {
	if _, ok := s.declared.lookup(DashboardPiSpawnKey); !ok {
		return false
	}
	got, err := s.resolveConfigGet("", DashboardPiSpawnKey)
	if err != nil {
		return false
	}
	on, _ := got.Value.(bool)
	return on
}

// piListed reports whether tools/list shows the pi.* tools: piUnavailable,
// resolved once per server.
func (s *Server) piListed() bool {
	s.pi.listOnce.Do(func() { s.pi.listErr = s.piUnavailable() })
	return s.pi.listErr == nil
}

// piParent is the parent reference of the children sessionKey spawns.
func (s *Server) piParent(sessionKey string) wsagent.ParentRef {
	return wsagent.ParentRef{Kind: wsagent.ParentLead, ReplyID: s.mailboxReplyID(sessionKey)}
}

func (s *Server) handlePiTool(ctx context.Context, id json.RawMessage, tool string, args map[string]any) response {
	if s.piForwards() {
		return s.handlePiForward(ctx, id, tool, args)
	}
	key, err := s.requireLeadSessionKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	if err := s.piUnavailable(); err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	parent := s.piParent(key)
	if tool == "pi.spawn" {
		return s.handlePiSpawn(id, key, parent, args)
	}
	if tool == "pi.list" {
		children := s.pi.agents.Children(parent)
		if wantsJSON(args) {
			out := make([]piChildOut, 0, len(children))
			for _, st := range children {
				out = append(out, piChild(st))
			}
			return toolJSONResponse(id, out, nil)
		}
		return toolTextResponse(id, formatPiList(children), nil)
	}
	child, err := stringArg(tool, "id", args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	if err := s.piOwned(tool, child, parent); err != nil {
		return toolTextResponse(id, "", err)
	}
	switch tool {
	case "pi.send":
		message, err := rawStringArg(tool, "message", args)
		if err == nil && strings.TrimSpace(message) == "" {
			err = fmt.Errorf("%s: message is required", tool)
		}
		if err != nil {
			return toolTextResponse(id, "", err)
		}
		st, err := s.pi.agents.Send(child, message)
		if err != nil {
			return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
		}
		if wantsJSON(args) {
			return toolJSONResponse(id, piChild(st), nil)
		}
		return toolTextResponse(id, fmt.Sprintf("sent to Pi subagent %s: starts a turn if idle, otherwise steers its running turn at the next tool boundary. That turn's completion mail covers the message.\n", child), nil)
	case "pi.stop":
		st, err := s.pi.agents.CloseConversation(child)
		if err != nil {
			return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
		}
		if wantsJSON(args) {
			return toolJSONResponse(id, piChild(st), nil)
		}
		return toolTextResponse(id, fmt.Sprintf("closed Pi subagent %s: its process stopped and it takes no more messages; pi.transcript still reads it.\n", child), nil)
	case "pi.transcript":
		records, err := s.pi.agents.Transcript(child)
		if err != nil {
			return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
		}
		limit := intFromArgument(args["limit"], piTranscriptDefault)
		if len(records) > limit {
			records = records[len(records)-limit:]
		}
		if wantsJSON(args) {
			return toolJSONResponse(id, piTranscriptJSON(records), nil)
		}
		return toolTextResponse(id, formatPiTranscript(child, records), nil)
	case "pi.wait":
		timeout := piWaitDefault
		if v, ok := args["timeout_s"]; ok {
			if d := durationFromSeconds(v); d > 0 {
				timeout = d
			}
		}
		timeout = min(timeout, piWaitMax)
		// The wait also ends when the server begins to drain.
		waitCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		stop := context.AfterFunc(s.drainContext(), cancel)
		defer stop()
		res, err := s.pi.agents.Wait(waitCtx, child, timeout)
		if err != nil {
			return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
		}
		if wantsJSON(args) {
			return toolJSONResponse(id, res, nil)
		}
		return toolTextResponse(id, formatPiWait(res, "its completion mail arrives when the turn ends"), nil)
	}
	return toolTextResponse(id, "", fmt.Errorf("unknown tool: %s", tool))
}

// piOwned refuses a child id that is not one of parent's children.
func (s *Server) piOwned(tool, child string, parent wsagent.ParentRef) error {
	st, err := s.pi.agents.Get(child)
	if err == nil && st.Parent != nil && *st.Parent == parent {
		return nil
	}
	return fmt.Errorf("%s: %s is not one of this session's Pi subagents (pi.list lists them)", tool, child)
}

func (s *Server) handlePiSpawn(id json.RawMessage, key string, parent wsagent.ParentRef, args map[string]any) response {
	const tool = "pi.spawn"
	prompt, err := rawStringArg(tool, "prompt", args)
	if err == nil && strings.TrimSpace(prompt) == "" {
		err = fmt.Errorf("%s: prompt is required", tool)
	}
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	rawTier, err := stringArg(tool, "tier", args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	tier := wsconfig.ModelAlias(rawTier)
	if tier == "" {
		return toolTextResponse(id, "", fmt.Errorf("%s: tier must be small, medium, large, or xlarge; got %q", tool, rawTier))
	}
	tiers, err := wsconfig.HarnessTiers(s.sessionConfigOptions(key), piHarness)
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: read agents.tier: %w", tool, err))
	}
	mapping, ok := tiers[tier]
	if !ok {
		return toolTextResponse(id, "", wsagent.PiTierRefusal(tool, tier, tiers))
	}
	entry, found := s.sessions.lookup(key)
	if !found {
		return toolTextResponse(id, "", fmt.Errorf("%s: unknown_session: session key not found", tool))
	}
	cwd, _ := args["cwd"].(string)
	cwd = strings.TrimSpace(cwd)
	switch {
	case cwd == "":
		cwd = entry.root
	case !filepath.IsAbs(cwd):
		cwd = filepath.Join(entry.root, cwd)
	}
	label, _ := args["label"].(string)
	if strings.TrimSpace(label) == "" {
		label = prompt
	}
	st, err := s.pi.agents.Spawn(wsagent.SpawnSpec{
		Profile: wsagent.PiWorkerProfile,
		Root:    entry.root,
		Cwd:     cwd,
		Model:   mapping.Model,
		Effort:  mapping.Effort,
		Tier:    tier,
		Prompt:  prompt,
		Label:   label,
		Parent:  &parent,
	})
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	out := piChild(st)
	out.Tier = tier
	if wantsJSON(args) {
		return toolJSONResponse(id, out, nil)
	}
	return toolTextResponse(id, fmt.Sprintf("spawned Pi subagent %s (tier %s: %s, effort %s) in %s; its first turn runs now.\nIts completion mail (status and final message) arrives in this session's mailbox when the turn ends: wait for it, or call pi.wait on a host that does not start a turn on mail. Steer it with pi.send; close it with pi.stop.\n", st.ID, tier, orDash(st.Model), orDash(st.Effort), piCwd(st)), nil)
}

// piChildOut is one child as pi.spawn, pi.send, pi.stop, and pi.list
// answer it in JSON.
type piChildOut struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Label   string `json:"label,omitempty"`
	Tier    string `json:"tier,omitempty"`
	Model   string `json:"model,omitempty"`
	Effort  string `json:"effort,omitempty"`
	Cwd     string `json:"cwd"`
	Held    int    `json:"held,omitempty"`
	Error   string `json:"error,omitempty"`
	Created string `json:"created_at,omitempty"`
	Closed  string `json:"closed_at,omitempty"`
}

func piChild(st wsagent.State) piChildOut {
	return piChildOut{ID: st.ID, Status: st.Status, Label: st.Label, Tier: st.Tier, Model: st.Model, Effort: st.Effort, Cwd: piCwd(st), Held: st.Held, Error: st.Error, Created: st.CreatedAt, Closed: st.ClosedAt}
}

// piCwd is the directory a child runs in.
func piCwd(st wsagent.State) string {
	if st.Cwd != "" {
		return st.Cwd
	}
	return st.Root
}

func orDash(v string) string {
	if v == "" {
		return "-"
	}
	return v
}

func formatPiList(children []wsagent.State) string {
	if len(children) == 0 {
		return "no Pi subagents\n"
	}
	var b strings.Builder
	for _, st := range children {
		fmt.Fprintf(&b, "%s  %s  %s/%s  %s", st.ID, st.Status, orDash(st.Model), orDash(st.Effort), piCwd(st))
		if st.Label != "" {
			fmt.Fprintf(&b, "  %q", st.Label)
		}
		if st.Held > 0 {
			fmt.Fprintf(&b, "  held %d", st.Held)
		}
		if st.Error != "" {
			fmt.Fprintf(&b, "  error: %s", oneLine(st.Error))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// formatPiWait is pi.wait's answer; later says how the turn's report
// arrives when the wait ends first.
func formatPiWait(res wsagent.TurnResult, later string) string {
	var b strings.Builder
	switch res.Status {
	case wsagent.TurnRunning:
		fmt.Fprintf(&b, "Pi subagent %s: still running; %s (or call pi.wait again)\n", res.ID, later)
		return b.String()
	case wsagent.TurnClosed:
		fmt.Fprintf(&b, "Pi subagent %s: closed\n", res.ID)
		return b.String()
	}
	if res.Delivered {
		fmt.Fprintf(&b, "Pi subagent %s: turn %s; its report already reached you (steered into your turn or sent as your next message), so it is not repeated here\n", res.ID, res.Status)
		return b.String()
	}
	fmt.Fprintf(&b, "Pi subagent %s: turn %s", res.ID, res.Status)
	if res.Error != "" {
		fmt.Fprintf(&b, ": %s", res.Error)
	}
	b.WriteString("\n")
	if res.Message != "" {
		b.WriteString("\n" + res.Message + "\n")
	}
	return b.String()
}

// piTranscriptEntry is one transcript line in JSON.
type piTranscriptEntry struct {
	At    string `json:"at"`
	Kind  string `json:"kind"`
	Text  string `json:"text,omitempty"`
	Tools string `json:"tools,omitempty"`
}

func piTranscriptJSON(records []wslive.RingRecord) []piTranscriptEntry {
	out := make([]piTranscriptEntry, 0, len(records))
	for _, r := range records {
		out = append(out, piTranscriptEntry{At: r.At.UTC().Format(time.RFC3339), Kind: r.Kind, Text: r.Text, Tools: piToolsLine(r)})
	}
	return out
}

func formatPiTranscript(id string, records []wslive.RingRecord) string {
	if len(records) == 0 {
		return fmt.Sprintf("Pi subagent %s: no transcript yet\n", id)
	}
	var b strings.Builder
	for _, r := range records {
		stamp := r.At.UTC().Format(time.RFC3339)
		switch {
		case r.Kind == wslive.RingUser || r.Kind == wslive.RingReply:
			fmt.Fprintf(&b, "[%s] %s:\n%s\n\n", stamp, r.Kind, r.Text)
		case piToolsLine(r) != "":
			fmt.Fprintf(&b, "[%s] tools: %s\n\n", stamp, piToolsLine(r))
		case r.Kind == wslive.RingTurn:
			fmt.Fprintf(&b, "[%s] turn ended (%d tool calls, %d failed, %.0fs)\n\n", stamp, r.Done+r.Failed, r.Failed, float64(r.DurationMs)/1000)
		case r.Text != "":
			fmt.Fprintf(&b, "[%s] %s: %s\n\n", stamp, r.Kind, r.Text)
		}
	}
	return b.String()
}

// piToolsLine summarizes a tools entry's calls by tool, "" for another kind.
func piToolsLine(r wslive.RingRecord) string {
	if r.Kind != wslive.RingTools || r.Tools == nil {
		return ""
	}
	parts := make([]string, 0, len(r.Tools.Tools)+1)
	for _, t := range r.Tools.Tools {
		parts = append(parts, fmt.Sprintf("%s x%d", t.Name, t.Calls))
	}
	if r.Tools.OtherCalls > 0 {
		parts = append(parts, fmt.Sprintf("other x%d", r.Tools.OtherCalls))
	}
	line := fmt.Sprintf("%d calls", r.Tools.Calls)
	if r.Tools.Failed > 0 {
		line += fmt.Sprintf(", %d failed", r.Tools.Failed)
	}
	if len(parts) > 0 {
		line += " (" + strings.Join(parts, ", ") + ")"
	}
	return line
}

// PiReady is the daemon runner's Options.ChildCheck: an installed Pi of at
// least piMinMajor.piMinMinor, probed once per process as for a lead's
// pi.* tools.
func PiReady() error {
	version, err := installedPiVersion()
	if err != nil {
		return fmt.Errorf("no usable pi: %v", err)
	}
	if !wsagent.VersionAtLeast(version, piMinMajor, piMinMinor) {
		return fmt.Errorf("pi %s is installed; Pi subagents need pi %d.%d or later", version, piMinMajor, piMinMinor)
	}
	return nil
}

// PiTurnReporter is the daemon runner's Options.Report: it posts each
// settled turn of a lead's Pi subagent to the lead's reply-id queue as a
// system message and wakes the mailbox waits. The registry is the one the
// daemon's own environment resolves, as for every session it hosts. A
// failed post is logged to log.
func PiTurnReporter(log io.Writer) func(wsagent.TurnReport) {
	return func(rep wsagent.TurnReport) {
		if rep.Parent.Kind != wsagent.ParentLead {
			return
		}
		path, err := wsmailbox.Paths{}.ReplyRegistry()
		if err == nil {
			err = wsmailbox.PostSystem(path, rep.Parent.ReplyID, wsagent.ReportText(rep), time.Now())
		}
		if err != nil {
			if log != nil {
				fmt.Fprintf(log, "agent %s: report to %s: %v\n", rep.Result.ID, rep.Parent.ReplyID, err)
			}
			return
		}
		wswake.Mailbox.Wake()
	}
}

// piToolDefinitions are the pi.* tools' definitions.
func piToolDefinitions() []map[string]any {
	key := stringProperty("Caller's ws session key (see ws:workflow-manual); a lead session's.")
	format := stringProperty(`Optional output format. Use "json" for structured output.`)
	child := stringProperty("The Pi subagent's id, as pi.spawn returned it.")
	return []map[string]any{
		{
			"name":        "pi.spawn",
			"description": "Spawn a Pi subagent: a headless Pi coding agent the dashboard daemon runs for this session, with Pi's coding tools (read, bash, edit, write, grep, find, ls) and the ws git tools, working in cwd. prompt is its first message; a rendered playbook prompt works as it does for a native subagent. tier picks the model from the pi bucket of agents.tier alone; an unset tier refuses and names the tiers that are set. Returns the subagent's id at once while its turn runs. Each time a turn ends or fails, a completion mail (the id, the status, and the final assistant message) arrives in this session's mailbox: that is the primary signal, and on a host with mailbox push it starts this session's next turn. pi.wait is the fallback for a host without push. Steer it with pi.send, read it with pi.transcript, list this session's subagents with pi.list, and close one with pi.stop.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": key,
					"prompt":      stringProperty("The subagent's first message."),
					"tier":        stringProperty(`Capability tier: "small", "medium", "large", or "xlarge", resolved from agents.tier's pi bucket.`),
					"cwd":         stringProperty("Directory the subagent works in: absolute, or relative to the session's root. Default: the session's root."),
					"label":       stringProperty("Optional one-line name for the subagent's agents-panel row and its completion mail; default: the prompt's first line."),
					"format":      format,
				},
				"required": []string{"session_key", "prompt", "tier"},
			},
		},
		{
			"name":        "pi.send",
			"description": "Send one of this session's Pi subagents a follow-up message. With no turn running it starts the next turn now; during a turn it steers that running turn at the next tool boundary, without cancelling in-flight tool calls. The running turn's completion mail covers the steered message; no extra turn is queued after it is taken. pi.wait is the fallback.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": key,
					"id":          child,
					"message":     stringProperty("The message."),
					"format":      format,
				},
				"required": []string{"session_key", "id", "message"},
			},
		},
		{
			"name":        "pi.list",
			"description": "List the Pi subagents this session spawned, open and closed: id, status (idle, running, or closed), model and effort, working directory, label, held messages, and the last error.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": key,
					"format":      format,
				},
				"required": []string{"session_key"},
			},
		},
		{
			"name":        "pi.stop",
			"description": "Close one of this session's Pi subagents: its process stops, a running turn ends with it, and it takes no more messages. No completion mail follows. Its record stays: pi.list shows it closed and pi.transcript still reads it.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": key,
					"id":          child,
					"format":      format,
				},
				"required": []string{"session_key", "id"},
			},
		},
		{
			"name":        "pi.transcript",
			"description": "Read one of this session's Pi subagents' conversation, oldest first: the messages it was sent, its replies, its tool calls by tool, and its turn ends. Works for a closed subagent too.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": key,
					"id":          child,
					"limit":       integerProperty(fmt.Sprintf("How many of the newest entries to return; default %d.", piTranscriptDefault)),
					"format":      format,
				},
				"required": []string{"session_key", "id"},
			},
		},
		{
			"name":        "pi.wait",
			"description": fmt.Sprintf("Fallback for a host without mailbox push; the completion mail is the primary signal. Block until the turn of one of this session's Pi subagents that took your last pi.send (or pi.spawn's prompt) ends or fails, and return its status and final assistant message; it never returns an earlier turn's message, and a turn that already ended returns at once. timeout_s defaults to %d and is capped at %d; when it passes first the status is running. The host may cut a long call off sooner (a tool-call timeout), so prefer a timeout below the host's. The completion mail still arrives after a wait collected the result.", int(piWaitDefault/time.Second), int(piWaitMax/time.Second)),
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": key,
					"id":          child,
					"timeout_s":   numberProperty(fmt.Sprintf("Seconds to wait at most; default %d, at most %d.", int(piWaitDefault/time.Second), int(piWaitMax/time.Second))),
					"format":      format,
				},
				"required": []string{"session_key", "id"},
			},
		},
	}
}
