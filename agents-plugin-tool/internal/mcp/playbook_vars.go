// playbook_vars.go resolves playbook tier, model, namespace, and caller variables.
package mcp

import (
	"fmt"
	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsrsrc"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// resolveEffectiveTier returns tierOverride in place of frontmatterTier when
// tierOverride is non-empty (after trimming), rejecting an unknown/unresolvable
// override rather than silently coercing it. Absent tierOverride,
// frontmatterTier passes through unchanged — the existing frontmatter-tier
// render path is byte-identical.
//
// Validation reuses ResolveAgentTierForHarness, which rejects an unknown tier
// outright (unlike ResolveAgentForHarnessConfig, which coerces an
// empty/unrecognized tier to "medium" for its own callers). A render-time typo
// in tier_override must fail loudly here rather than silently resolving to the
// medium tier a few calls downstream.
func resolveEffectiveTier(frontmatterTier, tierOverride, harness string, configOpts wsconfig.Options) (string, error) {
	trimmed := strings.TrimSpace(tierOverride)
	if trimmed == "" {
		return frontmatterTier, nil
	}
	if _, _, _, _, err := wsconfig.ResolveAgentTierForHarness(configOpts, trimmed, harness); err != nil {
		return "", fmt.Errorf("tier_override: %w", err)
	}
	// Return the canonical first-class spelling, not the caller's accepted
	// alias verbatim: ResolveAgentTierForHarness validates "opus"/"Large"/etc
	// as valid tiers but does not itself return the normalized form, and the
	// recommended-tier return channel must stay in first-class vocabulary
	// (small/medium/large/xlarge) regardless of which alias the caller typed.
	return wsconfig.NormalizedTier(trimmed), nil
}

// resolveTierModel resolves a single tier string to a concrete per-harness
// model via the shared config seam, returning "" on resolver error. This is
// used for the playbook's own declared tier; fixed-tier variables resolve
// the model and effort together through the same config seam.
func resolveTierModel(harness, tier string, configOpts wsconfig.Options) string {
	_, model, _, err := wsconfig.ResolveAgentForHarnessConfig(configOpts, tier, "", "", harness)
	if err != nil {
		return ""
	}
	return model
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
	return map[string]string{"RoleModel": resolveTierModel(harness, tier, configOpts)}
}

// fixedTierVarNames maps each fixed capability tier (matches normalizedTier /
// ModelAlias vocabulary) to its render var name, in canonical
// Small/Medium/Large/XLarge taxonomy order (not Light/Core/Deep aliases).
var fixedTierVarNames = []struct {
	tier    string
	varName string
}{
	{"small", "SmallTierModel"},
	{"medium", "MediumTierModel"},
	{"large", "LargeTierModel"},
	{"xlarge", "XLargeTierModel"},
}

// resolveTierModelVars resolves the fixed-tier model and reasoning-effort vars —
// one config-resolved model and effort per capability tier, available to any playbook
// body unconditionally (see ImplicitVariableNames), not just the playbook's
// own declared tier.
//
// Author doctrine: {{.RoleModel}} names *this delegate's own* declared-tier
// model (varies per playbook); {{.*TierModel}} names *a specific tier's*
// model in prose regardless of the current playbook's own tier. Prefer tier
// language ("escalate to the medium tier") in guidance generally; use
// {{.*TierModel}} only where naming a concrete model materially helps (e.g.
// the lead must type a literal `model:` value on a spawn call). These vars
// only make such an instruction concrete — they do not wire the model
// themselves; the lead must still pass `model:` explicitly.
//
// Empty-value fallback: unlike RoleModel (which may render ""), these vars
// sit mid-sentence in prose, so a resolver error/misconfig falls back to a
// stable descriptive label ("the small-tier model") instead of an empty
// string — an empty slot mid-sentence reads as a rendering bug, whereas the
// label degrades gracefully. Efforts remain empty on resolver error or when
// unconfigured, so a playbook can instruct the host to omit the effort binding.
func resolveTierModelVars(harness string, configOpts wsconfig.Options) map[string]string {
	vars := make(map[string]string, 2*len(fixedTierVarNames))
	for _, entry := range fixedTierVarNames {
		_, model, effort, err := wsconfig.ResolveAgentForHarnessConfig(configOpts, entry.tier, "", "", harness)
		if err != nil {
			model, effort = "", ""
		}
		if model == "" {
			model = "the " + entry.tier + "-tier model"
		}
		vars[entry.varName] = model
		// Empty effort means omit the host binding, not a literal default value.
		// The label is substituted into line-oriented prose, so quote control
		// characters the same way the render payload does.
		vars[strings.TrimSuffix(entry.varName, "Model")+"ReasoningEffort"] = formatEffortForText(effort)
	}
	return vars
}

// resolveWorkflowLangVar generates the WorkflowLang instruction text from the
// resolved workflow.lang config value. Returns "" when lang is empty.
func resolveWorkflowLangVar(lang string) string {
	return wsagent.WorkflowLanguageInstruction(lang)
}

func resolveNamespaceVars(namespace string) map[string]string {
	return map[string]string{
		"McpNamespace":   namespace,
		"SkillNamespace": namespace,
	}
}

// mailboxWaitCommandGeneric is the host-neutral fallback rendered into
// {{.MailboxWaitCommand}} whenever a concrete, runnable command cannot be
// produced — a keyless (fresh) render, or a session_key that resolves to no
// live record. It reproduces the abstract command form the playbook body
// shipped before render-time injection, so a session with no key still learns
// the shape rather than being handed a broken command.
const mailboxWaitCommandGeneric = "python3 <plugin>/bin/ws-mcp-launcher.py mailbox wait --session-key <your key> [--slug <your address>] [--timeout <duration>]"

// envCLIName is the declared CLI shim or shell-quoted Python launcher command.
const envCLIName = "WS_MCP_CLI_NAME"

// Unquoted paths use shlex's ordinary word characters; quoted paths may
// contain any printable character except the quote itself. Shell operators
// are safe only inside those quotes, never as extra words or commands.
var launcherCommandPattern = regexp.MustCompile(`^(?:'(?:[^'\x00-\x1f\x7f]*[/\\])?python(?:3(?:\.[0-9]+)?)?(?:\.exe)?'|(?:[A-Za-z0-9_./:\\@%+=,\-]*[/\\])?python(?:3(?:\.[0-9]+)?)?(?:\.exe)?)[ ]+(?:'[^'\x00-\x1f\x7f]*ws-mcp-launcher\.py'|[A-Za-z0-9_./:\\@%+=,\-]*ws-mcp-launcher\.py)$`)

// CLICommandName returns a launcher or declared shim prefix for CLI guidance.
func CLICommandName() string { return CLICommandNameFrom(os.Getenv) }

// CLICommandNameFrom falls back to an illustrative launcher path, never the
// runtime executable, when the caller's launcher command is not recognizable.
func CLICommandNameFrom(getenv func(string) string) string {
	name := strings.TrimSpace(getenv(envCLIName))
	if name == "ws-cli" || name == "wsflow-cli" || launcherCommandPattern.MatchString(name) {
		return name
	}
	return "python3 <plugin>/bin/ws-mcp-launcher.py"
}

// mailboxWaitCommandVar resolves {{.MailboxWaitCommand}} to a concrete,
// runnable `mailbox wait` invocation for the caller's own session, or the
// generic fallback when one cannot be produced. The command starts with
// the caller's CLICommandName, the same name the re-arm command the `mailbox wait` CLI
// prints on return uses.
//
// The --slug is the caller's REGISTERED self address (owner-gated, the same
// source mailbox.lookup_peers' self.address and the workflow_manual ambient
// block read), never a re-derivation of WS_MAILBOX/WS_MAILBOX_AUTO from the
// environment: under WS_MAILBOX_AUTO the env carries no minted stem, so
// re-deriving it would emit a different address than the server registered. No
// registered address (env-less, or a non-owner session sharing this process)
// → the reply-id-only form (`--session-key` alone).
func mailboxWaitCommandVar(s *Server, sessionKey string) string {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return mailboxWaitCommandGeneric
	}
	entry, found := s.lookupSession(sessionKey)
	if !found {
		return mailboxWaitCommandGeneric
	}
	bin := s.cliCommandName()
	if bin == "" {
		return mailboxWaitCommandGeneric
	}
	parts := []string{bin, "mailbox", "wait", "--session-key", sessionKey}
	if isOwner, identity, err := s.mailboxOwnerCheck(sessionKey, entry.root); err == nil && isOwner {
		if addr := identity.address(); addr != "" {
			parts = append(parts, "--slug", addr)
		}
	}
	return strings.Join(parts, " ")
}

