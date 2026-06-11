package mcp

import (
	"bytes"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

// Phase 2c: mercenary delegation surface — render-minted child keys,
// prefer_mercenary render-mode flip, always-on tip, register schema narrowing.

// implementerPlaybookContent is a delegate-eligible (role: implementer) playbook.
const implementerPlaybookContent = `---
kind: render
delegates: true
role: implementer
---
# Implementer Playbook

Do the work.
`

// leafPlaybookContent is a leaf-role delegation playbook.
const leafPlaybookContent = `---
kind: render
delegates: true
role: leaf
---
# Leaf Playbook

Leaf work.
`

var sessionKeyInBodyRe = regexp.MustCompile("session_key: `([^`]+)`")

func extractSplicedKey(t *testing.T, body string) string {
	t.Helper()
	m := sessionKeyInBodyRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("body has no spliced session_key block:\n%s", body)
	}
	return m[1]
}

func TestChildRoleForPlaybookRole(t *testing.T) {
	cases := []struct {
		role      string
		wantScope toolRole
		wantOK    bool
	}{
		{"implementer", roleDelegate, true},
		{"reviewer", roleDelegate, true},
		{"delegate", roleDelegate, true},
		{"Implementer", roleDelegate, true}, // case-insensitive
		{" reviewer ", roleDelegate, true},  // trimmed
		{"leaf", roleLeaf, true},
		{"lead", "", false},
		{"", "", false},
		{"bogus", "", false},
	}
	for _, c := range cases {
		gotScope, gotOK := childRoleForPlaybookRole(c.role)
		if gotScope != c.wantScope || gotOK != c.wantOK {
			t.Errorf("childRoleForPlaybookRole(%q) = (%q,%v), want (%q,%v)",
				c.role, gotScope, gotOK, c.wantScope, c.wantOK)
		}
	}
}

func TestRenderMintsChildKeyForLeadDelegatePlaybook(t *testing.T) {
	root := buildTestRsrcTree(t, map[string]string{
		"impl-pb/impl-pb.md": implementerPlaybookContent,
	})
	s := newTestServerWithHarness(t, "claude")
	mintRoot := "/work/tree-a"

	body, err := renderPlaybookBody(s, root, "impl-pb", nil, wsconfig.Options{}, mintRoot, false)
	if err != nil {
		t.Fatalf("renderPlaybookBody: %v", err)
	}
	key := extractSplicedKey(t, body)
	entry, ok := s.sessions.lookup(key)
	if !ok {
		t.Fatalf("minted key %q not found in registry", key)
	}
	if entry.root != mintRoot {
		t.Errorf("minted key root = %q, want %q", entry.root, mintRoot)
	}
	if entry.scope != roleDelegate {
		t.Errorf("minted key scope = %q, want %q (implementer → delegate)", entry.scope, roleDelegate)
	}
	if entry.scope == roleLead {
		t.Errorf("child key must never be lead-scoped")
	}

	// A second render mints a DISTINCT key (registry uniqueness).
	body2, err := renderPlaybookBody(s, root, "impl-pb", nil, wsconfig.Options{}, mintRoot, false)
	if err != nil {
		t.Fatalf("renderPlaybookBody (2nd): %v", err)
	}
	if key2 := extractSplicedKey(t, body2); key2 == key {
		t.Errorf("second render reused the same child key %q; must be distinct", key2)
	}
}

func TestRenderNoMintForNonLeadCaller(t *testing.T) {
	root := buildTestRsrcTree(t, map[string]string{
		"impl-pb/impl-pb.md": implementerPlaybookContent,
	})
	s := newTestServerWithHarness(t, "claude")

	// mintRoot empty → caller is not a lead → no mint, no key block.
	body, err := renderPlaybookBody(s, root, "impl-pb", nil, wsconfig.Options{}, "", false)
	if err != nil {
		t.Fatalf("renderPlaybookBody: %v", err)
	}
	if sessionKeyInBodyRe.MatchString(body) {
		t.Errorf("non-lead render must not splice a session_key:\n%s", body)
	}
	if strings.Contains(body, "Your ws session_key") {
		t.Errorf("non-lead render must not contain a credential block:\n%s", body)
	}
}

