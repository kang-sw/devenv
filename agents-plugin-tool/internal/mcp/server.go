package mcp

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/execjob"
	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdoc"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsprompt"
	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wsstore"
)

type Server struct {
	root                  string
	version               string
	sourceCommit          string
	role                  toolRole
	api                   apiRuntime
	rootMu                sync.RWMutex
	sessionRoot           string
	sessionHarness        string
	sessionActorID        string
	sessionActorAuthority string
	sessions              *sessionRegistry
}

type toolRole string

const (
	roleLead     toolRole = "lead"
	roleDelegate toolRole = "delegate"
	roleLeaf     toolRole = "leaf"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type cancelledNotificationParams struct {
	RequestID json.RawMessage `json:"requestId"`
	Reason    string          `json:"reason,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const ProtocolVersion = "2025-03-26"

const maxDebugEvents = 256

const leadWorkflowBootstrapMethod = "lead-workflow-bootstrap"

const (
	envNoAgent   = "WS_MCP_NO_AGENT"
	envNamespace = "WS_MCP_NAMESPACE"
)

var debugEvents = struct {
	sync.Mutex
	events []map[string]any
}{}

func NewServer(root, version string, sourceCommit ...string) *Server {
	commit := "dev"
	if len(sourceCommit) > 0 && sourceCommit[0] != "" {
		commit = sourceCommit[0]
	}
	cleanRoot := filepath.Clean(root)
	role := requestedToolRole()
	return &Server{root: cleanRoot, version: version, sourceCommit: commit, role: role, sessions: newSessionRegistry()}
}

func (s *Server) ServeStdio(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	encoder := json.NewEncoder(out)
	var writeMu sync.Mutex
	var wg sync.WaitGroup
	var requests sync.Map

	writeResponse := func(resp response) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return encoder.Encode(resp)
	}

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		default:
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			appendDebugEvent("parse_error", map[string]any{"error": err.Error()})
			if err := writeResponse(errorResponse(nil, -32700, "parse error")); err != nil {
				return err
			}
			continue
		}
		if len(req.ID) == 0 {
			s.handleNotification(req, &requests)
			continue
		}
		appendDebugEvent("request.received", map[string]any{"id": rawMessageString(req.ID), "method": req.Method})
		reqCtx, cancel := context.WithCancel(ctx)
		id := rawMessageString(req.ID)
		requests.Store(id, cancel)
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer cancel()
			defer requests.Delete(id)
			if err := writeResponse(s.handle(reqCtx, req)); err != nil {
				appendDebugEvent("response.write_error", map[string]any{"id": id, "error": err.Error()})
			}
		}()
	}
	err := scanner.Err()
	wg.Wait()
	return err
}

func (s *Server) handleNotification(req request, requests *sync.Map) {
	switch req.Method {
	case "notifications/cancelled":
		var params cancelledNotificationParams
		fields := map[string]any{"method": req.Method}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			fields["error"] = err.Error()
		} else {
			requestID := rawMessageString(params.RequestID)
			fields["request_id"] = requestID
			if params.Reason != "" {
				fields["reason"] = params.Reason
			}
			if cancelValue, ok := requests.Load(requestID); ok {
				if cancel, ok := cancelValue.(context.CancelFunc); ok {
					cancel()
					fields["matched"] = true
				}
			}
		}
		appendDebugEvent("notification.cancelled", fields)
	default:
		appendDebugEvent("notification.ignored", map[string]any{"method": req.Method})
	}
}

func (s *Server) handle(ctx context.Context, req request) response {
	switch req.Method {
	case "initialize":
		s.observeHarness("initialize", detectHarnessFromRaw(req.Params))
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"protocolVersion": ProtocolVersion,
			"serverInfo": map[string]string{
				"name":    "ws-mcp",
				"version": s.version,
			},
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
		}}
	case "tools/list":
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": s.filteredTools()}}
	case "tools/call":
		return s.callTool(ctx, req)
	default:
		return errorResponse(req.ID, -32601, "method not found")
	}
}

func appendDebugEvent(event string, fields map[string]any) {
	record := map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"event": event,
	}
	for key, value := range fields {
		record[key] = value
	}
	debugEvents.Lock()
	debugEvents.events = append(debugEvents.events, record)
	if len(debugEvents.events) > maxDebugEvents {
		debugEvents.events = append([]map[string]any(nil), debugEvents.events[len(debugEvents.events)-maxDebugEvents:]...)
	}
	debugEvents.Unlock()

	path := strings.TrimSpace(os.Getenv("WS_MCP_DEBUG_LOG"))
	if path == "" {
		return
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.Write(append(raw, '\n'))
}

func recentDebugEvents(limit int) []map[string]any {
	if limit <= 0 || limit > maxDebugEvents {
		limit = maxDebugEvents
	}
	debugEvents.Lock()
	defer debugEvents.Unlock()
	start := len(debugEvents.events) - limit
	if start < 0 {
		start = 0
	}
	out := make([]map[string]any, len(debugEvents.events[start:]))
	copy(out, debugEvents.events[start:])
	return out
}

func debugEventsJSONL(limit int) (string, error) {
	var b strings.Builder
	for _, event := range recentDebugEvents(limit) {
		raw, err := json.Marshal(event)
		if err != nil {
			return "", err
		}
		b.Write(raw)
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func rawMessageString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	return string(raw)
}

func NoAgentMode() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(envNoAgent)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func RuntimeNamespace() string {
	value := strings.TrimSpace(os.Getenv(envNamespace))
	if value == "" {
		return "ws"
	}
	return value
}

func (s *Server) callTool(ctx context.Context, req request) response {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
		Meta      map[string]any `json:"_meta"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "invalid params")
	}
	if params.Arguments == nil {
		params.Arguments = map[string]any{}
	}
	s.observeHarness("tools.call.meta", detectHarnessFromMeta(params.Meta))
	if NoAgentMode() && noAgentHiddenTool(params.Name) {
		return errorResponse(req.ID, -32601, fmt.Sprintf("%s agentless mode disables agent-backed tool: %s", RuntimeNamespace(), params.Name))
	}
	if !NoAgentMode() && wsflowOnlyTool(params.Name) {
		return errorResponse(req.ID, -32601, fmt.Sprintf("%s: tool not available in full ws mode: %s", RuntimeNamespace(), params.Name))
	}
	if !s.toolAllowed(params.Name) {
		return errorResponse(req.ID, -32601, fmt.Sprintf("tool not available in current %s MCP profile: %s", RuntimeNamespace(), params.Name))
	}
	if !s.subqueryAgentAccessAllowed(params.Name, params.Arguments) {
		return errorResponse(req.ID, -32601, fmt.Sprintf("tool available only for subquery-* agents in current %s MCP profile: %s", RuntimeNamespace(), params.Name))
	}
	// Keyed capability gate: when a session_key is present and maps to a known
	// non-lead scope, enforce roleAllowsTool for this call. Unknown session keys
	// are not rejected here; root-aware tools surface the unknown_session error
	// via resolveToolRoot. Tools that do not call resolveToolRoot (e.g.
	// runtime.info) silently ignore an unrecognised session_key.
	//
	// ws.lead.* tools are additionally blocked for any non-lead scoped key to
	// prevent self-login escalation: a delegate or leaf key must not be able to
	// call ws.lead.login and receive a lead-scoped key, bypassing all capability
	// restrictions. A KEYLESS caller (no session_key) is unaffected — the normal
	// lead bootstrap path remains open.
	if keyStr, ok := params.Arguments["session_key"].(string); ok && strings.TrimSpace(keyStr) != "" {
		if entry, found := s.sessions.lookup(keyStr); found && entry.scope != roleLead {
			if strings.HasPrefix(params.Name, "ws.lead.") || !roleAllowsTool(entry.scope, params.Name) {
				return errorResponse(req.ID, -32601, fmt.Sprintf("tool not available in current %s MCP profile: %s", RuntimeNamespace(), params.Name))
			}
		}
	}

	switch params.Name {
	case "runtime.info":
		info, err := runtimeInfo(s.version, s.sourceCommit)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, info, nil)
		}
		return toolTextResponse(req.ID, formatRuntimeInfo(info), nil)
	case "runtime.debug_events":
		text, err := debugEventsJSONL(intFromArgument(params.Arguments["limit"], 80))
		return toolTextResponse(req.ID, text, err)
	case "session.set_default_root":
		return toolTextResponse(req.ID, "", fmt.Errorf("session default roots were removed; call ws.lead.login(root) and pass session_key"))
	case "session.get_default_root":
		return toolTextResponse(req.ID, "", fmt.Errorf("session default roots were removed; call ws.lead.login(root) and pass session_key"))
	case "ws.lead.login":
		return s.handleLeadLogin(req.ID, params.Arguments)
	case "api.list":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		domains, err := apiListDomains(root)
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, domains, err)
		}
		return toolTextResponse(req.ID, formatStringLines(domains), err)
	case "api.ask":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		prompt, _ := params.Arguments["prompt"].(string)
		hint, _ := params.Arguments["domain_hint"].(string)
		text, err := s.askAPI(ctx, root, prompt, hint)
		if err != nil && text != "" {
			return toolErrorTextResponse(req.ID, text+"\n"+err.Error())
		}
		return toolTextResponse(req.ID, text, err)
	case "api.ask_async":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		prompt, _ := params.Arguments["prompt"].(string)
		hint, _ := params.Arguments["domain_hint"].(string)
		result, err := s.startAPIJob(ctx, root, prompt, hint)
		return toolJSONResponse(req.ID, result, err)
	case "api.status":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		key, _ := params.Arguments["api_job_key"].(string)
		result, err := s.statusAPIJob(ctx, root, key)
		return toolJSONResponse(req.ID, result, err)
	case "api.result":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		key, _ := params.Arguments["api_job_key"].(string)
		text, err := s.resultAPIJob(ctx, root, key)
		if err != nil && text != "" {
			return toolErrorTextResponse(req.ID, text+"\n"+err.Error())
		}
		return toolTextResponse(req.ID, text, err)
	case "api.cancel":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		key, _ := params.Arguments["api_job_key"].(string)
		result, err := s.cancelAPIJob(ctx, root, key)
		return toolJSONResponse(req.ID, result, err)

	case "exec.spawn":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		cmd, _ := params.Arguments["cmd"].(string)
		workingDir, _ := params.Arguments["working_dir"].(string)
		stdin, _ := params.Arguments["stdin"].(string)
		result, err := execjob.Launch(execjob.LaunchOptions{Root: root, WorkingDir: workingDir, Cmd: cmd, Args: stringList(params.Arguments["args"]), Env: stringMapArgument(params.Arguments["env"]), Stdin: stdin})
		return execResponse(req.ID, result, err, true)
	case "exec.shell":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		command, _ := params.Arguments["command"].(string)
		workingDir, _ := params.Arguments["working_dir"].(string)
		stdin, _ := params.Arguments["stdin"].(string)
		shell, _ := params.Arguments["shell"].(string)
		result, err := execjob.Launch(execjob.LaunchOptions{Root: root, WorkingDir: workingDir, Command: command, Shell: shell, Env: stringMapArgument(params.Arguments["env"]), Stdin: stdin, ShellMode: true})
		return execResponse(req.ID, result, err, true)
	case "exec.status":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		key, _ := params.Arguments["exec_key"].(string)
		result, err := execjob.Status(root, key)
		return execResponse(req.ID, result, err, false)
	case "exec.result":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		key, _ := params.Arguments["exec_key"].(string)
		timeout := durationFromSeconds(params.Arguments["timeout_seconds"])
		result, err := execjob.ResultWithTimeout(root, key, timeout)
		return execResponse(req.ID, result, err, true)
	case "exec.abort":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		key, _ := params.Arguments["exec_key"].(string)
		result, err := execjob.Abort(root, key)
		return execResponse(req.ID, result, err, false)
	case "exec.raw.tail":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		key, _ := params.Arguments["exec_key"].(string)
		stream, _ := params.Arguments["stream"].(string)
		result, err := execjob.Tail(root, key, stream, intFromArgument(params.Arguments["lines"], 0))
		return execRawTailResponse(req.ID, result, err)
	case "exec.raw.read":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		key, _ := params.Arguments["exec_key"].(string)
		stream, _ := params.Arguments["stream"].(string)
		result, err := execjob.Read(root, key, stream, int64FromArgument(params.Arguments["offset"], 0), int64FromArgument(params.Arguments["limit"], 0))
		return execRawReadResponse(req.ID, result, err)
	case "exec.raw.grep":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		key, _ := params.Arguments["exec_key"].(string)
		stream, _ := params.Arguments["stream"].(string)
		pattern, _ := params.Arguments["pattern"].(string)
		result, err := execjob.Grep(root, key, stream, pattern, intFromArgument(params.Arguments["before"], 0), intFromArgument(params.Arguments["after"], 0), intFromArgument(params.Arguments["max_matches"], 0), boolArgument(params.Arguments["regex"]))
		return execRawGrepResponse(req.ID, result, err)
	case "config.show":
		view, err := wsconfig.Show(wsconfig.Options{})
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, view, err)
		}
		return toolTextResponse(req.ID, formatConfigView(view), err)
	case "config.agents_tier":
		tier, _ := params.Arguments["tier"].(string)
		backend, _ := params.Arguments["backend"].(string)
		model, _ := params.Arguments["model"].(string)
		harness, _ := params.Arguments["harness"].(string)
		if strings.TrimSpace(harness) == "" {
			harness = s.currentHarness()
		}
		var cfg wsconfig.Config
		var err error
		if effort, ok := params.Arguments["effort"].(string); ok {
			cfg, err = wsconfig.SetAgentsTierForHarness(wsconfig.Options{}, tier, backend, model, harness, effort)
		} else {
			cfg, err = wsconfig.SetAgentsTierForHarness(wsconfig.Options{}, tier, backend, model, harness)
		}
		return toolJSONResponse(req.ID, cfg, err)

	case "git.status":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		result, err := wsgit.NewClient().Status(context.Background(), root)
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, err)
		}
		return toolTextResponse(req.ID, formatGitStatus(result), err)
	case "git.diff":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		rangeValue, _ := params.Arguments["range"].(string)
		mode, _ := params.Arguments["mode"].(string)
		result, err := wsgit.NewClient().Diff(context.Background(), root, wsgit.DiffOptions{Range: rangeValue, Mode: mode, Paths: stringList(params.Arguments["paths"])})
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, err)
		}
		return toolTextResponse(req.ID, result.Output, err)
	case "git.log":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		rangeValue, _ := params.Arguments["range"].(string)
		includeBody, _ := params.Arguments["include_body"].(bool)
		result, err := wsgit.NewClient().Log(context.Background(), root, wsgit.LogOptions{Range: rangeValue, Limit: intFromArgument(params.Arguments["limit"], 20), IncludeBody: includeBody})
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, err)
		}
		return toolTextResponse(req.ID, formatGitLog(result), err)
	case "git.merge_base":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		base, _ := params.Arguments["base"].(string)
		head, _ := params.Arguments["head"].(string)
		result, err := wsgit.NewClient().MergeBase(context.Background(), root, base, head)
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, err)
		}
		return toolTextResponse(req.ID, formatMergeBase(result), err)
	case "git.commit":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		title, _ := params.Arguments["title"].(string)
		description, _ := params.Arguments["description"].(string)
		result, err := wsgit.NewClient().Commit(context.Background(), root, wsgit.CommitOptions{
			Paths:               stringList(params.Arguments["paths"]),
			Title:               title,
			Description:         description,
			AIContext:           stringList(params.Arguments["ai_context"]),
			MentalModelNotes:    stringList(params.Arguments["mental_model_notes"]),
			UpdatedTickets:      stringList(params.Arguments["updated_tickets"]),
			UpdatedSpecs:        stringList(params.Arguments["updated_specs"]),
			UpdatedMentalModels: stringList(params.Arguments["updated_mental_models"]),
		})
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, err)
		}
		return toolTextResponse(req.ID, formatGitCommit(result), err)
	case "project_tree":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		text, err := wsdoc.ProjectTree(root)
		return toolTextResponse(req.ID, text, err)
	case "infra.read":
		name, _ := params.Arguments["name"].(string)
		text, err := wsdoc.ReadInfra(name)
		return toolTextResponse(req.ID, text, err)
	case "convention.read":
		name, _ := params.Arguments["name"].(string)
		text, err := wsdoc.ReadConvention(name)
		return toolTextResponse(req.ID, text, err)
	case "spec_stem.generate":
		slug, _ := params.Arguments["slug"].(string)
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		stem, err := wsdoc.GenerateSpecStem(root, slug, time.Now())
		return toolTextResponse(req.ID, stem+"\n", err)
	case "spec_index.verify":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		text, err := wsdoc.VerifySpecIndex(root)
		return toolTextResponse(req.ID, text, err)
	case "specs.list":
		if hasTicketStemArgument(params.Arguments) {
			return toolTextResponse(req.ID, "", fmt.Errorf("specs.list does not accept ticket_stem parameters"))
		}
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		result, err := wsdoc.SpecsList(root)
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, err)
		}
		return toolTextResponse(req.ID, formatSpecs(result), err)
	case "specs.find":
		if _, ok := params.Arguments["mentions_ticket_stem"]; ok {
			return toolTextResponse(req.ID, "", fmt.Errorf("specs.find uses ticket_stem, not mentions_ticket_stem"))
		}
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		query, _ := params.Arguments["query"].(string)
		specStem, _ := params.Arguments["spec_stem"].(string)
		ticketStem, _ := params.Arguments["ticket_stem"].(string)
		result, err := wsdoc.SpecsFind(root, wsdoc.SpecFindOptions{Query: query, SpecStem: specStem, TicketStem: ticketStem})
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, err)
		}
		if strings.TrimSpace(query) != "" {
			return toolTextResponse(req.ID, formatSpecFind(query, result), err)
		}
		return toolTextResponse(req.ID, formatSpecs(result), err)
	case "specs.status":
		if hasTicketStemArgument(params.Arguments) {
			return toolTextResponse(req.ID, "", fmt.Errorf("specs.status uses spec_stem"))
		}
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		specStem, _ := params.Arguments["spec_stem"].(string)
		result, err := wsdoc.SpecsStatus(root, wsdoc.SpecStatusOptions{SpecStem: specStem})
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, err)
		}
		return toolTextResponse(req.ID, formatSpecStatus(result), err)
	case "mental_models.list":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		text, err := wsdoc.MentalModelsList(root)
		return toolTextResponse(req.ID, text, err)
	case "mental_models.find":
		if hasTicketStemArgument(params.Arguments) {
			return toolTextResponse(req.ID, "", fmt.Errorf("mental_models.find uses spec_stem, not ticket_stem"))
		}
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		query, _ := params.Arguments["query"].(string)
		specStem, _ := params.Arguments["spec_stem"].(string)
		domain, _ := params.Arguments["domain"].(string)
		result, err := wsdoc.MentalModelsFind(root, wsdoc.MentalModelFindOptions{Query: query, SpecStem: specStem, Domain: domain})
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, err)
		}
		if strings.TrimSpace(query) != "" {
			return toolTextResponse(req.ID, formatMentalModelFind(query, result), err)
		}
		return toolTextResponse(req.ID, formatMentalModels(result), err)
	case "mental_models.status":
		if hasSpecStemArgument(params.Arguments) {
			return toolTextResponse(req.ID, "", fmt.Errorf("mental_models.status uses domain or path"))
		}
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		domain, _ := params.Arguments["domain"].(string)
		path, _ := params.Arguments["path"].(string)
		result, err := wsdoc.MentalModelsStatus(root, wsdoc.MentalModelStatusOptions{Domain: domain, Path: path})
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, err)
		}
		return toolTextResponse(req.ID, formatMentalModels(result), err)
	case "references.trace":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		ticketStem, _ := params.Arguments["ticket_stem"].(string)
		specStem, _ := params.Arguments["spec_stem"].(string)
		result, err := wsdoc.ReferencesTrace(root, wsdoc.ReferenceTraceOptions{TicketStem: ticketStem, SpecStem: specStem})
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, err)
		}
		return toolTextResponse(req.ID, formatReferenceTrace(result), err)
	case "tickets.list":
		if hasSpecStemArgument(params.Arguments) {
			return toolTextResponse(req.ID, "", fmt.Errorf("tickets tools use ticket_stem, not spec_stem"))
		}
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		result, err := wsdoc.TicketsList(root, wsdoc.TicketListOptions{
			Statuses:       stringList(params.Arguments["statuses"]),
			IncludeDone:    boolArgument(params.Arguments["include_done"]),
			IncludeDropped: boolArgument(params.Arguments["include_dropped"]),
		})
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, err)
		}
		return toolTextResponse(req.ID, formatTickets(result), err)
	case "tickets.find":
		if hasSpecStemArgument(params.Arguments) {
			return toolTextResponse(req.ID, "", fmt.Errorf("tickets tools use ticket_stem, not spec_stem"))
		}
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		query, _ := params.Arguments["query"].(string)
		ticketStem, _ := params.Arguments["ticket_stem"].(string)
		mentionsTicketStem, _ := params.Arguments["mentions_ticket_stem"].(string)
		result, err := wsdoc.TicketsFind(root, wsdoc.TicketFindOptions{
			Statuses:           stringList(params.Arguments["statuses"]),
			IncludeDone:        boolArgument(params.Arguments["include_done"]),
			IncludeDropped:     boolArgument(params.Arguments["include_dropped"]),
			Query:              query,
			TicketStem:         ticketStem,
			MentionsTicketStem: mentionsTicketStem,
		})
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, err)
		}
		return toolTextResponse(req.ID, formatTickets(result), err)
	case "tickets.status":
		if hasSpecStemArgument(params.Arguments) {
			return toolTextResponse(req.ID, "", fmt.Errorf("tickets tools use ticket_stem, not spec_stem"))
		}
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		ticketStem, _ := params.Arguments["ticket_stem"].(string)
		result, err := wsdoc.TicketsStatus(root, wsdoc.TicketStatusOptions{
			TicketStem:     ticketStem,
			IncludeDone:    boolArgument(params.Arguments["include_done"]),
			IncludeDropped: boolArgument(params.Arguments["include_dropped"]),
		})
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, err)
		}
		if result == nil {
			return toolTextResponse(req.ID, "", err)
		}
		return toolTextResponse(req.ID, formatTickets([]wsdoc.TicketInfo{*result}), err)
	case "subquery":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		question, _ := params.Arguments["question"].(string)
		if question == "" {
			question, _ = params.Arguments["prompt"].(string)
		}
		deepResearch, _ := params.Arguments["deep_research"].(bool)
		actorID := s.actorScopeForAgentTool(root, params.Arguments)
		child, err := s.childActorSetupForSubquery(ctx, root, actorID)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		text, err := wsagent.NewManager(wsagent.Options{}).Subquery(wsagent.SubqueryOptions{
			Root:                  root,
			ActorID:               actorID,
			Question:              question,
			DeepResearch:          deepResearch,
			Harness:               s.currentHarness(),
			ChildActorID:          child.ActorID,
			ChildActorAuthority:   child.Authority,
			ChildSetupInstruction: child.Instruction,
		})
		return toolTextResponse(req.ID, text, err)
	case "path.generate":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		kind, _ := params.Arguments["kind"].(string)
		generated, err := wsstate.NewManager(wsstate.Options{}).GeneratePaths(root, kind, stringList(params.Arguments["stems"]))
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		text := ""
		for _, path := range generated {
			text += path.Path + "\n"
		}
		return toolTextResponse(req.ID, text, nil)
	case "prompt.render":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		stem, _ := params.Arguments["stem"].(string)
		promptPath, err := renderPrompt(root, stem, stringMapArgument(params.Arguments["context"]))
		return toolTextResponse(req.ID, promptPath+"\n", err)

	case "playbook.print":
		// Phase 2: name + context; rsrc root is call-site-overridable seam for M3.
		// Argument parsing is named/extensible (not positional) for forward-compat
		// with M3's session_key prepend.
		name, _ := params.Arguments["name"].(string)
		callerContext := stringMapArgument(params.Arguments["context"])
		// M3 forward-compat: rsrc root resolved here so M3 can pass root_override.
		rsrcRoot, err := resolveRsrcRoot("")
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		body, err := printPlaybook(s, rsrcRoot, name, callerContext, wsconfig.Options{})
		return toolTextResponse(req.ID, body+"\n", err)

	case "playbook.render":
		// Phase 2: name + context; worktree root for tmp file; rsrc root overridable.
		name, _ := params.Arguments["name"].(string)
		callerContext := stringMapArgument(params.Arguments["context"])
		worktreeRoot, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		// M3 forward-compat: rsrc root resolved here so M3 can pass root_override.
		rsrcRoot, err := resolveRsrcRoot("")
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		path, err := renderPlaybook(s, rsrcRoot, worktreeRoot, name, callerContext, wsconfig.Options{})
		return toolTextResponse(req.ID, path+"\n", err)

	case "agents.register":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		actorID := s.actorScopeForAgentTool(root, params.Arguments)
		name, _ := params.Arguments["name"].(string)
		backend, _ := params.Arguments["backend"].(string)
		tier, _ := params.Arguments["tier"].(string)
		model, _ := params.Arguments["model"].(string)
		systemPromptText, _ := params.Arguments["system_prompt_text"].(string)
		child, err := s.childActorSetupForAgent(ctx, root, name, actorID, false)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		agent, _, err := wsagent.NewManager(wsagent.Options{}).Register(wsagent.RegisterOptions{
			Root:                  root,
			ActorID:               actorID,
			Name:                  name,
			Backend:               backend,
			Harness:               s.currentHarness(),
			Tier:                  tier,
			Model:                 model,
			Prompts:               stringList(params.Arguments["prompts"]),
			PromptRefs:            stringList(params.Arguments["prompt_refs"]),
			SystemPromptText:      systemPromptText,
			ChildActorID:          child.ActorID,
			ChildActorAuthority:   child.Authority,
			ChildSetupInstruction: child.Instruction,
		})
		return toolTextResponse(req.ID, agent.Name+"\n", err)
	case "agents.call":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		actorID := s.actorScopeForAgentTool(root, params.Arguments)
		name, _ := params.Arguments["name"].(string)
		prompt, _ := params.Arguments["prompt"].(string)
		child, err := s.childActorSetupForAgent(ctx, root, name, actorID, true)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		result, err := wsagent.NewManager(wsagent.Options{}).Call(wsagent.CallOptions{
			Root:                  root,
			ActorID:               actorID,
			Name:                  name,
			Prompt:                prompt,
			ChildActorID:          child.ActorID,
			ChildActorAuthority:   child.Authority,
			ChildSetupInstruction: child.Instruction,
		})
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		return toolTextResponse(req.ID, fmt.Sprintf("%s\t%s\tpid=%d\nfollow_up: agents.result --timeout 10m | agents.wait --timeout 10m | agents.status | agents.tail | agents.cancel\n", result.AgentName, result.Status, result.PID), nil)
	case "agents.wait":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		actorID := s.actorScopeForAgentTool(root, params.Arguments)
		name, _ := params.Arguments["name"].(string)
		names := stringList(params.Arguments["names"])
		text, err := wsagent.NewManager(wsagent.Options{}).Wait(wsagent.WaitOptions{
			Root:    root,
			ActorID: actorID,
			Name:    name,
			Names:   names,
			Timeout: durationFromSeconds(params.Arguments["timeout_seconds"]),
			Context: ctx,
		})
		return toolTextResponse(req.ID, text, err)
	case "agents.result":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		actorID := s.actorScopeForAgentTool(root, params.Arguments)
		name, _ := params.Arguments["name"].(string)
		text, err := wsagent.NewManager(wsagent.Options{}).Result(wsagent.ResultOptions{
			Root:    root,
			ActorID: actorID,
			Name:    name,
			Timeout: durationFromSeconds(params.Arguments["timeout_seconds"]),
			Context: ctx,
			OnEphemeralErased: func(agent wsagent.Agent) {
				s.markActorInactive(context.Background(), root, agent.ChildActorID)
			},
		})
		return toolTextResponse(req.ID, text, err)
	case "agents.status":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		actorID := s.actorScopeForAgentTool(root, params.Arguments)
		name, _ := params.Arguments["name"].(string)
		text, err := wsagent.NewManager(wsagent.Options{}).StatusScoped(root, name, actorID)
		return toolTextResponse(req.ID, text, err)
	case "agents.interrupt":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		actorID := s.actorScopeForAgentTool(root, params.Arguments)
		name, _ := params.Arguments["name"].(string)
		message, _ := params.Arguments["message"].(string)
		result, err := wsagent.NewManager(wsagent.Options{}).Interrupt(wsagent.InterruptOptions{
			Root:    root,
			ActorID: actorID,
			Name:    name,
			Message: message,
		})
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		return toolTextResponse(req.ID, fmt.Sprintf("%s\tqueued\tmessage=%s\n", result.AgentName, result.MessageID), nil)
	case "agents.tail":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		actorID := s.actorScopeForAgentTool(root, params.Arguments)
		name, _ := params.Arguments["name"].(string)
		lines := intFromArgument(params.Arguments["lines"], 40)
		text, err := wsagent.NewManager(wsagent.Options{}).Tail(wsagent.TailOptions{
			Root:    root,
			ActorID: actorID,
			Name:    name,
			Lines:   lines,
		})
		return toolTextResponse(req.ID, text, err)
	case "agents.debug.tail":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		actorID := s.actorScopeForAgentTool(root, params.Arguments)
		name, _ := params.Arguments["name"].(string)
		lines := intFromArgument(params.Arguments["lines"], 40)
		text, err := wsagent.NewManager(wsagent.Options{}).Tail(wsagent.TailOptions{
			Root:    root,
			ActorID: actorID,
			Name:    name,
			Lines:   lines,
			Raw:     true,
		})
		return toolTextResponse(req.ID, text, err)
	case "agents.debug.stdout", "agents.debug.stderr", "agents.debug.runtime_log", "agents.debug.events":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		actorID := s.actorScopeForAgentTool(root, params.Arguments)
		name, _ := params.Arguments["name"].(string)
		lines := intFromArgument(params.Arguments["lines"], 40)
		stream := strings.TrimPrefix(params.Name, "agents.debug.")
		text, err := wsagent.NewManager(wsagent.Options{}).DiagnosticStream(wsagent.DiagnosticStreamOptions{
			Root:    root,
			ActorID: actorID,
			Name:    name,
			Stream:  stream,
			Lines:   lines,
		})
		return toolTextResponse(req.ID, text, err)
	case "agents.cancel":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		actorID := s.actorScopeForAgentTool(root, params.Arguments)
		name, _ := params.Arguments["name"].(string)
		text, err := wsagent.NewManager(wsagent.Options{}).CancelScoped(root, name, actorID)
		return toolTextResponse(req.ID, text, err)
	case "agents.recall":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		actorID := s.actorScopeForAgentTool(root, params.Arguments)
		name, _ := params.Arguments["name"].(string)
		prompt, _ := params.Arguments["prompt"].(string)
		text, err := wsagent.NewManager(wsagent.Options{}).Recall(wsagent.RecallOptions{
			Root:    root,
			ActorID: actorID,
			Name:    name,
			Prompt:  prompt,
		})
		return toolTextResponse(req.ID, text, err)
	case "agents.print":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		actorID := s.actorScopeForAgentTool(root, params.Arguments)
		name, _ := params.Arguments["name"].(string)
		text, err := wsagent.NewManager(wsagent.Options{}).PrintScoped(root, name, actorID)
		return toolTextResponse(req.ID, text, err)
	case "agents.erase":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		actorID := s.actorScopeForAgentTool(root, params.Arguments)
		name, _ := params.Arguments["name"].(string)
		err = wsagent.NewManager(wsagent.Options{}).EraseScoped(root, name, actorID)
		return toolTextResponse(req.ID, "erased\n", err)
	default:
		return errorResponse(req.ID, -32602, fmt.Sprintf("unknown tool: %s", params.Name))
	}
}

