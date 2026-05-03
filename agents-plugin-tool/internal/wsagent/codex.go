package wsagent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
)

type Runner interface {
	Call(RunnerRequest) (RunnerResult, error)
}

type RunnerRequest struct {
	Root             string
	Prompt           string
	Model            string
	SessionID        string
	SystemPromptPath string
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

	cmd := exec.Command("codex", args...)
	cmd.Dir = req.Root
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return RunnerResult{}, fmt.Errorf("codex failed: %w: %s", err, stderr.String())
		}
		return RunnerResult{}, fmt.Errorf("codex failed: %w", err)
	}
	result, err := parseCodexJSONL(stdout.Bytes())
	if err != nil {
		return RunnerResult{}, err
	}
	return result, nil
}

func parseCodexJSONL(raw []byte) (RunnerResult, error) {
	var result RunnerResult
	scanner := bufio.NewScanner(bytes.NewReader(raw))
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
