package mcp

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// isLeadOnlyTool reports whether a tool is restricted to lead-scoped session
// keys. It is the authority for the keyed-gate escalation block. The bootstrap
// tool must be listed explicitly because it no longer lives under the
// `lead.*` prefix (260617 obscurity rename) — a prefix-only check would
// silently stop blocking it for non-lead keys. workflow_manual is likewise
// listed explicitly: delegate/leaf callers must not reach the handler (Phase 3a
// security hardening), mirroring the ferrule guard. workflow_state (260702) is
// gated the same way as its sibling workflow_manual: it is a cheaper view of
// the same lead-bootstrap/recovery surface, not a general todo/agenda
// accessor, so it stays lead-only even though the underlying todo.*/agenda.*
// data is itself scope-open. tickets.sage_stamp (260723 Phase 2, renamed from
// tickets.sage_record) is listed explicitly because it stays inside the
// `tickets.*` prefix (no dedicated prefix to gate on): it is the sole
// terminal writer of sage-review posture + the ## Blocked companion, and
// reviewers must never write frontmatter directly (single-writer property,
// {#260720-sage-gate-record-tools}) — this is new enforcement, not a
// preserved no-op, since the pre-rename tickets.sage_record was reachable by
// a delegate-scoped key.
func isLeadOnlyTool(name string) bool {
	return sessionHireTool(name) || isPiTool(name) || isAgentTool(name) || name == bootstrapToolName || name == "workflow_manual" || name == "workflow_state" || name == "tickets.sage_stamp" || name == "tickets.index_init" || name == "git.merge" || name == "worktree.acquire" || name == "worktree.release" || name == "worktree.remove" || name == "worktree.list" || strings.HasPrefix(name, "lead.") || workflowPreferenceWriterTool(name)
}

func workflowPreferenceWriterTool(name string) bool {
	entry, ok := configKeyEntryForTool(name)
	return ok && entry.RequiresLeadAuthority
}

// wsNamespaceRef matches the ws namespace prefix token (ws/ or ws:) anchored at
// a word boundary so that words containing "ws" as an interior substring (e.g.
// "news/", "rows:", "workflows/") are never mangled.
var wsNamespaceRef = regexp.MustCompile(`\bws([/:])`)

func LeadToolNames() []string { return leadToolNames(os.Getenv) }

// LeadToolNamesFromEnv reports the runtime inventory under a connecting
// caller's product mode, without applying a session's tool-profile filter.
func LeadToolNamesFromEnv(env []string) []string { return leadToolNames(newProcEnv(env).get) }