func runtimeInfo(version, sourceCommit string) (map[string]any, error) {
	bundle, err := wsprompt.Bundle(sourceCommit)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"version":       version,
		"source_commit": sourceCommit,
		"prompt_bundle": bundle,
	}, nil
}

func wantsJSON(arguments map[string]any) bool {
	format, _ := arguments["format"].(string)
	return strings.EqualFold(strings.TrimSpace(format), "json")
}

func formatRuntimeInfo(info map[string]any) string {
	var b strings.Builder
	if version, _ := info["version"].(string); version != "" {
		fmt.Fprintf(&b, "version: %s\n", version)
	}
	if commit, _ := info["source_commit"].(string); commit != "" {
		fmt.Fprintf(&b, "source_commit: %s\n", commit)
	}
	if bundle, ok := info["prompt_bundle"].(wsprompt.BundleInfo); ok {
		formatPromptBundle(&b, bundle)
	} else if bundle, ok := info["prompt_bundle"].(map[string]any); ok {
		formatPromptBundleMap(&b, bundle)
	}
	return b.String()
}

func formatPromptBundle(b *strings.Builder, bundle wsprompt.BundleInfo) {
	fmt.Fprintf(b, "prompt_bundle: %d prompts", len(bundle.Prompts))
	if bundle.ContentSHA256 != "" {
		fmt.Fprintf(b, " sha256=%s", bundle.ContentSHA256)
	}
	if bundle.SourceCommit != "" {
		fmt.Fprintf(b, " source_commit=%s", bundle.SourceCommit)
	}
	b.WriteString("\n")
	if len(bundle.Prompts) > 0 {
		b.WriteString("prompts:\n")
		for _, prompt := range bundle.Prompts {
			fmt.Fprintf(b, "  - %s\n", prompt)
		}
	}
}

