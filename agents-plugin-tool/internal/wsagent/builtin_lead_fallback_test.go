package wsagent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLeadProfileForBuiltinFallbackMatchesRegistry(t *testing.T) {
	registry := map[string]BackendRegistration{
		BackendClaude: {LeadProfile: LeadProfileClaude},
		BackendPi:     {LeadProfile: LeadProfilePi},
	}
	leads := map[string]string{}
	for key, raw := range BuiltinProfiles() {
		var p Profile
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Fatal(err)
		}
		if !p.Lead {
			continue
		}
		if previous, ok := leads[p.Backend]; ok {
			t.Fatalf("builtin backend %q has multiple lead profiles: %s, %s", p.Backend, previous, key)
		}
		leads[p.Backend] = strings.TrimPrefix(key, ProfilePrefix)
	}
	if len(leads) != len(registry) {
		t.Fatalf("builtin leads = %v, registry = %v", leads, registry)
	}
	for _, backend := range []string{BackendClaude, BackendPi, "other", "", " pi "} {
		t.Run(backend, func(t *testing.T) {
			want := registry[backend].LeadProfile
			if leads[backend] != want {
				t.Fatalf("builtin lead = %q, registry = %q", leads[backend], want)
			}
			for i := 0; i < 20; i++ {
				if got := LeadProfileFor(backend); got != want || got != LeadProfileFor(backend, registry) {
					t.Fatalf("fallback = %q, want registry's %q", got, want)
				}
			}
			if got := LeadProfileFor(backend, nil); got != "" {
				t.Fatalf("explicit empty registry fell back to %q", got)
			}
		})
	}
}
