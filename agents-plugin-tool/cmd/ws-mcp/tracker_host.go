package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/url"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// dashboardRegisterTimeout bounds a registration from the dashboard: the
// same bound a tk:register gets for a clone of the remote.
const dashboardRegisterTimeout = 3 * time.Minute

// dashboardUnregisterTimeout bounds an unregister from the dashboard: the
// host refuses within seconds when the store is busy, so the rest is the
// deletion of the local clone, which a large store can stretch.
const dashboardUnregisterTimeout = time.Minute

// dashboardViewWait bounds how long a page read waits for a store's reload
// before it serves the last folded state (wstrackerhost.ViewWithin): a
// reload can wait behind a wiki save or another process's write holding
// the store's lock.
const dashboardViewWait = 500 * time.Millisecond

// dashboardLockWait bounds how long a page's wiki or image read waits for
// its store's lock, which a wiki save holds up to a network timeout, and,
// for an image this clone has yet to fetch, for the store's network lock,
// which a fetch or push holds as long; past it the read answers
// wsdashboard.ErrTrackerBusy and the page retries.
const dashboardLockWait = 500 * time.Millisecond

// pageRead bounds the store lock wait of the page reads run with it.
func pageRead(ctx context.Context) context.Context {
	return wstrackerhost.WithLockWait(ctx, dashboardLockWait)
}

// busyError maps a read that gave up on its store's lock onto
// wsdashboard.ErrTrackerBusy; ok is false for any other error.
func busyError(err error) (error, bool) {
	if errors.Is(err, wstrackerhost.ErrStoreBusy) {
		return fmt.Errorf("%w: %v", wsdashboard.ErrTrackerBusy, redacted(err)), true
	}
	return nil, false
}

// readError is a page read's error: busy, or only redacted.
func readError(err error) error {
	if b, ok := busyError(err); ok {
		return b
	}
	return redacted(err)
}

// dashboardTracker is the daemon's wsdashboard.TrackerHost over its one
// wstrackerhost.Host, the host its background poll also runs on. err is
// wstrackerhost.New's error: then every method returns it, so the tracker
// routes answer 503 and the page's "+ store" dialog shows why. Every string
// it hands the dashboard has credentials redacted. unregistered, when set,
// runs after each unregister that removed a store.
type dashboardTracker struct {
	h            *wstrackerhost.Host
	err          error
	unregistered func()
}

var (
	_ wsdashboard.TrackerHost   = (*dashboardTracker)(nil)
	_ wsdashboard.CounterSource = (*dashboardTracker)(nil)
)

// Counters returns the tracker host's reload counters
// (wsdashboard.CounterSource): the page reads' background reloads, those a
// read stopped waiting for, and those of the latter that signalled a
// change. A host that could not be created has none.
func (t *dashboardTracker) Counters() map[string]int64 {
	if t.err != nil || t.h == nil {
		return nil
	}
	s := t.h.ReloadStats()
	return map[string]int64{
		"tracker_reloads":              s.Reloads,
		"tracker_late_reloads":         s.Late,
		"tracker_late_reloads_changed": s.LateChanged,
	}
}

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
		out = append(out, dashboardStore(e))
	}
	return out, nil
}

// dashboardStore is a registry entry as the stores route lists it, its
// remote redacted.
func dashboardStore(e wstrackerhost.StoreEntry) wsdashboard.TrackerStore {
	return wsdashboard.TrackerStore{ID: e.ID, Label: e.Label, Member: e.Member, Remote: wstrackerhost.RedactCredentials(e.Remote), Hidden: e.Hidden}
}

// registryError maps a registry write's error onto the dashboard's: a
// store no longer registered is ErrTrackerUnknownStore, a refused label
// ErrTrackerBadLabel, each keeping the redacted message; the rest is only
// redacted.
func registryError(err error) error {
	for _, m := range []struct{ host, dash error }{
		{wstrackerhost.ErrNotRegistered, wsdashboard.ErrTrackerUnknownStore},
		{wstrackerhost.ErrBadLabel, wsdashboard.ErrTrackerBadLabel},
	} {
		if errors.Is(err, m.host) {
			return fmt.Errorf("%w: %v", m.dash, redacted(err))
		}
	}
	return redacted(err)
}

