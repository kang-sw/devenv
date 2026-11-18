package wsagent

import (
	"slices"
	"strconv"
	"strings"

	"github.com/kang-sw/devenv/internal/wslive"
)

// catalog.go is the harness capability catalog: the declarative common
// interface of a hosted harness. A harness joins by filling a profile of
// graded items per harness version and writing a Backend; shared code reads
// the grades, never a harness name.
//
// The catalog is a ceiling, not a replacement for the instance handles:
// runtime support of an optional control is the catalog grade AND the
// process's handle (controlsFor), and ErrUnsupported stays the restart
// signal of the required Process controls. A backend registered without a
// profile (BackendRegistration.Catalog nil) stays handle-only: its readers
// hide nothing, demote nothing, and warn of no version.
//
// At each process start the backend reports the harness version and the
// items its init event shows a mismatched shape for (HarnessReport); the
// runner resolves the profile at that version, demotes those items to
// unsupported, and shows the result as State.Capabilities, in memory only.
// A version outside the guaranteed range runs on the nearest bound's
// grades, marked unverified. The init-field to item mapping that gates the
// demotion is the backend's (claudeInitShape for Claude).
//
// Admission conditions are not items: a harness without headless JSON event
// streaming or MCP server injection is not usable at all, so nothing is
// graded for them.
//
// Inventory of the optional process controls (processControls):
//   - approval, mode    -> ItemApproval
//   - steer, reports    -> ItemSteer (reports is the worker steer path)
//   - commands          -> ItemCompaction (the command mapping /compact uses)
//   - mcp               -> left out: MCP status and reconnect recover the MCP
//     injection admission condition, which is never graded
//   - turns, recovery   -> left out: rejoin and holder replay reconciliation,
//     internal to recovery and not a capability a caller selects
//
// and of the required Process controls: Interrupt -> ItemInterrupt;
// SetModel, SetEffort -> ItemModelEffort; Send -> ItemFollowUp (the runner
// holds the message and sends it when the turn ends).

// CapabilityGrade is what the ws adapter actually delivers for an item on a
// harness version: not what the harness could do natively, so an item with
// no ws wiring is unsupported until it is wired.
type CapabilityGrade string

const (
	// GradeNative: the harness's own mechanism delivers the item.
	GradeNative CapabilityGrade = "native"
	// GradeSubstitute: ws delivers the item from outside the harness.
	GradeSubstitute CapabilityGrade = "ws-substitute"
	// GradeUnsupported: neither delivers it.
	GradeUnsupported CapabilityGrade = "unsupported"
)

// CapabilityItem names one catalog item.
type CapabilityItem string

// The catalog items, each a capability a harness may deliver.
const (
	// ItemSkillList: the session's skill list is known to ws.
	ItemSkillList CapabilityItem = "skill_list"
	// ItemSkillInjection: a skill's text reaches the session on request.
	ItemSkillInjection CapabilityItem = "skill_injection"
	// ItemCompaction: a compaction command pushed from outside the harness
	// (the dashboard's /compact) compacts the session.
	ItemCompaction CapabilityItem = "compaction"
	// ItemRewind: the conversation can be rewound (hidden or rebuilt).
	ItemRewind CapabilityItem = "rewind"
	// ItemToolRemoval: default tools can be removed at launch; Values is
	// the removable tool set (CatalogAnyValue: every built-in tool).
	ItemToolRemoval CapabilityItem = "tool_removal"
	// ItemSubagentRemoval: the native subagent tool can be removed, the key
	// item of tool removal; Values names that tool.
	ItemSubagentRemoval CapabilityItem = "subagent_removal"
	// ItemModelEffort: model and effort are selectable. The offered choices
	// come from the backend's ChoiceLister, not from the catalog.
	ItemModelEffort CapabilityItem = "model_effort"
	// ItemApproval: tool approval prompts reach ws and the permission mode
	// changes. The modes come from BackendRegistration.PermissionModes and
	// PermissionLevels, not from the catalog.
	ItemApproval CapabilityItem = "approval"
	// ItemSteer: a message reaches the running turn.
	ItemSteer CapabilityItem = "steer"
	// ItemInterrupt: the running turn stops and the session stays.
	ItemInterrupt CapabilityItem = "interrupt"
	// ItemFollowUp: a message is queued for delivery when the turn ends.
	ItemFollowUp CapabilityItem = "follow_up"
	// ItemResume: a later process continues the same backend session.
	ItemResume CapabilityItem = "resume"
	// ItemReasoningStream: the model's reasoning reaches the conversation.
	ItemReasoningStream CapabilityItem = "reasoning_stream"
	// ItemUsageQuery: ws asks the harness for its usage figures.
	ItemUsageQuery CapabilityItem = "usage_query"
)

