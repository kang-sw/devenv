package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

// envConfigManifests lists adapter key manifest paths, separated by the
// platform path-list separator (":" on POSIX, ";" on Windows). An adapter's
// launcher sets it to its own package's manifest so the ws-mcp instance it
// starts can validate, catalog, and read that adapter's keys. Only an instance
// launched with the variable knows those keys; that scoping is intended.
const envConfigManifests = "WS_MCP_CONFIG_MANIFESTS"

// configManifestSchemaVersion is the only manifest schema_version accepted.
const configManifestSchemaVersion = 1

// Declared key types. Values are stored as strings in every config scope; the
// type governs config.tune validation and config.get's typed read.
const (
	declaredTypeInteger = "integer"
	declaredTypeBoolean = "boolean"
	declaredTypeString  = "string"
	declaredTypeEnum    = "enum"
)

var (
	manifestNamespacePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*\.$`)
	manifestKeySuffixPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]*$`)
	// reservedConfigPrefixes are built-in key families a manifest namespace
	// may not claim: prompt.<pointId> overrides and the agents.tier writer.
	reservedConfigPrefixes = []string{"prompt.", "agents."}
)

// configManifestFile is the on-disk adapter key manifest:
//
//	{"schema_version": 1, "namespace": "example.", "keys": [
//	  {"key": "example.limit", "type": "integer", "minimum": 0, "maximum": 10,
//	   "default": 3, "default_scope": "project", "description": "..."}]}
//
// Unknown fields are rejected so a misspelled constraint never loads silently.
type configManifestFile struct {
	SchemaVersion int                 `json:"schema_version"`
	Namespace     string              `json:"namespace"`
	Keys          []configManifestKey `json:"keys"`
}

type configManifestKey struct {
	Key          string          `json:"key"`
	Type         string          `json:"type"`
	Minimum      *int64          `json:"minimum,omitempty"`
	Maximum      *int64          `json:"maximum,omitempty"`
	Enum         []string        `json:"enum,omitempty"`
	Default      json.RawMessage `json:"default"`
	DefaultScope string          `json:"default_scope,omitempty"`
	Description  string          `json:"description"`
}

// declaredConfigKey is one validated manifest key.
type declaredConfigKey struct {
	Key          string
	Type         string
	Minimum      *int64
	Maximum      *int64
	Enum         []string
	Description  string
	DefaultScope wsconfig.Scope
	// Default is the canonical stored-string form of the declared default;
	// DefaultValue is the same value typed for config.get.
	Default      string
	DefaultValue any
	// Manifest is the path of the manifest that declared the key.
	Manifest string
}

// declaredConfig is the set of adapter-declared keys this server loaded at
// startup, plus one message per rejected manifest.
type declaredConfig struct {
	keys     map[string]declaredConfigKey
	rejected []string
}

