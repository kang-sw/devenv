package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wswake"
)

// agent_tools.go is the agent.* tools: agent.spawn, agent.interrupt,
// agent.list and agent.tiers. A lead session's server calls the dashboard
// daemon's runner (ServerOptions.Agents) with the lead as the caller; a
// spawned agent's attached ws-mcp has no runner and forwards each call to
// the runner's token route on its capability token, as pi.* does
// (pi_forward.go). Harness availability is the runner's per-tier check,
// so no knob or Pi probe gates these tools here.

// AgentRunner is the daemon runner's agent.* side (*wsagent.Runner).
type AgentRunner interface {
	AgentSpawn(wsagent.AgentCaller, wsagent.AgentSpawnRequest) (wsagent.AgentSpawnResult, error)
	AgentInterrupt(wsagent.AgentCaller, string) (wsagent.AgentInterruptResult, error)
	AgentList(wsagent.AgentCaller) []wsagent.AgentChild
	AgentTiers(wsagent.AgentCaller) (wsagent.AgentTiers, error)
	// Deliver is the wake seam: Send, plus the sender's notice when a
	// sleeping child's start admission refuses.
	Deliver(id string, d wsagent.Delivery) (wsagent.State, error)
}

func isAgentTool(name string) bool {
	switch name {
	case "agent.spawn", "agent.interrupt", "agent.list", "agent.tiers":
		return true
	}
	return false
}

// agentListed reports whether tools/list shows the agent.* tools: where a
// call can reach a runner, the daemon's own (a lead) or over a token (a
// spawned agent).
func (s *Server) agentListed() bool {
	return s.pi.agents != nil || s.piForwards()
}

func (s *Server) handleAgentTool(ctx context.Context, id json.RawMessage, tool string, args map[string]any) response {
	if s.piForwards() {
		return s.handleAgentForward(ctx, id, tool, args)
	}
	key, err := s.requireLeadSessionKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	if s.pi.agents == nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: spawned agents run in the dashboard daemon's agent runner, and this MCP server is not hosted by the dashboard daemon (it runs as a ws-mcp process of its own, after the daemon refused its connection); restart the session once the dashboard daemon accepts it", tool))
	}
	entry, found := s.lookupSession(key)
	if !found {
		return toolTextResponse(id, "", fmt.Errorf("%s: unknown_session: session key not found", tool))
	}
	caller := wsagent.LeadCaller(s.mailboxReplyID(key), entry.root, s.currentHarness(), s.sessionConfigOptions(key))
	runner := s.pi.agents
	switch tool {
	case "agent.spawn":
		req, err := agentSpawnRequest(args)
		if err != nil {
			return toolTextResponse(id, "", err)
		}
		res, err := runner.AgentSpawn(caller, req)
		if err != nil {
			return agentFailure(id, tool, err)
		}
		if wantsJSON(args) {
			return toolJSONResponse(id, res, nil)
		}
		return toolTextResponse(id, formatAgentSpawn(res, false), nil)
	case "agent.interrupt":
		address, err := stringArg(tool, "address", args)
		if err != nil {
			return toolTextResponse(id, "", err)
		}
		res, err := runner.AgentInterrupt(caller, address)
		if err != nil {
			return agentFailure(id, tool, err)
		}
		if wantsJSON(args) {
			return toolJSONResponse(id, res, nil)
		}
		return toolTextResponse(id, formatAgentInterrupt(res), nil)
	case "agent.list":
		children := runner.AgentList(caller)
		if wantsJSON(args) {
			return toolJSONResponse(id, agentListJSON(children), nil)
		}
		return toolTextResponse(id, formatAgentList(children), nil)
	case "agent.tiers":
		res, err := runner.AgentTiers(caller)
		if err != nil {
			return agentFailure(id, tool, err)
		}
		if wantsJSON(args) {
			return toolJSONResponse(id, res, nil)
		}
		return toolTextResponse(id, formatAgentTiers(res), nil)
	}
	return toolTextResponse(id, "", fmt.Errorf("unknown tool: %s", tool))
}

