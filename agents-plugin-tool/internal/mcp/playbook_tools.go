package mcp

import (
	"fmt"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsrsrc"
	"github.com/kang-sw/devenv/internal/wsstate"
	"io/fs"
	"os"
	"strings"
)

// printPlaybook loads a playbook and returns its rendered body text inline.
//
// The global delegation preference adds an invocation hint, not an executor
// procedure: eligibility remains owned by lead-delegate.
//
// printPlaybook never mints child keys (mintRoot="").
//
// rsrcRoot selects the plugin resource tree.
// configOpts controls config-backed model alias resolution.
// overrideLookup: when non-nil, the session-keyed closure for resolving prompt
// override-point values; pass nil to render every override-point with its seed.
func printPlaybook(s *Server, rsrcRoot fs.FS, name string, callerContext map[string]string, configOpts wsconfig.Options, workflowLang string, overrideLookup overrideLookupFn) (string, string, error) {
	return printPlaybookForCaller(s, "", "", rsrcRoot, name, callerContext, configOpts, workflowLang, overrideLookup)
}

// printPlaybookForCaller is printPlaybook for the caller holding callerKey
// and selecting profile (see renderPlaybookBodyForCaller).
func printPlaybookForCaller(s *Server, callerKey, profile string, rsrcRoot fs.FS, name string, callerContext map[string]string, configOpts wsconfig.Options, workflowLang string, overrideLookup overrideLookupFn) (string, string, error) {
	// playbook.read exposes no tier_override arg — always "" (frontmatter tier).
	body, recommendedTier, err := renderPlaybookBodyForCaller(s, callerKey, profile, rsrcRoot, name, callerContext, configOpts, "", "", workflowLang, overrideLookup, "")
	if err != nil {
		return "", "", err
	}
	if name == workflowManualPlaybookName {
		enabled, err := workflowPreferSubagentEnabled(configOpts)
		if err != nil {
			return "", "", fmt.Errorf("resolve %s: %w", wsconfig.ItemWorkflowPreferSubagent, err)
		}
		if enabled {
			body += "\n\nUse `" + s.skillRef("lead-delegate") + "` for bounded investigation, diagnosis, drafting, and low-impact operational work."
		}
	}
	return body, recommendedTier, nil
}

// renderPlaybook loads a playbook, renders it (with optional child-key mint and
// splice), writes it to a worktree-scoped tmp file, and returns the file path.
// The caller hands this path to a host-native subagent.
//
// rsrcRoot selects plugin resources; worktreeRoot selects the artifact cache.
// mintRoot: when non-empty, caller is a lead and a child key is minted for the delegate.
// configOpts controls config-backed model alias resolution.
// overrideLookup: when non-nil, the session-keyed closure for resolving prompt
// override-point values; pass nil to render every override-point with its seed.
// tierOverride: when non-empty, replaces the playbook's frontmatter tier for
// this render only — see renderPlaybookBody and resolveEffectiveTier. Pass ""
// to leave frontmatter-tier behavior unchanged.
func renderPlaybook(s *Server, rsrcRoot fs.FS, worktreeRoot, name string, callerContext map[string]string, configOpts wsconfig.Options, mintRoot string, parentKey string, workflowLang string, overrideLookup overrideLookupFn, tierOverride string) (string, string, error) {
	path, _, recommendedTier, err := renderPlaybookWithPlan(s, "", "", rsrcRoot, worktreeRoot, name, callerContext, configOpts, mintRoot, parentKey, workflowLang, overrideLookup, tierOverride, "")
	return path, recommendedTier, err
}

// renderPlaybookWithPlan is renderPlaybook plus an optional write-once plan
// reservation. When planStem is non-empty it reserves an empty plan file under
// worktreeRoot through the plan path generator and ends the rendered body with
// a plan-path line naming it, returning that path as planPath. The plan is
// reserved only after the body rendered, and removed again when the prompt
// cannot be written, so a failed render never leaves an orphan plan that a
// later commit could pick up as a false contract. callerKey is the render
// caller's session key and profile its playbook profile (see
// renderPlaybookBodyForCaller).
func renderPlaybookWithPlan(s *Server, callerKey, profile string, rsrcRoot fs.FS, worktreeRoot, name string, callerContext map[string]string, configOpts wsconfig.Options, mintRoot string, parentKey string, workflowLang string, overrideLookup overrideLookupFn, tierOverride string, planStem string) (promptPath string, planPath string, recommendedTier string, err error) {
	templateContext := callerContext
	var renderContext map[string]string
	if s.noAgentMode() && wsflowRenderEligibleStems[name] && len(callerContext) > 0 {
		// Phase 2 of wsflow convergence: legacy wsflow delegate callers pass
		// arbitrary context as prompt data. Preserve that behavior for undeclared
		// keys while allowing declared variables to render normally.
		declared, err := declaredPlaybookVariables(s, rsrcRoot, name, profile)
		if err != nil {
			return "", "", "", err
		}
		templateContext, renderContext = splitDeclaredRenderContext(callerContext, declared)
	}
	body, recommendedTier, err := renderPlaybookBodyForCaller(s, callerKey, profile, rsrcRoot, name, templateContext, configOpts, mintRoot, parentKey, workflowLang, overrideLookup, tierOverride)
	if err != nil {
		return "", "", "", err
	}
	body = appendRenderContext(body, renderContext)
	manager := wsstate.NewManager(s.stateOptions())
	if planStem != "" {
		plans, err := manager.GeneratePaths(worktreeRoot, "plan", []string{planStem})
		if err != nil {
			return "", "", "", fmt.Errorf("allocate plan path: %w", err)
		}
		planPath = plans[0].Path
		body = strings.TrimRight(body, "\n") + "\n\n" + planPathLinePrefix + planPath + "\n"
	}
	discardPlan := func() {
		if planPath != "" {
			_ = os.Remove(planPath)
		}
	}
	generated, err := manager.GeneratePaths(worktreeRoot, "prompt", []string{name})
	if err != nil {
		discardPlan()
		return "", "", "", fmt.Errorf("allocate playbook path: %w", err)
	}
	if err := os.WriteFile(generated[0].Path, []byte(body), 0o644); err != nil {
		discardPlan()
		return "", "", "", fmt.Errorf("write playbook %s: %w", generated[0].Path, err)
	}
	return generated[0].Path, planPath, recommendedTier, nil
}

