package wsagent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
)

// codexFake is SYNTHETIC protocol output over the existing fake HolderHost,
// not an installed Codex process or a recording. The real CodexBackend,
// stream, holder routing and Runner execute; only the child is simulated.
// Tests control turn completion, approvals, EOF and recovery independently
// of control acknowledgements, as the recorded app-server protocol requires.
type codexFake struct {
	h                    *leadHarness
	mu                   sync.Mutex
	replyID              string
	nextTurn             int
	rowPIDs              []int
	tokens               []string
	recovery             string
	openError            string
	steerError           string
	omitPermissionReport bool
	reportDefaults       bool
	reportedSandbox      string
	reportedPolicy       string
	readGate             <-chan struct{}
	readStarted          chan struct{}
}

func newCodexLeadHarness(t *testing.T) (*leadHarness, *codexFake) {
	t.Helper()
	h := newLeadHarness(t)
	h.opts.Backends[BackendCodex] = BackendRegistration{Backend: CodexBackend{Command: "/opt/harness/codex"}, LeadProfile: LeadProfileCodex, PermissionModes: CodexPermissionModes, PermissionLevels: LeadPermissionLevels(), Catalog: CodexCatalog()}
	h.opts.WSMCP = func(root string) (MCPCommand, error) {
		return MCPCommand{Command: "/opt/harness/python3", Env: map[string]string{"WS_MCP_PROJECT_ROOT": root}}, nil
	}
	f := &codexFake{h: h}
	h.holders.acp = f.answer
	return h, f
}

func (f *codexFake) answer(holder, method string, params json.RawMessage) (string, string, bool) {
	switch method {
	case "initialize":
		return `{"codexHome":"/synthetic/codex-home"}`, "", true
	case "thread/start", "thread/resume":
		var requested struct {
			ApprovalPolicy string `json:"approvalPolicy"`
			Sandbox        string `json:"sandbox"`
		}
		_ = json.Unmarshal(params, &requested)
		spawn := f.h.holders.spawned(holder)
		token := spawn.Env[EnvMCPSessionToken]
		f.mu.Lock()
		f.tokens = append(f.tokens, token)
		openError := f.openError
		if row, err := wsdashboard.ReadRegistration(f.h.dir, f.replyID); err == nil {
			f.rowPIDs = append(f.rowPIDs, row.HostPID)
		} else {
			f.rowPIDs = append(f.rowPIDs, -1)
		}
		f.mu.Unlock()
		if openError != "" {
			return "", openError, true
		}
		// Simulate the HTTP connection during thread initialization. This
		// checks token/context admission, not the real network or Codex MCP.
		f.h.runner.ResolveMCPToken(token)
		session, _ := jsonAt(params, "threadId").(string)
		if session == "" {
			session = "codex-s1"
		}
		result := map[string]any{"thread": map[string]any{"id": session, "cliVersion": "0.160.0", "model": "synthetic-model"}}
		sandbox, policy := requested.Sandbox, requested.ApprovalPolicy
		if f.reportDefaults && sandbox == "" {
			sandbox, policy = "workspace-write", "on-request"
		}
		if !f.omitPermissionReport && policy != "" && sandbox != "" {
			if f.reportedSandbox != "" {
				sandbox = f.reportedSandbox
			}
			if f.reportedPolicy != "" {
				policy = f.reportedPolicy
			}
			result["approvalPolicy"] = policy
			result["sandbox"] = map[string]any{"type": codexSandboxKind(sandbox)}
		}
		return jsonLine(result), "", true
	case "model/list":
		return `{"data":[{"model":"synthetic-model","supportedReasoningEfforts":[{"reasoningEffort":"low"},{"reasoningEffort":"high"}]}]}`, "", true
	case "turn/start":
		var requested struct {
			ThreadID       string `json:"threadId"`
			ApprovalPolicy string `json:"approvalPolicy"`
			SandboxPolicy  struct {
				Type string `json:"type"`
			} `json:"sandboxPolicy"`
		}
		_ = json.Unmarshal(params, &requested)
		f.mu.Lock()
		f.nextTurn++
		turn := fmt.Sprintf("t%d", f.nextTurn)
		f.mu.Unlock()
		kind, policy := requested.SandboxPolicy.Type, requested.ApprovalPolicy
		if f.reportDefaults && kind == "" {
			kind, policy = "workspaceWrite", "on-request"
		}
		if !f.omitPermissionReport && policy != "" && kind != "" {
			if f.reportedSandbox != "" {
				kind = codexSandboxKind(f.reportedSandbox)
			}
			if f.reportedPolicy != "" {
				policy = f.reportedPolicy
			}
			f.h.holders.out(holder, codexSyntheticNotification("thread/settings/updated", map[string]any{
				"threadId":       requested.ThreadID,
				"threadSettings": map[string]any{"approvalPolicy": policy, "sandboxPolicy": map[string]any{"type": kind}},
			}))
		}
		f.h.holders.out(holder, codexSyntheticNotification("turn/started", map[string]any{"threadId": "codex-s1", "turn": map[string]any{"id": turn, "status": "inProgress"}}))
		return jsonLine(map[string]any{"turn": map[string]any{"id": turn, "status": "inProgress"}}), "", true
	case "turn/steer":
		f.mu.Lock()
		steerError := f.steerError
		f.mu.Unlock()
		if steerError != "" {
			return "", steerError, true
		}
		return `{}`, "", true
	case "turn/interrupt", "thread/compact/start":
		return `{}`, "", true
	case "thread/read":
		f.mu.Lock()
		recovery, gate, started := f.recovery, f.readGate, f.readStarted
		f.readStarted = nil
		f.mu.Unlock()
		if started != nil {
			close(started)
		}
		if gate != nil {
			<-gate
		}
		return recovery, "", true
	}
	return "", "", false
}

