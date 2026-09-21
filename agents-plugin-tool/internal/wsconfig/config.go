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
	// ConfigHome overrides the global config directory. Resolution order:
	// ConfigHome → $WS_CONFIG_HOME → ~/.ws/config.json (note: ~/.ws, not ~/.cache).
	// Mirrors the CacheHome test-seam shape so tests can use Options{ConfigHome: t.TempDir()}.
	ConfigHome string
	// RepoRoot anchors the committed repo scope: when non-empty the resolver
	// reads <RepoRoot>/.ws-workflow/config.json (see RepoPath). It is the caller's
	// canonical Git worktree root — the same root the session key resolves to, so
	// each worktree reads its own checked-out copy of the committed file. Empty
	// means "no repo scope for this resolver" (the file scopes fall through to
	// global/builtin); the repo layer is never an error just for being absent.
	RepoRoot string
}

type Config struct {
	SchemaVersion int          `json:"schema_version"`
	Agents        AgentsConfig `json:"agents"`
	// Overrides is a generic key→value overlay for scope-aware config items.
	// Keys are item identifiers; values are string-encoded config values.
	// The field is additive: existing configs without it parse with a nil map.
	Overrides map[string]string `json:"overrides,omitempty"`
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
	stored, err := loadProjectConfig(opts)
	if err != nil {
		return Config{}, err
	}
	cfg := effectiveAgentConfig(stored)
	cfg.Overrides = stored.Overrides
	return cfg, nil
}

// loadProjectConfig reads only the persisted project layer. Unlike Load, it
// does not add builtin aliases: callers that combine project and global layers
// must distinguish an absent leaf from a builtin fallback.
func loadProjectConfig(opts Options) (Config, error) {
	path, err := Path(opts)
	if err != nil {
		return Config{}, err
	}
	return loadConfigFile(path, "project")
}

func loadConfigFile(path, scope string) (Config, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read %s ws config: %w", scope, err)
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s ws config: %w", scope, err)
	}
	return cfg, nil
}

// effectiveAgentConfig overlays sparse config layers onto builtin tier aliases.
// Later layers win per tier/harness leaf, rather than replacing a whole tier map.
func effectiveAgentConfig(layers ...Config) Config {
	cfg := Config{
		SchemaVersion: schemaVersion,
		Agents: AgentsConfig{
			Tiers:        map[string]AgentTier{},
			ModelAliases: map[string]map[string]AgentTier{},
		},
	}
	for _, layer := range layers {
		normalizeLegacyTierKeys(layer.Agents.Tiers, layer.Agents.ModelAliases)
		mergeAgentConfig(&cfg, layer)
	}
	explicitTiers := make(map[string]bool, len(cfg.Agents.Tiers))
	for tier := range cfg.Agents.Tiers {
		explicitTiers[tier] = true
	}
	applyDefaultTiers(cfg.Agents.Tiers)

	// Legacy configurations that define a tier but no aliases expect that tier
	// to seed every alias. Once a layer defines any alias leaf, however, missing
	// siblings are builtin leaves, not implicit copies of that layer's default
	// tier: this preserves sparse project/global leaf overlays.
	aliasTiers := defaultConfig().Agents.Tiers
	for tier, mapping := range cfg.Agents.Tiers {
		if explicitTiers[tier] || len(cfg.Agents.ModelAliases[tier]) == 0 {
			aliasTiers[tier] = mapping
		}
	}
	storedAliases := cfg.Agents.ModelAliases
	cfg.Agents.ModelAliases = defaultModelAliases(aliasTiers)
	mergeAgentConfig(&cfg, Config{Agents: AgentsConfig{ModelAliases: storedAliases}})
	return cfg
}

func mergeAgentConfig(dst *Config, src Config) {
	for tier, mapping := range src.Agents.Tiers {
		dst.Agents.Tiers[tier] = mapping
	}
	for tier, byHarness := range src.Agents.ModelAliases {
		if dst.Agents.ModelAliases[tier] == nil {
			dst.Agents.ModelAliases[tier] = map[string]AgentTier{}
		}
		for harness, mapping := range byHarness {
			dst.Agents.ModelAliases[tier][harness] = mapping
		}
	}
}

