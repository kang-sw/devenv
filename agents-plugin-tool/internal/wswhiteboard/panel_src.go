package wswhiteboard

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// panelSourceExts are the file types a wb:panel src may name: the panel
// payload types (Markdown with inline HTML) and SVG.
var panelSourceExts = map[string]bool{".html": true, ".htm": true, ".svg": true, ".md": true}

// ReadPanelSource returns the content of the file a wb:panel src="path"
// names, read once at update time. A relative path resolves against root,
// the session's project root (the root board images resolve against); an
// absolute path is accepted only inside root. A path that leaves root
// lexically or through a symlink, a missing or non-regular file, an
// extension other than .html, .htm, .svg or .md, and a file over
// MaxPanelBytes are refused with a reason naming what failed. At most
// MaxPanelBytes+1 bytes are read. The caller applies the panel payload
// rules (UTF-8, full document) to the result.
func ReadPanelSource(root, path string) (string, error) {
	switch {
	case root == "":
		return "", errors.New("the session has no project root to read from")
	case path == "" || strings.TrimSpace(path) == "":
		return "", errors.New("the path is empty")
	case strings.ContainsRune(path, 0):
		return "", errors.New("the path holds a NUL character")
	}
	root = filepath.Clean(root)
	abs := filepath.FromSlash(path)
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, abs)
	}
	abs = filepath.Clean(abs)
	if rel, err := filepath.Rel(root, abs); err != nil || !belowRoot(rel) {
		return "", errors.New("the path leaves the project root; keep the file inside the project")
	}
	if !panelSourceExts[strings.ToLower(filepath.Ext(abs))] {
		return "", errors.New("only .html, .htm, .svg and .md files can be a panel source")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("the project root cannot be resolved: %v", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errors.New("no such file")
		}
		return "", fmt.Errorf("the file cannot be resolved: %v", err)
	}
	if rel, err := filepath.Rel(realRoot, resolved); err != nil || !belowRoot(rel) {
		return "", errors.New("the path resolves outside the project root through a symlink")
	}
	if !panelSourceExts[strings.ToLower(filepath.Ext(resolved))] {
		return "", errors.New("the symlink target is not a .html, .htm, .svg or .md file")
	}
	// Stat before opening: opening a FIFO would block the update call.
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("the file cannot be read: %v", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("not a regular file")
	}
	if info.Size() > MaxPanelBytes {
		return "", errors.New(sourceTooLarge)
	}
	f, err := os.Open(resolved)
	if err != nil {
		return "", fmt.Errorf("the file cannot be read: %v", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxPanelBytes+1))
	if err != nil {
		return "", fmt.Errorf("the file cannot be read: %v", err)
	}
	if len(data) > MaxPanelBytes {
		return "", errors.New(sourceTooLarge)
	}
	return strings.TrimPrefix(string(data), "\ufeff"), nil
}

const sourceTooLarge = "the file exceeds 2 MiB; split it into several files and panels"

// belowRoot reports whether a filepath.Rel result names an entry strictly
// below its base.
func belowRoot(rel string) bool {
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
