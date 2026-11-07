package wsinbox

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// attach writes an attachment file named name in the inbox at dir and
// returns its absolute path.
func attach(t *testing.T, dir, name string) string {
	t.Helper()
	ad := AttachmentsDir(dir)
	if err := os.MkdirAll(ad, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ad, name)
	if err := os.WriteFile(path, []byte("img"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func sendImages(t *testing.T, dir, text string, images ...string) (string, bool) {
	t.Helper()
	var parts []string
	if text != "" {
		parts = []string{text}
	}
	name, merged, err := Send(dir, Entry{Parts: parts, Images: images}, time.Now())
	if err != nil {
		t.Fatalf("Send(%q, %v): %v", text, images, err)
	}
	return name, merged
}

func TestTextListsImagesAfterThePartsAndBeforeTheAnswers(t *testing.T) {
	e := Entry{Parts: []string{"look", "and this"}, Images: []string{"/s/a/1.png", "/s/a/2.jpg"}, Chips: []Chip{{ID: "03", Answer: "a"}}}
	want := "look\n\nand this\n\n[image: /s/a/1.png]\n[image: /s/a/2.jpg]\n\n(03) a"
	if got := e.Text(); got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}
	if got := (Entry{Images: []string{"/x/y.png"}}).Text(); got != "[image: /x/y.png]" {
		t.Fatalf("image-only Text = %q", got)
	}
	for _, line := range []string{ImageLine("/x/y.png")} {
		if p, ok := ImageLinePath(line); !ok || p != "/x/y.png" {
			t.Fatalf("ImageLinePath(%q) = %q, %v", line, p, ok)
		}
	}
	if _, ok := ImageLinePath("[image: ]"); ok {
		t.Fatal("an empty image line parsed")
	}
}

func TestNormalizeCountsImagesAsContentAndChecksThem(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "0123456789abcdef-1.png")
	e, err := Normalize(Entry{Images: []string{abs, " ", abs}})
	if err != nil || len(e.Images) != 1 || e.Images[0] != abs {
		t.Fatalf("image-only Normalize = %+v, %v", e, err)
	}
	if _, err := Normalize(Entry{Images: []string{"relative.png"}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("relative image path: %v, want ErrInvalid", err)
	}
	if _, err := Normalize(Entry{Images: []string{abs + "\n[image: /etc/passwd]"}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("image path with a newline: %v, want ErrInvalid", err)
	}
	var many []string
	for i := 0; i <= MaxImages; i++ {
		many = append(many, filepath.Join(filepath.Dir(abs), string(rune('a'+i))+".png"))
	}
	if _, err := Normalize(Entry{Images: many}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("%d images: %v, want ErrInvalid", len(many), err)
	}
	if _, err := Normalize(Entry{Images: []string{" "}}); !errors.Is(err, ErrEmpty) {
		t.Fatalf("blank image only: %v, want ErrEmpty", err)
	}
}

func TestMergeAccumulatesImagesAndCancelKeepsTheFiles(t *testing.T) {
	dir := Dir(t.TempDir())
	a := attach(t, dir, "0123456789abcdef-1.png")
	b := attach(t, dir, "fedcba9876543210-1.webp")
	sendImages(t, dir, "first", a)
	name, merged := sendImages(t, dir, "", b, a)
	if !merged {
		t.Fatal("second send did not merge")
	}
	held, _ := Held(dir)
	if len(held) != 1 || len(held[0].Entry.Images) != 2 || held[0].Entry.Images[0] != a || held[0].Entry.Images[1] != b {
		t.Fatalf("merged images = %+v", held)
	}
	e, err := Cancel(dir, name)
	if err != nil || len(e.Images) != 2 {
		t.Fatalf("Cancel = %+v, %v", e, err)
	}
	if !exists(a) || !exists(b) {
		t.Fatal("cancel deleted an attachment")
	}
}

func TestAttachmentNameConfinesToTheDir(t *testing.T) {
	ad := AttachmentsDir(Dir(t.TempDir()))
	good := filepath.Join(ad, "0123456789abcdef-3.jpg")
	if n, ok := AttachmentName(ad, good); !ok || n != "0123456789abcdef-3.jpg" {
		t.Fatalf("AttachmentName(good) = %q, %v", n, ok)
	}
	for _, p := range []string{
		filepath.Join(ad, "sub", "0123456789abcdef-3.jpg"),
		filepath.Join(filepath.Dir(ad), "0123456789abcdef-3.jpg"),
		filepath.Join(ad, "..", "0123456789abcdef-3.jpg"),
		filepath.Join(ad, "0123456789abcdef-3.svg"),
		filepath.Join(ad, "0123456789abcdef-0.png"),
		filepath.Join(ad, "x.png"),
		"0123456789abcdef-3.jpg",
		"",
	} {
		if _, ok := AttachmentName(ad, p); ok {
			t.Errorf("AttachmentName(%q) accepted", p)
		}
	}
}

func TestTrimDeletesTheTrimmedRecordsImagesUnlessStillListed(t *testing.T) {
	dir := Dir(t.TempDir())
	first := attach(t, dir, "0000000000000001-1.png")
	shared := attach(t, dir, "0000000000000002-1.png")
	stray := filepath.Join(t.TempDir(), "0000000000000003-1.png")
	if err := os.WriteFile(stray, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The oldest record lists first, shared, and a path outside the
	// attachments dir; a later record lists shared again.
	sendImages(t, dir, "oldest", first, shared, stray)
	if _, err := Drain(dir); err != nil {
		t.Fatal(err)
	}
	sendImages(t, dir, "again", shared)
	if _, err := Drain(dir); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < DrainedCap-2; i++ {
		send(t, dir, "m")
		if _, err := Drain(dir); err != nil {
			t.Fatal(err)
		}
	}
	if !exists(first) {
		t.Fatal("an image was deleted before its record was trimmed")
	}
	// The next drain trims the oldest record.
	send(t, dir, "m")
	if _, err := Drain(dir); err != nil {
		t.Fatal(err)
	}
	if exists(first) {
		t.Error("the trimmed record's image survived")
	}
	if !exists(shared) {
		t.Error("an image a remaining record lists was deleted")
	}
	if !exists(stray) {
		t.Error("trim deleted a path outside the attachments dir")
	}
}

func TestTrimKeepsAFileAHeldEntryLists(t *testing.T) {
	dir := Dir(t.TempDir())
	img := attach(t, dir, "0000000000000001-1.gif")
	sendImages(t, dir, "sent", img)
	if _, err := Drain(dir); err != nil {
		t.Fatal(err)
	}
	// Fill the drained records so the next trim removes the one listing img,
	// while a held entry lists it too.
	for i := 0; i < DrainedCap-1; i++ {
		send(t, dir, "m")
		if _, err := Drain(dir); err != nil {
			t.Fatal(err)
		}
	}
	sendImages(t, dir, "held", img)
	// Write one more record straight into drained/ so trimming runs without
	// draining the held entry.
	if _, err := writeEntry(DrainedDir(dir), Entry{SchemaVersion: SchemaVersion, Parts: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	trimDrained(dir)
	if names, _ := DrainedNames(dir); len(names) != DrainedCap {
		t.Fatalf("drained = %d after trim", len(names))
	}
	if !exists(img) {
		t.Fatal("trim deleted an image a held entry lists")
	}
}

func TestSweepDeletesOnlyOldUnreferencedAttachments(t *testing.T) {
	dir := Dir(t.TempDir())
	now := time.Now()
	old := now.Add(-25 * time.Hour)
	orphan := attach(t, dir, "0000000000000001-1.png")
	young := attach(t, dir, "0000000000000002-1.png")
	heldImg := attach(t, dir, "0000000000000003-1.png")
	sentImg := attach(t, dir, "0000000000000004-1.png")
	foreign := attach(t, dir, "notes.png")
	for _, p := range []string{orphan, heldImg, sentImg, foreign} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	sendImages(t, dir, "sent", sentImg)
	if _, err := Drain(dir); err != nil {
		t.Fatal(err)
	}
	sendImages(t, dir, "held", heldImg)
	SweepAttachments(dir, now, 24*time.Hour)
	if exists(orphan) {
		t.Error("an old unreferenced attachment survived the sweep")
	}
	for _, p := range []string{young, heldImg, sentImg, foreign} {
		if !exists(p) {
			t.Errorf("the sweep deleted %s", filepath.Base(p))
		}
	}
}
