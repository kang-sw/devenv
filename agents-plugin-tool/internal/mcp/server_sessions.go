package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsgit"
)

func canonicalSetupRoot(runner wsgit.Runner, root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "<cwd>" {
		return "", fmt.Errorf("root must be an absolute repository path; the MCP server cannot infer the agent's current directory from %q", root)
	}
	return canonicalGitRoot(runner, root)
}

// handleLeadLogin implements the session-bootstrap tool (bootstrapToolName): canonicalize root, mint an
// ephemeral session key, store the {root, scope} entry in the registry, and
// return the key to the caller.
func (s *Server) handleLeadLogin(id json.RawMessage, arguments map[string]any) response {
	// Best-effort prune of stale key records, throttled to at most once per
	// pruneScanCadence internally. Piggybacks on every bootstrap call rather
	// than a dedicated tool or background goroutine; a prune hiccup must never
	// fail bootstrap, so its result is intentionally discarded.
	s.sessions.maybePrune()

	rootArg, _ := arguments["root"].(string)
	if strings.TrimSpace(rootArg) == "" {
		return toolTextResponse(id, "", fmt.Errorf("session bootstrap: root is required"))
	}
	canonical, err := canonicalSetupRoot(s.gitRunner(), rootArg)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	scope := parseCapabilityScope(arguments["capability"])
	parentKey, _ := arguments["parent_session_key"].(string)
	parentKey = strings.TrimSpace(parentKey)
	if relogin, _ := arguments["relogin_session_key"].(string); strings.TrimSpace(relogin) != "" {
		return s.handleLeadRelogin(id, arguments, strings.TrimSpace(relogin), canonical, scope, parentKey)
	}
	resolved, err := s.resolveSession(sessionAccess{mode: sessionAccessBootstrap, root: canonical, scope: scope, parent: parentKey})
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	key := resolved.key
	if resolved.hosted {
		s.rebindMailboxOwnerAtFerrule(key, "", roleLead, canonical)
		s.recordHarnessSession(key, arguments)
		return s.leadLoginResponse(id, arguments, key, canonical)
	}
	// Ferrule owner binding (mailbox core Decision 3): a parent-less
	// lead-capability mint is a top-lead (re-)login, so it rebinds this
	// process's active mailbox identity's owner pointer to the freshly minted
	// key. A parent-carrying mint, or a parent-less delegate/leaf mint (a
	// child launched without its lead's key), never touches it.
	s.rebindMailboxOwnerAtFerrule(key, parentKey, scope, canonical)
	// The same top-lead mint is the manual's documented session start, so it
	// runs the tracker's session-start check as the FRESH manual path does.
	if parentKey == "" && scope == roleLead {
		s.startTrackerSessionCheck()
		s.recordHarnessSession(key, arguments)
	}
	return s.leadLoginResponse(id, arguments, key, canonical)
}

// handleLeadRelogin is the bootstrap tool's hidden re-login: `relogin_session_key`
// names an existing key, and the call mints nothing. The argument is
// deliberately absent from the advertised tools/list schema: an agent that
// forgets its key mints a new one by convention, and an advertised re-login
// would invite re-logging in with another session's key to take its mailbox.
// The only intended caller is a harness adapter that persisted the lead key
// with its own session file and re-enters the same session (restart, reload,
// or revive with the key the human supplied). It is not named `session_key`
// because an adapter that default-fills `session_key` into every bridged call
// would turn every agent bootstrap into a re-login with the default key.
//
// Only a parent-less lead-capability key bound to this call's canonical root
// is accepted; it then takes the mailbox owner pointer under the ordinary
// rebind rules (rebindMailboxOwnerAtFerrule), including never stealing a name
// another live process holds. That rule is what keeps a copied session file
// (an import keeps the original's session id, so the copy re-logs in with the
// original's key) from becoming a mailbox takeover while the original process
// is alive. Any other key - unknown, parent-carrying, delegate or leaf, or
// bound to another root - is refused without a mint or an owner change, as is
// a re-login that also carries a parent or a non-lead capability, since those
// describe a key the re-login could never return.
func (s *Server) handleLeadRelogin(id json.RawMessage, arguments map[string]any, key, canonical string, scope toolRole, parentKey string) response {
	if _, err := s.resolveSession(sessionAccess{mode: sessionAccessRelogin, key: key, root: canonical, scope: scope, parent: parentKey}); err != nil {
		return toolTextResponse(id, "", err)
	}
	s.rebindMailboxOwnerAtFerrule(key, "", roleLead, canonical)
	s.recordHarnessSession(key, arguments)
	return s.leadLoginResponse(id, arguments, key, canonical)
}

