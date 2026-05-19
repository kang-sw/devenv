package wsdoc

import "testing"

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
