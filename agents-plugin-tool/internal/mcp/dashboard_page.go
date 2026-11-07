package mcp

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// The dashboard daemon's session page, read through DashboardSessionSource:
// the board and its panels rendered with the static page's templates under
// the served-page link rule (a file link is an action link for the page's
// link menu, opening the file view inside the session's project root; a
// board image inside the root is the image route's URL, inlined as a data:
// URL in the sandboxed Now frame, and path text otherwise), the mail correspondents, and the mailbox history bodies.
// Everything here reaches a browser, so no returned field may carry the
// session key.

var _ wsdashboard.PageSource = (*DashboardSessionSource)(nil)

// ReadPage reads the page parts of the session reg names. It reports false
// exactly when ReadSession does. A session without a board still reads, with
// HasBoard false, since the dashboard lists it for its live snapshot.
func (src *DashboardSessionSource) ReadPage(reg wsdashboard.Registration) (wsdashboard.PageContent, bool) {
	key, _, record, ok := src.lookupRecord(reg)
	if !ok {
		return wsdashboard.PageContent{}, false
	}
	page := wsdashboard.PageContent{
		Contacts:   src.readContacts(reg),
		MailBodies: map[string]string{},
	}
	if history, err := readMailboxHistory(key); err == nil {
		for _, r := range history {
			page.MailBodies[r.Name] = wslive.ScrubSessionKeys(r.Body, key)
		}
	}
	if record.Whiteboard == nil {
		return page, true
	}
	page.HasBoard = true
	page.Seq = record.Whiteboard.Seq
	// BuildView only reads the state: the served page never begins a render
	// or writes a file, so the stored board and the static page are untouched.
	view := wswhiteboard.BuildView(record.Whiteboard, wswhiteboard.ViewInput{
		Layout:   wswhiteboard.LayoutFull,
		ReplyID:  reg.ReplyID,
		Panels:   servedWhiteboardPanels(record, key, reg.ReplyID),
		FileText: servedFileText(record.Root),
		ImageSrc: func(path string) string {
			return wsdashboard.ImageSrc(record.Root, reg.ReplyID, path)
		},
		// Now renders in a sandboxed frame that sends no cookie, so its
		// images are inlined rather than loaded through the image route.
		NowImageSrc: func(path string) string {
			return wsdashboard.ImageDataURL(record.Root, path)
		},
		FileActions: wsdashboard.FileActions(record.Root, reg.ReplyID),
	})
	regions, err := wswhiteboard.RenderRegions(view)
	if err != nil {
		appendDebugEvent("dashboard.page_render_failed", map[string]any{"error": err.Error()})
		return page, true
	}
	page.Board = regions
	return page, true
}

// servedWhiteboardPanels builds the full page's panels for the served page.
// The daemon process serves no session itself, so it holds no mailbox
// identity: the mailbox panel shows the session's reply-id address and its
// reply-id unread count, never a named inbox the session may own.
func servedWhiteboardPanels(record sessionRecord, key, replyID string) []wswhiteboard.Panel {
	return whiteboardPanels(whiteboardPanelInput{
		rec:        record,
		layout:     wswhiteboard.LayoutFull,
		root:       record.Root,
		sessionKey: key,
		replyID:    replyID,
		served:     true,
	})
}

// servedFileText shows a file path an item's markdown names as text on a
// served page, taking a relative path from the session root the way
// whiteboardItemFileLink does: a relative path is shown cleaned, an absolute
// path inside root relative to root, and any other absolute path as is, all
// with forward slashes.
func servedFileText(root string) func(string) string {
	return func(path string) string {
		native := filepath.FromSlash(path)
		if !filepath.IsAbs(native) {
			return filepath.ToSlash(filepath.Clean(native))
		}
		if rel, err := filepath.Rel(root, native); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(rel)
		}
		return filepath.ToSlash(native)
	}
}

// readContacts resolves the session's mail correspondents, newest exchange
// first, reading the reply registry and each named scope's store once. An
// id: address resolves to its own reply-id and is live while its registry
// entry was seen within mailboxLivenessThreshold (a reply entry carries no
// pid, so only that half of mailboxPresenceLive applies); a named address
// resolves through its presence record's owner and is live per
// mailboxPresenceLive. An unreadable store leaves the fields it would fill
// empty.
func (src *DashboardSessionSource) readContacts(reg wsdashboard.Registration) []wsdashboard.Contact {
	path, err := wsmailbox.ReplyRegistryPath()
	if err != nil {
		return nil
	}
	replies, err := wsmailbox.LoadReplyStore(path)
	if err != nil {
		return nil
	}
	sorted := wsmailbox.SortedContacts(replies.Entries[reg.ReplyID].Contacts)
	if len(sorted) == 0 {
		return nil
	}
	now := src.clock()
	stores := map[wsmailbox.Scope]*wsmailbox.StoreFile{}
	scopeStore := func(scope wsmailbox.Scope) *wsmailbox.StoreFile {
		if store, seen := stores[scope]; seen {
			return store
		}
		stores[scope] = nil
		if path, err := wsmailbox.PathForScope(scope, reg.Project); err == nil {
			if store, err := wsmailbox.Load(path); err == nil {
				stores[scope] = &store
			}
		}
		return stores[scope]
	}
	out := make([]wsdashboard.Contact, 0, len(sorted))
	for _, rec := range sorted {
		c := wsdashboard.Contact{Address: rec.Address}
		if at, err := time.Parse(time.RFC3339, rec.LastExchange); err == nil {
			c.LastExchange = at
		}
		if id, isReply := strings.CutPrefix(rec.Address, wsmailbox.ReplyIDPrefix); isReply {
			c.ReplyID = id
			if entry, ok := replies.Entries[id]; ok {
				if seen, err := time.Parse(time.RFC3339, entry.LastSeen); err == nil {
					c.PresenceLive = now.Sub(seen) < mailboxLivenessThreshold
				}
			}
		} else if name, scope, err := wsmailbox.ParseSlugScope(rec.Address); err == nil {
			if store := scopeStore(scope); store != nil {
				if p, ok := store.Presence[name]; ok {
					if p.Owner != "" {
						c.ReplyID = wsmailbox.ReplyID(p.Owner)
					}
					c.PresenceLive = mailboxPresenceLive(p, now)
				}
			}
		}
		out = append(out, c)
	}
	return out
}
