package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestServeStdioRecoversPanicAndPersistsCrashTrace is the Phase 1 regression
// test for 260724-bug-windows-mcp-mid-session-disconnect: a deliberately
// panicking write handler (todo.append, forced via testPanicHook) must fail
// only its own request with a visible JSON-RPC error, persist the panic value
// and stack to the always-on crash file, and leave the process able to serve
// a normal subsequent request. Silently swallowing the panic (no crash-file
// evidence, or no top-level error) is a regression, not a pass.
func TestServeStdioRecoversPanicAndPersistsCrashTrace(t *testing.T) {
	useLeadProfile(t)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheHome)

	const panicMessage = "deliberate test panic: todo.append"
	testPanicHook = func(name string) {
		if name == "todo.append" {
			panic(panicMessage)
		}
	}
	t.Cleanup(func() { testPanicHook = nil })

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":"init-1","method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":"panic-1","method":"tools/call","params":{"name":"todo.append","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":"after-1","method":"tools/call","params":{"name":"runtime.info","arguments":{}}}`,
	}, "\n")

	var out bytes.Buffer
	server := NewServer(t.TempDir(), "test")
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	byID := responseLinesByID(t, lines)

	panicLine, ok := byID["panic-1"]
	if !ok {
		t.Fatalf("no response for panic-1 request:\n%s", out.String())
	}
	var panicResp struct {
		Result *json.RawMessage `json:"result"`
		Error  *rpcError        `json:"error"`
	}
	if err := json.Unmarshal([]byte(panicLine), &panicResp); err != nil {
		t.Fatalf("decode panic-1 response: %v\n%s", err, panicLine)
	}
	if panicResp.Result != nil {
		t.Fatalf("panic-1 response must not carry a result, panic was silently swallowed: %s", panicLine)
	}
	if panicResp.Error == nil {
		t.Fatalf("panic-1 response must carry a top-level error: %s", panicLine)
	}
	if panicResp.Error.Code != -32000 {
		t.Fatalf("panic-1 error code = %d, want -32000: %s", panicResp.Error.Code, panicLine)
	}

	afterLine, ok := byID["after-1"]
	if !ok {
		t.Fatalf("no response for after-1 request, process did not keep serving after panic:\n%s", out.String())
	}
	if strings.Contains(afterLine, `"error"`) {
		t.Fatalf("after-1 response must be a normal successful result, not an error: %s", afterLine)
	}
	var afterResp struct {
		Result map[string]any `json:"result"`
	}
	if err := json.Unmarshal([]byte(afterLine), &afterResp); err != nil {
		t.Fatalf("decode after-1 response: %v\n%s", err, afterLine)
	}
	if afterResp.Result == nil {
		t.Fatalf("after-1 response missing result: %s", afterLine)
	}

	crashPath := filepath.Join(cacheHome, "crash", "mcp-panic.log")
	crashBytes, err := os.ReadFile(crashPath)
	if err != nil {
		t.Fatalf("read crash log %s: %v", crashPath, err)
	}
	crashText := string(crashBytes)
	if !strings.Contains(crashText, `"event":"request.panic"`) {
		t.Fatalf("crash log missing request.panic event:\n%s", crashText)
	}
	if !strings.Contains(crashText, panicMessage) {
		t.Fatalf("crash log missing panic message %q:\n%s", panicMessage, crashText)
	}
}
