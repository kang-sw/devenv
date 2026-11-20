package wslive

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sync"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// OutputRateName is the additive session-local rate record file, separate from
// snapshot.json (which older typed writers replace, dropping unknown fields)
// and separate from model-evidence (which stores answering-model observations
// only). Older daemons never read, write, or prune it.
const OutputRateName = "output-rate.json"

const (
	outputRatePreviousName = "output-rate.previous.json"
	outputRateSegmentBytes = int64(64 * 1024)
)

// OutputRatePath is the output-rate sidecar file in sessionDir.
func OutputRatePath(sessionDir string) string {
	return filepath.Join(sessionDir, OutputRateName)
}

// OutputRate is a session's completed-response output rate independent of
// answering-model evidence (ModelObservation). It carries no observed model
// and no observation source.
type OutputRate struct {
	OutputTps float64 `json:"outputTps"`
	At        int64   `json:"at"`
}

func cleanOutputRate(r *OutputRate) *OutputRate {
	if r == nil || r.At <= 0 {
		return nil
	}
	if math.IsNaN(r.OutputTps) || math.IsInf(r.OutputTps, 0) || r.OutputTps <= 0 || r.OutputTps > MaxOutputTps {
		return nil
	}
	v := *r
	return &v
}

func cloneOutputRate(r *OutputRate) *OutputRate {
	if r == nil {
		return nil
	}
	v := *r
	return &v
}

// SeedOutputRate restores durable output-rate state without creating activity.
// A newer rate already held in memory wins over an older persisted value.
func (st *State) SeedOutputRate(r *OutputRate) {
	cleaned := cleanOutputRate(r)
	if cleaned == nil {
		return
	}
	if st.outputRate != nil && cleaned.At < st.outputRate.At {
		return
	}
	st.outputRate = cleaned
}

func (st *State) setOutputRate(r *OutputRate) (changed bool, persist *OutputRate) {
	cleaned := cleanOutputRate(r)
	if cleaned == nil {
		return false, nil
	}
	if st.outputRate != nil && cleaned.At < st.outputRate.At {
		return false, nil
	}
	if st.outputRate != nil && *st.outputRate == *cleaned {
		return false, nil
	}
	st.outputRate = cleaned
	return true, cloneOutputRate(cleaned)
}

// OverlayOutputRate restores retained output-rate state onto snap after a
// legacy snapshot rewrite. A newer rate already present in snap wins.
func OverlayOutputRate(snap *Snapshot, rate *OutputRate) {
	if snap == nil {
		return
	}
	cleaned := cleanOutputRate(rate)
	if cleaned == nil {
		return
	}
	if snap.Lead.OutputRate == nil || cleaned.At >= snap.Lead.OutputRate.At {
		snap.Lead.OutputRate = cloneOutputRate(cleaned)
	}
}

// OutputRateLog is the held append handle for one session's additive rate
// recovery log. Its zero value needs NewOutputRateLog before Append.
type OutputRateLog struct {
	mu       sync.Mutex
	dir      string
	file     *os.File
	bytes    int64
	maxBytes int64 // test seam; zero uses outputRateSegmentBytes
}

// NewOutputRateLog returns a lazy log: construction performs no file I/O, and
// the first eligible recovery flush opens the file for the log's lifetime.
func NewOutputRateLog(dir string) *OutputRateLog { return &OutputRateLog{dir: dir} }

