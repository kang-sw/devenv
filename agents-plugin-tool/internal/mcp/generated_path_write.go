package mcp

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

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
	mu      sync.Mutex
	file    *os.File
	created time.Time
}

const maxGeneratedPathGrants = 64

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
	eligible := make([]wsstate.GeneratedPath, 0, len(generated))
	for _, item := range generated {
		if item.Kind != "review" && item.Kind != "prompt" {
			continue
		}
		eligible = append(eligible, item)
	}
	if len(eligible) > maxGeneratedPathGrants {
		return fmt.Errorf("path.generate: requested %d writable cache artifacts; maximum is %d", len(eligible), maxGeneratedPathGrants)
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if g.paths == nil {
		g.paths = make(map[string]map[string]*generatedPathGrant)
	}
	for g.grantCountLocked()+len(eligible) > maxGeneratedPathGrants {
		g.evictOldestLocked()
	}

	opened := make(map[string]*generatedPathGrant, len(eligible))
	for _, item := range eligible {
		before, err := os.Lstat(item.Path)
		if err != nil {
			closeGeneratedPathGrants(opened)
			return fmt.Errorf("path.generate: inspect writable cache artifact %q: %w", item.Path, err)
		}
		if !before.Mode().IsRegular() {
			closeGeneratedPathGrants(opened)
			return fmt.Errorf("path.generate: writable cache artifact %q is not a regular file", item.Path)
		}
		file, err := os.OpenFile(item.Path, os.O_RDWR, 0)
		if err != nil {
			closeGeneratedPathGrants(opened)
			return fmt.Errorf("path.generate: open writable cache artifact %q: %w", item.Path, err)
		}
		after, err := file.Stat()
		if err != nil || !os.SameFile(before, after) {
			_ = file.Close()
			closeGeneratedPathGrants(opened)
			return fmt.Errorf("path.generate: writable cache artifact %q changed while it was granted", item.Path)
		}
		opened[item.Path] = &generatedPathGrant{file: file, created: time.Now()}
	}
	for path, grant := range opened {
		if g.paths[sessionKey] == nil {
			g.paths[sessionKey] = make(map[string]*generatedPathGrant)
		}
		if prior := g.paths[sessionKey][path]; prior != nil {
			prior.mu.Lock()
			_ = prior.file.Close()
			prior.mu.Unlock()
		}
		g.paths[sessionKey][path] = grant
	}
	return nil
}

func closeGeneratedPathGrants(grants map[string]*generatedPathGrant) {
	for _, grant := range grants {
		_ = grant.file.Close()
	}
}

func (g *generatedPathWrites) grantCountLocked() int {
	total := 0
	for _, paths := range g.paths {
		total += len(paths)
	}
	return total
}

func (g *generatedPathWrites) evictOldestLocked() {
	var oldestKey, oldestPath string
	var oldest *generatedPathGrant
	for key, paths := range g.paths {
		for path, grant := range paths {
			if oldest == nil || grant.created.Before(oldest.created) {
				oldestKey, oldestPath, oldest = key, path, grant
			}
		}
	}
	if oldest == nil {
		return
	}
	delete(g.paths[oldestKey], oldestPath)
	if len(g.paths[oldestKey]) == 0 {
		delete(g.paths, oldestKey)
	}
	oldest.mu.Lock()
	_ = oldest.file.Close()
	oldest.mu.Unlock()
}

func (g *generatedPathWrites) write(sessionKey, path, content string) error {
	sessionKey = strings.TrimSpace(sessionKey)
	g.mu.Lock()
	grant := g.paths[sessionKey][path]
	if grant == nil {
		g.mu.Unlock()
		return fmt.Errorf("path.write refused: path was not generated for this session key as a writable cache artifact")
	}
	delete(g.paths[sessionKey], path)
	if len(g.paths[sessionKey]) == 0 {
		delete(g.paths, sessionKey)
	}
	g.mu.Unlock()
	grant.mu.Lock()
	defer func() {
		_ = grant.file.Close()
		grant.mu.Unlock()
	}()

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
