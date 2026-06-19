package mcp

// prefer_mercenary_phase2_test.go — integration tests for Phase 2 of the
// layered config scope migration (260619). Covers the 260618 bug closer:
// prefer_mercenary is now desired-state (enable AND disable on the same key)
// routed through the session Overrides overlay, replacing the one-way flip.

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

// TestPreferMercenaryOnOffRenderGuidance is the 260618 repro test. It verifies:
//   - enable (enabled:true) → render of implementer playbook emits guidance block.
//   - disable (enabled:false) on the SAME key → render omits guidance block.
//
// Both transitions must be observable on a single session key.
func TestPreferMercenaryOnOffRenderGuidance(t *testing.T) {
	root := buildTestRsrcTree(t, map[string]string{
		"impl-pb/impl-pb.md": implementerPlaybookContent,
	})
	s := newTestServerWithHarness(t, "claude")

	// Mint a lead key.
	key, err := s.sessions.mint("/work/root", roleLead)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	adapter := sessionConfigAdapter{s: s.sessions}
	resolver := wsconfig.NewResolver(wsconfig.Options{}, nil, adapter, adapter)

	// --- Enable ---
	if err := resolver.Set(wsconfig.ItemPreferMercenary, "true", wsconfig.SetOptions{SessionKey: key}); err != nil {
		t.Fatalf("enable prefer_mercenary: %v", err)
	}

	enabled, scope, err := resolver.GetBool(key, wsconfig.ItemPreferMercenary)
	if err != nil {
		t.Fatalf("GetBool after enable: %v", err)
	}
	if !enabled || scope != wsconfig.ScopeSession {
		t.Fatalf("after enable: got=%v scope=%s, want true/session", enabled, scope)
	}

	bodyOn, _, err := renderPlaybookBody(s, root, "impl-pb", nil, wsconfig.Options{}, "", enabled)
	if err != nil {
		t.Fatalf("renderPlaybookBody on: %v", err)
	}
	if !strings.Contains(bodyOn, "prefer_mercenary active") {
		t.Errorf("guidance block must be present after enable:\n%s", bodyOn)
	}

	// --- Disable ---
	if err := resolver.Set(wsconfig.ItemPreferMercenary, "false", wsconfig.SetOptions{SessionKey: key}); err != nil {
		t.Fatalf("disable prefer_mercenary: %v", err)
	}

	disabled, scope2, err := resolver.GetBool(key, wsconfig.ItemPreferMercenary)
	if err != nil {
		t.Fatalf("GetBool after disable: %v", err)
	}
	if disabled || scope2 != wsconfig.ScopeSession {
		t.Fatalf("after disable: got=%v scope=%s, want false/session", disabled, scope2)
	}

	bodyOff, _, err := renderPlaybookBody(s, root, "impl-pb", nil, wsconfig.Options{}, "", disabled)
	if err != nil {
		t.Fatalf("renderPlaybookBody off: %v", err)
	}
	if strings.Contains(bodyOff, "prefer_mercenary active") {
		t.Errorf("guidance block must be absent after disable:\n%s", bodyOff)
	}

	// --- Re-enable (on again) ---
	if err := resolver.Set(wsconfig.ItemPreferMercenary, "true", wsconfig.SetOptions{SessionKey: key}); err != nil {
		t.Fatalf("re-enable prefer_mercenary: %v", err)
	}
	reEnabled, _, err := resolver.GetBool(key, wsconfig.ItemPreferMercenary)
	if err != nil {
		t.Fatalf("GetBool after re-enable: %v", err)
	}
	if !reEnabled {
		t.Fatalf("re-enable: got false, want true")
	}
	bodyOn2, _, err := renderPlaybookBody(s, root, "impl-pb", nil, wsconfig.Options{}, "", reEnabled)
	if err != nil {
		t.Fatalf("renderPlaybookBody re-on: %v", err)
	}
	if !strings.Contains(bodyOn2, "prefer_mercenary active") {
		t.Errorf("guidance block must return after re-enable:\n%s", bodyOn2)
	}
}

// TestPreferMercenaryLegacyEnableShape verifies backward-compatible call shape:
// ws.lead.prefer_mercenary called without "enabled" argument still enables.
func TestPreferMercenaryLegacyEnableShape(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	// No "enabled" argument — omitting it defaults to true.
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ws.lead.prefer_mercenary","arguments":{}}}` + "\n"
	var out bytes.Buffer
	if err := serveStdioWithSession(t, NewServer(root, "test"), root, input, &out); err != nil {
		t.Fatalf("ServeStdio: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	text := toolText(t, byID["1"])
	if !strings.Contains(text, "prefer_mercenary: enabled") {
		t.Fatalf("legacy shape (no enabled arg) must enable: %s", byID["1"])
	}
}

// TestPreferMercenaryDisableViaEnabledFalse verifies the new desired-state
// disable path: ws.lead.prefer_mercenary(session_key, enabled:false) → "disabled".
func TestPreferMercenaryDisableViaEnabledFalse(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	// Enable first (no enabled arg = default true).
	inputEnable := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ws.lead.prefer_mercenary","arguments":{}}}` + "\n"
	// Then disable.
	inputDisable := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ws.lead.prefer_mercenary","arguments":{"enabled":false}}}` + "\n"
	input := inputEnable + inputDisable
	var out bytes.Buffer
	s := NewServer(root, "test")
	if err := serveStdioWithSession(t, s, root, input, &out); err != nil {
		t.Fatalf("ServeStdio: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if !strings.Contains(toolText(t, byID["1"]), "prefer_mercenary: enabled") {
		t.Fatalf("first call (enable) must return enabled: %s", byID["1"])
	}
	if !strings.Contains(toolText(t, byID["2"]), "prefer_mercenary: disabled") {
		t.Fatalf("second call (disable) must return disabled: %s", byID["2"])
	}
}

// TestPreferMercenaryConfigShowReportsIt verifies that config.show with a
// session_key reports prefer_mercenary with its resolved scope after a set.
func TestPreferMercenaryConfigShowReportsIt(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	// Use callToolOnce to issue sequential (non-concurrent) MCP calls so the
	// session write from the enable call is visible to the subsequent show call.
	// ServeStdio dispatches requests concurrently, so bundling both calls in one
	// ServeStdio invocation creates a race; sequential callToolOnce calls avoid it.
	s := NewServer(root, "test")

	// Bootstrap a lead session key.
	key, _ := parseLoginResponse(t, callLogin(t, s, 900001, root, nil))

	// Enable prefer_mercenary for the lead key.
	enableResp := callToolOnce(t, s, 1, "ws.lead.prefer_mercenary", map[string]any{"session_key": key})
	enableText := toolText(t, enableResp)
	if !strings.Contains(enableText, "prefer_mercenary: enabled") {
		t.Fatalf("enable call did not succeed: %s", enableResp)
	}

	// config.show must now report prefer_mercenary with session scope.
	showResp := callToolOnce(t, s, 2, "config.show", map[string]any{"session_key": key})
	showText := toolText(t, showResp)
	if !strings.Contains(showText, "prefer_mercenary") {
		t.Fatalf("config.show must report prefer_mercenary: %s", showText)
	}
	if !strings.Contains(showText, "session") {
		t.Fatalf("config.show must report session scope for prefer_mercenary: %s", showText)
	}
}
