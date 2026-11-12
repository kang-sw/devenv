package wsdashboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The session display label: a name the user gives one session on the
// dashboard, shown in place of its reply-id in the session page's head, its
// tab title, the session rail, and the index list. It is display only: the
// reply-id stays the session's address, URL, and mailbox id everywhere else.
//
// The label lives in the session's registry directory (SessionDir) as
// labelFileName, written only by POST /s/<reply-id>/label {"label"}. The
// route takes the board-answer write routes' gate (a POST carrying the
// daemon's own Origin, for a session the index holds) and needs no live
// adapter, so an ended session can be renamed too. An empty label removes
// the file, which shows the reply-id again. The index reads the file on each
// re-read of the session (Session.Label), so a change raises the session's
// revision and every page's head and rail re-render through their streams.
// A missing or unreadable file is no label; an older daemon that does not
// know the file leaves it alone, and the registry prunes it with the
// session's directory.

const (
	// labelFileName is the label's file in a session directory.
	labelFileName = "label.json"
	// maxLabelRunes bounds a label's length, in characters.
	maxLabelRunes = 64
	// labelBodyLimit bounds the label route's body.
	labelBodyLimit = 4 << 10
)

// labelRecord is the label file's content. Readers ignore unknown fields.
type labelRecord struct {
	Label string `json:"label"`
}

// errLabelTooLong and errLabelControl are cleanLabel's refusals.
var (
	errLabelTooLong = fmt.Errorf("a label holds at most %d characters", maxLabelRunes)
	errLabelControl = errors.New("a label is one line of text: no line breaks or control characters")
)

// cleanLabel trims raw and checks it is a label: valid UTF-8, one line
// without control characters, and at most maxLabelRunes characters. An empty
// result means no label.
func cleanLabel(raw string) (string, error) {
	label := strings.TrimSpace(raw)
	if !utf8.ValidString(label) {
		return "", errLabelControl
	}
	for _, r := range label {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return "", errLabelControl
		}
	}
	if utf8.RuneCountInString(label) > maxLabelRunes {
		return "", errLabelTooLong
	}
	return label, nil
}

// readSessionLabel reads the label in the session directory sessionDir; a
// missing, unreadable, or invalid file reads as no label.
func readSessionLabel(sessionDir string) string {
	raw, err := os.ReadFile(filepath.Join(sessionDir, labelFileName))
	if err != nil {
		return ""
	}
	var rec labelRecord
	if json.Unmarshal(raw, &rec) != nil {
		return ""
	}
	label, err := cleanLabel(rec.Label)
	if err != nil {
		return ""
	}
	return label
}

// writeSessionLabel replaces the label in sessionDir atomically, or removes
// it when label is empty. label is already clean (cleanLabel).
func writeSessionLabel(sessionDir, label string) error {
	if label == "" {
		err := os.Remove(filepath.Join(sessionDir, labelFileName))
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	raw, err := json.Marshal(labelRecord{Label: label})
	if err != nil {
		return err
	}
	return writePrivateFile(sessionDir, labelFileName, append(raw, '\n'))
}

// handleSessionLabel serves POST /s/<reply-id>/label: it sets or clears the
// session's display label and answers {"label"}, the label as stored.
func (d *Daemon) handleSessionLabel(w http.ResponseWriter, r *http.Request, replyID string) {
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
	if _, ok := d.index.Session(replyID); !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, labelBodyLimit))
	if err != nil {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	var req labelRecord
	if err := json.Unmarshal(raw, &req); err != nil {
		http.Error(w, "malformed request body", http.StatusBadRequest)
		return
	}
	label, err := cleanLabel(req.Label)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := writeSessionLabel(SessionDir(d.dir, replyID), label); err != nil {
		fmt.Fprintf(d.log, "dashboard: write label %s: %v\n", replyID, err)
		http.Error(w, "the label could not be saved", http.StatusInternalServerError)
		return
	}
	// The re-read finds the label changed, raises the session's revision,
	// and wakes every stream that shows it.
	d.index.Reread(replyID)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, labelRecord{Label: label})
}
