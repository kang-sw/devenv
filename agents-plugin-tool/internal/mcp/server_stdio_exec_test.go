package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestServeStdioDoesNotBlockToolsListBehindLongCall pins that ServeStdio
// dispatches concurrently: a tools/call that blocks (here exec.result waiting
// on a still-running job) must not delay a tools/list that arrives after it.
// A serialized dispatch loop would answer id 1 first and stall the client.
func TestServeStdioDoesNotBlockToolsListBehindLongCall(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	var spawnOut bytes.Buffer
	spawnServer := NewServer(root, "test")
	// This test's own pass condition depends on the spawned job still being
	// "running" when the exec.result call below (timeout_seconds:2) is
	// issued, not just on a nominal duration: if the job finishes first,
	// exec.result returns immediately and can win the first-output-line race
	// against tools/list, flipping the assertion below for a reason unrelated
	// to the dispatch-ordering behavior this test pins. mcpLongShellArgs's
	// ~6s nominal duration leaves under ~1s of slack after the 5s
	// ForegroundWindow launch plus this function's own pipe/session setup, so
	// use mcpAbortShellArgs's much longer duration instead for a wide margin
	// (same rationale as its doc comment). Reap it via t.Cleanup — LIFO,
	// registered after the WS_CACHE_HOME t.Setenv above so it runs first —
	// so the still-running job doesn't race root's t.TempDir() RemoveAll on
	// Windows (see reapExecKeys and TestExecMCPRunningLargeAndAbort).
	if err := serveStdioWithSession(t, spawnServer, root, toolCallLine(t, 1, "exec.shell", mcpAbortShellArgs())+"\n", &spawnOut); err != nil {
		t.Fatal(err)
	}
	running := execKeyFromText(t, toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(spawnOut.String()), "\n"))["1"]))
	t.Cleanup(func() { reapExecKeys(t, spawnServer, root, []string{running}) })

	reader, writer := io.Pipe()
	outReader, outWriter := io.Pipe()
	done := make(chan error, 1)
	streamServer := NewServer(root, "test")
	go func() {
		done <- streamServer.ServeStdio(context.Background(), reader, outWriter)
		_ = outWriter.Close()
	}()

	key, _ := parseLoginResponse(t, callLogin(t, streamServer, 900002, root, nil))
	fmt.Fprintln(writer, fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"exec.result","arguments":{"exec_key":%q,"timeout_seconds":2,"session_key":%q}}}`, running, key))
	fmt.Fprintln(writer, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)

	lineCh := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(outReader)
		// The tools/list line is ~60 KB, near the default 64 KB token cap;
		// past it the scanner stops and the test would misreport a block.
		scanner.Buffer(nil, 16<<20)
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
			t.Fatalf("first response was not tools/list while the long call was running: %s", line)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("tools/list was blocked behind a long-running tools/call")
	}
	_ = writer.Close()
	_ = reader.Close()
	// ServeStdio exits only after the in-flight exec.result returns: up to
	// its 2s timeout plus a final poll of the job record (file I/O). A loaded
	// Windows runner overran a 3s bound here with this path unchanged, so the
	// bound only has to tell a slow exit from one that never comes.
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("ServeStdio did not exit after input close")
	}
}

func TestExecToolsListNoAgentAndMCPFlow(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "README.md", "x\n")
	mustWrite(t, root, "sub/.keep", "x\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
		toolCallLine(t, 2, "exec.spawn", map[string]any{"cmd": os.Args[0], "args": []string{"-test.run=TestMCPExecHelperProcess", "--", "flow"}, "env": map[string]string{"GO_WANT_MCP_EXEC_HELPER": "1"}, "working_dir": "sub"}),
		toolCallLine(t, 9, "exec.shell", mcpShellShapeArgs()),
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := serveStdioWithSession(t, NewServer(root, "test"), root, input, &out); err != nil {
		t.Fatal(err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	// exec.* tools are permanently hidden from the public MCP surface in all modes.
	for _, name := range []string{"exec.spawn", "exec.shell", "exec.status", "exec.result", "exec.abort", "exec.raw.tail", "exec.raw.read", "exec.raw.grep"} {
		if strings.Contains(byID["1"], name) {
			t.Fatalf("tools/list should not advertise exec tool %s: %s", name, byID["1"])
		}
	}
	text := toolText(t, byID["2"])
	shellText := toolText(t, byID["9"])
	if !strings.Contains(shellText, "shell-shape") || !containsResolvedPath(shellText, root) {
		t.Fatalf("shell response = %s", byID["9"])
	}
	if strings.HasPrefix(strings.TrimSpace(text), "{") || !strings.Contains(text, "status: succeeded") || !containsResolvedPath(text, filepath.Join(root, "sub")) || !strings.Contains(text, "========== stdout ==========") || !strings.Contains(text, "========== stderr ==========") || !strings.Contains(text, "err") {
		t.Fatalf("spawn response = %s", byID["2"])
	}
	launch := execToolResponse{ExecKey: execKeyFromText(t, text)}

	input = strings.Join([]string{
		fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"exec.status","arguments":{"exec_key":%q}}}`, launch.ExecKey),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"exec.result","arguments":{"exec_key":%q}}}`, launch.ExecKey),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"exec.raw.tail","arguments":{"exec_key":%q,"stream":"stdout","lines":1}}}`, launch.ExecKey),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"exec.raw.read","arguments":{"exec_key":%q,"stream":"stderr","offset":0,"limit":20}}}`, launch.ExecKey),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"exec.raw.grep","arguments":{"exec_key":%q,"pattern":"beta42"}}}`, launch.ExecKey),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"exec.raw.grep","arguments":{"exec_key":%q,"pattern":"beta[0-9]+","regex":true}}}`, launch.ExecKey),
	}, "\n") + "\n"
	out.Reset()
	if err := serveStdioWithSession(t, NewServer(root, "test"), root, input, &out); err != nil {
		t.Fatal(err)
	}
	byID = responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	for id, want := range map[string]string{"3": "result_ready:", "4": "========== stdout ==========", "5": "beta42", "6": "next_offset:", "7": "beta42", "8": "beta42"} {
		if !strings.Contains(byID[id], want) {
			t.Fatalf("response %s missing %s: %s", id, want, byID[id])
		}
	}
	tailText := toolText(t, byID["5"])
	if strings.HasPrefix(strings.TrimSpace(tailText), "{") || !strings.Contains(tailText, "exec_key: ") || !strings.Contains(tailText, "stream: stdout") || !strings.Contains(tailText, "========== text ==========") {
		t.Fatalf("tail response was not readable text: %s", byID["5"])
	}
	readText := toolText(t, byID["6"])
	if strings.HasPrefix(strings.TrimSpace(readText), "{") || !strings.Contains(readText, "stream: stderr") || !strings.Contains(readText, "offset: 0") || !strings.Contains(readText, "========== text ==========") {
		t.Fatalf("read response was not readable text: %s", byID["6"])
	}
	for _, id := range []string{"7", "8"} {
		grepText := toolText(t, byID[id])
		if strings.HasPrefix(strings.TrimSpace(grepText), "{") || !strings.Contains(grepText, "stream: stdout") || !strings.Contains(grepText, "matches: 1") || !strings.Contains(grepText, "========== matches ==========") {
			t.Fatalf("grep response %s was not readable text: %s", id, byID[id])
		}
	}

	t.Setenv("WS_MCP_NO_AGENT", "1")
	t.Setenv("WS_MCP_NAMESPACE", "wsflow")
	out.Reset()
	input = `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}` + "\n" + `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"exec.spawn","arguments":{"cmd":"echo"}}}` + "\n" + `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"exec.raw.tail","arguments":{"exec_key":"exec-1-0000000000000000"}}}` + "\n"
	if err := serveStdioWithSession(t, NewServer(root, "test"), root, input, &out); err != nil {
		t.Fatal(err)
	}
	byID = responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if strings.Contains(byID["1"], "exec.spawn") || strings.Contains(byID["1"], "exec.raw.tail") {
		t.Fatalf("no-agent listed exec tools: %s", byID["1"])
	}
	if !strings.Contains(byID["2"], "agentless mode disables") || !strings.Contains(byID["3"], "agentless mode disables") {
		t.Fatalf("no-agent calls not rejected: %s\n%s", byID["2"], byID["3"])
	}
}

func TestExecMCPRunningLargeAndAbort(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "README.md", "x\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	server := NewServer(root, "test")
	var execKeys []string
	// t.Cleanup is LIFO across ALL cleanups registered in this test, not just
	// t.TempDir()'s own. Registering the reap here — after the WS_CACHE_HOME
	// t.Setenv and its backing t.TempDir("cache") above, and after root's own
	// t.TempDir() earlier still — makes it the LAST registration, so it is the
	// FIRST to run: before WS_CACHE_HOME is restored, before the cache dir is
	// removed, and before root's own RemoveAll. That ordering matters twice
	// over: exec.status/exec.abort resolve the job store through WS_CACHE_HOME
	// and root, so reaping must happen while both are still the test's live
	// values; and every exec job this test spawns (the running/aborted job and
	// the large job) must reach a terminal status before root's RemoveAll
	// runs, because a still-"running" job on Windows holds root as its
	// child's CWD and keeps open stdout/stderr/combined file handles under
	// it — RemoveAll racing that live process fails with "The process cannot
	// access the file because it is being used by another process" (POSIX
	// unlink tolerates this; Windows does not). Registering the reap earlier
	// (e.g. immediately after root's t.TempDir()) would run it too early:
	// LIFO would then unwind WS_CACHE_HOME's restore and the cache dir's
	// RemoveAll first, and exec.status/exec.abort would report the job "not
	// found" against the now-reset cache home.
	t.Cleanup(func() { reapExecKeys(t, server, root, execKeys) })

	input := toolCallLine(t, 1, "exec.shell", mcpAbortShellArgs()) + "\n"
	var out bytes.Buffer
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatal(err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	text := toolText(t, byID["1"])
	if strings.HasPrefix(strings.TrimSpace(text), "{") || !strings.Contains(text, "status: running") || !strings.Contains(text, "exec.ask") || strings.Contains(text, "done") {
		t.Fatalf("running launch response = %s", byID["1"])
	}
	running := execToolResponse{ExecKey: execKeyFromText(t, text)}
	execKeys = append(execKeys, running.ExecKey)
	out.Reset()
	input = strings.Join([]string{
		fmt.Sprintf(`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"exec.result","arguments":{"exec_key":%q}}}`, running.ExecKey),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"exec.result","arguments":{"exec_key":%q,"timeout_seconds":0}}}`, running.ExecKey),
	}, "\n") + "\n"
	started := time.Now()
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatal(err)
	}
	// Both result calls are non-blocking (default and explicit timeout_seconds:0),
	// so they must not wait for the long-running job. The job has ~25s left at
	// this point, so a regression that blocked on completion would take that
	// long; the generous 5s ceiling cleanly catches it while tolerating
	// serveStdioWithSession setup jitter under full-suite scheduling pressure
	// (observed ~1.16s spikes that tripped the old sub-second budget).
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("non-blocking result calls took %s", elapsed)
	}
	resultByID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	for _, id := range []string{"10", "11"} {
		resultText := toolText(t, resultByID[id])
		if strings.Contains(resultByID[id], `"isError":true`) || strings.HasPrefix(strings.TrimSpace(resultText), "{") || !strings.Contains(resultText, "status: running") || !strings.Contains(resultText, "guidance:") || strings.Contains(resultText, "done") {
			t.Fatalf("non-blocking result %s = %s", id, resultByID[id])
		}
	}
	out.Reset()
	input = fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"exec.abort","arguments":{"exec_key":%q}}}`, running.ExecKey) + "\n"
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	var abortText string
	for time.Now().Before(deadline) {
		out.Reset()
		input = fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"exec.status","arguments":{"exec_key":%q}}}`, running.ExecKey) + "\n"
		if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
			t.Fatal(err)
		}
		abortText = toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["3"])
		if strings.Contains(abortText, "status: cancelled") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(abortText, "status: cancelled") {
		t.Fatalf("abort status = %s", abortText)
	}

	out.Reset()
	input = toolCallLine(t, 4, "exec.shell", mcpLargeShellArgs()) + "\n"
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatal(err)
	}
	launchText := toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["4"])
	large := execToolResponse{ExecKey: execKeyFromText(t, launchText)}
	execKeys = append(execKeys, large.ExecKey)

	// Do not assert combined_bytes off the synchronous launch response: on a
	// loaded Windows runner, powershell cold-start can exceed the 5s
	// ForegroundWindow (execjob.ForegroundWindow), so the launch can still
	// return "status: running" / "combined_bytes: 0" while the job keeps
	// writing. Drive exec.result with a generous timeout so it polls
	// (execjob.ResultWithTimeout) until the job reaches a terminal status —
	// only then is combined_bytes final and exec.abort's t.Cleanup-time reap
	// guaranteed a closed file handle.
	out.Reset()
	input = fmt.Sprintf(`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"exec.result","arguments":{"exec_key":%q,"timeout_seconds":30}}}`, large.ExecKey) + "\n"
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatal(err)
	}
	largeText := toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["5"])
	if strings.Contains(largeText, strings.Repeat("x", 100)) || !strings.Contains(largeText, "status: succeeded") || !strings.Contains(largeText, "combined_bytes: 5000") || !strings.Contains(largeText, "exec.raw.*") {
		t.Fatalf("large response = %s", largeText)
	}
}

