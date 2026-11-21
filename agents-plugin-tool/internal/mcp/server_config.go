package mcp

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsconfigservice"
	"github.com/kang-sw/devenv/internal/wsdoc"
)

func builtinConfigDefaults() map[string]string {
	defaults := map[string]string{
		wsconfig.HostedPluginPackageKey:        "wsflow",
		wsconfig.ItemWorkflowPreferSubagent:    "off",
		wsconfig.ItemWorkflowWhiteboardEnabled: "off",
		wsconfig.ItemWorkflowTrackerEnabled:    "off",
		wsconfig.ItemWorkflowProfile:           "direct",
		wsconfig.ItemSageReview:                "auto",
		wsconfig.ItemSageReviewDesign:          "off",
		wsconfig.ItemTicketReviewAutoAck:       "on",
		wsconfig.ItemReviewPhase:               "lite",
		wsconfig.ItemBootstrapAlarm:            "on",
		wsconfig.ItemWorktreePool:              defaultWorktreePoolTemplate,
		wsconfig.ItemTicketAssigneeAware:       "off",
		wsconfig.ItemWorktreeNudgeStaleDays:    worktreeNudgeStaleDaysKey.Default,
		wsconfig.ItemWorktreeNudgePreservedGB:  worktreeNudgePreservedGBKey.Default,
		wsconfig.ItemGitMergeSizeSoftLines:     gitMergeSizeSoftLinesKey.Default,
		wsconfig.ItemGitMergeSizeHardLines:     gitMergeSizeHardLinesKey.Default,
		wsconfig.ItemGitMergeSizeExtensions:    gitMergeSizeExtensionsKey.Default,
		wsconfig.ItemAgentChildWaitingDays:     strconv.Itoa(wsagent.DefaultChildWaitingDays),
	}
	for key, value := range wsconfig.CompactionDefaults {
		defaults[key] = value
	}
	return defaults
}

func builtinConfigAndPromptDefaults() map[string]string {
	defaults := builtinConfigDefaults()
	for k, v := range builtinPromptOverrideDefaults() {
		defaults[k] = v
	}
	return defaults
}

// sessionConfigOptions anchors the committed repo scope at sessionKey's
// canonical worktree root. A keyless call or an unknown key leaves RepoRoot
// empty, so the repo scope drops out rather than erroring.
func (s *Server) sessionConfigOptions(sessionKey string) wsconfig.Options {
	opts := s.configOptions()
	if key := strings.TrimSpace(sessionKey); key != "" {
		if entry, found := s.lookupSession(key); found {
			opts.RepoRoot = entry.root
		}
	}
	return opts
}

// sessionResolver is the single constructor for a layered-config read made on
// behalf of sessionKey: session overrides through the session store, and the
// repo scope anchored by sessionConfigOptions. A reader whose keys are all
// global-only or not resolver-backed may skip it, since the repo layer never
// applies to those.
func (s *Server) sessionResolver(sessionKey string, builtins map[string]string) wsconfig.Resolver {
	adapter := sessionConfigAdapter{s: s.sessions}
	return wsconfig.NewResolver(s.sessionConfigOptions(sessionKey), builtins, adapter, adapter)
}

// resolveBucketedOverrideRows re-resolves the bare-key row of each
// harness-bucketed registry knob in the scoped overrides view for this
// session's harness. ScopedShow reads the bare key only, which for a bucketed
// knob is the legacy value or builtin; the effective value lives under the
// bucket keys. The per-bucket rows are left as ScopedShow reported them: they
// show where stored values live.
func (s *Server) resolveBucketedOverrideRows(resolver *wsconfig.Resolver, sessionKey string, view *wsconfig.View) error {
	for i := range view.ResolvedOverrides {
		item := &view.ResolvedOverrides[i]
		entry := registryEntryByKey(item.Key)
		if !entry.HarnessBucketed() {
			continue
		}
		rv, _, err := resolveHarnessBucketed(resolver, sessionKey, item.Key, s.playbookHarness())
		if err != nil {
			return err
		}
		item.Value, item.Scope = rv.Value, rv.Scope
	}
	return nil
}

func formatConfigView(view wsconfig.View) string {
	var b strings.Builder
	fmt.Fprintf(&b, "path: %s\n", view.Path)
	aliases := []string{"small", "medium", "large", "xlarge"}
	b.WriteString("model_aliases:\n")
	for _, alias := range aliases {
		byHarness := view.Config.Agents.ModelAliases[alias]
		keys := sortedAgentTierKeys(byHarness)
		if len(keys) == 0 {
			continue
		}
		fmt.Fprintf(&b, "  %s:\n", alias)
		for _, key := range keys {
			tier := byHarness[key]
			fmt.Fprintf(&b, "    %s: %s/%s", key, displayOrDash(tier.Backend), displayOrDash(tier.Model))
			if tier.Effort != "" {
				fmt.Fprintf(&b, " effort=%s", formatEffortForText(tier.Effort))
			}
			b.WriteString("\n")
		}
	}
	if view.RepoScope != nil {
		b.WriteString("repo_scope:\n")
		if view.RepoScope.Path == "" {
			b.WriteString("  path: none (no session root anchors it; pass session_key)\n")
		} else {
			fmt.Fprintf(&b, "  path: %s\n", view.RepoScope.Path)
			fmt.Fprintf(&b, "  exists: %t\n", view.RepoScope.Exists)
		}
		fmt.Fprintf(&b, "  shape: %s\n", view.RepoScope.Shape)
	}
	// Scope-resolved overrides are present when config.show is invoked with a
	// session key (ScopedShow path). Print each resolved item with its source
	// scope and whether the committed repo scope can set it.
	if len(view.ResolvedOverrides) > 0 {
		b.WriteString("overrides:\n")
		for _, item := range view.ResolvedOverrides {
			fmt.Fprintf(&b, "  %s: %s  [scope:%s] [repo_scope:%s]\n", item.Key, item.Value, item.Scope, yesNo(item.RepoScope))
		}
	}
	return b.String()
}

