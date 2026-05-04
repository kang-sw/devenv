package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsstate"
)

func TestServeStdioToolsListAndCall(t *testing.T) {
	useLeadProfile(t)
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
		`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"runtime.debug_events","arguments":{"limit":10}}}`,
		`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"config.show","arguments":{}}}`,
	}, "\n")

	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 9 {
		t.Fatalf("expected 9 responses, got %d\n%s", len(lines), out.String())
	}
	byID := responseLinesByID(t, lines)

	var listResp map[string]any
	if err := json.Unmarshal([]byte(byID["2"]), &listResp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(byID["2"], "project_tree") {
		t.Fatalf("tools/list missing project_tree: %s", byID["2"])
	}
	if !strings.Contains(byID["2"], "agents.call") {
		t.Fatalf("tools/list missing agents.call: %s", byID["2"])
	}
	if strings.Contains(byID["2"], "agents.call_async") {
		t.Fatalf("tools/list still includes agents.call_async: %s", byID["2"])
	}
	if strings.Contains(byID["2"], "agents.oneshot") {
		t.Fatalf("tools/list still includes agents.oneshot: %s", byID["2"])
	}
	if !strings.Contains(byID["2"], "subquery") {
		t.Fatalf("tools/list missing subquery: %s", byID["2"])
	}
	if !strings.Contains(byID["2"], "path.generate") {
		t.Fatalf("tools/list missing path.generate: %s", byID["2"])
	}
	if !strings.Contains(byID["2"], "runtime.info") {
		t.Fatalf("tools/list missing runtime.info: %s", byID["2"])
	}
	if !strings.Contains(byID["2"], "config.agents_tier") {
		t.Fatalf("tools/list missing config.agents_tier: %s", byID["2"])
	}
	if !strings.Contains(byID["2"], "config.show") {
		t.Fatalf("tools/list missing config.show: %s", byID["2"])
	}
	if !strings.Contains(byID["2"], "\"prompts\"") {
		t.Fatalf("tools/list missing prompts field: %s", byID["2"])
	}
	for _, tool := range []string{"agents.wait", "agents.status", "agents.tail", "agents.debug.tail", "agents.debug.stdout", "agents.debug.stderr", "agents.debug.runtime_log", "agents.debug.events", "agents.cancel", "git.status", "git.diff", "git.log", "git.merge_base"} {
		if !strings.Contains(byID["2"], tool) {
			t.Fatalf("tools/list missing %s: %s", tool, byID["2"])
		}
	}
	if !strings.Contains(byID["3"], "tickets:") {
		t.Fatalf("project_tree response missing tickets: %s", byID["3"])
	}
	if !strings.Contains(byID["4"], "example") {
		t.Fatalf("infra response missing example: %s", byID["4"])
	}
	if !strings.Contains(byID["5"], "review-paths") || !strings.Contains(byID["5"], "-direct.md") {
		t.Fatalf("path.generate response missing review path: %s", byID["5"])
	}
	if !strings.Contains(byID["6"], "prompt_bundle") || !strings.Contains(byID["6"], "code-reviewer") {
		t.Fatalf("runtime.info response missing prompt bundle: %s", byID["6"])
	}
	if !strings.Contains(byID["7"], "changed_files") || !strings.Contains(byID["7"], "branch") {
		t.Fatalf("git.status response missing status JSON: %s", byID["7"])
	}
	if !strings.Contains(toolText(t, byID["8"]), `"event":"request.received"`) {
		t.Fatalf("runtime.debug_events missing request evidence: %s", byID["8"])
	}
	configText := toolText(t, byID["9"])
	if !strings.Contains(configText, `"path"`) || !strings.Contains(configText, `config.json`) || !strings.Contains(configText, `"config"`) {
		t.Fatalf("config.show response missing path/config: %s", byID["9"])
	}
}

