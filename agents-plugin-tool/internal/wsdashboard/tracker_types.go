// tracker_types.go holds the tracker host contract and JSON types.
package wsdashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// trackerBodyLimit bounds a tracker POST body.
const trackerBodyLimit = 256 << 10

// trackerAttachmentBodyLimit is the hard ceiling of an attachment upload
// body, above any store's own attachments limit.
const trackerAttachmentBodyLimit = 256 << 20

// The errors a TrackerHost's attachment methods wrap, which the
// attachment routes map to their status.
var (
	// ErrTrackerNotImage: the upload is not an accepted image.
	ErrTrackerNotImage = errors.New("attachment is not a supported image")
	// ErrTrackerAttachmentTooLarge: the upload exceeds the store's limit.
	ErrTrackerAttachmentTooLarge = errors.New("attachment too large")
	// ErrTrackerAttachmentNotFound: the store has no such attachment.
	ErrTrackerAttachmentNotFound = errors.New("attachment not found")
	// ErrTrackerWikiNotFound: a wiki read names no wiki or page.
	ErrTrackerWikiNotFound = errors.New("wiki or page not found")
	// ErrTrackerWikiRefused: a wiki save the wiki rules refuse (a malformed
	// name, path, or edit; a new wiki that exists; a page already there or
	// gone; a pasted image whose staged copy expired); the message says why.
	ErrTrackerWikiRefused = errors.New("wiki save refused")
	// ErrTrackerBusy: a wiki or image read gave up waiting for the store
	// (held by a wiki save, or, for an image not fetched yet, the network
	// held by a fetch or push); the route answers 503 with Retry-After.
	ErrTrackerBusy = errors.New("store busy")
	// ErrTrackerBadLabel: a store label the host refuses (a control
	// character, or longer than its limit); the label route answers 400.
	ErrTrackerBadLabel = errors.New("bad store label")
	// ErrTrackerUnknownStore: the store was unregistered after the route
	// found it listed; the store routes that wrap it answer 404.
	ErrTrackerUnknownStore = errors.New("unknown store")
)

// The kinds of a TrackerUnregisterRefusal.
const (
	// TrackerUnregisterConfirm: the store has no remote, and the confirm
	// did not name it.
	TrackerUnregisterConfirm = "confirm"
	// TrackerUnregisterBusy: a write, or a local step of a check or sync,
	// holds the store.
	TrackerUnregisterBusy = "busy"
	// TrackerUnregisterUnpushed: the clone holds writes its remote does not;
	// a sync can push them.
	TrackerUnregisterUnpushed = "unpushed"
	// TrackerUnregisterStuck: the clone holds writes no sync can push.
	TrackerUnregisterStuck = "stuck"
)

// TrackerUnregisterRefusal is an unregister the host refused, the store
// left exactly as it was: Kind is one of the TrackerUnregister* kinds,
// Message a full sentence the page shows.
type TrackerUnregisterRefusal struct {
	Kind    string
	Message string
}

func (e *TrackerUnregisterRefusal) Error() string { return e.Message }

// trackerBusyRetry is the Retry-After, in seconds, of a busy read's 503.
const trackerBusyRetry = "1"

// trackerBusy answers a read that found the store busy: 503 with
// Retry-After, so the page reads again shortly.
func trackerBusy(w http.ResponseWriter, err error) {
	w.Header().Set("Retry-After", trackerBusyRetry)
	trackerError(w, http.StatusServiceUnavailable, err.Error())
}

