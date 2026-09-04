package wsdoc

import (
	"strings"
	"testing"
)

func TestSpecsListHandlesNestedSpecsAndDuplicateFilenames(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/demo.md", "---\ntitle: Root Demo\nsummary: Root summary\nfeatures:\n  - 🚧 Root feature [260504-ticket-demo/p1]\n---\n# Root Demo\n\n## Root Feature {#260504-root-feature}\n")
	mustWrite(t, root, "ai-docs/spec/nested/demo.md", "---\ntitle: Nested Demo\nfeatures:\n  - done [260504-other-ticket/p1]\n---\n# Nested Demo\n\n## Nested Feature {#260504-nested-feature}\n")

	got, err := SpecsList(root)
	if err != nil {
		t.Fatalf("SpecsList returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("SpecsList length = %d: %#v", len(got), got)
	}
	if got[0].Path != "ai-docs/spec/demo.md" || got[1].Path != "ai-docs/spec/nested/demo.md" {
		t.Fatalf("spec paths = %#v", got)
	}
	rootSpec := got[0]
	if rootSpec.Title != "Root Demo" || rootSpec.Summary != "Root summary" {
		t.Fatalf("frontmatter = %#v", rootSpec)
	}
	if len(rootSpec.Anchors) != 1 || rootSpec.Anchors[0].SpecStem != "260504-root-feature" || rootSpec.Anchors[0].Heading != "Root Feature" {
		t.Fatalf("anchors = %#v", rootSpec.Anchors)
	}
	if joined(rootSpec.TicketRefs) != "260504-ticket-demo" {
		t.Fatalf("ticket refs = %#v", rootSpec.TicketRefs)
	}
	if joined(rootSpec.MarkerContexts) != "- 🚧 Root feature [260504-ticket-demo/p1]" {
		t.Fatalf("marker contexts = %#v", rootSpec.MarkerContexts)
	}
}

// markerContext's looseness is a retained surface: its `planned` and `wip`
// substring branches feed specs.query match scoring, and the emoji branch alone
// is only a third of it. TestSpecsListHandlesNestedSpecsAndDuplicateFilenames
// pins the emoji branch through MarkerContexts; this pins the other two, so a
// later narrowing of the predicate cannot pass CI unnoticed.
func TestMarkerContextRetainsLooseSubstringBranches(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{"emoji", "  - 🚧 Root feature [260504-ticket-demo/p1]", "- 🚧 Root feature [260504-ticket-demo/p1]"},
		{"planned lowercase", "  - planned work [260504-ticket-demo/p2]", "- planned work [260504-ticket-demo/p2]"},
		{"planned mixed case", "Planned behavior arrives later.", "Planned behavior arrives later."},
		{"wip lowercase", "  - wip work [260504-ticket-demo/p3]", "- wip work [260504-ticket-demo/p3]"},
		{"wip mixed case", "WIP: registry pruning", "WIP: registry pruning"},
		{"no marker token", "  - done [260504-other-ticket/p1]", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := markerContext(tc.line); got != tc.want {
				t.Fatalf("markerContext(%q) = %q, want %q", tc.line, got, tc.want)
			}
		})
	}

	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/loose.md", "---\ntitle: Loose\nfeatures:\n  - planned work [260504-ticket-demo/p2]\n  - wip work [260504-ticket-demo/p3]\n---\n# Loose\n\n## Loose {#260504-loose}\n")
	got, err := SpecsList(root)
	if err != nil {
		t.Fatalf("SpecsList returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("SpecsList = %#v", got)
	}
	if joined(got[0].MarkerContexts) != "- planned work [260504-ticket-demo/p2],- wip work [260504-ticket-demo/p3]" {
		t.Fatalf("marker contexts = %#v", got[0].MarkerContexts)
	}
	// The looseness must stay out of the legacy-marker advisory: neither line is
	// a marker shape at line start.
	if got[0].LegacyMarkerAdvisory != "" {
		t.Fatalf("loose marker context leaked into the advisory: %q", got[0].LegacyMarkerAdvisory)
	}
}