func codexSandboxKind(mode string) string {
	return map[string]string{"read-only": "readOnly", "workspace-write": "workspaceWrite", "danger-full-access": "dangerFullAccess"}[mode]
}

func TestPermissionCodexLeadPerRun(t *testing.T) {
	for _, tc := range []struct {
		level   PermissionLevel
		sandbox string
		kind    string
		policy  string
		quality string
	}{
		{PermissionReadOnly, "read-only", "readOnly", "on-request", "partial"},
		{PermissionWorkdirWrite, "workspace-write", "workspaceWrite", "on-request", "advisory"},
		{PermissionFull, "danger-full-access", "dangerFullAccess", "never", "advisory"},
	} {
		t.Run(string(tc.level), func(t *testing.T) {
			h, f := newCodexLeadHarness(t)
			h.startAt(codexSyntheticEndpoint)
			st := h.lead(BackendCodex, LeadOptions{
				PermissionLevel: tc.level, PermissionFullAck: tc.level == PermissionFull, permissionUser: true,
			})
			f.replyID = st.ID
			h.send(st.ID, "exercise per-run settings")
			holder := h.holder(st.ID)
			eventually(t, "reported Codex permission settings", func() bool {
				got := h.state(st.ID)
				return got.ActiveMode == tc.sandbox && got.PermissionEnforcement == tc.quality
			})

			open := h.holders.acpInputs(holder, "thread/start")
			turns := h.holders.acpInputs(holder, "turn/start")
			if len(open) != 1 || jsonAt(open[0], "sandbox") != tc.sandbox || jsonAt(open[0], "approvalPolicy") != tc.policy {
				t.Fatalf("thread/start settings: %s", open)
			}
			if jsonAt(open[0], "config", "sandbox") != nil || jsonAt(open[0], "config", "approvalPolicy") != nil || jsonAt(open[0], "config", "approval_policy") != nil {
				t.Fatalf("per-run permission leaked into global Codex config: %s", open[0])
			}
			if len(turns) != 1 || jsonAt(turns[0], "sandboxPolicy", "type") != tc.kind || jsonAt(turns[0], "approvalPolicy") != tc.policy {
				t.Fatalf("turn/start settings: %s", turns)
			}
			got := h.state(st.ID)
			if got.PermissionLevel != tc.level || got.PermissionSource != "explicit" || got.PermissionMode != tc.sandbox || got.ActiveMode != tc.sandbox || got.PermissionEnforcement != tc.quality {
				t.Fatalf("configured and observed permission state: %+v", got)
			}
			assertPermissionSelection(t, wsdashboard.SessionDir(h.dir, st.ID), tc.level, false)
			if !strings.Contains(h.log.String(), "Permission "+string(tc.level)+" is advisory on codex") {
				t.Fatalf("launch nudge missing: %s", h.log.String())
			}
		})
	}

	t.Run("reported settings differ", func(t *testing.T) {
		h, f := newCodexLeadHarness(t)
		f.reportedSandbox, f.reportedPolicy = "read-only", "never"
		h.startAt(codexSyntheticEndpoint)
		st := h.lead(BackendCodex, LeadOptions{PermissionLevel: PermissionReadOnly, permissionUser: true})
		f.replyID = st.ID
		h.send(st.ID, "observe actual settings")
		h.holder(st.ID)
		eventually(t, "mismatched reported approval policy", func() bool {
			got := h.state(st.ID)
			return got.ActiveMode == "read-only (approval never)" && got.PermissionEnforcement == "advisory"
		})
		got := h.state(st.ID)
		if got.PermissionLevel != PermissionReadOnly || got.PermissionMode != "read-only" {
			t.Fatalf("requested intent changed by observation: %+v", got)
		}
	})

	t.Run("unavailable observation", func(t *testing.T) {
		h, f := newCodexLeadHarness(t)
		f.omitPermissionReport = true
		h.startAt(codexSyntheticEndpoint)
		st := h.lead(BackendCodex, LeadOptions{PermissionLevel: PermissionReadOnly, permissionUser: true})
		f.replyID = st.ID
		h.send(st.ID, "no reported settings")
		h.holder(st.ID)
		eventually(t, "pending Codex observation", func() bool { return h.state(st.ID).Process })
		got := h.state(st.ID)
		if got.PermissionLevel != PermissionReadOnly || got.PermissionMode != "read-only" || got.ActiveMode != "" || got.PermissionEnforcement != "advisory" {
			t.Fatalf("unavailable observation must stay advisory: %+v", got)
		}
	})

	t.Run("new process invalidates old observation", func(t *testing.T) {
		h, f := newCodexLeadHarness(t)
		h.startAt(codexSyntheticEndpoint)
		st := h.lead(BackendCodex, LeadOptions{PermissionLevel: PermissionReadOnly, permissionUser: true})
		f.replyID = st.ID
		h.send(st.ID, "reported settings")
		holder := h.holder(st.ID)
		eventually(t, "first process observation", func() bool { return h.state(st.ID).ActiveMode == "read-only" })
		h.holders.out(holder, codexSyntheticEnd("t1", "done", "completed"))
		h.idle(st.ID)
		h.holders.exit(holder, 0)
		eventually(t, "first process exit", func() bool { return !h.state(st.ID).Process })
		c := h.runner.convs[st.ID]
		c.mu.Lock()
		c.activeMode = "read-only" // Model stale evidence at the new-process boundary.
		c.mu.Unlock()
		f.omitPermissionReport = true
		h.send(st.ID, "no new report")
		h.holder(st.ID)
		eventually(t, "second process without observation", func() bool { return h.state(st.ID).Process })
		got := h.state(st.ID)
		if got.ActiveMode != "" || got.PermissionEnforcement != "advisory" {
			t.Fatalf("new process retained stale evidence: %+v", got)
		}
	})

	t.Run("legacy omission and child ceiling", func(t *testing.T) {
		h, f := newCodexLeadHarness(t)
		f.reportDefaults = true
		h.startAt(codexSyntheticEndpoint)
		legacy := h.lead(BackendCodex, LeadOptions{})
		f.replyID = legacy.ID
		h.send(legacy.ID, "keep native defaults")
		holder := h.holder(legacy.ID)
		if open := h.holders.acpInputs(holder, "thread/start"); len(open) != 1 || jsonAt(open[0], "sandbox") != nil || jsonAt(open[0], "approvalPolicy") != nil {
			t.Fatalf("legacy omission changed Codex defaults: %s", open)
		}
		if _, err := os.Stat(filepath.Join(wsdashboard.SessionDir(h.dir, legacy.ID), "permission.json")); !os.IsNotExist(err) {
			t.Fatalf("legacy omission wrote a sidecar: %v", err)
		}
		eventually(t, "legacy default observation", func() bool {
			got := h.state(legacy.ID)
			return got.ActiveMode == "workspace-write" && got.PermissionLevel == PermissionWorkdirWrite && got.PermissionSource == "legacy-derived"
		})
		if rec := h.record(legacy.ID); rec.PermissionMode != "" {
			t.Fatalf("legacy observation became a launch override: %+v", rec)
		}
		h.holders.out(holder, codexSyntheticEnd("t1", "done", "completed"))
		h.idle(legacy.ID)
		h.holders.exit(holder, 0)
		eventually(t, "legacy process exit", func() bool { return !h.state(legacy.ID).Process })
		h.send(legacy.ID, "keep defaults on resume")
		fresh := h.holder(legacy.ID)
		resume := h.holders.acpInputs(fresh, "thread/resume")
		if len(resume) != 1 || jsonAt(resume[0], "sandbox") != nil || jsonAt(resume[0], "approvalPolicy") != nil {
			t.Fatalf("legacy observation froze the resumed defaults: %s", resume)
		}

		parent := h.lead(BackendCodex, LeadOptions{PermissionLevel: PermissionReadOnly, permissionUser: true})
		parentRef := ParentRef{Kind: ParentLead, ReplyID: parent.ID}
		before := len(h.runner.List())
		_, err := h.runner.Spawn(SpawnSpec{
			Profile: PiWorkerProfile, Root: h.root, Prompt: "above parent", Parent: &parentRef,
			PermissionLevel: PermissionWorkdirWrite, delegatedAuthority: true,
		})
		if err == nil || !strings.Contains(err.Error(), "above the parent's current level read-only") || len(h.runner.List()) != before {
			t.Fatalf("Codex parent ceiling: %v", err)
		}
	})
}

