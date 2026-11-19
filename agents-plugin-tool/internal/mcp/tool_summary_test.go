package mcp

import (
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

func recordedToolSummaries(s *Server, name string) []string {
	var summaries []string
	for _, event := range s.debug.recent(256) {
		if event["event"] == "tools.call.summary" && event["tool"] == name {
			summaries = append(summaries, event["summary"].(string))
		}
	}
	return summaries
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
	if summaries := recordedToolSummaries(s, "runtime.read"); len(summaries) != 6 || summaries[0] != "Read runtime metadata" {
		t.Fatalf("harvested summaries = %v", summaries)
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

func TestSummaryMetadataIsOptionalAndScrubbed(t *testing.T) {
	s, key := newSummaryTestServer(t)
	defer s.Close()
	for _, args := range []map[string]any{
		{},
		{"summary": " \n ", "toolSummary": 7, "tool_summary": nil},
	} {
		if refusal, ok := s.precheckToolArguments("runtime.read", args); !ok || len(args) != 0 {
			t.Fatalf("optional metadata must not refuse a call: %v %s", args, refusal)
		}
	}
	if got := recordedToolSummaries(s, "runtime.read"); len(got) != 0 {
		t.Fatalf("absent/invalid metadata recorded a summary: %v", got)
	}
	args := map[string]any{"session_key": key, "summary": "Read " + key + " password=hunter2"}
	if _, ok := s.precheckToolArguments("runtime.read", args); !ok {
		t.Fatal("summary refused")
	}
	got := recordedToolSummaries(s, "runtime.read")
	if len(got) != 1 || strings.Contains(got[0], key) || strings.Contains(got[0], "hunter2") || !strings.Contains(got[0], "password=***") {
		t.Fatalf("debug summary leaked a key or credential: %v", got)
	}
}

func TestCallInnerDispatchHarvestsSummary(t *testing.T) {
	setupMailboxTestEnv(t)
	t.Setenv(envAllowedTools, "")
	s := newCallTestServer(t.TempDir())
	s.debug = &debugRing{logPath: func() string { return "" }}
	defer s.Close()
	resp := callToolOnce(t, s, 1, "call", map[string]any{
		"tool": "runtime.read", "toolSummary": "Use fallback dispatch",
		"arguments": map[string]any{"format": "json", "tool_summary": "Read runtime metadata"},
	})
	if callResponseIsError(t, resp) {
		t.Fatalf("inner metadata must pass the same precheck: %s", resp)
	}
	if got := recordedToolSummaries(s, "runtime.read"); !reflect.DeepEqual(got, []string{"Read runtime metadata"}) {
		t.Fatalf("inner summary = %v", got)
	}
	if got := recordedToolSummaries(s, "call"); !reflect.DeepEqual(got, []string{"Use fallback dispatch"}) {
		t.Fatalf("envelope summary = %v", got)
	}
}
