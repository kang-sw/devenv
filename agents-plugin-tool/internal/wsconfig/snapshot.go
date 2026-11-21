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
	scopes := []Scope{ScopeProject, ScopeGlobal}
	for _, scope := range scopes {
		stored := files[scope]
		if leaf, ok := stored.Agents.ModelAliases[tier][harness]; ok {
			result.Layers[scope] = leaf
		}
		if _, ok := stored.Agents.ModelAliases[tier][result.Bucket]; ok && result.Scope == ScopeBuiltin {
			result.Scope = scope
		}
	}
	if result.Scope == ScopeBuiltin {
		for _, scope := range scopes {
			if _, ok := files[scope].Agents.Tiers[tier]; ok {
				result.Scope = scope
				break
			}
		}
	}
	return result
}
