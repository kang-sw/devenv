// Store registration and member profile initialization.
package wstrackerhost

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RegisterRequest registers a store on this machine: creating it (an empty
// repository, or no remote at all) or joining it (a repository holding a
// store).
type RegisterRequest struct {
	// ID names the store on this machine; a valid wsledger id other than
	// "keys".
	ID string
	// Label is the optional display label.
	Label string
	// Remote is the repository URL; empty creates a store with no remote.
	Remote string
	// Member is this machine's member id in the store.
	Member string
	// Profile is the member's profile; nil uses the saved profile used
	// last.
	Profile *wstracker.Profile
	// Origin is the origin of the profile write ("human", "agent"); empty
	// is human.
	Origin string
}

// RemoteHeldError is a Register failure that came after the store's
// genesis or this member's join reached the remote: nothing is registered
// on this machine, but the remote keeps what was pushed. Registering again
// with the same member id finishes the registration; this machine's key is
// then already a member, so it writes no new join.
type RemoteHeldError struct {
	Err error
	// Creator: what the remote holds is the new store, with this member as
	// its sole admin; otherwise it is this member's join.
	Creator bool
}

func (e *RemoteHeldError) Error() string { return e.Err.Error() }
func (e *RemoteHeldError) Unwrap() error { return e.Err }

// RegisterResult reports a registration.
type RegisterResult struct {
	Entry StoreEntry
	// Creator: the store's genesis lists this member, so this registration
	// (or an earlier one with the same key) created it.
	Creator bool
	// Profile is the profile the member now has in the store.
	Profile wstracker.Profile
}

// Register creates or joins a store, writes the member's profile, records
// the store in the registry, and saves the profile as the one used last.
// It keeps the store open. A store that cannot be registered leaves no
// directory or registry entry behind; wsledger.ErrIDTaken means another
// join holds the member id, and a *RemoteHeldError means the remote already
// holds what this registration pushed.
func (h *Host) Register(ctx context.Context, req RegisterRequest) (RegisterResult, error) {
	var res RegisterResult
	if !wsledger.ValidID(req.ID) || req.ID == keysDir {
		return res, fmt.Errorf("tracker: store id %q is not valid (lowercase letters, digits, '_' and '-'; not %q)", req.ID, keysDir)
	}
	if !wsledger.ValidID(req.Member) {
		return res, fmt.Errorf("tracker: member id %q is not valid (lowercase letters, digits, '_' and '-')", req.Member)
	}
	var profile wstracker.Profile
	if req.Profile != nil {
		profile = *req.Profile
	} else {
		saved, err := h.Profiles()
		if err != nil {
			return res, err
		}
		last, ok := saved.LastProfile()
		if !ok {
			return res, errors.New("tracker: no profile given and none saved")
		}
		profile = last
	}
	profile, err := cleanProfile(profile)
	if err != nil {
		return res, err
	}
	if _, err := h.Entry(req.ID); err == nil {
		return res, fmt.Errorf("tracker: store %q is already registered", req.ID)
	}
	// A handle this process still caches for an earlier registration of the
	// id, which another process unregistered, is retired first.
	if stores, err := h.Stores(); err == nil {
		h.retireStale(stores)
	}
	entry := StoreEntry{ID: req.ID, Label: req.Label, Remote: req.Remote, Dir: filepath.Join(h.root, req.ID), Member: req.Member, Nonce: newNonce()}
	if err := os.MkdirAll(h.root, 0o700); err != nil {
		return res, err
	}
	// Creating the directory claims the id against a concurrent registration.
	if err := os.Mkdir(entry.Dir, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return res, fmt.Errorf("tracker: %s already exists; remove it or register under another id", entry.Dir)
		}
		return res, err
	}
	// The store enters this process's map, locked, before anything is
	// written, so a concurrent check that finds the new registry entry waits
	// for this registration instead of opening a second ledger on the
	// directory. The network mutex is taken first, as everywhere, and held
	// through the sync that publishes the profile.
	st := &store{id: entry.ID, nonce: entry.Nonce, entry: entry}
	st.net.Lock()
	defer st.net.Unlock()
	st.mu.Lock()
	unlock := sync.OnceFunc(st.mu.Unlock)
	defer unlock()
	h.mu.Lock()
	if h.closed || h.stores[entry.ID] != nil {
		closed := h.closed
		h.mu.Unlock()
		os.Remove(entry.Dir)
		if closed {
			return res, errors.New("tracker: host closed")
		}
		return res, fmt.Errorf("tracker: store %q is already registered", req.ID)
	}
	h.stores[entry.ID] = st
	h.mu.Unlock()
	pushed := false
	err = func() error {
		opts := h.ledgerOptions(st, profile.Given)
		var l *wsledger.Ledger
		var err error
		if entry.Remote == "" {
			l, err = wstracker.Create(ctx, opts)
		} else {
			l, err = wstracker.Register(ctx, opts, entry.Remote)
		}
		if err != nil {
			// A failure after the push still leaves the remote holding it.
			var held *wsledger.PushedError
			if errors.As(err, &held) {
				pushed = true
				res.Creator = genesisLists(held.Genesis, req.Member)
			}
			return err
		}
		// wsledger.Register has pushed the genesis or the join by now.
		pushed = entry.Remote != ""
		st.ledger, st.tracker = l, wstracker.New(l, l.MemberID()+"."+l.CloneID())
		res.Creator = genesisLists(l.Identity().Genesis, req.Member)
		if res.Profile, err = writeProfile(ctx, st, profile, req.Origin); err != nil {
			return err
		}
		return h.addEntry(entry)
	}()
	if err != nil {
		if st.ledger != nil {
			st.ledger.Close()
			st.ledger, st.tracker = nil, nil
		}
		os.RemoveAll(entry.Dir)
		h.mu.Lock()
		if h.stores[entry.ID] == st {
			delete(h.stores, entry.ID)
		}
		h.mu.Unlock()
		if pushed {
			return RegisterResult{}, &RemoteHeldError{Err: err, Creator: res.Creator}
		}
		return RegisterResult{}, err
	}
	l := st.ledger
	// The registration is complete: its lock is released before the sync,
	// whose local steps take it (applyGate).
	unlock()
	if entry.Remote != "" {
		// Publish the profile now; a failure leaves it to the idle push.
		_ = h.attempt(ctx, st, networkTimeout, l.Sync)
	}
	res.Entry = entry
	_ = h.SaveProfile(profile) // the store is registered; a lost profile save costs only the default
	return res, nil
}

