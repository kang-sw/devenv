// tracker_helpers.go holds tracker revision, response, and request helpers.
package wsdashboard

import (
	"encoding/json"
	"errors"
	"net/http"
)

// TrackerChanged bumps the tracker revision and wakes every stream, which
// then sends the new revision. Safe from any goroutine, also before the
// first request. It drops the cached page reads (trackerAnswers) too: the
// data revision moves before the revision the streams send, so a page that
// reads after the new revision never gets an answer cached before it.
func (d *Daemon) TrackerChanged() {
	d.trackerData.Add(1)
	d.trackerSignal()
}

// trackerSignal bumps the revision the streams send and wakes them,
// keeping the cached page reads: for a change the daemon applied to those
// answers itself, or one no answer shows.
func (d *Daemon) trackerSignal() {
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

func hasTrackerStore(stores []TrackerStore, id string) bool {
	for _, s := range stores {
		if s.ID == id {
			return true
		}
	}
	return false
}
