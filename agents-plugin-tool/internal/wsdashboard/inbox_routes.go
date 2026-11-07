package wsdashboard

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
)

// The browser write routes: POST /s/<reply-id>/message holds a message the
// user sent from the composer in the session's board-answer inbox (merged
// into the held one when there is one), and POST /s/<reply-id>/cancel takes a
// held message back. The inbox is the daemon's one write target; the
// session's host adapter drains it at the end of a turn and injects the
// message as the user's own. Both routes are offered only for a session with
// a live host adapter, require an Origin equal to the daemon's own (which
// stops cross-site form posts), and send no CORS headers.
//
// A message may attach images: new ones as base64 data, and ones a cancel
// returned to the composer as references to their already written files.
// The route writes the new ones into the inbox's attachments directory in the
// same request that holds the message, and the held entry lists their
// absolute paths; the session receives each as an "[image: <path>]" line it
// opens with its own read tool.

const (
	// inboxBodyLimit bounds a write route's request body, and a message's
	// body when it attaches no image.
	inboxBodyLimit = 256 << 10
	// maxImageBytes bounds one attached image, decoded.
	maxImageBytes = 10 << 20
	// attachmentSweepAge is how old an attachment no entry lists must be
	// before a message write sweeps it (wsinbox.SweepAttachments).
	attachmentSweepAge = 24 * time.Hour
)

// messageBodyLimit bounds a message's body when it attaches images: every
// image at its cap as base64, plus the text-only limit.
var messageBodyLimit = inboxBodyLimit + wsinbox.MaxImages*base64.StdEncoding.EncodedLen(maxImageBytes)

// messageRequest is the message route's body. Unknown fields are ignored.
type messageRequest struct {
	Text  string         `json:"text"`
	Chips []wsinbox.Chip `json:"chips"`
	// Images are new images; Refs name files a cancel returned, inside this
	// session's attachments directory.
	Images []imageUpload `json:"images"`
	Refs   []string      `json:"refs"`
}

// imageUpload is one new image: the browser's file name, used only in
// refusals, and the file's bytes as standard base64.
type imageUpload struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

// decodedImage is an upload that passed the checks, with the extension of
// its sniffed type.
type decodedImage struct {
	ext  string
	data []byte
}

// sniffedExt maps the content types the route accepts, as
// http.DetectContentType sniffs them, to the file extension it writes: the
// formats both hosts' read tools open as images.
var sniffedExt = map[string]string{
	"image/png":  "png",
	"image/jpeg": "jpg",
	"image/gif":  "gif",
	"image/webp": "webp",
}

// sessionInbox is the absolute inbox directory of the session replyID in
// the state directory dir. It is absolute because an entry lists its images
// by absolute path, which the page then matches against it.
func sessionInbox(dir, replyID string) string {
	inbox := wsinbox.Dir(SessionDir(dir, replyID))
	if abs, err := filepath.Abs(inbox); err == nil {
		return abs
	}
	return inbox
}

// cancelRequest is the cancel route's body.
type cancelRequest struct {
	ID string `json:"id"`
}

// sameOrigin reports whether r carries an Origin equal to the daemon's own
// origin as the request addressed it: http and the request's Host. A missing
// or opaque ("null") Origin is not the daemon's.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" || origin == "null" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

// handleInboxWrite serves the message and cancel routes of one session.
func (d *Daemon) handleInboxWrite(w http.ResponseWriter, r *http.Request, replyID, route string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return
	}
	if d.index == nil || !ValidReplyID(replyID) {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	s, ok := d.index.Session(replyID)
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	now := time.Now()
	if !adapterPresent(d.pages().readSnapshot(replyID), now) {
		http.Error(w, "this session has no live host adapter: copy the answers into its terminal instead", http.StatusConflict)
		return
	}
	limit := inboxBodyLimit
	if route == "message" {
		limit = messageBodyLimit
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, int64(limit)+1))
	if err != nil {
		http.Error(w, "request body unreadable", http.StatusBadRequest)
		return
	}
	if len(raw) > limit {
		status := http.StatusBadRequest
		if route == "message" {
			status = http.StatusRequestEntityTooLarge
		}
		http.Error(w, "request body too large", status)
		return
	}
	dir := sessionInbox(d.dir, replyID)

	var status int
	var answer any
	if route == "message" {
		status, answer = d.messageWrite(dir, raw, now)
	} else {
		d.inboxMu.Lock()
		status, answer = d.inboxWrite(dir, route, raw)
		d.inboxMu.Unlock()
	}

	if status == http.StatusOK {
		d.index.Nudge(replyID, s.Project)
		d.broker.wake(replyID)
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, status, answer)
		return
	}
	http.Error(w, fmt.Sprint(answer), status)
}