func TestServeStdioAgentDebugToolCalls(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	cache := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cache)
	_, layout, err := wsagent.NewManager(wsagent.Options{}).Register(wsagent.RegisterOptions{Root: root, Name: "impl"})
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Dir(layout.CurrentStdout), filepath.Base(layout.CurrentStdout), "stdout old\nstdout new\n")
	mustWrite(t, filepath.Dir(layout.CurrentRuntimeLog), filepath.Base(layout.CurrentRuntimeLog), "runtime old\nruntime new\n")

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"agents.debug.stdout","arguments":{"name":"impl","lines":1}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"agents.debug.runtime_log","arguments":{"name":"impl","lines":1}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"agents.debug.tail","arguments":{"name":"impl","lines":1}}}`,
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
	byID := responseLinesByID(t, lines)
	if got := toolText(t, byID["1"]); got != "stdout new\n" {
		t.Fatalf("stdout debug response = %q", got)
	}
	if got := toolText(t, byID["2"]); got != "runtime new\n" {
		t.Fatalf("runtime debug response = %q", got)
	}
	if got := toolText(t, byID["3"]); !strings.Contains(got, "== stdout ==") || !strings.Contains(got, "stdout new") || !strings.Contains(got, "== runtime ==") {
		t.Fatalf("debug tail response mismatch: %q", got)
	}
}

func TestServeStdioConfigShow(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	cache := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cache)

	var out bytes.Buffer
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.show","arguments":{}}}`+"\n",
	), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	showBefore := toolText(t, byID["1"])
	if !strings.Contains(showBefore, filepath.Join(cache, "config.json")) || !strings.Contains(showBefore, `"schema_version":1`) {
		t.Fatalf("config.show default response mismatch: %s", byID["1"])
	}

	if _, err := wsconfig.SetAgentsTier(wsconfig.Options{}, "light", "", "gemini-3-1-pro"); err != nil {
		t.Fatalf("SetAgentsTier returned error: %v", err)
	}
	out.Reset()
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"config.show","arguments":{}}}`+"\n",
	), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID = responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	showAfter := toolText(t, byID["2"])
	if !strings.Contains(showAfter, `"backend":"gemini"`) || !strings.Contains(showAfter, `"model":"gemini-3-1-pro"`) {
		t.Fatalf("config.show response missing tier mapping: %s", byID["2"])
	}
}

func TestServeStdioConfigAgentsTier(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	var out bytes.Buffer
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.agents_tier","arguments":{"tier":"light","model":"gemini-3-1-pro"}}}`+"\n",
	), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	configText := toolText(t, byID["1"])
	if !strings.Contains(configText, `"backend":"gemini"`) || !strings.Contains(configText, `"model":"gemini-3-1-pro"`) {
		t.Fatalf("config response missing tier mapping: %s", byID["1"])
	}

	out.Reset()
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"agents.register","arguments":{"name":"survey","tier":"light"}}}`+"\n",
	), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	status, err := wsagent.NewManager(wsagent.Options{}).Status(root, "survey")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, "backend: gemini") || !strings.Contains(status, "model: gemini-3-1-pro") {
		t.Fatalf("registered status missing configured backend/model:\n%s", status)
	}
}

func TestServeStdioLogsCancellationNotificationsWhenEnabled(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	debugLog := filepath.Join(t.TempDir(), "mcp-debug.jsonl")
	t.Setenv("WS_MCP_DEBUG_LOG", debugLog)

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":"wait-1","method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"wait-1","reason":"user interrupt"}}`,
	}, "\n")

	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected notification to produce no response, got %d lines\n%s", len(lines), out.String())
	}

	logBytes, err := os.ReadFile(debugLog)
	if err != nil {
		t.Fatal(err)
	}
	logText := string(logBytes)
	if !strings.Contains(logText, `"event":"request.received"`) ||
		!strings.Contains(logText, `"id":"wait-1"`) ||
		!strings.Contains(logText, `"event":"notification.cancelled"`) ||
		!strings.Contains(logText, `"request_id":"wait-1"`) ||
		!strings.Contains(logText, `"reason":"user interrupt"`) {
		t.Fatalf("debug log missing cancellation evidence:\n%s", logText)
	}
}

