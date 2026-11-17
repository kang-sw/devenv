package wsagent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

// fixtureInit is the first system/init line of a recorded Claude fixture.
func fixtureInit(t *testing.T, name string) map[string]any {
	t.Helper()
	return fixtureInits(t, name)[0]
}

// fixtureInits are every system/init line of a recorded Claude fixture, one
// per turn.
func fixtureInits(t *testing.T, name string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if bytes.HasPrefix(sc.Bytes(), []byte(">> ")) {
			continue
		}
		var line map[string]any
		if json.Unmarshal(sc.Bytes(), &line) == nil && line["type"] == "system" && line["subtype"] == "init" {
			out = append(out, line)
		}
	}
	if len(out) == 0 {
		t.Fatalf("%s has no init line", name)
	}
	return out
}

// reportOf maps init through a mapper for spec and returns the catalog
// report it sends, failing on a refused launch.
func reportOf(t *testing.T, spec StartSpec, init map[string]any) HarnessReport {
	t.Helper()
	raw, err := json.Marshal(init)
	if err != nil {
		t.Fatal(err)
	}
	var report *HarnessReport
	for _, ev := range newClaudeMapper(spec).mapLine(raw, nopReplies{}) {
		if ev.Fatal != nil {
			t.Fatalf("the init is refused: %v", ev.Fatal)
		}
		if ev.Harness != nil {
			if report != nil {
				t.Fatal("one report per init")
			}
			report = ev.Harness
		}
	}
	if report == nil {
		t.Fatal("the init sends no catalog report")
	}
	return *report
}

// resolveFixture is the capabilities a session started from fixture's init
// shows: the Claude profile at its version, demoted by its shape.
func resolveFixture(t *testing.T, spec StartSpec, init map[string]any) Capabilities {
	t.Helper()
	role := RoleWorker
	if spec.Lead != nil {
		role = RoleLead
	}
	report := reportOf(t, spec, init)
	caps := ClaudeCatalog().Resolve(role, report.Version)
	caps.demote(report.Demoted)
	return caps
}

func fixtureWorkerSpec(t *testing.T) StartSpec {
	return claudeSpec(t, `{"backend":"claude","tools":["Read"]}`)
}

// TestClaudeFixturesResolveByVersion: each recorded init resolves at its
// own version inside the guaranteed range, with nothing demoted; one below
// the range (the 2.1.319 worker turns) and one above it run on the nearest
// bound's grades, marked unverified.
func TestClaudeFixturesResolveByVersion(t *testing.T) {
	p := ClaudeCatalog()
	for _, tc := range []struct {
		fixture, version, at string
		lead, unverified     bool
	}{
		{fixture: "claude-tool-turn.jsonl", version: "2.1.319", at: p.Oldest, unverified: true},
		{fixture: "claude-worker-turn-2.1.322.jsonl", version: "2.1.322", at: "2.1.322"},
		{fixture: "claude-worker-turn-2.1.326.jsonl", version: "2.1.326", at: "2.1.326"},
		{fixture: "claude-lead-slash-commands.jsonl", version: "2.1.322", at: "2.1.322", lead: true},
		{fixture: "claude-lead-turn-2.1.326.jsonl", version: "2.1.326", at: "2.1.326", lead: true},
		{fixture: "claude-lead-turn-2.1.326.jsonl", version: "2.1.399", at: p.Newest, lead: true, unverified: true},
		{fixture: "claude-worker-turn-2.1.326.jsonl", version: "2.1.399", at: p.Newest, unverified: true},
	} {
		t.Run(tc.fixture+"@"+tc.version, func(t *testing.T) {
			init := fixtureInit(t, tc.fixture)
			if init["claude_code_version"] != tc.version {
				if !tc.unverified {
					t.Fatalf("the fixture records %v", init["claude_code_version"])
				}
				init["claude_code_version"] = tc.version
			}
			spec := fixtureWorkerSpec(t)
			role := RoleWorker
			if tc.lead {
				spec, role = leadSpec(), RoleLead
			}
			caps := resolveFixture(t, spec, init)
			if caps.Version != tc.version || caps.ProfileVersion != tc.at || caps.Unverified != tc.unverified || caps.Role != role {
				t.Fatalf("resolved %+v, want version %s at %s unverified %v", caps, tc.version, tc.at, tc.unverified)
			}
			if len(caps.Demoted) != 0 {
				t.Fatalf("a recorded init demotes nothing: %v", caps.Demoted)
			}
			want := p.Resolve(role, tc.at)
			for _, item := range CatalogItems {
				if caps.Grade(item) != want.Grade(item) {
					t.Errorf("%s = %s, want the bound's %s", item, caps.Grade(item), want.Grade(item))
				}
			}
		})
	}
}