func codexSyntheticNotification(method string, params any) string {
	return jsonLine(map[string]any{"method": method, "params": params})
}

func codexSyntheticEnd(turn, text, status string) string {
	items := []map[string]any{}
	if text != "" {
		items = append(items, map[string]any{"id": "reply-" + turn, "type": "agentMessage", "text": text})
	}
	return codexSyntheticNotification("turn/completed", map[string]any{"threadId": "codex-s1", "turn": map[string]any{"id": turn, "status": status, "items": items}})
}

func codexUserInputs(h *leadHarness, holder string) []string {
	var texts []string
	for _, raw := range h.holders.acpInputs(holder, "turn/start") {
		var params struct {
			Input []struct{ Type, Text string } `json:"input"`
		}
		_ = json.Unmarshal(raw, &params)
		if len(params.Input) == 1 && params.Input[0].Type == "text" {
			texts = append(texts, params.Input[0].Text)
		}
	}
	return texts
}

func codexCallIDs(h *leadHarness, holder, method string) []string {
	var ids []string
	for _, raw := range h.holders.inputs(holder, "") {
		var msg codexWire
		if json.Unmarshal([]byte(raw), &msg) == nil && msg.Method == method {
			ids = append(ids, rawString(msg.ID))
		}
	}
	return ids
}

