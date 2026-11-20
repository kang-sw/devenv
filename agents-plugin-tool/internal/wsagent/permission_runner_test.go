package wsagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

func permissionHarness(t *testing.T) *leadHarness {
	t.Helper()
	h := newLeadHarness(t)
	for name, reg := range h.opts.Backends {
		reg.PermissionLevels = LeadPermissionLevels()
		h.opts.Backends[name] = reg
	}
	return h
}

func TestPermissionLaunchMatrix(t *testing.T) {
	literalModes := map[PermissionLevel]string{PermissionReadOnly: "plan", PermissionWorkdirWrite: "acceptEdits", PermissionFull: "bypassPermissions"}
	for _, backend := range []string{BackendClaude, BackendPi} {
		for _, level := range LeadPermissionLevels() {
			t.Run(backend+"/"+string(level), func(t *testing.T) {
				h := permissionHarness(t).start()
				st := h.lead(backend, LeadOptions{PermissionLevel: level, PermissionFullAck: level == PermissionFull, permissionUser: true})
				if st.PermissionLevel != level || st.PermissionSource != "explicit" || st.PermissionEnforcement != "advisory" || st.Parent != nil {
					t.Fatalf("root pending state: %+v", st)
				}
				h.send(st.ID, "inspect")
				holder := h.holder(st.ID)
				args := h.holders.spawned(holder).Argv
				mode, present := flagValue(args, "--permission-mode")
				if backend == BackendClaude && (!present || mode != literalModes[level]) {
					t.Fatalf("Claude launch: %v", args)
				}
				if backend == BackendPi && present {
					t.Fatalf("invented Pi mode: %v", args)
				}
				if got := h.log.String(); !strings.Contains(got, "Permission "+string(level)+" is advisory on "+backend+":") {
					t.Fatalf("missing launch nudge: %s", got)
				}
				if backend == BackendClaude {
					// A requested mode is not evidence; mismatching observation stays advisory.
					h.holders.out(holder, initLine("permission-session", "auto"))
					h.sync(st.ID, holder)
					if got := h.state(st.ID); got.ActiveMode != "auto" || got.PermissionEnforcement != "advisory" || got.PermissionLevel != level {
						t.Fatalf("mismatch: %+v", got)
					}
					h.holders.out(holder, initLine("permission-session", "plan"))
					h.sync(st.ID, holder)
					want := "advisory"
					if level == PermissionReadOnly {
						want = "partial"
					}
					if got := h.state(st.ID); got.PermissionEnforcement != want {
						t.Fatalf("observation: %+v", got)
					}
				}
				rec := h.record(st.ID)
				raw, err := os.ReadFile(recordPath(wsdashboard.SessionDir(h.dir, st.ID)))
				if err != nil {
					t.Fatal(err)
				}
				var members map[string]json.RawMessage
				_ = json.Unmarshal(raw, &members)
				if _, ok := members["permission_level"]; ok || rec.ResolvedDefinition != nil {
					t.Fatalf("common state leaked into record: %s", raw)
				}
			})
		}
	}
}

func TestPermissionLegacyNoWriteback(t *testing.T) {
	for _, backend := range []string{BackendClaude, BackendPi} {
		t.Run(backend, func(t *testing.T) {
			h := permissionHarness(t).start()
			st := h.lead(backend, LeadOptions{})
			if st.PermissionLevel != PermissionWorkdirWrite || st.PermissionSource != "legacy-derived" || st.PermissionEnforcement != "advisory" {
				t.Fatalf("legacy: %+v", st)
			}
			h.send(st.ID, "legacy")
			args := h.holders.spawned(h.holder(st.ID)).Argv
			mode, present := flagValue(args, "--permission-mode")
			if backend == BackendClaude && (!present || mode != "auto") {
				t.Fatalf("changed auto: %v", args)
			}
			if backend == BackendPi && present {
				t.Fatalf("invented mode: %v", args)
			}
			if _, err := os.Stat(filepath.Join(wsdashboard.SessionDir(h.dir, st.ID), "permission.json")); !os.IsNotExist(err) {
				t.Fatalf("legacy wrote sidecar: %v", err)
			}
		})
	}
}

