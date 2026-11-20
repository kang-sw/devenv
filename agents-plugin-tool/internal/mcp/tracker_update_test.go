// Tracker update tests.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
)

const trackerPersonal = `<tk:register store="me" member="ada" org="Acme" family="Kim" given="Ada"/>`

func TestTrackerUpdateRegistersAndJoinsWithSavedProfile(t *testing.T) {
	f := newTrackerLeadFixture(t)
	ctx := context.Background()
	out := f.trackerUpdate(t, trackerPersonal)
	for _, want := range []string{"registered me: created, member ada, no remote", `profile: org="Acme" family="Kim" given="Ada"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("personal register output missing %q:\n%s", want, out)
		}
	}

	// Another machine creates the team store on the remote; this one joins
	// it with no profile attributes, so with the profile saved above.
	remote := initTrackerRemote(t)
	bo := trackerOtherMachine(t)
	if _, err := bo.Register(ctx, wstrackerhost.RegisterRequest{ID: "team", Remote: remote, Member: "bo", Profile: &wstracker.Profile{Given: "Bo"}}); err != nil {
		t.Fatalf("bo creates: %v", err)
	}
	out = f.trackerUpdate(t, `<tk:register store="team" member="ada" remote="`+remote+`" label="Team"/>`)
	for _, want := range []string{"registered team: joined, member ada, remote " + remote, `profile: org="Acme" family="Kim" given="Ada"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("join output missing %q:\n%s", want, out)
		}
	}
	if _, err := bo.Sync(ctx, "team"); err != nil {
		t.Fatalf("bo syncs: %v", err)
	}
	v, err := bo.View(ctx, "team", 0)
	if err != nil {
		t.Fatal(err)
	}
	if m, ok := v.Members["ada"]; !ok || m.Profile != (wstracker.Profile{Org: "Acme", Family: "Kim", Given: "Ada"}) {
		t.Fatalf("bo must see ada joined with the saved profile: %+v", v.Members["ada"])
	}

	list := f.trackerQuery(t, nil)
	for _, want := range []string{"- me, member ada, no remote", "- team (Team), member ada, remote " + remote, `- org="Acme" family="Kim" given="Ada" (used last)`} {
		if !strings.Contains(list, want) {
			t.Fatalf("store list missing %q:\n%s", want, list)
		}
	}

	text := f.trackerRefused(t, "tracker.update", map[string]any{"commands": `<tk:register store="team" member="ada" remote="` + remote + `"/>`})
	if !strings.Contains(text, `"team" is already registered on this machine`) {
		t.Fatalf("a second register of one store id must be refused before the network: %s", text)
	}
	text = f.trackerRefused(t, "tracker.update", map[string]any{"commands": `<tk:register store="team2" member="bo" remote="` + remote + `"/>`})
	if !strings.Contains(text, "Nothing was registered") {
		t.Fatalf("a join under a taken member id must fail and register nothing: %s", text)
	}
	if strings.Contains(f.trackerQuery(t, nil), "team2") {
		t.Fatalf("a failed register must leave no registry entry")
	}
}

// trackerCorruptAfterPush breaks the tracker registry once, after the
// first push succeeds, so a registration fails after it reached the remote.
type trackerCorruptAfterPush struct {
	registry string
	done     bool
}

func (r *trackerCorruptAfterPush) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	out, err := wsgit.ExecRunner{}.RunGit(ctx, root, args...)
	if err == nil && !r.done && slices.Contains(args, "push") {
		r.done = true
		if werr := os.WriteFile(r.registry, []byte("not json"), 0o600); werr != nil {
			return nil, werr
		}
	}
	return out, err
}

// A register that fails after its push says what the remote already holds
// and how to finish, instead of claiming nothing was registered; sending
// the same command again finishes it.
func TestTrackerRegisterFailureAfterPushNamesWhatTheRemoteHolds(t *testing.T) {
	f := newTrackerLeadFixture(t)
	root, err := wstrackerhost.Root(wsconfig.Options{})
	if err != nil {
		t.Fatal(err)
	}
	registry := filepath.Join(root, "stores.json")
	f.s.tracker.opts.Runner = &trackerCorruptAfterPush{registry: registry}
	register := `<tk:register store="team" member="ada" given="Ada" remote="` + initTrackerRemote(t) + `"/>`
	text := f.trackerRefused(t, "tracker.update", map[string]any{"commands": register})
	if !strings.Contains(text, "Nothing was registered on this machine, but the remote already holds the new store, with member ada as its sole admin; send the same tk:register again to finish") {
		t.Fatalf("a failure after the push must name what the remote holds: %s", text)
	}
	if err := os.Remove(registry); err != nil {
		t.Fatal(err)
	}
	if out := f.trackerUpdate(t, register); !strings.Contains(out, "registered team: created, member ada") {
		t.Fatalf("sending the register again must finish it: %s", out)
	}
}

