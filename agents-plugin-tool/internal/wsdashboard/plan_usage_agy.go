package wsdashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/execjob"
)

const (
	planUsageAgyCache        = "plan-usage-agy.json"
	planUsageAgyHistoryCache = "plan-usage-agy-history.json"
	agyUsageSource           = "via agy CLI"
	agyStatusSignInRequired  = "sign-in required"
	agyAuthRequiredMarker    = "Authentication required. Please visit the URL"
	agyDefaultDebounce       = 10 * time.Minute
	agyDefaultTimeout        = 30 * time.Second
	agyPrintTimeoutArg       = "20s"
	agyMaxOutputBytes        = 512 << 10
)

type agyGroupSpec struct {
	key   string
	label string
	order int
}

// Known agy model groups mapped to fixed ASCII keys and downstream-neutral
// labels. Raw subprocess group names never enter cache files or history.
var agyKnownGroups = map[string]agyGroupSpec{
	"gemini models":         {key: "gemini", label: "Gemini Models", order: 0},
	"claude and gpt models": {key: "claude_gpt", label: "Claude and GPT models", order: 1},
}

var agyGroupByKey = map[string]agyGroupSpec{
	"gemini":     {key: "gemini", label: "Gemini Models", order: 0},
	"claude_gpt": {key: "claude_gpt", label: "Claude and GPT models", order: 1},
}

var agyHistorySpec = planUsageHistorySpec{
	cacheName:    planUsageAgyHistoryCache,
	valid:        validAgyPlanUsageSample,
	requiredKeys: []string{"harness", "group", "kind", "source", "resetsAt", "asOf", "percent"},
}

func validAgyPlanUsageSample(v planUsageSample) bool {
	if _, ok := agyGroupByKey[v.Group]; !ok {
		return false
	}
	kindOK := v.Kind == "five_hour" || v.Kind == "seven_day"
	return v.Harness == "agy" && v.Source == agyUsageSource && kindOK && v.AsOf > 0 && v.ResetsAt >= 0 && validUsage(v.Percent)
}

type planUsageAgyFile struct {
	SchemaVersion int              `json:"schema_version"`
	Groups        []planUsageEntry `json:"groups"`
}

func (s *planUsageStore) agyDebounceInterval() time.Duration {
	if s.agyDebounce > 0 {
		return s.agyDebounce
	}
	return agyDefaultDebounce
}

func (s *planUsageStore) agyProbeTimeout() time.Duration {
	if s.agyTimeout > 0 {
		return s.agyTimeout
	}
	return agyDefaultTimeout
}

func (s *planUsageStore) loadAgyLocked() {
	raw, err := os.ReadFile(filepath.Join(s.dir, planUsageAgyCache))
	if err != nil {
		return
	}
	var saved planUsageAgyFile
	if json.Unmarshal(raw, &saved) != nil || saved.SchemaVersion != 1 {
		return
	}
	groups := make([]planUsageEntry, 0, len(saved.Groups))
	seen := map[string]bool{}
	for _, g := range saved.Groups {
		spec, ok := agyGroupByKey[g.Group]
		if !ok || seen[spec.key] {
			continue
		}
		windows := make([]planUsageWindow, 0, len(g.Windows))
		for _, w := range g.Windows {
			if (w.Kind != "five_hour" && w.Kind != "seven_day") || !validUsage(w.Percent) || w.ResetsAt < 0 {
				continue
			}
			windows = append(windows, planUsageWindow{
				Kind:     w.Kind,
				Percent:  w.Percent,
				ResetsAt: w.ResetsAt,
				AsOf:     w.AsOf,
				Disabled: w.Disabled,
			})
		}
		if len(windows) == 0 {
			continue
		}
		seen[spec.key] = true
		sortAgyWindows(windows)
		groups = append(groups, planUsageEntry{
			Harness: "agy",
			Group:   spec.key,
			Label:   spec.label,
			Source:  agyUsageSource,
			AsOf:    g.AsOf,
			Windows: windows,
		})
	}
	if len(groups) == 0 {
		return
	}
	sortAgyGroups(groups)
	s.agyGroups = groups
	if canonical, err := json.Marshal(planUsageAgyFile{SchemaVersion: 1, Groups: groups}); err == nil && bytes.Equal(canonical, raw) {
		s.agyLastSaved = canonical
	}
}