func codexApplied(t *testing.T, h *leadHarness, id, holder string) {
	t.Helper()
	target := uint64(h.holders.records(holder))
	eventually(t, "all Codex holder records applied", func() bool {
		var applied bool
		h.runner.conv(id).do(func() { applied = h.runner.conv(id).seq >= target })
		return applied
	})
}

const codexSyntheticEndpoint = "http://127.0.0.1:1"

func TestCodexLeadSteerReturnsRefusal(t *testing.T) {
	h, f := newCodexLeadHarness(t)
	h.startAt(codexSyntheticEndpoint)
	id := h.lead(BackendCodex, LeadOptions{}).ID
	f.replyID = id
	h.send(id, "initial")
	holder := h.holder(id)
	eventually(t, "the initial Codex turn", func() bool {
		return slices.Equal(codexUserInputs(h, holder), h.envelopes("initial"))
	})
	codexApplied(t, h, id, holder)
	eventually(t, "the active Codex turn", func() bool {
		active := false
		h.runner.conv(id).do(func() { active = h.runner.conv(id).turn })
		return active
	})
	f.mu.Lock()
	f.steerError = "synthetic steer refusal"
	f.mu.Unlock()
	var steer steerer
	h.runner.conv(id).do(func() { steer = controlsFor(h.runner.conv(id).proc).steer })
	if err := steer.Steer("refused steer"); err == nil || !strings.Contains(err.Error(), "synthetic steer refusal") {
		t.Fatalf("lead steer error = %v, want provider refusal", err)
	}
	if calls := h.holders.acpInputs(holder, "turn/steer"); len(calls) != 1 || !strings.Contains(string(calls[0]), "refused steer") {
		t.Fatalf("lead steer calls = %s", calls)
	}
}

