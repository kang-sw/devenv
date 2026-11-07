package wsdashboard

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

// Bytes that http.DetectContentType sniffs as each accepted type.
var (
	pngBytes  = "\x89PNG\r\n\x1a\n" + strings.Repeat("p", 24)
	jpegBytes = "\xff\xd8\xff\xe0" + strings.Repeat("j", 24)
	gifBytes  = "GIF89a" + strings.Repeat("g", 24)
	webpBytes = "RIFF\x00\x00\x00\x00WEBPVP8 " + strings.Repeat("w", 24)
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// imagesJSON is an images array of name/data pairs.
func imagesJSON(pairs ...string) string {
	var items []string
	for i := 0; i+1 < len(pairs); i += 2 {
		name, _ := json.Marshal(pairs[i])
		items = append(items, `{"name":`+string(name)+`,"data":"`+b64(pairs[i+1])+`"}`)
	}
	return "[" + strings.Join(items, ",") + "]"
}

func (f *pageFixture) attachDir() string {
	return wsinbox.AttachmentsDir(sessionInbox(f.dir, "livesession"))
}

func (f *pageFixture) attachments(t *testing.T) []string {
	t.Helper()
	des, err := os.ReadDir(f.attachDir())
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var names []string
	for _, de := range des {
		names = append(names, de.Name())
	}
	return names
}

func (f *pageFixture) postMessage(t *testing.T, body string) (int, string) {
	t.Helper()
	code, _, raw := postJSON(t, f.base+"/s/livesession/message", f.base, body)
	return code, raw
}

// TestMessageRouteAttachesSniffedImages: the four accepted types are
// written into the session's attachments dir under the attachment grammar,
// with the extension of the sniffed type whatever the browser named them;
// the held entry lists their absolute paths, and the delivered text carries
// one image line each after the typed text and before the answers.
func TestMessageRouteAttachesSniffedImages(t *testing.T) {
	f := startPageFixture(t)
	id, merged := f.send(t, `{"text":"look","chips":[{"id":"03","answer":"a"}],"images":`+imagesJSON("a.png", pngBytes, "b.jpeg", jpegBytes, "c.gif", gifBytes, "named.png", webpBytes)+`}`)
	if merged {
		t.Fatal("first send merged")
	}
	held := f.held(t)
	if len(held) != 1 || held[0].ID() != id || len(held[0].Entry.Images) != 4 {
		t.Fatalf("held = %+v", held)
	}
	var lines []string
	for i, path := range held[0].Entry.Images {
		name, ok := wsinbox.AttachmentName(f.attachDir(), path)
		if !ok {
			t.Fatalf("image %d path %q is not in the attachments dir", i, path)
		}
		if want := []string{".png", ".jpg", ".gif", ".webp"}[i]; filepath.Ext(name) != want {
			t.Errorf("image %d written as %s, want %s", i, name, want)
		}
		if raw, err := os.ReadFile(path); err != nil || string(raw) != []string{pngBytes, jpegBytes, gifBytes, webpBytes}[i] {
			t.Errorf("image %d bytes = %q, %v", i, raw, err)
		}
		lines = append(lines, "[image: "+path+"]")
	}
	if got, want := held[0].Entry.Text(), "look\n\n"+strings.Join(lines, "\n")+"\n\n(03) a"; got != want {
		t.Fatalf("delivered text = %q, want %q", got, want)
	}

	// An image alone is a message.
	if _, merged := f.send(t, `{"images":`+imagesJSON("x.png", pngBytes)+`}`); !merged {
		t.Fatal("image-only send did not merge")
	}
	if held := f.held(t); len(held[0].Entry.Images) != 5 {
		t.Fatalf("merged images = %d, want 5", len(held[0].Entry.Images))
	}
}

// TestMessageRouteRefusesBadImages: a mislabeled file is sniffed and
// refused with 415, bad base64 is 400, and the count and size caps are 413,
// the merged entry's cap included; a refused request writes no file and
// holds nothing.
func TestMessageRouteRefusesBadImages(t *testing.T) {
	f := startPageFixture(t)
	nine := []string{}
	for i := 0; i < wsinbox.MaxImages+1; i++ {
		nine = append(nine, "p.png", pngBytes)
	}
	big := "\x89PNG\r\n\x1a\n" + strings.Repeat("x", maxImageBytes)
	cases := []struct {
		name, body string
		code       int
		says       string
	}{
		{"mislabeled text", `{"images":` + imagesJSON("shot.png", "just some text, not an image") + `}`, http.StatusUnsupportedMediaType, "shot.png"},
		{"svg", `{"images":` + imagesJSON("v.svg", `<svg xmlns="http://www.w3.org/2000/svg"></svg>`) + `}`, http.StatusUnsupportedMediaType, "v.svg"},
		{"bmp", `{"images":` + imagesJSON("b.bmp", "BM"+strings.Repeat("b", 30)) + `}`, http.StatusUnsupportedMediaType, "b.bmp"},
		{"bad base64", `{"images":[{"name":"q.png","data":"!!!not base64"}]}`, http.StatusBadRequest, "q.png"},
		{"empty data", `{"images":[{"name":"e.png","data":""}]}`, http.StatusBadRequest, "e.png"},
		{"too many", `{"images":` + imagesJSON(nine...) + `}`, http.StatusRequestEntityTooLarge, "at most 8"},
		{"too large", `{"images":` + imagesJSON("big.png", big) + `}`, http.StatusRequestEntityTooLarge, "big.png"},
	}
	for _, c := range cases {
		code, raw := f.postMessage(t, c.body)
		if code != c.code || !strings.Contains(raw, c.says) {
			t.Errorf("%s = %d (%s), want %d naming %q", c.name, code, strings.TrimSpace(raw), c.code, c.says)
		}
	}
	if held := f.held(t); len(held) != 0 {
		t.Fatalf("refused sends held %+v", held)
	}
	if names := f.attachments(t); len(names) != 0 {
		t.Fatalf("refused sends wrote %v", names)
	}

	// A refusal after the files were written (Send's own text cap) deletes
	// them.
	before := len(f.attachments(t))
	huge := strings.Repeat("y", 70<<10)
	if code, raw := f.postMessage(t, `{"text":"`+huge+`","images":`+imagesJSON("z.png", pngBytes)+`}`); code != http.StatusBadRequest {
		t.Fatalf("over-long text with an image = %d (%s), want 400", code, raw)
	}
	if after := len(f.attachments(t)); after != before {
		t.Fatalf("a refused send left %d new files", after-before)
	}

	// The merged cap: six held, three more refused before any file is
	// written; two more fill the entry to eight.
	six := []string{}
	for i := 0; i < 6; i++ {
		six = append(six, "p.png", pngBytes)
	}
	f.send(t, `{"text":"six","images":`+imagesJSON(six...)+`}`)
	code, raw := f.postMessage(t, `{"images":`+imagesJSON("a.png", pngBytes, "b.png", pngBytes, "c.png", pngBytes)+`}`)
	if code != http.StatusRequestEntityTooLarge || !strings.Contains(raw, "9 images") {
		t.Fatalf("send over the merged cap = %d (%s), want 413", code, raw)
	}
	if names := f.attachments(t); len(names) != 6 {
		t.Fatalf("the refused merge left %d files, want 6", len(names))
	}
	if _, merged := f.send(t, `{"images":`+imagesJSON("a.png", pngBytes, "b.png", pngBytes)+`}`); !merged {
		t.Fatal("send up to the cap did not merge")
	}

	// A text-only body is still refused above the text-only limit.
	long := strings.Repeat("x", inboxBodyLimit+10)
	if code, _ := f.postMessage(t, `{"text":"`+long+`"}`); code != http.StatusBadRequest {
		t.Errorf("oversized text-only body = %d, want 400", code)
	}
}

// TestImageCapSkipsASteeredEntry: a steered entry takes no merge, so a
// later send's images are capped against the new entry it starts, not
// against the steered one's images.
func TestImageCapSkipsASteeredEntry(t *testing.T) {
	f := startPageFixture(t)
	full := []string{}
	for i := 0; i < wsinbox.MaxImages; i++ {
		full = append(full, "p.png", pngBytes)
	}
	id, _ := f.send(t, `{"text":"full","images":`+imagesJSON(full...)+`}`)
	if code, _, raw := postJSON(t, f.base+"/s/livesession/steer", f.base, `{"id":"`+id+`"}`); code != http.StatusOK {
		t.Fatalf("steer = %d: %s", code, raw)
	}
	held := f.held(t)
	if len(held) != 1 || !held[0].Entry.Steer || len(held[0].Entry.Images) != wsinbox.MaxImages {
		t.Fatalf("held after steer = %+v", held)
	}
	// Counting the steered entry would refuse this as 16 images.
	if _, merged := f.send(t, `{"text":"next","images":`+imagesJSON(full...)+`}`); merged {
		t.Fatal("a send merged into the steered entry")
	}
	held = f.held(t)
	if len(held) != 2 || !held[0].Entry.Steer || held[1].Entry.Steer || len(held[0].Entry.Images) != wsinbox.MaxImages || len(held[1].Entry.Images) != wsinbox.MaxImages {
		t.Fatalf("held = %+v", held)
	}
	// The new entry is the merge target now, and it is full.
	code, raw := f.postMessage(t, `{"images":`+imagesJSON("one.png", pngBytes)+`}`)
	if code != http.StatusRequestEntityTooLarge || !strings.Contains(raw, "9 images") {
		t.Fatalf("send over the new entry's cap = %d (%s), want 413", code, raw)
	}
}

// TestImageMessageNeedsOriginAndAdapter: an image-bearing message keeps the
// write route's Origin check and its no-adapter refusal, and writes nothing.
func TestImageMessageNeedsOriginAndAdapter(t *testing.T) {
	f := startPageFixture(t)
	body := `{"images":` + imagesJSON("a.png", pngBytes) + `}`
	if code, _, _ := postJSON(t, f.base+"/s/livesession/message", "http://evil.example", body); code != http.StatusForbidden {
		t.Errorf("cross-origin image send = %d, want 403", code)
	}
	if code, _, _ := postJSON(t, f.base+"/s/boardsession/message", f.base, body); code != http.StatusConflict {
		t.Errorf("image send without an adapter = %d, want 409", code)
	}
	if names := f.attachments(t); len(names) != 0 {
		t.Fatalf("refused image sends wrote %v", names)
	}
	if des, _ := os.ReadDir(wsinbox.AttachmentsDir(sessionInbox(f.dir, "boardsession"))); len(des) != 0 {
		t.Fatal("the no-adapter refusal wrote a file")
	}
}

// TestCancelReturnsImagesAndAResendReferencesThem: cancel keeps the files
// and returns their names; a re-send by reference lists the same files
// without a new upload; a reference that names another path or a missing
// file is refused, naming it.
func TestCancelReturnsImagesAndAResendReferencesThem(t *testing.T) {
	f := startPageFixture(t)
	id, _ := f.send(t, `{"text":"pic","images":`+imagesJSON("a.png", pngBytes, "b.gif", gifBytes)+`}`)
	paths := f.held(t)[0].Entry.Images
	code, _, raw := postJSON(t, f.base+"/s/livesession/cancel", f.base, `{"id":"`+id+`"}`)
	if code != http.StatusOK {
		t.Fatalf("cancel = %d: %s", code, raw)
	}
	var ans struct {
		Text   string   `json:"text"`
		Images []string `json:"images"`
	}
	if err := json.Unmarshal([]byte(raw), &ans); err != nil {
		t.Fatal(err)
	}
	if ans.Text != "pic" || len(ans.Images) != 2 || ans.Images[0] != filepath.Base(paths[0]) || ans.Images[1] != filepath.Base(paths[1]) {
		t.Fatalf("cancel answer = %+v", ans)
	}
	if names := f.attachments(t); len(names) != 2 {
		t.Fatalf("cancel left %v", names)
	}
	refs, _ := json.Marshal(ans.Images)
	f.send(t, `{"text":"again","refs":`+string(refs)+`}`)
	held := f.held(t)
	if len(held) != 1 || len(held[0].Entry.Images) != 2 || held[0].Entry.Images[0] != paths[0] || held[0].Entry.Images[1] != paths[1] {
		t.Fatalf("re-sent entry = %+v, want paths %v", held, paths)
	}
	if names := f.attachments(t); len(names) != 2 {
		t.Fatalf("a re-send by reference wrote files: %v", names)
	}
	for _, ref := range []string{"../../registration.json", "0123456789abcdef-1.png", "x.png", filepath.Join(f.attachDir(), ans.Images[0])} {
		r, _ := json.Marshal([]string{ref})
		code, raw := f.postMessage(t, `{"text":"bad ref","refs":`+string(r)+`}`)
		if code != http.StatusBadRequest || !strings.Contains(raw, ref) {
			t.Errorf("ref %q = %d (%s), want 400 naming it", ref, code, strings.TrimSpace(raw))
		}
	}
	if held := f.held(t); len(held[0].Entry.Parts) != 1 {
		t.Fatalf("a refused ref changed the held entry: %+v", held)
	}
}

// TestMessageWriteSweepsOldUnreferencedAttachments: a file no entry lists
// and older than a day goes on the next message write, and a re-send that
// references it is refused.
func TestMessageWriteSweepsOldUnreferencedAttachments(t *testing.T) {
	f := startPageFixture(t)
	id, _ := f.send(t, `{"images":`+imagesJSON("a.png", pngBytes)+`}`)
	path := f.held(t)[0].Entry.Images[0]
	if code, _, raw := postJSON(t, f.base+"/s/livesession/cancel", f.base, `{"id":"`+id+`"}`); code != http.StatusOK {
		t.Fatalf("cancel = %d: %s", code, raw)
	}
	old := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	code, raw := f.postMessage(t, `{"refs":["`+filepath.Base(path)+`"]}`)
	if code != http.StatusBadRequest || !strings.Contains(raw, filepath.Base(path)) {
		t.Fatalf("send of a swept ref = %d (%s), want 400 naming it", code, raw)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the old unreferenced file survived: %v", err)
	}
}

// TestAttachmentRouteServesOnlyThisSessionsAttachments pins the attachment
// route's confinement: a written attachment is served with its type; a name
// outside the grammar, a symlink, a missing file, and another session's path
// all answer 404.
func TestAttachmentRouteServesOnlyThisSessionsAttachments(t *testing.T) {
	f := startPageFixture(t)
	f.send(t, `{"images":`+imagesJSON("a.png", pngBytes)+`}`)
	name := filepath.Base(f.held(t)[0].Entry.Images[0])
	resp, err := testClient().Get(f.base + "/s/livesession/attachment/" + name)
	if err != nil {
		t.Fatal(err)
	}
	body := make([]byte, 64)
	n, _ := resp.Body.Read(body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body[:n]) != pngBytes || resp.Header.Get("Content-Type") != "image/png" ||
		resp.Header.Get("X-Content-Type-Options") != "nosniff" || resp.Header.Get("Content-Security-Policy") != imageContentSecurityPolicy {
		t.Fatalf("attachment = %d %v %q", resp.StatusCode, resp.Header, body[:n])
	}
	secret := filepath.Join(t.TempDir(), "secret.png")
	writeFile(t, secret, pngBytes)
	link := "00000000000000aa-1.png"
	symlinked := os.Symlink(secret, filepath.Join(f.attachDir(), link)) == nil
	writeFile(t, filepath.Join(f.attachDir(), "notes.png"), pngBytes)
	refused := []string{
		"/s/livesession/attachment/notes.png",
		"/s/livesession/attachment/0123456789abcdef-1.png",
		"/s/livesession/attachment/..%2F..%2Fregistration.json",
		"/s/livesession/attachment/" + name + "/x",
		"/s/boardsession/attachment/" + name,
		"/s/nosuchsession/attachment/" + name,
	}
	if symlinked {
		refused = append(refused, "/s/livesession/attachment/"+link)
	}
	for _, path := range refused {
		if code, body := get(t, f.base+path); code != http.StatusNotFound || strings.Contains(body, "PNG") {
			t.Errorf("GET %s = %d, want 404 without the file", path, code)
		}
	}
}

// TestPageShowsAttachmentThumbnails: the held bubble shows its images
// through the attachment route; a delivered user message's image lines
// naming this session's existing attachments become thumbnails, and a line
// naming any other path, or an attachment since removed, stays text.
func TestPageShowsAttachmentThumbnails(t *testing.T) {
	f := startPageFixture(t)
	f.send(t, `{"text":"held one","images":`+imagesJSON("a.png", pngBytes)+`}`)
	held := f.held(t)[0].Entry.Images[0]
	heldName := filepath.Base(held)
	delivered := filepath.Join(f.attachDir(), "00000000000000bb-1.jpg")
	if err := os.WriteFile(delivered, []byte("jpg"), 0o600); err != nil {
		t.Fatal(err)
	}
	removed := filepath.Join(f.attachDir(), "00000000000000cc-1.png")
	text := "see these\n\n[image: " + delivered + "]\n[image: /elsewhere/shot.png]\n[image: " + removed + "]\n\n(03) a"
	if err := wslive.AppendRing(wslive.RingDir(SessionDir(f.dir, "livesession")), []wslive.RingEntry{{Kind: wslive.RingUser, Origin: wslive.OriginWhiteboard, Text: text}}); err != nil {
		t.Fatal(err)
	}
	_, page := get(t, f.base+"/s/livesession")
	for _, want := range []string{
		`<div class="hpart">held one</div><div class="thumbs"><a class="thumb" href="/s/livesession/attachment/` + heldName + `"`,
		`<img src="/s/livesession/attachment/` + heldName + `" alt="` + heldName + `"`,
		`see these` + "\n\n" + `[image: /elsewhere/shot.png]` + "\n" + `[image: ` + removed + `]` + "\n\n" + `(03) a<div class="thumbs"><a class="thumb" href="/s/livesession/attachment/00000000000000bb-1.jpg"`,
		`id="cimgs"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("session page lacks %q", want)
		}
	}
	if strings.Contains(page, "[image: "+delivered+"]") {
		t.Error("the delivered attachment's image line is still shown as text")
	}
	if strings.Contains(page, "attachment/00000000000000cc-1.png") {
		t.Error("a removed attachment renders as an image")
	}
}

func TestUserImagesKeepsForeignLinesAndTidiesBlanks(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "attachments")
	in := filepath.Join(dir, "0123456789abcdef-1.png")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(in, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(dir, "0123456789abcdef-2.png")
	cases := []struct{ text, want string }{
		{"plain", "plain"},
		{"[image: " + in + "]", ""},
		{"hi\n\n[image: " + in + "]\n\n(03) a", "hi\n\n(03) a"},
		{"hi\n\n[image: /x/y.png]", "hi\n\n[image: /x/y.png]"},
		{"hi\n\n[image: " + gone + "]", "hi\n\n[image: " + gone + "]"},
	}
	for _, c := range cases {
		if got, _ := userImages(c.text, dir, "livesession"); got != c.want {
			t.Errorf("userImages(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}
