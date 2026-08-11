package wsdoc

import "testing"

func TestManualsListMissingDirectoryReturnsZeroManuals(t *testing.T) {
	root := t.TempDir()

	got, err := ManualsList(root)
	if err != nil {
		t.Fatalf("ManualsList returned error for missing directory: %v", err)
	}
	if got != nil {
		t.Fatalf("ManualsList = %#v, want nil for missing ai-docs/manuals/", got)
	}
}

func TestManualsListEmptyDirectoryReturnsZeroManuals(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/manuals/.gitkeep", "")

	got, err := ManualsList(root)
	if err != nil {
		t.Fatalf("ManualsList returned error for empty directory: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ManualsList = %#v, want zero manuals for a directory with no .md files", got)
	}
}

func TestManualsListReturnsManualWithSummary(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/manuals/deploy.md", "---\nsummary: How to deploy the service.\n---\n# Deploy\n")

	got, err := ManualsList(root)
	if err != nil {
		t.Fatalf("ManualsList returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ManualsList = %#v, want exactly one manual", got)
	}
	if got[0].Path != "ai-docs/manuals/deploy.md" {
		t.Fatalf("Path = %q", got[0].Path)
	}
	if got[0].Summary != "How to deploy the service." {
		t.Fatalf("Summary = %q", got[0].Summary)
	}
}

// TestManualsListReportsManualWithNoSummary verifies a manual missing the
// `summary:` frontmatter line is still listed (Summary == ""), not dropped —
// the ticket's verification requirement (b).
func TestManualsListReportsManualWithNoSummary(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/manuals/no-summary.md", "# No Summary\n\nBody text with no frontmatter at all.\n")

	got, err := ManualsList(root)
	if err != nil {
		t.Fatalf("ManualsList returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ManualsList = %#v, want the summary-less manual still reported", got)
	}
	if got[0].Path != "ai-docs/manuals/no-summary.md" {
		t.Fatalf("Path = %q", got[0].Path)
	}
	if got[0].Summary != "" {
		t.Fatalf("Summary = %q, want empty", got[0].Summary)
	}
}

func TestManualsListSortsByPathAndIgnoresNonMarkdown(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/manuals/zeta.md", "---\nsummary: Zeta manual.\n---\n# Zeta\n")
	mustWrite(t, root, "ai-docs/manuals/alpha.md", "---\nsummary: Alpha manual.\n---\n# Alpha\n")
	mustWrite(t, root, "ai-docs/manuals/notes.txt", "not a manual")

	got, err := ManualsList(root)
	if err != nil {
		t.Fatalf("ManualsList returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ManualsList = %#v, want exactly two manuals (non-.md ignored)", got)
	}
	if got[0].Path != "ai-docs/manuals/alpha.md" || got[1].Path != "ai-docs/manuals/zeta.md" {
		t.Fatalf("ManualsList not sorted by path: %#v", got)
	}
}

func TestManualsFindFiltersByQueryAcrossSummaryAndBody(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/manuals/deploy.md", "---\nsummary: How to deploy the release pipeline.\n---\n# Deploy\n\nRelease pipeline steps.\n")
	mustWrite(t, root, "ai-docs/manuals/onboarding.md", "---\nsummary: New teammate onboarding checklist.\n---\n# Onboarding\n")

	got, err := ManualsFind(root, "deploy")
	if err != nil {
		t.Fatalf("ManualsFind returned error: %v", err)
	}
	if len(got) != 1 || got[0].Path != "ai-docs/manuals/deploy.md" {
		t.Fatalf("ManualsFind(query=deploy) = %#v", got)
	}
}

func TestManualsFindWithEmptyQueryReturnsFullList(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/manuals/deploy.md", "---\nsummary: Deploy notes.\n---\n# Deploy\n")
	mustWrite(t, root, "ai-docs/manuals/onboarding.md", "---\nsummary: Onboarding notes.\n---\n# Onboarding\n")

	got, err := ManualsFind(root, "")
	if err != nil {
		t.Fatalf("ManualsFind returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ManualsFind(query=\"\") = %#v, want both manuals", got)
	}
}
