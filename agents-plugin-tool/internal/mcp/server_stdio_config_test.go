package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

func TestServeStdioConfigShow(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	cache := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cache)

	var out bytes.Buffer
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.list","arguments":{"format":"json"}}}`+"\n",
	), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	showBefore := toolText(t, byID["1"])
	var before struct {
		Path   string `json:"path"`
		Config struct {
			SchemaVersion int `json:"schema_version"`
		} `json:"config"`
	}
	if err := json.Unmarshal([]byte(showBefore), &before); err != nil {
		t.Fatalf("config.show json response is not JSON: %v\n%s", err, showBefore)
	}
	wantConfigPath := filepath.Join(canonicalTestPath(t, cache), "config.json")
	if before.Path != wantConfigPath || before.Config.SchemaVersion != 1 {
		t.Fatalf("config.show json response mismatch: %s", byID["1"])
	}

	if _, err := wsconfig.SetAgentsTier(wsconfig.Options{}, "small", "", "claude-sonnet-4", "low"); err != nil {
		t.Fatalf("SetAgentsTier returned error: %v", err)
	}
	out.Reset()
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"config.list","arguments":{"format":"json"}}}`+"\n",
	), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID = responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	showAfter := toolText(t, byID["2"])
	if !strings.Contains(showAfter, `"backend":"claude"`) || !strings.Contains(showAfter, `"model":"claude-sonnet-4"`) || !strings.Contains(showAfter, `"effort":"low"`) {
		t.Fatalf("config.show response missing tier mapping: %s", byID["2"])
	}

	out.Reset()
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"config.list","arguments":{}}}`+"\n",
	), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID = responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if readable := toolText(t, byID["3"]); !strings.Contains(readable, "effort=low") {
		t.Fatalf("readable config.show missing effort: %s", readable)
	}
}

func TestServeStdioConfigAgentsTier(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	var out bytes.Buffer
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.tune","arguments":{"key":"agents.tier","value":{"tier":"small","model":"claude-sonnet-4"}}}}`+"\n",
	), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	configText := toolText(t, byID["1"])
	if !strings.Contains(configText, `"backend":"claude"`) || !strings.Contains(configText, `"model":"claude-sonnet-4"`) {
		t.Fatalf("config response missing tier mapping: %s", byID["1"])
	}

}

