package wsnote

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// Layer identifies which non-tracked note substrate a call targets.
type Layer string

const (
	LayerMachine  Layer = "machine"
	LayerWorktree Layer = "worktree"
	LayerRepo     Layer = "repo"
	LayerClone    Layer = "clone"
)

// lockTimeout bounds how long a note-store RMW waits to acquire its flock
// before giving up, mirroring wsconfig/resolver.go's setOverrideInFileRMW.
const lockTimeout = 2 * time.Second

// MachinePath resolves the machine-layer note store path: the sibling
// "notes.json" of wsconfig.GlobalPath's "config.json" (e.g. ~/.ws/notes.json
// by default, honoring the same opts.ConfigHome / WS_CONFIG_HOME chain).
func MachinePath(opts wsconfig.Options) (string, error) {
	configPath, err := wsconfig.GlobalPath(opts)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(configPath), "notes.json"), nil
}

// WorktreePath resolves the worktree-layer note store path under the
// existing per-worktree wsstate cache directory for the canonical root.
func WorktreePath(root string) (string, error) {
	layout, _, _, err := wsstate.NewManager(wsstate.Options{}).Ensure(root)
	if err != nil {
		return "", err
	}
	return filepath.Join(layout.WorktreeDir, "notes.json"), nil
}

// ClonePath resolves the clone-layer note store path: project-scoped and
// worktree-agnostic, under the existing per-project wsstate cache directory
// (Layout.ProjectDir) for the canonical root. Unlike WorktreePath, this is
// shared across every worktree of the same project (keyed on projectKey, not
// worktreeKey), but still lives outside the working tree, so it is never
// staged by git.
func ClonePath(root string) (string, error) {
	layout, _, _, err := wsstate.NewManager(wsstate.Options{}).Ensure(root)
	if err != nil {
		return "", err
	}
	return filepath.Join(layout.ProjectDir, "notes.json"), nil
}

// RepoDir resolves the repo-layer note store directory: the tracked
// ai-docs/ws-notes/ directory under root. Unlike MachinePath/WorktreePath,
// this cannot fail (it is a pure filepath.Join), so it returns no error.
func RepoDir(root string) string {
	return filepath.Join(root, "ai-docs", "ws-notes")
}

// Load reads the note store at path into a key->Record map. A missing file
// returns an empty, non-nil map — not an error — mirroring
// wsconfig.loadGlobalConfig's "no file yet" contract.
func Load(path string) (map[string]Record, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]Record{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read note store %s: %w", path, err)
	}
	return decodeRecords(path, raw)
}

// Write performs an flock-serialized read-modify-write on the note store at
// path, upserting each record (full overwrite, including priority) keyed by
// its Key. This is the sole mutation path for note.write. Visible is never
// taken from the caller-supplied record: an overwrite of an existing key
// preserves that key's current Visible value, and a brand new key always
// initializes Visible to true — note.write has no wire-level way to accept
// or mutate visibility (see note.mute/note.unmute/SetVisible instead).
func Write(path string, records []Record) error {
	return rmw(path, func(current map[string]Record) map[string]Record {
		for _, rec := range records {
			if existing, ok := current[rec.Key]; ok {
				rec.Visible = existing.Visible
			} else {
				rec.Visible = true
			}
			current[rec.Key] = rec
		}
		return current
	})
}

// Erase performs an flock-serialized read-modify-write on the note store at
// path, removing each listed key. A missing key is a no-op, matching
// todo.erase's precedent for erase-by-key verbs.
func Erase(path string, keys []string) error {
	return rmw(path, func(current map[string]Record) map[string]Record {
		for _, key := range keys {
			delete(current, key)
		}
		return current
	})
}

// SetVisible performs an flock-serialized read-modify-write on the note
// store at path, setting Visible to visible for each listed key already
// present in the store. This is the sole mutation path for note.mute/
// note.unmute: idempotent (setting the same value again is a harmless
// no-op), and it never touches any other field — WrittenAt in particular
// stays byte-identical across a mute/unmute call. A missing key is silently
// skipped, matching Erase's missing-key no-op precedent.
func SetVisible(path string, keys []string, visible bool) error {
	return rmw(path, func(current map[string]Record) map[string]Record {
		for _, key := range keys {
			rec, ok := current[key]
			if !ok {
				continue
			}
			rec.Visible = visible
			current[key] = rec
		}
		return current
	})
}

// rmw performs the shared flock + temp-file + atomic-rename read-modify-write
// pattern, copied from wsconfig/resolver.go's setOverrideInFileRMW. Each note
// store gets its own sibling ".lock" file — it is never shared with the
// wsconfig config lock, even though both live under the same directory for
// the machine layer.
func rmw(path string, transform func(map[string]Record) map[string]Record) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create note store dir: %w", err)
	}

	lockPath := path + ".lock"
	fl := flock.New(lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), lockTimeout)
	defer cancel()

	locked, err := fl.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil {
		return fmt.Errorf("acquire note store lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("timed out waiting for note store lock: %s", lockPath)
	}
	defer fl.Unlock() //nolint:errcheck

	current, err := readForUpdate(path)
	if err != nil {
		return err
	}
	current = transform(current)

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp note store: %w", err)
	}
	tmpName := tmp.Name()
	payload, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("encode note store: %w", err)
	}
	payload = append(payload, '\n')
	if _, werr := tmp.Write(payload); werr != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp note store: %w", werr)
	}
	if cerr := tmp.Close(); cerr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp note store: %w", cerr)
	}
	if rerr := os.Rename(tmpName, path); rerr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("atomic rename note store: %w", rerr)
	}
	return nil
}

func readForUpdate(path string) (map[string]Record, error) {
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read note store for update %s: %w", path, err)
	}
	if os.IsNotExist(err) {
		return map[string]Record{}, nil
	}
	return decodeRecords(path, raw)
}

func decodeRecords(path string, raw []byte) (map[string]Record, error) {
	var records map[string]Record
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, fmt.Errorf("parse note store %s: %w", path, err)
	}
	if records == nil {
		records = map[string]Record{}
	}
	return records, nil
}
