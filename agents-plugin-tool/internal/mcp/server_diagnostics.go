package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// appendDebugEvent records an event that belongs to no server: the
// process ring (processDebug). A server's own events go through
// Server.debugEvent, so a hosted server's ring holds only its connection's.
func appendDebugEvent(event string, fields map[string]any) {
	processDebug.append(event, fields)
}

// crashLogPath resolves the always-on, cross-platform panic-trace sink under
// the shared cache root (WS_CACHE_HOME or ~/.cache/ws@...), the same
// root-independent seam sessionStore.dirs uses for the session key store: a
// panicking request may not know which project root it belonged to, so the
// crash file lives in a flat top-level dir under CacheRoot(), not under any
// per-project layout. It creates the directory (0o755) if needed and returns
// the fixed file path; callers append-only (no rotation), matching the
// existing WS_MCP_DEBUG_LOG sink's precedent.
func crashLogPath() (string, error) {
	dir, err := crashDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mcp-panic.log"), nil
}

// crashDir resolves and creates the always-on, cross-platform crash-dir sink
// under the shared cache root (WS_CACHE_HOME or ~/.cache/ws@...). Shared by
// crashLogPath (mcp-panic.log) and RecordLifecycleEvent (mcp-lifecycle.log) so
// both sinks live in the same durable, root-independent location family.
func crashDir() (string, error) {
	cacheRoot, err := wsstate.CacheRoot(wsstate.Options{})
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cacheRoot, "crash")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create crash log dir: %w", err)
	}
	return dir, nil
}

// recordPanic persists a recovered request-goroutine panic to the always-on
// crash file (required sink) and mirrors it into the in-memory debug ring
// (and, if WS_MCP_DEBUG_LOG is set, that opt-in secondary log) via
// appendDebugEvent. Unlike appendDebugEvent's best-effort mirror, failure to
// resolve or write the crash file falls back to stderr so a broken/unwritable
// cache home never fully swallows the trace.
func recordPanic(method, id string, recovered any, stack []byte) {
	processDebug.append("request.panic", recordPanicToCrashLog(method, id, recovered, stack))
}

// recordPanic is the package function's server form: the crash file plus
// this server's ring.
func (s *Server) recordPanic(method, id string, recovered any, stack []byte) {
	s.debugEvent("request.panic", recordPanicToCrashLog(method, id, recovered, stack))
}

func recordPanicToCrashLog(method, id string, recovered any, stack []byte) map[string]any {
	fields := map[string]any{
		"id":     id,
		"method": method,
		"panic":  fmt.Sprint(recovered),
		"stack":  string(stack),
	}
	record := map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"event": "request.panic",
	}
	for key, value := range fields {
		record[key] = value
	}
	if raw, err := json.Marshal(record); err == nil {
		if path, err := crashLogPath(); err == nil {
			if file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
				_, writeErr := file.Write(append(raw, '\n'))
				closeErr := file.Close()
				if writeErr != nil || closeErr != nil {
					fmt.Fprintf(os.Stderr, "mcp: failed to append crash log %s: write=%v close=%v\n", path, writeErr, closeErr)
				}
			} else {
				fmt.Fprintf(os.Stderr, "mcp: failed to open crash log %s: %v\n", path, err)
			}
		} else {
			fmt.Fprintf(os.Stderr, "mcp: failed to resolve crash log path: %v\n", err)
		}
	} else {
		fmt.Fprintf(os.Stderr, "mcp: failed to marshal crash record: %v\n", err)
	}
	return fields
}

// RecordLifecycleEvent persists a serve-process lifecycle event (e.g. detected
// parent-process death) to the always-on crash-dir sink (<cache-root>/crash/
// mcp-lifecycle.log), the same durable location family as recordPanic, and
// mirrors it into the in-memory debug ring / opt-in WS_MCP_DEBUG_LOG via
// appendDebugEvent. Exported so cmd/ws-mcp's Windows parent-death watcher can
// leave a durable trace before it self-terminates the orphaned process. Failure
// to resolve or write the file falls back to stderr so a broken cache home
// never fully swallows the trace.
func RecordLifecycleEvent(event string, fields map[string]any) {
	record := map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"event": event,
	}
	for key, value := range fields {
		record[key] = value
	}
	if raw, err := json.Marshal(record); err == nil {
		if dir, err := crashDir(); err == nil {
			path := filepath.Join(dir, "mcp-lifecycle.log")
			if file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
				_, writeErr := file.Write(append(raw, '\n'))
				closeErr := file.Close()
				if writeErr != nil || closeErr != nil {
					fmt.Fprintf(os.Stderr, "mcp: failed to append lifecycle log %s: write=%v close=%v\n", path, writeErr, closeErr)
				}
			} else {
				fmt.Fprintf(os.Stderr, "mcp: failed to open lifecycle log %s: %v\n", path, err)
			}
		} else {
			fmt.Fprintf(os.Stderr, "mcp: failed to resolve lifecycle log path: %v\n", err)
		}
	} else {
		fmt.Fprintf(os.Stderr, "mcp: failed to marshal lifecycle record: %v\n", err)
	}

	appendDebugEvent(event, fields)
}

func recentDebugEvents(limit int) []map[string]any { return processDebug.recent(limit) }
