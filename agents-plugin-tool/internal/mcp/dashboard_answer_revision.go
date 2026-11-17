package mcp

import (
	"fmt"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// RecordDashboardAnswer resolves the registered lead using the same identity
// checks as dashboard board reads before writing its answer-arrival marker.
func (src *DashboardSessionSource) RecordDashboardAnswer(reg wsdashboard.Registration, id string, at time.Time) error {
	key, _, record, ok, err := src.lookupRecord(reg)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("dashboard answer: session identity unavailable")
	}
	return whiteboardFolder(record.Root, key).RecordDashboardAnswer(id, at)
}
