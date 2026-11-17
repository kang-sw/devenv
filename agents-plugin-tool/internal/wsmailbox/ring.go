package wsmailbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/gofrs/flock"
)

// ring.go is the memory store's restore backing: a segmented JSONL ring
// under <cache root>/mailbox-ring/, read only when an instance opens. Every
// segment opens with a snapshot record of the whole live state, written to
// a temp file and renamed in, so a segment never lacks its snapshot; the
// records after it are the deltas committed since (memstore.go). Restore
// reads the newest segment alone. A segment that has taken SegmentBytes of
// deltas rotates: the live state, reaped by today's lifetimes (state.reap),
// becomes the next segment's snapshot, and segments past SegmentCount are
// deleted oldest first. Live state is never dropped: each snapshot carries
// all of it, so the cap bounds the count of segments, and a live state
// larger than SegmentBytes only makes each segment larger (with a warning).
//
// Each record is one write() of one line to the newest segment, through a
// descriptor a persistent instance keeps open, before the call returns: a
// returned call survives a daemon crash. There is no fsync, so an OS crash
// may lose the tail, and a torn last line is cut at the next open.

const (
	// RingDirName is the ring's directory under the cache root.
	RingDirName = "mailbox-ring"
	// SegmentCount is the number of segments kept.
	SegmentCount = 10
	ringLockName = "lock"
)

// persistentLockWait bounds how long a persistent instance waits for the
// ring lock: a replacement daemon may start while its predecessor still
// exports and releases (a test seam).
var persistentLockWait = 30 * time.Second

// segmentBytes is the delta volume after which a segment rotates (a test
// seam; 1 MiB).
var segmentBytes int64 = 1 << 20

// ringWarn reports a ring condition worth a log line (a test seam).
var ringWarn = func(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ws-mcp mailbox: "+format+"\n", args...)
}

var segmentNamePattern = regexp.MustCompile(`^[0-9]{16}\.jsonl$`)

const (
	recordSnapshot = "snapshot"
	recordTx       = "tx"
)

// record is one ring line: a snapshot of the whole state, or the delta of
// one committed transaction (op names what made it: send, recv, presence,
// import, export, ...).
type record struct {
	Kind      string                      `json:"kind"`
	Op        string                      `json:"op,omitempty"`
	At        string                      `json:"at"`
	State     *state                      `json:"state,omitempty"`
	Named     map[string]*namedDelta      `json:"named,omitempty"`
	Reply     *replyDelta                 `json:"reply,omitempty"`
	History   map[string][]HistoryEntry   `json:"history,omitempty"`
	Listening map[string]*ListeningMarker `json:"listening,omitempty"`
	Baseline  *baseline                   `json:"baseline,omitempty"`
}

func (r *record) empty() bool {
	return len(r.Named) == 0 && r.Reply == nil && len(r.History) == 0 && len(r.Listening) == 0 && r.Baseline == nil
}

type ring struct {
	dir        string
	persistent bool
	lock       *flock.Flock
	file       *os.File // persistent only
	seq        uint64
	size       int64
	// sinceSnapshot is the delta bytes after the newest segment's snapshot.
	sinceSnapshot int64
	buf           bytes.Buffer
}

func segmentName(seq uint64) string { return fmt.Sprintf("%016d.jsonl", seq) }

// errRingHeld reports a ring lock another process holds past the wait.
var errRingHeld = errors.New("mailbox ring held")

