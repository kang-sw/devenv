package wsdashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// The tracker routes serve the tracker drawer of the dashboard pages. The
// daemon hosts no tracker itself: DaemonOptions.Tracker is a host the
// caller supplies, and this package knows it only through the JSON-level
// types below, so the dashboard stays free of the tracker packages.
//
//	GET  /tracker/stores                 {"stores": [...], "rev": N, "support": B}
//	GET  /tracker/stores/<id>/view       {"view": ..., "seen": {"badge", "read"}}
//	GET  /tracker/stores/<id>/search?q=<query>
//	                                     {"ids": [...]}: the items passing the search, in tree order;
//	                                     a malformed query is 400 with the host's message
//	POST /tracker/stores/<id>/dispatch   {"events": [...]} -> {"ids": [...]}; a refusal is 409
//	POST /tracker/stores/<id>/check      the sync report
//	POST /tracker/stores/<id>/sync       the sync report
//	POST /tracker/stores/<id>/seen       {"badge", "read"} -> 204
//	POST /tracker/stores/<id>/render     {"text", "attachments"} -> {"html"}: an editor's preview
//	POST /tracker/stores/<id>/attachments?name=<name>
//	                                     raw image bytes -> {"oid", "name", "size", "mime"}
//	GET  /tracker/stores/<id>/attachments/<oid>
//	                                     the image bytes
//	GET  /tracker/stores/<id>/wiki/<name>/attachments/<sha256>
//	                                     a wiki page's image bytes
//	GET  /tracker/stores/<id>/wiki       {"wikis": [{"name", "tip", "unpushed"}]}
//	GET  /tracker/stores/<id>/wiki/<name>/pages
//	                                     {"pages": [...], "commit"}: the pages at the tip
//	GET  /tracker/stores/<id>/wiki/<name>/page?path=<path>
//	                                     {"path", "content", "commit"}: the commit is the save's base
//	POST /tracker/stores/<id>/wiki/<name>/save
//	                                     a TrackerWikiSave -> {"commit", "merged", "created", "warnings"};
//	                                     a conflict is 409 with "conflict", a refusal 422
//	GET  /tracker/profiles               the saved profiles
//	POST /tracker/profiles               save one; the profiles after the save
//	POST /tracker/register               register a store
//
// Every route answers JSON, errors as {"error": ...}. A store route names a
// store the host lists, else 404; a host whose Stores fails (one that could
// not be created) answers 503 with its error on every route. Every POST
// requires an Origin equal to the daemon's own (sameOrigin), which stops
// cross-site form posts, and reads a size-limited body. A dispatched
// event's origin is always "human": the page is the user's own hand.
//
// "support" reports that the daemon serves the store chats
// (SupportRoutePrefix), so the drawer shows each store's chat tab.
//
// The attachment upload stages an image for a later dispatch, whose event
// names it by the returned oid; it reads a raw body (not JSON) under the
// hard ceiling trackerAttachmentBodyLimit, and the host applies the
// store's own, usually smaller, limit. A body that is not an accepted
// image answers 415, one over either limit 413. The attachment read serves
// only an oid of 64 lowercase hex digits, only content that sniffs as
// PNG, JPEG, GIF, or WebP, and refuses a request another site started
// (crossSiteFetch); every refusal, like a missing attachment, answers 404.
// Its content is addressed by its hash, so it is cacheable for good. The
// wiki image read serves an image of wiki <name> (a lowercase name, see
// validWikiName) under the same rules.
//
// The wiki routes serve the drawer's wiki card over the store's wiki
// branches. A page read returns the commit it read, which the save of that
// edit sends back as its base; a save whose base is no longer the tip
// merges, and one that cannot merge cleanly answers 409 with both texts
// (TrackerWikiConflict), so the editor shows them and the saver merges. A
// pasted image goes up through the attachment upload like an item's; the
// save names it in its attachments list, and the host writes the staged
// copy into the wiki with the page. The render route's wiki mode (a
// request with "wiki") shows such an image from the wiki's own tree or
// from the staged copy, so the preview before the save and the page after
// it look alike.
//
// The revision counter (TrackerChanged) bumps after every write these
// routes run and on the host's own poll round; each SSE stream sends it as
// "event: tracker" when it changed (sse.go), and the page re-reads.

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
)

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
	// ErrTrackerAttachmentNotFound.
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
// edit sends. HTML is the content rendered as the render route renders it
// in that wiki's mode (the tree's images show), so a page read draws at
// once.
type TrackerWikiPage struct {
	Path    string `json:"path"`
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
type TrackerStore struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Member string `json:"member"`
	Remote string `json:"remote"`
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

// trackerStoreOps are the operations of the store route
// /tracker/stores/<id>/<op>; view, search, and wiki (the wiki list) are
// the GETs.
// The attachment read, /tracker/stores/<id>/attachments/<oid>, is the op
// "attachment". A route under /tracker/stores/<id>/wiki/<name>/ is a
// wiki op (trackerWikiOps) with the op prefixed "wiki-"; the image read
// /tracker/stores/<id>/wiki/<name>/attachments/<oid> is "wiki-image".
var trackerStoreOps = map[string]string{
	"view":        http.MethodGet,
	"search":      http.MethodGet,
	"dispatch":    http.MethodPost,
	"check":       http.MethodPost,
	"sync":        http.MethodPost,
	"seen":        http.MethodPost,
	"attachments": http.MethodPost,
	"render":      http.MethodPost,
	"wiki":        http.MethodGet,
}

// trackerWikiOps are the operations of one wiki,
// /tracker/stores/<id>/wiki/<name>/<op>.
var trackerWikiOps = map[string]string{
	"pages": http.MethodGet,
	"page":  http.MethodGet,
	"save":  http.MethodPost,
}

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

// validWikiName reports a wiki name: a lowercase letter or digit, then up
// to 63 lowercase letters, digits, or hyphens.
func validWikiName(name string) bool {
	if name == "" || len(name) > 64 || name[0] == '-' {
		return false
	}
	for i := 0; i < len(name); i++ {
		if c := name[i]; (c < '0' || c > '9') && (c < 'a' || c > 'z') && c != '-' {
			return false
		}
	}
	return true
}

// TrackerChanged bumps the tracker revision and wakes every stream, which
// then sends the new revision. Safe from any goroutine, also before the
// first request.
func (d *Daemon) TrackerChanged() {
	d.trackerRev.Add(1)
	d.stats.WakeTracker.Add(1)
	d.broker.signalAll()
}

// trackerError writes the {"error": msg} answer.
func trackerError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// nonNil returns s, or an empty list for nil, so it marshals as [].
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// decodeTrackerBody reads r's body, at most trackerBodyLimit bytes, into
// v; a malformed or oversize body answers 400 and returns false.
func decodeTrackerBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, trackerBodyLimit)).Decode(v); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			trackerError(w, http.StatusBadRequest, "request body too large")
		} else {
			trackerError(w, http.StatusBadRequest, "malformed request body")
		}
		return false
	}
	return true
}

