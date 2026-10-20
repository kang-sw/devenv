package wsconfig

import (
	"crypto/sha256"
	"encoding/hex"
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
// overrides map (string values) and the structured_overrides map (object
// values) contribute (see loadRepoConfig), each keyed by the item key verbatim.
// config.list publishes it so a lead can draft the file without carrying the
// format itself.
const RepoOverridesShape = `{"overrides": {"<knob>": "<value>"}, "structured_overrides": {"<structured knob>": {...}}}`

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

// updateRepoConfigFile edits the committed repo-scope file in place. It
// rewrites only the overrides and structured_overrides members and keeps every
// other member verbatim, so a write never adds agent-tier or schema_version
// noise to a tracked file and never drops a member a newer ws wrote. It does
// not stage the file: staging rides the caller's git.commit.
//
// The lock lives in the OS temp dir, keyed by the file path, not beside the
// file: a sibling lock file would sit in the working tree as an untracked file.
func updateRepoConfigFile(path string, mutate func(overrides map[string]string, structured map[string]json.RawMessage) error) error {
	sum := sha256.Sum256([]byte(filepath.Clean(path)))
	lockPath := filepath.Join(os.TempDir(), "ws-repo-config-"+hex.EncodeToString(sum[:8])+".lock")
	return rewriteLocked(path, lockPath, func(raw []byte, exists bool) ([]byte, error) {
		members := map[string]json.RawMessage{}
		if exists {
			if err := json.Unmarshal(raw, &members); err != nil {
				return nil, fmt.Errorf("parse repo ws config %s: %w", path, err)
			}
		}
		overrides := map[string]string{}
		structured := map[string]json.RawMessage{}
		if v, ok := members["overrides"]; ok {
			if err := json.Unmarshal(v, &overrides); err != nil {
				return nil, fmt.Errorf("parse repo ws config %s: overrides: %w", path, err)
			}
		}
		if v, ok := members["structured_overrides"]; ok {
			if err := json.Unmarshal(v, &structured); err != nil {
				return nil, fmt.Errorf("parse repo ws config %s: structured_overrides: %w", path, err)
			}
		}
		if err := mutate(overrides, structured); err != nil {
			return nil, err
		}
		for name, value := range map[string]any{"overrides": overrides, "structured_overrides": structured} {
			encoded, err := json.Marshal(value)
			if err != nil {
				return nil, fmt.Errorf("encode repo ws config: %w", err)
			}
			if string(encoded) == "{}" || string(encoded) == "null" {
				delete(members, name)
				continue
			}
			members[name] = encoded
		}
		payload, err := json.MarshalIndent(members, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("encode repo ws config: %w", err)
		}
		return append(payload, '\n'), nil
	})
}
