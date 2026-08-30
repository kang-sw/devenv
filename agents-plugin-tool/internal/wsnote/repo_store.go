package wsnote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

// repoKeyFilename encodes a note key into the tracked filename that holds
// it: hex of the raw UTF-8 key bytes, plus a ".json" suffix. Hex encoding is
// a pure function of the key — deterministic across every clone/OS/locale
// (no hashing, no timezone/case dependence) and fully collision-free
// (distinct keys never hex-encode to the same string) — so it round-trips
// exactly for RepoErase (it recomputes the identical filename from the same
// key). It also sidesteps the slash/dot-in-key hazard entirely: hex output
// only ever contains [0-9a-f], so a slash-bearing or dotted key can never
// nest a directory or collide with ".."/hidden-file handling.
func repoKeyFilename(key string) string {
	return hex.EncodeToString([]byte(key)) + ".json"
}

// RepoLoad reads every key file under dir into a key->Record map, keyed by
// each record's Key field (the file content is the source of truth, not the
// filename). A missing directory returns an empty, non-nil map — not an
// error — mirroring Load's "no file yet" contract. Non-".json" entries
// (".lock" files, "*.tmp" temp-write artifacts, and any subdirectory) are
// skipped.
func RepoLoad(dir string) (map[string]Record, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return map[string]Record{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read repo note dir %s: %w", dir, err)
	}

	records := map[string]Record{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read repo note file %s: %w", path, err)
		}
		var rec Record
		if err := json.Unmarshal(raw, &rec); err != nil {
			return nil, fmt.Errorf("parse repo note file %s: %w", path, err)
		}
		records[rec.Key] = rec
	}
	return records, nil
}

// RepoWrite writes each record to its own tracked file under dir (full
// overwrite per key, matching note.write's existing per-key contract),
// creating dir if needed. Each file's write is serialized by its own per-key
// flock via the same flock + temp-file + atomic-rename pattern rmw uses, but
// scoped to one record per file rather than rmw's whole-layer map-transform —
// each file is independently owned, which is the point of "one key = one
// file" filesystem-level conflict resolution. Unlike rmw's sibling ".lock"
// file, the per-key lock here lives under os.TempDir() (see
// repoLockPath), not beside the target file: dir is git-tracked, and a
// sibling "<hex>.json.lock" would be picked up by the caller's ordinary
// `git status`/`git add ai-docs/ws-notes/` and committed as orphaned litter,
// defeating the layer's clean-tracking contract.
func RepoWrite(dir string, records []Record) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create repo note dir: %w", err)
	}
	for _, rec := range records {
		if err := writeRepoRecordFile(dir, rec); err != nil {
			return err
		}
	}
	return nil
}

// repoLockPath derives the non-tracked flock path for the tracked note file
// at targetPath: os.TempDir() joined with a sha256 hex digest of targetPath's
// absolute form, so distinct key files (even across different repo roots)
// get distinct, deterministic, collision-free lock paths without ever
// placing a lock artifact inside the tracked ai-docs/ws-notes/ tree.
func repoLockPath(targetPath string) (string, error) {
	abs, err := filepath.Abs(targetPath)
	if err != nil {
		return "", fmt.Errorf("resolve repo note lock path for %s: %w", targetPath, err)
	}
	sum := sha256.Sum256([]byte(abs))
	return filepath.Join(os.TempDir(), "ws-notes-locks", hex.EncodeToString(sum[:])+".lock"), nil
}

// lockRepoRecordFile acquires the per-key flock for the tracked file at
// path, mirroring rmw's flock+timeout pattern but scoped to one key. The
// caller must call the returned unlock func (typically via defer) once done;
// it is nil (call it anyway — it's a no-op) only on error, which is always
// non-nil when unlock is nil.
func lockRepoRecordFile(path string) (unlock func(), err error) {
	lockPath, err := repoLockPath(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("create repo note lock dir: %w", err)
	}
	fl := flock.New(lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), lockTimeout)
	defer cancel()

	locked, err := fl.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil {
		return nil, fmt.Errorf("acquire repo note lock: %w", err)
	}
	if !locked {
		return nil, fmt.Errorf("timed out waiting for repo note lock: %s", lockPath)
	}
	return func() { fl.Unlock() }, nil //nolint:errcheck
}

