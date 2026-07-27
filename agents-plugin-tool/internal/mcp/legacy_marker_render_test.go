package mcp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdoc"
)

// legacyMarkerAdvisoryPrefix mirrors wsdoc's note prefix, including the marker's
// 1-based line number, so a render-side change to the note is caught here too.
const legacyMarkerAdvisoryPrefix = "legacy planned marker (contract-first planned-entry mechanism being retired by " +
	"260726-refactor-retire-spec-planned-marker-mechanism): 1 marker(s) at line 10"

func legacyMarkerRenderRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/demo.md", "---\ntitle: Demo\n---\n# Demo\n\n"+
		"## Sibling Behavior {#260101-sibling}\n\nDeterministic workspace root pruning is already implemented.\n\n"+
		"> [!note] Planned 🚧 {#260101-anchor}\n> The registry will prune stale roots.\n")
	mustWrite(t, root, "ai-docs/tickets/todo/260101-feat-unrelated.md",
		"---\ntitle: Unrelated\nspec:\n  - 260101-sibling\n---\n# Unrelated\n")
	return root
}

// formatSpecFind delegates wholly to formatDocumentFind, which knows nothing of
// SpecInfo, so the query path needs its own advisory append. This exercises the
// query path end-to-end rather than the no-query fallback that formatSpecs
// serves.
func TestFormatSpecSurfacesRenderLegacyMarkerAdvisory(t *testing.T) {
	root := legacyMarkerRenderRoot(t)

	list, err := wsdoc.SpecsList(root)
	if err != nil {
		t.Fatalf("SpecsList returned error: %v", err)
	}
	listText := formatSpecs(list)
	if !strings.Contains(listText, "  legacy-marker: "+legacyMarkerAdvisoryPrefix+"; no live ticket references this spec") {
		t.Fatalf("formatSpecs text = %q", listText)
	}

	found, err := wsdoc.SpecsFind(root, wsdoc.SpecFindOptions{Query: "deterministic workspace root pruning"})
	if err != nil {
		t.Fatalf("SpecsFind returned error: %v", err)
	}
	findText := formatSpecFind("deterministic workspace root pruning", found)
	if !strings.Contains(findText, "candidate spec for query=") {
		t.Fatalf("formatSpecFind lost its delegated body: %q", findText)
	}
	// A blank line separates the advisory from the last hit line: document
	// blocks are emitted with a leading "\n", so without it the note runs flush
	// against the preceding hit.
	if !strings.Contains(findText, "\n\nlegacy-marker: ai-docs/spec/demo.md: "+legacyMarkerAdvisoryPrefix+"; no live ticket references this spec") {
		t.Fatalf("formatSpecFind text = %q", findText)
	}

	status, err := wsdoc.SpecsStatus(root, wsdoc.SpecStatusOptions{SpecStem: "260101-anchor"})
	if err != nil {
		t.Fatalf("SpecsStatus returned error: %v", err)
	}
	statusText := formatSpecStatus(status)
	if !strings.Contains(statusText, "legacy-marker: ai-docs/spec/demo.md: "+legacyMarkerAdvisoryPrefix) {
		t.Fatalf("formatSpecStatus text = %q", statusText)
	}
}

// formatSpecFind's advisory loop is bounded by the same maxFindTextDocuments cut
// the delegated body applies, so the note can never name a spec that was
// truncated out of the listing above it.
func TestFormatSpecFindDropsAdvisoriesForTruncatedDocuments(t *testing.T) {
	specs := []wsdoc.SpecInfo{}
	for i := 0; i < maxFindTextDocuments; i++ {
		specs = append(specs, wsdoc.SpecInfo{Path: fmt.Sprintf("ai-docs/spec/rank%02d.md", i), MatchScore: 100 - i})
	}
	specs = append(specs, wsdoc.SpecInfo{
		Path:                 "ai-docs/spec/truncated.md",
		MatchScore:           1,
		LegacyMarkerAdvisory: "legacy planned marker: 1 marker(s) at line 3; no live ticket references this spec",
	})

	got := formatSpecFind("prune policy", specs)
	if strings.Contains(got, "ai-docs/spec/truncated.md") {
		t.Fatalf("formatSpecFind named a truncated document: %q", got)
	}
	if strings.Contains(got, "legacy-marker") {
		t.Fatalf("formatSpecFind emitted an advisory for a truncated document: %q", got)
	}
}

func TestFormatSpecSurfacesOmitAdvisoryWhenNoMarker(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/clean.md", "---\ntitle: Clean\n---\n# Clean\n\n## Clean {#260101-clean}\n\nDeterministic workspace root pruning is implemented.\n")

	list, err := wsdoc.SpecsList(root)
	if err != nil {
		t.Fatalf("SpecsList returned error: %v", err)
	}
	if got := formatSpecs(list); strings.Contains(got, "legacy-marker") {
		t.Fatalf("formatSpecs emitted advisory without a marker: %q", got)
	}

	found, err := wsdoc.SpecsFind(root, wsdoc.SpecFindOptions{Query: "deterministic workspace root pruning"})
	if err != nil {
		t.Fatalf("SpecsFind returned error: %v", err)
	}
	if got := formatSpecFind("deterministic workspace root pruning", found); strings.Contains(got, "legacy-marker") {
		t.Fatalf("formatSpecFind emitted advisory without a marker: %q", got)
	}

	status, err := wsdoc.SpecsStatus(root, wsdoc.SpecStatusOptions{SpecStem: "260101-clean"})
	if err != nil {
		t.Fatalf("SpecsStatus returned error: %v", err)
	}
	if got := formatSpecStatus(status); strings.Contains(got, "legacy-marker") {
		t.Fatalf("formatSpecStatus emitted advisory without a marker: %q", got)
	}
}
