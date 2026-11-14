package wstrackerhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gofrs/flock"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wstracker"
)

const (
	registryFile = "stores.json"
	profilesFile = "profiles.json"
	keysDir      = "keys"
	rootLockFile = ".lock"
	// lockWait bounds a registry or profile read-modify-write's wait for the
	// root lock.
	lockWait = 5 * time.Second
)

// Root is the tracker root for opts: the "tracker" directory beside
// wsconfig.GlobalPath's config file (opts.ConfigHome, then
// $WS_CONFIG_HOME, then ~/.ws).
func Root(opts wsconfig.Options) (string, error) {
	config, err := wsconfig.GlobalPath(opts)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(config), "tracker"), nil
}

// StoreEntry is one registered store.
type StoreEntry struct {
	// ID names the store on this machine: the registry key and its
	// directory name under the root.
	ID string `json:"id"`
	// Label is the display label; empty shows the id.
	Label string `json:"label,omitempty"`
	// Remote is the repository URL; empty for a store with no remote.
	Remote string `json:"remote,omitempty"`
	// Dir is the store's wsledger directory.
	Dir string `json:"dir"`
	// Member is this machine's member id in the store.
	Member string `json:"member"`
	// Hidden: the user hid the store on this machine (SetHidden). A viewer
	// leaves it out of its store list; reads, writes, the poll, and the
	// watch treat it as any other store.
	Hidden bool `json:"hidden,omitempty"`
	// Nonce identifies this registration: Register writes a fresh one, so
	// an id unregistered and registered again differs from the entry a
	// process still holds open. Empty for an entry written before the field
	// existed.
	Nonce string `json:"nonce,omitempty"`
}

// ErrNotRegistered: the registry holds no store under the id.
var ErrNotRegistered = errors.New("tracker: no registered store")

// ErrBadLabel: Relabel refused the label.
var ErrBadLabel = errors.New("tracker: bad store label")

// MaxLabelLen is the longest display label, in characters.
const MaxLabelLen = 80

// DisplayLabel is the label, or the id when there is none.
func (e StoreEntry) DisplayLabel() string {
	if e.Label != "" {
		return e.Label
	}
	return e.ID
}

type registryDoc struct {
	Version int          `json:"version"`
	Stores  []StoreEntry `json:"stores"`
}

// SavedProfiles is the machine-local saved-profile list the join step
// defaults from. It never enters a ledger.
type SavedProfiles struct {
	Version int `json:"version"`
	// Last indexes the profile used last; -1 when none was.
	Last     int                 `json:"last"`
	Profiles []wstracker.Profile `json:"profiles"`
}

// LastProfile is the profile used last.
func (p SavedProfiles) LastProfile() (wstracker.Profile, bool) {
	if p.Last < 0 || p.Last >= len(p.Profiles) {
		return wstracker.Profile{}, false
	}
	return p.Profiles[p.Last], true
}

// readJSON decodes path into v; a missing file leaves v as it is.
func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("tracker: %s: %w", path, err)
	}
	return nil
}

