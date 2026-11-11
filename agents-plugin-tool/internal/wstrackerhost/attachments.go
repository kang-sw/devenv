package wstrackerhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
)

// Attachments. A browser uploads an image before the event that names it:
// StageAttachment keeps the bytes at <store dir>/staged/<sha256> and hands
// back the {oid, name, size, mime} the event's attachments field carries.
// DispatchWith then loads the staged bytes the batch names and writes them
// with the events (wsledger.Blob); the staged copy is deleted once the
// append holds them. An oid that is neither staged nor already stored in
// an attachment tree is refused, so an event never names content no clone
// can fetch. Staged files a dispatch never claimed expire after stageExpiry
// (unless a pending event of this clone still names them); the sweep runs
// at store open and about hourly, under the store's write lock like every
// change to the staged directory.
//
// ReadAttachment serves a blob by oid: a verified attachment tree first
// (the referencing author's before any other), then this clone's pending
// writes, then the staged copy. Every path checks the content's sha256
// against the oid, and the type is read from the bytes, never from a
// stored name.

const (
	stagedDir   = "staged"
	stageExpiry = 24 * time.Hour
	sweepEvery  = time.Hour
	// defaultAttachmentName is the name of an upload without one.
	defaultAttachmentName = "image"
	maxAttachmentName     = 255
)

var (
	// ErrAttachmentTooLarge: the upload exceeds the store's
	// attachments.max_bytes.
	ErrAttachmentTooLarge = errors.New("tracker: attachment exceeds attachments.max_bytes")
	// ErrNotImage: the upload's bytes are not a PNG, JPEG, GIF, or WebP.
	ErrNotImage = errors.New("tracker: attachment is not a supported image")
	// ErrAttachmentNotFound: no stored or staged blob hashes to the oid.
	ErrAttachmentNotFound = errors.New("tracker: attachment not found")
)

// Attachment is a staged upload as the event's attachments field names it.
type Attachment struct {
	OID  string `json:"oid"`
	Name string `json:"name"`
	Size int64  `json:"size"`
	Mime string `json:"mime"`
}

// imageMime is the image type the bytes declare (their magic numbers), or
// "" when they are not an accepted image.
func imageMime(data []byte) string {
	switch mime := http.DetectContentType(data); mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return mime
	}
	return ""
}

func attachmentOID(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// stagedPath is the staged copy of oid in a store directory.
func stagedPath(dir, oid string) string { return filepath.Join(dir, stagedDir, oid) }

// StageAttachment stores an upload for a later dispatch of store id. The
// name is reduced to its base and cut to maxAttachmentName bytes
// (defaultAttachmentName when empty); the bytes must be an accepted image
// (ErrNotImage) within the store's attachments.max_bytes
// (ErrAttachmentTooLarge).
func (h *Host) StageAttachment(ctx context.Context, id, name string, data []byte) (Attachment, error) {
	mime := imageMime(data)
	if mime == "" {
		return Attachment{}, ErrNotImage
	}
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = defaultAttachmentName
	}
	if len(name) > maxAttachmentName {
		name = name[:maxAttachmentName]
	}
	att := Attachment{OID: attachmentOID(data), Name: name, Size: int64(len(data)), Mime: mime}
	err := h.with(ctx, id, func(st *store) error {
		st.tracker.Refresh()
		if limit := st.tracker.View(h.now(), 0).Attachments.MaxBytes; float64(len(data)) > limit {
			return fmt.Errorf("%w: %d > %.0f bytes", ErrAttachmentTooLarge, len(data), limit)
		}
		return h.withWriteLock(ctx, st, func() error {
			if err := writeStaged(st.entry.Dir, att.OID, data); err != nil {
				return err
			}
			h.sweepStagedIfDue(st)
			return nil
		})
	})
	return att, err
}