// A write reaches the other clone through tk:sync and the other clone's
// write reaches this one through the fetch before a query.
func TestTrackerSyncCommandPushesAndQueryFetches(t *testing.T) {
	f := newTrackerLeadFixture(t)
	ctx := context.Background()
	remote := initTrackerRemote(t)
	out := f.trackerUpdate(t, `<tk:register store="team" member="ada" remote="`+remote+`" given="Ada"/>`)
	if !strings.Contains(out, "registered team: created") {
		t.Fatalf("register on an empty remote must create: %s", out)
	}
	bo := trackerOtherMachine(t)
	if _, err := bo.Register(ctx, wstrackerhost.RegisterRequest{ID: "team", Remote: remote, Member: "bo", Profile: &wstracker.Profile{Given: "Bo"}}); err != nil {
		t.Fatalf("bo joins: %v", err)
	}
	ids := trackerWrote(t, f.trackerUpdate(t, `<tk:store name="team"><tk:item.create kind="task" title="for bo"/></tk:store>`))
	if out := f.trackerUpdate(t, `<tk:sync store="team"/>`); !strings.Contains(out, "team: pushed") {
		t.Fatalf("tk:sync must push the write: %s", out)
	}
	if _, err := bo.Sync(ctx, "team"); err != nil {
		t.Fatal(err)
	}
	v, err := bo.View(ctx, "team", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := v.Items[strings.Replace(ids["1"], "-", "#", 1)]; !ok {
		t.Fatalf("bo must see ada's synced write")
	}

	payload, _ := json.Marshal(map[string]any{"kind": "task", "title": "from bo"})
	if _, err := bo.Dispatch(ctx, "team", []wsledger.NewEvent{{Type: wstracker.TypeItemCreate, Payload: payload}}); err != nil {
		t.Fatal(err)
	}
	if _, err := bo.Sync(ctx, "team"); err != nil {
		t.Fatal(err)
	}
	if all := f.trackerQuery(t, map[string]any{"store": "team", "scope": "all"}); !strings.Contains(all, "from bo") {
		t.Fatalf("a query must fetch the other clone's write first:\n%s", all)
	}
	if out := f.trackerUpdate(t, `<tk:sync/>`); !strings.Contains(out, "team: ") {
		t.Fatalf("tk:sync without store must sync every store: %s", out)
	}
}

// A wiki sync warning carries git's error text, which can name the remote
// with its credentials; the sync report must not echo them.
func TestTrackerSyncReportRedactsWarningCredentials(t *testing.T) {
	rep := wsledger.SyncReport{Warnings: []wsledger.Warning{{
		Ref:    "refs/ws/wiki/docs",
		Kind:   wsledger.WarnWikiSyncFailed,
		Detail: "ledger: wiki push: fatal: unable to access 'https://user:s3cr3t-token@example.com/store.git/': 403",
	}}}
	got := formatTrackerSyncReport(rep)
	if strings.Contains(got, "s3cr3t-token") || strings.Contains(got, "user:") {
		t.Fatalf("sync report leaks the remote's credentials: %q", got)
	}
	if !strings.Contains(got, "warning: refs/ws/wiki/docs "+wsledger.WarnWikiSyncFailed) || !strings.Contains(got, "https://***@example.com/store.git/") {
		t.Fatalf("sync report = %q, want the warning with its redacted remote", got)
	}
}

// trackerEntry returns store id's registry entry through the fixture's
// host, or ok false when it is not registered.
func (f *whiteboardFixture) trackerEntry(t *testing.T, id string) (wstrackerhost.StoreEntry, bool) {
	t.Helper()
	h, err := f.s.trackerHost()
	if err != nil {
		t.Fatal(err)
	}
	e, err := h.Entry(id)
	if errors.Is(err, wstrackerhost.ErrNotRegistered) {
		return wstrackerhost.StoreEntry{}, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return e, true
}

// tk:unregister of a store with a remote is refused while it holds an
// unpushed write, confirm or not; once synced it removes the entry and the
// clone, confirm ignored, leaves the remote, and tk:register restores it.
func TestTrackerUnregisterRemoteStore(t *testing.T) {
	f := newTrackerLeadFixture(t)
	remote := initTrackerRemote(t)
	f.trackerUpdate(t, `<tk:register store="team" member="ada" remote="`+remote+`" given="Ada"/>`)
	f.trackerUpdate(t, `<tk:store name="team"><tk:item.create kind="task" title="kept"/></tk:store>`)
	for _, commands := range []string{`<tk:unregister store="team"/>`, `<tk:unregister store="team" confirm="team"/>`} {
		text := f.trackerRefused(t, "tracker.update", map[string]any{"commands": commands})
		for _, want := range []string{`"team" was not unregistered`, "1 own event that is not pushed yet", "sync the store first"} {
			if !strings.Contains(text, want) {
				t.Fatalf("%s with an unpushed write must be refused, missing %q: %s", commands, want, text)
			}
		}
		if _, ok := f.trackerEntry(t, "team"); !ok {
			t.Fatalf("%s: a refused unregister must keep the store", commands)
		}
	}
	e, _ := f.trackerEntry(t, "team")
	f.trackerUpdate(t, `<tk:sync store="team"/>`)
	out := f.trackerUpdate(t, `<tk:unregister store="team" confirm="not-the-id"/>`)
	for _, want := range []string{"unregistered team: removed from this machine", "the remote " + remote + " was left as it is", "<tk:register> can register the store again"} {
		if !strings.Contains(out, want) {
			t.Fatalf("unregister answer missing %q: %s", want, out)
		}
	}
	if _, ok := f.trackerEntry(t, "team"); ok {
		t.Fatalf("an unregistered store must leave the registry")
	}
	if _, err := os.Stat(e.Dir); !os.IsNotExist(err) {
		t.Fatalf("an unregistered store's clone must be deleted: %v", err)
	}
	if list := f.trackerQuery(t, nil); strings.Contains(list, "team") {
		t.Fatalf("the store list must drop an unregistered store:\n%s", list)
	}
	if out := f.trackerUpdate(t, `<tk:register store="team" member="ada" remote="`+remote+`"/>`); !strings.Contains(out, "registered team: created, member ada, remote "+remote) {
		// "created": the store's genesis lists ada, its creator.
		t.Fatalf("the remote must be left so the store registers again: %s", out)
	}
	if all := f.trackerQuery(t, map[string]any{"store": "team", "scope": "all"}); !strings.Contains(all, "kept") {
		t.Fatalf("the re-registered store must hold the pushed write:\n%s", all)
	}
}

// A store with no remote holds its only copy here: tk:unregister removes it
// only when confirm types its id, and the refusal says so.
func TestTrackerUnregisterNoRemoteStoreNeedsConfirm(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)
	e, _ := f.trackerEntry(t, "me")
	for _, commands := range []string{`<tk:unregister store="me"/>`, `<tk:unregister store="me" confirm="you"/>`} {
		text := f.trackerRefused(t, "tracker.update", map[string]any{"commands": commands})
		for _, want := range []string{`"me" was not unregistered`, "no remote", "lost permanently", `<tk:unregister store="me" confirm="me"/>`} {
			if !strings.Contains(text, want) {
				t.Fatalf("%s must be refused, missing %q: %s", commands, want, text)
			}
		}
		if _, ok := f.trackerEntry(t, "me"); !ok {
			t.Fatalf("%s: a refused unregister must keep the store", commands)
		}
	}
	out := f.trackerUpdate(t, `<tk:unregister store="me" confirm="me"/>`)
	if !strings.Contains(out, "unregistered me: removed from this machine, and its data deleted") {
		t.Fatalf("a confirmed unregister answer: %s", out)
	}
	if _, ok := f.trackerEntry(t, "me"); ok {
		t.Fatalf("a confirmed unregister must leave the registry")
	}
	if _, err := os.Stat(e.Dir); !os.IsNotExist(err) {
		t.Fatalf("a confirmed unregister must delete the clone: %v", err)
	}
}

// tk:relabel sets and clears the machine-local label and refuses a bad one.
func TestTrackerRelabel(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)
	if out := f.trackerUpdate(t, `<tk:relabel store="me" label=" Mine "/>`); !strings.Contains(out, `relabeled me: label "Mine"`) {
		t.Fatalf("relabel answer: %s", out)
	}
	if list := f.trackerQuery(t, nil); !strings.Contains(list, "- me (Mine), member ada") {
		t.Fatalf("the store list must show the new label:\n%s", list)
	}
	if out := f.trackerUpdate(t, `<tk:relabel store="me" label=""/>`); !strings.Contains(out, "relabeled me: label cleared") {
		t.Fatalf("an empty label must clear it: %s", out)
	}
	if list := f.trackerQuery(t, nil); !strings.Contains(list, "- me, member ada") {
		t.Fatalf("a cleared label must show the id alone:\n%s", list)
	}
	text := f.trackerRefused(t, "tracker.update", map[string]any{"commands": `<tk:relabel store="me" label="` + strings.Repeat("x", wstrackerhost.MaxLabelLen+1) + `"/>`})
	if !strings.Contains(text, "at most 80 characters") || !strings.Contains(text, "Nothing changed") {
		t.Fatalf("a too-long label must be refused: %s", text)
	}
	if e, _ := f.trackerEntry(t, "me"); e.Label != "" {
		t.Fatalf("a refused relabel must change nothing: %q", e.Label)
	}
	text = f.trackerRefused(t, "tracker.update", map[string]any{"commands": `<tk:relabel store="me"/>`})
	if !strings.Contains(text, `label: required (label="" clears the label back to the store id)`) {
		t.Fatalf("relabel without label must be refused: %s", text)
	}
}

