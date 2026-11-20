// playbook_render.go renders playbook bodies and product mode sections.
package mcp

import (
	"fmt"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsrsrc"
	"io/fs"
	"os"
	"strings"
)

func renderProductModePlaybookBody(body string, noAgent bool) string {
	return selectProductModeBlocks(body, noAgent)
}

const (
	fullOnlyStart   = "<!-- ws:full-only:start -->"
	fullOnlyEnd     = "<!-- ws:full-only:end -->"
	wsflowOnlyStart = "<!-- ws:wsflow-only:start -->"
	wsflowOnlyEnd   = "<!-- ws:wsflow-only:end -->"
)

// selectProductModeBlocks removes marker comments and keeps only the sections
// that apply to the current product mode. The source rsrc remains shared; the
// rendered playbook is the product-specific contract. noAgent selects the
// agentless (wsflow) product mode.
func selectProductModeBlocks(body string, noAgent bool) string {
	lines := strings.Split(body, "\n")
	filtered := make([]string, 0, len(lines))
	fullOnly := false
	wsflowOnly := false
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
		}
		if (fullOnly && noAgent) || (wsflowOnly && !noAgent) {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

// resolveRsrcRoot selects build-owned resources. The internal explicit path is
// only for authoring inspection; connection environment never selects it.
func (s *Server) resolveRsrcRoot(rsrcRootOverride string) (fs.FS, error) {
	if strings.TrimSpace(rsrcRootOverride) != "" {
		return os.DirFS(rsrcRootOverride), nil
	}
	return s.rsrcRoot()
}

// renderPlaybookBody is the shared core for printPlaybook and renderPlaybook.
// It loads the playbook once without substitution so include and overlay
// semantics stay in wsrsrc, then applies MCP-layer substitution. The MCP layer
// owns implicit namespace variables because their values come from runtime
// product mode rather than playbook frontmatter.
//
// The delegation tip is appended after substitution when meta.Delegates is true.
//
// rsrcRoot selects the plugin playbook, manifest, and includes.
// configOpts is forwarded to resolveModelVars; tests pass Options{CacheHome:tmpDir}.
//
// mintRoot: when non-empty, the caller is a lead and this function will mint a
// fresh child key (role taken from the playbook's Meta.Role) bound to mintRoot.
// The minted key is spliced as a clearly-delimited block into the rendered body
// so the delegate's ws calls are pre-keyed. When mintRoot is empty or the playbook
// role is not delegate-eligible, no key is minted and the body is unchanged.
//
// overrideLookup: when non-nil, a session-keyed resolver closure used to resolve
// override-point marker values before product-mode selection. Pass nil (e.g. from
// printPlaybook or unit tests that do not seed overrides) to render every
// override-point with its inline seed default.
//
// tierOverride: when non-empty, replaces the playbook's frontmatter tier for
// this render only — both RoleModel resolution and the recommended binding
// drive off it instead of pb.Meta.Tier. See resolveEffectiveTier. Pass "" to
// leave frontmatter-tier behavior unchanged (printPlaybook never overrides).
//
// Returns (body, recommendedTier, error): recommendedTier is the first-class tier
// declared in the playbook frontmatter, or tierOverride when supplied, surfaced
// so the caller can pick the delegate's model from one render call.
func renderPlaybookBody(s *Server, rsrcRoot fs.FS, name string, callerContext map[string]string, configOpts wsconfig.Options, mintRoot string, parentKey string, workflowLang string, overrideLookup overrideLookupFn, tierOverride string) (string, string, error) {
	return renderPlaybookBodyForCaller(s, "", "", rsrcRoot, name, callerContext, configOpts, mintRoot, parentKey, workflowLang, overrideLookup, tierOverride)
}

// renderPlaybookBodyForCaller is renderPlaybookBody for the caller holding
// callerKey (empty for a keyless render): the caller's own launch record, not
// only the connection's detected harness, selects its terminology row (see
// playbookTerminology). profile is the caller's playbook profile ("" for the
// base files; see callerPlaybookProfile).
func renderPlaybookBodyForCaller(s *Server, callerKey, profile string, rsrcRoot fs.FS, name string, callerContext map[string]string, configOpts wsconfig.Options, mintRoot string, parentKey string, workflowLang string, overrideLookup overrideLookupFn, tierOverride string) (string, string, error) {
	harness := s.playbookHarness()
	descriptor := s.connectionDescriptor()
	// A hosted lead with no ws skills (playbookCommands) is under the
	// native-subagent removal policy (wsagent.RemovesSubagents): it
	// delegates through agent.spawn, where its connection allows it.
	daemonAgentClient := (harness == "pi" && s.clientPiDaemonAgents()) || descriptor.playbookCommands || s.callerDaemonAgents(callerKey)
	daemonAgents := daemonAgentClient && s.agentListed() && s.toolAllowed("agent.spawn") && s.toolAllowed("mailbox.send")
	terminology := playbookTerminology(harness, daemonAgents, s.runtimeNamespace())

	// The workflow manual's harness overlay maps a render onto that
	// harness's own spawn tool; a caller delegating through agent.spawn reads
	// the base binding instead. Every other overlay keeps the harness.
	loadHarness := harness
	if daemonAgents && name == workflowManualPlaybookName {
		loadHarness = ""
	}

	// Load once with nil vars so the MCP playbook layer can add reserved
	// namespace variables without teaching product-mode semantics to wsrsrc.
	pb, err := wsrsrc.LoadFS(rsrcRoot, name, loadHarness, profile, nil)
	if err != nil {
		return "", "", err
	}
	if descriptor.playbookCommands {
		// Its sessions have no ws skills: a skill reference names the
		// dashboard command that runs the playbook (skillRef).
		pb.Body = strings.ReplaceAll(pb.Body, "{{.SkillNamespace}}:", "/")
	}

	// recommendedTier is the first-class tier declared in frontmatter, surfaced to
	// the caller as a host model-selection guide for the spawned subagent — unless
	// tierOverride replaces it for this render (validated, not coerced).
	recommendedTier, err := resolveEffectiveTier(pb.Meta.Tier, tierOverride, harness, configOpts)
	if err != nil {
		return "", "", err
	}

	vars, err := buildPlaybookVars(pb.Meta.Variables, callerContext, harness, terminology, recommendedTier, configOpts, workflowLang, s.runtimeNamespace())
	if err != nil {
		return "", "", err
	}
	body, err := substitutePlaybookVars(pb.Body, pb.Meta.Variables, vars)
	if err != nil {
		return "", "", err
	}
	if pb.Meta.Delegates {
		body += delegationTip(terminology)
	}

	// Unit 1: render-minted child key — only when caller is lead (mintRoot != "").
	if strings.TrimSpace(mintRoot) != "" {
		if childScope, ok := childRoleForPlaybookRole(pb.Meta.Role); ok {
			resolved, err := s.resolveSession(sessionAccess{mode: sessionAccessMint, root: mintRoot, scope: childScope, parent: parentKey})
			if err != nil {
				return "", "", fmt.Errorf("mint child session key: %w", err)
			}
			childKey := resolved.key
			if strings.ToLower(strings.TrimSpace(pb.Meta.Role)) == subleadPlaybookRole {
				if err := s.sessions.writeSubleadMarker(childKey, name); err != nil {
					return "", "", fmt.Errorf("mark sub-lead session key: %w", err)
				}
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
	// A pre-marker (0.50.7) lead-ticket resource states the auto-ack rule as
	// plain text that no override marker reaches.
	body, err = s.applyLegacyLeadTicketAutoAck(callerKey, name, body)
	if err != nil {
		return "", "", err
	}

	return renderProductModePlaybookBody(body, s.noAgentMode()), recommendedTier, nil
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
