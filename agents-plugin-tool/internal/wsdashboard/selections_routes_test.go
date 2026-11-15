package wsdashboard

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
)

// These exercise the actual request/write/cancel handlers without starting
// a listener, holder, adapter, or provider process.
func TestSelectionTextTransportLifecycle(t *testing.T) {
	d := &Daemon{log: io.Discard}
	dir := wsinbox.Dir(t.TempDir())
	now := time.Now()
	first := wsinbox.Selection{
		Mentions: []wsinbox.Mention{
			{Topic: "t", ID: "3", Entry: "e7", Text: " first [Image #1] <source> "},
			{Topic: "t", Entry: "e8", Text: "next source"},
		},
		Comment: "memo <wb:selection> [Image #1]", Images: []int{1},
	}
	second := wsinbox.Selection{Mentions: []wsinbox.Mention{{Topic: "u", Entry: "e9", Text: "other [Image #1]"}}, Images: []int{1}}
	block := func(s wsinbox.Selection) string {
		t.Helper()
		text, err := wsinbox.SelectionBlock(s)
		if err != nil {
			t.Fatal(err)
		}
		return text
	}
	post := func(req messageRequest, at time.Time) string {
		t.Helper()
		raw, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		code, answer := d.messageWrite(dir, "livesession", raw, true, at)
		if code != http.StatusOK {
			t.Fatalf("messageWrite = %d: %v", code, answer)
		}
		ans := answer.(map[string]any)
		if ans["merged"] != false || ans["id"] == "" {
			t.Fatalf("send did not create an independent message: %+v", ans)
		}
		return ans["id"].(string)
	}
	one, two := "typed before\n\n"+block(first), block(second)+"\n\ntyped after"
	secondInSameMessage := second
	secondInSameMessage.Images = []int{2}
	multi := one + "\n\n" + block(secondInSameMessage)
	ids := []string{
		post(messageRequest{Text: one, Images: []imageUpload{{Name: "a.png", Data: b64(pngBytes), N: 1}}}, now),
		post(messageRequest{Text: two, Images: []imageUpload{{Name: "b.jpg", Data: b64(jpegBytes), N: 1}}}, now.Add(time.Millisecond)),
		post(messageRequest{Text: multi, Images: []imageUpload{{Name: "c.png", Data: b64(pngBytes), N: 1}, {Name: "d.jpg", Data: b64(jpegBytes), N: 2}}}, now.Add(2*time.Millisecond)),
	}
	held, err := wsinbox.Held(dir)
	if err != nil || len(held) != 3 || ids[0] == ids[1] || ids[1] == ids[2] {
		t.Fatalf("independent held messages = %+v, %v", held, err)
	}
	check := func(e wsinbox.Entry, text string, nums []int, at int64) {
		t.Helper()
		if !reflect.DeepEqual(e.Parts, []string{text}) || len(e.Mentions) != 0 || !reflect.DeepEqual(e.ImageNums, nums) || len(e.Images) != len(nums) || e.SentAt != at {
			t.Fatalf("transport changed message ownership: %+v", e)
		}
		var lines []string
		for i, path := range e.Images {
			if _, ok := wsinbox.AttachmentName(wsinbox.AttachmentsDir(dir), path); !ok {
				t.Fatalf("image escaped session confinement: %q", path)
			}
			wantBytes := pngBytes
			if text == two || (text == multi && i == 1) {
				wantBytes = jpegBytes
			}
			if raw, err := os.ReadFile(path); err != nil || string(raw) != wantBytes {
				t.Fatalf("stored image ownership = %q, %v; want %q", raw, err, wantBytes)
			}
			lines = append(lines, wsinbox.ImageLine(nums[i], path))
		}
		if inboxBody(t, e) != text+"\n\n"+strings.Join(lines, "\n") {
			t.Fatalf("delivery changed grouping: %q", inboxBody(t, e))
		}
	}
	check(held[0].Entry, one, []int{1}, now.UnixMilli())
	check(held[1].Entry, two, []int{1}, now.Add(time.Millisecond).UnixMilli())
	check(held[2].Entry, multi, []int{1, 2}, now.Add(2*time.Millisecond).UnixMilli())
	paths := [][]string{held[0].Entry.Images, held[1].Entry.Images, held[2].Entry.Images}
	seen := map[string]bool{}
	for _, images := range paths {
		for _, path := range images {
			if seen[path] {
				t.Fatalf("independent uploads shared an attachment path: %q", path)
			}
			seen[path] = true
		}
	}
	code, answer := d.inboxWrite(dir, "cancel", []byte(`{"id":"`+ids[1]+`"}`), now)
	if code != http.StatusOK {
		t.Fatalf("cancel = %d: %v", code, answer)
	}
	cancelled := answer.(map[string]any)
	if cancelled["text"] != two || cancelled["sentAt"] != now.Add(time.Millisecond).UnixMilli() || !reflect.DeepEqual(cancelled["nums"], []int{1}) || len(cancelled["mentions"].([]wsinbox.Mention)) != 0 {
		t.Fatalf("cancel changed original text/number/time: %+v", cancelled)
	}
	for key := range cancelled {
		switch key {
		case "text", "chips", "topics", "mentions", "picks", "consoles", "images", "nums", "sentAt":
		default:
			t.Fatalf("new grouping metadata: %q", key)
		}
	}
	resendAt := now.Add(time.Second)
	ids[1] = post(messageRequest{Text: two, Refs: cancelled["images"].([]string), RefNums: cancelled["nums"].([]int)}, resendAt)
	steer := func(id string) string {
		t.Helper()
		code, answer := d.inboxWrite(dir, "steer", []byte(`{"id":"`+id+`"}`), now)
		if code != http.StatusOK {
			t.Fatalf("steer = %d: %v", code, answer)
		}
		fresh := answer.(map[string]any)["id"].(string)
		if fresh <= id {
			t.Fatalf("promotion did not assign a fresh later ID: %q -> %q", id, fresh)
		}
		return fresh
	}
	oldFirst := ids[0]
	ids[2], ids[0] = steer(ids[2]), steer(ids[0])
	if ids[0] <= ids[2] {
		t.Fatal("later promotion did not join the steering tail")
	}
	if code, _ := d.inboxWrite(dir, "cancel", []byte(`{"id":"`+oldFirst+`"}`), now); code != http.StatusConflict {
		t.Fatal("stale pre-steer ID remained cancellable")
	}
	held, err = wsinbox.Held(dir)
	if err != nil || len(held) != 3 || held[0].ID() != ids[2] || held[1].ID() != ids[0] || held[2].ID() != ids[1] {
		t.Fatalf("steering-first held order = %+v, %v", held, err)
	}
	check(held[0].Entry, multi, []int{1, 2}, now.Add(2*time.Millisecond).UnixMilli())
	check(held[1].Entry, one, []int{1}, now.UnixMilli())
	check(held[2].Entry, two, []int{1}, resendAt.UnixMilli())
	if !reflect.DeepEqual(held[2].Entry.Images, paths[1]) {
		t.Fatal("cancel/resend moved the second message's image")
	}
	code, answer = d.inboxWrite(dir, "interrupt", nil, now)
	if code != http.StatusOK {
		t.Fatalf("interrupt = %d: %v", code, answer)
	}
	interrupted := answer.(map[string]any)["held"].([]map[string]any)
	wantTexts := []string{multi, one, two}
	wantTimes := []int64{now.Add(2 * time.Millisecond).UnixMilli(), now.UnixMilli(), resendAt.UnixMilli()}
	if len(interrupted) != 3 {
		t.Fatalf("interrupt = %+v", interrupted)
	}
	for i, item := range interrupted {
		if item["text"] != wantTexts[i] || item["sentAt"] != wantTimes[i] {
			t.Fatalf("interrupt ownership/order/time = %+v", interrupted)
		}
	}
	// HTTP resends are new sends with new timestamps, not new metadata.
	// Reinsert the follow-up first, then promote the other messages again.
	var again [3]string
	for _, i := range []int{2, 0, 1} {
		item := interrupted[i]
		again[i] = post(messageRequest{Text: item["text"].(string), Refs: item["images"].([]string), RefNums: item["nums"].([]int)}, now.Add(2*time.Second))
	}
	steer(again[0])
	steer(again[1])
	drained, err := wsinbox.Drain(dir)
	if err != nil || len(drained) != 3 {
		t.Fatalf("drain = %+v, %v", drained, err)
	}
	for i, r := range drained {
		nums := []int{1}
		if i == 0 {
			nums = []int{1, 2}
		}
		check(r.Entry, wantTexts[i], nums, now.Add(2*time.Second).UnixMilli())
		if !reflect.DeepEqual(r.Entry.Images, paths[[]int{2, 0, 1}[i]]) {
			t.Fatalf("queue lifecycle moved image ownership: %+v", r.Entry)
		}
	}
}

