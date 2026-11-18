package mcp

import (
	"path/filepath"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// LockBoardIncarnation uses trusted index identity and the folder's live gate,
// not lookupRecord (which would add file reads to interactive admission).
func (src *DashboardSessionSource) LockBoardIncarnation(reg wsdashboard.Registration, token string) (func(), bool) {
	folder := wswhiteboard.FolderStore{Dir: filepath.Join(reg.Project, filepath.FromSlash(whiteboardDirRel), reg.ReplyID)}
	return folder.LockIncarnation(token)
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
	return whiteboardFolder(record.Root, key).RecordDashboardAnswer(id, at)
}