// TrackerHost is the daemon's tracker host: the stores the user registered
// and the operations the tracker drawer runs on them. Every string it
// returns already has credentials redacted. Its methods may run
// concurrently.
type TrackerHost interface {
	// Stores lists the registered stores in handle order. An error is the
	// host's own failure (for example, it could not be created).
	Stores() ([]TrackerStore, error)
	// View returns store id's view and its seen marks.
	View(ctx context.Context, id string) (TrackerView, error)
	// Search returns the ids of store id's items that pass query, parsed
	// by the tracker's search grammar (the one tracker.query's q takes),
	// in tree order, hidden items included when they pass. A query the
	// grammar refuses is a *TrackerSearchError.
	Search(ctx context.Context, id, query string) ([]string, error)
	// SearchHits runs the same search as Search, with the same grammar and
	// the same *TrackerSearchError for a query it refuses, and returns the
	// places the items matched instead of their ids: per item, in tree
	// order, its body hit, then up to three comment hits, or one "item" hit
	// when neither matched (see TrackerSearchHit).
	SearchHits(ctx context.Context, id, query string) ([]TrackerSearchHit, error)
	// Dispatch appends events to store id in one batch and returns their
	// ids. A refusal is a *TrackerRefusal error; nothing was written.
	Dispatch(ctx context.Context, id string, events []TrackerEvent) ([]string, error)
	// Check fetches store id's remote without pushing.
	Check(ctx context.Context, id string) (TrackerSyncReport, error)
	// Sync fetches and pushes store id.
	Sync(ctx context.Context, id string) (TrackerSyncReport, error)
	// MarkSeen adds marks to store id's seen marks.
	MarkSeen(id string, add TrackerSeen) error
	// Profiles returns the saved identity profiles.
	Profiles() (TrackerProfiles, error)
	// SaveProfile saves one identity profile.
	SaveProfile(p TrackerProfile) error
	// Register registers (and when new, creates) a store.
	Register(ctx context.Context, req TrackerRegisterRequest) (TrackerRegisterResult, error)
	// Unregister removes store id from this machine, its registration and
	// its local clone, and keeps the remote. confirm is the store id the
	// user typed, which a store with no remote requires. A refusal is a
	// *TrackerUnregisterRefusal; the store is then as it was.
	Unregister(ctx context.Context, id, confirm string) error
	// Relabel sets store id's display label on this machine and returns
	// the store as written; an empty label clears it back to the id. A
	// label the host refuses wraps ErrTrackerBadLabel.
	Relabel(id, label string) (TrackerStore, error)
	// SetHidden hides or shows store id on this machine and returns the
	// store as written.
	SetHidden(id string, hidden bool) (TrackerStore, error)
	// Render renders text written for store id the way the view renders
	// an item body or comment text, so an editor's preview matches the
	// saved view. It writes nothing.
	Render(ctx context.Context, id string, req TrackerRenderRequest) (string, error)
	// StageAttachment stores an uploaded image for a later dispatch of store id and
	// returns what the event's attachments field names. ErrTrackerNotImage when the
	// bytes are not an accepted image; ErrTrackerAttachmentTooLarge when over the
	// store's attachments.max_bytes.
	StageAttachment(ctx context.Context, id, name string, data []byte) (TrackerAttachment, error)
	// ReadAttachment returns the content of attachment oid of store id, or
	// ErrTrackerAttachmentNotFound. This read and the wiki reads below may
	// answer ErrTrackerBusy instead of waiting long for a busy store.
	ReadAttachment(ctx context.Context, id, oid string) ([]byte, error)
	// ReadWikiImage returns the content of image oid of wiki of store id, or
	// ErrTrackerAttachmentNotFound when the wiki or the image is absent.
	ReadWikiImage(ctx context.Context, id, wiki, oid string) ([]byte, error)
	// WikiList lists the wikis of store id.
	WikiList(ctx context.Context, id string) ([]TrackerWiki, error)
	// WikiPages lists the pages of a wiki of store id at its tip, or
	// ErrTrackerWikiNotFound.
	WikiPages(ctx context.Context, id, wiki string) (TrackerWikiPages, error)
	// WikiRead returns a page of a wiki of store id at its tip, or
	// ErrTrackerWikiNotFound.
	WikiRead(ctx context.Context, id, wiki, path string) (TrackerWikiPage, error)
	// WikiSave applies one edit to a wiki of store id. A save that cannot
	// merge is a *TrackerWikiConflict error; one the wiki rules refuse is
	// ErrTrackerWikiRefused; any other error is the store's or the remote's.
	WikiSave(ctx context.Context, id, wiki string, req TrackerWikiSave) (TrackerWikiSaveResult, error)
}