// CatalogItems lists every item, in display order.
var CatalogItems = []CapabilityItem{
	ItemSkillList, ItemSkillInjection, ItemCompaction, ItemRewind,
	ItemToolRemoval, ItemSubagentRemoval, ItemModelEffort, ItemApproval,
	ItemSteer, ItemInterrupt, ItemFollowUp, ItemResume,
	ItemReasoningStream, ItemUsageQuery,
}

// CatalogAnyValue in a value range stands for every value of its kind.
const CatalogAnyValue = "*"

// CatalogRole is the role dimension of an entry, for a harness whose
// delivery differs between a lead and a worker conversation.
type CatalogRole string

const (
	RoleLead   CatalogRole = "lead"
	RoleWorker CatalogRole = "worker"
)

// CapabilityEntry declares one item's grade for a role and harness version
// range. Since is inclusive and Until exclusive; an empty bound is open. An
// empty Role applies to every role.
type CapabilityEntry struct {
	Item  CapabilityItem
	Role  CatalogRole
	Since string
	Until string
	Grade CapabilityGrade
	// Values is the value range: the set or grade the item delivers (the
	// removable tools, for example). It is not the version range.
	Values []string
	// Source cites the wiring that delivers the grade.
	Source string
}

// HarnessProfile is one harness's catalog: its guaranteed version range and
// its entries. Oldest and Newest are the oldest and newest harness versions
// with a recorded stream fixture; a version outside them runs on the
// nearest bound's grades, marked unverified.
type HarnessProfile struct {
	Oldest  string
	Newest  string
	Entries []CapabilityEntry
}

// Capability is one item as resolved for a session.
type Capability struct {
	Grade  CapabilityGrade `json:"grade"`
	Values []string        `json:"values,omitempty"`
}

// Capabilities is a profile resolved for one role and harness version.
type Capabilities struct {
	Role CatalogRole `json:"role"`
	// Version is the harness version the session reported, empty when it
	// is unknown; ProfileVersion is the version the grades were resolved at
	// (Version inside the guaranteed range, else the nearest bound).
	Version        string `json:"version,omitempty"`
	ProfileVersion string `json:"profile_version"`
	// Oldest and Newest are the profile's guaranteed range.
	Oldest string `json:"oldest"`
	Newest string `json:"newest"`
	// Unverified marks a version outside the guaranteed range, or none.
	Unverified bool `json:"unverified,omitempty"`
	// Demoted are the items the session's init shape demoted to
	// unsupported from a better grade (demote).
	Demoted []CapabilityItem              `json:"demoted,omitempty"`
	Items   map[CapabilityItem]Capability `json:"items"`
}

// HarnessReport is what a backend reads from a process's start: the
// harness version (empty when the harness reports none) and the items
// whose init shape does not match what their grade relies on.
type HarnessReport struct {
	Version string
	Demoted []CapabilityItem
}

// demote grades items unsupported, listing in Demoted those it took a
// better grade from.
func (c *Capabilities) demote(items []CapabilityItem) {
	for _, item := range items {
		if c.Grade(item) == GradeUnsupported {
			continue
		}
		c.Items[item] = Capability{Grade: GradeUnsupported}
		c.Demoted = append(c.Demoted, item)
	}
}

// clone is a copy that shares no slice or map with c.
func (c Capabilities) clone() Capabilities {
	out := c
	out.Demoted = slices.Clone(c.Demoted)
	out.Items = make(map[CapabilityItem]Capability, len(c.Items))
	for item, got := range c.Items {
		out.Items[item] = Capability{Grade: got.Grade, Values: slices.Clone(got.Values)}
	}
	return out
}

