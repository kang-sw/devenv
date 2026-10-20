package wsconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// StructuredValue is one resolved structured (JSON-object) item: the winning
// value for Key, the scope it came from, and the file it was read from (empty
// for the builtin floor). Source lets a caller name the offending file and key
// when the value fails its own validation.
type StructuredValue struct {
	Key    string
	Value  json.RawMessage
	Scope  Scope
	Source string
}

// ListStructured resolves every structured item whose key starts with prefix,
// walking project > repo > global > builtin per key; the first scope holding a
// key supplies its whole value (no field-level merge). Session scope never
// holds structured values. builtin supplies the code-default floor. The result
// is sorted by key. A malformed file fails the whole listing, naming the file.
func (r *Resolver) ListStructured(prefix string, builtin map[string]json.RawMessage) ([]StructuredValue, error) {
	layers, err := r.structuredLayers()
	if err != nil {
		return nil, err
	}
	winners := map[string]StructuredValue{}
	for _, layer := range layers {
		for key, value := range layer.values {
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			if _, taken := winners[key]; taken {
				continue
			}
			winners[key] = StructuredValue{Key: key, Value: value, Scope: layer.scope, Source: layer.source}
		}
	}
	for key, value := range builtin {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		if _, taken := winners[key]; taken {
			continue
		}
		winners[key] = StructuredValue{Key: key, Value: value, Scope: ScopeBuiltin}
	}
	out := make([]StructuredValue, 0, len(winners))
	for _, v := range winners {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

type structuredLayer struct {
	scope  Scope
	source string
	values map[string]json.RawMessage
}

// structuredLayers loads the file scopes in precedence order. A scope whose
// path cannot be resolved (no repo root) contributes nothing.
func (r *Resolver) structuredLayers() ([]structuredLayer, error) {
	var layers []structuredLayer
	projectPath, err := Path(r.opts)
	if err != nil {
		return nil, fmt.Errorf("resolver: resolve project config path: %w", err)
	}
	projectCfg, err := loadProjectConfig(r.opts)
	if err != nil {
		return nil, fmt.Errorf("resolver: load project config %s: %w", projectPath, err)
	}
	layers = append(layers, structuredLayer{ScopeProject, projectPath, projectCfg.StructuredOverrides})

	if repoPath, ok := RepoPath(r.opts); ok {
		repoCfg, err := loadRepoConfig(r.opts)
		if err != nil {
			return nil, fmt.Errorf("resolver: load repo config: %w", err)
		}
		layers = append(layers, structuredLayer{ScopeRepo, repoPath, repoCfg.StructuredOverrides})
	}

	globalPath, err := GlobalPath(r.opts)
	if err != nil {
		return nil, err
	}
	globalCfg, err := loadGlobalConfig(r.opts)
	if err != nil {
		return nil, fmt.Errorf("resolver: load global config %s: %w", globalPath, err)
	}
	layers = append(layers, structuredLayer{ScopeGlobal, globalPath, globalCfg.StructuredOverrides})
	return layers, nil
}

// StructuredScopePath reports the file a structured write to scope lands in.
// Session and builtin hold no structured values.
func (r *Resolver) StructuredScopePath(scope Scope) (string, error) {
	switch scope {
	case ScopeProject:
		return Path(r.opts)
	case ScopeRepo:
		return r.repoWritePath()
	case ScopeGlobal:
		return GlobalPath(r.opts)
	default:
		return "", fmt.Errorf("resolver: structured values have no %q scope; use project, repo, or global", scope)
	}
}

// SetStructured writes value (already validated by the caller) as the whole
// structured value of itemKey in scope.
func (r *Resolver) SetStructured(itemKey string, value json.RawMessage, scope Scope) error {
	path, err := r.StructuredScopePath(scope)
	if err != nil {
		return err
	}
	if scope == ScopeRepo {
		return updateRepoConfigFile(path, func(_ map[string]string, structured map[string]json.RawMessage) error {
			structured[itemKey] = value
			return nil
		})
	}
	_, err = updateConfigFile(path, func(cfg *Config) error {
		if cfg.StructuredOverrides == nil {
			cfg.StructuredOverrides = map[string]json.RawMessage{}
		}
		cfg.StructuredOverrides[itemKey] = value
		return nil
	})
	return err
}

// UnsetStructured removes itemKey's structured value from scope, so the key
// resolves from the next broader scope again. Removing an absent key, or from
// an absent file, is a no-op that writes nothing.
func (r *Resolver) UnsetStructured(itemKey string, scope Scope) error {
	path, err := r.StructuredScopePath(scope)
	if err != nil {
		return err
	}
	if !configFileExists(path) {
		return nil
	}
	if scope == ScopeRepo {
		err = updateRepoConfigFile(path, func(_ map[string]string, structured map[string]json.RawMessage) error {
			if _, exists := structured[itemKey]; !exists {
				return errConfigUnchanged
			}
			delete(structured, itemKey)
			return nil
		})
	} else {
		_, err = updateConfigFile(path, func(cfg *Config) error {
			if _, exists := cfg.StructuredOverrides[itemKey]; !exists {
				return errConfigUnchanged
			}
			delete(cfg.StructuredOverrides, itemKey)
			return nil
		})
	}
	if errors.Is(err, errConfigUnchanged) {
		return nil
	}
	return err
}
