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
	CacheHome  string
	// ConfigHome overrides the global config directory. Resolution order:
	// ConfigHome → $WS_CONFIG_HOME → ~/.ws/config.json (note: ~/.ws, not ~/.cache).
	// Mirrors the CacheHome test-seam shape so tests can use Options{ConfigHome: t.TempDir()}.
	ConfigHome string
}

type Config struct {
	SchemaVersion int               `json:"schema_version"`
	Agents        AgentsConfig      `json:"agents"`
	// Overrides is a generic key→value overlay for scope-aware config items.
	// Keys are item identifiers; values are string-encoded config values.
	// The field is additive: existing configs without it parse with a nil map.
	Overrides     map[string]string `json:"overrides,omitempty"`
}

type AgentsConfig struct {
	Tiers        map[string]AgentTier            `json:"tiers,omitempty"`
	ModelAliases map[string]map[string]AgentTier `json:"model_aliases,omitempty"`
}

type View struct {
	Path   string `json:"path"`
	Config Config `json:"config"`
	// ResolvedOverrides carries each scoped config item's value and the scope it
	// resolved from. Populated by ScopedShow; nil when populated by plain Show.
	ResolvedOverrides []ScopedItem `json:"resolved_overrides,omitempty"`
}

// ScopedItem pairs an item key with its resolved value and source scope for
// human-readable and JSON show output.
type ScopedItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Scope Scope  `json:"scope"`
}

type AgentTier struct {
	Backend string `json:"backend,omitempty"`
	Model   string `json:"model,omitempty"`
	Effort  string `json:"effort,omitempty"`
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
	// Load-time key migration: normalize legacy light/core/deep keys to capability
	// vocabulary (small/medium/large). This is in-memory only — no file rewrite,
	// no schemaVersion bump. Precedence: if both a legacy key and its capability
	// key already exist, the capability key wins; the legacy duplicate is dropped.
	normalizeLegacyTierKeys(cfg.Agents.Tiers, cfg.Agents.ModelAliases)
	applyDefaultTiers(cfg.Agents.Tiers)
	if cfg.Agents.ModelAliases == nil {
		cfg.Agents.ModelAliases = map[string]map[string]AgentTier{}
	}
	applyDefaultModelAliases(cfg.Agents.Tiers, cfg.Agents.ModelAliases)
	return cfg, nil
}

// normalizeLegacyTierKeys migrates persisted light/core/deep map keys to their
// capability equivalents (small/medium/large) in-memory. Capability key wins on
// collision; legacy key is dropped. Operates on both Tiers and ModelAliases.
func normalizeLegacyTierKeys(tiers map[string]AgentTier, aliases map[string]map[string]AgentTier) {
	// Normalize Tiers map.
	for legacyKey, val := range tiers {
		capKey := normalizedTier(legacyKey)
		if capKey == "" || capKey == legacyKey {
			// Already a capability key or unrecognized — skip.
			continue
		}
		if _, exists := tiers[capKey]; !exists {
			tiers[capKey] = val
		}
		// Capability key wins; drop the legacy duplicate.
		delete(tiers, legacyKey)
	}
	if aliases == nil {
		return
	}
	// Normalize ModelAliases outer (alias) key.
	for legacyKey, byHarness := range aliases {
		capKey := normalizedTier(legacyKey)
		if capKey == "" || capKey == legacyKey {
			continue
		}
		if _, exists := aliases[capKey]; !exists {
			aliases[capKey] = byHarness
		}
		delete(aliases, legacyKey)
	}
}

func Show(opts Options) (View, error) {
	path, err := Path(opts)
	if err != nil {
		return View{}, err
	}
	cfg, err := Load(opts)
	if err != nil {
		return View{}, err
	}
	return View{Path: path, Config: cfg}, nil
}

func SetAgentsTier(opts Options, tier, backend, model string, effortValues ...string) (Config, error) {
	return SetAgentsTierForHarness(opts, tier, backend, model, "", effortValues...)
}

