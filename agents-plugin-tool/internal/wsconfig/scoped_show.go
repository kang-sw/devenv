package wsconfig

import (
	"fmt"
	"sort"
)

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

	// Gather all known item keys across all scopes so we can report each one.
	allKeys := map[string]struct{}{}

	// Project scope keys.
	for k := range cfg.Overrides {
		allKeys[k] = struct{}{}
	}

	// Global scope keys.
	globalCfg, err := loadGlobalConfig(opts)
	if err != nil {
		return View{}, fmt.Errorf("scoped show: load global overrides: %w", err)
	}
	for k := range globalCfg.Overrides {
		allKeys[k] = struct{}{}
	}

	// Session scope keys: when a sessionR is available, enumerate all session
	// overrides by asking the reader for the full key set. This ensures that
	// session-only keys (not present in project or global) are included.
	// SessionReader exposes ListOverrideKeys for this purpose; if the reader does
	// not implement the optional KeyLister, we fall back to the keys already
	// discovered from the file scopes.
	if r != nil && r.sessionR != nil && sessionKey != "" {
		if lister, ok := r.sessionR.(interface {
			ListOverrideKeys(sessionKey string) []string
		}); ok {
			for _, k := range lister.ListOverrideKeys(sessionKey) {
				allKeys[k] = struct{}{}
			}
		}
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
		sort.Slice(view.ResolvedOverrides, func(i, j int) bool {
			return view.ResolvedOverrides[i].Key < view.ResolvedOverrides[j].Key
		})
	}

	return view, nil
}
