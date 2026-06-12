package mcp

import (
	"fmt"
	"os"
	"strings"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsrsrc"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// playbookTerminologyTable is the bundled harness-aware terminology table.
//
// Keys are harness names ("claude", "codex"); "" is the host-neutral fallback for
// any unrecognized or undetected harness.
// Values map PascalCase variable names (matching playbook frontmatter declarations)
// to their harness-specific text.
//
// Model names are NOT in this table — they are always resolved from config at
// render time via resolveModelVars. Only non-model idioms belong here.
var playbookTerminologyTable = map[string]map[string]string{
	"claude": {
		"ExploreAgent":  "the Explore agent",
		"SpawnIdiom":    `Agent({subagent_type: "general-purpose", ...})`,
		"ContinueIdiom": "SendMessage(to: <agentId>)",
	},
	"codex": {
		"ExploreAgent":  "a search agent",
		"SpawnIdiom":    "creating a new Codex task",
		"ContinueIdiom": "resuming the agent using its task id",
	},
	// "" = host-neutral fallback for unknown/undetected harness.
	"": {
		"ExploreAgent":  "an exploration agent",
		"SpawnIdiom":    "spawning a subagent",
		"ContinueIdiom": "resuming the agent using its returned id",
	},
}

// reservedToolVarNames documents the complete set of variable names that the
// tool layer may inject (from the terminology table or model alias resolution).
// This variable is a documentation artifact: the "tool-injected wins on
// collision" invariant is achieved by layer ordering in buildPlaybookVars
// (terminology and model alias layers overwrite the caller context layer),
// not by a guard that references this set. Tests use it to assert that the
// documented reserved set is complete.
var reservedToolVarNames = func() map[string]bool {
	set := map[string]bool{}
	for _, vars := range playbookTerminologyTable {
		for name := range vars {
			set[name] = true
		}
	}
	// Model alias reserved names.
	set["LightModel"] = true
	set["CoreModel"] = true
	set["DeepModel"] = true
	return set
}()

// terminologyForHarness returns the terminology table for the given harness.
// If harness is not recognized ("" or any value other than "claude"/"codex"),
// the host-neutral ("") table is returned.
func terminologyForHarness(harness string) map[string]string {
	if table, ok := playbookTerminologyTable[harness]; ok {
		return table
	}
	return playbookTerminologyTable[""]
}

// resolveModelVars resolves model alias vars from config for the given harness.
// Returns PascalCase var names ("LightModel", "CoreModel", "DeepModel") mapped to
// concrete model strings. Values may be empty if the config has no alias set for
// that tier/harness combination.
//
// configOpts is a call-site-overridable seam; MCP tool handlers pass
// wsconfig.Options{} (empty, env-driven); tests pass Options{CacheHome: tmpDir}.
func resolveModelVars(harness string, configOpts wsconfig.Options) map[string]string {
	type aliasEntry struct {
		varName string
		alias   string
	}
	entries := []aliasEntry{
		{"LightModel", "light"},
		{"CoreModel", "core"},
		{"DeepModel", "deep"},
	}
	result := make(map[string]string, len(entries))
	for _, e := range entries {
		_, model, _, err := wsconfig.ResolveAgentForHarnessConfig(configOpts, e.alias, "", "", harness)
		if err != nil {
			model = ""
		}
		result[e.varName] = model
	}
	return result
}

// buildPlaybookVars assembles the final vars map for wsrsrc.Load.
//
// Merge rules:
//  1. Caller-supplied context vars are accepted as-is.
//  2. Tool-injected terminology vars overwrite caller context for reserved names.
//  3. Tool-injected model alias vars overwrite caller context for reserved names.
//  4. Only keys present in declared are included in the result (so wsrsrc.Load
//     never sees undeclared keys from the tool-injected tables).
//
// Caller context keys that are not in declared return an ErrUndeclaredVar so the
// contract "undeclared caller context → loud error" is preserved.
//
// configOpts is forwarded to resolveModelVars unchanged.
func buildPlaybookVars(declared []string, callerContext map[string]string, harness string, configOpts wsconfig.Options) (map[string]string, error) {
	declaredSet := make(map[string]bool, len(declared))
	for _, v := range declared {
		declaredSet[v] = true
	}

	// Check caller context keys against declared — fail loudly on undeclared.
	for k := range callerContext {
		if !declaredSet[k] {
			return nil, wsrsrc.ErrUndeclaredVar{Name: k}
		}
	}

	merged := make(map[string]string, len(declared))

	// Layer 1: caller context (validated above).
	for k, v := range callerContext {
		merged[k] = v
	}

	// Layer 2: terminology vars overwrite caller context for reserved names.
	for k, v := range terminologyForHarness(harness) {
		if declaredSet[k] {
			merged[k] = v
		}
		// Not declared → silently skip; tool does not force error for unused idioms.
	}

	// Layer 3: model alias vars overwrite caller context for reserved names.
	for k, v := range resolveModelVars(harness, configOpts) {
		if declaredSet[k] {
			merged[k] = v
		}
	}

	return merged, nil
}

// delegationTip returns the harness-aware continuity tip fragment appended to the
// rendered body of delegates:true playbooks. It includes the always-on mercenary tip
// noting the ws/agents.call path is always reachable on request.
func delegationTip(harness string) string {
	term := terminologyForHarness(harness)
	continueIdiom := term["ContinueIdiom"]
	var sb strings.Builder
	sb.WriteString("\n\n---\n")
	sb.WriteString("**Continuity tip:** This playbook delegates to a subagent. ")
	sb.WriteString("When the subagent returns an agent id, use `")
	sb.WriteString(continueIdiom)
	sb.WriteString("` to send follow-up messages to the same agent rather than spawning a new one. ")
	sb.WriteString("The playbook surface keeps no agent registry; ")
	sb.WriteString("record the agent id in your workflow state if you need it across turns.")
	// Unit 3: always-on mercenary tip — present in every delegates:true rendering.
	sb.WriteString("\n\n**Mercenary path (always available):** A ws-managed external subprocess agent")
	sb.WriteString(" (mercenary) is always reachable on request via `ws/agents.call`, even without")
	sb.WriteString(" `ws.lead.prefer_mercenary`. Pass the session_key received with this prompt and")
	sb.WriteString(" a self-contained prompt from `ws/playbook.render`; the returned handle is an")
	sb.WriteString(" agent id you can resume with the same continuation idiom.")
	return sb.String()
}

// firstClassTierToAlias maps a first-class capability tier (small/medium/large/
// xlarge) declared in playbook frontmatter down to the conventional alias layer
// (light/core/deep) that config.agents_tier is keyed by. This is the intended
// translation boundary between the two tier planes — the abstraction layer
// (first-class tier, spoken by frontmatter/skills/defaults) and the concrete-model
// layer (aliases alongside vendor model names, where per-harness model config
// lives) — not a temporary shim: config.agents_tier stays alias-keyed by design,
// since the taxonomy demotes light/core/deep to the concrete-model layer. Alias
// values pass through unchanged so a playbook may declare an alias directly.
// xlarge has no legacy alias and maps to deep (the highest configured tier).
// Empty/unknown returns "" so the register path falls back to its built-in
// default rather than routing an unrecognized tier.
//
// playbook.render surfaces the first-class frontmatter tier as a recommended
// tier, and agents.register maps it here before setting RegisterOptions.Tier.
func firstClassTierToAlias(tier string) string {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case "small", "light":
		return "light"
	case "medium", "core":
		return "core"
	case "large", "deep":
		return "deep"
	case "xlarge":
		return "deep"
	default:
		return ""
	}
}

