package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

const testManifest = `{
  "schema_version": 1,
  "namespace": "acme.",
  "keys": [
    {"key": "acme.limit", "type": "integer", "minimum": 1, "maximum": 10, "default": 3, "description": "Run limit."},
    {"key": "acme.retention_days", "type": "integer", "minimum": 0, "default": 30, "default_scope": "global", "description": "Retention in days; 0 disables."},
    {"key": "acme.animate", "type": "boolean", "default": true, "description": "Animate waits."},
    {"key": "acme.mode", "type": "enum", "enum": ["fast", "safe"], "default": "safe", "description": "Run mode."},
    {"key": "acme.label", "type": "string", "default": "Hello", "description": "Free label."},
    {"key": "acme.code", "type": "string", "pattern": "[a-z]+", "default": "abc", "description": "Lowercase code."}
  ]
}`

func writeManifest(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestLoadDeclaredConfigAcceptsValidManifest covers the typed defaults and
// declared default scopes a valid manifest yields.
func TestLoadDeclaredConfigAcceptsValidManifest(t *testing.T) {
	declared := loadDeclaredConfig(writeManifest(t, "ok.json", testManifest))
	if len(declared.rejected) != 0 {
		t.Fatalf("valid manifest rejected: %v", declared.rejected)
	}
	limit, ok := declared.lookup("acme.limit")
	if !ok || limit.Default != "3" || limit.DefaultValue != int64(3) || limit.DefaultScope != "project" {
		t.Fatalf("acme.limit = %+v", limit)
	}
	retention, _ := declared.lookup("acme.retention_days")
	if retention.DefaultScope != "global" {
		t.Fatalf("acme.retention_days default scope = %q, want global", retention.DefaultScope)
	}
	animate, _ := declared.lookup("acme.animate")
	if animate.Default != "true" || animate.DefaultValue != true {
		t.Fatalf("acme.animate = %+v", animate)
	}
	label, _ := declared.lookup("acme.label")
	if label.DefaultValue != "Hello" {
		t.Fatalf("acme.label = %+v", label)
	}
}

// TestLoadDeclaredConfigRejectsInvalidManifests: each malformed manifest is
// rejected as a whole with a reason, and never panics or fails the load.
func TestLoadDeclaredConfigRejectsInvalidManifests(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{"bad_json", `{`, "parse manifest"},
		{"unknown_field", `{"schema_version":1,"namespace":"acme.","keys":[{"key":"acme.a","type":"integer","default":1,"description":"d","minimun":0}]}`, "unknown field"},
		{"schema_version", `{"schema_version":2,"namespace":"acme.","keys":[{"key":"acme.a","type":"string","default":"x","description":"d"}]}`, "schema_version"},
		{"namespace_shape", `{"schema_version":1,"namespace":"Acme","keys":[{"key":"Acme.a","type":"string","default":"x","description":"d"}]}`, "namespace"},
		{"reserved_namespace", `{"schema_version":1,"namespace":"prompt.","keys":[{"key":"prompt.a","type":"string","default":"x","description":"d"}]}`, "reserved"},
		{"builtin_collision", `{"schema_version":1,"namespace":"workflow.","keys":[{"key":"workflow.a","type":"string","default":"x","description":"d"}]}`, "collides with built-in key"},
		{"no_keys", `{"schema_version":1,"namespace":"acme.","keys":[]}`, "no keys"},
		{"key_outside_namespace", `{"schema_version":1,"namespace":"acme.","keys":[{"key":"other.a","type":"string","default":"x","description":"d"}]}`, "must be \"acme.\""},
		{"duplicate_key", `{"schema_version":1,"namespace":"acme.","keys":[{"key":"acme.a","type":"string","default":"x","description":"d"},{"key":"acme.a","type":"string","default":"y","description":"d"}]}`, "declared twice"},
		{"bad_type", `{"schema_version":1,"namespace":"acme.","keys":[{"key":"acme.a","type":"float","default":1,"description":"d"}]}`, "type must be"},
		{"default_out_of_range", `{"schema_version":1,"namespace":"acme.","keys":[{"key":"acme.a","type":"integer","minimum":1,"maximum":5,"default":9,"description":"d"}]}`, "from 1 to 5"},
		{"default_fraction", `{"schema_version":1,"namespace":"acme.","keys":[{"key":"acme.a","type":"integer","default":1.5,"description":"d"}]}`, "integer"},
		{"default_quoted_integer", `{"schema_version":1,"namespace":"acme.","keys":[{"key":"acme.a","type":"integer","default":"5","description":"d"}]}`, "JSON integer"},
		{"default_wrong_json_type", `{"schema_version":1,"namespace":"acme.","keys":[{"key":"acme.a","type":"boolean","default":"true","description":"d"}]}`, "JSON boolean"},
		{"default_missing", `{"schema_version":1,"namespace":"acme.","keys":[{"key":"acme.a","type":"string","description":"d"}]}`, "default is required"},
		{"enum_default_not_member", `{"schema_version":1,"namespace":"acme.","keys":[{"key":"acme.a","type":"enum","enum":["x","y"],"default":"z","description":"d"}]}`, "one of x, y"},
		{"enum_empty", `{"schema_version":1,"namespace":"acme.","keys":[{"key":"acme.a","type":"enum","default":"z","description":"d"}]}`, "non-empty enum"},
		{"min_above_max", `{"schema_version":1,"namespace":"acme.","keys":[{"key":"acme.a","type":"integer","minimum":5,"maximum":1,"default":3,"description":"d"}]}`, "exceeds maximum"},
		{"bad_default_scope", `{"schema_version":1,"namespace":"acme.","keys":[{"key":"acme.a","type":"string","default":"x","default_scope":"repo","description":"d"}]}`, "default_scope"},
		{"pattern_on_integer", `{"schema_version":1,"namespace":"acme.","keys":[{"key":"acme.a","type":"integer","pattern":"[0-9]+","default":1,"description":"d"}]}`, "pattern applies only to type string"},
		{"pattern_on_enum", `{"schema_version":1,"namespace":"acme.","keys":[{"key":"acme.a","type":"enum","enum":["x"],"pattern":"x","default":"x","description":"d"}]}`, "pattern applies only to type string"},
		{"pattern_does_not_compile", `{"schema_version":1,"namespace":"acme.","keys":[{"key":"acme.a","type":"string","pattern":"(","default":"x","description":"d"}]}`, "does not compile"},
		{"pattern_lookahead_not_re2", `{"schema_version":1,"namespace":"acme.","keys":[{"key":"acme.a","type":"string","pattern":"(?=x)x","default":"x","description":"d"}]}`, "does not compile"},
		{"default_fails_pattern", `{"schema_version":1,"namespace":"acme.","keys":[{"key":"acme.a","type":"string","pattern":"[a-z]+","default":"x1","description":"d"}]}`, "matching the pattern [a-z]+"},
		{"missing_description", `{"schema_version":1,"namespace":"acme.","keys":[{"key":"acme.a","type":"string","default":"x"}]}`, "description is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			declared := loadDeclaredConfig(writeManifest(t, tc.name+".json", tc.body))
			if len(declared.rejected) != 1 || !strings.Contains(declared.rejected[0], tc.want) {
				t.Fatalf("rejected = %v, want one message containing %q", declared.rejected, tc.want)
			}
			if len(declared.keys) != 0 {
				t.Fatalf("rejected manifest still declared keys: %v", declared.keys)
			}
		})
	}
	t.Run("missing_file", func(t *testing.T) {
		declared := loadDeclaredConfig(filepath.Join(t.TempDir(), "absent.json"))
		if len(declared.rejected) != 1 || !strings.Contains(declared.rejected[0], "read manifest") {
			t.Fatalf("rejected = %v", declared.rejected)
		}
	})
	t.Run("namespace_taken_by_earlier_manifest", func(t *testing.T) {
		first := writeManifest(t, "first.json", testManifest)
		second := writeManifest(t, "second.json", `{"schema_version":1,"namespace":"acme.","keys":[{"key":"acme.other","type":"string","default":"x","description":"d"}]}`)
		declared := loadDeclaredConfig(first + string(os.PathListSeparator) + second)
		if len(declared.rejected) != 1 || !strings.Contains(declared.rejected[0], "already declared by "+first) {
			t.Fatalf("rejected = %v", declared.rejected)
		}
		if _, ok := declared.lookup("acme.limit"); !ok {
			t.Fatal("earlier manifest's keys must survive a later collision")
		}
		if _, ok := declared.lookup("acme.other"); ok {
			t.Fatal("colliding manifest's keys must not load")
		}
	})
}

