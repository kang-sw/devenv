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
	if p.Oldest != "0.160.0" || p.Newest != "0.160.0" || len(p.Entries) != 2*len(CatalogItems) {
		t.Fatalf("profile bounds or entries: %+v", p)
	}
	seen := map[CatalogRole]map[CapabilityItem]bool{RoleLead: {}, RoleWorker: {}}
	for _, e := range p.Entries {
		if seen[e.Role] == nil || seen[e.Role][e.Item] || e.Since != "" || e.Until != "" || e.Source == "" {
			t.Errorf("invalid entry: %+v", e)
			continue
		}
		seen[e.Role][e.Item] = true
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
				// A worker shares the lead's wiring except approval: its
				// requests are auto-allowed, not asked.
				wantWorker := want[item]
				if item == ItemApproval {
					wantWorker = GradeSubstitute
				}
				if worker.Grade(item) != wantWorker {
					t.Errorf("worker %s: got %s, want %s", item, worker.Grade(item), wantWorker)
				}
			}
			if !reflect.DeepEqual(lead.Items[ItemSubagentRemoval].Values, []string{"spawn_agent"}) ||
				!reflect.DeepEqual(worker.Items[ItemSubagentRemoval].Values, []string{"spawn_agent"}) {
				t.Errorf("subagent removal values: %+v, %+v", lead.Items[ItemSubagentRemoval], worker.Items[ItemSubagentRemoval])
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

func TestCodexWorkerProfileAndAdmission(t *testing.T) {
	profiles := BuiltinProfiles()
	for name, raw := range profiles {
		var p Profile
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Fatal(err)
		}
		if p.Backend == BackendCodex && name != ProfilePrefix+LeadProfileCodex && name != ProfilePrefix+AgentWorkerProfile(BackendCodex) {
			t.Fatalf("unexpected Codex profile: %s", name)
		}
	}
	p, err := ParseProfile(profiles[ProfilePrefix+AgentWorkerProfile(BackendCodex)])
	if err != nil || p.Lead || p.Backend != BackendCodex || !slices.Equal(p.WSTools, AgentWSTools) || len(p.Tools) != 0 {
		t.Fatalf("worker profile: %+v, %v", p, err)
	}
	text, ok := p.BuiltinInstructions()
	if !ok || !strings.Contains(text, "spawn_agent") || !strings.Contains(text, "agent.spawn") || !strings.Contains(text, "mailbox.send") {
		t.Fatalf("worker instructions: %q", text)
	}
	if !slices.Contains(BuiltinInstructionNames(), "agent-worker-codex") || !isAgentWorkerProfile(AgentWorkerProfile(BackendCodex)) {
		t.Fatal("the worker instructions and the reserved namespace must name Codex")
	}
	// Admission is the descriptor's: without worker fields Codex stays
	// lead-only, with them it is a spawnable harness.
	lookup := func(string) (string, error) { return "codex", nil }
	setLookPath(t, lookup)
	r := &Runner{opts: Options{Backends: map[string]BackendRegistration{
		BackendCodex: {Backend: CodexBackend{}, LeadProfile: LeadProfileCodex},
	}}}
	if slices.Contains(r.workerHarnesses(), BackendCodex) || !strings.Contains(r.spawnableHarness(BackendCodex), "hosted leads only") {
		t.Fatal("a Codex registration with no worker fields must remain lead-only")
	}
	r.opts.Backends[BackendCodex] = BackendRegistration{Backend: CodexBackend{}, LeadProfile: LeadProfileCodex, WorkerCapable: true, WorkerProfile: AgentWorkerProfile(BackendCodex)}
	if !slices.Equal(r.workerHarnesses(), []string{BackendCodex}) || r.spawnableHarness(BackendCodex) != "" {
		t.Fatalf("a worker-capable Codex registration must be spawnable: %q", r.spawnableHarness(BackendCodex))
	}
}
