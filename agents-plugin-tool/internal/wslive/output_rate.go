package wslive

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// OutputRateName is the additive session-local rate record file, separate from
// snapshot.json (which older typed writers replace, dropping unknown fields)
// and separate from model-evidence (which stores answering-model observations
// only). Older daemons never read, write, or prune it.
const OutputRateName = "output-rate.json"

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

// WriteOutputRate atomically writes the session's additive output-rate sidecar.
// Invalid or nil rates write nothing.
func WriteOutputRate(dir string, rate *OutputRate) error {
	cleaned := cleanOutputRate(rate)
	if cleaned == nil {
		return nil
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(cleaned)
	if err != nil {
		return err
	}
	tmp, err := wsstate.CreateTemp(dir, ".output-rate-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	err = tmp.Chmod(0600)
	if err == nil {
		_, err = tmp.Write(data)
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = wsstate.AtomicReplaceFile(name, OutputRatePath(dir))
	}
	if err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("output rate: %w", err)
	}
	return nil
}

// ReadOutputRate reads the session's additive output-rate sidecar, tolerating
// missing, truncated, or invalid files as absent.
func ReadOutputRate(dir string) (*OutputRate, error) {
	raw, err := readReplacedFile(OutputRatePath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) > 4096 {
		return nil, nil
	}
	var r OutputRate
	if json.Unmarshal(raw, &r) != nil {
		return nil, nil
	}
	return cleanOutputRate(&r), nil
}

const outputRateValidationInterval = time.Second

// OutputRateCache caches decoded per-session OutputRate sidecars, sharing
// directory/file validation for one second and negatively caching absent files
// so sessions without output-rate.json avoid repeated opens. Zero value is ready.
type OutputRateCache struct {
	mu       sync.Mutex
	sessions map[string]*outputRateSlot
	tick     uint64
	stat     func(string) (os.FileInfo, error)
	now      func() time.Time
}

type outputRateSlot struct {
	used      uint64
	info      os.FileInfo
	rate      *OutputRate
	checkedAt time.Time
	checked   bool
}

// Load returns the session's persisted OutputRate, or nil when absent/invalid.
func (c *OutputRateCache) Load(dir string) *OutputRate {
	if c == nil {
		r, _ := ReadOutputRate(dir)
		return r
	}
	clock := time.Now
	if c.now != nil {
		clock = c.now
	}
	now := clock()
	c.mu.Lock()
	slot := c.sessions[dir]
	if slot != nil {
		c.tick++
		slot.used = c.tick
		if elapsed := now.Sub(slot.checkedAt); slot.checked && elapsed >= 0 && elapsed < outputRateValidationInterval {
			rate := slot.rate
			c.mu.Unlock()
			return rate
		}
		prevInfo, prevRate := slot.info, slot.rate
		c.mu.Unlock()
		return c.refresh(dir, prevInfo, prevRate, now)
	}
	c.mu.Unlock()
	return c.refresh(dir, nil, nil, now)
}

func (c *OutputRateCache) refresh(dir string, prevInfo os.FileInfo, prevRate *OutputRate, now time.Time) *OutputRate {
	stat := c.stat
	if stat == nil {
		stat = wsstate.Stat
	}
	path := OutputRatePath(dir)
	info, err := stat(path)
	var rate *OutputRate
	if err != nil {
		info = nil
		rate = nil
	} else if prevInfo != nil && wsstate.SameFileMetadata(prevInfo, info) {
		rate = prevRate
	} else {
		r, readErr := ReadOutputRate(dir)
		if readErr != nil {
			info = nil
			rate = nil
		} else {
			rate = r
		}
	}
	c.mu.Lock()
	if c.sessions == nil {
		c.sessions = map[string]*outputRateSlot{}
	}
	if _, exists := c.sessions[dir]; !exists && len(c.sessions) >= observedSessionCap {
		oldest, oldestUsed, found := "", uint64(0), false
		for d, s := range c.sessions {
			if !found || s.used < oldestUsed {
				oldest, oldestUsed, found = d, s.used, true
			}
		}
		delete(c.sessions, oldest)
	}
	c.tick++
	c.sessions[dir] = &outputRateSlot{
		used:      c.tick,
		info:      info,
		rate:      rate,
		checkedAt: now,
		checked:   true,
	}
	c.mu.Unlock()
	return rate
}

// Invalidate expires validation for dir while keeping the last decoded rate.
func (c *OutputRateCache) Invalidate(dir string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if slot := c.sessions[dir]; slot != nil {
		slot.checked = false
	}
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
