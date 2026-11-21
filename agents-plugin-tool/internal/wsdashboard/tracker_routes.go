package wsdashboard

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// The tracker routes serve the tracker drawer of the dashboard pages. The
// daemon hosts no tracker itself: DaemonOptions.Tracker is a host the
// caller supplies, and this package knows it only through the JSON-level
// types in tracker_types.go, so the dashboard stays free of the tracker packages.
//
//	GET  /tracker/stores                 {"stores": [...], "rev": N, "support": B}
//	GET  /tracker/stores/<id>/view       {"view": ..., "seen": {"badge", "read"}}
//	GET  /tracker/stores/<id>/summary    {"cursor", "reset", "items", "removed", "globals", "feed"}
//	GET  /tracker/stores/<id>/delta?since=<cursor>  changed rows/global fields, or inline reset; equal cursor 304
//	GET  /tracker/stores/<id>/detail?id=<item>&id=<item>  versioned bundles from the same published snapshot
//	GET  /tracker/stores/<id>/search?q=<query>[&excerpts=1]
//	                                     {"ids": [...]}: the items passing the search, in tree order;
//	                                     with excerpts=1 also "hits": [...], one per place an item
//	                                     matched (TrackerSearchHit), ids being the distinct hit ids;
//	                                     a malformed query is 400 with the host's message
//	POST /tracker/stores/<id>/dispatch   {"events": [...]} -> {"ids": [...]}; a refusal is 409
//	POST /tracker/stores/<id>/check      the sync report
//	POST /tracker/stores/<id>/sync       the sync report
//	POST /tracker/stores/<id>/seen       {"badge", "read"} -> 204
//	POST /tracker/stores/<id>/render     {"text", "attachments"} -> {"html"}: an editor's preview
//	POST /tracker/stores/<id>/unregister {"confirm"} -> 204: the store leaves this machine, its
//	                                     local clone deleted, the remote kept; a refusal is 409
//	                                     with {"error", "kind"}
//	POST /tracker/stores/<id>/label      {"label"} -> the store as written; a refused label is 400
//	POST /tracker/stores/<id>/hidden     {"hidden"} -> the store as written
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
// The store list and the view carry an ETag and answer a matching
// If-None-Match 304 with no body; while nothing changed they answer from
// memory without reading the host (trackerAnswers, tracker_answers.go).
//
// The store list holds every registered store, a hidden one marked
// "hidden": hiding is the page's to apply (it leaves the store out of its
// handle row), and every store route serves a hidden store as any other.
// Unregister, label, and hidden change only this machine's registry, never
// the store. Unregister's confirm is the store id the user typed, which a
// store with no remote (its only copy is the one deleted) requires; the
// host ignores it for a store with a remote, whose refusal guards unpushed
// writes instead. The refusal's kind is one of the TrackerUnregister*
// kinds. A store removed meanwhile (by another process) answers 404 as an
// unknown store does.
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