func TestServeStdioExposesCancellationNotificationsInDebugEvents(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":"wait-2","method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"wait-2","reason":"user interrupt"}}`,
		`{"jsonrpc":"2.0","id":"debug","method":"tools/call","params":{"name":"runtime.debug_events","arguments":{"limit":5}}}`,
	}, "\n")

	var out bytes.Buffer
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	debugText := toolText(t, byID["debug"])
	if !strings.Contains(debugText, `"event":"notification.cancelled"`) ||
		!strings.Contains(debugText, `"request_id":"wait-2"`) ||
		!strings.Contains(debugText, `"reason":"user interrupt"`) {
		t.Fatalf("runtime.debug_events missing cancellation evidence:\n%s", debugText)
	}
}

func TestServeStdioFiltersToolsByProfile(t *testing.T) {
	t.Setenv("WS_MCP_ALLOWED_TOOLS", "")
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_MCP_TOOL_PROFILE", "leaf")
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"agents.status","arguments":{"name":"impl"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"runtime.info","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"config.agents_tier","arguments":{"tier":"light","model":"gpt-5.2"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"config.show","arguments":{}}}`,
	}, "\n")

	var out bytes.Buffer
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if strings.Contains(byID["1"], "agents.status") || strings.Contains(byID["1"], "subquery") || strings.Contains(byID["1"], "config.agents_tier") || strings.Contains(byID["1"], "config.show") {
		t.Fatalf("leaf tools/list exposed recursive tools: %s", byID["1"])
	}
	if !strings.Contains(byID["1"], "runtime.info") {
		t.Fatalf("leaf tools/list hid runtime.info: %s", byID["1"])
	}
	if !strings.Contains(byID["2"], "tool not available") {
		t.Fatalf("leaf tools/call did not reject agents.status: %s", byID["2"])
	}
	if !strings.Contains(byID["3"], "prompt_bundle") {
		t.Fatalf("leaf tools/call rejected runtime.info: %s", byID["3"])
	}
	if !strings.Contains(byID["4"], "tool not available") {
		t.Fatalf("leaf tools/call did not reject config.agents_tier: %s", byID["4"])
	}
	if !strings.Contains(byID["5"], "tool not available") {
		t.Fatalf("leaf tools/call did not reject config.show: %s", byID["5"])
	}
}

func TestServeStdioNonOwnerCannotEscalateWithLeadProfile(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	cache := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cache)
	layout, _, worktree, err := wsstate.NewManager(wsstate.Options{}).Ensure(root)
	if err != nil {
		t.Fatal(err)
	}
	owner := startHelperProcess(t)
	lock := wsstate.OrchestratorLock{
		SchemaVersion: 1,
		PID:           owner.Process.Pid,
		StartedAt:     time.Now().UTC().Format(time.RFC3339),
		Root:          worktree.WorktreePath,
		WorktreeKey:   worktree.WorktreeKey,
		Version:       "test",
	}
	mustWrite(t, layout.WorktreeLocksDir, "orchestrator.lock", string(mustMarshalForTest(t, lock)))

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"agents.status","arguments":{"name":"impl"}}}`,
	}, "\n")

	var out bytes.Buffer
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if strings.Contains(byID["1"], "agents.status") || strings.Contains(byID["1"], "config.show") {
		t.Fatalf("non-owner lead profile exposed lead tools: %s", byID["1"])
	}
	if !strings.Contains(byID["1"], "subquery") {
		t.Fatalf("delegate profile unexpectedly hid subquery: %s", byID["1"])
	}
	if !strings.Contains(byID["2"], "tool not available") {
		t.Fatalf("non-owner tools/call did not reject agents.status: %s", byID["2"])
	}
}

func TestExplicitAllowedToolsCannotBypassEffectiveRole(t *testing.T) {
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_MCP_TOOL_PROFILE", "leaf")
	t.Setenv("WS_MCP_ALLOWED_TOOLS", "agents.status,runtime.info")

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"agents.status","arguments":{"name":"impl"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"runtime.info","arguments":{}}}`,
	}, "\n")

	var out bytes.Buffer
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if strings.Contains(byID["1"], "agents.status") {
		t.Fatalf("explicit allowlist bypassed leaf role in tools/list: %s", byID["1"])
	}
	if !strings.Contains(byID["1"], "runtime.info") {
		t.Fatalf("explicit allowlist hid runtime.info: %s", byID["1"])
	}
	if !strings.Contains(byID["2"], "tool not available") {
		t.Fatalf("explicit allowlist bypassed leaf role in tools/call: %s", byID["2"])
	}
	if !strings.Contains(byID["3"], "prompt_bundle") {
		t.Fatalf("explicit allowlist rejected allowed runtime.info: %s", byID["3"])
	}
}