func SetAgentsTierForHarness(opts Options, tier, backend, model, harness string, effortValues ...string) (Config, error) {
	tier = normalizedTier(tier)
	if tier == "" {
		return Config{}, fmt.Errorf("tier must be small, medium, large, or xlarge")
	}
	backend = strings.TrimSpace(backend)
	model = strings.TrimSpace(model)
	hasBackendInput := backend != ""
	hasModelInput := model != ""
	effort, hasEffort, err := normalizeOptionalEffort(effortValues...)
	if err != nil {
		return Config{}, err
	}
	if backend == "" {
		backend = InferBackend(model)
	}
	cfg, err := Load(opts)
	if err != nil {
		return Config{}, err
	}
	if cfg.Agents.ModelAliases == nil {
		cfg.Agents.ModelAliases = map[string]map[string]AgentTier{}
	}
	if cfg.Agents.ModelAliases[tier] == nil {
		cfg.Agents.ModelAliases[tier] = map[string]AgentTier{}
	}
	key, err := aliasTargetKey(harness)
	if err != nil {
		return Config{}, err
	}
	existing := cfg.Agents.ModelAliases[tier][key]
	if fallback, ok := cfg.Agents.Tiers[tier]; ok {
		if strings.TrimSpace(existing.Backend) == "" && strings.TrimSpace(existing.Model) == "" {
			existing = fallback
		}
	}
	mapping := AgentTier{}
	if !hasBackendInput && !hasModelInput {
		mapping = existing
	}
	if backend != "" {
		mapping.Backend = backend
	}
	if model != "" {
		mapping.Model = model
		if backend == "" {
			mapping.Backend = InferBackend(model)
		}
	}
	if hasEffort {
		mapping.Effort = effort
	} else {
		mapping.Effort = ""
	}
	cfg.Agents.ModelAliases[tier][key] = mapping
	if key == "default" {
		cfg.Agents.Tiers[tier] = mapping
	}
	return cfg, save(opts, cfg)
}

func ResolveAgent(opts Options, tier, backend, model string) (string, string, error) {
	return ResolveAgentForHarness(opts, tier, backend, model, "")
}

func ResolveAgentForHarness(opts Options, tier, backend, model, harness string) (string, string, error) {
	backend, model, _, err := ResolveAgentForHarnessConfig(opts, tier, backend, model, harness)
	return backend, model, err
}

func ResolveAgentForHarnessConfig(opts Options, tier, backend, model, harness string) (string, string, string, error) {
	tier = normalizedTier(tier)
	backend = strings.TrimSpace(backend)
	model = strings.TrimSpace(model)
	if alias := ModelAlias(model); alias != "" {
		tier = alias
		model = ""
	}
	if tier == "" {
		tier = "medium"
	}
	if model != "" {
		if backend == "" {
			backend = InferBackend(model)
		}
		if backend == "" {
			backend = "codex"
		}
		return backend, model, "", nil
	}
	cfg, err := Load(opts)
	if err != nil {
		return "", "", "", err
	}
	effort := ""
	if mapping, ok := resolveAliasMapping(cfg, tier, backend, harness); ok {
		if useAliasMappingForBackend(backend, mapping) {
			if model == "" {
				model = strings.TrimSpace(mapping.Model)
			}
			if backend == "" {
				backend = strings.TrimSpace(mapping.Backend)
			}
			if backend == "" {
				backend = InferBackend(model)
			}
			effort = strings.TrimSpace(mapping.Effort)
		}
	}
	if backend == "" {
		backend = "codex"
	}
	return backend, model, effort, nil
}

func ModelAlias(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "small", "light":
		return "small"
	case "medium", "core":
		return "medium"
	case "large", "deep":
		return "large"
	case "xlarge":
		return "xlarge"
	default:
		return ""
	}
}

