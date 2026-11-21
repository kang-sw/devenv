package wsconfigservice

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

var (
	ErrInvalid = errors.New("invalid configuration request")
	ErrScope   = errors.New("unsupported configuration scope")
)

type Rule struct {
	Key, Type        string
	Default          string
	Enum             []string
	Minimum, Maximum *int64
	Harnesses        []string
	GlobalOnly       bool
	NoSession        bool
	NoRepo           bool
	ReadOnly         bool
	Structured       bool
	Compaction       bool
	Preference       bool
	Normalize        func(string) (string, error)
	Decode           func(string) (any, error)
	ValidateName     func(string) error
	NormalizeObject  func(any, wsconfig.Scope) (json.RawMessage, error)
	LegacyKey        func(harness string) string
}

type Catalog struct {
	rules map[string]Rule
	keys  []string
}

func NewCatalog(rules []Rule) (*Catalog, error) {
	c := &Catalog{rules: make(map[string]Rule, len(rules))}
	for _, rule := range rules {
		if rule.Key == "" {
			return nil, errors.New("configuration key is empty")
		}
		if _, exists := c.rules[rule.Key]; exists {
			return nil, fmt.Errorf("duplicate configuration key %q", rule.Key)
		}
		c.rules[rule.Key] = rule
	}
	c.keys = slices.Sorted(maps.Keys(c.rules))
	return c, nil
}

func (c *Catalog) Keys() []string {
	return slices.Clone(c.keys)
}

func (c *Catalog) rule(key string) (Rule, bool) {
	if rule, ok := c.rules[key]; ok {
		return rule, true
	}
	switch {
	case wsconfig.ValidPreferenceKey(key):
		rule, ok := c.rules["preference.*"]
		rule.Key = key
		return rule, ok
	case strings.HasPrefix(key, "worktree.profile."), strings.HasPrefix(key, "agent.profile."):
		family := FamilyKey(key)
		rule, ok := c.rules[family]
		rule.Key = key
		return rule, ok
	default:
		return Rule{}, false
	}
}

func FamilyKey(key string) string {
	for _, prefix := range []string{"preference.", "prompt.", "worktree.profile.", "agent.profile."} {
		if strings.HasPrefix(key, prefix) {
			return prefix + "*"
		}
	}
	return key
}

// Compare checks the two directions independently so extra UI rows are caught
// even when all the live settings have a row.
func (c *Catalog) Compare(uiKeys []string) error {
	want := make(map[string]bool)
	for _, key := range c.keys {
		want[FamilyKey(key)] = true
	}
	seen := make(map[string]bool)
	for _, key := range uiKeys {
		if seen[key] {
			return fmt.Errorf("duplicate settings UI key %q", key)
		}
		seen[key] = true
		if !want[key] {
			return fmt.Errorf("extra settings UI key %q", key)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(want)) {
		if !seen[key] {
			return fmt.Errorf("missing settings UI key %q", key)
		}
	}
	return nil
}

func (r Rule) scopeReason(scope wsconfig.Scope) string {
	switch {
	case scope == wsconfig.ScopeBuiltin:
		return "builtin_read_only"
	case r.Structured:
		return "profile_read_only"
	case r.ReadOnly:
		return "setting_read_only"
	case r.GlobalOnly && scope != wsconfig.ScopeGlobal:
		return "global_only"
	case r.Key == "agents.tier" && scope != wsconfig.ScopeGlobal && scope != wsconfig.ScopeProject:
		return "tier_scope"
	case !r.scopeMayResolve(scope):
		return "scope_unsupported"
	default:
		return ""
	}
}

func (r Rule) scopeMayResolve(scope wsconfig.Scope) bool {
	switch scope {
	case wsconfig.ScopeSession:
		return !r.GlobalOnly && !r.NoSession && !r.Structured && r.Key != "agents.tier"
	case wsconfig.ScopeRepo:
		return !r.GlobalOnly && !r.NoRepo && r.Key != "agents.tier"
	case wsconfig.ScopeProject:
		return !r.GlobalOnly
	default:
		return true
	}
}
