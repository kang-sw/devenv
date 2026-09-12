package mcp

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// playbook.render surface: render-minted child keys, the delegation
// continuity tip, and the recommended-tier return channel.

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
		{"worker", roleLead, true},
		{"Worker", roleLead, true}, // case-insensitive
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

	body, _, err := renderPlaybookBody(s, root, "impl-pb", nil, wsconfig.Options{}, mintRoot, "", "", nil)
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
	body2, _, err := renderPlaybookBody(s, root, "impl-pb", nil, wsconfig.Options{}, mintRoot, "", "", nil)
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
	body, _, err := renderPlaybookBody(s, root, "impl-pb", nil, wsconfig.Options{}, "", "", "", nil)
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
	body, _, err := renderPlaybookBody(s, root, "delegate-pb", nil, wsconfig.Options{}, "/work/tree-a", "", "", nil)
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
	body, _, err := renderPlaybookBody(s, root, "impl-pb", nil, wsconfig.Options{}, overrideRoot, "", "", nil)
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

func TestRenderDispatchSeparatesResourceAndWorktreeRoots(t *testing.T) {
	useLeadProfile(t)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"impl-pb/impl-pb.md": strings.Replace(implementerPlaybookContent, "role: implementer", "role: implementer\nincludes:\n  - shared", 1),
		"shared.md":          "Included from plugin resources.\n",
	})
	t.Setenv("WS_RSRC_ROOT", rsrcRoot)
	repo := initGitRepo(t)
	runGit(t, repo, "commit", "--allow-empty", "-m", "initial")
	worktree := filepath.Join(t.TempDir(), "linked")
	runGit(t, repo, "worktree", "add", "-b", "delegate", worktree)
	if _, err := os.Stat(filepath.Join(worktree, "manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("worktree must not contain a resource manifest: %v", err)
	}
	s := NewServer(repo, "test")
	s.observeHarness("test", "claude")
	leadKey, err := s.sessions.mint(repo, roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, override := range []string{"", worktree} {
		t.Run(map[bool]string{true: "override", false: "no-override"}[override != ""], func(t *testing.T) {
			wantRoot := repo
			args := map[string]any{"name": "impl-pb", "session_key": leadKey}
			if override != "" {
				args["root_override"] = override
				wantRoot = override
			}
			resp := callToolOnce(t, s, 1, "playbook.render", args)
			if toolIsError(t, resp) {
				t.Fatalf("playbook.render: %s", resp)
			}
			path := strings.Split(strings.TrimSpace(toolText(t, resp)), "\n")[0]
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), "Do the work.") || !strings.Contains(string(body), "Included from plugin resources.") {
				t.Fatalf("missing plugin playbook or include: %s", body)
			}
			layout, _, _, err := wsstate.NewManager(wsstate.Options{}).Ensure(wantRoot)
			if err != nil {
				t.Fatal(err)
			}
			if filepath.Dir(path) != layout.PromptDir {
				t.Errorf("artifact directory = %q, want %q", filepath.Dir(path), layout.PromptDir)
			}
			entry, ok := s.sessions.lookup(extractSplicedKey(t, string(body)))
			if !ok || entry.root != wantRoot || entry.scope != roleDelegate || entry.parent != leadKey {
				t.Errorf("child session = %+v, found=%v; want delegate bound to %q with parent %q", entry, ok, wantRoot, leadKey)
			}
		})
	}
}

// --- integration: workflow preference writers ---