// messageWrite performs the message route against the inbox at dir. It
// parses and checks the body, decoding and sniffing every new image, before
// it takes d.inboxMu for the writes.
func (d *Daemon) messageWrite(dir string, raw []byte, now time.Time) (int, any) {
	var req messageRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return http.StatusBadRequest, "malformed message"
	}
	if len(req.Images) == 0 && len(req.Refs) == 0 && len(raw) > inboxBodyLimit {
		return http.StatusBadRequest, "request body too large"
	}
	if n := len(req.Images) + len(req.Refs); n > wsinbox.MaxImages {
		return http.StatusRequestEntityTooLarge, fmt.Sprintf("%d images attached: a message holds at most %d", n, wsinbox.MaxImages)
	}
	images := make([]decodedImage, 0, len(req.Images))
	for _, up := range req.Images {
		img, status, msg := decodeImage(up)
		if status != http.StatusOK {
			return status, msg
		}
		images = append(images, img)
	}
	d.inboxMu.Lock()
	defer d.inboxMu.Unlock()
	return d.sendMessage(dir, req, images, now)
}

// decodeImage checks one new image: valid base64, at most maxImageBytes
// decoded, and content that sniffs as one of sniffedExt's types, whatever
// the browser labelled it.
func decodeImage(up imageUpload) (decodedImage, int, string) {
	label := up.Name
	if len(label) > 80 {
		label = label[:80]
	}
	if base64.StdEncoding.DecodedLen(len(up.Data)) > maxImageBytes+2 {
		return decodedImage{}, http.StatusRequestEntityTooLarge, fmt.Sprintf("image %q is larger than %d MiB", label, maxImageBytes>>20)
	}
	data, err := base64.StdEncoding.DecodeString(up.Data)
	if err != nil || len(data) == 0 {
		return decodedImage{}, http.StatusBadRequest, fmt.Sprintf("image %q is not valid base64 data", label)
	}
	if len(data) > maxImageBytes {
		return decodedImage{}, http.StatusRequestEntityTooLarge, fmt.Sprintf("image %q is larger than %d MiB", label, maxImageBytes>>20)
	}
	ext, ok := sniffedExt[http.DetectContentType(data)]
	if !ok {
		return decodedImage{}, http.StatusUnsupportedMediaType, fmt.Sprintf("image %q is not a PNG, JPEG, GIF, or WebP image", label)
	}
	return decodedImage{ext: ext, data: data}, http.StatusOK, ""
}

