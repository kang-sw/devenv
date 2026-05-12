package wsagent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// CONTRACT: GeminiRunner is the ws named-agent backend adapter for Gemini CLI.
// It must keep registration, async lifecycle, diagnostics, cancel, and recall
// behavior on the shared Manager path; only Gemini invocation and stream-json
// parsing belong here.
type GeminiRunner struct{}

type geminiInvocation struct {
	Args           []string
	PromptStdin    string
	PromptDelivery string
}

func (GeminiRunner) Call(req RunnerRequest) (RunnerResult, error) {
	invocation, err := buildGeminiInvocation(req)
	if err != nil {
		return RunnerResult{}, err
	}

	backendVersion := geminiVersion()
	var ctx context.Context
	var cancel context.CancelFunc
	if req.Timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), req.Timeout)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()
	cmd := geminiCommandContext(ctx, invocation.Args)
	if !req.InheritProcessGroup {
		configureRunnerCommand(cmd)
	}
	cmd.Dir = req.Root
	if req.ToolProfile != "" {
		cmd.Env = append(cmd.Environ(), "WS_MCP_TOOL_PROFILE="+req.ToolProfile)
	}
	if invocation.PromptDelivery == "stdin" {
		cmd.Stdin = strings.NewReader(invocation.PromptStdin)
	}
	var stderr bytes.Buffer
	if req.Stderr != nil {
		cmd.Stderr = io.MultiWriter(&stderr, req.Stderr)
	} else {
		cmd.Stderr = &stderr
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return RunnerResult{}, fmt.Errorf("open gemini stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return RunnerResult{}, fmt.Errorf("start gemini: %w", err)
	}
	if req.Stdout != nil {
		stdout = struct {
			io.Reader
			io.Closer
		}{
			Reader: io.TeeReader(stdout, req.Stdout),
			Closer: stdout,
		}
	}
	result, parseErr := parseGeminiStreamJSON(stdout, req.OnSessionID)
	var callbackErr geminiSessionCallbackError
	if errors.As(parseErr, &callbackErr) {
		cancel()
		_ = stdout.Close()
	}
	waitErr := cmd.Wait()
	if errors.As(parseErr, &callbackErr) {
		return RunnerResult{}, parseErr
	}
	if waitErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return RunnerResult{}, fmt.Errorf("gemini timed out after %s", req.Timeout)
		}
		if stderr.Len() > 0 {
			return RunnerResult{}, fmt.Errorf("gemini failed: %w: %s", waitErr, stderr.String())
		}
		return RunnerResult{}, fmt.Errorf("gemini failed: %w", waitErr)
	}
	if parseErr != nil {
		return RunnerResult{}, parseErr
	}
	result.BackendVersion = backendVersion
	result.PromptDelivery = invocation.PromptDelivery
	return result, nil
}

func buildGeminiInvocation(req RunnerRequest) (geminiInvocation, error) {
	args := []string{"--output-format", "stream-json", "--approval-mode", "yolo"}
	if model := strings.TrimSpace(req.Model); model != "" && !isBackendShorthand(model) {
		args = append(args, "-m", model)
	}
	if sessionID := strings.TrimSpace(req.SessionID); sessionID != "" {
		args = append(args, "--resume", sessionID)
	}
	prompt := req.Prompt
	if req.SystemPromptPath != "" {
		systemPrompt, err := os.ReadFile(req.SystemPromptPath)
		if err != nil {
			return geminiInvocation{}, fmt.Errorf("read gemini system prompt: %w", err)
		}
		if strings.TrimSpace(string(systemPrompt)) != "" {
			prompt = "System instructions:\n\n" + string(systemPrompt) + "\n\nUser prompt:\n\n" + req.Prompt
		}
	}
	return geminiInvocation{
		Args:           args,
		PromptStdin:    prompt,
		PromptDelivery: "stdin",
	}, nil
}

func parseGeminiStreamJSON(r io.Reader, onSessionID func(string) error) (RunnerResult, error) {
	var result RunnerResult
	var terminalSeen bool
	var terminalErr error
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) == 0 && err == io.EOF {
			break
		}
		if err != nil && err != io.EOF {
			return RunnerResult{}, fmt.Errorf("read gemini stream-json: %w", err)
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			if err == io.EOF {
				break
			}
			continue
		}
		var event map[string]any
		if jsonErr := json.Unmarshal(line, &event); jsonErr != nil {
			if err == io.EOF {
				break
			}
			continue
		}
		if result.SessionID == "" {
			if sessionID := geminiSessionID(event); sessionID != "" {
				result.SessionID = sessionID
				if onSessionID != nil {
					if callbackErr := onSessionID(sessionID); callbackErr != nil {
						return RunnerResult{}, geminiSessionCallbackError{err: callbackErr}
					}
				}
			}
		}
		if chunk := geminiAssistantContent(event); chunk != "" {
			result.Text += chunk
			result.FinalEventShape = "message/assistant/content"
		}
		if status, err := geminiResultStatus(event); status != "" {
			terminalSeen = true
			result.FinalEventShape = "result/" + status
			switch status {
			case "success":
			case "error":
				terminalErr = geminiTerminalError(event)
			default:
				terminalErr = fmt.Errorf("gemini returned unexpected result status %q", status)
			}
		} else if err != nil {
			terminalSeen = true
			terminalErr = err
			result.FinalEventShape = "result/error"
		}
		if err == io.EOF {
			break
		}
	}
	if terminalErr != nil {
		return RunnerResult{}, terminalErr
	}
	if !terminalSeen {
		return RunnerResult{}, fmt.Errorf("gemini output missing terminal result")
	}
	if result.SessionID == "" {
		return RunnerResult{}, fmt.Errorf("gemini output missing init.session_id")
	}
	if result.Text == "" {
		return RunnerResult{}, fmt.Errorf("gemini output missing assistant message.content")
	}
	return result, nil
}

func geminiVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := geminiCommandContext(ctx, []string{"--version"}).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func geminiCommandContext(ctx context.Context, args []string) *exec.Cmd {
	exe, err := exec.LookPath("gemini")
	if err != nil {
		return exec.CommandContext(ctx, "gemini", args...)
	}
	if runtime.GOOS == "windows" {
		lower := strings.ToLower(exe)
		if strings.HasSuffix(lower, ".cmd") || strings.HasSuffix(lower, ".bat") {
			ps1 := strings.TrimSuffix(exe, filepath.Ext(exe)) + ".ps1"
			if _, err := os.Stat(ps1); err == nil {
				psArgs := append([]string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", ps1}, args...)
				return exec.CommandContext(ctx, "powershell", psArgs...)
			}
			cmdArgs := append([]string{"/d", "/c", "call", exe}, args...)
			return exec.CommandContext(ctx, "cmd", cmdArgs...)
		}
	}
	return exec.CommandContext(ctx, exe, args...)
}

func geminiSessionID(event map[string]any) string {
	if strings.EqualFold(stringField(event, "type"), "init") {
		if sessionID := stringField(event, "session_id"); sessionID != "" {
			return sessionID
		}
		if sessionID := stringField(event, "sessionId"); sessionID != "" {
			return sessionID
		}
	}
	for _, key := range []string{"init", "session"} {
		if nested, ok := mapField(event, key); ok {
			if sessionID := stringField(nested, "session_id"); sessionID != "" {
				return sessionID
			}
			if sessionID := stringField(nested, "sessionId"); sessionID != "" {
				return sessionID
			}
		}
	}
	return ""
}

func geminiAssistantContent(event map[string]any) string {
	if strings.EqualFold(stringField(event, "type"), "tool_use") ||
		strings.EqualFold(stringField(event, "type"), "tool_result") {
		return ""
	}
	message, ok := mapField(event, "message")
	if !ok && strings.EqualFold(stringField(event, "type"), "message") {
		message = event
		ok = true
	}
	if !ok {
		return ""
	}
	if messageType := stringField(message, "type"); strings.EqualFold(messageType, "tool_use") ||
		strings.EqualFold(messageType, "tool_result") {
		return ""
	}
	role := stringField(message, "role")
	if role == "" {
		role = stringField(event, "role")
	}
	if !strings.EqualFold(role, "assistant") {
		return ""
	}
	return textFromValue(message["content"])
}

func geminiResultStatus(event map[string]any) (string, error) {
	if result, ok := mapField(event, "result"); ok {
		if status := strings.ToLower(strings.TrimSpace(stringField(result, "status"))); status != "" {
			return status, nil
		}
	}
	if strings.EqualFold(stringField(event, "type"), "result") {
		if status := strings.ToLower(strings.TrimSpace(stringField(event, "status"))); status != "" {
			return status, nil
		}
		if result, ok := event["result"].(string); ok {
			switch status := strings.ToLower(strings.TrimSpace(result)); status {
			case "success", "error":
				return status, nil
			}
		}
	}
	return "", nil
}

func geminiTerminalError(event map[string]any) error {
	errorFields, ok := mapField(event, "error")
	if !ok {
		if result, resultOK := mapField(event, "result"); resultOK {
			errorFields, ok = mapField(result, "error")
		}
	}
	if !ok {
		return fmt.Errorf("gemini returned error")
	}
	errorType := strings.TrimSpace(stringField(errorFields, "type"))
	errorMessage := strings.TrimSpace(stringField(errorFields, "message"))
	switch {
	case errorType != "" && errorMessage != "":
		return fmt.Errorf("gemini returned error: %s: %s", errorType, errorMessage)
	case errorMessage != "":
		return fmt.Errorf("gemini returned error: %s", errorMessage)
	case errorType != "":
		return fmt.Errorf("gemini returned error: %s", errorType)
	default:
		return fmt.Errorf("gemini returned error")
	}
}

func textFromValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		var b strings.Builder
		for _, part := range typed {
			b.WriteString(textFromValue(part))
		}
		return b.String()
	case map[string]any:
		if partType := stringField(typed, "type"); strings.EqualFold(partType, "tool_use") ||
			strings.EqualFold(partType, "tool_result") {
			return ""
		}
		if text := stringField(typed, "text"); text != "" {
			return text
		}
		if content := textFromValue(typed["content"]); content != "" {
			return content
		}
	}
	return ""
}

func mapField(value map[string]any, key string) (map[string]any, bool) {
	field, ok := value[key]
	if !ok {
		return nil, false
	}
	nested, ok := field.(map[string]any)
	return nested, ok
}

func stringField(value map[string]any, key string) string {
	field, ok := value[key]
	if !ok {
		return ""
	}
	text, ok := field.(string)
	if !ok {
		return ""
	}
	return text
}

type geminiSessionCallbackError struct {
	err error
}

func (e geminiSessionCallbackError) Error() string {
	return fmt.Sprintf("handle gemini session id: %v", e.err)
}

func (e geminiSessionCallbackError) Unwrap() error {
	return e.err
}
