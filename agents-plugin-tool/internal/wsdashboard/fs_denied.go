package wsdashboard

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// Keep the default locations protected even when a tool relocates its current
// credentials. Relocated homes protect the same credential files as defaults,
// not unrelated history/session files in those directories.
func deniedBasePaths(homes []string) []string {
	deniedBasePathBuilds.Add(1)
	var bases []string
	seen := map[string]bool{}
	add := func(path string) {
		if path == "" {
			return
		}
		path = filepath.FromSlash(homePath(path))
		if !filepath.IsAbs(path) {
			var err error
			path, err = filepath.Abs(path)
			if err != nil {
				return
			}
		}
		path = filepath.Clean(path)
		if !seen[path] {
			seen[path] = true
			bases = append(bases, path)
		}
	}
	addDaemonDir := func(dir string) {
		add(dir)
		// Individual identities also protect hardlinks outside the directory.
		for _, name := range []string{localhostSecretFile, LANTokenFileName, endpointFileName} {
			add(filepath.Join(dir, name))
		}
	}
	for _, home := range homes {
		for _, name := range outsideDenied {
			add(filepath.Join(home, filepath.FromSlash(name)))
		}
		if dir, err := Dir(filepath.Join(home, ".cache", wsstate.DefaultCacheDirName)); err == nil {
			addDaemonDir(dir)
		}
	}
	// Use the daemon's own resolver, including WS_CACHE_HOME relocation.
	if dir, err := Dir(""); err == nil {
		addDaemonDir(dir)
	}
	for _, key := range []string{"GH_CONFIG_DIR", "AWS_SHARED_CREDENTIALS_FILE", "AWS_CONFIG_FILE"} {
		add(os.Getenv(key))
	}
	for _, c := range []struct{ key, name string }{
		{"XDG_CONFIG_HOME", "gh"}, {"XDG_CONFIG_HOME", "gcloud"},
		{"APPDATA", "GitHub CLI"}, {"APPDATA", "gcloud"},
		{"CODEX_HOME", "auth.json"}, {"CLAUDE_CONFIG_DIR", ".credentials.json"},
	} {
		if dir := os.Getenv(c.key); dir != "" {
			add(filepath.Join(dir, c.name))
		}
	}
	for _, path := range filepath.SplitList(os.Getenv("KUBECONFIG")) {
		add(path)
	}
	return bases
}

// deniedBasePathBuilds counts deniedBasePaths calls, the start of every
// deny-set computation; tests use it to pin O(1) computations per request.
var deniedBasePathBuilds atomic.Int64

// denySet is one request's view of the always-denied credential locations:
// the homes, the denied base paths, and the identities of the bases that
// exist, each computed at most once per request instead of once per path
// checked. A fresh set per request keeps freshly created or replaced
// credentials protected on the next request, which a process-wide TTL cache
// would not. It also memoizes the identity verdict of resolved directories,
// which a request's candidates share. Not safe for concurrent use.
type denySet struct {
	fixed, loaded, statted bool
	homes, bases           []string
	// lower are bases lowercased, for the case-insensitive prefix check.
	lower []string
	// ids are the identities of the bases that exist, at idPaths.
	ids     []os.FileInfo
	idPaths []string
	above   map[string]bool
}

// newDenySet is a lazy deny set for one request, for the current homes.
func newDenySet() *denySet { return &denySet{} }

func denySetFor(homes []string) *denySet { return &denySet{fixed: true, homes: homes} }

func (s *denySet) load() {
	if s.loaded {
		return
	}
	s.loaded = true
	if !s.fixed {
		s.homes = homeDirs()
	}
	s.bases = deniedBasePaths(s.homes)
	for _, base := range s.bases {
		s.lower = append(s.lower, strings.ToLower(base))
	}
}

// homeDirs are the homes this set checks against.
func (s *denySet) homeDirs() []string {
	s.load()
	return s.homes
}

// lexical is the string-only part of alwaysDenied: Linux process
// environments, a path at or under a denied base without regard to case,
// and, without a home, a denied name anywhere in the path. It touches no
// filesystem once the set is loaded.
func (s *denySet) lexical(path string) bool {
	if deniedProcEnviron(path) {
		return true
	}
	s.load()
	lower := strings.ToLower(filepath.Clean(path))
	if len(s.homes) == 0 {
		slashed := "/" + filepath.ToSlash(lower) + "/"
		for _, name := range outsideDenied {
			if strings.Contains(slashed, "/"+strings.ToLower(name)+"/") {
				return true
			}
		}
	}
	for _, base := range s.lower {
		// At or under base, without regard to case: a case-insensitive
		// volume reads ~/.SSH as ~/.ssh. Both sides are clean, so a prefix
		// ending at a separator is filepath.Rel's answer without its cost.
		if lower == base || strings.HasPrefix(lower, base) &&
			(os.IsPathSeparator(base[len(base)-1]) || os.IsPathSeparator(lower[len(base)])) {
			return true
		}
	}
	return false
}

