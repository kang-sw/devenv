package mcp

import (
	"slices"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// recordWhiteboardEffects is a best-effort projection, never a second board
// transaction. Call only with evidence that this call's board intent committed,
// after releasing its locks. A crash here or append failure loses this entry;
// reads/recovery never replay it and the board remains the source of truth.
func (s *Server) recordWhiteboardEffects(key string, effects []wswhiteboard.Effect) {
	if len(effects) == 0 {
		return // semantic no-op/render-only, independent of tool summaries
	}
	dir, ok := s.liveSessionDir(key)
	if !ok {
		return // unchanged dashboard-enabled and top-level-lead policy
	}
	effects = slices.Clone(effects)
	for i := range effects {
		e := &effects[i]
		e.Topic = wslive.ScrubSessionKeys(e.Topic, key)
		e.Before = wslive.ScrubSessionKeys(e.Before, key)
		e.After = wslive.ScrubSessionKeys(e.After, key)
	}
	entry, _ := wslive.NewWhiteboardEffects(effects)
	var slot wsdashboard.HostedSlot
	if ls := s.live.existing(key); ls != nil {
		slot = ls.slot
		ls.ringMu.Lock()
		defer ls.ringMu.Unlock()
	}
	appended := s.appendLiveRing(key, dir, slot, []wslive.RingEntry{entry})
	s.debugEvent("whiteboard.conversation_projection", map[string]any{"appended": appended})
}