func formatPromptBundleMap(b *strings.Builder, bundle map[string]any) {
	prompts := stringAnySlice(bundle["prompts"])
	fmt.Fprintf(b, "prompt_bundle: %d prompts", len(prompts))
	if sha, _ := bundle["content_sha256"].(string); sha != "" {
		fmt.Fprintf(b, " sha256=%s", sha)
	}
	if commit, _ := bundle["source_commit"].(string); commit != "" {
		fmt.Fprintf(b, " source_commit=%s", commit)
	}
	b.WriteString("\n")
	if len(prompts) > 0 {
		b.WriteString("prompts:\n")
		for _, prompt := range prompts {
			fmt.Fprintf(b, "  - %s\n", prompt)
		}
	}
}

func stringAnySlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

func (s *Server) setupState(source string) map[string]any {
	s.rootMu.RLock()
	sessionRoot := s.sessionRoot
	sessionHarness := s.sessionHarness
	actorID := s.sessionActorID
	actorAuthority := s.sessionActorAuthority
	s.rootMu.RUnlock()
	return map[string]any{
		"root":                 sessionRoot,
		"has_root":             sessionRoot != "",
		"actor_id":             actorID,
		"has_actor":            actorID != "",
		"actor_authority":      actorAuthority,
		"recovery_guidance":    recoveryGuidance(actorID),
		"session_default_root": sessionRoot,
		"has_session_default":  sessionRoot != "",
		"session_harness":      sessionHarness,
		"env_project_root":     strings.TrimSpace(os.Getenv("WS_MCP_PROJECT_ROOT")),
		"server_root":          s.root,
		"source":               source,
	}
}

func formatSetupState(values map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "root: %s\n", displayString(values["root"]))
	fmt.Fprintf(&b, "has_root: %t\n", boolValue(values["has_root"]))
	fmt.Fprintf(&b, "actor_id: %s\n", displayString(values["actor_id"]))
	fmt.Fprintf(&b, "has_actor: %t\n", boolValue(values["has_actor"]))
	fmt.Fprintf(&b, "actor_authority: %s\n", displayString(values["actor_authority"]))
	if guidance := displayString(values["recovery_guidance"]); guidance != "" {
		fmt.Fprintf(&b, "recovery_guidance: %s\n", guidance)
	}
	fmt.Fprintf(&b, "session_harness: %s\n", displayString(values["session_harness"]))
	fmt.Fprintf(&b, "server_root: %s\n", displayString(values["server_root"]))
	fmt.Fprintf(&b, "env_project_root: %s\n", displayString(values["env_project_root"]))
	return b.String()
}

func recoveryGuidance(actorID string) string {
	if actorID == "" {
		return "Lead bootstrap requires ws.lead.login(root)."
	}
	return "Actor recovery was removed; call ws.lead.login(root) and pass session_key."
}

func (s *Server) applySetup(ctx context.Context, arguments map[string]any) error {
	if id, _ := arguments["id"].(string); strings.TrimSpace(id) != "" {
		return s.restoreActor(ctx, strings.TrimSpace(id))
	}
	method, _ := arguments["method"].(string)
	method = strings.TrimSpace(method)
	if method != "" {
		if method != leadWorkflowBootstrapMethod {
			return fmt.Errorf("unsupported setup method %q", method)
		}
		return s.bootstrapLeadActor(ctx, arguments)
	}
	if root, _ := arguments["root"].(string); strings.TrimSpace(root) != "" {
		canonical, err := canonicalSetupRoot(root)
		if err != nil {
			return err
		}
		s.rootMu.Lock()
		s.sessionRoot = canonical
		s.rootMu.Unlock()
	}
	return nil
}

func (s *Server) bootstrapLeadActor(ctx context.Context, arguments map[string]any) error {
	root, _ := arguments["root"].(string)
	root = strings.TrimSpace(root)
	if root == "" {
		return fmt.Errorf("root is required for setup method %q; pass the repository's absolute filesystem path", leadWorkflowBootstrapMethod)
	}
	if root == "<cwd>" {
		return fmt.Errorf("root for setup method %q must be an absolute repository path; the MCP server cannot infer the agent's current directory from %q", leadWorkflowBootstrapMethod, root)
	}
	if !filepath.IsAbs(root) {
		return fmt.Errorf("root for setup method %q must be an absolute repository path", leadWorkflowBootstrapMethod)
	}
	canonical, err := canonicalSetupRoot(root)
	if err != nil {
		return err
	}
	store, err := wsstore.NewManager(wsstore.Options{}).Open(canonical)
	if err != nil {
		return err
	}
	defer store.Close()
	worktreeKey := store.Layout().WorktreeKey
	actorID, err := mintUniqueActorID(ctx, store, "lead")
	if err != nil {
		return err
	}
	actor := wsstore.Actor{
		ActorID:     actorID,
		Authority:   "lead",
		RootPath:    canonical,
		WorktreeKey: worktreeKey,
		Status:      "active",
		Pinned:      true,
	}
	if err := store.UpsertActor(ctx, actor); err != nil {
		return err
	}
	s.bindActor(actor)
	return nil
}

func canonicalSetupRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "<cwd>" {
		return "", fmt.Errorf("root must be an absolute repository path; the MCP server cannot infer the agent's current directory from %q", root)
	}
	return canonicalGitRoot(root)
}