// sendMessage holds one checked message in the inbox at dir; d.inboxMu is
// held. It sweeps old unreferenced attachments, resolves the references,
// checks the merged entry's image cap before writing any file, writes the new
// images, and sends. A refusal after files were written deletes them.
func (d *Daemon) sendMessage(dir string, req messageRequest, images []decodedImage, now time.Time) (int, any) {
	wsinbox.SweepAttachments(dir, now, attachmentSweepAge)
	attachDir := wsinbox.AttachmentsDir(dir)
	var paths []string
	for _, ref := range req.Refs {
		if !wsinbox.ValidAttachmentName(ref) {
			return http.StatusBadRequest, fmt.Sprintf("attachment %q is not an image of this session", ref)
		}
		path := filepath.Join(attachDir, ref)
		if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() {
			return http.StatusBadRequest, fmt.Sprintf("attachment %q no longer exists: attach the image again", ref)
		}
		paths = append(paths, path)
	}
	if count := len(paths) + len(images); count > 0 {
		// A send merges into the newest held entry, so the cap applies to
		// the two together. A drain racing this can only make the count
		// over-strict.
		if held, _ := wsinbox.Held(dir); len(held) > 0 {
			count += len(held[len(held)-1].Entry.Images)
		}
		if count > wsinbox.MaxImages {
			return http.StatusRequestEntityTooLarge, fmt.Sprintf("the held message would carry %d images: a message holds at most %d", count, wsinbox.MaxImages)
		}
	}
	written, err := writeAttachments(attachDir, images)
	if err != nil {
		fmt.Fprintf(d.log, "dashboard: write attachments: %v\n", err)
		return http.StatusInternalServerError, "could not store the images"
	}
	entry := wsinbox.Entry{Parts: []string{req.Text}, Chips: req.Chips, Images: append(paths, written...)}
	name, merged, err := wsinbox.Send(dir, entry, now)
	if err != nil {
		removeFiles(written)
		if errors.Is(err, wsinbox.ErrEmpty) || errors.Is(err, wsinbox.ErrInvalid) {
			return http.StatusBadRequest, err.Error()
		}
		fmt.Fprintf(d.log, "dashboard: inbox send: %v\n", err)
		return http.StatusInternalServerError, "could not hold the message"
	}
	return http.StatusOK, map[string]any{"id": wsinbox.Record{Name: name}.ID(), "merged": merged}
}

// writeAttachments writes images into attachDir as
// <upload-id>-<n>.<ext>, with one upload id minted for this request, and
// returns their absolute paths. On an error it removes what it wrote.
func writeAttachments(attachDir string, images []decodedImage) ([]string, error) {
	if len(images) == 0 {
		return nil, nil
	}
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	uploadID := hex.EncodeToString(buf)
	if err := os.MkdirAll(attachDir, 0o755); err != nil {
		return nil, err
	}
	var written []string
	for i, img := range images {
		path := filepath.Join(attachDir, fmt.Sprintf("%s-%d.%s", uploadID, i+1, img.ext))
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			removeFiles(written)
			return nil, err
		}
		written = append(written, path)
		_, werr := f.Write(img.data)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			removeFiles(written)
			return nil, werr
		}
	}
	return written, nil
}

func removeFiles(paths []string) {
	for _, p := range paths {
		_ = os.Remove(p)
	}
}

// inboxWrite performs the cancel route against the inbox at dir; d.inboxMu
// is held, so the daemon's own sends and cancels never race each other, and
// the inbox's claims settle a race with the adapter's drain.
func (d *Daemon) inboxWrite(dir, route string, raw []byte) (int, any) {
	switch route {
	case "cancel":
		var req cancelRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return http.StatusBadRequest, "malformed cancel"
		}
		name, ok := wsinbox.NameForID(req.ID)
		if !ok {
			return http.StatusBadRequest, "malformed message id"
		}
		entry, err := wsinbox.Cancel(dir, name)
		if errors.Is(err, wsinbox.ErrGone) {
			return http.StatusConflict, "the session already received this message"
		}
		if err != nil {
			fmt.Fprintf(d.log, "dashboard: inbox cancel: %v\n", err)
			return http.StatusInternalServerError, "could not cancel the message"
		}
		chips := entry.Chips
		if chips == nil {
			chips = []wsinbox.Chip{}
		}
		// The images stay on disk: the page gets their names back as chips
		// that reference the files, and a re-send references them.
		images := []string{}
		attachDir := wsinbox.AttachmentsDir(dir)
		for _, path := range entry.Images {
			if name, ok := wsinbox.AttachmentName(attachDir, path); ok {
				images = append(images, name)
			}
		}
		return http.StatusOK, map[string]any{"text": strings.Join(entry.Parts, "\n\n"), "chips": chips, "images": images}
	}
	return http.StatusNotFound, "not found"
}
