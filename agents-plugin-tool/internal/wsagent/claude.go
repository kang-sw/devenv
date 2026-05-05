package wsagent

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type ClaudeRunner struct{}

func (ClaudeRunner) Call(req RunnerRequest) (RunnerResult, error) {
	sessionID := strings.TrimSpace(req.SessionID)
	firstCall := sessionID == ""
	if firstCall {
		var err error
		sessionID, err = newClaudeSessionID()
		if err != nil {
			return RunnerResult{}, err
		}
	}

	args, err := claudeArgs(req, sessionID, firstCall, req.Prompt)
	if err != nil {
		return RunnerResult{}, err
	}

	if firstCall && req.OnSessionID != nil {
		if err := req.OnSessionID(sessionID); err != nil {
			return RunnerResult{}, fmt.Errorf("handle claude session id: %w", err)
		}
	}
	result, err := runClaude(req, args, sessionID)
	if err != nil {
		return RunnerResult{}, err
	}
	if result.Interrupted {
		resumeArgs, err := claudeArgs(req, sessionID, false, "Continue after applying the lead message delivered by the hook.")
		if err != nil {
			return RunnerResult{}, err
		}
		result, err = runClaude(req, resumeArgs, sessionID)
		if err != nil {
			return RunnerResult{}, err
		}
	}
	return result, nil
}

func claudeArgs(req RunnerRequest, sessionID string, firstCall bool, prompt string) ([]string, error) {
	args := []string{"-p", "--dangerously-skip-permissions", "--output-format", "json"}
	if model := strings.TrimSpace(req.Model); model != "" && !isBackendShorthand(model) {
		args = append(args, "--model", model)
	}
	if req.SystemPromptPath != "" {
		systemPrompt, err := os.ReadFile(req.SystemPromptPath)
		if err != nil {
			return nil, fmt.Errorf("read claude system prompt: %w", err)
		}
		if strings.TrimSpace(string(systemPrompt)) != "" {
			args = append(args, "--system-prompt", string(systemPrompt))
		}
	}
	if req.InterruptHookCommand != "" {
		settings, err := claudeSettingsJSON(req.InterruptHookCommand)
		if err != nil {
			return nil, err
		}
		args = append(args, "--settings", settings)
	}
	if firstCall {
		args = append(args, "--session-id", sessionID)
	} else {
		args = append(args, "--resume", sessionID)
	}
	args = append(args, prompt)
	return args, nil
}

func runClaude(req RunnerRequest, args []string, sessionID string) (RunnerResult, error) {
	ctx := context.Background()
	var cancel context.CancelFunc
	if req.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}
	cmd := claudeCommandContext(ctx, args)
	if !req.InheritProcessGroup {
		configureRunnerCommand(cmd)
	}
	cmd.Dir = req.Root
	if req.ToolProfile != "" {
		cmd.Env = append(cmd.Environ(), "WS_MCP_TOOL_PROFILE="+req.ToolProfile)
	}
	var stderr bytes.Buffer
	if req.Stderr != nil {
		cmd.Stderr = io.MultiWriter(&stderr, req.Stderr)
	} else {
		cmd.Stderr = &stderr
	}
	var stdout bytes.Buffer
	if req.Stdout != nil {
		cmd.Stdout = io.MultiWriter(&stdout, req.Stdout)
	} else {
		cmd.Stdout = &stdout
	}
	if err := cmd.Start(); err != nil {
		return RunnerResult{}, fmt.Errorf("start claude: %w", err)
	}
	waitErr := cmd.Wait()
	if waitErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return RunnerResult{}, fmt.Errorf("claude timed out after %s", req.Timeout)
		}
		if stderr.Len() > 0 {
			return RunnerResult{}, fmt.Errorf("claude failed: %w: %s", waitErr, stderr.String())
		}
		return RunnerResult{}, fmt.Errorf("claude failed: %w", waitErr)
	}
	result, err := parseClaudeJSON(stdout.Bytes(), sessionID)
	if err != nil {
		return RunnerResult{}, err
	}
	return result, nil
}

func claudeSettingsJSON(hookCommand string) (string, error) {
	settings := map[string]any{
		"hooks": map[string]any{
			"PostToolBatch": []map[string]any{
				{
					"hooks": []map[string]any{
						{
							"type":    "command",
							"command": hookCommand,
							"timeout": 5,
						},
					},
				},
			},
		},
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return "", fmt.Errorf("encode claude settings: %w", err)
	}
	return string(raw), nil
}

func parseClaudeJSON(raw []byte, sessionID string) (RunnerResult, error) {
	var event struct {
		Result         string `json:"result"`
		IsError        bool   `json:"is_error"`
		Error          string `json:"error"`
		TerminalReason string `json:"terminal_reason"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(raw), &event); err != nil {
		return RunnerResult{}, fmt.Errorf("parse claude json: %w", err)
	}
	if event.IsError {
		if strings.TrimSpace(event.Result) != "" {
			return RunnerResult{}, fmt.Errorf("claude returned error: %s", event.Result)
		}
		if strings.TrimSpace(event.Error) != "" {
			return RunnerResult{}, fmt.Errorf("claude returned error: %s", event.Error)
		}
		return RunnerResult{}, fmt.Errorf("claude returned error")
	}
	if event.TerminalReason == "hook_stopped" {
		return RunnerResult{SessionID: sessionID, Interrupted: true}, nil
	}
	return RunnerResult{SessionID: sessionID, Text: event.Result}, nil
}

func newClaudeSessionID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate claude session id: %w", err)
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16]), nil
}

func isBackendShorthand(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "claude", "codex", "gemini":
		return true
	default:
		return false
	}
}

func claudeCommandContext(ctx context.Context, args []string) *exec.Cmd {
	exe, err := exec.LookPath("claude")
	if err != nil {
		return exec.CommandContext(ctx, "claude", args...)
	}
	if runtime.GOOS == "windows" {
		lower := strings.ToLower(exe)
		if strings.HasSuffix(lower, ".cmd") || strings.HasSuffix(lower, ".bat") {
			line := windowsCmdQuote(exe)
			for _, arg := range args {
				line += " " + windowsCmdQuote(arg)
			}
			return exec.CommandContext(ctx, "cmd", "/d", "/s", "/c", line)
		}
	}
	return exec.CommandContext(ctx, exe, args...)
}

func windowsCmdQuote(value string) string {
	if value == "" {
		return `""`
	}
	value = strings.ReplaceAll(value, "%", "%%")
	var b strings.Builder
	b.WriteByte('"')
	backslashes := 0
	for _, r := range value {
		switch r {
		case '\\':
			backslashes++
		case '"':
			b.WriteString(strings.Repeat(`\`, backslashes*2+1))
			b.WriteRune(r)
			backslashes = 0
		default:
			if backslashes > 0 {
				b.WriteString(strings.Repeat(`\`, backslashes))
				backslashes = 0
			}
			b.WriteRune(r)
		}
	}
	if backslashes > 0 {
		b.WriteString(strings.Repeat(`\`, backslashes*2))
	}
	b.WriteByte('"')
	return b.String()
}
