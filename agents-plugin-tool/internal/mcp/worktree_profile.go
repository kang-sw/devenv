package mcp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

// worktreeProfilePrefix is the config key family one worktree profile lives
// under: worktree.profile.<name>. A profile is a pool-partition key (a pooled
// worktree is reused only by an acquire naming the same profile) plus the
// lifecycle definition below.
const worktreeProfilePrefix = "worktree.profile."

var worktreeProfileNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Builtin profile names. full is a plain checkout; housekeeping is the
// ticket-board checkout a lead uses for ticket and doc work off an occupied
// primary worktree.
const (
	worktreeProfileFull         = "full"
	worktreeProfileHousekeeping = "housekeeping"
)

// worktreeProfile is the normative profile shape. Field names are a committed
// on-disk format (structured_overrides in a config file); do not rename them.
type worktreeProfile struct {
	Description string                 `json:"description,omitempty"`
	Sparse      *worktreeProfileSparse `json:"sparse,omitempty"`
	Preserve    []string               `json:"preserve,omitempty"`
	Hooks       *worktreeProfileHooks  `json:"hooks,omitempty"`
}

// worktreeProfileSparse declares the sparse-checkout a worktree gets when it
// enters the profile. mode "cone" takes directory paths; mode "patterns" is
// non-cone sparse-checkout, so an exclusion is declarable without a shell.
type worktreeProfileSparse struct {
	Mode  string   `json:"mode"`
	Paths []string `json:"paths"`
}

type worktreeProfileHooks struct {
	PostProvision []worktreeProfileHook `json:"post_provision,omitempty"`
	PostAcquire   []worktreeProfileHook `json:"post_acquire,omitempty"`
	PreRelease    []worktreeProfileHook `json:"pre_release,omitempty"`
	PreRemove     []worktreeProfileHook `json:"pre_remove,omitempty"`
}

// worktreeProfileHook is one hook command: an argv array (never a shell
// string) run in the worktree, or in Cwd relative to it when set.
type worktreeProfileHook struct {
	Argv []string `json:"argv"`
	Cwd  *string  `json:"cwd,omitempty"`
}

const (
	worktreeSparseModeCone     = "cone"
	worktreeSparseModePatterns = "patterns"
)

// worktreeProfileValueSchema is the JSON Schema config.list publishes for the
// family's value. parseWorktreeProfile enforces the same rules; keep the two in
// step.
func worktreeProfileValueSchema() map[string]any {
	hookList := map[string]any{
		"type": "array",
		"items": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"argv"},
			"properties": map[string]any{
				"argv": map[string]any{
					"type":        "array",
					"minItems":    1,
					"items":       map[string]any{"type": "string", "minLength": 1},
					"description": "Command and arguments, run without a shell. Values arrive as env vars WS_WORKTREE_DIR, WS_BASE, WS_TARGET_BRANCH.",
				},
				"cwd": map[string]any{
					"type":        []string{"string", "null"},
					"description": "Working directory relative to the worktree; null or absent runs in the worktree root.",
				},
			},
		},
	}
	hookRef := map[string]any{"$ref": "#/$defs/hook_list"}
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$defs":                map[string]any{"hook_list": hookList},
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"description": map[string]any{"type": "string"},
			"sparse": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"mode", "paths"},
				"description":          "Sparse-checkout applied when a worktree enters the profile. Absent means a full checkout.",
				"properties": map[string]any{
					"mode":  map[string]any{"enum": []string{worktreeSparseModeCone, worktreeSparseModePatterns}},
					"paths": map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string", "minLength": 1}},
				},
			},
			"preserve": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string", "minLength": 1},
				"description": "Ignored-path patterns the hygiene git clean keeps (git clean -e <pattern>) at acquire and release.",
			},
			"hooks": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"post_provision": hookRef,
					"post_acquire":   hookRef,
					"pre_release":    hookRef,
					"pre_remove":     hookRef,
				},
			},
		},
	}
}

