package wsagent

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
)

// recordFileName is the conversation record's file in its session
// directory, beside the wslive snapshot and ring.
const recordFileName = "agent.json"

const recordSchemaVersion = 1

// Record is one conversation's persisted state: everything a process start
// needs after a daemon restart. Field names are an on-disk format. The file
// is private to the user (it holds the ws session key).
type Record struct {
	SchemaVersion int    `json:"schema_version"`
	ReplyID       string `json:"reply_id"`
	Profile       string `json:"profile"`
	Backend       string `json:"backend"`
	Root          string `json:"root"`
	// SessionID is the backend session the next process resumes; empty
	// until the first process reported one.
	SessionID string `json:"backend_session_id,omitempty"`
	Model     string `json:"model,omitempty"`
	Effort    string `json:"effort,omitempty"`
	// WSKey is the conversation's ws session key; it never leaves this file,
	// the composed instruction file, and the child processes.
	WSKey string `json:"ws_session_key"`
	// Listed lists the conversation in the dashboard session index (the
	// profile's list_in_index when the conversation last started).
	Listed    bool   `json:"listed,omitempty"`
	CreatedAt string `json:"created_at"`
	ClosedAt  string `json:"closed_at,omitempty"`
}

func recordPath(sessionDir string) string { return filepath.Join(sessionDir, recordFileName) }

func readRecord(sessionDir string) (Record, error) {
	raw, err := os.ReadFile(recordPath(sessionDir))
	if err != nil {
		return Record{}, err
	}
	var rec Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		return Record{}, fmt.Errorf("parse %s: %w", recordPath(sessionDir), err)
	}
	if rec.ReplyID != filepath.Base(sessionDir) || rec.WSKey == "" || rec.Root == "" {
		return Record{}, fmt.Errorf("%s is not a complete agent record", recordPath(sessionDir))
	}
	return rec, nil
}

func writeRecord(sessionDir string, rec Record) error {
	if rec.SchemaVersion == 0 {
		rec.SchemaVersion = recordSchemaVersion
	}
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(recordPath(sessionDir), append(raw, '\n'), 0o600)
}

// writeFileAtomic replaces path with data through a temporary file in the
// same directory.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	fail := func(err error) error {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		return fail(err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

// liveWriter keeps one conversation's wslive state and writes it to the
// session directory: ring entries as they finish, the snapshot after every
// change and on each heartbeat.
type liveWriter struct {
	dir string
	st  *wslive.State
	now func() time.Time
	log io.Writer
}

func newLiveWriter(dir, key, adapter string, now func() time.Time, log io.Writer) *liveWriter {
	st := wslive.NewState(key)
	st.SetAdapter(adapter)
	return &liveWriter{dir: dir, st: st, now: now, log: log}
}

// apply applies ev and writes what it changed; it reports whether anything
// was written.
func (w *liveWriter) apply(ev wslive.Event) bool {
	res, err := w.st.Apply(ev, w.now().UnixMilli())
	if err != nil {
		fmt.Fprintf(w.log, "agent %s: drop %s event: %v\n", w.st.ReplyID(), ev.Kind, err)
		return false
	}
	if len(res.Ring) > 0 {
		if err := wslive.AppendRing(wslive.RingDir(w.dir), res.Ring); err != nil {
			fmt.Fprintf(w.log, "agent %s: ring write: %v\n", w.st.ReplyID(), err)
		}
	}
	if !res.Changed && len(res.Ring) == 0 {
		return false
	}
	w.writeSnapshot()
	return true
}

// setAdapter changes the snapshot's adapter name ("" for a closed
// conversation, so the page's composer refuses input) and writes it.
func (w *liveWriter) setAdapter(name string) {
	w.st.SetAdapter(name)
	w.writeSnapshot()
}

func (w *liveWriter) writeSnapshot() {
	raw, err := json.Marshal(w.st.Snapshot(w.now().UnixMilli()))
	if err == nil {
		err = writeFileAtomic(wslive.SnapshotPath(w.dir), raw, 0o644)
	}
	if err != nil {
		fmt.Fprintf(w.log, "agent %s: snapshot write: %v\n", w.st.ReplyID(), err)
	}
}

// registration is the conversation's session-registry entry.
func registration(rec Record, now time.Time) wsdashboard.Registration {
	reg := wsdashboard.Registration{
		ReplyID:      rec.ReplyID,
		Project:      rec.Root,
		Harness:      rec.Backend,
		RegisteredAt: rec.CreatedAt,
		HeartbeatAt:  now.UTC().Format(time.RFC3339),
		Agent:        true,
		Unlisted:     !rec.Listed,
	}
	if rec.ClosedAt != "" {
		reg.EndedAt = rec.ClosedAt
	}
	return reg
}