// renderPlanStem decides whether a playbook.render call reserves a plan and
// under which stem. It consumes planStemContextKey from callerContext for the
// delegate-implementer stem whoever the caller is, so the key never reaches
// variable validation, and returns a stem only for a top-level lead (a lead
// key with no parent; a worker's key carries one). An absent or blank stem
// falls back to defaultPlanStem.
func renderPlanStem(name string, callerContext map[string]string, topLevelLead bool) (map[string]string, string) {
	if name != delegateImplementerPlaybookName {
		return callerContext, ""
	}
	stem, ok := callerContext[planStemContextKey]
	if ok {
		rest := make(map[string]string, len(callerContext)-1)
		for key, value := range callerContext {
			if key != planStemContextKey {
				rest[key] = value
			}
		}
		callerContext = rest
	}
	if !topLevelLead {
		return callerContext, ""
	}
	if stem = strings.TrimSpace(stem); stem == "" {
		stem = defaultPlanStem
	}
	return callerContext, stem
}

func declaredPlaybookVariables(s *Server, rsrcRoot fs.FS, name, profile string) (map[string]bool, error) {
	pb, err := wsrsrc.LoadFS(rsrcRoot, name, s.playbookHarness(), profile, nil)
	if err != nil {
		return nil, err
	}
	declared := make(map[string]bool, len(pb.Meta.Variables))
	for _, variable := range pb.Meta.Variables {
		declared[variable] = true
	}
	return declared, nil
}

func splitDeclaredRenderContext(callerContext map[string]string, declared map[string]bool) (map[string]string, map[string]string) {
	templateContext := map[string]string{}
	renderContext := map[string]string{}
	for key, value := range callerContext {
		if declared[key] {
			templateContext[key] = value
			continue
		}
		renderContext[key] = value
	}
	if len(templateContext) == 0 {
		templateContext = nil
	}
	if len(renderContext) == 0 {
		renderContext = nil
	}
	return templateContext, renderContext
}

// readPlaybook is playbook.read's text for playbook name read with keyStr
// (empty for a keyless read) and callerContext: the rendered body with its
// knob variants and recommended tier, without the trailing newline the tool
// result adds.
func (s *Server) readPlaybook(name, keyStr string, callerContext map[string]string) (string, error) {
	rsrcRoot, err := s.resolveRsrcRoot("")
	if err != nil {
		return "", err
	}
	// Provide the render-time {{.MailboxWaitCommand}} value (concrete when the
	// key resolves an owned mailbox identity, generic otherwise) for the one
	// stem whose body substitutes it. Because it is a reserved implicit var,
	// any path that substitutes the body must supply it or substitution fails
	// with ErrUnprovidedVar — so the render dispatch below gates on the same
	// stem to keep playbook.render of this stem from failing closed.
	if name == mailboxWaitPlaybookName {
		callerContext = s.injectMailboxWaitCommand(callerContext, keyStr)
	}
	printOverrideLookup, err := buildOverrideLookup(s, keyStr)
	if err != nil {
		return "", err
	}
	// Resolve workflow.lang for language-binding injection.
	printLangResolver := s.sessionResolver(keyStr, nil)
	printWorkflowLangRV, err := printLangResolver.Get(keyStr, wsconfig.ItemWorkflowLang)
	if err != nil {
		return "", err
	}
	printProfile, err := s.callerPlaybookProfile(keyStr)
	if err != nil {
		return "", err
	}
	body, recommendedTier, err := printPlaybookForCaller(s, strings.TrimSpace(keyStr), printProfile, rsrcRoot, name, callerContext, s.configOptions(), printWorkflowLangRV.Value, printOverrideLookup)
	if err != nil {
		return "", err
	}
	// The remaining manual/discussion knob variants run after rendering;
	// ticket knob markers resolve in the shared render core.
	knobResolver := s.sessionResolver(keyStr, builtinConfigDefaults())
	whiteboardOn, err := s.whiteboardEnabled(&knobResolver, keyStr)
	if err != nil {
		return "", err
	}
	trackerOn, err := s.trackerEnabled(&knobResolver, keyStr)
	if err != nil {
		return "", err
	}
	// The tracker section is never a spawned key's (as workflow_manual
	// renders it); its tools refuse that key, except a store's support
	// conversation, which the tracker gate admits.
	if entry, found := s.lookupSession(keyStr); found && trackerOn && !entry.isTopLevelLead() && !s.trackerSupportReadsTracker(keyStr, entry) {
		trackerOn = false
	}
	body, err = applyKnobVariants(s, rsrcRoot, name, body, whiteboardOn, trackerOn)
	if err != nil {
		return "", err
	}
	if name == whiteboardPlaybookName || name == trackerPlaybookName {
		if body, err = appendCommandSyntax(rsrcRoot, body); err != nil {
			return "", err
		}
	}
	return withRecommendedTier(body, recommendedTier), nil
}
