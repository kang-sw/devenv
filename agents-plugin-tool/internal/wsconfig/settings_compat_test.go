package wsconfig

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestSettingsConfigWritePreservesUnknownMembersAndKnownSiblingLeaves(t *testing.T) {
	opts := Options{ConfigHome: t.TempDir(), CacheHome: t.TempDir()}
	path, err := GlobalPath(opts)
	if err != nil {
		t.Fatal(err)
	}
	raw := `{"schema_version":1,"future":{"flag":true},"agents":{"future_agent":{"flag":true},"model_aliases":{"small":{"codex":{"backend":"codex","model":"before","future_leaf":{"flag":true}},"pi":{"backend":"pi","model":"sibling"}},"large":{"codex":{"backend":"codex","model":"untouched"}}}},"overrides":{"unknown.setting":"old"},"structured_overrides":{"worktree.profile.old":{"future_object":{"flag":true}}}}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SetGlobalAgentsTierForHarness(opts, "small", "codex", "after", "codex"); err != nil {
		t.Fatal(err)
	}
	resolver := NewResolver(opts, nil, nil, nil)
	if err := resolver.Set("workflow.lang", "Korean", SetOptions{ExplicitScope: ScopeGlobal}); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"future", "future_agent", "future_leaf", "future_object", "after", "sibling", "untouched", "unknown.setting"} {
		if !strings.Contains(string(out), `"`+key+`"`) {
			t.Errorf("write dropped %s: %s", key, out)
		}
	}

	// The prior writer reserialized Config without unknown struct members.
	// Known maps and every previously supported tier leaf survive that rewrite.
	var legacy Config
	if err := json.Unmarshal(out, &legacy); err != nil {
		t.Fatal(err)
	}
	legacy.Overrides["legacy.writer"] = "touched"
	rewritten, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, rewritten, 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := FileLayers(opts)
	if err != nil {
		t.Fatal(err)
	}
	if value := files[ScopeGlobal].Overrides["workflow.lang"]; value != "Korean" {
		t.Errorf("legacy rewrite lost settings value: %q", value)
	}
	if _, removed, err := UnsetGlobalAgentsTierForHarness(opts, "small", "codex"); err != nil || !removed {
		t.Fatalf("remove one leaf: removed=%t, err=%v", removed, err)
	}
	files, err = FileLayers(opts)
	if err != nil {
		t.Fatal(err)
	}
	aliases := files[ScopeGlobal].Agents.ModelAliases
	if _, ok := aliases["small"]["codex"]; ok || aliases["small"]["pi"].Model != "sibling" || aliases["large"]["codex"].Model != "untouched" {
		t.Errorf("reset affected the wrong leaves: %#v", aliases)
	}
	if files[ScopeGlobal].Overrides["legacy.writer"] != "touched" || files[ScopeGlobal].StructuredOverrides["worktree.profile.old"] == nil {
		t.Errorf("new writer lost existing map members: %#v", files)
	}
}

func TestSettingsLegacyTierSeedHasProvenanceWithoutASelectedHarnessOverride(t *testing.T) {
	opts := Options{ConfigHome: t.TempDir(), CacheHome: t.TempDir()}
	path, err := GlobalPath(opts)
	if err != nil {
		t.Fatal(err)
	}
	raw := `{"schema_version":1,"agents":{"tiers":{"small":{"backend":"codex","model":"legacy-small"}}}}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := FileLayers(opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, harness := range []string{"default", "codex", "pi"} {
		info := InspectTier(files, "small", harness)
		layer := info.Layers[ScopeGlobal]
		if info.Scope != ScopeGlobal || info.Value.Model != "legacy-small" || !layer.Legacy || layer.Selected != nil {
			t.Errorf("legacy seed for %s = %#v", harness, info)
		}
	}
	if info := InspectTier(files, "small", "claude"); info.Scope != ScopeBuiltin || info.Value.Model == "legacy-small" {
		t.Errorf("legacy mapping replaced Claude's builtin alias: %#v", info)
	}
	if _, removed, err := UnsetGlobalAgentsTierForHarness(opts, "small", "codex"); err != nil || removed {
		t.Errorf("leaf reset removed a legacy tier: removed=%t, err=%v", removed, err)
	}
	out, err := os.ReadFile(path)
	if err != nil || string(out) != raw {
		t.Errorf("no-op reset changed the legacy file: %s, %v", out, err)
	}
}
