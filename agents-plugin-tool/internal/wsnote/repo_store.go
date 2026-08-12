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

func writeRepoRecordFile(dir string, rec Record) error {
	path := filepath.Join(dir, repoKeyFilename(rec.Key))

	lockPath, err := repoLockPath(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return fmt.Errorf("create repo note lock dir: %w", err)
	}
	fl := flock.New(lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), lockTimeout)
	defer cancel()

	locked, err := fl.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil {
		return fmt.Errorf("acquire repo note lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("timed out waiting for repo note lock: %s", lockPath)
	}
	defer fl.Unlock() //nolint:errcheck

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