func TestServeStdioDoesNotBlockToolsListBehindWait(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	agent, layout, err := wsagent.NewManager(wsagent.Options{}).Register(wsagent.RegisterOptions{Root: root, Name: "impl"})
	if err != nil {
		t.Fatal(err)
	}
	call, err := wsagent.NewManager(wsagent.Options{}).BeginCurrentCall(layout, agent)
	if err != nil {
		t.Fatal(err)
	}
	call.Status = wsagent.CallStatusRunning
	call.PID = os.Getpid()
	if err := os.WriteFile(layout.CurrentStateFile, mustMarshalForTest(t, call), 0o644); err != nil {
		t.Fatal(err)
	}

	reader, writer := io.Pipe()
	outReader, outWriter := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- NewServer(root, "test").ServeStdio(context.Background(), reader, outWriter)
		_ = outWriter.Close()
	}()

	fmt.Fprintln(writer, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"agents.wait","arguments":{"name":"impl","timeout_seconds":2}}}`)
	fmt.Fprintln(writer, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)

	lineCh := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(outReader)
		for scanner.Scan() {
			select {
			case lineCh <- scanner.Text():
			default:
			}
		}
	}()
	select {
	case line := <-lineCh:
		if !strings.Contains(line, `"id":2`) || !strings.Contains(line, "tools") {
			t.Fatalf("first response was not tools/list while wait was running: %s", line)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("tools/list was blocked behind agents.wait")
	}
	_ = writer.Close()
	_ = reader.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("ServeStdio did not exit after input close")
	}
}

func useLeadProfile(t *testing.T) {
	t.Helper()
	t.Setenv("WS_MCP_TOOL_PROFILE", "lead")
	t.Setenv("WS_MCP_ALLOWED_TOOLS", "")
}

func startHelperProcess(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	return cmd
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	time.Sleep(30 * time.Second)
	os.Exit(0)
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

func mustMarshalForTest(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
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
	useLeadProfile(t)
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
	byID := responseLinesByID(t, lines)

	var diff struct {
		Mode   string   `json:"mode"`
		Paths  []string `json:"paths"`
		Output string   `json:"output"`
	}
	if err := json.Unmarshal([]byte(toolText(t, byID["1"])), &diff); err != nil {
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
	if err := json.Unmarshal([]byte(toolText(t, byID["2"])), &log); err != nil {
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
	if err := json.Unmarshal([]byte(toolText(t, byID["3"])), &mergeBase); err != nil {
		t.Fatal(err)
	}
	if mergeBase.Base != "HEAD" || mergeBase.Head != "HEAD" || mergeBase.MergeBase != head {
		t.Fatalf("merge-base response = %#v, want hash %s", mergeBase, head)
	}
}

func responseLinesByID(t *testing.T, lines []string) map[string]string {
	t.Helper()
	byID := make(map[string]string, len(lines))
	for _, line := range lines {
		var resp struct {
			ID json.RawMessage `json:"id"`
		}
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatal(err)
		}
		byID[rawIDForTest(t, resp.ID)] = line
	}
	return byID
}

func rawIDForTest(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var number int
	if err := json.Unmarshal(raw, &number); err == nil {
		return strconv.Itoa(number)
	}
	return string(raw)
}

func toolIsError(t *testing.T, line string) bool {
	t.Helper()
	var resp struct {
		Result struct {
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.Result.IsError
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

type fakeAPIRuntime struct {
	mu          sync.Mutex
	routeCalls  []string
	routeOutput string
	routeErr    error
	managerHits []string
	answers     map[string]string
	errs        map[string]error
	active      map[string]int
	maxActive   map[string]int
	delay       time.Duration
}

func (f *fakeAPIRuntime) Route(ctx context.Context, root, prompt string) (string, error) {
	f.mu.Lock()
	f.routeCalls = append(f.routeCalls, prompt)
	output := f.routeOutput
	err := f.routeErr
	f.mu.Unlock()
	if output == "" {
		output = "go\npython\n"
	}
	return output, err
}

func (f *fakeAPIRuntime) AskManager(ctx context.Context, root, domain, prompt string) (string, error) {
	f.mu.Lock()
	if f.active == nil {
		f.active = map[string]int{}
	}
	if f.maxActive == nil {
		f.maxActive = map[string]int{}
	}
	f.managerHits = append(f.managerHits, domain+":"+prompt)
	f.active[domain]++
	if f.active[domain] > f.maxActive[domain] {
		f.maxActive[domain] = f.active[domain]
	}
	f.mu.Unlock()
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	f.mu.Lock()
	f.active[domain]--
	answer := "answer for " + domain
	if f.answers != nil && f.answers[domain] != "" {
		answer = f.answers[domain]
	}
	err := error(nil)
	if f.errs != nil {
		err = f.errs[domain]
	}
	f.mu.Unlock()
	return answer, err
}

func TestAPIListDomains(t *testing.T) {
	root := t.TempDir()
	useLeadProfile(t)
	initGit(t, root)
	server := NewServer(root, "test")
	domains, err := apiListDomains(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 0 {
		t.Fatalf("empty deps domains = %v", domains)
	}
	mustWrite(t, root, "ai-docs/.deps/go/README.md", "go")
	mustWrite(t, root, "ai-docs/.deps/.hidden/README.md", "hidden")
	mustWrite(t, root, "ai-docs/.deps/python/README.md", "python")
	mustWrite(t, root, "ai-docs/.deps/file.txt", "not dir")

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"api.list","arguments":{}}}` + "\n"
	var out bytes.Buffer
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	got := toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["1"])
	if got != "[\"go\",\"python\"]\n" {
		t.Fatalf("api.list = %q", got)
	}
}

