package mcp

import (
	"strings"
	"testing"
)

// TestComputeManualsRendersAnchorWhenNoManualsExist verifies the block is an
// always-on authoring anchor: even with no ai-docs/manuals/ directory, it
// renders the header, the fixed authoring-guidance paragraph, and a
// "- (none yet)" placeholder rather than the empty string.
func TestComputeManualsRendersAnchorWhenNoManualsExist(t *testing.T) {
	root := t.TempDir()

	got := computeManuals(root)
	if !strings.HasPrefix(got, "# Manuals") {
		t.Fatalf("computeManuals = %q, want the always-on anchor with a leading '# Manuals' header", got)
	}
	if !strings.Contains(got, manualsAuthoringGuidance) {
		t.Fatalf("computeManuals = %q, want the authoring-guidance paragraph even when no manuals exist", got)
	}
	if !strings.Contains(got, "- (none yet)") {
		t.Fatalf("computeManuals = %q, want a '- (none yet)' placeholder when no manuals exist", got)
	}
}

// TestComputeManualsSkipsSummaryForLocalManual verifies a *.local.md manual is
// listed as a bare `- <path>` line: no summary rendered, and no no-summary nag,
// even when the local file has no summary frontmatter.
func TestComputeManualsSkipsSummaryForLocalManual(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/manuals/creds.local.md", "# Local creds\n\nhost: 10.0.0.1\n")
	mustWrite(t, root, "ai-docs/manuals/notes.local.md", "---\nsummary: Should not be rendered for a local manual.\n---\n# Local\n")

	got := computeManuals(root)
	if !strings.Contains(got, "- ai-docs/manuals/creds.local.md\n") && !strings.HasSuffix(got, "- ai-docs/manuals/creds.local.md") {
		t.Fatalf("computeManuals must list the local manual as a bare path line: %q", got)
	}
	if strings.Contains(got, "no summary") {
		t.Fatalf("computeManuals must not nag a *.local.md manual for a missing summary: %q", got)
	}
	if strings.Contains(got, "Should not be rendered for a local manual.") {
		t.Fatalf("computeManuals must not render a *.local.md manual's summary: %q", got)
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