// manifestServer starts a server with testManifest declared and a lead
// session bound to a fresh repository.
func manifestServer(t *testing.T, extraManifests ...string) (server *Server, root, key string) {
	t.Helper()
	useLeadProfile(t)
	t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))
	root = t.TempDir()
	mustWrite(t, root, "README.md", "# Test\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	paths := append([]string{writeManifest(t, "acme.json", testManifest)}, extraManifests...)
	t.Setenv(envConfigManifests, strings.Join(paths, string(os.PathListSeparator)))
	server = NewServer(root, "test")
	key, root = parseLoginResponse(t, callLogin(t, server, 930000, root, nil))
	return server, root, key
}

// keyedCall is callToolOnce with session_key set; it returns the raw response
// line so callers can check isError.
func keyedCall(t *testing.T, server *Server, id int, key, name string, args map[string]any) string {
	t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	args["session_key"] = key
	return callToolOnce(t, server, id, name, args)
}

func configGetJSON(t *testing.T, server *Server, id int, args map[string]any) configGetResult {
	t.Helper()
	args["format"] = "json"
	resp := callToolOnce(t, server, id, "config.get", args)
	if toolIsError(t, resp) {
		t.Fatalf("config.get(%v) failed: %s", args, toolText(t, resp))
	}
	var result configGetResult
	if err := json.Unmarshal([]byte(toolText(t, resp)), &result); err != nil {
		t.Fatalf("config.get json: %v\n%s", err, toolText(t, resp))
	}
	return result
}

