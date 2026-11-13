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
	"github.com/kang-sw/devenv/internal/wsdashboard"
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
const piForwardSlack = wsagent.PiForwardSlack

// piForwardTimeout bounds every other forwarded call.
const piForwardTimeout = 60 * time.Second

// piForward finds the current daemon in this server's cache, since the
// process's inherited endpoint can name a daemon replaced while it ran.
// The inherited endpoint remains a fallback for older setups and tests.
func (s *Server) piForward() (token, endpoint string, ok bool) {
	token = strings.TrimSpace(s.getenv(wsagent.EnvPiToken))
	endpoint = strings.TrimRight(strings.TrimSpace(s.getenv(wsagent.EnvPiEndpoint)), "/")
	if token != "" {
		if dir, err := s.dashboard.stateDir(); err == nil {
			if ep, err := wsdashboard.ReadEndpoint(dir); err == nil && ep.Port > 0 && ep.Port <= 65535 {
				endpoint = fmt.Sprintf("http://127.0.0.1:%d", ep.Port)
			}
		}
	}
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
	op := strings.TrimPrefix(tool, "pi.")
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var waitDeadline time.Time
	if op == "wait" {
		waitDeadline = time.Now().Add(durationFromSeconds(body["timeout_s"]))
		// A retry changes only its remaining wait, not the caller's arguments.
		copyBody := make(map[string]any, len(body))
		for key, value := range body {
			copyBody[key] = value
		}
		body = copyBody
	}
	for {
		token, endpoint, _ := s.piForward()
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+wsagent.RoutePrefix+"/by-token/"+op, bytes.NewReader(raw))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			err = fmt.Errorf("the dashboard daemon, which runs this subagent's Pi subagents, did not answer at %s: %w", endpoint, err)
		} else {
			var answer []byte
			answer, err = io.ReadAll(io.LimitReader(resp.Body, 16<<20))
			resp.Body.Close()
			if err != nil {
				err = fmt.Errorf("read the dashboard daemon's answer: %w", err)
			} else {
				if resp.StatusCode == http.StatusServiceUnavailable && op == "wait" {
					// A graceful daemon detach has no settlement to return.
					err = fmt.Errorf("%s", strings.TrimSpace(string(answer)))
				} else {
					if resp.StatusCode/100 != 2 {
						return fmt.Errorf("%s", strings.TrimSpace(string(answer)))
					}
					if err := json.Unmarshal(answer, out); err != nil {
						return fmt.Errorf("the dashboard daemon's answer is not JSON: %v", err)
					}
					return nil
				}
			}
		}
		// Only a disconnected wait is safe to reissue. A spawn/send/stop may
		// have taken effect before the connection broke; never repeat it.
		if op != "wait" || ctx.Err() != nil || time.Until(waitDeadline) <= 0 {
			return err
		}
		timer := time.NewTimer(min(100*time.Millisecond, time.Until(waitDeadline)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		remaining := time.Until(waitDeadline)
		if remaining <= 0 {
			return err
		}
		body["timeout_s"] = remaining.Seconds()
	}
}

// handlePiForward runs a pi.* call of a Pi subagent through the daemon.
func (s *Server) handlePiForward(ctx context.Context, id json.RawMessage, tool string, args map[string]any) response {
	fail := func(err error) response {
		// The daemon's refusals already name the tool.
		if !strings.HasPrefix(err.Error(), tool+": ") {
			err = fmt.Errorf("%s: %w", tool, err)
		}
		return toolTextResponse(id, "", err)
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
		profile, err := piSpawnProfile(args)
		if err != nil {
			return toolTextResponse(id, "", err)
		}
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
		if err := s.piCall(ctx, tool, map[string]any{"prompt": prompt, "tier": tier, "profile": profile, "cwd": cwd, "label": label}, piForwardTimeout, &st); err != nil {
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
		return toolTextResponse(id, fmt.Sprintf("sent to Pi subagent %s: starts a turn if idle, otherwise steers its running turn at the next tool boundary. That turn's report covers the message.\n", child), nil)
	case "pi.stop":
		var st wsagent.State
		if err := s.piCall(ctx, tool, map[string]any{"id": child}, piForwardTimeout, &st); err != nil {
			return fail(err)
		}
		if wantsJSON(args) {
			return toolJSONResponse(id, piChild(st), nil)
		}
		return toolTextResponse(id, fmt.Sprintf("closed Pi subagent %s and its own subagents: its process stopped; pi.send resumes the same conversation if its session file still exists, and pi.transcript still reads it.\n", child), nil)
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
		"pi.spawn":      "Spawn a Pi subagent of your own: a headless Pi coding agent the dashboard daemon runs for you, working in cwd. profile names any resolvable non-lead Pi agent profile; omitted means pi-worker (coding and ws git tools), while pi-mini-lead adds bounded ticket workflow tools. The child's resolved ws_tools, tools, and mcp_servers must be within your launched profile's tools; escalation is refused naming both profiles and the excess tools. prompt is its first message; a rendered playbook prompt works as it does for a native subagent. tier picks the model from the pi bucket of agents.tier alone and may not be above your own tier. The spawn tree is at most two levels below the lead, so a subagent of yours gets a refusal naming the cap when it tries to spawn. Returns the subagent's id at once while its turn runs. Each time a turn ends or fails, its report (the id, the status, and the final assistant message) comes to you: steered into your running turn, or as your next message when you are idle. Your own report to your parent waits until every open subagent's report has reached you, so you may end your turn while they work. Steer it with pi.send, read it with pi.transcript, list yours with pi.list, and close one with pi.stop.",
		"pi.send":       "Send one of your Pi subagents a follow-up message. A closed subagent resumes on its existing session file with the same id and settings; a missing file is refused. With no turn running it starts the next turn now; during a turn it steers that running turn at the next tool boundary, without cancelling in-flight tool calls. That turn's report covers the steered message; no extra turn is queued after it is taken.",
		"pi.list":       "List the Pi subagents you spawned, open and closed: id, status (idle, running, or closed), model and effort, working directory, label, held messages, and the last error.",
		"pi.stop":       "Close one of your Pi subagents and, first, its own subagents: its process stops and a running turn ends with it. No report follows. Its record stays: pi.list shows it closed, pi.transcript still reads it, and pi.send resumes it if its session file still exists.",
		"pi.transcript": "Read one of your Pi subagents' conversation, oldest first: the messages it was sent, its replies, its tool calls by tool, and its turn ends. Works for a closed subagent too.",
		"pi.wait":       fmt.Sprintf("Use only when you must have your own child's result before continuing; otherwise end your turn and let its report arrive. Block until the turn of one of your Pi subagents that took your last pi.send (or pi.spawn's prompt) ends or fails, and return its status and final assistant message; it never returns an earlier turn's message, and a turn that already ended returns at once. The answer is that turn's report: it is not also steered into your turn. A turn whose report already reached you before the wait returns its status alone, saying so. timeout_s defaults to %d and is capped at %d; when it passes first the status is running, and the report comes to you as usual.", int(piWaitDefault/time.Second), int(piWaitMax/time.Second)),
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