// handleLeadLogin implements the ws.lead.login tool: canonicalize root, mint an
// ephemeral session key, store the {root, scope} entry in the registry, and
// return the key to the caller. It does NOT participate in the ws.setup fence.
func (s *Server) handleLeadLogin(id json.RawMessage, arguments map[string]any) response {
	rootArg, _ := arguments["root"].(string)
	if strings.TrimSpace(rootArg) == "" {
		return toolTextResponse(id, "", fmt.Errorf("ws.lead.login: root is required"))
	}
	canonical, err := canonicalSetupRoot(rootArg)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	scope := parseCapabilityScope(arguments["capability"])
	key, err := s.sessions.mint(canonical, scope)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	result := map[string]any{
		"session_key": key,
		"root":        canonical,
	}
	if wantsJSON(arguments) {
		return toolJSONResponse(id, result, nil)
	}
	return toolTextResponse(id, fmt.Sprintf("session_key: %s\nroot: %s\n", key, canonical), nil)
}

// parseCapabilityScope maps the optional capability argument to a toolRole.
// An absent, nil, or "lead" capability maps to roleLead (unrestricted).
func parseCapabilityScope(raw any) toolRole {
	s, _ := raw.(string)
	switch strings.TrimSpace(s) {
	case "delegate":
		return roleDelegate
	case "leaf":
		return roleLeaf
	default:
		return roleLead
	}
}

func (s *Server) restoreActor(ctx context.Context, actorID string) error {
	actorID = strings.ToLower(strings.TrimSpace(actorID))
	if _, err := actorAuthority(actorID); err != nil {
		return err
	}
	manager := wsstore.NewManager(wsstore.Options{})
	var store *wsstore.Store
	var actor wsstore.Actor
	var ok bool
	if worktreeKey, err := actorWorktreeKey(actorID); err == nil {
		opened, err := manager.OpenWorktreeKey(worktreeKey)
		if err != nil {
			return err
		}
		store = opened
		defer store.Close()
		actor, ok, err = store.Actor(ctx, actorID)
		if err != nil {
			return err
		}
	} else {
		found, foundOK, err := manager.FindActor(ctx, actorID)
		if err != nil {
			return err
		}
		actor, ok = found, foundOK
		if ok {
			opened, err := manager.OpenWorktreeKey(actor.WorktreeKey)
			if err != nil {
				return err
			}
			store = opened
			defer store.Close()
		}
	}
	if !ok {
		return fmt.Errorf("actor id %q was not found; actor bootstrap was removed; call ws.lead.login(root)", actorID)
	}
	if actor.Status != "" && actor.Status != "active" {
		return fmt.Errorf("actor id %q is not active: %s", actorID, actor.Status)
	}
	if store == nil {
		return fmt.Errorf("actor id %q has no recoverable worktree state", actorID)
	}
	if err := store.UpsertActor(ctx, actor); err != nil {
		return err
	}
	s.bindActor(actor)
	return nil
}

func (s *Server) bindActor(actor wsstore.Actor) {
	s.rootMu.Lock()
	defer s.rootMu.Unlock()
	s.sessionRoot = actor.RootPath
	s.sessionActorID = actor.ActorID
	s.sessionActorAuthority = actor.Authority
}

var generateActorPayload = randomActorPayload

func mintUniqueActorID(ctx context.Context, store *wsstore.Store, authority string) (string, error) {
	if !validActorAuthority(authority) {
		return "", fmt.Errorf("invalid actor authority %q", authority)
	}
	manager := wsstore.NewManager(wsstore.Options{})
	for attempt := 0; attempt < 32; attempt++ {
		actorID, err := mintActorID(authority)
		if err != nil {
			return "", err
		}
		if _, ok, err := store.Actor(ctx, actorID); err != nil {
			return "", err
		} else if ok {
			continue
		}
		if _, ok, err := manager.FindActor(ctx, actorID); err != nil {
			return "", err
		} else if ok {
			continue
		}
		return actorID, nil
	}
	return "", fmt.Errorf("could not mint unique %s actor id after collision retries", authority)
}

func mintActorID(authority string) (string, error) {
	payload, err := generateActorPayload(8)
	if err != nil {
		return "", err
	}
	return authority + "-" + payload, nil
}

func randomActorPayload(length int) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	if length <= 0 {
		return "", nil
	}
	out := make([]byte, length)
	for i := range out {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		out[i] = alphabet[n.Int64()]
	}
	return string(out), nil
}

func actorAuthority(actorID string) (string, error) {
	authority, rest, ok := strings.Cut(strings.TrimSpace(actorID), "-")
	if !ok || rest == "" || !validActorAuthority(authority) || !validActorIDRest(rest) {
		return "", fmt.Errorf("invalid actor id %q", actorID)
	}
	return authority, nil
}

func validActorIDRest(value string) bool {
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '@' {
			continue
		}
		return false
	}
	return true
}

func actorWorktreeKey(actorID string) (string, error) {
	authority, rest, ok := strings.Cut(actorID, "-")
	if !ok || !validActorAuthority(authority) {
		return "", fmt.Errorf("invalid actor id %q", actorID)
	}
	idx := strings.LastIndex(rest, "-")
	if idx <= 0 || idx == len(rest)-1 {
		return "", fmt.Errorf("invalid actor id %q", actorID)
	}
	worktreeKey := rest[:idx]
	if _, err := wsstate.LayoutForWorktreeKey("", worktreeKey); err != nil {
		return "", fmt.Errorf("invalid actor id %q: %w", actorID, err)
	}
	return worktreeKey, nil
}

func validActorAuthority(value string) bool {
	switch value {
	case "lead", "delegate", "reader":
		return true
	default:
		return false
	}
}

func blankDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

type childActorSetup struct {
	ActorID     string
	Authority   string
	Instruction string
}

func (s *Server) actorScopeForAgentTool(root string, arguments map[string]any) string {
	if value, ok := arguments["root"].(string); ok && strings.TrimSpace(value) != "" {
		return ""
	}
	if !s.actorBoundToRoot(root) {
		return ""
	}
	return s.currentActorID()
}

func (s *Server) childActorSetupForAgent(ctx context.Context, root, name, actorID string, requireExisting bool) (childActorSetup, error) {
	if strings.TrimSpace(actorID) == "" {
		return childActorSetup{}, nil
	}
	if agent, err := wsagent.NewManager(wsagent.Options{}).AgentScoped(root, name, actorID); err == nil {
		if strings.TrimSpace(agent.ChildActorID) != "" {
			return s.ensureChildActor(ctx, root, strings.TrimSpace(agent.ChildActorID), blankDefault(agent.ChildActorAuthority, "delegate"))
		}
	} else if requireExisting {
		return childActorSetup{}, err
	}
	return s.ensureChildActor(ctx, root, "", "delegate")
}

func (s *Server) childActorSetupForSubquery(ctx context.Context, root, actorID string) (childActorSetup, error) {
	if strings.TrimSpace(actorID) == "" {
		return childActorSetup{}, nil
	}
	return s.ensureChildActor(ctx, root, "", "reader")
}

func (s *Server) actorBoundToRoot(root string) bool {
	s.rootMu.RLock()
	defer s.rootMu.RUnlock()
	return s.sessionActorID != "" && s.sessionRoot == root
}

func (s *Server) currentActorID() string {
	s.rootMu.RLock()
	defer s.rootMu.RUnlock()
	return s.sessionActorID
}

func (s *Server) ensureChildActor(ctx context.Context, root, actorID, authority string) (childActorSetup, error) {
	store, err := wsstore.NewManager(wsstore.Options{}).Open(root)
	if err != nil {
		return childActorSetup{}, err
	}
	defer store.Close()
	authority = blankDefault(authority, "delegate")
	if actorID == "" {
		actorID, err = mintUniqueActorID(ctx, store, authority)
		if err != nil {
			return childActorSetup{}, err
		}
	}
	actor := wsstore.Actor{
		ActorID:       actorID,
		Authority:     authority,
		RootPath:      root,
		WorktreeKey:   store.Layout().WorktreeKey,
		ParentActorID: s.currentActorID(),
		Status:        "active",
		Pinned:        authority == "delegate",
	}
	if err := store.UpsertActor(ctx, actor); err != nil {
		return childActorSetup{}, err
	}
	return childActorSetup{
		ActorID:     actorID,
		Authority:   authority,
		Instruction: childActorInstruction(actorID),
	}, nil
}

func childActorInstruction(actorID string) string {
	if strings.TrimSpace(actorID) == "" {
		return ""
	}
	return fmt.Sprintf("Before root-omitted ws MCP tool calls in this child process, call MCP tool `ws.setup` with `id: %q` to recover your assigned actor context.", actorID)
}

func (s *Server) markActorInactive(ctx context.Context, root, actorID string) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return
	}
	store, err := wsstore.NewManager(wsstore.Options{}).Open(root)
	if err != nil {
		appendDebugEvent("actor.inactive.error", map[string]any{"actor_id": actorID, "error": err.Error()})
		return
	}
	defer store.Close()
	actor, ok, err := store.Actor(ctx, actorID)
	if err != nil || !ok {
		fields := map[string]any{"actor_id": actorID}
		if err != nil {
			fields["error"] = err.Error()
		}
		appendDebugEvent("actor.inactive.missing", fields)
		return
	}
	actor.Status = "inactive"
	actor.Pinned = false
	if err := store.UpsertActor(ctx, actor); err != nil {
		appendDebugEvent("actor.inactive.error", map[string]any{"actor_id": actorID, "error": err.Error()})
	}
}

func (s *Server) actorGate(name string, arguments map[string]any) error {
	if !rootOmittedActorTool(name) {
		return nil
	}
	if value, ok := arguments["root"].(string); ok && strings.TrimSpace(value) != "" {
		return nil
	}
	s.rootMu.RLock()
	hasActor := s.sessionActorID != ""
	s.rootMu.RUnlock()
	if hasActor {
		return nil
	}
	return fmt.Errorf("session_key required before root-omitted %s; call ws.lead.login(root)", name)
}

func rootOmittedActorTool(name string) bool {
	switch name {
	case "agents.register", "agents.call", "subquery":
		return true
	default:
		return false
	}
}

func formatStringLines(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.Join(values, "\n") + "\n"
}

