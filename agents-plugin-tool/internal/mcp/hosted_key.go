package mcp

import (
	"fmt"
	"strings"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// hostedLineageDepth bounds hostedDescendant's lineage walks: real chains
// are short (a lead, its children, theirs), and a
// corrupt cycle must still end.
const hostedLineageDepth = 16

// hostedSessionKeyRule holds a hosted lead connection's calls to its own
// session, before anything else reads the call's session_key: an omitted
// (or null, or blank) session_key is the connection's key, filled in for a tool
// whose schema declares one; an explicit key must be the connection's own
// or a key of one of its descendant sessions (hostedDescendant). It returns
// the refusal for any other key, "" to proceed. Every other connection,
// child connections included, keeps its call as it is. An omitted key and
// the connection's own key are settled by comparison alone, so the common
// call reads no file.
func (s *Server) hostedSessionKeyRule(tool string, args map[string]any) string {
	hosted := s.hostedSessionKey()
	if hosted == "" {
		return ""
	}
	raw, present := args["session_key"]
	key, isString := raw.(string)
	// A JSON null is how many clients send an optional argument they omit.
	if !present || raw == nil || (isString && strings.TrimSpace(key) == "") {
		if toolDeclaresSessionKey(tool) {
			args["session_key"] = hosted
		}
		return ""
	}
	if !isString {
		// Not a key at all: the tool's own argument handling answers it.
		return ""
	}
	key = strings.TrimSpace(key)
	if key == hosted || s.hostedDescendant(hosted, key) {
		return ""
	}
	return hostedDescendantRefusal(tool, hosted, key).Error()
}

// hostedDescendantRefusal names the rule a hosted lead connection refused a
// session_key by.
func hostedDescendantRefusal(tool, hosted, key string) error {
	return fmt.Errorf("%s: this session is hosted and its session key is fixed: session_key %q is neither its key %q (%s) nor the key of one of its descendant sessions, so it is refused; omit session_key or pass %q", tool, key, hosted, envHostedSessionKey, hosted)
}

// toolDeclaresSessionKey reports whether tool's input schema has a
// session_key property.
func toolDeclaresSessionKey(tool string) bool {
	properties, _ := toolInputSchema(tool)["properties"].(map[string]any)
	_, ok := properties["session_key"]
	return ok
}

// hostedDescendant checks runner authority first (agent.spawn keys have no
// key-record parent), then the session-key parent chain for keys minted by
// tools such as playbook.render and worktree.acquire.
func (s *Server) hostedDescendant(hosted, key string) bool {
	cacheKey := [2]string{hosted, key}
	if _, ok := s.hostedDescendants.Load(cacheKey); ok {
		return true
	}
	if func() bool {
		rec, ok := s.callerRecord(key)
		if !ok {
			return false
		}
		dir, err := s.dashboard.stateDir()
		if err != nil {
			return false
		}
		target := wsmailbox.ReplyID(hosted)
		for depth := 0; rec.Parent != nil && depth < hostedLineageDepth; depth++ {
			parent := rec.Parent.ReplyID
			if parent == target {
				return true
			}
			if !wsdashboard.ValidReplyID(parent) {
				return false
			}
			if rec, err = wsagent.ReadRecord(dir, parent); err != nil {
				return false
			}
		}
		return false
	}() {
		return true
	}
	for depth := 0; depth < hostedLineageDepth; depth++ {
		entry, ok := s.lookupSession(key)
		if !ok || entry.parent == "" {
			return false
		}
		if entry.parent == hosted {
			s.hostedDescendants.Store(cacheKey, struct{}{})
			return true
		}
		key = entry.parent
	}
	return false
}