// configListView is the combined config.list JSON payload (260814 Phase 2):
// the embedded wsconfig.View flattens config.show's path/config/
// resolved_overrides at the top level exactly as config.show emitted them, and
// Knobs carries config.tuning's per-key catalog under a new "knobs" key. Both
// contracts are preserved side by side rather than one subsuming the other.
type configListView struct {
	wsconfig.View
	Knobs []tuningKnob `json:"knobs"`
	// ManifestErrors names each adapter key manifest rejected at startup.
	ManifestErrors []string `json:"manifest_errors,omitempty"`
}

// resolveAgentTierResult is the config.resolve_agent JSON payload (260905
// Phase 3): the resolved backend/model/effort for a fixed tier under a
// harness, plus which alias bucket answered (Open Decisions #2) so a
// caller (the Pi adapter) can tell a real pi-bucket hit from a
// cross-harness fallback without re-deriving the resolution chain itself.
type resolveAgentTierResult struct {
	Backend      string `json:"backend"`
	Model        string `json:"model"`
	Effort       string `json:"effort"`
	ResolvedFrom string `json:"resolved_from"`
}

// promptOverrideValue is one resolved override for a declared point: the harness
// bucket it applies to, the scope it resolved from, and the stored value.
type promptOverrideValue struct {
	Harness string `json:"harness"`
	Scope   string `json:"scope"`
	Value   string `json:"value"`
}

// promptOverridePoint is a declared override-point plus any current override
// values, shaped for the config.prompt JSON listing.
type promptOverridePoint struct {
	PointId   string                `json:"pointId"`
	Desc      string                `json:"desc"`
	Overrides []promptOverrideValue `json:"overrides"`
}

// buildPromptOverrideListing resolves the current override value+scope for each
// declared point across every harness bucket. Unset buckets (empty resolved
// value) are omitted. Session-scope values are only visible when sessionKey is
// non-empty (mirroring config.show).
func buildPromptOverrideListing(points []overridePointDecl, resolver *wsconfig.Resolver, sessionKey string) []promptOverridePoint {
	listing := make([]promptOverridePoint, 0, len(points))
	for _, p := range points {
		entry := promptOverridePoint{PointId: p.PointId, Desc: p.Desc, Overrides: []promptOverrideValue{}}
		for _, harness := range harnessBuckets {
			rv, _ := resolver.Get(sessionKey, harnessBucketKey("prompt."+p.PointId, harness))
			if strings.TrimSpace(rv.Value) == "" {
				continue
			}
			entry.Overrides = append(entry.Overrides, promptOverrideValue{
				Harness: harness,
				Scope:   string(rv.Scope),
				Value:   rv.Value,
			})
		}
		listing = append(listing, entry)
	}
	return listing
}

