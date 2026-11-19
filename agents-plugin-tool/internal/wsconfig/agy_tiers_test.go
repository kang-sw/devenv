package wsconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAgyBuiltinTiers(t *testing.T) {
	opts := Options{CacheHome: t.TempDir(), ConfigHome: t.TempDir()}
	want := map[string]AgentTier{
		"small":  {Backend: "agy", Model: "gemini-4-hydrogen", Effort: "medium"},
		"medium": {Backend: "agy", Model: "gemini-4-neon", Effort: "high"},
		"large":  {Backend: "agy", Model: "gemini-4-neon", Effort: "xhigh"},
		"xlarge": {Backend: "agy", Model: "gemini-4-carbon", Effort: "high"},
	}
	got, err := HarnessTiers(opts, " AGY ")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("HarnessTiers = %v, %v; want %v", got, err, want)
	}
	for tier, mapping := range want {
		backend, model, effort, source, err := ResolveAgentTierForHarness(opts, tier, "agy")
		if err != nil || (AgentTier{Backend: backend, Model: model, Effort: effort}) != mapping || source != "agy" {
			t.Errorf("%s = %q/%q/%q from %q, %v; want %v from agy", tier, backend, model, effort, source, err, mapping)
		}
		backend, model, effort, err = ResolveAgentForHarnessConfig(opts, tier, "agy", "", "claude")
		if err != nil || (AgentTier{Backend: backend, Model: model, Effort: effort}) != mapping {
			t.Errorf("explicit agy backend %s = %q/%q/%q, %v", tier, backend, model, effort, err)
		}
	}
}

func TestAgyTierOverlayAndReset(t *testing.T) {
	opts := Options{CacheHome: t.TempDir(), ConfigHome: t.TempDir()}
	global := AgentTier{Backend: "agy", Model: "global-base-model", Effort: "low"}
	project := AgentTier{Backend: "agy", Model: "project-base-model", Effort: "medium"}
	if _, err := SetGlobalAgentsTierForHarness(opts, "small", global.Backend, global.Model, "agy", global.Effort); err != nil {
		t.Fatal(err)
	}
	if _, err := SetAgentsTierForHarness(opts, "small", project.Backend, project.Model, " AGY ", project.Effort); err != nil {
		t.Fatal(err)
	}
	check := func(want AgentTier) {
		t.Helper()
		got, err := HarnessTiers(opts, "agy")
		if err != nil || got["small"] != want || got["large"] != (AgentTier{Backend: "agy", Model: "gemini-4-neon", Effort: "xhigh"}) {
			t.Fatalf("overlay = %v, %v; want small %v and builtin large", got, err, want)
		}
	}
	check(project)
	if present, err := HasProjectAgentsTierLeaf(opts, "small", "agy"); err != nil || !present {
		t.Fatalf("project leaf = %v, %v", present, err)
	}
	if _, removed, err := UnsetAgentsTierForHarness(opts, "small", "agy"); err != nil || !removed {
		t.Fatalf("project reset = %v, %v", removed, err)
	}
	check(global)
	if _, removed, err := UnsetGlobalAgentsTierForHarness(opts, "small", "agy"); err != nil || !removed {
		t.Fatalf("global reset = %v, %v", removed, err)
	}
	check(AgentTier{Backend: "agy", Model: "gemini-4-hydrogen", Effort: "medium"})
}

// Harness bucket names are map keys, not an enum in the stored shape. Exercise
// the pre-agy read/modify/write path with an unrelated known-harness edit and a
// still-unknown bucket, so accepting agy cannot mask filtering unknown buckets.
// This covers existing AgentTier fields, not arbitrary future JSON members.
func TestTierWritesPreserveUnknownHarnessBuckets(t *testing.T) {
	for _, global := range []bool{false, true} {
		name := "project"
		if global {
			name = "global"
		}
		t.Run(name, func(t *testing.T) {
			opts := Options{CacheHome: t.TempDir(), ConfigHome: t.TempDir()}
			path, err := agentsConfigPath(opts, global)
			if err != nil {
				t.Fatal(err)
			}
			fixture := `{"schema_version":1,"agents":{"model_aliases":{"small":{"agy":{"backend":"agy","model":"custom-base","effort":"low"},"future-harness":{"backend":"future","model":"future-base","effort":"high"}},"future-tier":{"future-harness":{"backend":"future","model":"future-tier-model","effort":"medium"}}}},"overrides":{"future.knob":"retained"},"structured_overrides":{"future.object":{"nested":[1,"retained"]}}}`
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(fixture), 0o644); err != nil {
				t.Fatal(err)
			}
			before, err := loadConfigFile(path, name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := setAgentsTierForHarness(opts, global, "medium", "claude", "sonnet", "claude", "high"); err != nil {
				t.Fatal(err)
			}
			if _, removed, err := unsetAgentsTierForHarness(opts, global, "medium", "claude"); err != nil || !removed {
				t.Fatalf("unrelated reset = %v, %v", removed, err)
			}
			after, err := loadConfigFile(path, name)
			if err != nil {
				t.Fatal(err)
			}
			if canonicalConfigJSON(t, after) != canonicalConfigJSON(t, before) {
				t.Fatalf("unrelated writes lost bucket data:\nbefore: %+v\nafter: %+v", before, after)
			}
			// Reopen through the new resolver: the user's agy value, not the
			// builtin, must survive the older-style writer's round trip.
			got, err := LoadAgentTierConfig(opts)
			if err != nil || got.Agents.ModelAliases["small"]["agy"] != before.Agents.ModelAliases["small"]["agy"] {
				t.Fatalf("reopened agy = %v, %v", got.Agents.ModelAliases, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var persisted Config
			if err := json.Unmarshal(raw, &persisted); err != nil || canonicalConfigJSON(t, persisted) != canonicalConfigJSON(t, before) {
				t.Fatalf("persisted buckets = %+v, %v", persisted, err)
			}
		})
	}
}

func canonicalConfigJSON(t *testing.T, cfg Config) string {
	t.Helper()
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestTierAliasesAcceptAgy(t *testing.T) {
	got, err := ParseTierAliases("fast=AGY.small deliberate=agy.deep")
	want := map[string]string{"fast": "agy.small", "deliberate": "agy.large"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("aliases = %v, %v; want %v", got, err, want)
	}
}
