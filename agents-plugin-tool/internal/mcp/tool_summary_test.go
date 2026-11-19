package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

func newSummaryTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	s, key := newPrecheckServer(t)
	s.debug = &debugRing{logPath: func() string { return "" }}
	return s, key
}

func TestPrecheckHarvestsAndStripsHiddenSummaries(t *testing.T) {
	s, key := newSummaryTestServer(t)
	defer s.Close()
	for _, name := range wslive.ToolSummaryKeys {
		args := map[string]any{"format": "json", "session_key": key, name: "  Read runtime metadata  "}
		if refusal, ok := s.precheckToolArguments("runtime.read", args); !ok {
			t.Fatalf("%s on a closed schema: %s", name, refusal)
		}
		if !reflect.DeepEqual(args, map[string]any{"format": "json", "session_key": key}) {
			t.Fatalf("%s was not stripped without changing ordinary inputs: %v", name, args)
		}
		properties := toolInputSchema("runtime.read")["properties"].(map[string]any)
		if _, advertised := properties[name]; advertised {
			t.Fatalf("%s must remain hidden", name)
		}
		resp := callToolOnce(t, s, 1, "runtime.read", map[string]any{"format": "json", name: "Read runtime metadata"})
		if callResponseIsError(t, resp) {
			t.Fatalf("%s refused before the real handler: %s", name, resp)
		}
	}
	for _, event := range s.debug.recent(256) {
		if event["event"] == "tools.call.summary" {
			t.Fatalf("harvesting summaries must not add debug records: %v", event)
		}
	}
	args := map[string]any{"summary": "Read metadata", "other_unknown": true}
	if refusal, ok := s.precheckToolArguments("runtime.read", args); ok || !strings.Contains(refusal, "unknown arguments: other_unknown\n") {
		t.Fatalf("summary must not exempt other unknown fields: %s", refusal)
	}
	// Harvest precedes the handler-guided required-argument shortcut too.
	args = map[string]any{"toolSummary": "Provision a worker"}
	if _, ok := s.precheckToolArguments("worktree.acquire", args); !ok || len(args) != 0 {
		t.Fatalf("handler-guided arguments bypassed harvest: %v", args)
	}
}

func TestHarvestLeavesDeclaredSummaryArgumentsAlone(t *testing.T) {
	for _, name := range wslive.ToolSummaryKeys {
		// A hypothetical tool genuinely owns this property: the hidden
		// convention must not consume it or relax its handler's validation.
		schema := map[string]any{"type": "object", "properties": map[string]any{
			name: map[string]any{"type": "object"},
		}}
		value := map[string]any{"domain": "tool-owned summary"}
		args := map[string]any{name: value}
		if summary := harvestToolSummary(args, schema["properties"].(map[string]any)); summary != "" || !reflect.DeepEqual(args[name], value) {
			t.Fatalf("declared %s was harvested or changed: summary=%q, args=%v", name, summary, args)
		}
	}
	args := map[string]any{"summary": "domain value", "toolSummary": "call description", "tool_summary": "lower priority"}
	properties := map[string]any{"summary": map[string]any{"type": "string"}}
	if got := harvestToolSummary(args, properties); got != "call description" || !reflect.DeepEqual(args, map[string]any{"summary": "domain value"}) {
		t.Fatalf("declared summary and hidden aliases: got %q, args %v", got, args)
	}
}

func TestSummaryMetadataIsOptionalAndDoesNotLog(t *testing.T) {
	s, _ := newSummaryTestServer(t)
	defer s.Close()
	logPath := filepath.Join(t.TempDir(), "debug.log")
	s.debug.logPath = func() string { return logPath }
	for _, args := range []map[string]any{
		{},
		{"summary": " \n ", "toolSummary": 7, "tool_summary": nil},
		{"summary": "Read runtime metadata"},
	} {
		if refusal, ok := s.precheckToolArguments("runtime.read", args); !ok || len(args) != 0 {
			t.Fatalf("optional metadata must not refuse a call: %v %s", args, refusal)
		}
	}
	if events := s.debug.recent(256); len(events) != 0 {
		t.Fatalf("summary harvesting recorded debug events: %v", events)
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("summary harvesting touched the configured debug log: %v", err)
	}
}

func TestCallInnerDispatchHarvestsSummary(t *testing.T) {
	setupMailboxTestEnv(t)
	t.Setenv(envAllowedTools, "")
	s := newCallTestServer(t.TempDir())
	logPath := filepath.Join(t.TempDir(), "debug.log")
	s.debug = &debugRing{logPath: func() string { return logPath }}
	defer s.Close()
	params, err := json.Marshal(map[string]any{"name": "call", "arguments": map[string]any{
		"tool": "runtime.read", "toolSummary": "Use fallback dispatch",
		"arguments": map[string]any{"format": "json", "tool_summary": "Read runtime metadata"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	// Exercise dispatch without the transport's ordinary request.received
	// diagnostics: neither harvest may add its own log write.
	resp := s.callTool(context.Background(), request{ID: json.RawMessage("1"), Method: "tools/call", Params: params})
	if responseIsError(resp) {
		t.Fatalf("inner metadata must pass the same precheck: %+v", resp)
	}
	if events := s.debug.recent(256); len(events) != 0 {
		t.Fatalf("envelope and inner summaries must not emit debug records: %v", events)
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("fallback dispatch touched the configured debug log: %v", err)
	}
}