func TestServeStdioConfigAgentsTierUsesDetectedHarness(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260513-feat-harness-local-agent-tier-config")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	var out bytes.Buffer
	initializeInput := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"Claude Code","version":"test"}}}`
	if err := server.ServeStdio(context.Background(), strings.NewReader(initializeInput), &out); err != nil {
		t.Fatalf("ServeStdio initialize returned error: %v", err)
	}

	out.Reset()
	configInput := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"config.tune","arguments":{"key":"agents.tier","value":{"tier":"medium","backend":"codex","model":"gpt-5.4","effort":"medium"}}}}`
	if err := server.ServeStdio(context.Background(), strings.NewReader(configInput), &out); err != nil {
		t.Fatalf("ServeStdio config returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	configText := toolText(t, byID["2"])
	if !strings.Contains(configText, `"claude":{"backend":"codex","model":"gpt-5.4","effort":"medium"}`) {
		t.Fatalf("config response missing claude harness mapping: %s", byID["2"])
	}

	// The write landed in the detected harness bucket, so tier resolution under
	// that same detected harness reads it back.
	out.Reset()
	resolveInput := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"config.resolve_agent","arguments":{"tier":"medium","format":"json"}}}`
	if err := server.ServeStdio(context.Background(), strings.NewReader(resolveInput), &out); err != nil {
		t.Fatalf("ServeStdio config.resolve_agent returned error: %v", err)
	}
	byID = responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	var resolved resolveAgentTierResponse
	if err := json.Unmarshal([]byte(toolText(t, byID["3"])), &resolved); err != nil {
		t.Fatalf("decode config.resolve_agent response: %v", err)
	}
	if resolved.ResolvedFrom != "claude" || resolved.Backend != "codex" || resolved.Model != "gpt-5.4" || resolved.Effort != "medium" {
		t.Fatalf("resolved = %#v, want resolved_from=claude backend=codex model=gpt-5.4 effort=medium", resolved)
	}
}

// TestServeStdioConfigAgentsTierUsesDetectedPiHarness mirrors
// TestServeStdioConfigAgentsTierUsesDetectedHarness for the Pi harness: an
// `initialize` request whose clientInfo.name is exactly "ws-pi-bridge" must
// be detected via the structured clientInfo check
// (detectHarnessFromInitializeParams) and used as the config.tune harness
// default.
func TestServeStdioConfigAgentsTierUsesDetectedPiHarness(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260513-feat-harness-local-agent-tier-config")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	var out bytes.Buffer
	initializeInput := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"ws-pi-bridge","version":"test"}}}`
	if err := server.ServeStdio(context.Background(), strings.NewReader(initializeInput), &out); err != nil {
		t.Fatalf("ServeStdio initialize returned error: %v", err)
	}

	out.Reset()
	configInput := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"config.tune","arguments":{"key":"agents.tier","value":{"tier":"medium","backend":"pi","model":"pi-model","effort":"medium"}}}}`
	if err := server.ServeStdio(context.Background(), strings.NewReader(configInput), &out); err != nil {
		t.Fatalf("ServeStdio config returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	configText := toolText(t, byID["2"])
	if !strings.Contains(configText, `"pi":{"backend":"pi","model":"pi-model","effort":"medium"}`) {
		t.Fatalf("config response missing pi harness mapping: %s", byID["2"])
	}
}

// resolveAgentTierResponse mirrors the config.resolve_agent JSON payload
// (resolveAgentTierResult) for test-side decoding.
type resolveAgentTierResponse struct {
	Backend      string `json:"backend"`
	Model        string `json:"model"`
	Effort       string `json:"effort"`
	ResolvedFrom string `json:"resolved_from"`
}

// TestServeStdioConfigResolveAgentUsesDetectedPiHarness covers the ticket's
// "each tier under pi with a set value" case: for every fixed tier, seed a
// "pi" bucket via config.tune under a detected "pi" harness session, then
// call config.resolve_agent with no explicit harness and assert it reports
// resolved_from == "pi" and the exact backend/model/effort that was set
// (260905 Phase 3). Mirrors
// TestServeStdioConfigAgentsTierUsesDetectedPiHarness.
func TestServeStdioConfigResolveAgentUsesDetectedPiHarness(t *testing.T) {
	for _, tier := range []string{"small", "medium", "large", "xlarge"} {
		t.Run(tier, func(t *testing.T) {
			useLeadProfile(t)
			root := initTicketRepo(t, "260905-feat-ws-pi-harness-config-layer")
			t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

			server := NewServer(root, "test")
			var out bytes.Buffer
			initializeInput := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"ws-pi-bridge","version":"test"}}}`
			if err := server.ServeStdio(context.Background(), strings.NewReader(initializeInput), &out); err != nil {
				t.Fatalf("ServeStdio initialize returned error: %v", err)
			}

			out.Reset()
			model := "pi-model-" + tier
			configInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"config.tune","arguments":{"key":"agents.tier","value":{"tier":%q,"backend":"pi","model":%q,"effort":"high"}}}}`, tier, model)
			if err := server.ServeStdio(context.Background(), strings.NewReader(configInput), &out); err != nil {
				t.Fatalf("ServeStdio config.tune returned error: %v", err)
			}

			out.Reset()
			resolveInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"config.resolve_agent","arguments":{"tier":%q,"format":"json"}}}`, tier)
			if err := server.ServeStdio(context.Background(), strings.NewReader(resolveInput), &out); err != nil {
				t.Fatalf("ServeStdio config.resolve_agent returned error: %v", err)
			}
			byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
			if toolIsError(t, byID["3"]) {
				t.Fatalf("config.resolve_agent returned an error: %s", byID["3"])
			}
			var result resolveAgentTierResponse
			if err := json.Unmarshal([]byte(toolText(t, byID["3"])), &result); err != nil {
				t.Fatalf("decode config.resolve_agent response: %v", err)
			}
			if result.Backend != "pi" || result.Model != model || result.Effort != "high" {
				t.Fatalf("result = %#v, want backend=pi model=%s effort=high", result, model)
			}
			if result.ResolvedFrom != "pi" {
				t.Fatalf("resolved_from = %q, want %q", result.ResolvedFrom, "pi")
			}
		})
	}
}