// open locks the ring (waiting up to wait), loads the newest segment into
// st, and leaves the ring ready to append (creating the first segment when
// there is none).
func (r *ring) open(dir string, persistent bool, wait time.Duration, st *state) error {
	r.dir, r.persistent = dir, persistent
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create mailbox ring dir: %w", err)
	}
	r.lock = flock.New(filepath.Join(dir, ringLockName))
	var (
		locked bool
		err    error
	)
	if wait <= 0 {
		locked, err = r.lock.TryLock()
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), wait)
		locked, err = r.lock.TryLockContext(ctx, 20*time.Millisecond)
		cancel()
	}
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		r.lock = nil
		return fmt.Errorf("lock mailbox ring %s: %w", dir, err)
	}
	if !locked {
		r.lock = nil
		return fmt.Errorf("%w: %s is held by another process (the ws daemon); mailbox writes from a directly run ws-mcp binary are unsupported while the daemon runs", errRingHeld, dir)
	}
	// A snapshot temp file a crash left mid-rotation is never read.
	if stale, _ := filepath.Glob(filepath.Join(dir, ".segment-*")); len(stale) > 0 {
		for _, p := range stale {
			_ = os.Remove(p)
		}
	}
	seqs, err := r.segments()
	if err != nil {
		r.close()
		return err
	}
	*st = emptyState()
	newest := len(seqs) - 1
	loadedAt := -1
	for i := newest; i >= 0; i-- {
		loaded, size, sinceSnapshot, ok, err := loadSegment(filepath.Join(dir, segmentName(seqs[i])), true)
		if err != nil {
			r.close()
			return err
		}
		if ok {
			*st, r.seq, r.size, r.sinceSnapshot, loadedAt = loaded, seqs[i], size, sinceSnapshot, i
			break
		}
	}
	if loadedAt != newest || newest < 0 {
		// No ring yet, or a newer segment without a whole snapshot: start a
		// fresh segment past the newest from what loaded.
		if newest >= 0 {
			r.seq = seqs[newest]
		}
		if err := r.rotate(st); err != nil {
			r.close()
			return err
		}
		return nil
	}
	if persistent {
		f, err := os.OpenFile(filepath.Join(dir, segmentName(r.seq)), os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			r.close()
			return fmt.Errorf("open mailbox ring segment: %w", err)
		}
		r.file = f
	}
	return nil
}

// loadView loads the newest segment that opens with a whole snapshot into
// st without the ring lock (openView); no ring yet leaves st empty. A
// segment deleted by a concurrent rotation is skipped for an older one.
func loadView(dir string, st *state) error {
	r := ring{dir: dir}
	seqs, err := r.segments()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for i := len(seqs) - 1; i >= 0; i-- {
		loaded, _, _, ok, err := loadSegment(filepath.Join(dir, segmentName(seqs[i])), false)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if ok {
			*st = loaded
			return nil
		}
	}
	return nil
}

// segments lists the ring's segment numbers, oldest first.
func (r *ring) segments() ([]uint64, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, fmt.Errorf("list mailbox ring: %w", err)
	}
	var seqs []uint64
	for _, e := range entries {
		if !segmentNamePattern.MatchString(e.Name()) {
			continue
		}
		var seq uint64
		fmt.Sscanf(e.Name(), "%016d.jsonl", &seq)
		seqs = append(seqs, seq)
	}
	sort.Slice(seqs, func(i, j int) bool { return seqs[i] < seqs[j] })
	return seqs, nil
}

// loadSegment replays one segment: its opening snapshot, then each delta.
// ok is false when the segment does not open with a whole snapshot. A torn
// last line (a crash mid-write) is ignored, and cut off the file when cut
// is set (the lock holder); any other line that does not parse is skipped
// with a warning.
func loadSegment(path string, cut bool) (st state, size, sinceSnapshot int64, ok bool, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return state{}, 0, 0, false, fmt.Errorf("read mailbox ring segment: %w", err)
	}
	if end := bytes.LastIndexByte(raw, '\n'); end+1 != len(raw) {
		raw = raw[:end+1]
		if cut {
			if terr := os.Truncate(path, int64(len(raw))); terr != nil {
				return state{}, 0, 0, false, fmt.Errorf("cut torn mailbox ring record: %w", terr)
			}
			ringWarn("cut a torn record off %s", path)
		}
	}
	first, rest, _ := bytes.Cut(raw, []byte{'\n'})
	var snap record
	if json.Unmarshal(first, &snap) != nil || snap.Kind != recordSnapshot || snap.State == nil {
		return state{}, 0, 0, false, nil
	}
	st = *snap.State
	st.fill()
	skipped := 0
	for len(rest) > 0 {
		var line []byte
		line, rest, _ = bytes.Cut(rest, []byte{'\n'})
		var rec record
		if json.Unmarshal(line, &rec) != nil || rec.Kind != recordTx {
			skipped++
			continue
		}
		st.apply(&rec)
	}
	if skipped > 0 {
		ringWarn("skipped %d unreadable records in %s", skipped, path)
	}
	return st, int64(len(raw)), int64(len(raw) - len(first) - 1), true, nil
}

