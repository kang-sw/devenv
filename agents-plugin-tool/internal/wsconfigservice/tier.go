package wsconfigservice

import (
	"fmt"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

// WriteTier applies the existing config.tune compound value to its tier leaf.
func WriteTier(opts wsconfig.Options, value any, harness string, scope wsconfig.Scope, reset bool) (wsconfig.Config, bool, error) {
	fields, ok := value.(map[string]any)
	if !ok {
		return wsconfig.Config{}, false, fmt.Errorf("config.tune: agents.tier value must be an object with tier/backend/model/effort fields (only tier when reset is true)")
	}
	// Tier validation belongs to wsconfig: it accepts legacy synonyms as well
	// as canonical tiers, matching the CLI and the former writer.
	tier, _ := fields["tier"].(string)
	if reset {
		if len(fields) != 1 {
			return wsconfig.Config{}, false, fmt.Errorf("config.tune: agents.tier reset value must contain only tier (no backend, model, or effort)")
		}
	}
	global := scope == wsconfig.ScopeGlobal
	var cfg wsconfig.Config
	var err error
	removed := false
	if reset {
		if global {
			cfg, removed, err = wsconfig.UnsetGlobalAgentsTierForHarness(opts, tier, harness)
		} else {
			cfg, removed, err = wsconfig.UnsetAgentsTierForHarness(opts, tier, harness)
		}
	} else {
		backend, _ := fields["backend"].(string)
		model, _ := fields["model"].(string)
		if effort, ok := fields["effort"].(string); ok {
			if global {
				cfg, err = wsconfig.SetGlobalAgentsTierForHarness(opts, tier, backend, model, harness, effort)
			} else {
				cfg, err = wsconfig.SetAgentsTierForHarness(opts, tier, backend, model, harness, effort)
			}
		} else if global {
			cfg, err = wsconfig.SetGlobalAgentsTierForHarness(opts, tier, backend, model, harness)
		} else {
			cfg, err = wsconfig.SetAgentsTierForHarness(opts, tier, backend, model, harness)
		}
	}
	return cfg, removed, err
}