// loadDeclaredConfig loads every manifest named in listValue (the
// envConfigManifests value). A manifest that fails validation is rejected as
// a whole and reported in rejected; it never fails the server. Manifests load
// in list order, so on a namespace collision the earlier manifest keeps it.
func loadDeclaredConfig(listValue string) declaredConfig {
	out := declaredConfig{keys: map[string]declaredConfigKey{}}
	builtinKeys := builtinConfigKeySet()
	namespaces := map[string]string{}
	for _, path := range filepath.SplitList(listValue) {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		namespace, keys, err := parseConfigManifest(path, builtinKeys, namespaces)
		if err != nil {
			out.rejected = append(out.rejected, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		namespaces[namespace] = path
		for _, k := range keys {
			out.keys[k.Key] = k
		}
	}
	return out
}

// builtinConfigKeySet is every key ws-mcp itself owns: the tuning registry,
// the builtin defaults, and every wsconfig item with a scope declaration.
func builtinConfigKeySet() map[string]struct{} {
	set := map[string]struct{}{}
	for _, entry := range configRegistry {
		set[entry.Key] = struct{}{}
	}
	for key := range builtinConfigDefaults() {
		set[key] = struct{}{}
	}
	for _, key := range wsconfig.RegisteredItemKeys() {
		set[key] = struct{}{}
	}
	return set
}

// builtinConfigKey reports whether ws-mcp itself owns key, including the
// dynamic prompt.* family.
func builtinConfigKey(key string) bool {
	if _, ok := builtinConfigKeySet()[key]; ok {
		return true
	}
	_, ok := resolveConfigEntryForKey(key)
	return ok
}

func parseConfigManifest(path string, builtinKeys map[string]struct{}, namespaces map[string]string) (string, []declaredConfigKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("read manifest: %w", err)
	}
	var file configManifestFile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return "", nil, fmt.Errorf("parse manifest: %w", err)
	}
	if file.SchemaVersion != configManifestSchemaVersion {
		return "", nil, fmt.Errorf("schema_version must be %d; got %d", configManifestSchemaVersion, file.SchemaVersion)
	}
	namespace := file.Namespace
	if !manifestNamespacePattern.MatchString(namespace) {
		return "", nil, fmt.Errorf("namespace %q must be one lowercase segment ending in a dot, e.g. \"example.\"", namespace)
	}
	for _, reserved := range reservedConfigPrefixes {
		if namespace == reserved {
			return "", nil, fmt.Errorf("namespace %q is reserved for built-in keys", namespace)
		}
	}
	for key := range builtinKeys {
		if strings.HasPrefix(key, namespace) {
			return "", nil, fmt.Errorf("namespace %q collides with built-in key %q", namespace, key)
		}
	}
	if owner, taken := namespaces[namespace]; taken {
		return "", nil, fmt.Errorf("namespace %q is already declared by %s", namespace, owner)
	}
	if len(file.Keys) == 0 {
		return "", nil, fmt.Errorf("manifest declares no keys")
	}
	seen := map[string]struct{}{}
	keys := make([]declaredConfigKey, 0, len(file.Keys))
	for _, raw := range file.Keys {
		key, err := validateManifestKey(raw, namespace)
		if err != nil {
			return "", nil, err
		}
		if _, dup := seen[key.Key]; dup {
			return "", nil, fmt.Errorf("key %q is declared twice", key.Key)
		}
		seen[key.Key] = struct{}{}
		key.Manifest = path
		keys = append(keys, key)
	}
	return namespace, keys, nil
}

func validateManifestKey(raw configManifestKey, namespace string) (declaredConfigKey, error) {
	if !strings.HasPrefix(raw.Key, namespace) || !manifestKeySuffixPattern.MatchString(strings.TrimPrefix(raw.Key, namespace)) {
		return declaredConfigKey{}, fmt.Errorf("key %q must be %q followed by lowercase letters, digits, '_', '.', or '-'", raw.Key, namespace)
	}
	key := declaredConfigKey{Key: raw.Key, Type: raw.Type, Description: strings.TrimSpace(raw.Description)}
	if key.Description == "" {
		return declaredConfigKey{}, fmt.Errorf("key %q: description is required", raw.Key)
	}
	switch raw.Type {
	case declaredTypeInteger:
		if len(raw.Enum) > 0 {
			return declaredConfigKey{}, fmt.Errorf("key %q: enum applies only to type enum", raw.Key)
		}
		if raw.Minimum != nil && raw.Maximum != nil && *raw.Minimum > *raw.Maximum {
			return declaredConfigKey{}, fmt.Errorf("key %q: minimum %d exceeds maximum %d", raw.Key, *raw.Minimum, *raw.Maximum)
		}
		key.Minimum, key.Maximum = raw.Minimum, raw.Maximum
	case declaredTypeEnum:
		if raw.Minimum != nil || raw.Maximum != nil {
			return declaredConfigKey{}, fmt.Errorf("key %q: minimum and maximum apply only to type integer", raw.Key)
		}
		if len(raw.Enum) == 0 {
			return declaredConfigKey{}, fmt.Errorf("key %q: type enum requires a non-empty enum list", raw.Key)
		}
		members := map[string]struct{}{}
		for _, member := range raw.Enum {
			if _, dup := members[member]; dup || strings.TrimSpace(member) != member || member == "" {
				return declaredConfigKey{}, fmt.Errorf("key %q: enum members must be distinct, non-empty, and free of surrounding whitespace; got %q", raw.Key, member)
			}
			members[member] = struct{}{}
		}
		key.Enum = append([]string(nil), raw.Enum...)
	case declaredTypeBoolean, declaredTypeString:
		if raw.Minimum != nil || raw.Maximum != nil || len(raw.Enum) > 0 {
			return declaredConfigKey{}, fmt.Errorf("key %q: type %s takes no minimum, maximum, or enum", raw.Key, raw.Type)
		}
	default:
		return declaredConfigKey{}, fmt.Errorf("key %q: type must be integer, boolean, string, or enum; got %q", raw.Key, raw.Type)
	}
	switch raw.DefaultScope {
	case "":
		key.DefaultScope = wsconfig.ScopeProject
	case string(wsconfig.ScopeSession), string(wsconfig.ScopeProject), string(wsconfig.ScopeGlobal):
		key.DefaultScope = wsconfig.Scope(raw.DefaultScope)
	default:
		return declaredConfigKey{}, fmt.Errorf("key %q: default_scope must be one of %s; got %q", raw.Key, strings.Join(wsconfig.ScopeSchemaEnum(), ", "), raw.DefaultScope)
	}
	stored, err := manifestDefaultString(key, raw.Default)
	if err != nil {
		return declaredConfigKey{}, fmt.Errorf("key %q: default %w", raw.Key, err)
	}
	typed, err := key.parse(stored)
	if err != nil {
		return declaredConfigKey{}, fmt.Errorf("key %q: default %w", raw.Key, err)
	}
	key.Default, key.DefaultValue = stored, typed
	return key, nil
}

