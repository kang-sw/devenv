// Package wsdevassets reads the dashboard and whiteboard page assets from an
// agents-plugin-tool source tree instead of the embedded copies, so a
// template, stylesheet, or script edit shows on the next request with no
// rebuild. It is a development aid: the daemon turns it on only when asked.
package wsdevassets

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AssetDirs are the asset directories, relative to the source root, a dev
// root must hold.
var AssetDirs = []string{
	filepath.Join("internal", "wsdashboard", "assets"),
	filepath.Join("internal", "wswhiteboard", "assets"),
}

// Root is a source root whose asset files may be read. It serves nothing
// outside its own real path: a file that resolves elsewhere, through a symlink
// in the assets directory or in the path, is refused.
type Root struct {
	real string
}

// Open resolves dir, the agents-plugin-tool source root, and checks that it
// holds the asset directories.
func Open(dir string) (*Root, error) {
	if dir == "" {
		return nil, errors.New("dev assets: empty source root")
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, fmt.Errorf("dev assets: source root: %w", err)
	}
	if real, err = filepath.Abs(real); err != nil {
		return nil, fmt.Errorf("dev assets: source root: %w", err)
	}
	r := &Root{real: real}
	for _, sub := range AssetDirs {
		if _, err := r.resolve(sub); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Dir is the resolved source root.
func (r *Root) Dir() string { return r.real }

// ReadFile reads rel, a path relative to the source
// root. It refuses an absolute or ".."-escaping rel, and a file whose real
// path is outside the root.
func (r *Root) ReadFile(rel string) ([]byte, error) {
	path, err := r.resolve(rel)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (r *Root) resolve(rel string) (string, error) {
	if !filepath.IsLocal(rel) {
		return "", fmt.Errorf("dev assets: %q is not a path under the source root", rel)
	}
	real, err := filepath.EvalSymlinks(filepath.Join(r.real, rel))
	if err != nil {
		return "", fmt.Errorf("dev assets: %w", err)
	}
	if !within(r.real, real) {
		return "", fmt.Errorf("dev assets: %q resolves outside the source root", rel)
	}
	return real, nil
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}
