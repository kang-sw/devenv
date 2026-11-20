package wsagent

import (
	"encoding/json"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsholder"
)

func TestDowngradeSettledChildRequiresAttributedSettlement(t *testing.T) {
	at := time.Date(2026, 11, 20, 12, 0, 5, 0, time.UTC)
	child := Record{HolderID: "holder", HandedOffAt: "fence", TurnOpen: true, TurnStartedAt: at.Add(-time.Second).UnixMilli(), Parent: &ParentRef{Kind: ParentConversation, ReplyID: "parent"}}
	parent := Record{Sends: 1, LastActiveAt: at.Format(time.RFC3339), LastTurn: &PendingReport{Mark: 1, Result: TurnResult{Status: TurnDone}}}
	if !downgradeSettledChild(child, parent) {
		t.Fatal("ordered settled parent must retire its fenced open child")
	}
	for _, tc := range []struct {
		name string
		edit func(*Record, *Record)
	}{
		{"lead parent", func(c, p *Record) { c.Parent = &ParentRef{Kind: ParentLead} }},
		{"unfenced", func(c, p *Record) { c.HandedOffAt = "" }},
		{"no holder", func(c, p *Record) { c.HolderID = "" }},
		{"idle child", func(c, p *Record) { c.TurnOpen = false }},
		{"unknown child start", func(c, p *Record) { c.TurnStartedAt = 0 }},
		{"mail woken later", func(c, p *Record) { c.TurnStartedAt = at.Add(time.Second).UnixMilli() }},
		{"equal", func(c, p *Record) { c.TurnStartedAt = at.UnixMilli() }},
		{"same second", func(c, p *Record) { c.TurnStartedAt = at.Add(123 * time.Millisecond).UnixMilli() }},
		{"held mail", func(c, p *Record) { p.Mail = []string{"pending"} }},
		{"queued input", func(c, p *Record) { p.Queued = []QueuedInput{{Text: "pending"}} }},
		{"pending send", func(c, p *Record) { p.Sends = 2 }},
		{"parent open", func(c, p *Record) { p.TurnOpen = true }},
		{"parent held", func(c, p *Record) { p.HeldTurn = &TurnResult{Status: TurnDone} }},
		{"no terminal", func(c, p *Record) { p.LastTurn = nil }},
		{"bad stamp", func(c, p *Record) { p.LastActiveAt = "bad" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, p := child, parent
			tc.edit(&c, &p)
			if downgradeSettledChild(c, p) {
				t.Fatal("ambiguous or current work must be adopted")
			}
		})
	}
}

func TestWorkerHolderOwnership(t *testing.T) {
	for _, backend := range []string{BackendClaude, BackendCodex, BackendAgy, BackendPi} {
		t.Run(backend, func(t *testing.T) {
			rec := Record{Backend: backend, ReplyID: "child", Root: "/root", Cwd: "/work", HostPID: 123, Capability: "secret"}
			meta := holderSession{Kind: holderKindAgentWorker, Backend: backend, ReplyID: "child", Cwd: "/work", Capability: capabilityHash("secret")}
			child := wsholder.ChildStatus{PID: 123}
			if !workerHolderMatches(rec, meta, &child) {
				t.Fatal("matching worker rejected")
			}
			for _, tc := range []struct {
				name string
				edit func(*holderSession, *wsholder.ChildStatus)
			}{
				{"kind", func(m *holderSession, c *wsholder.ChildStatus) { m.Kind = holderKindLead }},
				{"backend", func(m *holderSession, c *wsholder.ChildStatus) { m.Backend = "foreign" }},
				{"reply", func(m *holderSession, c *wsholder.ChildStatus) { m.ReplyID = "foreign" }},
				{"cwd", func(m *holderSession, c *wsholder.ChildStatus) { m.Cwd = "/foreign" }},
				{"capability", func(m *holderSession, c *wsholder.ChildStatus) { m.Capability = "foreign" }},
				{"pid", func(m *holderSession, c *wsholder.ChildStatus) { c.PID = 456 }},
			} {
				t.Run(tc.name, func(t *testing.T) {
					m, c := meta, child
					tc.edit(&m, &c)
					if workerHolderMatches(rec, m, &c) {
						t.Fatal("foreign holder accepted")
					}
				})
			}
			meta.Kind, meta.Backend = holderKindPiWorker, ""
			if workerHolderMatches(rec, meta, &child) != (backend == BackendPi) {
				t.Fatal("legacy descriptor applies only to Pi")
			}
		})
	}
}