// TestClaudeInitShapeDemotes: an init field missing what an item's grade
// relies on demotes that item, and only an item graded better than
// unsupported is listed; the Agent/Task init alias is no mismatch, and an
// absent command list (an older harness) leaves compaction alone. A
// worker's init check stays the fatal one.
func TestClaudeInitShapeDemotes(t *testing.T) {
	lead := func(edit func(init map[string]any)) Capabilities {
		init := fixtureInit(t, "claude-lead-turn-2.1.326.jsonl")
		edit(init)
		return resolveFixture(t, leadSpec(), init)
	}
	for name, tc := range map[string]struct {
		edit func(map[string]any)
		want []CapabilityItem
	}{
		"no permission mode":       {func(i map[string]any) { delete(i, "permissionMode") }, []CapabilityItem{ItemApproval}},
		"commands without compact": {func(i map[string]any) { i["slash_commands"] = []string{"review"} }, []CapabilityItem{ItemCompaction}},
		"no command list":          {func(i map[string]any) { delete(i, "slash_commands") }, nil},
		"no model":                 {func(i map[string]any) { delete(i, "model") }, []CapabilityItem{ItemModelEffort}},
		"no tools":                 {func(i map[string]any) { delete(i, "tools") }, []CapabilityItem{ItemToolRemoval, ItemSubagentRemoval}},
	} {
		t.Run("lead "+name, func(t *testing.T) {
			caps := lead(tc.edit)
			if !slices.Equal(caps.Demoted, tc.want) {
				t.Fatalf("demoted %v, want %v", caps.Demoted, tc.want)
			}
			for _, item := range tc.want {
				if caps.Allows(item) {
					t.Fatalf("%s still allowed", item)
				}
			}
			if caps.Unverified {
				t.Fatal("a shape mismatch is not a version warning")
			}
		})
	}

	worker := func(profile string, edit func(init map[string]any)) Capabilities {
		init := fixtureInit(t, "claude-worker-turn-2.1.326.jsonl")
		edit(init)
		return resolveFixture(t, claudeSpec(t, profile), init)
	}
	removal := []CapabilityItem{ItemToolRemoval, ItemSubagentRemoval}
	if got := worker(`{"backend":"claude","tools":["Read","Agent"]}`, func(map[string]any) {}).Demoted; !slices.Equal(got, removal) {
		t.Fatalf("a profile tool the init does not list: demoted %v", got)
	}
	if got := worker(`{"backend":"claude","tools":["Read","Agent"]}`, func(i map[string]any) { i["tools"] = []string{"Read", "Task"} }).Demoted; len(got) != 0 {
		t.Fatalf("the older init name of Agent is no mismatch: demoted %v", got)
	}
	if got := worker(`{"backend":"claude","tools":["Read"]}`, func(i map[string]any) { delete(i, "tools") }).Demoted; !slices.Equal(got, removal) {
		t.Fatalf("no tool list: demoted %v", got)
	}
	if got := worker(`{"backend":"claude","tools":["Read"]}`, func(i map[string]any) { delete(i, "slash_commands") }).Demoted; len(got) != 0 {
		t.Fatalf("a worker's compaction is already unsupported: demoted %v", got)
	}

	init := fixtureInit(t, "claude-worker-turn-2.1.326.jsonl")
	init["tools"] = []string{"Read", "Bash"}
	raw, _ := json.Marshal(init)
	evs := newClaudeMapper(fixtureWorkerSpec(t)).mapLine(raw, nopReplies{})
	if len(evs) != 1 || evs[0].Fatal == nil {
		t.Fatalf("a tool outside the profile still refuses the launch: %+v", evs)
	}
}

// TestClaudeInitShapeSubtractsRemovedTools: the tools a launch removed on
// purpose are subtracted from the expected init tool set, so their absence
// demotes nothing, while a removed tool the init still lists (the removal
// did not take, or a process started without it rejoined) demotes tool and
// subagent removal for a lead and refuses a worker's launch.
func TestClaudeInitShapeSubtractsRemovedTools(t *testing.T) {
	removal := []CapabilityItem{ItemToolRemoval, ItemSubagentRemoval}
	lead := leadSpec()
	lead.RemoveSubagents = true
	for name, tc := range map[string]struct {
		tools []string
		want  []CapabilityItem
	}{
		"removed":           {[]string{"Bash", "Read"}, nil},
		"still listed":      {[]string{"Task", "Bash", "Read"}, removal},
		"still listed, new": {[]string{"Agent", "Bash", "Read"}, removal},
	} {
		t.Run("lead "+name, func(t *testing.T) {
			init := fixtureInit(t, "claude-lead-turn-2.1.326.jsonl")
			init["tools"] = tc.tools
			if got := resolveFixture(t, lead, init).Demoted; !slices.Equal(got, tc.want) {
				t.Fatalf("demoted %v, want %v", got, tc.want)
			}
		})
	}

	// A worker profile that lists the subagent tool: the launch's removal
	// wins, so the init without it is no mismatch, and with it the leak
	// refuses the launch.
	worker := claudeSpec(t, `{"backend":"claude","tools":["Read","Agent"]}`)
	worker.RemoveSubagents = true
	init := fixtureInit(t, "claude-worker-turn-2.1.326.jsonl")
	init["tools"] = []string{"Read"}
	if got := resolveFixture(t, worker, init).Demoted; len(got) != 0 {
		t.Fatalf("a removed profile tool missing from init: demoted %v", got)
	}
	for _, leaked := range []string{"Agent", "Task"} {
		init["tools"] = []string{"Read", leaked}
		raw, _ := json.Marshal(init)
		evs := newClaudeMapper(worker).mapLine(raw, nopReplies{})
		if len(evs) != 1 || evs[0].Fatal == nil {
			t.Fatalf("a removed tool %s the init lists must refuse the worker's launch: %+v", leaked, evs)
		}
	}
}

