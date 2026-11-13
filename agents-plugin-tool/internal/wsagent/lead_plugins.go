package wsagent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kang-sw/devenv/internal/wsrsrc"
)

// bundledPluginRoot uses the daemon's package, never the session's project or
// an installed marketplace copy. It resolves only through the resource root
// (WS_RSRC_ROOT or the executable-derived bundle); a source build gets its
// package from the launcher environment, never from a source-tree path.
func bundledPluginRoot() (string, error) {
	rsrc, err := wsrsrc.ResolveRoot()
	if err != nil {
		return "", fmt.Errorf("resolve bundled plugin: %w", err)
	}
	root, err := filepath.Abs(filepath.Dir(filepath.Clean(rsrc)))
	if err != nil {
		return "", fmt.Errorf("resolve bundled plugin path: %w", err)
	}
	if err := validateBundledPlugin(root); err != nil {
		if os.Getenv(wsrsrc.EnvRsrcRoot) != "" {
			// An explicit resource root must not silently select a different
			// package (in particular ws in place of wsflow).
			return "", err
		}
		return "", fmt.Errorf("bundled plugin not found at %s; start the daemon through its plugin launcher: %w", root, err)
	}
	return root, nil
}

func validateBundledPlugin(root string) error {
	path := filepath.Join(root, ".claude-plugin", "plugin.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read bundled plugin manifest %s: %w", path, err)
	}
	var manifest struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return fmt.Errorf("read bundled plugin manifest %s: %w", path, err)
	}
	if manifest.Name != "ws" && manifest.Name != "wsflow" {
		return fmt.Errorf("bundled plugin manifest %s has unsupported name %q", path, manifest.Name)
	}
	return nil
}