// TestServeStdioConfigResolveAgentFallsBackToDefault covers the ticket's
// "fallback to default with the answering bucket reported" case: under a
// detected "pi" harness session with nothing set for the "pi" bucket,
// config.resolve_agent must resolve at the seeded "default" bucket
// (applyDefaultModelAliases always seeds "default"/"codex", never "pi") and
// report resolved_from == "default".
func TestServeStdioConfigResolveAgentFallsBackToDefault(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260905-feat-ws-pi-harness-config-layer")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	var out bytes.Buffer
	initializeInput := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"ws-pi-bridge","version":"test"}}}`
	if err := server.ServeStdio(context.Background(), strings.NewReader(initializeInput), &out); err != nil {
		t.Fatalf("ServeStdio initialize returned error: %v", err)
	}

	out.Reset()
	resolveInput := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"config.resolve_agent","arguments":{"tier":"medium","format":"json"}}}`
	if err := server.ServeStdio(context.Background(), strings.NewReader(resolveInput), &out); err != nil {
		t.Fatalf("ServeStdio config.resolve_agent returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	var result resolveAgentTierResponse
	if err := json.Unmarshal([]byte(toolText(t, byID["2"])), &result); err != nil {
		t.Fatalf("decode config.resolve_agent response: %v", err)
	}
	if result.Backend != "codex" || result.Model != "gpt-6-luna" || result.Effort != "xhigh" {
		t.Fatalf("result = %#v, want the seeded default medium tier", result)
	}
	if result.ResolvedFrom != "default" {
		t.Fatalf("resolved_from = %q, want %q", result.ResolvedFrom, "default")
	}
}

// TestServeStdioConfigResolveAgentNoDetectedHarnessFallsBackToDefault covers
// the "no detected harness" case: without an initialize call establishing a
// harness, config.resolve_agent defaults its harness argument to
// s.currentHarness() (empty), which resolveAliasMapping/aliasResolutionKeys
// degrade to the same "default" bucket.
func TestServeStdioConfigResolveAgentNoDetectedHarnessFallsBackToDefault(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260905-feat-ws-pi-harness-config-layer")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	var out bytes.Buffer
	resolveInput := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.resolve_agent","arguments":{"tier":"medium","format":"json"}}}`
	if err := server.ServeStdio(context.Background(), strings.NewReader(resolveInput), &out); err != nil {
		t.Fatalf("ServeStdio config.resolve_agent returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	var result resolveAgentTierResponse
	if err := json.Unmarshal([]byte(toolText(t, byID["1"])), &result); err != nil {
		t.Fatalf("decode config.resolve_agent response: %v", err)
	}
	if result.Backend != "codex" || result.Model != "gpt-6-luna" || result.Effort != "xhigh" {
		t.Fatalf("result = %#v, want the seeded default medium tier", result)
	}
	if result.ResolvedFrom != "default" {
		t.Fatalf("resolved_from = %q, want %q", result.ResolvedFrom, "default")
	}
}

// TestServeStdioConfigResolveAgentUsesDefaultCodexTierTable pins each shipped
// Codex model/effort pair through config.resolve_agent when no harness is
// detected, including the answering default bucket.
func TestServeStdioConfigResolveAgentUsesDefaultCodexTierTable(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260923-feat-agent-tier-arbitrary-effort-and-codex-defaults")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "global"))

	server := NewServer(root, "test")
	for i, tc := range []struct {
		tier, model, effort string
	}{
		{"small", "gpt-6-luna", "high"},
		{"medium", "gpt-6-luna", "xhigh"},
		{"large", "gpt-6-sol", "high"},
		{"xlarge", "gpt-6-sol", "xhigh"},
	} {
		var out bytes.Buffer
		input := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.resolve_agent","arguments":{"tier":%q,"format":"json"}}}`, tc.tier)
		if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
			t.Fatalf("ServeStdio config.resolve_agent(%s) returned error: %v", tc.tier, err)
		}
		byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
		var result resolveAgentTierResponse
		if err := json.Unmarshal([]byte(toolText(t, byID["1"])), &result); err != nil {
			t.Fatalf("decode config.resolve_agent(%s) response: %v", tc.tier, err)
		}
		if result.Backend != "codex" || result.Model != tc.model || result.Effort != tc.effort || result.ResolvedFrom != "default" {
			t.Errorf("case %d config.resolve_agent(%s) = %#v, want default/codex/%s/%s", i, tc.tier, result, tc.model, tc.effort)
		}
	}
}

// TestServeStdioConfigResolveAgentExplicitHarnessOverridesDetected proves an
// explicit harness argument to config.resolve_agent overrides the detected
// session harness: the session detects "codex", but a "pi" bucket is seeded
// and the caller passes harness:"pi" explicitly, so the answer must come
// from the pi bucket, not the detected codex one.
func TestServeStdioConfigResolveAgentExplicitHarnessOverridesDetected(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260905-feat-ws-pi-harness-config-layer")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	var out bytes.Buffer
	initializeInput := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"codex-cli","version":"test"}}}`
	if err := server.ServeStdio(context.Background(), strings.NewReader(initializeInput), &out); err != nil {
		t.Fatalf("ServeStdio initialize returned error: %v", err)
	}

	out.Reset()
	seedCodexInput := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"config.tune","arguments":{"key":"agents.tier","value":{"tier":"medium","backend":"codex","model":"codex-medium-model"}}}}`
	if err := server.ServeStdio(context.Background(), strings.NewReader(seedCodexInput), &out); err != nil {
		t.Fatalf("ServeStdio config.tune (codex) returned error: %v", err)
	}

	out.Reset()
	seedPiInput := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"config.tune","arguments":{"key":"agents.tier","harness":"pi","value":{"tier":"medium","backend":"pi","model":"pi-medium-model"}}}}`
	if err := server.ServeStdio(context.Background(), strings.NewReader(seedPiInput), &out); err != nil {
		t.Fatalf("ServeStdio config.tune (pi) returned error: %v", err)
	}

	out.Reset()
	resolveInput := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"config.resolve_agent","arguments":{"tier":"medium","harness":"pi","format":"json"}}}`
	if err := server.ServeStdio(context.Background(), strings.NewReader(resolveInput), &out); err != nil {
		t.Fatalf("ServeStdio config.resolve_agent returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	var result resolveAgentTierResponse
	if err := json.Unmarshal([]byte(toolText(t, byID["4"])), &result); err != nil {
		t.Fatalf("decode config.resolve_agent response: %v", err)
	}
	if result.Backend != "pi" || result.Model != "pi-medium-model" {
		t.Fatalf("explicit harness argument did not override detected codex harness: %#v", result)
	}
	if result.ResolvedFrom != "pi" {
		t.Fatalf("resolved_from = %q, want %q", result.ResolvedFrom, "pi")
	}
}

// TestServeStdioConfigResolveAgentRejectsUnknownTier covers the ticket's
// "unknown tier rejected" case at the MCP dispatch layer.
func TestServeStdioConfigResolveAgentRejectsUnknownTier(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260905-feat-ws-pi-harness-config-layer")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	var out bytes.Buffer
	resolveInput := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.resolve_agent","arguments":{"tier":"bogus"}}}`
	if err := server.ServeStdio(context.Background(), strings.NewReader(resolveInput), &out); err != nil {
		t.Fatalf("ServeStdio config.resolve_agent returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if !toolIsError(t, byID["1"]) {
		t.Fatalf("expected config.resolve_agent to error on unknown tier: %s", byID["1"])
	}
	if !strings.Contains(toolText(t, byID["1"]), "tier must be small, medium, large, or xlarge") {
		t.Fatalf("error text missing valid tier set: %s", byID["1"])
	}
}

// TestServeStdioConfigAgentsTierAcceptsHarnessCaseInsensitively is a
// review-relay regression test: before this diff, agents.tier's harness
// selector had no Enum, so an arbitrary-case value reached
// wsconfig.aliasTargetKey unchecked, which lowercases before matching. Once
// this phase gave the selector an explicit Enum, the config.tune guard
// started exact-matching (case-sensitive) against it, silently narrowing
// previously-accepted input such as "Claude"/"CODEX"/"Default". The guard
// must lowercase the trimmed harness before the enum check so this input
// keeps working, matching wsconfig's and the CLI's case-insensitive
// behavior.
func TestServeStdioConfigAgentsTierAcceptsHarnessCaseInsensitively(t *testing.T) {
	cases := []struct {
		name      string
		harness   string
		bucketKey string
	}{
		{"mixed-case-claude", "Claude", "claude"},
		{"upper-case-codex", "CODEX", "codex"},
		{"mixed-case-default", "Default", "default"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useLeadProfile(t)
			root := initTicketRepo(t, "260513-feat-harness-local-agent-tier-config")
			t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

			server := NewServer(root, "test")
			var out bytes.Buffer
			uniqueModel := "case-test-model-" + tc.bucketKey
			configInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"config.tune","arguments":{"key":"agents.tier","harness":%q,"value":{"tier":"medium","backend":"custom-backend","model":%q}}}}`, tc.harness, uniqueModel)
			if err := server.ServeStdio(context.Background(), strings.NewReader(configInput+"\n"), &out); err != nil {
				t.Fatalf("ServeStdio config returned error: %v", err)
			}
			byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
			resp := byID["2"]
			if strings.Contains(resp, `"isError":true`) {
				t.Fatalf("config.tune rejected case-variant harness %q (should accept case-insensitively): %s", tc.harness, resp)
			}
			configText := toolText(t, resp)
			expected := fmt.Sprintf(`"%s":{"backend":"custom-backend","model":%q}`, tc.bucketKey, uniqueModel)
			if !strings.Contains(configText, expected) {
				t.Fatalf("expected lowercase bucket key %q with stored mapping in response, got: %s", tc.bucketKey, configText)
			}
		})
	}
}

func TestServeStdioConfigAgentsTierScopesWriteAndResolve(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260921-feat-config-tune-agents-tier-global-scope")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "global"))

	server := NewServer(root, "test")
	inputs := []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.tune","arguments":{"key":"agents.tier","scope":"global","harness":"claude","value":{"tier":"medium","backend":"claude","model":"global-claude","effort":"low"}}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"config.tune","arguments":{"key":"agents.tier","scope":"project","harness":"pi","value":{"tier":"medium","backend":"pi","model":"project-pi","effort":"high"}}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"config.resolve_agent","arguments":{"tier":"medium","harness":"claude","format":"json"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"config.resolve_agent","arguments":{"tier":"medium","harness":"pi","format":"json"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"config.tune","arguments":{"key":"agents.tier","scope":"session","value":{"tier":"medium"}}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"config.tune","arguments":{"key":"agents.tier","scope":"repo","value":{"tier":"medium"}}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"config.list","arguments":{"format":"json"}}}`,
	}

	var out bytes.Buffer
	for _, input := range inputs {
		out.Reset()
		if err := server.ServeStdio(context.Background(), strings.NewReader(input+"\n"), &out); err != nil {
			t.Fatalf("ServeStdio: %v", err)
		}
		byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
		for _, id := range []string{"1", "2", "3", "4"} {
			if resp, ok := byID[id]; ok && toolIsError(t, resp) {
				t.Fatalf("request %s failed: %s", id, resp)
			}
		}
		if resp, ok := byID["5"]; ok && !toolIsError(t, resp) {
			t.Fatalf("session scope unexpectedly succeeded: %s", resp)
		}
		if resp, ok := byID["6"]; ok && !toolIsError(t, resp) {
			t.Fatalf("repo scope unexpectedly succeeded: %s", resp)
		}
		if resp, ok := byID["3"]; ok && !strings.Contains(toolText(t, resp), `"model":"global-claude"`) {
			t.Fatalf("global resolution = %s", resp)
		}
		if resp, ok := byID["4"]; ok && !strings.Contains(toolText(t, resp), `"model":"project-pi"`) {
			t.Fatalf("project leaf resolution = %s", resp)
		}
		if resp, ok := byID["7"]; ok && !strings.Contains(toolText(t, resp), `"model":"global-claude"`) {
			t.Fatalf("config.list omitted the global tier leaf: %s", resp)
		}
	}
}

// TestServeStdioConfigAgentsTierAcceptsTierSynonyms guards the 260814 config
// surface swap against a regression: config.tune must NOT enum-validate the
// agents.tier `tier` up front. The removed config.agents_tier passed the raw
// string to SetAgentsTierForHarness, whose normalizedTier accepts documented
// synonyms (light/core/deep, haiku/sonnet/opus) case/whitespace-insensitively
// (see wsconfig TestResolveAgentLegacyTierSynonyms). The surviving CLI path
// still does. So config.tune must too.
func TestServeStdioConfigAgentsTierAcceptsTierSynonyms(t *testing.T) {
	cases := []struct {
		name         string
		tier         string
		canonicalKey string
	}{
		{"synonym-core", "core", "medium"},
		{"synonym-deep", "deep", "large"},
		{"synonym-sonnet", "sonnet", "medium"},
		{"case-insensitive", "Small", "small"},
		{"whitespace", "  large  ", "large"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useLeadProfile(t)
			root := initTicketRepo(t, "260513-feat-harness-local-agent-tier-config")
			t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

			server := NewServer(root, "test")
			var out bytes.Buffer
			model := "model-" + tc.canonicalKey
			configInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"config.tune","arguments":{"key":"agents.tier","harness":"claude","value":{"tier":%q,"model":%q}}}}`, tc.tier, model)
			if err := server.ServeStdio(context.Background(), strings.NewReader(configInput+"\n"), &out); err != nil {
				t.Fatalf("ServeStdio config returned error: %v", err)
			}
			byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
			resp := byID["2"]
			if strings.Contains(resp, `"isError":true`) {
				t.Fatalf("config.tune rejected tier synonym %q (should accept, matching CLI): %s", tc.tier, resp)
			}
			// The synonym must normalize to its canonical tier key and store the
			// harness mapping there. toolText unescapes the inner config JSON.
			configText := toolText(t, resp)
			if !strings.Contains(configText, `"`+tc.canonicalKey+`"`) || !strings.Contains(configText, model) {
				t.Fatalf("config.tune tier synonym %q did not land under canonical tier %q with model %q: %s", tc.tier, tc.canonicalKey, model, configText)
			}
		})
	}
}

func TestFormatConfigViewQuotesControlCharactersInEffort(t *testing.T) {
	view := wsconfig.View{Config: wsconfig.Config{Agents: wsconfig.AgentsConfig{
		ModelAliases: map[string]map[string]wsconfig.AgentTier{
			"medium": {"codex": {Backend: "codex", Model: "model", Effort: "max\n  forged: value"}},
		},
	}}}
	got := formatConfigView(view)
	if strings.Contains(got, "\n  forged: value") || !strings.Contains(got, `effort="max\n  forged: value"`) {
		t.Fatalf("effort broke the config view row: %q", got)
	}
}

func TestServeStdioConfigAgentsTierRoundTripsArbitraryEffortLabels(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260923-feat-agent-tier-arbitrary-effort-and-codex-defaults")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "global"))
	t.Setenv("WS_MCP_NO_AGENT", "")
	t.Setenv("WS_MCP_NAMESPACE", "")
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"model-pb/model-pb.md":   modelAliasPlaybookContent,
		"effort-pb/effort-pb.md": "---\nkind: print\ndelegates: false\n---\n# Effort Playbook\n\nEffort: {{.MediumTierReasoningEffort}}\nEnd of effort.\n",
	})
	useResourceFixture(t, rsrcRoot)

	server := NewServer(root, "test")
	server.observeHarness("test", "codex")
	leadKey, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))
	const model = "mcp-arbitrary-effort-model"

	// display is the effort as line-oriented text shows it, written out
	// independently of formatEffortForText: a label with control characters
	// appears in its Go-quoted form.
	for _, tc := range []struct {
		name, input, want, display string
	}{
		{"case-normalized max", "  MaX  ", "max", "max"},
		{"provider-specific label", "provider-specific-reasoning", "provider-specific-reasoning", "provider-specific-reasoning"},
		{"control characters", "max\nrecommended-model: forged", "max\nrecommended-model: forged", `"max\nrecommended-model: forged"`},
		{"empty unset", "", "", ""},
		{"none unset", "none", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			written := callToolOnce(t, server, 2, "config.tune", map[string]any{
				"session_key": leadKey,
				"key":         "agents.tier",
				"harness":     "codex",
				"value": map[string]any{
					"tier": "medium", "backend": "codex", "model": model, "effort": tc.input,
				},
			})
			if toolIsError(t, written) {
				t.Fatalf("config.tune rejected effort %q: %s", tc.input, written)
			}

			catalog := parseTuningCatalogResponse(t, callToolOnce(t, server, 3, "config.list", map[string]any{
				"session_key": leadKey,
				"format":      "json",
			}))
			rows, ok := requireTuningKnob(t, catalog, "agents.tier").Current.([]any)
			if !ok {
				t.Fatalf("agents.tier current has type %T, want rows", requireTuningKnob(t, catalog, "agents.tier").Current)
			}
			found := false
			for _, raw := range rows {
				row, ok := raw.(map[string]any)
				if !ok || row["tier"] != "medium" || row["harness"] != "codex" || row["model"] != model {
					continue
				}
				found = true
				got, _ := row["effort"].(string)
				if got != tc.want {
					t.Fatalf("config.list effort for %s/codex = %q, want %q (row: %#v)", model, got, tc.want, row)
				}
			}
			if !found {
				t.Fatalf("config.list omitted tuned medium/codex mapping for %s: %#v", model, rows)
			}

			resolvedResp := callToolOnce(t, server, 4, "config.resolve_agent", map[string]any{
				"session_key": leadKey,
				"tier":        "medium",
				"harness":     "codex",
				"format":      "json",
			})
			if toolIsError(t, resolvedResp) {
				t.Fatalf("config.resolve_agent failed: %s", resolvedResp)
			}
			var resolved resolveAgentTierResponse
			if err := json.Unmarshal([]byte(toolText(t, resolvedResp)), &resolved); err != nil {
				t.Fatalf("decode config.resolve_agent response: %v", err)
			}
			if resolved.Backend != "codex" || resolved.Model != model || resolved.Effort != tc.want || resolved.ResolvedFrom != "codex" {
				t.Fatalf("config.resolve_agent = %#v, want codex/%s/%q from codex", resolved, model, tc.want)
			}

			// The text form is line-oriented: an effort label must stay on its
			// own "effort:" line rather than inject further fields.
			resolvedText := callToolOnce(t, server, 6, "config.resolve_agent", map[string]any{
				"session_key": leadKey,
				"tier":        "medium",
				"harness":     "codex",
			})
			if toolIsError(t, resolvedText) {
				t.Fatalf("config.resolve_agent (text) failed: %s", resolvedText)
			}
			gotText := toolText(t, resolvedText)
			if lines := strings.Split(strings.TrimSuffix(gotText, "\n"), "\n"); len(lines) != 4 {
				t.Fatalf("config.resolve_agent text has %d lines, want exactly 4 (no injected field line): %q", len(lines), gotText)
			}
			wantText := fmt.Sprintf("backend: codex\nmodel: %s\neffort: %s\nresolved_from: codex\n", model, tc.display)
			if gotText != wantText {
				t.Fatalf("config.resolve_agent text = %q, want %q", gotText, wantText)
			}

			// Playbook bodies substitute the tier effort vars into prose; an
			// effort label must not break the substituted line.
			effortRendered := callToolOnce(t, server, 7, "playbook.render", map[string]any{
				"name":        "effort-pb",
				"session_key": leadKey,
			})
			if toolIsError(t, effortRendered) {
				t.Fatalf("playbook.render effort-pb failed: %s", effortRendered)
			}
			effortPath := strings.Split(strings.TrimSpace(toolText(t, effortRendered)), "\n")[0]
			effortBody, err := os.ReadFile(effortPath)
			if err != nil {
				t.Fatal(err)
			}
			wantLine := "Effort: " + tc.display + "\nEnd of effort."
			if !strings.Contains(string(effortBody), wantLine) || strings.Contains(string(effortBody), "\nrecommended-model: forged") {
				t.Fatalf("rendered body effort line = %q, want line %q", effortBody, wantLine)
			}

			if tc.want != "" {
				rendered := callToolOnce(t, server, 5, "playbook.render", map[string]any{
					"name":        "model-pb",
					"session_key": leadKey,
				})
				if toolIsError(t, rendered) {
					t.Fatalf("playbook.render failed: %s", rendered)
				}
				responseText := toolText(t, rendered)
				if !strings.Contains(responseText, "recommended-model: "+model) || !strings.Contains(responseText, "recommended-reasoning-effort: "+tc.display) {
					t.Fatalf("playbook.render omitted tuned model/effort %s/%s:\n%s", model, tc.want, responseText)
				}
				if strings.Contains(responseText, "\nrecommended-model: forged") {
					t.Fatalf("configured effort injected a render metadata line: %q", responseText)
				}
				path := strings.Split(strings.TrimSpace(responseText), "\n")[0]
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(body), "Model: "+model) {
					t.Fatalf("rendered body omitted tuned model %s:\n%s", model, body)
				}
			}
		})
	}
}

func TestServeStdioConfigAgentsTierOmittedEffortClearsExistingEffort(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260513-feat-agent-tier-effort-config")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	var out bytes.Buffer
	server := NewServer(root, "test")
	inputs := []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.tune","arguments":{"key":"agents.tier","harness":"codex","value":{"tier":"medium","model":"gpt-5.5","effort":"medium"}}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"config.tune","arguments":{"key":"agents.tier","harness":"codex","value":{"tier":"medium","model":"gpt-5.4"}}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"config.resolve_agent","arguments":{"tier":"medium","harness":"codex","format":"json"}}}`,
	}
	for _, input := range inputs {
		out.Reset()
		if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
			t.Fatalf("ServeStdio returned error: %v", err)
		}
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	var resolved resolveAgentTierResponse
	if err := json.Unmarshal([]byte(toolText(t, byID["3"])), &resolved); err != nil {
		t.Fatalf("decode config.resolve_agent response: %v", err)
	}
	if resolved.Model != "gpt-5.4" || resolved.Effort != "" {
		t.Fatalf("resolved = %#v, want model=gpt-5.4 and cleared effort", resolved)
	}
}