// TestConfigTuneValidatesDeclaredKeys: config.tune accepts declared keys,
// parses the string value against the declared type, stores the canonical
// form, and writes the manifest's default scope when the call names none.
func TestConfigTuneValidatesDeclaredKeys(t *testing.T) {
	server, root, key := manifestServer(t)
	tune := func(id int, args map[string]any) string {
		t.Helper()
		return keyedCall(t, server, id, key, "config.tune", args)
	}
	rejects := []struct {
		args map[string]any
		want string
	}{
		{map[string]any{"key": "acme.limit", "value": "11"}, "from 1 to 10"},
		{map[string]any{"key": "acme.limit", "value": "0"}, "from 1 to 10"},
		{map[string]any{"key": "acme.limit", "value": "2.5"}, "from 1 to 10"},
		{map[string]any{"key": "acme.limit", "value": 4}, "must be a string"},
		{map[string]any{"key": "acme.limit"}, "value is required"},
		{map[string]any{"key": "acme.animate", "value": "yes"}, "true or false"},
		{map[string]any{"key": "acme.mode", "value": "slow"}, "one of fast, safe"},
		// The unanchored pattern still matches the whole value.
		{map[string]any{"key": "acme.code", "value": "abc1"}, "config.tune: acme.code value must be a string matching the pattern [a-z]+"},
		{map[string]any{"key": "acme.code", "value": "1abc"}, "matching the pattern [a-z]+"},
		{map[string]any{"key": "acme.code", "value": ""}, "matching the pattern [a-z]+"},
		{map[string]any{"key": "acme.undeclared", "value": "x"}, "unknown config key"},
	}
	for i, tc := range rejects {
		resp := tune(930100+i, tc.args)
		if !toolIsError(t, resp) || !strings.Contains(toolText(t, resp), tc.want) {
			t.Fatalf("config.tune(%v) = %s, want error containing %q", tc.args, resp, tc.want)
		}
	}

	if resp := tune(930200, map[string]any{"key": "acme.limit", "value": " 07 "}); !strings.Contains(toolText(t, resp), "acme.limit: 7 [scope:project]") {
		t.Fatalf("integer write echo: %s", resp)
	}
	// An explicit repo write lands in the session worktree's committed file.
	if resp := tune(930208, map[string]any{"key": "acme.limit", "value": "4", "scope": "repo"}); !strings.Contains(toolText(t, resp), "acme.limit: 4 [scope:repo]") || !strings.Contains(toolText(t, resp), filepath.Join(root, ".ws-workflow", "config.json")) {
		t.Fatalf("repo write echo: %s", resp)
	}
	if resp := tune(930201, map[string]any{"key": "acme.animate", "value": "FALSE"}); !strings.Contains(toolText(t, resp), "acme.animate: false [scope:project]") {
		t.Fatalf("boolean write echo: %s", resp)
	}
	if resp := tune(930202, map[string]any{"key": "acme.mode", "value": "fast", "scope": "session"}); !strings.Contains(toolText(t, resp), "acme.mode: fast [scope:session]") {
		t.Fatalf("enum session write echo: %s", resp)
	}
	if resp := tune(930203, map[string]any{"key": "acme.label", "value": "Mixed Case"}); !strings.Contains(toolText(t, resp), "acme.label: Mixed Case [scope:project]") {
		t.Fatalf("string write must keep its case: %s", resp)
	}
	if resp := tune(930207, map[string]any{"key": "acme.code", "value": "xyz"}); !strings.Contains(toolText(t, resp), "acme.code: xyz [scope:project]") {
		t.Fatalf("patterned string write echo: %s", resp)
	}
	if resp := tune(930204, map[string]any{"key": "acme.retention_days", "value": "0"}); !strings.Contains(toolText(t, resp), "acme.retention_days: 0 [scope:global]") {
		t.Fatalf("declared default scope not applied: %s", resp)
	}
	if got := configGetJSON(t, server, 930205, map[string]any{"key": "acme.retention_days"}); got.Value != float64(0) || got.Scope != "global" {
		t.Fatalf("retention after global write = %+v", got)
	}
	// Reset drops the override from the declared default scope and echoes the
	// manifest default.
	if resp := tune(930206, map[string]any{"key": "acme.retention_days", "reset": true}); !strings.Contains(toolText(t, resp), "acme.retention_days: 30 [scope:builtin]") {
		t.Fatalf("reset echo: %s", resp)
	}
}