// formatPromptOverrideListing renders the canonical text view of the override
// listing: one block per point and a single trailing pointer to ws:lead-tune.
func formatPromptOverrideListing(listing []promptOverridePoint) string {
	var b strings.Builder
	for _, p := range listing {
		fmt.Fprintf(&b, "%s\n", p.PointId)
		if p.Desc != "" {
			fmt.Fprintf(&b, "  %s\n", p.Desc)
		}
		if len(p.Overrides) == 0 {
			b.WriteString("  (no overrides set)\n")
		} else {
			for _, o := range p.Overrides {
				fmt.Fprintf(&b, "  harness=%s scope=%s\n", o.Harness, o.Scope)
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("Tuning manual & how-to: run the ws:lead-tune skill.\n")
	return b.String()
}

type tuningCatalog struct {
	Knobs []tuningKnob `json:"knobs"`
}

type tuningKnob struct {
	ID             string        `json:"id"`
	Kind           string        `json:"kind"`
	Description    string        `json:"description,omitempty"`
	Writer         tuningWriter  `json:"writer"`
	Reset          *tuningWriter `json:"reset,omitempty"`
	SelectorFields []tuningField `json:"selector_fields,omitempty"`
	ValueFields    []tuningField `json:"value_fields,omitempty"`
	Current        any           `json:"current"`
	// Default is the declared default of an adapter-declared knob; built-in
	// knobs state theirs in Description.
	Default *string `json:"default,omitempty"`
	// RepoScope reports whether the committed repo-scope file can set this
	// knob (configKeyEntry.RepoScoped).
	RepoScope bool `json:"repo_scope"`
	// RepoKey is the key a repo-scope file entry stores this knob under, set
	// only when RepoScope is true. It differs from ID for prompt.* knobs,
	// whose stored keys carry a harness bucket suffix.
	RepoKey string `json:"repo_key,omitempty"`
	// RepoMember names the config-file member RepoKey lives in when it is not
	// overrides: structured_overrides for a structured (object) knob.
	RepoMember string `json:"repo_member,omitempty"`
	// ValueSchema is the JSON Schema of a structured knob's value.
	ValueSchema any `json:"value_schema,omitempty"`
	// Error reports why Current could not be resolved (an invalid stored
	// value, naming its file and key); config.list still lists the knob so
	// the lead can fix it.
	Error string `json:"error,omitempty"`
}

type tuningWriter struct {
	Tool           string            `json:"tool"`
	FixedArguments map[string]string `json:"fixed_arguments,omitempty"`
}

type tuningField struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Required    bool     `json:"required,omitempty"`
}

type tuningScopedValue struct {
	Value string `json:"value"`
	Scope string `json:"scope"`
}

type tuningAgentTierCurrent struct {
	Tier    string `json:"tier"`
	Harness string `json:"harness"`
	Backend string `json:"backend,omitempty"`
	Model   string `json:"model,omitempty"`
	Effort  string `json:"effort,omitempty"`
}

// harness selects the bucket a harness-bucketed knob's current value is
// resolved for (the session's detected harness; empty reads only all).
func buildTuningCatalog(rsrcRoot fs.FS, resolver *wsconfig.Resolver, sessionKey, harness string, noAgentMode bool, configOpts wsconfig.Options, registries ...map[string]wsagent.BackendRegistration) (tuningCatalog, error) {
	points, err := scanOverridePoints(rsrcRoot)
	if err != nil {
		return tuningCatalog{}, err
	}
	promptListing := buildPromptOverrideListing(points, resolver, sessionKey)

	catalog := tuningCatalog{Knobs: make([]tuningKnob, 0, len(promptListing)+5)}
	// appendKnob drives the noAgentMode cut off each entry's NoAgentVisible
	// flag rather than a positional early-return, so a knob's agentless
	// visibility is declared per-entry instead of depending on append order.
	appendKnob := func(entry configKeyEntry, knob tuningKnob) {
		if noAgentMode && !entry.NoAgentVisible {
			return
		}
		knob.RepoScope = entry.RepoScoped()
		if knob.RepoScope {
			knob.RepoKey = entry.RepoKey()
		}
		catalog.Knobs = append(catalog.Knobs, knob)
	}

	for _, p := range promptListing {
		entry := promptKnobEntry(p.PointId)
		appendKnob(entry, tuningKnob{
			ID:          entry.Key,
			Kind:        "prompt_override",
			Description: p.Desc,
			Writer: tuningWriter{
				Tool:           entry.WriterTool,
				FixedArguments: map[string]string{"key": entry.Key},
			},
			Reset: &tuningWriter{
				Tool:           entry.ResetTool,
				FixedArguments: map[string]string{"key": entry.Key, "reset": "true"},
			},
			SelectorFields: entry.SelectorFields,
			ValueFields:    entry.ValueFields,
			Current:        p.Overrides,
		})
	}

	subagentEntry := registryEntryByKey(wsconfig.ItemWorkflowPreferSubagent)
	appendKnob(subagentEntry, tuningKnob{
		ID:          "workflow.prefer_subagent",
		Kind:        "workflow_preference",
		Description: "Delegation posture, global-only (all of the user's projects). on adds a standing line, from the next workflow-manual load, that sends bounded investigation, diagnosis, drafting, and low-impact operational work to lead-delegate subagents, subject to its routing gate; off (builtin) leaves that work in the lead's own context. Cost: on spends one fresh subagent context per delegated task and keeps the lead's context lean over a long session; off spawns no extra agents but fills the lead's context with that work.",
		Writer:      tuningWriter{Tool: subagentEntry.WriterTool, FixedArguments: map[string]string{"key": subagentEntry.Key}},
		Reset: &tuningWriter{
			Tool:           subagentEntry.ResetTool,
			FixedArguments: map[string]string{"key": subagentEntry.Key, "reset": "true"},
		},
		ValueFields: subagentEntry.ValueFields,
		Current:     currentWorkflowPreference(resolver, sessionKey, wsconfig.ItemWorkflowPreferSubagent),
	})

	whiteboardEntry := registryEntryByKey(wsconfig.ItemWorkflowWhiteboardEnabled)
	appendKnob(whiteboardEntry, tuningKnob{
		ID:          whiteboardEntry.Key,
		Kind:        "workflow_preference",
		Description: "Session whiteboard as the lead's primary channel (builtin off), per harness. on adds a workflow-manual section, from the next workflow-manual load, that puts every item asked of the user and every explanation on a local HTML page the server renders, leaving the terminal a one-line pointer; off keeps asks and explanations in terminal text. The whiteboard tools work either way, and a session's first successful whiteboard.update turns this on at session scope when it resolves off (an explicit session value stays). A write lands in the session's harness bucket unless harness names another (* for every harness), and in global scope (all of the user's projects) unless scope names another. A read takes the session's harness bucket from any scope first, then the * bucket from any scope. Cost: on needs a browser tab beside the terminal and one whiteboard update per turn; off keeps everything in the terminal.",
		Writer:      tuningWriter{Tool: whiteboardEntry.WriterTool, FixedArguments: map[string]string{"key": whiteboardEntry.Key}},
		Reset: &tuningWriter{
			Tool:           whiteboardEntry.ResetTool,
			FixedArguments: map[string]string{"key": whiteboardEntry.Key, "reset": "true"},
		},
		SelectorFields: whiteboardEntry.SelectorFields,
		ValueFields:    whiteboardEntry.ValueFields,
		Current:        currentHarnessBucketedValue(resolver, sessionKey, whiteboardEntry.Key, harness),
	})

	trackerEntry := registryEntryByKey(wsconfig.ItemWorkflowTrackerEnabled)
	appendKnob(trackerEntry, tuningKnob{
		ID:          trackerEntry.Key,
		Kind:        "workflow_preference",
		Description: "Tracker section of the workflow manual (builtin off), per harness. on adds the tracker section, from the next workflow-manual load, which tells the lead to record work in the user's registered tracker stores; off leaves it out. The tracker tools work either way in a top-level lead session, and a session's first successful tracker tool call turns this on at session scope when it resolves off (an explicit session value stays). The background poll and the lead-session-start check run for every registered store whatever this says. A write lands in the session's harness bucket unless harness names another (* for every harness), and in global scope (all of the user's projects) unless scope names another. A read takes the session's harness bucket from any scope first, then the * bucket from any scope.",
		Writer:      tuningWriter{Tool: trackerEntry.WriterTool, FixedArguments: map[string]string{"key": trackerEntry.Key}},
		Reset: &tuningWriter{
			Tool:           trackerEntry.ResetTool,
			FixedArguments: map[string]string{"key": trackerEntry.Key, "reset": "true"},
		},
		SelectorFields: trackerEntry.SelectorFields,
		ValueFields:    trackerEntry.ValueFields,
		Current:        currentHarnessBucketedValue(resolver, sessionKey, trackerEntry.Key, harness),
	})

	hostedPackageEntry := registryEntryByKey(wsconfig.HostedPluginPackageKey)
	appendKnob(hostedPackageEntry, tuningKnob{
		ID:          hostedPackageEntry.Key,
		Kind:        "hosted",
		Description: "Plugin package for daemon-launched Claude and Pi leads and the ws MCP attached to spawned conversations. Global-only; builtin wsflow, independent of the daemon starter. Changes apply at the next process start, not to running or holder-reattached processes. Non-hosted sessions keep their own package. If the selected package is unavailable, the daemon uses its own package with a visible warning.",
		Writer:      tuningWriter{Tool: hostedPackageEntry.WriterTool, FixedArguments: map[string]string{"key": hostedPackageEntry.Key}},
		Reset: &tuningWriter{
			Tool:           hostedPackageEntry.ResetTool,
			FixedArguments: map[string]string{"key": hostedPackageEntry.Key, "reset": "true"},
		},
		ValueFields: hostedPackageEntry.ValueFields,
		Current:     currentWorkflowPreference(resolver, sessionKey, hostedPackageEntry.Key),
	})

	workflowProfileEntry := registryEntryByKey(wsconfig.ItemWorkflowProfile)
	appendKnob(workflowProfileEntry, tuningKnob{
		ID:          workflowProfileEntry.Key,
		Kind:        "workflow_preference",
		Description: "Lead playbook profile (builtin direct), per harness. direct reads the base lead playbooks; orchestrate selects the orchestrate variant of each lead playbook that has one, from the next workflow-manual or playbook load, and the lead then works through sub-leads, which run tickets through workers that delegate and explore. It applies to top-level lead sessions only; a sub-lead or worker always reads the base playbooks. orchestrate needs a host that supports nested subagent spawn (lead -> sub-lead -> worker -> delegate -> explore); config.tune warns when a harness it is set for is not verified to. A write lands in the session's harness bucket unless harness names another (* for every harness), and in global scope (all of the user's projects) unless scope names another. A read takes the session's harness bucket from any scope first, then the * bucket from any scope. Cost: orchestrate spends more agents per ticket and keeps the lead's own context lean; direct spawns fewer agents and keeps more work in the lead's context.",
		Writer:      tuningWriter{Tool: workflowProfileEntry.WriterTool, FixedArguments: map[string]string{"key": workflowProfileEntry.Key}},
		Reset: &tuningWriter{
			Tool:           workflowProfileEntry.ResetTool,
			FixedArguments: map[string]string{"key": workflowProfileEntry.Key, "reset": "true"},
		},
		SelectorFields: workflowProfileEntry.SelectorFields,
		ValueFields:    workflowProfileEntry.ValueFields,
		Current:        currentHarnessBucketedValue(resolver, sessionKey, workflowProfileEntry.Key, harness),
	})

	bootstrapEntry := registryEntryByKey(wsconfig.ItemBootstrapAlarm)
	appendKnob(bootstrapEntry, tuningKnob{
		ID:          "bootstrap_alarm",
		Kind:        "workflow_preference",
		Description: "Select whether the session-bootstrap staleness warning fires when this project's AGENTS.md template is behind the shipped lead-bootstrap template.",
		Writer:      tuningWriter{Tool: bootstrapEntry.WriterTool, FixedArguments: map[string]string{"key": bootstrapEntry.Key}},
		Reset: &tuningWriter{
			Tool:           bootstrapEntry.ResetTool,
			FixedArguments: map[string]string{"key": bootstrapEntry.Key, "reset": "true"},
		},
		ValueFields: bootstrapEntry.ValueFields,
		Current:     currentWorkflowPreference(resolver, sessionKey, wsconfig.ItemBootstrapAlarm),
	})

	gitRootsEntry := registryEntryByKey(wsconfig.ItemAgentGitSearchRoots)
	appendKnob(gitRootsEntry, tuningKnob{
		ID:          gitRootsEntry.Key,
		Kind:        "workflow_preference",
		Description: "Extra directories the read-only git.search tool of a hosted agent conversation may read, beyond the project roots of the sessions the dashboard saw on this machine in the last 7 days: absolute paths joined with the platform path-list separator (: on POSIX, ; on Windows). A repository at or inside one of them is readable; a relative or missing entry is ignored, and git.search names it when it lists its roots. Unset (builtin) adds none. Global-only.",
		Writer:      tuningWriter{Tool: gitRootsEntry.WriterTool, FixedArguments: map[string]string{"key": gitRootsEntry.Key}},
		Reset: &tuningWriter{
			Tool:           gitRootsEntry.ResetTool,
			FixedArguments: map[string]string{"key": gitRootsEntry.Key, "reset": "true"},
		},
		ValueFields: gitRootsEntry.ValueFields,
		Current:     currentWorkflowPreference(resolver, sessionKey, gitRootsEntry.Key),
	})

	tierAliasesEntry := registryEntryByKey(wsconfig.ItemAgentTierAliases)
	appendKnob(tierAliasesEntry, tuningKnob{
		ID:          tierAliasesEntry.Key,
		Kind:        "workflow_preference",
		Description: "Your own names for agent.spawn tiers: whitespace-separated name=<harness>.<tier> pairs, for example \"mockup=pi.large quick=claude.small\", with tier small, medium, large, or xlarge. agent.spawn takes an alias as its tier, and agent.tiers lists each alias with the model it resolves to now. A name has no dot; a write that does not parse is refused. Unset (builtin) defines none. Global-only.",
		Writer:      tuningWriter{Tool: tierAliasesEntry.WriterTool, FixedArguments: map[string]string{"key": tierAliasesEntry.Key}},
		Reset: &tuningWriter{
			Tool:           tierAliasesEntry.ResetTool,
			FixedArguments: map[string]string{"key": tierAliasesEntry.Key, "reset": "true"},
		},
		ValueFields: tierAliasesEntry.ValueFields,
		Current:     currentWorkflowPreference(resolver, sessionKey, tierAliasesEntry.Key),
	})

	for _, key := range []string{"compaction.lead_advisory", "compaction.lead_hard", "compaction.worker_advisory", "compaction.worker_hard"} {
		entry := registryEntryByKey(key)
		rv, _, _ := resolveCompactionValue(resolver, sessionKey, key, harness, registries...)
		def := wsconfig.CompactionDefaults[key]
		appendKnob(entry, tuningKnob{ID: key, Kind: "workflow_preference", Description: "Hosted native compaction threshold, checked at idle boundaries. Advisory asks for durable agenda state; hard queues native compaction. Read at process start and reloaded on config.tune before the next threshold check.", Writer: tuningWriter{Tool: "config.tune", FixedArguments: map[string]string{"key": key}}, Reset: &tuningWriter{Tool: "config.tune", FixedArguments: map[string]string{"key": key, "reset": "true"}}, SelectorFields: entry.SelectorFields, ValueFields: entry.ValueFields, Current: tuningScopedValue{Value: rv.Value, Scope: string(rv.Scope)}, Default: &def})
	}

	childWaitingEntry := registryEntryByKey(wsconfig.ItemAgentChildWaitingDays)
	appendKnob(childWaitingEntry, tuningKnob{
		ID:          childWaitingEntry.Key,
		Kind:        "workflow_preference",
		Description: fmt.Sprintf("Days a spawned child agent may stay asleep with no turn after its last activity before the dashboard daemon deletes it and its own children. A whole number, at least 1; builtin %d. The daemon reads it once per start. Global-only.", wsagent.DefaultChildWaitingDays),
		Writer:      tuningWriter{Tool: childWaitingEntry.WriterTool, FixedArguments: map[string]string{"key": childWaitingEntry.Key}},
		Reset: &tuningWriter{
			Tool:           childWaitingEntry.ResetTool,
			FixedArguments: map[string]string{"key": childWaitingEntry.Key, "reset": "true"},
		},
		ValueFields: childWaitingEntry.ValueFields,
		Current:     currentWorkflowPreference(resolver, sessionKey, childWaitingEntry.Key),
	})

	sageReviewEntry := registryEntryByKey(wsconfig.ItemSageReview)
	appendKnob(sageReviewEntry, tuningKnob{
		ID:          sageReviewEntry.Key,
		Kind:        "sage_review",
		Description: "Completeness review at actionable ready/ promotion (builtin auto). auto requires one completeness-reviewer pass before the ticket reaches ready/; ask asks the user whether to run it; off skips it. Cost: one medium-tier reviewer (builtin tier) per promotion. Lowering it loses the independent check that the ticket's facts, phases, and verification are complete enough for a worker to run without stopping; those gaps then surface mid-implementation. It does not govern the design stage; see sage_review_design.",
		Writer:      tuningWriter{Tool: sageReviewEntry.WriterTool, FixedArguments: map[string]string{"key": sageReviewEntry.Key}},
		Reset: &tuningWriter{
			Tool:           sageReviewEntry.ResetTool,
			FixedArguments: map[string]string{"key": sageReviewEntry.Key, "reset": "true"},
		},
		SelectorFields: sageReviewEntry.SelectorFields,
		ValueFields:    sageReviewEntry.ValueFields,
		Current:        currentWorkflowPreference(resolver, sessionKey, sageReviewEntry.Key),
	})

	sageReviewDesignEntry := registryEntryByKey(wsconfig.ItemSageReviewDesign)
	appendKnob(sageReviewDesignEntry, tuningKnob{
		ID:          sageReviewDesignEntry.Key,
		Kind:        "sage_review",
		Description: "Design review at actionable ready/ promotion (builtin off: design review is opt-in). auto requires one design-reviewer pass before the ticket reaches ready/; ask asks the user whether to run it; off skips it. Cost: one large-tier reviewer (builtin tier) per promotion. Lowering it loses an independent challenge of the ticket's decisions before implementation; a flawed design then surfaces in code review or after merge, where changing it costs more. Epic design review runs whenever it is invoked, regardless of this knob.",
		Writer:      tuningWriter{Tool: sageReviewDesignEntry.WriterTool, FixedArguments: map[string]string{"key": sageReviewDesignEntry.Key}},
		Reset: &tuningWriter{
			Tool:           sageReviewDesignEntry.ResetTool,
			FixedArguments: map[string]string{"key": sageReviewDesignEntry.Key, "reset": "true"},
		},
		SelectorFields: sageReviewDesignEntry.SelectorFields,
		ValueFields:    sageReviewDesignEntry.ValueFields,
		Current:        currentWorkflowPreference(resolver, sessionKey, sageReviewDesignEntry.Key),
	})

	autoAckEntry := registryEntryByKey(wsconfig.ItemTicketReviewAutoAck)
	appendKnob(autoAckEntry, tuningKnob{
		ID:          autoAckEntry.Key,
		Kind:        "workflow_preference",
		Description: "Open Decision Queue confirmation of review-raised items during ticket settlement (builtin on). on lets the ticket-handler confirm, on its own judgment, an announced default or technical decision raised by a fact-populator decision gap or by a reviewer issue marked autonomous, and list it as auto-confirmed in the round's settlement report; policy questions, reversals, and reviewer issues marked missing are still asked. off asks the caller to confirm every such item, adding one caller round trip before ready/ promotion. Takes effect at the next ticket-handler playbook read or render.",
		Writer:      tuningWriter{Tool: autoAckEntry.WriterTool, FixedArguments: map[string]string{"key": autoAckEntry.Key}},
		Reset: &tuningWriter{
			Tool:           autoAckEntry.ResetTool,
			FixedArguments: map[string]string{"key": autoAckEntry.Key, "reset": "true"},
		},
		SelectorFields: autoAckEntry.SelectorFields,
		ValueFields:    autoAckEntry.ValueFields,
		Current:        currentWorkflowPreference(resolver, sessionKey, autoAckEntry.Key),
	})

	reviewPhaseEntry := registryEntryByKey(wsconfig.ItemReviewPhase)
	appendKnob(reviewPhaseEntry, tuningKnob{
		ID:          reviewPhaseEntry.Key,
		Kind:        "workflow_preference",
		Description: "The ticket worker's per-phase independent code review. lite (builtin) runs one medium-tier reviewer covering correctness and test integrity, one pass per phase; full runs the risk-keyed allocation (one reviewer, or one per risky correctness, fit, and test partition) with a second round that verifies the fixes; off runs none, leaving the pre-merge range review as the integration net. Lowering it moves defect discovery to that range review, where a fix costs another worker cycle; full adds fit review on risky changes and the fix-verification round that lite lacks. A lead's session value reaches the workers it spawns. An explicit policy.review.override of single or partitioned wins over every value.",
		Writer:      tuningWriter{Tool: reviewPhaseEntry.WriterTool, FixedArguments: map[string]string{"key": reviewPhaseEntry.Key}},
		Reset: &tuningWriter{
			Tool:           reviewPhaseEntry.ResetTool,
			FixedArguments: map[string]string{"key": reviewPhaseEntry.Key, "reset": "true"},
		},
		SelectorFields: reviewPhaseEntry.SelectorFields,
		ValueFields:    reviewPhaseEntry.ValueFields,
		Current:        currentWorkflowPreference(resolver, sessionKey, reviewPhaseEntry.Key),
	})

	langEntry := registryEntryByKey(wsconfig.ItemWorkflowLang)
	appendKnob(langEntry, tuningKnob{
		ID:          langEntry.Key,
		Kind:        "workflow_preference",
		Description: "The user's conversation language, as a language name (for example Korean). It sets the language of lead responses and of user-facing summaries an adapter produces. Unset (builtin) binds no language: the lead answers in the language the user writes in. Defaults to global scope, since language is a cross-project preference.",
		Writer:      tuningWriter{Tool: langEntry.WriterTool, FixedArguments: map[string]string{"key": langEntry.Key}},
		Reset: &tuningWriter{
			Tool:           langEntry.ResetTool,
			FixedArguments: map[string]string{"key": langEntry.Key, "reset": "true"},
		},
		SelectorFields: langEntry.SelectorFields,
		ValueFields:    langEntry.ValueFields,
		Current:        currentWorkflowPreference(resolver, sessionKey, langEntry.Key),
	})

	preferences, err := resolver.ListScalars(sessionKey, wsconfig.PreferencePrefix)
	if err != nil {
		return tuningCatalog{}, err
	}
	preferenceEntry := preferenceKnobEntry("preference.<slot>")
	appendKnob(preferenceEntry, tuningKnob{
		ID:             preferenceEntry.Key,
		Kind:           "user_preference",
		Description:    "Named standing preferences. Replace <slot> with a name matching ^[a-z0-9][a-z0-9_-]*$. Slots accumulate across session > project > repo > global; the first value per slot wins. An empty string hides the slot, reset removes the chosen scope's entry. The manual prints default first, then other names ascending, without labels. default falls back to the legacy UserPreferenceSection override, then the seed. workflow.lang is independent of every slot.",
		Writer:         tuningWriter{Tool: preferenceEntry.WriterTool, FixedArguments: map[string]string{"key": preferenceEntry.Key}},
		Reset:          &tuningWriter{Tool: preferenceEntry.ResetTool, FixedArguments: map[string]string{"key": preferenceEntry.Key, "reset": "true"}},
		SelectorFields: preferenceEntry.SelectorFields,
		ValueFields:    preferenceEntry.ValueFields,
		Current:        preferences,
	})

	agentTiers, err := currentAgentTierMappings(configOpts)
	if err != nil {
		return tuningCatalog{}, err
	}
	agentsTierEntry := registryEntryByKey("agents.tier")
	appendKnob(agentsTierEntry, tuningKnob{
		ID:             "agents.tier",
		Kind:           "model_tier",
		Description:    "Map a capability tier (small, medium, large, xlarge) to the backend/model that every delegate rendered at that tier launches with, per harness, in project or global scope. The builtin design reviewer runs at large and the lite phase reviewer and completeness reviewer at medium; the ticket worker's tier follows its route's risk. A cheaper model at a tier lowers the cost of all work at that tier and loses capability on it. Reset takes only value.tier and removes the selected scope's harness leaf.",
		Writer:         tuningWriter{Tool: agentsTierEntry.WriterTool, FixedArguments: map[string]string{"key": "agents.tier"}},
		Reset:          &tuningWriter{Tool: agentsTierEntry.ResetTool, FixedArguments: map[string]string{"key": "agents.tier", "reset": "true"}},
		SelectorFields: agentsTierEntry.SelectorFields,
		ValueFields:    agentsTierEntry.ValueFields,
		Current:        agentTiers,
	})

	profileEntry := worktreeProfileKnobEntry("<name>")
	profileKnob := tuningKnob{
		ID:          profileEntry.Key,
		Kind:        "worktree_profile",
		Description: "One worktree profile per key: the pool partition and lifecycle worktree.acquire(profile: <name>) provisions. A pooled worktree is reused only by an acquire naming the same profile. The value is a whole profile object (value_schema); a higher scope replaces a lower scope's profile of the same name, with no field merge. Builtins: full (plain checkout) and housekeeping (cone sparse of ai-docs). Hooks run as argv arrays from any scope, so a repo-scope profile carries commands the repository's scripts provide; each runs under a 10-minute timeout. post_provision runs when a worktree enters the profile, post_acquire on every acquire, pre_release before release hygiene, pre_remove before worktree.remove. A failing post_provision or post_acquire quarantines the worktree (never reused; worktree.remove clears it); a failing pre_release or pre_remove fails that call and leaves the worktree as it was. current lists every resolved profile with the scope and file it came from.",
		Writer:      tuningWriter{Tool: profileEntry.WriterTool, FixedArguments: map[string]string{"key": profileEntry.Key}},
		Reset: &tuningWriter{
			Tool:           profileEntry.ResetTool,
			FixedArguments: map[string]string{"key": profileEntry.Key, "reset": "true"},
		},
		SelectorFields: profileEntry.SelectorFields,
		ValueFields:    profileEntry.ValueFields,
		ValueSchema:    worktreeProfileValueSchema(),
		RepoMember:     "structured_overrides",
	}
	if profiles, err := resolveWorktreeProfiles(resolver); err != nil {
		profileKnob.Error = err.Error()
	} else {
		profileKnob.Current = profiles
	}
	appendKnob(profileEntry, profileKnob)

	appendKnob(agentProfileTuningKnob(resolver, registries...))

	for _, k := range []declaredConfigKey{worktreeNudgeStaleDaysKey, worktreeNudgePreservedGBKey} {
		entry := k.entry()
		appendKnob(entry, tuningKnob{
			ID:          entry.Key,
			Kind:        "worktree_nudge",
			Description: k.Description + " Value: " + k.domain() + "; builtin " + k.Default + ".",
			Writer:      tuningWriter{Tool: entry.WriterTool, FixedArguments: map[string]string{"key": entry.Key}},
			Reset: &tuningWriter{
				Tool:           entry.ResetTool,
				FixedArguments: map[string]string{"key": entry.Key, "reset": "true"},
			},
			SelectorFields: entry.SelectorFields,
			ValueFields:    entry.ValueFields,
			Current:        currentWorkflowPreference(resolver, sessionKey, entry.Key),
		})
	}
	for _, k := range gitMergeSizeKeys {
		entry := k.entry()
		appendKnob(entry, tuningKnob{
			ID:          entry.Key,
			Kind:        "git_merge_size",
			Description: k.Description + " Value: " + k.domain() + "; builtin " + strconv.Quote(k.Default) + ".",
			Writer:      tuningWriter{Tool: entry.WriterTool, FixedArguments: map[string]string{"key": entry.Key}},
			Reset: &tuningWriter{
				Tool:           entry.ResetTool,
				FixedArguments: map[string]string{"key": entry.Key, "reset": "true"},
			},
			SelectorFields: entry.SelectorFields,
			ValueFields:    entry.ValueFields,
			Current:        currentWorkflowPreference(resolver, sessionKey, entry.Key),
		})
	}

	return catalog, nil
}

// agentsTierTuneResult preserves the existing full-config response and adds
// diagnostics only for this compound writer.
type agentsTierTuneResult struct {
	wsconfig.Config
	Warnings []string `json:"warnings"`
}

func tuneAgentsTier(opts wsconfig.Options, value any, harness string, scope wsconfig.Scope, reset bool) (agentsTierTuneResult, error) {
	cfg, removed, err := wsconfigservice.WriteTier(opts, value, harness, scope, reset)
	if err != nil {
		return agentsTierTuneResult{}, err
	}
	fields := value.(map[string]any)
	tier, _ := fields["tier"].(string)
	global := scope == wsconfig.ScopeGlobal
	result := agentsTierTuneResult{Config: cfg, Warnings: []string{}}
	if reset && !removed {
		result.Warnings = append(result.Warnings, "agents.tier leaf was absent in the selected scope; reset made no change")
	}
	if global {
		// Shadow detection is advisory: an unreadable project config cannot
		// prevent a successful write or reset of the selected global file.
		shadowed, shadowErr := wsconfig.HasProjectAgentsTierLeaf(opts, tier, harness)
		if shadowErr != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("could not check project scope for agents.tier shadowing: %v", shadowErr))
		} else if shadowed {
			result.Warnings = append(result.Warnings, "project scope stores this agents.tier tier/harness leaf and shadows the global value")
		}
	}
	return result, nil
}

