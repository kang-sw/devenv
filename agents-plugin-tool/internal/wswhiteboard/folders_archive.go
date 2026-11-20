package wswhiteboard

// Board archives: the dashboard's zip download of a board folder and the
// whiteboard.load replace that installs one into another session's folder.
// A zip holds the folder's files at its root, as stored; .lock, pending.json,
// and dot-named temp files never travel.

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// Zip-bomb guard for a loaded archive. The largest real board measured held
// about 420 files and 9 MB; these caps stay an order of magnitude or more
// above it, while bounding what the daemon holds in memory for one load.
// Variables only so tests can lower them.
var (
	archiveMaxEntries       = 50000
	archiveMaxBytes   int64 = 128 << 20
)

// present reports whether the folder holds any root authority file, without
// taking the lock or creating anything.
func (f FolderStore) present() (bool, error) {
	for _, name := range []string{"meta.json", "pending.json", "committed.json"} {
		_, err := wsstate.Stat(filepath.Join(f.Dir, name))
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}
	return false, nil
}

// ExportArchive zips the board folder under its lock after recovery, so a
// concurrent update cannot tear it. A folder without a board is
// os.ErrNotExist; nothing is created for it.
func (f FolderStore) ExportArchive() ([]byte, error) {
	if ok, err := f.present(); err != nil || !ok {
		if err == nil {
			err = os.ErrNotExist
		}
		return nil, err
	}
	var buf bytes.Buffer
	err := f.withLock(false, func() error {
		state, meta, err := f.load(nil)
		if err != nil {
			return err
		}
		if state == nil {
			return os.ErrNotExist
		}
		files, err := f.boardFiles(meta)
		if err != nil {
			return err
		}
		zw := zip.NewWriter(&buf)
		for _, rel := range files {
			data, err := wsstate.ReadFile(filepath.Join(f.Dir, filepath.FromSlash(rel)))
			if err != nil {
				return err
			}
			w, err := zw.Create(rel)
			if err != nil {
				return err
			}
			if _, err := w.Write(data); err != nil {
				return err
			}
		}
		return zw.Close()
	})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// boardFiles lists the board's regular files as slash paths: the root files
// and the topic folders meta references, plus the revision receipts. It
// leaves out the lock file, a pending intent, dot-named temp files, and
// unreferenced topic folders (a replaced board's, when their removal failed),
// so they never travel or get backed up. The caller holds the lock: on
// Windows the held .lock cannot be read through a second handle.
func (f FolderStore) boardFiles(meta *folderSession) ([]string, error) {
	held := map[string]bool{folderRevisionDir: true}
	for _, ref := range meta.Topics {
		held[ref.Dir] = true
	}
	var out []string
	err := filepath.WalkDir(f.Dir, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(f.Dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if top, _, nested := strings.Cut(rel, "/"); entry.IsDir() && rel != "." && !nested && !held[top] {
			return fs.SkipDir
		}
		if entry.IsDir() || !entry.Type().IsRegular() || strings.HasPrefix(entry.Name(), ".") || rel == "pending.json" {
			return nil
		}
		out = append(out, rel)
		return nil
	})
	return out, err
}

// LoadArchive replaces this board with the board in the zip at path. The zip
// is extracted to a temporary folder and opened with the store (schema guard,
// pending replay, consistency checks) before the destination is touched, so
// a refused archive leaves the destination as it was and creates no backup.
// Every value, the ID counters included, stays as loaded, except the revision
// receipts (metadata and stale-nudge revisions), which
// the destination's receipt floors because viewers compare them, the render
// sequence, which replace raises past the destination's, and LoadedAt, set
// at the locked swap (not the backup-name time now), so answers sent to the
// replaced board attach to none of the loaded board's reused item IDs.
// backup is the folder now holding the previous board ("" when there was
// none), named <dir>.bak-<YYMMDD_hhmm> in now's time zone, with -2, -3, ... on
// a collision.
func (f FolderStore) LoadArchive(path string, now time.Time) (backup string, topics int, err error) {
	tmp, err := os.MkdirTemp("", "ws-whiteboard-load-*")
	if err != nil {
		return "", 0, err
	}
	defer os.RemoveAll(tmp)
	// Opening the extraction memoizes its topic logs in the process-wide
	// memo; removing the folder does not free them.
	defer forgetFolderMemo(tmp)
	defer boardIncarnations.Delete(tmp)
	if err := extractArchive(path, tmp); err != nil {
		return "", 0, err
	}
	board, err := FolderStore{Dir: tmp}.Open(nil)
	if err != nil {
		return "", 0, fmt.Errorf("the archive is not a valid board: %w", err)
	}
	if board == nil {
		return "", 0, fmt.Errorf("the archive holds no board")
	}
	return f.replace(board, f.Dir+".bak-"+now.Format("060102_1504"))
}

// extractArchive writes the zip's files under dir, refusing an entry that
// would land outside it, an archive past the caps, and one without the two
// root authority files.
func extractArchive(zipPath, dir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	if len(r.File) > archiveMaxEntries {
		return fmt.Errorf("the archive has %d entries, over the %d limit", len(r.File), archiveMaxEntries)
	}
	var total int64
	seen := map[string]bool{}
	for _, zf := range r.File {
		name := strings.TrimSuffix(zf.Name, "/")
		if name == "" || strings.ContainsAny(name, `\:`) || path.IsAbs(name) || path.Clean(name) != name || !filepath.IsLocal(filepath.FromSlash(name)) {
			return fmt.Errorf("the archive entry %q is not a relative path inside the board folder", zf.Name)
		}
		if zf.FileInfo().IsDir() || name == ".lock" {
			continue
		}
		if !zf.Mode().IsRegular() {
			return fmt.Errorf("the archive entry %q is not a regular file", zf.Name)
		}
		if err := extractEntry(zf, filepath.Join(dir, filepath.FromSlash(name)), archiveMaxBytes-total, &total); err != nil {
			return err
		}
		seen[name] = true
	}
	if !seen["meta.json"] || !seen["committed.json"] {
		return fmt.Errorf("the archive is not a board: meta.json and committed.json must be at its root")
	}
	return nil
}

// extractEntry copies one entry, counting the bytes actually inflated rather
// than the header's declared size. O_EXCL refuses a duplicate name.
func extractEntry(zf *zip.File, target string, budget int64, total *int64) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	in, err := zf.Open()
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := wsstate.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("the archive entry %q: %w", zf.Name, err)
	}
	n, err := io.Copy(out, io.LimitReader(in, budget+1))
	*total += n
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("the archive entry %q: %w", zf.Name, err)
	}
	if n > budget {
		return fmt.Errorf("the archive inflates past the %d-byte limit", archiveMaxBytes)
	}
	return nil
}

