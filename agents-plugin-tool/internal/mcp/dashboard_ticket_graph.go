package mcp

import (
	"errors"
	"os"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsdoc"
)

var _ wsdashboard.TicketGraphSource = (*DashboardSessionSource)(nil)

// ticketMapJSON is the board read, replaceable so tests can observe the root
// it is asked for.
var ticketMapJSON = wsdoc.TicketMapJSON

// ReadTicketGraph is the ticket map's demand-only read. A board with no
// tickets directory is served as an empty map, not an error.
func (src *DashboardSessionSource) ReadTicketGraph(reg wsdashboard.Registration) ([]byte, bool, error) {
	_, _, record, ok, err := src.lookupRecord(reg)
	if !ok || err != nil {
		return nil, false, err
	}
	payload, err := ticketMapJSON(record.Root)
	if errors.Is(err, os.ErrNotExist) {
		payload, err = wsdoc.EmptyTicketMapJSON()
	}
	if err != nil {
		return nil, true, err
	}
	return payload, true, nil
}
