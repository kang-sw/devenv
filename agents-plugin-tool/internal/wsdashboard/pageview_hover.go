package wsdashboard

import (
	"math"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// railHover reads only this session's own response evidence. Missing evidence
// never borrows a child's telemetry or a hosted session's requested settings.
type railHover struct {
	Band                      BandView
	Phase                     string
	RegisteredAt, SnapshotAt  int64
	TurnStartedAt, LastTurnMs int64
	Stale                     bool
	Observed                  *wslive.ObservedModel
	Rate, RateAt              int64
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
	h.Observed = snap.Lead.Observed
	if !h.Stale {
		h.Phase = snap.Lead.Phase
		if leadWorking(h.Phase) || h.Phase == wslive.PhasePermission {
			h.TurnStartedAt = snap.Lead.TurnStartedAt
		}
		var rate *wslive.ModelObservation
		if h.TurnStartedAt > 0 && h.Observed != nil && h.Observed.Current.At >= h.TurnStartedAt {
			rate = &h.Observed.Current
		} else if !leadWorking(h.Phase) && h.Phase != wslive.PhasePermission {
			rate = snap.Lead.LastTurnOutput
		}
		if rate != nil && rate.OutputTps != nil && *rate.OutputTps > 0 {
			h.Rate, h.RateAt = max(int64(math.Round(*rate.OutputTps)), 1), rate.At
		}
	}
	return h
}