// parseWorktreeProfile strictly decodes and validates one profile value.
func parseWorktreeProfile(raw json.RawMessage) (worktreeProfile, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return worktreeProfile{}, fmt.Errorf("value must be a JSON object")
	}
	var p worktreeProfile
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return worktreeProfile{}, fmt.Errorf("value does not match the profile schema: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return worktreeProfile{}, fmt.Errorf("value must be a single JSON object")
	}
	if p.Sparse != nil {
		switch p.Sparse.Mode {
		case worktreeSparseModeCone, worktreeSparseModePatterns:
		default:
			return worktreeProfile{}, fmt.Errorf("sparse.mode must be %q or %q; got %q", worktreeSparseModeCone, worktreeSparseModePatterns, p.Sparse.Mode)
		}
		if len(p.Sparse.Paths) == 0 {
			return worktreeProfile{}, fmt.Errorf("sparse.paths must list at least one path")
		}
		for i, path := range p.Sparse.Paths {
			if strings.TrimSpace(path) == "" {
				return worktreeProfile{}, fmt.Errorf("sparse.paths[%d] must be non-empty", i)
			}
		}
	}
	for i, pattern := range p.Preserve {
		if strings.TrimSpace(pattern) == "" {
			return worktreeProfile{}, fmt.Errorf("preserve[%d] must be non-empty", i)
		}
	}
	if p.Hooks != nil {
		for _, point := range []struct {
			name  string
			hooks []worktreeProfileHook
		}{
			{"post_provision", p.Hooks.PostProvision},
			{"post_acquire", p.Hooks.PostAcquire},
			{"pre_release", p.Hooks.PreRelease},
			{"pre_remove", p.Hooks.PreRemove},
		} {
			for i, hook := range point.hooks {
				if len(hook.Argv) == 0 || strings.TrimSpace(hook.Argv[0]) == "" {
					return worktreeProfile{}, fmt.Errorf("hooks.%s[%d].argv must be a non-empty array whose first element names the command", point.name, i)
				}
				for j, arg := range hook.Argv {
					if arg == "" {
						return worktreeProfile{}, fmt.Errorf("hooks.%s[%d].argv[%d] must be non-empty", point.name, i, j)
					}
				}
				if hook.Cwd != nil && !filepath.IsLocal(filepath.FromSlash(*hook.Cwd)) {
					return worktreeProfile{}, fmt.Errorf("hooks.%s[%d].cwd must be a relative path inside the worktree; got %q", point.name, i, *hook.Cwd)
				}
			}
		}
	}
	return p, nil
}

// validateWorktreeProfileName enforces the family's name rule.
func validateWorktreeProfileName(name string) error {
	if !worktreeProfileNamePattern.MatchString(name) {
		return fmt.Errorf("worktree profile name %q must match %s", name, worktreeProfileNamePattern.String())
	}
	return nil
}