// Append writes one complete delta. A leading newline isolates this record
// from a torn prior tail, and the next append does the same if this write tears.
func (l *OutputRateLog) Append(rate *OutputRate) error {
	cleaned := cleanOutputRate(rate)
	if l == nil || cleaned == nil {
		return nil
	}
	data, err := json.Marshal(cleaned)
	if err != nil {
		return err
	}
	line := make([]byte, 0, len(data)+2)
	line = append(line, '\n')
	line = append(line, data...)
	line = append(line, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.ensureOpen(); err != nil {
		return err
	}
	limit := l.maxBytes
	if limit <= 0 {
		limit = outputRateSegmentBytes
	}
	if l.bytes > 0 && l.bytes+int64(len(line)) > limit {
		if err := l.rotate(); err != nil {
			return err
		}
	}
	n, err := l.file.Write(line)
	l.bytes += int64(n)
	return err
}

func (l *OutputRateLog) ensureOpen() error {
	if l.file != nil {
		return nil
	}
	if err := os.MkdirAll(l.dir, 0700); err != nil {
		return err
	}
	f, err := wsstate.OpenFile(OutputRatePath(l.dir), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	l.file = f
	if info, statErr := f.Stat(); statErr == nil {
		l.bytes = info.Size()
	}
	return nil
}

// rotate retains one prior whole segment. Every record is a full latest-value
// snapshot, so either segment alone is sufficient for recovery.
func (l *OutputRateLog) rotate() error {
	if err := l.file.Close(); err != nil {
		return err
	}
	l.file = nil
	active := OutputRatePath(l.dir)
	previous := filepath.Join(l.dir, outputRatePreviousName)
	if err := wsstate.AtomicReplaceFile(active, previous); err != nil {
		return err
	}
	l.bytes = 0
	return l.ensureOpen()
}

// Close releases the held append handle.
func (l *OutputRateLog) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	l.bytes = 0
	return err
}

// WriteOutputRate is the one-shot fixture/maintenance form of OutputRateLog.
// Production writers retain one log and append only on recovery flushes.
func WriteOutputRate(dir string, rate *OutputRate) error {
	log := NewOutputRateLog(dir)
	if err := log.Append(rate); err != nil {
		_ = log.Close()
		return err
	}
	return log.Close()
}

// ReadOutputRate reads the latest valid delta from the session's additive
// sidecar, tolerating missing files, blank lines and a torn final append.
func ReadOutputRate(dir string) (*OutputRate, error) {
	var latest *OutputRate
	for _, path := range []string{filepath.Join(dir, outputRatePreviousName), OutputRatePath(dir)} {
		raw, err := readReplacedFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, line := range bytes.Split(raw, []byte{'\n'}) {
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}
			var r OutputRate
			if json.Unmarshal(line, &r) != nil {
				continue
			}
			cleaned := cleanOutputRate(&r)
			if cleaned != nil && (latest == nil || cleaned.At >= latest.At) {
				latest = cleaned
			}
		}
	}
	return latest, nil
}

// OutputRateCache reads each sidecar at most once between explicit invalidation
// signals. Ordinary snapshots carry OutputRate themselves, so this cache is a
// recovery overlay only after a legacy snapshot rewrite, never a polling path.
type OutputRateCache struct {
	mu       sync.Mutex
	sessions map[string]*outputRateSlot
}

type outputRateSlot struct {
	rate *OutputRate
}

// Load returns the session's persisted OutputRate, or nil when absent/invalid.
func (c *OutputRateCache) Load(dir string) *OutputRate {
	if c == nil {
		r, _ := ReadOutputRate(dir)
		return r
	}
	c.mu.Lock()
	if slot, ok := c.sessions[dir]; ok {
		rate := cloneOutputRate(slot.rate)
		c.mu.Unlock()
		return rate
	}
	c.mu.Unlock()
	rate, _ := ReadOutputRate(dir)
	c.mu.Lock()
	if c.sessions == nil {
		c.sessions = map[string]*outputRateSlot{}
	}
	if len(c.sessions) >= observedSessionCap {
		for old := range c.sessions {
			delete(c.sessions, old)
			break
		}
	}
	c.sessions[dir] = &outputRateSlot{rate: cloneOutputRate(rate)}
	c.mu.Unlock()
	return cloneOutputRate(rate)
}

// Invalidate makes the next recovery overlay reload dir.
func (c *OutputRateCache) Invalidate(dir string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.sessions, dir)
	c.mu.Unlock()
}

// Forget drops cached output-rate state for dir.
func (c *OutputRateCache) Forget(dir string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.sessions, dir)
	c.mu.Unlock()
}