func TestAPIAskMCPExactHintSkipsRouter(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	mustWrite(t, root, "ai-docs/.deps/go/README.md", "go")
	fake := &fakeAPIRuntime{answers: map[string]string{"go": "go answer"}}
	server := NewServer(root, "test")
	server.api = fake
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"api.ask","arguments":{"prompt":"How do modules work?","domain_hint":"go"}}}` + "\n"
	var out bytes.Buffer
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if !strings.Contains(byID["1"], "api.ask") || !strings.Contains(byID["1"], "api.list") {
		t.Fatalf("tools/list missing api tools: %s", byID["1"])
	}
	if toolIsError(t, byID["2"]) {
		t.Fatalf("api.ask returned tool error: %s", byID["2"])
	}
	text := toolText(t, byID["2"])
	if len(fake.routeCalls) != 0 {
		t.Fatalf("exact hint invoked pre-router: %v", fake.routeCalls)
	}
	if !strings.Contains(text, "## Domain: go") || !strings.Contains(text, "go answer") {
		t.Fatalf("api.ask response missing boundary/answer:\n%s", text)
	}
}

func TestAPIAskExistingDomainMentionStillUsesRouter(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/.deps/ratatui/README.md", "ratatui")
	fake := &fakeAPIRuntime{
		routeOutput: "ratatui\n",
		answers:     map[string]string{"ratatui": "ratatui answer"},
	}
	server := NewServer(root, "test")
	server.api = fake
	text, err := server.askAPI(context.Background(), root, "For ratatui, how do I render a widget?", "")
	if err != nil {
		t.Fatalf("askAPI returned error: %v\n%s", err, text)
	}
	if len(fake.routeCalls) != 1 {
		t.Fatalf("existing domain mention did not invoke pre-router: %v", fake.routeCalls)
	}
	if !strings.Contains(text, "## Domain: ratatui\nratatui answer") {
		t.Fatalf("api.ask response missing existing domain answer:\n%s", text)
	}
}

func TestAPIAskRecoversExistingDomainFromRouterProse(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/.deps/ratatui/README.md", "ratatui")
	fake := &fakeAPIRuntime{
		routeOutput: "Use frame.render_widget for ratatui widgets.\n",
		answers:     map[string]string{"ratatui": "ratatui answer"},
	}
	server := NewServer(root, "test")
	server.api = fake
	text, err := server.askAPI(context.Background(), root, "For ratatui, how do I render a widget?", "")
	if err != nil {
		t.Fatalf("askAPI returned error: %v\n%s", err, text)
	}
	if !strings.Contains(text, "## Domain: ratatui\nratatui answer") {
		t.Fatalf("api.ask response missing recovered existing domain answer:\n%s", text)
	}
}

func TestAPIAskPreRouterPartialFailureBoundaries(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/.deps/go/README.md", "go")
	mustWrite(t, root, "ai-docs/.deps/python/README.md", "python")
	fake := &fakeAPIRuntime{answers: map[string]string{"go": "go ok"}, errs: map[string]error{"python": fmt.Errorf("python unavailable")}}
	server := NewServer(root, "test")
	server.api = fake
	text, err := server.askAPI(context.Background(), root, "Compare clients", "")
	if err != nil {
		t.Fatalf("partial success should not error: %v\n%s", err, text)
	}
	if len(fake.routeCalls) != 1 || !strings.Contains(fake.routeCalls[0], "Existing domains:\ngo\npython\nPrompt: Compare clients") {
		t.Fatalf("pre-router input mismatch: %#v", fake.routeCalls)
	}
	if !strings.Contains(text, "## Domain: go\ngo ok") || !strings.Contains(text, "## Domain: python\nERROR: python unavailable") {
		t.Fatalf("partial response missing boundaries:\n%s", text)
	}
}

func TestAPIAskMCPAllDomainFailureReturnsToolErrorWithMetadata(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	fake := &fakeAPIRuntime{errs: map[string]error{"go": fmt.Errorf("go failed"), "python": fmt.Errorf("python failed")}}
	server := NewServer(root, "test")
	server.api = fake
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"api.ask","arguments":{"prompt":"question"}}}` + "\n"
	var out bytes.Buffer
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if !toolIsError(t, byID["1"]) {
		t.Fatalf("expected api.ask tool error: %s", byID["1"])
	}
	text := toolText(t, byID["1"])
	if !strings.Contains(text, "## Domain: go\nERROR: go failed") ||
		!strings.Contains(text, "## Domain: python\nERROR: python failed") ||
		!strings.Contains(text, "api.ask failed for all resolved domains") {
		t.Fatalf("all failure text missing metadata:\n%s", text)
	}
}