// provisionHash fingerprints the profile's provisioning inputs: sparse and
// hooks.post_provision only, so editing description, preserve, or another hook
// never re-provisions the pool. A pooled worktree whose marker hash differs is
// re-adopted (declared sparse re-applied, post_provision re-run).
func (p worktreeProfile) provisionHash() string {
	inputs := struct {
		Sparse        *worktreeProfileSparse `json:"sparse"`
		PostProvision []worktreeProfileHook  `json:"post_provision"`
	}{Sparse: p.Sparse}
	if p.Hooks != nil {
		inputs.PostProvision = p.Hooks.PostProvision
	}
	raw, _ := json.Marshal(inputs)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// builtinWorktreeProfiles is the builtin floor of the family. A higher scope
// may replace either by name.
func builtinWorktreeProfiles() map[string]json.RawMessage {
	return map[string]json.RawMessage{
		worktreeProfilePrefix + worktreeProfileFull:         json.RawMessage(`{"description":"Full checkout of the base."}`),
		worktreeProfilePrefix + worktreeProfileHousekeeping: json.RawMessage(`{"description":"Ticket and doc housekeeping: a cone sparse checkout of ai-docs only.","sparse":{"mode":"cone","paths":["ai-docs"]}}`),
	}
}

// resolvedWorktreeProfile is one profile after layered resolution: the
// winning scope's whole value and the file it came from (empty for builtin).
type resolvedWorktreeProfile struct {
	Name       string          `json:"name"`
	Scope      wsconfig.Scope  `json:"scope"`
	Source     string          `json:"source,omitempty"`
	Definition worktreeProfile `json:"definition"`
}

// resolveWorktreeProfiles resolves the whole family through resolver. Any
// invalid entry fails the resolution, naming the file and key, so a bad
// committed profile stops acquire and the manual loudly instead of being
// skipped.
func resolveWorktreeProfiles(resolver *wsconfig.Resolver) ([]resolvedWorktreeProfile, error) {
	values, err := resolver.ListStructured(worktreeProfilePrefix, builtinWorktreeProfiles())
	if err != nil {
		return nil, err
	}
	out := make([]resolvedWorktreeProfile, 0, len(values))
	for _, v := range values {
		name := strings.TrimPrefix(v.Key, worktreeProfilePrefix)
		where := string(v.Scope) + " scope"
		if v.Source != "" {
			where = fmt.Sprintf("%s (%s scope)", v.Source, v.Scope)
		}
		if err := validateWorktreeProfileName(name); err != nil {
			return nil, fmt.Errorf("invalid worktree profile key %q in %s: %w", v.Key, where, err)
		}
		profile, err := parseWorktreeProfile(v.Value)
		if err != nil {
			return nil, fmt.Errorf("invalid worktree profile key %q in %s: %w", v.Key, where, err)
		}
		out = append(out, resolvedWorktreeProfile{Name: name, Scope: v.Scope, Source: v.Source, Definition: profile})
	}
	return out, nil
}

// findWorktreeProfile picks name out of a resolved list.
func findWorktreeProfile(profiles []resolvedWorktreeProfile, name string) (resolvedWorktreeProfile, bool) {
	for _, p := range profiles {
		if p.Name == name {
			return p, true
		}
	}
	return resolvedWorktreeProfile{}, false
}

// worktreeProfileNames lists the resolved names for an error message.
func worktreeProfileNames(profiles []resolvedWorktreeProfile) string {
	names := make([]string, 0, len(profiles))
	for _, p := range profiles {
		names = append(names, p.Name)
	}
	return strings.Join(names, ", ")
}

// worktreeProfileKnobEntry is the config registry template for one
// worktree.profile.<name> key, generated per key like promptKnobEntry. Its
// value is a structured object, so config.tune routes it to the resolver's
// structured writer; session scope is refused.
func worktreeProfileKnobEntry(name string) configKeyEntry {
	return configKeyEntry{
		Key:        worktreeProfilePrefix + name,
		WriterTool: "config.tune",
		ResetTool:  "config.tune",
		SelectorFields: []tuningField{{
			Name:        "scope",
			Description: "Storage scope: project (machine-local, the default), repo (the committed .ws-workflow/config.json; include it in your next git.commit), or global. Never session.",
			Enum:        worktreeProfileScopeEnum,
		}},
		ValueFields: []tuningField{{
			Name:        "value",
			Description: "The whole profile object (see value_schema); it replaces any lower-scope definition of the same name. Omit when reset is true.",
			Required:    true,
		}},
		NoAgentVisible: true,
		ResolverBacked: true,
		Structured:     true,
	}
}

var worktreeProfileScopeEnum = []string{string(wsconfig.ScopeProject), string(wsconfig.ScopeRepo), string(wsconfig.ScopeGlobal)}

// worktreeProfileManualHeading heads the resolved profile list the workflow
// manual's Git guidance points at ("the resolved list below").
const worktreeProfileManualHeading = "#### Worktree profiles"

// worktreeProfileManualBlock renders the resolved profile list for the
// workflow manual: name, scope it came from, and description.
func worktreeProfileManualBlock(profiles []resolvedWorktreeProfile) string {
	var b strings.Builder
	b.WriteString(worktreeProfileManualHeading + "\n\n")
	for _, p := range profiles {
		fmt.Fprintf(&b, "- `%s` [%s]", p.Name, p.Scope)
		if d := strings.TrimSpace(p.Definition.Description); d != "" {
			b.WriteString(": " + d)
		}
		b.WriteString("\n")
	}
	return b.String()
}
