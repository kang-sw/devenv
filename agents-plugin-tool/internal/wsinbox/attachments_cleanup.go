// Cleanup of attachments no longer referenced by inbox entries.
package wsinbox

import (
	"github.com/kang-sw/devenv/internal/wsstate"
	"os"
	"path/filepath"
	"time"
)

// trimDrained trims the drained records of the inbox at dir to the newest
// DrainedCap, then deletes the images the trimmed records listed that no
// held entry and no remaining record lists.
func trimDrained(dir string) {
	drained := DrainedDir(dir)
	names, err := entryNames(drained)
	if err != nil || len(names) <= DrainedCap {
		return
	}
	attachDir := AttachmentsDir(dir)
	var listed []string
	for _, name := range names[:len(names)-DrainedCap] {
		if e, err := readEntry(filepath.Join(drained, name)); err == nil {
			for _, path := range e.Images {
				if n, ok := AttachmentName(attachDir, path); ok {
					listed = append(listed, n)
				}
			}
		}
		_ = remove(filepath.Join(drained, name))
	}
	if len(listed) == 0 {
		return
	}
	referenced := referencedAttachments(dir)
	for _, n := range listed {
		if !referenced[n] {
			_ = os.Remove(filepath.Join(attachDir, n))
		}
	}
}

// referencedAttachments is the set of attachment names some entry of the
// inbox at dir lists. It reads the held side, then the drained records, then
// the held side again: a drain moves an entry from held to drained, so an
// entry that moves between the reads is seen in one of them. The held side
// is every file directly in dir that parses as an entry, so an entry a steer
// or cancel has claimed, or a steer is writing, counts too.
func referencedAttachments(dir string) map[string]bool {
	attachDir := AttachmentsDir(dir)
	out := map[string]bool{}
	add := func(e Entry) {
		for _, path := range e.Images {
			if n, ok := AttachmentName(attachDir, path); ok {
				out[n] = true
			}
		}
	}
	readHeldSide := func() {
		des, err := wsstate.ReadDir(dir)
		if err != nil {
			return
		}
		for _, de := range des {
			if de.IsDir() {
				continue
			}
			if e, err := readEntry(filepath.Join(dir, de.Name())); err == nil {
				add(e)
			}
		}
	}
	readHeldSide()
	if records, err := readAll(DrainedDir(dir)); err == nil {
		for _, r := range records {
			add(r.Entry)
		}
	}
	readHeldSide()
	return out
}

// SweepAttachments deletes each attachment of the inbox at dir that was last
// modified more than maxAge before now and that no held entry, no drained
// record, and no name in keep lists: an image whose message was cancelled and
// never sent again. The dashboard daemon runs it under its inbox lock on each
// message write, keeping the attachments that write references.
func SweepAttachments(dir string, now time.Time, maxAge time.Duration, keep ...string) {
	attachDir := AttachmentsDir(dir)
	des, err := wsstate.ReadDir(attachDir)
	if err != nil {
		return
	}
	var old []string
	for _, de := range des {
		if !de.Type().IsRegular() || !ValidAttachmentName(de.Name()) {
			continue
		}
		info, err := de.Info()
		if err != nil || now.Sub(info.ModTime()) <= maxAge {
			continue
		}
		old = append(old, de.Name())
	}
	if len(old) == 0 {
		return
	}
	referenced := referencedAttachments(dir)
	for _, n := range keep {
		referenced[n] = true
	}
	for _, n := range old {
		if !referenced[n] {
			_ = os.Remove(filepath.Join(attachDir, n))
		}
	}
}