// TestWorkflowPreferenceWritersRequireLeadSessionKey verifies that global
// workflow preference writers still require lead authority even though they
// write global config.
func TestWorkflowPreferenceWritersRequireLeadSessionKey(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	// Mint a delegate-scoped key directly (a delegate never logs in; it receives
	// a render-minted key). The keyed gate must reject workflow preference writes.
	delegateKey, err := server.sessions.mint(root, roleDelegate, "")
	if err != nil {
		t.Fatalf("mint delegate key: %v", err)
	}

	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{
			name: "config.tune",
			args: map[string]any{"key": "workflow.prefer_subagent", "value": "on"},
		},
	} {
		resp := callToolOnce(t, server, 1, tc.name, tc.args)
		if !toolIsError(t, resp) || !strings.Contains(toolText(t, resp), "session_key is required") {
			t.Fatalf("%s keyless write must require session_key: %s", tc.name, resp)
		}

		argsWithDelegateKey := map[string]any{}
		for key, value := range tc.args {
			argsWithDelegateKey[key] = value
		}
		argsWithDelegateKey["session_key"] = delegateKey
		resp = callToolOnce(t, server, 2, tc.name, argsWithDelegateKey)
		if strings.Contains(resp, "workflow.prefer_") && strings.Contains(resp, "[scope:global]") {
			t.Fatalf("delegate key must NOT write %s: %s", tc.name, resp)
		}
		if !strings.Contains(resp, tc.name) || !strings.Contains(resp, `"error"`) {
			t.Fatalf("expected a keyed-gate error rejecting %s: %s", tc.name, resp)
		}
	}

	showResp := callToolOnce(t, server, 2, "config.list", map[string]any{"format": "json"})
	if strings.Contains(toolText(t, showResp), `"scope":"global"`) {
		t.Fatalf("rejected delegate call must not write global workflow preference: %s", showResp)
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
			var ctx map[string]string
			if name == "implementer" {
				ctx = shippedImplementerContext()
			}
			body, _, err := renderPlaybookBody(s, rsrcRoot, name, ctx, wsconfig.Options{CacheHome: t.TempDir()}, mintRoot, "", "", nil)
			if err != nil {
				t.Fatalf("renderPlaybookBody(%s): %v", name, err)
			}
			if !strings.Contains(body, "Your ws session_key") {
				t.Fatalf("shipped %s render missing credential block:\n%s", name, body)
			}
			if name == "reviewer" {
				for _, want := range []string{
					"delegate-grade wrapper for full-scope code review",
					"Use only the authority named by the prompt frame",
					"otherwise cover correctness, standards, contracts, security, tests, edge cases, and reuse",
					"Always write the detailed report to the invocation's findings path",
					"Return only `clean`, `clean with N minor remaining`, or `non-clean: M critical/important`",
				} {
					if !strings.Contains(body, want) {
						t.Fatalf("shipped reviewer wrapper missing shared contract %q:\n%s", want, body)
					}
				}
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

// TestRenderGoldenShippedDelegatesContainNoModelAliases verifies concrete model
// names are returned as render metadata, not self-reported in worker prompts.
func TestRenderGoldenShippedDelegatesContainNoModelAliases(t *testing.T) {
	rsrcRoot := shippedRsrcRootForTest()

	render := func(t *testing.T, name, harness string) string {
		t.Helper()
		s := newTestServerWithHarness(t, harness)
		var ctx map[string]string
		if name == "implementer" {
			ctx = shippedImplementerContext()
		}
		body, _, err := renderPlaybookBody(s, rsrcRoot, name, ctx, wsconfig.Options{CacheHome: t.TempDir()}, "", "", "", nil)
		if err != nil {
			t.Fatalf("renderPlaybookBody(%s, %q): %v", name, harness, err)
		}
		return body
	}

	// Removing the RoleModel line makes the shared implementer body identical
	// across harnesses.
	implClaude := render(t, "implementer", "claude")
	implCodex := render(t, "implementer", "codex")
	if implClaude != implCodex {
		t.Error("implementer body must not vary by the removed model alias")
	}

	for name, body := range map[string]string{
		"implementer":         implCodex,
		"reviewer":            render(t, "reviewer", "claude"),
		"reference-discovery": render(t, "reference-discovery", "claude"),
	} {
		if strings.Contains(body, "Alias model for this role:") {
			t.Errorf("%s still self-reports a model alias:\n%s", name, body)
		}
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
		"reference-discovery",
		"ticket-reviewer-design", "ticket-reviewer-completeness",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			s := newTestServerWithHarness(t, "claude")
			body, _, err := renderPlaybookBody(s, rsrcRoot, name, nil, wsconfig.Options{CacheHome: t.TempDir()}, mintRoot, "", "", nil)
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
			body, _, err := renderPlaybookBody(s, rsrcRoot, name, nil, wsconfig.Options{CacheHome: t.TempDir()}, "", "", "", nil)
			if err != nil {
				t.Fatalf("renderPlaybookBody(%s): %v", name, err)
			}
			if !strings.Contains(body, "defect signal density") {
				t.Errorf("%s missing included code-reviewer base content:\n%s", name, body)
			}
			if !strings.Contains(body, "Partition scope") {
				t.Errorf("%s missing partition-specific scope section:\n%s", name, body)
			}
			for _, want := range []string{
				"Use only the authority named by the prompt frame: ticket or accepted inline contract.",
				"Read the ticket or inline contract named by the prompt frame; never require a ticket for inline authority.",
			} {
				if !strings.Contains(body, want) {
					t.Errorf("%s missing reviewer base contract %q:\n%s", name, want, body)
				}
			}
		})
	}
}