// manifestDefaultString converts the manifest's JSON default into the stored
// string form, requiring the JSON type that matches the declared type.
func manifestDefaultString(key declaredConfigKey, raw json.RawMessage) (string, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return "", fmt.Errorf("is required")
	}
	switch key.Type {
	case declaredTypeInteger:
		var n json.Number
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&n); err != nil {
			return "", fmt.Errorf("must be a JSON integer")
		}
		return n.String(), nil
	case declaredTypeBoolean:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return "", fmt.Errorf("must be a JSON boolean")
		}
		return strconv.FormatBool(b), nil
	default:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", fmt.Errorf("must be a JSON string")
		}
		return s, nil
	}
}

// parse converts a stored string to the key's typed value, or reports why the
// string is not a valid value of the declared type.
func (k declaredConfigKey) parse(stored string) (any, error) {
	switch k.Type {
	case declaredTypeInteger:
		n, err := strconv.ParseInt(strings.TrimSpace(stored), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("must be %s; got %q", k.domain(), stored)
		}
		if (k.Minimum != nil && n < *k.Minimum) || (k.Maximum != nil && n > *k.Maximum) {
			return nil, fmt.Errorf("must be %s; got %d", k.domain(), n)
		}
		return n, nil
	case declaredTypeBoolean:
		switch strings.ToLower(strings.TrimSpace(stored)) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
		return nil, fmt.Errorf("must be %s; got %q", k.domain(), stored)
	case declaredTypeEnum:
		value := strings.TrimSpace(stored)
		if !enumContains(k.Enum, value) {
			return nil, fmt.Errorf("must be %s; got %q", k.domain(), stored)
		}
		return value, nil
	default:
		return stored, nil
	}
}

// normalize validates a config.tune value and returns the canonical string
// to store.
func (k declaredConfigKey) normalize(value string) (string, error) {
	typed, err := k.parse(value)
	if err != nil {
		return "", err
	}
	switch v := typed.(type) {
	case int64:
		return strconv.FormatInt(v, 10), nil
	case bool:
		return strconv.FormatBool(v), nil
	default:
		return v.(string), nil
	}
}

// domain describes the key's accepted values in one phrase.
func (k declaredConfigKey) domain() string {
	switch k.Type {
	case declaredTypeInteger:
		switch {
		case k.Minimum != nil && k.Maximum != nil:
			return fmt.Sprintf("an integer from %d to %d", *k.Minimum, *k.Maximum)
		case k.Minimum != nil:
			return fmt.Sprintf("an integer of at least %d", *k.Minimum)
		case k.Maximum != nil:
			return fmt.Sprintf("an integer of at most %d", *k.Maximum)
		}
		return "an integer"
	case declaredTypeBoolean:
		return "true or false"
	case declaredTypeEnum:
		return "one of " + strings.Join(k.Enum, ", ")
	default:
		return "any string"
	}
}

// entry projects the key onto the shared config registry row shape so
// config.tune's gating and scope handling treat it like a built-in scalar.
func (k declaredConfigKey) entry() configKeyEntry {
	declared := k
	return configKeyEntry{
		Key:        k.Key,
		WriterTool: "config.tune",
		ResetTool:  "config.tune",
		SelectorFields: []tuningField{{
			Name:        "scope",
			Description: fmt.Sprintf("Storage scope. When omitted the write lands in the key's declared default scope (%s).", k.DefaultScope),
			Enum:        wsconfig.ScopeSchemaEnum(),
		}},
		ValueFields: []tuningField{{
			Name:        "value",
			Description: fmt.Sprintf("Value as a string: %s. Omit when reset is true.", k.domain()),
			Enum:        k.valueEnum(),
		}},
		NoAgentVisible: true,
		ResolverBacked: true,
		Declared:       &declared,
	}
}

