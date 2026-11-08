package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
)

// repoWriteNote is the line a config.tune result carries after it wrote or
// reset the committed repo-scope file: config.tune edits the tracked file and
// never stages it, following note.write's repo layer, so the caller's next
// git.commit must name the path. Any other scope adds nothing.
func repoWriteNote(resolver *wsconfig.Resolver, scope wsconfig.Scope) string {
	if scope != wsconfig.ScopeRepo {
		return ""
	}
	path, err := resolver.RepoWritePath()
	if err != nil {
		return ""
	}
	return fmt.Sprintf("written: %s (not staged; include this path in your next git.commit paths)\n", path)
}

// structuredFamily is one structured (JSON-object) config key family
// config.tune writes through tuneStructured: the key prefix, its name rule,
// the value validator, and how the family resolves, so the write path stays
// one function however many families exist.
type structuredFamily struct {
	prefix string
	// label names the family in resolution warnings ("worktree profiles").
	label        string
	validateName func(string) error
	// parse strictly decodes and validates one value; the decoded result is
	// what gets stored, so the committed file has one canonical field order.
	parse func(json.RawMessage) (any, error)
	// effective resolves the family through the resolver and reports the
	// entry named name; found is false when no scope defines it.
	effective func(resolver *wsconfig.Resolver, name string) (scope wsconfig.Scope, source string, found bool, err error)
}

var worktreeProfileFamily = structuredFamily{
	prefix:       worktreeProfilePrefix,
	label:        "worktree profiles",
	validateName: validateWorktreeProfileName,
	parse: func(raw json.RawMessage) (any, error) {
		return parseWorktreeProfile(raw)
	},
	effective: func(resolver *wsconfig.Resolver, name string) (wsconfig.Scope, string, bool, error) {
		profiles, err := resolveWorktreeProfiles(resolver)
		if err != nil {
			return "", "", false, err
		}
		p, found := findWorktreeProfile(profiles, name)
		return p.Scope, p.Source, found, nil
	},
}

var agentProfileFamily = structuredFamily{
	prefix:       wsagent.ProfilePrefix,
	label:        "agent profiles",
	validateName: wsagent.ValidateProfileName,
	parse: func(raw json.RawMessage) (any, error) {
		return wsagent.ParseProfile(raw)
	},
	effective: func(resolver *wsconfig.Resolver, name string) (wsconfig.Scope, string, bool, error) {
		profiles, err := wsagent.ResolveProfiles(resolver)
		if err != nil {
			return "", "", false, err
		}
		for _, p := range profiles {
			if p.Name == name {
				return p.Scope, p.Source, true, nil
			}
		}
		return "", "", false, nil
	},
}

var structuredFamilies = []structuredFamily{worktreeProfileFamily, agentProfileFamily}

// structuredFamilyForKey picks the family a structured key belongs to by its
// prefix.
func structuredFamilyForKey(key string) (structuredFamily, bool) {
	for _, family := range structuredFamilies {
		if strings.HasPrefix(key, family.prefix) {
			return family, true
		}
	}
	return structuredFamily{}, false
}

// tuneStructured is config.tune for a structured (JSON-object) key: the
// worktree.profile.<name> and agent.profile.<name> families. The value is
// validated before any write, session scope is refused, and a reset removes
// the whole value from the selected scope.
func (s *Server) tuneStructured(entry configKeyEntry, rawValue any, sessionKey string, explicitScope wsconfig.Scope, reset bool) (string, error) {
	family, ok := structuredFamilyForKey(entry.Key)
	if !ok {
		return "", fmt.Errorf("config.tune: %s is not a structured config key", entry.Key)
	}
	name := strings.TrimPrefix(entry.Key, family.prefix)
	if err := family.validateName(name); err != nil {
		return "", fmt.Errorf("config.tune: %w", err)
	}
	scope := explicitScope
	if scope == "" {
		scope = wsconfig.ScopeProject
	}
	scopes := fieldEnum(entry.SelectorFields, "scope")
	if !enumContains(scopes, string(scope)) {
		return "", fmt.Errorf("config.tune: %s is never %s-scoped; use one of %s", entry.Key, scope, strings.Join(scopes, ", "))
	}
	resolver := s.sessionResolver(sessionKey, nil)
	path, err := resolver.StructuredScopePath(scope)
	if err != nil {
		return "", fmt.Errorf("config.tune: %w", err)
	}

	var text string
	if reset {
		if rawValue != nil {
			return "", fmt.Errorf("config.tune: value and reset are mutually exclusive; %s reset takes no value", entry.Key)
		}
		if err := resolver.UnsetStructured(entry.Key, scope); err != nil {
			return "", fmt.Errorf("config.tune: %w", err)
		}
		text = fmt.Sprintf("%s: reset [scope:%s] in %s\n", entry.Key, scope, path)
	} else {
		object, ok := rawValue.(map[string]any)
		if !ok {
			if rawValue == nil {
				return "", fmt.Errorf("config.tune: %s value is required unless reset is true", entry.Key)
			}
			return "", fmt.Errorf("config.tune: %s value must be a profile object (see value_schema in config.list), got %s", entry.Key, jsonValueTypeName(rawValue))
		}
		encoded, err := json.Marshal(object)
		if err != nil {
			return "", fmt.Errorf("config.tune: encode %s value: %w", entry.Key, err)
		}
		profile, err := family.parse(encoded)
		if err != nil {
			return "", fmt.Errorf("config.tune: %s %w", entry.Key, err)
		}
		// Store the decoded profile, not the caller's bytes: one canonical field
		// order in the committed file however the caller spelled the object.
		stored, err := json.Marshal(profile)
		if err != nil {
			return "", fmt.Errorf("config.tune: encode %s value: %w", entry.Key, err)
		}
		if err := resolver.SetStructured(entry.Key, stored, scope); err != nil {
			return "", fmt.Errorf("config.tune: %w", err)
		}
		text = fmt.Sprintf("%s: %s [scope:%s] in %s\n", entry.Key, stored, scope, path)
	}
	text += repoWriteNote(&resolver, scope)

	// Report the profile that now applies. The write already happened, so a
	// resolution failure (another invalid profile) is reported beside it.
	effectiveScope, source, found, err := family.effective(&resolver, name)
	switch {
	case err != nil:
		text += fmt.Sprintf("warning: %s unresolvable: %v\n", family.label, err)
	case !found:
		text += fmt.Sprintf("effective: %s is not defined in any scope\n", entry.Key)
	case reset:
		text += fmt.Sprintf("effective: %s [scope:%s]%s\n", entry.Key, effectiveScope, sourceSuffix(source))
	case effectiveScope != scope:
		// A write below a set scope does not take effect; say which does.
		text += fmt.Sprintf("shadowed: effective %s [scope:%s]%s\n", entry.Key, effectiveScope, sourceSuffix(source))
	}
	return text, nil
}

func sourceSuffix(source string) string {
	if source == "" {
		return ""
	}
	return " in " + source
}
