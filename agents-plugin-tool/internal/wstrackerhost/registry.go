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

	"github.com/gofrs/flock"

	"github.com/kang-sw/devenv/internal/wsconfig"
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
}

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
// or the new file, never a partial one.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
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
		werr = os.Rename(tmp.Name(), path)
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
	return StoreEntry{}, fmt.Errorf("tracker: no registered store %q", id)
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
