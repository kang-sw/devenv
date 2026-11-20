// tracker_registry_routes_test.go tests tracker store registry routes.
package wsdashboard

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TestTrackerRegistryRoutes pins the store registry writes: label and
// hidden answer the store as written and bump the revision, the store list
// carries the hidden flag (the page filters), a refused label is 400 and a
// refused unregister 409 with its kind, neither bumping the revision, and
// an unregistered store leaves the list, after which every route naming it
// answers 404 as an unknown store.
func TestTrackerRegistryRoutes(t *testing.T) {
	fake := newFakeTracker()
	fake.stores = append(fake.stores, TrackerStore{ID: "solo", Label: "solo", Member: "ada"})
	d := startTrackerDaemon(t, fake)
	rev := trackerRev(t, d)
	bumped := func(what string, want bool) {
		t.Helper()
		now := trackerRev(t, d)
		if (now > rev) != want {
			t.Errorf("after %s the revision went %d -> %d; want bumped=%v", what, rev, now, want)
		}
		rev = now
	}

	code, body := call(d, http.MethodPost, "/tracker/stores/team/label", `{"label":"Renamed"}`)
	if code != http.StatusOK || body != `{"id":"team","label":"Renamed","member":"ada","remote":"https://***@example.invalid/x.git"}`+"\n" {
		t.Fatalf("label = %d %s", code, body)
	}
	bumped("a relabel", true)
	code, body = call(d, http.MethodPost, "/tracker/stores/team/label", `{"label":"two\nlines"}`)
	if code != http.StatusBadRequest || !strings.Contains(body, "bad store label") {
		t.Fatalf("bad label = %d %s, want 400", code, body)
	}
	bumped("a refused label", false)

	code, body = call(d, http.MethodPost, "/tracker/stores/solo/hidden", `{"hidden":true}`)
	if code != http.StatusOK || body != `{"id":"solo","label":"solo","member":"ada","remote":"","hidden":true}`+"\n" {
		t.Fatalf("hide = %d %s", code, body)
	}
	bumped("a hide", true)
	code, body = call(d, http.MethodGet, "/tracker/stores", "")
	if code != http.StatusOK || !strings.Contains(body, `{"id":"team","label":"Renamed","member":"ada","remote":"https://***@example.invalid/x.git"},{"id":"solo","label":"solo","member":"ada","remote":"","hidden":true}`) {
		t.Fatalf("stores after the hide = %d %s; want every store listed, the hidden one marked", code, body)
	}
	// A hidden store's routes serve it as any other.
	if code, body := call(d, http.MethodGet, "/tracker/stores/solo/view", ""); code != http.StatusOK {
		t.Fatalf("view of a hidden store = %d %s", code, body)
	}

	// An absent field is malformed, not a clear or a show, and reaches no host.
	before := len(fake.recorded())
	for _, op := range []string{"label", "hidden"} {
		if code, body := call(d, http.MethodPost, "/tracker/stores/team/"+op, `{}`); code != http.StatusBadRequest || !strings.Contains(body, op+" required") {
			t.Errorf("%s with no field = %d %s, want 400", op, code, body)
		}
	}
	if got := fake.recorded(); len(got) != before {
		t.Fatalf("a body with no field reached the host: %v", got[before:])
	}

	fake.unregErr = &TrackerUnregisterRefusal{Kind: TrackerUnregisterUnpushed, Message: "The store team was not unregistered: sync the store first."}
	code, body = call(d, http.MethodPost, "/tracker/stores/team/unregister", `{}`)
	if code != http.StatusConflict || body != `{"error":"The store team was not unregistered: sync the store first.","kind":"unpushed"}`+"\n" {
		t.Fatalf("refused unregister = %d %s", code, body)
	}
	bumped("a refused unregister", false)
	fake.unregErr = fmt.Errorf("%w: removed by another process", ErrTrackerUnknownStore)
	if code, body := call(d, http.MethodPost, "/tracker/stores/team/unregister", `{}`); code != http.StatusNotFound || body != `{"error":"unknown store"}`+"\n" {
		t.Fatalf("unregister of a store removed meanwhile = %d %s, want 404", code, body)
	}
	fake.unregErr = nil

	code, body = call(d, http.MethodPost, "/tracker/stores/solo/unregister", `{"confirm":"solo"}`)
	if code != http.StatusNoContent || body != "" {
		t.Fatalf("unregister = %d %s", code, body)
	}
	if got := fake.recorded(); got[len(got)-1] != "unregister solo solo" {
		t.Fatalf("the confirm did not reach the host: %v", got)
	}
	bumped("an unregister", true)
	if _, body := call(d, http.MethodGet, "/tracker/stores", ""); strings.Contains(body, `"solo"`) {
		t.Fatalf("an unregistered store is still listed: %s", body)
	}
	before = len(fake.recorded())
	for _, r := range []struct{ method, path, body string }{
		{http.MethodGet, "/tracker/stores/solo/view", ""},
		{http.MethodPost, "/tracker/stores/solo/sync", ""},
		{http.MethodPost, "/tracker/stores/solo/seen", `{"badge":["b"]}`},
		{http.MethodPost, "/tracker/stores/solo/unregister", `{"confirm":"solo"}`},
		{http.MethodPost, "/tracker/stores/solo/label", `{"label":"x"}`},
		{http.MethodPost, "/tracker/stores/solo/hidden", `{"hidden":false}`},
	} {
		if code, body := call(d, r.method, r.path, r.body); code != http.StatusNotFound || body != `{"error":"unknown store"}`+"\n" {
			t.Errorf("%s %s after the unregister = %d %s, want 404 unknown store", r.method, r.path, code, body)
		}
	}
	if got := fake.recorded(); len(got) != before {
		t.Fatalf("a route naming the removed store reached the host: %v", got[before:])
	}
}

// TestTrackerUnregisterRequiresTheDaemonsOwnOrigin: a cross-site unregister
// reaches no host and leaves the store; trackerPosts covers label and
// hidden, whose writes the other routes' checks need the store for.
func TestTrackerUnregisterRequiresTheDaemonsOwnOrigin(t *testing.T) {
	fake := newFakeTracker()
	d := startTrackerDaemon(t, fake)
	base := endpointURL(d.Endpoint(), "")
	for _, origin := range []string{"", "null", "http://evil.example"} {
		code, header, _ := postJSON(t, base+"/tracker/stores/team/unregister", origin, `{"confirm":"team"}`)
		if code != http.StatusForbidden {
			t.Errorf("unregister with Origin %q = %d, want 403", origin, code)
		}
		assertNoCORS(t, header)
	}
	if got := fake.recorded(); len(got) != 0 {
		t.Fatalf("a cross-origin unregister reached the host: %v", got)
	}
	if code, _, body := postJSON(t, base+"/tracker/stores/team/unregister", base, `{"confirm":"team"}`); code != http.StatusNoContent {
		t.Fatalf("same-origin unregister = %d %s", code, body)
	}
}
