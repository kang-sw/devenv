// Git commit path staging and staged-status validation.
package wsgit

import (
	"fmt"
	"path/filepath"
	"strings"
)

func stagingCommandsForCommit(paths []string, status StatusResult, sparseActive bool) [][]string {
	addPaths := []string{}
	rmPaths := []string{}
	seen := map[string]bool{}
	for _, path := range paths {
		deletedPaths := deletedPathsUnderCommitRoot(status, path)
		if len(deletedPaths) > 0 && !commitRootHasAddableStatus(status, path) {
			for _, deletedPath := range deletedPaths {
				if !seen["rm:"+deletedPath] {
					rmPaths = append(rmPaths, deletedPath)
					seen["rm:"+deletedPath] = true
				}
			}
			continue
		}
		if !seen["add:"+path] {
			addPaths = append(addPaths, path)
			seen["add:"+path] = true
		}
	}
	commands := [][]string{}
	if len(addPaths) > 0 {
		addArgs := []string{"add", "-A"}
		if sparseActive {
			addArgs = append(addArgs, "--sparse")
		}
		addArgs = append(addArgs, "--")
		commands = append(commands, append(addArgs, addPaths...))
	}
	if len(rmPaths) > 0 {
		commands = append(commands, append([]string{"rm", "--cached", "--ignore-unmatch", "--"}, rmPaths...))
	}
	return commands
}

func deletedPathsUnderCommitRoot(status StatusResult, root string) []string {
	root = filepath.ToSlash(filepath.Clean(root))
	seen := map[string]bool{}
	var deleted []string
	for _, file := range status.ChangedFiles {
		if file.WorktreeStatus == "D" || file.IndexStatus == "D" {
			path := filepath.ToSlash(filepath.Clean(file.Path))
			if pathInCommitSet(path, []string{root}) && !seen[path] {
				deleted = append(deleted, path)
				seen[path] = true
			}
		}
		if file.OldPath != "" && (file.WorktreeStatus == "D" || file.IndexStatus == "D" || strings.HasPrefix(file.Status, "R")) {
			path := filepath.ToSlash(filepath.Clean(file.OldPath))
			if pathInCommitSet(path, []string{root}) && !seen[path] {
				deleted = append(deleted, path)
				seen[path] = true
			}
		}
	}
	return deleted
}

func commitRootHasAddableStatus(status StatusResult, root string) bool {
	root = filepath.ToSlash(filepath.Clean(root))
	for _, file := range status.ChangedFiles {
		path := filepath.ToSlash(filepath.Clean(file.Path))
		if !pathInCommitSet(path, []string{root}) {
			continue
		}
		if file.Status == "?" || file.Status == "!" {
			return true
		}
		if file.WorktreeStatus != "D" && file.IndexStatus != "D" {
			return true
		}
	}
	return false
}

// validateCommitPath refuses a path git.commit will not stage. The relative
// and inside-the-repository refusals name root, the directory every path
// resolves against, so a caller whose shell runs in another checkout can tell
// which checkout its session is bound to; an absolute path under root also
// gets its relative spelling. Absolute paths stay refused, never rebased.
func validateCommitPath(root, path string) error {
	if path == "" {
		return fmt.Errorf("paths may not contain empty entries")
	}
	if strings.HasPrefix(path, "-") {
		return fmt.Errorf("path %q must not start with '-'", path)
	}
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") {
		msg := fmt.Sprintf("path %q must be relative to the commit root %s, which every path resolves against", path, root)
		if rel, err := filepath.Rel(root, path); err == nil && rel != "." && rel != ".." && !strings.HasPrefix(filepath.ToSlash(rel), "../") {
			msg += fmt.Sprintf("; pass %q", filepath.ToSlash(rel))
		}
		return fmt.Errorf("%s", msg)
	}
	cleaned := filepath.Clean(path)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || strings.HasPrefix(filepath.ToSlash(cleaned), "../") {
		return fmt.Errorf("path %q must stay inside the commit root %s, which every path resolves against", path, root)
	}
	return nil
}

func validateCommitStatus(status StatusResult, paths []string) error {
	staged := 0
	for _, file := range status.ChangedFiles {
		if strings.HasPrefix(file.Status, "U") {
			return fmt.Errorf("cannot commit while unmerged path %q is present", file.Path)
		}
		if file.IndexStatus == "" || file.IndexStatus == "." {
			continue
		}
		if !pathInCommitSet(file.Path, paths) && (file.OldPath == "" || !pathInCommitSet(file.OldPath, paths)) {
			return fmt.Errorf("refusing to commit unrelated staged path %q", file.Path)
		}
		staged++
	}
	if staged == 0 {
		return fmt.Errorf("no staged changes in requested paths")
	}
	return nil
}

func expandCommitPathsForTicketMoves(status StatusResult, paths []string) []string {
	stems := map[string]bool{}
	for _, path := range paths {
		if _, stem, ok := ticketStatusStem(path); ok {
			stems[stem] = true
		}
	}
	if len(stems) == 0 {
		return paths
	}
	expanded := append([]string(nil), paths...)
	seen := map[string]bool{}
	for _, path := range paths {
		seen[filepath.ToSlash(filepath.Clean(path))] = true
	}
	for _, file := range status.ChangedFiles {
		for _, path := range []string{file.Path, file.OldPath} {
			if path == "" {
				continue
			}
			_, stem, ok := ticketStatusStem(path)
			if !ok || !stems[stem] {
				continue
			}
			cleaned := filepath.ToSlash(filepath.Clean(path))
			if !seen[cleaned] {
				expanded = append(expanded, path)
				seen[cleaned] = true
			}
		}
	}
	return expanded
}

// filterIndexDeleteSidePaths drops paths that are the delete side of a staged
// rename (IndexStatus "R", via OldPath) or an outright staged deletion
// (IndexStatus "D") from the given expanded path list. It exists so Commit
// can hand the Verifier only paths that still exist to read; see the call
// site comment referencing {#260720-wsdoc-commit-boundary} for why this
// filter applies to the verifier call only, not to validateCommitStatus.
func filterIndexDeleteSidePaths(status StatusResult, paths []string) []string {
	deleteSide := map[string]bool{}
	for _, file := range status.ChangedFiles {
		if file.IndexStatus == "D" {
			deleteSide[filepath.ToSlash(filepath.Clean(file.Path))] = true
		}
		if file.IndexStatus == "R" && file.OldPath != "" {
			deleteSide[filepath.ToSlash(filepath.Clean(file.OldPath))] = true
		}
	}
	kept := make([]string, 0, len(paths))
	for _, path := range paths {
		if deleteSide[filepath.ToSlash(filepath.Clean(path))] {
			continue
		}
		kept = append(kept, path)
	}
	return kept
}

func pathInCommitSet(path string, roots []string) bool {
	path = filepath.ToSlash(filepath.Clean(path))
	for _, root := range roots {
		root = filepath.ToSlash(filepath.Clean(root))
		if path == root || strings.HasPrefix(path, root+"/") {
			return true
		}
	}
	return false
}
