package wsconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kang-sw/devenv/internal/wsstate"
)

const schemaVersion = 1

type Options struct {
	CacheHome string
}

type Config struct {
	SchemaVersion int          `json:"schema_version"`
	Agents        AgentsConfig `json:"agents"`
}

type AgentsConfig struct {
	Tiers map[string]AgentTier `json:"tiers,omitempty"`
}

type AgentTier struct {
	Backend string `json:"backend,omitempty"`
	Model   string `json:"model,omitempty"`
}

func Load(opts Options) (Config, error) {
	path, err := Path(opts)
	if err != nil {
		return Config{}, err
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return defaultConfig(), nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read ws config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse ws config: %w", err)
	}
	if cfg.SchemaVersion == 0 {
		cfg.SchemaVersion = schemaVersion
	}
	if cfg.Agents.Tiers == nil {
		cfg.Agents.Tiers = map[string]AgentTier{}
	}
	return cfg, nil
}

func SetAgentsTier(opts Options, tier, backend, model string) (Config, error) {
	tier = normalizedTier(tier)
	if tier == "" {
		return Config{}, fmt.Errorf("tier must be light, core, or deep")
	}
	backend = strings.TrimSpace(backend)
	model = strings.TrimSpace(model)
	if backend == "" {
		backend = InferBackend(model)
	}
	cfg, err := Load(opts)
	if err != nil {
		return Config{}, err
	}
	cfg.Agents.Tiers[tier] = AgentTier{Backend: backend, Model: model}
	return cfg, save(opts, cfg)
}

func ResolveAgent(opts Options, tier, backend, model string) (string, string, error) {
	tier = normalizedTier(tier)
	if tier == "" {
		tier = "core"
	}
	backend = strings.TrimSpace(backend)
	model = strings.TrimSpace(model)
	if model != "" {
		if backend == "" {
			backend = InferBackend(model)
		}
		if backend == "" {
			backend = "codex"
		}
		return backend, model, nil
	}
	cfg, err := Load(opts)
	if err != nil {
		return "", "", err
	}
	if mapping, ok := cfg.Agents.Tiers[tier]; ok {
		if model == "" {
			model = strings.TrimSpace(mapping.Model)
		}
		if backend == "" {
			backend = strings.TrimSpace(mapping.Backend)
		}
		if backend == "" {
			backend = InferBackend(model)
		}
	}
	if backend == "" {
		backend = "codex"
	}
	return backend, model, nil
}

func InferBackend(model string) string {
	value := strings.ToLower(strings.TrimSpace(model))
	switch {
	case value == "":
		return ""
	case strings.Contains(value, "gemini"):
		return "gemini"
	case strings.HasPrefix(value, "gpt-") || strings.Contains(value, "codex"):
		return "codex"
	case strings.Contains(value, "haiku") || strings.Contains(value, "sonnet") ||
		strings.Contains(value, "opus") || strings.Contains(value, "claude"):
		return "claude"
	default:
		return ""
	}
}

func Path(opts Options) (string, error) {
	root, err := wsstate.CacheRoot(wsstate.Options{CacheHome: opts.CacheHome})
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "config.json"), nil
}

func defaultConfig() Config {
	return Config{
		SchemaVersion: schemaVersion,
		Agents: AgentsConfig{
			Tiers: map[string]AgentTier{},
		},
	}
}

func save(opts Options, cfg Config) error {
	path, err := Path(opts)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create ws config dir: %w", err)
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode ws config: %w", err)
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}

func normalizedTier(tier string) string {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case "haiku", "light":
		return "light"
	case "sonnet", "core":
		return "core"
	case "opus", "deep":
		return "deep"
	default:
		return ""
	}
}
