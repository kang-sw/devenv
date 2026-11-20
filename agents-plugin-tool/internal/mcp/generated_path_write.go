package mcp

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// generatedPathWrites records cache-artifact capabilities minted by
// path.generate. Repository-local plan paths and clone-shared documents are
// deliberately excluded: path.write exists only for sandbox-independent
// review/prompt handoffs.
type generatedPathWrites struct {
	mu    sync.RWMutex
	paths map[string]map[string]*generatedPathGrant
}

type generatedPathGrant struct {
	mu   sync.Mutex
	file *os.File
}

// processGeneratedWrites is shared by the daemon's per-connection MCP
// servers, so an ordinary reconnect does not lose a session's minted path.
// Session keys are globally unique within the cache root; a daemon restart
// intentionally revokes these short-lived cache-write capabilities instead
// of introducing a persisted authority record.
var processGeneratedWrites generatedPathWrites

func (g *generatedPathWrites) grant(sessionKey string, generated []wsstate.GeneratedPath) error {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return fmt.Errorf("path.generate: session_key is required for writable cache artifacts")
	}
	opened := make(map[string]*generatedPathGrant)
	for _, item := range generated {
		if item.Kind != "review" && item.Kind != "prompt" {
			continue
		}
		before, err := os.Lstat(item.Path)
		if err != nil {
			return fmt.Errorf("path.generate: inspect writable cache artifact %q: %w", item.Path, err)
		}
		if !before.Mode().IsRegular() {
			return fmt.Errorf("path.generate: writable cache artifact %q is not a regular file", item.Path)
		}
		file, err := os.OpenFile(item.Path, os.O_RDWR, 0)
		if err != nil {
			return fmt.Errorf("path.generate: open writable cache artifact %q: %w", item.Path, err)
		}
		after, err := file.Stat()
		if err != nil || !os.SameFile(before, after) {
			_ = file.Close()
			return fmt.Errorf("path.generate: writable cache artifact %q changed while it was granted", item.Path)
		}
		opened[item.Path] = &generatedPathGrant{file: file}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for path, grant := range opened {
		if g.paths == nil {
			g.paths = make(map[string]map[string]*generatedPathGrant)
		}
		if g.paths[sessionKey] == nil {
			g.paths[sessionKey] = make(map[string]*generatedPathGrant)
		}
		if prior := g.paths[sessionKey][path]; prior != nil {
			_ = prior.file.Close()
		}
		g.paths[sessionKey][path] = grant
	}
	return nil
}

func (g *generatedPathWrites) write(sessionKey, path, content string) error {
	sessionKey = strings.TrimSpace(sessionKey)
	g.mu.RLock()
	grant := g.paths[sessionKey][path]
	g.mu.RUnlock()
	if grant == nil {
		return fmt.Errorf("path.write refused: path was not generated for this session key as a writable cache artifact")
	}
	grant.mu.Lock()
	defer grant.mu.Unlock()

	pathInfo, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("path.write inspect target: %w", err)
	}
	fileInfo, err := grant.file.Stat()
	if err != nil {
		return fmt.Errorf("path.write inspect granted artifact: %w", err)
	}
	if !pathInfo.Mode().IsRegular() || !fileInfo.Mode().IsRegular() || !os.SameFile(pathInfo, fileInfo) {
		return fmt.Errorf("path.write refused: generated target is not a regular file")
	}
	if err := grant.file.Truncate(0); err != nil {
		return fmt.Errorf("path.write truncate artifact: %w", err)
	}
	if _, err := grant.file.Seek(0, 0); err != nil {
		return fmt.Errorf("path.write seek artifact: %w", err)
	}
	if _, err := grant.file.WriteString(content); err != nil {
		return fmt.Errorf("path.write content: %w", err)
	}
	return nil
}
