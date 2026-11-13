package wsagent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Claude enables these plugins without an enabledPlugins entry. Keep this
// separate from the child profile's builtin policy: only isolated leads
// disable all builtins, including ones the CLI may not allow turning off.
var claudeLeadBuiltinPlugins = []string{
	"cc-plugin-agents-md@builtin",
	"cc-plugin-plugin-authoring@builtin",
	"cc-plugin-privacy@builtin",
	"cc-plugin-telemetry@builtin",
}

// claudeLeadPluginArgs overrides only plugin enablement. Leaving setting
// sources and MCP configuration alone preserves permissions, hooks, memory,
// non-plugin MCP servers, and (in default mode) unrelated plugins.
func claudeLeadPluginArgs(spec StartSpec) ([]string, error) {
	isolation := spec.Lead.PluginIsolation
	if isolation != "" && isolation != "default" && isolation != "isolated" {
		return nil, fmt.Errorf("claude lead: invalid plugin isolation %q", isolation)
	}
	root, err := bundledPluginRoot()
	if err != nil {
		return nil, fmt.Errorf("claude lead: %w", err)
	}
	enabled, err := claudeLeadEnabledPlugins(spec)
	if err != nil {
		return nil, fmt.Errorf("claude lead: %w", err)
	}
	disabled := map[string]bool{}
	for id, on := range enabled {
		name, _, _ := strings.Cut(id, "@")
		if on && (isolation == "isolated" || name == "ws" || name == "wsflow") {
			disabled[id] = false
		}
	}
	settings, err := json.Marshal(map[string]any{"enabledPlugins": disabled})
	if err != nil {
		return nil, fmt.Errorf("claude lead plugin settings: %w", err)
	}
	return []string{"--settings", string(settings), "--plugin-dir", root}, nil
}

// claudeLeadEnabledPlugins merges plugin entries in Claude's user, project,
// local precedence order, including explicit false overrides. Read errors
// other than an absent settings file refuse launch: ignoring one could load
// the installed ws copy alongside the bundled plugin.
func claudeLeadEnabledPlugins(spec StartSpec) (map[string]bool, error) {
	env := map[string]string{}
	for _, kv := range claudeLeadEnv(spec) {
		if name, value, ok := strings.Cut(kv, "="); ok {
			env[name] = value
		}
	}
	homeKey := "HOME"
	if runtime.GOOS == "windows" {
		homeKey = "USERPROFILE"
	}
	home := env[homeKey]
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve Claude settings directory: %w", err)
		}
	}
	configDir := env["CLAUDE_CONFIG_DIR"]
	if configDir == "" {
		configDir = filepath.Join(home, ".claude")
	}
	// Relative paths are resolved from the child's cwd, not the daemon's.
	if !filepath.IsAbs(configDir) {
		configDir = filepath.Join(spec.workDir(), configDir)
	}
	paths := []string{
		filepath.Join(configDir, "settings.json"),
		filepath.Join(spec.workDir(), ".claude", "settings.json"),
		filepath.Join(spec.workDir(), ".claude", "settings.local.json"),
	}
	if root := claudeLeadLocalSettingsRoot(spec.workDir(), home); root != "" {
		// Claude merges the legacy cwd local file first, then the
		// ownership-checked canonical repository's local settings.
		paths = append(paths, filepath.Join(root, ".claude", "settings.local.json"))
	}
	enabled := map[string]bool{}
	for _, id := range claudeLeadBuiltinPlugins {
		enabled[id] = true
	}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read Claude plugin settings %s: %w", path, err)
		}
		var settings struct {
			EnabledPlugins map[string]bool `json:"enabledPlugins"`
		}
		if err := json.Unmarshal(raw, &settings); err != nil {
			return nil, fmt.Errorf("read Claude plugin settings %s: %w", path, err)
		}
		for id, on := range settings.EnabledPlugins {
			enabled[id] = on
		}
	}
	return enabled, nil
}

// Claude 2.1.323's f_n(projectSettings) uses cwd, not a parent walk.
// f_n(localSettings), resolveLocalSettingsStoreRoot (qN), and Xu additionally
// merge local settings from the canonical Git root on POSIX, except a home
// repository or a root/.git/.claude not owned by the current user. Windows
// has no uid ownership check and keeps the cwd store.
func claudeLeadLocalSettingsRoot(cwd, home string) string {
	if runtime.GOOS == "windows" {
		return ""
	}
	cwd, err := filepath.Abs(cwd)
	if err != nil {
		return ""
	}
	root := cwd
	for {
		if _, err := os.Lstat(filepath.Join(root, ".git")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			return ""
		}
		root = parent
	}
	// A linked worktree's consent store belongs to the main checkout.
	gitEntry := filepath.Join(root, ".git")
	if raw, err := os.ReadFile(gitEntry); err == nil && strings.HasPrefix(strings.TrimSpace(string(raw)), "gitdir:") {
		gitDir := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(raw)), "gitdir:"))
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(root, gitDir)
		}
		if common, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
			commonDir := strings.TrimSpace(string(common))
			if !filepath.IsAbs(commonDir) {
				commonDir = filepath.Join(gitDir, commonDir)
			}
			commonDir = filepath.Clean(commonDir)
			if filepath.Base(commonDir) == ".git" && filepath.Dir(filepath.Clean(gitDir)) == filepath.Join(commonDir, "worktrees") {
				back, err := os.ReadFile(filepath.Join(gitDir, "gitdir"))
				backPath := strings.TrimSpace(string(back))
				if !filepath.IsAbs(backPath) {
					backPath = filepath.Join(gitDir, backPath)
				}
				if err == nil && filepath.Clean(backPath) == gitEntry {
					root = filepath.Dir(commonDir)
				}
			}
		}
	}
	if root == cwd {
		return ""
	}
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil || root == filepath.Clean(realHome) {
		return ""
	}
	if !claudeSettingsOwnedByUser(root, false) || !claudeSettingsOwnedByUser(filepath.Join(root, ".git"), true) {
		return ""
	}
	claudeDir := filepath.Join(root, ".claude")
	if _, err := os.Lstat(claudeDir); !os.IsNotExist(err) && !claudeSettingsOwnedByUser(claudeDir, true) {
		return ""
	}
	return root
}