// trackerStoreOps are the operations of the store route
// /tracker/stores/<id>/<op>; view, search, and wiki (the wiki list) are
// the GETs.
// The attachment read, /tracker/stores/<id>/attachments/<oid>, is the op
// "attachment". A route under /tracker/stores/<id>/wiki/<name>/ is a
// wiki op (trackerWikiOps) with the op prefixed "wiki-"; the image read
// /tracker/stores/<id>/wiki/<name>/attachments/<oid> is "wiki-image".
var trackerStoreOps = map[string]string{
	"view":        http.MethodGet,
	"summary":     http.MethodGet,
	"delta":       http.MethodGet,
	"detail":      http.MethodGet,
	"search":      http.MethodGet,
	"dispatch":    http.MethodPost,
	"check":       http.MethodPost,
	"sync":        http.MethodPost,
	"seen":        http.MethodPost,
	"attachments": http.MethodPost,
	"render":      http.MethodPost,
	"wiki":        http.MethodGet,
	"unregister":  http.MethodPost,
	"label":       http.MethodPost,
	"hidden":      http.MethodPost,
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
	// The revision is read before the data revision, and both before any
	// read of the host: an answer is never newer than the revision it names
	// (see TrackerChanged). The page's two reads (the store list and a
	// view) answer from trackerAnswers while the data revision stands.
	rev := d.trackerRev.Load()
	data := d.trackerData.Load()
	now := time.Now()
	page := method == http.MethodGet && (rest == "stores" || op == "view" || op == "summary" || op == "delta" || op == "detail")
	var stores []TrackerStore
	cached := false
	if page {
		stores, cached = d.trackerAnswers.storeList(data, now)
	}
	if !cached {
		var err error
		if stores, err = d.tracker.Stores(); err != nil {
			trackerError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		if page {
			d.trackerAnswers.putStoreList(stores, trackerStamp{data: data, at: now})
		}
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
		body, err := json.Marshal(map[string]any{"stores": nonNil(stores), "rev": rev, "support": d.support != nil})
		if err != nil {
			trackerError(w, http.StatusInternalServerError, err.Error())
			return
		}
		body = append(body, '\n')
		writeTrackerAnswer(w, r, body, trackerETag(body))
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
		// No store list or view shows a profile.
		d.trackerSignal()
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
	case op == "summary" || op == "delta" || op == "detail":
		d.trackerProjectionRead(w, r, storeID, op, data, now)
	case op == "view":
		if a, ok := d.trackerAnswers.view(storeID, data, now); ok {
			writeTrackerAnswer(w, r, a.body, a.etag)
			return
		}
		gen := d.trackerAnswers.seenGeneration()
		v, err := d.tracker.View(r.Context(), storeID)
		if err != nil {
			trackerError(w, http.StatusInternalServerError, err.Error())
			return
		}
		v.Seen = TrackerSeen{Badge: nonNil(v.Seen.Badge), Read: nonNil(v.Seen.Read)}
		a := trackerViewAnswer{stamp: trackerStamp{data: data, at: now}, view: v}
		if err := a.encode(); err != nil {
			trackerError(w, http.StatusInternalServerError, err.Error())
			return
		}
		d.trackerAnswers.putView(storeID, a, gen)
		writeTrackerAnswer(w, r, a.body, a.etag)
	case op == "search":
		// A read: nothing changed, so the revision stays.
		query := r.URL.Query()
		var ids []string
		var hits []TrackerSearchHit
		var err error
		excerpts := query.Get("excerpts") == "1"
		if excerpts {
			hits, err = d.tracker.SearchHits(r.Context(), storeID, query.Get("q"))
		} else {
			ids, err = d.tracker.Search(r.Context(), storeID, query.Get("q"))
		}
		if err != nil {
			var bad *TrackerSearchError
			if errors.As(err, &bad) {
				trackerError(w, http.StatusBadRequest, bad.Message)
				return
			}
			trackerError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !excerpts {
			writeJSON(w, http.StatusOK, map[string]any{"ids": nonNil(ids)})
			return
		}
		// Hits of one item are adjacent, so the distinct ids keep tree order.
		ids = []string{}
		for _, h := range hits {
			if len(ids) == 0 || ids[len(ids)-1] != h.ID {
				ids = append(ids, h.ID)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"ids": ids, "hits": nonNil(hits)})
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
		// Only this daemon writes the marks, so the cached view takes them
		// as the file did, and no view is read again for them.
		d.trackerAnswers.addSeen(storeID, add)
		d.trackerSignal()
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
	case op == "unregister" || op == "label" || op == "hidden":
		d.trackerRegistryWrite(ctx, w, r, storeID, op)
	case op == "attachments":
		d.trackerStageAttachment(ctx, w, r, storeID)
	case op == "attachment":
		d.trackerReadAttachment(w, r, storeID, oid)
	case op == "wiki-image":
		d.trackerReadWikiImage(w, r, storeID, wiki, oid)
	case op == "wiki":
		wikis, err := d.tracker.WikiList(r.Context(), storeID)
		if err != nil {
			trackerWikiError(w, err)
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