// resolveReviewPhase resolves the review_phase knob under sessionKey; the
// session parent walk lets a lead's session value reach the worker keys it
// spawns, and the session's root anchors the committed repo scope so a team
// can commit the value. A read error is returned, never replaced by the
// builtin: a malformed committed file must not silently pick a review tier.
func (s *Server) resolveReviewPhase(sessionKey string) (string, error) {
	r := s.sessionResolver(sessionKey, builtinConfigDefaults())
	resolved, err := r.Get(sessionKey, wsconfig.ItemReviewPhase)
	if err != nil {
		return "", err
	}
	return resolved.Value, nil
}

// resolveSageReviewConfig resolves both Sage stage knobs under one session
// key: the design stage from sage_review_design and the completeness stage
// from sage_review. Each resolves independently — an old sage_review override
// never implies design review. A read error is returned: an empty value maps
// to a skipped posture, so swallowing it would silently bypass review.
func (s *Server) resolveSageReviewConfig(sessionKey string) (wsdoc.SageReviewConfig, error) {
	r := s.sessionResolver(sessionKey, builtinConfigDefaults())
	design, err := r.Get(sessionKey, wsconfig.ItemSageReviewDesign)
	if err != nil {
		return wsdoc.SageReviewConfig{}, err
	}
	completeness, err := r.Get(sessionKey, wsconfig.ItemSageReview)
	if err != nil {
		return wsdoc.SageReviewConfig{}, err
	}
	return wsdoc.SageReviewConfig{Design: design.Value, Completeness: completeness.Value}, nil
}