// --- Phase 1 (260620): capability tier routing — register pass-through to capability vocabulary ---

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

func TestWithRecommendedRenderBinding(t *testing.T) {
	cases := []struct {
		name    string
		harness string
		setup   func(t *testing.T, cacheHome string)
		want    string
	}{
		{
			name:    "default codex model and effort",
			harness: "codex",
			want: "path\nrecommended-tier: medium\nrecommended-model: gpt-5.6-terra\n" +
				"recommended-reasoning-effort: high",
		},
		{
			name:    "codex local override",
			harness: "codex",
			setup: func(t *testing.T, cacheHome string) {
				t.Helper()
				if _, err := wsconfig.SetAgentsTierForHarness(wsconfig.Options{CacheHome: cacheHome}, "medium", "codex", "local-model", "codex", "xhigh"); err != nil {
					t.Fatalf("set codex override: %v", err)
				}
			},
			want: "path\nrecommended-tier: medium\nrecommended-model: local-model\n" +
				"recommended-reasoning-effort: xhigh",
		},
		{
			name:    "claude omits effort",
			harness: "claude",
			want:    "path\nrecommended-tier: medium\nrecommended-model: sonnet",
		},
		{
			name:    "resolver failure preserves tier",
			harness: "codex",
			setup: func(t *testing.T, cacheHome string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(cacheHome, "config.json"), []byte("{not valid json"), 0o644); err != nil {
					t.Fatalf("write malformed config: %v", err)
				}
			},
			want: "path\nrecommended-tier: medium",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cacheHome := t.TempDir()
			if tc.setup != nil {
				tc.setup(t, cacheHome)
			}
			got := withRecommendedRenderBinding("path", tc.harness, "medium", wsconfig.Options{CacheHome: cacheHome})
			if got != tc.want {
				t.Fatalf("render metadata = %q, want %q", got, tc.want)
			}
		})
	}
	if got := withRecommendedRenderBinding("path", "codex", "", wsconfig.Options{}); got != "path" {
		t.Errorf("empty tier must not add render metadata, got %q", got)
	}
}

// TestRenderReturnsFrontmatterRecommendedTier verifies renderPlaybookBody surfaces
// the first-class frontmatter tier from the REAL shipped delegate playbooks. This
// is the value the lead routes to the host as the subagent's model guide.
func TestRenderReturnsFrontmatterRecommendedTier(t *testing.T) {
	rsrcRoot := shippedRsrcRootForTest()
	want := map[string]string{
		"implementer":                  "medium",
		"reviewer":                     "large",
		"ticket-reviewer-design":       "large",
		"ticket-reviewer-completeness": "medium",
	}
	for name, wantTier := range want {
		s := newTestServerWithHarness(t, "claude")
		var ctx map[string]string
		if name == "implementer" {
			ctx = shippedImplementerContext()
		}
		_, tier, err := renderPlaybookBody(s, rsrcRoot, name, ctx, wsconfig.Options{CacheHome: t.TempDir()}, "", "", "", nil)
		if err != nil {
			t.Fatalf("renderPlaybookBody(%s): %v", name, err)
		}
		if tier != wantTier {
			t.Errorf("shipped %s recommended tier = %q, want %q (from frontmatter)", name, tier, wantTier)
		}
	}
}
