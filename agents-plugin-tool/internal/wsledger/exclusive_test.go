package wsledger

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

// TestOpenDoesNotCreateTheStoreDirectory: opening a store whose directory
// is gone fails and leaves nothing where it was.
func TestOpenDoesNotCreateTheStoreDirectory(t *testing.T) {
	w := newWorld(t)
	o := w.remoteOpts("ada", "c1")
	l := w.register(o, w.newRemote("r"))
	l.Close()
	if err := os.RemoveAll(o.Dir); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), o); err == nil {
		t.Fatal("opening a removed store must fail")
	}
	if _, err := os.Stat(o.Dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the open recreated the store directory: %v", err)
	}
}

// TestExclusive: fn runs under the store lock with the clone's unpushed
// state as the directory holds it, another process's append included; a
// lock held elsewhere is ErrBusy and fn does not run; and a call after fn
// renamed the directory away fails without recreating it. Windows refuses
// to rename a directory with the lock file open in it, so there the rename
// follows Exclusive, as wstrackerhost's Unregister does.
func TestExclusive(t *testing.T) {
	w := newWorld(t)
	o := w.remoteOpts("ada", "c1")
	l := w.register(o, w.newRemote("r"))
	syncOK(t, l)
	ctx := context.Background()

	var got Unpushed
	read := func(u Unpushed) error { got = u; return nil }
	if err := l.Exclusive(ctx, time.Second, read); err != nil || got.Pending.Count != 0 || len(got.Wikis) != 0 {
		t.Fatalf("synced = %+v, %v", got, err)
	}
	// Another process on the same directory appends.
	twin, err := Open(ctx, o)
	if err != nil {
		t.Fatal(err)
	}
	defer twin.Close()
	appendOne(t, twin, "note", note("n1"))
	if err := l.Exclusive(ctx, time.Second, read); err != nil || got.Pending.Count != 1 {
		t.Fatalf("after another process's append = %+v, %v", got, err)
	}
	if _, err := l.WikiSave(ctx, WikiEdit{Wiki: "docs", NewWiki: true, Op: WikiOpWrite, Path: "home", Content: "x\n"}); err != nil {
		t.Fatal(err)
	}
	if err := l.git.deleteRef(ctx, knownRef(wikiRef("docs")), refTip(t, l, knownRef(wikiRef("docs")))); err != nil {
		t.Fatal(err)
	}
	if err := l.Exclusive(ctx, time.Second, read); err != nil || len(got.Wikis) != 1 || got.Wikis[0] != "docs" {
		t.Fatalf("with a wiki the remote is not known to hold = %+v, %v", got, err)
	}

	fl := flock.New(filepath.Join(o.Dir, LockFile))
	if locked, err := fl.TryLock(); err != nil || !locked {
		t.Fatalf("lock: %v %v", locked, err)
	}
	ran := false
	err = l.Exclusive(ctx, 60*time.Millisecond, func(Unpushed) error { ran = true; return nil })
	if !errors.Is(err, ErrBusy) || ran {
		t.Fatalf("with the lock held elsewhere = %v (ran: %v), want ErrBusy", err, ran)
	}
	fl.Unlock()

	aside := o.Dir + ".removed"
	if runtime.GOOS == "windows" {
		if err := l.Exclusive(ctx, time.Second, func(Unpushed) error { return nil }); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(o.Dir, aside); err != nil {
			t.Fatalf("renaming the directory away after the lock: %v", err)
		}
	} else if err := l.Exclusive(ctx, time.Second, func(Unpushed) error { return os.Rename(o.Dir, aside) }); err != nil {
		t.Fatalf("renaming the directory away under the lock: %v", err)
	}
	if _, err := l.Sync(ctx); err == nil {
		t.Fatal("a sync of a removed store must fail")
	}
	if _, err := twin.Append(ctx, []NewEvent{{Type: "note", Payload: []byte(`{"n":"late"}`)}}); err == nil {
		t.Fatal("an append to a removed store must fail")
	}
	if _, err := os.Stat(o.Dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a late call recreated the store directory: %v", err)
	}
}
