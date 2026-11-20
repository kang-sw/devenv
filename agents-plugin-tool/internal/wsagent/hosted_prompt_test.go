package wsagent

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

// TestSubagentSteerFollowsPolicyAndCatalog: the native-subagent line goes
// to a session under the removal policy (a lead, an agent.spawn worker)
// whose backend's catalog names its native subagent tool for that role,
// whatever the harness can physically remove; a tracker support
// conversation, a backend with no catalog, and a role the catalog names no
// tool for (agy's lead) get none.
func TestSubagentSteerFollowsPolicyAndCatalog(t *testing.T) {
	r := &Runner{opts: Options{Backends: map[string]BackendRegistration{
		BackendClaude: {Catalog: ClaudeCatalog()},
		BackendCodex:  {Catalog: CodexCatalog()},
		BackendAgy:    {Catalog: AgyCatalog()},
		BackendPi:     {},
	}}}
	steer := func(tool string) string {
		return "Never use the native `" + tool + "` tool. Start a subagent only with the ws `agent.spawn` tool, and continue one with the ws `mailbox.send` tool."
	}
	for _, tc := range []struct {
		name string
		rec  Record
		want string
	}{
		{"claude lead", Record{Backend: BackendClaude, Lead: true}, steer("Agent")},
		{"claude worker", Record{Backend: BackendClaude, Profile: AgentWorkerProfile(BackendClaude)}, steer("Agent")},
		{"codex lead", Record{Backend: BackendCodex, Lead: true}, steer("spawn_agent")},
		{"agy lead names no tool", Record{Backend: BackendAgy, Lead: true}, ""},
		{"tracker support is outside the policy", Record{Backend: BackendClaude, Profile: "tracker-support"}, ""},
		{"no catalog", Record{Backend: BackendPi, Lead: true}, ""},
		{"unregistered backend", Record{Backend: "other", Lead: true}, ""},
	} {
		if got := r.subagentSteer(tc.rec); got != tc.want {
			t.Errorf("%s: steer = %q, want %q", tc.name, got, tc.want)
		}
	}
	var none *HarnessProfile
	if got := none.NativeSubagentTool(RoleLead); got != "" {
		t.Fatalf("a nil catalog names %q", got)
	}
}

// TestPlaybookCommandTurnText: a rendered playbook rides in the turn with
// the command's arguments, so the session needs no playbook.read; an empty
// body points at that read instead.
func TestPlaybookCommandTurnText(t *testing.T) {
	got := playbookCommandTurn(wsinbox.Command{Name: "lead-run", Args: " ticket-x "}, "# Lead Run\nbody")
	want := "The user ran the `/lead-run` command with these arguments:\n\nticket-x\n\n" +
		"Run the `lead-run` playbook below; it is the whole playbook, so it needs no `playbook.read`.\n\n" +
		"<playbook name=\"lead-run\">\n# Lead Run\nbody\n</playbook>"
	if got != want {
		t.Fatalf("turn =\n%s\nwant\n%s", got, want)
	}
	got = playbookCommandTurn(wsinbox.Command{Name: "lead-review"}, "")
	want = "The user ran the `/lead-review` command with no arguments.\n\n" +
		"Read the `lead-review` playbook with the ws `playbook.read` tool and run it."
	if got != want {
		t.Fatalf("pointer turn =\n%s\nwant\n%s", got, want)
	}
}

