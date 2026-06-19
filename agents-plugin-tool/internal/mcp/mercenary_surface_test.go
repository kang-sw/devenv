package mcp

import (
	"bytes"
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsagent"
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

	body, _, err := renderPlaybookBody(s, root, "impl-pb", nil, wsconfig.Options{}, mintRoot, "", false)
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
		// roleDelegate != roleLead, so this assertion also guarantees the child
		// key is never lead-scoped.
		t.Errorf("minted key scope = %q, want %q (implementer → delegate)", entry.scope, roleDelegate)
	}

	// A second render mints a DISTINCT key (registry uniqueness).
	body2, _, err := renderPlaybookBody(s, root, "impl-pb", nil, wsconfig.Options{}, mintRoot, "", false)
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
	body, _, err := renderPlaybookBody(s, root, "impl-pb", nil, wsconfig.Options{}, "", "", false)
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
	// delegatePlaybookContent is defined in playbook_tools_test.go (same package):
	// delegates:true but NO `role:` field → childRoleForPlaybookRole returns false,
	// so a lead caller still mints nothing.
	root := buildTestRsrcTree(t, map[string]string{
		"delegate-pb/delegate-pb.md": delegatePlaybookContent,
	})
	s := newTestServerWithHarness(t, "claude")

	// Lead caller (mintRoot set) but the playbook role is not delegate-eligible → no mint.
	body, _, err := renderPlaybookBody(s, root, "delegate-pb", nil, wsconfig.Options{}, "/work/tree-a", "", false)
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
	body, _, err := renderPlaybookBody(s, root, "impl-pb", nil, wsconfig.Options{}, overrideRoot, "", false)
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
	bodyOff, _, err := renderPlaybookBody(s, root, "impl-pb", nil, wsconfig.Options{}, "", "", false)
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
	bodyOn, _, err := renderPlaybookBody(s, root, "impl-pb", nil, wsconfig.Options{}, "", "", true)
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
	body, _, err := renderPlaybookBody(s, root, "leaf-pb", nil, wsconfig.Options{}, "", "", true)
	if err != nil {
		t.Fatalf("renderPlaybookBody: %v", err)
	}
	if strings.Contains(body, "prefer_mercenary active") {
		t.Errorf("prefer-mercenary guidance must be implementer/reviewer only, not leaf:\n%s", body)
	}
}