func TestCodexLeadSyntheticCreateControlsAndResume(t *testing.T) {
	h, f := newCodexLeadHarness(t)
	environ = func() []string {
		return append(slices.Clone(leadDaemonEnv), "CODEX_HOME=/synthetic/account", "CODEX_THREAD_ID=parent", "CODEX_API_KEY=parent-secret", "PI_SESSION_ID=parent", "ORCA_SESSION_KEY=parent", "KEEP_ME=yes")
	}
	manual := &leadManualRender{text: "# Synthetic workflow manual\n\nBinding test instructions"}
	h.opts.LeadManual = manual.render
	h.startAt(codexSyntheticEndpoint)
	id := h.lead(BackendCodex, LeadOptions{Model: "synthetic-model", Effort: "low", PermissionMode: "read-only"}).ID
	f.replyID = id
	h.send(id, "hello")
	holder := h.holder(id)
	eventually(t, "first Codex input", func() bool { return slices.Equal(codexUserInputs(h, holder), h.envelopes("hello")) })
	codexApplied(t, h, id, holder)
	spawn, rec := h.holders.spawned(holder), h.record(id)
	if !slices.Equal(spawn.Argv, []string{"/opt/harness/codex", "app-server"}) || !sameDir(spawn.Cwd, h.root) || spawn.Mode != "pipe" {
		t.Fatalf("sustained stdio process: %+v", spawn)
	}
	if spawn.Env["CODEX_HOME"] != "/synthetic/account" || spawn.Env["KEEP_ME"] != "yes" || spawn.Env[EnvMCPSessionToken] != rec.MCPToken || len(rec.MCPToken) != 64 {
		t.Fatalf("harness home and child-only token: %v", spawn.Env)
	}
	for _, name := range []string{"CODEX_THREAD_ID", "CODEX_API_KEY", "PI_SESSION_ID", "ORCA_SESSION_KEY", "CLAUDECODE", "WS_MAILBOX"} {
		if _, ok := spawn.Env[name]; ok {
			t.Errorf("inherited session identity %s", name)
		}
	}
	if spawn.Env[EnvHostedSession] != "1" || spawn.Env[EnvHostedSessionKey] != rec.WSKey || rec.SessionID != "codex-s1" || rec.HostPID == 0 {
		t.Fatalf("hosted and native thread identity: %+v %v", rec, spawn.Env)
	}
	if strings.Contains(strings.Join(h.holders.inputs(holder, ""), " ")+strings.Join(spawn.Argv, " "), rec.MCPToken) {
		t.Fatal("bearer token leaked into argv or protocol config")
	}
	init := h.holders.acpInputs(holder, "initialize")
	open := h.holders.acpInputs(holder, "thread/start")
	if len(init) != 1 || jsonAt(init[0], "clientInfo", "version") != h.opts.DaemonVersion || jsonAt(init[0], "capabilities", "experimentalApi") != true || len(open) != 1 {
		t.Fatalf("one initialize and thread/start: %s %s", init, open)
	}
	p := open[0]
	if jsonAt(p, "cwd") != h.root || jsonAt(p, "model") != "synthetic-model" || jsonAt(p, "sandbox") != "read-only" || jsonAt(p, "approvalPolicy") != "on-request" || jsonAt(p, "config", "agents", "enabled") != false {
		t.Fatalf("thread configuration: %s", p)
	}
	wantServers := map[string]any{
		"ws":     map[string]any{"url": codexSyntheticEndpoint + wsdashboard.ProxyHTTPPath, "bearer_token_env_var": EnvMCPSessionToken, "required": true},
		"wsflow": map[string]any{"url": codexSyntheticEndpoint + wsdashboard.ProxyHTTPPath, "enabled": false},
	}
	if servers := jsonAt(p, "config", "mcp_servers"); !reflect.DeepEqual(servers, wantServers) {
		t.Fatalf("start must enable only ws and disable the wsflow shadow: %v, want %v", servers, wantServers)
	}
	if jsonAt(p, "baseInstructions") != nil || jsonAt(p, "config", "features") != nil || jsonAt(p, "config", "plugins") != nil {
		t.Fatalf("must not replace defaults, disable hooks or disable plugins: %s", p)
	}
	instructions, _ := jsonAt(p, "developerInstructions").(string)
	if !strings.Contains(instructions, leadKeySentence(rec.WSKey)) || !strings.Contains(instructions, manual.text) {
		t.Fatalf("additive developer instructions: %q", instructions)
	}
	f.mu.Lock()
	pids, tokens := slices.Clone(f.rowPIDs), slices.Clone(f.tokens)
	f.mu.Unlock()
	if !slices.Equal(pids, []int{rec.HostPID}) || !slices.Equal(tokens, []string{rec.MCPToken}) {
		t.Fatalf("physical pid published before MCP handshake: %v %v", pids, tokens)
	}
	if hello, _, ok := h.runner.ResolveMCPToken(rec.MCPToken); !ok || hello.Env[EnvHostedSessionKey] != rec.WSKey || !sameDir(hello.ProjectRoot, h.root) {
		t.Fatalf("MCP identity: %+v %v", hello, ok)
	}

	// Steering is distinct from a held next turn, and settings apply to
	// that next turn without restarting the holder.
	h.send(id, "steer now")
	h.steerHeld(id)
	h.send(id, "later")
	eventually(t, "active-turn steer", func() bool { return len(h.holders.acpInputs(holder, "turn/steer")) == 1 })
	steer := h.holders.acpInputs(holder, "turn/steer")[0]
	if jsonAt(steer, "expectedTurnId") != "t1" || jsonAt(steer, "threadId") != "codex-s1" || !strings.Contains(string(steer), "steer now") || len(codexUserInputs(h, holder)) != 1 || h.state(id).Held != 1 {
		t.Fatalf("steer must not create another turn or release follow-up: %s %+v", steer, h.state(id))
	}
	model, effort := "synthetic-next", "high"
	if _, err := h.runner.SetSettings(id, &model, &effort); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.setPermissionMode(id, "danger-full-access", true); err != nil {
		t.Fatal(err)
	}
	h.holders.out(holder, codexSyntheticEnd("t1", "first", "completed"))
	eventually(t, "held follow-up", func() bool { return slices.Equal(codexUserInputs(h, holder), h.envelopes("hello", "later")) })
	codexApplied(t, h, id, holder)
	next := h.holders.acpInputs(holder, "turn/start")[1]
	if jsonAt(next, "model") != model || jsonAt(next, "effort") != effort || jsonAt(next, "approvalPolicy") != "never" || jsonAt(next, "sandboxPolicy", "type") != "dangerFullAccess" || h.holders.opCount("spawn") != 1 {
		t.Fatalf("next-turn settings, in-stream: %s %v", next, h.holders.opList())
	}
	if _, _, err := h.runner.Interrupt(id); err != nil {
		t.Fatal(err)
	}
	eventually(t, "interrupt request", func() bool { return len(h.holders.acpInputs(holder, "turn/interrupt")) == 1 })
	interrupt := h.holders.acpInputs(holder, "turn/interrupt")[0]
	if jsonAt(interrupt, "threadId") != "codex-s1" || jsonAt(interrupt, "turnId") != "t2" || h.state(id).Status != StatusRunning {
		t.Fatalf("acknowledgement is not turn completion: %s %+v", interrupt, h.state(id))
	}
	h.holders.out(holder, codexSyntheticEnd("t2", "", "interrupted"))
	h.idle(id)

	// EOF is process exit, not a fabricated reply. The next send resumes
	// the same native thread under a new process and fresh MCP token.
	h.holders.exit(holder, 0)
	eventually(t, "Codex EOF", func() bool { return !h.state(id).Process && h.record(id).HolderID == "" })
	h.send(id, "resume")
	fresh := h.holder(id)
	eventually(t, "resumed input", func() bool { return len(codexUserInputs(h, fresh)) == 1 })
	resumes := h.holders.acpInputs(fresh, "thread/resume")
	if fresh == holder || len(resumes) != 1 || jsonAt(resumes[0], "threadId") != "codex-s1" || h.record(id).MCPToken == rec.MCPToken || jsonAt(resumes[0], "config", "agents", "enabled") != false || jsonAt(resumes[0], "config", "mcp_servers", "ws") == nil {
		t.Fatalf("resume re-passes identity and config: %s %+v", resumes, h.record(id))
	}
	if servers := jsonAt(resumes[0], "config", "mcp_servers"); !reflect.DeepEqual(servers, wantServers) {
		t.Fatalf("resume must enable only ws and disable the wsflow shadow: %v, want %v", servers, wantServers)
	}
	h.holders.out(fresh, codexSyntheticEnd("t3", "resumed", "completed"))
	h.idle(id)
}

