package wstrackerhost

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
)

// TestWikiSaveReadAcrossMachines: a write with a pasted image on one
// machine is readable on another after its check; the image is typed by
// its bytes and bounded by the store's limit; a refused save is not
// recorded as a sync error while a landed one is recorded as a sync.
func TestWikiSaveReadAcrossMachines(t *testing.T) {
	w := newWorld(t)
	a, b := w.machine("a"), w.machine("b")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	w.register(b, "team", "bo", &wstracker.Profile{Given: "Bo"})
	ctx := context.Background()

	img := pngBytes("pasted")
	edit := wsledger.WikiEdit{Wiki: "docs", NewWiki: true, Op: wsledger.WikiOpWrite, Path: "home", Content: "![x](attachment:" + attachmentOID(img) + ")\n"}
	if _, err := a.WikiSave(ctx, "team", WikiSave{Edit: edit, Images: []WikiImageInput{{Data: []byte("not an image")}}}); !errors.Is(err, ErrNotImage) {
		t.Fatalf("non-image = %v", err)
	}
	huge := pngBytes(string(make([]byte, wstracker.DefaultAttachmentMax)))
	if _, err := a.WikiSave(ctx, "team", WikiSave{Edit: edit, Images: []WikiImageInput{{Data: huge}}}); !errors.Is(err, ErrAttachmentTooLarge) {
		t.Fatalf("oversize = %v", err)
	}
	res, err := a.WikiSave(ctx, "team", WikiSave{Edit: edit, Images: []WikiImageInput{{Data: img}}})
	if err != nil || !res.Created {
		t.Fatalf("save = %+v, %v", res, err)
	}
	if s := readSyncState(a.stores["team"].entry.Dir); s.SyncedAt.IsZero() || s.Error != "" {
		t.Fatalf("sync state after a landed save = %+v", s)
	}
	// A refused save: the base is stale.
	if _, err := a.WikiSave(ctx, "team", WikiSave{Edit: wsledger.WikiEdit{Wiki: "docs", NewWiki: true, Op: wsledger.WikiOpWrite, Path: "home", Content: "x"}}); !errors.Is(err, wsledger.ErrWikiExists) {
		t.Fatalf("new_wiki twice = %v", err)
	}
	if s := readSyncState(a.stores["team"].entry.Dir); s.Error != "" {
		t.Fatalf("a refusal was recorded as a sync error: %+v", s)
	}

	if _, err := b.Check(ctx, "team"); err != nil {
		t.Fatal(err)
	}
	content, at, err := b.WikiRead(ctx, "team", "docs", "home", "")
	if err != nil || content != edit.Content || at != res.Commit {
		t.Fatalf("bo reads %q at %s, %v", content, at, err)
	}
	data, err := b.ReadWikiImage(ctx, "team", "docs", attachmentOID(img))
	if err != nil || !bytes.Equal(data, img) {
		t.Fatalf("bo reads image: %d bytes, %v", len(data), err)
	}
	if _, err := b.ReadWikiImage(ctx, "team", "docs", attachmentOID([]byte("other"))); !errors.Is(err, ErrAttachmentNotFound) {
		t.Fatalf("missing image = %v", err)
	}
	if _, err := b.ReadWikiImage(ctx, "team", "nope", attachmentOID(img)); !errors.Is(err, ErrAttachmentNotFound) {
		t.Fatalf("missing wiki image = %v", err)
	}
	list, err := b.WikiList(ctx, "team")
	if err != nil || len(list) != 1 || list[0].Name != "docs" {
		t.Fatalf("list = %+v, %v", list, err)
	}
	pages, _, err := b.WikiPages(ctx, "team", "docs", "")
	if err != nil || len(pages) != 1 || pages[0] != "home" {
		t.Fatalf("pages = %v, %v", pages, err)
	}
	hits, _, err := b.WikiSearch(ctx, "team", "docs", "attachment:")
	if err != nil || len(hits) != 1 {
		t.Fatalf("search = %+v, %v", hits, err)
	}
	hist, err := b.WikiHistory(ctx, "team", "docs", "", 0)
	if err != nil || len(hist) != 1 || !hist[0].Verified || hist[0].Signer != "ada" {
		t.Fatalf("history = %+v, %v", hist, err)
	}
	// bo edits on the base it read; ada sees it after a check.
	res2, err := b.WikiSave(ctx, "team", WikiSave{Edit: wsledger.WikiEdit{Wiki: "docs", Base: at, Op: wsledger.WikiOpWrite, Path: "home", Content: edit.Content + "\nmore\n"}})
	if err != nil {
		t.Fatal(err)
	}
	patch, err := b.WikiDiff(ctx, "team", "docs", "home", "", res2.Commit)
	if err != nil || !bytes.Contains([]byte(patch), []byte("+more")) {
		t.Fatalf("diff = %q, %v", patch, err)
	}
	if _, err := a.Check(ctx, "team"); err != nil {
		t.Fatal(err)
	}
	if content, _, _ := a.WikiRead(ctx, "team", "docs", "home", ""); content != edit.Content+"\nmore\n" {
		t.Fatalf("ada reads %q", content)
	}
	if points, err := a.WikiRecovery(ctx, "team", ""); err != nil || len(points) != 0 {
		t.Fatalf("recovery = %+v, %v", points, err)
	}
}