func TestSelectionTextMalformedEnvelopeWithoutImages(t *testing.T) {
	text := "<wb:selection>\n<!-- </wb:selection>\n-->\n<wb:selection>\n" +
		`<wb:mention topic="t">literal [Image #1]</wb:mention>` + "\n<wb:comment>memo [Image #1]</wb:comment>\n[Image #1]\n</wb:selection>\n</wb:selection>"
	dir := wsinbox.Dir(t.TempDir())
	d := &Daemon{log: io.Discard}
	raw, err := json.Marshal(messageRequest{Text: text})
	if err != nil {
		t.Fatal(err)
	}
	if code, answer := d.messageWrite(dir, "livesession", raw, true, time.Now()); code != http.StatusOK {
		t.Fatalf("zero-image submit rejected malformed ordinary text: %d: %v", code, answer)
	}
	held, err := wsinbox.Held(dir)
	if err != nil || len(held) != 1 || inboxBody(t, held[0].Entry) != text || len(held[0].Entry.Images) != 0 {
		t.Fatalf("zero-image submit changed malformed ordinary text: %+v, %v", held, err)
	}
}

func TestSelectionTextOrdinaryPrefixImageValidation(t *testing.T) {
	prefix := "1 < 2\n\n"
	failed := "<wb:selection>\n<!-- </wb:selection>\n-->\n<wb:selection>\n" +
		`<wb:mention topic="t">literal [Image #1]</wb:mention>` + "\n<wb:comment>memo [Image #1]</wb:comment>\n[Image #1]\n</wb:selection>\n</wb:selection>"
	valid, err := wsinbox.SelectionBlock(wsinbox.Selection{Mentions: []wsinbox.Mention{{Topic: "u", Text: "source [Image #1]"}}, Comment: "memo [Image #1]", Images: []int{1}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		text   string
		code   int
		images []imageUpload
	}{
		{name: "opaque without images", text: prefix + failed, code: http.StatusOK},
		{name: "valid without attachment", text: prefix + valid, code: http.StatusBadRequest},
		{name: "valid sibling without attachment", text: prefix + failed + "\n\n" + valid, code: http.StatusBadRequest},
		{name: "valid with attachment", text: prefix + valid, code: http.StatusOK, images: []imageUpload{{Name: "a.png", Data: b64(pngBytes), N: 1}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := wsinbox.Dir(t.TempDir())
			d := &Daemon{log: io.Discard}
			raw, err := json.Marshal(messageRequest{Text: tc.text, Images: tc.images})
			if err != nil {
				t.Fatal(err)
			}
			if code, answer := d.messageWrite(dir, "livesession", raw, true, time.Now()); code != tc.code {
				t.Fatalf("prefixed submit = %d: %v, want %d", code, answer, tc.code)
			}
			held, err := wsinbox.Held(dir)
			if err != nil {
				t.Fatal(err)
			}
			if tc.code == http.StatusOK {
				if len(held) != 1 || strings.Join(held[0].Entry.Parts, "\n\n") != tc.text || len(held[0].Entry.Images) != len(tc.images) {
					t.Fatalf("accepted prefixed submit changed text/ownership: %+v", held)
				}
			} else if len(held) != 0 {
				t.Fatalf("rejected prefixed submit was held: %+v", held)
			}
		})
	}
}

func TestSelectionTextRetainsExistingImageGuards(t *testing.T) {
	s := wsinbox.Selection{Mentions: []wsinbox.Mention{{Topic: "t", Text: "source"}}, Images: []int{1}}
	block, err := wsinbox.SelectionBlock(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		req  messageRequest
		code int
	}{
		{"dangling reference", messageRequest{Text: block}, http.StatusBadRequest},
		{"dangling reference after upload", messageRequest{Text: strings.Replace(block, "\n[Image #1]\n", "\n[Image #2]\n", 1), Images: []imageUpload{{Name: "a.png", Data: b64(pngBytes), N: 1}}}, http.StatusBadRequest},
		{"bad image", messageRequest{Text: block, Images: []imageUpload{{Name: "bad.png", Data: b64("not an image"), N: 1}}}, http.StatusUnsupportedMediaType},
		{"foreign ref", messageRequest{Text: block, Refs: []string{"../elsewhere.png"}, RefNums: []int{1}}, http.StatusBadRequest},
		{"oversized image", messageRequest{Text: block, Images: []imageUpload{{Name: "big.png", Data: b64(pngBytes + strings.Repeat("x", maxImageBytes)), N: 1}}}, http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := wsinbox.Dir(t.TempDir())
			d := &Daemon{log: io.Discard}
			raw, err := json.Marshal(tc.req)
			if err != nil {
				t.Fatal(err)
			}
			if code, ans := d.messageWrite(dir, "livesession", raw, true, time.Now()); code != tc.code {
				t.Fatalf("message = %d: %v, want %d", code, ans, tc.code)
			}
			if held, err := wsinbox.Held(dir); err != nil || len(held) != 0 {
				t.Fatalf("refused send persisted: %+v, %v", held, err)
			}
			if files, _ := os.ReadDir(wsinbox.AttachmentsDir(dir)); len(files) != 0 {
				t.Fatalf("refused send leaked images: %+v", files)
			}
		})
	}
	// Each message can independently carry eight images numbered 1..8.
	// A ninth image in one request is refused without leaking any file.
	dir := wsinbox.Dir(t.TempDir())
	d := &Daemon{log: io.Discard}
	var uploads []imageUpload
	var nums []int
	for i := 1; i <= wsinbox.MaxImages; i++ {
		uploads = append(uploads, imageUpload{Name: "a.png", Data: b64(pngBytes), N: i})
		nums = append(nums, i)
	}
	s.Images = nums
	fullBlock, err := wsinbox.SelectionBlock(s)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		images := uploads
		want := http.StatusOK
		if i == 2 {
			images = append(append([]imageUpload{}, uploads...), imageUpload{Name: "ninth.png", Data: b64(pngBytes), N: 9})
			want = http.StatusRequestEntityTooLarge
		}
		raw, err := json.Marshal(messageRequest{Text: fullBlock, Images: images})
		if err != nil {
			t.Fatal(err)
		}
		if code, ans := d.messageWrite(dir, "livesession", raw, true, time.Now()); code != want {
			t.Fatalf("per-message cap send %d = %d: %v, want %d", i, code, ans, want)
		}
	}
	held, err := wsinbox.Held(dir)
	if err != nil || len(held) != 2 {
		t.Fatalf("independent capped messages = %+v, %v", held, err)
	}
	for _, r := range held {
		if len(r.Entry.Images) != wsinbox.MaxImages || !reflect.DeepEqual(r.Entry.ImageNums, nums) || !reflect.DeepEqual(r.Entry.Parts, []string{fullBlock}) {
			t.Fatalf("per-message cap/numbering = %+v", r.Entry)
		}
	}
	files, err := os.ReadDir(wsinbox.AttachmentsDir(dir))
	if err != nil || len(files) != 2*wsinbox.MaxImages {
		t.Fatalf("per-message cap files = %d, %v", len(files), err)
	}
	// No storage field, upload route, or attachment naming rule changed.
	for _, file := range files {
		if !wsinbox.ValidAttachmentName(filepath.Base(file.Name())) {
			t.Fatalf("new attachment shape: %q", file.Name())
		}
	}
}