// handleAgentForward runs a spawned agent's agent.* call through the
// daemon's token route (wsagent serveAgentToken).
func (s *Server) handleAgentForward(ctx context.Context, id json.RawMessage, tool string, args map[string]any) response {
	key, _ := args["session_key"].(string)
	if strings.TrimSpace(key) == "" {
		return toolTextResponse(id, "", fmt.Errorf("%s: session_key is required", tool))
	}
	if _, found := s.lookupSession(strings.TrimSpace(key)); !found {
		return toolTextResponse(id, "", fmt.Errorf("%s: unknown_session: session key not found", tool))
	}
	switch tool {
	case "agent.spawn":
		req, err := agentSpawnRequest(args)
		if err != nil {
			return toolTextResponse(id, "", err)
		}
		body := map[string]any{"prompt": req.Prompt, "tier": req.Tier, "authority": req.Authority,
			"system_prompt": req.SystemPrompt, "system_prompt_path": req.SystemPromptPath,
			"cwd": req.Cwd, "worktree": req.Worktree, "label": req.Label}
		var res wsagent.AgentSpawnResult
		if err := s.piCall(ctx, tool, body, piForwardTimeout, &res); err != nil {
			return agentFailure(id, tool, err)
		}
		if wantsJSON(args) {
			return toolJSONResponse(id, res, nil)
		}
		return toolTextResponse(id, formatAgentSpawn(res, true), nil)
	case "agent.interrupt":
		address, err := stringArg(tool, "address", args)
		if err != nil {
			return toolTextResponse(id, "", err)
		}
		var res wsagent.AgentInterruptResult
		if err := s.piCall(ctx, tool, map[string]any{"address": address}, piForwardTimeout, &res); err != nil {
			return agentFailure(id, tool, err)
		}
		if wantsJSON(args) {
			return toolJSONResponse(id, res, nil)
		}
		return toolTextResponse(id, formatAgentInterrupt(res), nil)
	case "agent.list":
		var answer struct {
			Agents []wsagent.AgentChild `json:"agents"`
		}
		if err := s.piCall(ctx, tool, map[string]any{}, piForwardTimeout, &answer); err != nil {
			return agentFailure(id, tool, err)
		}
		if wantsJSON(args) {
			return toolJSONResponse(id, agentListJSON(answer.Agents), nil)
		}
		return toolTextResponse(id, formatAgentList(answer.Agents), nil)
	case "agent.tiers":
		var res wsagent.AgentTiers
		if err := s.piCall(ctx, tool, map[string]any{}, piForwardTimeout, &res); err != nil {
			return agentFailure(id, tool, err)
		}
		if wantsJSON(args) {
			return toolJSONResponse(id, res, nil)
		}
		return toolTextResponse(id, formatAgentTiers(res), nil)
	}
	return toolTextResponse(id, "", fmt.Errorf("unknown tool: %s", tool))
}

// agentSpawnRequest reads agent.spawn's arguments. The runner checks
// authority, tier, and the at-most-one pairs.
func agentSpawnRequest(args map[string]any) (wsagent.AgentSpawnRequest, error) {
	const tool = "agent.spawn"
	prompt, err := rawStringArg(tool, "prompt", args)
	if err == nil && strings.TrimSpace(prompt) == "" {
		err = fmt.Errorf("%s: prompt is required", tool)
	}
	if err != nil {
		return wsagent.AgentSpawnRequest{}, err
	}
	str := func(name string) string {
		v, _ := args[name].(string)
		return v
	}
	return wsagent.AgentSpawnRequest{
		Prompt:           prompt,
		Tier:             str("tier"),
		SystemPrompt:     str("system_prompt"),
		SystemPromptPath: str("system_prompt_path"),
		Authority:        str("authority"),
		Cwd:              str("cwd"),
		Worktree:         str("worktree"),
		Label:            str("label"),
	}, nil
}