func TestCodexLeadSyntheticReattachMidTurn(t *testing.T) {
	for _, pending := range []bool{false, true} {
		t.Run(fmt.Sprintf("pending-approval-%t", pending), func(t *testing.T) {
			h, f := newCodexLeadHarness(t)
			h.startAt(codexSyntheticEndpoint)
			id := h.lead(BackendCodex, LeadOptions{}).ID
			f.replyID = id
			h.send(id, "one")
			holder := h.holder(id)
			h.holders.out(holder, `{"method":"item/agentMessage/delta","params":{"threadId":"codex-s1","turnId":"t1","itemId":"reply-t1","delta":"working"}}`)
			if pending {
				h.holders.out(holder,
					`{"method":"item/started","params":{"threadId":"codex-s1","turnId":"t1","item":{"id":"tool","type":"commandExecution","command":"ls"}}}`,
					`{"id":"approval-7","method":"item/commandExecution/requestApproval","params":{"threadId":"codex-s1","turnId":"t1","itemId":"tool","command":"ls"}}`)
				eventually(t, "pending string-id approval", func() bool { return len(h.state(id).Approvals) == 1 })
			}
			codexApplied(t, h, id, holder)
			h.send(id, "two")
			before := h.ring(id)
			inputs := h.holders.inputs(holder, "")
			token := h.record(id).MCPToken
			h.runner.Close()
			h.holders.restart()
			h.startAt(codexSyntheticEndpoint)
			eventually(t, "same running thread after replay", func() bool {
				st := h.state(id)
				return st.Process && st.Status == StatusRunning && len(st.Approvals) == map[bool]int{true: 1, false: 0}[pending] && h.record(id).MCPToken == token
			})
			codexApplied(t, h, id, holder)
			if !reflect.DeepEqual(h.ring(id), before) || !slices.Equal(h.holders.inputs(holder, ""), inputs) || h.holders.opCount("spawn") != 1 {
				t.Fatalf("reattach must neither replay user input nor append transcript: %v", h.holders.opList())
			}
			if pending {
				// The stable ws id contains JSON quotes, preserving the
				// distinction between numeric and string JSON-RPC ids.
				if got := h.state(id).Approvals[0].ID; got != `"approval-7"` {
					t.Fatalf("replayed approval id: %q", got)
				}
				// The shared HTTP boundary still denies untrusted answers.
				response := h.route(lanBrowser, http.MethodPost, "/agents/"+id+`/approvals/%22approval-7%22`, map[string]any{"behavior": "allow"})
				if response.status == http.StatusOK || !slices.Equal(h.holders.inputs(holder, ""), inputs) {
					t.Fatalf("untrusted approval answer reached child: %+v", response)
				}
				if _, err := h.runner.AnswerApproval(id, `"approval-7"`, true, ""); err != nil {
					t.Fatal(err)
				}
				last := h.holders.inputs(holder, "")
				if len(last) != len(inputs)+1 || last[len(last)-1] != `{"id":"approval-7","result":{"decision":"accept"}}` {
					t.Fatalf("answer echoes original string id: %q", last)
				}
				h.holders.out(holder, `{"method":"serverRequest/resolved","params":{"threadId":"codex-s1","requestId":"approval-7"}}`, `{"method":"item/completed","params":{"threadId":"codex-s1","turnId":"t1","item":{"id":"tool","type":"commandExecution","status":"completed","exitCode":0}}}`)
			}
			h.send(id, "three")
			if len(codexUserInputs(h, holder)) != 1 || h.state(id).Held != 2 {
				t.Fatalf("follow-ups held in order across restart: %+v", h.state(id))
			}
			h.holders.out(holder, codexSyntheticEnd("t1", "first", "completed"))
			joined := h.envelope("two") + "\n\n" + h.envelope("three")
			eventually(t, "held input once after old turn ends", func() bool { return slices.Equal(codexUserInputs(h, holder), []string{h.envelope("one"), joined}) })
			ids := codexCallIDs(h, holder, "turn/start")
			if len(ids) != 2 || ids[0] == ids[1] || strings.Split(ids[0], "/")[2] == strings.Split(ids[1], "/")[2] || len(h.holders.acpInputs(holder, "initialize")) != 1 {
				t.Fatalf("per-attach request epoch, no new handshake: %v", ids)
			}
			h.holders.out(holder, codexSyntheticEnd("t2", "second", "completed"))
			h.idle(id)
			if got := ringTexts(h.ring(id), wslive.RingReply); !slices.Equal(got, []string{"first", "second"}) {
				t.Fatalf("replies once: %q", got)
			}
		})
	}
}

