package wsdashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

const planUsageCache = "plan-usage.json"

// These values alone cross the viewer boundary and the private cache. Never
// retain auth documents, account identifiers, subprocess output, or errors here.
type planUsageWindow struct {
	Kind     string  `json:"kind"`
	Percent  float64 `json:"percent"`
	ResetsAt int64   `json:"resetsAt,omitempty"`
	AsOf     int64   `json:"asOf"`
	Disabled bool    `json:"disabled,omitempty"`
}
type planUsageEntry struct {
	Harness string            `json:"harness"`
	Group   string            `json:"group,omitempty"`
	Label   string            `json:"label,omitempty"`
	Pending bool              `json:"pending,omitempty"`
	Plan    string            `json:"plan,omitempty"`
	Source  string            `json:"source,omitempty"`
	Status  string            `json:"status,omitempty"`
	AsOf    int64             `json:"asOf,omitempty"`
	Windows []planUsageWindow `json:"windows"`
}
type planUsageStore struct {
	mu          sync.Mutex
	queryMu     sync.Mutex
	dir, home   string
	getenv      func(string) string
	claude, gpt planUsageEntry
	lastQuery   time.Time
	// lastSaved holds the last successfully written cache bytes, guarded by mu.
	lastSaved []byte
	// probing marks the one background probe in flight; probes lets tests and
	// daemon shutdown wait for it. closed and lifetimeCtx/lifetimeCancel tie
	// background probes to the owning daemon's lifetime. Guarded by mu.
	probing        bool
	probes         sync.WaitGroup
	lifetimeCtx    context.Context
	lifetimeCancel context.CancelFunc
	closed         bool
	hist           planUsageHistoryMemo
	// agy state is kept separate from Claude/Codex so plan-usage.json and
	// plan-usage-history.json stay byte-identical (T0). All fields below
	// except resolveAgy, agyDebounce, agyTimeout, agyEnv, and now are guarded by mu.
	resolveAgy     func() (string, []string, error)
	agyGroups      []planUsageEntry
	agyLastSaved   []byte
	agyLastProbe   time.Time
	agyProbing     bool
	agyProbed      bool
	agyAuthHold    bool
	agyBilledLatch bool
	agyStatus      string
	agyHist        planUsageHistoryMemo
	agyDebounce    time.Duration
	agyTimeout     time.Duration
	agyEnv         []string
	now            func() time.Time
	// Request dependencies are private test seams, not configuration knobs.
	// Production always uses the fixed HTTPS origin below for both readers.
	usageURL string
	client   *http.Client
	// File-system seams for the history memo and detection reads; nil means
	// os.Stat/os.ReadFile.
	statFile func(string) (os.FileInfo, error)
	readFile func(string) ([]byte, error)
	// Write seam for all usage files; nil means writePrivateFile.
	writeFile func(dir, name string, data []byte) error
	// detected memoizes detect() by the credential files' stats. Guarded by mu.
	detected usageDetectMemo
}

// usageCredStat identifies one credential file version. Only the detected
// plan/mode/presence is memoized, never file contents.
type usageCredStat struct {
	path    string
	missing bool
	size    int64
	modTime int64
}
type usageDetectMemo struct {
	ok        bool
	stats     [3]usageCredStat
	plan      string
	codex, pi bool
}

func (s *planUsageStore) write(name string, data []byte) error {
	if s.writeFile != nil {
		return s.writeFile(s.dir, name, data)
	}
	return writePrivateFile(s.dir, name, data)
}

