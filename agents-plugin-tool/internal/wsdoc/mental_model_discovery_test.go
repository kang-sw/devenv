package wsdoc

import "testing"

func TestMentalModelsFindByDomainSpecStemAndQuery(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/mental-model/workflow/index.md", "---\ndomain: workflow\ndescription: Workflow domain\nsources:\n  - ai-docs/spec/workflow.md#260504-workflow-spec\n---\n# Workflow\n\nReferences deterministic discovery.\n")
	mustWrite(t, root, "ai-docs/mental-model/other.md", "---\ndomain: other\n---\n# Other\n")

	got, err := MentalModelsFind(root, MentalModelFindOptions{Domain: "workflow", SpecStem: "260504-workflow-spec", Query: "deterministic"})
	if err != nil {
		t.Fatalf("MentalModelsFind returned error: %v", err)
	}
	if len(got) != 1 || got[0].Path != "ai-docs/mental-model/workflow/index.md" || !got[0].MatchesDomain || !got[0].MatchesSpecStem {
		t.Fatalf("find result = %#v", got)
	}
	if got[0].Domain != "workflow" || got[0].Description != "Workflow domain" {
		t.Fatalf("frontmatter = %#v", got[0])
	}
	if joined(got[0].Sources) != "ai-docs/spec/workflow.md#260504-workflow-spec" {
		t.Fatalf("sources = %#v", got[0].Sources)
	}
	if joined(got[0].SpecRefs) != "260504-workflow-spec" {
		t.Fatalf("spec refs = %#v", got[0].SpecRefs)
	}
	if joined(got[0].MatchingSnippets) != "References deterministic discovery." {
		t.Fatalf("snippets = %#v", got[0].MatchingSnippets)
	}
}

func TestMentalModelsStatusReturnsAncestorAndIndexHints(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/mental-model/workflow/index.md", "---\ndomain: workflow\n---\n# Workflow\n")
	mustWrite(t, root, "ai-docs/mental-model/workflow/runtime/state.md", "---\ndescription: Runtime state\n---\n# State\n")

	got, err := MentalModelsStatus(root, MentalModelStatusOptions{Path: "ai-docs/mental-model/workflow/runtime/state.md"})
	if err != nil {
		t.Fatalf("MentalModelsStatus returned error: %v", err)
	}
	if len(got) != 1 || got[0].Domain != "state" {
		t.Fatalf("status = %#v", got)
	}
	if joined(got[0].AncestorHints) != "ai-docs/mental-model/workflow,ai-docs/mental-model/workflow/runtime" {
		t.Fatalf("ancestor hints = %#v", got[0].AncestorHints)
	}
	if joined(got[0].IndexHints) != "ai-docs/mental-model/workflow/index.md" {
		t.Fatalf("index hints = %#v", got[0].IndexHints)
	}
}

func TestMentalModelsStatusRequiresDomainOrPath(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/mental-model/workflow.md", "---\ndomain: workflow\n---\n# Workflow\n")

	if _, err := MentalModelsStatus(root, MentalModelStatusOptions{Domain: "workflow"}); err != nil {
		t.Fatalf("MentalModelsStatus returned error: %v", err)
	}
	if _, err := MentalModelsStatus(root, MentalModelStatusOptions{}); err == nil {
		t.Fatal("MentalModelsStatus accepted empty selector")
	}
	if _, err := MentalModelsStatus(root, MentalModelStatusOptions{Path: "../workflow.md"}); err == nil {
		t.Fatal("MentalModelsStatus accepted path traversal")
	}
}