// TestConfigListCatalogsDeclaredKeys: the tuning catalog lists each declared
// key with its description, value domain, default, and current value.
func TestConfigListCatalogsDeclaredKeys(t *testing.T) {
	server, _, key := manifestServer(t)
	keyedCall(t, server, 930300, key, "config.tune", map[string]any{"key": "acme.limit", "value": "5"})

	resp := keyedCall(t, server, 930301, key, "config.list", map[string]any{"format": "json"})
	var view configListView
	if err := json.Unmarshal([]byte(toolText(t, resp)), &view); err != nil {
		t.Fatalf("config.list json: %v", err)
	}
	var limit, code *tuningKnob
	for i := range view.Knobs {
		switch view.Knobs[i].ID {
		case "acme.limit":
			limit = &view.Knobs[i]
		case "acme.code":
			code = &view.Knobs[i]
		}
	}
	if limit == nil {
		t.Fatalf("config.list did not catalog acme.limit: %s", toolText(t, resp))
	}
	if limit.Kind != "adapter_setting" || limit.Description != "Run limit." || limit.Default == nil || *limit.Default != "3" {
		t.Fatalf("acme.limit knob = %+v", *limit)
	}
	if limit.Writer.Tool != "config.tune" || limit.Writer.FixedArguments["key"] != "acme.limit" || !limit.RepoScope {
		t.Fatalf("acme.limit writer/repo = %+v", *limit)
	}
	if len(limit.ValueFields) != 1 || !strings.Contains(limit.ValueFields[0].Description, "from 1 to 10") {
		t.Fatalf("acme.limit value field = %+v", limit.ValueFields)
	}
	if code == nil || len(code.ValueFields) != 1 || !strings.Contains(code.ValueFields[0].Description, "a string matching the pattern [a-z]+") {
		t.Fatalf("a patterned key's value field must name its pattern: %+v", code)
	}
	current, _ := json.Marshal(limit.Current)
	if string(current) != `{"scope":"project","value":"5"}` {
		t.Fatalf("acme.limit current = %s", current)
	}

	text := toolText(t, keyedCall(t, server, 930302, key, "config.list", nil))
	for _, want := range []string{"acme.mode (adapter_setting)", "values: value[fast|safe]", "default: safe", "acme.animate (adapter_setting)", "value[true|false]"} {
		if !strings.Contains(text, want) {
			t.Fatalf("config.list text missing %q:\n%s", want, text)
		}
	}
}

