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
		return Config{}, fmt.Errorf("read repo ws config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse repo ws config: %w", err)
	}
	return cfg, nil
}
