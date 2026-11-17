package wsmailbox

import (
	"fmt"
	"regexp"
)

// listening.go implements the wake path's discoverable "armed wait" marker
// (260913-feat-cross-session-mailbox-wake Decision 6): a record kept for the
// duration of a `mailbox wait` invocation or a daemon lead mail watch and
// cleared on exit. Markers live in the memory store (memstore.go) and its
// restore ring; a previous version kept one file per marker under
// <cache root>/mailbox-listening/, which this version neither reads nor
// writes (a marker belongs to a running wait, which a daemon restart ends).
//
// Keyed by the caller's own session_key: a session can only usefully arm
// one wait at a time, and session_key is always present regardless of
// whether the caller also carries a durable slug.

// listeningKeyPattern bounds which session_key strings may key a marker:
// no separators, no dots, lowercase alnum + hyphen only. Deliberately the
// same guard as internal/mcp/session_auth.go's sessionKeyFilenamePattern
// (duplicated rather than imported: internal/mcp already imports this
// package, so importing back would cycle). It was the marker file name's
// path-safety guard and stays the key's validation.
var listeningKeyPattern = regexp.MustCompile(`^[a-z0-9-]{1,128}$`)

// ListeningMarker is one armed wait's discoverable record.
type ListeningMarker struct {
	SessionKey string `json:"session_key"`
	Slug       string `json:"slug,omitempty"`
	PID        int    `json:"pid"`
	StartedAt  string `json:"started_at"`
	TimeoutAt  string `json:"timeout_at,omitempty"`
}

// WriteListeningMarker is Paths{}.WriteListeningMarker.
func WriteListeningMarker(marker ListeningMarker) error { return Paths{}.WriteListeningMarker(marker) }

// ClearListeningMarker is Paths{}.ClearListeningMarker.
func ClearListeningMarker(sessionKey string, ownerPID int) error {
	return Paths{}.ClearListeningMarker(sessionKey, ownerPID)
}

// ReadListeningMarker is Paths{}.ReadListeningMarker.
func ReadListeningMarker(sessionKey string) (marker ListeningMarker, ok bool, err error) {
	return Paths{}.ReadListeningMarker(sessionKey)
}

// WriteListeningMarker records marker as its session's armed wait in the
// memory store (one ring record, no file of its own).
func (p Paths) WriteListeningMarker(marker ListeningMarker) error {
	if !listeningKeyPattern.MatchString(marker.SessionKey) {
		return fmt.Errorf("wsmailbox: invalid session_key %q for a listening marker", marker.SessionKey)
	}
	return p.Store().Update("listen", func(tx *Tx) error {
		tx.SetListening(marker)
		return nil
	})
}

// ClearListeningMarker removes sessionKey's marker only when it belongs to
// ownerPID (the marker's recorded PID equals ownerPID). Two waits can share
// one session_key — a host reload arms a replacement while the old wait is
// still exiting — and the later write wins, so an exiting wait that cleared
// unconditionally would delete the live replacement's marker. A marker owned
// by another PID is left in place and reported as success; clearing an
// already-clear (or never-armed) marker is a valid no-op. The compare and
// the removal are one store transaction.
func (p Paths) ClearListeningMarker(sessionKey string, ownerPID int) error {
	if !listeningKeyPattern.MatchString(sessionKey) {
		return fmt.Errorf("wsmailbox: invalid session_key %q for a listening marker", sessionKey)
	}
	return p.Store().Update("unlisten", func(tx *Tx) error {
		tx.ClearListening(sessionKey, ownerPID)
		return nil
	})
}

// ReadListeningMarker reads sessionKey's marker, reporting ok=false (err=nil)
// when no wait is currently armed for it.
func (p Paths) ReadListeningMarker(sessionKey string) (marker ListeningMarker, ok bool, err error) {
	if !listeningKeyPattern.MatchString(sessionKey) {
		return ListeningMarker{}, false, fmt.Errorf("wsmailbox: invalid session_key %q for a listening marker", sessionKey)
	}
	return p.Store().Listening(sessionKey)
}
