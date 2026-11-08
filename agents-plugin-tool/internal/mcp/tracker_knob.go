package mcp

import (
	"fmt"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

// trackerEnabled reads the knob that adds the tracker section to the workflow
// manual, for this server's session harness. resolver carries the reader's
// scopes and must be built with builtinConfigDefaults so the bare key's
// builtin (off) applies.
func (s *Server) trackerEnabled(resolver *wsconfig.Resolver, sessionKey string) (bool, error) {
	return s.bucketedKnobOn(resolver, sessionKey, wsconfig.ItemWorkflowTrackerEnabled)
}

// enableTrackerKnobForSession turns workflow.tracker_enabled on at session
// scope when it resolves off; an explicit session-scope value stays. The
// Phase 2 tracker tools call it after a successful call, past the
// top-level-lead gate on those tools, so a session that started recording
// work keeps the manual's tracker section across revive.
func (s *Server) enableTrackerKnobForSession(sessionKey string) error {
	if err := s.enableBucketedKnobForSession(sessionKey, wsconfig.ItemWorkflowTrackerEnabled); err != nil {
		return fmt.Errorf("tracker knob not enabled: %w", err)
	}
	return nil
}