// Unregister removes the store from this machine. A refusal becomes the
// dashboard's, its reason a sentence that names the store.
func (t *dashboardTracker) Unregister(ctx context.Context, id, confirm string) error {
	if t.err != nil {
		return redacted(t.err)
	}
	ctx, cancel := context.WithTimeout(ctx, dashboardUnregisterTimeout)
	defer cancel()
	err := t.h.Unregister(ctx, id, confirm)
	if err == nil && t.unregistered != nil {
		t.unregistered()
	}
	var refusal *wstrackerhost.UnregisterRefusal
	if errors.As(err, &refusal) {
		return &wsdashboard.TrackerUnregisterRefusal{Kind: refusal.Kind,
			Message: wstrackerhost.RedactCredentials(fmt.Sprintf("The store %s was not unregistered: %s.", refusal.Store, refusal.Reason))}
	}
	return registryError(err)
}

func (t *dashboardTracker) Relabel(id, label string) (wsdashboard.TrackerStore, error) {
	if t.err != nil {
		return wsdashboard.TrackerStore{}, redacted(t.err)
	}
	e, err := t.h.Relabel(id, label)
	if err != nil {
		return wsdashboard.TrackerStore{}, registryError(err)
	}
	return dashboardStore(e), nil
}

func (t *dashboardTracker) SetHidden(id string, hidden bool) (wsdashboard.TrackerStore, error) {
	if t.err != nil {
		return wsdashboard.TrackerStore{}, redacted(t.err)
	}
	e, err := t.h.SetHidden(id, hidden)
	if err != nil {
		return wsdashboard.TrackerStore{}, registryError(err)
	}
	return dashboardStore(e), nil
}

func (t *dashboardTracker) View(ctx context.Context, id string) (wsdashboard.TrackerView, error) {
	if t.err != nil {
		return wsdashboard.TrackerView{}, redacted(t.err)
	}
	v, err := t.h.ViewWithin(ctx, id, 0, dashboardViewWait)
	if err != nil {
		return wsdashboard.TrackerView{}, redacted(err)
	}
	v.Redact()
	raw, err := json.Marshal(dashboardView{StoreView: v, HTML: renderViewHTML(id, v)})
	if err != nil {
		return wsdashboard.TrackerView{}, err
	}
	seen, err := t.h.Seen(id)
	if err != nil {
		return wsdashboard.TrackerView{}, redacted(err)
	}
	return wsdashboard.TrackerView{View: raw, Seen: wsdashboard.TrackerSeen{Badge: seen.Badge, Read: seen.Read}}, nil
}

// Search runs the search grammar over the same view the page reads, so
// the ids it answers name the items the page holds.
func (t *dashboardTracker) Search(ctx context.Context, id, query string) ([]string, error) {
	if t.err != nil {
		return nil, redacted(t.err)
	}
	q, err := wstracker.ParseQuery(query)
	if err != nil {
		return nil, &wsdashboard.TrackerSearchError{Message: err.Error()}
	}
	v, err := t.h.ViewWithin(ctx, id, 0, dashboardViewWait)
	if err != nil {
		return nil, redacted(err)
	}
	return v.View.Search(q), nil
}

// SearchHits is Search's twin for the palette: the same grammar and view,
// answering the places the items matched. The conversion to the dashboard's
// JSON-level type lives here because wsdashboard does not import wstracker.
func (t *dashboardTracker) SearchHits(ctx context.Context, id, query string) ([]wsdashboard.TrackerSearchHit, error) {
	if t.err != nil {
		return nil, redacted(t.err)
	}
	q, err := wstracker.ParseQuery(query)
	if err != nil {
		return nil, &wsdashboard.TrackerSearchError{Message: err.Error()}
	}
	v, err := t.h.ViewWithin(ctx, id, 0, dashboardViewWait)
	if err != nil {
		return nil, redacted(err)
	}
	found := v.View.SearchHits(q)
	out := make([]wsdashboard.TrackerSearchHit, len(found))
	for i, h := range found {
		out[i] = wsdashboard.TrackerSearchHit(h)
	}
	return out, nil
}