func TestRenderNoMintForNonDelegateRole(t *testing.T) {
	// delegatePlaybookContent has delegates:true but NO role → not delegate-eligible.
	root := buildTestRsrcTree(t, map[string]string{
		"delegate-pb/delegate-pb.md": delegatePlaybookContent,
	})
	s := newTestServerWithHarness(t, "claude")

	// Lead caller (mintRoot set) but the playbook role is not delegate-eligible → no mint.
	body, err := renderPlaybookBody(s, root, "delegate-pb", nil, wsconfig.Options{}, "/work/tree-a", false)
	if err != nil {
		t.Fatalf("renderPlaybookBody: %v", err)
	}
	if sessionKeyInBodyRe.MatchString(body) {
		t.Errorf("non-delegate-role render must not splice a session_key:\n%s", body)
	}
}

func TestRenderRootOverrideBindsChildKey(t *testing.T) {
	root := buildTestRsrcTree(t, map[string]string{
		"impl-pb/impl-pb.md": implementerPlaybookContent,
	})
	s := newTestServerWithHarness(t, "claude")
	overrideRoot := "/work/worktree-override"

	// renderPlaybookBody binds the minted key to mintRoot; the dispatch passes
	// root_override as mintRoot when set (server.go playbook.render handler).
	body, err := renderPlaybookBody(s, root, "impl-pb", nil, wsconfig.Options{}, overrideRoot, false)
	if err != nil {
		t.Fatalf("renderPlaybookBody: %v", err)
	}
	key := extractSplicedKey(t, body)
	entry, ok := s.sessions.lookup(key)
	if !ok {
		t.Fatalf("minted key %q not found", key)
	}
	if entry.root != overrideRoot {
		t.Errorf("minted key bound to %q, want override root %q", entry.root, overrideRoot)
	}
}

func TestPreferMercenaryGuidanceAndAlwaysOnTip(t *testing.T) {
	root := buildTestRsrcTree(t, map[string]string{
		"impl-pb/impl-pb.md": implementerPlaybookContent,
	})
	s := newTestServerWithHarness(t, "claude")

	// preferMercenary=false: always-on mercenary tip present (delegates:true),
	// but the prefer-mercenary "Delegation mode" guidance block absent.
	bodyOff, err := renderPlaybookBody(s, root, "impl-pb", nil, wsconfig.Options{}, "", false)
	if err != nil {
		t.Fatalf("renderPlaybookBody off: %v", err)
	}
	if !strings.Contains(bodyOff, "Mercenary path (always available)") {
		t.Errorf("always-on mercenary tip missing for delegates:true playbook:\n%s", bodyOff)
	}
	if strings.Contains(bodyOff, "prefer_mercenary active") {
		t.Errorf("prefer-mercenary guidance must be absent when flag off:\n%s", bodyOff)
	}

	// preferMercenary=true on an implementer playbook: guidance block present.
	bodyOn, err := renderPlaybookBody(s, root, "impl-pb", nil, wsconfig.Options{}, "", true)
	if err != nil {
		t.Fatalf("renderPlaybookBody on: %v", err)
	}
	if !strings.Contains(bodyOn, "prefer_mercenary active") {
		t.Errorf("prefer-mercenary guidance missing when flag on for implementer:\n%s", bodyOn)
	}
	if !strings.Contains(bodyOn, "Mercenary path (always available)") {
		t.Errorf("always-on tip must remain present when flag on:\n%s", bodyOn)
	}
}

