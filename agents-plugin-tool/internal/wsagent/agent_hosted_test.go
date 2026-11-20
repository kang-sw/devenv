// agent_hosted_test.go holds hosted lead and child reporting tests.
package wsagent

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
)

func agentLeadHarness(t *testing.T) (*leadHarness, *wakeRefusals) {
	t.Helper()
	h := newLeadHarness(t)
	refusals := &wakeRefusals{}
	got := &reports{}
	h.opts.Report = got.add
	h.opts.WakeRefused = refusals.add
	h.opts.WSMCP = func(string) (MCPCommand, error) { return MCPCommand{Command: "/unused/ws-mcp"}, nil }
	claude := h.opts.Backends[BackendClaude]
	claude.PermissionLevels = LeadPermissionLevels()
	h.opts.Backends[BackendClaude] = claude
	h.opts.Backends["codex"] = BackendRegistration{Backend: &fakeBackend{}, LeadProfile: "codex-lead"}
	h.globalProfile("codex-lead", `{"backend":"codex","lead":true}`)
	for tier, model := range map[string]string{"small": "prov/small", "medium": "prov/medium"} {
		if _, err := wsconfig.SetGlobalAgentsTierForHarness(h.opts.Config, tier, "", model, PiTierHarness, "low"); err != nil {
			t.Fatal(err)
		}
	}
	h.start()
	h.runner.SetEndpoint("http://127.0.0.1:1")
	return h, refusals
}

func (h *leadHarness) caller(lead State) AgentCaller {
	return LeadCaller(lead.ID, h.root, lead.Backend, h.opts.Config)
}

func TestAgentSpawnCapsAtAHostedLeadsLevel(t *testing.T) {
	h, _ := agentLeadHarness(t)
	spawn := func(caller AgentCaller, level PermissionLevel) (string, error) {
		res, err := h.runner.AgentSpawn(caller, AgentSpawnRequest{Prompt: "x", Tier: "pi.small", Authority: string(level)})
		return agentID(res.Address), err
	}

	// A legacy-derived claude lead caps at its mode's level: full for
	// bypassPermissions, admitted at create and at the process start.
	full := h.lead(BackendClaude, LeadOptions{PermissionMode: "bypassPermissions"})
	id, err := spawn(h.caller(full), PermissionFull)
	if err != nil {
		t.Fatalf("a full child of a full lead: %v", err)
	}
	h.holder(id)
	if lvl := childLevel(wsdashboard.SessionDir(h.dir, id)); lvl != PermissionFull {
		t.Fatalf("the child's level = %q", lvl)
	}
	if st := h.state(id); st.Error != "" {
		t.Fatalf("the full child's start = %+v", st)
	}

	// auto: workdir-write.
	auto := h.lead(BackendClaude, LeadOptions{PermissionMode: "auto"})
	_, err = spawn(h.caller(auto), PermissionFull)
	agentRefused(t, err, http.StatusForbidden, "authority full is above this session's own level workdir-write")
	if _, err := spawn(h.caller(auto), PermissionWorkdirWrite); err != nil {
		t.Fatal(err)
	}
	// Its explicit level (the sidecar) is its current one.
	if err := writePermissionSelection(wsdashboard.SessionDir(h.dir, auto.ID), PermissionReadOnly); err != nil {
		t.Fatal(err)
	}
	_, err = spawn(h.caller(auto), PermissionWorkdirWrite)
	agentRefused(t, err, http.StatusForbidden, "above this session's own level read-only")

	// A lead with no common level on its backend, and a lead reply-id with
	// no Runner record, cap at workdir-write.
	codex := h.lead("codex", LeadOptions{})
	_, err = spawn(h.caller(codex), PermissionFull)
	agentRefused(t, err, http.StatusForbidden, "above this session's own level workdir-write")
	if _, err := spawn(h.caller(codex), PermissionWorkdirWrite); err != nil {
		t.Fatal(err)
	}
	_, err = h.runner.AgentSpawn(h.caller(codex), AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "read-only"})
	// No bare tier is listed for a lead-only harness.
	agentRefused(t, err, http.StatusBadRequest, `harness "codex" is registered for hosted leads only`, "valid tiers: claude.small", "pi.small, pi.medium")
	ghost := LeadCaller("noleadrecordhere", h.root, BackendClaude, h.opts.Config)
	_, err = h.runner.AgentSpawn(ghost, AgentSpawnRequest{Prompt: "x", Tier: "pi.small", Authority: "full"})
	agentRefused(t, err, http.StatusForbidden, "above this session's own level workdir-write")
}

