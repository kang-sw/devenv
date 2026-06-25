package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
// render time via resolveRoleModelVar. Only non-model idioms belong here.
var playbookTerminologyTable = map[string]map[string]string{
	"claude": {
		"ExploreAgent":  "the Explore agent",
		"SpawnIdiom":    `Agent({subagent_type: "general-purpose", ...})`,
		"ContinueIdiom": "SendMessage(to: <agentId>)",
	},
	"codex": {
		"ExploreAgent":  "an explorer subagent",
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
// tool layer may inject (from the terminology table, namespace table, or model
// alias resolution).
// This variable is a documentation artifact: the "tool-injected wins on
// collision" invariant is achieved by layer ordering in buildPlaybookVars
// (terminology, namespace, and model alias layers overwrite the caller context
// layer), not by a guard that references this set. Tests use it to assert that the
// documented reserved set is complete.
var reservedToolVarNames = func() map[string]bool {
	set := map[string]bool{}
	for _, vars := range playbookTerminologyTable {
		for name := range vars {
			set[name] = true
		}
	}
	// Tier-derived model var reserved name.
	set["RoleModel"] = true
	// workflow.lang language-binding injection.
	set["WorkflowLang"] = true
	for _, name := range wsrsrc.ImplicitVariableNames {
		set[name] = true
	}
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

// resolveRoleModelVar resolves the single RoleModel var from the playbook's
// declared capability tier. The tier string ("small"/"medium"/"large"/"xlarge")
// maps directly to a concrete per-harness model via config, so the playbook author
// only needs to declare a tier — not pick an alias name.
//
// Returns map[string]string{"RoleModel": model}. On error or empty model result,
// RoleModel is set to "" (same graceful-empty behavior the old per-alias resolver used).
//
// configOpts is a call-site-overridable seam; MCP tool handlers pass
// wsconfig.Options{} (empty, env-driven); tests pass Options{CacheHome: tmpDir}.
func resolveRoleModelVar(harness, tier string, configOpts wsconfig.Options) map[string]string {
	_, model, _, err := wsconfig.ResolveAgentForHarnessConfig(configOpts, tier, "", "", harness)
	if err != nil {
		model = ""
	}
	return map[string]string{"RoleModel": model}
}

// resolveWorkflowLangVar generates the WorkflowLang instruction text from the
// resolved workflow.lang config value. Returns "" when lang is empty.
func resolveWorkflowLangVar(lang string) string {
	if strings.TrimSpace(lang) == "" {
		return ""
	}
	return "Respond to the user in " + strings.TrimSpace(lang) +
		". Keep all internal reasoning, subagent prompts, code comments, and AI-authored artifacts in English."
}

func resolveNamespaceVars() map[string]string {
	namespace := RuntimeNamespace()
	return map[string]string{
		"McpNamespace":   namespace,
		"SkillNamespace": namespace,
	}
}

func isReservedNamespaceVar(name string) bool {
	for _, reserved := range wsrsrc.ImplicitVariableNames {
		if name == reserved {
			return true
		}
	}
	return false
}

// buildPlaybookVars assembles the final vars map for wsrsrc.Load.
//
// Merge rules:
//  1. Caller-supplied context vars are accepted as-is.
//  2. Tool-injected terminology vars overwrite caller context for reserved names.
//  3. Tool-injected RoleModel var (tier-derived) overwrites caller context for reserved names.
//  4. Tool-injected namespace vars overwrite caller context for reserved names.
//  5. Only keys present in declared, plus namespace reserved vars, are included
//     in the result.
//
// Caller context keys that are neither declared nor namespace reserved return an
// ErrUndeclaredVar so the contract "undeclared caller context → loud error" is
// preserved while common namespace vars do not require frontmatter declarations.
//
// tier is the playbook's declared capability tier (from pb.Meta.Tier); it drives
// RoleModel resolution. configOpts is forwarded to resolveRoleModelVar unchanged.
// workflowLang is the resolved workflow.lang value; it drives WorkflowLang injection.
func buildPlaybookVars(declared []string, callerContext map[string]string, harness, tier string, configOpts wsconfig.Options, workflowLang string) (map[string]string, error) {
	declaredSet := make(map[string]bool, len(declared))
	for _, v := range declared {
		declaredSet[v] = true
	}

	// Check caller context keys against declared — fail loudly on undeclared.
	for k := range callerContext {
		if !declaredSet[k] && !isReservedNamespaceVar(k) {
			return nil, wsrsrc.ErrUndeclaredVar{Name: k}
		}
	}

	merged := make(map[string]string, len(declared)+len(wsrsrc.ImplicitVariableNames))

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

	// Layer 3: tier-derived RoleModel var overwrites caller context for reserved names.
	for k, v := range resolveRoleModelVar(harness, tier, configOpts) {
		if declaredSet[k] {
			merged[k] = v
		}
	}

	// Layer 4: namespace vars are available to all playbooks and override caller
	// context so display namespace cannot be spoofed through render context.
	for k, v := range resolveNamespaceVars() {
		merged[k] = v
	}

	// Layer 5: workflow.lang language-binding instruction — injected only when
	// the playbook declares WorkflowLang and a language is configured.
	if declaredSet["WorkflowLang"] {
		merged["WorkflowLang"] = resolveWorkflowLangVar(workflowLang)
	}

	return merged, nil
}

// delegationTip returns the harness-aware continuity tip fragment appended to the
// rendered body of delegates:true playbooks. Full ws output includes the
// always-on mercenary tip; wsflow no-agent output omits it because the mercenary
// surface is hidden there.
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
	if !NoAgentMode() {
		// Unit 3: always-on mercenary tip — present in every full-ws delegates:true rendering.
		sb.WriteString("\n\n**Mercenary path (always available):** A ws-managed external subprocess agent")
		sb.WriteString(" (mercenary) is always reachable on request via `ws.mercenary.call`, even without")
		sb.WriteString(" `ws.lead.prefer_mercenary`. Pass the session_key received with this prompt and")
		sb.WriteString(" a self-contained prompt from `ws/playbook.render`; the returned handle is an")
		sb.WriteString(" agent id you can resume with the same continuation idiom.")
	}
	return sb.String()
}

// withRecommendedTier appends a `recommended-tier: <first-class>` metadata line to
// a playbook tool payload when the playbook declares a tier. Empty tier leaves the
// payload unchanged. This is the render/print return channel the lead reads to
// route a delegation's model selection — native uses it as a host model-selection
// guide, mercenary passes it to ws.mercenary.register's pass-through tier arg.
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
		" to use the mercenary path (`ws.mercenary.call`) rather than a host-native subagent." +
		" The native subagent path remains available if you prefer it."
}

// overrideLookupFn is an injectable function for resolving override values for a
// named override-point and harness. It returns the override text and true when a
// value is stored for the (pointId, harness) pair, or ("", false) to signal that
// no override is stored and the inline seed default should be used.
//
// A nil overrideLookupFn means "no overrides available" — every override-point
// renders its inline seed default. This allows printPlaybook and unit tests to
// pass nil without constructing a resolver.
type overrideLookupFn func(pointId, harness string) (value string, found bool)

const (
	// overrideOpenPrefix and overrideClosePrefix are the trimmed-line prefixes used
	// to detect override-point open and close markers. The open marker has the form:
	//   <!-- ws:override:<pointId> desc="..." -->
	// The close marker has the form:
	//   <!-- ws:/override:<pointId> -->
	overrideOpenPrefix  = "<!-- ws:override:"
	overrideClosePrefix = "<!-- ws:/override:"
)

// parseOverrideMarkerPointId extracts the pointId token from a trimmed marker
// line whose prefix has already been verified, returning (pointId, true) when the
// line is a well-formed marker (ends with `-->`). Parsing is symmetric for open
// and close markers and tolerant of spacing: both `<!-- ws:override:Foo -->` and
// `<!-- ws:override:Foo-->` (no space before `-->`) yield pointId "Foo". An
// empty pointId or a missing `-->` terminator returns ("", false).
func parseOverrideMarkerPointId(trimmed, prefix string) (string, bool) {
	if !strings.HasPrefix(trimmed, prefix) || !strings.HasSuffix(trimmed, "-->") {
		return "", false
	}
	// Strip the prefix and the trailing `-->`, then take the first whitespace-
	// delimited token as the pointId. This handles both `Foo -->`, `Foo-->`, and
	// `Foo desc="..." -->` forms identically.
	rest := strings.TrimSuffix(trimmed[len(prefix):], "-->")
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", false
	}
	pointId := rest
	if spaceIdx := strings.IndexAny(rest, " \t"); spaceIdx >= 0 {
		pointId = rest[:spaceIdx]
	}
	pointId = strings.TrimSpace(pointId)
	if pointId == "" {
		return "", false
	}
	return pointId, true
}

