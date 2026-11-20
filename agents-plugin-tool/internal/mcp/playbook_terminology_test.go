// playbook_terminology_test.go checks harness wording and delegation text.
package mcp

import (
	"github.com/kang-sw/devenv/internal/wsconfig"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// playbook.read — golden harness rendering
// ---------------------------------------------------------------------------

func TestPlaybookPrintUnknownHarness(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"delegate-pb/delegate-pb.md": delegatePlaybookContent,
	})
	s := newTestServerWithHarness(t, "") // no harness → host-neutral

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "delegate-pb", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	neutral := terminologyForHarness("", RuntimeNamespace())
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, neutral[varName]) {
			t.Errorf("body %q: expected neutral %s %q", body, varName, neutral[varName])
		}
	}
	// Placeholders must be substituted.
	if strings.Contains(body, "{{.") {
		t.Errorf("body %q: unsubstituted placeholder remains", body)
	}
}

func TestPlaybookPrintClaudeHarness(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"delegate-pb/delegate-pb.md": delegatePlaybookContent,
	})
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "delegate-pb", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	claudeTerm := terminologyForHarness("claude", RuntimeNamespace())
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, claudeTerm[varName]) {
			t.Errorf("body %q: expected claude %s %q", body, varName, claudeTerm[varName])
		}
	}
	// Codex terms must NOT appear for any var (proves harness selection on all vars).
	codexTerm := terminologyForHarness("codex", RuntimeNamespace())
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if claudeTerm[varName] != codexTerm[varName] {
			if strings.Contains(body, codexTerm[varName]) {
				t.Errorf("body %q: codex %s term %q must not appear in claude render", body, varName, codexTerm[varName])
			}
		}
	}
}

// TestPlaybookPrintPiHarnessSelectsOverlay proves a `.pi.md` structural
// overlay is selected only under a detected `pi` harness — the only piece
// gating overlay selection is that s.currentHarness() can now return "pi"
// (wsrsrc.Load itself validates harness only as a bare filesystem stem, with
// no closed enum, so no wsrsrc code change is required for this phase).
func TestPlaybookPrintPiHarnessSelectsOverlay(t *testing.T) {
	const baseText = "Base playbook body."
	const overlayText = "Pi overlay playbook body."
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"overlay-pb/overlay-pb.md":    "---\nkind: print\ndelegates: false\n---\n# Overlay Playbook\n\n" + baseText + "\n",
		"overlay-pb/overlay-pb.pi.md": "---\nkind: print\ndelegates: false\n---\n# Overlay Playbook (Pi)\n\n" + overlayText + "\n",
	})

	piServer := newTestServerWithHarness(t, "pi")
	piBody, _, err := printPlaybook(piServer, os.DirFS(rsrcRoot), "overlay-pb", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook (pi): %v", err)
	}
	if !strings.Contains(piBody, overlayText) {
		t.Errorf("pi body %q: expected pi overlay text %q", piBody, overlayText)
	}
	if strings.Contains(piBody, baseText) {
		t.Errorf("pi body %q: base text must not appear when overlay is selected", piBody)
	}

	for _, harness := range []string{"", "codex"} {
		s := newTestServerWithHarness(t, harness)
		body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "overlay-pb", nil, wsconfig.Options{}, "", nil)
		if err != nil {
			t.Fatalf("printPlaybook (%q): %v", harness, err)
		}
		if !strings.Contains(body, baseText) {
			t.Errorf("harness %q body %q: expected base text", harness, body)
		}
		if strings.Contains(body, overlayText) {
			t.Errorf("harness %q body %q: pi overlay must not be selected for a non-pi harness", harness, body)
		}
	}
}

// An unmarked Pi client receives neutral idioms rather than unavailable tools.
func TestPlaybookPrintPiHarnessUsesPiTerminology(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"delegate-pb/delegate-pb.md": delegatePlaybookContent,
	})
	s := newTestServerWithHarness(t, "pi")

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "delegate-pb", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	want := map[string]string{
		"ExploreAgent":  "an exploration agent",
		"SpawnIdiom":    "spawning a subagent",
		"ContinueIdiom": "resuming the agent using its returned id",
	}
	piTerm := terminologyForHarness("pi", RuntimeNamespace())
	for varName, term := range want {
		if piTerm[varName] != term {
			t.Errorf("terminologyForHarness(pi)[%s] = %q, want %q", varName, piTerm[varName], term)
		}
		if !strings.Contains(body, term) {
			t.Errorf("body %q: expected Pi %s %q", body, varName, term)
		}
	}
}

// TestPlaybookPrintPiDelegationFlowPlaybooks exercises the shared sources that
// dogfood relies on: their Pi render must need terminology substitution only,
// without a structural .pi.md overlay.
func TestPlaybookPrintPiDelegationFlowPlaybooks(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "pi")
	piTerm := terminologyForHarness("pi", RuntimeNamespace())

	cases := map[string][]string{
		"lead-discuss":  {"ExploreAgent", "ContinueIdiom"},
		"lead-run":      {"SpawnIdiom"},
		"ticket-worker": {"ExploreAgent", "SpawnIdiom", "ContinueIdiom"},
	}
	for name, variableNames := range cases {
		t.Run(name, func(t *testing.T) {
			body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), name, nil, isolatedPlaybookConfigOptions(t), "", nil)
			if err != nil {
				t.Fatalf("printPlaybook: %v", err)
			}
			for _, variableName := range variableNames {
				if !strings.Contains(body, piTerm[variableName]) {
					t.Errorf("body missing Pi %s %q:\n%s", variableName, piTerm[variableName], body)
				}
			}
		})
	}
}

func TestPlaybookPrintCodexHarness(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"delegate-pb/delegate-pb.md": delegatePlaybookContent,
	})
	s := newTestServerWithHarness(t, "codex")

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "delegate-pb", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	codexTerm := terminologyForHarness("codex", RuntimeNamespace())
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, codexTerm[varName]) {
			t.Errorf("body %q: expected codex %s %q", body, varName, codexTerm[varName])
		}
	}
}

// ---------------------------------------------------------------------------
// playbook.read — delegation tip injection
// ---------------------------------------------------------------------------

func TestPlaybookPrintDelegatesTipPresent(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"delegate-pb/delegate-pb.md": delegatePlaybookContent,
	})
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "delegate-pb", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: expected delegation tip for delegates:true playbook", body)
	}
	// Tip must include the claude ContinueIdiom.
	claudeTerm := terminologyForHarness("claude", RuntimeNamespace())
	if !strings.Contains(body, claudeTerm["ContinueIdiom"]) {
		t.Errorf("body %q: expected tip to include claude ContinueIdiom %q", body, claudeTerm["ContinueIdiom"])
	}
}

func TestPlaybookPrintDelegatesTipAbsent(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"plain-pb/plain-pb.md": plainPlaybookContent,
	})
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "plain-pb", map[string]string{"WorktreeID": "wt-123"}, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: delegation tip must not appear for delegates:false playbook", body)
	}
}
