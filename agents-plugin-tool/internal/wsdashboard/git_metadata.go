package wsdashboard

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const gitSafetyInterval = 5 * time.Minute

// A suppression hint, never admission authority. The owning reader serializes
// it; failed probes/reads are not cached. Metadata cannot see every worktree
// edit, so successful complete reads have an independent hard safety deadline.
type gitReadGate struct {
	stamp string
	at    time.Time
}

func (g gitReadGate) needed(stamp string, now time.Time, force bool) bool {
	return force || stamp == "" || g.at.IsZero() || stamp != g.stamp || now.Before(g.at) || now.Sub(g.at) >= gitSafetyInterval
}

func (g *gitReadGate) complete(stamp string, now time.Time, success bool) {
	if !success || stamp == "" {
		*g = gitReadGate{}
		return
	}
	g.stamp, g.at = stamp, now
}

func (g gitReadGate) delay(now time.Time, interval time.Duration) time.Duration {
	if !g.at.IsZero() {
		return min(interval, max(time.Nanosecond, g.at.Add(gitSafetyInterval).Sub(now)))
	}
	return interval
}

func writeGitMetadata(w io.Writer, path string, info fs.FileInfo) {
	var size, modified int64
	// Directory size is undefined in Windows enumeration, and directory times
	// can settle independently of file contents. WalkDir fingerprints child
	// names and file metadata, so neither directory field adds a useful hint.
	if !info.IsDir() {
		size = info.Size()
		modified = info.ModTime().UnixNano()
	}
	fmt.Fprintf(w, "%q:%d:%d:%d\n", path, info.Mode(), size, modified)
}

// gitMetadataStamp reads only Git administration metadata, never worktree
// contents or objects. The old status/list commands read these and much more;
// a stable hint suppresses their process launches until the safety deadline.
// Unsupported layouts, symlinks and errors fail open to the existing reader.
func gitMetadataStamp(root string) (string, error) {
	for _, name := range []string{"GIT_DIR", "GIT_COMMON_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE"} {
		if os.Getenv(name) != "" {
			return "", fmt.Errorf("custom %s", name)
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("git root is not a directory")
	}
	h := sha256.New()
	metadata := func(path string, info fs.FileInfo) { writeGitMetadata(h, path, info) }
	var admin string
	for dir := root; ; dir = filepath.Dir(dir) {
		path := filepath.Join(dir, ".git")
		info, err := os.Lstat(path)
		if err == nil {
			metadata(path, info)
			switch {
			case info.IsDir():
				admin = path
			case info.Mode().IsRegular():
				raw, err := os.ReadFile(path)
				if err != nil {
					return "", err
				}
				text := strings.TrimSpace(string(raw))
				if !strings.HasPrefix(text, "gitdir:") {
					return "", fmt.Errorf("invalid git pointer")
				}
				admin = strings.TrimSpace(strings.TrimPrefix(text, "gitdir:"))
				if admin == "" {
					return "", fmt.Errorf("empty git pointer")
				}
				fmt.Fprintf(h, "pointer:%q\n", raw)
				if !filepath.IsAbs(admin) {
					admin = filepath.Join(dir, admin)
				}
			default:
				return "", fmt.Errorf("unsupported git pointer")
			}
			break
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		if filepath.Dir(dir) == dir {
			return "", fmt.Errorf("no git directory")
		}
	}
	admin = filepath.Clean(admin)
	common := admin
	if raw, err := os.ReadFile(filepath.Join(admin, "commondir")); err == nil {
		common = strings.TrimSpace(string(raw))
		if common == "" {
			return "", fmt.Errorf("empty common directory")
		}
		fmt.Fprintf(h, "commondir:%q\n", raw)
		if !filepath.IsAbs(common) {
			common = filepath.Join(admin, common)
		}
		common = filepath.Clean(common)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	for _, dir := range []string{admin, common} {
		info, err := os.Stat(dir)
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", fmt.Errorf("invalid git directory")
		}
	}
	files := map[string]bool{}
	for _, dir := range []string{admin, common} {
		for _, name := range []string{"HEAD", "index", "commondir", "gitdir", "config", "config.worktree", "packed-refs", "shallow"} {
			files[filepath.Join(dir, name)] = true
		}
	}
	// Sort through WalkDir for deterministic fingerprints, including nested
	// loose refs and other linked worktrees' HEAD/index/lock/prunable records.
	walked := map[string]bool{}
	for _, dir := range []string{admin, common} {
		for _, name := range []string{"refs", "worktrees", "reftable"} {
			path := filepath.Join(dir, name)
			if walked[path] {
				continue
			}
			walked[path] = true
			err := filepath.WalkDir(path, func(path string, entry fs.DirEntry, err error) error {
				if os.IsNotExist(err) {
					fmt.Fprintf(h, "missing:%q\n", path)
					return nil
				}
				if err != nil {
					return err
				}
				if entry.Type()&os.ModeSymlink != 0 {
					return fmt.Errorf("symlink in git metadata")
				}
				info, err := entry.Info()
				if err != nil {
					return err
				}
				metadata(path, info)
				return nil
			})
			if err != nil {
				return "", err
			}
		}
	}
	// Map iteration must not make an unchanged root look changed.
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			fmt.Fprintf(h, "missing:%q\n", path)
			continue
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("symlink in git metadata")
		}
		metadata(path, info)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