// dashboardView is the view JSON the tracker drawer reads: the store view
// plus html, its item bodies and comment texts rendered as markdown. The
// rendering is added here, not in wstrackerhost's view state, whose shape
// the mockup README and wstracker's golden tests pin and which the MCP
// tools also serve, and not in wsdashboard, which knows the view only as
// JSON; the page falls back to the plain text when html is absent (the
// mockup).
type dashboardView struct {
	*wstrackerhost.StoreView
	HTML viewHTML `json:"html"`
}

// viewHTML maps an item id to its rendered body and a comment id to its
// rendered text. An empty text, and a hidden comment's, has no entry.
type viewHTML struct {
	Items    map[string]template.HTML `json:"items"`
	Comments map[string]template.HTML `json:"comments"`
}

func renderViewHTML(id string, v *wstrackerhost.StoreView) viewHTML {
	out := viewHTML{Items: map[string]template.HTML{}, Comments: map[string]template.HTML{}}
	if v == nil || v.View == nil {
		return out
	}
	for iid, it := range v.Items {
		if it.Body != "" {
			out.Items[iid] = trackerText(it.Body, ownAttachments(id, attachmentOIDs(it.Attachments)))
		}
	}
	for cid, c := range v.Comments {
		if c.Body != "" && c.Hidden == nil {
			out.Comments[cid] = trackerText(c.Body, ownAttachments(id, attachmentOIDs(c.Attachments)))
		}
	}
	return out
}

func attachmentOIDs(l []wstracker.AttachmentView) []string {
	out := make([]string, len(l))
	for i, a := range l {
		out[i] = a.OID
	}
	return out
}

// trackerText renders tracker text as markdown. The text is any store
// member's, so it renders under the zero TextTargets but for own and the
// wiki link: a tag the author types is escaped text, a file path or local
// image is path text, a network image is its alt text, only an http(s)
// link (opening in a new tab, rel="noopener noreferrer"), an in-page
// fragment, or a wiki page link is an anchor, and item references and
// mermaid fences stay literal. An image written ![name](attachment:<oid>)
// is an <img> only when own serves that oid: the attachments of the same
// body or comment (ownAttachments), or of the wiki page being rendered
// (wikiAttachments); nil serves none. A link written [text](wiki:<name>/<path>)
// to a well-formed wiki name and page path is an anchor the page opens
// itself (href "#", data-wiki, data-page), as it opens an item reference.
func trackerText(s string, own func(oid string) string) template.HTML {
	return wswhiteboard.RenderText(s, wswhiteboard.TextTargets{AttachmentSrc: own, WikiLink: wikiLink})
}

// wikiLink is trackerText's wiki page link: the page itself opens the
// card from the anchor's marks, so the href is only an in-page anchor.
func wikiLink(name, path string) string {
	if !wsledger.ValidWikiName(name) || !wsledger.ValidWikiPath(path) {
		return ""
	}
	return "#"
}

// ownAttachments serves each oid of oids, a text's own attachment list, from
// store id's attachment read route; any other oid is not served.
func ownAttachments(id string, oids []string) func(string) string {
	return attachmentSources(id, "", nil, oids)
}

// attachmentSources serves the oids of tree from wiki's image route of
// store id and the oids of held from the store's attachment read route (a
// staged copy or a stored blob); any other oid is not served. Nil when
// there is nothing to serve.
func attachmentSources(id, wiki string, tree, held []string) func(string) string {
	if len(tree) == 0 && len(held) == 0 {
		return nil
	}
	store := "/tracker/stores/" + url.PathEscape(id)
	src := make(map[string]string, len(tree)+len(held))
	for _, oid := range held {
		if wsledger.ValidAttachmentOID(oid) {
			src[oid] = store + "/attachments/" + oid
		}
	}
	for _, oid := range tree {
		if wsledger.ValidAttachmentOID(oid) {
			src[oid] = store + "/wiki/" + wiki + "/attachments/" + oid
		}
	}
	return func(oid string) string { return src[oid] }
}