// writeJSON replaces path atomically, so a concurrent reader sees the old
// or the new file, never a partial one. It never creates path's directory:
// a late write into a store directory Unregister removed fails instead of
// leaving a stub that Register would then refuse as "already exists". The
// root's writers create the root themselves (withRootLock).
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(append(data, '\n'))
	cerr := tmp.Close()
	if werr == nil {
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

// withRootLock serializes registry and profile read-modify-writes across
// processes.
func (h *Host) withRootLock(fn func() error) error {
	if err := os.MkdirAll(h.root, 0o700); err != nil {
		return err
	}
	fl := flock.New(filepath.Join(h.root, rootLockFile))
	ctx, cancel := context.WithTimeout(context.Background(), lockWait)
	defer cancel()
	locked, err := fl.TryLockContext(ctx, 20*time.Millisecond)
	if err != nil || !locked {
		return fmt.Errorf("tracker: root lock %s not acquired: %v", fl.Path(), err)
	}
	defer fl.Unlock() //nolint:errcheck
	return fn()
}

// Stores lists the registered stores in registration order.
func (h *Host) Stores() ([]StoreEntry, error) {
	var doc registryDoc
	if err := readJSON(filepath.Join(h.root, registryFile), &doc); err != nil {
		return nil, err
	}
	return doc.Stores, nil
}

// Entry returns the registered store id.
func (h *Host) Entry(id string) (StoreEntry, error) {
	stores, err := h.Stores()
	if err != nil {
		return StoreEntry{}, err
	}
	for _, e := range stores {
		if e.ID == id {
			return e, nil
		}
	}
	return StoreEntry{}, fmt.Errorf("%w %q", ErrNotRegistered, id)
}

// editEntry rewrites the registry entry id through edit under the root
// lock and returns the entry as written; ErrNotRegistered when there is
// none.
func (h *Host) editEntry(id string, edit func(*StoreEntry) error) (StoreEntry, error) {
	var out StoreEntry
	err := h.withRootLock(func() error {
		path := filepath.Join(h.root, registryFile)
		doc := registryDoc{Version: 1}
		if err := readJSON(path, &doc); err != nil {
			return err
		}
		for i := range doc.Stores {
			if doc.Stores[i].ID != id {
				continue
			}
			if err := edit(&doc.Stores[i]); err != nil {
				return err
			}
			out = doc.Stores[i]
			doc.Version = 1
			return writeJSON(path, doc)
		}
		return fmt.Errorf("%w %q", ErrNotRegistered, id)
	})
	return out, err
}

// Relabel sets store id's display label on this machine and returns the
// entry as written. The label is machine-local: nothing is written to the
// store. It is trimmed; an empty one clears the label back to the id, and
// one holding a control character or longer than MaxLabelLen characters is
// refused (ErrBadLabel).
func (h *Host) Relabel(id, label string) (StoreEntry, error) {
	label = strings.TrimSpace(label)
	if utf8.RuneCountInString(label) > MaxLabelLen {
		return StoreEntry{}, fmt.Errorf("%w: longer than %d characters", ErrBadLabel, MaxLabelLen)
	}
	if !utf8.ValidString(label) || strings.ContainsFunc(label, unicode.IsControl) {
		return StoreEntry{}, fmt.Errorf("%w: it holds a control character", ErrBadLabel)
	}
	return h.editEntry(id, func(e *StoreEntry) error {
		e.Label = label
		return nil
	})
}

// SetHidden hides or shows store id on this machine (StoreEntry.Hidden) and
// returns the entry as written. Nothing is deleted and nothing is written
// to the store.
func (h *Host) SetHidden(id string, hidden bool) (StoreEntry, error) {
	return h.editEntry(id, func(e *StoreEntry) error {
		e.Hidden = hidden
		return nil
	})
}

// removeEntry takes entry id out of the registry when it is still the
// registration nonce names, and returns its position.
func (h *Host) removeEntry(id, nonce string) (int, error) {
	at := -1
	err := h.withRootLock(func() error {
		path := filepath.Join(h.root, registryFile)
		doc := registryDoc{Version: 1}
		if err := readJSON(path, &doc); err != nil {
			return err
		}
		for i, e := range doc.Stores {
			if e.ID == id && e.Nonce == nonce {
				at = i
				doc.Stores = append(doc.Stores[:i], doc.Stores[i+1:]...)
				doc.Version = 1
				return writeJSON(path, doc)
			}
		}
		return fmt.Errorf("%w %q", ErrNotRegistered, id)
	})
	return at, err
}

// restoreEntry puts e back at position at (or the end), unless the id was
// registered again meanwhile.
func (h *Host) restoreEntry(e StoreEntry, at int) error {
	return h.withRootLock(func() error {
		path := filepath.Join(h.root, registryFile)
		doc := registryDoc{Version: 1}
		if err := readJSON(path, &doc); err != nil {
			return err
		}
		for _, have := range doc.Stores {
			if have.ID == e.ID {
				return nil
			}
		}
		if at < 0 || at > len(doc.Stores) {
			at = len(doc.Stores)
		}
		doc.Stores = append(doc.Stores[:at], append([]StoreEntry{e}, doc.Stores[at:]...)...)
		doc.Version = 1
		return writeJSON(path, doc)
	})
}

// addEntry appends e to the registry; an id already there is refused.
func (h *Host) addEntry(e StoreEntry) error {
	return h.withRootLock(func() error {
		path := filepath.Join(h.root, registryFile)
		doc := registryDoc{Version: 1}
		if err := readJSON(path, &doc); err != nil {
			return err
		}
		for _, have := range doc.Stores {
			if have.ID == e.ID {
				return fmt.Errorf("tracker: store %q is already registered", e.ID)
			}
		}
		doc.Version = 1
		doc.Stores = append(doc.Stores, e)
		return writeJSON(path, doc)
	})
}

// Profiles reads the saved-profile list.
func (h *Host) Profiles() (SavedProfiles, error) {
	p := SavedProfiles{Version: 1, Last: -1}
	if err := readJSON(filepath.Join(h.root, profilesFile), &p); err != nil {
		return SavedProfiles{}, err
	}
	return p, nil
}

// SaveProfile records p as the profile used last: an equal saved entry is
// reused, anything else (an edit included) becomes a new entry.
func (h *Host) SaveProfile(p wstracker.Profile) error {
	p, err := cleanProfile(p)
	if err != nil {
		return err
	}
	return h.withRootLock(func() error {
		saved, err := h.Profiles()
		if err != nil {
			return err
		}
		saved.Version = 1
		saved.Last = -1
		for i, have := range saved.Profiles {
			if have == p {
				saved.Last = i
			}
		}
		if saved.Last < 0 {
			saved.Profiles = append(saved.Profiles, p)
			saved.Last = len(saved.Profiles) - 1
		}
		return writeJSON(filepath.Join(h.root, profilesFile), saved)
	})
}

// cleanProfile trims every field; given is required.
func cleanProfile(p wstracker.Profile) (wstracker.Profile, error) {
	p = wstracker.Profile{Org: strings.TrimSpace(p.Org), Family: strings.TrimSpace(p.Family), Given: strings.TrimSpace(p.Given)}
	if p.Given == "" {
		return p, errors.New("tracker: a profile needs a given name")
	}
	return p, nil
}