func InferBackend(model string) string {
	value := strings.ToLower(strings.TrimSpace(model))
	switch {
	case value == "":
		return ""
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
	tiers := map[string]AgentTier{}
	applyDefaultTiers(tiers)
	return Config{
		SchemaVersion: schemaVersion,
		Agents: AgentsConfig{
			Tiers:        tiers,
			ModelAliases: defaultModelAliases(tiers),
		},
	}
}

func applyDefaultTiers(tiers map[string]AgentTier) {
	defaults := map[string]AgentTier{
		"small":  {Backend: "codex", Model: "gpt-5.6-luna", Effort: "medium"},
		"medium": {Backend: "codex", Model: "gpt-5.6-terra", Effort: "high"},
		"large":  {Backend: "codex", Model: "gpt-5.6-sol", Effort: "high"},
		"xlarge": {Backend: "codex", Model: "gpt-5.6-sol", Effort: "xhigh"},
	}
	for tier, mapping := range defaults {
		if _, ok := tiers[tier]; !ok {
			tiers[tier] = mapping
		}
	}
}

func applyDefaultModelAliases(tiers map[string]AgentTier, aliases map[string]map[string]AgentTier) {
	defaults := defaultModelAliases(tiers)
	for alias, byHarness := range defaults {
		if aliases[alias] == nil {
			aliases[alias] = map[string]AgentTier{}
		}
		for harness, mapping := range byHarness {
			if _, ok := aliases[alias][harness]; !ok {
				aliases[alias][harness] = mapping
			}
		}
	}
}

func defaultModelAliases(tiers map[string]AgentTier) map[string]map[string]AgentTier {
	return map[string]map[string]AgentTier{
		"small": {
			"default": tierOrDefault(tiers, "small", AgentTier{Backend: "codex", Model: "gpt-5.6-luna", Effort: "medium"}),
			"codex":   tierOrDefault(tiers, "small", AgentTier{Backend: "codex", Model: "gpt-5.6-luna", Effort: "medium"}),
			"claude":  {Backend: "claude", Model: "haiku"},
		},
		"medium": {
			"default": tierOrDefault(tiers, "medium", AgentTier{Backend: "codex", Model: "gpt-5.6-terra", Effort: "high"}),
			"codex":   tierOrDefault(tiers, "medium", AgentTier{Backend: "codex", Model: "gpt-5.6-terra", Effort: "high"}),
			"claude":  {Backend: "claude", Model: "sonnet"},
		},
		"large": {
			"default": tierOrDefault(tiers, "large", AgentTier{Backend: "codex", Model: "gpt-5.6-sol", Effort: "high"}),
			"codex":   tierOrDefault(tiers, "large", AgentTier{Backend: "codex", Model: "gpt-5.6-sol", Effort: "high"}),
			"claude":  {Backend: "claude", Model: "opus"},
		},
		"xlarge": {
			"default": tierOrDefault(tiers, "xlarge", AgentTier{Backend: "codex", Model: "gpt-5.6-sol", Effort: "xhigh"}),
			"codex":   tierOrDefault(tiers, "xlarge", AgentTier{Backend: "codex", Model: "gpt-5.6-sol", Effort: "xhigh"}),
			"claude":  {Backend: "claude", Model: "opus"},
		},
	}
}

func tierOrDefault(tiers map[string]AgentTier, tier string, fallback AgentTier) AgentTier {
	if mapping, ok := tiers[tier]; ok {
		return mapping
	}
	return fallback
}

func resolveAliasMapping(cfg Config, alias, backend, harness string) (AgentTier, bool) {
	value := alias
	alias = ModelAlias(value)
	if alias == "" {
		alias = normalizedTier(value)
	}
	if alias == "" {
		return AgentTier{}, false
	}
	byHarness := cfg.Agents.ModelAliases[alias]
	for _, key := range aliasResolutionKeys(backend, harness) {
		if mapping, ok := byHarness[key]; ok {
			return mapping, true
		}
	}
	if mapping, ok := cfg.Agents.Tiers[alias]; ok {
		return mapping, true
	}
	return AgentTier{}, false
}

func aliasResolutionKeys(backend, harness string) []string {
	keys := []string{}
	if key := normalizedHarness(backend); key != "" {
		keys = append(keys, key)
	}
	if key := normalizedHarness(harness); key != "" {
		keys = append(keys, key)
	}
	keys = append(keys, "default", "codex")
	seen := map[string]bool{}
	result := []string{}
	for _, key := range keys {
		if !seen[key] {
			seen[key] = true
			result = append(result, key)
		}
	}
	return result
}

func normalizedHarness(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "codex":
		return "codex"
	case "claude":
		return "claude"
	default:
		return ""
	}
}

func aliasTargetKey(harness string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(harness))
	if value == "" || value == "default" {
		return "default", nil
	}
	if key := normalizedHarness(value); key != "" {
		return key, nil
	}
	return "", fmt.Errorf("harness must be codex, claude, or default")
}

func normalizeOptionalEffort(values ...string) (string, bool, error) {
	if len(values) == 0 {
		return "", false, nil
	}
	value := strings.ToLower(strings.TrimSpace(values[0]))
	switch value {
	case "", "none":
		return "", true, nil
	case "low", "medium", "high", "xhigh":
		return value, true, nil
	default:
		return "", true, fmt.Errorf("effort must be none, low, medium, high, or xhigh")
	}
}

func useAliasMappingForBackend(explicitBackend string, mapping AgentTier) bool {
	explicitKey := normalizedHarness(explicitBackend)
	if explicitKey == "" {
		return true
	}
	mappingKey := normalizedHarness(mapping.Backend)
	if mappingKey != "" {
		return mappingKey == explicitKey
	}
	inferredKey := normalizedHarness(InferBackend(mapping.Model))
	if inferredKey == "" {
		return true
	}
	return inferredKey == explicitKey
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
	case "small", "light", "haiku":
		return "small"
	case "medium", "core", "sonnet":
		return "medium"
	case "large", "deep", "opus":
		return "large"
	case "xlarge":
		return "xlarge"
	default:
		return ""
	}
}
