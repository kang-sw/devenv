package mcp

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

func TestCompactionRegistryManifestParity(t *testing.T) {
	legacy, _ := loadDeclaredConfig(piManifestPath).lookup("pi.compaction_advisory_percent")
	if legacy.Pattern != wsconfig.ThresholdPattern {
		t.Fatal("threshold grammar drifted from shipped manifests")
	}
	for key, def := range wsconfig.CompactionDefaults {
		e, ok := resolveConfigEntryForKey(key)
		if !ok || e.HarnessBucketed() || !e.HarnessStored() || !e.RepoScoped() {
			t.Fatal(key, e)
		}
		if builtinConfigDefaults()[key] != def {
			t.Fatal(key)
		}
		for _, v := range thresholdAccepted {
			if _, err := e.Normalize(v); err != nil {
				t.Fatalf("%s rejected %s", key, v)
			}
		}
		for _, v := range thresholdRejected {
			if _, err := e.Normalize(v); err == nil {
				t.Fatalf("%s accepted %s", key, v)
			}
		}
	}
}

func TestCompactionConfigGetTuneAndLegacy(t *testing.T) {
	s, _, key := manifestServer(t)
	s.observeHarness("test", "pi")
	tune := func(k, v, harness, scope string) {
		t.Helper()
		r := keyedCall(t, s, 1, key, "config.tune", map[string]any{"key": k, "value": v, "harness": harness, "scope": scope})
		if toolIsError(t, r) {
			t.Fatal(r)
		}
	}
	// New harness values remain scalar overrides, including agy.
	tune("compaction.lead_advisory", "70%", "pi", "global")
	tune("compaction.lead_advisory", "40%", "*", "project")
	result, err := s.resolveConfigGet(key, "compaction.lead_advisory")
	if err != nil || result.Value != "40%" || result.Scope != "project" {
		t.Fatal(result, err)
	}
	tune("compaction.worker_hard", "90%", "agy", "project")
	resolver := s.sessionResolver(key, builtinConfigDefaults())
	rv, err := resolver.Get(key, "compaction.worker_hard.agy")
	if err != nil || rv.Value != "90%" {
		t.Fatal(rv, err)
	}
	// Unknown tool refusal remains the older-daemon contract (no dispatch).
	r := keyedCall(t, s, 2, key, "compact", map[string]any{})
	if toolIsError(t, r) || !strings.Contains(toolText(t, r), "unsupported") {
		t.Fatal(r)
	}
}
