// tracker_attachment_routes.go holds attachment upload and image read routes.
package wsdashboard

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
)

// trackerAttachmentTypes are the content types the attachment read serves.
var trackerAttachmentTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
}

// validAttachmentOID reports an oid of 64 lowercase hex digits.
func validAttachmentOID(oid string) bool {
	if len(oid) != 64 {
		return false
	}
	for i := 0; i < len(oid); i++ {
		if c := oid[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// trackerStageAttachment serves the attachment upload of store id. Staging
// changes no view, so it does not bump the revision; the dispatch naming
// the attachment does.
func (d *Daemon) trackerStageAttachment(ctx context.Context, w http.ResponseWriter, r *http.Request, id string) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, trackerAttachmentBodyLimit))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			trackerError(w, http.StatusRequestEntityTooLarge, "attachment too large")
		} else {
			trackerError(w, http.StatusBadRequest, "unreadable request body")
		}
		return
	}
	att, err := d.tracker.StageAttachment(ctx, id, r.URL.Query().Get("name"), data)
	switch {
	case errors.Is(err, ErrTrackerNotImage):
		trackerError(w, http.StatusUnsupportedMediaType, err.Error())
	case errors.Is(err, ErrTrackerAttachmentTooLarge):
		trackerError(w, http.StatusRequestEntityTooLarge, err.Error())
	case err != nil:
		// Not the upload's fault: the store's write lock or disk.
		trackerError(w, http.StatusInternalServerError, err.Error())
	default:
		writeJSON(w, http.StatusOK, att)
	}
}

// trackerReadAttachment serves attachment oid of store id.
func (d *Daemon) trackerReadAttachment(w http.ResponseWriter, r *http.Request, id, oid string) {
	if crossSiteFetch(r) {
		trackerError(w, http.StatusNotFound, "not found")
		return
	}
	data, err := d.tracker.ReadAttachment(r.Context(), id, oid)
	serveTrackerImage(w, data, err)
}

// serveTrackerImage answers an image read: data when it sniffs as an
// accepted image type, else 404, as for ErrTrackerAttachmentNotFound; 503
// for a busy store.
func serveTrackerImage(w http.ResponseWriter, data []byte, err error) {
	if errors.Is(err, ErrTrackerAttachmentNotFound) {
		trackerError(w, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, ErrTrackerBusy) {
		trackerBusy(w, err)
		return
	}
	if err != nil {
		trackerError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ctype := http.DetectContentType(data)
	if !trackerAttachmentTypes[ctype] {
		trackerError(w, http.StatusNotFound, "not found")
		return
	}
	h := w.Header()
	h.Set("Content-Type", ctype)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", imageContentSecurityPolicy)
	h.Set("Cache-Control", "private, max-age=31536000, immutable")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
	h.Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
