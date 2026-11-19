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
	prompt := appendedPrompt(t, h.holders.spawned(holder).Argv)
	if !strings.HasPrefix(prompt, leadKeySentence(key)+"Never use the native `Agent` tool. Start a subagent only with the ws `agent.spawn` tool") ||
		!strings.Contains(prompt, "This session has no ws skills: a `/<name>` reference such as `/lead-run` names a workflow playbook.") {
		t.Fatalf("the lead prompt carries no steer or command note:\n%s", prompt)
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
}