func formatConfigView(view wsconfig.View) string {
	var b strings.Builder
	fmt.Fprintf(&b, "path: %s\n", view.Path)
	aliases := []string{"light", "core", "deep"}
	b.WriteString("model_aliases:\n")
	for _, alias := range aliases {
		byHarness := view.Config.Agents.ModelAliases[alias]
		keys := sortedAgentTierKeys(byHarness)
		if len(keys) == 0 {
			continue
		}
		fmt.Fprintf(&b, "  %s:\n", alias)
		for _, key := range keys {
			tier := byHarness[key]
			fmt.Fprintf(&b, "    %s: %s/%s", key, displayOrDash(tier.Backend), displayOrDash(tier.Model))
			if tier.Effort != "" {
				fmt.Fprintf(&b, " effort=%s", tier.Effort)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func sortedAgentTierKeys(values map[string]wsconfig.AgentTier) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func formatGitStatus(result wsgit.StatusResult) string {
	var b strings.Builder
	head := displayOrDash(result.Branch.Head)
	oid := shortHash(result.Branch.OID)
	if oid != "" {
		fmt.Fprintf(&b, "%s %s", head, oid)
	} else {
		fmt.Fprintf(&b, "%s", head)
	}
	if result.Branch.Upstream != "" {
		fmt.Fprintf(&b, " %s", result.Branch.Upstream)
	}
	if result.Branch.Ahead != 0 || result.Branch.Behind != 0 {
		fmt.Fprintf(&b, " ahead=%d behind=%d", result.Branch.Ahead, result.Branch.Behind)
	}
	if result.Clean {
		b.WriteString(" clean\n")
		return b.String()
	}
	fmt.Fprintf(&b, " dirty: %d files\n", len(result.ChangedFiles))
	for _, file := range result.ChangedFiles {
		status := file.Status
		if status == "" {
			status = file.IndexStatus + file.WorktreeStatus
		}
		if status == "" {
			status = "??"
		}
		if file.OldPath != "" {
			fmt.Fprintf(&b, "%s %s <- %s\n", status, file.Path, file.OldPath)
		} else {
			fmt.Fprintf(&b, "%s %s\n", status, file.Path)
		}
	}
	return b.String()
}

func formatGitLog(result wsgit.LogResult) string {
	var b strings.Builder
	if result.Range != "" {
		fmt.Fprintf(&b, "range: %s\n", result.Range)
	}
	if len(result.Commits) == 0 {
		return b.String()
	}
	for i, commit := range result.Commits {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "commit %s\n", commit.Hash)
		if commit.Subject != "" {
			fmt.Fprintf(&b, "subject: %s\n", commit.Subject)
		}
		if commit.Author != "" {
			fmt.Fprintf(&b, "author: %s\n", commit.Author)
		}
		if commit.Date != "" {
			fmt.Fprintf(&b, "date: %s\n", commit.Date)
		}
		if strings.TrimSpace(commit.Body) != "" {
			b.WriteString("\n")
			b.WriteString(strings.TrimSpace(commit.Body))
			b.WriteString("\n")
		}
	}
	return b.String()
}

func formatMergeBase(result wsgit.MergeBaseResult) string {
	return fmt.Sprintf("merge_base: %s (%s %s)\n", result.MergeBase, result.Base, result.Head)
}

func formatGitCommit(result wsgit.CommitResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "commit: %s\n", result.Hash)
	fmt.Fprintf(&b, "title: %s\n", result.Title)
	if len(result.Paths) > 0 {
		b.WriteString("paths:\n")
		for _, path := range result.Paths {
			fmt.Fprintf(&b, "  - %s\n", path)
		}
	}
	if len(result.TicketChanges) > 0 {
		b.WriteString("ticket_changes:\n")
		for _, change := range result.TicketChanges {
			fmt.Fprintf(&b, "  - %s", change.Stem)
			if change.FromStatus != "" || change.ToStatus != "" {
				fmt.Fprintf(&b, " %s->%s", displayOrDash(change.FromStatus), displayOrDash(change.ToStatus))
			}
			if change.ResultAdded {
				b.WriteString(" result")
				if change.ResultHeading != "" {
					fmt.Fprintf(&b, " %q", change.ResultHeading)
				}
			}
			if change.Path != "" {
				fmt.Fprintf(&b, " (%s)", change.Path)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func formatSpecs(specs []wsdoc.SpecInfo) string {
	var b strings.Builder
	for _, spec := range specs {
		fmt.Fprintf(&b, "%s", spec.Path)
		if spec.Title != "" {
			fmt.Fprintf(&b, " - %s", spec.Title)
		}
		if spec.Summary != "" {
			fmt.Fprintf(&b, " # %s", spec.Summary)
		}
		flags := []string{}
		if spec.MatchesSpecStem {
			flags = append(flags, "matches_spec_stem")
		}
		if spec.MatchesTicketRef {
			flags = append(flags, "matches_ticket_ref")
		}
		if len(spec.Anchors) > 0 {
			flags = append(flags, fmt.Sprintf("anchors=%d", len(spec.Anchors)))
		}
		if len(spec.TicketRefs) > 0 {
			flags = append(flags, "tickets="+strings.Join(spec.TicketRefs, ","))
		}
		if len(flags) > 0 {
			fmt.Fprintf(&b, " [%s]", strings.Join(flags, " "))
		}
		b.WriteString("\n")
		writeIndentedLines(&b, "  snippet: ", spec.MatchingSnippets)
		writeIndentedLines(&b, "  marker: ", spec.MarkerContexts)
	}
	return b.String()
}

func formatSpecFind(query string, specs []wsdoc.SpecInfo) string {
	return formatDocumentFind(query, "spec", "specs", len(specs), func(writeDoc func(path string, score, hits int, matches []wsdoc.MatchEvidence)) {
		for _, spec := range specs {
			writeDoc(spec.Path, spec.MatchScore, len(spec.Matches), spec.Matches)
		}
	})
}

func formatMentalModelFind(query string, models []wsdoc.MentalModelInfo) string {
	return formatDocumentFind(query, "mental model", "mental models", len(models), func(writeDoc func(path string, score, hits int, matches []wsdoc.MatchEvidence)) {
		for _, model := range models {
			writeDoc(model.Path, model.MatchScore, len(model.Matches), model.Matches)
		}
	})
}

const (
	maxFindTextDocuments      = 10
	maxFindTextEvidencePerDoc = 3
)

func formatDocumentFind(query, singular, plural string, count int, each func(func(string, int, int, []wsdoc.MatchEvidence))) string {
	type doc struct {
		path    string
		score   int
		hits    int
		matches []wsdoc.MatchEvidence
	}
	docs := []doc{}
	each(func(path string, score, hits int, matches []wsdoc.MatchEvidence) {
		docs = append(docs, doc{path: path, score: score, hits: hits, matches: matches})
	})

	var b strings.Builder
	label := plural
	if count == 1 {
		label = singular
	}
	truncatedDocs := len(docs) > maxFindTextDocuments
	truncatedHits := false
	for _, d := range docs {
		if len(d.matches) > maxFindTextEvidencePerDoc {
			truncatedHits = true
			break
		}
	}
	fmt.Fprintf(&b, "%d candidate %s for query=%q", count, label, query)
	if truncatedDocs || truncatedHits {
		fmt.Fprintf(&b, " (showing subset: first %d documents, up to %d hits each)", maxFindTextDocuments, maxFindTextEvidencePerDoc)
	}
	b.WriteString("\n")
	if count == 0 {
		fmt.Fprintf(&b, "No candidates met the query threshold; retry with shorter noun phrases.\n")
		return b.String()
	}
	if len(docs) > maxFindTextDocuments {
		docs = docs[:maxFindTextDocuments]
	}
	for _, d := range docs {
		matches := selectFindTextEvidence(d.matches)
		fmt.Fprintf(&b, "\n%s\tscore=%d\thits=%d\n", d.path, d.score, d.hits)
		for _, match := range matches {
			fmt.Fprintf(&b, "  %d: %s\n", match.Line, match.Snippet)
		}
	}
	return b.String()
}

func selectFindTextEvidence(matches []wsdoc.MatchEvidence) []wsdoc.MatchEvidence {
	selected := append([]wsdoc.MatchEvidence(nil), matches...)
	if len(selected) > maxFindTextEvidencePerDoc {
		sort.SliceStable(selected, func(i, j int) bool {
			if len(selected[i].MatchedTerms) != len(selected[j].MatchedTerms) {
				return len(selected[i].MatchedTerms) > len(selected[j].MatchedTerms)
			}
			return selected[i].Line < selected[j].Line
		})
		selected = selected[:maxFindTextEvidencePerDoc]
	}
	sort.SliceStable(selected, func(i, j int) bool { return selected[i].Line < selected[j].Line })
	return selected
}

func formatSpecStatus(status *wsdoc.SpecAnchorStatus) string {
	if status == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "spec_stem: %s\n", status.SpecStem)
	if len(status.Locations) > 0 {
		b.WriteString("locations:\n")
		for _, loc := range status.Locations {
			fmt.Fprintf(&b, "  - line %d", loc.Line)
			if loc.Heading != "" {
				fmt.Fprintf(&b, " %s", loc.Heading)
			}
			if loc.MarkerContext != "" {
				fmt.Fprintf(&b, " # %s", loc.MarkerContext)
			}
			b.WriteString("\n")
		}
	}
	if len(status.Files) > 0 {
		b.WriteString("files:\n")
		for _, spec := range status.Files {
			fmt.Fprintf(&b, "  - %s", spec.Path)
			if spec.Title != "" {
				fmt.Fprintf(&b, " - %s", spec.Title)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func formatTickets(tickets []wsdoc.TicketInfo) string {
	var b strings.Builder
	for _, ticket := range tickets {
		fmt.Fprintf(&b, "[%s] %s", ticket.Status, ticket.Stem)
		if ticket.Title != "" {
			fmt.Fprintf(&b, " - %s", ticket.Title)
		}
		if ticket.Path != "" {
			fmt.Fprintf(&b, " (%s)", ticket.Path)
		}
		flags := []string{}
		if ticket.ResultPresent {
			flags = append(flags, "result")
		}
		if ticket.MentionsTicketStem {
			flags = append(flags, "mentions_ticket_stem")
		}
		if ticket.Parent != "" {
			flags = append(flags, "parent="+ticket.Parent)
		}
		if len(ticket.Specs) > 0 {
			flags = append(flags, "spec="+strings.Join(ticket.Specs, ","))
		}
		if len(flags) > 0 {
			fmt.Fprintf(&b, " [%s]", strings.Join(flags, " "))
		}
		b.WriteString("\n")
		writeIndentedLines(&b, "  unresolved: ", ticket.UnresolvedPhases)
		writeIndentedLines(&b, "  snippet: ", ticket.MatchingSnippets)
	}
	return b.String()
}

func formatMentalModels(models []wsdoc.MentalModelInfo) string {
	var b strings.Builder
	for _, model := range models {
		fmt.Fprintf(&b, "%s - %s", model.Path, displayOrDash(model.Domain))
		if model.Description != "" {
			fmt.Fprintf(&b, " # %s", model.Description)
		}
		flags := []string{}
		if model.MatchesDomain {
			flags = append(flags, "matches_domain")
		}
		if model.MatchesSpecStem {
			flags = append(flags, "matches_spec_stem")
		}
		if len(model.SpecRefs) > 0 {
			flags = append(flags, fmt.Sprintf("spec_refs=%d", len(model.SpecRefs)))
		}
		if len(flags) > 0 {
			fmt.Fprintf(&b, " [%s]", strings.Join(flags, " "))
		}
		b.WriteString("\n")
		writeIndentedLines(&b, "  source: ", model.Sources)
		writeIndentedLines(&b, "  ancestor: ", model.AncestorHints)
		writeIndentedLines(&b, "  index: ", model.IndexHints)
		writeIndentedLines(&b, "  snippet: ", model.MatchingSnippets)
	}
	return b.String()
}

func formatReferenceTrace(trace *wsdoc.ReferenceTrace) string {
	if trace == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "input: %s %s\n", trace.InputType, trace.Input)
	if len(trace.Tickets) > 0 {
		b.WriteString("tickets:\n")
		for _, ticket := range trace.Tickets {
			fmt.Fprintf(&b, "  [%s] %s", ticket.Status, ticket.Stem)
			if ticket.Title != "" {
				fmt.Fprintf(&b, " - %s", ticket.Title)
			}
			if ticket.Path != "" {
				fmt.Fprintf(&b, " (%s)", ticket.Path)
			}
			b.WriteString("\n")
		}
	}
	if len(trace.Specs) > 0 {
		b.WriteString("specs:\n")
		for _, spec := range trace.Specs {
			fmt.Fprintf(&b, "  %s", spec.Path)
			if spec.Title != "" {
				fmt.Fprintf(&b, " - %s", spec.Title)
			}
			if len(spec.Anchors) > 0 {
				fmt.Fprintf(&b, " [anchors=%d]", len(spec.Anchors))
			}
			b.WriteString("\n")
		}
	}
	if len(trace.MentalModels) > 0 {
		b.WriteString("mental_models:\n")
		for _, model := range trace.MentalModels {
			fmt.Fprintf(&b, "  %s - %s\n", model.Path, displayOrDash(model.Domain))
		}
	}
	return b.String()
}

func writeIndentedLines(b *strings.Builder, prefix string, lines []string) {
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fmt.Fprintf(b, "%s%s\n", prefix, strings.TrimSpace(line))
	}
}

func displayString(value any) string {
	text, _ := value.(string)
	return displayOrDash(text)
}

func displayOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func boolValue(value any) bool {
	got, _ := value.(bool)
	return got
}

func shortHash(value string) string {
	if len(value) > 7 && isHexString(value) {
		return value[:7]
	}
	return value
}

func isHexString(value string) bool {
	for _, ch := range value {
		if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F') {
			continue
		}
		return false
	}
	return value != ""
}

func (s *Server) resolveToolRoot(arguments map[string]any, meta map[string]any) (string, error) {
	// Session-key branch: highest priority. When present, the key authoritatively
	// resolves the root, bypassing every fallback in the chain below.
	if key, ok := arguments["session_key"].(string); ok && strings.TrimSpace(key) != "" {
		entry, found := s.sessions.lookup(key)
		if !found {
			return "", fmt.Errorf("unknown_session: session key not found in registry; " +
				"re-login via ws.lead.login(root) with your known root and retry the call")
		}
		return entry.root, nil
	}

	return "", fmt.Errorf("mandatory_session_key: root-aware ws tools require session_key; call ws.lead.login(root) first and pass the returned session_key")
}

func canonicalGitRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", fmt.Errorf("root is required")
	}
	cmd := exec.Command("git", "-C", root, "rev-parse", "--show-toplevel")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("root %q is not inside a Git worktree: %s", root, strings.TrimSpace(string(output)))
	}
	canonical := strings.TrimSpace(string(output))
	if canonical == "" {
		return "", fmt.Errorf("root %q did not resolve to a Git worktree", root)
	}
	if evaluated, err := filepath.EvalSymlinks(canonical); err == nil {
		canonical = evaluated
	}
	return filepath.Clean(canonical), nil
}

func codexWorkspaceRoots(meta map[string]any) []string {
	turn, ok := meta["x-codex-turn-metadata"].(map[string]any)
	if !ok {
		return nil
	}
	rawWorkspaces, ok := turn["workspaces"].(map[string]any)
	if !ok || len(rawWorkspaces) == 0 {
		return nil
	}
	roots := make([]string, 0, len(rawWorkspaces))
	for root := range rawWorkspaces {
		if strings.TrimSpace(root) != "" {
			roots = append(roots, root)
		}
	}
	return roots
}

func (s *Server) observeHarness(source, harness string) {
	harness = normalizedHarness(harness)
	if harness == "" {
		return
	}
	s.rootMu.Lock()
	defer s.rootMu.Unlock()
	if s.sessionHarness == "" {
		s.sessionHarness = harness
		appendDebugEvent("harness.detected", map[string]any{"source": source, "harness": harness})
		return
	}
	if s.sessionHarness != harness {
		appendDebugEvent("harness.conflict", map[string]any{
			"source":   source,
			"current":  s.sessionHarness,
			"observed": harness,
		})
	}
}

func (s *Server) currentHarness() string {
	s.rootMu.RLock()
	defer s.rootMu.RUnlock()
	return s.sessionHarness
}

func detectHarnessFromRaw(raw json.RawMessage) string {
	text := strings.ToLower(string(raw))
	if text == "" {
		return ""
	}
	if strings.Contains(text, "x-codex-turn-metadata") || strings.Contains(text, "codex") {
		return "codex"
	}
	if strings.Contains(text, "claude") || strings.Contains(text, "anthropic") {
		return "claude"
	}
	return ""
}

func detectHarnessFromMeta(meta map[string]any) string {
	if len(codexWorkspaceRoots(meta)) > 0 {
		return "codex"
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return ""
	}
	return detectHarnessFromRaw(raw)
}

func normalizedHarness(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "codex":
		return "codex"
	case "claude":
		return "claude"
	default:
		return ""
	}
}

func execResponse(id json.RawMessage, r execjob.Response, err error, includeStreams bool) response {
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	return toolTextResponse(id, formatExecResponse(r, includeStreams), nil)
}

func formatExecResponse(r execjob.Response, includeStreams bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "exec_key: %s\n", r.ExecKey)
	fmt.Fprintf(&b, "status: %s\n", r.Status)
	fmt.Fprintf(&b, "result_ready: %t\n", r.ResultReady)
	if r.PID != 0 {
		fmt.Fprintf(&b, "pid: %d\n", r.PID)
	}
	if r.ExitCode != 0 || r.ResultReady {
		fmt.Fprintf(&b, "exit_code: %d\n", r.ExitCode)
	}
	if r.StartedAt != "" {
		fmt.Fprintf(&b, "started_at: %s\n", r.StartedAt)
	}
	if r.UpdatedAt != "" {
		fmt.Fprintf(&b, "updated_at: %s\n", r.UpdatedAt)
	}
	if r.CompletedAt != "" {
		fmt.Fprintf(&b, "completed_at: %s\n", r.CompletedAt)
	}
	fmt.Fprintf(&b, "stdout_bytes: %d\n", r.StdoutBytes)
	fmt.Fprintf(&b, "stderr_bytes: %d\n", r.StderrBytes)
	fmt.Fprintf(&b, "combined_bytes: %d\n", r.CombinedBytes)
	if r.Error != "" {
		fmt.Fprintf(&b, "error: %s\n", r.Error)
	}
	if r.Guidance != "" {
		fmt.Fprintf(&b, "guidance: %s\n", r.Guidance)
	}
	if includeStreams && (r.Stdout != "" || r.Stderr != "") {
		b.WriteString("========== stdout ==========" + "\n")
		b.WriteString(r.Stdout)
		if r.Stdout != "" && !strings.HasSuffix(r.Stdout, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("========== stderr ==========" + "\n")
		b.WriteString(r.Stderr)
		if r.Stderr != "" && !strings.HasSuffix(r.Stderr, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func execRawTailResponse(id json.RawMessage, r execjob.RawTailResponse, err error) response {
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "exec_key: %s\n", r.ExecKey)
	fmt.Fprintf(&b, "stream: %s\n", r.Stream)
	b.WriteString("========== text ==========" + "\n")
	b.WriteString(r.Text)
	if r.Text != "" && !strings.HasSuffix(r.Text, "\n") {
		b.WriteString("\n")
	}
	return toolTextResponse(id, b.String(), nil)
}

func execRawReadResponse(id json.RawMessage, r execjob.RawReadResponse, err error) response {
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "exec_key: %s\n", r.ExecKey)
	fmt.Fprintf(&b, "stream: %s\n", r.Stream)
	fmt.Fprintf(&b, "offset: %d\n", r.Offset)
	fmt.Fprintf(&b, "next_offset: %d\n", r.NextOffset)
	fmt.Fprintf(&b, "limit: %d\n", r.Limit)
	fmt.Fprintf(&b, "size: %d\n", r.Size)
	fmt.Fprintf(&b, "eof: %t\n", r.EOF)
	b.WriteString("========== text ==========" + "\n")
	b.WriteString(r.Text)
	if r.Text != "" && !strings.HasSuffix(r.Text, "\n") {
		b.WriteString("\n")
	}
	return toolTextResponse(id, b.String(), nil)
}

func execRawGrepResponse(id json.RawMessage, r execjob.RawGrepResponse, err error) response {
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "exec_key: %s\n", r.ExecKey)
	fmt.Fprintf(&b, "stream: %s\n", r.Stream)
	fmt.Fprintf(&b, "matches: %d\n", len(r.Matches))
	fmt.Fprintf(&b, "truncated: %t\n", r.Truncated)
	b.WriteString("========== matches ==========" + "\n")
	for i, m := range r.Matches {
		if i > 0 {
			b.WriteString("--\n")
		}
		for _, line := range m.Before {
			fmt.Fprintf(&b, "%d- %s\n", m.Line, line)
		}
		fmt.Fprintf(&b, "%d: %s\n", m.Line, m.Text)
		for _, line := range m.After {
			fmt.Fprintf(&b, "%d+ %s\n", m.Line, line)
		}
	}
	return toolTextResponse(id, b.String(), nil)
}

func toolJSONResponse(id json.RawMessage, value any, err error) response {
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	return toolTextResponse(id, string(raw)+"\n", nil)
}

func toolTextResponse(id json.RawMessage, text string, err error) response {
	if err != nil {
		return toolErrorTextResponse(id, err.Error())
	}
	return response{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"content": []map[string]string{{
			"type": "text",
			"text": text,
		}},
	}}
}

func toolErrorTextResponse(id json.RawMessage, text string) response {
	return response{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"isError": true,
		"content": []map[string]string{{
			"type": "text",
			"text": text,
		}},
	}}
}

