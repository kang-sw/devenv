package wstrackerhook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// stateDirName, in the state directory, holds one state file per store.
const stateDirName = "tracker-hooks"

// window bounds catch-up and the processed-id set: a catch-up read drops
// the events older than it, and an id is forgotten once both its
// processing time and its ts are older than it.
const window = 24 * time.Hour

// storeState is one store's hook state: where the next read starts, the
// events already processed (delivered or filtered out), and the due_soon
// events fired.
type storeState struct {
	V int `json:"v"`
	// Nonce is the registration the state is for (StoreEntry.Nonce); a
	// store registered again starts over.
	Nonce  string          `json:"nonce"`
	Cursor wsledger.Cursor `json:"cursor"`
	// Processed maps a ledger event id to when it was processed and its ts.
	Processed map[string]mark `json:"processed"`
	// Due maps "<item id>:<due date>" to when its due_soon event fired.
	Due map[string]time.Time `json:"due"`
}

type mark struct {
	At time.Time `json:"at"`
	TS time.Time `json:"ts"`
}

const stateVersion = 1

func statePath(dir, store string) string {
	return filepath.Join(dir, stateDirName, store+".json")
}

// loadState reads store's state; ok is false when there is none for this
// registration (absent, unreadable, or another nonce), which starts it over.
// The due marks of an unreadable or other-version file are not kept, those
// of an earlier registration neither.
func loadState(dir, store, nonce string) (st storeState, ok bool) {
	st = storeState{V: stateVersion, Nonce: nonce, Processed: map[string]mark{}, Due: map[string]time.Time{}}
	data, err := os.ReadFile(statePath(dir, store))
	if err != nil {
		return st, false
	}
	var got storeState
	if json.Unmarshal(data, &got) != nil || got.V != stateVersion || got.Nonce != nonce {
		return st, false
	}
	if got.Processed == nil {
		got.Processed = map[string]mark{}
	}
	if got.Due == nil {
		got.Due = map[string]time.Time{}
	}
	return got, true
}

// saveState replaces store's state file atomically.
func saveState(dir, store string, st storeState) error {
	path := statePath(dir, store)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = wsstate.AtomicReplaceFile(tmp.Name(), path)
	}
	if werr != nil {
		os.Remove(tmp.Name())
	}
	return werr
}

// prune forgets the processed ids whose processing time and ts are both
// older than the window, and the due marks whose due date ended more than
// a window ago.
func (st *storeState) prune(now time.Time, loc *time.Location) {
	for id, m := range st.Processed {
		if now.Sub(m.At) > window && now.Sub(m.TS) > window {
			delete(st.Processed, id)
		}
	}
	for key := range st.Due {
		i := strings.LastIndexByte(key, ':')
		at, ok := dueTime(key[i+1:], loc)
		if i < 0 || !ok || now.Sub(at) > 2*window {
			delete(st.Due, key)
		}
	}
}