// agentFailure is a refused agent.* call; the runner's refusals already
// name the tool.
func agentFailure(id json.RawMessage, tool string, err error) response {
	if !strings.HasPrefix(err.Error(), tool+": ") {
		err = fmt.Errorf("%s: %w", tool, err)
	}
	return toolTextResponse(id, "", err)
}

// agentFollowUp is how a caller sends a child a follow-up until mailbox
// delivery to a child's address lands; delete it then, with its uses.
func agentFollowUp(address string) string {
	return fmt.Sprintf("Until mail to a child's address is delivered, send it a follow-up with pi.send(id: %s).", strings.TrimPrefix(address, "id:"))
}

func formatAgentSpawn(res wsagent.AgentSpawnResult, worker bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "spawned agent %s (%s); its first turn runs now.\n", res.Address, res.Status)
	if worker {
		b.WriteString("Its report (status and final message) comes to you when each turn ends: steered into your running turn, or as your next message when you are idle. Your own report to your parent waits until every open child's report has reached you, so you may end your turn while it works.\n")
		return b.String()
	}
	b.WriteString("Its completion mail (status and final message) arrives in this session's mailbox when each turn ends: end your turn and let it arrive. It sleeps after its idle timer and wakes on its next message.\n")
	b.WriteString(agentFollowUp(res.Address) + "\n")
	return b.String()
}

func formatAgentInterrupt(res wsagent.AgentInterruptResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "interrupted agent %s (%s); it stays, and its next message starts a new turn.\n", res.Address, res.Status)
	if len(res.Returned) > 0 {
		b.WriteString("Messages it had not taken yet, handed back:\n")
		for _, m := range res.Returned {
			fmt.Fprintf(&b, "- %s\n", oneLine(m))
		}
	}
	return b.String()
}

func agentListJSON(children []wsagent.AgentChild) []wsagent.AgentChild {
	if children == nil {
		return []wsagent.AgentChild{}
	}
	return children
}

func formatAgentList(children []wsagent.AgentChild) string {
	if len(children) == 0 {
		return "no child agents\n"
	}
	var b strings.Builder
	for _, c := range children {
		fmt.Fprintf(&b, "%s  %s  %s  %s  %s/%s  %s", c.Address, c.State, orDash(c.Tier), orDash(string(c.Authority)), orDash(c.Model), orDash(c.Effort), orDash(c.Cwd))
		if c.Label != "" {
			fmt.Fprintf(&b, "  %q", oneLine(c.Label))
		}
		if c.Held > 0 {
			fmt.Fprintf(&b, "  held %d", c.Held)
		}
		if c.Error != "" {
			fmt.Fprintf(&b, "  error: %s", oneLine(c.Error))
		}
		b.WriteString("\n")
		if c.LastReport != "" {
			fmt.Fprintf(&b, "  last report: %s\n", c.LastReport)
		}
	}
	return b.String()
}

func formatAgentTiers(t wsagent.AgentTiers) string {
	var b strings.Builder
	entries := func(title string, list []wsagent.AgentTierEntry) {
		fmt.Fprintf(&b, "%s:\n", title)
		if len(list) == 0 {
			b.WriteString("  (none)\n")
		}
		for _, e := range list {
			fmt.Fprintf(&b, "  %s: %s/%s\n", e.Tier, orDash(e.Model), orDash(e.Effort))
		}
	}
	entries(fmt.Sprintf("native (bare tiers on this session's harness, %s)", orDash(t.Harness)), t.Native)
	entries("ext (<harness>.<tier>)", t.Ext)
	b.WriteString("aliases (agent.tier_aliases):\n")
	if len(t.Aliases) == 0 {
		b.WriteString("  (none)\n")
	}
	for _, a := range t.Aliases {
		if a.Error != "" {
			fmt.Fprintf(&b, "  %s -> %s: not spawnable now: %s\n", a.Alias, a.Target, a.Error)
			continue
		}
		fmt.Fprintf(&b, "  %s -> %s: %s/%s\n", a.Alias, a.Target, orDash(a.Model), orDash(a.Effort))
	}
	if len(t.Unavailable) > 0 {
		b.WriteString("unavailable harnesses:\n")
		for _, u := range t.Unavailable {
			fmt.Fprintf(&b, "  %s\n", oneLine(u))
		}
	}
	return b.String()
}

