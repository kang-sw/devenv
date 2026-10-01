package wsconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// repoConfigDirName is the committed, version-tracked project config directory
// at the repo root. Its config.json is checked into the downstream repository so
// a project-wide decision is shared across every contributor and read
// deterministically by the tools — the gap the machine-local project scope
// (~/.ws@<id>/config.json) cannot fill.
const repoConfigDirName = ".ws-workflow"

// RepoOverridesShape is the committed repo-scope file's accepted shape: only the
// overrides map contributes (see loadRepoConfig), keyed by the item key verbatim.
// config.list publishes it so a lead can draft the file without carrying the
// format itself.
const RepoOverridesShape = `{"overrides": {"<knob>": "<value>"}}`

// RepoScopeInfo describes the committed repo scope a resolver reads: the file
// path anchored at the caller's worktree root (empty when no root anchors it),
// whether that file exists, and the accepted shape.
type RepoScopeInfo struct {
	Path   string `json:"path,omitempty"`
	Exists bool   `json:"exists"`
	Shape  string `json:"shape"`
}

// RepoScope reports the repo-scope file opts anchors. With no RepoRoot the
// path is empty and the repo scope applies to nothing.
func RepoScope(opts Options) RepoScopeInfo {
	info := RepoScopeInfo{Shape: RepoOverridesShape}
	path, ok := RepoPath(opts)
	if !ok {
		return info
	}
	info.Path = path
	if _, err := os.Stat(path); err == nil {
		info.Exists = true
	}
	return info
}

// RepoPath resolves the committed repo-scope config file path:
// <RepoRoot>/.ws-workflow/config.json. It returns ok=false when opts.RepoRoot is
// empty — the repo scope is simply absent (no root to anchor it), which is never
// an error. The join is intentionally pure (no Git invocation): opts.RepoRoot is
// the caller's already-canonical worktree root, and the committed file is checked
// out at that worktree's toplevel, mirroring the repo-layer note store's use of
// the same root.
func RepoPath(opts Options) (string, bool) {
	root := strings.TrimSpace(opts.RepoRoot)
	if root == "" {
		return "", false
	}
	return filepath.Join(root, repoConfigDirName, "config.json"), true
}

// loadRepoConfig reads the committed repo-scope config file. An empty RepoRoot or
// a missing file returns an empty Config (equivalent to "no repo overrides"),
// mirroring loadGlobalConfig's "no file yet" contract — absence is never an
// error. Unlike Load (the project scope), no tier/default normalization is
// applied: the repo scope only contributes to the Overrides overlay.
func loadRepoConfig(opts Options) (Config, error) {
	path, ok := RepoPath(opts)
	if !ok {
		return Config{}, nil
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Config{}, nil // absent committed file — empty repo layer, not an error
	}
	if err != nil {
		return Config{}, fmt.Errorf("read repo ws config %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse repo ws config %s: %w", path, err)
	}
	return cfg, nil
}