func TestPermissionFullUserGate(t *testing.T) {
	h := permissionHarness(t).start()
	for _, opts := range []LeadOptions{
		{PermissionLevel: PermissionFull, PermissionFullAck: true},
		{PermissionLevel: PermissionFull, permissionUser: true},
		{PermissionLevel: PermissionFull, PermissionMode: "bypassPermissions", PermissionFullAck: true},
	} {
		if st, err := h.runner.CreateLead(BackendClaude, h.root, opts); err == nil || st.ID != "" {
			t.Fatalf("untrusted full create: %+v %v", st, err)
		}
	}
	if h.keys.n != 0 || len(h.runner.List()) != 0 {
		t.Fatal("refusal minted or created")
	}
	st := h.lead(BackendClaude, LeadOptions{PermissionLevel: PermissionReadOnly})
	if _, err := h.runner.SetPermissionLevel(st.ID, PermissionWorkdirWrite); requestStatus(err) != 403 {
		t.Fatalf("agent raise: %v", err)
	}
	if _, err := h.runner.SetPermissionMode(st.ID, "acceptEdits"); requestStatus(err) != 403 {
		t.Fatalf("native agent raise: %v", err)
	}
	if _, err := h.runner.setPermissionMode(st.ID, "bypassPermissions", true); requestStatus(err) != 403 {
		t.Fatalf("native bypass omitted explicit ack: %v", err)
	}
	assertPermissionSelection(t, wsdashboard.SessionDir(h.dir, st.ID), PermissionReadOnly, false)
}