func (s *planUsageStore) saveAgyLocked() {
	raw, err := json.Marshal(planUsageAgyFile{SchemaVersion: 1, Groups: s.agyGroups})
	if err == nil && !bytes.Equal(raw, s.agyLastSaved) {
		if s.write(planUsageAgyCache, raw) == nil {
			s.agyLastSaved = raw
		}
	}
}

func (s *planUsageStore) agyEntriesLocked(now int64) []planUsageEntry {
	if len(s.agyGroups) > 0 {
		out := make([]planUsageEntry, 0, len(s.agyGroups))
		for _, g := range s.agyGroups {
			e := g
			e.Harness = "agy"
			if s.agyStatus != "" {
				e.Status = s.agyStatus
			}
			out = append(out, currentUsage(e, now))
		}
		return out
	}
	if s.agyProbed {
		status := s.agyStatus
		if status == "" {
			status = "usage unavailable"
		}
		return []planUsageEntry{{
			Harness: "agy",
			Source:  agyUsageSource,
			Status:  status,
			Windows: []planUsageWindow{},
		}}
	}
	return nil
}

// refreshAgy schedules at most one asynchronous agy CLI /usage probe on a
// user-triggered popover refresh, subject to the process-lifetime latches and
// ~10-minute debounce. Never blocks the HTTP handler.
func (s *planUsageStore) refreshAgy() {
	s.mu.Lock()
	if s.closed || s.resolveAgy == nil || s.agyProbing || s.agyBilledLatch || s.agyAuthHold {
		s.mu.Unlock()
		return
	}
	if !s.agyLastProbe.IsZero() && s.nowTime().Sub(s.agyLastProbe) < s.agyDebounceInterval() {
		s.mu.Unlock()
		return
	}
	resolve := s.resolveAgy
	parentCtx := s.lifetimeCtx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	s.agyProbing = true
	s.probes.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.probes.Done()
		s.probeAgy(parentCtx, resolve)
	}()
}

func (s *planUsageStore) probeAgy(parentCtx context.Context, resolve func() (string, []string, error)) {
	defer func() {
		s.mu.Lock()
		s.agyProbing = false
		s.mu.Unlock()
	}()
	if parentCtx.Err() != nil {
		return
	}
	prog, pre, err := resolve()
	if err != nil || strings.TrimSpace(prog) == "" {
		// Binary absent: skip silently with no entry and no debounce penalty.
		return
	}
	// The debounce window starts when the probe is spawned, regardless of outcome.
	s.mu.Lock()
	if s.closed || parentCtx.Err() != nil || s.agyBilledLatch || s.agyAuthHold {
		s.mu.Unlock()
		return
	}
	s.agyLastProbe = s.nowTime()
	timeout := s.agyProbeTimeout()
	baseEnv := s.agyEnv
	s.mu.Unlock()

	stdout, stderr, stdoutOverflow, stderrOverflow, cmdErr := runAgyUsageCommand(parentCtx, prog, pre, baseEnv, timeout)
	if parentCtx.Err() != nil {
		return
	}
	at := s.nowTime().UnixMilli()
	outcome, groups := parseAgyUsageOutput(stdout, stderr, stdoutOverflow, stderrOverflow, cmdErr, at)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || parentCtx.Err() != nil {
		return
	}
	s.agyProbed = true
	switch outcome {
	case agyOutcomeOK:
		s.agyGroups = groups
		s.agyStatus = ""
		s.saveAgyLocked()
		var samples []planUsageSample
		for _, g := range groups {
			samples = append(samples, usageSamples(g)...)
		}
		s.appendHistoryFileLocked(&s.agyHist, agyHistorySpec, samples, at)
	case agyOutcomeAuthHold:
		s.agyAuthHold = true
		s.agyStatus = agyStatusSignInRequired
	case agyOutcomeBilledLatch:
		s.agyBilledLatch = true
		s.agyStatus = "usage unavailable"
	default:
		s.agyStatus = "usage unavailable"
	}
}

type agyProbeOutcome int

const (
	agyOutcomeUnavailable agyProbeOutcome = iota
	agyOutcomeOK
	agyOutcomeAuthHold
	agyOutcomeBilledLatch
)

type agyUsageEnvelope struct {
	Status         string           `json:"status"`
	NumTurns       int              `json:"num_turns"`
	ConversationID string           `json:"conversation_id"`
	Command        *agyUsageCommand `json:"command"`
}

type agyUsageCommand struct {
	Name string        `json:"name"`
	Data *agyUsageData `json:"data"`
}

type agyUsageData struct {
	Groups []json.RawMessage `json:"groups"`
}