// leadKeyRefusal is why key cannot re-log in as the top-level lead at
// canonical, as the predicate that follows the key in a refusal; empty when
// it can: a known, parent-less, lead-scope key bound to canonical.
func (s *Server) leadKeyRefusal(key, canonical string) string {
	entry, ok := s.lookupSession(key)
	switch {
	case !ok:
		return "is not a known session key"
	case entry.parent != "":
		return "carries a parent; only a parent-less lead key can re-log in"
	case entry.scope != roleLead:
		return fmt.Sprintf("is a %s key; only a lead key can re-log in", entry.scope)
	case entry.root != canonical:
		return "is bound to another root"
	}
	return ""
}

// hostedKeyRefusal is the refusal of key in a session hosted under the
// daemon-minted key hosted: it names the key to use instead.
func hostedKeyRefusal(tool, hosted, key string) error {
	return fmt.Errorf("%s: this session is hosted and its session key is fixed: use session_key %q (%s); %q is refused", tool, hosted, envHostedSessionKey, key)
}

// leadLoginResponse renders the bootstrap tool's result for key bound to
// canonical, shared by a fresh mint and a re-login so both carry the same
// fields and the same bootstrap staleness warning.
func (s *Server) leadLoginResponse(id json.RawMessage, arguments map[string]any, key, canonical string) response {
	result := map[string]any{
		"session_key": key,
		"root":        canonical,
	}
	text := fmt.Sprintf("session_key: %s\nroot: %s\n", key, canonical)
	if skillsRoot, srErr := s.skillsRoot(); srErr == nil {
		adapter := sessionConfigAdapter{s: s.sessions}
		resolver := wsconfig.NewResolver(s.configOptions(), builtinConfigDefaults(), adapter, adapter)
		warning := bootstrapStalenessWarning(canonical, skillsRoot, &resolver, "")
		if warning != "" {
			result["bootstrap_alarm"] = warning
			text += "\n" + warning + "\n"
		}
	}
	if wantsJSON(arguments) {
		return toolJSONResponse(id, result, nil)
	}
	return toolTextResponse(id, text, nil)
}

type sessionChildOutput struct {
	Key    string `json:"key"`
	Scope  string `json:"scope"`
	Parent string `json:"parent"`
	Depth  int    `json:"depth"`
	Live   bool   `json:"live"`
	Root   string `json:"root"`
	Note   string `json:"note,omitempty"`
}

func (s *Server) handleSessionChildren(id json.RawMessage, arguments map[string]any) response {
	sessionKey, _ := arguments["session_key"].(string)
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return toolTextResponse(id, "", fmt.Errorf("session.children: session_key is required"))
	}

	depth := 1
	if raw, ok := arguments["depth"]; ok {
		if f, ok := raw.(float64); ok {
			depth = int(f)
		}
	}
	includeDead, _ := arguments["include_dead"].(bool)

	scope := "any"
	if raw, exists := arguments["scope"]; exists {
		value, ok := raw.(string)
		if !ok || (value != "any" && value != "control" && value != "delegate") {
			return toolTextResponse(id, "", fmt.Errorf("session.children: scope must be control, delegate, or any"))
		}
		scope = value
	}
	unnotedOnly := false
	if raw, exists := arguments["unnoted_only"]; exists {
		value, ok := raw.(bool)
		if !ok {
			return toolTextResponse(id, "", fmt.Errorf("session.children: unnoted_only must be a boolean"))
		}
		unnotedOnly = value
	}

	children, err := s.sessions.children(sessionKey, depth)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	filtered := make([]sessionChild, 0, len(children))
	for _, child := range children {
		if !child.live && !includeDead {
			continue
		}
		if scope != "any" && sessionChildScopeLabel(child.scope) != scope {
			continue
		}
		if unnotedOnly && child.note != "" {
			continue
		}
		filtered = append(filtered, child)
	}

	out := make([]sessionChildOutput, 0, len(filtered))
	for _, child := range filtered {
		out = append(out, sessionChildOutput{
			Key:    child.key,
			Scope:  sessionChildScopeLabel(child.scope),
			Parent: child.parent,
			Depth:  child.depth,
			Live:   child.live,
			Root:   child.root,
			Note:   child.note,
		})
	}

	if wantsJSON(arguments) {
		return toolJSONResponse(id, map[string]any{
			"session_key": sessionKey,
			"depth":       depth,
			"children":    out,
		}, nil)
	}
	return toolTextResponse(id, formatSessionChildren(sessionKey, out, includeDead), nil)
}

