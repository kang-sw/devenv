package wsagent

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestCodexCatalog(t *testing.T) {
	p := CodexCatalog()
	want := map[CapabilityItem]CapabilityGrade{
		ItemSkillList: GradeUnsupported, ItemSkillInjection: GradeNative,
		ItemCompaction: GradeNative, ItemRewind: GradeUnsupported,
		ItemToolRemoval: GradeUnsupported, ItemSubagentRemoval: GradeNative,
		ItemModelEffort: GradeNative, ItemApproval: GradeNative,
		ItemSteer: GradeNative, ItemInterrupt: GradeNative,
		ItemFollowUp: GradeSubstitute, ItemResume: GradeNative,
		ItemReasoningStream: GradeUnsupported, ItemUsageQuery: GradeNative,
	}
	if p.Oldest != "0.160.0" || p.Newest != "0.160.0" || len(p.Entries) != len(CatalogItems) {
		t.Fatalf("profile bounds or entries: %+v", p)
	}
	seen := map[CapabilityItem]bool{}
	for _, e := range p.Entries {
		if seen[e.Item] || e.Role != RoleLead || e.Since != "" || e.Until != "" || e.Source == "" {
			t.Errorf("invalid entry: %+v", e)
		}
		seen[e.Item] = true
	}
	for _, version := range []string{"0.159.0", "0.160.0", "0.161.0", "", "codex-cli 0.160.0"} {
		t.Run(version, func(t *testing.T) {
			lead := p.Resolve(RoleLead, version)
			if lead.Version != version || lead.ProfileVersion != "0.160.0" || lead.Unverified != (version != "0.160.0") {
				t.Fatalf("resolved range: %+v", lead)
			}
			worker := p.Resolve(RoleWorker, version)
			for _, item := range CatalogItems {
				if lead.Grade(item) != want[item] {
					t.Errorf("lead %s: got %s, want %s", item, lead.Grade(item), want[item])
				}
				if worker.Grade(item) != GradeUnsupported {
					t.Errorf("worker %s is supported", item)
				}
			}
			if !reflect.DeepEqual(lead.Items[ItemSubagentRemoval].Values, []string{"spawn_agent"}) {
				t.Errorf("subagent removal values: %+v", lead.Items[ItemSubagentRemoval])
			}
		})
	}
}

func TestCodexStandaloneLeadProfileAndModes(t *testing.T) {
	if BackendCodex != "codex" || LeadProfileCodex != "lead-codex" || LeadProfileFor(BackendCodex) != LeadProfileCodex {
		t.Fatal("Codex lead identity or fallback missing")
	}
	p, err := ParseProfile(BuiltinProfiles()[ProfilePrefix+LeadProfileCodex])
	if err != nil || !p.Lead || p.Backend != BackendCodex || p.PermissionMode != "" || p.IdleTimeout() != 0 {
		t.Fatalf("standalone lead: %+v, %v", p, err)
	}
	want := []string{"read-only", "workspace-write", "danger-full-access"}
	if !reflect.DeepEqual(CodexPermissionModes, want) || !reflect.DeepEqual(permissionModes(BackendCodex, nil), want) {
		t.Fatalf("native modes: %v", CodexPermissionModes)
	}
	for _, mode := range append(slices.Clone(want), "auto", "never", "bypassPermissions") {
		raw, err := json.Marshal(Profile{Backend: BackendCodex, Lead: true, PermissionMode: mode})
		if err != nil {
			t.Fatal(err)
		}
		_, err = ParseProfile(raw)
		if (err == nil) != slices.Contains(want, mode) {
			t.Errorf("mode %s: %v", mode, err)
		}
	}
	registry := map[string]BackendRegistration{BackendCodex: {LeadProfile: LeadProfileCodex, PermissionModes: []string{"fixture-mode"}}}
	if !reflect.DeepEqual(permissionModes(BackendCodex, []map[string]BackendRegistration{registry}), []string{"fixture-mode"}) {
		t.Fatal("registry must override standalone native mode fallback")
	}
	if LeadProfileFor(BackendCodex, map[string]BackendRegistration{}) != "" {
		t.Fatal("standalone lead fallback must not bypass registry admission")
	}
}

func TestCodexHasNoWorkerAdmission(t *testing.T) {
	for name, raw := range BuiltinProfiles() {
		var p Profile
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Fatal(err)
		}
		if p.Backend == BackendCodex && (!p.Lead || name != ProfilePrefix+LeadProfileCodex) {
			t.Fatalf("unexpected Codex profile: %s", name)
		}
	}
	// The compatibility guard reserves the entire internal namespace;
	// recognizing this spelling does not grant descriptor worker admission.
	if !isAgentWorkerProfile(AgentWorkerProfile(BackendCodex)) {
		t.Fatal("the internal worker namespace must stay reserved for Codex too")
	}
	r := &Runner{opts: Options{Backends: map[string]BackendRegistration{
		BackendCodex: {Backend: PiBackend{}, LeadProfile: LeadProfileCodex},
	}}}
	if slices.Contains(r.workerHarnesses(), BackendCodex) || !strings.Contains(r.spawnableHarness(BackendCodex), "hosted leads only") {
		t.Fatal("registered Codex must remain lead-only")
	}
}
