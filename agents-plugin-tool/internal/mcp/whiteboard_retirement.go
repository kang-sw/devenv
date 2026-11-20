package mcp

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// One sweep per project/cache pair in the daemon, shared by its connections
// and page reader. No recovery marker is needed: repeating the closed-set
// sweep after a restart is harmless, including after an older writer ran.
type whiteboardRetirement struct {
	roots sync.Map
	// sweep is an instance-local test seam for holding the background scan.
	sweep func(string, func() (map[string]bool, bool))
}

var retiredWhiteboardPages whiteboardRetirement

func (r *whiteboardRetirement) afterOpen(root, cacheHome string, live func() (map[string]bool, bool)) {
	if strings.TrimSpace(root) == "" {
		return
	}
	key := struct{ root, cache string }{filepath.Clean(root), cacheHome}
	if _, loaded := r.roots.LoadOrStore(key, true); loaded {
		return
	}
	sweep := r.sweep
	if sweep == nil {
		sweep = sweepRetiredWhiteboardPages
	}
	go sweep(filepath.Join(root, filepath.FromSlash(whiteboardDirRel)), live)
}

var (
	retiredMermaidFile = regexp.MustCompile(`^wb-mermaid-[0-9a-f]{12}\.js$`)
	retiredEmbedFile   = regexp.MustCompile(`^([a-z0-9]+)-[0-9a-f]+\.html$`)
)

func sweepRetiredWhiteboardPages(dir string, live func() (map[string]bool, bool)) {
	// A failed session listing cannot establish that an embed is dead.
	liveIDs, known := live()
	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		name := entry.Name()
		remove := name == "index.html" || name == "index.stamp.js" || retiredMermaidFile.MatchString(name)
		// Legacy embeds were flat children of this directory. Never walk a
		// host-owned embed folder or interpret topic panel files as embeds.
		if !remove && known && filepath.Dir(path) == dir {
			match := retiredEmbedFile.FindStringSubmatch(name)
			remove = match != nil && !liveIDs[match[1]]
		}
		if remove {
			_ = os.Remove(path)
		}
		return nil
	})
}

func (s *sessionStore) retireWhiteboardPages(root string) {
	// Session lookup already resolved and memoized this path. Use the same
	// identity for a hosted caller's explicit cache and the daemon's environment.
	cacheHome, err := wsstate.CacheRoot(wsstate.Options{CacheHome: s.cacheHome})
	if err != nil {
		return
	}
	r := s.boardRetirement
	if r == nil {
		r = &retiredWhiteboardPages
	}
	r.afterOpen(root, cacheHome, s.liveReplyIDs)
}
