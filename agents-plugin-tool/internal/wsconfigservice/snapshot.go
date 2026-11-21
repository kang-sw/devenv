package wsconfigservice

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

type Layer struct {
	Scope     wsconfig.Scope `json:"scope"`
	Bucket    string         `json:"bucket"`
	Evaluated bool           `json:"evaluated"`
	Status    string         `json:"status"`
	Present   *bool          `json:"present,omitempty"`
	Value     any            `json:"value,omitempty"`
	Selected  any            `json:"selected,omitempty"`
}

type Entry struct {
	Key            string            `json:"key"`
	Selectors      map[string]string `json:"selectors"`
	Default        any               `json:"default"`
	Value          any               `json:"value"`
	SourceScope    wsconfig.Scope    `json:"source_scope"`
	SourceBucket   string            `json:"source_bucket"`
	Conditional    bool              `json:"conditional"`
	Layers         []Layer           `json:"layers"`
	Editable       bool              `json:"editable"`
	DisabledReason string            `json:"disabled_reason"`
	Type           string            `json:"type"`
	Enum           []string          `json:"enum,omitempty"`
	Minimum        *int64            `json:"minimum,omitempty"`
	Maximum        *int64            `json:"maximum,omitempty"`
	Seed           *string           `json:"seed,omitempty"`
}

type Layers struct {
	Files            map[wsconfig.Scope]wsconfig.Config
	Session          map[string]string
	SessionAvailable bool
	Structured       map[string]json.RawMessage
	Seeds            map[string]string
}

var precedence = []wsconfig.Scope{wsconfig.ScopeSession, wsconfig.ScopeProject, wsconfig.ScopeRepo, wsconfig.ScopeGlobal, wsconfig.ScopeBuiltin}

func (c *Catalog) Entries(scope wsconfig.Scope, layers Layers) []Entry {
	keys := make(map[string]bool, len(c.keys))
	for _, key := range c.keys {
		if !strings.HasSuffix(key, ".*") {
			keys[key] = true
		}
	}
	add := func(key string) {
		if _, ok := c.rule(key); ok {
			keys[key] = true
		}
	}
	for _, layer := range layers.Files {
		for key := range layer.StructuredOverrides {
			add(key)
		}
		for key := range layer.Overrides {
			if wsconfig.ValidPreferenceKey(key) {
				add(key)
			}
		}
	}
	for key := range layers.Structured {
		add(key)
	}
	for key := range layers.Session {
		if wsconfig.ValidPreferenceKey(key) {
			add(key)
		}
	}
	keys["preference.default"] = true

	out := make([]Entry, 0, len(keys))
	for _, key := range slices.Sorted(maps.Keys(keys)) {
		rule, _ := c.rule(key)
		switch {
		case key == "agents.tier":
			for _, tier := range []string{"small", "medium", "large", "xlarge"} {
				for _, harness := range rule.Harnesses {
					out = append(out, buildEntry(rule, map[string]string{"tier": tier, "harness": harness}, scope, layers))
				}
			}
		case len(rule.Harnesses) > 0:
			for _, harness := range rule.Harnesses {
				out = append(out, buildEntry(rule, map[string]string{"harness": harness}, scope, layers))
			}
		default:
			out = append(out, buildEntry(rule, map[string]string{}, scope, layers))
		}
	}
	return out
}

