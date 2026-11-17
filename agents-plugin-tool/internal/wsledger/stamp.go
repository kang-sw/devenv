package wsledger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// A refresh is the store's most frequent read (every Reload, Append, and
// fetch apply runs one), and its git runs (for-each-ref of the ledger and
// attachment refs, and of the own known ref after a Reload) cost a process
// spawn each. Stamps let it skip what did not move: the refs stamp keys a
// memo of those listings, and when the journal's stamp did not move either
// the whole refresh is skipped, its state being what the last one left.
//
// A stamp is the size and modification time of every file it covers,
// taken before the read it vouches for. Git and the journal replace or
// append to those files, so a change moves the size or the time, except
// two writes within one tick of the file system's clock. Like git's racy
// index check, a stamp therefore vouches only for files last written more
// than racyWindow before it was taken: a later write lands at a later time
// than any it recorded. A stamp that cannot vouch is "", which matches
// nothing, so the read runs.
//
// Taking a stamp and then reading is atomic only against writers holding
// the store lock, as every writer of a store directory's refs and journal
// does: a ref created after the stamp and deleted before the next one by a
// writer that does not (a repository shared through Options.RepoDir, which
// only tests do) could leave the state holding it until another covered
// file moves.
const racyWindow = 2 * time.Second

// stampedRefDirs are the loose-ref directories, relative to the repository,
// of every ref a refresh or a Reload lists: the ledger refs, the attachment
// refs, and the known-on-the-remote refs.
var stampedRefDirs = []string{"refs/heads/tracker", "refs/heads/attach", "refs/ledger/known"}

// refsStamp is the stamp of the refs under stampedRefDirs in the bare
// repository gitDir: its loose ref files and its packed-refs. "" when it
// cannot vouch: a covered file is younger than racyWindow at now, a
// directory could not be read, or the repository keeps its refs in a
// reftable, which this stamp does not cover.
func refsStamp(gitDir string, now time.Time) string {
	if _, err := os.Lstat(filepath.Join(gitDir, "reftable")); !errors.Is(err, fs.ErrNotExist) {
		return ""
	}
	var b strings.Builder
	settled := true
	add := func(rel string, info fs.FileInfo) {
		if info.ModTime().After(now.Add(-racyWindow)) {
			settled = false
		}
		fmt.Fprintf(&b, "%s %d %d\n", rel, info.Size(), info.ModTime().UnixNano())
	}
	switch info, err := os.Stat(filepath.Join(gitDir, "packed-refs")); {
	case err == nil:
		add("packed-refs", info)
	case !errors.Is(err, fs.ErrNotExist):
		return ""
	}
	for _, dir := range stampedRefDirs {
		root := filepath.Join(gitDir, filepath.FromSlash(dir))
		if _, err := os.Lstat(root); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(gitDir, path)
			if err != nil {
				return err
			}
			add(filepath.ToSlash(rel), info)
			return nil
		})
		if err != nil {
			return ""
		}
	}
	if !settled {
		return ""
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// fileStamp is the stamp of the one file path: "" when it cannot vouch
// (the file is younger than racyWindow at now, or cannot be read). A
// missing file has a stamp of its own.
func fileStamp(path string, now time.Time) string {
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return path + " missing"
	case err != nil || info.ModTime().After(now.Add(-racyWindow)):
		return ""
	}
	return fmt.Sprintf("%s %d %d", path, info.Size(), info.ModTime().UnixNano())
}

// listRefsAt is listRefs for a caller that took the refs stamp at before
// reading, with the store lock held since: the listing a git run gave at
// the same stamp is answered from memory. at "" always runs git, and so
// does a prefix outside stampedRefDirs, which the stamp does not cover.
// The returned map is shared and read-only.
func (l *Ledger) listRefsAt(ctx context.Context, at, prefix string) (map[string]string, error) {
	if at == "" || !stamped(prefix) {
		return l.git.listRefs(ctx, prefix)
	}
	if at != l.refListsAt {
		l.refLists, l.refListsAt = map[string]map[string]string{}, at
	}
	if refs, ok := l.refLists[prefix]; ok {
		return refs, nil
	}
	refs, err := l.git.listRefs(ctx, prefix)
	if err == nil {
		l.refLists[prefix] = refs
	}
	return refs, err
}

// stamped reports whether every ref under prefix lies in stampedRefDirs.
func stamped(prefix string) bool {
	for _, dir := range stampedRefDirs {
		if strings.HasPrefix(prefix, dir+"/") {
			return true
		}
	}
	return false
}

// RefsVersion identifies the refs the store's state was last loaded from:
// two loads that answer the same non-empty value read the same ledger,
// attachment, and known refs, so whatever a caller derived from the refs at
// the first still holds. "" when the last load could not vouch for its
// refs (one was written within the last seconds), or failed.
func (l *Ledger) RefsVersion() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.refsAt
}
