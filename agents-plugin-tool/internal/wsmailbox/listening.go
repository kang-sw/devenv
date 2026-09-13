package wsmailbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// listening.go implements the wake path's discoverable "armed wait" marker
// (260913-feat-cross-session-mailbox-wake Decision 6): a small on-disk record
// written for the duration of a `mailbox wait` CLI invocation and cleared on
// exit. Phase 1 only defines write/clear; no reader consumes it yet — arming
// as a host mechanism (Codex Stop hook, Claude run_in_background) is Phases
// 2-3's territory, and this file exists so that mechanism has something
// durable to point at later.
//
// Keyed by the caller's own session_key (mirroring <cache-root>/keys/<session
// -key>.json's established convention of using the raw session key as a
// filename, per ai-docs/manuals/ws-mcp.md's troubleshooting section): a
// session can only usefully arm one wait at a time, and session_key is always
// present regardless of whether the caller also carries a durable slug.

const listeningDirName = "mailbox-listening"

// listeningKeyPattern bounds which session_key strings may become a marker
// filename component: no separators, no dots, lowercase alnum + hyphen only.
// Deliberately the same path-safety guard as internal/mcp/session_auth.go's
// sessionKeyFilenamePattern (duplicated rather than imported: internal/mcp
// already imports this package, so importing back would cycle). A caller
// today only ever supplies its own already-minted session_key, but
// WriteListeningMarker/ClearListeningMarker/ReadListeningMarker are exported
// for Phase 2/3 adapters that will feed a key sourced from hook args/env, at
// which point an unvalidated key becomes a straight path traversal: write
// outside the cache root, then remove that same out-of-tree path on clear.
var listeningKeyPattern = regexp.MustCompile(`^[a-z0-9-]{1,128}$`)

// ListeningMarker is one armed wait's discoverable record.
type ListeningMarker struct {
	SessionKey string `json:"session_key"`
	Slug       string `json:"slug,omitempty"`
	PID        int    `json:"pid"`
	StartedAt  string `json:"started_at"`
	TimeoutAt  string `json:"timeout_at,omitempty"`
}

// ListeningMarkerPath resolves the marker file path for sessionKey, after
// validating sessionKey against listeningKeyPattern (path-safety: rejects
// separators/traversal such as "../../etc/passwd").
func ListeningMarkerPath(sessionKey string) (string, error) {
	if !listeningKeyPattern.MatchString(sessionKey) {
		return "", fmt.Errorf("wsmailbox: invalid session_key %q for a listening marker path", sessionKey)
	}
	root, err := wsstate.CacheRoot(wsstate.Options{})
	if err != nil {
		return "", err
	}
	return filepath.Join(root, listeningDirName, sessionKey+".json"), nil
}

// WriteListeningMarker writes marker to disk (temp-file + atomic rename,
// mirroring store.go/replyid.go's write discipline), creating the listening
// directory on first use.
func WriteListeningMarker(marker ListeningMarker) error {
	path, err := ListeningMarkerPath(marker.SessionKey)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create mailbox listening dir: %w", err)
	}
	payload, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return fmt.Errorf("encode mailbox listening marker: %w", err)
	}
	payload = append(payload, '\n')
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp mailbox listening marker: %w", err)
	}
	tmpName := tmp.Name()
	if _, werr := tmp.Write(payload); werr != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp mailbox listening marker: %w", werr)
	}
	if cerr := tmp.Close(); cerr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp mailbox listening marker: %w", cerr)
	}
	if rerr := os.Rename(tmpName, path); rerr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("atomic rename mailbox listening marker: %w", rerr)
	}
	return nil
}

// ClearListeningMarker removes sessionKey's marker file. A missing file is
// not an error: clearing an already-clear (or never-armed) marker is a valid
// no-op, matching the caller's unconditional-defer usage.
func ClearListeningMarker(sessionKey string) error {
	path, err := ListeningMarkerPath(sessionKey)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clear mailbox listening marker: %w", err)
	}
	return nil
}

// ReadListeningMarker reads sessionKey's marker, reporting ok=false (err=nil)
// when no wait is currently armed for it.
func ReadListeningMarker(sessionKey string) (marker ListeningMarker, ok bool, err error) {
	path, perr := ListeningMarkerPath(sessionKey)
	if perr != nil {
		return ListeningMarker{}, false, perr
	}
	raw, rerr := os.ReadFile(path)
	if os.IsNotExist(rerr) {
		return ListeningMarker{}, false, nil
	}
	if rerr != nil {
		return ListeningMarker{}, false, fmt.Errorf("read mailbox listening marker: %w", rerr)
	}
	if jerr := json.Unmarshal(raw, &marker); jerr != nil {
		return ListeningMarker{}, false, fmt.Errorf("parse mailbox listening marker: %w", jerr)
	}
	return marker, true, nil
}
