package wsgit

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// A tree entry's absence is its zero value, including for staged deletions.
// Object identity alone is insufficient: executable bits and object kinds are
// part of the content the caller authorizes.
type commitTreeEntry struct {
	mode string
	oid  string
}

type commitMergePins struct {
	head      string
	incoming  string
	mergeFile string
	mergeRaw  []byte
	indexRaw  []byte
}

// admitMergeStaging runs only when ordinary path membership would refuse the
// commit. It never stages anything outside the grant. Equality proves allowed
// tree bytes, not which actor staged them; a filename or MERGE_HEAD alone does
// not establish permission.
func admitMergeStaging(ctx context.Context, runner Runner, root string, grant []string) ([]string, *commitMergePins, error) {
	pins, err := readCommitMergePins(ctx, runner, root)
	if err != nil {
		return nil, nil, err
	}
	baseOut, err := runner.RunGit(ctx, root, "merge-base", "--all", pins.head, pins.incoming)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot establish a merge base: %w", err)
	}
	bases := strings.Fields(string(baseOut))
	if len(bases) != 1 || !commitObjectID(bases[0]) {
		return nil, nil, fmt.Errorf("automatic merge admission requires one unambiguous merge base")
	}
	base, err := readCommitTree(ctx, runner, root, bases[0])
	if err != nil {
		return nil, nil, err
	}
	head, err := readCommitTree(ctx, runner, root, pins.head)
	if err != nil {
		return nil, nil, err
	}
	incoming, err := readCommitTree(ctx, runner, root, pins.incoming)
	if err != nil {
		return nil, nil, err
	}
	pins.indexRaw, err = runner.RunGit(ctx, root, "ls-files", "--stage", "-z")
	if err != nil {
		return nil, nil, err
	}
	index, err := parseCommitEntries(pins.indexRaw, true)
	if err != nil {
		return nil, nil, err
	}
	changedOut, err := runner.RunGit(ctx, root, "diff", "--cached", "--no-renames", "--name-only", "-z", pins.head, "--")
	if err != nil {
		return nil, nil, err
	}
	var admitted []string
	for _, path := range commitNULPaths(changedOut) {
		if pathInCommitSet(path, grant) {
			continue
		}
		if head[path] != base[path] || index[path] != incoming[path] {
			return nil, nil, fmt.Errorf("staged path %q does not equal unchanged-HEAD incoming content", path)
		}
		if commitFileDirectoryRelationship(path, base, head, incoming, index) {
			return nil, nil, fmt.Errorf("unsupported file/directory relationship at %q", path)
		}
		admitted = append(admitted, path)
	}
	if len(admitted) == 0 {
		return nil, nil, fmt.Errorf("no incoming staged paths can be admitted")
	}
	// An incoming rename is supported only when both endpoints satisfy the
	// same equality proof. A rename on our side, or a composite rename, needs
	// the caller's explicit grant rather than inferring filename provenance.
	for _, side := range []struct {
		tree string
		ours bool
	}{{pins.head, true}, {pins.incoming, false}} {
		out, err := runner.RunGit(ctx, root, "diff-tree", "-r", "--no-commit-id", "--no-ext-diff", "--no-textconv", "--name-status", "-z", "--find-renames", bases[0], side.tree, "--")
		if err != nil {
			return nil, nil, err
		}
		if err := validateCommitMergeRenames(out, admitted, side.ours, base, head, incoming, index); err != nil {
			return nil, nil, err
		}
	}
	// The verifier reads working files, not blobs. Do not let an unstaged
	// replacement hide the staged incoming ticket content from that gate.
	dirty, err := runner.RunGit(ctx, root, "diff-files", "--name-only", "-z", "--ignore-submodules=none", "--")
	if err != nil {
		return nil, nil, err
	}
	for _, path := range commitNULPaths(dirty) {
		if containsCommitPath(admitted, path) {
			return nil, nil, fmt.Errorf("incoming staged path %q has unstaged changes; automatic verification requires matching working content", path)
		}
	}
	return admitted, pins, nil
}

