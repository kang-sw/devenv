package wsagent

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

// recordFileName is the conversation record's file in its session
// directory, beside the wslive snapshot and ring.
const recordFileName = wsdashboard.AgentRecordFileName

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

	// HandedOffAt is when a lead session was handed to a terminal harness
	// (conv.release): the daemon serves it no more, as it serves no closed
	// one, until a takeover clears it.
	HandedOffAt string `json:"handed_off_at,omitempty"`

	// Lead marks a hosted lead session (Profile.Lead when it was opened):
	// WSKey is a lead key, Root its canonical git root, and the fields
	// below apply.
	Lead bool `json:"lead,omitempty"`
	// PermissionMode is the permission mode the next process launches in.
	PermissionMode string `json:"permission_mode,omitempty"`
	// Cwd is the directory the session's process runs in when that is not
	// Root: a session taken over from a terminal harness keeps the
	// directory that harness ran in, since the harness resolves its
	// sessions by it, and a spawned conversation (Runner.Spawn) the one its
	// spawner chose. SessionFile is the session file of a taken-over
	// session's harness when it keeps its conversations in files of its
	// own (pi), used in place of the conversation's own file. Both are
	// empty for any other session the daemon started.
	Cwd         string `json:"cwd,omitempty"`
	SessionFile string `json:"session_file,omitempty"`
	// HolderID is the session holder the conversation's process runs under,
	// empty when it has none; HostPID is that process. No new process
	// starts while this holder's child still runs.
	HolderID string `json:"holder_id,omitempty"`
	HostPID  int    `json:"host_pid,omitempty"`
	// HolderSeq is the last record sequence of HolderID the runner applied
	// whole: after a daemon restart the holder replays its ring, and
	// records up to this one rebuild state without being written again.
	HolderSeq uint64 `json:"holder_seq,omitempty"`
	// Answered are the request ids of the tool permission prompts answered
	// and not yet seen settled. The answer went to the process's input,
	// which the holder's ring does not hold, so a replay shows such a
	// prompt as still waiting; this list keeps it answered exactly once.
	Answered []string `json:"answered,omitempty"`
	// Mail holds the mailbox messages claimed for the session and not yet
	// delivered as a turn, each as the text the model reads.
	Mail []string `json:"mail,omitempty"`
	// Queued holds what a drain took from the inbox and no turn has sent
	// yet, oldest first: a slash command runs as a turn of its own, so the
	// messages and commands drained after it wait here across turn ends,
	// although the inbox already shows them received.
	Queued []QueuedInput `json:"queued,omitempty"`

	// Parent is the session that spawned the conversation (Runner.Spawn),
	// to which each of its settled turns is reported (Options.Report); nil
	// for one opened from the dashboard or the REST routes. Label is the
	// one-line name its spawner gave it.
	Parent *ParentRef `json:"parent,omitempty"`
	Label  string     `json:"label,omitempty"`
	// TurnOpen is set on a conversation with a Parent while one of its
	// turns runs: a daemon that exits mid-turn leaves it set, and the next
	// daemon reports that turn as interrupted when it loads the record.
	TurnOpen bool `json:"turn_open,omitempty"`
	// Tier is the capability tier its spawner chose (small, medium, large,
	// or xlarge); a conversation it spawns may ask for no higher one.
	// Empty for a conversation spawned without one, which spawns none.
	Tier string `json:"tier,omitempty"`
	// PendingReports are the reported turns of a conversation whose parent
	// is a conversation (ParentConversation) that the runner has not handed
	// to that parent yet, oldest first: a report's absence is its
	// "delivered" mark. A report whose parent is closed stays here.
	PendingReports []PendingReport `json:"pending_reports,omitempty"`
	// Reports are its own children's reports the runner handed to this
	// conversation and no turn has taken yet, oldest first: they go with
	// its next turn, as a lead's claimed Mail does.
	Reports []string `json:"reports,omitempty"`
	// LastTurn is the last reported turn of a conversation with a Parent,
	// with the id of the pending report it made (empty when its parent's
	// wait took it instead): the next daemon answers a wait with it at
	// once, since the settles a wait reads are not persisted. Nil before
	// the first report and while a done turn is held for the children.
	LastTurn *PendingReport `json:"last_turn,omitempty"`
}

// PendingReport is a settled turn's report waiting for its conversation
// parent (Record.PendingReports). ID tells one report from another, so
// the wait that answers with a turn takes that turn's report alone.
type PendingReport struct {
	ID     string     `json:"id"`
	Result TurnResult `json:"result"`
}

// Parent kinds a ParentRef names.
const (
	// ParentLead is a lead session, reached through its mailbox reply-id:
	// its child's turn reports arrive as mail.
	ParentLead = "lead"
	// ParentConversation is a conversation of this runner (a spawned Pi
	// worker), reached by its id: the runner delivers its child's turn
	// reports into it itself (children.go).
	ParentConversation = "conversation"
)

// ParentRef names the session that spawned a conversation. It holds the
// parent's reply-id, never its ws session key, so no credential of the
// parent's lands in the child's record; Kind says how the parent is
// reached (ParentLead, ParentConversation).
type ParentRef struct {
	Kind    string `json:"kind"`
	ReplyID string `json:"reply_id"`
}

