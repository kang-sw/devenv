package wsdashboard

import (
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// railHover deliberately has no model, effort, output rate or history: the
// existing lead snapshot supplies none with output-evidence provenance. Do not
// borrow a child's telemetry or a hosted session's requested settings here.
type railHover struct {
	Band                      BandView
	Phase                     string
	RegisteredAt, SnapshotAt  int64
	TurnStartedAt, LastTurnMs int64
	Stale                     bool
}

func sessionHover(reg Registration, snap *wslive.Snapshot, live bool, now time.Time) railHover {
	h := railHover{Stale: !live || reg.HostStopped || snap == nil}
	if at, err := time.Parse(time.RFC3339Nano, reg.RegisteredAt); err == nil {
		h.RegisteredAt = at.UnixMilli()
	}
	if snap == nil {
		return h
	}
	h.Stale = h.Stale || !adapterPresent(snap, now)
	h.SnapshotAt = snap.UpdatedAt // snapshot receipt, never a model observation
	h.Band = bandView(snap)
	h.LastTurnMs = snap.Lead.PreviousTurnMs
	if !h.Stale {
		h.Phase = snap.Lead.Phase
		if leadWorking(h.Phase) || h.Phase == wslive.PhasePermission {
			h.TurnStartedAt = snap.Lead.TurnStartedAt
		}
	}
	return h
}
