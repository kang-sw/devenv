// tracker_registry_routes.go holds tracker profile and store registry routes.
package wsdashboard

import (
	"context"
	"errors"
	"net/http"
)

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

// trackerRegistryWrite serves the registry writes of store id: unregister,
// label, and hidden. A refused unregister writes nothing, so it leaves the
// revision; every write that landed bumps it.
func (d *Daemon) trackerRegistryWrite(ctx context.Context, w http.ResponseWriter, r *http.Request, id, op string) {
	var body struct {
		Confirm string  `json:"confirm"`
		Label   *string `json:"label"`
		Hidden  *bool   `json:"hidden"`
	}
	if !decodeTrackerBody(w, r, &body) {
		return
	}
	var store TrackerStore
	var err error
	switch op {
	case "unregister":
		err = d.tracker.Unregister(ctx, id, body.Confirm)
	case "label":
		// An absent label is a malformed request, not a clear: "" clears.
		if body.Label == nil {
			trackerError(w, http.StatusBadRequest, "label required")
			return
		}
		store, err = d.tracker.Relabel(id, *body.Label)
	case "hidden":
		if body.Hidden == nil {
			trackerError(w, http.StatusBadRequest, "hidden required")
			return
		}
		store, err = d.tracker.SetHidden(id, *body.Hidden)
	}
	var refusal *TrackerUnregisterRefusal
	switch {
	case errors.As(err, &refusal):
		writeJSON(w, http.StatusConflict, map[string]string{"error": refusal.Message, "kind": refusal.Kind})
		return
	case errors.Is(err, ErrTrackerUnknownStore):
		trackerError(w, http.StatusNotFound, "unknown store")
		return
	case errors.Is(err, ErrTrackerBadLabel):
		trackerError(w, http.StatusBadRequest, err.Error())
		return
	case err != nil:
		trackerError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.TrackerChanged()
	if op == "unregister" {
		d.trackerAnswers.forgetProjection(id)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, store)
}