// tk:hide and tk:unhide flip the machine-local hidden flag; a hidden store
// keeps answering every tracker read and write.
func TestTrackerHideUnhide(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)
	if out := f.trackerUpdate(t, `<tk:hide store="me"/>`); !strings.Contains(out, "hid me:") {
		t.Fatalf("hide answer: %s", out)
	}
	if e, _ := f.trackerEntry(t, "me"); !e.Hidden {
		t.Fatalf("hide must set the registry flag")
	}
	if list := f.trackerQuery(t, nil); !strings.Contains(list, "- me, member ada, hidden on the dashboard, no remote") {
		t.Fatalf("the store list must keep a hidden store, marked:\n%s", list)
	}
	resp := f.call(t, "tracker.query", map[string]any{"format": "json"})
	var list struct {
		Stores []struct {
			ID     string `json:"id"`
			Hidden bool   `json:"hidden"`
		} `json:"stores"`
	}
	if err := json.Unmarshal([]byte(toolText(t, resp)), &list); err != nil || len(list.Stores) != 1 || !list.Stores[0].Hidden {
		t.Fatalf("the JSON store list must mark a hidden store: %v %s", err, toolText(t, resp))
	}
	f.trackerUpdate(t, `<tk:store name="me"><tk:item.create kind="task" title="still written"/></tk:store>`)
	if all := f.trackerQuery(t, map[string]any{"store": "me", "scope": "all"}); !strings.Contains(all, "still written") {
		t.Fatalf("a hidden store must keep answering reads and writes:\n%s", all)
	}
	if out := f.trackerUpdate(t, `<tk:unhide store="me"/>`); !strings.Contains(out, "unhid me:") {
		t.Fatalf("unhide answer: %s", out)
	}
	if e, _ := f.trackerEntry(t, "me"); e.Hidden {
		t.Fatalf("unhide must clear the registry flag")
	}
	if list := f.trackerQuery(t, nil); strings.Contains(list, "hidden") {
		t.Fatalf("an unhidden store must not be marked:\n%s", list)
	}
}

// The machine-local store commands name a registered store and stand alone
// in their batch.
func TestTrackerLocalStoreCommandsRefuseUnknownStoresAndCompanions(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)
	for _, commands := range []string{`<tk:unregister store="nope"/>`, `<tk:relabel store="nope" label="x"/>`, `<tk:hide store="nope"/>`, `<tk:unhide store="nope"/>`} {
		text := f.trackerRefused(t, "tracker.update", map[string]any{"commands": commands})
		if !strings.Contains(text, `store: "nope" is not a registered store (registered: me)`) {
			t.Errorf("%s must be refused naming the registered stores: %s", commands, text)
		}
	}
	text := f.trackerRefused(t, "tracker.update", map[string]any{"commands": `<tk:hide store="me"/><tk:relabel store="me" label="x"/>`})
	if !strings.Contains(text, "command 1 <tk:hide> (line 1): changes this machine's store registry, not the store, and cannot be rolled back with a batch, so it stands alone in its batch") {
		t.Fatalf("a store command must stand alone: %s", text)
	}
	if e, _ := f.trackerEntry(t, "me"); e.Hidden || e.Label != "" {
		t.Fatalf("a refused batch must change nothing: %+v", e)
	}
}
