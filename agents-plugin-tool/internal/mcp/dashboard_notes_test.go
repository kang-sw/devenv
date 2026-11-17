package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsnote"
	"github.com/kang-sw/devenv/internal/wsstate"
)

func TestDashboardNotesUsesHostedCallerStoreIdentity(t *testing.T) {
	setupNoteTestEnv(t)
	cache := wsnote.NewCache()
	env := map[string]string{}
	for _, item := range os.Environ() {
		key, value, _ := strings.Cut(item, "=")
		env[key] = value
	}
	env[wsstate.EnvCacheHome] = filepath.Join(t.TempDir(), "caller-cache")
	env[wsconfig.EnvConfigHome] = filepath.Join(t.TempDir(), "caller-config")
	s := NewServerWithOptions(ServerOptions{Root: t.TempDir(), Version: "test", Env: EnvFromMap(env), Notes: cache})
	root, key := mintRootKey(t, s, 1)
	caller, err := wsnote.ReadPaths(root, s.configOptions(), s.stateOptions())
	if err != nil {
		t.Fatal(err)
	}
	daemon, err := wsnote.ReadPaths(root, wsconfig.Options{}, wsstate.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, layer := range []wsnote.Layer{wsnote.LayerMachine, wsnote.LayerClone, wsnote.LayerWorktree} {
		if caller[layer] == daemon[layer] {
			t.Fatalf("fixture did not isolate %s", layer)
		}
		if err := wsnote.Write(caller[layer], []wsnote.Record{{Key: "caller", Value: string(layer)}}); err != nil {
			t.Fatal(err)
		}
		if err := wsnote.Write(daemon[layer], []wsnote.Record{{Key: "daemon", Value: "wrong store"}}); err != nil {
			t.Fatal(err)
		}
	}
	// A normal authenticated keyed call binds the per-connection context in
	// memory, even when the dashboard manifest is not loaded in this fixture.
	callToolWithKey(t, s, 2, key, "note.query", map[string]any{"layer": "machine"})
	src := NewDashboardSessionSource()
	src.Notes = cache
	reg := wsdashboard.Registration{Project: root, ReplyID: keyReplyID(key)}
	layers, err := src.ReadNotes(reg, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, layer := range layers {
		if layer.Layer == wsnote.LayerRepo {
			continue
		}
		if layer.Error || len(layer.Notes) != 1 || layer.Notes[0].Key != "caller" {
			t.Fatalf("drawer used daemon context: %+v", layers)
		}
	}
	// The same root in another session may intentionally use other stores.
	other, err := src.ReadNotes(wsdashboard.Registration{Project: root, ReplyID: "other-session"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(other[0].Notes) != 1 || other[0].Notes[0].Key != "daemon" {
		t.Fatalf("session identities merged: %+v", other)
	}
}

func TestDashboardNotesLocalFailureDoesNotSuppressRepo(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer(t.TempDir(), "test")
	root, key := mintRootKey(t, s, 1)
	paths, err := wsnote.ReadPaths(root, wsconfig.Options{}, wsstate.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths[wsnote.LayerMachine]), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths[wsnote.LayerMachine], []byte("bad-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := wsnote.RepoWrite(paths[wsnote.LayerRepo], []wsnote.Record{{Key: "handoff", Value: "LAN handoff"}}); err != nil {
		t.Fatal(err)
	}
	src := NewDashboardSessionSource()
	layers, err := src.ReadNotes(wsdashboard.Registration{Project: root, ReplyID: keyReplyID(key)}, false)
	if err != nil || len(layers) != 4 || !layers[0].Error || layers[2].Error || len(layers[2].Notes) != 1 || layers[2].Notes[0].Key != "handoff" {
		t.Fatalf("local error suppressed repo: %+v %v", layers, err)
	}
}

func TestDashboardNotesDaemonWritesAndExplicitRefresh(t *testing.T) {
	setupNoteTestEnv(t)
	cache := wsnote.NewCache()
	s := NewServerWithOptions(ServerOptions{Root: t.TempDir(), Version: "test", Notes: cache})
	root, key := mintRootKey(t, s, 1)
	src := NewDashboardSessionSource()
	src.Notes = cache
	reg := wsdashboard.Registration{Project: root, ReplyID: keyReplyID(key)}
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
	refreshed, err := src.ReadNotes(reg, true)
	if err != nil || len(refreshed) != 4 || !refreshed[0].Error || refreshed[2].Error || len(refreshed[2].Notes) != 1 {
		t.Fatalf("explicit refresh must report local errors and retain repo: %+v %v", refreshed, err)
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