// withRecommendedTier appends a `recommended-tier: <first-class>` metadata line to
// a playbook tool payload when the playbook declares a tier. Empty tier leaves the
// payload unchanged. This is the render/print return channel the lead reads to
// route a delegation's model selection — native uses it as a host model-selection
// guide, mercenary passes it to agents.register's pass-through tier arg.
func withRecommendedTier(payload, tier string) string {
	if strings.TrimSpace(tier) == "" {
		return payload
	}
	return payload + "\nrecommended-tier: " + strings.TrimSpace(tier)
}

// childRoleForPlaybookRole maps a playbook frontmatter role string to the child key scope.
// Returns (scope, true) for delegate-eligible roles; ("", false) for non-eligible roles
// (lead, empty, or unknown).
//
// Mapping:
//   - "implementer", "reviewer", "delegate" → roleDelegate
//   - "leaf" → roleLeaf
//   - "lead", "", unknown → ("", false): lead playbooks never mint child keys.
func childRoleForPlaybookRole(role string) (toolRole, bool) {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "implementer", "reviewer", "delegate":
		return roleDelegate, true
	case "leaf":
		return roleLeaf, true
	default:
		return "", false
	}
}

// mercenaryGuidanceBlock returns the prefer-mercenary guidance text fragment for
// implementer/reviewer delegation playbooks when preferMercenary is true.
// When preferMercenary is false, the always-on tip in delegationTip already
// mentions the mercenary path as available on request.
func mercenaryGuidanceBlock() string {
	return "\n\n**Delegation mode (prefer_mercenary active):** Default guidance for this session is" +
		" to use the mercenary path (`ws/agents.call`) rather than a host-native subagent." +
		" The native subagent path remains available if you prefer it."
}

