package mcp

import (
	"fmt"
	"strings"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// followupsArgument decodes git.commit's followups argument: an array of
// {level, category, content} objects. present reports whether the argument
// was supplied non-null, which is what the root-lead gate keys on.
func followupsArgument(arguments map[string]any) (followups []wsgit.Followup, present bool, err error) {
	raw, ok := arguments["followups"]
	if !ok || raw == nil {
		return nil, false, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, true, fmt.Errorf("followups must be an array of {level, category, content} objects; got %s", jsonValueTypeName(raw))
	}
	for i, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, true, fmt.Errorf("followups[%d] must be a {level, category, content} object; got %s", i, jsonValueTypeName(item))
		}
		var f wsgit.Followup
		for field, dst := range map[string]*string{"level": &f.Level, "category": &f.Category, "content": &f.Content} {
			value, ok := obj[field].(string)
			if !ok {
				return nil, true, fmt.Errorf("followups[%d].%s must be a string", i, field)
			}
			*dst = value
		}
		followups = append(followups, f)
	}
	return followups, true, nil
}

// requireRootLeadForFollowups is git.commit's argument-level gate: recording a
// follow-up is reserved to a root lead session (scope lead, no parent), while
// the tool itself stays shared with workers and delegates. A missing key, an
// unknown key, a non-lead scope, and a child lead key (a worker) all reject.
func (s *Server) requireRootLeadForFollowups(arguments map[string]any) error {
	key, _ := arguments["session_key"].(string)
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("git.commit refused: followups require the session_key of a root lead session; nothing was committed")
	}
	entry, found := s.sessions.lookup(key)
	if !found {
		return fmt.Errorf("git.commit refused: followups require a root lead session, and this session key is unknown; nothing was committed")
	}
	if entry.scope != roleLead || strings.TrimSpace(entry.parent) != "" {
		return fmt.Errorf("git.commit refused: followups may be recorded only by a root lead session (this key is a child or non-lead session); report the finding to your lead instead; nothing was committed")
	}
	return nil
}