// commandItems are the dashboard commands that deliver a catalog item: a
// command is offered only while its item is not unsupported.
var commandItems = map[string]CapabilityItem{"compact": ItemCompaction}

// allowedCommands is list without the commands whose item caps grade
// unsupported; nil caps (no profile, or no report yet) keeps every one.
func allowedCommands(list []wslive.SlashCommand, caps *Capabilities) []wslive.SlashCommand {
	if caps == nil {
		return list
	}
	return slices.DeleteFunc(slices.Clone(list), func(c wslive.SlashCommand) bool {
		item, ok := commandItems[c.Name]
		return ok && !caps.Allows(item)
	})
}

// Grade is item's grade; an item the profile does not declare is
// unsupported.
func (c Capabilities) Grade(item CapabilityItem) CapabilityGrade {
	if got, ok := c.Items[item]; ok && got.Grade != "" {
		return got.Grade
	}
	return GradeUnsupported
}

// Allows reports whether item is delivered at all, natively or by ws.
func (c Capabilities) Allows(item CapabilityItem) bool {
	return c.Grade(item) != GradeUnsupported
}

// Resolve resolves the profile for role at the harness version a session
// reported. An empty or unreadable version, or one outside the guaranteed
// range, resolves at the nearest bound (the newest for an unreadable one)
// and is marked Unverified: the session still runs, loudly.
func (p HarnessProfile) Resolve(role CatalogRole, version string) Capabilities {
	at, unverified := p.Newest, true
	if v, ok := parseHarnessVersion(version); ok {
		oldest, _ := parseHarnessVersion(p.Oldest)
		newest, _ := parseHarnessVersion(p.Newest)
		switch {
		case compareHarnessVersions(v, oldest) < 0:
			at = p.Oldest
		case compareHarnessVersions(v, newest) > 0:
			at = p.Newest
		default:
			at, unverified = version, false
		}
	}
	caps := Capabilities{Role: role, Version: version, ProfileVersion: at, Oldest: p.Oldest, Newest: p.Newest, Unverified: unverified, Items: map[CapabilityItem]Capability{}}
	for _, item := range CatalogItems {
		caps.Items[item] = Capability{Grade: GradeUnsupported}
	}
	resolved := map[CapabilityItem]bool{}
	for _, e := range p.Entries {
		if resolved[e.Item] || !e.applies(role, at) {
			continue
		}
		resolved[e.Item] = true
		caps.Items[e.Item] = Capability{Grade: e.Grade, Values: slices.Clone(e.Values)}
	}
	return caps
}

// applies reports whether e declares role at version.
func (e CapabilityEntry) applies(role CatalogRole, version string) bool {
	if e.Role != "" && e.Role != role {
		return false
	}
	return versionInRange(version, e.Since, e.Until)
}

// versionInRange reports whether version lies in [since, until); an empty
// bound is open. An unreadable version lies only in the fully open range.
func versionInRange(version, since, until string) bool {
	if since == "" && until == "" {
		return true
	}
	v, ok := parseHarnessVersion(version)
	if !ok {
		return false
	}
	if lo, ok := parseHarnessVersion(since); since != "" && (!ok || compareHarnessVersions(v, lo) < 0) {
		return false
	}
	if hi, ok := parseHarnessVersion(until); until != "" && (!ok || compareHarnessVersions(v, hi) >= 0) {
		return false
	}
	return true
}

// parseHarnessVersion reads a harness version's dotted numeric core:
// "2.1.326", "v1.5.2", or a version line such as "2.1.326 (Claude Code)".
// A pre-release or build suffix ("-beta.1", "+abc") is ignored.
func parseHarnessVersion(v string) ([]int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, " \t"); i >= 0 {
		v = v[:i]
	}
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	if v == "" {
		return nil, false
	}
	parts := strings.Split(v, ".")
	core := make([]int, len(parts))
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return nil, false
		}
		core[i] = n
	}
	return core, true
}