func readCommitMergePins(ctx context.Context, runner Runner, root string) (*commitMergePins, error) {
	out, err := runner.RunGit(ctx, root, "rev-parse", "--git-path", "MERGE_HEAD")
	if err != nil {
		return nil, err
	}
	mergeFile := strings.TrimSpace(string(out))
	if mergeFile == "" {
		return nil, fmt.Errorf("no readable merge state")
	}
	if !filepath.IsAbs(mergeFile) {
		mergeFile = filepath.Join(root, mergeFile)
	}
	raw, err := os.ReadFile(mergeFile)
	if err != nil {
		return nil, fmt.Errorf("cannot read MERGE_HEAD: %w", err)
	}
	heads := strings.Fields(string(raw))
	if len(heads) != 1 || !commitObjectID(heads[0]) {
		return nil, fmt.Errorf("automatic merge admission requires a single valid MERGE_HEAD")
	}
	incomingOut, err := runner.RunGit(ctx, root, "rev-parse", "--verify", heads[0]+"^{commit}")
	if err != nil || strings.TrimSpace(string(incomingOut)) != heads[0] {
		return nil, fmt.Errorf("MERGE_HEAD does not name a valid commit")
	}
	headOut, err := runner.RunGit(ctx, root, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return nil, err
	}
	head := strings.TrimSpace(string(headOut))
	if !commitObjectID(head) {
		return nil, fmt.Errorf("cannot pin HEAD for automatic merge admission")
	}
	return &commitMergePins{head: head, incoming: heads[0], mergeFile: mergeFile, mergeRaw: raw}, nil
}

func (pins *commitMergePins) recheck(ctx context.Context, runner Runner, root, branch string) error {
	if currentBranch(ctx, runner, root) != branch {
		return fmt.Errorf("git.commit refused: branch changed during merge admission; verify whether you should be committing here")
	}
	raw, err := os.ReadFile(pins.mergeFile)
	if err != nil || !bytes.Equal(raw, pins.mergeRaw) {
		return fmt.Errorf("git.commit refused: MERGE_HEAD changed during merge admission; retry with a fresh snapshot")
	}
	head, err := runner.RunGit(ctx, root, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || strings.TrimSpace(string(head)) != pins.head {
		return fmt.Errorf("git.commit refused: HEAD changed during merge admission; retry with a fresh snapshot")
	}
	index, err := runner.RunGit(ctx, root, "ls-files", "--stage", "-z")
	if err != nil || !bytes.Equal(index, pins.indexRaw) {
		return fmt.Errorf("git.commit refused: staged index changed during merge admission; retry with a fresh snapshot")
	}
	return nil
}

func readCommitTree(ctx context.Context, runner Runner, root, tree string) (map[string]commitTreeEntry, error) {
	out, err := runner.RunGit(ctx, root, "ls-tree", "-r", "-z", "--full-tree", tree)
	if err != nil {
		return nil, err
	}
	return parseCommitEntries(out, false)
}

func parseCommitEntries(raw []byte, index bool) (map[string]commitTreeEntry, error) {
	entries := map[string]commitTreeEntry{}
	for _, record := range commitNULPaths(raw) {
		meta, path, ok := strings.Cut(record, "\t")
		fields := strings.Fields(meta)
		if !ok || path == "" || len(fields) != 3 {
			return nil, fmt.Errorf("cannot parse Git tree/index entry")
		}
		oid := fields[2]
		if index {
			if fields[2] != "0" {
				return nil, fmt.Errorf("cannot commit while unmerged path %q is present", path)
			}
			oid = fields[1]
		}
		if !commitObjectID(oid) {
			return nil, fmt.Errorf("invalid Git object at %q", path)
		}
		entries[path] = commitTreeEntry{mode: fields[0], oid: oid}
	}
	return entries, nil
}

func commitObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func commitNULPaths(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
}

func containsCommitPath(paths []string, path string) bool {
	for _, candidate := range paths {
		if candidate == path {
			return true
		}
	}
	return false
}

func commitFileDirectoryRelationship(path string, trees ...map[string]commitTreeEntry) bool {
	for _, tree := range trees {
		for other := range tree {
			if strings.HasPrefix(other, path+"/") || strings.HasPrefix(path, other+"/") {
				return true
			}
		}
	}
	return false
}

func validateCommitMergeRenames(raw []byte, admitted []string, ours bool, base, head, incoming, index map[string]commitTreeEntry) error {
	parts := commitNULPaths(raw)
	for i := 0; i < len(parts); {
		status := parts[i]
		i++
		if status == "" {
			continue
		}
		if i >= len(parts) {
			return fmt.Errorf("cannot parse merge rename relationships")
		}
		oldPath := parts[i]
		i++
		if !strings.HasPrefix(status, "R") {
			continue
		}
		if i >= len(parts) {
			return fmt.Errorf("cannot parse merge rename destination")
		}
		newPath := parts[i]
		i++
		if !containsCommitPath(admitted, oldPath) && !containsCommitPath(admitted, newPath) {
			continue
		}
		if ours || head[oldPath] != base[oldPath] || head[newPath] != base[newPath] || index[oldPath] != incoming[oldPath] || index[newPath] != incoming[newPath] {
			return fmt.Errorf("unsupported rename relationship %q -> %q", oldPath, newPath)
		}
	}
	return nil
}