// TestClaudeSubagentRemovedFixtures: the recorded hosted lead (a turn
// delegating through agent.spawn, then the turn the child's completion
// mail woke) and the recorded agent.spawn child, both launched without the
// native subagent tool, list neither of its names at any init. Under their
// removal spec nothing is demoted and both removal items stay native. A
// lead whose init still lists the tool, as one started before the removal
// and rejoined after it does, demotes both. The lead's tool list is
// shortened to its native tools and a few ws tools; ids and paths are
// neutral.
func TestClaudeSubagentRemovedFixtures(t *testing.T) {
	lead := leadSpec()
	lead.RemoveSubagents = true
	p, err := ParseProfile(BuiltinProfiles()[ProfilePrefix+AgentWorkerProfile(BackendClaude)])
	if err != nil {
		t.Fatal(err)
	}
	worker := StartSpec{Root: "/work", Profile: p, WSMCP: &MCPCommand{Command: "/bin/ws-mcp"}, RemoveSubagents: true}
	for _, tc := range []struct {
		fixture string
		spec    StartSpec
		inits   int
	}{
		{"claude-lead-subagent-removed-2.1.326.jsonl", lead, 2},
		{"claude-worker-subagent-removed-2.1.326.jsonl", worker, 1},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			inits := fixtureInits(t, tc.fixture)
			if len(inits) != tc.inits {
				t.Fatalf("%d init lines, want %d", len(inits), tc.inits)
			}
			for _, init := range inits {
				tools, _ := init["tools"].([]any)
				if len(tools) == 0 || slices.Contains(tools, any("Agent")) || slices.Contains(tools, any("Task")) {
					t.Fatalf("init tools %v", tools)
				}
				caps := resolveFixture(t, tc.spec, init)
				if caps.Version != "2.1.326" || caps.Unverified || len(caps.Demoted) != 0 {
					t.Fatalf("resolved %+v", caps)
				}
				for _, item := range []CapabilityItem{ItemToolRemoval, ItemSubagentRemoval} {
					if caps.Grade(item) != GradeNative {
						t.Errorf("%s = %s, want native", item, caps.Grade(item))
					}
				}
			}
		})
	}
	if got := resolveFixture(t, lead, fixtureInit(t, "claude-lead-turn-2.1.326.jsonl")).Demoted; !slices.Equal(got, []CapabilityItem{ItemToolRemoval, ItemSubagentRemoval}) {
		t.Fatalf("an init listing the removed tool demoted %v", got)
	}

	// The lead's recording replays as two sent turns, the second answering
	// with the child's reply.
	r := newLeadReplay(t)
	r.all(readLeadFixture(t, "claude-lead-subagent-removed-2.1.326.jsonl"))
	if got := ringTexts(r.ring, "reply"); r.ends != 2 || !slices.Equal(r.opened, []string{"send", "send"}) || len(got) != 2 || got[1] != "CHILD SAID: `# probe`" {
		t.Fatalf("replay: ends %d, opened %v, replies %q", r.ends, r.opened, got)
	}
}

// TestAllowedCommands: a command is offered while its item is allowed;
// without a resolved profile every command is.
func TestAllowedCommands(t *testing.T) {
	list := []wslive.SlashCommand{{Name: "compact"}, {Name: "review"}}
	if got := allowedCommands(list, nil); !slices.Equal(got, list) {
		t.Fatalf("no profile: %v", got)
	}
	caps := ClaudeCatalog().Resolve(RoleLead, "2.1.326")
	if got := allowedCommands(list, &caps); !slices.Equal(got, list) {
		t.Fatalf("native compaction: %v", got)
	}
	caps.demote([]CapabilityItem{ItemCompaction})
	if got := allowedCommands(list, &caps); !slices.Equal(got, list[1:]) {
		t.Fatalf("unsupported compaction: %v", got)
	}
	if len(list) != 2 {
		t.Fatal("the list given is not changed")
	}
}
