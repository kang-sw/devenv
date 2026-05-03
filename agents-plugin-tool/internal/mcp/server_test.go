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
	}, "\n")

	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 responses, got %d\n%s", len(lines), out.String())
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
	for _, tool := range []string{"agents.wait", "agents.status", "agents.tail", "agents.cancel"} {
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