// TestWikiSaveStagedImages: a save names images uploaded earlier through
// StageAttachment by oid; an oid with no staged copy that the wiki does
// not hold is refused as the saver's problem, an oid the wiki already
// holds needs no copy, the staged copy outlives the save (the sweep takes
// it), and WikiImages lists the tip's images, none for an absent wiki.
func TestWikiSaveStagedImages(t *testing.T) {
	w := newWorld(t)
	a, b := w.machine("a"), w.machine("b")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	w.register(b, "team", "bo", &wstracker.Profile{Given: "Bo"})
	ctx := context.Background()

	img := pngBytes("held")
	att := stageOK(t, a, "team", "held.png", img)
	ghost := attachmentOID([]byte("never uploaded"))
	edit := wsledger.WikiEdit{Wiki: "docs", NewWiki: true, Op: wsledger.WikiOpWrite, Path: "home", Content: "![x](attachment:" + att.OID + ")\n"}
	if oids, err := a.WikiImages(ctx, "team", "docs"); err != nil || oids != nil {
		t.Fatalf("images of an absent wiki = %v, %v", oids, err)
	}
	_, err := a.WikiSave(ctx, "team", WikiSave{Edit: edit, Staged: []string{att.OID, ghost}})
	if !errors.Is(err, ErrAttachmentNotFound) || !IsWikiRefusal(err) {
		t.Fatalf("save with an expired image = %v, want a refusal", err)
	}
	if s := readSyncState(a.stores["team"].entry.Dir); s.Error != "" {
		t.Fatalf("a refusal was recorded as a sync error: %+v", s)
	}
	res, err := a.WikiSave(ctx, "team", WikiSave{Edit: edit, Staged: []string{att.OID}})
	if err != nil || !res.Created {
		t.Fatalf("save = %+v, %v", res, err)
	}
	if _, ok := readStaged(a.stores["team"].entry.Dir, att.OID); !ok {
		t.Fatal("the staged copy was dropped by the save")
	}
	if oids, err := a.WikiImages(ctx, "team", "docs"); err != nil || len(oids) != 1 || oids[0] != att.OID {
		t.Fatalf("images = %v, %v", oids, err)
	}
	if _, err := b.Check(ctx, "team"); err != nil {
		t.Fatal(err)
	}
	if data, err := b.ReadWikiImage(ctx, "team", "docs", att.OID); err != nil || !bytes.Equal(data, img) {
		t.Fatalf("bo reads image: %d bytes, %v", len(data), err)
	}
	// bo names the image the wiki holds without a staged copy of its own.
	_, at, err := b.WikiRead(ctx, "team", "docs", "home", "")
	if err != nil {
		t.Fatal(err)
	}
	res2, err := b.WikiSave(ctx, "team", WikiSave{Edit: wsledger.WikiEdit{Wiki: "docs", Base: at, Op: wsledger.WikiOpWrite, Path: "home", Content: edit.Content + "again\n"}, Staged: []string{att.OID}})
	if err != nil || res2.Commit == "" {
		t.Fatalf("save naming a held image = %+v, %v", res2, err)
	}
	if oids, err := b.WikiImages(ctx, "team", "docs"); err != nil || len(oids) != 1 {
		t.Fatalf("images after = %v, %v", oids, err)
	}
}