func leadToolNames(getenv func(string) string) []string {
	names := make([]string, 0, len(tools()))
	for _, tool := range tools() {
		name, _ := tool["name"].(string)
		if permanentlyHiddenTool(name) {
			continue
		}
		if envFlagOn(getenv(envNoAgent)) && noAgentHiddenTool(name) {
			continue
		}
		if hiddenUnlessAllowlisted(name) && !parseAllowedTools(getenv(envAllowedTools))[name] {
			continue
		}
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func (s *Server) filteredTools() []map[string]any {
	namespace := s.runtimeNamespace()
	descriptor := s.connectionDescriptor()
	base := tools()
	filtered := make([]map[string]any, 0, len(base))
	for _, tool := range base {
		name, _ := tool["name"].(string)
		if permanentlyHiddenTool(name) {
			continue
		}
		if isPiTool(name) {
			// Never listed: agent.* and mailbox.send replace them. Their
			// handlers still answer (piUnavailable gates a call), so a
			// session started on an older plugin keeps working until they
			// are deleted.
			continue
		}
		if isAgentTool(name) {
			// Listed where a runner answers them, the daemon's or the token
			// route's; the agentless product mode keeps them, since
			// delegation runs through them there too.
			if !s.agentListed() {
				continue
			}
			if s.piForwards() {
				tool = agentWorkerTool(tool)
			}
		}
		if s.toolAllowed(name) {
			definition := publicToolDefinition(tool, descriptor.aliases.advertised(name), namespace)
			if descriptor.toolsAlwaysLoad {
				meta := make(map[string]any)
				if existing, ok := definition["_meta"].(map[string]any); ok {
					for key, value := range existing {
						meta[key] = value
					}
				}
				meta["antigravity/alwaysLoad"] = true
				definition["_meta"] = meta
			}
			filtered = append(filtered, definition)
		}
	}
	return filtered
}

func publicToolDefinition(tool map[string]any, advertisedName, namespace string) map[string]any {
	clone := make(map[string]any, len(tool))
	for key, value := range tool {
		clone[key] = value
	}
	if advertisedName != "" {
		clone["name"] = advertisedName
	}
	if description, _ := clone["description"].(string); description != "" {
		clone["description"] = namespaceText(description, namespace)
	}
	if schema, ok := clone["inputSchema"].(map[string]any); ok {
		clone["inputSchema"] = namespaceValue(schema, namespace)
	}
	return clone
}

func (s *Server) toolAllowed(name string) bool {
	if s.noAgentMode() && noAgentHiddenTool(name) {
		return false
	}
	if allowed := s.explicitAllowedTools(); len(allowed) > 0 {
		return allowed[name]
	}
	return !hiddenUnlessAllowlisted(name)
}

// isNoteMutationTool reports whether name changes note state every session
// sees. Mute and unmute count: they change what every Notes block shows.
func isNoteMutationTool(name string) bool {
	switch name {
	case "note.write", "note.erase", "note.mute", "note.unmute":
		return true
	default:
		return false
	}
}

// noteReadOnlyDenial is the refusal a spawned session gets for a note
// mutation. It names the hand-off, because the Notes footer tells every
// reader to erase stale notes and a worker must learn that erasing is its
// lead's job.
func noteReadOnlyDenial(name string) string {
	return fmt.Sprintf("%s is not available to a spawned session: notes are read-only here (note.query still works). Put the note's content (key, value, and the layer it needs) in your final report so your lead can write it; erasing, muting, and unmuting notes is also your lead's job.", name)
}

func roleAllowsTool(role toolRole, name string) bool {
	switch role {
	case roleLead:
		return true
	case roleDelegate:
		if strings.HasPrefix(name, "session.") {
			return false
		}
		// config.get is a read-only single-key read; a child must reach it to
		// inherit its lead's session-scope values. config.resolve_agent is a
		// read-only tier resolution a worker playbook runs before each spawn.
		return name == "config.get" || name == "config.resolve_agent" || !strings.HasPrefix(name, "config.")
	case roleLeaf:
		if name == "config.get" {
			return true
		}
		return !strings.HasPrefix(name, "config.") && !strings.HasPrefix(name, "session.") && name != "git.commit"
	default:
		return false
	}
}

func namespaceText(text, namespace string) string {
	if namespace == "ws" {
		return text
	}
	return namespaceTerms(text, namespace)
}

func namespaceTerms(text, namespace string) string {
	text = wsNamespaceRef.ReplaceAllString(text, namespace+"$1")
	for _, term := range []string{"MCP", "plugin", "project", "runtime", "tool", "tools", "workflow", "user", "agent", "agents"} {
		pattern := regexp.MustCompile(`\bws ` + regexp.QuoteMeta(term) + `\b`)
		text = pattern.ReplaceAllString(text, namespace+" "+term)
	}
	for _, term := range []string{"managed", "owned"} {
		pattern := regexp.MustCompile(`\bws-` + regexp.QuoteMeta(term) + `\b`)
		text = pattern.ReplaceAllString(text, namespace+"-"+term)
	}
	return text
}

func namespaceValue(value any, namespace string) any {
	switch typed := value.(type) {
	case string:
		return namespaceText(typed, namespace)
	case map[string]any:
		clone := make(map[string]any, len(typed))
		for key, child := range typed {
			clone[key] = namespaceValue(child, namespace)
		}
		return clone
	case map[string]string:
		clone := make(map[string]string, len(typed))
		for key, child := range typed {
			clone[key] = namespaceText(child, namespace)
		}
		return clone
	case []any:
		clone := make([]any, len(typed))
		for i, child := range typed {
			clone[i] = namespaceValue(child, namespace)
		}
		return clone
	case []string:
		clone := make([]string, len(typed))
		for i, child := range typed {
			clone[i] = namespaceText(child, namespace)
		}
		return clone
	default:
		return value
	}
}

// permanentlyHiddenTool returns true for tools that must never appear on the
// public MCP surface regardless of mode. exec.* tools are under active
// development (epic 260524) and not yet documented in lead-workflow-manual;
// they are hidden until the surface stabilizes.
func permanentlyHiddenTool(name string) bool {
	return strings.HasPrefix(name, "exec.")
}

func noAgentHiddenTool(name string) bool {
	if permanentlyHiddenTool(name) {
		return true
	}
	// A config.* knob declaring NoAgentVisible: false is hidden on the
	// agentless surface. Every other config.* knob, including the bootstrap
	// tool, stays visible (wsflow still needs session-key bootstrap).
	if strings.HasPrefix(name, "config.") {
		if entry, ok := configKeyEntryForTool(name); ok {
			return !entry.NoAgentVisible
		}
	}
	return false
}

func parseAllowedTools(raw string) map[string]bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	allowed := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if name != "" {
			allowed[name] = true
		}
	}
	return allowed
}
