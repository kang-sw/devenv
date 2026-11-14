package wsdashboard

import (
	"path/filepath"
	"slices"
	"strings"
)

// cleanWorktreeOrder retains absent paths too: discovery loss must not
// erase a user's group ordering. Only empty/duplicate entries are removed.
func cleanWorktreeOrder(paths []string) []string {
	var out []string
	for _, p := range paths {
		if p != "" {
			out = append(out, filepath.Clean(p))
		}
	}
	return uniqueIDs(out)
}

// fullWTSeq combines retained saved entries with newly visible groups at
// the tail in fallback order. A move splices this full sequence, never a
// visible-only replacement that would shift absent slots incidentally.
func fullWTSeq(doc *railLayoutDoc, f railFacts, workspace string) []string {
	var seq []string
	if doc != nil && doc.Workspaces[workspace] != nil {
		seq = slices.Clone(doc.Workspaces[workspace].WorktreeOrder)
	}
	for _, p := range f.worktrees[workspace] {
		if !slices.Contains(seq, p) {
			seq = append(seq, p)
		}
	}
	return seq
}

func railWTSeq(doc *railLayoutDoc, f railFacts, workspace string) []string {
	seq := fullWTSeq(doc, f, workspace)
	return slices.DeleteFunc(seq, func(p string) bool { return !slices.Contains(f.worktrees[workspace], p) })
}

func layoutWTMove(doc *railLayoutDoc, f railFacts, workspace, path, ref string, after bool) string {
	for _, p := range []string{workspace, path, ref} {
		if p == "" || strings.ContainsRune(p, '\x00') || filepath.Clean(p) != p {
			return layoutInvalid
		}
	}
	groups := f.worktrees[workspace]
	switch {
	case !slices.Contains(groups, path) || !slices.Contains(groups, ref):
		return layoutUnknown
	case path == ref:
		return layoutSelf
	}
	visible := railWTSeq(doc, f, workspace)
	if slices.Equal(visible, insertAt(slices.Clone(visible), path, ref, after)) {
		return ""
	}
	full := fullWTSeq(doc, f, workspace)
	s := doc.scope(workspace, f, true)
	s.w.WorktreeOrder = insertAt(full, path, ref, after)
	return ""
}