func TestServeStdioInitializeDetectsClaudeHarnessForAgentAlias(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260508-feat-claude-harness")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "global"))

	initializeInput := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"Claude Code","version":"test"}}}`
	resolveInput := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"config.resolve_agent","arguments":{"tier":"medium","format":"json"}}}`

	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := server.ServeStdio(context.Background(), strings.NewReader(initializeInput), &out); err != nil {
		t.Fatalf("ServeStdio initialize returned error: %v", err)
	}
	out.Reset()
	if err := serveStdioWithSession(t, server, root, resolveInput, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	var resolved resolveAgentTierResponse
	if err := json.Unmarshal([]byte(toolText(t, byID["3"])), &resolved); err != nil {
		t.Fatalf("decode config.resolve_agent response: %v", err)
	}
	if resolved.ResolvedFrom != "claude" || resolved.Backend != "claude" || resolved.Model != "sonnet" {
		t.Fatalf("resolved = %#v, want resolved_from/backend claude and model sonnet", resolved)
	}
}

func TestServeStdioCodexMetadataDetectsHarnessForAgentAlias(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260508-feat-codex-harness")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "global"))

	setupInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"project_tree","arguments":{"root":%q},"_meta":{"x-codex-turn-metadata":{"workspaces":{%q:{}}}}}}`, root, root)
	resolveInput := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"config.resolve_agent","arguments":{"tier":"medium","format":"json"}}}`
	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := serveStdioWithSession(t, server, root, setupInput, &out); err != nil {
		t.Fatalf("ServeStdio setup returned error: %v", err)
	}
	out.Reset()
	if err := serveStdioWithSession(t, server, root, resolveInput, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	var resolved resolveAgentTierResponse
	if err := json.Unmarshal([]byte(toolText(t, byID["2"])), &resolved); err != nil {
		t.Fatalf("decode config.resolve_agent response: %v", err)
	}
	if resolved.ResolvedFrom != "codex" || resolved.Backend != "codex" || resolved.Model != "gpt-6-luna" {
		t.Fatalf("resolved = %#v, want resolved_from/backend codex and model gpt-6-luna", resolved)
	}
}