// LoadAgentTierConfig returns effective AgentTier values after applying the
// builtin < global < project leaf overlay. It intentionally leaves Overrides
// alone: scalar overrides remain the Resolver's responsibility.
func LoadAgentTierConfig(opts Options) (Config, error) {
	globalCfg, err := loadGlobalConfig(opts)
	if err != nil {
		return Config{}, err
	}
	projectCfg, err := loadProjectConfig(opts)
	if err != nil {
		return Config{}, err
	}
	return effectiveAgentConfig(globalCfg, projectCfg), nil
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
	return View{Path: path, Config: presentationAgentConfig(cfg)}, nil
}

// ShowResolved is Show's display-layer sibling: it composes the agents.tier
// model_aliases the same builtin<global<project way LoadAgentTierConfig
// already resolves them (the same precedence the MCP config.list tuning
// catalog surfaces via currentAgentTierMappings), instead of Show's
// project-only Load. Path and Overrides stay project-scoped, matching Show;
// only the agent-tier mapping gains global-scope visibility. Intended for CLI
// display callers that want the MCP-consistent resolved view without
// reimplementing precedence.
func ShowResolved(opts Options) (View, error) {
	path, err := Path(opts)
	if err != nil {
		return View{}, err
	}
	tierCfg, err := LoadAgentTierConfig(opts)
	if err != nil {
		return View{}, err
	}
	projectCfg, err := loadProjectConfig(opts)
	if err != nil {
		return View{}, err
	}
	cfg := presentationAgentConfig(tierCfg)
	cfg.Overrides = projectCfg.Overrides
	return View{Path: path, Config: cfg}, nil
}

// presentationAgentConfig keeps the legacy Tiers view aligned with an explicit
// default alias without making that compatibility view part of read precedence.
func presentationAgentConfig(cfg Config) Config {
	for tier, aliases := range cfg.Agents.ModelAliases {
		if mapping, ok := aliases["default"]; ok {
			cfg.Agents.Tiers[tier] = mapping
		}
	}
	return cfg
}

func SetAgentsTier(opts Options, tier, backend, model string, effortValues ...string) (Config, error) {
	return SetAgentsTierForHarness(opts, tier, backend, model, "", effortValues...)
}

// SetAgentsTierForHarness writes one tier/harness leaf to project scope.
func SetAgentsTierForHarness(opts Options, tier, backend, model, harness string, effortValues ...string) (Config, error) {
	cfg, err := setAgentsTierForHarness(opts, false, tier, backend, model, harness, effortValues...)
	if err != nil {
		return Config{}, err
	}
	return presentationAgentConfig(effectiveAgentConfig(cfg)), nil
}

// SetGlobalAgentsTierForHarness writes one tier/harness leaf to the cross-project
// global config. It stays separate from the resolver because AgentTier is a
// structured value, not a resolver override string.
func SetGlobalAgentsTierForHarness(opts Options, tier, backend, model, harness string, effortValues ...string) (Config, error) {
	cfg, err := setAgentsTierForHarness(opts, true, tier, backend, model, harness, effortValues...)
	if err != nil {
		return Config{}, err
	}
	return presentationAgentConfig(effectiveAgentConfig(cfg)), nil
}