// compareHarnessVersions orders two parsed cores; a missing component
// counts as 0, so "2.1" equals "2.1.0".
func compareHarnessVersions(a, b []int) int {
	for i := range max(len(a), len(b)) {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// claudeSubagentTool is the native subagent tool a Claude worker profile
// removes by leaving it out of --tools, and a hosted lead's or agent.spawn
// child's launch removes with --disallowedTools (claudeRemovedTools); its
// init may name it "Task" (claudeInitToolNames).
const claudeSubagentTool = "Agent"

// ClaudeCatalog is the reference profile: what the ws adapter delivers for
// Claude Code today. The guaranteed range runs from the oldest to the
// newest version with a recorded stream fixture of both a lead and a
// worker (testdata, claude_code_version).
func ClaudeCatalog() *HarnessProfile {
	return &HarnessProfile{
		Oldest: "2.1.322",
		Newest: "2.1.326",
		Entries: []CapabilityEntry{
			{Item: ItemSkillList, Grade: GradeUnsupported,
				Source: "the init's skills field is not parsed (claudeInit)"},
			{Item: ItemSkillInjection, Grade: GradeNative,
				Source: "Claude Code's own skills: a lead's bundled plugin (claudeLeadPluginArgs), a worker profile's plugin directories with Skills set (claudeArgs)"},
			{Item: ItemCompaction, Role: RoleLead, Grade: GradeNative,
				Source: "the dashboard's /compact reaches the harness as its typed line (processControls.runCommand); the mapper reads the compaction (claudeMapper)"},
			{Item: ItemCompaction, Role: RoleWorker, Grade: GradeUnsupported,
				Source: "a worker reports no command list (the initialize ask and leadInit are lead-only), so the dashboard refuses /compact; auto-compaction is the harness's own"},
			{Item: ItemRewind, Grade: GradeUnsupported, Source: "no ws wiring"},
			{Item: ItemToolRemoval, Role: RoleWorker, Grade: GradeNative, Values: []string{CatalogAnyValue},
				Source: "--tools allowlist of the profile's built-in tools (claudeArgs)"},
			{Item: ItemToolRemoval, Role: RoleLead, Grade: GradeNative, Values: []string{claudeSubagentTool},
				Source: "--disallowedTools of the subagent tool, the one tool ws removes from a lead; the user's own setup keeps the rest (claudeLeadArgs, claudeRemovedTools)"},
			{Item: ItemSubagentRemoval, Role: RoleWorker, Grade: GradeNative, Values: []string{claudeSubagentTool},
				Source: "the subagent tool stays out of --tools unless the profile lists it, and an agent.spawn child's launch disallows it as well (claudeArgs, claudeRemovedTools)"},
			{Item: ItemSubagentRemoval, Role: RoleLead, Grade: GradeNative, Values: []string{claudeSubagentTool},
				Source: "every hosted lead launches with --disallowedTools of the subagent tool (claudeLeadArgs, RemovesSubagents)"},
			{Item: ItemModelEffort, Grade: GradeNative,
				Source: "set_model and apply_flag_settings controls (claudeProcess.SetModel, SetEffort); choices from ClaudeBackend.Choices"},
			{Item: ItemApproval, Role: RoleLead, Grade: GradeNative,
				Source: "can_use_tool prompts and set_permission_mode; lead-only handles (claudeProcess.processControls)"},
			{Item: ItemApproval, Role: RoleWorker, Grade: GradeUnsupported,
				Source: "a worker runs in dontAsk (claudeArgs, checkClaudeInit)"},
			{Item: ItemSteer, Role: RoleLead, Grade: GradeNative,
				Source: "a user message the running turn takes up (claudeProcess.Steer; conv.steer for leads)"},
			{Item: ItemSteer, Role: RoleWorker, Grade: GradeUnsupported,
				Source: "no recoverable worker steer path: steerWorker serves Pi only and a Claude process sets no reports handle"},
			{Item: ItemInterrupt, Grade: GradeNative,
				Source: "the interrupt control request (claudeProcess.Interrupt, Runner.Interrupt)"},
			{Item: ItemFollowUp, Grade: GradeSubstitute,
				Source: "the ws inbox holds the message and the runner sends it when the turn ends (Runner.Send)"},
			{Item: ItemResume, Grade: GradeNative,
				Source: "--resume of the recorded session id (claudeArgs, claudeLeadArgs)"},
			{Item: ItemReasoningStream, Grade: GradeUnsupported,
				Source: "lead thinking lines are skipped (claudeRingSkip); no summary producer yet"},
			{Item: ItemUsageQuery, Role: RoleLead, Grade: GradeNative,
				Source: "the get_context_usage control (claudeMapper.askContextUsage, asked from leadInit)"},
			{Item: ItemUsageQuery, Role: RoleWorker, Grade: GradeUnsupported,
				Source: "a worker's usage comes only from its result lines; nothing asks"},
		},
	}
}

// AgyCatalog is what the ws adapter delivers for an Antigravity (agy) lead:
// every entry is lead-only, because agy has no agent-worker profile. The
// guaranteed range runs over the versions the recorded ACP sessions report
// (testdata/agy-*.jsonl, agentInfo.version: the lead session's is 1.5.10,
// the handshake's 1.5.9); the mapper's HarnessReport
// (agyMapper.initialized) demotes what a session's shape lacks.
func AgyCatalog() *HarnessProfile {
	return &HarnessProfile{
		Oldest: "1.5.9",
		Newest: "1.5.10",
		Entries: []CapabilityEntry{
			{Item: ItemSkillList, Role: RoleLead, Grade: GradeUnsupported,
				Source: "available_commands_update is shown as the command list but not read as a skill list"},
			{Item: ItemSkillInjection, Role: RoleLead, Grade: GradeNative,
				Source: "agy's own skills, invoked by a /name prompt and listed in available_commands_update; ws's skills arrive with the common prompt assembly, not agy plugin import"},
			{Item: ItemCompaction, Role: RoleLead, Grade: GradeNative,
				Source: "the dashboard's /compact goes out as a session/prompt of that text, a command agy lists (agyMapper.commands demotes a list without it); no compaction event, only the usage drop"},
			{Item: ItemRewind, Role: RoleLead, Grade: GradeUnsupported, Source: "no ACP method"},
			{Item: ItemToolRemoval, Role: RoleLead, Grade: GradeUnsupported,
				Source: "the lead keeps the user's tool setup; only a custom main agent removes tools"},
			{Item: ItemSubagentRemoval, Role: RoleLead, Grade: GradeUnsupported,
				Source: "the lead keeps agy's subagent tools; the per-harness playbook line covers it"},
			{Item: ItemModelEffort, Role: RoleLead, Grade: GradeNative,
				Source: "session/set_config_option of the model and thought_level options (agyProcess.SetModel, SetEffort); choices from the session's configOptions (AgyBackend.Choices)"},
			{Item: ItemApproval, Role: RoleLead, Grade: GradeNative,
				Source: "session/request_permission answered by option kind, and session/set_mode (agyProcess.AnswerApproval, SetPermissionMode)"},
			{Item: ItemSteer, Role: RoleLead, Grade: GradeNative,
				Source: "a session/prompt sent while a turn runs carries the run on (agyProcess.Steer)"},
			{Item: ItemInterrupt, Role: RoleLead, Grade: GradeNative,
				Source: "session/cancel; the prompt answers cancelled (agyProcess.Interrupt)"},
			{Item: ItemFollowUp, Role: RoleLead, Grade: GradeSubstitute,
				Source: "the ws inbox holds the message and the runner sends it when the turn ends (Runner.Send); a mid-turn prompt would steer instead"},
			{Item: ItemResume, Role: RoleLead, Grade: GradeNative,
				Source: "session/resume of the recorded session id (agyProcess.open)"},
			{Item: ItemReasoningStream, Role: RoleLead, Grade: GradeUnsupported,
				Source: "agent_thought_chunk lines are dropped (agyRingSkip); no summary producer yet"},
			{Item: ItemUsageQuery, Role: RoleLead, Grade: GradeNative,
				Source: "agy pushes usage_update after each model call; nothing is asked (agyMapper.update)"},
		},
	}
}