func errorResponse(id json.RawMessage, code int, message string) response {
	return response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}}
}

func tools() []map[string]any {
	return []map[string]any{
		{
			"name":        "runtime.info",
			"description": "Return ws-mcp runtime metadata for compatibility checks.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"format": stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
			},
		},
		{
			"name":        "runtime.debug_events",
			"description": "Return recent in-process MCP debug events as JSONL.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit": integerProperty("Maximum number of events to return. Defaults to 80 and is capped."),
				},
			},
		},
		{
			"name":        "ws.lead.login",
			"description": "Mint an ephemeral word-chain session key for the given repository root. The returned session_key may be passed to any root-aware ws tool to identify the call root without relying on the server session state. The key is opaque; do not parse it.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":       stringProperty("Absolute Git worktree root to bind to the session key."),
					"capability": enumStringProperty(`Optional capability scope. Omit or pass "lead" for unrestricted access. Pass "delegate" or "leaf" to restrict the key to that role's allowed tools.`, []string{"lead", "delegate", "leaf"}),
					"format":     stringProperty(`Optional output format. Use "json" for structured output.`),
				},
				"required": []string{"root"},
			},
		},
		{
			"name":        "api.list",
			"description": "Return sorted API documentation cache domain names under ai-docs/.deps.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":   stringProperty("Repository root. Defaults to the server root."),
					"format": stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
			},
		},
		{
			"name":        "api.ask",
			"description": "Ask cached or fetchable third-party API documentation through per-domain manager sessions.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":        stringProperty("Repository root. Defaults to the server root."),
					"prompt":      stringProperty("API documentation question to answer."),
					"domain_hint": stringProperty("Optional API documentation domain hint."),
				},
				"required": []string{"prompt"},
			},
		},
		{
			"name":        "api.ask_async",
			"description": "Start a recoverable asynchronous API documentation lookup job and return an api_job_key immediately.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":        stringProperty("Repository root. Defaults to the server root."),
					"prompt":      stringProperty("API documentation question to answer asynchronously."),
					"domain_hint": stringProperty("Optional API documentation domain hint."),
				},
				"required": []string{"prompt"},
			},
		},
		{
			"name":        "api.status",
			"description": "Inspect a recoverable asynchronous API documentation job by api_job_key.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":        stringProperty("Repository root. Defaults to the server root."),
					"api_job_key": stringProperty("Recoverable async API documentation job key."),
				},
				"required": []string{"api_job_key"},
			},
		},
		{
			"name":        "api.result",
			"description": "Return the final answer for a recoverable asynchronous API documentation job by api_job_key.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":        stringProperty("Repository root. Defaults to the server root."),
					"api_job_key": stringProperty("Recoverable async API documentation job key."),
				},
				"required": []string{"api_job_key"},
			},
		},
		{
			"name":        "api.cancel",
			"description": "Best-effort cancel a recoverable asynchronous API documentation job by api_job_key.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":        stringProperty("Repository root. Defaults to the server root."),
					"api_job_key": stringProperty("Recoverable async API documentation job key."),
				},
				"required": []string{"api_job_key"},
			},
		},
		{
			"name":        "exec.spawn",
			"description": "Start a durable structured command job using argv, with bounded inline output when it completes quickly.",
			"inputSchema": execLaunchSchema(false),
		},
		{
			"name":        "exec.shell",
			"description": "Start a durable explicit shell command job, with bounded inline output when it completes quickly.",
			"inputSchema": execLaunchSchema(true),
		},
		{
			"name":        "exec.status",
			"description": "Return lifecycle status and stream sizes for a durable exec job.",
			"inputSchema": execKeySchema(),
		},
		{
			"name":        "exec.result",
			"description": "Return terminal exec job metadata and at most the fixed 4096-byte inline output budget; positive timeout_seconds waits for completion.",
			"inputSchema": execResultSchema(),
		},
		{
			"name":        "exec.abort",
			"description": "Best-effort terminate a running exec job while preserving partial output.",
			"inputSchema": execKeySchema(),
		},
		{
			"name":        "exec.raw.tail",
			"description": "Read bounded tail text from a persisted exec stdout, stderr, or combined stream.",
			"inputSchema": execRawTailSchema(),
		},
		{
			"name":        "exec.raw.read",
			"description": "Read bytes from a persisted exec stream with offset continuation.",
			"inputSchema": execRawReadSchema(),
		},
		{
			"name":        "exec.raw.grep",
			"description": "Search persisted exec streams with literal matching by default and opt-in regex mode.",
			"inputSchema": execRawGrepSchema(),
		},
		{
			"name":        "config.show",
			"description": "Return the current ws user-local configuration and resolved config path without modifying it.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"format": stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
			},
		},
		{
			"name":        "config.agents_tier",
			"description": "Compatibility surface for configuring the current or selected harness backend/model mapping for a ws agent model alias.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tier":    enumStringProperty("Model alias to configure.", []string{"light", "core", "deep"}),
					"backend": stringProperty("Optional backend name. When omitted, ws infers it from the model when possible."),
					"model":   stringProperty("Concrete model for this alias."),
					"effort":  enumStringProperty("Optional portable reasoning effort for this alias. Empty, omitted, or none leaves backend effort unset.", []string{"", "none", "low", "medium", "high", "xhigh"}),
					"harness": stringProperty("Optional harness alias key to configure. When omitted, ws uses the detected MCP session harness, or default when none is known."),
				},
				"required": []string{"tier"},
			},
		},
		{
			"name":        "git.status",
			"description": "Return read-only Git branch and worktree status. Defaults to compact text; use format=json for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":   stringProperty("Repository root. Defaults to the server root."),
					"format": stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
			},
		},
		{
			"name":        "git.diff",
			"description": "Return read-only Git diff output. Defaults to stat text; use mode=full for patch content or format=json for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":   stringProperty("Repository root. Defaults to the server root."),
					"range":  stringProperty("Optional revision range."),
					"paths":  stringArrayProperty("Optional path filters appended after --."),
					"mode":   enumStringProperty("Diff mode. Defaults to stat.", []string{"full", "stat", "name_only"}),
					"format": stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
			},
		},
		{
			"name":        "git.log",
			"description": "Return a bounded read-only Git commit log. Defaults to text blocks; use format=json for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":         stringProperty("Repository root. Defaults to the server root."),
					"range":        stringProperty("Optional revision range."),
					"limit":        integerProperty("Maximum commits to return. Defaults to 20 and is capped at 100."),
					"include_body": boolProperty("Include commit body text."),
					"format":       stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
			},
		},
		{
			"name":        "git.merge_base",
			"description": "Return the read-only Git merge-base hash for two revisions. Defaults to text; use format=json for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":   stringProperty("Repository root. Defaults to the server root."),
					"base":   stringProperty("Base revision."),
					"head":   stringProperty("Head revision."),
					"format": stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
				"required": []string{"base", "head"},
			},
		},
		{
			"name":        "git.commit",
			"description": "Create a workflow-aware Git commit from explicit paths and structured message fields. Defaults to compact text; use format=json for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":                  stringProperty("Repository root. Defaults to the server root."),
					"paths":                 stringArrayProperty("Explicit paths to stage and commit. Only these paths are staged."),
					"title":                 stringProperty("Single-line commit title."),
					"description":           stringProperty("Optional commit message body before AI Context."),
					"ai_context":            stringArrayProperty("Required AI Context bullets for the commit message."),
					"mental_model_notes":    stringArrayProperty("Optional Mental Model Notes bullets rendered as an H3 subsection under AI Context."),
					"updated_tickets":       stringArrayProperty("Optional ticket update summaries. If omitted, staged ticket moves and Result/Edition headings are detected."),
					"updated_specs":         stringArrayProperty("Optional spec update summaries."),
					"updated_mental_models": stringArrayProperty("Optional mental-model update summaries."),
					"format":                stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
				"required": []string{"paths", "title", "ai_context"},
			},
		},
		{
			"name":        "project_tree",
			"description": "Render the ws project document map, spec inventory, and active ticket inventory.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root": map[string]string{
						"type":        "string",
						"description": "Repository root. Defaults to the server root.",
					},
				},
			},
		},
		{
			"name":        "infra.read",
			"description": "Read a bundled ws infra document by bare stem or filename.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]string{
						"type":        "string",
						"description": "Infra document stem or filename, for example impl-playbook.",
					},
				},
				"required": []string{"name"},
			},
		},
		{
			"name":        "convention.read",
			"description": "Read a bundled ws convention document by bare stem or filename.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]string{
						"type":        "string",
						"description": "Convention document stem or filename, for example ticket-conventions.",
					},
				},
				"required": []string{"name"},
			},
		},
		{
			"name":        "spec_stem.generate",
			"description": "Generate a collision-free spec anchor stem for a slug.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"slug": map[string]string{
						"type":        "string",
						"description": "Descriptive slug seed.",
					},
					"root": map[string]string{
						"type":        "string",
						"description": "Repository root. Defaults to the server root.",
					},
				},
				"required": []string{"slug"},
			},
		},
		{
			"name":        "spec_index.verify",
			"description": "Verify basic spec anchor index health.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root": map[string]string{
						"type":        "string",
						"description": "Repository root. Defaults to the server root.",
					},
				},
			},
		},
		{
			"name":        "specs.list",
			"description": "List spec files. Defaults to compact text; use format=json for frontmatter, anchors, ticket refs, and marker metadata.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":   stringProperty("Repository root. Defaults to the server root."),
					"format": stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
			},
		},
		{
			"name":        "specs.find",
			"description": "Find spec files by query, spec anchor stem, or ticket stem reference. Defaults to compact text; use format=json for structured metadata.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":        stringProperty("Repository root. Defaults to the server root."),
					"query":       stringProperty("Optional case-insensitive text query."),
					"spec_stem":   stringProperty("Optional exact spec anchor stem."),
					"ticket_stem": stringProperty("Optional ticket stem referenced by spec frontmatter or feature entries."),
					"format":      stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
			},
		},
		{
			"name":        "specs.status",
			"description": "Return locations and file metadata for one spec anchor stem. Defaults to compact text; use format=json for structured metadata.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":      stringProperty("Repository root. Defaults to the server root."),
					"spec_stem": stringProperty("Spec anchor stem to inspect."),
					"format":    stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
				"required": []string{"spec_stem"},
			},
		},
		{
			"name":        "mental_models.list",
			"description": "List mental-model documents with domains, descriptions, and sources.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root": map[string]string{
						"type":        "string",
						"description": "Repository root. Defaults to the server root.",
					},
				},
			},
		},
		{
			"name":        "mental_models.find",
			"description": "Find mental-model paths by query, spec stem reference, or domain. Defaults to compact text; use format=json for structured metadata.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":      stringProperty("Repository root. Defaults to the server root."),
					"query":     stringProperty("Optional case-insensitive text query."),
					"spec_stem": stringProperty("Optional spec anchor stem referenced by the mental model."),
					"domain":    stringProperty("Optional mental-model domain."),
					"format":    stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
			},
		},
		{
			"name":        "mental_models.status",
			"description": "Return path-first metadata for mental-model documents selected by domain or path. Defaults to compact text; use format=json for structured metadata.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":   stringProperty("Repository root. Defaults to the server root."),
					"domain": stringProperty("Optional mental-model domain."),
					"path":   stringProperty("Optional relative path under ai-docs/mental-model."),
					"format": stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
			},
		},
		{
			"name":        "references.trace",
			"description": "Trace ticket/spec/mental-model references from exactly one ticket_stem or spec_stem. Defaults to compact text; use format=json for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":        stringProperty("Repository root. Defaults to the server root."),
					"ticket_stem": stringProperty("Optional ticket stem to trace."),
					"spec_stem":   stringProperty("Optional spec anchor stem to trace."),
					"format":      stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
			},
		},
		{
			"name":        "tickets.list",
			"description": "List ticket paths and status metadata without reading full document bodies. Defaults to compact text; use format=json for structured metadata.",
			"inputSchema": ticketDiscoverySchema(false),
		},
		{
			"name":        "tickets.find",
			"description": "Find ticket paths by query, ticket stem, or mentions of another ticket stem. Defaults to compact text; use format=json for structured metadata.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":                 stringProperty("Repository root. Defaults to the server root."),
					"statuses":             stringArrayProperty("Optional ticket statuses to scan: ready, todo, idea, done, dropped."),
					"include_done":         boolProperty("Include ai-docs/tickets/.done when true."),
					"include_dropped":      boolProperty("Include ai-docs/tickets/.dropped when true."),
					"query":                stringProperty("Optional case-insensitive text query."),
					"ticket_stem":          stringProperty("Optional exact ticket stem."),
					"mentions_ticket_stem": stringProperty("Optional ticket stem that result tickets must mention."),
					"format":               stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
			},
		},
		{
			"name":        "tickets.status",
			"description": "Return status metadata for a single ticket stem. Defaults to compact text; use format=json for structured metadata.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":            stringProperty("Repository root. Defaults to the server root."),
					"ticket_stem":     stringProperty("Ticket stem to inspect."),
					"include_done":    boolProperty("Allow lookup under ai-docs/tickets/.done when true."),
					"include_dropped": boolProperty("Allow lookup under ai-docs/tickets/.dropped when true."),
					"format":          stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
				"required": []string{"ticket_stem"},
			},
		},
		{
			"name":        "subquery",
			"description": "Start an async scoped codebase or documentation query and return a subquery key.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"question":      stringProperty("Scoped question to answer."),
					"deep_research": boolProperty("Use deep model alias for broad tracing or research."),
				},
				"required": []string{"question"},
			},
		},
		{
			"name":        "path.generate",
			"description": "Generate worktree-scoped writable paths for workflow artifacts.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":  stringProperty("Repository root. Defaults to the server root."),
					"kind":  stringProperty("Generated path kind. Initially supports review."),
					"stems": stringArrayProperty("Logical file stems to allocate in stable order."),
				},
				"required": []string{"kind", "stems"},
			},
		},
		{
			"name":        "prompt.render",
			"description": namespaceText("Render a bundled delegate prompt by stem with namespace substitution and injected context; returns a tmp prompt file path (wsflow only)."),
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":    stringProperty("Repository root. Defaults to the server root."),
					"stem":    stringProperty("Bundled prompt stem to render (e.g. code-reviewer, reference-discovery)."),
					"context": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Optional string key-value pairs injected as a ## Render Context block at the end of the rendered prompt."},
				},
				"required": []string{"stem"},
			},
		},
		{
			"name":        "playbook.print",
			"description": namespaceText("Return a playbook's rendered procedure text inline (harness-aware, includes resolved, declared variables substituted). Full ws; not wsflow-only."),
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":    stringProperty("Playbook name (bare stem resolvable by the rsrc loader)."),
					"context": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Optional caller-supplied substitution values for variables declared in the playbook's frontmatter."},
				},
				"required": []string{"name"},
			},
		},
		{
			"name":        "playbook.render",
			"description": namespaceText("Render a playbook to a worktree-scoped tmp file and return the path (harness-aware, includes resolved, declared variables substituted). Full ws; not wsflow-only."),
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":    stringProperty("Repository root for the tmp file. Defaults to the server root."),
					"name":    stringProperty("Playbook name (bare stem resolvable by the rsrc loader)."),
					"context": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Optional caller-supplied substitution values for variables declared in the playbook's frontmatter."},
				},
				"required": []string{"name"},
			},
		},
		{
			"name":        "agents.register",
			"description": "Register a durable ws agent session for the current worktree.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":               stringProperty("Agent name."),
					"backend":            stringProperty("Optional backend name. Model aliases use the detected harness when omitted."),
					"tier":               stringProperty("Deprecated compatibility alias selector: light, core, or deep."),
					"model":              stringProperty("Optional model alias or concrete backend model. Aliases: light, core, deep."),
					"prompts":            stringArrayProperty("Embedded prompt stems or absolute prompt paths."),
					"prompt_refs":        stringArrayProperty("Logical role prompt references."),
					"system_prompt_text": stringProperty("Optional materialized system prompt text."),
				},
				"required": []string{"name"},
			},
		},
		{
			"name":        "agents.call",
			"description": "Start an asynchronous call for a registered ws agent and return immediately.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":   stringProperty("Agent name."),
					"prompt": stringProperty("Prompt to send to the agent."),
				},
				"required": []string{"name", "prompt"},
			},
		},
		{
			"name":        "agents.wait",
			"description": "Wait for one or more registered ws agents to become ready; returns status metadata, not final output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":            stringProperty("Agent name. Compatibility alias for a single name."),
					"names":           stringArrayProperty("Agent names to wait for."),
					"timeout_seconds": numberProperty("Maximum seconds to wait. Defaults to 600."),
				},
			},
		},
		{
			"name":        "agents.result",
			"description": "Return a completed agent result, optionally waiting; successful ephemeral results are consumed and erased.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":            stringProperty("Agent name."),
					"timeout_seconds": numberProperty("Maximum seconds to wait. Omit or set 0 for a non-blocking read."),
				},
				"required": []string{"name"},
			},
		},
		{
			"name":        "agents.status",
			"description": "Return current status for a registered ws agent.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": stringProperty("Agent name."),
				},
				"required": []string{"name"},
			},
		},
		{
			"name":        "agents.interrupt",
			"description": "Queue an interrupt or redirect message for a registered ws agent.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":    stringProperty("Agent name."),
					"message": stringProperty("Interrupt or redirect message to deliver to the agent."),
				},
				"required": []string{"name", "message"},
			},
		},
		{
			"name":        "agents.tail",
			"description": "Return context-bounded recent event, stream, and output lines for a registered ws agent.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":  stringProperty("Agent name."),
					"lines": integerProperty("Number of lines per section. Defaults to 40."),
				},
				"required": []string{"name"},
			},
		},
		{
			"name":        "agents.debug.tail",
			"description": "Debug only: return raw diagnostic tail sections for a registered ws agent.",
			"inputSchema": agentDebugSchema("Number of lines per section. Defaults to 40."),
		},
		{
			"name":        "agents.debug.stdout",
			"description": "Debug only: return recent raw stdout lines for the current agent call.",
			"inputSchema": agentDebugSchema("Number of stdout lines. Defaults to 40."),
		},
		{
			"name":        "agents.debug.stderr",
			"description": "Debug only: return recent raw stderr lines for the current agent call.",
			"inputSchema": agentDebugSchema("Number of stderr lines. Defaults to 40."),
		},
		{
			"name":        "agents.debug.runtime_log",
			"description": "Debug only: return recent raw runtime log lines for the current agent call.",
			"inputSchema": agentDebugSchema("Number of runtime log lines. Defaults to 40."),
		},
		{
			"name":        "agents.debug.events",
			"description": "Debug only: return recent raw agent events log lines.",
			"inputSchema": agentDebugSchema("Number of event log lines. Defaults to 40."),
		},
		{
			"name":        "agents.cancel",
			"description": "Best-effort cancel the current async call for a registered ws agent.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": stringProperty("Agent name."),
				},
				"required": []string{"name"},
			},
		},
		{
			"name":        "agents.print",
			"description": "Deprecated compatibility alias: return the last plain-text output without consuming ephemeral agents.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": stringProperty("Agent name."),
				},
				"required": []string{"name"},
			},
		},
		{
			"name":        "agents.erase",
			"description": "Erase a registered ws agent directory for the current worktree.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": stringProperty("Agent name."),
				},
				"required": []string{"name"},
			},
		},
	}
}