func setAgentsTierForHarness(opts Options, global bool, tier, backend, model, harness string, effortValues ...string) (Config, error) {
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

	var stored Config
	if global {
		stored, err = loadGlobalConfig(opts)
	} else {
		stored, err = loadProjectConfig(opts)
	}
	if err != nil {
		return Config{}, err
	}
	normalizeLegacyTierKeys(stored.Agents.Tiers, stored.Agents.ModelAliases)
	effective := effectiveAgentConfig(stored)
	if stored.Agents.Tiers == nil {
		stored.Agents.Tiers = map[string]AgentTier{}
	}
	if stored.Agents.ModelAliases == nil {
		stored.Agents.ModelAliases = map[string]map[string]AgentTier{}
	}
	if stored.Agents.ModelAliases[tier] == nil {
		stored.Agents.ModelAliases[tier] = map[string]AgentTier{}
	}
	key, err := aliasTargetKey(harness)
	if err != nil {
		return Config{}, err
	}
	existing := effective.Agents.ModelAliases[tier][key]
	if fallback, ok := effective.Agents.Tiers[tier]; ok {
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
	stored.Agents.ModelAliases[tier][key] = mapping
	if global {
		return stored, saveGlobal(opts, stored)
	}
	return stored, save(opts, stored)
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
	cfg, err := LoadAgentTierConfig(opts)
	if err != nil {
		return "", "", "", err
	}
	effort := ""
	if mapping, _, ok := resolveAliasMapping(cfg, tier, backend, harness); ok {
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

// ResolveAgentTierForHarness resolves a fixed tier's {backend, model, effort}
// under a given harness, applying the same alias-resolution fallback chain
// ResolveAgentForHarnessConfig uses, and reports which aliasResolutionKeys
// bucket answered (resolvedFrom) so a caller can distinguish a harness-local
// hit from a cross-harness fallback without re-deriving the chain itself.
// Unlike ResolveAgentForHarnessConfig (which silently coerces an
// empty/unrecognized tier to "medium" for its existing callers), this
// function rejects an unknown tier outright.
func ResolveAgentTierForHarness(opts Options, tier, harness string) (backend, model, effort, resolvedFrom string, err error) {
	normalized := normalizedTier(tier)
	if normalized == "" {
		return "", "", "", "", fmt.Errorf("tier must be small, medium, large, or xlarge; got %q", tier)
	}
	cfg, err := LoadAgentTierConfig(opts)
	if err != nil {
		return "", "", "", "", err
	}
	mapping, source, ok := resolveAliasMapping(cfg, normalized, "", harness)
	if !ok {
		return "", "", "", "", fmt.Errorf("no configuration found for tier %q", normalized)
	}
	backend = strings.TrimSpace(mapping.Backend)
	model = strings.TrimSpace(mapping.Model)
	effort = strings.TrimSpace(mapping.Effort)
	if backend == "" {
		backend = InferBackend(model)
	}
	return backend, model, effort, source, nil
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

func resolveAliasMapping(cfg Config, alias, backend, harness string) (AgentTier, string, bool) {
	value := alias
	alias = ModelAlias(value)
	if alias == "" {
		alias = normalizedTier(value)
	}
	if alias == "" {
		return AgentTier{}, "", false
	}
	byHarness := cfg.Agents.ModelAliases[alias]
	for _, key := range aliasResolutionKeys(backend, harness) {
		if mapping, ok := byHarness[key]; ok {
			return mapping, key, true
		}
	}
	if mapping, ok := cfg.Agents.Tiers[alias]; ok {
		return mapping, "tiers", true
	}
	return AgentTier{}, "", false
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
	case "pi":
		return "pi"
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
	return "", fmt.Errorf("harness must be codex, claude, pi, or default")
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
	return saveConfigFile(path, cfg)
}

func saveGlobal(opts Options, cfg Config) error {
	path, err := GlobalPath(opts)
	if err != nil {
		return err
	}
	return saveConfigFile(path, cfg)
}

func saveConfigFile(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create ws config dir: %w", err)
	}
	// A brand-new config's zero-value Config carries SchemaVersion 0 until now
	// (setAgentsTierForHarness's `stored` layer starts from loadProjectConfig/
	// loadGlobalConfig, which return Config{} for a missing file). The legacy
	// Load path always stamped 1 in-memory via effectiveAgentConfig before any
	// caller saw it, but that stamping never reached the persisted bytes for a
	// first-ever config.tune write. Stamp it here, at the single save chokepoint
	// for both project and global scope, so what lands on disk matches the
	// established schema_version:1 contract regardless of caller. This is
	// persistence-only: in-memory resolution (effectiveAgentConfig) already
	// normalizes to schemaVersion unconditionally and is untouched.
	cfg.SchemaVersion = schemaVersion
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

// NormalizedTier exposes normalizedTier's alias-folding to callers outside this
// package: any accepted spelling ("Large", "opus", "deep", ...) collapses to
// its first-class taxonomy value ("small"/"medium"/"large"/"xlarge"); an
// unknown/unrecognized tier returns "". Callers that already validated the
// tier via ResolveAgentTierForHarness use this to recover the canonical form
// for a return channel (e.g. a recommended-tier line) that must stay in
// first-class vocabulary regardless of which accepted alias the caller typed.
func NormalizedTier(tier string) string {
	return normalizedTier(tier)
}
