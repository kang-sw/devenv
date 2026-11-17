package wsdashboard

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// homePath is expanded only by local request handlers, never while rendering
// shared markup (which LAN viewers receive too).
func homePath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, filepath.FromSlash(path[2:]))
		}
	}
	return path
}

// localPath checks both spellings before a local viewer reads a file or
// directory. Network/device paths and sensitive home paths stay closed even
// when reached through a symlink or a .. step.
func localPath(root, path string) (full, real string, ok bool) {
	return newDenySet().localPath(root, path)
}

func (s *denySet) localPath(root, path string) (full, real string, ok bool) {
	if len(path) > openTargetMax || strings.ContainsRune(path, 0) || networkPath(path) {
		return "", "", false
	}
	full = resolveAgainst(root, filepath.FromSlash(homePath(path)))
	if !filepath.IsAbs(full) || !driveOrRootPath(full) || networkPath(full) || deviceElement(full) || s.lexical(full) {
		return "", "", false
	}
	real, err := filepath.EvalSymlinks(full)
	// The identity of real and its ancestors also stands for full's, which
	// resolves to it.
	if err != nil || networkPath(real) || !driveOrRootPath(real) || deviceElement(real) || s.lexical(real) || s.identityAtOrAbove(real) {
		return "", "", false
	}
	return full, real, true
}

// openViewerFile preserves the confined open for LAN/token viewers. A local
// read opens the resolved regular file using the same identity check.
func openViewerFile(r *http.Request, deny *denySet, root, path string) (*os.File, string, os.FileInfo, bool) {
	if !localViewer(r) {
		return openContained(root, path)
	}
	if path == "" || crossSiteFetch(r) {
		return nil, "", nil, false
	}
	full, real, ok := deny.localPath(root, path)
	if !ok {
		return nil, "", nil, false
	}
	rel, err := filepath.Rel(filepath.Clean(root), full)
	if err != nil {
		// Windows paths on another drive cannot be relative to this root.
		rel = full
	}
	f, _, info, ok := deny.openLocal(filepath.Dir(real), filepath.Base(real))
	return f, rel, info, ok
}

func openLocalContained(root, path string) (*os.File, string, os.FileInfo, bool) {
	return newDenySet().openLocal(root, path)
}

// openLocal opens a file for a local viewer. Before the open it checks both
// spellings by string and the resolved file's ancestors by identity; the file
// itself is checked by identity on the opened handle, which needs no path
// syscall and sees the file actually read.
func (s *denySet) openLocal(root, path string) (*os.File, string, os.FileInfo, bool) {
	return openContainedVia(root, path, func(full, resolved string) bool {
		return !networkPath(resolved) && driveOrRootPath(resolved) && !deviceElement(resolved) &&
			!s.lexical(full) && !s.lexical(resolved) && !s.identityAtOrAbove(filepath.Dir(resolved))
	}, s.opened)
}

// ownRootFile prevents local symlink escapes from gaining an in-root HTML
// capability or markdown link rules. Outside browsing remains source-only.
func ownRootFile(root, path string) bool {
	f, _, _, ok := openContained(root, path)
	if ok {
		f.Close()
	}
	return ok
}

// readViewerDir reads through the checked handle, not a pathname that could
// have been redirected after viewerDir approved it.
func readViewerDir(r *http.Request, full string) ([]os.DirEntry, error) {
	return newDenySet().readViewerDir(r, full)
}

func (s *denySet) readViewerDir(r *http.Request, full string) ([]os.DirEntry, error) {
	if !localViewer(r) {
		return os.ReadDir(full)
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.IsDir() || s.opened(info) {
		return nil, os.ErrPermission
	}
	_, real, ok := s.localPath(full, ".")
	if !ok {
		return nil, os.ErrPermission
	}
	checked, err := os.Stat(real)
	if err != nil || !os.SameFile(info, checked) {
		return nil, os.ErrPermission
	}
	return f.ReadDir(-1)
}

// viewerCandidates are the name-search candidates a local viewer may see.
// They come from the root's file list (git's, or a walk that follows no
// symlinked directory), so below the root no name resolves elsewhere: each
// one is checked by string, with no syscall per candidate, under the root as
// written and as resolved, against the denied bases plus their spellings
// under a root that aliases one of their ancestors (aliasRoots). A symlink or
// hardlink among them shows by its own name only; reading it runs the full
// identity check (openLocal).
func viewerCandidates(r *http.Request, deny *denySet, root string, candidates []fileCandidate) []fileCandidate {
	if !localViewer(r) {
		return candidates
	}
	if crossSiteFetch(r) {
		return nil
	}
	roots := []string{filepath.Clean(root)}
	if real, err := filepath.EvalSymlinks(root); err == nil && real != roots[0] {
		roots = append(roots, real)
	}
	deny.aliasRoots(roots)
	var allowed []fileCandidate
	for _, c := range candidates {
		if deny.listedName(roots, c.path) {
			allowed = append(allowed, c)
		}
	}
	return allowed
}

// listedName is localPath's string checks of path under each of roots.
func (s *denySet) listedName(roots []string, path string) bool {
	if len(path) > openTargetMax || strings.ContainsRune(path, 0) || networkPath(path) {
		return false
	}
	native := filepath.FromSlash(homePath(path))
	for _, root := range roots {
		full := resolveAgainst(root, native)
		if !filepath.IsAbs(full) || !driveOrRootPath(full) || networkPath(full) || deviceElement(full) || s.lexical(full) {
			return false
		}
	}
	return true
}

func viewerDir(r *http.Request, deny *denySet, root, path string) (full, rel string, ok bool) {
	if !localViewer(r) {
		return containedDir(root, path)
	}
	if crossSiteFetch(r) {
		return "", "", false
	}
	full, real, ok := deny.localPath(root, path)
	if !ok {
		return "", "", false
	}
	if info, err := os.Stat(real); err != nil || !info.IsDir() {
		return "", "", false
	}
	rel, err := filepath.Rel(filepath.Clean(root), full)
	if err != nil {
		rel = full
	}
	if rel == "." {
		rel = ""
	}
	return full, rel, true
}
