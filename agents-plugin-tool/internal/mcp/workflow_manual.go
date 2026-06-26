package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kang-sw/devenv/internal/wsconfig"
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

// handleWorkflowManual implements the ws.workflow_manual tool. A valid
// session_key is required; keyless calls receive a hard error. Behaviour by key:
//   - reserved freshBootstrapKey sentinel -> FRESH (manual + gated bootstrap line);
//   - resolves to a record -> CONTINUE (manual, bootstrap stripped, + Session State);
//   - syntactically valid but unresolvable, non-sentinel -> FAIL-LOUD: a minimal
//     no-restore notice with NO manual body rendered (see below), never minting.
func (s *Server) handleWorkflowManual(id json.RawMessage, args map[string]any) response {
	key, _ := args["session_key"].(string)
	key = strings.TrimSpace(key)

	// 1. Key absent — reject with a required-key error. Do not name the sentinel
	//    or ferrule in the error so no bootstrap hint leaks to keyless callers.
	if key == "" {
		return toolTextResponse(id, "", fmt.Errorf("ws.workflow_manual: a valid session_key is required"))
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
		return toolTextResponse(id, "", fmt.Errorf("ws.workflow_manual: resolve rsrc root: %w", err))
	}

	overrideLookup := buildOverrideLookup(s, key)

	// Resolve workflow.lang for language-binding injection.
	langAdapter := sessionConfigAdapter{s: s.sessions}
	langResolver := wsconfig.NewResolver(wsconfig.Options{}, nil, langAdapter, langAdapter)
	workflowLangRV, _ := langResolver.Get(key, wsconfig.ItemWorkflowLang)
	workflowLang := workflowLangRV.Value

	body, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, wsconfig.Options{}, workflowLang, overrideLookup)
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("ws.workflow_manual: render playbook: %w", err))
	}

	// Only FRESH (sentinel) and CONTINUE (recOK) remain; the sentinel branch is
	// checked first so it never depends on a (non-existent) sentinel record.
	if key == freshBootstrapKey {
		// 3. FRESH (sentinel): keep the gated bootstrap line; strip only markers.
		body = stripModeGatedRegion(body, true)
	} else {
		// 4. CONTINUE (recOK): strip both markers and inner content; append state.
		body = stripModeGatedRegion(body, false)
		body += "\n\n" + renderSessionState(rec)
	}

	return toolTextResponse(id, body+"\n", nil)
}
