package wsagent

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

func setSpawnProfiles(t *testing.T, h *harness, profiles map[string]string) {
	t.Helper()
	opts := h.config.Config
	opts.RepoRoot = h.root
	resolver := wsconfig.NewResolver(opts, nil, nil, nil)
	for key, raw := range profiles {
		if err := resolver.SetStructured(key, json.RawMessage(raw), wsconfig.ScopeGlobal); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPiMiniLeadLaunchSurface(t *testing.T) {
	h, _ := childHarness(t)
	st, err := h.runner.Spawn(SpawnSpec{Profile: PiMiniLeadProfile, Root: h.root, Prompt: "lead", Tier: "medium", Parent: &leadParent})
	if err != nil {
		t.Fatal(err)
	}
	spec := h.backend.waitProcStart(t, 0)
	if !slices.Equal(spec.Profile.Tools, PiWorkerTools) || spec.WSMCP.Env[EnvAllowedTools] != strings.Join(PiMiniLeadWSTools, ",") {
		t.Fatalf("mini-lead launch surface: %+v", spec)
	}
	args := piArgs(spec, "bridge.ts")
	tools, _ := flagValue(args, "--tools")
	for _, tool := range PiMiniLeadWSTools {
		if !slices.Contains(strings.Split(tools, ","), "mcp__ws__"+strings.ReplaceAll(tool, ".", "_")) {
			t.Errorf("Pi --tools lacks %s: %s", tool, tools)
		}
	}
	if rec := h.record(st.ID); rec.ResolvedDefinition == nil || !reflect.DeepEqual(*rec.ResolvedDefinition, spec.Profile) {
		t.Fatalf("persisted definition must be exactly what launched: %+v", rec)
	}
	// Omission is pi-worker, not the caller's mini-lead profile.
	kid := h.spawnChild(h.token(0), "small", "review")
	if kid.Profile != PiWorkerProfile {
		t.Fatalf("default child profile = %s", kid.Profile)
	}
}

func TestPiProfileSubsetRefusalAndOverrides(t *testing.T) {
	h, _ := childHarness(t)
	h.spawnWorker("medium")
	token := h.token(0)
	code, body := h.tokenCall(token, "spawn", ChildSpawn{Prompt: "x", Tier: "small", Profile: PiMiniLeadProfile})
	if code != http.StatusBadRequest {
		t.Fatalf("worker must refuse mini-lead: %d %s", code, body)
	}
	for _, want := range []string{PiWorkerProfile, PiMiniLeadProfile, "ws_tools.tickets.close", "ws_tools.route.resolve_implement"} {
		if !strings.Contains(body, want) {
			t.Errorf("refusal must name %s: %s", want, body)
		}
	}
	// A mini-lead override can be narrower than a worker. Names have no rank.
	setSpawnProfiles(t, h, map[string]string{
		ProfilePrefix + PiMiniLeadProfile: `{"backend":"pi","tools":["read"],"ws_tools":["git.diff"]}`,
	})
	code, body = h.tokenCall(token, "spawn", ChildSpawn{Prompt: "x", Tier: "small", Profile: PiMiniLeadProfile})
	if code != http.StatusCreated {
		t.Fatalf("a narrower override should be allowed: %d %s", code, body)
	}
	spec := h.backend.waitProcStart(t, 1)
	if !slices.Equal(spec.Profile.Tools, []string{"read"}) || spec.WSMCP.Env[EnvAllowedTools] != "git.diff" {
		t.Fatalf("override not honored: %+v", spec)
	}
}

func TestPiProfileSubsetCapabilities(t *testing.T) {
	parent := Profile{Tools: []string{"read"}, WSTools: []string{"git.diff"}, MCPServers: map[string]json.RawMessage{"docs": json.RawMessage(`{"command":"docs","args":["a"]}`)}}
	for _, tc := range []struct {
		name  string
		child Profile
		want  []string
	}{
		{"subset", parent, nil},
		{"tool", Profile{Tools: []string{"read", "bash"}}, []string{"tools.bash"}},
		{"ws", Profile{WSTools: []string{"git.diff", "tickets.close"}}, []string{"ws_tools.tickets.close"}},
		{"server", Profile{MCPServers: map[string]json.RawMessage{"other": json.RawMessage(`{}`)}}, []string{"mcp_servers.other"}},
		{"changed server", Profile{MCPServers: map[string]json.RawMessage{"docs": json.RawMessage(`{"command":"other"}`)}}, []string{"mcp_servers.docs"}},
		{"equivalent server", Profile{MCPServers: map[string]json.RawMessage{"docs": json.RawMessage(`{ "args": ["a"], "command": "docs" }`)}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := excessProfileTools(tc.child, parent); !slices.Equal(got, tc.want) {
				t.Fatalf("excess = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPiParentSnapshotDoesNotWidenWithConfigOrDaemonRestart(t *testing.T) {
	h, _ := childHarness(t)
	w := h.spawnWorker("medium")
	h.backend.waitProcStart(t, 0)
	before := h.record(w.ID).ResolvedDefinition
	setSpawnProfiles(t, h, map[string]string{
		ProfilePrefix + PiWorkerProfile: string(BuiltinProfiles()[ProfilePrefix+PiMiniLeadProfile]),
	})
	spawn := func() {
		t.Helper()
		_, err := h.runner.Spawn(SpawnSpec{Profile: PiMiniLeadProfile, Root: h.root, Prompt: "x", Tier: "small", Parent: &ParentRef{Kind: ParentConversation, ReplyID: w.ID}})
		if err == nil || !strings.Contains(err.Error(), "ws_tools.tickets.close") {
			t.Fatalf("widened parent config must not grant its running process more tools: %v", err)
		}
	}
	spawn()
	h.runner.Close()
	h.start()
	if rec := h.record(w.ID); !reflect.DeepEqual(rec.ResolvedDefinition, before) {
		t.Fatalf("daemon restart lost the launch snapshot: %+v", rec)
	}
	spawn()
}

func TestPiChildWidenedProfileRefusedOnRestart(t *testing.T) {
	h, _ := childHarness(t)
	w := h.spawnWorker("medium")
	h.backend.waitProcStart(t, 0)
	kid := h.spawnChild(h.token(0), "small", "work")
	h.backend.waitProc(t, 1).nextSent(t)
	if _, err := h.runner.Stop(kid.ID); err != nil {
		t.Fatal(err)
	}
	setSpawnProfiles(t, h, map[string]string{
		ProfilePrefix + PiWorkerProfile: string(BuiltinProfiles()[ProfilePrefix+PiMiniLeadProfile]),
	})
	if _, err := h.runner.Send(kid.ID, "again"); err != nil {
		t.Fatal(err)
	}
	res, err := h.runner.Wait(context.Background(), kid.ID, 5*time.Second)
	if err != nil || res.Status != TurnFailed || !strings.Contains(res.Error, "ws_tools.tickets.close") {
		t.Fatalf("widened child restart must fail: %+v, %v", res, err)
	}
	if h.backend.startCount() != 2 || slices.Contains(h.record(w.ID).ResolvedDefinition.WSTools, "tickets.close") {
		t.Fatal("refused restart must launch nothing or widen the parent's snapshot")
	}
}

func TestPiChildWidenedBetweenSpawnAndStartRefused(t *testing.T) {
	h, _ := childHarness(t)
	w := h.spawnWorker("medium")
	h.backend.waitProcStart(t, 0)
	setSpawnProfiles(t, h, map[string]string{ProfilePrefix + "custom-pi": `{"backend":"pi","tools":["read"]}`})
	gate := h.keys.gateMint(t, h.root)
	type result struct {
		st  State
		err error
	}
	done := make(chan result, 1)
	go func() {
		st, err := h.runner.Spawn(SpawnSpec{Profile: "custom-pi", Root: h.root, Prompt: "work", Tier: "small", Parent: &ParentRef{Kind: ParentConversation, ReplyID: w.ID}})
		done <- result{st, err}
	}()
	gate.awaitEntered(t) // Creation has resolved and validated; first start has not.
	setSpawnProfiles(t, h, map[string]string{ProfilePrefix + "custom-pi": `{"backend":"pi","tools":["read"],"ws_tools":["tickets.close"]}`})
	gate.release()
	got := <-done
	if got.err != nil {
		t.Fatal(got.err)
	}
	res, err := h.runner.Wait(context.Background(), got.st.ID, 5*time.Second)
	if err != nil || res.Status != TurnFailed || !strings.Contains(res.Error, "custom-pi") || !strings.Contains(res.Error, "tickets.close") || h.backend.startCount() != 1 {
		t.Fatalf("first start must re-check changed child: %+v, %v", res, err)
	}
}

func TestPiLeadCanSelectCustomProfileAndRefusesOtherBackends(t *testing.T) {
	h, _ := childHarness(t)
	setSpawnProfiles(t, h, map[string]string{
		ProfilePrefix + "custom-pi":  `{"backend":"pi","tools":["read"],"ws_tools":["tickets.close"],"mcp_servers":{"docs":{"command":"docs"}}}`,
		ProfilePrefix + "not-pi":     `{"backend":"claude"}`,
		ProfilePrefix + "not-worker": `{"backend":"pi","lead":true}`,
	})
	st, err := h.runner.Spawn(SpawnSpec{Profile: "custom-pi", Root: h.root, Prompt: "work", Tier: "small", Parent: &leadParent})
	if err != nil {
		t.Fatal(err)
	}
	if spec := h.backend.waitProcStart(t, 0); spec.Profile.MCPServers["docs"] == nil || st.Profile != "custom-pi" {
		t.Fatalf("lead custom profile not honored: %+v", spec)
	}
	for _, profile := range []string{"not-pi", "not-worker", "unknown-profile"} {
		if _, err := h.runner.Spawn(SpawnSpec{Profile: profile, Root: h.root, Prompt: "x", Tier: "small", Parent: &leadParent}); err == nil {
			t.Errorf("must refuse %s", profile)
		}
	}
}

func TestPiLegacyRecordSnapshotsOnLaunch(t *testing.T) {
	h, _ := childHarness(t)
	st := h.create(PiWorkerProfile)
	if h.record(st.ID).ResolvedDefinition != nil {
		t.Fatal("an unlaunched record has no launch snapshot")
	}
	h.runner.Close()
	h.start()
	if _, err := h.runner.Send(st.ID, "first launch"); err != nil {
		t.Fatal(err)
	}
	spec := h.backend.waitProcStart(t, 0)
	if rec := h.record(st.ID); rec.ResolvedDefinition == nil || !reflect.DeepEqual(*rec.ResolvedDefinition, spec.Profile) {
		t.Fatalf("legacy record must snapshot the first launch: %+v", rec)
	}
}