// Render is an editor's preview: the text through trackerText, the same
// renderer renderViewHTML runs, so the preview and the saved view match.
// The text's own attachments are the pending list the request carries,
// each kept only when its oid is staged on this clone or already stored
// (what a write may name), so a preview cannot open an image no save
// could. In wiki mode (req.Wiki) the images of that wiki's tip tree are
// served too, from the wiki image route, as the page's saved view serves
// them; a wiki that does not exist yet holds none. The route already
// checked that the store is listed.
func (t *dashboardTracker) Render(ctx context.Context, id string, req wsdashboard.TrackerRenderRequest) (string, error) {
	if t.err != nil {
		return "", redacted(t.err)
	}
	var held []string
	for _, a := range req.Attachments {
		ok, err := t.h.HasAttachment(ctx, id, a.OID)
		if err != nil {
			return "", redacted(err)
		}
		if ok {
			held = append(held, a.OID)
		}
	}
	var tree []string
	if req.Wiki != "" {
		if !wsledger.ValidWikiName(req.Wiki) {
			return "", fmt.Errorf("%w: %v", wsdashboard.ErrTrackerWikiNotFound, wsledger.ErrWikiBadName)
		}
		var err error
		if tree, err = t.h.WikiImages(ctx, id, req.Wiki); err != nil {
			return "", redacted(err)
		}
	}
	return string(trackerText(req.Text, attachmentSources(id, req.Wiki, tree, held))), nil
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

// attachmentError maps a host attachment error onto the dashboard's
// sentinel it matches, keeping the redacted message; any other error is
// only redacted.
func attachmentError(err error) error {
	if b, ok := busyError(err); ok {
		return b
	}
	for _, m := range []struct{ host, dash error }{
		{wstrackerhost.ErrNotImage, wsdashboard.ErrTrackerNotImage},
		{wstrackerhost.ErrAttachmentTooLarge, wsdashboard.ErrTrackerAttachmentTooLarge},
		{wstrackerhost.ErrAttachmentNotFound, wsdashboard.ErrTrackerAttachmentNotFound},
	} {
		if errors.Is(err, m.host) {
			return fmt.Errorf("%w: %v", m.dash, redacted(err))
		}
	}
	return redacted(err)
}

func (t *dashboardTracker) StageAttachment(ctx context.Context, id, name string, data []byte) (wsdashboard.TrackerAttachment, error) {
	if t.err != nil {
		return wsdashboard.TrackerAttachment{}, redacted(t.err)
	}
	a, err := t.h.StageAttachment(ctx, id, name, data)
	if err != nil {
		return wsdashboard.TrackerAttachment{}, attachmentError(err)
	}
	return wsdashboard.TrackerAttachment{OID: a.OID, Name: a.Name, Size: a.Size, Mime: a.Mime}, nil
}

func (t *dashboardTracker) ReadAttachment(ctx context.Context, id, oid string) ([]byte, error) {
	if t.err != nil {
		return nil, redacted(t.err)
	}
	data, err := t.h.ReadAttachment(pageRead(ctx), id, oid)
	if err != nil {
		return nil, attachmentError(err)
	}
	return data, nil
}

func (t *dashboardTracker) ReadWikiImage(ctx context.Context, id, wiki, oid string) ([]byte, error) {
	if t.err != nil {
		return nil, redacted(t.err)
	}
	data, err := t.h.ReadWikiImage(pageRead(ctx), id, wiki, oid)
	if err != nil {
		return nil, attachmentError(err)
	}
	return data, nil
}

// wikiError maps a host wiki error onto the dashboard's: an unknown wiki
// or page, or a malformed name or path, is ErrTrackerWikiNotFound on a
// read; a *WikiConflict is the dashboard's conflict; any other refusal of
// the saver's is ErrTrackerWikiRefused with its reason; the rest is only
// redacted.
func wikiError(err error, read bool) error {
	if b, ok := busyError(err); ok {
		return b
	}
	var conflict *wsledger.WikiConflict
	if errors.As(err, &conflict) {
		return &wsdashboard.TrackerWikiConflict{Path: conflict.Path, Kind: conflict.Kind, Reason: wstrackerhost.RedactCredentials(conflict.Reason),
			Base: conflict.Base, Theirs: conflict.Theirs, OursText: conflict.OursText, TheirsText: conflict.TheirsText}
	}
	if read {
		for _, e := range []error{wsledger.ErrWikiNotFound, wsledger.ErrWikiPageNotFound, wsledger.ErrWikiBadName, wsledger.ErrWikiBadPath} {
			if errors.Is(err, e) {
				return fmt.Errorf("%w: %v", wsdashboard.ErrTrackerWikiNotFound, redacted(err))
			}
		}
	} else if wstrackerhost.IsWikiRefusal(err) {
		return fmt.Errorf("%w: %v", wsdashboard.ErrTrackerWikiRefused, redacted(err))
	}
	return redacted(err)
}

func (t *dashboardTracker) WikiList(ctx context.Context, id string) ([]wsdashboard.TrackerWiki, error) {
	if t.err != nil {
		return nil, redacted(t.err)
	}
	list, err := t.h.WikiList(pageRead(ctx), id)
	if err != nil {
		return nil, readError(err)
	}
	out := make([]wsdashboard.TrackerWiki, 0, len(list))
	for _, w := range list {
		out = append(out, wsdashboard.TrackerWiki{Name: w.Name, Tip: w.Tip, Unpushed: w.Unpushed})
	}
	return out, nil
}

func (t *dashboardTracker) WikiPages(ctx context.Context, id, wiki string) (wsdashboard.TrackerWikiPages, error) {
	if t.err != nil {
		return wsdashboard.TrackerWikiPages{}, redacted(t.err)
	}
	pages, at, err := t.h.WikiPages(pageRead(ctx), id, wiki, "")
	if err != nil {
		return wsdashboard.TrackerWikiPages{}, wikiError(err, true)
	}
	return wsdashboard.TrackerWikiPages{Pages: pages, Commit: at}, nil
}

func (t *dashboardTracker) WikiRead(ctx context.Context, id, wiki, path string) (wsdashboard.TrackerWikiPage, error) {
	if t.err != nil {
		return wsdashboard.TrackerWikiPage{}, redacted(t.err)
	}
	ctx = pageRead(ctx)
	content, at, err := t.h.WikiRead(ctx, id, wiki, path, "")
	if err != nil {
		return wsdashboard.TrackerWikiPage{}, wikiError(err, true)
	}
	// The read succeeded, so path is a page path.
	path, _ = wsledger.CanonicalWikiPath(path)
	page := wsdashboard.TrackerWikiPage{Path: path, Kind: wsledger.WikiPageKind(path), Content: content, Commit: at}
	if page.Kind == wsledger.WikiKindHTML {
		// An HTML page is any store member's whole document: it runs only
		// in the sandboxed frame (wswhiteboard.SandboxedPage), never in the
		// dashboard's origin, and its attachment: references stay literal.
		page.HTML = string(wswhiteboard.SandboxedPage(wiki+"/"+path, content))
		return page, nil
	}
	tree, err := t.h.WikiImages(ctx, id, wiki)
	if err != nil {
		return wsdashboard.TrackerWikiPage{}, readError(err)
	}
	page.HTML = string(trackerText(content, attachmentSources(id, wiki, tree, nil)))
	return page, nil
}

// WikiSave applies the page's edit. The images the request names are the
// staged uploads of this clone (the paste went up through the attachment
// upload); the host writes each into the wiki with the page, and refuses
// one whose staged copy expired, as a dispatch does.
func (t *dashboardTracker) WikiSave(ctx context.Context, id, wiki string, req wsdashboard.TrackerWikiSave) (wsdashboard.TrackerWikiSaveResult, error) {
	if t.err != nil {
		return wsdashboard.TrackerWikiSaveResult{}, redacted(t.err)
	}
	save := wstrackerhost.WikiSave{Edit: wsledger.WikiEdit{Wiki: wiki, Base: req.Base, NewWiki: req.NewWiki, Op: req.Op, Path: req.Path, To: req.To,
		Content: req.Content, Commit: req.Commit, Message: req.Message}}
	for _, a := range req.Attachments {
		save.Staged = append(save.Staged, a.OID)
	}
	res, err := t.h.WikiSave(ctx, id, save)
	if err != nil {
		return wsdashboard.TrackerWikiSaveResult{}, wikiError(err, false)
	}
	out := wsdashboard.TrackerWikiSaveResult{Commit: res.Commit, Merged: res.Merged, Created: res.Created}
	for _, w := range res.Warnings {
		out.Warnings = append(out.Warnings, wstrackerhost.RedactCredentials(w.Ref+": "+w.Kind+" "+w.Detail))
	}
	return out, nil
}
