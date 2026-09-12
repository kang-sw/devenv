package mcp

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLeadTicketResearchDerivationContract(t *testing.T) {
	for _, namespace := range []string{"ws", "wsflow"} {
		t.Run(namespace, func(t *testing.T) {
			useLeadProfile(t)
			t.Setenv(envNamespace, namespace)
			pkg := "agents-plugin"
			if namespace == "wsflow" {
				t.Setenv(envNoAgent, "1")
				pkg = "agents-plugin-wsflow"
			}
			t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", pkg, "rsrc"))
			t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
			t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
			s := newTestServerWithHarness(t, "codex")
			body := toolText(t, callToolOnce(t, s, 1, "playbook.read", map[string]any{"name": "lead-ticket"}))
			for _, want := range []string{
				"Persist only decisions the user confirmed. Research tickets may preserve explicitly non-authoritative proposals and open questions in their Outcome Ledger.",
				"When deriving actionable work from research, treat the Outcome Ledger as the sole authority: use `Verified Findings` as evidence and `Confirmed Decisions` as contract; read the narrative only as supporting context, and never promote `Proposals`, `Open Questions`, or unlisted narrative into the child.",
				"If the research has no Outcome Ledger, stop and ask whether to add one or settle the child’s decisions directly through the Open Decision Queue.",
				namespace + "/tickets.checklist",
			} {
				if !strings.Contains(body, want) {
					t.Errorf("rendered contract missing %q", want)
				}
			}
			normalized := strings.Join(strings.Fields(body), " ")
			if !strings.Contains(normalized, "may be preserved without settlement; they do not open a queue item unless a decision is needed") {
				t.Error("Open Decision Queue must allow non-authoritative research capture")
			}
		})
	}
}
