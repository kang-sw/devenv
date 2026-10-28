package mcp

import (
	"path/filepath"
	"strings"
	"testing"
)

// The executor is a host-native procedure. These checks exercise its real MCP
// entry and namespace rendering; they do not simulate native agent lifecycle.
func TestLeadDelegateReadContract(t *testing.T) {
	for _, namespace := range []string{"ws", "wsflow"} {
		t.Run(namespace, func(t *testing.T) {
			useLeadProfile(t)
			t.Setenv(envNamespace, namespace)
			if namespace == "wsflow" {
				t.Setenv(envNoAgent, "1")
			}
			t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))
			t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
			t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
			s := newTestServerWithHarness(t, "codex")
			body := toolText(t, callToolOnce(t, s, 1, "playbook.read", map[string]any{"name": "lead-delegate"}))
			for _, want := range []string{
				"# Delegate",
				"Route collaborative direction-setting to " + namespace + ":lead-discuss",
				namespace + ":lead-ticket", namespace + ":lead-review", namespace + ":lead-ship",
				"Default to delegate when the outcome is settled and you can name the files\nbefore starting",
				"a decision is still open, the work adds new structure (a new type, exported\nsymbol, module, or externally observed contract) whose reuse and fit need a\ndesign read before code, the work spans phases or outlives this session, or\nthe scope cannot be named up front",
				"A ready ticket that already owns the work routes\nto " + namespace + ":lead-run directly",
				"A local NEEDS FIX repair handed off\nby " + namespace + ":lead-review takes this same gate",
				"Independent review comes after the commit, so the need for one does not route\nwork to a ticket",
				"`AGENTS.md` `### Review Policy` declares a\n`review-track`",
				"Without one, offer " + namespace + ":lead-review over the delegate's commits\nonce it returns",
				"stops before\nthat expanded change",
				"captures the\nwork as a ticket through " + namespace + ":lead-ticket",
				"carrying the delegation's brief into the ticket",
				"ready ticket then routes to " + namespace + ":lead-run",
				namespace + "/playbook.render(name:\n\"delegate-implementer\"",
				"context: {plan_stem: <short\nkebab-case task name>}",
				"When the render also returns a `plan-path:` line",
				"write the brief into it before spawning",
				"Never edit it\nonce the executor starts",
				"Continuing the same agent\ngets no new render or plan",
				"Delete the plan when no executor commits it",
				"there is no\nfixed executor role or read-only default",
				"Permissions stay within existing\nuser authorization",
				"Start a fresh agent for a new assignment or an independent judgment",
				"Continue the same agent for follow-up work on its existing assignment",
				"session-local label, native task handle, purpose, and active or\nreleased state",
				"through lead compaction; no cross-session registry is required",
				"If the native handle is unavailable, start a fresh agent",
				"replacement, not a resumed agent",
				"Use the host's lifecycle capabilities",
				"Send corrections or missing\nwork back to the same agent",
				"no fixed terminal\nblock is required",
			} {
				if !strings.Contains(body, want) {
					t.Errorf("rendered contract missing %q", want)
				}
			}
			for _, forbidden := range []string{
				"{{.", "lead-prefer-subagent", "route.resolve_implement", "## Worker Protocol",
				// The retired touched-surface gate sent nearly every shipped-text
				// change to a ticket; it must not come back.
				"public behavior, an API, protocol, schema, template",
				"independent review is needed",
				"bounded, reversible,\nself-verifying task",
			} {
				if strings.Contains(body, forbidden) {
					t.Errorf("rendered contract contains %q", forbidden)
				}
			}
			retired := callToolOnce(t, s, 2, "playbook.read", map[string]any{"name": "lead-prefer-subagent"})
			if !strings.Contains(retired, `"isError":true`) {
				t.Fatalf("retired procedure still resolves: %s", retired)
			}
		})
	}
}
