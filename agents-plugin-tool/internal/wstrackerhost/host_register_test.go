// Store registration and failure recovery tests.
package wstrackerhost

import (
	"context"
	"errors"
	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
)

// TestRegisterCreateJoinRoundTrip: one machine creates a store on an empty
// repository, another joins it with a saved Korean profile; the registry
// and saved profiles survive a new Host, and both clones see both members'
// profiles after a check.
func TestRegisterCreateJoinRoundTrip(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	a, b := w.machine("a"), w.machine("b")
	adaProfile := wstracker.Profile{Org: "Search", Family: "Park", Given: "Ada"}
	res := w.register(a, "team", "ada", &adaProfile)
	if !res.Creator || res.Profile != adaProfile {
		t.Fatalf("ada's registration = %+v, want creator with her profile", res)
	}
	if want := (StoreEntry{ID: "team", Remote: w.remote, Dir: filepath.Join(a.Root(), "team"), Member: "ada", Nonce: res.Entry.Nonce}); res.Entry != want || res.Entry.Nonce == "" {
		t.Fatalf("entry = %+v, want %+v with a registration nonce", res.Entry, want)
	}

	jon := wstracker.Profile{Org: "SW개발팀", Family: "홍", Given: "길동"}
	if err := b.SaveProfile(jon); err != nil {
		t.Fatal(err)
	}
	res = w.register(b, "alpha", "jon", nil)
	if res.Creator || res.Profile != jon {
		t.Fatalf("jon's registration = %+v, want a join with the saved profile", res)
	}

	// Registry and profiles round trip through a new Host on the same root.
	again, err := New(Options{Root: b.Root(), Clock: w.clock})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	stores, err := again.Stores()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stores, []StoreEntry{res.Entry}) {
		t.Fatalf("registry = %+v, want [%+v]", stores, res.Entry)
	}
	saved, err := again.Profiles()
	if err != nil {
		t.Fatal(err)
	}
	if last, ok := saved.LastProfile(); !ok || last != jon || len(saved.Profiles) != 1 {
		t.Fatalf("saved profiles = %+v, want jon's alone and last", saved)
	}

	if _, err := a.Check(ctx, "team"); err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, v := range []*StoreView{view(t, a, "team"), view(t, again, "alpha")} {
		if got := v.Members["jon"]; got.Profile != jon || got.Name != "홍길동" {
			t.Fatalf("%s sees jon as %+v", v.Viewer.Stream, got)
		}
		if got := v.Members["ada"]; got.Profile != adaProfile || got.Name != "Ada Park" {
			t.Fatalf("%s sees ada as %+v", v.Viewer.Stream, got)
		}
	}
}

func TestRegisterDefaultsToLastSavedProfile(t *testing.T) {
	w := newWorld(t)
	a := w.machine("a")
	if _, err := a.Register(context.Background(), RegisterRequest{ID: "team", Remote: w.remote, Member: "ada"}); err == nil {
		t.Fatal("a registration with no profile and none saved must fail")
	}
	first := wstracker.Profile{Given: "Ada"}
	edited := wstracker.Profile{Org: "Search", Given: "Ada"}
	for _, p := range []wstracker.Profile{first, edited, first} {
		if err := a.SaveProfile(p); err != nil {
			t.Fatal(err)
		}
	}
	saved, _ := a.Profiles()
	if len(saved.Profiles) != 2 || saved.Last != 0 {
		t.Fatalf("saved = %+v, want two entries (an edit is a new one) with the first used last", saved)
	}
	if err := a.SaveProfile(edited); err != nil {
		t.Fatal(err)
	}
	res := w.register(a, "team", "ada", nil)
	if res.Profile != edited || view(t, a, "team").Members["ada"].Profile != edited {
		t.Fatalf("registration profile = %+v, want the last used %+v", res.Profile, edited)
	}
}

// TestRegisterFailureLeavesNothing: invalid ids are refused before any
// write, and a join that loses its member id leaves no directory or
// registry entry, so registering again under another id works.
func TestRegisterFailureLeavesNothing(t *testing.T) {
	w := newWorld(t)
	a, b := w.machine("a"), w.machine("b")
	p := &wstracker.Profile{Given: "Ada"}
	for _, req := range []RegisterRequest{
		{ID: "keys", Remote: w.remote, Member: "ada", Profile: p},
		{ID: "Team", Remote: w.remote, Member: "ada", Profile: p},
		{ID: "team", Remote: w.remote, Member: "a.b", Profile: p},
		{ID: "team", Remote: w.remote, Member: "ada", Profile: &wstracker.Profile{Org: "x"}},
	} {
		if _, err := a.Register(context.Background(), req); err == nil {
			t.Fatalf("register %+v must fail", req)
		}
	}
	w.register(a, "team", "ada", p)
	if _, err := a.Register(context.Background(), RegisterRequest{ID: "team", Remote: w.remote, Member: "ada", Profile: p}); err == nil {
		t.Fatal("registering an id twice must fail")
	}

	_, err := b.Register(context.Background(), RegisterRequest{ID: "team", Remote: w.remote, Member: "ada", Profile: p})
	if !errors.Is(err, wsledger.ErrIDTaken) {
		t.Fatalf("a second machine joining as ada = %v, want ErrIDTaken", err)
	}
	if _, err := os.Stat(filepath.Join(b.Root(), "team")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the failed registration left its directory: %v", err)
	}
	if b.HasStores() {
		t.Fatal("the failed registration left a registry entry")
	}
	w.register(b, "team", "bo", &wstracker.Profile{Given: "Bo"})
}