// AgentWakeRefused is the daemon runner's Options.WakeRefused: it tells the
// sender of a message queued for a sleeping agent whose wake admission
// refused, with one system mail to the sender's reply-id queue, and wakes
// the mailbox waits. A failed post is logged to log.
func AgentWakeRefused(log io.Writer) func(from, id, reason string) {
	return func(from, id, reason string) {
		address := wsagent.AgentAddress(id)
		content := fmt.Sprintf("The wake of agent %s was refused: %s\nYour message stays queued for it and reaches it when it next wakes.", address, reason)
		if err := (wsmailbox.Paths{}).Store().PostSystem(from, "agent:"+id, content, time.Now()); err != nil {
			if log != nil {
				fmt.Fprintf(log, "agent %s: wake refusal to %s: %v\n", id, from, err)
			}
			return
		}
		wswake.Mailbox.Wake()
	}
}

// agentToolDefinitions are the agent.* tools' definitions as a lead reads
// them; agentWorkerTool rewrites them for a spawned agent.
func agentToolDefinitions() []map[string]any {
	key := stringProperty("Caller's ws session key (see ws:workflow-manual); a lead session's.")
	format := stringProperty(`Optional output format. Use "json" for structured output.`)
	return []map[string]any{
		{
			"name":        "agent.spawn",
			"description": agentSpawnDescription(false),
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key":        key,
					"prompt":             stringProperty("The child's first message."),
					"tier":               stringProperty("A bare tier (small, medium, large, xlarge) on this session's own harness, <harness>.<tier> (for example pi.small) for another harness, or an alias from the agent.tier_aliases config knob. agent.tiers lists what is spawnable now."),
					"system_prompt":      stringProperty("Optional text appended to the harness's own system prompt. Give at most one of system_prompt and system_prompt_path."),
					"system_prompt_path": stringProperty("Optional file whose text is appended to the harness's own system prompt, read once at spawn: absolute, or relative to the session's root. Give at most one of system_prompt and system_prompt_path."),
					"authority":          stringProperty("Required: read-only, workdir-write, or full; at most this session's own permission level."),
					"cwd":                stringProperty("Optional directory the child works in: absolute, or relative to the session's root. Default: the session's root. Give at most one of cwd and worktree."),
					"worktree":           stringProperty("Optional root of an existing git worktree this session acquired: absolute, or relative to the session's root. The child runs there, with it as its project root. Give at most one of cwd and worktree."),
					"label":              stringProperty("Optional one-line name for the child's dashboard row and its completion mail; default: the prompt's first line."),
					"format":             format,
				},
				"required": []string{"session_key", "prompt", "tier", "authority"},
			},
		},
		{
			"name":        "agent.interrupt",
			"description": "Interrupt the running turn of one of this session's child agents. The child stays: the messages it had not taken yet are handed back in the answer, and its next message starts a new turn. An address this session did not spawn directly is refused.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": key,
					"address":     stringProperty("The child's address, as agent.spawn returned it (id:<id>)."),
					"format":      format,
				},
				"required": []string{"session_key", "address"},
			},
		},
		{
			"name":        "agent.list",
			"description": "List the child agents this session spawned directly: address, state (running, starting, idle, waiting: asleep with no turn, or closed), tier as <harness>.<tier>, authority, model and effort, working directory, label, held messages, the last error, and the last report (its status and first line).",
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
			"name":        "agent.tiers",
			"description": "List the tiers agent.spawn accepts now, each with the model and effort it resolves to: native (bare tiers on this session's own harness), ext (<harness>.<tier> on each harness that can run a spawned agent now), and the aliases of the agent.tier_aliases config knob. A harness or alias that cannot spawn now says why.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": key,
					"format":      format,
				},
				"required": []string{"session_key"},
			},
		},
	}
}

