package wsagent

import (
	"fmt"
	"sort"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

func (r *Runner) startHeartbeatSweeper() {
	r.wg.Add(1)
	go r.heartbeatSweeper()
}

func (r *Runner) heartbeatSweeper() {
	defer r.wg.Done()
	ticker := time.NewTicker(r.opts.heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.sweepHeartbeats()
		case <-r.quit:
			return
		}
	}
}

func (r *Runner) sweepThreshold() time.Duration {
	return wsdashboard.HeartbeatInterval + min(r.opts.heartbeat, wsdashboard.HeartbeatInterval/2)
}

func (c *conv) sweepDue(now time.Time) bool {
	return now.Sub(c.lastRegAt()) >= c.r.sweepThreshold()
}

// sweepHeartbeats visits a stable, deterministic snapshot. Each attempt is
// non-blocking, so a held row lock cannot delay a later healthy session.
func (r *Runner) sweepHeartbeats() {
	type entry struct {
		id string
		c  *conv
	}
	r.mu.Lock()
	convs := make([]entry, 0, len(r.convs))
	for id, c := range r.convs {
		convs = append(convs, entry{id: id, c: c})
	}
	r.mu.Unlock()
	sort.Slice(convs, func(i, j int) bool { return convs[i].id < convs[j].id })

	now := r.opts.Now()
	for _, item := range convs {
		item.c.sweepHeartbeat(now)
	}
}

func (c *conv) sweepHeartbeat(now time.Time) {
	if c.isForeign() || !c.sweepDue(now) || c.rowHeld.Load() > 0 {
		return
	}
	rec := c.record()
	if !rec.Lead || rec.ClosedAt != "" {
		return
	}
	if c.writeSweptHeartbeat(now, rec) {
		c.r.notify(rec.ReplyID, rec.Root)
	}
}

// writeSweptHeartbeat performs the fenced read-modify-write. Its deferred
// releases run before the caller notifies the dashboard.
func (c *conv) writeSweptHeartbeat(now time.Time, rec Record) bool {
	rowUnlock, locked, err := wsdashboard.TryLockRegistration(c.r.opts.Dir, rec.ReplyID)
	if err != nil {
		fmt.Fprintf(c.r.opts.Log, "agent %s: heartbeat sweep: %v\n", rec.ReplyID, err)
		return false
	}
	if !locked {
		return false
	}
	defer rowUnlock()

	ownerUnlock, owned := c.r.lockSummaryOwner(rec.ReplyID, c)
	if !owned {
		return false
	}
	defer ownerUnlock()
	if c.isForeign() || !c.sweepDue(now) {
		return false
	}

	row, err := wsdashboard.ReadRegistration(c.r.opts.Dir, rec.ReplyID)
	if err != nil {
		return false
	}
	if isForeignRow(row, nil) {
		c.markForeign()
		return false
	}
	if !row.Hosted || row.EndedAt != "" || row.Harness != rec.Backend || row.HostPID != rec.HostPID {
		return false
	}
	row.HeartbeatAt = now.UTC().Format(time.RFC3339)
	if err := wsdashboard.WriteRegistration(c.r.opts.Dir, row); err != nil {
		fmt.Fprintf(c.r.opts.Log, "agent %s: registration heartbeat: %v\n", rec.ReplyID, err)
		return false
	}
	c.setLastReg(now)
	return true
}
