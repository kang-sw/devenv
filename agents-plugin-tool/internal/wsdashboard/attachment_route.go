package wsdashboard

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/kang-sw/devenv/internal/wsinbox"
)

// The attachment route: GET /s/<reply-id>/attachment/<name> serves an image
// the user attached to a message from the page, from that session's inbox
// attachments directory. name must match the attachment name grammar
// (wsinbox.ValidAttachmentName), so it never names another path, and the file
// must be a regular file there, not a symlink. It has the image route's
// access rules: the daemon's guard admits a loopback browser, or the LAN
// token in LAN mode. Every refusal answers 404.

// AttachmentSrc is the attachment route URL for the attachment name of the
// session replyID.
func AttachmentSrc(replyID, name string) string {
	return "/s/" + replyID + "/attachment/" + name
}

// handleAttachment serves the attachment route of one session.
func (d *Daemon) handleAttachment(w http.ResponseWriter, r *http.Request, replyID, name string) {
	if d.index == nil || !ValidReplyID(replyID) || !wsinbox.ValidAttachmentName(name) {
		http.Error(w, "attachment not found", http.StatusNotFound)
		return
	}
	if _, ok := d.index.Session(replyID); !ok {
		http.Error(w, "attachment not found", http.StatusNotFound)
		return
	}
	ctype, ok := imageType(name)
	if !ok {
		http.Error(w, "attachment not found", http.StatusNotFound)
		return
	}
	path := filepath.Join(wsinbox.AttachmentsDir(sessionInbox(d.dir, replyID)), name)
	// Lstat before opening: a symlink is refused, and opening a FIFO would
	// block the handler.
	if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() {
		http.Error(w, "attachment not found", http.StatusNotFound)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "attachment not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "attachment not found", http.StatusNotFound)
		return
	}
	h := w.Header()
	h.Set("Content-Type", ctype)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", imageContentSecurityPolicy)
	h.Set("Cache-Control", "no-cache")
	h.Set("Referrer-Policy", "no-referrer")
	http.ServeContent(w, r, "", info.ModTime(), f)
}
