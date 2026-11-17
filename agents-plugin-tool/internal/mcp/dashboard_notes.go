package mcp

import (
	"fmt"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsnote"
)

// ReadNotes is called only by the drawer's background read, not by a page
// render, live event or timer. It never Ensure's a cache directory.
func (src *DashboardSessionSource) ReadNotes(reg wsdashboard.Registration, refresh bool) ([]wsdashboard.NotesLayer, error) {
	if src.Notes == nil {
		return nil, fmt.Errorf("notes unavailable")
	}
	// Resolution may fail for local stores (for example a caller with no Git
	// on PATH). The repo path remains usable, and never depends on them.
	paths, _ := src.Notes.Paths(reg.ReplyID, reg.Project, refresh)
	layers := make([]wsdashboard.NotesLayer, 0, 4)
	for _, layer := range []wsnote.Layer{wsnote.LayerMachine, wsnote.LayerClone, wsnote.LayerRepo, wsnote.LayerWorktree} {
		out := wsdashboard.NotesLayer{Layer: layer, Notes: []wsnote.Record{}}
		if paths[layer] == "" {
			out.Error = true
		} else {
			notes, err := src.Notes.Read(paths[layer], layer == wsnote.LayerRepo, refresh)
			if err != nil {
				out.Error = true
			} else {
				out.Notes = notes
			}
		}
		layers = append(layers, out)
	}
	return layers, nil
}
