package wsexec

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

type gitSearchKey struct {
	explicit      bool
	path, pathext string
}

type gitPathCache struct {
	sync.Mutex
	key  gitSearchKey
	path string
}

var resolvedGit gitPathCache

// GitPath is LookPath("git", environ) with one shared successful-entry cache.
// Only cwd-independent searches are reusable. Changing the effective PATH,
// PATHEXT or nil/explicit mode resolves again; failures always retry. Under an
// unchanged key a success may stay stale after installation, removal, chmod or
// symlink changes: no re-stat, TTL or launch-error retry is performed.
func GitPath(environ []string) (string, error) {
	return resolvedGit.resolve(environ, LookPath)
}

func (c *gitPathCache) resolve(environ []string, lookup func(string, []string) (string, error)) (string, error) {
	key, reusable := gitSearchContext(environ)
	if !reusable {
		return lookup("git", environ)
	}
	// Hold the lock across the cold search, not only publication: concurrent
	// identical cold callers must not all pay the slow PATH-prefix cost.
	c.Lock()
	defer c.Unlock()
	if c.path != "" && c.key == key {
		return c.path, nil
	}
	c.path = ""
	path, err := lookup("git", environ)
	if err == nil && filepath.IsAbs(path) {
		c.key, c.path = key, path
	}
	return path, err
}

func gitSearchContext(environ []string) (gitSearchKey, bool) {
	path, pathext := searchPath(environ)
	key := gitSearchKey{explicit: environ != nil, path: path, pathext: pathext}
	if environ == nil && runtime.GOOS == "windows" {
		// Go's process lookup implicitly searches cwd unless this variable is
		// present, even with an empty value. Explicit LookPath never does.
		if _, disabled := os.LookupEnv("NoDefaultCurrentDirectoryInExePath"); !disabled {
			return key, false
		}
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" && (environ != nil || runtime.GOOS == "windows") {
			continue // These empty entries are skipped by the underlying lookup.
		}
		if !filepath.IsAbs(dir) {
			// Includes Windows drive-relative and root-relative entries. Neither
			// -C nor cmd.Dir defines the cwd used during executable lookup.
			return key, false
		}
	}
	return key, true
}