func TestPermissionWorkerFullUserGate(t *testing.T) {
	h, _ := childHarness(t)
	before := len(h.runner.List())
	for name, spec := range map[string]SpawnSpec{
		"untrusted-ack": {Profile: PiWorkerProfile, Root: h.root, Prompt: "work", PermissionLevel: PermissionFull, PermissionFullAck: true},
		"user-no-ack":   {Profile: PiWorkerProfile, Root: h.root, Prompt: "work", PermissionLevel: PermissionFull, permissionUser: true},
	} {
		t.Run(name, func(t *testing.T) {
			if st, err := h.runner.Spawn(spec); err == nil || st.ID != "" {
				t.Fatalf("generic full worker accepted: %+v %v", st, err)
			}
		})
	}
	if len(h.runner.List()) != before {
		t.Fatal("refused generic full worker created a conversation")
	}
	st, err := h.runner.Spawn(SpawnSpec{
		Profile: PiWorkerProfile, Root: h.root, Prompt: "trusted full", PermissionLevel: PermissionFull,
		PermissionFullAck: true, permissionUser: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if spec := h.backend.waitProcStart(t, 0); spec.PermissionLevel != PermissionFull {
		t.Fatalf("trusted full worker input: %+v", spec)
	}
	if delegatedPermission(wsdashboard.SessionDir(h.dir, st.ID)) {
		t.Fatal("generic trusted worker recorded agent.spawn delegation")
	}
}

func TestPermissionSelectionLoweringRecovery(t *testing.T) {
	h := permissionHarness(t).start()
	st := h.lead(BackendClaude, LeadOptions{PermissionLevel: PermissionFull, PermissionFullAck: true, permissionUser: true})
	h.send(st.ID, "first")
	holder := h.holder(st.ID)
	h.holders.out(holder, initLine("permission-resume", "bypassPermissions"), replyLine("p1", "done"), resultLine(), stateLine(HarnessIdle))
	h.idle(st.ID)
	lowered, err := h.runner.SetPermissionLevel(st.ID, PermissionReadOnly)
	if err != nil || lowered.PermissionLevel != PermissionReadOnly {
		t.Fatalf("lower: %+v %v", lowered, err)
	}
	dir := wsdashboard.SessionDir(h.dir, st.ID)
	assertPermissionSelection(t, dir, PermissionReadOnly, false)
	h.holders.out(holder, initLine("permission-resume", "plan"))
	h.sync(st.ID, holder)
	h.restart()
	h.quiesce(st.ID)
	if h.holders.opCount("launch") != 1 {
		t.Fatalf("reattach duplicated process: %v", h.holders.opList())
	}
	if got := h.state(st.ID); got.PermissionLevel != PermissionReadOnly || got.PermissionSource != "explicit" {
		t.Fatalf("rejoin: %+v", got)
	}
	if _, err := h.runner.Restart(st.ID); err != nil {
		t.Fatal(err)
	}
	holder = h.holder(st.ID)
	mode, _ := flagValue(h.holders.spawned(holder).Argv, "--permission-mode")
	if mode != "plan" {
		t.Fatalf("restart restored higher mode: %s", mode)
	}
	if _, err := h.runner.CloseConversation(st.ID); err != nil {
		t.Fatal(err)
	}
	if got := h.state(st.ID); got.PermissionLevel != PermissionReadOnly || got.PermissionEnforcement != "advisory" {
		t.Fatalf("closed observation stale: %+v", got)
	}
	if _, err := h.runner.Reopen(st.ID); err != nil {
		t.Fatal(err)
	}
	h.send(st.ID, "revive")
	holder = h.holder(st.ID)
	mode, _ = flagValue(h.holders.spawned(holder).Argv, "--permission-mode")
	if mode != "plan" {
		t.Fatalf("revive: %s", mode)
	}
	assertPermissionSelection(t, dir, PermissionReadOnly, false)
}

func TestPermissionCorruptLaunchAdmission(t *testing.T) {
	h := permissionHarness(t).start()
	st := h.lead(BackendPi, LeadOptions{PermissionLevel: PermissionReadOnly})
	path := filepath.Join(wsdashboard.SessionDir(h.dir, st.ID), "permission.json")
	if err := os.WriteFile(path, []byte(`{"permission_level":"broken"}`), 0600); err != nil {
		t.Fatal(err)
	}
	h.send(st.ID, "must not start")
	if got := h.state(st.ID); got.Process || !strings.Contains(got.Error, "invalid permission_level") {
		t.Fatalf("corrupt launch: %+v", got)
	}
	if h.holders.opCount("launch") != 0 {
		t.Fatal("corrupt selection launched")
	}
	if _, err := h.runner.SetPermissionLevel(st.ID, PermissionReadOnly); err == nil {
		t.Fatal("corrupt selection silently repaired")
	}
}

type permissionRootKeys struct {
	*fakeLeadKeys
	reject bool
}

func (k *permissionRootKeys) EnsureLead(root, key string) error {
	if k.reject {
		return errors.New("root identity unavailable")
	}
	return k.fakeLeadKeys.EnsureLead(root, key)
}
func TestPermissionRootAdmission(t *testing.T) {
	h := permissionHarness(t)
	keys := &permissionRootKeys{fakeLeadKeys: h.keys, reject: true}
	h.opts.LeadKeys = keys
	h.start()
	st := h.lead(BackendPi, LeadOptions{PermissionLevel: PermissionWorkdirWrite})
	if st.Parent != nil {
		t.Fatal("fabricated root parent")
	}
	h.send(st.ID, "root")
	if got := h.state(st.ID); got.Process || !strings.Contains(got.Error, "root identity unavailable") {
		t.Fatalf("root admission: %+v", got)
	}
	if h.holders.opCount("launch") != 0 {
		t.Fatal("invalid root started")
	}
}

type permissionGateBackend struct {
	Backend
	entered, release chan struct{}
	once             sync.Once
}

func (b *permissionGateBackend) Start(ctx context.Context, spec StartSpec) (Process, error) {
	b.once.Do(func() { close(b.entered); <-b.release })
	return b.Backend.Start(ctx, spec)
}
func TestPermissionConcurrentUpdateStart(t *testing.T) {
	h := permissionHarness(t)
	reg := h.opts.Backends[BackendClaude]
	gate := &permissionGateBackend{Backend: reg.Backend, entered: make(chan struct{}), release: make(chan struct{})}
	reg.Backend = gate
	h.opts.Backends[BackendClaude] = reg
	h.start()
	st := h.lead(BackendClaude, LeadOptions{PermissionLevel: PermissionFull, PermissionFullAck: true, permissionUser: true})
	if _, err := h.runner.Send(st.ID, "start"); err != nil {
		t.Fatal(err)
	}
	wait(t, "backend entry", gate.entered)
	lowered := make(chan error, 1)
	go func() { _, err := h.runner.SetPermissionLevel(st.ID, PermissionReadOnly); lowered <- err }()
	select {
	case err := <-lowered:
		t.Fatalf("lowering acknowledged during stale start: %v", err)
	default:
	}
	close(gate.release)
	if err := <-lowered; err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.Stop(st.ID); err != nil {
		t.Fatal(err)
	}
	h.send(st.ID, "after acknowledged lowering")
	holder := h.holder(st.ID)
	mode, _ := flagValue(h.holders.spawned(holder).Argv, "--permission-mode")
	if mode != "plan" {
		t.Fatalf("stale check after lowering: %s", mode)
	}
	assertPermissionSelection(t, wsdashboard.SessionDir(h.dir, st.ID), PermissionReadOnly, false)
}

func TestPermissionWorkerInputStateAndControl(t *testing.T) {
	h, _ := childHarness(t)
	var log bytes.Buffer
	h.runner.Close()
	h.config.Log = &log
	h.start()
	st, err := h.runner.Spawn(SpawnSpec{
		Profile: PiWorkerProfile, Root: h.root, Prompt: "work", Tier: "medium",
		Label: "worker", Parent: &leadParent, PermissionLevel: PermissionWorkdirWrite,
		delegatedAuthority: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	spec := h.backend.waitProcStart(t, 0)
	if !slices.Equal(spec.Profile.Tools, PiWorkerTools) || spec.PermissionLevel != PermissionWorkdirWrite {
		t.Fatalf("worker launch input: %+v", spec)
	}
	state, err := h.runner.Get(st.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.PermissionLevel != PermissionWorkdirWrite || state.PermissionSource != "explicit" || state.PermissionEnforcement != "advisory" {
		t.Fatalf("worker State: %+v", state)
	}
	assertPermissionSelection(t, wsdashboard.SessionDir(h.dir, st.ID), PermissionWorkdirWrite, false)
	if got := log.String(); !strings.Contains(got, "Permission workdir-write is advisory on pi: current-parent admission passed") {
		t.Fatalf("authority-accurate worker nudge: %s", got)
	}
	lowered, err := h.runner.SetPermissionLevel(st.ID, PermissionReadOnly)
	if err != nil || lowered.PermissionLevel != PermissionReadOnly || lowered.PermissionSource != "explicit" {
		t.Fatalf("lower worker: %+v %v", lowered, err)
	}
	if !lowered.Process {
		t.Fatal("active worker stopped before its turn boundary")
	}
	if _, err := h.runner.SetPermissionLevel(st.ID, PermissionWorkdirWrite); requestStatus(err) != http.StatusForbidden {
		t.Fatalf("worker raise: %v", err)
	}
	if _, err := h.runner.Send(st.ID, "again"); err != nil {
		t.Fatal(err)
	}
	h.backend.waitProc(t, 0).reply("done")
	if next := h.backend.waitProcStart(t, 1); next.PermissionLevel != PermissionReadOnly {
		t.Fatalf("lowered restart input: %+v", next)
	}
	if !h.backend.proc(0).isClosed() {
		t.Fatal("worker lowering did not replace the prior process at the turn boundary")
	}
	root, err := h.runner.Create(PiWorkerProfile, h.root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.Send(root.ID, "root work"); err != nil {
		t.Fatal(err)
	}
	h.backend.waitProcStart(t, 2)
	if got := log.String(); !strings.Contains(got, "Permission workdir-write is advisory on pi: root worker has no authority parent") {
		t.Fatalf("root worker nudge: %s", got)
	}
}

func TestPermissionCurrentParentAdmission(t *testing.T) {
	h := permissionHarness(t).start()
	parent := h.lead(BackendClaude, LeadOptions{PermissionLevel: PermissionWorkdirWrite})
	parentRef := ParentRef{Kind: ParentLead, ReplyID: parent.ID}
	child, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "work", Parent: &parentRef})
	if err != nil {
		t.Fatal(err)
	}
	h.holder(child.ID)
	if got := h.state(child.ID); got.PermissionLevel != PermissionWorkdirWrite || got.PermissionSource != "legacy-derived" || got.PermissionEnforcement != "advisory" {
		t.Fatalf("legacy worker projection: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(wsdashboard.SessionDir(h.dir, child.ID), "permission.json")); !os.IsNotExist(err) {
		t.Fatalf("legacy worker wrote selection: %v", err)
	}
	if _, err := h.runner.Stop(child.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.SetPermissionLevel(parent.ID, PermissionReadOnly); err != nil {
		t.Fatal(err)
	}
	h.send(child.ID, "wake below the new ceiling")
	if got := h.state(child.ID); got.Process || !strings.Contains(got.Error, "authority workdir-write is above the parent's current level read-only") {
		t.Fatalf("worker started above current parent: %+v", got)
	}
	if _, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "new child", Parent: &parentRef}); err == nil || !strings.Contains(err.Error(), "above the parent's current level read-only") {
		t.Fatalf("creation admission: %v", err)
	}
}

func TestPermissionMissingParentAdmission(t *testing.T) {
	h, _ := childHarness(t)
	before := len(h.runner.List())
	missing := ParentRef{Kind: ParentConversation, ReplyID: "missingparentrecord"}
	_, err := h.runner.Spawn(SpawnSpec{
		Profile: AgentWorkerProfile(BackendPi), Root: h.root, Prompt: "work", Parent: &missing,
		PermissionLevel: PermissionWorkdirWrite, delegatedAuthority: true,
	})
	if err == nil || !strings.Contains(err.Error(), "has no record") {
		t.Fatalf("missing parent admission: %v", err)
	}
	if len(h.runner.List()) != before {
		t.Fatal("missing-parent refusal created a conversation")
	}
	missingLead := ParentRef{Kind: ParentLead, ReplyID: "missingleadrecord"}
	_, err = h.runner.Spawn(SpawnSpec{
		Profile: PiWorkerProfile, Root: h.root, Prompt: "explicit generic work", Parent: &missingLead,
		PermissionLevel: PermissionWorkdirWrite,
	})
	if err == nil || !strings.Contains(err.Error(), "parent lead") || !strings.Contains(err.Error(), "has no hosted record") {
		t.Fatalf("missing generic lead admission: %v", err)
	}
	if len(h.runner.List()) != before {
		t.Fatal("missing-lead refusal created a conversation")
	}
	legacy, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "legacy work", Parent: &missingLead})
	if err != nil {
		t.Fatalf("legacy detached-lead compatibility: %v", err)
	}
	if legacy.PermissionLevel != PermissionWorkdirWrite || legacy.PermissionSource != "legacy-derived" {
		t.Fatalf("legacy detached-lead projection: %+v", legacy)
	}
	if _, err := os.Stat(filepath.Join(wsdashboard.SessionDir(h.dir, legacy.ID), "permission.json")); !os.IsNotExist(err) {
		t.Fatalf("legacy detached-lead spawn wrote permission sidecar: %v", err)
	}
	delegated, err := h.runner.Spawn(SpawnSpec{
		Profile: PiWorkerProfile, Root: h.root, Prompt: "delegated work", Parent: &missingLead,
		PermissionLevel: PermissionWorkdirWrite, delegatedAuthority: true,
	})
	if err != nil {
		t.Fatalf("agent.spawn detached-lead exception: %v", err)
	}
	if !delegatedPermission(wsdashboard.SessionDir(h.dir, delegated.ID)) {
		t.Fatal("agent.spawn authority provenance was not persisted")
	}
	root, err := h.runner.Create(PiWorkerProfile, h.root)
	if err != nil {
		t.Fatal(err)
	}
	if root.Parent != nil || root.PermissionLevel != PermissionWorkdirWrite || root.PermissionSource != "legacy-derived" {
		t.Fatalf("root worker admission: %+v", root)
	}
}

func TestPermissionConcurrentParentLoweringWorkerStart(t *testing.T) {
	h, _ := childHarness(t)
	parent, err := h.runner.Create(PiWorkerProfile, h.root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.Send(parent.ID, "establish parent capability snapshot"); err != nil {
		t.Fatal(err)
	}
	h.backend.waitProc(t, 0)
	if _, err := h.runner.Stop(parent.ID); err != nil {
		t.Fatal(err)
	}
	gate := h.backend.gateStarts(t)
	parentRef := ParentRef{Kind: ParentConversation, ReplyID: parent.ID}
	child, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "work", Parent: &parentRef})
	if err != nil {
		t.Fatal(err)
	}
	gate.awaitEntered(t)
	lowered := make(chan error, 1)
	go func() {
		_, err := h.runner.SetPermissionLevel(parent.ID, PermissionReadOnly)
		lowered <- err
	}()
	select {
	case err := <-lowered:
		t.Fatalf("parent lowering acknowledged during child start: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	gate.release()
	if err := <-lowered; err != nil {
		t.Fatal(err)
	}
	if spec := h.backend.waitProcStart(t, 1); spec.PermissionLevel != PermissionWorkdirWrite {
		t.Fatalf("serialized child launch: %+v", spec)
	}
	if _, err := h.runner.Stop(child.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.Send(child.ID, "wake above lowered parent"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "post-lowering launch refusal", func() bool {
		got := h.state(child.ID)
		return !got.Process && strings.Contains(got.Error, "above the parent's current level read-only")
	})
}

func TestPermissionNativeParentLoweringWaitsForAdmission(t *testing.T) {
	h := permissionHarness(t).start()
	parent := h.lead(BackendClaude, LeadOptions{
		PermissionLevel: PermissionFull, PermissionFullAck: true, permissionUser: true,
	})
	parentRef := ParentRef{Kind: ParentLead, ReplyID: parent.ID}
	releaseAdmission := h.runner.holdParentAdmission(&parentRef)

	lowered := make(chan error, 1)
	go func() {
		_, err := h.runner.setPermissionMode(parent.ID, "plan", true)
		lowered <- err
	}()
	select {
	case err := <-lowered:
		t.Fatalf("native parent lowering acknowledged during child admission: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	releaseAdmission()
	if err := <-lowered; err != nil {
		t.Fatal(err)
	}
	if got := h.state(parent.ID); got.PermissionLevel != PermissionReadOnly {
		t.Fatalf("native parent lowering was not persisted: %+v", got)
	}
}

func TestPermissionLegacyWorkerMixedLoadouts(t *testing.T) {
	h := permissionHarness(t).start()
	parent := h.lead(BackendClaude, LeadOptions{PermissionLevel: PermissionWorkdirWrite})
	parentRef := ParentRef{Kind: ParentLead, ReplyID: parent.ID}
	h.globalProfile("mixed-create", `{"backend":"pi","tools":["bash","mystery"]}`)
	before := len(h.runner.List())
	if _, err := h.runner.Spawn(SpawnSpec{Profile: "mixed-create", Root: h.root, Prompt: "work", Parent: &parentRef}); err == nil || !strings.Contains(err.Error(), "own permission level is unresolved") {
		t.Fatalf("mixed creation admission: %v", err)
	}
	if len(h.runner.List()) != before {
		t.Fatal("mixed creation refusal created a conversation")
	}

	h.globalProfile("mixed-restart", `{"backend":"pi","tools":["bash"]}`)
	child, err := h.runner.Spawn(SpawnSpec{Profile: "mixed-restart", Root: h.root, Prompt: "work", Parent: &parentRef})
	if err != nil {
		t.Fatal(err)
	}
	h.holder(child.ID)
	if _, err := h.runner.Stop(child.ID); err != nil {
		t.Fatal(err)
	}
	h.globalProfile("mixed-restart", `{"backend":"pi","tools":["bash","mystery"]}`)
	h.send(child.ID, "restart with changed capabilities")
	if got := h.state(child.ID); got.Process || !strings.Contains(got.Error, "own permission level is unresolved") {
		t.Fatalf("mixed restart admission: %+v", got)
	}

	for name, def := range map[string]Profile{
		"plugin":     {Tools: slices.Clone(PiWorkerTools), Plugins: []string{"unsafe"}},
		"unknown-ws": {Tools: slices.Clone(PiWorkerTools), WSTools: []string{"git.status", "unknown.tool"}},
	} {
		if got := legacyWorkerPermission(def); got != "" {
			t.Errorf("%s legacy level = %q", name, got)
		}
	}
}

func permissionDeliveredModes(h *leadHarness, holder string) []string {
	var modes []string
	for _, control := range h.controls(holder) {
		if body, ok := control.(map[string]any); ok && body["subtype"] == "set_permission_mode" {
			modes = append(modes, body["mode"].(string))
		}
	}
	return modes
}

func permissionRejoined(h *leadHarness, id string) {
	h.t.Helper()
	eventually(h.t, "permission holder replay to finish", func() bool {
		ready := false
		c := h.runner.conv(id)
		return c != nil && c.do(func() { ready = !c.replaying }) && ready
	})
}

func TestPermissionNativeModeLiteralDelivery(t *testing.T) {
	h := permissionHarness(t).start()
	st := h.lead(BackendClaude, LeadOptions{PermissionLevel: PermissionWorkdirWrite})
	h.send(st.ID, "native mode")
	holder := h.holder(st.ID)
	h.holders.out(holder, initLine("native-literal", "acceptEdits"), resultLine(), stateLine(HarnessIdle))
	h.idle(st.ID)
	res := h.route(localBrowser, http.MethodPost, "/agents/"+st.ID+"/settings", map[string]any{"permission_mode": "auto"})
	if res.status != http.StatusOK {
		t.Fatalf("trusted native auto: %d %s", res.status, res.body)
	}
	if modes := permissionDeliveredModes(h, holder); !slices.Equal(modes, []string{"auto"}) {
		t.Fatalf("delivered modes = %v; want literal auto", modes)
	}
	if got := h.record(st.ID); got.PermissionMode != "auto" {
		t.Fatalf("recorded native target: %+v", got)
	}
	if got := h.state(st.ID); got.PermissionLevel != PermissionWorkdirWrite || got.ActiveMode != "acceptEdits" {
		t.Fatalf("intent/observation conflated: %+v", got)
	}
	h.holders.out(holder, initLine("native-literal", "auto"))
	h.sync(st.ID, holder)
	h.restart()
	h.sync(st.ID, holder)
	permissionRejoined(h, st.ID)
	if modes := permissionDeliveredModes(h, holder); !slices.Equal(modes, []string{"auto"}) {
		t.Fatalf("satisfied native target was remapped on attach: %v", modes)
	}
	if h.holders.opCount("launch") != 1 || h.holder(st.ID) != holder {
		t.Fatalf("native control/reattach replaced process: %v", h.holders.opList())
	}
	assertPermissionSelection(t, wsdashboard.SessionDir(h.dir, st.ID), PermissionWorkdirWrite, false)
}

// A crash can persist the common selection before the native Record target.
// Rejoin must reconstruct the lower target rather than replay the stale mode.
func TestPermissionInconsistentRecordReattachDelivery(t *testing.T) {
	for _, refuse := range []bool{false, true} {
		t.Run(map[bool]string{false: "delivered", true: "unenforceable"}[refuse], func(t *testing.T) {
			h := permissionHarness(t).start()
			st := h.lead(BackendClaude, LeadOptions{PermissionLevel: PermissionFull, PermissionFullAck: true, permissionUser: true})
			h.send(st.ID, "original input")
			holder := h.holder(st.ID)
			h.holders.out(holder, initLine("inconsistent-record", "bypassPermissions"), stateLine(HarnessRunning))
			h.sync(st.ID, holder)
			original := h.userInputs(holder)
			h.runner.Close()
			h.holders.restart()
			dir := wsdashboard.SessionDir(h.dir, st.ID)
			if err := writePermissionSelection(dir, PermissionReadOnly); err != nil {
				t.Fatal(err)
			}
			if rec := h.record(st.ID); rec.PermissionMode != "bypassPermissions" {
				t.Fatalf("inconsistent-record prerequisite missing: %+v", rec)
			}
			h.start()
			h.sync(st.ID, holder)
			permissionRejoined(h, st.ID)
			if got := h.state(st.ID); !got.Process || got.PermissionLevel != PermissionReadOnly || got.PermissionMode != "plan" || got.ActiveMode != "bypassPermissions" || got.PermissionEnforcement != "advisory" {
				t.Fatalf("reconstructed target/observation: %+v", got)
			}
			if modes := permissionDeliveredModes(h, holder); len(modes) != 0 {
				t.Fatalf("control crossed active-turn boundary: %v", modes)
			}
			if refuse {
				h.holders.setControl("set_permission_mode", "mode cannot be enforced", "")
			}
			h.holders.out(holder, resultLine(), stateLine(HarnessIdle))
			h.idle(st.ID)
			if modes := permissionDeliveredModes(h, holder); !slices.Equal(modes, []string{"plan"}) {
				t.Fatalf("recovered control = %v; want plan", modes)
			}
			if rec := h.record(st.ID); rec.PermissionMode != "plan" {
				t.Fatalf("stale native target retained: %+v", rec)
			}
			if got := h.state(st.ID); !got.Process || got.ActiveMode != "bypassPermissions" || got.PermissionEnforcement != "advisory" {
				t.Fatalf("control fabricated observation: %+v", got)
			}
			if refuse && !strings.Contains(h.log.String(), "permission reconciliation: mode cannot be enforced") {
				t.Fatalf("refusal not diagnosed: %s", h.log.String())
			}
			if h.holders.opCount("launch") != 1 || h.holders.opCount("stop") != 0 || h.holder(st.ID) != holder || !slices.Equal(h.userInputs(holder), original) {
				t.Fatalf("rejoin duplicated process/input: %v; %q", h.holders.opList(), h.userInputs(holder))
			}
			assertPermissionSelection(t, dir, PermissionReadOnly, false)
		})
	}
}

func TestPermissionPendingLoweringReattachDelivery(t *testing.T) {
	for _, refuse := range []bool{false, true} {
		t.Run(map[bool]string{false: "delivered", true: "unenforceable"}[refuse], func(t *testing.T) {
			h := permissionHarness(t).start()
			st := h.lead(BackendClaude, LeadOptions{PermissionLevel: PermissionFull, PermissionFullAck: true, permissionUser: true})
			h.send(st.ID, "original input")
			holder := h.holder(st.ID)
			h.holders.out(holder, initLine("pending-lower", "bypassPermissions"), stateLine(HarnessRunning))
			h.sync(st.ID, holder)
			original := h.userInputs(holder)
			if len(original) != 1 {
				t.Fatalf("original input count: %q", original)
			}
			if _, err := h.runner.SetPermissionLevel(st.ID, PermissionReadOnly); err != nil {
				t.Fatal(err)
			}
			if modes := permissionDeliveredModes(h, holder); len(modes) != 0 {
				t.Fatalf("control ran during active turn: %v", modes)
			}
			if got := h.state(st.ID); got.ActiveMode != "bypassPermissions" || got.PermissionEnforcement != "advisory" {
				t.Fatalf("premature enforcement: %+v", got)
			}
			// No plan observation or idle boundary exists before replacement.
			// The successor must replay the old bypass observation and reconstruct
			// the pending control through this same holder, not its ignored flags.
			h.restart()
			h.sync(st.ID, holder)
			permissionRejoined(h, st.ID)
			if got := h.state(st.ID); got.ActiveMode != "bypassPermissions" || got.Status != StatusRunning {
				t.Fatalf("old bypass replay missing: %+v", got)
			}
			if modes := permissionDeliveredModes(h, holder); len(modes) != 0 {
				t.Fatalf("replay crossed unsafe boundary: %v", modes)
			}
			if refuse {
				h.holders.setControl("set_permission_mode", "mode cannot be enforced", "")
			}
			h.holders.out(holder, replyLine("pending1", "done"), resultLine(), stateLine(HarnessIdle))
			h.idle(st.ID)
			if modes := permissionDeliveredModes(h, holder); !slices.Equal(modes, []string{"plan"}) {
				t.Fatalf("recovered control = %v; want literal plan", modes)
			}
			if h.holders.opCount("launch") != 1 || h.holder(st.ID) != holder || !slices.Equal(h.userInputs(holder), original) {
				t.Fatalf("reconciliation duplicated process/input: %v; %q", h.holders.opList(), h.userInputs(holder))
			}
			if got := h.state(st.ID); !got.Process || got.ActiveMode != "bypassPermissions" || got.PermissionEnforcement != "advisory" {
				t.Fatalf("control acknowledgement fabricated observation: %+v", got)
			}
			if refuse && !strings.Contains(h.log.String(), "permission reconciliation:") {
				t.Fatalf("unenforceability not reported: %s", h.log.String())
			}
			assertPermissionSelection(t, wsdashboard.SessionDir(h.dir, st.ID), PermissionReadOnly, false)
			if refuse {
				return
			}
			// Stop before a plan report, then resume and reopen: each deliberate
			// new process still launches the durable lower level with one input.
			if _, err := h.runner.Stop(st.ID); err != nil {
				t.Fatal(err)
			}
			h.send(st.ID, "after stop")
			resumed := h.holder(st.ID)
			mode, _ := flagValue(h.holders.spawned(resumed).Argv, "--permission-mode")
			// fakeHolders.Stop deletes the old holder and its input history. Its
			// no-duplicate assertion was made above, before the deliberate stop.
			if mode != "plan" || !slices.Equal(h.userInputs(resumed), h.envelopes("after stop")) || h.holders.opCount("launch") != 2 {
				t.Fatalf("stop/resume changed selection or input: %s %q", mode, h.userInputs(resumed))
			}
			if _, err := h.runner.CloseConversation(st.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := h.runner.Reopen(st.ID); err != nil {
				t.Fatal(err)
			}
			h.send(st.ID, "after reopen")
			revived := h.holder(st.ID)
			mode, _ = flagValue(h.holders.spawned(revived).Argv, "--permission-mode")
			if mode != "plan" || !slices.Equal(h.userInputs(revived), h.envelopes("after reopen")) || h.holders.opCount("launch") != 3 {
				t.Fatalf("revive: %s %q", mode, h.userInputs(revived))
			}
			assertPermissionSelection(t, wsdashboard.SessionDir(h.dir, st.ID), PermissionReadOnly, false)
		})
	}
}
