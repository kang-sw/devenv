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
	e := Entry{Parts: []string{"look [Image #1]", "and this [Image #3]"}, Images: []string{"/s/a/1.png", "/s/a/2.jpg"}, ImageNums: []int{1, 3}, Chips: []Chip{{ID: "03", Answer: "a"}}}
	want := "look [Image #1]\n\nand this [Image #3]\n\nImage #1: /s/a/1.png\nImage #3: /s/a/2.jpg\n\n(03) a"
	if got := e.Text(); got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}
	// An entry written before images were numbered numbers them 1..n.
	if got := (Entry{Images: []string{"/x/y.png", "/x/z.png"}}).Text(); got != "Image #1: /x/y.png\nImage #2: /x/z.png" {
		t.Fatalf("unnumbered image-only Text = %q", got)
	}
}

// TestParseImageLineReadsBothForms: the numbered line and the unnumbered
// "[image: <path>]" line older messages carry both parse; anything else does
// not.
func TestParseImageLineReadsBothForms(t *testing.T) {
	cases := []struct {
		line string
		n    int
		path string
		ok   bool
	}{
		{ImageLine(4, "/x/y.png"), 4, "/x/y.png", true},
		{"[image: /x/y.png]", 0, "/x/y.png", true},
		{"[image: ]", 0, "", false},
		{"Image #0: /x/y.png", 0, "", false},
		{"Image #2:", 0, "", false},
		{"see Image #2: /x/y.png", 0, "", false},
		{"plain", 0, "", false},
	}
	for _, c := range cases {
		n, path, ok := ParseImageLine(c.line)
		if n != c.n || path != c.path || ok != c.ok {
			t.Errorf("ParseImageLine(%q) = %d, %q, %v; want %d, %q, %v", c.line, n, path, ok, c.n, c.path, c.ok)
		}
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

// TestNormalizeNumbersImages: given numbers are kept, gaps included; a
// repeated path drops its number with it; an unnumbered image gets the next
// number above the highest; a malformed numbering is refused.
func TestNormalizeNumbersImages(t *testing.T) {
	d := t.TempDir()
	a, b, c := filepath.Join(d, "a.png"), filepath.Join(d, "b.png"), filepath.Join(d, "c.png")
	e, err := Normalize(Entry{Images: []string{a, " ", b, a}, ImageNums: []int{2, 9, 5, 7}})
	if err != nil || len(e.ImageNums) != 2 || e.ImageNums[0] != 2 || e.ImageNums[1] != 5 {
		t.Fatalf("Normalize with numbers = %+v, %v", e, err)
	}
	if e, err = Normalize(Entry{Images: []string{a, b, c}, ImageNums: []int{0, 4, 0}}); err != nil || e.ImageNums[0] != 5 || e.ImageNums[1] != 4 || e.ImageNums[2] != 6 {
		t.Fatalf("Normalize with unnumbered images = %+v, %v", e, err)
	}
	if e, err = Normalize(Entry{Images: []string{a, b}}); err != nil || e.ImageNums[0] != 1 || e.ImageNums[1] != 2 {
		t.Fatalf("Normalize without numbers = %+v, %v", e, err)
	}
	for _, bad := range []Entry{
		{Images: []string{a, b}, ImageNums: []int{1}},
		{Images: []string{a, b}, ImageNums: []int{3, 3}},
		{Images: []string{a}, ImageNums: []int{-1}},
		{Images: []string{a}, ImageNums: []int{maxImageNum + 1}},
	} {
		if _, err := Normalize(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("Normalize(%+v) = %v, want ErrInvalid", bad, err)
		}
	}
}

// TestMergeRenumbersTheLaterSendsImagesAndMarkers: the later send's images
// continue the earlier ones' numbers, gaps included, and its markers follow;
// a marker naming none of its images and an image the earlier send already
// lists keep the earlier numbering; the earlier send's text is untouched.
func TestMergeRenumbersTheLaterSendsImagesAndMarkers(t *testing.T) {
	base := Entry{Parts: []string{"one [Image #1] two [Image #3]"}, Images: []string{"/a.png", "/b.png"}, ImageNums: []int{1, 3}}
	add := Entry{Parts: []string{"[Image #2] then [Image #1] and [Image #1], old [Image #9], same [Image #3]"}, Images: []string{"/c.png", "/d.png", "/b.png"}, ImageNums: []int{1, 2, 3}}
	got := Merge(base, add)
	if want := []string{"one [Image #1] two [Image #3]", "[Image #5] then [Image #4] and [Image #4], old [Image #9], same [Image #3]"}; len(got.Parts) != 2 || got.Parts[0] != want[0] || got.Parts[1] != want[1] {
		t.Fatalf("merged parts = %q, want %q", got.Parts, want)
	}
	if len(got.Images) != 4 || got.Images[2] != "/c.png" || got.Images[3] != "/d.png" {
		t.Fatalf("merged images = %v", got.Images)
	}
	if want := []int{1, 3, 4, 5}; len(got.ImageNums) != 4 || got.ImageNums[0] != want[0] || got.ImageNums[1] != want[1] || got.ImageNums[2] != want[2] || got.ImageNums[3] != want[3] {
		t.Fatalf("merged numbers = %v, want %v", got.ImageNums, want)
	}
	// An entry written before numbering counts as 1..n.
	legacy := Merge(Entry{Parts: []string{"old"}, Images: []string{"/a.png", "/b.png"}}, Entry{Parts: []string{"[Image #1]"}, Images: []string{"/c.png"}, ImageNums: []int{1}})
	if legacy.Parts[1] != "[Image #3]" || len(legacy.ImageNums) != 3 || legacy.ImageNums[2] != 3 {
		t.Fatalf("merge onto an unnumbered entry = %+v", legacy)
	}
}

// TestSendMergesWithRenumberingAndSteerKeepsIt: the second send into a held
// entry delivers its image as the next number, marker included, and a steer
// keeps the numbering.
func TestSendMergesWithRenumberingAndSteerKeepsIt(t *testing.T) {
	dir := Dir(t.TempDir())
	a := attach(t, dir, "0123456789abcdef-1.png")
	b := attach(t, dir, "0123456789abcdef-2.png")
	c := attach(t, dir, "fedcba9876543210-1.png")
	if _, _, err := Send(dir, Entry{Parts: []string{"first [Image #1] [Image #2]"}, Images: []string{a, b}, ImageNums: []int{1, 2}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	name, merged, err := Send(dir, Entry{Parts: []string{"second [Image #1]"}, Images: []string{c}, ImageNums: []int{1}}, time.Now())
	if err != nil || !merged {
		t.Fatalf("second Send = %v, merged %v", err, merged)
	}
	steered, err := Steer(dir, name)
	if err != nil {
		t.Fatal(err)
	}
	held, _ := Held(dir)
	if len(held) != 1 || held[0].Name != steered {
		t.Fatalf("held = %+v", held)
	}
	want := "first [Image #1] [Image #2]\n\nsecond [Image #3]\n\nImage #1: " + a + "\nImage #2: " + b + "\nImage #3: " + c
	if got := held[0].Entry.Text(); got != want {
		t.Fatalf("steered merged Text = %q, want %q", got, want)
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

// TestSteerKeepsTheEntrysImages: a steer rewrites the whole entry, so the
// steered entry lists the same image paths, the files stay, and the
// steer-only drain hands the paths to the adapter.
func TestSteerKeepsTheEntrysImages(t *testing.T) {
	dir := Dir(t.TempDir())
	a := attach(t, dir, "0123456789abcdef-1.png")
	b := attach(t, dir, "0123456789abcdef-2.jpg")
	name, _ := sendImages(t, dir, "look", a, b)
	steered, err := Steer(dir, name)
	if err != nil {
		t.Fatal(err)
	}
	held, _ := Held(dir)
	if len(held) != 1 || held[0].Name != steered || !held[0].Entry.Steer {
		t.Fatalf("held after steer = %+v", held)
	}
	if imgs := held[0].Entry.Images; len(imgs) != 2 || imgs[0] != a || imgs[1] != b {
		t.Fatalf("steered images = %v, want [%s %s]", imgs, a, b)
	}
	if !exists(a) || !exists(b) {
		t.Fatal("steer deleted an attachment")
	}
	got, err := DrainSteer(dir)
	if err != nil || len(got) != 1 || len(got[0].Entry.Images) != 2 {
		t.Fatalf("DrainSteer = %+v, %v", got, err)
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