func currentWorkflowPreference(resolver *wsconfig.Resolver, sessionKey, itemKey string) tuningScopedValue {
	rv, _ := resolver.Get(sessionKey, itemKey)
	return tuningScopedValue{
		Value: rv.Value,
		Scope: string(rv.Scope),
	}
}

// currentHarnessBucketedValue is the catalog's current value of a
// harness-bucketed knob: the value that applies to harness.
func currentHarnessBucketedValue(resolver *wsconfig.Resolver, sessionKey, itemKey, harness string) tuningScopedValue {
	rv, _, _ := resolveHarnessBucketed(resolver, sessionKey, itemKey, harness)
	return tuningScopedValue{
		Value: rv.Value,
		Scope: string(rv.Scope),
	}
}

func currentAgentTierMappings(opts wsconfig.Options) ([]tuningAgentTierCurrent, error) {
	cfg, err := wsconfig.LoadAgentTierConfig(opts)
	if err != nil {
		return nil, err
	}
	rows := make([]tuningAgentTierCurrent, 0)
	for _, tier := range sortedAgentModelAliasKeys(cfg.Agents.ModelAliases) {
		byHarness := cfg.Agents.ModelAliases[tier]
		for _, harness := range sortedAgentTierKeys(byHarness) {
			value := byHarness[harness]
			rows = append(rows, tuningAgentTierCurrent{
				Tier:    tier,
				Harness: harness,
				Backend: value.Backend,
				Model:   value.Model,
				Effort:  value.Effort,
			})
		}
	}
	return rows, nil
}

