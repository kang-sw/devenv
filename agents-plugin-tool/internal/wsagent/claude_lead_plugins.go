package wsagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// claudeLeadPluginArgs keeps every ws package out of a hosted lead: the
// daemon supplies its ws-mcp (claudeLeadMCPArgs) and its workflow manual
// (the instruction file), so the user's installed ws and wsflow plugins are
// turned off for this launch and no bundle is loaded. It overrides only
// plugin enablement; setting sources stay, so permissions, hooks, memory,
// MCP servers, and unrelated plugins are the user's own. The record's
// plugin isolation does not apply to a Claude lead.
func claudeLeadPluginArgs(spec StartSpec) ([]string, error) {
	if spec.Lead.PluginWarning != nil {
		spec.Lead.PluginWarning(spec.Lead.PluginNote)
	}
	enabled, err := claudeLeadEnabledPlugins(spec)
	if err != nil {
		return nil, fmt.Errorf("claude lead: %w", err)
	}
	disabled := map[string]bool{}
	for id, on := range enabled {
		name, _, _ := strings.Cut(id, "@")
		if on && slices.Contains(hostedPluginPackages, name) {
			disabled[id] = false
		}
	}
	settings, err := json.Marshal(map[string]any{"enabledPlugins": disabled})
	if err != nil {
		return nil, fmt.Errorf("claude lead plugin settings: %w", err)
	}
	return []string{"--settings", string(settings)}, nil
}

// claudeLeadMCPArgs adds the daemon's ws-mcp to a hosted lead as the MCP
// server ReservedMCPServer: over the daemon's HTTP route when the start has
// a session, else the same server over stdio. There is no
// --strict-mcp-config, so the user's own MCP servers load beside it. A lead
// with neither is refused: without a ws plugin it would have no ws tools.
func claudeLeadMCPArgs(spec StartSpec) ([]string, error) {
	var entry map[string]any
	switch {
	case spec.MCPSession != nil:
		entry = claudeHTTPServer(spec.MCPSession.URL)
	case spec.WSMCP != nil:
		entry = map[string]any{"type": "stdio", "command": spec.WSMCP.Command, "args": spec.WSMCP.Args, "env": spec.WSMCP.Env}
	default:
		return nil, errors.New("claude lead: no ws-mcp to attach (the daemon's MCP route and the bundled launcher are both unavailable)")
	}
	raw, err := json.Marshal(map[string]any{"mcpServers": map[string]any{ReservedMCPServer: entry}})
	if err != nil {
		return nil, err
	}
	return []string{"--mcp-config", string(raw)}, nil
}

// claudeLeadEnabledPlugins merges plugin entries in Claude's user, project,
// local precedence order, including explicit false overrides. Read errors
// other than an absent settings file refuse launch: ignoring one could load
// an installed ws copy beside the daemon's ws-mcp.
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
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return ""
	}
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil || filepath.Clean(realRoot) == filepath.Clean(realHome) {
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
