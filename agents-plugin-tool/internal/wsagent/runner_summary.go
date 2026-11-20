package wsagent

import (
	"fmt"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
)

// lockSummaryWriter is the hosted equivalent of terminal connection/slot
// fencing. The registration lock serializes against handoff and terminal
// login, and the session's own summary lock (lockSummaryOwner) pins this
// actual conversation generation through mutation/candidate freeze. Another
// session's lock is never taken, so one session's stalled ring I/O does not
// block another's snapshot publish.
func (c *conv) lockSummaryWriter() (func(), bool) {
	rec := c.record()
	if !rec.Lead || c.isForeign() {
		return nil, false
	}
	unlock, row, ok := c.lockRowRead()
	if !ok {
		return nil, false
	}
	release, owned := c.r.lockSummaryOwner(rec.ReplyID, c)
	if !owned {
		unlock()
		return nil, false
	}
	if row == nil || !row.Hosted || row.HostPID != rec.HostPID || row.Harness != rec.Backend {
		release()
		unlock()
		return nil, false
	}
	return func() { release(); unlock() }, true
}

// lockSummaryOwner pins replyID's summary owner to want: it takes the
// session's summary lock and checks, under the brief map guard, that want
// is still the owner. On success the summary lock stays held until unlock,
// so no newer generation (setSummaryOwner) takes over meanwhile.
//
// The caller holds replyID's registration lock first (lock order: row flock
// -> summary lock -> slot owner lock -> leaf mutexes). Every other holder
// of the summary lock either holds that flock too or holds the summary
// lock only briefly (setSummaryOwner), so a flock holder never waits here
// behind ring I/O.
func (r *Runner) lockSummaryOwner(replyID string, want *conv) (unlock func(), ok bool) {
	l := r.summaryLock(replyID)
	l.Lock()
	if r.summaryOwner(replyID) != want {
		l.Unlock()
		return nil, false
	}
	return l.Unlock, true
}

func (c *conv) summaryStart(ev Event) {
	if !c.rec.Lead || c.rec.Backend != BackendPi || c.live.summarySource != "" || c.summaryGap || ev.Seq == 0 || c.rec.HolderID == "" {
		return
	}
	c.live.summarySource = fmt.Sprintf("%s/%d", c.rec.HolderID, ev.Seq)
}

func (c *conv) summaryProjection(ev Event, quiet bool) {
	if !c.rec.Lead || c.rec.Backend != BackendPi || !c.turn || c.summaryGap || c.live.summarySource == "" || ev.Seq == 0 || ev.ReasoningSummary == nil {
		return
	}
	// No reset/start/end, accounting, or other authority is copied from UI.
	c.apply(wslive.Event{Kind: wslive.KindAgent, ReasoningSummary: ev.ReasoningSummary}, quiet)
}

func (w *liveWriter) withOwner(write func() error) error {
	if w.owner != nil {
		release, ok := w.owner()
		if !ok {
			return fmt.Errorf("hosted live writer ownership changed")
		}
		defer release()
	}
	if owner, ok := w.currentSlot().(wsdashboard.LiveSlotOwner); ok {
		release, owned := owner.LockOwner()
		if !owned {
			return fmt.Errorf("hosted live snapshot slot ownership changed")
		}
		defer release()
	}
	return write()
}

func (w *liveWriter) summaryStore() *wslive.SummaryCommits {
	return wslive.SummaryStore(wslive.RingDir(w.dir))
}

// applySummary freezes a real natural-turn record while the real holder and
// session owner still hold it. It returns ordinary non-summary ring entries;
// retrying a candidate never repeats those tools, replies, or lifecycle counts.
func (w *liveWriter) applySummary(ev wslive.Event) (wslive.Result, error) {
	if w.owner == nil || w.summarySource == "" {
		return wslive.Result{}, fmt.Errorf("summary has no observed hosted turn owner")
	}
	var res wslive.Result
	err := w.withOwner(func() error {
		if ev.Kind != wslive.KindTurnEnd || w.st.Snapshot(w.now().UnixMilli()).Lead.ReasoningSummary == nil {
			if err := w.summaryStore().Available(); err != nil {
				return err
			}
		}
		var err error
		res, err = w.st.Apply(ev, w.now().UnixMilli())
		if err != nil {
			return err
		}
		wslive.PrepareSummaryRing(res.Ring)
		ordinary := res.Ring[:0]
		for _, entry := range res.Ring {
			if entry.SummaryHistory == nil {
				ordinary = append(ordinary, entry)
				continue
			}
			if !w.summaryNaturalEnd {
				continue
			} // A wrapped/idle recovery is not a native settlement.
			entry.SummaryHistory.Commit = wslive.SummaryCommit(w.st.ReplyID(), entry.SummaryHistory.Commit+"/"+w.summarySource)
			if err := w.summaryStore().Register(entry); err != nil {
				return err
			}
		}
		res.Ring = ordinary
		return nil
	})
	return res, err
}

func (w *liveWriter) retrySummaries() {
	n, err := w.summaryStore().Try(wslive.RingDir(w.dir))
	if err != nil {
		fmt.Fprintf(w.log, "agent %s: summary ring retry: %v\n", w.st.ReplyID(), err)
	}
	if n > 0 && w.wake != nil {
		w.wake()
	}
}

// flushSummaries runs on graceful shutdown / slot close and forces a full
// Retry (not Try) so a previously failed pending summary turn is flushed
// best-effort before shutdown.
func (w *liveWriter) flushSummaries() {
	n, err := w.summaryStore().Retry(wslive.RingDir(w.dir))
	if err != nil {
		fmt.Fprintf(w.log, "agent %s: summary ring shutdown retry: %v\n", w.st.ReplyID(), err)
	}
	if n > 0 && w.wake != nil {
		w.wake()
	}
}