// readRepoRecordFileLocked reads and decodes the record file at path. The
// caller must already hold that file's per-key flock. A missing file returns
// ok=false and a nil error (not an error condition), matching RepoLoad's
// "no file yet" contract.
func readRepoRecordFileLocked(path string) (rec Record, ok bool, err error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, fmt.Errorf("read repo note file %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, &rec); err != nil {
		return Record{}, false, fmt.Errorf("parse repo note file %s: %w", path, err)
	}
	return rec, true, nil
}

// atomicWriteRepoRecordFileLocked marshals rec and rewrites path via the
// shared temp-file + atomic-rename sequence. The caller must already hold
// that file's per-key flock.
func atomicWriteRepoRecordFileLocked(dir, path string, rec Record) error {
	payload, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("encode repo note %s: %w", rec.Key, err)
	}
	payload = append(payload, '\n')

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp repo note file: %w", err)
	}
	tmpName := tmp.Name()
	if _, werr := tmp.Write(payload); werr != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp repo note file: %w", werr)
	}
	if cerr := tmp.Close(); cerr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp repo note file: %w", cerr)
	}
	if rerr := os.Rename(tmpName, path); rerr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("atomic rename repo note file: %w", rerr)
	}
	return nil
}

// writeRepoRecordFile writes rec to its tracked per-key file under dir. Like
// Write's preserve/default logic, Visible is never taken from the
// caller-supplied rec: it reads the existing file (if any) under the same
// per-key flock this function already holds before marshaling, so the
// preserve-on-overwrite / default-true-on-new-key contract is enforced
// race-free at the same serialization point as the write itself — never as a
// separate Load-then-Write pair.
func writeRepoRecordFile(dir string, rec Record) error {
	path := filepath.Join(dir, repoKeyFilename(rec.Key))

	unlock, err := lockRepoRecordFile(path)
	if err != nil {
		return err
	}
	defer unlock()

	if existing, ok, rerr := readRepoRecordFileLocked(path); rerr != nil {
		return rerr
	} else if ok {
		rec.Visible = existing.Visible
	} else {
		rec.Visible = true
	}

	return atomicWriteRepoRecordFileLocked(dir, path, rec)
}

// RepoSetVisible sets Visible to visible for each listed key's tracked file
// under dir, under that key's own per-key flock (the same one
// writeRepoRecordFile uses) so it composes safely with concurrent
// note.write/note.mute calls on the same key. This is the sole mutation path
// for note.mute/note.unmute on the repo layer: idempotent, and it never
// touches any other field — WrittenAt in particular stays byte-identical
// across a mute/unmute call. A missing key is silently skipped, matching
// RepoErase's missing-key no-op precedent.
func RepoSetVisible(dir string, keys []string, visible bool) error {
	for _, key := range keys {
		if err := setRepoRecordVisible(dir, key, visible); err != nil {
			return err
		}
	}
	return nil
}

func setRepoRecordVisible(dir, key string, visible bool) error {
	path := filepath.Join(dir, repoKeyFilename(key))

	unlock, err := lockRepoRecordFile(path)
	if err != nil {
		return err
	}
	defer unlock()

	rec, ok, err := readRepoRecordFileLocked(path)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	rec.Visible = visible

	return atomicWriteRepoRecordFileLocked(dir, path, rec)
}

// RepoErase removes each listed key's tracked file from dir. A missing key
// is a no-op, matching Erase's contract.
func RepoErase(dir string, keys []string) error {
	for _, key := range keys {
		path := filepath.Join(dir, repoKeyFilename(key))
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove repo note file %s: %w", path, err)
		}
	}
	return nil
}