// TestSetPreferMercenaryViaResolver verifies that setting prefer_mercenary through
// the layered config resolver writes to the session Overrides overlay and can be
// read back via GetBool, replacing the former one-way setPreferMercenary path.
func TestSetPreferMercenaryViaResolver(t *testing.T) {
	s := newTestServerWithHarness(t, "claude")
	key, err := s.sessions.mint("/work/root", roleLead, "")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	adapter := sessionConfigAdapter{s: s.sessions}
	resolver := wsconfig.NewResolver(wsconfig.Options{}, nil, adapter, adapter)

	// Enable.
	if err := resolver.Set(wsconfig.ItemPreferMercenary, "true", wsconfig.SetOptions{SessionKey: key}); err != nil {
		t.Fatalf("set prefer_mercenary=true: %v", err)
	}
	got, scope, err := resolver.GetBool(key, wsconfig.ItemPreferMercenary)
	if err != nil {
		t.Fatalf("GetBool: %v", err)
	}
	if !got || scope != wsconfig.ScopeSession {
		t.Errorf("after enable: got=%v scope=%s, want true/session", got, scope)
	}

	// Verify the session entry root/scope were not corrupted.
	entry, _ := s.sessions.lookup(key)
	if entry.root != "/work/root" || entry.scope != roleLead {
		t.Errorf("set corrupted entry: %+v", entry)
	}

	// Disable.
	if err := resolver.Set(wsconfig.ItemPreferMercenary, "false", wsconfig.SetOptions{SessionKey: key}); err != nil {
		t.Fatalf("set prefer_mercenary=false: %v", err)
	}
	got2, scope2, err := resolver.GetBool(key, wsconfig.ItemPreferMercenary)
	if err != nil {
		t.Fatalf("GetBool after disable: %v", err)
	}
	if got2 || scope2 != wsconfig.ScopeSession {
		t.Errorf("after disable: got=%v scope=%s, want false/session", got2, scope2)
	}

	// Unknown key must return an error.
	if err := resolver.Set(wsconfig.ItemPreferMercenary, "true", wsconfig.SetOptions{SessionKey: "no-such-key"}); err == nil {
		t.Errorf("set for unknown session key must return an error")
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
	props := toolPropertiesByName(t, byID["1"], "ws.mercenary.register")
	// prompts/prompt_refs/model stay removed; `tier` is re-introduced in Phase 2
	// (260611) as a pass-through of playbook.render's recommended-tier.
	for _, dropped := range []string{"prompts", "prompt_refs", "model"} {
		if _, present := props[dropped]; present {
			t.Errorf("ws.mercenary.register schema still exposes removed field %q", dropped)
		}
	}
	for _, kept := range []string{"name", "backend", "system_prompt_text", "tier"} {
		if _, present := props[kept]; !present {
			t.Errorf("ws.mercenary.register schema missing expected field %q", kept)
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
	// the bootstrap tool stays visible (wsflow still needs bootstrap).
	if !strings.Contains(byID["1"], `"name":"ws.ferrule"`) {
		t.Fatalf("ws.ferrule must remain visible in no-agent mode: %s", byID["1"])
	}
}

// TestPreferMercenaryRejectedForNonLeadKey exercises the lead-only failure path:
// a delegate-scoped key calling ws.lead.prefer_mercenary is rejected by the
// server-side keyed-handler ws.lead.* gate (not by a tool-local check).
func TestPreferMercenaryRejectedForNonLeadKey(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	// Mint a delegate-scoped key directly (a delegate never logs in; it receives
	// a render-minted key). The keyed gate must reject its ws.lead.* call.
	delegateKey, err := server.sessions.mint(root, roleDelegate, "")
	if err != nil {
		t.Fatalf("mint delegate key: %v", err)
	}
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ws.lead.prefer_mercenary","arguments":{"session_key":"` + delegateKey + `"}}}` + "\n"
	var out bytes.Buffer
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio: %v", err)
	}
	line := strings.TrimSpace(out.String())
	if strings.Contains(line, "prefer_mercenary: enabled") {
		t.Fatalf("delegate key must NOT enable prefer_mercenary: %s", line)
	}
	if !strings.Contains(line, "ws.lead.prefer_mercenary") || !strings.Contains(line, `"error"`) {
		t.Fatalf("expected a keyed-gate error rejecting ws.lead.prefer_mercenary: %s", line)
	}
	// The delegate key's prefer_mercenary override must remain completely unset in
	// the session Overrides map — the keyed-gate rejection fires before any resolver
	// write, so no value (not even "false") should appear for this key.
	_, ok := server.sessions.getOverride(delegateKey, wsconfig.ItemPreferMercenary)
	if ok {
		t.Fatalf("rejected call must not have written ANY prefer_mercenary value for the delegate key")
	}
}

// TestAgentCallHandleTextShape verifies the native-shaped continuation handle
// (agentId=<name>) so the lead reuses one continuation idiom across the native
// and mercenary paths (Phase 2c parity). Unit-tested directly because the full
// ws.mercenary.call dispatch would require spawning a real backend.
func TestAgentCallHandleTextShape(t *testing.T) {
	got := agentCallHandleText("implementer", "running", 4242)
	if !strings.HasPrefix(got, "agentId=implementer\t") {
		t.Errorf("handle must lead with agentId=<name>: %q", got)
	}
	for _, want := range []string{"status=running", "pid=4242", "SendMessage(to: agentId)"} {
		if !strings.Contains(got, want) {
			t.Errorf("handle text missing %q: %q", want, got)
		}
	}
}

// shippedRsrcRootForTest is the real shipped rsrc tree
// (internal/mcp → repo root → agents-plugin/rsrc).
func shippedRsrcRootForTest() string {
	return filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
}

// TestRenderGoldenShippedDelegateChildKey exercises the render-minted child-key
// splice on the REAL shipped delegate playbooks (implementer, reviewer), not an
// in-memory fixture. This is the coverage that catches "the mechanism exists but
// no shipped asset declares role:" — the gap that 260609 Edition 379ff5e5
// described (every real render had meta.Role == "" so the credential block never
// fired). Closes 260611 Phase 1 gap 1.
func TestRenderGoldenShippedDelegateChildKey(t *testing.T) {
	rsrcRoot := shippedRsrcRootForTest()
	mintRoot := "/work/tree-a"

	for _, name := range []string{"implementer", "reviewer"} {
		t.Run(name, func(t *testing.T) {
			s := newTestServerWithHarness(t, "claude")
			body, _, err := renderPlaybookBody(s, rsrcRoot, name, nil, wsconfig.Options{CacheHome: t.TempDir()}, mintRoot, "", false)
			if err != nil {
				t.Fatalf("renderPlaybookBody(%s): %v", name, err)
			}
			if !strings.Contains(body, "Your ws session_key") {
				t.Fatalf("shipped %s render missing credential block:\n%s", name, body)
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
				t.Errorf("minted key scope = %q, want %q (delegate role)", entry.scope, roleDelegate)
			}
		})
	}
}

// TestRenderGoldenShippedDelegateModelVarsPerHarness verifies the tier model vars
// resolve to per-harness model strings on the REAL shipped delegate playbooks.
// implementer declares {{.CoreModel}} (tier medium↦core alias); reviewer declares
// {{.DeepModel}} (tier large↦deep alias). An isolated empty CacheHome yields the
// built-in default aliases (claude: core→sonnet, deep→opus; codex: core/deep→gpt-5.5),
// so the assertions are deterministic and config-independent. Closes 260611 Phase 1
// gap 2 (260609 Edition 379ff5e5: tier model vars never surfaced on a shipped asset).
func TestRenderGoldenShippedDelegateModelVarsPerHarness(t *testing.T) {
	rsrcRoot := shippedRsrcRootForTest()

	render := func(t *testing.T, name, harness string) string {
		t.Helper()
		s := newTestServerWithHarness(t, harness)
		body, _, err := renderPlaybookBody(s, rsrcRoot, name, nil, wsconfig.Options{CacheHome: t.TempDir()}, "", "", false)
		if err != nil {
			t.Fatalf("renderPlaybookBody(%s, %q): %v", name, harness, err)
		}
		return body
	}

	// implementer → CoreModel: claude=sonnet, codex=gpt-5.5 (distinct per harness).
	implClaude := render(t, "implementer", "claude")
	implCodex := render(t, "implementer", "codex")
	if !strings.Contains(implClaude, "sonnet") {
		t.Errorf("implementer (claude) body must surface CoreModel 'sonnet':\n%s", implClaude)
	}
	if !strings.Contains(implCodex, "gpt-5.5") {
		t.Errorf("implementer (codex) body must surface CoreModel 'gpt-5.5':\n%s", implCodex)
	}
	if implClaude == implCodex {
		t.Error("implementer render did not diverge per harness — model var not resolved per harness")
	}

	// reviewer → DeepModel: claude=opus.
	revClaude := render(t, "reviewer", "claude")
	if !strings.Contains(revClaude, "opus") {
		t.Errorf("reviewer (claude) body must surface DeepModel 'opus':\n%s", revClaude)
	}
}

// TestRenderGoldenShippedPhase4Delegates exercises the remaining shipped delegate
// playbooks ported in Phase 4 (260611): the three review partitions and the four
// auxiliary delegates. Each declares a delegate-eligible `role:` so a lead render
// must splice a render-minted child key (scope roleDelegate), and each declares a
// tier model var that must fully substitute (no leftover placeholder).
func TestRenderGoldenShippedPhase4Delegates(t *testing.T) {
	rsrcRoot := shippedRsrcRootForTest()
	mintRoot := "/work/tree-p4"

	names := []string{
		"code-review-correctness", "code-review-fit", "code-review-test",
		"reference-discovery", "mental-model-updater",
		"plan-populator-survey", "plan-populator-research",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			s := newTestServerWithHarness(t, "claude")
			body, _, err := renderPlaybookBody(s, rsrcRoot, name, nil, wsconfig.Options{CacheHome: t.TempDir()}, mintRoot, "", false)
			if err != nil {
				t.Fatalf("renderPlaybookBody(%s): %v", name, err)
			}
			if !strings.Contains(body, "Your ws session_key") {
				t.Fatalf("shipped %s render missing credential block:\n%s", name, body)
			}
			key := extractSplicedKey(t, body)
			entry, ok := s.sessions.lookup(key)
			if !ok {
				t.Fatalf("minted key %q not found in registry", key)
			}
			if entry.root != mintRoot {
				t.Errorf("%s minted key root = %q, want %q", name, entry.root, mintRoot)
			}
			if entry.scope != roleDelegate {
				t.Errorf("%s minted key scope = %q, want %q", name, entry.scope, roleDelegate)
			}
			if strings.Contains(body, "{{.") {
				t.Errorf("%s render has an unsubstituted variable placeholder:\n%s", name, body)
			}
		})
	}
}