func TestDowngradeRetirementPersistsSleepingWorker(t *testing.T) {
	h, reports := newFakePiRecoveryHarness(t)
	worker, holder := spawnRecoveryWorker(t, h)
	h.runner.Close()
	h.holders.restart()
	child := h.record(worker.ID)
	now := h.opts.Now().UTC().Truncate(time.Second)
	child.Parent = &ParentRef{Kind: ParentConversation, ReplyID: "zzparent"}
	child.TurnStartedAt = now.Add(-2 * time.Second).UnixMilli()
	parent := Record{ReplyID: "zzparent", Root: h.root, Backend: BackendPi, Profile: PiWorkerProfile, WSKey: "parent-key", Parent: &leadParent,
		ResolvedDefinition: child.ResolvedDefinition, CreatedAt: now.Format(time.RFC3339), LastActiveAt: now.Format(time.RFC3339), Sends: 1, Drained: 1,
		LastTurn: &PendingReport{Mark: 1, Result: TurnResult{ID: "zzparent", Status: TurnDone}}}
	for _, rec := range []Record{child, parent} {
		if err := writeRecord(wsdashboard.SessionDir(h.dir, rec.ReplyID), rec); err != nil {
			t.Fatal(err)
		}
	}
	// Completion accumulated during downgrade must never publish a second report.
	h.holders.out(holder, recoveryPiReply("late result")...)
	h.start()
	wait(t, "holder scan", h.runner.rejoined)
	eventually(t, "silent retirement", func() bool { return h.record(worker.ID).HolderID == "" })
	assertSleeping := func() {
		rec := h.record(worker.ID)
		if rec.TurnOpen || rec.TurnStartedAt != 0 || rec.HolderID != "" || rec.HostPID != 0 || rec.HolderSeq != 0 || rec.HandedOffAt != "" || rec.ClosedAt != "" || rec.LastTurn != nil {
			t.Fatalf("retirement did not preserve sleeping shape: %+v", rec)
		}
		if len(reports.list()) != 0 {
			t.Fatalf("retirement reported again: %+v", reports.list())
		}
	}
	assertSleeping()
	if h.holders.opCount("stop "+holder) != 1 {
		t.Fatal("retired holder was not stopped once")
	}
	h.restart()
	wait(t, "second scan", h.runner.rejoined)
	assertSleeping()
	h.send(worker.ID, "wake again")
	eventually(t, "sleeping child wakes", func() bool {
		rec := h.record(worker.ID)
		return rec.HolderID != "" && rec.HolderID != holder && rec.TurnOpen
	})
}

func TestWorkerMCPRestoreRequiresExactTuple(t *testing.T) {
	for _, mutation := range []string{"valid", "holder", "url", "environment", "token"} {
		t.Run(mutation, func(t *testing.T) {
			r := NewRunner(Options{Dir: t.TempDir(), Backends: map[string]BackendRegistration{BackendCodex: {WorkerHTTPMCP: true}}})
			r.SetEndpoint("http://127.0.0.1:1")
			rec := Record{ReplyID: "child", Backend: BackendCodex, Root: "/root", HolderID: "holder", HostPID: 123, MCPToken: "existing-token", MCPURL: r.mcpURL(), MCPHolder: "holder", MCPEnv: map[string]string{EnvAgentSessionKey: "child-key"}}
			switch mutation {
			case "holder":
				rec.MCPHolder = "foreign"
			case "url":
				rec.MCPURL = "http://foreign"
			case "environment":
				rec.MCPEnv = nil
			case "token":
				rec.MCPToken = ""
			}
			c := &conv{r: r, rec: rec}
			c.restoreWorkerMCP()
			hello, _, ok := r.ResolveMCPToken("existing-token")
			if ok != (mutation == "valid") || c.mcpStale == (mutation == "valid") {
				t.Fatalf("tuple %s: restored=%v stale=%v", mutation, ok, c.mcpStale)
			}
			if ok && hello.Env[EnvAgentSessionKey] != "child-key" {
				t.Fatal("restored hello lost its worker binding")
			}
		})
	}
}

func TestWorkerOwnershipMismatchNeverStopsForeignHolder(t *testing.T) {
	h, _ := newFakePiRecoveryHarness(t)
	worker, holder := spawnRecoveryWorker(t, h)
	h.runner.Close()
	h.holders.restart()
	h.holders.set(func() {
		var meta holderSession
		if err := json.Unmarshal(h.holders.holders[holder].session, &meta); err != nil {
			t.Fatal(err)
		}
		meta.Cwd = "/foreign"
		raw, _ := json.Marshal(meta)
		h.holders.holders[holder].session = raw
	})
	h.start()
	wait(t, "ownership scan", h.runner.rejoined)
	eventually(t, "ownership rejected", func() bool { return h.record(worker.ID).HolderID == "" })
	if h.holders.opCount("stop") != 0 {
		t.Fatalf("foreign holder stopped: %v", h.holders.opList())
	}
	if rec := h.record(worker.ID); rec.HandedOffAt != "" || rec.Capability != "" || rec.TurnOpen {
		t.Fatal("rejected association retained an active capability or fence")
	}
}
