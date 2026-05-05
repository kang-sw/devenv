package wsagent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"time"
)

type Runner interface {
	Call(RunnerRequest) (RunnerResult, error)
}

type RunnerRequest struct {
	Root                 string
	Prompt               string
	Model                string
	SessionID            string
	SystemPromptPath     string
	InterruptHookCommand string
	Stdout               io.Writer
	Stderr               io.Writer
	OnSessionID          func(string) error
	Timeout              time.Duration
	InheritProcessGroup  bool
	ToolProfile          string
}

type RunnerResult struct {
	SessionID   string
	Text        string
	Interrupted bool
}

type CodexRunner struct{}

func (CodexRunner) Call(req RunnerRequest) (RunnerResult, error) {
	args := []string{"exec"}
	if req.SessionID != "" {
		args = append(args, "resume")
	}
	args = append(args, "--dangerously-bypass-approvals-and-sandbox", "--json")
	if req.Model != "" {
		args = append(args, "-m", req.Model)
	}
	if req.SystemPromptPath != "" {
		args = append(args, "-c", fmt.Sprintf("model_instructions_file=%q", req.SystemPromptPath))
	}
	if req.InterruptHookCommand != "" {
		hookCommand, err := json.Marshal(req.InterruptHookCommand)
		if err != nil {
			return RunnerResult{}, fmt.Errorf("quote interrupt hook command: %w", err)
		}
		hookTOML := fmt.Sprintf(`[{hooks=[{type="command",command=%s,timeout=5}]}]`, hookCommand)
		args = append(args, "-c", "features.codex_hooks=true", "-c", "hooks.PostToolUse="+hookTOML)
	}
	if req.SessionID != "" {
		args = append(args, req.SessionID)
	}
	args = append(args, req.Prompt)

	ctx := context.Background()
	var cancel context.CancelFunc
	if req.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "codex", args...)
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
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return RunnerResult{}, fmt.Errorf("open codex stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return RunnerResult{}, fmt.Errorf("start codex: %w", err)
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
	result, parseErr := parseCodexJSONLStreamPartial(stdout, req.OnSessionID)
	waitErr := cmd.Wait()
	if waitErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return RunnerResult{}, fmt.Errorf("codex timed out after %s", req.Timeout)
		}
		if stderr.Len() > 0 {
			return RunnerResult{}, fmt.Errorf("codex failed: %w: %s", waitErr, stderr.String())
		}
		return RunnerResult{}, fmt.Errorf("codex failed: %w", waitErr)
	}
	if parseErr != nil {
		return RunnerResult{}, parseErr
	}
	return result, nil
}

func parseCodexJSONL(raw []byte) (RunnerResult, error) {
	result, err := parseCodexJSONLStreamPartial(bytes.NewReader(raw), nil)
	if err != nil {
		return RunnerResult{}, err
	}
	return requireCompleteCodexResult(result)
}

func parseCodexJSONLStream(r io.Reader, onSessionID func(string) error) (RunnerResult, error) {
	result, err := parseCodexJSONLStreamPartial(r, onSessionID)
	if err != nil {
		return RunnerResult{}, err
	}
	return requireCompleteCodexResult(result)
}

func parseCodexJSONLStreamPartial(r io.Reader, onSessionID func(string) error) (RunnerResult, error) {
	var result RunnerResult
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) == 0 && err == io.EOF {
			break
		}
		if err != nil && err != io.EOF {
			return RunnerResult{}, fmt.Errorf("read codex jsonl: %w", err)
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			if err == io.EOF {
				break
			}
			continue
		}
		var event struct {
			Type     string `json:"type"`
			ThreadID string `json:"thread_id"`
			Item     struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
		}
		if err := json.Unmarshal(line, &event); err != nil {
			if result.SessionID != "" && result.Text != "" {
				if err == io.EOF {
					break
				}
				continue
			}
			return RunnerResult{}, fmt.Errorf("parse codex jsonl: %w", err)
		}
		if event.Type == "thread.started" && event.ThreadID != "" {
			result.SessionID = event.ThreadID
			if onSessionID != nil {
				if err := onSessionID(event.ThreadID); err != nil {
					return RunnerResult{}, fmt.Errorf("handle codex session id: %w", err)
				}
			}
		}
		if event.Type == "item.completed" && event.Item.Type == "agent_message" {
			result.Text = event.Item.Text
		}
		if err == io.EOF {
			break
		}
	}
	return result, nil
}

func requireCompleteCodexResult(result RunnerResult) (RunnerResult, error) {
	if result.SessionID == "" {
		return RunnerResult{}, fmt.Errorf("codex output missing thread.started thread_id")
	}
	if result.Text == "" {
		return RunnerResult{}, fmt.Errorf("codex output missing agent_message text")
	}
	return result, nil
}
