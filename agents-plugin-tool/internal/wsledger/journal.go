package wsledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// The journal is this clone's durable, append-only record of every event it
// wrote, one JSON object per line:
//
//	{"k":"open","seen":{...}}   the seen of the open batch; it applies to every
//	                            event not yet in a commit
//	{"k":"ev","seq":N,"ts":"...","type":"...","origin":"...","payload":...,"of":M}
//	                            one event; "of" names the seq this event
//	                            re-issues under a new key
//
// An event is pending while its seq is above the highest seq on this clone's
// own ref; the ref, not the journal, says what was committed, so a crash
// between a ref update and a journal write loses nothing. Events stay in the
// journal after their batch closes and is pushed: re-issue reads them.

type journalRec struct {
	K       string          `json:"k"`
	Seen    map[string]int  `json:"seen,omitempty"`
	Seq     int             `json:"seq,omitempty"`
	TS      *time.Time      `json:"ts,omitempty"`
	Type    string          `json:"type,omitempty"`
	Origin  string          `json:"origin,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Of      int             `json:"of,omitempty"`
}

type journalEvent struct {
	Seq     int
	TS      time.Time
	Type    string
	Origin  string
	Payload json.RawMessage
	Of      int
}

type journal struct {
	path      string
	offset    int64
	events    []journalEvent // by seq
	bySeq     map[int]int    // seq -> index in events
	batchSeen map[string]int // seen of the latest open record
	reissued  map[int]bool   // seqs some later event re-issues
}

func newJournal(path string) *journal {
	return &journal{path: path, bySeq: map[int]int{}, reissued: map[int]bool{}}
}

// reload reads records appended since the last read. A torn last line (a
// crash mid-write) is cut off so the next append starts on a clean line.
func (j *journal) reload() error {
	f, err := os.OpenFile(j.path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() < j.offset {
		// The file shrank under us: start over.
		*j = *newJournal(j.path)
	}
	if _, err := f.Seek(j.offset, io.SeekStart); err != nil {
		return err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	end := bytes.LastIndexByte(data, '\n') + 1
	if end < len(data) {
		if err := f.Truncate(j.offset + int64(end)); err != nil {
			return err
		}
	}
	for _, line := range bytes.Split(data[:end], []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var r journalRec
		if err := json.Unmarshal(line, &r); err != nil {
			return fmt.Errorf("ledger journal %s: %w", j.path, err)
		}
		j.absorb(r)
	}
	j.offset += int64(end)
	return nil
}

func (j *journal) absorb(r journalRec) {
	switch r.K {
	case "open":
		j.batchSeen = r.Seen
		if j.batchSeen == nil {
			j.batchSeen = map[string]int{}
		}
	case "ev":
		if _, dup := j.bySeq[r.Seq]; dup || r.Seq <= 0 {
			return
		}
		e := journalEvent{Seq: r.Seq, Type: r.Type, Origin: r.Origin, Payload: r.Payload, Of: r.Of}
		if r.TS != nil {
			e.TS = *r.TS
		}
		j.bySeq[r.Seq] = len(j.events)
		j.events = append(j.events, e)
		if r.Of > 0 {
			j.reissued[r.Of] = true
		}
	}
}

// write appends records durably and absorbs them.
func (j *journal) write(recs ...journalRec) error {
	var buf bytes.Buffer
	for _, r := range recs {
		line, err := json.Marshal(r)
		if err != nil {
			return err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	f, err := os.OpenFile(j.path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// Absorb through reload so offset stays exact even when another
	// process appended in between (the caller holds the store lock, so it
	// cannot have).
	return j.reload()
}

func (j *journal) maxSeq() int {
	m := 0
	for _, e := range j.events {
		m = max(m, e.Seq)
	}
	return m
}

// pending returns the events above committed, by seq.
func (j *journal) pending(committed int) []journalEvent {
	var out []journalEvent
	for _, e := range j.events {
		if e.Seq > committed {
			out = append(out, e)
		}
	}
	return out
}

func (j *journal) event(seq int) (journalEvent, bool) {
	i, ok := j.bySeq[seq]
	if !ok {
		return journalEvent{}, false
	}
	return j.events[i], true
}

var errJournalGap = errors.New("ledger journal: pending events are not continuous with the own ref")
