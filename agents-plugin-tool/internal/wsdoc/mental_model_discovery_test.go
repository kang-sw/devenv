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

func TestMentalModelsFindToleratesBroadHumanQueryWithEvidence(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/mental-model/runtime/index.md", "---\ndomain: runtime\ndescription: Runtime CLI mirror readable defaults\nsources:\n  - ai-docs/spec/mcp.md#260519-runtime-readable\n---\n# Runtime\n\nThe MCP runtime provides readable CLI mirror output with structured evidence.\nReadable runtime output includes CLI mirror diagnostics.\n")
	mustWrite(t, root, "ai-docs/mental-model/runtime/agent.md", "---\ndomain: agent\ndescription: Runtime readable notes\n---\n# Agent\n\nRuntime readable notes are separate.\n")
	mustWrite(t, root, "ai-docs/mental-model/noise.md", "---\ndomain: noise\n---\n# Noise\n\nRuntime only.\n")

	got, err := MentalModelsFind(root, MentalModelFindOptions{Query: "runtime readable CLI mirror"})
	if err != nil {
		t.Fatalf("MentalModelsFind returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("MentalModelsFind length = %d: %#v", len(got), got)
	}
	if got[0].Path != "ai-docs/mental-model/runtime/index.md" || got[0].MatchScore <= got[1].MatchScore {
		t.Fatalf("ordering/scores = %#v", got)
	}
	if len(got[0].Matches) == 0 || got[0].Matches[0].Line == 0 || joined(got[0].Matches[0].MatchedTerms) == "" || got[0].Matches[0].Snippet == "" {
		t.Fatalf("match evidence = %#v", got[0])
	}

	narrowed, err := MentalModelsFind(root, MentalModelFindOptions{Domain: "agent", Query: "runtime readable CLI mirror"})
	if err != nil {
		t.Fatalf("MentalModelsFind narrowed returned error: %v", err)
	}
	if len(narrowed) != 1 || narrowed[0].Domain != "agent" || !narrowed[0].MatchesDomain {
		t.Fatalf("domain filter changed = %#v", narrowed)
	}
}
