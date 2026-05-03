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
	"time"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsdoc"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsprompt"
	"github.com/kang-sw/devenv/internal/wsstate"
)

type Server struct {
	root         string
	version      string
	sourceCommit string
}

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

func NewServer(root, version string, sourceCommit ...string) *Server {
	commit := "dev"
	if len(sourceCommit) > 0 && sourceCommit[0] != "" {
		commit = sourceCommit[0]
	}
	return &Server{root: filepath.Clean(root), version: version, sourceCommit: commit}
}

func (s *Server) ServeStdio(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	encoder := json.NewEncoder(out)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
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
			if err := encoder.Encode(errorResponse(nil, -32700, "parse error")); err != nil {
				return err
			}
			continue
		}
		if len(req.ID) == 0 {
			s.handleNotification(req)
			continue
		}
		appendDebugEvent("request.received", map[string]any{"id": rawMessageString(req.ID), "method": req.Method})
		if err := encoder.Encode(s.handle(req)); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (s *Server) handleNotification(req request) {
	switch req.Method {
	case "notifications/cancelled":
		var params cancelledNotificationParams
		fields := map[string]any{"method": req.Method}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			fields["error"] = err.Error()
		} else {
			fields["request_id"] = rawMessageString(params.RequestID)
			if params.Reason != "" {
				fields["reason"] = params.Reason
			}
		}
		appendDebugEvent("notification.cancelled", fields)
	default:
		appendDebugEvent("notification.ignored", map[string]any{"method": req.Method})
	}
}

func (s *Server) handle(req request) response {
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
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": tools()}}
	case "tools/call":
		return s.callTool(req)
	default:
		return errorResponse(req.ID, -32601, "method not found")
	}
}

func appendDebugEvent(event string, fields map[string]any) {
	path := strings.TrimSpace(os.Getenv("WS_MCP_DEBUG_LOG"))
	if path == "" {
		return
	}
	record := map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"event": event,
	}
	for key, value := range fields {
		record[key] = value
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

func (s *Server) callTool(req request) response {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "invalid params")
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
		_, text, err := wsagent.NewManager(wsagent.Options{}).Call(wsagent.CallOptions{
			Root:   root,
			Name:   name,
			Prompt: prompt,
		})
		return toolTextResponse(req.ID, text, err)
	case "agents.call_async":
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
		}
		name, _ := params.Arguments["name"].(string)
		prompt, _ := params.Arguments["prompt"].(string)
		result, err := wsagent.NewManager(wsagent.Options{}).CallAsync(wsagent.CallAsyncOptions{
			Root:   root,
			Name:   name,
			Prompt: prompt,
		})
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		return toolTextResponse(req.ID, fmt.Sprintf("%s\t%s\tpid=%d\n", result.AgentName, result.Status, result.PID), nil)
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
	case "agents.oneshot":
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
		}
		name, _ := params.Arguments["name"].(string)
		backend, _ := params.Arguments["backend"].(string)
		tier, _ := params.Arguments["tier"].(string)
		model, _ := params.Arguments["model"].(string)
		systemPromptText, _ := params.Arguments["system_prompt_text"].(string)
		prompt, _ := params.Arguments["prompt"].(string)
		text, err := wsagent.NewManager(wsagent.Options{}).OneShot(wsagent.OneShotOptions{
			Root:             root,
			Name:             name,
			Backend:          backend,
			Tier:             tier,
			Model:            model,
			Prompts:          stringList(params.Arguments["prompts"]),
			PromptRefs:       stringList(params.Arguments["prompt_refs"]),
			SystemPromptText: systemPromptText,
			Prompt:           prompt,
		})
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
			"description": "Call a registered ws agent and resume its stored backend session when available.",
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
			"name":        "agents.call_async",
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
			"name":        "agents.oneshot",
			"description": "Register, call, and erase a temporary ws agent.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":               stringProperty("Repository root. Defaults to the server root."),
					"name":               stringProperty("Optional temporary agent name."),
					"backend":            stringProperty("Backend name. Defaults to codex."),
					"tier":               stringProperty("Workload tier: light, core, or deep. Defaults to core."),
					"model":              stringProperty("Optional concrete backend model override."),
					"prompts":            stringArrayProperty("Embedded prompt stems or absolute prompt paths."),
					"prompt_refs":        stringArrayProperty("Logical role prompt references."),
					"system_prompt_text": stringProperty("Optional materialized system prompt text."),
					"prompt":             stringProperty("Prompt to send to the temporary agent."),
				},
				"required": []string{"prompt"},
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