// parseOverrideOpenMarkerDesc extracts the pointId and optional desc from a
// trimmed open-marker line of the form `<!-- ws:override:<pointId> desc="..." -->`.
// It mirrors parseOverrideMarkerPointId for the pointId token but additionally
// surfaces the `desc="..."` attribute (the render engine discards desc, so this is
// a separate parser dedicated to the config.prompt listing).
//
// Returns (pointId, desc, true) for a well-formed open marker; desc is "" when the
// attribute is absent. A non-marker line or empty pointId returns ("", "", false).
func parseOverrideOpenMarkerDesc(trimmed string) (pointId, desc string, ok bool) {
	if !strings.HasPrefix(trimmed, overrideOpenPrefix) || !strings.HasSuffix(trimmed, "-->") {
		return "", "", false
	}
	rest := strings.TrimSuffix(trimmed[len(overrideOpenPrefix):], "-->")
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", "", false
	}
	pointId = rest
	if spaceIdx := strings.IndexAny(rest, " \t"); spaceIdx >= 0 {
		pointId = rest[:spaceIdx]
	}
	pointId = strings.TrimSpace(pointId)
	if pointId == "" {
		return "", "", false
	}
	// Extract desc="..." from the remainder, if present. Find the opening
	// `desc="` then the next `"` to delimit the value.
	if start := strings.Index(rest, `desc="`); start >= 0 {
		valueStart := start + len(`desc="`)
		if end := strings.IndexByte(rest[valueStart:], '"'); end >= 0 {
			desc = rest[valueStart : valueStart+end]
		}
	}
	return pointId, desc, true
}

