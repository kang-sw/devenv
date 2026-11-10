package wstrackerhost

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
)

// TestLockWaitBoundsReads: while a store's lock is held (as a fetch or
// push in flight holds it), a read whose context carries a lock wait
// answers ErrStoreBusy, and the same read succeeds once the lock is free;
// a read with no bound, started while the lock is held, keeps waiting
// past the bounded one's answer and completes after the release.
func TestLockWaitBoundsReads(t *testing.T) {
	w := newWorld(t)
	a := w.machine("a")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	ctx := context.Background()

	img := pngBytes("wiki image")
	edit := wsledger.WikiEdit{Wiki: "docs", NewWiki: true, Op: wsledger.WikiOpWrite, Path: "home", Content: "![x](attachment:" + attachmentOID(img) + ")\n"}
	if _, err := a.WikiSave(ctx, "team", WikiSave{Edit: edit, Images: []WikiImageInput{{Data: img}}}); err != nil {
		t.Fatal(err)
	}
	att := stageOK(t, a, "team", "a.png", pngBytes("attachment"))
	if _, err := createWithAttachment(t, a, "team", att); err != nil {
		t.Fatal(err)
	}

	st := a.stores["team"]
	st.mu.Lock()
	unbounded := make(chan error, 1)
	go func() {
		_, err := a.WikiList(ctx, "team")
		unbounded <- err
	}()

	bounded := WithLockWait(ctx, 20*time.Millisecond)
	if _, err := a.WikiList(bounded, "team"); !errors.Is(err, ErrStoreBusy) {
		t.Fatalf("bounded WikiList under a held lock = %v, want ErrStoreBusy", err)
	}
	if _, _, err := a.WikiRead(bounded, "team", "docs", "home", ""); !errors.Is(err, ErrStoreBusy) {
		t.Fatalf("bounded WikiRead under a held lock = %v, want ErrStoreBusy", err)
	}
	if _, err := a.ReadAttachment(bounded, "team", att.OID); !errors.Is(err, ErrStoreBusy) {
		t.Fatalf("bounded ReadAttachment under a held lock = %v, want ErrStoreBusy", err)
	}
	if _, err := a.ReadWikiImage(bounded, "team", "docs", attachmentOID(img)); !errors.Is(err, ErrStoreBusy) {
		t.Fatalf("bounded ReadWikiImage under a held lock = %v, want ErrStoreBusy", err)
	}
	select {
	case err := <-unbounded:
		t.Fatalf("an unbounded read returned while the lock was held: %v", err)
	default:
	}

	// A context that ends first answers its own error, not ErrStoreBusy.
	canceled, cancel := context.WithCancel(WithLockWait(ctx, time.Hour))
	cancel()
	if _, err := a.WikiList(canceled, "team"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled bounded WikiList = %v, want context.Canceled", err)
	}

	st.mu.Unlock()
	if err := <-unbounded; err != nil {
		t.Fatalf("unbounded WikiList after the release = %v", err)
	}
	if list, err := a.WikiList(bounded, "team"); err != nil || len(list) != 1 {
		t.Fatalf("bounded WikiList after the release = %+v, %v", list, err)
	}
	if data, err := a.ReadAttachment(bounded, "team", att.OID); err != nil || !bytes.Equal(data, pngBytes("attachment")) {
		t.Fatalf("bounded ReadAttachment after the release = %d bytes, %v", len(data), err)
	}
	if data, err := a.ReadWikiImage(bounded, "team", "docs", attachmentOID(img)); err != nil || !bytes.Equal(data, img) {
		t.Fatalf("bounded ReadWikiImage after the release = %d bytes, %v", len(data), err)
	}
}