func (k declaredConfigKey) valueEnum() []string {
	switch k.Type {
	case declaredTypeBoolean:
		return []string{"true", "false"}
	case declaredTypeEnum:
		return k.Enum
	}
	return nil
}

// sortedKeys returns the declared keys in key order.
func (d declaredConfig) sortedKeys() []declaredConfigKey {
	keys := make([]declaredConfigKey, 0, len(d.keys))
	for _, k := range d.keys {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Key < keys[j].Key })
	return keys
}

// withDefaults returns base plus every declared key's default, so a resolver
// built from it floors declared keys at their manifest default (builtin scope).
func (d declaredConfig) withDefaults(base map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(d.keys))
	for k, v := range base {
		out[k] = v
	}
	for key, declared := range d.keys {
		out[key] = declared.Default
	}
	return out
}

// lookup returns the declared key, if any.
func (d declaredConfig) lookup(key string) (declaredConfigKey, bool) {
	k, ok := d.keys[key]
	return k, ok
}

// knobs projects the declared keys into tuning-catalog rows.
func (d declaredConfig) knobs(resolver *wsconfig.Resolver, sessionKey string) []tuningKnob {
	keys := d.sortedKeys()
	knobs := make([]tuningKnob, 0, len(keys))
	for _, k := range keys {
		entry := k.entry()
		knob := tuningKnob{
			ID:          k.Key,
			Kind:        "adapter_setting",
			Description: k.Description,
			Writer:      tuningWriter{Tool: entry.WriterTool, FixedArguments: map[string]string{"key": k.Key}},
			Reset: &tuningWriter{
				Tool:           entry.ResetTool,
				FixedArguments: map[string]string{"key": k.Key, "reset": "true"},
			},
			SelectorFields: entry.SelectorFields,
			ValueFields:    entry.ValueFields,
			Current:        currentWorkflowPreference(resolver, sessionKey, k.Key),
			Default:        &k.Default,
			RepoScope:      entry.RepoScoped(),
		}
		if knob.RepoScope {
			knob.RepoKey = entry.RepoKey()
		}
		knobs = append(knobs, knob)
	}
	return knobs
}

// configGetResult is config.get's JSON payload. Value is typed (number,
// boolean, or string) for a declared key and a string otherwise; it is null
// with scope "unset" for a key no scope holds and no registry or manifest
// declares.
type configGetResult struct {
	Key      string   `json:"key"`
	Value    any      `json:"value"`
	Scope    string   `json:"scope"`
	Warnings []string `json:"warnings,omitempty"`
}

// configGetUnsetScope is the scope config.get reports for an unset unknown key.
const configGetUnsetScope = "unset"

// resolveConfigGet resolves one key for config.get. A declared key whose
// stored string fails its declared type resolves to the manifest default at
// builtin scope with a warning naming the invalid value and its scope.
func (s *Server) resolveConfigGet(sessionKey, key string) (configGetResult, error) {
	builtins := s.declared.withDefaults(builtinConfigDefaults())
	resolver := s.sessionResolver(sessionKey, builtins)
	rv, err := resolver.Get(sessionKey, key)
	if err != nil {
		return configGetResult{}, err
	}
	result := configGetResult{Key: key, Value: rv.Value, Scope: string(rv.Scope)}
	if declared, ok := s.declared.lookup(key); ok {
		typed, parseErr := declared.parse(rv.Value)
		if parseErr != nil {
			result.Value = declared.DefaultValue
			result.Scope = string(wsconfig.ScopeBuiltin)
			result.Warnings = append(result.Warnings, fmt.Sprintf("stored value %q at scope %s is invalid (%v); using the declared default", rv.Value, rv.Scope, parseErr))
			return result, nil
		}
		result.Value = typed
		return result, nil
	}
	if _, hasBuiltin := builtins[key]; rv.Scope == wsconfig.ScopeBuiltin && !hasBuiltin && !builtinConfigKey(key) {
		result.Value = nil
		result.Scope = configGetUnsetScope
	}
	return result, nil
}

func formatConfigGet(result configGetResult) string {
	var b strings.Builder
	if result.Scope == configGetUnsetScope {
		fmt.Fprintf(&b, "%s: <unset>\n", result.Key)
	} else {
		fmt.Fprintf(&b, "%s: %v [scope:%s]\n", result.Key, result.Value, result.Scope)
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(&b, "warning: %s\n", warning)
	}
	return b.String()
}