// corruptAfterPush is a runner that breaks the registry file once a push
// has succeeded, so a registration fails after it reached the remote.
type corruptAfterPush struct{ registry string }

func (r corruptAfterPush) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	out, err := batchRunner{}.RunGit(ctx, root, args...)
	if err == nil && slices.Contains(args, "push") {
		if werr := os.WriteFile(r.registry, []byte("not json"), 0o600); werr != nil {
			return nil, werr
		}
	}
	return out, err
}

// TestRegisterFailureAfterPushSaysTheRemoteHoldsIt: a registration that
// fails after its genesis or join reached the remote leaves nothing on this
// machine, says the remote holds what it pushed, and registering again with
// the same member id finishes it without a new claim.
func TestRegisterFailureAfterPushSaysTheRemoteHoldsIt(t *testing.T) {
	w := newWorld(t)
	p := &wstracker.Profile{Given: "Ada"}
	for _, creator := range []bool{true, false} {
		name, member := "creator", "ada"
		if !creator {
			name, member = "joiner", "jon"
		}
		root := filepath.Join(w.dir, name)
		registry := filepath.Join(root, registryFile)
		h, err := New(Options{Root: root, Clock: w.clock, Runner: corruptAfterPush{registry: registry}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = h.Register(context.Background(), RegisterRequest{ID: "team", Remote: w.remote, Member: member, Profile: p})
		h.Close()
		var held *RemoteHeldError
		if !errors.As(err, &held) || held.Creator != creator {
			t.Fatalf("%s: a failure after the push = %v, want a RemoteHeldError with Creator %v", name, err, creator)
		}
		if _, err := os.Stat(filepath.Join(root, "team")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s: the failed registration left its directory: %v", name, err)
		}
		claim := w.remoteTip("refs/heads/tracker/id/" + member)
		if claim == "" {
			t.Fatalf("%s: the remote must hold the member's claim", name)
		}
		if err := os.Remove(registry); err != nil {
			t.Fatal(err)
		}
		again := w.machine(name)
		res := w.register(again, "team", member, p)
		if res.Creator != creator {
			t.Fatalf("%s: re-registering = %+v, want Creator %v", name, res, creator)
		}
		if got := w.remoteTip("refs/heads/tracker/id/" + member); got != claim {
			t.Fatalf("%s: re-registering must write no new claim: %s, was %s", name, got, claim)
		}
		if got := view(t, again, "team").Members[member].Profile; got != *p {
			t.Fatalf("%s: re-registering must finish the profile: %+v", name, got)
		}
	}
}

// failFetchAfterPush is a runner whose first fetch after a successful push
// fails, so wsledger.Register itself fails once the remote holds its push.
type failFetchAfterPush struct {
	mu            sync.Mutex
	pushed, fired bool
}

func (r *failFetchAfterPush) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	r.mu.Lock()
	fail := r.pushed && !r.fired && slices.Contains(args, "fetch")
	r.fired = r.fired || fail
	r.mu.Unlock()
	if fail {
		return nil, errors.New("fetch broke")
	}
	out, err := batchRunner{}.RunGit(ctx, root, args...)
	if err == nil && slices.Contains(args, "push") {
		r.mu.Lock()
		r.pushed = true
		r.mu.Unlock()
	}
	return out, err
}

// TestRegisterFailureInsideTheLedgerAfterPush: a failure inside the ledger's
// own registration, after its push landed, says the remote holds the push
// too, naming the new store for its creator.
func TestRegisterFailureInsideTheLedgerAfterPush(t *testing.T) {
	w := newWorld(t)
	p := &wstracker.Profile{Given: "Ada"}
	for _, creator := range []bool{true, false} {
		name, member := "creator2", "ada"
		if !creator {
			name, member = "joiner2", "jon"
		}
		h, err := New(Options{Root: filepath.Join(w.dir, name), Clock: w.clock, Runner: &failFetchAfterPush{}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = h.Register(context.Background(), RegisterRequest{ID: "team", Remote: w.remote, Member: member, Profile: p})
		h.Close()
		var held *RemoteHeldError
		if !errors.As(err, &held) || held.Creator != creator || !strings.Contains(err.Error(), "fetch broke") {
			t.Fatalf("%s: a ledger failure after the push = %v, want a RemoteHeldError with Creator %v", name, err, creator)
		}
		if w.remoteTip("refs/heads/tracker/id/"+member) == "" {
			t.Fatalf("%s: the remote must hold the member's claim", name)
		}
	}
}
