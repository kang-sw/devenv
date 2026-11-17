package mcp

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsdoc"
)

var _ wsdashboard.TicketIdeasSource = (*DashboardSessionSource)(nil)

// ReadTicketIdeas is the drawer's demand-only ticket read. The live panel
// deliberately never lists or watches idea/.
func (src *DashboardSessionSource) ReadTicketIdeas(reg wsdashboard.Registration) ([]wsdashboard.IdeaTicket, bool, error) {
	_, _, record, ok, err := src.lookupRecord(reg)
	if !ok || err != nil {
		return nil, false, err
	}
	tickets, err := whiteboardTicketsList(record.Root, wsdoc.TicketListOptions{Statuses: []string{"idea"}})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, true, err
	}
	out := make([]wsdashboard.IdeaTicket, 0, len(tickets))
	for _, t := range tickets {
		out = append(out, wsdashboard.IdeaTicket{Category: ticketCategory(t.Stem), Title: t.Title, Stem: t.Stem, Path: filepath.ToSlash(t.Path), Abs: filepath.Join(record.Root, filepath.FromSlash(t.Path))})
	}
	return out, true, nil
}