// TestConfigListReportsRejectedManifest: a rejected manifest leaves the
// server running, and config.list names it.
func TestConfigListReportsRejectedManifest(t *testing.T) {
	bad := writeManifest(t, "bad.json", `{"schema_version":1,"namespace":"acme.","keys":[{"key":"acme.x","type":"string","default":"x","description":"d"}]}`)
	server, _, key := manifestServer(t, bad)
	text := toolText(t, keyedCall(t, server, 930400, key, "config.list", nil))
	if !strings.Contains(text, "config manifest rejected: "+bad) {
		t.Fatalf("config.list text does not name the rejected manifest:\n%s", text)
	}
	resp := keyedCall(t, server, 930401, key, "config.list", map[string]any{"format": "json"})
	var view configListView
	if err := json.Unmarshal([]byte(toolText(t, resp)), &view); err != nil {
		t.Fatalf("config.list json: %v", err)
	}
	if len(view.ManifestErrors) != 1 || !strings.Contains(view.ManifestErrors[0], bad) {
		t.Fatalf("manifest_errors = %v", view.ManifestErrors)
	}
	if got := configGetJSON(t, server, 930402, map[string]any{"key": "acme.limit"}); got.Value != float64(3) {
		t.Fatalf("accepted manifest must still resolve: %+v", got)
	}
}

// TestConfigGetPrecedence walks one declared key down the scope stack:
// session (own, then inherited by a leaf child through the parent chain) >
// project > repo > global > manifest default.
func TestConfigGetPrecedence(t *testing.T) {
	server, root, key := manifestServer(t)
	const child = "child-leaf-key-00"
	writeSessionRecordForTest(t, server.sessions, child, root, roleLeaf, key)
	get := func(id int, sessionKey string) configGetResult {
		args := map[string]any{"key": "acme.limit"}
		if sessionKey != "" {
			args["session_key"] = sessionKey
		}
		return configGetJSON(t, server, id, args)
	}
	tune := func(id int, args map[string]any) {
		t.Helper()
		args["key"] = "acme.limit"
		if resp := keyedCall(t, server, id, key, "config.tune", args); toolIsError(t, resp) {
			t.Fatalf("config.tune(%v): %s", args, resp)
		}
	}
	want := func(got configGetResult, value float64, scope string) {
		t.Helper()
		if got.Value != value || got.Scope != scope || len(got.Warnings) != 0 {
			t.Fatalf("config.get = %+v, want %v [scope:%s]", got, value, scope)
		}
	}

	want(get(930500, key), 3, "builtin")
	tune(930501, map[string]any{"value": "4", "scope": "global"})
	want(get(930502, key), 4, "global")
	mustWrite(t, root, ".ws-workflow/config.json", `{"schema_version":1,"overrides":{"acme.limit":"5"}}`+"\n")
	want(get(930503, key), 5, "repo")
	want(get(930504, ""), 4, "global") // keyless: no repo anchor, no session
	tune(930505, map[string]any{"value": "6", "scope": "project"})
	want(get(930506, key), 6, "project")
	tune(930507, map[string]any{"value": "7", "scope": "session"})
	want(get(930508, key), 7, "session")
	want(get(930509, child), 7, "session")
}

