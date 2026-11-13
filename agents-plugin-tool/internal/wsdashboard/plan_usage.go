package wsdashboard

import (
	"encoding/json"
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
}
type planUsageEntry struct {
	Harness string            `json:"harness"`
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
	// Request dependencies are private test seams, not configuration knobs.
	codexBinary string
	usageURL    string
	client      *http.Client
}

func newPlanUsageStore(dir, home string, getenv func(string) string) *planUsageStore {
	s := &planUsageStore{dir: dir, home: home, getenv: getenv, usageURL: "https://chatgpt.com/backend-api/wham/usage", client: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	var saved struct {
		Claude planUsageEntry `json:"claude"`
		GPT    planUsageEntry `json:"gpt"`
	}
	if raw, err := os.ReadFile(filepath.Join(dir, planUsageCache)); err == nil && json.Unmarshal(raw, &saved) == nil {
		s.claude, s.gpt = saved.Claude, saved.GPT
	}
	return s
}
func (s *planUsageStore) saveLocked() {
	raw, err := json.Marshal(struct {
		Claude planUsageEntry `json:"claude"`
		GPT    planUsageEntry `json:"gpt"`
	}{s.claude, s.gpt})
	if err == nil {
		_ = writePrivateFile(s.dir, planUsageCache, raw)
	}
}
func validUsage(percent float64) bool {
	return !math.IsNaN(percent) && !math.IsInf(percent, 0) && percent >= 0 && percent <= 100
}

func (s *planUsageStore) pushClaude(snap *wslive.Snapshot) {
	if s == nil || snap == nil || snap.Adapter != "claude" || snap.Usage == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for _, v := range snap.Usage.Limits {
		if (v.Kind != wslive.LimitFiveHour && v.Kind != wslive.LimitSevenDay) || !validUsage(v.Percent) {
			continue
		}
		next := planUsageWindow{Kind: v.Kind, Percent: v.Percent, ResetsAt: v.ResetsAt, AsOf: snap.UpdatedAt}
		if next.AsOf <= 0 {
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
		if next.AsOf > s.claude.AsOf {
			s.claude.AsOf = next.AsOf
		}
	}
	if changed {
		s.claude.Harness = "claude"
		s.claude.Source = "Claude push"
		s.saveLocked()
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

// Detection decodes only plan/mode/presence, never credential values.
func (s *planUsageStore) detect() (claudePlan string, codexLogin, piLogin bool) {
	var claude struct {
		OAuth struct {
			Plan string `json:"subscriptionType"`
		} `json:"claudeAiOauth"`
	}
	if readUsageJSON(filepath.Join(s.configHome("CLAUDE_CONFIG_DIR", ".claude"), ".credentials.json"), &claude) {
		claudePlan = claude.OAuth.Plan
	}
	var codex struct {
		Mode   string    `json:"auth_mode"`
		Tokens *struct{} `json:"tokens"`
	}
	if readUsageJSON(filepath.Join(s.configHome("CODEX_HOME", ".codex"), "auth.json"), &codex) {
		codexLogin = codex.Mode == "chatgpt" || (codex.Mode == "" && codex.Tokens != nil)
	}
	var pi struct {
		Codex *struct{} `json:"openai-codex"`
	}
	if readUsageJSON(filepath.Join(s.home, ".pi", "agent", "auth.json"), &pi) {
		piLogin = pi.Codex != nil
	}
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
	now := time.Now().UnixMilli()
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
		_ = json.NewEncoder(w).Encode(map[string]any{"entries": []planUsageEntry{}, "warnPercent": bandWarnPercent, "critPercent": bandCritPercent})
		return
	}
	if d.index != nil {
		for _, session := range d.index.Sessions() {
			if session.Harness == "claude" {
				d.planUsage.pushClaude(d.snapshots.Snapshot(d.dir, session.ReplyID))
			}
		}
	}
	// Cross-origin pages cannot set the custom header without a CORS preflight, which the daemon does not grant.
	refresh := r.URL.Query().Get("refresh") == "1" && (r.Header.Get("Sec-Fetch-Site") == "same-origin" || sameOrigin(r) || r.Header.Get("X-WS-Usage-Refresh") == "1")
	d.planUsage.query(r.Context(), refresh)
	_ = json.NewEncoder(w).Encode(map[string]any{"entries": d.planUsage.entries(), "warnPercent": bandWarnPercent, "critPercent": bandCritPercent})
}