func TestAPIAskRejectsMalformedRouterDomainSlug(t *testing.T) {
	root := t.TempDir()
	fake := &fakeAPIRuntime{routeOutput: "1. go\ngo docs\ngo:latest\n"}
	server := NewServer(root, "test")
	server.api = fake
	_, err := server.askAPI(context.Background(), root, "question", "")
	if err == nil || !strings.Contains(err.Error(), "invalid domain") {
		t.Fatalf("expected invalid domain error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "ai-docs", ".deps", "1. go")); !os.IsNotExist(statErr) {
		t.Fatalf("malformed router output created cache directory, stat err=%v", statErr)
	}
}

func TestAPIManagerExpiredUsesRecentUseTimestamp(t *testing.T) {
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-apiManagerTTL + time.Second).Format(time.RFC3339)
	old := now.Add(-apiManagerTTL - time.Second).Format(time.RFC3339)
	if apiManagerExpired(wsagent.Agent{LastCallAt: recent, CreatedAt: old}, now) {
		t.Fatal("recent last call expired")
	}
	if !apiManagerExpired(wsagent.Agent{LastCallAt: old}, now) {
		t.Fatal("old last call did not expire")
	}
	if !apiManagerExpired(wsagent.Agent{LastSeenAt: old}, now) {
		t.Fatal("old last seen did not expire")
	}
}

func TestAPIAskSameDomainCallsSerialize(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/.deps/go/README.md", "go")
	fake := &fakeAPIRuntime{delay: 50 * time.Millisecond}
	server := NewServer(root, "test")
	server.api = fake
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := server.askAPI(context.Background(), root, "question", "go")
			if err != nil {
				t.Errorf("askAPI returned error: %v", err)
			}
		}()
	}
	wg.Wait()
	fake.mu.Lock()
	max := fake.maxActive["go"]
	fake.mu.Unlock()
	if max != 1 {
		t.Fatalf("same-domain calls were not serialized; max active = %d", max)
	}
}
