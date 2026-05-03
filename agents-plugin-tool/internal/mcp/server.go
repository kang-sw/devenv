package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	role := effectiveToolRole(cleanRoot, version)
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
			"protocolVersion": "2025-03-26",
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
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "invalid params")
	}
	if !s.toolAllowed(params.Name) {
		return errorResponse(req.ID, -32601, fmt.Sprintf("tool not available in current ws MCP profile: %s", params.Name))
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
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
		}
		result, err := wsgit.NewClient().Status(context.Background(), root)
		return toolJSONResponse(req.ID, result, err)
	case "git.diff":
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
		}
		rangeValue, _ := params.Arguments["range"].(string)
		mode, _ := params.Arguments["mode"].(string)
		result, err := wsgit.NewClient().Diff(context.Background(), root, wsgit.DiffOptions{Range: rangeValue, Mode: mode, Paths: stringList(params.Arguments["paths"])})
		return toolJSONResponse(req.ID, result, err)
	case "git.log":
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
		}
		rangeValue, _ := params.Arguments["range"].(string)
		includeBody, _ := params.Arguments["include_body"].(bool)
		result, err := wsgit.NewClient().Log(context.Background(), root, wsgit.LogOptions{Range: rangeValue, Limit: intFromArgument(params.Arguments["limit"], 20), IncludeBody: includeBody})
		return toolJSONResponse(req.ID, result, err)
	case "git.merge_base":
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
		}
		base, _ := params.Arguments["base"].(string)
		head, _ := params.Arguments["head"].(string)
		result, err := wsgit.NewClient().MergeBase(context.Background(), root, base, head)
		return toolJSONResponse(req.ID, result, err)
	case "project_tree":
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
		}
		text, err := wsdoc.ProjectTree(root)
		return toolTextResponse(req.ID, text, err)
	case "infra.read":
		name, _ := params.Arguments["name"].(string)
		text, err := wsdoc.ReadInfra(s.root, name)
		return toolTextResponse(req.ID, text, err)
	case "convention.read":
		name, _ := params.Arguments["name"].(string)
		text, err := wsdoc.ReadConvention(name)
		return toolTextResponse(req.ID, text, err)
	case "spec_stem.generate":
		slug, _ := params.Arguments["slug"].(string)
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
		}
		stem, err := wsdoc.GenerateSpecStem(root, slug, time.Now())
		return toolTextResponse(req.ID, stem+"\n", err)
	case "spec_index.verify":
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
		}
		text, err := wsdoc.VerifySpecIndex(root)
		return toolTextResponse(req.ID, text, err)
	case "mental_models.list":
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
		}
		text, err := wsdoc.MentalModelsList(root)
		return toolTextResponse(req.ID, text, err)
	case "subquery":
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
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
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
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
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
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
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
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
		return toolTextResponse(req.ID, fmt.Sprintf("%s\t%s\tpid=%d\nfollow_up: agents.wait | agents.status | agents.cancel\n", result.AgentName, result.Status, result.PID), nil)
	case "agents.wait":
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
		}
		name, _ := params.Arguments["name"].(string)
		text, err := wsagent.NewManager(wsagent.Options{}).Wait(wsagent.WaitOptions{
			Root:    root,
			Name:    name,
			Timeout: durationFromSeconds(params.Arguments["timeout_seconds"]),
			Context: ctx,
		})
		return toolTextResponse(req.ID, text, err)
	case "agents.status":
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
		}
		name, _ := params.Arguments["name"].(string)
		text, err := wsagent.NewManager(wsagent.Options{}).Status(root, name)
		return toolTextResponse(req.ID, text, err)
	case "agents.tail":
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
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
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
		}
		name, _ := params.Arguments["name"].(string)
		lines := intFromArgument(params.Arguments["lines"], 40)
		text, err := wsagent.NewManager(wsagent.Options{}).Tail(wsagent.TailOptions{
			Root:  root,
			Name:  name,
			Lines: lines,
		})
		return toolTextResponse(req.ID, text, err)
	case "agents.debug.stdout", "agents.debug.stderr", "agents.debug.runtime_log", "agents.debug.events":
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
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
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
		}
		name, _ := params.Arguments["name"].(string)
		text, err := wsagent.NewManager(wsagent.Options{}).Cancel(root, name)
		return toolTextResponse(req.ID, text, err)
	case "agents.print":
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
		}
		name, _ := params.Arguments["name"].(string)
		text, err := wsagent.NewManager(wsagent.Options{}).Print(root, name)
		return toolTextResponse(req.ID, text, err)
	case "agents.erase":
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
		}
		name, _ := params.Arguments["name"].(string)
		err := wsagent.NewManager(wsagent.Options{}).Erase(root, name)
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
		return response{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"isError": true,
			"content": []map[string]string{{
				"type": "text",
				"text": err.Error(),
			}},
		}}
	}
	return response{JSONRPC: "2.0", ID: id, Result: map[string]any{
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
			"description": "Return read-only Git diff output in full, stat, or name-only mode.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":  stringProperty("Repository root. Defaults to the server root."),
					"range": stringProperty("Optional revision range."),
					"paths": stringArrayProperty("Optional path filters appended after --."),
					"mode":  enumStringProperty("Diff mode.", []string{"full", "stat", "name_only"}),
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
			"name":        "subquery",
			"description": "Run a scoped one-turn codebase or documentation query through a temporary ws delegate.",
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
			"description": "Wait for the current async call for a registered ws agent.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":            stringProperty("Repository root. Defaults to the server root."),
					"name":            stringProperty("Agent name."),
					"timeout_seconds": numberProperty("Maximum seconds to wait. Defaults to no timeout."),
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
			"name":        "agents.tail",
			"description": "Return recent event, stream, and output lines for a registered ws agent.",
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
			"description": "Return the last plain-text output for a registered ws agent.",
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

func effectiveToolRole(root, version string) toolRole {
	base := roleDelegate
	lock, err := wsstate.NewManager(wsstate.Options{}).AcquireOrchestratorLock(root, version)
	if err != nil {
		appendDebugEvent("orchestrator_lock.error", map[string]any{"root": root, "error": err.Error()})
	} else if lock.Owner || lock.Lock.PID == os.Getpid() {
		base = roleLead
		appendDebugEvent("orchestrator_lock.owner", map[string]any{"root": root, "path": lock.Path})
	} else {
		appendDebugEvent("orchestrator_lock.delegate", map[string]any{"root": root, "path": lock.Path, "owner_pid": lock.Lock.PID})
	}
	return minRole(base, requestedToolRole())
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

func minRole(base, requested toolRole) toolRole {
	if roleRank(requested) < roleRank(base) {
		return requested
	}
	return base
}

func roleRank(role toolRole) int {
	switch role {
	case roleLeaf:
		return 0
	case roleDelegate:
		return 1
	default:
		return 2
	}
}

func roleAllowsTool(role toolRole, name string) bool {
	switch role {
	case roleLead:
		return true
	case roleDelegate:
		return !strings.HasPrefix(name, "agents.") && !strings.HasPrefix(name, "config.")
	case roleLeaf:
		return !strings.HasPrefix(name, "agents.") && !strings.HasPrefix(name, "config.") && name != "subquery"
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