// identities stats each base once for this request, rather than once per
// candidate ancestor. ENOTDIR, missing, and unreadable bases cannot supply
// identities, but remain lexically protected; one broken base never blocks
// other reads.
func (s *denySet) identities() []os.FileInfo {
	if s.statted {
		return s.ids
	}
	s.statted = true
	s.load()
	for _, base := range s.bases {
		// Do not trigger network/device access just to inspect a configured
		// credential location. Such paths remain lexically denied.
		if networkPath(base) || deviceElement(base) {
			continue
		}
		if info, err := os.Stat(base); err == nil {
			s.ids = append(s.ids, info)
			s.idPaths = append(s.idPaths, base)
		}
	}
	return s.ids
}

// aliasRoots extends the string bases with each existing denied base as
// spelled under any of roots that is, by identity, the base or one of its
// ancestors: a subst drive, a macOS firmlink, or a case or normalization
// spelling that EvalSymlinks keeps. String checks of names under such a root
// then catch what only per-name identity checks did before. It costs one
// stat per root and per distinct base ancestor, whatever the names checked.
func (s *denySet) aliasRoots(roots []string) {
	s.identities()
	var rootInfos []os.FileInfo
	var rootPaths []string
	for _, root := range roots {
		if info, err := os.Stat(root); err == nil {
			rootInfos, rootPaths = append(rootInfos, info), append(rootPaths, root)
		}
	}
	if len(rootInfos) == 0 {
		return
	}
	seen := map[string]os.FileInfo{}
	for _, base := range s.idPaths {
		for dir := base; ; dir = filepath.Dir(dir) {
			info, ok := seen[dir]
			if !ok {
				// A directory that does not stat stays nil and matches no root.
				if fi, err := os.Stat(dir); err == nil {
					info = fi
				}
				seen[dir] = info
			}
			for i, root := range rootInfos {
				if info == nil || !os.SameFile(info, root) {
					continue
				}
				if rel, err := filepath.Rel(dir, base); err == nil {
					s.lower = append(s.lower, strings.ToLower(filepath.Join(rootPaths[i], rel)))
				}
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}
}

// opened inspects the identity of an actually opened handle: a pathname can
// change between authorization and open, or gain a credential hardlink in
// that window. It costs no syscall beyond the handle's own Stat.
func (s *denySet) opened(info os.FileInfo) bool {
	return sameDeniedFile(info, s.identities())
}

// identityAtOrAbove checks the resolved path and each ancestor by identity.
// EvalSymlinks alone does not canonicalize APFS names or firmlinks; SameFile
// does not depend on their spelling. Stat also covers aliases of denylisted
// individual files. The verdict per resolved path is memoized for the
// request; resolution itself never is, so a re-resolve after open still sees
// a swapped path.
func (s *denySet) identityAtOrAbove(real string) bool {
	if deniedProcEnviron(real) {
		return true
	}
	bases := s.identities()
	if len(bases) == 0 {
		return false
	}
	return s.aboveDenied(filepath.Clean(real), bases)
}

func (s *denySet) aboveDenied(path string, bases []os.FileInfo) bool {
	if denied, ok := s.above[path]; ok {
		return denied
	}
	info, err := os.Stat(path)
	denied := err == nil && sameDeniedFile(info, bases)
	if parent := filepath.Dir(path); !denied && parent != path {
		denied = s.aboveDenied(parent, bases)
	}
	if s.above == nil {
		s.above = map[string]bool{}
	}
	s.above[path] = denied
	return denied
}

// denied is alwaysDenied under this set: the lexical check, then the identity
// of the path resolved through its symlinks and of each ancestor.
func (s *denySet) denied(path string) bool {
	if s.lexical(path) {
		return true
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	return s.identityAtOrAbove(path)
}

func sameDeniedFile(info os.FileInfo, bases []os.FileInfo) bool {
	for _, base := range bases {
		if os.SameFile(info, base) {
			return true
		}
	}
	return false
}

// deniedOpenedFile is opened under a fresh set, for a caller with none.
func deniedOpenedFile(info os.FileInfo) bool {
	return newDenySet().opened(info)
}

func deniedProcEnviron(path string) bool {
	if runtime.GOOS != "linux" || !filepath.IsAbs(path) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(filepath.Clean(path), "/"), "/")
	return len(parts) >= 3 && parts[0] == "proc" && parts[len(parts)-1] == "environ" &&
		(len(parts) == 3 || (len(parts) == 5 && parts[2] == "task"))
}