// TestHostedLeadPlaybookCommands: a hosted lead of a backend with
// PlaybookCommands offers the lead playbooks as dashboard commands from its
// creation, merged into the harness's own list (a harness command of the
// same name gives way), and its prompt carries the native-subagent line and
// says what a `/lead-*` reference is. Running one sends the rendered
// playbook as the turn (one render per run, none at launch), a render
// failure sends the pointer to playbook.read, and any other command still
// runs as the harness's own; the page's user entry is the typed line. The
// list is read once per runner.
func TestHostedLeadPlaybookCommands(t *testing.T) {
	h := newLeadHarness(t)
	reg := h.opts.Backends[BackendClaude]
	reg.Catalog, reg.PlaybookCommands = ClaudeCatalog(), true
	h.opts.Backends[BackendClaude] = reg
	var loads atomic.Int32
	h.opts.PlaybookCommands = func() ([]wslive.SlashCommand, error) {
		loads.Add(1)
		return []wslive.SlashCommand{{Name: "lead-run", Description: "Run a ticket", Source: "pkg", Layer: "x"}, {Name: "review", Description: "Review playbook"}}, nil
	}
	var mu sync.Mutex
	var expands [][4]string
	h.opts.ExpandCommand = func(backend, dir, key, name string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		expands = append(expands, [4]string{backend, dir, key, name})
		if name == "review" {
			return "partial", errors.New("render broke")
		}
		return "\n# Lead Run\nbody\n", nil
	}
	h.opts.LeadManual = (&leadManualRender{text: "\n# Workflow Manual\nrendered\n"}).render
	h.start()

	playbooks := []wslive.SlashCommand{
		{Name: "lead-run", Description: "Run a ticket", Source: PlaybookCommandSource},
		{Name: "review", Description: "Review playbook", Source: PlaybookCommandSource},
	}
	if got := h.snapshot(h.lead(BackendClaude, LeadOptions{}).ID).Commands; !slices.Equal(got, playbooks) {
		t.Fatalf("a new lead offers its playbook commands before any launch: %+v", got)
	}
	if got := h.snapshot(h.lead(BackendPi, LeadOptions{}).ID).Commands; len(got) != 0 {
		t.Fatalf("a backend without PlaybookCommands offers none: %+v", got)
	}

	h.holders.setControl("initialize", "", `{"commands":[{"name":"compact","description":"Free up context"},{"name":"review","description":"Harness review"}]}`)
	id, holder := h.runLead("one")
	key := h.record(id).WSKey
	merged := append([]wslive.SlashCommand{{Name: "compact", Description: "Free up context"}}, playbooks...)
	eventually(t, "the merged command list", func() bool { return slices.Equal(h.snapshot(id).Commands, merged) })
	wantPrompt := leadKeySentence(key) +
		"Never use the native `Agent` tool. Start a subagent only with the ws `agent.spawn` tool, and continue one with the ws `mailbox.send` tool. " +
		"This session has no ws skills: a `/<name>` reference such as `/lead-run` names a workflow playbook. " +
		"The user runs one as that dashboard command, which sends you its playbook; you run one by reading it with the ws `playbook.read` tool and following it. " +
		"The workflow manual below is binding; its session state is as of this session's start. " +
		"It stays in this prompt, compactions included, so do not read it again with the workflow_manual tool: " +
		"for fresh session state, and where the manual says to invoke `/lead-revive`, call the ws `workflow_state` tool with this key.\n\n" +
		"# Workflow Manual\nrendered\n"
	if prompt := appendedPrompt(t, h.holders.spawned(holder).Argv); prompt != wantPrompt {
		t.Fatalf("lead prompt =\n%s\nwant\n%s", prompt, wantPrompt)
	}
	h.holders.out(holder, turnLines("sess-1", "u1", "first")...)
	h.idle(id)

	h.sendCommand(id, "lead-run", "ticket-x")
	run := playbookCommandTurn(wsinbox.Command{Name: "lead-run", Args: "ticket-x"}, "# Lead Run\nbody")
	eventually(t, "the playbook turn", func() bool { return slices.Equal(h.userInputs(holder), []string{h.envelope("one"), run}) })
	h.holders.out(holder, turnLines("sess-1", "u2", "ran")...)
	h.idle(id)

	h.sendCommand(id, "review", "")
	pointer := playbookCommandTurn(wsinbox.Command{Name: "review"}, "")
	eventually(t, "the pointer turn", func() bool { return slices.Equal(h.userInputs(holder), []string{h.envelope("one"), run, pointer}) })
	h.holders.out(holder, turnLines("sess-1", "u3", "read")...)
	h.idle(id)
	if !strings.Contains(h.log.String(), "/review: playbook not rendered, the turn points at it: render broke") {
		t.Fatalf("the render failure is not logged: %s", h.log.String())
	}

	h.sendCommand(id, "compact", "")
	eventually(t, "the harness command", func() bool {
		return slices.Equal(h.userInputs(holder), []string{h.envelope("one"), run, pointer, "/compact"})
	})
	h.holders.out(holder, turnLines("sess-1", "u4", "compacted")...)
	h.idle(id)

	if got, want := ringTexts(h.ring(id), wslive.RingUser), []string{h.envelope("one"), "/lead-run ticket-x", "/review", "/compact"}; !slices.Equal(got, want) {
		t.Fatalf("ring user entries = %q, want %q", got, want)
	}
	mu.Lock()
	got := slices.Clone(expands)
	mu.Unlock()
	if len(got) != 2 || got[0][0] != BackendClaude || !sameDir(got[0][1], h.root) || got[0][2] != key || got[0][3] != "lead-run" || got[1][3] != "review" {
		t.Fatalf("each playbook command renders once, for the lead's backend, directory, and key: %v", got)
	}
	if n := loads.Load(); n != 1 {
		t.Fatalf("the playbook command list is read once per runner: %d reads", n)
	}

	// A rejoined lead offers the new runner's playbook commands, in place of
	// the earlier runner's, beside the harness commands its snapshot kept,
	// without waiting for a new launch.
	h.opts.PlaybookCommands = func() ([]wslive.SlashCommand, error) {
		return []wslive.SlashCommand{{Name: "lead-run", Description: "Run a ticket"}, {Name: "lead-ship", Description: "Ship"}}, nil
	}
	// The holder scan, whose replay would bring the harness's list back, is
	// held: the list read is the one the rejoin itself offers.
	h.runner.Close()
	h.holders.restart()
	entered, release := h.gateScan()
	defer release()
	h.start()
	wait(t, "the first scan", entered)
	var offered []wslive.SlashCommand
	c := h.runner.conv(id)
	c.do(func() { offered = c.live.st.Snapshot(h.opts.Now().UnixMilli()).Commands })
	rejoined := []wslive.SlashCommand{
		{Name: "compact", Description: "Free up context"},
		{Name: "lead-run", Description: "Run a ticket", Source: PlaybookCommandSource}, {Name: "lead-ship", Description: "Ship", Source: PlaybookCommandSource},
	}
	if !slices.Equal(offered, rejoined) {
		t.Fatalf("rejoined command list = %+v, want %+v", offered, rejoined)
	}
}

