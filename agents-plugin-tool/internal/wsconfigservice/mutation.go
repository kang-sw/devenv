package wsconfigservice

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

type Operation struct {
	Key       string            `json:"key"`
	Selectors map[string]string `json:"selectors"`
	Action    string            `json:"action"`
	Value     any               `json:"value,omitempty"`
}

type Mutation struct {
	Operation
	StoredKey  string
	Scalar     string
	Structured json.RawMessage
}

func (c *Catalog) Validate(scope wsconfig.Scope, op Operation) (Mutation, error) {
	rule, ok := c.rule(op.Key)
	if !ok || strings.HasSuffix(op.Key, ".*") || (op.Action != "set" && op.Action != "reset") ||
		(op.Action == "set" && op.Value == nil) || (op.Action == "reset" && op.Value != nil) {
		return Mutation{}, ErrInvalid
	}
	if scope == wsconfig.ScopeBuiltin || rule.ReadOnly || !rule.scopeMayResolve(scope) ||
		(rule.Structured && scope != wsconfig.ScopeGlobal && scope != wsconfig.ScopeProject) ||
		(rule.Key == "agents.tier" && scope != wsconfig.ScopeGlobal && scope != wsconfig.ScopeProject) {
		return Mutation{}, ErrScope
	}
	harness := op.Selectors["harness"]
	if len(rule.Harnesses) > 0 && !slices.Contains(rule.Harnesses, harness) {
		return Mutation{}, ErrInvalid
	}
	for field := range op.Selectors {
		if (field != "harness" || len(rule.Harnesses) == 0) && (field != "tier" || op.Key != "agents.tier") {
			return Mutation{}, ErrInvalid
		}
	}
	m := Mutation{Operation: op, StoredKey: rule.StoreKey(harness)}
	if rule.Structured {
		if rule.ValidateName(strings.TrimPrefix(op.Key, strings.TrimSuffix(FamilyKey(op.Key), "*"))) != nil {
			return Mutation{}, ErrInvalid
		}
		if op.Action == "set" {
			value, err := rule.NormalizeObject(op.Value, scope)
			if err != nil {
				return Mutation{}, ErrInvalid
			}
			m.Structured = value
		}
		return m, nil
	}
	if op.Key == "agents.tier" {
		tier := op.Selectors["tier"]
		if tier == "" || wsconfig.NormalizedTier(tier) != tier {
			return Mutation{}, ErrInvalid
		}
		if op.Action == "set" && !validTierObject(op.Value, tier) {
			return Mutation{}, ErrInvalid
		}
		return m, nil
	}
	if op.Action == "reset" {
		return m, nil
	}
	value, ok := op.Value.(string)
	if !ok {
		switch v := op.Value.(type) {
		case bool:
			if rule.Type == "boolean" {
				value = "false"
				if v {
					value = "true"
				}
				ok = true
			}
		case json.Number:
			if rule.Type == "integer" {
				value, ok = v.String(), true
			}
		}
	}
	if !ok {
		return Mutation{}, ErrInvalid
	}
	if rule.Normalize != nil {
		var err error
		value, err = rule.Normalize(value)
		if err != nil {
			return Mutation{}, ErrInvalid
		}
	}
	m.Scalar = value
	return m, nil
}

func validTierObject(value any, tier string) bool {
	fields, ok := value.(map[string]any)
	if !ok || len(fields) == 0 {
		return false
	}
	for k, v := range fields {
		s, ok := v.(string)
		if !ok {
			return false
		}
		switch k {
		case "tier":
			if wsconfig.NormalizedTier(s) != tier {
				return false
			}
		case "backend", "model", "effort":
		default:
			return false
		}
	}
	return true
}

func (m Mutation) Apply(scope wsconfig.Scope, opts wsconfig.Options, sessionKey string, writer wsconfig.SessionWriter) error {
	if scope == wsconfig.ScopeRepo || scope == wsconfig.ScopeBuiltin {
		return ErrScope
	}
	if m.Key == "agents.tier" {
		return m.writeTier(scope, opts)
	}
	r := wsconfig.NewResolver(opts, nil, nil, writer)
	if m.Action == "reset" && strings.HasSuffix(FamilyKey(m.Key), ".profile.*") {
		return r.UnsetStructured(m.StoredKey, scope)
	}
	if m.Structured != nil {
		return r.SetStructured(m.StoredKey, m.Structured, scope)
	}
	target := wsconfig.SetOptions{ExplicitScope: scope, SessionKey: sessionKey}
	if m.Action == "reset" {
		return r.Unset(m.StoredKey, target)
	}
	return r.Set(m.StoredKey, m.Scalar, target)
}

func (m Mutation) writeTier(scope wsconfig.Scope, opts wsconfig.Options) error {
	fields := map[string]any{"tier": m.Selectors["tier"]}
	reset := m.Action == "reset"
	if !reset {
		maps.Copy(fields, m.Value.(map[string]any))
	}
	_, _, err := WriteTier(opts, fields, m.Selectors["harness"], scope, reset)
	return err
}

// AfterWrite records the committed scalar when refreshing a layer fails
// after a successful save. The response marks the resolution stale.
func (m Mutation) AfterWrite(scope wsconfig.Scope, layers Layers) Layers {
	if m.Key == "agents.tier" {
		return layers
	}
	file := layers.Files[scope]
	values := file.Overrides
	if scope == wsconfig.ScopeSession {
		values = layers.Session
	}
	if m.Structured != nil || strings.HasSuffix(FamilyKey(m.Key), ".profile.*") {
		if file.StructuredOverrides == nil {
			file.StructuredOverrides = make(map[string]json.RawMessage)
		}
		if m.Action == "reset" {
			delete(file.StructuredOverrides, m.StoredKey)
		} else {
			file.StructuredOverrides[m.StoredKey] = m.Structured
		}
	} else {
		if values == nil {
			values = make(map[string]string)
		}
		if m.Action == "reset" {
			delete(values, m.StoredKey)
		} else {
			values[m.StoredKey] = m.Scalar
		}
		if scope == wsconfig.ScopeSession {
			layers.Session = values
		} else {
			file.Overrides = values
		}
	}
	layers.Files[scope] = file
	return layers
}
