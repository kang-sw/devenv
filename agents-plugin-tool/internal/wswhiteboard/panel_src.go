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
// lexically, one with a symlink or junction anywhere below root, a missing
// or non-regular file, an extension other than .html, .htm, .svg or .md, and
// a file over MaxPanelBytes are refused with a reason naming what failed. At
// most MaxPanelBytes+1 bytes are read. The caller applies the panel payload
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
	rel, err := filepath.Rel(root, abs)
	if err != nil || !belowRoot(rel) {
		return "", fmt.Errorf("the path is outside the session root %s; keep the file inside it", root)
	}
	if !panelSourceExts[strings.ToLower(filepath.Ext(abs))] {
		return "", errors.New("only .html, .htm, .svg and .md files can be a panel source")
	}
	// Containment is checked on the lexical path, so no component below the
	// root may redirect it elsewhere: filepath.EvalSymlinks is not enough,
	// because on Windows (Go 1.23+) it does not resolve junctions (mount
	// points) while os.Stat and os.Open follow them. Each component is
	// Lstat-ed instead: a directory on the way and a regular file at the end
	// are accepted; a symlink or a junction (reported as ModeSymlink or
	// ModeIrregular) is refused wherever it sits, as is any other type.
	parts := strings.Split(rel, string(filepath.Separator))
	cur := root
	var info os.FileInfo
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		info, err = os.Lstat(cur)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return "", errors.New("no such file")
			}
			return "", fmt.Errorf("the file cannot be read: %v", err)
		}
		if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return "", fmt.Errorf("%s is a symlink or junction; a panel source and the folders above it, below the session root %s, must be plain", strings.Join(parts[:i+1], "/"), root)
		}
		if i < len(parts)-1 && !info.IsDir() {
			return "", fmt.Errorf("%s is not a folder", strings.Join(parts[:i+1], "/"))
		}
	}
	// Opening a FIFO would block the update call, so only a regular file opens.
	if !info.Mode().IsRegular() {
		return "", errors.New("not a regular file")
	}
	if info.Size() > MaxPanelBytes {
		return "", errors.New(sourceTooLarge)
	}
	f, err := os.Open(abs)
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
