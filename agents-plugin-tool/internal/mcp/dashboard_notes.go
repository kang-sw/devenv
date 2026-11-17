package mcp

import (
	"fmt"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsnote"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// ReadNotes is called only by the drawer's background read, not by a page
// render, live event or timer. It never Ensure's a cache directory.
func (src *DashboardSessionSource) ReadNotes(reg wsdashboard.Registration, refresh bool) ([]wsdashboard.NotesLayer, error) {
	if src.Notes == nil {
		return nil, fmt.Errorf("notes unavailable")
	}
	cached, ok := src.notePaths.Load(reg.Project)
	if !ok {
		paths, err := wsnote.ReadPaths(reg.Project, wsconfig.Options{}, wsstate.Options{})
		if err != nil {
			return nil, err
		}
		cached, _ = src.notePaths.LoadOrStore(reg.Project, paths)
	}
	paths := cached.(map[wsnote.Layer]string)
	layers := make([]wsdashboard.NotesLayer, 0, 4)
	for _, layer := range []wsnote.Layer{wsnote.LayerMachine, wsnote.LayerClone, wsnote.LayerRepo, wsnote.LayerWorktree} {
		notes, err := src.Notes.Read(paths[layer], layer == wsnote.LayerRepo, refresh)
		if err != nil {
			return nil, err
		}
		layers = append(layers, wsdashboard.NotesLayer{Layer: layer, Notes: notes})
	}
	return layers, nil
}