// TestConfigGetTypesAndFallbacks: declared keys read as their declared JSON
// type, other keys as strings, an invalid stored value falls back to the
// default with a warning, and an unknown unset key reads as unset.
func TestConfigGetTypesAndFallbacks(t *testing.T) {
	server, root, key := manifestServer(t)
	mustWrite(t, root, ".ws-workflow/config.json",
		`{"schema_version":1,"overrides":{"acme.limit":"lots","acme.animate":"false","acme.label":"  spaced  ","acme.mode":"fast","custom.flag":"x"}}`+"\n")

	get := func(id int, k string) configGetResult {
		return configGetJSON(t, server, id, map[string]any{"key": k, "session_key": key})
	}
	if got := get(930600, "acme.animate"); got.Value != false || got.Scope != "repo" {
		t.Fatalf("boolean = %+v", got)
	}
	if got := get(930601, "acme.label"); got.Value != "  spaced  " {
		t.Fatalf("string = %+v", got)
	}
	if got := get(930602, "acme.mode"); got.Value != "fast" {
		t.Fatalf("enum = %+v", got)
	}
	got := get(930603, "acme.limit")
	if got.Value != float64(3) || got.Scope != "builtin" || len(got.Warnings) != 1 ||
		!strings.Contains(got.Warnings[0], `"lots"`) || !strings.Contains(got.Warnings[0], "scope repo") {
		t.Fatalf("invalid stored value fallback = %+v", got)
	}
	if got := get(930604, "review_phase"); got.Value != "lite" || got.Scope != "builtin" {
		t.Fatalf("built-in key must read as its stored string = %+v", got)
	}
	if got := get(930605, "custom.flag"); got.Value != "x" || got.Scope != "repo" {
		t.Fatalf("undeclared stored key = %+v", got)
	}
	if got := get(930606, "custom.absent"); got.Value != nil || got.Scope != "unset" {
		t.Fatalf("unknown unset key = %+v", got)
	}

	text := toolText(t, keyedCall(t, server, 930607, key, "config.get", map[string]any{"key": "acme.limit"}))
	if !strings.HasPrefix(text, "acme.limit: 3 [scope:builtin]\nwarning: stored value \"lots\" at scope repo") {
		t.Fatalf("config.get text = %q", text)
	}
	if text := toolText(t, keyedCall(t, server, 930608, key, "config.get", map[string]any{"key": "custom.absent"})); text != "custom.absent: <unset>\n" {
		t.Fatalf("unset text = %q", text)
	}
}

// TestShippedAdapterManifestsLoad guards each adapter package's shipped
// manifest against drifting out of the loader's accepted format: a rejected
// manifest would only log at startup and silently leave every adapter knob
// untunable.
func TestShippedAdapterManifestsLoad(t *testing.T) {
	for _, path := range []string{piManifestPath, claudeManifestPath, wsflowClaudeManifestPath} {
		declared := loadDeclaredConfig(path)
		if len(declared.rejected) != 0 {
			t.Fatalf("shipped manifest rejected: %v", declared.rejected)
		}
		if len(declared.keys) == 0 {
			t.Fatalf("shipped manifest %s declares no keys", path)
		}
	}
}

var (
	piManifestPath     = filepath.Join("..", "..", "..", "agents-plugin-pi", "config-manifest.json")
	claudeManifestPath = filepath.Join("..", "..", "..", "agents-plugin", "config-manifest.json")
	// wsflowClaudeManifestPath is the wsflow package's own Claude manifest:
	// the same keys as claudeManifestPath, differing only in each default.
	wsflowClaudeManifestPath = filepath.Join("..", "..", "..", "agents-plugin-wsflow", "config-manifest.json")
)

// The compaction-threshold grammar corpus: the Pi adapter-config test runs it
// against Pi's parser, and the tests below against each patterned key.
var (
	thresholdAccepted = []string{"50", "50%", "1", "1%", "100", "100%", "150k", "1.5m", "0.5k", "1.0004k", "007k", "50%|150k", "150k|50%", "100|1m"}
	thresholdRejected = []string{"0", "0%", "101", "101%", "050", "50.5", "-5", "", "0k", "0.0m", "00.00k", "150K", "1.5M", "15ok", "k", ".5k", "5.k",
		"50%%", "50%|60%", "150k|1m", "50%|150k|1m", "50% | 150k", " 50", "50 ", "|50%", "50%|", "1e3k"}
)