// TestAgentChildPromptCarriesTheSteer: an agent.spawn child of a backend
// whose catalog names its native subagent tool gets the parent return address
// and the never-use line right after its key sentence.
func TestAgentChildPromptCarriesTheSteer(t *testing.T) {
	h, _ := agentHarness(t, func(o *Options) {
		reg := o.Backends[BackendClaude]
		reg.Catalog = ClaudeCatalog()
		o.Backends[BackendClaude] = reg
	})
	a, _ := h.agentRunning(h.leadCaller(), AgentSpawnRequest{Prompt: "go", Tier: "small", Authority: "read-only", SystemPrompt: "You are the cache probe."})
	text := instructionsOf(t, h.backend.start(0))
	want := "You are the cache probe.\n\nYour ws session key is `" + h.record(a).WSKey + "`. Pass it as the `session_key` argument of every ws tool call. " +
		"This key is bound to worktree `" + h.record(a).Root + "`; if work moves to another worktree, use a key minted for that destination (from a direct spawn there or a legitimate acquisition that returns one), because this key is never rebound. " +
		"Your direct parent's mailbox reply address is `id:" + leadParent.ReplyID + "`; send it reports or questions with `mailbox.send`. " +
		"Never use the native `Agent` tool. Start a subagent only with the ws `agent.spawn` tool, and continue one with the ws `mailbox.send` tool.\n"
	if text != want {
		t.Fatalf("the child's appended prompt = %q, want %q", text, want)
	}
}

// TestPlaybookCommandsFailedReadOffersNone: a failed list read is logged
// once and never retried, and a list a snapshot kept drops the playbook
// commands this runner cannot run; a backend without PlaybookCommands keeps
// its list as it is.
func TestPlaybookCommandsFailedReadOffersNone(t *testing.T) {
	log := &logBuffer{}
	reads := 0
	r := &Runner{opts: Options{Log: log, Backends: map[string]BackendRegistration{BackendClaude: {PlaybookCommands: true}, BackendPi: {}},
		PlaybookCommands: func() ([]wslive.SlashCommand, error) { reads++; return nil, errors.New("package broke") }}}
	kept := []wslive.SlashCommand{{Name: "compact"}, {Name: "lead-run", Source: PlaybookCommandSource}}
	for range 3 {
		if got := r.withPlaybookCommands(Record{Backend: BackendClaude, Lead: true}, kept); !slices.Equal(got, kept[:1]) {
			t.Fatalf("offered %+v, want the harness commands alone", got)
		}
	}
	if reads != 1 || strings.Count(log.String(), "no playbook commands: package broke") != 1 {
		t.Fatalf("a failed read is tried and logged once: %d reads, log %q", reads, log.String())
	}
	if got := r.withPlaybookCommands(Record{Backend: BackendPi, Lead: true}, kept); !slices.Equal(got, kept) {
		t.Fatalf("a backend without playbook commands changed its list: %+v", got)
	}
	if got := r.withPlaybookCommands(Record{Backend: BackendClaude}, kept); !slices.Equal(got, kept) {
		t.Fatalf("a worker's list changed: %+v", got)
	}
}