func formatTuningCatalog(catalog tuningCatalog) string {
	var b strings.Builder
	for _, knob := range catalog.Knobs {
		fmt.Fprintf(&b, "%s (%s)\n", knob.ID, knob.Kind)
		if knob.Description != "" {
			fmt.Fprintf(&b, "  %s\n", knob.Description)
		}
		fmt.Fprintf(&b, "  writer: %s\n", knob.Writer.Tool)
		if len(knob.SelectorFields) > 0 {
			fmt.Fprintf(&b, "  selectors: %s\n", strings.Join(tuningFieldLabels(knob.SelectorFields), ", "))
		}
		if len(knob.ValueFields) > 0 {
			fmt.Fprintf(&b, "  values: %s\n", strings.Join(tuningFieldLabels(knob.ValueFields), ", "))
		}
		if current := formatTuningCurrent(knob.Current); current != "" {
			fmt.Fprintf(&b, "  current: %s\n", current)
		}
		if knob.Default != nil {
			fmt.Fprintf(&b, "  default: %s\n", *knob.Default)
		}
		fmt.Fprintf(&b, "  repo_scope: %s\n", yesNo(knob.RepoScope))
		if knob.RepoKey != "" {
			fmt.Fprintf(&b, "  repo_key: %s\n", knob.RepoKey)
		}
		if knob.RepoMember != "" {
			fmt.Fprintf(&b, "  repo_member: %s\n", knob.RepoMember)
		}
		if knob.ValueSchema != nil {
			fmt.Fprintf(&b, "  value_schema: %s\n", formatTuningCurrent(knob.ValueSchema))
		}
		if knob.Error != "" {
			fmt.Fprintf(&b, "  error: %s\n", knob.Error)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// formatManifestErrors renders the rejected adapter key manifests after the
// tuning catalog, so a lead reading config.list sees why declared keys are
// missing.
func formatManifestErrors(rejected []string) string {
	var b strings.Builder
	for _, msg := range rejected {
		fmt.Fprintf(&b, "config manifest rejected: %s\n", msg)
	}
	return b.String()
}

// effectiveConfigValue resolves the value of a resolver-backed knob that
// applies to sessionKey, through the session-anchored resolver with the
// declared and builtin defaults, and returns the stored key that answered. A
// harness-bucketed knob resolves for this server's session harness.
func (s *Server) effectiveConfigValue(entry configKeyEntry, sessionKey string) (wsconfig.ResolvedValue, string, error) {
	resolver := s.sessionResolver(sessionKey, s.declared.withDefaults(builtinConfigDefaults()))
	if isCompactionKey(entry.Key) {
		return s.compactionValue(&resolver, sessionKey, entry.Key, s.playbookHarness())
	}
	if entry.HarnessBucketed() {
		return resolveHarnessBucketed(&resolver, sessionKey, entry.Key, s.playbookHarness())
	}
	rv, err := resolver.Get(sessionKey, entry.Key)
	return rv, entry.Key, err
}

// resetEchoScope names the scope a config.tune reset removed the override from.
func resetEchoScope(explicitScope wsconfig.Scope, entry configKeyEntry) wsconfig.Scope {
	if explicitScope != "" {
		return explicitScope
	}
	return entry.DefaultScope()
}

func tuningFieldLabels(fields []tuningField) []string {
	labels := make([]string, 0, len(fields))
	for _, field := range fields {
		label := field.Name
		if len(field.Enum) > 0 {
			values := make([]string, 0, len(field.Enum))
			for _, value := range field.Enum {
				if value == "" {
					values = append(values, "<empty>")
				} else {
					values = append(values, value)
				}
			}
			label += "[" + strings.Join(values, "|") + "]"
		}
		labels = append(labels, label)
	}
	return labels
}

func formatTuningCurrent(current any) string {
	raw, err := json.Marshal(current)
	if err != nil || string(raw) == "null" {
		return ""
	}
	return string(raw)
}

func sortedAgentModelAliasKeys(values map[string]map[string]wsconfig.AgentTier) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedAgentTierKeys(values map[string]wsconfig.AgentTier) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// tuneStringValueRejection returns a type-accurate error when config.tune's
// value is present and non-null but not a string, for a key whose writer reads
// a string. Absent and explicit null values return nil and fall through to the
// key's existing empty-value handling, matching typedArrayRejection.
func tuneStringValueRejection(key string, value any, present bool) error {
	if !present || value == nil {
		return nil
	}
	if _, ok := value.(string); ok {
		return nil
	}
	return fmt.Errorf("config.tune: %s value must be a string; got %s", key, jsonValueTypeName(value))
}
