package wsdashboard

import (
	"os"
	"strings"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

// HeldView is a message the user sent from the page that the session has
// not received yet: it sits in the session's board-answer inbox until the
// host adapter drains it at the end of a turn, and can be cancelled until
// then.
type HeldView struct {
	// ID is the inbox entry's ID, which the cancel control names.
	ID string
	// At is the first send's time (epoch ms).
	At int64
	// Parts are the merged sends' texts; the bubble divides them.
	Parts []string
	Chips []wsinbox.Chip
	// Images are the entry's attached images.
	Images []AttachmentView
}

// AttachmentView is one image a message carries. Src is the attachment
// route's URL when Path names a file in this session's attachments
// directory; the page shows any other path as text.
type AttachmentView struct {
	Path, Name, Src string
}

// attachmentView maps an image path to its view: a thumbnail for a file
// that exists in attachDir, the session replyID's attachments directory,
// else the path, so a removed attachment shows as text, not a broken image.
func attachmentView(attachDir, replyID, path string) AttachmentView {
	if name, ok := wsinbox.AttachmentName(attachDir, path); ok && ValidReplyID(replyID) && regularFile(path) {
		return AttachmentView{Path: path, Name: name, Src: AttachmentSrc(replyID, name)}
	}
	return AttachmentView{Path: path}
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// userEntryImages takes each user entry's attached images out of its text
// (userImages).
func userEntryImages(entries []ConvEntry, attachDir, replyID string) {
	for i, e := range entries {
		if e.Kind == wslive.RingUser {
			entries[i].Text, entries[i].Images = userImages(e.Text, attachDir, replyID)
		}
	}
}

// userImages takes the image lines (wsinbox.ImageLine) that name this
// session's attachments out of a delivered user message, returning the text
// left and their views. A line naming any other path stays in the text.
func userImages(text, attachDir, replyID string) (string, []AttachmentView) {
	if !strings.Contains(text, "[image: ") {
		return text, nil
	}
	var kept []string
	var images []AttachmentView
	for _, line := range strings.Split(text, "\n") {
		if path, ok := wsinbox.ImageLinePath(strings.TrimSpace(line)); ok {
			if v := attachmentView(attachDir, replyID, path); v.Src != "" {
				images = append(images, v)
				continue
			}
		}
		// Keep one blank line where a removed block left two.
		if strings.TrimSpace(line) == "" && len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
			continue
		}
		kept = append(kept, line)
	}
	if len(images) == 0 {
		return text, nil
	}
	return strings.Trim(strings.Join(kept, "\n"), "\n"), images
}

// SentMark is the latest answer sent for one board item, from the inbox:
// Held while the message holding it is undrained.
type SentMark struct {
	ID, Answer, Note string
	Held             bool
}

// heldViews lists the held entries, oldest first; attachDir and replyID
// map their images as attachmentView does.
func heldViews(held []wsinbox.Record, attachDir, replyID string) []HeldView {
	out := make([]HeldView, 0, len(held))
	for _, r := range held {
		v := HeldView{ID: r.ID(), At: r.Entry.SentAt, Parts: r.Entry.Parts, Chips: r.Entry.Chips}
		for _, path := range r.Entry.Images {
			v.Images = append(v.Images, attachmentView(attachDir, replyID, path))
		}
		out = append(out, v)
	}
	return out
}

// sentMarks derives each board item's sent mark from the inbox: the newest
// chip for the item across the drained records and then the held entries, so
// a held answer shows over an earlier delivered one. The drained records are
// on disk, so a mark survives a page reload while the newest DrainedCap
// records hold it.
func sentMarks(drained, held []wsinbox.Record) []SentMark {
	var order []string
	marks := map[string]SentMark{}
	add := func(records []wsinbox.Record, isHeld bool) {
		for _, r := range records {
			for _, c := range r.Entry.Chips {
				if _, seen := marks[c.ID]; !seen {
					order = append(order, c.ID)
				}
				marks[c.ID] = SentMark{ID: c.ID, Answer: c.Answer, Note: c.Note, Held: isHeld}
			}
		}
	}
	add(drained, false)
	add(held, true)
	out := make([]SentMark, 0, len(order))
	for _, id := range order {
		out = append(out, marks[id])
	}
	return out
}
