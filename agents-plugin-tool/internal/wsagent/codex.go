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
	Root                string
	Prompt              string
	Model               string
	SessionID           string
	SystemPromptPath    string
	Stdout              io.Writer
	Stderr              io.Writer
	OnSessionID         func(string) error
	Timeout             time.Duration
	InheritProcessGroup bool
}

type RunnerResult struct {
	SessionID string
	Text      string
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
	result, parseErr := parseCodexJSONLStream(stdout, req.OnSessionID)
	if err := cmd.Wait(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return RunnerResult{}, fmt.Errorf("codex timed out after %s", req.Timeout)
		}
		if stderr.Len() > 0 {
			return RunnerResult{}, fmt.Errorf("codex failed: %w: %s", err, stderr.String())
		}
		return RunnerResult{}, fmt.Errorf("codex failed: %w", err)
	}
	if parseErr != nil {
		return RunnerResult{}, parseErr
	}
	return result, nil
}

func parseCodexJSONL(raw []byte) (RunnerResult, error) {
	return parseCodexJSONLStream(bytes.NewReader(raw), nil)
}

func parseCodexJSONLStream(r io.Reader, onSessionID func(string) error) (RunnerResult, error) {
	var result RunnerResult
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		var event struct {
			Type     string `json:"type"`
			ThreadID string `json:"thread_id"`
			Item     struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
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
	}
	if err := scanner.Err(); err != nil {
		return RunnerResult{}, fmt.Errorf("read codex jsonl: %w", err)
	}
	if result.SessionID == "" {
		return RunnerResult{}, fmt.Errorf("codex output missing thread.started thread_id")
	}
	if result.Text == "" {
		return RunnerResult{}, fmt.Errorf("codex output missing agent_message text")
	}
	return result, nil
}