func TestCodexLeadSyntheticReplayGapOrdersRecoveryBeforeFollowUps(t *testing.T) {
	for _, loss := range []string{"wrapped", "gap during replay"} {
		for _, active := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/active-%t", loss, active), func(t *testing.T) {
				h, f := newCodexLeadHarness(t)
				h.startAt(codexSyntheticEndpoint)
				id := h.lead(BackendCodex, LeadOptions{}).ID
				f.replyID = id
				h.send(id, "one")
				holder := h.holder(id)
				codexApplied(t, h, id, holder)
				h.runner.Close()
				h.holders.restart()
				// Lose the old start; the remaining unknown notification
				// proves no boundary. The child may be idle or still active.
				h.holders.out(holder, `{"method":"synthetic/keepalive","params":{"threadId":"codex-s1"}}`)
				n := h.holders.records(holder)
				if loss == "wrapped" {
					h.holders.trim(holder, n-1)
				} else {
					h.holders.gapOnReplay(holder, 1, uint64(n))
				}
				gate, started := make(chan struct{}), make(chan struct{})
				var once sync.Once
				release := func() { once.Do(func() { close(gate) }) }
				defer release()
				status, turnStatus := "idle", "completed"
				if active {
					status, turnStatus = "active", "inProgress"
				}
				f.mu.Lock()
				f.readGate, f.readStarted = gate, started
				f.recovery = jsonLine(map[string]any{"thread": map[string]any{"id": "codex-s1", "status": map[string]any{"type": status}, "turns": []any{map[string]any{"id": "t1", "status": turnStatus, "items": []any{map[string]any{"id": "reply-t1", "type": "agentMessage", "text": "recovered first"}}}}}})
				f.mu.Unlock()
				h.startAt(codexSyntheticEndpoint)
				wait(t, "thread/read recovery request", started)
				h.send(id, "two")
				h.send(id, "three")
				if got := codexUserInputs(h, holder); !slices.Equal(got, h.envelopes("one")) || h.state(id).Held != 2 {
					t.Fatalf("uncertain recovery must hold, not duplicate input: %q %+v", got, h.state(id))
				}
				release()
				if active {
					codexApplied(t, h, id, holder)
					if len(codexUserInputs(h, holder)) != 1 || h.state(id).Held != 2 {
						t.Fatal("active recovery released input prematurely")
					}
					h.holders.out(holder, codexSyntheticEnd("t1", "recovered first", "completed"))
				}
				joined := h.envelope("two") + "\n\n" + h.envelope("three")
				eventually(t, "recovered turn followed by held input", func() bool { return slices.Equal(codexUserInputs(h, holder), []string{h.envelope("one"), joined}) })
				codexApplied(t, h, id, holder)
				var order []string
				for _, entry := range h.ring(id) {
					if entry.Kind == wslive.RingUser || entry.Kind == wslive.RingReply {
						order = append(order, entry.Kind+":"+entry.Text)
					}
				}
				want := []string{wslive.RingUser + ":" + h.envelope("one"), wslive.RingReply + ":recovered first", wslive.RingUser + ":" + joined}
				if !slices.Equal(order, want) || h.holders.opCount("spawn") != 1 || len(h.holders.acpInputs(holder, "thread/read")) != 1 {
					t.Fatalf("recovery final before next user, exactly once: %q want %q; %v", order, want, h.holders.opList())
				}
				h.holders.out(holder, codexSyntheticEnd("t2", "second", "completed"))
				h.idle(id)
			})
		}
	}
}