func checkThresholdCorpus(t *testing.T, name string, key declaredConfigKey) {
	t.Helper()
	for _, value := range thresholdAccepted {
		if _, err := key.normalize(value); err != nil {
			t.Errorf("%s rejects %q: %v", name, value, err)
		}
	}
	for _, value := range thresholdRejected {
		if _, err := key.normalize(value); err == nil {
			t.Errorf("%s accepts %q", name, value)
		}
	}
}

// TestShippedPiCompactionThresholdPattern runs the Pi compaction-threshold
// grammar corpus through the Go (RE2) loader; the Pi package's
// adapter-config test runs the same corpus against its own parser, so
// config.tune never accepts a value Pi would silently replace with the
// default.
func TestShippedPiCompactionThresholdPattern(t *testing.T) {
	declared := loadDeclaredConfig(piManifestPath)
	for _, name := range []string{"pi.compaction_advisory_percent", "pi.compaction_hard_percent"} {
		key, ok := declared.lookup(name)
		if !ok || key.Type != declaredTypeString || key.Pattern == "" {
			t.Fatalf("%s = %+v, want a patterned string key", name, key)
		}
		checkThresholdCorpus(t, name, key)
	}
}

// TestShippedClaudeAdvisoryMatchesPi: the Claude mod's advisory key accepts
// exactly what Pi's advisory key accepts (the Claude mod's parser copies Pi's
// grammar), carries the same pattern and default, and keeps the default
// project scope, since a declared adapter key may not default to repo.
func TestShippedClaudeAdvisoryMatchesPi(t *testing.T) {
	const name = "claude.compaction_advisory_percent"
	claude, ok := loadDeclaredConfig(claudeManifestPath).lookup(name)
	if !ok || claude.Type != declaredTypeString || claude.Pattern == "" {
		t.Fatalf("%s = %+v, want a patterned string key", name, claude)
	}
	pi, ok := loadDeclaredConfig(piManifestPath).lookup("pi.compaction_advisory_percent")
	if !ok {
		t.Fatal("pi.compaction_advisory_percent is not declared")
	}
	if claude.Pattern != pi.Pattern {
		t.Fatalf("%s pattern drifted from pi.compaction_advisory_percent:\n%s\n%s", name, claude.Pattern, pi.Pattern)
	}
	if claude.Default != pi.Default || claude.Default != "50%" {
		t.Fatalf("%s default = %q, pi default = %q, want both 50%%", name, claude.Default, pi.Default)
	}
	if claude.DefaultScope == wsconfig.ScopeRepo {
		t.Fatalf("%s defaults to repo scope", name)
	}
	checkThresholdCorpus(t, name, claude)
}

// TestConfigTuneValidatesClaudeAdvisory: a server launched with the Claude
// manifest validates the key at config.tune and reads it typed at config.get.
func TestConfigTuneValidatesClaudeAdvisory(t *testing.T) {
	server, _, key := manifestServer(t, claudeManifestPath)
	const name = "claude.compaction_advisory_percent"
	if got := configGetJSON(t, server, 930700, map[string]any{"key": name, "session_key": key}); got.Value != "50%" || got.Scope != "builtin" {
		t.Fatalf("unset %s = %+v", name, got)
	}
	for i, value := range []string{"50%|60%", "0", "150K"} {
		resp := keyedCall(t, server, 930710+i, key, "config.tune", map[string]any{"key": name, "value": value})
		if !toolIsError(t, resp) || !strings.Contains(toolText(t, resp), "matching the pattern") {
			t.Fatalf("config.tune(%q) = %s, want a pattern rejection", value, resp)
		}
	}
	if resp := keyedCall(t, server, 930720, key, "config.tune", map[string]any{"key": name, "value": "40%|300k"}); !strings.Contains(toolText(t, resp), name+": 40%|300k [scope:project]") {
		t.Fatalf("config.tune echo: %s", resp)
	}
	if got := configGetJSON(t, server, 930721, map[string]any{"key": name}); got.Value != "40%|300k" || got.Scope != "project" {
		t.Fatalf("tuned %s without a session key = %+v", name, got)
	}
}