// resolveRsrcRoot resolves the rsrc tree root for a playbook tool call.
// rsrcRootOverride carries playbook.render's root_override (Phase 2c, active): a
// non-empty value rebinds the auto-include resolution root; pass "" to fall back
// to wsrsrc.ResolveRoot() (WS_RSRC_ROOT env → exe-derived path).
//
// This is the call-site-overridable seam: the caller, not the internal logic,
// decides the rsrc root.
func resolveRsrcRoot(rsrcRootOverride string) (string, error) {
	if strings.TrimSpace(rsrcRootOverride) != "" {
		return rsrcRootOverride, nil
	}
	return wsrsrc.ResolveRoot()
}

// renderPlaybookBody is the shared core for printPlaybook and renderPlaybook.
// It loads the playbook via two wsrsrc.Load passes:
//  1. nil-vars pass to obtain the declared variables list (no substitution errors).
//  2. filtered-vars pass with terminology + model aliases + caller context merged.
//
// The delegation tip is appended after substitution when meta.Delegates is true.
//
// rsrcRoot is a call-site-overridable seam (root_override is threaded here).
// configOpts is forwarded to resolveModelVars; tests pass Options{CacheHome:tmpDir}.
//
// mintRoot: when non-empty, the caller is a lead and this function will mint a
// fresh child key (role taken from the playbook's Meta.Role) bound to mintRoot.
// The minted key is spliced as a clearly-delimited block into the rendered body
// so the delegate's ws calls are pre-keyed. When mintRoot is empty or the playbook
// role is not delegate-eligible, no key is minted and the body is unchanged.
//
// preferMercenary: when true and the playbook is an implementer/reviewer role,
// appends a guidance block advising the mercenary spawn idiom as primary.
//
// Returns (body, recommendedTier, error): recommendedTier is the first-class tier
// declared in the playbook frontmatter, surfaced so one render call routes both
// delegation paths — native uses it as a host model-selection guide, mercenary
// passes it to agents.register's pass-through tier arg.
func renderPlaybookBody(s *Server, rsrcRoot, name string, callerContext map[string]string, configOpts wsconfig.Options, mintRoot string, preferMercenary bool) (string, string, error) {
	harness := s.currentHarness()

	// Pass 1: nil vars → load metadata without triggering substitution errors.
	metaOnly, err := wsrsrc.Load(rsrcRoot, name, harness, nil)
	if err != nil {
		return "", "", err
	}

	// recommendedTier is the first-class tier declared in frontmatter, surfaced to
	// the caller so it can route both delegation paths from one render call: native
	// uses it as a host model-selection guide, mercenary passes it to agents.register.
	recommendedTier := metaOnly.Meta.Tier

	var body string

	// Fast path: no declared variables and no caller context.
	if len(metaOnly.Meta.Variables) == 0 && len(callerContext) == 0 {
		body = metaOnly.Body
		if metaOnly.Meta.Delegates {
			body += delegationTip(harness)
		}
	} else {
		// Build filtered vars: check caller context, inject terminology + model aliases.
		vars, err := buildPlaybookVars(metaOnly.Meta.Variables, callerContext, harness, configOpts)
		if err != nil {
			return "", "", err
		}

		// Pass 2: load with full vars for actual substitution.
		pb, err := wsrsrc.Load(rsrcRoot, name, harness, vars)
		if err != nil {
			return "", "", err
		}

		body = pb.Body
		if pb.Meta.Delegates {
			body += delegationTip(harness)
		}
	}

	// Unit 2: prefer_mercenary guidance — implementer/reviewer only.
	if preferMercenary {
		switch strings.ToLower(strings.TrimSpace(metaOnly.Meta.Role)) {
		case "implementer", "reviewer":
			body += mercenaryGuidanceBlock()
		}
	}

	// Unit 1: render-minted child key — only when caller is lead (mintRoot != "").
	if strings.TrimSpace(mintRoot) != "" {
		if childScope, ok := childRoleForPlaybookRole(metaOnly.Meta.Role); ok {
			childKey, err := s.sessions.mint(mintRoot, childScope)
			if err != nil {
				return "", "", fmt.Errorf("mint child session key: %w", err)
			}
			// Splice a clearly-delimited credential block into the body so the
			// delegate's ws calls are pre-keyed. Prepended so the delegate sees
			// the key before any procedure text.
			credBlock := "---\n" +
				"**Your ws session_key: `" + childKey + "`**\n" +
				"Use this key in all ws tool calls. Do not call `ws.lead.login` or `ws.lead.*` tools.\n" +
				"---\n\n"
			body = credBlock + body
		}
	}

	return body, recommendedTier, nil
}