// reapExecKeys aborts and waits for every exec job this test spawned to reach
// a terminal status. It is registered via t.Cleanup as the last registration
// in TestExecMCPRunningLargeAndAbort (see the comment at that call site for
// why) so it runs, LIFO, before WS_CACHE_HOME is restored and before Go's own
// TempDir RemoveAll cleanups, which on Windows cannot unlink a directory a
// live child process still holds as its CWD or has open file handles under.
// exec.abort on an already-terminal job is a documented no-op
// (execjob.Abort), so calling it unconditionally for every key — including
// one already confirmed cancelled by the test body — is safe.
func reapExecKeys(t *testing.T, server *Server, root string, keys []string) {
	t.Helper()
	for _, key := range keys {
		var out bytes.Buffer
		abortInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":990,"method":"tools/call","params":{"name":"exec.abort","arguments":{"exec_key":%q}}}`, key) + "\n"
		if err := serveStdioWithSession(t, server, root, abortInput, &out); err != nil {
			t.Logf("reapExecKeys: abort %s: %v", key, err)
			continue
		}
		deadline := time.Now().Add(10 * time.Second)
		for {
			out.Reset()
			statusInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":991,"method":"tools/call","params":{"name":"exec.status","arguments":{"exec_key":%q}}}`, key) + "\n"
			if err := serveStdioWithSession(t, server, root, statusInput, &out); err != nil {
				t.Logf("reapExecKeys: status %s: %v", key, err)
				break
			}
			text := out.String()
			if strings.Contains(text, "status: succeeded") || strings.Contains(text, "status: failed") || strings.Contains(text, "status: cancelled") {
				break
			}
			if time.Now().After(deadline) {
				t.Logf("reapExecKeys: %s did not reach terminal status before cleanup deadline: %s", key, text)
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func TestExecMCPResultReadableJSONStdoutAndTimeout(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "README.md", "x\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	server := NewServer(root, "test")

	jsonArgs := map[string]any{"command": `printf '{"ok":true}\n'`}
	if runtime.GOOS == "windows" {
		jsonArgs = map[string]any{"shell": "powershell", "command": `Write-Output '{"ok":true}'`}
	}
	var out bytes.Buffer
	if err := serveStdioWithSession(t, server, root, toolCallLine(t, 1, "exec.shell", jsonArgs)+"\n", &out); err != nil {
		t.Fatal(err)
	}
	text := toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["1"])
	if strings.HasPrefix(strings.TrimSpace(text), "{") || !strings.Contains(text, "========== stdout ==========") || !strings.Contains(text, `{"ok":true}`) || strings.Contains(text, `\"ok\"`) {
		t.Fatalf("json stdout response = %s", text)
	}

	out.Reset()
	if err := serveStdioWithSession(t, server, root, toolCallLine(t, 2, "exec.shell", mcpLongShellArgs())+"\n", &out); err != nil {
		t.Fatal(err)
	}
	running := execKeyFromText(t, toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["2"]))
	// Reap before root's t.TempDir() cleanup, LIFO (registered after the
	// WS_CACHE_HOME t.Setenv above so it runs first), matching
	// TestExecMCPRunningLargeAndAbort: on an early t.Fatal below, or if the
	// generous timeout_seconds budget below is ever exhausted, a still-live
	// child holding root as its CWD would otherwise race Windows' RemoveAll.
	t.Cleanup(func() { reapExecKeys(t, server, root, []string{running}) })
	out.Reset()
	// Do not race mcpLongShellArgs's ~6s nominal duration against a tight
	// timeout_seconds budget: on a loaded Windows runner the synchronous
	// exec.shell launch can itself consume most of execjob.ForegroundWindow
	// (5s) before the job even starts running its command, so a thin
	// remaining margin (the previous timeout_seconds:3, ~2s of slack) flakes
	// under CI load. Use a generous multiple of ForegroundWindow + the job's
	// nominal duration so exec.result's poll loop (execjob.ResultWithTimeout)
	// reaches the job's terminal status well before the budget expires,
	// matching the precedent pattern in TestExecMCPRunningLargeAndAbort.
	input := fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"exec.result","arguments":{"exec_key":%q,"timeout_seconds":30}}}`, running) + "\n"
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatal(err)
	}
	waitText := toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["3"])
	if !strings.Contains(waitText, "status: succeeded") || !strings.Contains(waitText, "done") {
		t.Fatalf("timeout result = %s", waitText)
	}
}

type execToolResponse struct {
	ExecKey string `json:"exec_key"`
}

func execKeyFromText(t *testing.T, text string) string {
	t.Helper()
	re := regexp.MustCompile(`(?m)^exec_key: (exec-(?:[0-9a-f]{8}|[0-9]+-[0-9a-f]{16}))$`)
	match := re.FindStringSubmatch(text)
	if len(match) != 2 {
		t.Fatalf("exec_key not found in text: %s", text)
	}
	return match[1]
}

func toolCallLine(t *testing.T, id int, name string, arguments map[string]any) string {
	t.Helper()
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      name,
			"arguments": arguments,
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func mcpShellShapeArgs() map[string]any {
	if runtime.GOOS == "windows" {
		return map[string]any{"command": "cd && echo shell-shape"}
	}
	return map[string]any{"command": "pwd; printf shell-shape"}
}

func mcpLongShellArgs() map[string]any {
	if runtime.GOOS == "windows" {
		return map[string]any{"command": "echo start & ping -n 7 127.0.0.1 >NUL & echo done"}
	}
	return map[string]any{"command": "echo start; sleep 6; echo done"}
}

// mcpAbortShellArgs is a deliberately long-running job for abort tests. It must
// outlive the 5s ForegroundWindow plus the non-blocking result calls and the
// abort round-trip by a wide margin, so the abort always lands while the job is
// still running instead of racing its natural completion (the prior sleep-6
// command left only ~1s after the foreground window, which flaked to
// "status: succeeded" under full-suite load). The abort terminates it promptly,
// so the test does not actually wait the full duration.
func mcpAbortShellArgs() map[string]any {
	if runtime.GOOS == "windows" {
		return map[string]any{"command": "echo start & ping -n 31 127.0.0.1 >NUL & echo done"}
	}
	return map[string]any{"command": "echo start; sleep 30; echo done"}
}

func mcpLargeShellArgs() map[string]any {
	if runtime.GOOS == "windows" {
		return map[string]any{"shell": "powershell", "command": "[Console]::Out.Write(('x' * 5000))"}
	}
	return map[string]any{"command": "i=0; while [ $i -lt 5000 ]; do printf x; i=$((i+1)); done"}
}

// containsResolvedPath reports whether haystack mentions directory p, tolerating
// the gap between the path t.TempDir() returns and the form a shell prints for
// the same directory. On Windows CI the temp root can come back as an 8.3 short
// name (C:\Users\RUNNER~1\...) while `cd` prints the long name
// (C:\Users\runneradmin\...); on macOS t.TempDir() lives under a /var ->
// /private/var symlink. filepath.EvalSymlinks canonicalizes both forms, so we
// accept either the raw or the resolved path. The assertions match against
// toolText() output, which json-decodes the content "text" field, so Windows
// paths carry native single backslashes with no extra escaping.
func containsResolvedPath(haystack, p string) bool {
	if strings.Contains(haystack, p) {
		return true
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil && resolved != p {
		return strings.Contains(haystack, resolved)
	}
	return false
}

func TestMCPExecHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_MCP_EXEC_HELPER") != "1" {
		return
	}
	args := os.Args
	for i, arg := range args {
		if arg == "--" && i+1 < len(args) {
			switch args[i+1] {
			case "flow":
				wd, _ := os.Getwd()
				_, _ = os.Stdout.WriteString(wd + "\nalpha\nbeta42\n")
				_, _ = os.Stderr.WriteString("err\n")
			default:
				_, _ = os.Stdout.WriteString(args[i+1] + "\n")
			}
			os.Exit(0)
		}
	}
	os.Exit(2)
}