// newNonce returns a fresh registration nonce (StoreEntry.Nonce).
func newNonce() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// writeProfile writes a member.profile carrying the fields of want that
// differ from what the store holds. wsledger writes only a name (read as
// the given name) into a genesis member or a join, so org and family always
// arrive this way.
func writeProfile(ctx context.Context, st *store, want wstracker.Profile, origin string) (wstracker.Profile, error) {
	member := st.entry.Member
	have := st.tracker.View(time.Time{}, 0).Members[member].Profile
	changed := map[string]string{}
	if want.Org != have.Org {
		changed["org"] = want.Org
	}
	if want.Family != have.Family {
		changed["family"] = want.Family
	}
	if want.Given != have.Given {
		changed["given"] = want.Given
	}
	if len(changed) == 0 {
		return have, nil
	}
	payload, err := json.Marshal(wstracker.MemberProfile{Profile: changed})
	if err != nil {
		return have, err
	}
	if _, err := st.tracker.Dispatch(ctx, []wsledger.NewEvent{{Type: wstracker.TypeMemberProfile, Origin: origin, Payload: payload}}); err != nil {
		return have, fmt.Errorf("tracker: write the profile: %w", err)
	}
	return want, nil
}

// genesisLists reports whether the genesis roles.json lists member.
func genesisLists(genesis json.RawMessage, member string) bool {
	var roles wsledger.Roles
	if json.Unmarshal(genesis, &roles) != nil {
		return false
	}
	for _, m := range roles.Members {
		if m.ID == member {
			return true
		}
	}
	return false
}
