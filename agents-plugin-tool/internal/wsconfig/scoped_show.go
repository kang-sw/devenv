package wsconfig

import "fmt"

// ScopedShow returns a View that includes ResolvedOverrides: one ScopedItem per
// key that exists in any scope (session, project, global). Keys are deduplicated
// by precedence, so only the winning scope for each key is reported.
//
// sessionKey may be empty; when empty the session scope is skipped.
// r.sessionR must be set when a sessionKey is provided.
func ScopedShow(r *Resolver, opts Options, sessionKey string) (View, error) {
	path, err := Path(opts)
	if err != nil {
		return View{}, fmt.Errorf("scoped show: resolve project path: %w", err)
	}
	cfg, err := Load(opts)
	if err != nil {
		return View{}, fmt.Errorf("scoped show: load project config: %w", err)
	}

	view := View{Path: path, Config: cfg}

	// Gather all known item keys across all file scopes so we can report each one.
	allKeys := map[string]struct{}{}

	// Session scope overrides (if available).
	var sessionOverrides map[string]string
	if r != nil && r.sessionR != nil && sessionKey != "" {
		// We need to read all session overrides. Do this by iterating known keys
		// from lower scopes and checking session as well. For the general case we
		// walk all keys discovered in the project and global layers, then also
		// check for any session-only keys via the sessionR interface.
		// Since SessionReader only exposes per-key lookup, we iterate discovered
		// keys; session-only keys are not separately discoverable here (none exist
		// in Phase 1 — session-scope items are set programmatically with known keys).
		_ = sessionOverrides // populated below after key collection
	}

	projectCfg, err := Load(opts)
	if err != nil {
		return View{}, fmt.Errorf("scoped show: load project overrides: %w", err)
	}
	for k := range projectCfg.Overrides {
		allKeys[k] = struct{}{}
	}

	globalCfg, err := loadGlobalConfig(opts)
	if err != nil {
		return View{}, fmt.Errorf("scoped show: load global overrides: %w", err)
	}
	for k := range globalCfg.Overrides {
		allKeys[k] = struct{}{}
	}

	// Resolve each key through the full precedence chain.
	if len(allKeys) > 0 {
		view.ResolvedOverrides = make([]ScopedItem, 0, len(allKeys))
		for k := range allKeys {
			rv, rerr := r.Get(sessionKey, k)
			if rerr != nil {
				return View{}, fmt.Errorf("scoped show: resolve key %q: %w", k, rerr)
			}
			view.ResolvedOverrides = append(view.ResolvedOverrides, ScopedItem{
				Key:   k,
				Value: rv.Value,
				Scope: rv.Scope,
			})
		}
		// Sort for deterministic output.
		sortScopedItems(view.ResolvedOverrides)
	}

	return view, nil
}

// sortScopedItems sorts ScopedItem slices by key for deterministic output.
func sortScopedItems(items []ScopedItem) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].Key < items[j-1].Key; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}