type agyUsageGroup struct {
	Name    string            `json:"name"`
	Buckets []json.RawMessage `json:"buckets"`
}

func parseAgyUsageOutput(stdout, stderr []byte, stdoutOverflow, stderrOverflow bool, cmdErr error, at int64) (agyProbeOutcome, []planUsageEntry) {
	if bytes.Contains(stderr, []byte(agyAuthRequiredMarker)) {
		return agyOutcomeAuthHold, nil
	}
	if stdoutOverflow {
		// Truncated stdout cannot prove the turn was unbilled; latch conservatively.
		return agyOutcomeBilledLatch, nil
	}
	var env agyUsageEnvelope
	parsed := len(bytes.TrimSpace(stdout)) > 0 && json.Unmarshal(stdout, &env) == nil
	if parsed && (env.NumTurns > 0 || strings.TrimSpace(env.ConversationID) != "") {
		return agyOutcomeBilledLatch, nil
	}
	if stderrOverflow || cmdErr != nil || !parsed {
		return agyOutcomeUnavailable, nil
	}
	if env.Status != "" && env.Status != "SUCCESS" {
		return agyOutcomeUnavailable, nil
	}
	if env.Command == nil || env.Command.Name != "usage" || env.Command.Data == nil || len(env.Command.Data.Groups) == 0 {
		return agyOutcomeUnavailable, nil
	}
	groups := make([]planUsageEntry, 0, len(env.Command.Data.Groups))
	seenGroup := map[string]bool{}
	for _, rawGroup := range env.Command.Data.Groups {
		var g agyUsageGroup
		if json.Unmarshal(rawGroup, &g) != nil {
			continue
		}
		spec, ok := agyKnownGroups[strings.ToLower(strings.TrimSpace(g.Name))]
		if !ok || seenGroup[spec.key] {
			continue
		}
		windows := make([]planUsageWindow, 0, len(g.Buckets))
		for _, rawBucket := range g.Buckets {
			if w, ok := parseAgyBucket(rawBucket, at); ok {
				windows = append(windows, w)
			}
		}
		if len(windows) == 0 {
			continue
		}
		seenGroup[spec.key] = true
		sortAgyWindows(windows)
		groups = append(groups, planUsageEntry{
			Harness: "agy",
			Group:   spec.key,
			Label:   spec.label,
			Source:  agyUsageSource,
			AsOf:    at,
			Windows: windows,
		})
	}
	if len(groups) == 0 {
		return agyOutcomeUnavailable, nil
	}
	sortAgyGroups(groups)
	return agyOutcomeOK, groups
}

func parseAgyBucket(raw json.RawMessage, at int64) (planUsageWindow, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return planUsageWindow{}, false
	}
	var window string
	if json.Unmarshal(fields["window"], &window) != nil {
		return planUsageWindow{}, false
	}
	var kind string
	switch window {
	case "5h":
		kind = "five_hour"
	case "weekly":
		kind = "seven_day"
	default:
		return planUsageWindow{}, false
	}
	rawFrac, ok := fields["remaining_fraction"]
	if !ok || string(bytes.TrimSpace(rawFrac)) == "null" {
		return planUsageWindow{}, false
	}
	var frac float64
	if json.Unmarshal(rawFrac, &frac) != nil || math.IsNaN(frac) || math.IsInf(frac, 0) {
		return planUsageWindow{}, false
	}
	pct := (1 - frac) * 100
	if pct < 0 {
		pct = 0
	} else if pct > 100 {
		pct = 100
	}
	pct = math.Round(pct*1e6) / 1e6
	resetsAt, ok := parseAgyResetTime(fields["reset_time"])
	if !ok {
		return planUsageWindow{}, false
	}
	var disabled bool
	if rawDisabled, has := fields["disabled"]; has && string(bytes.TrimSpace(rawDisabled)) != "null" {
		if json.Unmarshal(rawDisabled, &disabled) != nil {
			return planUsageWindow{}, false
		}
	}
	return planUsageWindow{
		Kind:     kind,
		Percent:  pct,
		ResetsAt: resetsAt,
		AsOf:     at,
		Disabled: disabled,
	}, true
}

func parseAgyResetTime(raw json.RawMessage) (int64, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return 0, false
	}
	if trimmed[0] == '"' {
		var text string
		if json.Unmarshal(trimmed, &text) != nil || text == "" {
			return 0, false
		}
		t, err := time.Parse(time.RFC3339Nano, text)
		if err != nil || t.UnixMilli() <= 0 {
			return 0, false
		}
		return t.UnixMilli(), true
	}
	var num float64
	if json.Unmarshal(trimmed, &num) == nil && num > 0 && !math.IsNaN(num) && !math.IsInf(num, 0) {
		if num < 1e12 {
			num *= 1000
		}
		return int64(num), true
	}
	return 0, false
}

