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
	if len(path) > openTargetMax || strings.ContainsRune(path, 0) || networkPath(path) {
		return "", "", false
	}
	full = resolveAgainst(root, filepath.FromSlash(homePath(path)))
	homes := homeDirs()
	if !filepath.IsAbs(full) || !driveOrRootPath(full) || networkPath(full) || deviceElement(full) || alwaysDenied(full, homes) {
		return "", "", false
	}
	real, err := filepath.EvalSymlinks(full)
	if err != nil || networkPath(real) || !driveOrRootPath(real) || deviceElement(real) || alwaysDenied(real, homes) {
		return "", "", false
	}
	return full, real, true
}

// openViewerFile preserves the confined open for LAN/token viewers. A local
// read opens the resolved regular file using the same identity check.
func openViewerFile(r *http.Request, root, path string) (*os.File, string, os.FileInfo, bool) {
	if !localViewer(r) {
		return openContained(root, path)
	}
	if path == "" || crossSiteFetch(r) {
		return nil, "", nil, false
	}
	full, real, ok := localPath(root, path)
	if !ok {
		return nil, "", nil, false
	}
	rel, err := filepath.Rel(filepath.Clean(root), full)
	if err != nil {
		// Windows paths on another drive cannot be relative to this root.
		rel = full
	}
	f, _, info, ok := openLocalContained(filepath.Dir(real), filepath.Base(real))
	return f, rel, info, ok
}

func openLocalContained(root, path string) (*os.File, string, os.FileInfo, bool) {
	return openContainedChecked(root, path, func(full, resolved string) bool {
		homes := homeDirs()
		return !networkPath(resolved) && driveOrRootPath(resolved) && !deviceElement(resolved) &&
			!alwaysDenied(full, homes) && !alwaysDenied(resolved, homes)
	})
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
	if !localViewer(r) {
		return os.ReadDir(full)
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.IsDir() || deniedOpenedFile(info) {
		return nil, os.ErrPermission
	}
	_, real, ok := localPath(full, ".")
	if !ok {
		return nil, os.ErrPermission
	}
	checked, err := os.Stat(real)
	if err != nil || !os.SameFile(info, checked) {
		return nil, os.ErrPermission
	}
	return f.ReadDir(-1)
}

func viewerCandidates(r *http.Request, root string, candidates []fileCandidate) []fileCandidate {
	if !localViewer(r) {
		return candidates
	}
	var allowed []fileCandidate
	if !crossSiteFetch(r) {
		for _, c := range candidates {
			if _, _, ok := localPath(root, c.path); ok {
				allowed = append(allowed, c)
			}
		}
	}
	return allowed
}

func viewerDir(r *http.Request, root, path string) (full, rel string, ok bool) {
	if !localViewer(r) {
		return containedDir(root, path)
	}
	if crossSiteFetch(r) {
		return "", "", false
	}
	full, real, ok := localPath(root, path)
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