// TestShippedClaudeCaptionStyleMatchesMod: the caption style key's manifest
// default is the Claude mod's own fallback text (DEFAULT_STYLE in
// captions.tsx), so a failed read draws captions in the style config.list
// shows. The mod's test kit has no file access, so the pin reads the mod source
// here.
func TestShippedClaudeCaptionStyleMatchesMod(t *testing.T) {
	const name = "claude.display_summary_style"
	style, ok := loadDeclaredConfig(claudeManifestPath).lookup(name)
	if !ok || style.Type != declaredTypeString {
		t.Fatalf("%s = %+v, want a string key", name, style)
	}
	source, err := os.ReadFile(filepath.Join("..", "..", "..", "agents-plugin", "hooks", "claude-mod", "captions.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`export const DEFAULT_STYLE = '([^'\\]*)'`).FindSubmatch(source)
	if match == nil {
		t.Fatal("captions.tsx declares no single-quoted DEFAULT_STYLE literal")
	}
	if style.Default != string(match[1]) {
		t.Fatalf("%s default drifted from the mod's DEFAULT_STYLE:\nmanifest: %s\nmod:      %s", name, style.Default, match[1])
	}
}

// TestConfigGetReadsClaudeCaptionKeys: a server launched with the Claude
// manifest answers each caption key and the status band switch typed and
// keyless, a tune included, refuses a context window below one token, and
// answers a session-scope tune of the band switch only to a keyed read.
func TestConfigGetReadsClaudeCaptionKeys(t *testing.T) {
	server, _, key := manifestServer(t, claudeManifestPath)
	for i, want := range []struct {
		name  string
		value any
	}{
		{"claude.display_summary_enabled", false},
		{"claude.display_summary_model", "haiku"},
		{"claude.display_summary_context_tokens", float64(200000)},
		{"claude.status_band_enabled", true},
	} {
		if got := configGetJSON(t, server, 930800+i, map[string]any{"key": want.name}); got.Value != want.value || got.Scope != "builtin" {
			t.Fatalf("unset %s = %+v, want %v", want.name, got, want.value)
		}
	}
	if resp := keyedCall(t, server, 930810, key, "config.tune", map[string]any{"key": "claude.display_summary_context_tokens", "value": "0"}); !toolIsError(t, resp) {
		t.Fatalf("config.tune(context_tokens=0) = %s, want a rejection", resp)
	}
	if resp := keyedCall(t, server, 930811, key, "config.tune", map[string]any{"key": "claude.display_summary_enabled", "value": "true"}); toolIsError(t, resp) {
		t.Fatalf("config.tune(enabled=true) = %s", resp)
	}
	if got := configGetJSON(t, server, 930812, map[string]any{"key": "claude.display_summary_enabled"}); got.Value != true || got.Scope != "project" {
		t.Fatalf("tuned claude.display_summary_enabled without a session key = %+v", got)
	}
	// The status band reads its switch with the held key, so a session-scope
	// tune turns it off for that session alone.
	if resp := keyedCall(t, server, 930813, key, "config.tune", map[string]any{"key": "claude.status_band_enabled", "value": "false", "scope": "session"}); toolIsError(t, resp) {
		t.Fatalf("config.tune(status_band_enabled=false, session) = %s", resp)
	}
	if got := configGetJSON(t, server, 930814, map[string]any{"key": "claude.status_band_enabled", "session_key": key}); got.Value != false || got.Scope != "session" {
		t.Fatalf("session-tuned claude.status_band_enabled with the session key = %+v", got)
	}
	if got := configGetJSON(t, server, 930815, map[string]any{"key": "claude.status_band_enabled"}); got.Value != true || got.Scope != "builtin" {
		t.Fatalf("session-tuned claude.status_band_enabled without a session key = %+v", got)
	}
}