func newPlanUsageStore(dir, home string, getenv func(string) string) *planUsageStore {
	lifeCtx, lifeCancel := context.WithCancel(context.Background())
	s := &planUsageStore{
		dir:            dir,
		home:           home,
		getenv:         getenv,
		lifetimeCtx:    lifeCtx,
		lifetimeCancel: lifeCancel,
		usageURL:       "https://chatgpt.com/backend-api/wham/usage",
		client:         &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
	var saved struct {
		Claude planUsageEntry `json:"claude"`
		GPT    planUsageEntry `json:"gpt"`
	}
	if raw, err := os.ReadFile(filepath.Join(dir, planUsageCache)); err == nil && json.Unmarshal(raw, &saved) == nil {
		s.claude, s.gpt = saved.Claude, saved.GPT
	}
	s.loadAgyLocked()
	return s
}

// beginClose marks the store closed and cancels its daemon-lifetime context so
// no new probe starts, in-flight probes terminate, and no further writes occur.
func (s *planUsageStore) beginClose() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.closed = true
	cancel := s.lifetimeCancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// waitClose waits for in-flight background probes until they finish or ctx
// expires. Call beginClose first so no new probe can join probes.
func (s *planUsageStore) waitClose(ctx context.Context) {
	if s == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		s.probes.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (s *planUsageStore) close(ctx context.Context) {
	s.beginClose()
	s.waitClose(ctx)
}

func (s *planUsageStore) nowTime() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}
func (s *planUsageStore) saveLocked() {
	raw, err := json.Marshal(struct {
		Claude planUsageEntry `json:"claude"`
		GPT    planUsageEntry `json:"gpt"`
	}{s.claude, s.gpt})
	if err == nil && !bytes.Equal(raw, s.lastSaved) {
		if s.write(planUsageCache, raw) == nil {
			s.lastSaved = raw
		}
	}
}
func validUsage(percent float64) bool {
	return !math.IsNaN(percent) && !math.IsInf(percent, 0) && percent >= 0 && percent <= 100
}

func (s *planUsageStore) pushClaude(snap *wslive.Snapshot) {
	if s == nil || snap == nil || snap.Adapter != "claude" || snap.Usage == nil {
		return
	}
	// A status without limits is not a complete usage response. Invalid
	// figures likewise cannot safely establish which windows are absent.
	if snap.UpdatedAt <= 0 || len(snap.Usage.Limits) == 0 {
		return
	}
	for _, v := range snap.Usage.Limits {
		if (v.Kind != wslive.LimitFiveHour && v.Kind != wslive.LimitSevenDay) || !validUsage(v.Percent) {
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if snap.UpdatedAt < s.claude.AsOf {
		return
	}
	// Sessions disagree near a window boundary: a stale one still reports the
	// expired five-hour window while a fresh one reports none. Taking either
	// as-is made the window flap on every refresh, and each flap rewrote the
	// whole history file. So an expired reported window is ignored, and a
	// report without a kind ends only a window that has already expired. Live
	// matches currentUsage: an unknown (zero) reset never expires.
	now := time.Now().UnixMilli()
	live := func(reset int64) bool { return reset == 0 || reset > now }
	changed := false
	present := make(map[string]bool, len(snap.Usage.Limits))
	for _, v := range snap.Usage.Limits {
		present[v.Kind] = true
	}
	kept := s.claude.Windows[:0]
	for _, old := range s.claude.Windows {
		if present[old.Kind] || live(old.ResetsAt) {
			kept = append(kept, old)
		} else {
			changed = true
		}
	}
	s.claude.Windows = kept
	var samples []planUsageSample
	for _, v := range snap.Usage.Limits {
		if (v.Kind != wslive.LimitFiveHour && v.Kind != wslive.LimitSevenDay) || !validUsage(v.Percent) {
			continue
		}
		next := planUsageWindow{Kind: v.Kind, Percent: v.Percent, ResetsAt: v.ResetsAt, AsOf: snap.UpdatedAt}
		if next.AsOf <= 0 || !live(next.ResetsAt) {
			continue
		}
		i := 0
		for i < len(s.claude.Windows) && s.claude.Windows[i].Kind != v.Kind {
			i++
		}
		if i < len(s.claude.Windows) {
			old := s.claude.Windows[i]
			// Reset is the window identity. Never resurrect an older window or
			// decrease usage within one, even when another session reports later.
			if next.ResetsAt < old.ResetsAt || (next.ResetsAt == old.ResetsAt && next.Percent <= old.Percent) {
				continue
			}
			if next.AsOf < old.AsOf {
				continue
			}
			s.claude.Windows[i] = next
		} else {
			s.claude.Windows = append(s.claude.Windows, next)
		}
		changed = true
		samples = append(samples, planUsageSample{Harness: "claude", Kind: next.Kind, Source: "Claude push", ResetsAt: next.ResetsAt, AsOf: next.AsOf, Percent: next.Percent})
		if next.AsOf > s.claude.AsOf {
			s.claude.AsOf = next.AsOf
		}
	}
	if changed {
		if snap.UpdatedAt > s.claude.AsOf {
			s.claude.AsOf = snap.UpdatedAt
		}
		s.claude.Harness = "claude"
		s.claude.Source = "Claude push"
		s.saveLocked()
		s.appendHistoryLocked(samples, now)
	}
}

func usageReset(raw json.RawMessage) int64 {
	text := strings.Trim(string(raw), "\"")
	if n, err := strconv.ParseFloat(text, 64); err == nil && n > 0 && !math.IsInf(n, 0) {
		if n < 1e12 {
			n *= 1000
		}
		return int64(n)
	}
	if t, err := time.Parse(time.RFC3339Nano, text); err == nil {
		return t.UnixMilli()
	}
	return 0
}
func readUsageJSON(path string, out any) bool {
	raw, err := os.ReadFile(path)
	return err == nil && json.Unmarshal(raw, out) == nil
}
func (s *planUsageStore) configHome(key, fallback string) string {
	if p := s.getenv(key); p != "" {
		return p
	}
	return filepath.Join(s.home, fallback)
}

// Detection decodes only plan/mode/presence, never credential values. Every
// usage request detects, so the result is memoized by the three files' stats
// (path, size, mtime): an unchanged set costs three stats and no reads. A
// rewrite keeping both size and mtime goes unnoticed until the next change.
// Stat-confirmed absence is cacheable; a read failure is not, so transient
// credential access failures remain retryable even without metadata changes.
func (s *planUsageStore) detect() (claudePlan string, codexLogin, piLogin bool) {
	paths := [3]string{
		filepath.Join(s.configHome("CLAUDE_CONFIG_DIR", ".claude"), ".credentials.json"),
		filepath.Join(s.configHome("CODEX_HOME", ".codex"), "auth.json"),
		filepath.Join(s.home, ".pi", "agent", "auth.json"),
	}
	var stats [3]usageCredStat
	cacheable := true
	for i, path := range paths {
		stats[i].path = path
		info, err := os.Stat(path)
		switch {
		case errors.Is(err, os.ErrNotExist):
			stats[i].missing = true
		case err != nil:
			cacheable = false
		default:
			stats[i].size, stats[i].modTime = info.Size(), info.ModTime().UnixNano()
		}
	}
	s.mu.Lock()
	m := s.detected
	s.mu.Unlock()
	if cacheable && m.ok && m.stats == stats {
		return m.plan, m.codex, m.pi
	}
	readJSON := func(index int, out any) bool {
		if stats[index].missing {
			return false
		}
		read := s.readFile
		if read == nil {
			read = os.ReadFile
		}
		raw, err := read(paths[index])
		if err != nil {
			cacheable = false
			return false
		}
		return json.Unmarshal(raw, out) == nil
	}
	var claude struct {
		OAuth struct {
			Plan string `json:"subscriptionType"`
		} `json:"claudeAiOauth"`
	}
	if readJSON(0, &claude) {
		claudePlan = claude.OAuth.Plan
	}
	var codex struct {
		Mode   string    `json:"auth_mode"`
		Tokens *struct{} `json:"tokens"`
	}
	if readJSON(1, &codex) {
		codexLogin = codex.Mode == "chatgpt" || (codex.Mode == "" && codex.Tokens != nil)
	}
	var pi struct {
		Codex *struct{} `json:"openai-codex"`
	}
	if readJSON(2, &pi) {
		piLogin = pi.Codex != nil
	}
	// Stat precedes read, so a replace in between memoizes newer content under
	// an older stat and costs only one extra read next time.
	s.mu.Lock()
	s.detected = usageDetectMemo{ok: cacheable, stats: stats, plan: claudePlan, codex: codexLogin, pi: piLogin}
	s.mu.Unlock()
	return
}
func currentUsage(e planUsageEntry, now int64) planUsageEntry {
	e.Windows = append([]planUsageWindow{}, e.Windows...)
	live := e.Windows[:0]
	for _, w := range e.Windows {
		if validUsage(w.Percent) && (w.ResetsAt == 0 || w.ResetsAt > now) {
			live = append(live, w)
		}
	}
	e.Windows = live
	if len(live) == 0 && e.Status == "" {
		e.Status = "usage unavailable"
	}
	return e
}
func (s *planUsageStore) entries() []planUsageEntry {
	plan, codex, pi := s.detect()
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.nowTime().UnixMilli()
	out := []planUsageEntry{}
	if plan != "" || len(s.claude.Windows) > 0 {
		c := s.claude
		c.Harness = "claude"
		c.Plan = plan
		out = append(out, currentUsage(c, now))
	}
	if codex || pi {
		x := s.gpt
		x.Harness = "codex"
		out = append(out, currentUsage(x, now))
	}
	out = append(out, s.agyEntriesLocked(now)...)
	return out
}

// GET is viewer-authorized by the daemon's ordinary access wrapper. Pages
// request while connected; there is no polling goroutine without viewers.
func (d *Daemon) handlePlanUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if d.planUsage == nil {
		body := map[string]any{"entries": []planUsageEntry{}, "warnPercent": bandWarnPercent, "critPercent": bandCritPercent}
		if r.URL.Query().Get("history") == "1" {
			body["history"] = d.planUsage.history(time.Now().UnixMilli())
		}
		_ = json.NewEncoder(w).Encode(body)
		return
	}
	if d.index != nil {
		for _, session := range d.index.Sessions() {
			if session.Harness == "claude" {
				d.planUsage.pushClaude(d.snapshots.snapshotWithoutObserved(d.dir, session.ReplyID))
			}
		}
	}
	// Cross-origin pages cannot set the custom header without a CORS preflight, which the daemon does not grant.
	refresh := r.URL.Query().Get("refresh") == "1" && (r.Header.Get("Sec-Fetch-Site") == "same-origin" || sameOrigin(r) || r.Header.Get("X-WS-Usage-Refresh") == "1")
	d.planUsage.refresh(r.Context(), refresh)
	entries := d.planUsage.entries()
	body := map[string]any{"entries": entries, "warnPercent": bandWarnPercent, "critPercent": bandCritPercent}
	if r.URL.Query().Get("history") == "1" {
		now := d.planUsage.nowTime().UnixMilli()
		body["history"] = d.planUsage.historyView(entries, now)
		for _, e := range entries {
			if e.Harness == "agy" {
				body["agyHistory"] = d.planUsage.agyHistoryView(entries, now)
				break
			}
		}
	}
	_ = json.NewEncoder(w).Encode(body)
}