// handleTracker serves every route under /tracker/ (see the file comment).
func (d *Daemon) handleTracker(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/tracker/")
	var method, storeID, op, oid, wiki string
	switch {
	case rest == "stores":
		method = http.MethodGet
	case rest == "profiles":
		method = http.MethodGet
		if r.Method == http.MethodPost {
			method = http.MethodPost
		}
	case rest == "register":
		method = http.MethodPost
	case strings.HasPrefix(rest, "stores/"):
		id, sub, ok := strings.Cut(strings.TrimPrefix(rest, "stores/"), "/")
		if a, ok := strings.CutPrefix(sub, "attachments/"); ok {
			if id == "" || !validAttachmentOID(a) {
				trackerError(w, http.StatusNotFound, "not found")
				return
			}
			method, storeID, op, oid = http.MethodGet, id, "attachment", a
			break
		}
		if wp, ok := strings.CutPrefix(sub, "wiki/"); ok {
			name, tail, found := strings.Cut(wp, "/")
			if id == "" || !found || !validWikiName(name) {
				trackerError(w, http.StatusNotFound, "not found")
				return
			}
			if a, ok := strings.CutPrefix(tail, "attachments/"); ok {
				if !validAttachmentOID(a) {
					trackerError(w, http.StatusNotFound, "not found")
					return
				}
				method, storeID, op, oid, wiki = http.MethodGet, id, "wiki-image", a, name
				break
			}
			m, known := trackerWikiOps[tail]
			if !known {
				trackerError(w, http.StatusNotFound, "not found")
				return
			}
			method, storeID, op, wiki = m, id, "wiki-"+tail, name
			break
		}
		m, known := trackerStoreOps[sub]
		if !ok || id == "" || !known {
			trackerError(w, http.StatusNotFound, "not found")
			return
		}
		method, storeID, op = m, id, sub
	default:
		trackerError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != method {
		trackerError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if method == http.MethodPost && !sameOrigin(r) {
		trackerError(w, http.StatusForbidden, "cross-origin request refused")
		return
	}
	stores, err := d.tracker.Stores()
	if err != nil {
		trackerError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if storeID != "" && !hasTrackerStore(stores, storeID) {
		trackerError(w, http.StatusNotFound, "unknown store")
		return
	}
	// A write runs on a context the request's end does not cancel: a sync
	// the page starts as it closes (the drawer collapsing, or the user
	// leaving the page) still completes and records its outcome.
	ctx := context.WithoutCancel(r.Context())
	switch {
	case rest == "stores":
		writeJSON(w, http.StatusOK, map[string]any{"stores": nonNil(stores), "rev": d.trackerRev.Load(), "support": d.support != nil})
	case rest == "profiles" && method == http.MethodGet:
		d.trackerProfiles(w)
	case rest == "profiles":
		var p TrackerProfile
		if !decodeTrackerBody(w, r, &p) {
			return
		}
		if err := d.tracker.SaveProfile(p); err != nil {
			trackerError(w, http.StatusBadRequest, err.Error())
			return
		}
		d.TrackerChanged()
		d.trackerProfiles(w)
	case rest == "register":
		var req TrackerRegisterRequest
		if !decodeTrackerBody(w, r, &req) {
			return
		}
		res, err := d.tracker.Register(ctx, req)
		if err != nil {
			trackerError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		d.TrackerChanged()
		writeJSON(w, http.StatusOK, res)
	case op == "view":
		v, err := d.tracker.View(r.Context(), storeID)
		if err != nil {
			trackerError(w, http.StatusInternalServerError, err.Error())
			return
		}
		v.Seen = TrackerSeen{Badge: nonNil(v.Seen.Badge), Read: nonNil(v.Seen.Read)}
		writeJSON(w, http.StatusOK, v)
	case op == "search":
		// A read: nothing changed, so the revision stays.
		ids, err := d.tracker.Search(r.Context(), storeID, r.URL.Query().Get("q"))
		if err != nil {
			var bad *TrackerSearchError
			if errors.As(err, &bad) {
				trackerError(w, http.StatusBadRequest, bad.Message)
				return
			}
			trackerError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ids": nonNil(ids)})
	case op == "dispatch":
		d.trackerDispatch(ctx, w, r, storeID)
	case op == "check" || op == "sync":
		run := d.tracker.Check
		if op == "sync" {
			run = d.tracker.Sync
		}
		rep, err := run(ctx, storeID)
		// The host records a failed attempt too, so the view changed either way.
		d.TrackerChanged()
		if err != nil {
			trackerError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, rep)
	case op == "seen":
		var add TrackerSeen
		if !decodeTrackerBody(w, r, &add) {
			return
		}
		if err := d.tracker.MarkSeen(storeID, add); err != nil {
			trackerError(w, http.StatusInternalServerError, err.Error())
			return
		}
		d.TrackerChanged()
		w.WriteHeader(http.StatusNoContent)
	case op == "render":
		// A read: nothing changed, so the revision stays.
		var req TrackerRenderRequest
		if !decodeTrackerBody(w, r, &req) {
			return
		}
		html, err := d.tracker.Render(r.Context(), storeID, req)
		if err != nil {
			trackerError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"html": html})
	case op == "attachments":
		d.trackerStageAttachment(ctx, w, r, storeID)
	case op == "attachment":
		d.trackerReadAttachment(w, r, storeID, oid)
	case op == "wiki-image":
		d.trackerReadWikiImage(w, r, storeID, wiki, oid)
	case op == "wiki":
		wikis, err := d.tracker.WikiList(r.Context(), storeID)
		if err != nil {
			trackerError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"wikis": nonNil(wikis)})
	case op == "wiki-pages":
		pages, err := d.tracker.WikiPages(r.Context(), storeID, wiki)
		if err != nil {
			trackerWikiError(w, err)
			return
		}
		pages.Pages = nonNil(pages.Pages)
		writeJSON(w, http.StatusOK, pages)
	case op == "wiki-page":
		path := r.URL.Query().Get("path")
		if path == "" {
			trackerError(w, http.StatusBadRequest, "path required")
			return
		}
		page, err := d.tracker.WikiRead(r.Context(), storeID, wiki, path)
		if err != nil {
			trackerWikiError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, page)
	case op == "wiki-save":
		var req TrackerWikiSave
		if !decodeTrackerBody(w, r, &req) {
			return
		}
		res, err := d.tracker.WikiSave(ctx, storeID, wiki, req)
		if err != nil {
			var conflict *TrackerWikiConflict
			if errors.As(err, &conflict) {
				writeJSON(w, http.StatusConflict, map[string]any{"error": conflict.Error(), "conflict": conflict})
				return
			}
			trackerWikiError(w, err)
			return
		}
		d.TrackerChanged()
		res.Warnings = nonNil(res.Warnings)
		writeJSON(w, http.StatusOK, res)
	}
}

// trackerWikiError answers a wiki read or save error: 404 for an unknown
// wiki or page, 422 for a refused save, else 500.
func trackerWikiError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrTrackerWikiNotFound):
		trackerError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrTrackerWikiRefused):
		trackerError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		trackerError(w, http.StatusInternalServerError, err.Error())
	}
}

