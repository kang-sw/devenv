// daemon_runtime.go holds daemon change hooks, polling, and shutdown.
package wsdashboard

import (
	"context"
	"fmt"
	"github.com/kang-sw/devenv/internal/wslive"
	"path/filepath"
	"time"
)

// sessionChanged is the index's change hook: it wakes the streams whose
// page shows the session, and drops a session's cached conversation entries,
// entry files, and snapshot, and the source's caches of it
// (SessionForgetter), once it left the index.
func (d *Daemon) sessionChanged(replyID string) {
	if d.index != nil {
		if session, ok := d.index.Session(replyID); ok {
			d.scheduleAnswerReplay(session.Registration)
			if session.Harness == "claude" {
				d.planUsage.pushClaude(d.snapshots.snapshotWithoutObserved(d.dir, replyID))
			}
		} else {
			d.entries.drop(replyID)
			d.entryFiles.drop(replyID)
			d.railMsgs.drop(replyID)
			d.live.drop(replyID)
			d.snapshots.Forget(d.dir, replyID)
			wslive.DropRing(wslive.RingDir(SessionDir(d.dir, replyID)))
			if f, ok := d.source.(SessionForgetter); ok {
				f.ForgetSession(replyID)
			}
		}
	}
	d.stats.WakeSession.Add(1)
	d.broker.wake(replyID)
	d.acks.poke(replyID)
	d.pokeHires()
}

// agentChanged is the agent runner's write hook, called from the
// conversation's loop for every write: it schedules a re-read of the one
// conversation, as a ws-mcp's nudge does for a lead session, and returns
// without waiting for it (Index.Reread). The page wakes at once, since its
// stream reads the conversation's own live files; the re-read's change, if
// any, wakes it again when it lands.
func (d *Daemon) agentChanged(replyID, _ string) {
	if d.index != nil {
		// The runner changes live stores, not ws-mcp's record/board/mailbox.
		d.index.RereadLive(replyID)
	}
	d.stats.WakeAgent.Add(1)
	d.broker.wake(replyID)
	d.acks.poke(replyID)
}

// AgyActivity schedules a usage refresh after a hosted agy turn settles.
// The handoff must not wait for the plan-usage store's lock.
func (d *Daemon) AgyActivity() {
	d.agyActivity(nil)
}

// agyActivity exposes completion of the lock-taking handoff to package tests.
func (d *Daemon) agyActivity(done chan<- struct{}) {
	if d.planUsage == nil {
		return
	}
	go func() {
		d.planUsage.refreshAgy()
		if done != nil {
			close(done)
		}
	}()
}

// poll fills the session index at once and then re-reads every registered
// session at the external safety interval. Clock-only checks retain the old
// cadence, with targeted reads only when known liveness evidence expires.
func (d *Daemon) poll(interval time.Duration) {
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	d.index.Refresh()
	d.pokeHires()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	clock := time.NewTicker(min(indexClockInterval, interval))
	defer clock.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			d.index.Refresh()
			d.pokeHires()
		case <-clock.C:
			d.index.RefreshClock()
		}
	}
}

// Index returns the daemon's session index; nil when it runs without a
// session source.
func (d *Daemon) Index() *Index { return d.index }

// Endpoint returns what the daemon wrote to its endpoint file.
func (d *Daemon) Endpoint() Endpoint { return d.endpoint }

// Done is closed once the daemon has stopped serving, removed its endpoint
// file, and released the lock.
func (d *Daemon) Done() <-chan struct{} { return d.done }

// Close stops the daemon. The endpoint file is removed before the lock is
// released, so a successor that takes the lock never has its own file
// deleted. Close is idempotent. A hosted call that outlives the drain
// budget is still running when Close returns (drainProxy); WaitProxy waits
// for it.
func (d *Daemon) Close() {
	d.closeOnce.Do(func() {
		close(d.stop)
		if d.hireDone != nil {
			<-d.hireDone
		}
		if d.leadNudges != nil {
			d.leadNudges.Close()
		}
		d.acks.close()
		// Shutdown closes the listener at once, so no new session or page
		// connects, and the hosted sessions drain meanwhile (proxy.go):
		// the whole stop stays under the successor's lock wait.
		ctx, cancel := context.WithTimeout(context.Background(), shutdownDeadline)
		if d.planUsage != nil {
			d.planUsage.beginClose()
		}
		shutdownDone := make(chan struct{})
		go func() {
			_ = d.server.Shutdown(ctx)
			close(shutdownDone)
		}()
		d.drainProxy()
		<-shutdownDone
		if d.planUsage != nil {
			d.planUsage.waitClose(ctx)
		}
		cancel()
		// Shutdown closes only the listeners Serve has registered, and Serve
		// runs on its own goroutine: a Close before that goroutine is
		// scheduled would leave the port bound past Close, so a restart could
		// not rebind it. Closing it here makes Close release the port.
		_ = d.listener.Close()
		// The web card listeners are servers of their own; closed here,
		// before the lock goes, a successor never races a leftover port.
		d.web.close()
		if d.agents != nil {
			d.agents.Close()
		}
		// Detaches only: the holders and their children outlive the daemon.
		if d.holders != nil {
			d.holders.Close()
		}
		d.foreign.close()
		_ = wslive.UnpinRingsUnder(filepath.Join(d.dir, sessionsDirName))
		if d.beforeUnlock != nil {
			d.beforeUnlock()
		}
		removeEndpointIfOwned(d.dir, d.endpoint.Token)
		_ = d.lock.Unlock()
		fmt.Fprintf(d.log, "dashboard: stopped (pid %d)\n", d.endpoint.PID)
		close(d.done)
	})
}