func buildEntry(rule Rule, selectors map[string]string, scope wsconfig.Scope, layers Layers) Entry {
	e := Entry{Key: rule.Key, Selectors: selectors, Type: rule.Type, Enum: rule.Enum, Minimum: rule.Minimum, Maximum: rule.Maximum}
	e.DisabledReason = rule.scopeReason(scope)
	e.Editable = e.DisabledReason == ""
	e.SourceScope = wsconfig.ScopeBuiltin
	harness := selectors["harness"]
	if seed, ok := layers.Seeds[rule.Key]; ok {
		e.Seed = &seed
	}
	if rule.Key == "agents.tier" {
		tier := wsconfig.InspectTier(layers.Files, selectors["tier"], harness)
		e.Default, e.Value, e.SourceScope, e.SourceBucket = tier.Default, tier.Value, tier.Scope, tier.Bucket
		for _, s := range precedence {
			leaf, present := tier.Layers[s]
			layer := layerStatus(rule, s, layers)
			if s == wsconfig.ScopeBuiltin {
				leaf, present = tier.Default, true
			}
			if layer.Evaluated {
				layer.set(harness, leaf, present)
				if present && s != wsconfig.ScopeBuiltin {
					layer.Selected = leaf
				}
			}
			e.Layers = append(e.Layers, layer)
		}
		return e
	}

	e.Default = rule.display(rule.Default)
	if rule.Structured {
		e.Default = safeStructured(rule.Key, layers.Structured[rule.Key])
	}
	e.Value = e.Default
	keys, buckets := rule.readKeys(harness)
	for _, s := range precedence {
		layer := layerStatus(rule, s, layers)
		if layer.Evaluated {
			if s == wsconfig.ScopeBuiltin {
				layer.set("", e.Default, e.Default != nil)
			} else {
				v, _, b, ok, selected := layerValue(rule, s, layers, keys, buckets, harness)
				layer.set(b, v, ok)
				layer.Selected = selected
			}
		}
		e.Layers = append(e.Layers, layer)
	}
	// Most scalar keys are scope-major. Harness-bucketed workflow/prompt
	// settings check every scope of the harness bucket before trying all.
	var candidates []struct {
		layer int
		key   int
	}
	if len(rule.Harnesses) > 0 && !rule.Compaction {
		for k := range keys {
			for l := range precedence {
				candidates = append(candidates, struct {
					layer int
					key   int
				}{l, k})
			}
		}
	} else {
		for l := range precedence {
			for k := range keys {
				candidates = append(candidates, struct {
					layer int
					key   int
				}{l, k})
			}
		}
	}
	for _, candidate := range candidates {
		layer := e.Layers[candidate.layer]
		if !layer.Evaluated {
			e.Conditional = e.Conditional || (layer.Status == "context_missing" && rule.scopeMayResolve(layer.Scope))
			continue
		}
		if layer.Scope == wsconfig.ScopeBuiltin {
			continue
		}
		k := candidate.key
		v, _, b, ok, _ := layerValue(rule, layer.Scope, layers, keys[k:k+1], buckets[k:k+1], harness)
		if ok {
			e.Value, e.SourceScope, e.SourceBucket = v, layer.Scope, b
			break
		}
	}
	return e
}

func layerStatus(rule Rule, scope wsconfig.Scope, layers Layers) Layer {
	layer := Layer{Scope: scope, Evaluated: true, Status: "evaluated"}
	switch {
	case scope == wsconfig.ScopeSession && !layers.SessionAvailable:
		layer.Evaluated, layer.Status = false, "context_missing"
	case scope == wsconfig.ScopeRepo:
		if _, evaluated := layers.Files[scope]; !evaluated {
			layer.Evaluated, layer.Status = false, "context_missing"
		}
	}
	if layer.Evaluated && !rule.scopeMayResolve(scope) {
		layer.Evaluated, layer.Status = false, "not_applicable"
	}

	return layer
}

func (l *Layer) set(bucket string, value any, present bool) {
	l.Present = &present
	if present {
		l.Bucket, l.Value = bucket, value
	}
}

func (r Rule) readKeys(harness string) ([]string, []string) {
	keys, buckets := []string{}, []string{}
	if harness != "" && harness != "*" {
		keys, buckets = append(keys, r.Key+"."+harness), append(buckets, harness)
	}
	if harness != "" && !r.Compaction {
		keys, buckets = append(keys, r.Key+".all"), append(buckets, "all")
	}
	keys, buckets = append(keys, r.Key), append(buckets, "")
	if r.LegacyKey != nil {
		if legacy := r.LegacyKey(harness); legacy != "" {
			keys, buckets = append(keys, legacy), append(buckets, legacy)
		}
	}
	return keys, buckets
}

func (r Rule) StoreKey(harness string) string {
	if harness == "" || (r.Compaction && harness == "*") {
		return r.Key
	}
	if harness == "*" {
		harness = "all"
	}
	return r.Key + "." + harness
}

func layerValue(r Rule, s wsconfig.Scope, state Layers, keys, buckets []string, harness string) (any, string, string, bool, any) {
	if r.Structured {
		raw, ok := state.Files[s].StructuredOverrides[r.Key]
		value := safeStructured(r.Key, raw)
		return value, r.Key, "", ok, value
	}
	overrides := state.Files[s].Overrides
	if s == wsconfig.ScopeSession {
		overrides = state.Session
	}
	var selected any
	if v, ok := overrides[r.StoreKey(harness)]; ok {
		selected = r.display(v)
	}
	if (r.GlobalOnly && s != wsconfig.ScopeGlobal) || (s == wsconfig.ScopeBuiltin) {
		return nil, "", "", false, nil
	}
	for i, k := range keys {
		if v, exists := overrides[k]; exists && (v != "" || (len(r.Harnesses) == 0 && !r.Compaction)) {
			return r.display(v), k, buckets[i], true, selected
		}
	}
	return nil, "", "", false, selected
}

func (r Rule) display(value string) any {
	if r.Decode != nil {
		if typed, err := r.Decode(value); err == nil {
			return typed
		}
	}
	return value
}

// Entry projects the same view as Entries for a key and its validated selectors.
func (c *Catalog) Entry(key string, selectors map[string]string, scope wsconfig.Scope, layers Layers) Entry {
	rule, _ := c.rule(key)
	if selectors == nil {
		selectors = map[string]string{}
	}
	return buildEntry(rule, selectors, scope, layers)
}
