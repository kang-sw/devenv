package wsledger

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
)

// newRemote makes an empty bare repository to serve as the shared remote.
func (w *world) newRemote(name string) string {
	w.t.Helper()
	dir := filepath.Join(w.root, "remotes", name+".git")
	if out, err := exec.Command("git", "init", "--quiet", "--bare", dir).CombinedOutput(); err != nil {
		w.t.Fatalf("init remote: %v: %s", err, out)
	}
	return dir
}

// remoteOpts is a store with its own repository, synced through a remote.
func (w *world) remoteOpts(member, clone string) Options {
	o := w.opts(member, clone)
	o.RepoDir = ""
	return o
}

func (w *world) register(o Options, url string) *Ledger {
	w.t.Helper()
	l, err := Register(context.Background(), o, url)
	if err != nil {
		w.t.Fatalf("register %s.%s: %v", o.Member, o.CloneID, err)
	}
	w.t.Cleanup(func() { l.Close() })
	return l
}

func TestRemoteSmokeRegisterSyncFetch(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	bo := w.register(w.remoteOpts("bo", "c1"), url)

	id := appendOne(t, ada, "note", map[string]int{"n": 1})
	if p := ada.Pending(); p.Count != 1 || p.PushDue.IsZero() {
		t.Fatalf("ada pending before sync = %+v, want one unpushed event with a push deadline", p)
	}
	rep := syncOK(t, ada)
	if !rep.Pushed || rep.Pending.Count != 0 {
		t.Fatalf("ada sync = %+v, want pushed and nothing pending", rep)
	}
	frep, err := bo.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if frep.Pushed {
		t.Fatalf("Fetch pushed")
	}
	got := eventsByID(bo)
	if e, ok := got[id]; !ok || e.Verdict != Valid {
		t.Fatalf("bo sees ada's event = %+v (present %v), want Valid", e, ok)
	}
	ident := bo.Identity()
	if len(ident.Members) != 2 {
		t.Fatalf("bo identity members = %+v, want ada and bo", ident.Members)
	}

	// A second clone of ada with the same key writes no join and no claim.
	ada2 := w.register(w.remoteOpts("ada", "c2"), url)
	if n := len(streamEvents(ada2, "ada.c2")); n != 0 {
		t.Fatalf("second clone wrote %d events, want none", n)
	}
	if e, ok := eventsByID(ada2)[id]; !ok || e.Verdict != Valid {
		t.Fatalf("second clone does not see ada's event as Valid: %+v", e)
	}
}
