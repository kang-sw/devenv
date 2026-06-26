package wsconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	envConfigHome       = "WS_CONFIG_HOME"
	defaultConfigDirName = ".ws"
)

// GlobalPath resolves the cross-project global config file path.
// Resolution order: opts.ConfigHome → $WS_CONFIG_HOME → ~/.ws/config.json.
func GlobalPath(opts Options) (string, error) {
	if opts.ConfigHome != "" {
		return filepath.Join(opts.ConfigHome, "config.json"), nil
	}
	if env := os.Getenv(envConfigHome); env != "" {
		return filepath.Join(env, "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir for global config: %w", err)
	}
	return filepath.Join(home, defaultConfigDirName, "config.json"), nil
}

// loadGlobalConfig reads the global config file. A missing file returns an
// empty Config (equivalent to "no global overrides"), mirroring the project
// Load pattern.
func loadGlobalConfig(opts Options) (Config, error) {
	path, err := GlobalPath(opts)
	if err != nil {
		return Config{}, err
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Config{}, nil // empty global layer — not an error
	}
	if err != nil {
		return Config{}, fmt.Errorf("read global ws config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse global ws config: %w", err)
	}
	return cfg, nil
}