// TestRenderGoldenShippedReviewPartitionIncludesBase verifies the partition
// reviewer playbooks resolve their `includes: [code-reviewer]` flat dep, so the
// shared reviewer base (severity model, output template, doctrine) renders
// alongside the partition-specific scope and checklist.
func TestRenderGoldenShippedReviewPartitionIncludesBase(t *testing.T) {
	rsrcRoot := shippedRsrcRootForTest()
	for _, name := range []string{"code-review-correctness", "code-review-fit", "code-review-test"} {
		t.Run(name, func(t *testing.T) {
			s := newTestServerWithHarness(t, "claude")
			body, _, err := renderPlaybookBody(s, rsrcRoot, name, nil, wsconfig.Options{CacheHome: t.TempDir()}, "", "", false)
			if err != nil {
				t.Fatalf("renderPlaybookBody(%s): %v", name, err)
			}
			if !strings.Contains(body, "defect signal density") {
				t.Errorf("%s missing included code-reviewer base content:\n%s", name, body)
			}
			if !strings.Contains(body, "Partition scope") {
				t.Errorf("%s missing partition-specific scope section:\n%s", name, body)
			}
		})
	}
}

// --- Phase 2 (260611): tier routing — render-returned recommended tier + register pass-through ---

// TestFirstClassTierToAlias pins the first-class→alias bridge the register handler
// applies to the render-returned recommended tier before wsconfig resolution.
func TestFirstClassTierToAlias(t *testing.T) {
	cases := map[string]string{
		"small":    "light",
		"medium":   "core",
		"large":    "deep",
		"xlarge":   "deep",  // no legacy alias → highest configured tier until Phase 3
		"light":    "light", // alias passes through
		"core":     "core",
		"deep":     "deep",
		"Large":    "deep", // case-insensitive
		" medium ": "core", // trimmed
		"":         "",     // empty → default at Register
		"bogus":    "",     // unknown → default at Register
	}
	for in, want := range cases {
		if got := firstClassTierToAlias(in); got != want {
			t.Errorf("firstClassTierToAlias(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestWithRecommendedTier verifies the render/print return channel: a declared tier
// appends a `recommended-tier:` line; an empty tier leaves the payload unchanged.
func TestWithRecommendedTier(t *testing.T) {
	if got := withRecommendedTier("body", "medium"); got != "body\nrecommended-tier: medium" {
		t.Errorf("withRecommendedTier with tier = %q", got)
	}
	if got := withRecommendedTier("body", "  "); got != "body" {
		t.Errorf("withRecommendedTier with blank tier must be unchanged, got %q", got)
	}
	if got := withRecommendedTier("path", "large"); got != "path\nrecommended-tier: large" {
		t.Errorf("withRecommendedTier path = %q", got)
	}
}

// TestRenderReturnsFrontmatterRecommendedTier verifies renderPlaybookBody surfaces
// the first-class frontmatter tier from the REAL shipped delegate playbooks. This
// is the value the lead routes to both native (model guide) and mercenary (register).
func TestRenderReturnsFrontmatterRecommendedTier(t *testing.T) {
	rsrcRoot := shippedRsrcRootForTest()
	want := map[string]string{"implementer": "medium", "reviewer": "large"}
	for name, wantTier := range want {
		s := newTestServerWithHarness(t, "claude")
		_, tier, err := renderPlaybookBody(s, rsrcRoot, name, nil, wsconfig.Options{CacheHome: t.TempDir()}, "", "", false)
		if err != nil {
			t.Fatalf("renderPlaybookBody(%s): %v", name, err)
		}
		if tier != wantTier {
			t.Errorf("shipped %s recommended tier = %q, want %q (from frontmatter)", name, tier, wantTier)
		}
	}
}

// TestMercenaryTierRoutingResolvesCustomModel is the Phase 2 key coverage: with
// light & deep customized via config.agents_tier, a mercenary registered for a
// small-tier role resolves the custom light model and a large-tier role resolves
// the custom deep model — NOT the built-in core default. firstClassTierToAlias is
// exactly what the ws.mercenary.register handler applies to the render-returned tier
// before Register; this exercises that mapping + the config resolution end to end.
// (A real subprocess is not spawned; Register resolves the backend/model the call
// would use.) Closes 260609 Edition 0c7c0f50 gap 3.
func TestMercenaryTierRoutingResolvesCustomModel(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	cache := filepath.Join(t.TempDir(), "cache")
	// Custom light & deep models distinct from the core default (gpt-5.5): a
	// resolved custom model proves the first-class tier flowed through.
	if _, err := wsconfig.SetAgentsTier(wsconfig.Options{CacheHome: cache}, "light", "", "claude-custom-light"); err != nil {
		t.Fatalf("set light tier: %v", err)
	}
	if _, err := wsconfig.SetAgentsTier(wsconfig.Options{CacheHome: cache}, "deep", "", "gpt-custom-deep"); err != nil {
		t.Fatalf("set deep tier: %v", err)
	}
	mgr := wsagent.NewManager(wsagent.Options{CacheHome: cache})
	cases := []struct {
		name, firstClass, wantBackend, wantModel string
	}{
		{"impl", "small", "claude", "claude-custom-light"},
		{"rev", "large", "codex", "gpt-custom-deep"},
	}
	for _, tc := range cases {
		agent, _, err := mgr.Register(wsagent.RegisterOptions{
			Root:             root,
			Name:             tc.name,
			Tier:             firstClassTierToAlias(tc.firstClass), // handler's pass-through mapping
			SystemPromptText: "x",
		})
		if err != nil {
			t.Fatalf("Register %s: %v", tc.name, err)
		}
		if agent.Backend != tc.wantBackend || agent.Model != tc.wantModel {
			t.Errorf("%s (first-class %q → alias %q): backend/model = %q/%q, want %q/%q (must not pin to core)",
				tc.name, tc.firstClass, firstClassTierToAlias(tc.firstClass),
				agent.Backend, agent.Model, tc.wantBackend, tc.wantModel)
		}
	}
}
