package mcp

import (
	"fmt"
	"path/filepath"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
)

func (s *Server) isTerminalRowOwned(row *wsdashboard.Registration) bool {
	return row != nil && !row.Hosted && !row.Agent && row.EndedAt == "" && row.HarnessPID == s.harnessPID() && row.MCPOwnerPID == s.ownerPID
}

func (s *Server) terminalRowOwned(dir, replyID string) bool {
	unlock, row, err := wsdashboard.LockRegistrationMemory(dir, replyID)
	if err != nil {
		return false
	}
	owned := s.isTerminalRowOwned(row)
	unlock()
	return owned
}

func (s *Server) summaryCapable(ls *liveSession) bool {
	_, owned := ls.slot.(wsdashboard.LiveSlotOwner)
	return s.intake != nil && owned && s.ownerPID > 0 && s.harnessPID() > 0 && s.currentHarness() == "pi" && s.hostedSessionKey() == ""
}

// lockSummaryOwner fences both the actual daemon slot generation and the
// terminal/hosted registration handoff, under the same in-process fence
// takeover uses without per-write OS flocks or disk reads.
func (s *Server) lockSummaryOwner(ls *liveSession) (func(), error) {
	owner, ok := ls.slot.(wsdashboard.LiveSlotOwner)
	if !ok {
		return nil, fmt.Errorf("reasoning summary receiver has no daemon ownership")
	}
	dir := filepath.Dir(filepath.Dir(ls.dir))
	unlock, row, err := wsdashboard.LockRegistrationMemory(dir, ls.replyID)
	if err != nil {
		return nil, err
	}
	if !s.isTerminalRowOwned(row) {
		unlock()
		return nil, fmt.Errorf("reasoning summary terminal ownership changed")
	}
	release, ok := owner.LockOwner()
	if !ok {
		unlock()
		return nil, fmt.Errorf("reasoning summary connection generation retired")
	}
	return func() { release(); unlock() }, nil
}

func (s *Server) publishSummaryOwned(ls *liveSession, snap wslive.Snapshot) {
	dir := filepath.Dir(filepath.Dir(ls.dir))
	unlock, row, err := wsdashboard.LockRegistrationMemory(dir, ls.replyID)
	if err != nil {
		return
	}
	if !s.isTerminalRowOwned(row) {
		unlock()
		ls.stop()
		return
	}
	// Publish checks its actual slot generation itself; keep the registration
	// fence through that check, without recursively taking the slot mutex.
	ls.slot.Publish(snap)
	unlock()
}

func (ls *liveSession) withSlotOwner(write func() error) error {
	if ls.owner != nil {
		release, err := ls.owner()
		if err != nil {
			return err
		}
		defer release()
		return write()
	}
	if owner, ok := ls.slot.(wsdashboard.LiveSlotOwner); ok {
		release, owned := owner.LockOwner()
		if !owned {
			return fmt.Errorf("live connection generation retired")
		}
		defer release()
	}
	return write()
}

func (ls *liveSession) summaryStore() *wslive.SummaryCommits {
	return wslive.SummaryStore(wslive.RingDir(ls.dir))
}

// retrySummaryCommits is receiver-owned recovery: callbacks run at the existing
// heartbeat cadence and never replay an intake batch. Accepted pending candidates
// survive connection/slot changes because the store is session-scoped RAM.
func (ls *liveSession) retrySummaryCommits() {
	if ls.slot == nil {
		return
	}
	n, err := ls.summaryStore().Try(wslive.RingDir(ls.dir))
	if err != nil {
		ls.debug("dashboard.summary_write_error", map[string]any{"error": err.Error()})
	}
	if n > 0 {
		ls.slot.RingAppended()
	}
}
