package mcp

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestServeStdioConfigAgyTierTuneAndReset(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "261118-feat-example-agy-config")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "global"))
	server := NewServer(root, "test")
	call := func(input, want string) {
		t.Helper()
		var out bytes.Buffer
		if err := server.ServeStdio(context.Background(), strings.NewReader(input+"\n"), &out); err != nil {
			t.Fatal(err)
		}
		resp := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["1"]
		if resp == "" || toolIsError(t, resp) || !strings.Contains(toolText(t, resp), want) {
			t.Fatalf("request %s: %s; want %s", input, resp, want)
		}
	}
	call(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.resolve_agent","arguments":{"tier":"small","harness":"agy","format":"json"}}}`, `"model":"gemini-4-hydrogen","effort":"medium"`)
	call(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.tune","arguments":{"key":"agents.tier","scope":"global","harness":"AGY","value":{"tier":"small","backend":"agy","model":"global-base","effort":"low"}}}}`, `"agy":{"backend":"agy","model":"global-base","effort":"low"}`)
	call(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.tune","arguments":{"key":"agents.tier","scope":"project","harness":"agy","value":{"tier":"small","backend":"agy","model":"project-base","effort":"high"}}}}`, `"agy":{"backend":"agy","model":"project-base","effort":"high"}`)
	call(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.resolve_agent","arguments":{"tier":"small","harness":"agy","format":"json"}}}`, `"model":"project-base","effort":"high"`)
	call(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.tune","arguments":{"key":"agents.tier","scope":"project","harness":"agy","reset":true,"value":{"tier":"small"}}}}`, `"warnings":[]`)
	call(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.resolve_agent","arguments":{"tier":"small","harness":"agy","format":"json"}}}`, `"model":"global-base","effort":"low"`)
	call(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.tune","arguments":{"key":"agents.tier","scope":"global","harness":"agy","reset":true,"value":{"tier":"small"}}}}`, `"warnings":[]`)
	call(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.resolve_agent","arguments":{"tier":"small","harness":"agy","format":"json"}}}`, `"model":"gemini-4-hydrogen","effort":"medium"`)
}