func TestPreferMercenaryGuidanceAbsentForNonImplementerRole(t *testing.T) {
	root := buildTestRsrcTree(t, map[string]string{
		"leaf-pb/leaf-pb.md": leafPlaybookContent,
	})
	s := newTestServerWithHarness(t, "claude")

	// preferMercenary=true but role is leaf (not implementer/reviewer): no guidance block.
	body, err := renderPlaybookBody(s, root, "leaf-pb", nil, wsconfig.Options{}, "", true)
	if err != nil {
		t.Fatalf("renderPlaybookBody: %v", err)
	}
	if strings.Contains(body, "prefer_mercenary active") {
		t.Errorf("prefer-mercenary guidance must be implementer/reviewer only, not leaf:\n%s", body)
	}
}

func TestSetPreferMercenaryRegistry(t *testing.T) {
	s := newTestServerWithHarness(t, "claude")
	key, err := s.sessions.mint("/work/root", roleLead)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if ok := s.sessions.setPreferMercenary(key); !ok {
		t.Fatalf("setPreferMercenary returned false for known key")
	}
	entry, _ := s.sessions.lookup(key)
	if !entry.preferMercenary {
		t.Errorf("preferMercenary flag not set after setPreferMercenary")
	}
	if entry.root != "/work/root" || entry.scope != roleLead {
		t.Errorf("setPreferMercenary corrupted entry: %+v", entry)
	}
	if ok := s.sessions.setPreferMercenary("no-such-key"); ok {
		t.Errorf("setPreferMercenary returned true for unknown key")
	}
}

// --- integration: register schema narrowing + prefer_mercenary handler ---

func TestRegisterSchemaDropsLegacyFields(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}` + "\n"
	var out bytes.Buffer
	if err := serveStdioWithSession(t, NewServer(root, "test"), root, input, &out); err != nil {
		t.Fatalf("ServeStdio: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	props := toolPropertiesByName(t, byID["1"], "agents.register")
	for _, dropped := range []string{"prompts", "prompt_refs", "tier", "model"} {
		if _, present := props[dropped]; present {
			t.Errorf("agents.register schema still exposes removed field %q", dropped)
		}
	}
	for _, kept := range []string{"name", "backend", "system_prompt_text"} {
		if _, present := props[kept]; !present {
			t.Errorf("agents.register schema missing expected field %q", kept)
		}
	}
}

func TestPreferMercenaryEnabledForLeadKey(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	// serveStdioWithSession logs in as a lead key and injects it into the call.
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ws.lead.prefer_mercenary","arguments":{}}}` + "\n"
	var out bytes.Buffer
	if err := serveStdioWithSession(t, NewServer(root, "test"), root, input, &out); err != nil {
		t.Fatalf("ServeStdio: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	text := toolText(t, byID["1"])
	if !strings.Contains(text, "prefer_mercenary: enabled") {
		t.Fatalf("lead prefer_mercenary did not enable: %s", byID["1"])
	}
}

func TestPreferMercenaryHiddenInNoAgentMode(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_MCP_NO_AGENT", "1")
	t.Setenv("WS_MCP_NAMESPACE", "wsflow")

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}` + "\n"
	var out bytes.Buffer
	if err := serveStdioWithSession(t, NewServer(root, "test"), root, input, &out); err != nil {
		t.Fatalf("ServeStdio: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	// Use a tool-name boundary check, not a raw substring (the prefer_mercenary
	// description text would otherwise false-match). The tool is hidden, so its
	// name token "ws.lead.prefer_mercenary" must not appear as a list entry.
	if strings.Contains(byID["1"], `"name":"ws.lead.prefer_mercenary"`) {
		t.Fatalf("ws.lead.prefer_mercenary must be hidden in no-agent (wsflow) mode: %s", byID["1"])
	}
	// ws.lead.login stays visible (wsflow still needs bootstrap).
	if !strings.Contains(byID["1"], `"name":"ws.lead.login"`) {
		t.Fatalf("ws.lead.login must remain visible in no-agent mode: %s", byID["1"])
	}
}