func TestSpecsFindBySpecStemTicketStemAndQuery(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/demo.md", "---\ntitle: Demo\nfeatures:\n  - planned work [260504-ticket-demo/p2]\n---\n# Demo\n\n## Feature {#260504-spec-demo}\n\nDeterministic spec discovery.\n")
	mustWrite(t, root, "ai-docs/spec/other.md", "---\ntitle: Other\n---\n# Other\n\n## Other {#260504-spec-other}\n")

	got, err := SpecsFind(root, SpecFindOptions{SpecStem: "260504-spec-demo", TicketStem: "260504-ticket-demo", Query: "deterministic"})
	if err != nil {
		t.Fatalf("SpecsFind returned error: %v", err)
	}
	if len(got) != 1 || got[0].Path != "ai-docs/spec/demo.md" || !got[0].MatchesSpecStem || !got[0].MatchesTicketRef {
		t.Fatalf("find result = %#v", got)
	}
	if joined(got[0].MatchingSnippets) != "Deterministic spec discovery." {
		t.Fatalf("snippets = %#v", got[0].MatchingSnippets)
	}
}

func TestSpecsStatusReturnsDuplicateAnchorLocations(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/a.md", "---\ntitle: A\n---\n# A\n\n## One {#260504-dup}\n")
	mustWrite(t, root, "ai-docs/spec/nested/a.md", "---\ntitle: Nested A\n---\n# Nested A\n\n## Two {#260504-dup}\n")

	got, err := SpecsStatus(root, SpecStatusOptions{SpecStem: "260504-dup"})
	if err != nil {
		t.Fatalf("SpecsStatus returned error: %v", err)
	}
	if got.SpecStem != "260504-dup" || len(got.Files) != 2 || len(got.Locations) != 2 {
		t.Fatalf("status = %#v", got)
	}
	paths := []string{got.Files[0].Path, got.Files[1].Path}
	if joined(paths) != "ai-docs/spec/a.md,ai-docs/spec/nested/a.md" {
		t.Fatalf("status paths = %#v", paths)
	}
}

func TestSpecsStatusRequiresSpecStem(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/demo.md", "# Demo\n\n## Feature {#260504-spec-demo}\n")

	if _, err := SpecsStatus(root, SpecStatusOptions{SpecStem: "260504-spec-demo"}); err != nil {
		t.Fatalf("SpecsStatus returned error: %v", err)
	}
	if _, err := SpecsStatus(root, SpecStatusOptions{SpecStem: "not-a-ticket-stem"}); err == nil {
		t.Fatal("SpecsStatus accepted invalid spec_stem")
	}
}

func TestSpecsFindToleratesBroadHumanQueryWithEvidence(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/plugin-runtime.md", "---\ntitle: Plugin Runtime\nsummary: Installer and marketplace packaging\n---\n# Plugin Runtime\n\n## Release {#260519-plugin-runtime}\n\nThe wsflow installer builds marketplace release packaging for plugin distribution.\nPackaging release notes mention marketplace installers.\n")
	mustWrite(t, root, "ai-docs/spec/claude-compatibility.md", "---\ntitle: Claude Compatibility\nsummary: Marketplace packaging compatibility\n---\n# Compatibility\n\n## Compat {#260519-compat}\n\nMarketplace packaging is documented for compatibility.\n")
	mustWrite(t, root, "ai-docs/spec/noise.md", "---\ntitle: Noise\n---\n# Noise\n\nInstaller only.\n")

	got, err := SpecsFind(root, SpecFindOptions{Query: "wsflow installer marketplace release packaging"})
	if err != nil {
		t.Fatalf("SpecsFind returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("SpecsFind length = %d: %#v", len(got), got)
	}
	if got[0].Path != "ai-docs/spec/plugin-runtime.md" || got[0].MatchScore <= got[1].MatchScore {
		t.Fatalf("ordering/scores = %#v", got)
	}
	if got[0].MatchScore == 0 || len(got[0].Matches) == 0 || got[0].Matches[0].Line == 0 || len(got[0].Matches[0].MatchedTerms) == 0 || got[0].Matches[0].Snippet == "" {
		t.Fatalf("match evidence = %#v", got[0])
	}
}

func TestCompactSnippetUsesRuneOffsets(t *testing.T) {
	line := strings.Repeat("한글", 80) + " marketplace release"
	got := compactSnippet(line, []string{"marketplace"})
	if !strings.Contains(got, "marketplace") {
		t.Fatalf("snippet = %q", got)
	}
}

func TestSpecsFindMetadataOnlyMatchHasNoSyntheticLineEvidence(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/marketplace-installer.md", "---\ntitle: Other\n---\n# Other\n\nNo matching body terms.\n")

	got, err := SpecsFind(root, SpecFindOptions{Query: "marketplace installer"})
	if err != nil {
		t.Fatalf("SpecsFind returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("SpecsFind length = %d: %#v", len(got), got)
	}
	for _, match := range got[0].Matches {
		if match.Line == 0 {
			t.Fatalf("synthetic line evidence = %#v", got[0].Matches)
		}
	}
}
