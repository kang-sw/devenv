// Claude process controls and stream input protocol.

package wsagent

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// claudeProtocol writes Claude Code's stream-json input lines.
type claudeProtocol struct{}

func (claudeProtocol) userLine(text string) any {
	return map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": text}}
}

func (claudeProtocol) requestLine(id string, request map[string]any) any {
	return map[string]any{"type": "control_request", "request_id": id, "request": request}
}

// claudeProcess adds Claude Code's control requests to a stream process.
type claudeProcess struct {
	*streamProcess
	// lead marks a lead conversation's process.
	lead bool
}

func (p claudeProcess) processControls() processControls {
	c := processControls{Process: p, steer: p}
	// Worker launches use dontAsk and have no in-stream permission-mode
	// or MCP recovery controls, despite sharing the lead's method set.
	if p.lead {
		c.approval, c.mode, c.mcp = p, p, p
	}
	return c
}

func (p claudeProcess) Interrupt() error {
	return p.request("interrupt", map[string]any{"subtype": "interrupt"})
}

// Steer writes text as a user message while a turn runs. Claude Code queues
// it and takes it up after the running tool call, inside the same turn:
// one result follows for both messages (seen on 2.1.322).
func (p claudeProcess) Steer(text string) error { return p.Send(text) }

// SetModel uses Claude Code's set_model control, which applies from the
// next turn. A set_model that names no model returns to the default one:
// the model a launch with no --model runs, whatever this launch named
// (seen on 2.1.322).
func (p claudeProcess) SetModel(model string) error {
	req := map[string]any{"subtype": "set_model"}
	if model != "" {
		req["model"] = model
	}
	return p.request("set_model", req)
}

// SetEffort changes the effort with Claude Code's apply_flag_settings
// control (2.1.322 answers it with a bare success), which applies from the
// next turn. The control merges into the settings the launch's --settings
// gave, so a profile's process keeps those. A null level clears the
// override and so returns to the default; the harness then runs the
// model's own default level until its next launch, which reads the user's
// settings again.
func (p claudeProcess) SetEffort(effort string) error {
	var level any
	if effort != "" {
		level = effort
	}
	return p.request("apply_flag_settings", map[string]any{"subtype": "apply_flag_settings", "settings": map[string]any{"effortLevel": level}})
}

// SetPermissionMode changes a lead's permission mode with Claude Code's
// set_permission_mode control. The mode the harness then runs in comes back
// on its stream (a status line's permissionMode).
func (p claudeProcess) SetPermissionMode(mode string) error {
	if !p.lead || mode == "" {
		return ErrUnsupported
	}
	return p.request("set_permission_mode", map[string]any{"subtype": "set_permission_mode", "mode": mode})
}

// mcpReconnectTimeout bounds the wait for an mcp_reconnect's answer, which
// comes once the server has started again.
var mcpReconnectTimeout = time.Minute

// MCPStatus asks a lead's harness for its MCP servers and their status
// (Claude Code's mcp_status control).
func (p claudeProcess) MCPStatus() ([]MCPServer, error) {
	if !p.lead {
		return nil, ErrUnsupported
	}
	body, err := p.requestBody("mcp_status", map[string]any{"subtype": "mcp_status"}, controlTimeout)
	if err != nil {
		return nil, err
	}
	var d struct {
		MCPServers []MCPServer `json:"mcpServers"`
	}
	if len(body) > 0 && json.Unmarshal(body, &d) != nil {
		return nil, errors.New("unreadable mcp_status answer")
	}
	return d.MCPServers, nil
}

// ReconnectMCP reconnects one of a lead's MCP servers in-stream (Claude
// Code's mcp_reconnect control, which answers once the server is back or
// failed again). The /mcp reconnect command is refused under print mode;
// the control is not.
func (p claudeProcess) ReconnectMCP(server string) error {
	if !p.lead {
		return ErrUnsupported
	}
	_, err := p.requestBody("mcp_reconnect", map[string]any{"subtype": "mcp_reconnect", "serverName": server}, mcpReconnectTimeout)
	return err
}

// AnswerApproval answers a can_use_tool request on the child's input: allow
// hands the tool call's input back unchanged, deny carries the message the
// model reads.
func (p claudeProcess) AnswerApproval(req ApprovalRequest, allow bool, message string) error {
	return p.writeLine(claudeApprovalResponse(req, allow, message))
}

// claudeApprovalResponse is the control_response line that answers req.
func claudeApprovalResponse(req ApprovalRequest, allow bool, message string) any {
	answer := map[string]any{"behavior": "deny", "message": message}
	if allow {
		input := req.Input
		if len(input) == 0 {
			input = json.RawMessage("{}")
		}
		answer = map[string]any{"behavior": "allow", "updatedInput": input}
	} else if strings.TrimSpace(message) == "" {
		answer["message"] = "The user denied this tool call."
	}
	return map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": req.ID, "response": answer}}
}

// claudeControlError is the control_response line that refuses the control
// request id with message.
func claudeControlError(id, message string) any {
	return map[string]any{"type": "control_response", "response": map[string]any{"subtype": "error", "request_id": id, "error": message}}
}
