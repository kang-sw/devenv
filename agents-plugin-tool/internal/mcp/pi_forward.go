package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wslive"
)

// pi_forward.go is the pi.* tools of a Pi subagent's attached ws-mcp: no
// runner here, so each call goes to the dashboard daemon's token route
// (wsagent children.go) on the capability token the runner put in this
// server's environment. The token, not the session key, authorizes the
// call, and scopes it to this subagent's own children; the key stays a
// delegate key, so every other lead-only tool stays refused.

// piForwardSlack is how much longer than its own wait a forwarded pi.wait
// waits for the daemon's answer.
const piForwardSlack = 30 * time.Second

// piForwardTimeout bounds every other forwarded call.
const piForwardTimeout = 60 * time.Second

// piForward is the token and the daemon's base URL; ok is false when
// either is missing.
func (s *Server) piForward() (token, endpoint string, ok bool) {
	token = strings.TrimSpace(s.getenv(wsagent.EnvPiToken))
	endpoint = strings.TrimRight(strings.TrimSpace(s.getenv(wsagent.EnvPiEndpoint)), "/")
	return token, endpoint, token != "" && endpoint != ""
}

// piForwards reports whether this server forwards the pi.* tools: it has
// no runner of its own and the runner gave it a token.
func (s *Server) piForwards() bool {
	if s.pi.agents != nil {
		return false
	}
	_, _, ok := s.piForward()
	return ok
}

// piCall posts one forwarded call to the token route and decodes its
// answer into out.
func (s *Server) piCall(ctx context.Context, tool string, body map[string]any, timeout time.Duration, out any) error {
	token, endpoint, _ := s.piForward()
	op := strings.TrimPrefix(tool, "pi.")
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+wsagent.RoutePrefix+"/by-token/"+op, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("the dashboard daemon, which runs this subagent's Pi subagents, did not answer at %s: %v", endpoint, err)
	}
	defer resp.Body.Close()
	answer, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return fmt.Errorf("read the dashboard daemon's answer: %v", err)
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s", strings.TrimSpace(string(answer)))
	}
	if err := json.Unmarshal(answer, out); err != nil {
		return fmt.Errorf("the dashboard daemon's answer is not JSON: %v", err)
	}
	return nil
}

