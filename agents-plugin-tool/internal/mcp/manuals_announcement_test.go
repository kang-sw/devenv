package mcp

import (
	"strings"
	"testing"
)

func TestComputeManualsReturnsEmptyWhenNoManualsExist(t *testing.T) {
	root := t.TempDir()

	if got := computeManuals(root); got != "" {
		t.Fatalf("computeManuals = %q, want empty string when ai-docs/manuals/ does not exist", got)
	}
}

func TestComputeManualsRendersPathAndSummary(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/manuals/deploy.md", "---\nsummary: How to deploy the service.\n---\n# Deploy\n")

	got := computeManuals(root)
	if !strings.HasPrefix(got, "# Manuals") {
		t.Fatalf("computeManuals = %q, want a leading '# Manuals' header", got)
	}
	if !strings.Contains(got, "ai-docs/manuals/deploy.md") || !strings.Contains(got, "How to deploy the service.") {
		t.Fatalf("computeManuals missing path/summary: %q", got)
	}
}

// TestComputeManualsReportsMissingSummary verifies a manual with no
// `summary:` frontmatter is still surfaced in the ambient block with an
// explicit no-summary marker, not silently dropped — the ticket's
// verification requirement (b).
func TestComputeManualsReportsMissingSummary(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/manuals/no-summary.md", "# No Summary\n\nBody with no frontmatter.\n")

	got := computeManuals(root)
	if !strings.Contains(got, "ai-docs/manuals/no-summary.md") {
		t.Fatalf("computeManuals dropped the summary-less manual: %q", got)
	}
	if !strings.Contains(got, "no summary") {
		t.Fatalf("computeManuals did not report the missing summary explicitly: %q", got)
	}
}
