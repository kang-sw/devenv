package wsdashboard

import (
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

// PageContent is what a PageSource reads for one session.
type PageContent struct {
	// HasBoard reports whether the session has a whiteboard; Board is empty
	// when it has none.
	HasBoard bool
	// Seq is the board's update sequence number.
	Seq int
	// Board is the session's whiteboard board and panels, rendered by the
	// static page's templates (wswhiteboard.RenderRegions) with the served
	// page's link rule: a file link renders as its repo-relative path text
	// (View.FileText), a board image inside the session's project root as
	// the image route's URL (View.ImageSrc, ImageSrc) and any other image
	// as path text, and a panel row lists its file as Path text, never as a
	// file: URL or a relative link.
	Board wswhiteboard.Regions
	// Contacts are the session's mail correspondents, newest exchange first.
	Contacts []Contact
	// MailBodies maps a mailbox history entry name (a conversation ring mail
	// line's HistoryKey) to the message body, for every entry the session's
	// mailbox history still holds.
	MailBodies map[string]string
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
}