// A hosted lead that lowered its level refuses its sleeping child's wake;
// the mail waits, and the wake after the lead's level is back takes it.
func TestRefusedWakeUnderAHostedLead(t *testing.T) {
	h, refusals := agentLeadHarness(t)
	lead := h.lead(BackendClaude, LeadOptions{PermissionMode: "auto"})
	res, err := h.runner.AgentSpawn(h.caller(lead), AgentSpawnRequest{Prompt: "work", Tier: "pi.small", Authority: "workdir-write"})
	if err != nil {
		t.Fatal(err)
	}
	x := agentID(res.Address)
	first := h.holder(x)
	h.holders.out(first, `{"type":"agent_start"}`)
	h.holders.out(first, recoveryPiReply("x done")...)
	eventually(t, "x's turn settled", func() bool { rec := h.record(x); return rec.LastTurn != nil && !rec.TurnOpen })
	if _, err := h.runner.Stop(x); err != nil {
		t.Fatal(err)
	}
	eventually(t, "x asleep", func() bool {
		st := h.state(x)
		return !st.Process && st.Lifecycle == wsdashboard.SessionChildWaiting
	})

	if err := writePermissionSelection(wsdashboard.SessionDir(h.dir, lead.ID), PermissionReadOnly); err != nil {
		t.Fatal(err)
	}
	launches := h.holders.opCount("launch")
	if _, err := h.runner.Deliver(x, Delivery{From: lead.ID, Text: "held mail"}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the refused start", func() bool { return strings.Contains(h.state(x).Error, "current level read-only") })
	time.Sleep(60 * time.Millisecond)
	if n := h.holders.opCount("launch"); n != launches {
		t.Fatalf("a refused wake launched %d", n-launches)
	}
	if st := h.state(x); st.Held == 0 || st.Lifecycle != wsdashboard.SessionChildWaiting {
		t.Fatalf("the refused child = %+v", st)
	}
	if got := refusals.list(); len(got) != 1 || got[0][0] != lead.ID || got[0][1] != x {
		t.Fatalf("the sender's notices = %v", got)
	}

	if err := writePermissionSelection(wsdashboard.SessionDir(h.dir, lead.ID), PermissionWorkdirWrite); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.Deliver(x, Delivery{From: lead.ID, Text: "second mail"}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the admitted wake", func() bool { return h.holders.opCount("launch") == launches+1 })
	next := h.holder(x)
	eventually(t, "both mails taken", func() bool {
		all := strings.Join(h.holders.inputs(next, "prompt"), "\n")
		return strings.Contains(all, "held mail") && strings.Contains(all, "second mail")
	})
	if got := refusals.list(); len(got) != 1 {
		t.Fatalf("an admitted wake told the sender: %v", got)
	}
}

// A child whose wake was refused sleeps: its kept input holds no done turn
// of its parent's, and a send over the parent's token route tells the
// parent (the sender) of the refusal as Deliver does.
func TestRefusedChildHoldsNoReportAndTheTokenSendTellsTheSender(t *testing.T) {
	refusals := &wakeRefusals{}
	h, _ := agentHarness(t, func(o *Options) { o.WakeRefused = refusals.add })
	a, b := settledPair(t, h)
	if err := writePermissionSelection(wsdashboard.SessionDir(h.dir, a), PermissionReadOnly); err != nil {
		t.Fatal(err)
	}
	starts := h.backend.startCount()
	if _, err := h.runner.Deliver(a, Delivery{Text: "go on"}); err != nil {
		t.Fatal(err)
	}
	ap := h.backend.waitProc(t, starts)
	ap.nextSent(t)
	code, body := h.tokenCall(h.token(starts), "send", map[string]any{"id": b, "message": "held for b"})
	if code != http.StatusOK {
		t.Fatalf("a's send to b = %d %s", code, body)
	}
	eventually(t, "b's refused start", func() bool {
		return strings.Contains(h.state(b).Error, "above the parent's current level read-only")
	})
	if got := refusals.list(); len(got) != 1 || got[0][0] != a || got[0][1] != b {
		t.Fatalf("the sender's notices = %v", got)
	}
	ap.reply("a again")
	eventually(t, "a's turn settled unheld", func() bool {
		rec := h.record(a)
		return rec.LastTurn != nil && rec.LastTurn.Result.Message == "a again" && rec.HeldTurn == nil && !rec.TurnOpen
	})
	if st := h.state(b); st.Held == 0 || st.Process {
		t.Fatalf("b lost its held mail or woke: %+v", st)
	}
}

// agent-worker profiles run only through agent.spawn, with an authority:
// the pi.spawn token route and a bare Runner.Spawn refuse them.
func TestAgentWorkerProfilesNeedAgentSpawn(t *testing.T) {
	h, _ := agentHarness(t, nil)
	h.agentRunning(h.leadCaller(), AgentSpawnRequest{Prompt: "parent", Tier: "medium", Authority: "read-only"})
	token := h.token(0)
	for _, profile := range []string{AgentWorkerProfile(BackendClaude), AgentWorkerProfile(BackendPi)} {
		code, body := h.tokenCall(token, "spawn", map[string]any{"prompt": "escape", "tier": "small", "profile": profile})
		if code != http.StatusBadRequest || !strings.Contains(body, "agent.spawn's internal profile") {
			t.Fatalf("pi spawn of %s = %d %s", profile, code, body)
		}
		parent := leadParent
		_, err := h.runner.Spawn(SpawnSpec{Profile: profile, Root: h.root, Prompt: "x", Tier: "small", Parent: &parent})
		agentRefused(t, err, http.StatusBadRequest, "needs a parent and an authority")
	}
}

func TestChildReportPublishesHeldFinalBeforeSuccessor(t *testing.T) {
	h, got := agentHarness(t, nil)
	lead := h.leadCaller()
	parent, pp := h.agentRunning(lead, AgentSpawnRequest{Prompt: "parent work", Tier: "medium", Authority: "workdir-write"})
	_, cp := h.agentRunning(h.holderOf(parent), AgentSpawnRequest{Prompt: "inner work", Tier: "small", Authority: "workdir-write"})

	pp.reply("parent final")
	eventually(t, "parent final held for inner child", func() bool {
		rec := h.record(parent)
		return rec.HeldTurn != nil && rec.HeldTurn.Message == "parent final"
	})
	cp.reply("inner complete")
	if turn := pp.nextSent(t); !strings.Contains(turn, "inner complete") {
		t.Fatalf("inner child's report turn = %q", turn)
	}
	eventually(t, "held parent final reported first", func() bool { return len(got.lead()) == 1 })
	if rep := got.lead()[0]; rep.Result.Status != TurnDone || rep.Result.Message != "parent final" {
		t.Fatalf("first parent report = %+v", rep.Result)
	}

	pp.reply("successor ack")
	eventually(t, "successor ack reported second", func() bool { return len(got.lead()) == 2 })
	if reps := got.lead(); reps[1].Result.Status != TurnDone || reps[1].Result.Message != "successor ack" {
		t.Fatalf("parent reports = %+v", reps)
	}
}

// TestWorkerBackgroundStateHoldsReportAndShowsStatus verifies the harness-neutral
// background state: while bgTasks > 0 after a turn ends, the worker reports
// state "background" in agent.list and SpawnAgentEvent, withholds its turn-done
// report, and still accepts mail; once bgTasks drops to 0, a self-resumed
// wakeup turn supersedes the held report and settles once, or the held report
// settles after bgResumeGrace when no wakeup arrives.
func TestWorkerBackgroundStateHoldsReportAndShowsStatus(t *testing.T) {
	restoreGrace := bgResumeGrace
	bgResumeGrace = 60 * time.Millisecond
	t.Cleanup(func() { bgResumeGrace = restoreGrace })

	t.Run("wakeup turn supersedes held report", func(t *testing.T) {
		h, got := agentHarness(t, nil)
		lead := h.leadCaller()
		a, ap := h.agentRunning(lead, AgentSpawnRequest{Prompt: "run build", Tier: "small", Authority: "workdir-write"})
		one, zero := 1, 0
		ap.events <- Event{BackgroundTasks: &one}
		ap.reply("started background build")

		eventually(t, "background state", func() bool {
			return findAgent(t, h.runner.AgentList(lead), a).State == AgentBackground
		})
		if len(got.list()) != 0 {
			t.Fatalf("report must be withheld while bgTasks > 0: %+v", got.list())
		}
		st := h.state(a)
		if st.Status != StatusIdle || st.BackgroundTasks != 1 {
			t.Fatalf("worker state = %+v", st)
		}
		if ev := SpawnAgentEvent(st, "k"); ev.Status != wslive.StatusDone || ev.BackgroundTasks != 1 {
			t.Fatalf("SpawnAgentEvent = %+v", ev)
		}

		// Mail still starts a turn while background work runs.
		if _, err := h.runner.Deliver(a, Delivery{Text: "status?"}); err != nil {
			t.Fatal(err)
		}
		if msg := ap.nextSent(t); !strings.Contains(msg, "status?") {
			t.Fatalf("mail while background = %q", msg)
		}
		ap.reply("still building")
		eventually(t, "back to background after mail turn", func() bool {
			return h.state(a).Status == StatusIdle && findAgent(t, h.runner.AgentList(lead), a).State == AgentBackground
		})
		if len(got.list()) != 0 {
			t.Fatalf("report must stay withheld while bgTasks > 0: %+v", got.list())
		}

		// Background task completes, followed by self-resumed wakeup turn.
		ap.events <- Event{BackgroundTasks: &zero}
		ap.events <- Event{TurnStart: true}
		ap.reply("build green")
		eventually(t, "single final report", func() bool { return len(got.list()) == 1 })
		if rep := got.list()[0]; rep.Result.Status != TurnDone || rep.Result.Message != "build green" {
			t.Fatalf("final report = %+v", rep)
		}
		if state := findAgent(t, h.runner.AgentList(lead), a).State; state != AgentIdle {
			t.Fatalf("state after wakeup = %q, want idle", state)
		}
	})

	t.Run("no wakeup settles after grace", func(t *testing.T) {
		h, got := agentHarness(t, nil)
		lead := h.leadCaller()
		a, ap := h.agentRunning(lead, AgentSpawnRequest{Prompt: "fire and forget", Tier: "small", Authority: "workdir-write"})
		one, zero := 1, 0
		ap.events <- Event{BackgroundTasks: &one}
		ap.reply("launched")
		eventually(t, "background state", func() bool {
			return findAgent(t, h.runner.AgentList(lead), a).State == AgentBackground
		})
		ap.events <- Event{BackgroundTasks: &zero}
		eventually(t, "grace settle report", func() bool { return len(got.list()) == 1 })
		if rep := got.list()[0]; rep.Result.Status != TurnDone || rep.Result.Message != "launched" {
			t.Fatalf("grace report = %+v", rep)
		}
	})

	t.Run("exit during grace settles held report", func(t *testing.T) {
		prev := bgResumeGrace
		bgResumeGrace = 10 * time.Second
		t.Cleanup(func() { bgResumeGrace = prev })

		h, got := agentHarness(t, nil)
		lead := h.leadCaller()
		a, ap := h.agentRunning(lead, AgentSpawnRequest{Prompt: "exit during grace", Tier: "small", Authority: "workdir-write"})
		one, zero := 1, 0
		ap.events <- Event{BackgroundTasks: &one}
		ap.reply("before exit")
		eventually(t, "background state", func() bool {
			return findAgent(t, h.runner.AgentList(lead), a).State == AgentBackground
		})
		ap.events <- Event{BackgroundTasks: &zero}
		eventually(t, "grace timer armed", func() bool {
			armed := false
			h.runner.conv(a).do(func() { armed = h.runner.conv(a).bgGraceTimer != nil })
			return armed
		})
		ap.exit()
		eventually(t, "report settled on process exit during grace", func() bool { return len(got.list()) == 1 })
		if rep := got.list()[0]; rep.Result.Status != TurnDone || rep.Result.Message != "before exit" {
			t.Fatalf("exit-during-grace report = %+v", rep)
		}
	})

	t.Run("daemon restart releases persisted HeldTurn", func(t *testing.T) {
		h, got := agentHarness(t, nil)
		lead := h.leadCaller()
		a, ap := h.agentRunning(lead, AgentSpawnRequest{Prompt: "held across restart", Tier: "small", Authority: "workdir-write"})
		one := 1
		ap.events <- Event{BackgroundTasks: &one}
		ap.reply("held before restart")
		eventually(t, "persisted HeldTurn", func() bool {
			rec := h.record(a)
			return rec.HeldTurn != nil && !rec.TurnOpen
		})
		if len(got.list()) != 0 {
			t.Fatalf("report must be withheld before restart: %+v", got.list())
		}
		h.runner.Close()
		h.start()
		eventually(t, "HeldTurn reported on restart", func() bool { return len(got.list()) == 1 })
		if rep := got.list()[0]; rep.Result.Status != TurnDone || rep.Result.Message != "held before restart" {
			t.Fatalf("restart report = %+v", rep)
		}
		if rec := h.record(a); rec.HeldTurn != nil {
			t.Fatalf("HeldTurn not cleared after restart settle: %+v", rec.HeldTurn)
		}
	})

	t.Run("idleNow ignores Codex unknown -1 sentinel", func(t *testing.T) {
		c := &conv{r: &Runner{convs: map[string]*conv{}}, bgTasks: -1}
		if !c.idleNow() {
			t.Fatal("idleNow must allow idle stop when bgTasks is -1 (unknown)")
		}
	})
}

// TestAgentChildMissingPermissionRecordFailsClosed verifies that an
// agent.spawn child whose permission.json is missing fails admission closed
// instead of launching with an empty authority, while legitimate callers
// without permission.json (Pi children, non-parented conversations, hosted
// top-level leads) keep working.
func TestAgentChildMissingPermissionRecordFailsClosed(t *testing.T) {
	refusals := &wakeRefusals{}
	h, _ := agentHarness(t, func(o *Options) { o.WakeRefused = refusals.add })
	lead := h.leadCaller()

	// 1. Spawn a read-only agent child, settle and sleep it, then delete permission.json.
	a, ap := h.agentRunning(lead, AgentSpawnRequest{Prompt: "audit", Tier: "small", Authority: "read-only"})
	ap.reply("done")
	eventually(t, "a's turn settled", func() bool { return h.state(a).Status == StatusIdle })
	h.asleep(a)
	permPath := filepath.Join(wsdashboard.SessionDir(h.dir, a), "permission.json")
	if err := os.Remove(permPath); err != nil {
		t.Fatal(err)
	}
	starts := h.backend.startCount()
	if _, err := h.runner.Deliver(a, Delivery{From: "senderreplyid", Text: "wake without permission.json"}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "missing permission.json refused", func() bool {
		return strings.Contains(h.state(a).Error, "no explicit authority record")
	})
	time.Sleep(80 * time.Millisecond)
	if n := h.backend.startCount(); n != starts {
		t.Fatalf("child without permission.json launched %d processes", n-starts)
	}
	if st := h.state(a); st.Process || st.Held == 0 || AgentStatus(st) != AgentWaiting {
		t.Fatalf("refused child state = %+v", st)
	}
	if got := refusals.list(); len(got) != 1 || got[0][1] != a || !strings.Contains(got[0][2], "no explicit authority record") {
		t.Fatalf("wake refusals = %v", got)
	}

	// A pre-provenance permission.json still identifies this internal-profile
	// child as agent.spawn delegation and lets the next delivery wake it.
	permissionSelections.Delete(wsdashboard.SessionDir(h.dir, a))
	if err := os.WriteFile(permPath, []byte("{\n  \"permission_level\": \"read-only\"\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.Deliver(a, Delivery{From: "senderreplyid", Text: "wake restored"}); err != nil {
		t.Fatal(err)
	}
	p := h.backend.waitProc(t, starts)
	p.nextSent(t)
	if lvl := h.backend.start(starts).WSMCP.Env[EnvPermissionLevel]; lvl != string(PermissionReadOnly) {
		t.Fatalf("restored child EnvPermissionLevel = %q, want %q", lvl, PermissionReadOnly)
	}

	// 2. Legitimate Pi child (pi.spawn) has no permission.json and still launches.
	piStarts := h.backend.startCount()
	parent := leadParent
	piChild, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "pi task", Tier: "small", Parent: &parent})
	if err != nil {
		t.Fatalf("PiWorkerProfile spawn: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wsdashboard.SessionDir(h.dir, piChild.ID), "permission.json")); !os.IsNotExist(err) {
		t.Fatalf("PiWorkerProfile should not write permission.json, stat err = %v", err)
	}
	h.backend.waitProc(t, piStarts).nextSent(t)

	// 3. Legitimate hosted top-level lead without permission.json still works.
	lh, _ := agentLeadHarness(t)
	hostedLead := lh.lead(BackendClaude, LeadOptions{PermissionMode: "auto"})
	if _, err := os.Stat(filepath.Join(wsdashboard.SessionDir(lh.dir, hostedLead.ID), "permission.json")); !os.IsNotExist(err) {
		t.Fatalf("legacy hosted lead should not have permission.json, stat err = %v", err)
	}
	if _, err := lh.runner.Send(hostedLead.ID, "hello lead"); err != nil {
		t.Fatalf("hosted lead send: %v", err)
	}
	lh.holder(hostedLead.ID)
}

// TestAgentSpawnWidthCapAtomicUnderConcurrentSpawns verifies that concurrent
// AgentSpawn calls racing for the remaining width slots cannot exceed
// MaxSpawnWidth.
