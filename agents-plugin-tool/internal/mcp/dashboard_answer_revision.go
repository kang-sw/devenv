package mcp

import (
	"path/filepath"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// LockBoardIncarnation uses the registered board's in-memory admission gate.
func (src *DashboardSessionSource) LockBoardIncarnation(reg wsdashboard.Registration, token string) (func(), bool) {
	dir := filepath.Join(reg.Project, filepath.FromSlash(whiteboardDirRel), reg.ReplyID)
	return wswhiteboard.LockBoardIncarnation(dir, token)
}

// RecordDashboardAnswer resolves the registered lead using the same identity
// checks as dashboard board reads before writing its answer-arrival marker.
func (src *DashboardSessionSource) RecordDashboardAnswer(reg wsdashboard.Registration, id string, at time.Time) error {
	key, _, record, ok, err := src.lookupRecord(reg)
	if err != nil {
		return err
	}
	if !ok {
		return wsdashboard.ErrDashboardAnswerIdentity
	}
	if src.store.boards == nil {
		return errWhiteboardOwner
	}
	return src.store.boards.use(record.Root, key, nil, true, func(slot *whiteboardSlot) error {
		if slot.board == nil {
			return nil
		}
		return slot.board.RecordDashboardAnswer(id, at)
	})
}