func LeadToolNames() []string {
	names := make([]string, 0, len(tools()))
	for _, tool := range tools() {
		name, _ := tool["name"].(string)
		name = advertisedToolName(name)
		if NoAgentMode() && noAgentHiddenTool(name) {
			continue
		}
		if !NoAgentMode() && wsflowOnlyTool(name) {
			continue
		}
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func (s *Server) filteredTools() []map[string]any {
	base := tools()
	filtered := make([]map[string]any, 0, len(base))
	for _, tool := range base {
		name, _ := tool["name"].(string)
		name = advertisedToolName(name)
		if s.toolAllowed(name) {
			filtered = append(filtered, publicToolDefinition(tool, name))
		}
	}
	return filtered
}

func publicToolDefinition(tool map[string]any, advertisedName string) map[string]any {
	name, _ := tool["name"].(string)
	clone := make(map[string]any, len(tool))
	for key, value := range tool {
		clone[key] = value
	}
	if advertisedName != "" {
		clone["name"] = advertisedName
	}
	if description, _ := clone["description"].(string); description != "" {
		clone["description"] = namespaceText(description)
	}
	if schema, ok := clone["inputSchema"].(map[string]any); ok {
		clone["inputSchema"] = namespaceValue(schema)
	}
	if !strings.HasPrefix(name, "agents.") {
		return clone
	}
	schema, ok := clone["inputSchema"].(map[string]any)
	if !ok {
		return clone
	}
	schemaClone := make(map[string]any, len(schema))
	for key, value := range schema {
		schemaClone[key] = value
	}
	if properties, ok := schema["properties"].(map[string]any); ok {
		propertiesClone := make(map[string]any, len(properties))
		for key, value := range properties {
			if strings.HasPrefix(name, "agents.") && key == "root" {
				continue
			}
			propertiesClone[key] = value
		}
		schemaClone["properties"] = propertiesClone
	}
	clone["inputSchema"] = schemaClone
	return clone
}

func (s *Server) toolAllowed(name string) bool {
	if NoAgentMode() && noAgentHiddenTool(name) {
		return false
	}
	if !NoAgentMode() && wsflowOnlyTool(name) {
		return false
	}
	if !roleAllowsTool(s.role, name) {
		return false
	}
	if allowed := explicitAllowedTools(); len(allowed) > 0 {
		return allowed[name]
	}
	return true
}

func requestedToolRole() toolRole {
	switch strings.TrimSpace(os.Getenv("WS_MCP_TOOL_PROFILE")) {
	case "", "lead":
		return roleLead
	case "delegate":
		return roleDelegate
	case "leaf":
		return roleLeaf
	default:
		return roleLead
	}
}

func roleAllowsTool(role toolRole, name string) bool {
	switch role {
	case roleLead:
		return true
	case roleDelegate:
		if strings.HasPrefix(name, "session.") || name == "ws.setup" {
			return false
		}
		if isSubqueryAgentTool(name) {
			return true
		}
		return !strings.HasPrefix(name, "agents.") && !strings.HasPrefix(name, "config.")
	case roleLeaf:
		return !strings.HasPrefix(name, "agents.") && !strings.HasPrefix(name, "config.") && !strings.HasPrefix(name, "session.") && !strings.HasPrefix(name, "api.") && name != "ws.setup" && name != "subquery" && name != "git.commit"
	default:
		return false
	}
}

func advertisedToolName(name string) string {
	return name
}

func namespaceText(text string) string {
	namespace := RuntimeNamespace()
	if namespace == "ws" {
		return text
	}
	replacer := strings.NewReplacer(
		"ws MCP", namespace+" MCP",
		"ws/", namespace+"/",
		"ws:", namespace+":",
		"ws project", namespace+" project",
		"ws runtime", namespace+" runtime",
		"ws workflow", namespace+" workflow",
		"ws user", namespace+" user",
		"ws agent", namespace+" agent",
		"ws agents", namespace+" agents",
		"ws ", namespace+" ",
	)
	return replacer.Replace(text)
}

func namespaceValue(value any) any {
	switch typed := value.(type) {
	case string:
		return namespaceText(typed)
	case map[string]any:
		clone := make(map[string]any, len(typed))
		for key, child := range typed {
			clone[key] = namespaceValue(child)
		}
		return clone
	case map[string]string:
		clone := make(map[string]string, len(typed))
		for key, child := range typed {
			clone[key] = namespaceText(child)
		}
		return clone
	case []any:
		clone := make([]any, len(typed))
		for i, child := range typed {
			clone[i] = namespaceValue(child)
		}
		return clone
	case []string:
		clone := make([]string, len(typed))
		for i, child := range typed {
			clone[i] = namespaceText(child)
		}
		return clone
	default:
		return value
	}
}

func noAgentHiddenTool(name string) bool {
	if strings.HasPrefix(name, "exec.") {
		return true
	}
	if strings.HasPrefix(name, "agents.") {
		return true
	}
	switch name {
	case "subquery", "config.agents_tier", "api.ask", "api.ask_async", "api.status", "api.result", "api.cancel":
		return true
	default:
		return false
	}
}

func wsflowOnlyTool(name string) bool {
	switch name {
	case "prompt.render":
		return true
	default:
		return false
	}
}

// wsNamespaceRef matches the ws namespace prefix token (ws/ or ws:) anchored at
// a word boundary so that words containing "ws" as an interior substring (e.g.
// "news/", "rows:", "workflows/") are never mangled.
var wsNamespaceRef = regexp.MustCompile(`\bws([/:])`)

// wsflowRenderEligibleStems is the exact set of prompt stems that are
// render-eligible from wsflow per spec #260529-prompt-render-tool.
// Add entries here as the spec expands the set.
var wsflowRenderEligibleStems = map[string]bool{
	"reference-discovery":     true,
	"plan-populator-survey":   true,
	"plan-populator-research": true,
	"code-reviewer":           true,
	"mental-model-updater":    true,
}

// renderPrompt loads a bundled prompt by stem, applies namespace substitution,
// appends an optional injected context block, writes the result to a
// worktree-scoped tmp file, and returns the path.
func renderPrompt(root, stem string, context map[string]string) (string, error) {
	if !wsflowRenderEligibleStems[stem] {
		return "", fmt.Errorf("prompt stem %q is not render-eligible in wsflow", stem)
	}
	body, err := wsprompt.RenderSource(stem)
	if err != nil {
		return "", fmt.Errorf("load prompt %q: %w", stem, err)
	}
	ns := RuntimeNamespace()
	body = wsNamespaceRef.ReplaceAllString(body, ns+"$1")
	if len(context) > 0 {
		keys := make([]string, 0, len(context))
		for k := range context {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var sb strings.Builder
		sb.WriteString("\n\n## Render Context\n")
		for _, k := range keys {
			sb.WriteString("- ")
			sb.WriteString(k)
			sb.WriteString(": ")
			sb.WriteString(context[k])
			sb.WriteString("\n")
		}
		body += sb.String()
	}
	generated, err := wsstate.NewManager(wsstate.Options{}).GeneratePaths(root, "prompt", []string{stem})
	if err != nil {
		return "", fmt.Errorf("allocate prompt path: %w", err)
	}
	if err := os.WriteFile(generated[0].Path, []byte(body), 0o644); err != nil {
		return "", fmt.Errorf("write prompt %s: %w", generated[0].Path, err)
	}
	return generated[0].Path, nil
}

func (s *Server) subqueryAgentAccessAllowed(toolName string, arguments map[string]any) bool {
	if s.role == roleLead || !isSubqueryAgentTool(toolName) {
		return true
	}
	name, _ := arguments["name"].(string)
	if name != "" && !strings.HasPrefix(name, "subquery-") {
		return false
	}
	for _, item := range stringList(arguments["names"]) {
		if !strings.HasPrefix(item, "subquery-") {
			return false
		}
	}
	return name != "" || len(stringList(arguments["names"])) > 0
}

func isSubqueryAgentTool(name string) bool {
	switch name {
	case "agents.wait", "agents.result", "agents.status", "agents.tail", "agents.cancel", "agents.print":
		return true
	default:
		return false
	}
}

func explicitAllowedTools() map[string]bool {
	raw := strings.TrimSpace(os.Getenv("WS_MCP_ALLOWED_TOOLS"))
	if raw == "" {
		return nil
	}
	allowed := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if name != "" {
			allowed[name] = true
		}
	}
	return allowed
}

func agentDebugSchema(linesDescription string) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":  stringProperty("Agent name."),
			"lines": integerProperty(linesDescription),
		},
		"required": []string{"name"},
	}
}

