package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdoc"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsprompt"
	"github.com/kang-sw/devenv/internal/wsstate"
)

type Server struct {
	root         string
	version      string
	sourceCommit string
	role         toolRole
	api          apiRuntime
	rootMu       sync.RWMutex
	sessionRoot  string
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
	return &Server{root: cleanRoot, version: version, sourceCommit: commit, role: role}
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
	if !s.toolAllowed(params.Name) {
		return errorResponse(req.ID, -32601, fmt.Sprintf("tool not available in current ws MCP profile: %s", params.Name))
	}
	if !s.subqueryAgentAccessAllowed(params.Name, params.Arguments) {
		return errorResponse(req.ID, -32601, fmt.Sprintf("tool available only for subquery-* agents in current ws MCP profile: %s", params.Name))
	}

	switch params.Name {
	case "runtime.info":
		info, err := runtimeInfo(s.version, s.sourceCommit)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		raw, err := json.Marshal(info)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		return toolTextResponse(req.ID, string(raw)+"\n", nil)
	case "runtime.debug_events":
		text, err := debugEventsJSONL(intFromArgument(params.Arguments["limit"], 80))
		return toolTextResponse(req.ID, text, err)
	case "session.set_default_root":
		root, _ := params.Arguments["root"].(string)
		canonical, err := canonicalGitRoot(root)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		s.rootMu.Lock()
		s.sessionRoot = canonical
		s.rootMu.Unlock()
		return toolJSONResponse(req.ID, map[string]any{
			"session_default_root": canonical,
			"source":               "session",
		}, nil)
	case "session.get_default_root":
		s.rootMu.RLock()
		sessionRoot := s.sessionRoot
		s.rootMu.RUnlock()
		return toolJSONResponse(req.ID, map[string]any{
			"session_default_root": sessionRoot,
			"has_session_default":  sessionRoot != "",
			"env_project_root":     strings.TrimSpace(os.Getenv("WS_MCP_PROJECT_ROOT")),
			"server_root":          s.root,
		}, nil)
	case "api.list":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		domains, err := apiListDomains(root)
		return toolJSONResponse(req.ID, domains, err)
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
	case "config.show":
		view, err := wsconfig.Show(wsconfig.Options{})
		return toolJSONResponse(req.ID, view, err)
	case "config.agents_tier":
		tier, _ := params.Arguments["tier"].(string)
		backend, _ := params.Arguments["backend"].(string)
		model, _ := params.Arguments["model"].(string)
		cfg, err := wsconfig.SetAgentsTier(wsconfig.Options{}, tier, backend, model)
		return toolJSONResponse(req.ID, cfg, err)

	case "git.status":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		result, err := wsgit.NewClient().Status(context.Background(), root)
		return toolJSONResponse(req.ID, result, err)
	case "git.diff":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		rangeValue, _ := params.Arguments["range"].(string)
		mode, _ := params.Arguments["mode"].(string)
		result, err := wsgit.NewClient().Diff(context.Background(), root, wsgit.DiffOptions{Range: rangeValue, Mode: mode, Paths: stringList(params.Arguments["paths"])})
		return toolJSONResponse(req.ID, result, err)
	case "git.log":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		rangeValue, _ := params.Arguments["range"].(string)
		includeBody, _ := params.Arguments["include_body"].(bool)
		result, err := wsgit.NewClient().Log(context.Background(), root, wsgit.LogOptions{Range: rangeValue, Limit: intFromArgument(params.Arguments["limit"], 20), IncludeBody: includeBody})
		return toolJSONResponse(req.ID, result, err)
	case "git.merge_base":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		base, _ := params.Arguments["base"].(string)
		head, _ := params.Arguments["head"].(string)
		result, err := wsgit.NewClient().MergeBase(context.Background(), root, base, head)
		return toolJSONResponse(req.ID, result, err)
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
			UpdatedTickets:      stringList(params.Arguments["updated_tickets"]),
			UpdatedSpecs:        stringList(params.Arguments["updated_specs"]),
			UpdatedMentalModels: stringList(params.Arguments["updated_mental_models"]),
		})
		return toolJSONResponse(req.ID, result, err)
	case "project_tree":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		text, err := wsdoc.ProjectTree(root)
		return toolTextResponse(req.ID, text, err)
	case "infra.read":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		name, _ := params.Arguments["name"].(string)
		text, err := wsdoc.ReadInfra(root, name)
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
		return toolJSONResponse(req.ID, result, err)
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
		return toolJSONResponse(req.ID, result, err)
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
		return toolJSONResponse(req.ID, result, err)
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
		return toolJSONResponse(req.ID, result, err)
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
		return toolJSONResponse(req.ID, result, err)
	case "references.trace":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		ticketStem, _ := params.Arguments["ticket_stem"].(string)
		specStem, _ := params.Arguments["spec_stem"].(string)
		result, err := wsdoc.ReferencesTrace(root, wsdoc.ReferenceTraceOptions{TicketStem: ticketStem, SpecStem: specStem})
		return toolJSONResponse(req.ID, result, err)
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
		return toolJSONResponse(req.ID, result, err)
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
		return toolJSONResponse(req.ID, result, err)
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
		return toolJSONResponse(req.ID, result, err)
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
		text, err := wsagent.NewManager(wsagent.Options{}).Subquery(wsagent.SubqueryOptions{
			Root:         root,
			Question:     question,
			DeepResearch: deepResearch,
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
	case "agents.register":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		name, _ := params.Arguments["name"].(string)
		backend, _ := params.Arguments["backend"].(string)
		tier, _ := params.Arguments["tier"].(string)
		model, _ := params.Arguments["model"].(string)
		systemPromptText, _ := params.Arguments["system_prompt_text"].(string)
		agent, _, err := wsagent.NewManager(wsagent.Options{}).Register(wsagent.RegisterOptions{
			Root:             root,
			Name:             name,
			Backend:          backend,
			Tier:             tier,
			Model:            model,
			Prompts:          stringList(params.Arguments["prompts"]),
			PromptRefs:       stringList(params.Arguments["prompt_refs"]),
			SystemPromptText: systemPromptText,
		})
		return toolTextResponse(req.ID, agent.Name+"\n", err)
	case "agents.call":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		name, _ := params.Arguments["name"].(string)
		prompt, _ := params.Arguments["prompt"].(string)
		result, err := wsagent.NewManager(wsagent.Options{}).Call(wsagent.CallOptions{
			Root:   root,
			Name:   name,
			Prompt: prompt,
		})
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		return toolTextResponse(req.ID, fmt.Sprintf("%s\t%s\tpid=%d\nfollow_up: agents.result --timeout 10m | agents.wait --timeout 10m | agents.status | agents.cancel\n", result.AgentName, result.Status, result.PID), nil)
	case "agents.wait":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		name, _ := params.Arguments["name"].(string)
		names := stringList(params.Arguments["names"])
		text, err := wsagent.NewManager(wsagent.Options{}).Wait(wsagent.WaitOptions{
			Root:    root,
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
		name, _ := params.Arguments["name"].(string)
		text, err := wsagent.NewManager(wsagent.Options{}).Result(wsagent.ResultOptions{
			Root:    root,
			Name:    name,
			Timeout: durationFromSeconds(params.Arguments["timeout_seconds"]),
			Context: ctx,
		})
		return toolTextResponse(req.ID, text, err)
	case "agents.status":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		name, _ := params.Arguments["name"].(string)
		text, err := wsagent.NewManager(wsagent.Options{}).Status(root, name)
		return toolTextResponse(req.ID, text, err)
	case "agents.interrupt":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		name, _ := params.Arguments["name"].(string)
		message, _ := params.Arguments["message"].(string)
		result, err := wsagent.NewManager(wsagent.Options{}).Interrupt(wsagent.InterruptOptions{
			Root:    root,
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
		name, _ := params.Arguments["name"].(string)
		lines := intFromArgument(params.Arguments["lines"], 40)
		text, err := wsagent.NewManager(wsagent.Options{}).Tail(wsagent.TailOptions{
			Root:  root,
			Name:  name,
			Lines: lines,
		})
		return toolTextResponse(req.ID, text, err)
	case "agents.debug.tail":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		name, _ := params.Arguments["name"].(string)
		lines := intFromArgument(params.Arguments["lines"], 40)
		text, err := wsagent.NewManager(wsagent.Options{}).Tail(wsagent.TailOptions{
			Root:  root,
			Name:  name,
			Lines: lines,
			Raw:   true,
		})
		return toolTextResponse(req.ID, text, err)
	case "agents.debug.stdout", "agents.debug.stderr", "agents.debug.runtime_log", "agents.debug.events":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		name, _ := params.Arguments["name"].(string)
		lines := intFromArgument(params.Arguments["lines"], 40)
		stream := strings.TrimPrefix(params.Name, "agents.debug.")
		text, err := wsagent.NewManager(wsagent.Options{}).DiagnosticStream(wsagent.DiagnosticStreamOptions{
			Root:   root,
			Name:   name,
			Stream: stream,
			Lines:  lines,
		})
		return toolTextResponse(req.ID, text, err)
	case "agents.cancel":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		name, _ := params.Arguments["name"].(string)
		text, err := wsagent.NewManager(wsagent.Options{}).Cancel(root, name)
		return toolTextResponse(req.ID, text, err)
	case "agents.print":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		name, _ := params.Arguments["name"].(string)
		text, err := wsagent.NewManager(wsagent.Options{}).Print(root, name)
		return toolTextResponse(req.ID, text, err)
	case "agents.erase":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		name, _ := params.Arguments["name"].(string)
		err = wsagent.NewManager(wsagent.Options{}).Erase(root, name)
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

func (s *Server) resolveToolRoot(arguments map[string]any, meta map[string]any) (string, error) {
	if value, ok := arguments["root"].(string); ok && strings.TrimSpace(value) != "" {
		return canonicalGitRoot(value)
	}

	s.rootMu.RLock()
	sessionRoot := s.sessionRoot
	s.rootMu.RUnlock()
	if sessionRoot != "" {
		return sessionRoot, nil
	}

	workspaces := codexWorkspaceRoots(meta)
	if len(workspaces) == 1 {
		return canonicalGitRoot(workspaces[0])
	}
	if len(workspaces) > 1 {
		return "", fmt.Errorf("multiple host workspaces are available; pass root explicitly or call session.set_default_root before using root-omitted ws tools")
	}

	if strings.TrimSpace(s.root) != "" && strings.TrimSpace(s.root) != "." {
		if root, err := canonicalGitRoot(s.root); err == nil {
			return root, nil
		}
	}

	if envRoot := strings.TrimSpace(os.Getenv("WS_MCP_PROJECT_ROOT")); envRoot != "" {
		return canonicalGitRoot(envRoot)
	}

	root, err := canonicalGitRoot(s.root)
	if err != nil {
		return "", fmt.Errorf("could not resolve a repository root from the MCP session; pass root explicitly or call session.set_default_root: %w", err)
	}
	return root, nil
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
				"type":       "object",
				"properties": map[string]any{},
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
			"name":        "session.set_default_root",
			"description": "Set the volatile repository root default for this MCP server process.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root": stringProperty("Git worktree root to use when later ws MCP tool calls omit root."),
				},
				"required": []string{"root"},
			},
		},
		{
			"name":        "session.get_default_root",
			"description": "Report the volatile repository root default and root fallback state for this MCP server process.",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			"name":        "api.list",
			"description": "Return sorted API documentation cache domain names under ai-docs/.deps.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root": stringProperty("Repository root. Defaults to the server root."),
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
			"name":        "config.show",
			"description": "Return the current ws user-local configuration and resolved config path without modifying it.",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			"name":        "config.agents_tier",
			"description": "Configure the default backend/model mapping for a ws agent workload tier.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tier":    enumStringProperty("Workload tier to configure.", []string{"light", "core", "deep"}),
					"backend": stringProperty("Optional backend name. When omitted, ws infers it from the model when possible."),
					"model":   stringProperty("Concrete model for this tier."),
				},
				"required": []string{"tier"},
			},
		},
		{
			"name":        "git.status",
			"description": "Return read-only Git branch and worktree status.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root": stringProperty("Repository root. Defaults to the server root."),
				},
			},
		},
		{
			"name":        "git.diff",
			"description": "Return read-only Git diff output. Defaults to stat mode; use mode=full for patch content.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":  stringProperty("Repository root. Defaults to the server root."),
					"range": stringProperty("Optional revision range."),
					"paths": stringArrayProperty("Optional path filters appended after --."),
					"mode":  enumStringProperty("Diff mode. Defaults to stat.", []string{"full", "stat", "name_only"}),
				},
			},
		},
		{
			"name":        "git.log",
			"description": "Return a bounded read-only Git commit log.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":         stringProperty("Repository root. Defaults to the server root."),
					"range":        stringProperty("Optional revision range."),
					"limit":        integerProperty("Maximum commits to return. Defaults to 20 and is capped at 100."),
					"include_body": boolProperty("Include commit body text."),
				},
			},
		},
		{
			"name":        "git.merge_base",
			"description": "Return the read-only Git merge-base hash for two revisions.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root": stringProperty("Repository root. Defaults to the server root."),
					"base": stringProperty("Base revision."),
					"head": stringProperty("Head revision."),
				},
				"required": []string{"base", "head"},
			},
		},
		{
			"name":        "git.commit",
			"description": "Create a workflow-aware Git commit from explicit paths and structured message fields.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":                  stringProperty("Repository root. Defaults to the server root."),
					"paths":                 stringArrayProperty("Explicit paths to stage and commit. Only these paths are staged."),
					"title":                 stringProperty("Single-line commit title."),
					"description":           stringProperty("Optional commit message body before AI Context."),
					"ai_context":            stringArrayProperty("Required AI Context bullets for the commit message."),
					"updated_tickets":       stringArrayProperty("Optional ticket update summaries. If omitted, staged ticket moves and Result headings are detected."),
					"updated_specs":         stringArrayProperty("Optional spec update summaries."),
					"updated_mental_models": stringArrayProperty("Optional mental-model update summaries."),
				},
				"required": []string{"paths", "title", "ai_context"},
			},
		},
		{
			"name":        "project_tree",
			"description": "Render the ws project document map, spec inventory, and active ticket queue.",
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
			"description": "Read a repository-local ws infra document by bare stem or filename.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]string{
						"type":        "string",
						"description": "Infra document stem or filename, for example ticket-conventions.",
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
			"description": "List spec files with frontmatter, anchors, ticket refs, and marker context metadata.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root": stringProperty("Repository root. Defaults to the server root."),
				},
			},
		},
		{
			"name":        "specs.find",
			"description": "Find spec files by query, spec anchor stem, or ticket stem reference.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":        stringProperty("Repository root. Defaults to the server root."),
					"query":       stringProperty("Optional case-insensitive text query."),
					"spec_stem":   stringProperty("Optional exact spec anchor stem."),
					"ticket_stem": stringProperty("Optional ticket stem referenced by spec frontmatter or feature entries."),
				},
			},
		},
		{
			"name":        "specs.status",
			"description": "Return locations and file metadata for one spec anchor stem.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":      stringProperty("Repository root. Defaults to the server root."),
					"spec_stem": stringProperty("Spec anchor stem to inspect."),
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
			"description": "Find mental-model paths by query, spec stem reference, or domain.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":      stringProperty("Repository root. Defaults to the server root."),
					"query":     stringProperty("Optional case-insensitive text query."),
					"spec_stem": stringProperty("Optional spec anchor stem referenced by the mental model."),
					"domain":    stringProperty("Optional mental-model domain."),
				},
			},
		},
		{
			"name":        "mental_models.status",
			"description": "Return path-first metadata for mental-model documents selected by domain or path.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":   stringProperty("Repository root. Defaults to the server root."),
					"domain": stringProperty("Optional mental-model domain."),
					"path":   stringProperty("Optional relative path under ai-docs/mental-model."),
				},
			},
		},
		{
			"name":        "references.trace",
			"description": "Trace ticket/spec/mental-model references from exactly one ticket_stem or spec_stem.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":        stringProperty("Repository root. Defaults to the server root."),
					"ticket_stem": stringProperty("Optional ticket stem to trace."),
					"spec_stem":   stringProperty("Optional spec anchor stem to trace."),
				},
			},
		},
		{
			"name":        "tickets.list",
			"description": "List ticket paths and structured status metadata without reading full document bodies.",
			"inputSchema": ticketDiscoverySchema(false),
		},
		{
			"name":        "tickets.find",
			"description": "Find ticket paths by query, ticket stem, or mentions of another ticket stem.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":                 stringProperty("Repository root. Defaults to the server root."),
					"statuses":             stringArrayProperty("Optional ticket statuses to scan: idea, todo, wip, done, dropped."),
					"include_done":         boolProperty("Include ai-docs/tickets/.done when true."),
					"include_dropped":      boolProperty("Include ai-docs/tickets/.dropped when true."),
					"query":                stringProperty("Optional case-insensitive text query."),
					"ticket_stem":          stringProperty("Optional exact ticket stem."),
					"mentions_ticket_stem": stringProperty("Optional ticket stem that result tickets must mention."),
				},
			},
		},
		{
			"name":        "tickets.status",
			"description": "Return structured status metadata for a single ticket stem.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":            stringProperty("Repository root. Defaults to the server root."),
					"ticket_stem":     stringProperty("Ticket stem to inspect."),
					"include_done":    boolProperty("Allow lookup under ai-docs/tickets/.done when true."),
					"include_dropped": boolProperty("Allow lookup under ai-docs/tickets/.dropped when true."),
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
					"root":          stringProperty("Repository root. Defaults to the server root."),
					"question":      stringProperty("Scoped question to answer."),
					"deep_research": boolProperty("Use deep workload tier for broad tracing or research."),
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
			"name":        "agents.register",
			"description": "Register a durable ws agent session for the current worktree.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":               stringProperty("Repository root. Defaults to the server root."),
					"name":               stringProperty("Agent name."),
					"backend":            stringProperty("Backend name. Defaults to codex."),
					"tier":               stringProperty("Workload tier: light, core, or deep. Defaults to core."),
					"model":              stringProperty("Optional concrete backend model override."),
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
					"root":   stringProperty("Repository root. Defaults to the server root."),
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
					"root":            stringProperty("Repository root. Defaults to the server root."),
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
					"root":            stringProperty("Repository root. Defaults to the server root."),
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
					"root": stringProperty("Repository root. Defaults to the server root."),
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
					"root":    stringProperty("Repository root. Defaults to the server root."),
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
					"root":  stringProperty("Repository root. Defaults to the server root."),
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
					"root": stringProperty("Repository root. Defaults to the server root."),
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
					"root": stringProperty("Repository root. Defaults to the server root."),
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
					"root": stringProperty("Repository root. Defaults to the server root."),
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
		if s.toolAllowed(name) {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

func (s *Server) toolAllowed(name string) bool {
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
		if strings.HasPrefix(name, "session.") {
			return false
		}
		if isSubqueryAgentTool(name) {
			return true
		}
		return !strings.HasPrefix(name, "agents.") && !strings.HasPrefix(name, "config.")
	case roleLeaf:
		return !strings.HasPrefix(name, "agents.") && !strings.HasPrefix(name, "config.") && !strings.HasPrefix(name, "session.") && !strings.HasPrefix(name, "api.") && name != "subquery" && name != "git.commit"
	default:
		return false
	}
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
			"root":  stringProperty("Repository root. Defaults to the server root."),
			"name":  stringProperty("Agent name."),
			"lines": integerProperty(linesDescription),
		},
		"required": []string{"name"},
	}
}

func ticketDiscoverySchema(requireTicketStem bool) map[string]any {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"root":            stringProperty("Repository root. Defaults to the server root."),
			"statuses":        stringArrayProperty("Optional ticket statuses to scan: idea, todo, wip, done, dropped."),
			"include_done":    boolProperty("Include ai-docs/tickets/.done when true."),
			"include_dropped": boolProperty("Include ai-docs/tickets/.dropped when true."),
		},
	}
	if requireTicketStem {
		schema["required"] = []string{"ticket_stem"}
	}
	return schema
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
