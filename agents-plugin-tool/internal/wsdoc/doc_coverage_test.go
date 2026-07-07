package wsdoc

import "testing"

// TestSpecAreaHasFrontmatterFile covers the missing-dir, only-non-frontmatter,
// and has-frontmatter cases for the spec doc-coverage helper.
func TestSpecAreaHasFrontmatterFile(t *testing.T) {
	t.Run("missing directory", func(t *testing.T) {
		root := t.TempDir()
		if SpecAreaHasFrontmatterFile(root) {
			t.Fatalf("missing ai-docs/spec must report false")
		}
	})

	t.Run("only non-frontmatter file", func(t *testing.T) {
		root := t.TempDir()
		mustWrite(t, root, "ai-docs/spec/demo.md", "# Demo\n\nNo frontmatter here.\n")
		if SpecAreaHasFrontmatterFile(root) {
			t.Fatalf("a spec dir with no frontmatter-bearing file must report false")
		}
	})

	t.Run("has frontmatter file", func(t *testing.T) {
		root := t.TempDir()
		mustWrite(t, root, "ai-docs/spec/plain.md", "# Plain\n\nNo frontmatter here.\n")
		mustWrite(t, root, "ai-docs/spec/demo.md", "---\ntitle: Demo\n---\n# Demo\n")
		if !SpecAreaHasFrontmatterFile(root) {
			t.Fatalf("a spec dir with a frontmatter-bearing file must report true")
		}
	})
}

// TestMentalModelAreaHasFrontmatterFile mirrors TestSpecAreaHasFrontmatterFile
// for the mental-model doc-coverage helper.
func TestMentalModelAreaHasFrontmatterFile(t *testing.T) {
	t.Run("missing directory", func(t *testing.T) {
		root := t.TempDir()
		if MentalModelAreaHasFrontmatterFile(root) {
			t.Fatalf("missing ai-docs/mental-model must report false")
		}
	})

	t.Run("only non-frontmatter file", func(t *testing.T) {
		root := t.TempDir()
		mustWrite(t, root, "ai-docs/mental-model/demo.md", "# Demo\n\nNo frontmatter here.\n")
		if MentalModelAreaHasFrontmatterFile(root) {
			t.Fatalf("a mental-model dir with no frontmatter-bearing file must report false")
		}
	})

	t.Run("has frontmatter file", func(t *testing.T) {
		root := t.TempDir()
		mustWrite(t, root, "ai-docs/mental-model/plain.md", "# Plain\n\nNo frontmatter here.\n")
		mustWrite(t, root, "ai-docs/mental-model/demo.md", "---\ndomain: demo\n---\n# Demo\n")
		if !MentalModelAreaHasFrontmatterFile(root) {
			t.Fatalf("a mental-model dir with a frontmatter-bearing file must report true")
		}
	})
}
