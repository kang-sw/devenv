package wsdashboard

import (
	"html/template"
	"time"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// PageSource reads the parts of a session's page that only the serving
// ws-mcp's code knows how to read: the whiteboard and its panels from the
// session record, and the mailbox correspondents and history, which are keyed
// by the session key. A SessionSource that also implements PageSource gives
// the daemon its session pages; the daemon reads the session's live snapshot
// and conversation ring (wslive) itself.
//
// Nothing a PageSource returns may carry a session key: all of it reaches a
// browser.
type PageSource interface {
	// ReadPage reads the session reg names. ok is false when the session no
	// longer exists, as for SessionSource.ReadSession.
	ReadPage(reg Registration) (page PageContent, ok bool)
}

// LangPageSource is a PageSource whose page text follows the viewer's
// language: the board and panels it renders carry text the dashboard draws
// (panel titles, counts, tooltips). The daemon reads it through ReadPageLang
// when the source has one; lang is a wsi18n code, empty for the default.
type LangPageSource interface {
	PageSource
	ReadPageLang(reg Registration, lang string) (page PageContent, ok bool)
}

// ClosedTopicsSource is a PageSource that also renders the closed topics a
// session's board leaves past its cap (wswhiteboard.ClosedCap), for the
// board's show-more control: n of them from index from in close order, the
// most recently closed first, as one fragment that ends with the next
// show-more control when more remain. Lang is a wsi18n code, as for
// LangPageSource; ok is false when the session no longer exists or has no
// board.
type ClosedTopicsSource interface {
	ReadClosedTopics(reg Registration, lang string, from, n int) (html template.HTML, ok bool)
}

// PageContent is what a PageSource reads for one session.
type PageContent struct {
	// HasBoard reports whether the session has a whiteboard; Board is empty
	// when it has none.
	HasBoard bool
	// Seq is the board's update sequence number.
	Seq int
	// Board is the session's whiteboard board and panels, rendered by the
	// static page's templates (wswhiteboard.RenderRegions) with the served
	// page's link rule: a file link renders as an action link titled with
	// its repo-relative path (View.FileText, View.FileActions, FileActions),
	// a board image inside the session's project root as the image route's
	// URL (View.ImageSrc, ImageSrc) and any other image as its path, and a
	// panel row lists its file as Path text, never as a file: URL or a
	// relative link.
	Board wswhiteboard.Regions
	// Reasked maps each re-asked board item's ID to its latest re-ask time,
	// epoch milliseconds (wswhiteboard.State.ReaskTimes); the page's sent
	// marks drop answers sent before it. Nil when no item was re-asked.
	Reasked map[int]int64
	// Contacts are the session's mail correspondents, newest exchange first.
	Contacts []Contact
	// MailBodies maps a mailbox history entry name (a conversation ring mail
	// line's HistoryKey) to the message body, for every entry the session's
	// mailbox history still holds.
	MailBodies map[string]string
	// Watch names every file and directory the read took page content from
	// that a writer may change without a nudge, so no session fingerprint
	// catches it within a stream's refresh interval: the ticket directories
	// and ticket files the panels list, and each image a topic body inlines
	// (a missing one included, since its appearance changes the page), and
	// the mailbox stores, history directory, and listening marker the
	// mailbox panel reads. A
	// stream's refresh tick skips its re-read only while every one of them
	// stats the same (stream_cache.go), so a source that reads such a file
	// and leaves it out shows a stale page until the session's next change.
	// The paths are the daemon's alone (some carry a session key) and never
	// reach a page, so they are kept out of any encoding.
	Watch []string `json:"-"`
	// Volatile reports page content no watched file covers, such as a
	// process's liveness (an armed mailbox wait whose process may die and
	// leave its marker): every refresh tick re-reads such a page.
	Volatile bool
}

// ContactSource is a PageSource that re-reads a session's contacts alone
// (PageContent.Contacts, the same read): a contact's liveness follows the
// clock and its process's lifetime, so a stream's refresh tick re-reads it
// to tell whether the page changed. ok false means it cannot (a wrapper
// over a source without the read); then, as without a ContactSource, a page
// that shows contacts is re-read on every refresh tick.
type ContactSource interface {
	ReadContacts(reg Registration) (contacts []Contact, ok bool)
}

// Contact is one mail correspondent of a session, resolved at read time.
type Contact struct {
	// Address is the correspondent's literal address: "id:<reply-id>" or a
	// named "slug@scope".
	Address string
	// ReplyID is the session the address resolves to now: an id: address's
	// own reply-id, or a named address's presence owner's reply-id. Empty
	// when a named address has no owner record.
	ReplyID string
	// LastExchange is the session's own last send to or receive from it.
	LastExchange time.Time
	// PresenceLive is the liveness of a correspondent this daemon does not
	// serve: a named address's presence record is live (its process runs and
	// it was seen recently), or an id: address's reply registry entry was
	// seen within the last 10 minutes.
	PresenceLive bool
	// Cwd is the correspondent's working directory: its reply registry entry
	// or presence record's, else the contact record's as known at the last
	// exchange. Empty when neither records one.
	Cwd string
}
