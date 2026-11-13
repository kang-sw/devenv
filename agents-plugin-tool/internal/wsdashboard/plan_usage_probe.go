package wsdashboard

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/execjob"
)

var errPlanUsage = errors.New("usage unavailable")

func (s *planUsageStore) query(ctx context.Context, force bool) {
	s.queryMu.Lock()
	defer s.queryMu.Unlock()
	// Coalesce simultaneous popover opens as well as ordinary page polls.
	interval := 5 * time.Minute
	if force {
		interval = 5 * time.Second
	}
	if time.Since(s.lastQuery) < interval {
		return
	}
	s.lastQuery = time.Now()
	_, codex, pi := s.detect()
	if !codex && !pi {
		return
	}
	var entry planUsageEntry
	var err error
	if codex {
		entry, err = s.queryCodex(ctx)
	} else {
		entry, err = s.queryPi(ctx)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.gpt.Status = "usage unavailable"
		if !codex && err == errPiExpired {
			s.gpt.Status = "Pi token expired - open Pi to refresh"
		}
		// Keep the last value's source, not the failed attempt's source.
		if s.gpt.Source == "" {
			if codex {
				s.gpt.Source = "via Codex CLI"
			} else {
				s.gpt.Source = "via Pi"
			}
		}
	} else {
		s.gpt = entry
	}
	s.saveLocked()
}

type codexUsageWindow struct {
	Percent *float64        `json:"usedPercent"`
	Minutes int             `json:"windowDurationMins"`
	Reset   json.RawMessage `json:"resetsAt"`
}
type codexUsageLimits struct {
	Plan      string            `json:"planType"`
	Primary   *codexUsageWindow `json:"primary"`
	Secondary *codexUsageWindow `json:"secondary"`
}

func codexUsageEntry(limits codexUsageLimits, source string) (planUsageEntry, error) {
	at := time.Now().UnixMilli()
	e := planUsageEntry{Harness: "codex", Plan: limits.Plan, Source: source, AsOf: at, Windows: []planUsageWindow{}}
	for _, w := range []*codexUsageWindow{limits.Primary, limits.Secondary} {
		// Primary and secondary can exchange positions across plans.
		if w == nil || w.Minutes != 10080 || w.Percent == nil || !validUsage(*w.Percent) {
			continue
		}
		e.Windows = append(e.Windows, planUsageWindow{Kind: "seven_day", Percent: *w.Percent, ResetsAt: usageReset(w.Reset), AsOf: at})
	}
	if len(e.Windows) == 0 {
		return planUsageEntry{}, errPlanUsage
	}
	return e, nil
}

func (s *planUsageStore) queryCodex(ctx context.Context) (planUsageEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	binary := s.codexBinary
	if binary == "" {
		var err error
		binary, err = exec.LookPath("codex")
		if err != nil {
			return planUsageEntry{}, errPlanUsage
		}
	}
	cmd := exec.CommandContext(ctx, binary, "app-server")
	if runtime.GOOS == "windows" && (strings.EqualFold(filepath.Ext(binary), ".cmd") || strings.EqualFold(filepath.Ext(binary), ".bat")) {
		cmd = exec.CommandContext(ctx, "cmd.exe", "/d", "/s", "/c", `"`+binary+`" app-server`)
	}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return execjob.KillTree(cmd.Process.Pid)
	}
	cmd.WaitDelay = time.Second
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return planUsageEntry{}, errPlanUsage
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return planUsageEntry{}, errPlanUsage
	}
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		return planUsageEntry{}, errPlanUsage
	}
	defer func() { _ = stdin.Close(); cancel(); _ = cmd.Wait() }()
	enc := json.NewEncoder(stdin)
	if enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"clientInfo": map[string]string{"name": "ws-dashboard", "version": "1"}}}) != nil {
		return planUsageEntry{}, errPlanUsage
	}
	scan := bufio.NewScanner(stdout)
	scan.Buffer(make([]byte, 4096), 1<<20)
	initialized := false
	for scan.Scan() {
		var reply struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if json.Unmarshal(scan.Bytes(), &reply) != nil {
			return planUsageEntry{}, errPlanUsage
		}
		if reply.ID == 1 && !initialized {
			if len(reply.Error) > 0 && string(reply.Error) != "null" {
				return planUsageEntry{}, errPlanUsage
			}
			if len(reply.Result) == 0 || string(reply.Result) == "null" {
				return planUsageEntry{}, errPlanUsage
			}
			initialized = true
			if enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "initialized"}) != nil || enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "account/rateLimits/read"}) != nil {
				return planUsageEntry{}, errPlanUsage
			}
		} else if reply.ID == 2 && initialized {
			if len(reply.Error) > 0 && string(reply.Error) != "null" {
				return planUsageEntry{}, errPlanUsage
			}
			var result struct {
				Limits codexUsageLimits `json:"rateLimits"`
			}
			if json.Unmarshal(reply.Result, &result) != nil {
				return planUsageEntry{}, errPlanUsage
			}
			return codexUsageEntry(result.Limits, "via Codex CLI")
		}
	}
	return planUsageEntry{}, errPlanUsage
}

var errPiExpired = errors.New("Pi token expired - open Pi to refresh")

func (s *planUsageStore) queryPi(ctx context.Context) (planUsageEntry, error) {
	// Only this fallback decodes credential values. They live for this request
	// only; neither upstream error text nor response bodies ever leave it.
	var auth struct {
		Codex struct {
			Type    string `json:"type"`
			Access  string `json:"access"`
			Account string `json:"accountId"`
			Expires int64  `json:"expires"`
		} `json:"openai-codex"`
	}
	if !readUsageJSON(filepath.Join(s.home, ".pi", "agent", "auth.json"), &auth) {
		return planUsageEntry{}, errPlanUsage
	}
	if auth.Codex.Expires <= time.Now().UnixMilli() {
		return planUsageEntry{}, errPiExpired
	}
	if auth.Codex.Type != "oauth" || auth.Codex.Access == "" {
		return planUsageEntry{}, errPlanUsage
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.usageURL, nil)
	if err != nil {
		return planUsageEntry{}, errPlanUsage
	}
	req.Header.Set("Authorization", "Bearer "+auth.Codex.Access)
	if auth.Codex.Account != "" {
		req.Header.Set("ChatGPT-Account-Id", auth.Codex.Account)
	}
	req.Header.Set("User-Agent", "codex-cli")
	req.Header.Set("OpenAI-Beta", "codex-1")
	req.Header.Set("originator", "Codex Desktop")
	response, err := s.client.Do(req)
	if err != nil {
		return planUsageEntry{}, errPlanUsage
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return planUsageEntry{}, errPlanUsage
	}
	type window struct {
		Percent *float64        `json:"used_percent"`
		Seconds int             `json:"limit_window_seconds"`
		Reset   json.RawMessage `json:"reset_at"`
	}
	var body struct {
		Plan   string `json:"plan_type"`
		Limits struct {
			Primary   *window `json:"primary_window"`
			Secondary *window `json:"secondary_window"`
		} `json:"rate_limit"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&body) != nil {
		return planUsageEntry{}, errPlanUsage
	}
	convert := func(w *window) *codexUsageWindow {
		if w == nil {
			return nil
		}
		return &codexUsageWindow{Percent: w.Percent, Minutes: w.Seconds / 60, Reset: w.Reset}
	}
	return codexUsageEntry(codexUsageLimits{Plan: body.Plan, Primary: convert(body.Limits.Primary), Secondary: convert(body.Limits.Secondary)}, "via Pi")
}
