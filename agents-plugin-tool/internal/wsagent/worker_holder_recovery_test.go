package wsagent

import (
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// workerRecoveryHarness runs the real runner and backend over a fake holder.
// The fake child speaks each backend's protocol without launching a provider.
func workerRecoveryHarness(t *testing.T, backend string, autoApprove ...bool) (*leadHarness, *reports) {
	t.Helper()
	var h *leadHarness
	switch backend {
	case BackendCodex:
		h, _ = newCodexLeadHarness(t)
	case BackendAgy:
		h, _ = newAgyLeadHarness(t)
	default:
		h = newLeadHarness(t)
	}
	got := &reports{}
	h.opts.Report = got.add
	h.opts.WSMCP = func(string) (MCPCommand, error) { return MCPCommand{Command: "/unused/ws-mcp"}, nil }
	reg := h.opts.Backends[backend]
	reg.WorkerCapable = true
	reg.WorkerProfile = AgentWorkerProfile(backend)
	reg.WorkerHTTPMCP = backend != BackendPi
	reg.WorkerAutoApprove = (backend == BackendCodex || backend == BackendAgy) && (len(autoApprove) == 0 || autoApprove[0])
	h.opts.Backends[backend] = reg
	model, effort := "claude-small", "high"
	switch backend {
	case BackendCodex:
		model, effort = "synthetic-model", "low"
	case BackendAgy:
		model, effort = "gemini-3.8-flash", "medium"
	case BackendPi:
		model, effort = "prov/small", "low"
	}
	if _, err := wsconfig.SetGlobalAgentsTierForHarness(h.opts.Config, "small", "", model, backend, effort); err != nil {
		t.Fatal(err)
	}
	h.startAt("http://127.0.0.1:1")
	return h, got
}

func spawnHeldRecoveryWorker(t *testing.T, h *leadHarness, backend, authority string) (string, string) {
	t.Helper()
	caller := LeadCaller(leadParent.ReplyID, h.root, BackendClaude, h.opts.Config)
	res, err := h.runner.AgentSpawn(caller, AgentSpawnRequest{Prompt: "work", Tier: backend + ".small", Authority: authority})
	if err != nil {
		t.Fatal(err)
	}
	id := agentID(res.Address)
	holder := h.holder(id)
	switch backend {
	case BackendClaude:
		h.holders.out(holder, stateLine(HarnessRunning), initLine("claude-s1", "dontAsk"))
	case BackendPi:
		h.holders.out(holder, `{"type":"agent_start"}`)
	case BackendCodex:
		eventually(t, "the Codex turn", func() bool { return len(h.holders.acpInputs(holder, "turn/start")) == 1 })
	case BackendAgy:
		eventually(t, "the agy prompt", func() bool { return len(agyPromptIDs(h, holder)) == 1 })
	}
	eventually(t, "the held worker's open turn", func() bool {
		rec := h.record(id)
		return rec.TurnOpen && rec.HolderSeq > 0
	})
	return id, holder
}

func completeRecoveryWorker(t *testing.T, h *leadHarness, backend, holder string) {
	t.Helper()
	switch backend {
	case BackendClaude:
		h.holders.out(holder, replyLine("u1", "held result"), resultLine(), stateLine(HarnessIdle))
	case BackendCodex:
		h.holders.out(holder, codexSyntheticEnd("t1", "held result", "completed"))
	case BackendAgy:
		prompt := agyPromptIDs(h, holder)[0]
		h.holders.out(holder,
			`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"agy-s1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"held result"}}}}`,
			jsonLine(map[string]any{"jsonrpc": "2.0", "id": prompt, "result": map[string]any{"stopReason": "end_turn"}}))
	case BackendPi:
		h.holders.out(holder, recoveryPiReply("held result")...)
	}
}

// TestCodexGrandchildCompletionReachesConversationParentExactlyOnce pins the
// holder-backed Codex path that dogfooding reported missing: the grandchild's
// terminal result starts the conversation parent's successor turn, while the
// parent's held final is published first and is not repeated after rejoin.
func TestCodexGrandchildCompletionReachesConversationParentExactlyOnce(t *testing.T) {
	h, got := workerRecoveryHarness(t, BackendCodex, true)
	caller := LeadCaller(leadParent.ReplyID, h.root, BackendClaude, h.opts.Config)
	parentResult, err := h.runner.AgentSpawn(caller, AgentSpawnRequest{Prompt: "parent work", Tier: "codex.small", Authority: "workdir-write"})
	if err != nil {
		t.Fatal(err)
	}
	parent := agentID(parentResult.Address)
	parentHolder := h.holder(parent)
	eventually(t, "the Codex parent turn", func() bool { return len(h.holders.acpInputs(parentHolder, "turn/start")) == 1 })

	childCaller := AgentCaller{Parent: ParentRef{Kind: ParentConversation, ReplyID: parent}, Root: h.root, Harness: BackendCodex, Config: h.opts.Config}
	childResult, err := h.runner.AgentSpawn(childCaller, AgentSpawnRequest{Prompt: "inner work", Tier: "codex.small", Authority: "workdir-write"})
	if err != nil {
		t.Fatal(err)
	}
	child := agentID(childResult.Address)
	childHolder := h.holder(child)
	eventually(t, "the Codex grandchild turn", func() bool { return len(h.holders.acpInputs(childHolder, "turn/start")) == 1 })

	h.holders.out(parentHolder, codexSyntheticEnd("t1", "parent final", "completed"))
	eventually(t, "the parent final held for its grandchild", func() bool {
		rec := h.record(parent)
		return rec.HeldTurn != nil && rec.HeldTurn.Message == "parent final"
	})
	h.holders.out(childHolder, codexSyntheticEnd("t2", "inner complete", "completed"))
	deadline := time.Now().Add(5 * time.Second)
	for len(h.holders.acpInputs(parentHolder, "turn/start")) != 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if gotStarts := len(h.holders.acpInputs(parentHolder, "turn/start")); gotStarts != 2 {
		t.Fatalf("grandchild report did not start the parent successor: starts=%d parent=%+v child=%+v reports=%+v", gotStarts, h.record(parent), h.record(child), got.list())
	}
	h.holders.out(parentHolder, codexSyntheticEnd("t3", "successor ack", "completed"))
	eventually(t, "the held final and successor report in order", func() bool { return len(got.lead()) == 2 })
	if rep := got.lead()[0]; rep.Result.ID != parent || rep.Result.Message != "parent final" || rep.Result.Status != TurnDone {
		t.Fatalf("first parent report = %+v", rep.Result)
	}
	if rep := got.lead()[1]; rep.Result.ID != parent || rep.Result.Message != "successor ack" || rep.Result.Status != TurnDone {
		t.Fatalf("second parent report = %+v", rep.Result)
	}
	if all := got.list(); len(all) != 3 || all[0].Parent.Kind != ParentConversation || all[0].Result.ID != child {
		t.Fatalf("grandchild completion delivery = %+v, want one conversation-parent report plus two lead reports", all)
	}

	h.runner.Close()
	h.holders.restart()
	h.startAt("http://127.0.0.1:1")
	time.Sleep(50 * time.Millisecond)
	if reports := got.list(); len(reports) != 3 {
		t.Fatalf("rejoin duplicated a delivered completion: %+v", reports)
	}
	if rec := h.record(child); len(rec.PendingReports) != 0 {
		t.Fatalf("the delivered grandchild report stayed pending: %+v", rec.PendingReports)
	}
}

// TestCodexAndAgyWorkerMailSteersIntoRunningTurn proves the common worker
// mail path uses each backend's acknowledged holder-safe steer contract. The
// daemon is replaced after the holder accepted the steer but before the
// acknowledgement is applied; replay then consumes it exactly once.
func TestCodexAndAgyWorkerMailSteersIntoRunningTurn(t *testing.T) {
	const (
		rawMessage = "mid-turn mail"
		message    = "(parent) " + rawMessage
	)
	for _, backend := range []string{BackendCodex, BackendAgy} {
		t.Run(backend, func(t *testing.T) {
			h, _ := workerRecoveryHarness(t, backend, true)
			id, holder := spawnHeldRecoveryWorker(t, h, backend, "workdir-write")
			key := h.record(id).WSKey
			eventually(t, "the worker mailbox watch", func() bool { return h.mail.watched(key) })
			if backend == BackendCodex {
				acp := h.holders.acp
				h.holders.acp = func(holder, method string, params json.RawMessage) (string, string, bool) {
					if method == "turn/steer" {
						return "", "", false
					}
					return acp(holder, method, params)
				}
			}
			h.mail.deliverFrom(key, leadParent.ReplyID, rawMessage)
			var steerID string
			switch backend {
			case BackendCodex:
				eventually(t, "the Codex steer write", func() bool {
					ids := codexCallIDs(h, holder, "turn/steer")
					if len(ids) == 1 {
						steerID = ids[0]
					}
					return steerID != ""
				})
			case BackendAgy:
				eventually(t, "the agy steer prompt", func() bool { return len(agyPromptIDs(h, holder)) == 2 })
			}
			eventually(t, "the persisted unacknowledged steer", func() bool {
				rec := h.record(id)
				return len(rec.Steered) == 1 && rec.Steered[0].Text == message && rec.SteerAnswers == 0
			})

			h.runner.Close()
			h.holders.restart()
			h.startAt("http://127.0.0.1:1")
			eventually(t, "the held worker rejoined", func() bool {
				c := h.runner.conv(id)
				if c == nil {
					return false
				}
				ready := false
				c.do(func() { ready = c.proc != nil && !c.replaying && c.turn })
				return ready
			})

			switch backend {
			case BackendCodex:
				h.holders.out(holder,
					jsonLine(map[string]any{"id": steerID, "result": map[string]any{"turnId": "t1"}}),
					codexSyntheticNotification("item/started", map[string]any{"threadId": "codex-s1", "turnId": "t1", "item": map[string]any{"id": "steered-user", "type": "userMessage", "content": []any{map[string]any{"type": "text", "text": message}}}}))
			case BackendAgy:
				prompts := agyPromptIDs(h, holder)
				h.holders.out(holder, jsonLine(map[string]any{"jsonrpc": "2.0", "id": prompts[0], "result": map[string]any{"stopReason": "end_turn"}}))
			}
			eventually(t, "the steer acknowledged and taken once", func() bool {
				rec := h.record(id)
				return rec.SteerAnswers == 1 && len(rec.Steered) == 0 && rec.Drained == rec.Sends && len(rec.Reports) == 0
			})
		})
	}
}

func TestHeldWorkersCompleteAcrossRunnerReplacement(t *testing.T) {
	for _, backend := range []string{BackendClaude, BackendCodex, BackendAgy, BackendPi} {
		for _, authority := range []string{"read-only", "workdir-write"} {
			if backend != BackendCodex && authority == "read-only" {
				continue
			}
			t.Run(backend+"/"+authority, func(t *testing.T) {
				h, got := workerRecoveryHarness(t, backend, true)
				id, holder := spawnHeldRecoveryWorker(t, h, backend, authority)
				before := h.record(id)
				if before.HolderID != holder || before.HostPID <= 0 || before.HandedOffAt == "" {
					t.Fatalf("new worker is not fenced under its holder: %+v", before)
				}
				if backend != BackendPi && (before.MCPToken == "" || before.MCPHolder != holder || before.MCPURL == "" || before.MCPEnv == nil) {
					t.Fatalf("worker MCP tuple was not persisted: %+v", before)
				}
				var beforeEnv map[string]string
				if backend != BackendPi {
					hello, _, ok := h.runner.ResolveMCPToken(before.MCPToken)
					if !ok {
						t.Fatal("launched worker's MCP token does not resolve")
					}
					beforeEnv = maps.Clone(hello.Env)
					if beforeEnv[EnvAgentSessionKey] != before.WSKey || beforeEnv[EnvPermissionLevel] != authority || beforeEnv["WS_MCP_HARNESS_PID"] != strconv.Itoa(before.HostPID) {
						t.Fatalf("launched worker MCP identity: key %q, level %q, pid %q", beforeEnv[EnvAgentSessionKey], beforeEnv[EnvPermissionLevel], beforeEnv["WS_MCP_HARNESS_PID"])
					}
				}
				h.runner.Close()
				if h.holders.opCount("stop") != 0 {
					t.Fatalf("runner shutdown stopped the worker: %v, record %+v", h.holders.opList(), h.record(id))
				}
				h.holders.restart()
				completeRecoveryWorker(t, h, backend, holder)
				h.startAt("http://127.0.0.1:1")
				eventually(t, "one terminal worker report", func() bool { return len(got.list()) == 1 })
				if rep := got.list()[0]; rep.Result.Status != TurnDone || rep.Result.Message != "held result" || rep.Result.ID != id {
					t.Fatalf("rejoined report: %+v", rep)
				}
				after := h.record(id)
				if after.HolderID != holder || after.HostPID != before.HostPID || after.TurnOpen || after.LastTurn == nil || after.LastTurn.Result.Status != TurnDone {
					t.Fatalf("rejoined worker lost holder or turn: before %+v, after %+v", before, after)
				}
				if h.holders.opCount("launch") != 1 || h.holders.opCount("spawn") != 1 || h.holders.opCount("stop") != 0 {
					t.Fatalf("replacement changed the physical process: %v", h.holders.opList())
				}
				if backend != BackendPi {
					if after.MCPToken != before.MCPToken || after.MCPURL != before.MCPURL || after.MCPHolder != before.MCPHolder {
						t.Fatalf("MCP tuple changed: before %+v, after %+v", before, after)
					}
					hello, _, ok := h.runner.ResolveMCPToken(before.MCPToken)
					if !ok {
						t.Fatal("rejoined worker's persisted MCP token does not resolve")
					}
					for _, name := range []string{EnvHostedSession, EnvHostedSessionKey, EnvAgentSessionKey, EnvPermissionLevel, "WS_MCP_HARNESS_PID"} {
						want, wasSet := beforeEnv[name]
						got, isSet := hello.Env[name]
						if got != want || isSet != wasSet {
							t.Errorf("rejoined MCP env %s = %q (set %v), launched %q (set %v)", name, got, isSet, want, wasSet)
						}
					}
				}
				if backend == BackendCodex {
					wantSandbox, wantPolicy := "workspace-write", "on-request"
					if authority == "read-only" {
						wantSandbox, wantPolicy = "read-only", "never"
					}
					turns := h.holders.acpInputs(holder, "turn/start")
					if len(turns) != 1 || jsonAt(turns[0], "approvalPolicy") != wantPolicy || jsonAt(turns[0], "sandboxPolicy", "type") != map[string]string{"read-only": "readOnly", "workspace-write": "workspaceWrite"}[wantSandbox] {
						t.Fatalf("first Codex turn mode: %s", turns)
					}
					h.send(id, "again")
					eventually(t, "the rejoined Codex turn", func() bool { return len(h.holders.acpInputs(holder, "turn/start")) == 2 })
					turns = h.holders.acpInputs(holder, "turn/start")
					if jsonAt(turns[1], "approvalPolicy") != wantPolicy || jsonAt(turns[1], "sandboxPolicy", "type") != map[string]string{"read-only": "readOnly", "workspace-write": "workspaceWrite"}[wantSandbox] {
						t.Fatalf("rejoined Codex turn mode: %s", turns[1])
					}
				}
				if len(got.list()) != 1 {
					t.Fatalf("replacement published more than one report: %+v", got.list())
				}
			})
		}
	}
}

func TestHeldWorkerApprovalRaisedDuringReplacementAutoAllowsOnce(t *testing.T) {
	for _, backend := range []string{BackendCodex, BackendAgy} {
		for _, pendingBefore := range []bool{false, true} {
			name := "raised while detached"
			if pendingBefore {
				name = "pending before replacement"
			}
			t.Run(backend+"/"+name, func(t *testing.T) {
				h, got := workerRecoveryHarness(t, backend, !pendingBefore)
				id, holder := spawnHeldRecoveryWorker(t, h, backend, "workdir-write")
				approval := func() {
					if backend == BackendCodex {
						h.holders.out(holder, `{"id":"approval-1","method":"item/commandExecution/requestApproval","params":{"threadId":"codex-s1","turnId":"t1","itemId":"cmd-1","command":"git status"}}`)
					} else {
						h.holders.out(holder, `{"jsonrpc":"2.0","method":"session/request_permission","params":{"sessionId":"agy-s1","toolCall":{"toolCallId":"call-1","name":"run_command","title":"git status"},"options":[{"optionId":"yes","kind":"allow_once"},{"optionId":"no","kind":"reject_once"}]},"id":"approval-1"}`)
					}
				}
				if pendingBefore {
					approval()
					target := uint64(h.holders.records(holder))
					eventually(t, "approval record applied before replacement", func() bool {
						var seq uint64
						h.runner.conv(id).do(func() { seq = h.runner.conv(id).seq })
						return seq >= target
					})
					var pending int
					h.runner.conv(id).do(func() { pending = len(h.runner.conv(id).approvals) })
					if pending != 1 {
						t.Fatalf("approval did not remain pending: count %d, inputs %v, log %s", pending, h.holders.inputs(holder, ""), h.log.String())
					}
				}
				h.runner.Close()
				if pendingBefore {
					reg := h.opts.Backends[backend]
					reg.WorkerAutoApprove = true
					h.opts.Backends[backend] = reg
				}
				h.holders.restart()
				if !pendingBefore {
					approval()
				}
				h.startAt("http://127.0.0.1:1")
				eventually(t, "one replayed approval answer", func() bool {
					return slices.ContainsFunc(h.holders.inputs(holder, ""), func(line string) bool {
						return strings.Contains(line, `"id":"approval-1"`) && (strings.Contains(line, `"accept"`) || strings.Contains(line, `"optionId":"yes"`))
					})
				})
				answers := 0
				for _, line := range h.holders.inputs(holder, "") {
					if strings.Contains(line, `"id":"approval-1"`) {
						answers++
					}
				}
				if answers != 1 {
					t.Fatalf("approval answered %d times", answers)
				}
				completeRecoveryWorker(t, h, backend, holder)
				eventually(t, "one completed report", func() bool { return len(got.list()) == 1 })
				if h.record(id).TurnOpen || h.holders.opCount("spawn") != 1 {
					t.Fatal("approval recovery changed process or left the turn open")
				}
			})
		}
	}
}

func TestHeldWorkerAttachUsesChildAuthorityWithAbsentParent(t *testing.T) {
	h, got := workerRecoveryHarness(t, BackendCodex, true)
	id, holder := spawnHeldRecoveryWorker(t, h, BackendCodex, "workdir-write")
	before := h.record(id)
	// The persisted sidecar is the child's explicit selection. On restart a
	// missing parent can only be an advisory ceiling, not an attach refusal.
	if level := childLevel(wsdashboard.SessionDir(h.dir, id)); level != PermissionWorkdirWrite {
		t.Fatalf("persisted worker selection = %q", level)
	}
	h.runner.Close()
	h.holders.restart()
	completeRecoveryWorker(t, h, BackendCodex, holder)
	h.startAt("http://127.0.0.1:1")
	eventually(t, "the child rejoins without a loaded parent", func() bool { return len(got.list()) == 1 })
	if after := h.record(id); after.HolderID != holder || after.HostPID != before.HostPID {
		t.Fatalf("child attach was gated on its absent parent: %+v", after)
	}
}

func TestHeldWorkerChildLoadsBeforeLowerCeilingParent(t *testing.T) {
	for _, tc := range []struct {
		name, kind string
		lead       bool
	}{
		{name: "conversation worker", kind: ParentConversation},
		{name: "conversation lead", kind: ParentConversation, lead: true},
		{name: "lead parent", kind: ParentLead, lead: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := workerRecoveryHarness(t, BackendCodex, true)
			id, holder := spawnHeldRecoveryWorker(t, h, BackendCodex, "workdir-write")
			before := h.record(id)
			h.runner.Close()
			// The directory scan is filename ordered. Reparent the already
			// launched worker to a sleeping record loaded after it.
			parentID := strings.Repeat("z", 64)
			if id >= parentID {
				t.Fatalf("fixture parent %q must sort after child %q", parentID, id)
			}
			before.Parent = &ParentRef{Kind: tc.kind, ReplyID: parentID}
			if err := writeRecord(wsdashboard.SessionDir(h.dir, id), before); err != nil {
				t.Fatal(err)
			}
			parentDir := wsdashboard.SessionDir(h.dir, parentID)
			parent := Record{ReplyID: parentID, Profile: PiWorkerProfile, Backend: BackendPi, Root: h.root,
				WSKey: "parent-key", CreatedAt: before.CreatedAt, Parent: &leadParent}
			if tc.lead {
				parent.Lead, parent.Profile, parent.Backend, parent.Parent = true, LeadProfileClaude, BackendClaude, nil
				reg := h.opts.Backends[BackendClaude]
				reg.PermissionLevels = LeadPermissionLevels()
				h.opts.Backends[BackendClaude] = reg
			}
			if err := writeRecord(parentDir, parent); err != nil {
				t.Fatal(err)
			}
			if err := writePermissionSelection(parentDir, PermissionReadOnly); err != nil {
				t.Fatal(err)
			}
			h.holders.restart()
			h.startAt("http://127.0.0.1:1")
			wait(t, "holder scan", h.runner.rejoined)
			eventually(t, "child reattached ahead of parent", func() bool {
				return h.state(id).Process && h.record(id).HolderID == holder
			})
			if rec := h.record(id); rec.HostPID != before.HostPID || !rec.TurnOpen {
				t.Fatalf("lower current parent ceiling changed the running child: %+v", rec)
			}
			var mode, policy string
			c := h.runner.conv(id)
			c.do(func() {
				if p, ok := c.proc.(*codexProcess); ok {
					mode, policy = p.mode, p.policy
				}
			})
			if mode != "workspace-write" || policy != "on-request" {
				t.Fatalf("rejoined child mode = %q, policy = %q", mode, policy)
			}
			if _, err := h.runner.childAdmission(h.record(id)); err == nil || !strings.Contains(err.Error(), "above the parent's current level read-only") {
				t.Fatalf("next launch should be refused by the current ceiling: %v", err)
			}
			if !strings.Contains(h.log.String(), "rejoin permission admission (advisory; applies on next launch)") || h.holders.opCount("stop") != 0 || h.holders.opCount("spawn") != 1 {
				t.Fatalf("advisory attach changed the holder: %v, log %s", h.holders.opList(), h.log.String())
			}
		})
	}
}
