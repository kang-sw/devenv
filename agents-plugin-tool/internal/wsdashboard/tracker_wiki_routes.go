// tracker_wiki_routes.go holds wiki route helpers and wiki image reads.
package wsdashboard

import (
	"errors"
	"net/http"
)

// trackerWikiOps are the operations of one wiki,
// /tracker/stores/<id>/wiki/<name>/<op>.
var trackerWikiOps = map[string]string{
	"pages": http.MethodGet,
	"page":  http.MethodGet,
	"save":  http.MethodPost,
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

// trackerWikiError answers a wiki read or save error: 404 for an unknown
// wiki or page, 422 for a refused save, 503 for a busy store, else 500.
func trackerWikiError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrTrackerBusy):
		trackerBusy(w, err)
	case errors.Is(err, ErrTrackerWikiNotFound):
		trackerError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrTrackerWikiRefused):
		trackerError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		trackerError(w, http.StatusInternalServerError, err.Error())
	}
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
