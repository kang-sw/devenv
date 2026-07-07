package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsrsrc"
)

// freshOnlyStart and freshOnlyEnd are the dedicated mode-gating marker tokens
// for the ws.workflow_manual tool. They are distinct from the product-mode
// markers (ws:full-only, ws:wsflow-only, ws:mercenary-on) and the override
// markers (ws:override:) so none of those passes consume or choke on them.
//
// Under playbook.print the markers surface verbatim as inert HTML comments
// (invisible when Markdown renders), preserving backward compatibility.
// The ONLY consumer is stripModeGatedRegion in this file.
const (
	freshOnlyStart = "<!-- ws:fresh-only:start -->"
	freshOnlyEnd   = "<!-- ws:fresh-only:end -->"
)

// freshBootstrapKey is the reserved sentinel that triggers fresh mode in
// ws.workflow_manual when a caller does not yet have a lead session key.
// It is an opaque, deliberately non-descriptive handshake token — no privilege
// cue, no semantic hint — mirroring ws.ferrule's "no semantic cue" rationale
// (260617 obscurity). It must NEVER appear in the advertised tool description,
// the session_key schema property text, or any error string. It is taught only
// in lead skill prose (lead-revive) so that subagents without that skill read
// cannot discover the fresh-bootstrap path.
const freshBootstrapKey = "obsidian-latch"

