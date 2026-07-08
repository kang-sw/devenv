package wsdoc

import (
	"os"
	"path/filepath"
)

// SpecAreaHasFrontmatterFile reports whether <root>/ai-docs/spec contains at
// least one .md file with a parsed frontmatter block. A missing directory
// returns false, not an error (fresh projects legitimately lack this
// directory before lead-forge-spec has run).
func SpecAreaHasFrontmatterFile(root string) bool {
	return dirHasFrontmatterFile(filepath.Join(root, "ai-docs", "spec"))
}

// MentalModelAreaHasFrontmatterFile reports whether
// <root>/ai-docs/mental-model contains at least one .md file with a parsed
// frontmatter block. A missing directory returns false, not an error (fresh
// projects legitimately lack this directory before lead-forge-mental-model
// has run).
func MentalModelAreaHasFrontmatterFile(root string) bool {
	return dirHasFrontmatterFile(filepath.Join(root, "ai-docs", "mental-model"))
}

// dirHasFrontmatterFile walks dir the same way scanSpecs/scanMentalModels do
// (filepath.WalkDir, skip directories and non-.md files) and returns true on
// the first file whose frontmatter() parse is non-nil. A missing or
// unreadable dir is treated as "no coverage" rather than an error.
func dirHasFrontmatterFile(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	found := false
	_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if found {
			return filepath.SkipAll
		}
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			return nil
		}
		if frontmatter(path) != nil {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}