// injectMailboxWaitCommand sets the tool-owned {{.MailboxWaitCommand}} render
// variable on the caller context, overriding any caller-supplied value so the
// concrete command cannot be spoofed through render context (mirroring the
// namespace-var override precedent). It is called on the playbook.read path,
// the only path that substitutes the mailbox playbook body.
func (s *Server) injectMailboxWaitCommand(callerContext map[string]string, sessionKey string) map[string]string {
	if callerContext == nil {
		callerContext = map[string]string{}
	}
	callerContext["MailboxWaitCommand"] = mailboxWaitCommandVar(s, sessionKey)
	return callerContext
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
//  5. Tool-injected fixed-tier model and reasoning-effort vars are available to all playbooks
//     unconditionally, same as namespace vars — see resolveTierModelVars.
//  6. Only keys present in declared, plus namespace reserved vars, are included
//     in the result.
//
// Caller context keys that are neither declared nor namespace reserved return an
// ErrUndeclaredVar so the contract "undeclared caller context → loud error" is
// preserved while common namespace vars do not require frontmatter declarations.
//
// terminology is the caller's idiom row (playbookTerminology); harness still
// drives model resolution.
// tier is the playbook's declared capability tier (from pb.Meta.Tier); it drives
// RoleModel resolution. configOpts is forwarded unchanged to both
// resolveRoleModelVar (RoleModel) and resolveTierModelVars (the fixed-tier vars).
// workflowLang is the resolved workflow.lang value; it drives WorkflowLang injection.
func buildPlaybookVars(declared []string, callerContext map[string]string, harness string, terminology map[string]string, tier string, configOpts wsconfig.Options, workflowLang, namespace string) (map[string]string, error) {
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
	for k, v := range terminology {
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
	for k, v := range resolveNamespaceVars(namespace) {
		merged[k] = v
	}

	// Layer 5: fixed-tier model and reasoning-effort vars are available to all playbooks
	// unconditionally, mirroring the namespace-var precedent exactly (no
	// frontmatter declaration required; overrides caller context for these
	// reserved names).
	for k, v := range resolveTierModelVars(harness, configOpts) {
		merged[k] = v
	}

	// Layer 6: workflow.lang language-binding instruction — injected only when
	// the playbook declares WorkflowLang and a language is configured.
	if declaredSet["WorkflowLang"] {
		merged["WorkflowLang"] = resolveWorkflowLangVar(workflowLang)
	}

	return merged, nil
}

// delegationTip returns the harness-aware continuity tip fragment appended to the
// rendered body of delegates:true playbooks. The text is product-neutral: both
// product modes delegate to host-native subagents, so the fragment differs only
// by the caller's continuation idiom (terminology, see playbookTerminology).
func delegationTip(terminology map[string]string) string {
	continueIdiom := terminology["ContinueIdiom"]
	var sb strings.Builder
	sb.WriteString("\n\n---\n")
	sb.WriteString("**Continuity tip:** This playbook delegates to a subagent. ")
	sb.WriteString("When the subagent returns an agent id, use `")
	sb.WriteString(continueIdiom)
	sb.WriteString("` to send follow-up messages to the same agent rather than spawning a new one. ")
	sb.WriteString("The playbook surface keeps no agent registry; ")
	sb.WriteString("record the agent id in your workflow state if you need it across turns.")
	return sb.String()
}

// withRecommendedTier appends a `recommended-tier: <first-class>` metadata line to
// a playbook tool payload when the playbook declares a tier. Empty tier leaves the
// payload unchanged. This is the render/print return channel the lead reads to
// route a delegation's model selection: the host uses it to pick the subagent's
// model tier.
func withRecommendedTier(payload, tier string) string {
	if strings.TrimSpace(tier) == "" {
		return payload
	}
	return payload + "\nrecommended-tier: " + strings.TrimSpace(tier)
}

// withRecommendedRenderBinding appends config-resolved native spawn bindings to
// a playbook.render payload. The stable path and recommended-tier lines remain
// first; model and reasoning effort are additive, optional metadata. Resolver
// failures intentionally preserve the tier-only contract so callers can still
// choose a host default from the tier alone.
//
// playbook.read deliberately continues to use withRecommendedTier only: its
// tier-only return contract is host-neutral and unchanged by render bindings.
func withRecommendedRenderBinding(payload, harness, tier string, configOpts wsconfig.Options) string {
	payload = withRecommendedTier(payload, tier)
	if strings.TrimSpace(tier) == "" {
		return payload
	}
	_, model, effort, err := wsconfig.ResolveAgentForHarnessConfig(configOpts, tier, "", "", harness)
	if err != nil {
		return payload
	}
	if model = strings.TrimSpace(model); model != "" {
		payload += "\nrecommended-model: " + model
	}
	if effort = strings.TrimSpace(effort); effort != "" {
		payload += "\nrecommended-reasoning-effort: " + formatEffortForText(effort)
	}
	return payload
}

// formatEffortForText keeps provider-specific labels intact in structured
// resolution, but quotes control characters at line-oriented display boundaries.
func formatEffortForText(effort string) string {
	if strings.IndexFunc(effort, unicode.IsControl) >= 0 {
		return strconv.Quote(effort)
	}
	return effort
}

// childRoleForPlaybookRole maps a playbook frontmatter role string to the child key scope.
// Returns (scope, true) for delegate-eligible roles; ("", false) for non-eligible roles
// (lead, empty, or unknown).
//
// Mapping:
//   - "implementer", "reviewer", "delegate" → roleDelegate
//   - "leaf" → roleLeaf
//   - "worker" → roleLead: a worker executes a whole unit of work end to end
//     (route, edit, verify, review, commit, close), so it needs the same
//     unrestricted scope a lead holds. The key is a distinct child of the
//     caller (parent = caller key) minted at the caller's root. That root is
//     the key's default binding, not a confinement: lead scope permits every
//     tool, ferrule included, so a worker is trusted at lead level by design,
//     with one exception keyed on its parent: note.write, note.erase,
//     note.mute, and note.unmute are denied to every key that is not a
//     top-level lead (see isNoteMutationTool), so a worker hands a note to its
//     lead through its report.
//   - "sublead" → roleLead, like "worker", and the render also writes the
//     sub-lead marker naming the playbook (writeSubleadMarker), which admits
//     the key to a status-only path onto its top-level lead's whiteboard.
//   - "lead", "", unknown → ("", false): lead playbooks never mint child keys.
func childRoleForPlaybookRole(role string) (toolRole, bool) {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "implementer", "reviewer", "delegate":
		return roleDelegate, true
	case "leaf":
		return roleLeaf, true
	case "worker", subleadPlaybookRole:
		return roleLead, true
	default:
		return "", false
	}
}

func workflowPreferSubagentEnabled(configOpts wsconfig.Options) (bool, error) {
	resolver := wsconfig.NewResolver(configOpts, builtinConfigDefaults(), nil, nil)
	rv, err := resolver.Get("", wsconfig.ItemWorkflowPreferSubagent)
	if err != nil {
		return false, err
	}
	return strings.ToLower(strings.TrimSpace(rv.Value)) == preferSubagentEnabledValue, nil
}