// TestDetectHarnessFromInitializeParamsPrecedence is a review-relay
// regression test for the Decision "Harness identity comes from a
// structured clientInfo parse, not the substring matcher": the structured
// clientInfo.name == "ws-pi-bridge" check must win even when other fields in
// the same initialize params blob contain the literal substrings "claude"
// and "codex" that detectHarnessFromRaw would otherwise match. A future
// refactor that reorders detectHarnessFromInitializeParams (e.g. tries
// detectHarnessFromRaw first and only falls back to the structured check on
// an empty result) would silently violate this precedence while every other
// test in this suite stays green, since none of them plant a decoy
// substring alongside the Pi clientInfo name.
func TestDetectHarnessFromInitializeParamsPrecedence(t *testing.T) {
	raw := json.RawMessage(`{
		"clientInfo": {"name": "ws-pi-bridge", "version": "codex-compat-1.0-claude-shim"},
		"capabilities": {"note": "this client emulates codex and claude tooling"}
	}`)
	if got := detectHarnessFromInitializeParams(raw); got != "pi" {
		t.Fatalf("detectHarnessFromInitializeParams with decoy codex/claude substrings = %q, want \"pi\"", got)
	}

	// Sanity check: with the structured name removed, the same decoy text
	// falls through to the substring detector as before (codex wins first in
	// detectHarnessFromRaw's if/else chain).
	rawWithoutPiName := json.RawMessage(`{
		"clientInfo": {"name": "some-other-client", "version": "codex-compat-1.0-claude-shim"},
		"capabilities": {"note": "this client emulates codex and claude tooling"}
	}`)
	if got := detectHarnessFromInitializeParams(rawWithoutPiName); got != "codex" {
		t.Fatalf("detectHarnessFromInitializeParams fallback without pi clientInfo = %q, want \"codex\" (substring fallback)", got)
	}
}
