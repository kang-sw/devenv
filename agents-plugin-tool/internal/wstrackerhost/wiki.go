package wstrackerhost

import (
	"context"
	"errors"
	"fmt"

	"github.com/kang-sw/devenv/internal/wsledger"
)

// Wikis: the host's thin spine over wsledger's wiki branches. Every call
// runs on the opened store under its lock; a save on a store with a remote
// is a network call (it fetches and pushes), recorded like a sync, except
// that a refusal (a conflict, a stale base, a bad name) is the saver's
// problem, not the remote's, and is not recorded as a sync error.

// WikiImageInput is one image a write pastes: raw bytes the host types by
// their magic numbers (PNG, JPEG, GIF, WebP) and bounds by the store's
// attachments.max_bytes.
type WikiImageInput struct {
	Data []byte
}

// WikiSave is one wiki edit on store id (wsledger.WikiEdit) with its
// pasted images, which the host types and hashes.
type WikiSave struct {
	Edit   wsledger.WikiEdit
	Images []WikiImageInput
}

// WikiList lists store id's wikis.
func (h *Host) WikiList(ctx context.Context, id string) ([]wsledger.WikiInfo, error) {
	var out []wsledger.WikiInfo
	err := h.with(ctx, id, func(st *store) error {
		var err error
		out, err = st.ledger.WikiList(ctx)
		return err
	})
	return out, err
}

// WikiPages lists the pages of a wiki at commit (the tip when "").
func (h *Host) WikiPages(ctx context.Context, id, wiki, commit string) ([]string, string, error) {
	var pages []string
	var at string
	err := h.with(ctx, id, func(st *store) error {
		var err error
		pages, at, err = st.ledger.WikiPages(ctx, wiki, commit)
		return err
	})
	return pages, at, err
}

// WikiRead returns a page and the commit it was read at.
func (h *Host) WikiRead(ctx context.Context, id, wiki, path, commit string) (string, string, error) {
	var content, at string
	err := h.with(ctx, id, func(st *store) error {
		var err error
		content, at, err = st.ledger.WikiRead(ctx, wiki, path, commit)
		return err
	})
	return content, at, err
}

// WikiSearch finds query in a wiki's pages at the tip.
func (h *Host) WikiSearch(ctx context.Context, id, wiki, query string) ([]wsledger.WikiMatch, string, error) {
	var hits []wsledger.WikiMatch
	var at string
	err := h.with(ctx, id, func(st *store) error {
		var err error
		hits, at, err = st.ledger.WikiSearch(ctx, wiki, query)
		return err
	})
	return hits, at, err
}

// WikiHistory lists a wiki's (or one page's) commits with their signature
// verdicts.
func (h *Host) WikiHistory(ctx context.Context, id, wiki, path string, limit int) ([]wsledger.WikiCommit, error) {
	var out []wsledger.WikiCommit
	err := h.with(ctx, id, func(st *store) error {
		var err error
		out, err = st.ledger.WikiHistory(ctx, wiki, path, limit)
		return err
	})
	return out, err
}

// WikiDiff returns a page's patch between two commits.
func (h *Host) WikiDiff(ctx context.Context, id, wiki, path, from, to string) (string, error) {
	var patch string
	err := h.with(ctx, id, func(st *store) error {
		var err error
		patch, err = st.ledger.WikiDiff(ctx, wiki, path, from, to)
		return err
	})
	return patch, err
}

// WikiRecovery lists the recovery points of a wiki (every wiki's when "").
func (h *Host) WikiRecovery(ctx context.Context, id, wiki string) ([]wsledger.WikiRecoveryPoint, error) {
	var out []wsledger.WikiRecoveryPoint
	err := h.with(ctx, id, func(st *store) error {
		var err error
		out, err = st.ledger.WikiRecovery(ctx, wiki)
		return err
	})
	return out, err
}

// ReadWikiImage returns the image oid of wiki's tip, or
// ErrAttachmentNotFound.
func (h *Host) ReadWikiImage(ctx context.Context, id, wiki, oid string) ([]byte, error) {
	if !wsledger.ValidAttachmentOID(oid) || !wsledger.ValidWikiName(wiki) {
		return nil, ErrAttachmentNotFound
	}
	var data []byte
	err := h.with(ctx, id, func(st *store) error {
		var err error
		data, err = st.ledger.WikiImageRead(ctx, wiki, oid)
		if errors.Is(err, wsledger.ErrWikiImageNotFound) || errors.Is(err, wsledger.ErrWikiNotFound) {
			return fmt.Errorf("%w: %v", ErrAttachmentNotFound, err)
		}
		return err
	})
	return data, err
}

// imageExt is the file extension of an accepted image mime.
func imageExt(mime string) string {
	switch mime {
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpg"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	}
	return ""
}

// WikiSave applies one edit to store id. Images are typed by their bytes
// (ErrNotImage) and bounded by attachments.max_bytes
// (ErrAttachmentTooLarge) before anything is written; the edit's Images
// field is replaced by them. Refusals are wsledger's (a *WikiConflict,
// ErrWikiExists, ...).
func (h *Host) WikiSave(ctx context.Context, id string, s WikiSave) (wsledger.WikiSaveResult, error) {
	var res wsledger.WikiSaveResult
	edit := s.Edit
	edit.Images = nil
	for _, img := range s.Images {
		mime := imageMime(img.Data)
		if mime == "" {
			return res, ErrNotImage
		}
		edit.Images = append(edit.Images, wsledger.WikiImage{OID: attachmentOID(img.Data), Ext: imageExt(mime), Data: img.Data})
	}
	err := h.with(ctx, id, func(st *store) error {
		if len(edit.Images) > 0 {
			st.tracker.Refresh()
			limit := st.tracker.View(h.now(), 0).Attachments.MaxBytes
			for _, img := range edit.Images {
				if float64(len(img.Data)) > limit {
					return fmt.Errorf("%w: %d > %.0f bytes", ErrAttachmentTooLarge, len(img.Data), limit)
				}
			}
		}
		if st.entry.Remote == "" {
			var err error
			res, err = st.ledger.WikiSave(ctx, edit)
			return err
		}
		var refused error
		err := h.attempt(ctx, st, networkTimeout, func(ctx context.Context) error {
			var err error
			res, err = st.ledger.WikiSave(ctx, edit)
			if isWikiRefusal(err) {
				refused = err
				return nil
			}
			return err
		})
		if err != nil {
			return err
		}
		return refused
	})
	return res, err
}

// isWikiRefusal reports whether err is the saver's refusal rather than a
// store or network failure.
func isWikiRefusal(err error) bool {
	var c *wsledger.WikiConflict
	if errors.As(err, &c) {
		return true
	}
	for _, e := range []error{wsledger.ErrWikiBadName, wsledger.ErrWikiBadPath, wsledger.ErrWikiNotFound,
		wsledger.ErrWikiExists, wsledger.ErrWikiNoBase, wsledger.ErrWikiPageNotFound, wsledger.ErrWikiPageExists,
		wsledger.ErrWikiNoChange, wsledger.ErrWikiBadOp, wsledger.ErrWikiBadEdit, wsledger.ErrBlobMismatch} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}