func agentSpawnDescription(worker bool) string {
	parts := []string{
		"Spawn a child agent: a headless agent the dashboard daemon runs for you, working in cwd or in worktree. prompt is its first message.",
		"tier picks its harness and model: a bare tier (small, medium, large, xlarge) on your own harness, <harness>.<tier> for another harness, or a user alias from the agent.tier_aliases config knob; agent.tiers lists what is spawnable now, and an unusable tier is refused with that list.",
		"authority is required: read-only, workdir-write, or full, capped at your own permission level; a read-only child's ws tools refuse commits and ticket changes.",
		"system_prompt or system_prompt_path (at most one) is appended to the harness's own system prompt. Give at most one of cwd and worktree; worktree is the root of an existing git worktree you acquired, and the child runs there.",
		"Returns the child's address (id:<id>): its mailbox address and the key agent.interrupt and agent.list take.",
	}
	if worker {
		parts = append(parts,
			"Each time its turn ends or fails, its report (status and final message) comes to you: steered into your running turn, or as your next message when you are idle. Your own report to your parent waits until every open child's report has reached you, so you may end your turn while they work.",
			"The child sleeps after its idle timer and wakes on its next message.")
		return strings.Join(parts, " ")
	}
	parts = append(parts,
		"Each time its turn ends or fails, a completion mail (status and final message) arrives in this session's mailbox: end your turn and let it arrive.",
		"The child sleeps after its idle timer and wakes on its next message.",
		// Until mailbox delivery to a child's address lands; delete then.
		`Until mail to a child's address is delivered, send it a follow-up with pi.send(id: <its address without the "id:" prefix>).`)
	return strings.Join(parts, " ")
}

// agentWorkerTool is tool, an agent.* definition, as a spawned agent
// reads it (piForwards).
func agentWorkerTool(tool map[string]any) map[string]any {
	name, _ := tool["name"].(string)
	clone := make(map[string]any, len(tool))
	for k, v := range tool {
		clone[k] = v
	}
	if name == "agent.spawn" {
		clone["description"] = agentSpawnDescription(true)
	}
	if desc, ok := clone["description"].(string); ok {
		desc = strings.ReplaceAll(desc, "this session's", "your")
		clone["description"] = strings.ReplaceAll(desc, "this session ", "you ")
	}
	schema, ok := tool["inputSchema"].(map[string]any)
	if !ok {
		return clone
	}
	props, _ := schema["properties"].(map[string]any)
	schemaClone := make(map[string]any, len(schema))
	for k, v := range schema {
		schemaClone[k] = v
	}
	propsClone := make(map[string]any, len(props))
	for k, v := range props {
		propsClone[k] = v
	}
	propsClone["session_key"] = stringProperty("Your own ws session key, from your instructions.")
	for _, arg := range []string{"cwd", "worktree", "system_prompt_path", "tier", "authority"} {
		if p, ok := propsClone[arg].(map[string]string); ok {
			d := strings.ReplaceAll(p["description"], "relative to the session's root", "relative to your own working directory")
			d = strings.ReplaceAll(d, "Default: the session's root", "Default: your own working directory")
			d = strings.ReplaceAll(d, "this session's own", "your own")
			d = strings.ReplaceAll(d, "this session", "you")
			propsClone[arg] = stringProperty(d)
		}
	}
	schemaClone["properties"] = propsClone
	clone["inputSchema"] = schemaClone
	return clone
}
