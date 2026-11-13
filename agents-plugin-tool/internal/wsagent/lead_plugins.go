package wsagent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kang-sw/devenv/internal/wsrsrc"
)

// bundledPluginRoot uses the daemon's package, never the session's project or
// an installed marketplace copy. Source builds without a launcher-provided
// resource root use the package beside this source tree.
func bundledPluginRoot() (string, error) {
	rsrc, err := wsrsrc.ResolveRoot()
	if err != nil {
		return "", fmt.Errorf("resolve bundled plugin: %w", err)
	}
	root, err := filepath.Abs(filepath.Dir(filepath.Clean(rsrc)))
	if err != nil {
		return "", fmt.Errorf("resolve bundled plugin path: %w", err)
	}
	if err := validateBundledPlugin(root); err == nil {
		return root, nil
	} else if os.Getenv(wsrsrc.EnvRsrcRoot) != "" {
		// An explicit resource root must not silently select a different
		// package (in particular ws in place of wsflow).
		return "", err
	}
	if _, source, _, ok := runtime.Caller(0); ok && filepath.IsAbs(source) {
		devRoot := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", "..", "agents-plugin"))
		if err := validateBundledPlugin(devRoot); err == nil {
			return devRoot, nil
		}
	}
	return "", fmt.Errorf("bundled plugin not found at %s; start the daemon through its plugin launcher", root)
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
