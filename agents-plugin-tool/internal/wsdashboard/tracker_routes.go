package wsdashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// The tracker routes serve the tracker drawer of the dashboard pages. The
// daemon hosts no tracker itself: DaemonOptions.Tracker is a host the
// caller supplies, and this package knows it only through the JSON-level
// types below, so the dashboard stays free of the tracker packages.
//
//	GET  /tracker/stores                 {"stores": [...], "rev": N}
//	GET  /tracker/stores/<id>/view       {"view": ..., "seen": {"badge", "read"}}
//	POST /tracker/stores/<id>/dispatch   {"events": [...]} -> {"ids": [...]}; a refusal is 409
//	POST /tracker/stores/<id>/check      the sync report
//	POST /tracker/stores/<id>/sync       the sync report
//	POST /tracker/stores/<id>/seen       {"badge", "read"} -> 204
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
// The revision counter (TrackerChanged) bumps after every write these
// routes run and on the host's own poll round; each SSE stream sends it as
// "event: tracker" when it changed (sse.go), and the page re-reads.

// trackerBodyLimit bounds a tracker POST body.
const trackerBodyLimit = 256 << 10

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
// /tracker/stores/<id>/<op>; view is the only GET.
var trackerStoreOps = map[string]string{
	"view":     http.MethodGet,
	"dispatch": http.MethodPost,
	"check":    http.MethodPost,
	"sync":     http.MethodPost,
	"seen":     http.MethodPost,
}

// TrackerChanged bumps the tracker revision and wakes every stream, which
// then sends the new revision. Safe from any goroutine, also before the
// first request.
func (d *Daemon) TrackerChanged() {
	d.trackerRev.Add(1)
	d.broker.wake("")
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
	var method, storeID, op string
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
		writeJSON(w, http.StatusOK, map[string]any{"stores": nonNil(stores), "rev": d.trackerRev.Load()})
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
