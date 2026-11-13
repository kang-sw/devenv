package wsconfig

import (
	"fmt"
	"sort"
	"strings"
)

// ScalarValue is the winning scalar value for one key, including empty values.
type ScalarValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Scope Scope  `json:"scope"`
}

// ListScalars lists keys with prefix in session > project > repo > global >
// builtin order, sorted by key. An empty value still wins: callers may use it
// as a tombstone rather than falling through to a broader scope.
func (r *Resolver) ListScalars(sessionKey, prefix string) ([]ScalarValue, error) {
	winners := map[string]ScalarValue{}
	add := func(values map[string]string, scope Scope) {
		for key, value := range values {
			if !strings.HasPrefix(key, prefix) || (GlobalOnly(key) && scope != ScopeGlobal && scope != ScopeBuiltin) {
				continue
			}
			if _, taken := winners[key]; !taken {
				winners[key] = ScalarValue{Key: key, Value: value, Scope: scope}
			}
		}
	}
	if r.sessionR != nil && sessionKey != "" {
		if lister, ok := r.sessionR.(interface{ ListOverrideKeys(string) []string }); ok {
			values := map[string]string{}
			for _, key := range lister.ListOverrideKeys(sessionKey) {
				if value, found := r.sessionR.GetOverride(sessionKey, key); found {
					values[key] = value
				}
			}
			add(values, ScopeSession)
		}
	}
	project, err := Load(r.opts)
	if err != nil {
		return nil, fmt.Errorf("resolver: load project config: %w", err)
	}
	add(project.Overrides, ScopeProject)
	repo, err := loadRepoConfig(r.opts)
	if err != nil {
		return nil, fmt.Errorf("resolver: load repo config: %w", err)
	}
	add(repo.Overrides, ScopeRepo)
	global, err := loadGlobalConfig(r.opts)
	if err != nil {
		return nil, fmt.Errorf("resolver: load global config: %w", err)
	}
	add(global.Overrides, ScopeGlobal)
	add(r.builtin, ScopeBuiltin)
	out := make([]ScalarValue, 0, len(winners))
	for _, value := range winners {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

const PreferencePrefix = "preference."

// ValidPreferenceKey reports whether key names a lowercase preference slot.
func ValidPreferenceKey(key string) bool {
	if !strings.HasPrefix(key, PreferencePrefix) {
		return false
	}
	slot := strings.TrimPrefix(key, PreferencePrefix)
	if slot == "" {
		return false
	}
	for i, c := range slot {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			continue
		}
		if i > 0 && (c == '_' || c == '-') {
			continue
		}
		return false
	}
	return true
}
