package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"time"

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
	case "ws.project_tree":
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
		}
		text, err := wsdoc.ProjectTree(root)
		return toolTextResponse(req.ID, text, err)
	case "ws.infra.read":
		name, _ := params.Arguments["name"].(string)
		text, err := wsdoc.ReadInfra(s.root, name)
		return toolTextResponse(req.ID, text, err)
	case "ws.convention.read":
		name, _ := params.Arguments["name"].(string)
		text, err := wsdoc.ReadConvention(name)
		return toolTextResponse(req.ID, text, err)
	case "ws.spec_stem.generate":
		slug, _ := params.Arguments["slug"].(string)
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
		}
		stem, err := wsdoc.GenerateSpecStem(root, slug, time.Now())
		return toolTextResponse(req.ID, stem+"\n", err)
	case "ws.spec_index.verify":
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
		}
		text, err := wsdoc.VerifySpecIndex(root)
		return toolTextResponse(req.ID, text, err)
	case "ws.mental_models.list":
		root := s.root
		if value, ok := params.Arguments["root"].(string); ok && value != "" {
			root = value
		}
		text, err := wsdoc.MentalModelsList(root)
		return toolTextResponse(req.ID, text, err)
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
			"name":        "ws.project_tree",
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
			"name":        "ws.infra.read",
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
			"name":        "ws.convention.read",
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
			"name":        "ws.spec_stem.generate",
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
			"name":        "ws.spec_index.verify",
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
			"name":        "ws.mental_models.list",
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
	}
}
