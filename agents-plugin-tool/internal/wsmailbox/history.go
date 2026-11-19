package wsmailbox

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// history.go holds each session's mail history window: the last HistoryCap
// messages it sent or drained, full bodies included, so mail a conversation
// lost can be recovered with mailbox.history. The window lives in the
// memory store (memstore.go) and its restore ring; the per-session entry
// files under <cache root>/mailbox-history/<session-key>/ are the layout a
// previous ws-mcp version reads and writes, kept only for the import at
// start and the export at a graceful daemon shutdown (legacy.go).

const (
	// HistoryCap is each session's window, received and sent together.
	HistoryCap = 10
	// HistoryRetention drops the window of a session idle past it when a
	// rotation carries live state forward. It equals the session key
	// retention, so stored bodies do not outlive the keys that reach them.
	HistoryRetention = 30 * 24 * time.Hour
	// HistoryDirName is the legacy history root under the cache root.
	HistoryDirName = "mailbox-history"
	// HistoryStampLayout is an entry name's stamp: fixed width with
	// nanoseconds, so names sort lexically in time order.
	HistoryStampLayout = "20060102T150405.000000000Z"

	HistoryRecv = "recv"
	HistorySent = "sent"
)

// HistoryEntry is one history record. Name is its stable key, the legacy
// entry file name (<stamp>-<random hex>.json), whose stamp is the single
// source of the exchange time; a conversation ring mail line carries it.
type HistoryEntry struct {
	Name         string `json:"name"`
	Direction    string `json:"direction"`
	Counterparty string `json:"counterparty"`
	Body         string `json:"body"`
}

// legacyHistoryFile is one legacy entry file's body; its name holds the rest.
type legacyHistoryFile struct {
	Direction    string `json:"direction"`
	Counterparty string `json:"counterparty"`
	Body         string `json:"body"`
}

// HistoryTime parses an entry name's stamp.
func HistoryTime(name string) (time.Time, bool) {
	stamp, _, ok := strings.Cut(name, "-")
	if !ok {
		return time.Time{}, false
	}
	at, err := time.Parse(HistoryStampLayout, stamp)
	if err != nil {
		return time.Time{}, false
	}
	return at.UTC(), true
}

// newHistoryName names an entry stamped at.
func newHistoryName(at time.Time) (string, error) {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return at.UTC().Format(HistoryStampLayout) + "-" + hex.EncodeToString(suffix[:]) + ".json", nil
}

// isHistoryEntryName reports whether name is a committed legacy entry file,
// excluding in-flight temp files (dot-prefixed, no .json suffix).
func isHistoryEntryName(name string) bool {
	if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".json") {
		return false
	}
	_, ok := HistoryTime(name)
	return ok
}

// legacyHistoryNames lists dir's committed entry names, newest first.
func legacyHistoryNames(dir string) ([]string, error) {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(dirEntries))
	for _, de := range dirEntries {
		if isHistoryEntryName(de.Name()) {
			names = append(names, de.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	return names, nil
}

func readLegacyHistoryEntry(dir, name string) (HistoryEntry, bool) {
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return HistoryEntry{}, false
	}
	var f legacyHistoryFile
	if json.Unmarshal(raw, &f) != nil {
		return HistoryEntry{}, false
	}
	return HistoryEntry{Name: name, Direction: f.Direction, Counterparty: f.Counterparty, Body: f.Body}, true
}

// writeLegacyHistoryEntry writes e under its own name through temp+rename,
// the way the previous version wrote it.
func writeLegacyHistoryEntry(dir string, e HistoryEntry) error {
	raw, err := json.Marshal(legacyHistoryFile{Direction: e.Direction, Counterparty: e.Counterparty, Body: e.Body})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create mailbox history temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write mailbox history entry: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("write mailbox history entry: %w", err)
	}
	if err := os.Rename(tmpPath, filepath.Join(dir, e.Name)); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("commit mailbox history entry: %w", err)
	}
	return nil
}

// trimHistory keeps the newest HistoryCap entries of window (oldest first).
func trimHistory(window []HistoryEntry) []HistoryEntry {
	if len(window) > HistoryCap {
		window = window[len(window)-HistoryCap:]
	}
	return window
}

// historyIdle reports whether window's newest entry is older than the
// retention at now (an empty window is idle).
func historyIdle(window []HistoryEntry, now time.Time) bool {
	if len(window) == 0 {
		return true
	}
	at, ok := HistoryTime(window[len(window)-1].Name)
	return !ok || now.Sub(at) > HistoryRetention
}
