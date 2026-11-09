package wstrackerhost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
)

// pngBytes is a byte string that sniffs as image/png.
func pngBytes(tail string) []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), tail...)
}

func stageOK(t *testing.T, h *Host, store, name string, data []byte) Attachment {
	t.Helper()
	att, err := h.StageAttachment(context.Background(), store, name, data)
	if err != nil {
		t.Fatalf("stage %q: %v", name, err)
	}
	return att
}

// createWithAttachment dispatches an item.create naming att.
func createWithAttachment(t *testing.T, h *Host, store string, att Attachment) ([]wsledger.EventID, error) {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"kind": "task", "title": "with image", "attachments": []Attachment{att}})
	return h.Dispatch(context.Background(), store, []wsledger.NewEvent{{Type: wstracker.TypeItemCreate, Payload: payload}})
}

// TestAttachmentsStageDispatchRead: an upload is refused unless it is an
// accepted image within the limit; a dispatch naming an oid that is neither
// staged nor stored is refused; a dispatch naming a staged oid writes the
// blob with the event and drops the staged copy; the blob is readable on
// the writer before the push and on another machine after it. HasAttachment
// answers true for a staged, pending, or stored oid and false otherwise.
func TestAttachmentsStageDispatchRead(t *testing.T) {
	w := newWorld(t)
	a, b := w.machine("a"), w.machine("b")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	w.register(b, "team", "bo", &wstracker.Profile{Given: "Bo"})
	ctx := context.Background()

	if _, err := a.StageAttachment(ctx, "team", "x.png", []byte("not an image at all")); !errors.Is(err, ErrNotImage) {
		t.Fatalf("non-image stage = %v, want ErrNotImage", err)
	}
	huge := pngBytes(string(make([]byte, wstracker.DefaultAttachmentMax)))
	if _, err := a.StageAttachment(ctx, "team", "x.png", huge); !errors.Is(err, ErrAttachmentTooLarge) {
		t.Fatalf("oversize stage = %v, want ErrAttachmentTooLarge", err)
	}

	data := pngBytes("screenshot")
	att := stageOK(t, a, "team", "/tmp/dir/shot.png", data)
	if att.OID != attachmentOID(data) || att.Name != "shot.png" || att.Size != int64(len(data)) || att.Mime != "image/png" {
		t.Fatalf("staged = %+v", att)
	}
	if got := stageOK(t, a, "team", "", data).Name; got != defaultAttachmentName {
		t.Fatalf("nameless upload named %q", got)
	}
	dir := a.stores["team"].entry.Dir
	if _, err := os.Stat(stagedPath(dir, att.OID)); err != nil {
		t.Fatalf("staged copy: %v", err)
	}
	if got, err := a.ReadAttachment(ctx, "team", att.OID); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("read of a staged blob = %q, %v", got, err)
	}

	unknown := att
	unknown.OID = attachmentOID([]byte("never uploaded"))
	hasAtt := func(h *Host, oid string, want bool, when string) {
		t.Helper()
		if got, err := h.HasAttachment(ctx, "team", oid); err != nil || got != want {
			t.Fatalf("HasAttachment(%s) %s = %v, %v; want %v", oid, when, got, err, want)
		}
	}
	hasAtt(a, att.OID, true, "staged")
	hasAtt(a, unknown.OID, false, "never uploaded")
	hasAtt(a, "zz", false, "malformed")
	hasAtt(b, att.OID, false, "staged on another machine")
	var refusal *wstracker.Refusal
	if _, err := createWithAttachment(t, a, "team", unknown); !errors.As(err, &refusal) {
		t.Fatalf("dispatch naming an unstaged oid = %v, want a refusal", err)
	}

	ids, err := createWithAttachment(t, a, "team", att)
	if err != nil {
		t.Fatalf("dispatch with a staged attachment: %v", err)
	}
	if _, err := os.Stat(stagedPath(dir, att.OID)); !os.IsNotExist(err) {
		t.Fatalf("staged copy after the dispatch: %v, want gone", err)
	}
	if got, err := a.ReadAttachment(ctx, "team", att.OID); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("read of a pending blob = %q, %v", got, err)
	}
	hasAtt(a, att.OID, true, "pending")
	if atts := view(t, a, "team").Items[ids[0].String()].Attachments; len(atts) != 1 || atts[0].OID != att.OID {
		t.Fatalf("item attachments = %+v", atts)
	}

	// A later edit may name the committed oid again without re-uploading.
	syncStore(t, a, "team")
	if _, err := createWithAttachment(t, a, "team", att); err != nil {
		t.Fatalf("dispatch naming a stored oid: %v", err)
	}

	if _, err := b.Check(ctx, "team"); err != nil {
		t.Fatal(err)
	}
	if !hasItem(view(t, b, "team"), ids[0].String()) {
		t.Fatal("bo did not receive the item")
	}
	hasAtt(b, att.OID, true, "stored by another member")
	if got, err := b.ReadAttachment(ctx, "team", att.OID); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("bo's read = %q, %v", got, err)
	}
	if _, err := b.ReadAttachment(ctx, "team", unknown.OID); !errors.Is(err, ErrAttachmentNotFound) {
		t.Fatalf("bo's read of an unknown oid = %v, want ErrAttachmentNotFound", err)
	}
	if _, err := b.ReadAttachment(ctx, "team", "zz"); !errors.Is(err, ErrAttachmentNotFound) {
		t.Fatalf("read of a malformed oid = %v, want ErrAttachmentNotFound", err)
	}
}

