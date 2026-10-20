package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

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

// tuneStructured is config.tune for a structured (JSON-object) key: today the
// worktree.profile.<name> family. The value is validated before any write,
// session scope is refused, and a reset removes the whole value from the
// selected scope.
func (s *Server) tuneStructured(entry configKeyEntry, rawValue any, sessionKey string, explicitScope wsconfig.Scope, reset bool) (string, error) {
	name := strings.TrimPrefix(entry.Key, worktreeProfilePrefix)
	if err := validateWorktreeProfileName(name); err != nil {
		return "", fmt.Errorf("config.tune: %w", err)
	}
	scope := explicitScope
	if scope == "" {
		scope = wsconfig.ScopeProject
	}
	if !enumContains(worktreeProfileScopeEnum, string(scope)) {
		return "", fmt.Errorf("config.tune: %s is never %s-scoped; use one of %s", entry.Key, scope, strings.Join(worktreeProfileScopeEnum, ", "))
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
		profile, err := parseWorktreeProfile(encoded)
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
	profiles, err := resolveWorktreeProfiles(&resolver)
	switch {
	case err != nil:
		text += fmt.Sprintf("warning: worktree profiles unresolvable: %v\n", err)
	default:
		effective, found := findWorktreeProfile(profiles, name)
		switch {
		case !found:
			text += fmt.Sprintf("effective: %s is not defined in any scope\n", entry.Key)
		case reset:
			text += fmt.Sprintf("effective: %s [scope:%s]%s\n", entry.Key, effective.Scope, sourceSuffix(effective.Source))
		case effective.Scope != scope:
			// A write below a set scope does not take effect; say which does.
			text += fmt.Sprintf("shadowed: effective %s [scope:%s]%s\n", entry.Key, effective.Scope, sourceSuffix(effective.Source))
		}
	}
	return text, nil
}

func sourceSuffix(source string) string {
	if source == "" {
		return ""
	}
	return " in " + source
}
