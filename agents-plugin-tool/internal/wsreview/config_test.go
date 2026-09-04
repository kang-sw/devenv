package wsreview

import (
	"os"
	"path/filepath"
	"testing"
)

func reviewLocalConfigPath(root string) string {
	return filepath.Join(root, "ai-docs", "_review.local.md")
}

func mustWriteReviewLocalConfig(t *testing.T, root, content string) {
	t.Helper()
	path := reviewLocalConfigPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create ai-docs dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestStalenessKnobDefaultsWhenFileAbsent(t *testing.T) {
	root := t.TempDir()
	if got := StalenessKnob(root); got != DefaultStalenessCommits {
		t.Fatalf("StalenessKnob = %d, want default %d", got, DefaultStalenessCommits)
	}
}

func TestStalenessKnobDefaultsWhenSectionAbsent(t *testing.T) {
	root := t.TempDir()
	mustWriteReviewLocalConfig(t, root, "# Review: example\n\n## Deep Review\nthreshold: 20 files / 500 lines\n")
	if got := StalenessKnob(root); got != DefaultStalenessCommits {
		t.Fatalf("StalenessKnob = %d, want default %d", got, DefaultStalenessCommits)
	}
}

func TestStalenessKnobDefaultsWhenValueMalformed(t *testing.T) {
	root := t.TempDir()
	mustWriteReviewLocalConfig(t, root, "## Checkpoint Nudge\nstaleness: not-a-number commits\n")
	if got := StalenessKnob(root); got != DefaultStalenessCommits {
		t.Fatalf("StalenessKnob = %d, want default %d", got, DefaultStalenessCommits)
	}
}

func TestStalenessKnobParsesValidValue(t *testing.T) {
	root := t.TempDir()
	mustWriteReviewLocalConfig(t, root, "# Review: example\n\n## Checkpoint Nudge\nstaleness: 15 commits\n\n## Deep Review\nthreshold: 20 files / 500 lines\n")
	if got := StalenessKnob(root); got != 15 {
		t.Fatalf("StalenessKnob = %d, want 15", got)
	}
}
