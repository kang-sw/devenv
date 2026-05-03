package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestServeStdioToolsListAndCall(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	mustWrite(t, root, "ai-docs/spec/demo.md", "---\ntitle: Demo\n---\n# Demo\n")
	mustWrite(t, root, "ai-docs/tickets/todo/260503-feat-demo.md", "---\ntitle: Demo ticket\n---\n# Demo\n")
	mustWrite(t, root, "claude-plugin/infra/example.md", "example")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"project_tree","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"infra.read","arguments":{"name":"example"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"path.generate","arguments":{"kind":"review","stems":["direct"]}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"runtime.info","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"git.status","arguments":{}}}`,
	}, "\n")

	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 7 {
		t.Fatalf("expected 7 responses, got %d\n%s", len(lines), out.String())
	}

	var listResp map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &listResp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(lines[1], "project_tree") {
		t.Fatalf("tools/list missing project_tree: %s", lines[1])
	}
	if !strings.Contains(lines[1], "agents.call_async") {
		t.Fatalf("tools/list missing agents.call_async: %s", lines[1])
	}
	if !strings.Contains(lines[1], "subquery") {
		t.Fatalf("tools/list missing subquery: %s", lines[1])
	}
	if !strings.Contains(lines[1], "path.generate") {
		t.Fatalf("tools/list missing path.generate: %s", lines[1])
	}
	if !strings.Contains(lines[1], "runtime.info") {
		t.Fatalf("tools/list missing runtime.info: %s", lines[1])
	}
	if !strings.Contains(lines[1], "\"prompts\"") {
		t.Fatalf("tools/list missing prompts field: %s", lines[1])
	}
	for _, tool := range []string{"agents.wait", "agents.status", "agents.tail", "agents.cancel", "git.status", "git.diff", "git.log", "git.merge_base"} {
		if !strings.Contains(lines[1], tool) {
			t.Fatalf("tools/list missing %s: %s", tool, lines[1])
		}
	}
	if !strings.Contains(lines[2], "tickets:") {
		t.Fatalf("project_tree response missing tickets: %s", lines[2])
	}
	if !strings.Contains(lines[3], "example") {
		t.Fatalf("infra response missing example: %s", lines[3])
	}
	if !strings.Contains(lines[4], "review-paths") || !strings.Contains(lines[4], "-direct.md") {
		t.Fatalf("path.generate response missing review path: %s", lines[4])
	}
	if !strings.Contains(lines[5], "prompt_bundle") || !strings.Contains(lines[5], "code-reviewer") {
		t.Fatalf("runtime.info response missing prompt bundle: %s", lines[5])
	}
	if !strings.Contains(lines[6], "changed_files") || !strings.Contains(lines[6], "branch") {
		t.Fatalf("git.status response missing status JSON: %s", lines[6])
	}
}

func mustWrite(t *testing.T, root, rel, text string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func initGit(t *testing.T, root string) {
	t.Helper()
	cmd := exec.Command("git", "init")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, string(out))
	}
}

func TestServeStdioGitToolCalls(t *testing.T) {
	root := t.TempDir()
	initGit(t, root)
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test User")
	mustWrite(t, root, "file.txt", "one\n")
	runGit(t, root, "add", "file.txt")
	runGit(t, root, "commit", "-m", "initial", "-m", "body text")
	mustWrite(t, root, "file.txt", "one\ntwo\n")

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git.diff","arguments":{"mode":"name_only","paths":["file.txt"]}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"git.log","arguments":{"limit":1,"include_body":true}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"git.merge_base","arguments":{"base":"HEAD","head":"HEAD"}}}`,
	}, "\n")

	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 responses, got %d\n%s", len(lines), out.String())
	}

	var diff struct {
		Mode   string   `json:"mode"`
		Paths  []string `json:"paths"`
		Output string   `json:"output"`
	}
	if err := json.Unmarshal([]byte(toolText(t, lines[0])), &diff); err != nil {
		t.Fatal(err)
	}
	if diff.Mode != "name_only" || !strings.Contains(diff.Output, "file.txt") || len(diff.Paths) != 1 || diff.Paths[0] != "file.txt" {
		t.Fatalf("diff response = %#v", diff)
	}

	var log struct {
		Limit       int  `json:"limit"`
		IncludeBody bool `json:"include_body"`
		Commits     []struct {
			Subject string `json:"subject"`
			Body    string `json:"body"`
		} `json:"commits"`
	}
	if err := json.Unmarshal([]byte(toolText(t, lines[1])), &log); err != nil {
		t.Fatal(err)
	}
	if log.Limit != 1 || !log.IncludeBody || len(log.Commits) != 1 || log.Commits[0].Subject != "initial" || log.Commits[0].Body != "body text" {
		t.Fatalf("log response = %#v", log)
	}

	head := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
	var mergeBase struct {
		Base      string `json:"base"`
		Head      string `json:"head"`
		MergeBase string `json:"merge_base"`
	}
	if err := json.Unmarshal([]byte(toolText(t, lines[2])), &mergeBase); err != nil {
		t.Fatal(err)
	}
	if mergeBase.Base != "HEAD" || mergeBase.Head != "HEAD" || mergeBase.MergeBase != head {
		t.Fatalf("merge-base response = %#v, want hash %s", mergeBase, head)
	}
}

func toolText(t *testing.T, line string) string {
	t.Helper()
	var resp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Result.Content) != 1 {
		t.Fatalf("unexpected content in response: %s", line)
	}
	return resp.Result.Content[0].Text
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	_ = runGitOutput(t, root, args...)
}

func runGitOutput(t *testing.T, root string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
	return out
}