// replace installs next as this folder's whole board under one lock: it
// copies the current board to a backup folder, commits a fresh-board intent
// through the ordinary intent protocol, then removes the old topic folders.
// The intent is planned against an empty prior board, so it deliberately
// bypasses the counter and history monotonicity checks of an update: the
// loaded counters may be lower than the ones they replace. Its topic folders
// are numbered after the old NextTopic and the new NextTopic counts them, so
// a topic minted later never reuses an installed folder or an old one whose
// removal failed. The render sequence is raised past the current board's:
// the published change stamp derives from it, and every stamp published so
// far is at most the committed sequence, so an open page sees a new stamp
// and reloads even when the loaded board's sequence is lower.
func (f FolderStore) replace(next *State, backupBase string) (backup string, topics int, err error) {
	finish := f.beginIncarnationLoad()
	var token string
	defer func() { finish(token, err) }()
	err = f.locked(func() error {
		prior, meta, err := f.load(nil)
		if err != nil {
			return err
		}
		if prior != nil {
			next.Seq = max(next.Seq, prior.Seq+1)
		}
		var intent *folderIntent
		if meta == nil {
			intent, err = f.plan(nil, nil, next)
		} else {
			// Only the generation and topic allocation carry over; replay
			// requires the generation to follow the committed one.
			intent, err = f.plan(&folderSession{Generation: meta.Generation, NextTopic: meta.NextTopic}, &State{}, next)
		}
		if err != nil {
			return err
		}
		if meta != nil {
			if backup, err = f.backupTo(backupBase, meta); err != nil {
				return err
			}
		}
		// A leftover folder at an allocated number (never referenced by a
		// committed board) would fail the intent's append offsets.
		for _, ref := range intent.Session.Topics {
			if err := os.RemoveAll(filepath.Join(f.Dir, ref.Dir)); err != nil {
				return err
			}
		}
		if err := f.planLoadFloor(intent); err != nil {
			return err
		}
		if err := f.commit(intent); err != nil {
			return err
		}
		token = intent.Session.State.LoadedAt
		topics = len(intent.Session.Topics)
		if meta != nil {
			// Best effort: an unreferenced folder a reader still holds open
			// on Windows stays behind harmlessly.
			for _, ref := range meta.Topics {
				_ = os.RemoveAll(filepath.Join(f.Dir, ref.Dir))
			}
		}
		return nil
	})
	return backup, topics, err
}

// backupTo copies the board's files (boardFiles' set) to a new folder named
// base, or base-2, base-3, ... when taken. An existing backup is never
// overwritten. Called under the board lock.
func (f FolderStore) backupTo(base string, meta *folderSession) (string, error) {
	files, err := f.boardFiles(meta)
	if err != nil {
		return "", err
	}
	for n := 1; ; n++ {
		dir := base
		if n > 1 {
			dir = fmt.Sprintf("%s-%d", base, n)
		}
		if err := os.Mkdir(dir, 0o755); errors.Is(err, fs.ErrExist) {
			continue
		} else if err != nil {
			return "", err
		}
		for _, rel := range files {
			if err := copyBoardFile(filepath.Join(f.Dir, filepath.FromSlash(rel)), filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
				_ = os.RemoveAll(dir)
				return "", fmt.Errorf("back up the board: %w", err)
			}
		}
		return dir, nil
	}
}

func copyBoardFile(from, to string) error {
	data, err := wsstate.ReadFile(from)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	return os.WriteFile(to, data, 0o644)
}
