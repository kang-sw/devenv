package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsdoc"
)

type Server struct {
	root    string
	version string
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
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

func NewServer(root, version string) *Server {
	return &Server{root: filepath.Clean(root), version: version}
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
			if err := encoder.Encode(errorResponse(nil, -32700, "parse error")); err != nil {
				return err
			}
			continue
		}
		if len(req.ID) == 0 {
			continue
		}
		if err := encoder.Encode(s.handle(req)); err != nil {
			return err
		}
	}
	return scanner.Err()
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

func (s *Server) callTool(req request) response {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "invalid params")
	}

	switch params.Name {
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
