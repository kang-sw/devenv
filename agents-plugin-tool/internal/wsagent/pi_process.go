// Pi process controls, RPC input, and recovery replies.

package wsagent

import (
	"encoding/json"
	"fmt"
	"github.com/kang-sw/devenv/internal/wsinbox"
	"strings"
	"sync"
)

// piProtocol writes Pi's RPC commands.
type piProtocol struct{}

func (piProtocol) userLine(text string) any {
	return map[string]any{"type": "prompt", "message": text}
}

func (piProtocol) requestLine(id string, request map[string]any) any {
	out := map[string]any{"id": id}
	for k, v := range request {
		out[k] = v
	}
	return out
}

// piProcess adds Pi's RPC commands to a stream process.
type piProcess struct {
	*streamProcess
	mapper *piMapper

	mu     sync.Mutex
	effort string
	// lead marks a lead conversation's process.
	lead bool
}

func (p *piProcess) processControls() processControls {
	return processControls{Process: p, steer: p, reports: p, commands: p, turns: p, recovery: p}
}

func (p *piProcess) Send(text string) error {
	p.mapper.aborted.Store(false)
	if rest, ok := piCompactCommand(text); p.lead && ok {
		// A typed /compact is no RPC built-in: it is the compact command.
		return p.compact(rest)
	}
	return p.streamProcess.Send(text)
}

// RunCommand runs a lead's dashboard slash command: /compact is the compact
// command, with its arguments as the instructions; any other command is a
// prompt of its typed line, which pi expands (an extension command, a
// prompt template, a skill). It is sent at a turn's end only, never as a
// steer or follow-up, which refuse extension commands.
func (p *piProcess) RunCommand(cmd wsinbox.Command) error {
	if cmd.Name == "compact" {
		p.mapper.aborted.Store(false)
		return p.compact(cmd.Args)
	}
	return p.Send(cmd.Line())
}

// compact writes the compact command, with instructions when there are any.
func (p *piProcess) compact(instructions string) error {
	line := map[string]any{"type": "compact"}
	if instructions = strings.TrimSpace(instructions); instructions != "" {
		line["customInstructions"] = instructions
	}
	return p.writeLine(line)
}

// piCompactCommand reports whether text is a typed /compact, bare or in a
// single timestamped user envelope, and returns the instructions after it.
// Other input is checked unchanged against the bare command rules.
func piCompactCommand(text string) (string, bool) {
	if messages, ok := wsinbox.ParseUserMessages(text); ok && len(messages) == 1 && messages[0].At != "" {
		text = messages[0].Text
	}
	text = strings.TrimSpace(text)
	if text == "/compact" {
		return "", true
	}
	if rest, ok := strings.CutPrefix(text, "/compact "); ok && !strings.Contains(rest, "\n") {
		return strings.TrimSpace(rest), true
	}
	return "", false
}

// Steer is pi's steer command: the running agent takes the message after
// its current tool calls, before its next model request.
func (p *piProcess) Steer(text string) error {
	return p.writeLine(map[string]any{"type": "steer", "message": text})
}

// ClearSteered is pi's clear_queue command: it drops the steered messages
// pi queued and no run took, once the run that would have taken them
// ended.
func (p *piProcess) ClearSteered() error {
	return p.request("clear_queue", map[string]any{"type": "clear_queue"})
}

// piUserText is a user message's text, as pi matches a queued message to
// the one its run took: a string content, or its text blocks joined.
func piUserText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var blocks []piBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "")
}

// Interrupt aborts the running prompt. Pi answers abort only once the run
// settles, so it is sent without waiting; the turn ends at agent_settled.
func (p *piProcess) Interrupt() error {
	p.mapper.aborted.Store(true)
	return p.writeLine(map[string]any{"type": "abort"})
}

// SetModel uses Pi's set_model command for an exact provider/model id; a
// fuzzy pattern, a thinking suffix, or a return to the default needs the
// --model of a restart. set_model resets the thinking level, so a set
// effort is applied again.
func (p *piProcess) SetModel(model string) error {
	provider, id, ok := strings.Cut(model, "/")
	if !ok || provider == "" || id == "" || strings.Contains(id, ":") {
		return ErrUnsupported
	}
	if err := p.request("set_model", map[string]any{"type": "set_model", "provider": provider, "modelId": id}); err != nil {
		return err
	}
	p.mu.Lock()
	effort := p.effort
	p.mu.Unlock()
	if effort == "" {
		return nil
	}
	return p.request("set_thinking_level", map[string]any{"type": "set_thinking_level", "level": effort})
}

// SetEffort uses Pi's set_thinking_level command; a return to the default
// level needs a restart.
func (p *piProcess) SetEffort(effort string) error {
	if effort == "" {
		return ErrUnsupported
	}
	if err := p.request("set_thinking_level", map[string]any{"type": "set_thinking_level", "level": effort}); err != nil {
		return err
	}
	p.mu.Lock()
	p.effort = effort
	p.mu.Unlock()
	return nil
}

// TurnRunning asks pi's get_state whether a run is under way: isStreaming
// stays set until agent_settled (a retry, a compaction, and a queued
// continuation included), and isCompacting covers a compact command, whose
// answer ends the runner's turn. An answer without isStreaming is an
// error, not an idle harness.
func (p *piProcess) TurnRunning() (bool, error) {
	id := p.nextID()
	p.feedMu.Lock()
	if p.mapper.nativeProbes == nil {
		p.mapper.nativeProbes = make(map[string]uint64)
	}
	p.mapper.nativeProbes[id] = p.mapper.nativeGeneration
	p.feedMu.Unlock()
	defer func() {
		p.feedMu.Lock()
		delete(p.mapper.nativeProbes, id)
		p.feedMu.Unlock()
	}()
	body, err := p.requestBodyID(id, "get_state", map[string]any{"type": "get_state"}, controlTimeout)
	if err != nil {
		return false, err
	}
	var d struct {
		IsStreaming  *bool `json:"isStreaming"`
		IsCompacting bool  `json:"isCompacting"`
	}
	if json.Unmarshal(body, &d) != nil || d.IsStreaming == nil {
		return false, fmt.Errorf("pi's get_state answer reports no isStreaming: %.200s", body)
	}
	return *d.IsStreaming || d.IsCompacting, nil
}

// recoveryReply reads Pi's saved RPC messages only after get_state says idle.
// The final message must be an assistant result, not an unfinished tool call.
func (p *piProcess) recoveryReply(startedAt int64) (string, string, []string, error) {
	body, err := p.requestBody("get_messages", map[string]any{"type": "get_messages"}, controlTimeout)
	if err != nil {
		return "", "", nil, err
	}
	var data struct {
		Messages []piMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", "", nil, err
	}
	if len(data.Messages) == 0 {
		return "", "", nil, fmt.Errorf("no saved final assistant message after a replay gap")
	}
	last := data.Messages[len(data.Messages)-1]
	if startedAt == 0 || last.Timestamp < startedAt || last.Role != "assistant" || (last.StopReason != "stop" && last.StopReason != "error" && last.StopReason != "aborted") {
		return "", "", nil, fmt.Errorf("saved messages do not identify a final assistant result after a replay gap")
	}
	var taken []string
	for _, message := range data.Messages {
		if message.Role == "user" && message.Timestamp >= startedAt {
			taken = append(taken, piUserText(message.Content))
		}
	}
	turnErr := ""
	if last.StopReason == "error" {
		turnErr = strings.TrimSpace(last.ErrorMessage)
		if turnErr == "" {
			turnErr = "turn failed"
		}
	}
	return piUserText(last.Content), turnErr, taken, nil
}