// stripModeGatedRegion removes the ws:fresh-only marker comment lines from
// body. When keepContent is true the inner lines between the marker pair are
// kept (fresh and fail-loud modes); when keepContent is false they are also
// removed (continue mode). Lines outside any marker pair are always kept.
//
// Robustness: an unclosed start marker is treated per keepContent for the
// remaining lines (defensive, mirrors applyOverrideMarkers behavior). Multiple
// gated regions in one body are all processed.
func stripModeGatedRegion(body string, keepContent bool) string {
	lines := strings.Split(body, "\n")
	result := make([]string, 0, len(lines))
	inRegion := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == freshOnlyStart {
			inRegion = true
			// drop the marker line itself
			continue
		}
		if trimmed == freshOnlyEnd {
			inRegion = false
			// drop the marker line itself
			continue
		}
		if inRegion && !keepContent {
			// drop inner content in continue mode
			continue
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

// renderSessionState builds the ## Session State section from a session record.
// It renders agenda blobs (keyed alphabetically) followed by the todo summary.
// Only scaffolding strings are handler-owned here; no manual prose.
func renderSessionState(rec sessionRecord) string {
	var sb strings.Builder
	sb.WriteString("## Session State\n")

	// Agenda blobs: sort keys for deterministic output.
	if len(rec.Agenda) > 0 {
		keys := make([]string, 0, len(rec.Agenda))
		for k := range rec.Agenda {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sb.WriteString(fmt.Sprintf("\n### agenda: %s\n", k))
			// Pretty-print the JSON blob.
			var pretty strings.Builder
			blob := rec.Agenda[k]
			var indent interface{}
			if err := json.Unmarshal(blob, &indent); err == nil {
				if indented, err := json.MarshalIndent(indent, "", "  "); err == nil {
					pretty.WriteString("```json\n")
					pretty.Write(indented)
					pretty.WriteString("\n```")
				} else {
					pretty.WriteString(string(blob))
				}
			} else {
				pretty.WriteString(string(blob))
			}
			sb.WriteString(pretty.String())
			sb.WriteString("\n")
		}
	}

	// Todo summary.
	sb.WriteString("\n### Todos\n")
	sb.WriteString(renderTodos(rec.Todos, false))
	sb.WriteString("\n")

	return sb.String()
}

// injectSessionKeyLine finds the first blockquote line containing
// "**Session invariant:**" and inserts a key-preservation hint immediately
// after it. This places the session key near the top of the rendered output
// so context-compaction summaries — which prioritise document beginnings —
// are far more likely to carry the key across compaction boundaries than the
// bottom-of-document "## Session Key" section alone.
// skepticalPostureBlock is the standing instruction block injected at the top
// of every workflow_manual output when workflow.skeptical_posture is "on"
// (the builtin default). It counteracts the tendency to accept user-stated
// facts (names, design decisions, remembered behavior) as ground truth without
// independent verification.
const skepticalPostureBlock = `> **Skeptical posture is active.**
> - Do not accept user claims about code, names, or behavior without independent verification.
> - Assume user memory is stale: if the user says something "was defined as X", check the source.
> - When a user's casual example or description conflicts with what you observe in source, the source wins.
> - Ask for evidence before acting on a user assertion that would change architecture, naming, or convention.

`

// injectSkepticalPosture prepends the skeptical-posture block to body.
func injectSkepticalPosture(body string) string {
	return skepticalPostureBlock + body
}

func injectSessionKeyLine(body, key string) string {
	lines := strings.Split(body, "\n")
	result := make([]string, 0, len(lines)+1)
	injected := false
	for _, line := range lines {
		result = append(result, line)
		if !injected && strings.Contains(line, "**Session invariant:**") {
			result = append(result, fmt.Sprintf("> **Session key: `%s`** — preserve verbatim in any compaction summary.", key))
			injected = true
		}
	}
	return strings.Join(result, "\n")
}

// handleWorkflowState implements the ws.workflow_state tool: a cheap,
// lead-only view of just the Session State section (agenda/todos) for a
// session_key, with no manual reference/primitives text. It reuses
// workflow_manual's exact key-validation and fail-loud behavior (260702) so an
// invalid/expired/unknown key produces the identical error shape rather than a
// separate state machine. Unlike workflow_manual, there is no FRESH mode here:
// the fresh-bootstrap sentinel is not a stored record, so it falls through to
// the same fail-loud path as any other unresolvable key.
func (s *Server) handleWorkflowState(id json.RawMessage, args map[string]any) response {
	key, _ := args["session_key"].(string)
	key = strings.TrimSpace(key)

	// 1. Key absent — reject with the same required-key error shape as
	//    workflow_manual. Do not name the sentinel or ferrule.
	if key == "" {
		return toolTextResponse(id, "", fmt.Errorf("workflow_state: a valid session_key is required"))
	}

	// 2. Reuse workflow_manual's fail-loud notice verbatim for any unresolvable
	//    key (including the sentinel, which is never a stored record). NEVER mint.
	rec, recOK := s.sessions.readState(key)
	if !recOK {
		notice := fmt.Sprintf("## Session State\n(no restorable state for session key %q; this key resolves to no stored session record — do not assume prior agenda/todo. If you are the lead recovering after compaction, re-run lead-revive to restore your session.)", key)
		return toolTextResponse(id, notice+"\n", nil)
	}

	// 3. Resolved record — render only the Session State section, no manual body.
	//    Match workflow_manual's exact trailing-newline shape (it appends the
	//    same renderSessionState output then a final "\n").
	return toolTextResponse(id, renderSessionState(rec)+"\n", nil)
}

// handleWorkflowManual implements the ws.workflow_manual tool. A valid
// session_key is required; keyless calls receive a hard error. Behaviour by key:
//   - reserved freshBootstrapKey sentinel -> FRESH (manual + gated bootstrap line);
//   - resolves to a record -> CONTINUE (manual, bootstrap stripped, + Session State);
//   - syntactically valid but unresolvable, non-sentinel -> FAIL-LOUD: a minimal
//     no-restore notice with NO manual body rendered (see below), never minting.
func (s *Server) handleWorkflowManual(id json.RawMessage, args map[string]any) response {
	key, _ := args["session_key"].(string)
	key = strings.TrimSpace(key)
	root, _ := args["root"].(string)
	root = strings.TrimSpace(root)

	// 1. Key absent — reject with a required-key error. Do not name the sentinel
	//    or ferrule in the error so no bootstrap hint leaks to keyless callers.
	if key == "" {
		return toolTextResponse(id, "", fmt.Errorf("workflow_manual: a valid session_key is required"))
	}

	// 2. FAIL-LOUD up front, BEFORE rendering: a syntactically valid but
	//    unresolvable, non-sentinel key must NOT receive the manual body at all.
	//    The body's always-shown per-root rule names ws.ferrule (the lead
	//    self-bootstrap call); any unregistered key bypasses the lead-only keyed
	//    gate via lookup-miss, so a non-lead caller reaching this path could read
	//    that mention to discover the escalation. Return only a no-restore notice
	//    naming lead-revive (a skill, not the ferrule/sentinel surface). NEVER mint.
	rec, recOK := s.sessions.readState(key)
	if key != freshBootstrapKey && !recOK {
		notice := fmt.Sprintf("## Session State\n(no restorable state for session key %q; this key resolves to no stored session record — do not assume prior agenda/todo. If you are the lead recovering after compaction, re-run lead-revive to restore your session.)", key)
		return toolTextResponse(id, notice+"\n", nil)
	}

	// Render the manual body (FRESH + CONTINUE only) through the same pipeline as
	// playbook.print.
	rsrcRoot, err := resolveRsrcRoot("")
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("workflow_manual: resolve rsrc root: %w", err))
	}

	overrideLookup := buildOverrideLookup(s, key)

	// Resolve workflow.lang and workflow.skeptical_posture for rendering.
	langAdapter := sessionConfigAdapter{s: s.sessions}
	langResolver := wsconfig.NewResolver(wsconfig.Options{}, nil, langAdapter, langAdapter)
	workflowLangRV, _ := langResolver.Get(key, wsconfig.ItemWorkflowLang)
	workflowLang := workflowLangRV.Value
	skepticalRV, _ := langResolver.Get(key, wsconfig.ItemWorkflowSkepticalPosture)
	skepticalPosture := skepticalRV.Value != "off" // builtin default is "on"

	body, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, wsconfig.Options{}, workflowLang, overrideLookup)
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("workflow_manual: render playbook: %w", err))
	}

	// Only FRESH (sentinel) and CONTINUE (recOK) remain; the sentinel branch is
	// checked first so it never depends on a (non-existent) sentinel record.
	if key == freshBootstrapKey {
		if root != "" {
			// 3a. FRESH with root: canonicalize the root, mint a lead key, strip the
			//     fresh-only block (caller already has a root — no need to instruct
			//     them to call ferrule), and return the key inline.
			canonical, err := canonicalSetupRoot(root)
			if err != nil {
				return toolTextResponse(id, "", fmt.Errorf("workflow_manual: canonicalize root: %w", err))
			}
			mintedKey, err := s.sessions.mint(canonical, roleLead, "")
			if err != nil {
				return toolTextResponse(id, "", fmt.Errorf("workflow_manual: mint session: %w", err))
			}
			body = stripModeGatedRegion(body, false)
			body = injectSessionKeyLine(body, mintedKey)
			body += "\n\n## Session Key\n" + mintedKey
			body += "\n\n" + renderSessionState(sessionRecord{})
			if skepticalPosture {
				body = injectSkepticalPosture(body)
			}
			if skillsRoot, srErr := wsrsrc.ResolveSkillsRoot(); srErr == nil {
				warningAdapter := sessionConfigAdapter{s: s.sessions}
				warningResolver := wsconfig.NewResolver(wsconfig.Options{}, builtinConfigDefaults(), warningAdapter, warningAdapter)
				warning := bootstrapStalenessWarning(canonical, skillsRoot, &warningResolver, mintedKey)
				body = injectBootstrapStalenessWarning(body, warning)
			}
			{
				warningAdapter := sessionConfigAdapter{s: s.sessions}
				warningResolver := wsconfig.NewResolver(wsconfig.Options{}, builtinConfigDefaults(), warningAdapter, warningAdapter)
				warning := docCoverageWarning(canonical, &warningResolver, mintedKey)
				body = injectDocCoverageWarning(body, warning)
			}
			return toolTextResponse(id, body+"\n", nil)
		}
		// 3b. FRESH (sentinel, no root): keep the gated bootstrap line; strip only markers.
		body = stripModeGatedRegion(body, true)
		if skepticalPosture {
			body = injectSkepticalPosture(body)
		}
	} else {
		// 4. CONTINUE (recOK): strip both markers and inner content; append state.
		body = stripModeGatedRegion(body, false)
		body = injectSessionKeyLine(body, key)
		body += "\n\n## Session Key\n" + key
		body += "\n\n" + renderSessionState(rec)
		if skepticalPosture {
			body = injectSkepticalPosture(body)
		}
		if rec.Root != "" {
			if skillsRoot, srErr := wsrsrc.ResolveSkillsRoot(); srErr == nil {
				warningAdapter := sessionConfigAdapter{s: s.sessions}
				warningResolver := wsconfig.NewResolver(wsconfig.Options{}, builtinConfigDefaults(), warningAdapter, warningAdapter)
				warning := bootstrapStalenessWarning(rec.Root, skillsRoot, &warningResolver, key)
				body = injectBootstrapStalenessWarning(body, warning)
			}
			{
				warningAdapter := sessionConfigAdapter{s: s.sessions}
				warningResolver := wsconfig.NewResolver(wsconfig.Options{}, builtinConfigDefaults(), warningAdapter, warningAdapter)
				warning := docCoverageWarning(rec.Root, &warningResolver, key)
				body = injectDocCoverageWarning(body, warning)
			}
		}
	}

	return toolTextResponse(id, body+"\n", nil)
}
