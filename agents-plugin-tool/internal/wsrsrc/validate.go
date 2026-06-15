package wsrsrc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Validate performs a full structural and integrity check of the rsrc tree at root.
//
// Checks performed:
//  1. manifest.json exists and is parseable with a compatible schema version.
//  2. All manifest-listed files exist on disk with matching hashes.
//  3. All files in the tree are listed in the manifest (no unlisted files).
//  4. Each subdirectory is treated as a playbook: its required base file
//     (<name>/<name>.md) must exist.
//  5. For each playbook file (base and overlays):
//     a. All declared includes resolve to a playbook-local or root-level file
//     that exists in both the manifest and on disk.
//     b. No undeclared variables ({{.Name}} patterns not in the variables list).
//
// Validate is used both by the CI tree check (TestValidateRealTree) and by
// callers that want pre-flight validation before loading.
func Validate(root string) error {
	// 1. Read manifest — checks existence and schema version.
	manifest, err := ReadManifest(root)
	if err != nil {
		return err
	}

	// 2. All manifest-listed files must exist with matching hashes.
	for relPath, expectedHash := range manifest.Files {
		filePath := filepath.Join(root, filepath.FromSlash(relPath))
		data, err := os.ReadFile(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				return ErrFileMissing{RelPath: relPath}
			}
			return fmt.Errorf("read %q: %w", relPath, err)
		}
		if got := hashHex(data); got != expectedHash {
			return ErrHashMismatch{RelPath: relPath, Got: got, Want: expectedHash}
		}
	}

	// 3. All files on disk must appear in the manifest.
	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "manifest.json" {
			return nil // manifest itself is excluded by convention
		}
		if _, ok := manifest.Files[rel]; !ok {
			return fmt.Errorf("file %q exists in rsrc tree but is not listed in manifest", rel)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// 4–5. Validate each playbook directory.
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read rsrc root %s: %w", root, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isBareStem(name) {
			continue
		}
		if err := validatePlaybookDir(root, name, manifest); err != nil {
			return err
		}
	}
	return nil
}

// validatePlaybookDir validates a single playbook directory.
func validatePlaybookDir(root, name string, manifest Manifest) error {
	dir := filepath.Join(root, name)

	// 4. Required base variant must exist.
	baseRelPath := name + "/" + name + ".md"
	basePath := filepath.Join(dir, name+".md")
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		return fmt.Errorf("playbook %q missing required base file %q", name, baseRelPath)
	}

	// 5. Validate all .md files in the directory.
	subEntries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read playbook dir %s: %w", dir, err)
	}
	for _, sub := range subEntries {
		if sub.IsDir() || !strings.HasSuffix(sub.Name(), ".md") {
			continue
		}
		if !isPlaybookVariantFilename(name, sub.Name()) {
			continue
		}
		filePath := filepath.Join(dir, sub.Name())
		if err := validatePlaybookFile(root, name, filePath, manifest); err != nil {
			return err
		}
	}
	return nil
}

// validatePlaybookFile validates a single playbook .md file for include
// references and undeclared variable usage.
func validatePlaybookFile(root, playbookName, filePath string, manifest Manifest) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read %q: %w", filePath, err)
	}
	fm, body := parseFrontmatter(string(data))
	meta := metaFromFrontmatter(fm)

	// 5a. All declared includes must resolve to a manifest-listed file on disk.
	harness := harnessFromPlaybookFilename(playbookName, filepath.Base(filePath))
	for _, inc := range meta.Includes {
		if !isBareStem(inc) {
			return fmt.Errorf("playbook %q: include name %q must be a bare stem", playbookName, inc)
		}
		incPath, incRelPath := resolveIncludePath(root, playbookName, inc, harness)
		if _, ok := manifest.Files[incRelPath]; !ok {
			return fmt.Errorf("playbook %q: include %q (%q) is not listed in manifest", playbookName, inc, incRelPath)
		}
		if _, err := os.Stat(incPath); os.IsNotExist(err) {
			return fmt.Errorf("playbook %q: dangling include %q: file not found at %s", playbookName, inc, incPath)
		}
	}

	// 5b. No undeclared variables in body.
	declaredSet := make(map[string]bool, len(meta.Variables))
	for _, v := range meta.Variables {
		declaredSet[v] = true
	}
	if err := scanUndeclaredVars(playbookName, body, declaredSet); err != nil {
		return err
	}
	return nil
}

func isPlaybookVariantFilename(playbookName, filename string) bool {
	if filename == playbookName+".md" {
		return true
	}
	prefix := playbookName + "."
	if !strings.HasPrefix(filename, prefix) || !strings.HasSuffix(filename, ".md") {
		return false
	}
	harness := strings.TrimSuffix(strings.TrimPrefix(filename, prefix), ".md")
	return isBareStem(harness)
}

func harnessFromPlaybookFilename(playbookName, filename string) string {
	if filename == playbookName+".md" {
		return ""
	}
	prefix := playbookName + "."
	if !strings.HasPrefix(filename, prefix) || !strings.HasSuffix(filename, ".md") {
		return ""
	}
	harness := strings.TrimSuffix(strings.TrimPrefix(filename, prefix), ".md")
	if !isBareStem(harness) {
		return ""
	}
	return harness
}

// scanUndeclaredVars scans text for {{.Name}} patterns and errors on any Name
// not present in declared.
func scanUndeclaredVars(playbookName, text string, declared map[string]bool) error {
	remaining := text
	for {
		idx := strings.Index(remaining, "{{.")
		if idx < 0 {
			break
		}
		end := strings.Index(remaining[idx:], "}}")
		if end < 0 {
			break
		}
		end += idx
		varName := remaining[idx+3 : end]
		if !declared[varName] {
			return fmt.Errorf("playbook %q uses undeclared variable %q", playbookName, varName)
		}
		remaining = remaining[end+2:]
	}
	return nil
}