// printPlaybook loads a playbook and returns its rendered body text inline.
//
// Zero-logic wrapper over renderPlaybookBody: the indirection is intentional
// forward-compat, where print and render may diverge (e.g., different
// session-scoped output constraints or inline vs. path semantics).
// printPlaybook never mints child keys (mintRoot="") and ignores preferMercenary.
//
// rsrcRoot is a call-site-overridable seam for root_override support.
// configOpts controls config-backed model alias resolution.
func printPlaybook(s *Server, rsrcRoot, name string, callerContext map[string]string, configOpts wsconfig.Options) (string, string, error) {
	return renderPlaybookBody(s, rsrcRoot, name, callerContext, configOpts, "", false)
}

// renderPlaybook loads a playbook, renders it (with optional child-key mint and
// splice), writes it to a worktree-scoped tmp file, and returns the file path.
// The caller hands this path to a host-native subagent or mercenary.
//
// rsrcRoot and worktreeRoot are call-site-overridable seams for root_override support.
// mintRoot: when non-empty, caller is a lead and a child key is minted for the delegate.
// preferMercenary: when true and playbook is implementer/reviewer, adds mercenary-primary guidance.
// configOpts controls config-backed model alias resolution.
func renderPlaybook(s *Server, rsrcRoot, worktreeRoot, name string, callerContext map[string]string, configOpts wsconfig.Options, mintRoot string, preferMercenary bool) (string, string, error) {
	body, recommendedTier, err := renderPlaybookBody(s, rsrcRoot, name, callerContext, configOpts, mintRoot, preferMercenary)
	if err != nil {
		return "", "", err
	}
	generated, err := wsstate.NewManager(wsstate.Options{}).GeneratePaths(worktreeRoot, "prompt", []string{name})
	if err != nil {
		return "", "", fmt.Errorf("allocate playbook path: %w", err)
	}
	if err := os.WriteFile(generated[0].Path, []byte(body), 0o644); err != nil {
		return "", "", fmt.Errorf("write playbook %s: %w", generated[0].Path, err)
	}
	return generated[0].Path, recommendedTier, nil
}