// QueuedInput is one drained dashboard input waiting for its turn: a
// message's text, or a slash command.
type QueuedInput struct {
	Text    string           `json:"text,omitempty"`
	Command *wsinbox.Command `json:"command,omitempty"`
	// after is the Runner.Send count the drains before the one that took
	// it covered (conv.drained); not persisted, as that count is not.
	after uint64
}

// workDir is the directory the conversation's process runs in.
func (rec Record) workDir() string {
	if rec.Cwd != "" {
		return rec.Cwd
	}
	return rec.Root
}

func recordPath(sessionDir string) string { return filepath.Join(sessionDir, recordFileName) }

// ReadRecord reads the conversation record of reply-id replyID from the
// dashboard state directory dir. ws-mcp reads it to recognise a
// conversation's own key (its tracker gate); the record is the daemon's,
// written only by the runner.
func ReadRecord(dir, replyID string) (Record, error) {
	if !wsdashboard.ValidReplyID(replyID) {
		return Record{}, fmt.Errorf("invalid reply-id %q", replyID)
	}
	return readRecord(wsdashboard.SessionDir(dir, replyID))
}

// WriteRecordForTest writes rec as reply-id rec.ReplyID's conversation
// record under dir, as the runner does. It exists for other packages' tests
// of ReadRecord callers; production code never writes a record outside the
// runner.
func WriteRecordForTest(dir string, rec Record) error {
	return writeRecord(wsdashboard.SessionDir(dir, rec.ReplyID), rec)
}

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
	if err := replaceFile(name, path); err != nil {
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

// newLiveWriter starts from the finished subagents of the snapshot an
// earlier daemon left in dir: the first write replaces that snapshot, and
// nothing else remembers them. A missing or unreadable one seeds nothing.
func newLiveWriter(dir, key, adapter string, now func() time.Time, log io.Writer) *liveWriter {
	st := wslive.NewState(key)
	st.SetAdapter(adapter)
	if prev, err := wslive.ReadSnapshot(wslive.SnapshotPath(dir)); err == nil {
		st.SeedFinished(prev)
		// The command list too: a session whose process is not running
		// reports it again only at its next turn (claude) or start (pi),
		// and the page offers commands only while the list is there.
		if len(prev.Commands) > 0 {
			_, _ = st.Apply(wslive.Event{Kind: wslive.KindCommands, Commands: prev.Commands}, now().UnixMilli())
		}
	}
	return &liveWriter{dir: dir, st: st, now: now, log: log}
}

// apply applies ev and writes what it changed; it reports whether anything
// was written.
func (w *liveWriter) apply(ev wslive.Event) bool {
	res, err := w.st.Apply(ev, w.now().UnixMilli())
	return w.write(res, err, ev.Kind)
}

// applyQuiet applies a replayed event to the state alone: its ring entries
// were written under an earlier daemon, and the snapshot is written once
// the replay ends.
func (w *liveWriter) applyQuiet(ev wslive.Event) {
	_, _ = w.st.Apply(ev, w.now().UnixMilli())
}

// card applies a tracker card reference the lead showed, as apply does.
func (w *liveWriter) card(ref wslive.CardRef) bool {
	res, err := w.st.Card(ref, w.now().UnixMilli())
	return w.write(res, err, wslive.RingCard)
}

// ui applies a dashboard UI command the lead sent, as apply does.
func (w *liveWriter) ui(cmd wslive.UICommand) bool {
	res, err := w.st.UI(cmd, w.now().UnixMilli())
	return w.write(res, err, wslive.RingUI)
}

// notice writes a typed failure line, as apply does.
func (w *liveWriter) notice(n wslive.Notice) bool {
	res, err := w.st.Notice(n, w.now().UnixMilli())
	return w.write(res, err, wslive.RingNotice)
}

// setHost replaces the snapshot's host state and, unless quiet, writes the
// snapshot when it changed, which it reports.
func (w *liveWriter) setHost(h *wslive.Host, quiet bool) bool {
	if !w.st.SetHost(h) {
		return false
	}
	if !quiet {
		w.writeSnapshot()
	}
	return true
}

// write writes what an applied change of kind brought; a refused one (err)
// is logged and dropped.
func (w *liveWriter) write(res wslive.Result, err error, kind string) bool {
	if err != nil {
		fmt.Fprintf(w.log, "agent %s: drop %s event: %v\n", w.st.ReplyID(), kind, err)
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

// endAgents marks every running subagent failed: the process that ran them
// is gone, so no end will come for them.
func (w *liveWriter) endAgents() {
	for _, a := range w.st.Snapshot(w.now().UnixMilli()).Agents {
		if a.Status == wslive.StatusRunning {
			w.apply(wslive.Event{Kind: wslive.KindAgent, AgentID: a.ID, Status: wslive.StatusFailed})
		}
	}
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
	if rec.Lead {
		// A lead session is an ordinary dashboard session: it has a session
		// record (its board, mail, and contacts are read as any lead's) and
		// is always listed. Only the writer of this row differs.
		reg.Agent, reg.Unlisted = false, false
		reg.Hosted, reg.HostPID = true, rec.HostPID
	}
	if rec.ClosedAt != "" {
		reg.EndedAt = rec.ClosedAt
	}
	return reg
}