// TestAttachmentsStagedExpiry: a staged copy no dispatch claimed is deleted
// once it is older than stageExpiry, by the sweep a poll round runs; a
// younger one stays.
func TestAttachmentsStagedExpiry(t *testing.T) {
	w := newWorld(t)
	a := w.machine("a")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	dir := a.stores["team"].entry.Dir

	old := stageOK(t, a, "team", "old.png", pngBytes("old"))
	young := stageOK(t, a, "team", "young.png", pngBytes("young"))
	// A staged copy a pending event names (the dispatch deleted it; here it
	// is written back, as after a crash between the append and the delete).
	// The store has no remote, so the push delay cannot commit the event
	// under the test.
	if _, err := a.Register(context.Background(), RegisterRequest{ID: "mine", Member: "ada", Profile: &wstracker.Profile{Given: "Ada"}}); err != nil {
		t.Fatal(err)
	}
	mine := a.stores["mine"].entry.Dir
	named := stageOK(t, a, "mine", "named.png", pngBytes("named"))
	if _, err := createWithAttachment(t, a, "mine", named); err != nil {
		t.Fatal(err)
	}
	if err := writeStaged(mine, named.OID, pngBytes("named")); err != nil {
		t.Fatal(err)
	}
	w.clock.Advance(2 * time.Hour) // the hourly sweep is due again
	stale := w.clock.Now().Add(-stageExpiry - time.Minute)
	for _, path := range []string{stagedPath(dir, old.OID), stagedPath(mine, named.OID)} {
		if err := os.Chtimes(path, stale, stale); err != nil {
			t.Fatal(err)
		}
	}
	if n := a.stores["mine"].ledger.Pending().Count; n != 1 {
		t.Fatalf("precondition: %d pending events, want the one naming the staged copy", n)
	}
	a.PollOnce(context.Background())
	if _, err := os.Stat(stagedPath(dir, old.OID)); !os.IsNotExist(err) {
		t.Fatalf("expired staged copy: %v, want gone", err)
	}
	if _, err := os.Stat(stagedPath(dir, young.OID)); err != nil {
		t.Fatalf("young staged copy: %v, want kept", err)
	}
	if _, err := os.Stat(stagedPath(mine, named.OID)); err != nil {
		t.Fatalf("staged copy named by a pending event: %v, want kept", err)
	}
}