func TestCodexEnvSyntheticScrubsParentWSIdentity(t *testing.T) {
	// A daemon may have been launched from Pi. That launcher's ws-side
	// identity is no more this new Codex lead's than WS_MAILBOX is.
	for _, value := range codexEnv(StartSpec{Env: slices.Clone(leadDaemonEnv), Lead: &LeadSpec{}}) {
		if strings.HasPrefix(value, "WS_PI_") {
			t.Errorf("parent ws/Pi identity leaked into Codex environment: %s", value)
		}
	}
}

func TestCodexLeadSyntheticEOFSettlesPendingTurn(t *testing.T) {
	for _, code := range []int{0, 17} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			h, f := newCodexLeadHarness(t)
			h.startAt(codexSyntheticEndpoint)
			id := h.lead(BackendCodex, LeadOptions{}).ID
			f.replyID = id
			h.send(id, "one")
			holder := h.holder(id)
			token := h.record(id).MCPToken
			h.holders.out(holder, `{"id":9,"method":"item/commandExecution/requestApproval","params":{"threadId":"codex-s1","turnId":"t1","itemId":"tool","command":"ls"}}`)
			eventually(t, "numeric approval before EOF", func() bool { return len(h.state(id).Approvals) == 1 })
			h.holders.exit(holder, code)
			eventually(t, "EOF clears process and approvals", func() bool {
				st := h.state(id)
				return !st.Process && st.Status == StatusIdle && len(st.Approvals) == 0 && h.record(id).HolderID == ""
			})
			if st := h.state(id); code != 0 && !strings.Contains(st.Error, "exited: exit status 17") {
				t.Fatalf("unexpected exit must be visible: %+v", st)
			}
			if _, _, ok := h.runner.ResolveMCPToken(token); ok {
				t.Fatal("exited process retained its MCP capability")
			}
			if got := ringTexts(h.ring(id), wslive.RingReply); len(got) != 0 {
				t.Fatalf("EOF must not invent a final answer: %q", got)
			}
		})
	}
}

func TestCodexLeadSyntheticRequiredMCPFailure(t *testing.T) {
	h, f := newCodexLeadHarness(t)
	f.openError = "required MCP servers failed to initialize: ws: HTTP 401"
	h.startAt(codexSyntheticEndpoint)
	id := h.lead(BackendCodex, LeadOptions{}).ID
	f.replyID = id
	if _, err := h.runner.Send(id, "hello"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "required server rejection", func() bool { return strings.Contains(h.state(id).Error, f.openError) })
	h.quiesce(id)
	if rec := h.record(id); rec.HostPID != 0 || rec.HolderID != "" || rec.MCPToken != "" {
		t.Fatalf("failed start retains no process/token: %+v", rec)
	}
	f.mu.Lock()
	tokens := slices.Clone(f.tokens)
	f.mu.Unlock()
	if len(tokens) != 1 {
		t.Fatalf("one rejected start: %v", tokens)
	}
	if _, _, ok := h.runner.ResolveMCPToken(tokens[0]); ok {
		t.Fatal("failed start retained its MCP capability")
	}
	if h.holders.opCount("stop") != 1 || h.holders.opCount("spawn") != 1 {
		t.Fatalf("failed child must be stopped: %v", h.holders.opList())
	}
}

func TestCodexLeadSyntheticAdmissionFailures(t *testing.T) {
	if _, err := (CodexBackend{}).Start(context.Background(), StartSpec{Lead: &LeadSpec{}}); err == nil || !strings.Contains(err.Error(), "requires a session holder") {
		t.Fatalf("holderless lead start: %v", err)
	}
	h, f := newCodexLeadHarness(t)
	h.start()
	id := h.lead(BackendCodex, LeadOptions{}).ID
	f.replyID = id
	if _, err := h.runner.Send(id, "hello"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "no-route failure", func() bool { return strings.Contains(h.state(id).Error, "HTTP MCP route is required") })
	if h.holders.opCount("spawn") != 0 {
		t.Fatalf("must not launch a lead without ws tools: %v", h.holders.opList())
	}
}