// handlePiForward runs a pi.* call of a Pi subagent through the daemon.
func (s *Server) handlePiForward(ctx context.Context, id json.RawMessage, tool string, args map[string]any) response {
	fail := func(err error) response {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	key, _ := args["session_key"].(string)
	if strings.TrimSpace(key) == "" {
		return toolTextResponse(id, "", fmt.Errorf("%s: session_key is required", tool))
	}
	if _, found := s.sessions.lookup(strings.TrimSpace(key)); !found {
		return toolTextResponse(id, "", fmt.Errorf("%s: unknown_session: session key not found", tool))
	}
	switch tool {
	case "pi.spawn":
		prompt, err := rawStringArg(tool, "prompt", args)
		if err == nil && strings.TrimSpace(prompt) == "" {
			err = fmt.Errorf("%s: prompt is required", tool)
		}
		if err != nil {
			return toolTextResponse(id, "", err)
		}
		tier, err := stringArg(tool, "tier", args)
		if err != nil {
			return toolTextResponse(id, "", err)
		}
		cwd, _ := args["cwd"].(string)
		label, _ := args["label"].(string)
		var st wsagent.State
		if err := s.piCall(ctx, tool, map[string]any{"prompt": prompt, "tier": tier, "cwd": cwd, "label": label}, piForwardTimeout, &st); err != nil {
			return fail(err)
		}
		if wantsJSON(args) {
			return toolJSONResponse(id, piChild(st), nil)
		}
		return toolTextResponse(id, fmt.Sprintf("spawned Pi subagent %s (tier %s: %s, effort %s) in %s; its first turn runs now.\nIts report (status and final message) comes to you when the turn ends: steered into your running turn, or as your next message when you are idle. Your own report to your parent waits until every open subagent's report has reached you, so you may end your turn while it works. Steer it with pi.send; close it with pi.stop.\n", st.ID, orDash(st.Tier), orDash(st.Model), orDash(st.Effort), piCwd(st)), nil)
	case "pi.list":
		var answer struct {
			Conversations []wsagent.State `json:"conversations"`
		}
		if err := s.piCall(ctx, tool, map[string]any{}, piForwardTimeout, &answer); err != nil {
			return fail(err)
		}
		if wantsJSON(args) {
			out := make([]piChildOut, 0, len(answer.Conversations))
			for _, st := range answer.Conversations {
				out = append(out, piChild(st))
			}
			return toolJSONResponse(id, out, nil)
		}
		return toolTextResponse(id, formatPiList(answer.Conversations), nil)
	}
	child, err := stringArg(tool, "id", args)
	if err != nil {
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
		var st wsagent.State
		if err := s.piCall(ctx, tool, map[string]any{"id": child, "message": message}, piForwardTimeout, &st); err != nil {
			return fail(err)
		}
		if wantsJSON(args) {
			return toolJSONResponse(id, piChild(st), nil)
		}
		how := "starts its next turn now"
		if st.Status == wsagent.StatusRunning {
			how = "waits for its running turn to end, then goes as its next turn (with any other message sent meanwhile)"
		}
		return toolTextResponse(id, fmt.Sprintf("sent to Pi subagent %s: the message %s. Its report comes to you when that turn ends.\n", child, how), nil)
	case "pi.stop":
		var st wsagent.State
		if err := s.piCall(ctx, tool, map[string]any{"id": child}, piForwardTimeout, &st); err != nil {
			return fail(err)
		}
		if wantsJSON(args) {
			return toolJSONResponse(id, piChild(st), nil)
		}
		return toolTextResponse(id, fmt.Sprintf("closed Pi subagent %s and its own subagents: its process stopped and it takes no more messages; pi.transcript still reads it.\n", child), nil)
	case "pi.transcript":
		var answer struct {
			Records []wslive.RingRecord `json:"records"`
		}
		if err := s.piCall(ctx, tool, map[string]any{"id": child}, piForwardTimeout, &answer); err != nil {
			return fail(err)
		}
		records := answer.Records
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
		waitCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		stop := context.AfterFunc(s.drainContext(), cancel)
		defer stop()
		var res wsagent.TurnResult
		if err := s.piCall(waitCtx, tool, map[string]any{"id": child, "timeout_s": timeout.Seconds()}, timeout+piForwardSlack, &res); err != nil {
			return fail(err)
		}
		if wantsJSON(args) {
			return toolJSONResponse(id, res, nil)
		}
		return toolTextResponse(id, formatPiWait(res, "its report comes to you when the turn ends"), nil)
	}
	return toolTextResponse(id, "", fmt.Errorf("unknown tool: %s", tool))
}

// piWorkerDescriptions are the pi.* tools' descriptions as a Pi subagent
// reads them (piForwards), by name.
func piWorkerDescriptions() map[string]string {
	return map[string]string{
		"pi.spawn":      "Spawn a Pi subagent of your own: a headless Pi coding agent the dashboard daemon runs for you, with Pi's coding tools (read, bash, edit, write, grep, find, ls) and the ws git tools, working in cwd. prompt is its first message; a rendered playbook prompt works as it does for a native subagent. tier picks the model from the pi bucket of agents.tier alone and may not be above your own tier. The spawn tree is at most two levels below the lead, so a subagent of yours gets a refusal naming the cap when it tries to spawn. Returns the subagent's id at once while its turn runs. Each time a turn ends or fails, its report (the id, the status, and the final assistant message) comes to you: steered into your running turn, or as your next message when you are idle. Your own report to your parent waits until every open subagent's report has reached you, so you may end your turn while they work. Steer it with pi.send, read it with pi.transcript, list yours with pi.list, and close one with pi.stop.",
		"pi.send":       "Send one of your Pi subagents a follow-up message. With no turn running it starts the next turn now; during a turn it waits for that turn to end and then goes as the next turn, merged with any other message sent meanwhile. Its report comes to you when the turn ends, as after pi.spawn.",
		"pi.list":       "List the Pi subagents you spawned, open and closed: id, status (idle, running, or closed), model and effort, working directory, label, held messages, and the last error.",
		"pi.stop":       "Close one of your Pi subagents and, first, its own subagents: its process stops, a running turn ends with it, and it takes no more messages. No report follows. Its record stays: pi.list shows it closed and pi.transcript still reads it.",
		"pi.transcript": "Read one of your Pi subagents' conversation, oldest first: the messages it was sent, its replies, its tool calls by tool, and its turn ends. Works for a closed subagent too.",
		"pi.wait":       fmt.Sprintf("Block until the turn of one of your Pi subagents that took your last pi.send (or pi.spawn's prompt) ends or fails, and return its status and final assistant message; it never returns an earlier turn's message, and a turn that already ended returns at once. The answer is that turn's report: it is not also steered into your turn. A turn whose report already reached you before the wait returns its status alone, saying so. timeout_s defaults to %d and is capped at %d; when it passes first the status is running, and the report comes to you as usual.", int(piWaitDefault/time.Second), int(piWaitMax/time.Second)),
	}
}

// piWorkerTool is tool, a pi.* definition, as a Pi subagent reads it.
func piWorkerTool(tool map[string]any) map[string]any {
	name, _ := tool["name"].(string)
	desc, ok := piWorkerDescriptions()[name]
	if !ok {
		return tool
	}
	clone := make(map[string]any, len(tool))
	for k, v := range tool {
		clone[k] = v
	}
	clone["description"] = desc
	if schema, ok := tool["inputSchema"].(map[string]any); ok {
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
		if _, ok := propsClone["cwd"]; ok {
			propsClone["cwd"] = stringProperty("Directory the subagent works in: absolute, or relative to your own working directory. Default: your own working directory.")
		}
		if _, ok := propsClone["id"]; ok {
			propsClone["id"] = stringProperty("Your Pi subagent's id, as pi.spawn returned it.")
		}
		schemaClone["properties"] = propsClone
		clone["inputSchema"] = schemaClone
	}
	return clone
}