func hasTrackerStore(stores []TrackerStore, id string) bool {
	for _, s := range stores {
		if s.ID == id {
			return true
		}
	}
	return false
}

// trackerProfiles answers the saved profiles.
func (d *Daemon) trackerProfiles(w http.ResponseWriter) {
	p, err := d.tracker.Profiles()
	if err != nil {
		trackerError(w, http.StatusInternalServerError, err.Error())
		return
	}
	p.Profiles = nonNil(p.Profiles)
	writeJSON(w, http.StatusOK, p)
}

// trackerDispatch serves the dispatch route of store id.
func (d *Daemon) trackerDispatch(ctx context.Context, w http.ResponseWriter, r *http.Request, id string) {
	var body struct {
		Events []TrackerEvent `json:"events"`
	}
	if !decodeTrackerBody(w, r, &body) {
		return
	}
	if len(body.Events) == 0 {
		trackerError(w, http.StatusBadRequest, "no events")
		return
	}
	for i := range body.Events {
		body.Events[i].Origin = "human"
	}
	ids, err := d.tracker.Dispatch(ctx, id, body.Events)
	if err != nil {
		var refusal *TrackerRefusal
		if errors.As(err, &refusal) {
			writeJSON(w, http.StatusConflict, map[string]any{"error": refusal.Error(), "refusal": refusal})
			return
		}
		trackerError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.TrackerChanged()
	writeJSON(w, http.StatusOK, map[string]any{"ids": nonNil(ids)})
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

// trackerReadWikiImage serves image oid of wiki of store id.
func (d *Daemon) trackerReadWikiImage(w http.ResponseWriter, r *http.Request, id, wiki, oid string) {
	if crossSiteFetch(r) {
		trackerError(w, http.StatusNotFound, "not found")
		return
	}
	data, err := d.tracker.ReadWikiImage(r.Context(), id, wiki, oid)
	serveTrackerImage(w, data, err)
}

// serveTrackerImage answers an image read: data when it sniffs as an
// accepted image type, else 404, as for ErrTrackerAttachmentNotFound.
func serveTrackerImage(w http.ResponseWriter, data []byte, err error) {
	if errors.Is(err, ErrTrackerAttachmentNotFound) {
		trackerError(w, http.StatusNotFound, "not found")
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