func execLaunchSchema(shell bool) map[string]any {
	props := map[string]any{
		"working_dir": stringProperty("Command working directory. Defaults to the resolved ws worktree root; relative paths resolve beneath that root."),
		"env":         map[string]any{"type": "object", "additionalProperties": map[string]string{"type": "string"}, "description": "Environment variable overlay."},
		"stdin":       stringProperty("Text stdin for the process."),
	}
	required := []string{"cmd"}
	if shell {
		props["command"] = stringProperty("Shell command string to execute.")
		props["shell"] = stringProperty("Optional explicit shell executable. Defaults to the platform shell.")
		required = []string{"command"}
	} else {
		props["cmd"] = stringProperty("Executable argv0 to run; not a shell command line.")
		props["args"] = stringArrayProperty("Optional argv arguments.")
	}
	return map[string]any{"type": "object", "properties": props, "required": required}
}

func execKeySchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"exec_key": stringProperty("Durable exec job key.")}, "required": []string{"exec_key"}}
}

func execResultSchema() map[string]any {
	s := execKeySchema()
	props := s["properties"].(map[string]any)
	props["timeout_seconds"] = numberProperty("Maximum seconds to wait for a running job to become terminal. Omit or set 0 for non-blocking behavior.")
	return s
}

func execRawTailSchema() map[string]any {
	s := execKeySchema()
	props := s["properties"].(map[string]any)
	props["stream"] = enumStringProperty("Stream to read. Defaults to stdout.", []string{"stdout", "stderr", "combined"})
	props["lines"] = integerProperty("Number of tail lines. Defaults to 40 and is capped.")
	return s
}
func execRawReadSchema() map[string]any {
	s := execKeySchema()
	props := s["properties"].(map[string]any)
	props["stream"] = enumStringProperty("Stream to read. Defaults to stdout.", []string{"stdout", "stderr", "combined"})
	props["offset"] = integerProperty("Byte offset. Defaults to 0.")
	props["limit"] = integerProperty("Maximum bytes to read. Defaults to 4096 and is capped.")
	return s
}
func execRawGrepSchema() map[string]any {
	s := execKeySchema()
	props := s["properties"].(map[string]any)
	props["stream"] = enumStringProperty("Stream to search. Defaults to stdout.", []string{"stdout", "stderr", "combined"})
	props["pattern"] = stringProperty("Search pattern. Literal unless regex is true.")
	props["before"] = integerProperty("Context lines before each match.")
	props["after"] = integerProperty("Context lines after each match.")
	props["max_matches"] = integerProperty("Maximum matches to return.")
	props["regex"] = boolProperty("Treat pattern as a regular expression when true.")
	s["required"] = []string{"exec_key", "pattern"}
	return s
}

func ticketDiscoverySchema(requireTicketStem bool) map[string]any {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"root":            stringProperty("Repository root. Defaults to the server root."),
			"statuses":        stringArrayProperty("Optional ticket statuses to scan: ready, todo, idea, done, dropped."),
			"include_done":    boolProperty("Include ai-docs/tickets/.done when true."),
			"include_dropped": boolProperty("Include ai-docs/tickets/.dropped when true."),
			"format":          stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
		},
	}
	if requireTicketStem {
		schema["required"] = []string{"ticket_stem"}
	}
	return schema
}

func stringMapArgument(value any) map[string]string {
	items, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(items))
	for key, value := range items {
		if text, ok := value.(string); ok {
			out[key] = text
		}
	}
	return out
}

func int64FromArgument(value any, fallback int64) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func stringList(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if ok && text != "" {
			out = append(out, text)
		}
	}
	return out
}

func boolArgument(value any) bool {
	result, _ := value.(bool)
	return result
}

func hasSpecStemArgument(arguments map[string]any) bool {
	_, ok := arguments["spec_stem"]
	return ok
}

func hasTicketOnlyArgument(arguments map[string]any) bool {
	_, ok := arguments["mentions_ticket_stem"]
	return ok
}

func hasTicketStemArgument(arguments map[string]any) bool {
	if _, ok := arguments["ticket_stem"]; ok {
		return true
	}
	return hasTicketOnlyArgument(arguments)
}

func stringProperty(description string) map[string]string {
	return map[string]string{
		"type":        "string",
		"description": description,
	}
}

func stringArrayProperty(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items": map[string]string{
			"type": "string",
		},
	}
}

func enumStringProperty(description string, values []string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
		"enum":        values,
	}
}

func numberProperty(description string) map[string]string {
	return map[string]string{
		"type":        "number",
		"description": description,
	}
}

func integerProperty(description string) map[string]string {
	return map[string]string{
		"type":        "integer",
		"description": description,
	}
}

func boolProperty(description string) map[string]string {
	return map[string]string{
		"type":        "boolean",
		"description": description,
	}
}

func durationFromSeconds(value any) time.Duration {
	switch v := value.(type) {
	case float64:
		if v <= 0 {
			return 0
		}
		return time.Duration(v * float64(time.Second))
	case string:
		if v == "" {
			return 0
		}
		duration, err := time.ParseDuration(v)
		if err == nil {
			return duration
		}
		seconds, err := strconv.ParseFloat(v, 64)
		if err != nil || seconds <= 0 {
			return 0
		}
		return time.Duration(seconds * float64(time.Second))
	default:
		return 0
	}
}

func intFromArgument(value any, fallback int) int {
	switch v := value.(type) {
	case float64:
		if v <= 0 {
			return fallback
		}
		return int(v)
	case string:
		if v == "" {
			return fallback
		}
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			return fallback
		}
		return parsed
	default:
		return fallback
	}
}