func sortAgyGroups(groups []planUsageEntry) {
	sort.SliceStable(groups, func(i, j int) bool {
		return agyGroupByKey[groups[i].Group].order < agyGroupByKey[groups[j].Group].order
	})
}

func sortAgyWindows(windows []planUsageWindow) {
	order := func(kind string) int {
		if kind == "five_hour" {
			return 0
		}
		return 1
	}
	sort.SliceStable(windows, func(i, j int) bool {
		return order(windows[i].Kind) < order(windows[j].Kind)
	})
}

// cappedBuffer enforces a per-stream byte cap, cancelling the subprocess as
// soon as that stream exceeds its budget while retaining bytes read up to the
// cap for auth-marker and latch inspection.
type cappedBuffer struct {
	mu         sync.Mutex
	remaining  int
	overflowed bool
	onOverflow func()
	buf        bytes.Buffer
}

func (w *cappedBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	if w.overflowed {
		w.mu.Unlock()
		return len(p), nil
	}
	if len(p) > w.remaining {
		if w.remaining > 0 {
			_, _ = w.buf.Write(p[:w.remaining])
			w.remaining = 0
		}
		w.overflowed = true
		cb := w.onOverflow
		w.mu.Unlock()
		if cb != nil {
			cb()
		}
		return len(p), nil
	}
	_, _ = w.buf.Write(p)
	w.remaining -= len(p)
	w.mu.Unlock()
	return len(p), nil
}

func (w *cappedBuffer) snapshot() ([]byte, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.buf.Bytes()), w.overflowed
}

func agyCommandEnv(base []string) []string {
	if base == nil {
		base = os.Environ()
	}
	const key = "AGY_CLI_DISABLE_AUTO_UPDATE"
	out := make([]string, 0, len(base)+1)
	for _, entry := range base {
		name, _, ok := strings.Cut(entry, "=")
		if ok && (name == key || (runtime.GOOS == "windows" && strings.EqualFold(name, key))) {
			continue
		}
		out = append(out, entry)
	}
	return append(out, key+"=true")
}

func runAgyUsageCommand(parentCtx context.Context, prog string, pre, baseEnv []string, timeout time.Duration) (stdout, stderr []byte, stdoutOverflow, stderrOverflow bool, err error) {
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()
	args := append(slices.Clone(pre), "-p", "/usage", "--output-format", "json", "--print-timeout", agyPrintTimeoutArg)
	cmd := exec.CommandContext(ctx, prog, args...)
	cmd.Stdin = nil
	cmd.Env = agyCommandEnv(baseEnv)
	configureAgyUsageCmd(cmd)
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return execjob.KillTree(cmd.Process.Pid)
		}
		return nil
	}
	cmd.WaitDelay = 2 * time.Second
	outBuf := &cappedBuffer{remaining: agyMaxOutputBytes, onOverflow: cancel}
	errBuf := &cappedBuffer{remaining: agyMaxOutputBytes, onOverflow: cancel}
	cmd.Stdout = outBuf
	cmd.Stderr = errBuf
	err = cmd.Run()
	stdout, stdoutOverflow = outBuf.snapshot()
	stderr, stderrOverflow = errBuf.snapshot()
	return stdout, stderr, stdoutOverflow, stderrOverflow, err
}

// agyHistory returns the full pruned agy usage history.
func (s *planUsageStore) agyHistory(now int64) planUsageHistoryProjection {
	if s == nil {
		return planUsageHistoryProjection{SchemaVersion: 1, Status: "empty", Samples: []planUsageSample{}}
	}
	return s.historyFile(&s.agyHist, agyHistorySpec, now)
}

// agyHistoryView returns the projected ?history=1 member for agy entries.
func (s *planUsageStore) agyHistoryView(entries []planUsageEntry, now int64) json.RawMessage {
	if s == nil {
		raw, _ := json.Marshal(planUsageHistoryProjection{SchemaVersion: 1, Status: "empty", Samples: []planUsageSample{}})
		return raw
	}
	var filtered []planUsageEntry
	for _, e := range entries {
		if e.Harness == "agy" {
			filtered = append(filtered, e)
		}
	}
	return s.historyFileView(&s.agyHist, agyHistorySpec, filtered, now)
}