// overridePointDecl is a single declared override-point discovered in the rsrc
// playbook tree: the pointId and its short desc (empty when undeclared).
type overridePointDecl struct {
	PointId string
	Desc    string
}

const preferSubagentInvocationGuidancePointID = "PreferSubagentInvocationGuidance"

const preferSubagentCodexInvocationGuidancePrompt = "" +
	"- Codex binding: call `spawn_agent(fork_context:true, message:<prompt>)`; " +
	"omit `agent_type`, `model`, and `reasoning_effort` for full-history forks unless the host permits them.\n" +
	"- If a typed fork is rejected, retry untyped with `fork_context:true`; " +
	"do not satisfy this posture with `agent_type: explorer` or `agent_type: worker` unless `fork_context:true` is active."

func builtinPromptOverrideDefaults() map[string]string {
	return map[string]string{
		"prompt." + preferSubagentInvocationGuidancePointID + ".codex": preferSubagentCodexInvocationGuidancePrompt,
	}
}

// scanOverridePoints walks the rsrc tree rooted at rsrcRoot, scans every `.md`
// file for override open markers, and returns the declared override-points
// deduped by pointId (the first non-empty desc wins) sorted by PointId. It is a
// pure function (root in, data out) so it unit-tests without a Server.
func scanOverridePoints(rsrcRoot string) ([]overridePointDecl, error) {
	descByPoint := map[string]string{}
	err := filepath.Walk(rsrcRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(data), "\n") {
			pointId, desc, ok := parseOverrideOpenMarkerDesc(strings.TrimSpace(line))
			if !ok {
				continue
			}
			if existing, seen := descByPoint[pointId]; !seen || (existing == "" && desc != "") {
				descByPoint[pointId] = desc
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	points := make([]overridePointDecl, 0, len(descByPoint))
	for id, desc := range descByPoint {
		points = append(points, overridePointDecl{PointId: id, Desc: desc})
	}
	sort.Slice(points, func(i, j int) bool { return points[i].PointId < points[j].PointId })
	return points, nil
}

// buildOverrideLookup returns an overrideLookupFn backed by the session-keyed
// layered-config resolver. When sessionKey is empty, only code-owned builtin
// prompt defaults participate; user/project/global prompt overrides still require
// a session-keyed render. It is the single construction site shared by the
// playbook.print and playbook.render dispatch paths, reusing the same
// sessionConfigAdapter + resolver shape as the prefer_mercenary read path.
//
// Override values are stored under dynamic keys `prompt.<pointId>.<harness>`; the
// resolver returns empty (not an error) for unset keys, so an absent override
// yields ("", false) and the override pass falls back to the inline seed.
func buildOverrideLookup(s *Server, sessionKey string) overrideLookupFn {
	capturedKey := strings.TrimSpace(sessionKey)
	if capturedKey == "" {
		builtins := builtinPromptOverrideDefaults()
		return func(pointId, harness string) (string, bool) {
			v := builtins["prompt."+pointId+"."+harness]
			return v, v != ""
		}
	}
	adapter := sessionConfigAdapter{s: s.sessions}
	resolver := wsconfig.NewResolver(wsconfig.Options{}, builtinPromptOverrideDefaults(), adapter, adapter)
	return func(pointId, harness string) (string, bool) {
		rv, _ := resolver.Get(capturedKey, "prompt."+pointId+"."+harness)
		return rv.Value, rv.Value != ""
	}
}

// applyOverrideMarkers processes override-point block markers in body, resolving
// each (pointId, harness) pair through the lookup closure and substituting the
// inline seed default when no override is stored. Marker lines are always stripped
// from the rendered output — the result contains only resolved content and never
// any override marker syntax.
//
// Resolution order per point: lookup(pointId, harness) → lookup(pointId, "all") →
// inline seed body. An empty seed body (open marker immediately followed by close
// marker) is an extension slot: it renders the stored override or nothing.
//
// Robustness guarantees:
//   - Open/close parsing is symmetric and tolerant of spacing before `-->`.
//   - Nested override blocks inside a seed body are processed recursively, so no
//     inner marker line ever survives in the output.
//   - An unclosed open marker (EOF reached with no matching close) is NOT treated
//     as an override block: its line is emitted unchanged so no playbook content
//     is consumed or truncated.
//
// A nil lookup is treated as "no overrides": every point falls back to its seed.
func applyOverrideMarkers(body, harness string, lookup overrideLookupFn) string {
	lines := strings.Split(body, "\n")
	result := make([]string, 0, len(lines))

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Detect open marker: <!-- ws:override:<pointId> ... -->
		pointId, ok := parseOverrideMarkerPointId(trimmed, overrideOpenPrefix)
		if !ok {
			result = append(result, line)
			i++
			continue
		}

		// Scan forward for the matching close marker, tracking nesting depth so an
		// inner same-or-other override block does not prematurely close the outer
		// one. The close that matches this open is the first close (for any pointId)
		// encountered at depth 0.
		seedLines := make([]string, 0)
		depth := 1
		closeIdx := -1
		for j := i + 1; j < len(lines); j++ {
			jt := strings.TrimSpace(lines[j])
			if _, openOK := parseOverrideMarkerPointId(jt, overrideOpenPrefix); openOK {
				depth++
				seedLines = append(seedLines, lines[j])
				continue
			}
			if _, closeOK := parseOverrideMarkerPointId(jt, overrideClosePrefix); closeOK {
				depth--
				if depth == 0 {
					closeIdx = j
					break
				}
				seedLines = append(seedLines, lines[j])
				continue
			}
			seedLines = append(seedLines, lines[j])
		}

		// Unclosed open marker: do NOT treat as a block. Emit the open line
		// unchanged and continue scanning from the next line so no content is lost.
		if closeIdx < 0 {
			result = append(result, line)
			i++
			continue
		}

		// Recursively process the seed so any nested override block inside it is
		// resolved and its marker lines are stripped (prevents orphaned markers).
		seed := applyOverrideMarkers(strings.Join(seedLines, "\n"), harness, lookup)

		// Resolve: harness-specific → all → seed.
		var resolved string
		if lookup != nil {
			if v, ok := lookup(pointId, harness); ok {
				resolved = v
			} else if v, ok := lookup(pointId, "all"); ok {
				resolved = v
			} else {
				resolved = seed
			}
		} else {
			resolved = seed
		}

		// Append the resolved text (may be empty for empty-seed extension slots
		// with no stored override).
		if resolved != "" {
			result = append(result, resolved)
		}
		// Advance past the close marker.
		i = closeIdx + 1
	}

	return strings.Join(result, "\n")
}

func renderProductModePlaybookBody(body string, mercenaryEnabled bool) string {
	return selectProductModeBlocks(body, mercenaryEnabled)
}

const (
	fullOnlyStart      = "<!-- ws:full-only:start -->"
	fullOnlyEnd        = "<!-- ws:full-only:end -->"
	wsflowOnlyStart    = "<!-- ws:wsflow-only:start -->"
	wsflowOnlyEnd      = "<!-- ws:wsflow-only:end -->"
	mercenaryOnlyStart = "<!-- ws:mercenary-on:start -->"
	mercenaryOnlyEnd   = "<!-- ws:mercenary-on:end -->"
)

// selectProductModeBlocks removes marker comments and keeps only the sections
// that apply to the current product mode and mercenary preference. The source
// rsrc remains shared; the rendered playbook is the product-specific contract.
// mercenaryEnabled=true preserves ws:mercenary-on blocks; false strips them.
// printPlaybook passes mercenaryEnabled=true to expose the full source view.
func selectProductModeBlocks(body string, mercenaryEnabled bool) string {
	lines := strings.Split(body, "\n")
	filtered := make([]string, 0, len(lines))
	fullOnly := false
	wsflowOnly := false
	mercenaryOnly := false
	noAgent := NoAgentMode()
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch trimmed {
		case fullOnlyStart:
			fullOnly = true
			continue
		case fullOnlyEnd:
			fullOnly = false
			continue
		case wsflowOnlyStart:
			wsflowOnly = true
			continue
		case wsflowOnlyEnd:
			wsflowOnly = false
			continue
		case mercenaryOnlyStart:
			mercenaryOnly = true
			continue
		case mercenaryOnlyEnd:
			mercenaryOnly = false
			continue
		}
		if (fullOnly && noAgent) || (wsflowOnly && !noAgent) || (mercenaryOnly && !mercenaryEnabled) {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
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
// It loads the playbook once without substitution so include and overlay
// semantics stay in wsrsrc, then applies MCP-layer substitution. The MCP layer
// owns implicit namespace variables because their values come from runtime
// product mode rather than playbook frontmatter.
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
// overrideLookup: when non-nil, a session-keyed resolver closure used to resolve
// override-point marker values before product-mode selection. Pass nil (e.g. from
// printPlaybook or unit tests that do not seed overrides) to render every
// override-point with its inline seed default.
//
// Returns (body, recommendedTier, error): recommendedTier is the first-class tier
// declared in the playbook frontmatter, surfaced so one render call routes both
// delegation paths — native uses it as a host model-selection guide, mercenary
// passes it to ws.mercenary.register's pass-through tier arg.
func renderPlaybookBody(s *Server, rsrcRoot, name string, callerContext map[string]string, configOpts wsconfig.Options, mintRoot string, parentKey string, preferMercenary bool, workflowLang string, overrideLookup overrideLookupFn) (string, string, error) {
	harness := s.currentHarness()

	// Load once with nil vars so the MCP playbook layer can add reserved
	// namespace variables without teaching product-mode semantics to wsrsrc.
	pb, err := wsrsrc.Load(rsrcRoot, name, harness, nil)
	if err != nil {
		return "", "", err
	}

	// recommendedTier is the first-class tier declared in frontmatter, surfaced to
	// the caller so it can route both delegation paths from one render call: native
	// uses it as a host model-selection guide, mercenary passes it to ws.mercenary.register.
	recommendedTier := pb.Meta.Tier

	vars, err := buildPlaybookVars(pb.Meta.Variables, callerContext, harness, recommendedTier, configOpts, workflowLang)
	if err != nil {
		return "", "", err
	}
	body, err := substitutePlaybookVars(pb.Body, pb.Meta.Variables, vars)
	if err != nil {
		return "", "", err
	}
	if pb.Meta.Delegates {
		body += delegationTip(harness)
	}

	// Unit 2: prefer_mercenary guidance — implementer/reviewer only.
	if preferMercenary {
		switch strings.ToLower(strings.TrimSpace(pb.Meta.Role)) {
		case "implementer", "reviewer":
			body += mercenaryGuidanceBlock()
		}
	}

	// Unit 1: render-minted child key — only when caller is lead (mintRoot != "").
	if strings.TrimSpace(mintRoot) != "" {
		if childScope, ok := childRoleForPlaybookRole(pb.Meta.Role); ok {
			childKey, err := s.sessions.mint(mintRoot, childScope, parentKey)
			if err != nil {
				return "", "", fmt.Errorf("mint child session key: %w", err)
			}
			// Splice a clearly-delimited credential block into the body so the
			// delegate's ws calls are pre-keyed. Prepended so the delegate sees
			// the key before any procedure text.
			credBlock := "---\n" +
				"**Your ws session_key: `" + childKey + "`**\n" +
				"Use this key in all ws tool calls. You are already authenticated; do not attempt to mint, log in, or escalate session credentials.\n" +
				"---\n\n"
			body = credBlock + body
		}
	}

	// Override-marker pass: runs before product-mode selection so that override
	// substitution operates on the shared body (including full-only sections)
	// before product-mode blocks are stripped. A nil lookup skips this pass and
	// every override-point renders its inline seed default.
	body = applyOverrideMarkers(body, harness, overrideLookup)

	return renderProductModePlaybookBody(body, preferMercenary), recommendedTier, nil
}

func substitutePlaybookVars(body string, declared []string, vars map[string]string) (string, error) {
	declaredSet := make(map[string]bool, len(declared)+len(wsrsrc.ImplicitVariableNames))
	names := make([]string, 0, len(declared)+len(wsrsrc.ImplicitVariableNames))
	for _, name := range declared {
		if declaredSet[name] {
			continue
		}
		declaredSet[name] = true
		names = append(names, name)
	}
	for _, name := range wsrsrc.ImplicitVariableNames {
		if declaredSet[name] {
			continue
		}
		declaredSet[name] = true
		names = append(names, name)
	}

	for k := range vars {
		if !declaredSet[k] {
			return "", wsrsrc.ErrUndeclaredVar{Name: k}
		}
	}

	var pairs []string
	for _, name := range names {
		placeholder := "{{." + name + "}}"
		if !strings.Contains(body, placeholder) {
			continue
		}
		value, provided := vars[name]
		if !provided {
			return "", wsrsrc.ErrUnprovidedVar{Name: name}
		}
		pairs = append(pairs, placeholder, value)
	}

	result := body
	if len(pairs) > 0 {
		result = strings.NewReplacer(pairs...).Replace(body)
	}

	remaining := result
	for {
		idx := strings.Index(remaining, "{{.")
		if idx < 0 {
			break
		}
		end := strings.Index(remaining[idx:], "}}")
		if end < 0 {
			break
		}
		end += idx
		varName := remaining[idx+3 : end]
		if !declaredSet[varName] {
			return "", wsrsrc.ErrUndeclaredVar{Name: varName}
		}
		remaining = remaining[end+2:]
	}

	return result, nil
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
// overrideLookup: when non-nil, the session-keyed closure for resolving prompt
// override-point values; pass nil to render every override-point with its seed.
func printPlaybook(s *Server, rsrcRoot, name string, callerContext map[string]string, configOpts wsconfig.Options, workflowLang string, overrideLookup overrideLookupFn) (string, string, error) {
	return renderPlaybookBody(s, rsrcRoot, name, callerContext, configOpts, "", "", false, workflowLang, overrideLookup)
}

// renderPlaybook loads a playbook, renders it (with optional child-key mint and
// splice), writes it to a worktree-scoped tmp file, and returns the file path.
// The caller hands this path to a host-native subagent or mercenary.
//
// rsrcRoot and worktreeRoot are call-site-overridable seams for root_override support.
// mintRoot: when non-empty, caller is a lead and a child key is minted for the delegate.
// preferMercenary: when true and playbook is implementer/reviewer, adds mercenary-primary guidance.
// configOpts controls config-backed model alias resolution.
// overrideLookup: when non-nil, the session-keyed closure for resolving prompt
// override-point values; pass nil to render every override-point with its seed.
func renderPlaybook(s *Server, rsrcRoot, worktreeRoot, name string, callerContext map[string]string, configOpts wsconfig.Options, mintRoot string, parentKey string, preferMercenary bool, workflowLang string, overrideLookup overrideLookupFn) (string, string, error) {
	templateContext := callerContext
	var renderContext map[string]string
	if NoAgentMode() && wsflowRenderEligibleStems[name] && len(callerContext) > 0 {
		// Phase 2 of wsflow convergence: legacy wsflow delegate callers pass
		// arbitrary context as prompt data, not template variables. Preserve that
		// behavior only for the legacy wsflow stem set so ordinary playbook.render
		// still fails loudly on undeclared template variables.
		templateContext = nil
		renderContext = callerContext
	}
	body, recommendedTier, err := renderPlaybookBody(s, rsrcRoot, name, templateContext, configOpts, mintRoot, parentKey, preferMercenary, workflowLang, overrideLookup)
	if err != nil {
		return "", "", err
	}
	body = appendRenderContext(body, renderContext)
	generated, err := wsstate.NewManager(wsstate.Options{}).GeneratePaths(worktreeRoot, "prompt", []string{name})
	if err != nil {
		return "", "", fmt.Errorf("allocate playbook path: %w", err)
	}
	if err := os.WriteFile(generated[0].Path, []byte(body), 0o644); err != nil {
		return "", "", fmt.Errorf("write playbook %s: %w", generated[0].Path, err)
	}
	return generated[0].Path, recommendedTier, nil
}
