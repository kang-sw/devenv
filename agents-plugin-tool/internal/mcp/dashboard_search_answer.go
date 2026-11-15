package mcp

import (
	"encoding/json"
	"os"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// SearchAnswerSnapshot reads only a top-level session's authoritative board.
// lookupRecord opens/replays the folder store under its lock, rebuilding the
// immutable log and every retained panel version, not the capped page render.
// Its returned state is independent of subsequent writes; keys never leave
// this boundary. A session with no board gets JSON null, not a new board.
func (src *DashboardSessionSource) SearchAnswerSnapshot(reg wsdashboard.Registration) (string, []byte, error) {
	if reg.Agent {
		return "", nil, os.ErrNotExist
	}
	_, _, record, ok, err := src.lookupRecord(reg)
	if err != nil {
		return "", nil, err
	}
	if !ok {
		return "", nil, os.ErrNotExist
	}
	data, err := json.Marshal(record.Whiteboard)
	return record.Root, data, err
}
