package main

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
)

// dashboardRegisterTimeout bounds a registration from the dashboard: the
// same bound a tk:register gets for a clone of the remote.
const dashboardRegisterTimeout = 3 * time.Minute

// dashboardTracker is the daemon's wsdashboard.TrackerHost over its one
// wstrackerhost.Host, the host its background poll also runs on. err is
// wstrackerhost.New's error: then every method returns it, so the tracker
// routes answer 503 and the page's "+ store" dialog shows why. Every string
// it hands the dashboard has credentials redacted.
type dashboardTracker struct {
	h   *wstrackerhost.Host
	err error
}

var _ wsdashboard.TrackerHost = (*dashboardTracker)(nil)

// redacted returns err with credentials redacted from its message; nil
// stays nil.
func redacted(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(wstrackerhost.RedactCredentials(err.Error()))
}

func (t *dashboardTracker) Stores() ([]wsdashboard.TrackerStore, error) {
	if t.err != nil {
		return nil, redacted(t.err)
	}
	entries, err := t.h.Stores()
	if err != nil {
		return nil, redacted(err)
	}
	out := make([]wsdashboard.TrackerStore, 0, len(entries))
	for _, e := range entries {
		out = append(out, wsdashboard.TrackerStore{ID: e.ID, Label: e.Label, Member: e.Member, Remote: wstrackerhost.RedactCredentials(e.Remote)})
	}
	return out, nil
}

func (t *dashboardTracker) View(ctx context.Context, id string) (wsdashboard.TrackerView, error) {
	if t.err != nil {
		return wsdashboard.TrackerView{}, redacted(t.err)
	}
	v, err := t.h.View(ctx, id, 0)
	if err != nil {
		return wsdashboard.TrackerView{}, redacted(err)
	}
	v.Redact()
	raw, err := json.Marshal(v)
	if err != nil {
		return wsdashboard.TrackerView{}, err
	}
	seen, err := t.h.Seen(id)
	if err != nil {
		return wsdashboard.TrackerView{}, redacted(err)
	}
	return wsdashboard.TrackerView{View: raw, Seen: wsdashboard.TrackerSeen{Badge: seen.Badge, Read: seen.Read}}, nil
}

func (t *dashboardTracker) Dispatch(ctx context.Context, id string, events []wsdashboard.TrackerEvent) ([]string, error) {
	if t.err != nil {
		return nil, redacted(t.err)
	}
	batch := make([]wsledger.NewEvent, len(events))
	for i, ev := range events {
		// The page is the user's own hand, whatever the request said.
		batch[i] = wsledger.NewEvent{Type: ev.Type, Origin: wstracker.OriginHuman, Payload: ev.Payload}
	}
	ids, err := t.h.DispatchBatch(ctx, id, batch)
	if err != nil {
		var refusal *wstracker.Refusal
		if errors.As(err, &refusal) {
			return nil, &wsdashboard.TrackerRefusal{Index: refusal.Index, Type: refusal.Type, Reason: wstrackerhost.RedactCredentials(refusal.Reason)}
		}
		return nil, redacted(err)
	}
	out := make([]string, len(ids))
	for i, eid := range ids {
		out[i] = eid.String()
	}
	return out, nil
}

func (t *dashboardTracker) Check(ctx context.Context, id string) (wsdashboard.TrackerSyncReport, error) {
	return t.syncReport(ctx, id, t.h.Check)
}

func (t *dashboardTracker) Sync(ctx context.Context, id string) (wsdashboard.TrackerSyncReport, error) {
	return t.syncReport(ctx, id, t.h.Sync)
}

// syncReport runs a check or sync of store id. A store with a remote whose
// call fails answers the offline report with no error: the reason is
// already the view's sync error, which the sync line shows, and the page
// shows a report's error as a failure toast, which an unreachable remote
// is not.
func (t *dashboardTracker) syncReport(ctx context.Context, id string, run func(context.Context, string) (wsledger.SyncReport, error)) (wsdashboard.TrackerSyncReport, error) {
	if t.err != nil {
		return wsdashboard.TrackerSyncReport{}, redacted(t.err)
	}
	e, err := t.h.Entry(id)
	if err != nil {
		return wsdashboard.TrackerSyncReport{}, redacted(err)
	}
	rep, err := run(ctx, id)
	switch {
	case e.Remote == "" && err != nil:
		return wsdashboard.TrackerSyncReport{}, redacted(err)
	case e.Remote == "":
		return wsdashboard.TrackerSyncReport{NoRemote: true, Closed: rep.Closed, Received: rep.Received, Unpushed: rep.Pending.Count}, nil
	case err != nil:
		return wsdashboard.TrackerSyncReport{Offline: true, Unpushed: rep.Pending.Count}, nil
	}
	return wsdashboard.TrackerSyncReport{Closed: rep.Closed, Pushed: rep.Pushed, Received: rep.Received, Unpushed: rep.Pending.Count}, nil
}

func (t *dashboardTracker) MarkSeen(id string, add wsdashboard.TrackerSeen) error {
	if t.err != nil {
		return redacted(t.err)
	}
	return redacted(t.h.MarkSeen(id, wstrackerhost.Seen{Badge: add.Badge, Read: add.Read}))
}

func (t *dashboardTracker) Profiles() (wsdashboard.TrackerProfiles, error) {
	if t.err != nil {
		return wsdashboard.TrackerProfiles{}, redacted(t.err)
	}
	saved, err := t.h.Profiles()
	if err != nil {
		return wsdashboard.TrackerProfiles{}, redacted(err)
	}
	out := wsdashboard.TrackerProfiles{Last: saved.Last, Profiles: make([]wsdashboard.TrackerProfile, 0, len(saved.Profiles))}
	for _, p := range saved.Profiles {
		out.Profiles = append(out.Profiles, wsdashboard.TrackerProfile(p))
	}
	return out, nil
}

func (t *dashboardTracker) SaveProfile(p wsdashboard.TrackerProfile) error {
	if t.err != nil {
		return redacted(t.err)
	}
	return redacted(t.h.SaveProfile(wstracker.Profile(p)))
}

func (t *dashboardTracker) Register(ctx context.Context, req wsdashboard.TrackerRegisterRequest) (wsdashboard.TrackerRegisterResult, error) {
	if t.err != nil {
		return wsdashboard.TrackerRegisterResult{}, redacted(t.err)
	}
	ctx, cancel := context.WithTimeout(ctx, dashboardRegisterTimeout)
	defer cancel()
	var profile *wstracker.Profile
	if req.Profile != nil {
		p := wstracker.Profile(*req.Profile)
		profile = &p
	}
	res, err := t.h.Register(ctx, wstrackerhost.RegisterRequest{ID: req.ID, Label: req.Label, Remote: req.Remote, Member: req.Member, Profile: profile, Origin: wstracker.OriginHuman})
	if err != nil {
		return wsdashboard.TrackerRegisterResult{}, redacted(err)
	}
	return wsdashboard.TrackerRegisterResult{Store: res.Entry.ID, Creator: res.Creator, Profile: wsdashboard.TrackerProfile(res.Profile)}, nil
}
