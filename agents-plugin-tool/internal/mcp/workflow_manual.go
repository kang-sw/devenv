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

// handleWorkflowManual implements the ws.workflow_manual tool. It renders the
// lead-workflow-manual playbook through the same pipeline as playbook.print,
// then branches on the session_key to produce fresh, continue, or fail-loud
// output. session_key is optional — an absent key is the fresh-bootstrap signal.
func (s *Server) handleWorkflowManual(id json.RawMessage, args map[string]any) response {
	key, _ := args["session_key"].(string)
	key = strings.TrimSpace(key)

	// Render the manual body through the same pipeline as playbook.print.
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

	// Branch on session_key and record resolution.
	rec, recOK := s.sessions.readState(key)
	switch {
	case key == "":
		// FRESH: keep the gated bootstrap line; strip only the marker comment lines.
		body = stripModeGatedRegion(body, true)

	case recOK:
		// CONTINUE: strip both markers and inner content; append Session State.
		body = stripModeGatedRegion(body, false)
		body += "\n\n" + renderSessionState(rec)

	default:
		// FAIL-LOUD: key given but no record. Keep bootstrap line (caller may need
		// to mint). Append an explicit no-restore notice. NEVER call s.sessions.mint.
		body = stripModeGatedRegion(body, true)
		body += "\n\n## Session State\n" +
			fmt.Sprintf("(no restorable state for session key %q; this key resolves to no stored session record — do not assume prior agenda/todo.)", key)
	}

	return toolTextResponse(id, body+"\n", nil)
}
