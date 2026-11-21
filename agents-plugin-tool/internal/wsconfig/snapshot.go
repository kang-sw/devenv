package wsconfig

// FileLayers captures the raw config from each file scope once. A caller
// supplies the worktree root only when its inspection context is known.
func FileLayers(opts Options) (map[Scope]Config, error) {
	layers := make(map[Scope]Config, 3)
	var err error
	layers[ScopeGlobal], err = loadGlobalConfig(opts)
	if err != nil {
		return nil, err
	}
	layers[ScopeProject], err = loadProjectConfig(opts)
	if err != nil {
		return nil, err
	}
	if _, ok := RepoPath(opts); ok {
		layers[ScopeRepo], err = loadRepoConfig(opts)
		if err != nil {
			return nil, err
		}
	}
	return layers, nil
}

// TierInspection preserves the same alias fallback and legacy tier seeding
// as ResolveAgentTierForHarness. Layers contain exact stored leaves.
type TierInspection struct {
	Default, Value AgentTier
	Scope          Scope
	Bucket         string
	Layers         map[Scope]AgentTier
}

func InspectTier(files map[Scope]Config, tier, harness string) TierInspection {
	global, project := files[ScopeGlobal], files[ScopeProject]
	result := TierInspection{Scope: ScopeBuiltin, Layers: make(map[Scope]AgentTier, 2)}
	result.Default, _, _ = resolveAliasMapping(effectiveAgentConfig(), tier, "", harness)
	result.Value, result.Bucket, _ = resolveAliasMapping(effectiveAgentConfig(global, project), tier, "", harness)
	for _, scope := range []Scope{ScopeProject, ScopeGlobal} {
		stored := files[scope]
		if leaf, ok := stored.Agents.ModelAliases[tier][harness]; ok {
			result.Layers[scope] = leaf
		}
		if result.Scope != ScopeBuiltin {
			continue
		}
		if _, ok := stored.Agents.ModelAliases[tier][result.Bucket]; ok {
			result.Scope = scope
			continue
		}
		if legacy, ok := stored.Agents.Tiers[tier]; ok && (result.Bucket == "tiers" || legacy == result.Value) {
			result.Scope = scope
		}
	}
	return result
}