// writeStaged writes the staged copy of oid atomically.
func writeStaged(dir, oid string, data []byte) error {
	path := stagedPath(dir, oid)
	// Mkdir, not MkdirAll: a removed store directory is never recreated.
	if err := os.Mkdir(filepath.Dir(path), 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// readStaged returns the staged copy of oid when it exists and still hashes
// to it.
func readStaged(dir, oid string) ([]byte, bool) {
	if !wsledger.ValidAttachmentOID(oid) {
		return nil, false
	}
	data, err := os.ReadFile(stagedPath(dir, oid))
	if err != nil || attachmentOID(data) != oid {
		return nil, false
	}
	return data, true
}

// ReadAttachment returns the content of attachment oid of store id, or
// ErrAttachmentNotFound. A copy this clone holds (stored or staged) is
// served under the store's lock alone, so it never waits for a fetch or
// push in flight. A blob this clone lacks is fetched from the remote
// outside that lock, under the store's network lock and networkTimeout: a
// stalled transfer must not outlast the bound whatever the caller's
// context, and a caller that bounds its lock wait (WithLockWait) is
// answered ErrStoreBusy while another fetch or push holds the network.
func (h *Host) ReadAttachment(ctx context.Context, id, oid string) ([]byte, error) {
	if !wsledger.ValidAttachmentOID(oid) {
		return nil, ErrAttachmentNotFound
	}
	var data []byte
	author, fetch := "", false
	err := h.with(ctx, id, func(st *store) error {
		if _, err := st.ledger.Reload(ctx); err != nil {
			return err
		}
		author = attachmentAuthor(st.tracker.View(h.now(), 0), oid)
		var err error
		data, err = st.ledger.ReadLocalAttachment(ctx, oid, author)
		if err == nil {
			return nil
		}
		notLocal := errors.Is(err, wsledger.ErrAttachmentNotLocal)
		if !notLocal && !errors.Is(err, wsledger.ErrAttachmentNotFound) {
			return err
		}
		if staged, ok := readStaged(st.entry.Dir, oid); ok {
			data = staged
			return nil
		}
		if notLocal {
			fetch = true
			return nil
		}
		return fmt.Errorf("%w: %v", ErrAttachmentNotFound, err)
	})
	if err != nil || !fetch {
		return data, err
	}
	err = h.withNet(ctx, id, func(ctx context.Context, st *store, e StoreEntry, l *wsledger.Ledger) error {
		fetchCtx, cancel := context.WithTimeout(ctx, networkTimeout)
		defer cancel()
		var err error
		data, err = l.ReadAttachment(fetchCtx, oid, author)
		if errors.Is(err, wsledger.ErrAttachmentNotFound) {
			return fmt.Errorf("%w: %v", ErrAttachmentNotFound, err)
		}
		return err
	})
	return data, err
}

// HasAttachment reports whether attachment oid of store id is staged on
// this clone or stored (a verified attachment tree of some member, or a
// pending write of this clone): the oids a write may name. It reads no
// content; ReadAttachment checks the hash when the blob is served.
func (h *Host) HasAttachment(ctx context.Context, id, oid string) (bool, error) {
	if !wsledger.ValidAttachmentOID(oid) {
		return false, nil
	}
	var ok bool
	err := h.with(ctx, id, func(st *store) error {
		if fi, err := os.Stat(stagedPath(st.entry.Dir, oid)); err == nil && fi.Mode().IsRegular() {
			ok = true
			return nil
		}
		ok = st.ledger.HasAttachment(oid)
		return nil
	})
	return ok, err
}

// attachmentAuthor is the member whose item or comment names oid ("" when
// none does): the tree ReadAttachment tries first.
func attachmentAuthor(v *wstracker.View, oid string) string {
	for _, it := range v.Items {
		for _, a := range it.Attachments {
			if a.OID == oid {
				return it.Author
			}
		}
	}
	for _, c := range v.Comments {
		for _, a := range c.Attachments {
			if a.OID == oid {
				return c.Author
			}
		}
	}
	return ""
}

// payloadOIDs lists the attachment oids a payload's attachments field names.
func payloadOIDs(payload json.RawMessage) []string {
	var p struct {
		Attachments []struct {
			OID string `json:"oid"`
		} `json:"attachments"`
	}
	if json.Unmarshal(payload, &p) != nil {
		return nil
	}
	var out []string
	for _, a := range p.Attachments {
		out = append(out, a.OID)
	}
	return out
}

// collectBlobs loads the staged copies of the attachments events name and
// refuses an oid that is neither staged nor stored in an attachment tree.
// It returns the blobs and the staged files to delete once appended.
func collectBlobs(st *store, events []wsledger.NewEvent) ([]wsledger.Blob, []string, error) {
	var blobs []wsledger.Blob
	var staged []string
	seen := map[string]bool{}
	for i, ev := range events {
		for _, oid := range payloadOIDs(ev.Payload) {
			if seen[oid] {
				continue
			}
			seen[oid] = true
			if data, ok := readStaged(st.entry.Dir, oid); ok {
				blobs = append(blobs, wsledger.Blob{OID: oid, Data: data})
				staged = append(staged, stagedPath(st.entry.Dir, oid))
				continue
			}
			if wsledger.ValidAttachmentOID(oid) && st.ledger.HasAttachment(oid) {
				continue
			}
			return nil, nil, &wstracker.Refusal{Index: i, Type: ev.Type,
				Reason: "attachment " + oid + " is neither staged nor stored; upload it first"}
		}
	}
	return blobs, staged, nil
}

// withWriteLock runs fn under the store's write lock (writeLockFile).
func (h *Host) withWriteLock(ctx context.Context, st *store, fn func() error) error {
	fl := flock.New(filepath.Join(st.entry.Dir, writeLockFile))
	lockCtx, cancel := context.WithTimeout(ctx, writeLockWait)
	defer cancel()
	if locked, err := fl.TryLockContext(lockCtx, 20*time.Millisecond); err != nil || !locked {
		return fmt.Errorf("tracker: store write lock %s not acquired: %v", fl.Path(), err)
	}
	defer fl.Unlock() //nolint:errcheck
	if err := h.checkRegistered(st); err != nil {
		return err
	}
	return fn()
}

// sweepStagedIfDue runs the staged-file sweep when sweepEvery has passed
// since the last one (or none ran yet). The caller holds the write lock.
func (h *Host) sweepStagedIfDue(st *store) {
	now := h.now()
	if !st.lastSweep.IsZero() && now.Sub(st.lastSweep) < sweepEvery {
		return
	}
	st.lastSweep = now
	h.sweepStaged(st, now)
}

// sweepStaged deletes staged files older than stageExpiry that no pending
// event of this clone names. The caller holds the write lock.
func (h *Host) sweepStaged(st *store, now time.Time) {
	entries, err := os.ReadDir(filepath.Join(st.entry.Dir, stagedDir))
	if err != nil {
		return
	}
	var named map[string]bool
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || e.IsDir() || now.Sub(info.ModTime()) < stageExpiry {
			continue
		}
		if named == nil {
			named = map[string]bool{}
			evs, _ := st.ledger.Events(wsledger.Cursor{})
			for _, ev := range evs {
				if ev.Pending {
					for _, oid := range payloadOIDs(ev.Payload) {
						named[oid] = true
					}
				}
			}
		}
		if named[e.Name()] {
			continue
		}
		os.Remove(filepath.Join(st.entry.Dir, stagedDir, e.Name()))
	}
}

// trySweep runs the sweep when due if the write lock is free right now;
// a busy lock leaves it to the next occasion.
func (h *Host) trySweep(st *store) {
	fl := flock.New(filepath.Join(st.entry.Dir, writeLockFile))
	if locked, err := fl.TryLock(); err != nil || !locked {
		return
	}
	defer fl.Unlock() //nolint:errcheck
	h.sweepStagedIfDue(st)
}