func sessionChildScopeLabel(scope toolRole) string {
	switch scope {
	case roleLead:
		return "control"
	case roleDelegate:
		return "delegate"
	case roleLeaf:
		return "leaf"
	default:
		return string(scope)
	}
}

func formatSessionChildren(sessionKey string, children []sessionChildOutput, includeDead bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "session_key: %s\n", sessionKey)
	if len(children) == 0 {
		b.WriteString("children: none\n")
		return b.String()
	}
	b.WriteString("children:\n")
	for _, child := range children {
		indent := strings.Repeat("  ", child.Depth-1)
		fmt.Fprintf(&b, "%s- key: %s scope: %s depth: %d", indent, child.Key, child.Scope, child.Depth)
		if includeDead {
			live := "no"
			if child.Live {
				live = "yes"
			}
			fmt.Fprintf(&b, " live: %s", live)
		}
		fmt.Fprintf(&b, " root: %s", child.Root)
		if child.Note != "" {
			fmt.Fprintf(&b, " note: %s", child.Note)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// parseCapabilityScope maps the optional capability argument to a toolRole.
// An absent, nil, or "lead" capability maps to roleLead (unrestricted).
func parseCapabilityScope(raw any) toolRole {
	s, _ := raw.(string)
	switch strings.TrimSpace(s) {
	case "delegate":
		return roleDelegate
	case "leaf":
		return roleLeaf
	default:
		return roleLead
	}
}

func (s *Server) resolveToolRoot(arguments map[string]any, meta map[string]any) (string, error) {
	// Session-key branch: highest priority. When present, the key authoritatively
	// resolves the root, bypassing every fallback in the chain below.
	if key, ok := arguments["session_key"].(string); ok && strings.TrimSpace(key) != "" {
		entry, found := s.lookupSession(key)
		if !found {
			return "", fmt.Errorf("unknown_session: session key not found; " +
				"if you are the lead, re-bootstrap your session per ws:workflow-manual with your known root and retry the call")
		}
		return entry.root, nil
	}

	return "", fmt.Errorf("mandatory_session_key: root-aware ws tools require a session_key; if you are the lead, obtain one per ws:workflow-manual and pass it")
}

// canonicalGitRoot resolves root's worktree top level with runner's git
// (a hosted server's runner carries the connection's environment).
func canonicalGitRoot(runner wsgit.Runner, root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", fmt.Errorf("root is required")
	}
	output, err := runner.RunGit(context.Background(), root, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("root %q is not inside a Git worktree: %v", root, err)
	}
	canonical := strings.TrimSpace(string(output))
	if canonical == "" {
		return "", fmt.Errorf("root %q did not resolve to a Git worktree", root)
	}
	if evaluated, err := filepath.EvalSymlinks(canonical); err == nil {
		canonical = evaluated
	}
	return filepath.Clean(canonical), nil
}

func (s *Server) requireLeadSessionKey(toolName string, arguments map[string]any) (string, error) {
	key, _ := arguments["session_key"].(string)
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("%s: session_key is required", toolName)
	}
	entry, found := s.lookupSession(key)
	if !found {
		return "", fmt.Errorf("%s: unknown_session: session key not found; if you are the lead, re-bootstrap your session per ws:workflow-manual with your known root and retry the call", toolName)
	}
	if entry.scope != roleLead {
		return "", fmt.Errorf("%s: session_key must belong to a lead session", toolName)
	}
	return key, nil
}