// TrackerSearchError is a search query the grammar refuses; Message names
// the token and what its key takes, and the page shows it.
type TrackerSearchError struct {
	Message string
}

func (e *TrackerSearchError) Error() string { return e.Message }

// TrackerSearchHit is one place an item matched a search, the element of the
// search route's "hits" with excerpts=1. Field is "body" or "comment" when a
// free-text term occurs in the item's body or in a shown comment (Comment is
// that comment's id, Text an excerpt around the match), and "item" with no
// Text when the item matched on its title, alias, id, project, or only on
// field filters. Status is the derived status, empty for a topic.
type TrackerSearchHit struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Alias   string `json:"alias"`
	Kind    string `json:"kind"`
	Status  string `json:"status"`
	Hidden  bool   `json:"hidden,omitempty"`
	Field   string `json:"field"`
	Comment string `json:"comment,omitempty"`
	Text    string `json:"text,omitempty"`
}

// TrackerRenderRequest is the render route's body: the markdown text an
// editor holds and the attachments its save would carry (the stored ones
// plus the images uploaded meanwhile), which the host may render inline
// when each is staged or stored. Wiki, when set, renders a page of that
// wiki: an image is also shown from the wiki's own tree.
type TrackerRenderRequest struct {
	Text        string              `json:"text"`
	Attachments []TrackerAttachment `json:"attachments,omitempty"`
	Wiki        string              `json:"wiki,omitempty"`
}

// TrackerWiki is one wiki of a store as the wiki list answers it.
type TrackerWiki struct {
	Name string `json:"name"`
	Tip  string `json:"tip"`
	// Unpushed is true when the store has a remote the tip is not known to
	// hold.
	Unpushed bool `json:"unpushed"`
}

// TrackerWikiPages is the page list of a wiki at Commit, its tip.
type TrackerWikiPages struct {
	Pages  []string `json:"pages"`
	Commit string   `json:"commit"`
}

// TrackerWikiPage is one page as read at Commit, the base a save of this
// edit sends. Path is the canonical page path and Kind the page's kind,
// "markdown" or "html". For a Markdown page HTML is the content rendered
// as the render route renders it in that wiki's mode (the tree's images
// show), so a page read draws at once; for an HTML page it is the
// sandboxed frame showing the content, built by the host so the browser
// never renders a page itself.
type TrackerWikiPage struct {
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	Content string `json:"content"`
	Commit  string `json:"commit"`
	HTML    string `json:"html"`
}

// TrackerWikiSave is the wiki save route's body: one edit of the wiki the
// route names. Base is the commit the edit started from (the page read's
// commit); NewWiki creates the wiki with this save instead, and is refused
// when it exists. Op is write (Content at Path), rename (Path to To),
// delete (Path), or restore (Path as Commit held it). Attachments are the
// images the page names that were uploaded through the attachment upload
// and not yet saved; the save writes each into the wiki beside the page.
type TrackerWikiSave struct {
	Base        string              `json:"base"`
	NewWiki     bool                `json:"newWiki"`
	Op          string              `json:"op"`
	Path        string              `json:"path"`
	To          string              `json:"to,omitempty"`
	Content     string              `json:"content"`
	Commit      string              `json:"commit,omitempty"`
	Message     string              `json:"message,omitempty"`
	Attachments []TrackerAttachment `json:"attachments,omitempty"`
}

// TrackerWikiSaveResult is a landed wiki save: the new tip, whether the
// save merged a moved tip, whether it created the wiki, and the warnings
// its fetch found.
type TrackerWikiSaveResult struct {
	Commit   string   `json:"commit"`
	Merged   bool     `json:"merged"`
	Created  bool     `json:"created"`
	Warnings []string `json:"warnings"`
}