// fill makes a decoded state's maps non-nil.
func (st *state) fill() {
	if st.Named == nil {
		st.Named = map[string]StoreFile{}
	}
	for path, s := range st.Named {
		st.Named[path] = cloneStore(s, false)
	}
	if st.Reply.Entries == nil {
		st.Reply.Entries = map[string]ReplyEntry{}
	}
	if st.Reply.Queues == nil {
		st.Reply.Queues = map[string][]Envelope{}
	}
	if st.History == nil {
		st.History = map[string][]HistoryEntry{}
	}
	if st.Listening == nil {
		st.Listening = map[string]ListeningMarker{}
	}
	st.Baseline.fill()
}

func (r *ring) encode(rec *record) error {
	r.buf.Reset()
	enc := json.NewEncoder(&r.buf)
	enc.SetEscapeHTML(false)
	return enc.Encode(rec)
}

// append writes rec as one line in one write() to the newest segment.
func (r *ring) append(rec *record) error {
	if err := r.encode(rec); err != nil {
		return fmt.Errorf("encode mailbox ring record: %w", err)
	}
	f := r.file
	if f == nil {
		var err error
		if f, err = os.OpenFile(filepath.Join(r.dir, segmentName(r.seq)), os.O_WRONLY|os.O_APPEND, 0o644); err != nil {
			return fmt.Errorf("open mailbox ring segment: %w", err)
		}
		defer f.Close()
	}
	n, err := f.Write(r.buf.Bytes())
	if err != nil {
		if n > 0 {
			// Cut the partial line so the next record starts on its own.
			_ = f.Truncate(r.size)
		}
		return fmt.Errorf("append mailbox ring record: %w", err)
	}
	r.size += int64(n)
	r.sinceSnapshot += int64(n)
	return nil
}

// rotate starts the next segment with a snapshot of st, renamed in whole,
// switches appends to it, and deletes segments past SegmentCount.
func (r *ring) rotate(st *state) error {
	if err := r.encode(&record{Kind: recordSnapshot, At: storeNow().Format(time.RFC3339Nano), State: st}); err != nil {
		return fmt.Errorf("encode mailbox ring snapshot: %w", err)
	}
	if int64(r.buf.Len()) > segmentBytes {
		ringWarn("the live mailbox state (%d bytes) outgrows a ring segment (%d bytes); segments grow past it", r.buf.Len(), segmentBytes)
	}
	tmp, err := os.CreateTemp(r.dir, ".segment-*")
	if err != nil {
		return fmt.Errorf("create mailbox ring segment: %w", err)
	}
	if _, err := tmp.Write(r.buf.Bytes()); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return fmt.Errorf("write mailbox ring snapshot: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("write mailbox ring snapshot: %w", err)
	}
	next := r.seq + 1
	path := filepath.Join(r.dir, segmentName(next))
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("commit mailbox ring segment: %w", err)
	}
	if r.persistent {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			// Appends stay on the current segment, so the new one must not
			// be the newest a restore reads.
			_ = os.Remove(path)
			return fmt.Errorf("open mailbox ring segment: %w", err)
		}
		if r.file != nil {
			r.file.Close()
		}
		r.file = f
	}
	r.seq, r.size, r.sinceSnapshot = next, int64(r.buf.Len()), 0
	if seqs, err := r.segments(); err == nil {
		for i := 0; i < len(seqs)-SegmentCount; i++ {
			_ = os.Remove(filepath.Join(r.dir, segmentName(seqs[i])))
		}
	}
	return nil
}

func (r *ring) close() error {
	var err error
	if r.file != nil {
		err = r.file.Close()
		r.file = nil
	}
	if r.lock != nil {
		if uerr := r.lock.Unlock(); err == nil {
			err = uerr
		}
		r.lock = nil
	}
	return err
}

// maybeRotate rotates once the newest segment has taken segmentBytes of
// deltas. It runs inline after the append that crossed the size, once per
// segment; a failure is logged and retried after the next append.
func (in *instance) maybeRotate() {
	if in.ring.sinceSnapshot < segmentBytes {
		return
	}
	in.st.reap(storeNow())
	if err := in.ring.rotate(&in.st); err != nil {
		ringWarn("rotate mailbox ring: %v", err)
	}
}
