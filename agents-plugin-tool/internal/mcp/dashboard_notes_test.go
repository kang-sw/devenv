package mcp

import (
	"os"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsnote"
	"github.com/kang-sw/devenv/internal/wsstate"
)

func TestDashboardNotesDaemonWritesAndExplicitRefresh(t *testing.T) {
	setupNoteTestEnv(t)
	cache := wsnote.NewCache()
	s := NewServerWithOptions(ServerOptions{Root: t.TempDir(), Version: "test", Notes: cache})
	root, key := mintRootKey(t, s, 1)
	src := NewDashboardSessionSource()
	src.Notes = cache
	reg := wsdashboard.Registration{Project: root}
	read := func(refresh bool) map[wsnote.Layer][]wsnote.Record {
		t.Helper()
		layers, err := src.ReadNotes(reg, refresh)
		if err != nil {
			t.Fatal(err)
		}
		want := []wsnote.Layer{wsnote.LayerMachine, wsnote.LayerClone, wsnote.LayerRepo, wsnote.LayerWorktree}
		out := map[wsnote.Layer][]wsnote.Record{}
		for i, layer := range layers {
			if layer.Layer != want[i] {
				t.Fatalf("layer order = %+v", layers)
			}
			out[layer.Layer] = layer.Notes
		}
		if len(layers) != 4 {
			t.Fatalf("layers = %+v", layers)
		}
		return out
	}
	read(false) // warm every store before the daemon writes
	paths, err := wsnote.ReadPaths(root, wsconfig.Options{}, wsstate.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, layer := range allNoteLayers {
		callToolWithKey(t, s, 2, key, "note.write", map[string]any{
			"layer": string(layer), "reason": "other projects need this fixture", "notes": []any{map[string]any{"key": "written", "value": "daemon", "priority": 3}},
		})
	}
	callToolWithKey(t, s, 3, key, "note.mute", map[string]any{"layer": "worktree", "keys": []any{"written"}})
	// A later open must not reread any local layer file. Invalid bytes make
	// such a reread observable without relying on stat-cache timing.
	for _, layer := range []wsnote.Layer{wsnote.LayerMachine, wsnote.LayerClone, wsnote.LayerWorktree} {
		if err := os.WriteFile(paths[layer], []byte("invalid external bytes"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rows := read(false)
	for _, layer := range allNoteLayers {
		if len(rows[layer]) != 1 || rows[layer][0].Value != "daemon" {
			t.Fatalf("cache lost daemon write: %s %+v", layer, rows[layer])
		}
	}
	if rows[wsnote.LayerWorktree][0].Visible {
		t.Fatal("daemon mute did not publish")
	}
	if _, err := src.ReadNotes(reg, true); err == nil {
		t.Fatal("explicit refresh did not reread invalid layer file")
	}
	for _, layer := range allNoteLayers {
		if layer != wsnote.LayerRepo {
			if err := os.Remove(paths[layer]); err != nil {
				t.Fatal(err)
			}
		}
		var err error
		records := []wsnote.Record{{Key: "written", Value: "external", Priority: 5}}
		if layer == wsnote.LayerRepo {
			err = wsnote.RepoWrite(paths[layer], records)
		} else {
			err = wsnote.Write(paths[layer], records)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	rows = read(false)
	if rows[wsnote.LayerMachine][0].Value != "daemon" || rows[wsnote.LayerRepo][0].Value != "external" {
		t.Fatalf("open invalidation policy = %+v", rows)
	}
	rows = read(true)
	for _, layer := range allNoteLayers {
		if len(rows[layer]) != 1 || rows[layer][0].Value != "external" {
			t.Fatalf("refresh missed %s: %+v", layer, rows[layer])
		}
	}
}