// TrackerWikiConflict is a wiki save that could not merge, nothing saved:
// the edit's Path, the Kind of conflict and its Reason, the edit's Base,
// the current tip Theirs ("" when the wiki is gone), and the two texts,
// the saver's and the tip's. The saver merges them and saves again with
// Theirs as the base (or NewWiki when Theirs is empty).
type TrackerWikiConflict struct {
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	Reason     string `json:"reason"`
	Base       string `json:"base"`
	Theirs     string `json:"theirs"`
	OursText   string `json:"oursText"`
	TheirsText string `json:"theirsText"`
}

func (c *TrackerWikiConflict) Error() string {
	return fmt.Sprintf("wiki %s refused on %s: %s", c.Kind, c.Path, c.Reason)
}

// TrackerAttachment is a staged attachment as the upload route answers it.
type TrackerAttachment struct {
	OID  string `json:"oid"`
	Name string `json:"name"`
	Size int64  `json:"size"`
	Mime string `json:"mime"`
}

// TrackerStore is one registered store as the stores route lists it.
// Hidden: the user hid it on this machine; the page leaves it out of its
// handle row.
type TrackerStore struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Member string `json:"member"`
	Remote string `json:"remote"`
	Hidden bool   `json:"hidden,omitempty"`
}

// TrackerSeen is what the viewer has seen on a store: the handle-badge
// seen marks and the notification read marks.
type TrackerSeen struct {
	Badge []string `json:"badge"`
	Read  []string `json:"read"`
}

// TrackerView is the view route's answer: the store's view JSON as the
// host renders it, and its seen marks.
type TrackerView struct {
	View json.RawMessage `json:"view"`
	Seen TrackerSeen     `json:"seen"`
	// Tag, when set, identifies View's content apart from the time of day
	// (two views with one Tag differ at most in their now, within the
	// hour, see cmd/ws-mcp dashboardViewTag): the
	// view route's ETag is made from it and the seen marks, so a view read
	// again unchanged answers a revalidating page 304. Empty, the ETag is
	// the answer's bytes.
	Tag string `json:"-"`
}

// TrackerEvent is one event the page dispatches; Payload is the event's
// JSON payload as the tracker defines it.
type TrackerEvent struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
	Origin  string          `json:"origin"`
}

// TrackerRefusal is a dispatch the tracker refused: the batch event at
// Index, of Type, would have been flagged for Reason. Nothing was written.
type TrackerRefusal struct {
	Index  int    `json:"index"`
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

func (e *TrackerRefusal) Error() string {
	return fmt.Sprintf("event %d (%s) would be flagged: %s", e.Index, e.Type, e.Reason)
}

// TrackerSyncReport is the outcome of a check or sync. NoRemote marks a
// store with no remote; Offline a remote that could not be reached (the
// reason is in the view's sync state, not here). Error, when set, is a
// failure the page shows.
type TrackerSyncReport struct {
	NoRemote bool   `json:"noRemote"`
	Offline  bool   `json:"offline"`
	Closed   bool   `json:"closed"`
	Pushed   bool   `json:"pushed"`
	Received int    `json:"received"`
	Unpushed int    `json:"unpushed"`
	Error    string `json:"error,omitempty"`
}

// TrackerProfile is one saved identity profile.
type TrackerProfile struct {
	Org    string `json:"org"`
	Family string `json:"family"`
	Given  string `json:"given"`
}

// TrackerProfiles is the saved profile list; Last is the index of the
// profile used last.
type TrackerProfiles struct {
	Last     int              `json:"last"`
	Profiles []TrackerProfile `json:"profiles"`
}

// TrackerRegisterRequest is the register route's body. Profile, when set,
// is the identity a new member joins with.
type TrackerRegisterRequest struct {
	ID      string          `json:"id"`
	Label   string          `json:"label"`
	Remote  string          `json:"remote"`
	Member  string          `json:"member"`
	Profile *TrackerProfile `json:"profile"`
}

// TrackerRegisterResult is the register route's answer: the registered
// store's id, whether this registration created the store, and the
// profile it joined with.
type TrackerRegisterResult struct {
	Store   string         `json:"store"`
	Creator bool           `json:"creator"`
	Profile TrackerProfile `json:"profile"`
}
