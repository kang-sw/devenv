package mcp

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

// The ws and wsflow packages ship one Claude mod and differ only in each
// knob's default, so their manifests must declare the same keys with every
// other field identical. These tests make drift between the two visible.

// readManifestJSON decodes a shipped manifest into generic JSON, so a field
// the loader adds later is compared without being named here.
func readManifestJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return out
}

func manifestKeyObjects(t *testing.T, path string, manifest map[string]any) []map[string]any {
	t.Helper()
	raw, ok := manifest["keys"].([]any)
	if !ok {
		t.Fatalf("%s: keys is not an array", path)
	}
	out := make([]map[string]any, 0, len(raw))
	for i, entry := range raw {
		obj, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("%s: keys[%d] is not an object", path, i)
		}
		out = append(out, obj)
	}
	return out
}

// TestShippedClaudeManifestsDifferOnlyInDefaults: both packages' manifests
// load, share schema_version, namespace, and the ordered key set, and every
// key's fields other than default are identical. A key whose default differs
// between the packages must not mention a default in its shared description.
func TestShippedClaudeManifestsDifferOnlyInDefaults(t *testing.T) {
	for _, path := range []string{claudeManifestPath, wsflowClaudeManifestPath} {
		if declared := loadDeclaredConfig(path); len(declared.rejected) != 0 || len(declared.keys) == 0 {
			t.Fatalf("%s: rejected %v, %d keys", path, declared.rejected, len(declared.keys))
		}
	}
	ws := readManifestJSON(t, claudeManifestPath)
	wsflow := readManifestJSON(t, wsflowClaudeManifestPath)
	for _, field := range []string{"schema_version", "namespace"} {
		if !reflect.DeepEqual(ws[field], wsflow[field]) {
			t.Fatalf("%s differs: ws %v, wsflow %v", field, ws[field], wsflow[field])
		}
	}
	wsKeys := manifestKeyObjects(t, claudeManifestPath, ws)
	wsflowKeys := manifestKeyObjects(t, wsflowClaudeManifestPath, wsflow)
	names := func(keys []map[string]any) []any {
		out := make([]any, len(keys))
		for i, k := range keys {
			out[i] = k["key"]
		}
		return out
	}
	if !reflect.DeepEqual(names(wsKeys), names(wsflowKeys)) {
		t.Fatalf("key sets differ:\nws     %v\nwsflow %v", names(wsKeys), names(wsflowKeys))
	}
	for i := range wsKeys {
		a, b := wsKeys[i], wsflowKeys[i]
		name := a["key"]
		defaultsDiffer := !reflect.DeepEqual(a["default"], b["default"])
		strip := func(obj map[string]any) map[string]any {
			out := make(map[string]any, len(obj))
			for k, v := range obj {
				if k != "default" {
					out[k] = v
				}
			}
			return out
		}
		if !reflect.DeepEqual(strip(a), strip(b)) {
			t.Errorf("%v: fields other than default differ:\nws     %v\nwsflow %v", name, strip(a), strip(b))
		}
		if desc, _ := a["description"].(string); defaultsDiffer && strings.Contains(strings.ToLower(desc), "default") {
			t.Errorf("%v: defaults differ between packages, so its shared description must not mention a default: %q", name, desc)
		}
	}
}

// claudePackageDefaults is each package's built-in value for the
// package-differing Claude knobs: ws is the opinionated distribution, wsflow
// the conservative one.
var claudePackageDefaults = []struct {
	key    string
	ws     any
	wsflow any
}{
	{"claude.compaction_advisory_enabled", true, false},
	{"claude.manual_injection_enabled", true, false},
	{"claude.mailbox_drain", "direct", "notice"},
	{"claude.display_summary_enabled", false, false},
	{"claude.status_band_enabled", true, false},
	{"claude.mcp_watchdog_enabled", true, false},
}

// TestShippedClaudePackageDefaults pins each package's knob defaults, both as
// loaded and as a keyless config.get answers them on an untuned server.
func TestShippedClaudePackageDefaults(t *testing.T) {
	for _, pkg := range []struct {
		name string
		path string
		pick func(i int) any
	}{
		{"ws", claudeManifestPath, func(i int) any { return claudePackageDefaults[i].ws }},
		{"wsflow", wsflowClaudeManifestPath, func(i int) any { return claudePackageDefaults[i].wsflow }},
	} {
		t.Run(pkg.name, func(t *testing.T) {
			declared := loadDeclaredConfig(pkg.path)
			server, _, sessionKey := manifestServer(t, pkg.path)
			for i, want := range claudePackageDefaults {
				key, ok := declared.lookup(want.key)
				if !ok {
					t.Fatalf("%s does not declare %s", pkg.path, want.key)
				}
				if key.DefaultValue != pkg.pick(i) {
					t.Errorf("%s default = %v, want %v", want.key, key.DefaultValue, pkg.pick(i))
				}
				got := configGetJSON(t, server, 931000+i, map[string]any{"key": want.key})
				if got.Value != pkg.pick(i) || got.Scope != "builtin" {
					t.Errorf("config.get(%s) = %+v, want %v from builtin", want.key, got, pkg.pick(i))
				}
			}
			// The drain is an enum: a value outside direct and notice is refused.
			if resp := keyedCall(t, server, 931100, sessionKey, "config.tune", map[string]any{"key": "claude.mailbox_drain", "value": "push"}); !toolIsError(t, resp) {
				t.Fatalf("config.tune(mailbox_drain=push) = %s, want a rejection", resp)
			}
			if resp := keyedCall(t, server, 931101, sessionKey, "config.tune", map[string]any{"key": "claude.mailbox_drain", "value": "notice"}); toolIsError(t, resp) {
				t.Fatalf("config.tune(mailbox_drain=notice) = %s", resp)
			}
		})
	}
}

// TestShippedClaudeWatchdogInterval pins the watchdog interval: the same
// positive-integer default of 10 minutes in both packages, as loaded and as a
// keyless config.get answers it, and a tune below 1 refused.
func TestShippedClaudeWatchdogInterval(t *testing.T) {
	const key = "claude.mcp_watchdog_interval_minutes"
	for _, path := range []string{claudeManifestPath, wsflowClaudeManifestPath} {
		t.Run(path, func(t *testing.T) {
			declared := loadDeclaredConfig(path)
			k, ok := declared.lookup(key)
			if !ok {
				t.Fatalf("%s does not declare %s", path, key)
			}
			if k.DefaultValue != int64(10) {
				t.Errorf("%s default = %v (%T), want 10", key, k.DefaultValue, k.DefaultValue)
			}
			server, _, sessionKey := manifestServer(t, path)
			if got := configGetJSON(t, server, 932000, map[string]any{"key": key}); got.Value != float64(10) || got.Scope != "builtin" {
				t.Errorf("config.get(%s) = %+v, want 10 from builtin", key, got)
			}
			if resp := keyedCall(t, server, 932001, sessionKey, "config.tune", map[string]any{"key": key, "value": "0"}); !toolIsError(t, resp) {
				t.Fatalf("config.tune(%s=0) = %s, want a rejection", key, resp)
			}
			if resp := keyedCall(t, server, 932002, sessionKey, "config.tune", map[string]any{"key": key, "value": "1"}); toolIsError(t, resp) {
				t.Fatalf("config.tune(%s=1) = %s", key, resp)
			}
		})
	}
}
