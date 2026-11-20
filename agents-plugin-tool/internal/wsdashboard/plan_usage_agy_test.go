package wsdashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeAgyEnv installs an isolated fake agy binary on PATH with a temporary HOME
// and returns a planUsageStore wired to resolve "agy" from that PATH only.
func fakeAgyEnv(t *testing.T, scriptBody string) (*planUsageStore, string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake agy helper runs on unix")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	cache := filepath.Join(root, "cache")
	binDir := filepath.Join(root, "bin")
	for _, dir := range []string{home, cache, binDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	countFile := filepath.Join(root, "count")
	argsFile := filepath.Join(root, "args")
	script := "#!/bin/sh\n" +
		"echo x >> " + shellQuote(countFile) + "\n" +
		"printf '%s\\n' \"$@\" > " + shellQuote(argsFile) + "\n" +
		"printf 'AUTO=%s HOME=%s\\n' \"$AGY_CLI_DISABLE_AUTO_UPDATE\" \"$HOME\" >> " + shellQuote(argsFile) + "\n" +
		scriptBody + "\n"
	binPath := filepath.Join(binDir, "agy")
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+"/usr/bin:/bin")
	s := newPlanUsageStore(cache, home, func(string) string { return "" })
	s.resolveAgy = func() (string, []string, error) {
		p, err := exec.LookPath("agy")
		return p, nil, err
	}
	return s, countFile, argsFile
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func fakeAgyCallCount(countFile string) int {
	raw, err := os.ReadFile(countFile)
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

func validAgyFixtureJSON(reset5h, resetWeekly string) string {
	return fmt.Sprintf(`{
  "status": "SUCCESS",
  "num_turns": 0,
  "conversation_id": "",
  "raw_secret_field": "RAW-STDOUT-SENTINEL",
  "command": {
    "name": "usage",
    "data": {
      "groups": [
        {
          "name": "Claude and GPT models",
          "buckets": [
            {"name": "weekly", "window": "weekly", "remaining_fraction": 0.65, "reset_time": %q, "disabled": false},
            {"name": "5h", "window": "5h", "remaining_fraction": 0.80, "reset_time": %q, "disabled": true}
          ]
        },
        {
          "name": "Gemini Models",
          "buckets": [
            {"name": "5h", "window": "5h", "remaining_fraction": -0.25, "reset_time": %q, "disabled": false},
            {"name": "weekly", "window": "weekly", "remaining_fraction": 1.40, "reset_time": %q, "disabled": false},
            {"name": "bad-window", "window": "monthly", "remaining_fraction": 0.5, "reset_time": %q},
            {"name": "missing-frac", "window": "5h", "reset_time": %q},
            {"name": "bad-frac", "window": "5h", "remaining_fraction": "oops", "reset_time": %q},
            {"name": "bad-reset", "window": "5h", "remaining_fraction": 0.5, "reset_time": "not-a-timestamp"}
          ]
        },
        {
          "name": "RAW-UNKNOWN-GROUP-SENTINEL",
          "buckets": [
            {"name": "5h", "window": "5h", "remaining_fraction": 0.5, "reset_time": %q}
          ]
        }
      ]
    }
  }
}`, resetWeekly, reset5h, reset5h, resetWeekly, reset5h, reset5h, reset5h, reset5h)
}

func waitAgyActivityProbe(t *testing.T, d *Daemon, s *planUsageStore) {
	t.Helper()
	started := make(chan struct{}, 1)
	resolve := s.resolveAgy
	s.resolveAgy = func() (string, []string, error) {
		started <- struct{}{}
		return resolve()
	}
	d.AgyActivity()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("agy activity did not start a probe")
	}
	s.probes.Wait()
}

func TestPlanUsageAgyActivityDoesNotWaitForStoreLock(t *testing.T) {
	base := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	s, countFile, _ := fakeAgyEnv(t, "cat <<'EOF'\n"+validAgyFixtureJSON(base.Add(3*time.Hour).Format(time.RFC3339), base.Add(4*24*time.Hour).Format(time.RFC3339))+"\nEOF")
	d := &Daemon{planUsage: s}
	s.mu.Lock()
	returned := make(chan struct{})
	go func() {
		d.AgyActivity()
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		s.mu.Unlock()
		t.Fatal("AgyActivity waited for the plan-usage store lock")
	}
	s.mu.Unlock()
	deadline := time.Now().Add(3 * time.Second)
	for fakeAgyCallCount(countFile) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := fakeAgyCallCount(countFile); got != 1 {
		t.Fatalf("activity probe count = %d, want 1", got)
	}
	s.probes.Wait()
}

func TestPlanUsageAgyActivitySharesPopoverProbePath(t *testing.T) {
	base := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	payload := validAgyFixtureJSON(base.Add(3*time.Hour).Format(time.RFC3339), base.Add(4*24*time.Hour).Format(time.RFC3339))
	var files [2][2][]byte
	for i, trigger := range []string{"activity", "popover"} {
		s, countFile, _ := fakeAgyEnv(t, "cat <<'EOF'\n"+payload+"\nEOF")
		s.now = func() time.Time { return base }
		d := &Daemon{planUsage: s}
		if trigger == "activity" {
			waitAgyActivityProbe(t, d, s)
		} else {
			req := httptest.NewRequest("GET", "/rail/usage?refresh=1", nil)
			req.Header.Set("X-WS-Usage-Refresh", "1")
			w := httptest.NewRecorder()
			d.handlePlanUsage(w, req)
			if w.Code != 200 {
				t.Fatalf("popover status = %d", w.Code)
			}
			s.probes.Wait()
		}
		if got := fakeAgyCallCount(countFile); got != 1 {
			t.Fatalf("%s probe count = %d, want 1", trigger, got)
		}
		for j, name := range []string{planUsageAgyCache, planUsageAgyHistoryCache} {
			var err error
			files[i][j], err = os.ReadFile(filepath.Join(s.dir, name))
			if err != nil {
				t.Fatal(err)
			}
			if !json.Valid(files[i][j]) {
				t.Fatalf("%s wrote invalid %s", trigger, name)
			}
		}
	}
	for j, name := range []string{planUsageAgyCache, planUsageAgyHistoryCache} {
		if !bytes.Equal(files[0][j], files[1][j]) {
			t.Fatalf("activity and popover wrote different %s shapes", name)
		}
	}
}

func TestPlanUsageAgyActivityRespectsDebounceAndHolds(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		advance    time.Duration
	}{
		{"debounce", `echo 'not json'; exit 1`, 9 * time.Minute},
		{"billed_turn", `echo '{"status":"SUCCESS","num_turns":1,"conversation_id":"","command":{"name":"usage","data":{"groups":[]}}}'`, 30 * time.Minute},
		{"auth_hold", `printf 'Authentication required. Please visit the URL https://example.invalid/auth\n' >&2; exit 1`, 30 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var clockMu sync.Mutex
			now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
			s, countFile, _ := fakeAgyEnv(t, tc.body)
			s.now = func() time.Time { clockMu.Lock(); defer clockMu.Unlock(); return now }
			d := &Daemon{planUsage: s}
			waitAgyActivityProbe(t, d, s)
			clockMu.Lock()
			now = now.Add(tc.advance)
			clockMu.Unlock()
			d.AgyActivity()
			// The second handoff has no probe to wait for; give its goroutine time to run.
			time.Sleep(50 * time.Millisecond)
			if got := fakeAgyCallCount(countFile); got != 1 {
				t.Fatalf("second activity spawned despite %s: count=%d", tc.name, got)
			}
		})
	}
}

func TestPlanUsageAgyCommandEnvStripsHarness(t *testing.T) {
	t.Setenv("WS_MCP_HARNESS", "agy")
	for _, base := range [][]string{nil, {"WS_MCP_HARNESS=agy", "AGY_CLI_DISABLE_AUTO_UPDATE=false", "OTHER=kept"}} {
		env := agyCommandEnv(base)
		var auto, other bool
		for _, entry := range env {
			if strings.HasPrefix(entry, "WS_MCP_HARNESS=") {
				t.Fatalf("probe inherited harness: %q", entry)
			}
			auto = auto || entry == "AGY_CLI_DISABLE_AUTO_UPDATE=true"
			other = other || entry == "OTHER=kept"
		}
		if !auto || base != nil && !other {
			t.Fatalf("probe environment lost required values: %v", env)
		}
	}
	if runtime.GOOS == "windows" {
		for _, entry := range agyCommandEnv([]string{"ws_mcp_harness=agy"}) {
			if strings.EqualFold(strings.SplitN(entry, "=", 2)[0], "WS_MCP_HARNESS") {
				t.Fatalf("probe inherited mixed-case harness: %q", entry)
			}
		}
	}
}

func TestParseAgyResetTimeRange(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want int64
		ok   bool
	}{
		{name: "seconds", raw: `1700000000`, want: 1700000000000, ok: true},
		{name: "milliseconds", raw: `1700000000000`, want: 1700000000000, ok: true},
		{name: "javascript date max", raw: `8640000000000000`, want: 8640000000000000, ok: true},
		{name: "above javascript date max", raw: `8640000000000001`},
		{name: "above int64", raw: `9223372036854775808`},
		{name: "overflowing float", raw: `1e309`},
		{name: "huge integer", raw: `999999999999999999999999999999999999`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseAgyResetTime(json.RawMessage(tc.raw))
			if ok != tc.ok || got != tc.want {
				t.Fatalf("parseAgyResetTime(%s) = %d, %v; want %d, %v", tc.raw, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestPlanUsageAgyBlockingGoHelper(t *testing.T) {
	if os.Getenv("WS_AGY_BLOCKING_HELPER") != "1" {
		return
	}
	if err := os.WriteFile(os.Getenv("WS_AGY_HELPER_STARTED"), []byte("started"), 0o600); err != nil {
		t.Fatal(err)
	}
	for {
		time.Sleep(time.Hour)
	}
}

func TestPlanUsageAgyDaemonShutdownCancelsGoHelper(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	dir := filepath.Join(root, "state")
	for _, path := range []string{home, dir} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	started := filepath.Join(root, "started")
	var probeStillRunningAtUnlock atomic.Bool
	var d *Daemon
	d, err = StartDaemon(DaemonOptions{
		Dir:        dir,
		Home:       home,
		noGitBadge: true,
		AgyCommand: func() (string, []string, error) {
			return exe, []string{"-test.run=^TestPlanUsageAgyBlockingGoHelper$", "--"}, nil
		},
		BeforeUnlock: func() {
			d.planUsage.mu.Lock()
			probeStillRunningAtUnlock.Store(d.planUsage.agyProbing)
			d.planUsage.mu.Unlock()
		},
	})
	if err != nil {
		t.Fatalf("StartDaemon: %v", err)
	}
	defer d.Close()
	d.planUsage.agyEnv = append(os.Environ(), "WS_AGY_BLOCKING_HELPER=1", "WS_AGY_HELPER_STARTED="+started)
	var writeCount atomic.Int32
	d.planUsage.writeFile = func(dir, name string, data []byte) error {
		writeCount.Add(1)
		return writePrivateFile(dir, name, data)
	}

	req := httptest.NewRequest("GET", "/rail/usage?refresh=1", nil)
	req.Header.Set("X-WS-Usage-Refresh", "1")
	w := httptest.NewRecorder()
	d.handlePlanUsage(w, req)
	if w.Code != 200 {
		t.Fatalf("forced /rail/usage status = %d", w.Code)
	}
	eventuallyFile := time.Now().Add(5 * time.Second)
	for time.Now().Before(eventuallyFile) {
		if _, err := os.Stat(started); err == nil {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if _, err := os.Stat(started); err != nil {
		t.Fatal("Go-built fake agy probe never started")
	}

	closeStart := time.Now()
	d.Close()
	if elapsed := time.Since(closeStart); elapsed > 5*time.Second {
		t.Fatalf("daemon Close did not bound in-flight agy probe: took %s", elapsed)
	}
	if probeStillRunningAtUnlock.Load() {
		t.Fatal("in-flight agy probe had not drained before daemon released its lock")
	}
	if got := writeCount.Load(); got != 0 {
		t.Fatalf("cancelled probe performed %d store writes, want 0", got)
	}
}

func TestPlanUsageAgyParseClampMalformedAndDisabled(t *testing.T) {
	base := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	reset5h := base.Add(3 * time.Hour).Format(time.RFC3339)
	resetWeekly := base.Add(4 * 24 * time.Hour).Format(time.RFC3339)
	payload := validAgyFixtureJSON(reset5h, resetWeekly)

	s, countFile, argsFile := fakeAgyEnv(t, "cat <<'EOF'\n"+payload+"\nEOF\nprintf 'RAW-STDERR-SENTINEL\\n' >&2")
	s.now = func() time.Time { return base }

	d := &Daemon{planUsage: s}
	// Ordinary poll on an empty store must never spawn agy.
	w := httptest.NewRecorder()
	d.handlePlanUsage(w, httptest.NewRequest("GET", "/rail/usage?history=1", nil))
	s.probes.Wait()
	if fakeAgyCallCount(countFile) != 0 {
		t.Fatalf("ordinary poll spawned agy %d times", fakeAgyCallCount(countFile))
	}
	if len(s.entries()) != 0 {
		t.Fatalf("empty store emitted entries before first probe: %+v", s.entries())
	}

	// Forced refresh spawns one background probe.
	req := httptest.NewRequest("GET", "/rail/usage?history=1&refresh=1", nil)
	req.Header.Set("X-WS-Usage-Refresh", "1")
	w = httptest.NewRecorder()
	d.handlePlanUsage(w, req)
	s.probes.Wait()
	if fakeAgyCallCount(countFile) != 1 {
		t.Fatalf("forced refresh spawned agy %d times, want 1", fakeAgyCallCount(countFile))
	}

	// Verify CLI arguments, env flag, and isolated HOME.
	argsRaw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	argsText := string(argsRaw)
	for _, want := range []string{"-p\n/usage\n--output-format\njson\n--print-timeout\n20s", "AUTO=true", "HOME=" + s.home} {
		if !strings.Contains(argsText, want) {
			t.Fatalf("missing expected arg/env %q in:\n%s", want, argsText)
		}
	}

	// Verify both groups parsed in canonical order with clamping, malformed drop, and disabled marker.
	entries := s.entries()
	if len(entries) != 2 {
		t.Fatalf("want 2 agy group entries, got %d: %+v", len(entries), entries)
	}
	gem, cg := entries[0], entries[1]
	if gem.Harness != "agy" || gem.Group != "gemini" || gem.Label != "Gemini Models" || gem.Status != "" {
		t.Fatalf("unexpected gemini entry: %+v", gem)
	}
	if len(gem.Windows) != 2 || gem.Windows[0].Kind != "five_hour" || gem.Windows[0].Percent != 100 || gem.Windows[0].Disabled ||
		gem.Windows[1].Kind != "seven_day" || gem.Windows[1].Percent != 0 || gem.Windows[1].Disabled {
		t.Fatalf("unexpected gemini windows (clamp/drop): %+v", gem.Windows)
	}
	if cg.Harness != "agy" || cg.Group != "claude_gpt" || cg.Label != "Claude and GPT models" || cg.Status != "" {
		t.Fatalf("unexpected claude_gpt entry: %+v", cg)
	}
	if len(cg.Windows) != 2 || cg.Windows[0].Kind != "five_hour" || cg.Windows[0].Percent != 20 || !cg.Windows[0].Disabled ||
		cg.Windows[1].Kind != "seven_day" || cg.Windows[1].Percent != 35 || cg.Windows[1].Disabled {
		t.Fatalf("unexpected claude_gpt windows (disabled marker): %+v", cg.Windows)
	}

	// Verify no raw stdout/stderr/unknown group strings leak into cache, history, or /rail/usage.
	cacheRaw, err := os.ReadFile(filepath.Join(s.dir, planUsageAgyCache))
	if err != nil {
		t.Fatal(err)
	}
	histRaw, err := os.ReadFile(filepath.Join(s.dir, planUsageAgyHistoryCache))
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	d.handlePlanUsage(w, httptest.NewRequest("GET", "/rail/usage?history=1", nil))
	for name, blob := range map[string]string{
		"cache":    string(cacheRaw),
		"history":  string(histRaw),
		"response": w.Body.String(),
	} {
		for _, sentinel := range []string{"RAW-STDOUT-SENTINEL", "RAW-STDERR-SENTINEL", "RAW-UNKNOWN-GROUP-SENTINEL"} {
			if strings.Contains(blob, sentinel) {
				t.Fatalf("%s leaked raw subprocess output %q", name, sentinel)
			}
		}
	}

	// Verify byte-identical save is skipped.
	writes := usageWriteCounter(s)
	s.mu.Lock()
	s.saveAgyLocked()
	s.mu.Unlock()
	if writes[planUsageAgyCache] != 0 {
		t.Fatalf("byte-identical saveAgyLocked wrote %d times, want 0", writes[planUsageAgyCache])
	}

	// Reopening store restores cached agy groups without spawning.
	reopened := newPlanUsageStore(s.dir, s.home, s.getenv)
	reopened.now = func() time.Time { return base }
	if got := reopened.entries(); len(got) != 2 || got[0].Group != "gemini" || got[1].Group != "claude_gpt" {
		t.Fatalf("reopened store did not restore agy groups: %+v", got)
	}
}

func TestPlanUsageAgyEmptyStoreDoesNotBlockForcedRefresh(t *testing.T) {
	base := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	reset5h := base.Add(3 * time.Hour).Format(time.RFC3339)
	resetWeekly := base.Add(4 * 24 * time.Hour).Format(time.RFC3339)
	gate := filepath.Join(t.TempDir(), "release")
	started := filepath.Join(t.TempDir(), "started")
	payload := validAgyFixtureJSON(reset5h, resetWeekly)

	script := fmt.Sprintf(
		"touch %s\nwhile [ ! -f %s ]; do sleep 0.02; done\ncat <<'EOF'\n%s\nEOF",
		shellQuote(started), shellQuote(gate), payload,
	)
	s, _, _ := fakeAgyEnv(t, script)
	s.now = func() time.Time { return base }
	d := &Daemon{planUsage: s}

	req := httptest.NewRequest("GET", "/rail/usage?history=1&refresh=1", nil)
	req.Header.Set("X-WS-Usage-Refresh", "1")
	w := httptest.NewRecorder()
	start := time.Now()
	d.handlePlanUsage(w, req)
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Fatalf("forced refresh on empty store blocked for %s", elapsed)
	}
	var resp struct {
		Entries []planUsageEntry `json:"entries"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Entries) != 2 {
		t.Fatalf("probe-start response returned %d entries, want 2 pending groups: %+v", len(resp.Entries), resp.Entries)
	}
	for i, want := range []string{"gemini", "claude_gpt"} {
		entry := resp.Entries[i]
		if entry.Harness != "agy" || entry.Group != want || !entry.Pending || len(entry.Windows) != 0 {
			t.Fatalf("pending entry %d = %+v, want agy/%s with no windows", i, entry, want)
		}
	}
	// Release fake agy and verify the background probe finishes and populates cache.
	if err := os.WriteFile(gate, []byte("go"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.probes.Wait()
	if got := s.entries(); len(got) != 2 || got[0].Pending || got[1].Pending {
		t.Fatalf("after background probe finished, want 2 non-pending entries, got %+v", got)
	}
}

func TestPlanUsageAgyBilledTurnLatch(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"num_turns", `{"status":"SUCCESS","num_turns":1,"conversation_id":"","command":{"name":"usage","data":{"groups":[]}}}`},
		{"conversation_id", `{"status":"SUCCESS","num_turns":0,"conversation_id":"conv-billed-1","command":{"name":"usage","data":{"groups":[]}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
			var clockMu sync.Mutex
			s, countFile, _ := fakeAgyEnv(t, "cat <<'EOF'\n"+tc.body+"\nEOF")
			s.now = func() time.Time { clockMu.Lock(); defer clockMu.Unlock(); return now }

			s.refresh(context.Background(), true)
			s.probes.Wait()
			if fakeAgyCallCount(countFile) != 1 {
				t.Fatalf("first probe count = %d, want 1", fakeAgyCallCount(countFile))
			}
			entries := s.entries()
			if len(entries) != 1 || entries[0].Harness != "agy" || entries[0].Group != "" || entries[0].Label != "" ||
				len(entries[0].Windows) != 0 || entries[0].Status != "usage unavailable" {
				t.Fatalf("want status-only unavailable entry after first-probe latch, got %+v", entries)
			}

			// Even past the 10-minute debounce, forced refresh must never spawn again.
			clockMu.Lock()
			now = now.Add(30 * time.Minute)
			clockMu.Unlock()
			s.refresh(context.Background(), true)
			s.probes.Wait()
			if fakeAgyCallCount(countFile) != 1 {
				t.Fatalf("billed-turn latch did not stop future probes: count=%d", fakeAgyCallCount(countFile))
			}
		})
	}
}

func TestPlanUsageAgyAuthHoldPersistsAcrossForcedRefreshes(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	var clockMu sync.Mutex
	s, countFile, _ := fakeAgyEnv(t, "printf 'Authentication required. Please visit the URL https://example.invalid/auth\\n' >&2; exit 1")
	s.now = func() time.Time { clockMu.Lock(); defer clockMu.Unlock(); return now }

	s.refresh(context.Background(), true)
	s.probes.Wait()
	if fakeAgyCallCount(countFile) != 1 {
		t.Fatalf("probe count = %d, want 1", fakeAgyCallCount(countFile))
	}
	entries := s.entries()
	if len(entries) != 1 || entries[0].Harness != "agy" || entries[0].Group != "" || entries[0].Label != "" ||
		len(entries[0].Windows) != 0 || entries[0].Status != "sign-in required" {
		t.Fatalf("want status-only sign-in required entry, got %+v", entries)
	}
	raw, _ := json.Marshal(entries)
	if strings.Contains(string(raw), "example.invalid") {
		t.Fatalf("auth URL leaked into entries: %s", raw)
	}

	// Past debounce, hold still prevents spawning until daemon restart.
	clockMu.Lock()
	now = now.Add(30 * time.Minute)
	clockMu.Unlock()
	s.refresh(context.Background(), true)
	s.probes.Wait()
	if fakeAgyCallCount(countFile) != 1 {
		t.Fatalf("auth hold did not persist across forced refresh: count=%d", fakeAgyCallCount(countFile))
	}
}

func TestPlanUsageAgyDebounceCoversFailedProbes(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	var clockMu sync.Mutex
	s, countFile, _ := fakeAgyEnv(t, "echo 'not json'; exit 1")
	s.now = func() time.Time { clockMu.Lock(); defer clockMu.Unlock(); return now }

	s.refresh(context.Background(), true)
	s.probes.Wait()
	if fakeAgyCallCount(countFile) != 1 {
		t.Fatalf("probe count = %d, want 1", fakeAgyCallCount(countFile))
	}
	// Inside 10-minute debounce, forced refresh must not retry the failed probe.
	clockMu.Lock()
	now = now.Add(9 * time.Minute)
	clockMu.Unlock()
	s.refresh(context.Background(), true)
	s.probes.Wait()
	if fakeAgyCallCount(countFile) != 1 {
		t.Fatalf("failed probe retried inside debounce: count=%d", fakeAgyCallCount(countFile))
	}
	// Once 10 minutes have elapsed since spawn, forced refresh spawns again.
	clockMu.Lock()
	now = now.Add(2 * time.Minute)
	clockMu.Unlock()
	s.refresh(context.Background(), true)
	s.probes.Wait()
	if fakeAgyCallCount(countFile) != 2 {
		t.Fatalf("probe did not run after debounce elapsed: count=%d", fakeAgyCallCount(countFile))
	}
}

func TestPlanUsageAgyTimeoutAndOversizeKeepPreviousValue(t *testing.T) {
	for _, mode := range []string{"timeout", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
			var clockMu sync.Mutex
			reset5h := now.Add(3 * time.Hour).Format(time.RFC3339)
			resetWeekly := now.Add(4 * 24 * time.Hour).Format(time.RFC3339)
			modeFile := filepath.Join(t.TempDir(), "mode")
			if err := os.WriteFile(modeFile, []byte("ok"), 0o600); err != nil {
				t.Fatal(err)
			}
			payload := validAgyFixtureJSON(reset5h, resetWeekly)
			script := fmt.Sprintf(`m=$(cat %s)
if [ "$m" = "ok" ]; then
  cat <<'EOF'
%s
EOF
elif [ "$m" = "timeout" ]; then
  sleep 10
elif [ "$m" = "oversize" ]; then
  dd if=/dev/zero bs=1024 count=600 2>/dev/null | tr '\0' 'A'
fi`, shellQuote(modeFile), payload)
			s, _, _ := fakeAgyEnv(t, script)
			s.now = func() time.Time { clockMu.Lock(); defer clockMu.Unlock(); return now }

			// Seed cached value via initial successful probe with normal timeout.
			s.refresh(context.Background(), true)
			s.probes.Wait()
			if len(s.entries()) != 2 {
				t.Fatalf("seed probe failed: %+v", s.entries())
			}

			// Switch fake binary to timeout or oversize and advance past debounce.
			if mode == "timeout" {
				s.agyTimeout = 250 * time.Millisecond
			}
			if err := os.WriteFile(modeFile, []byte(mode), 0o600); err != nil {
				t.Fatal(err)
			}
			clockMu.Lock()
			now = now.Add(15 * time.Minute)
			clockMu.Unlock()
			start := time.Now()
			s.refresh(context.Background(), true)
			s.probes.Wait()
			if mode == "timeout" && time.Since(start) > 3*time.Second {
				t.Fatalf("timeout probe was not bounded: took %s", time.Since(start))
			}

			entries := s.entries()
			if len(entries) != 2 {
				t.Fatalf("%s dropped previous cached entries: %+v", mode, entries)
			}
			for _, e := range entries {
				if e.Status != "usage unavailable" || len(e.Windows) == 0 {
					t.Fatalf("%s did not preserve windows with unavailable status: %+v", mode, e)
				}
			}
		})
	}
}

func TestPlanUsageAgyMissingBinarySkipsSpawn(t *testing.T) {
	home := t.TempDir()
	cache := t.TempDir()
	emptyBin := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", emptyBin)
	s := newPlanUsageStore(cache, home, func(string) string { return "" })
	s.resolveAgy = func() (string, []string, error) {
		p, err := exec.LookPath("agy")
		return p, nil, err
	}
	s.refresh(context.Background(), true)
	s.probes.Wait()
	if got := s.entries(); len(got) != 0 {
		t.Fatalf("missing binary created entries: %+v", got)
	}
	for _, name := range []string{planUsageAgyCache, planUsageAgyHistoryCache} {
		if _, err := os.Stat(filepath.Join(cache, name)); !os.IsNotExist(err) {
			t.Fatalf("missing binary created %s", name)
		}
	}
}

func TestPlanUsageAgyHistoryAppendPruneAndGroupIsolation(t *testing.T) {
	s := usageTestStore(t)
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC).UnixMilli()
	cutoff := now - planUsageHistoryAge.Milliseconds()
	resetGem := now + 3*3600*1000
	resetCG := now + 3*3600*1000

	// Append stale sample (dropped), plus 5002 gemini 5h samples and 2 claude_gpt 5h samples.
	samples := make([]planUsageSample, 0, 5005)
	samples = append(samples, planUsageSample{Harness: "agy", Group: "gemini", Kind: "five_hour", Source: agyUsageSource, ResetsAt: resetGem, AsOf: cutoff - 1, Percent: 1})
	for i := 0; i < 5002; i++ {
		samples = append(samples, planUsageSample{Harness: "agy", Group: "gemini", Kind: "five_hour", Source: agyUsageSource, ResetsAt: resetGem, AsOf: now - int64(5002-i)*1000, Percent: float64(i % 100)})
	}
	samples = append(samples,
		planUsageSample{Harness: "agy", Group: "claude_gpt", Kind: "five_hour", Source: agyUsageSource, ResetsAt: resetCG, AsOf: now - 60000, Percent: 12},
		planUsageSample{Harness: "agy", Group: "claude_gpt", Kind: "five_hour", Source: agyUsageSource, ResetsAt: resetCG, AsOf: now, Percent: 24},
	)
	s.mu.Lock()
	s.appendHistoryFileLocked(&s.agyHist, agyHistorySpec, samples, now)
	s.mu.Unlock()

	full := s.agyHistory(now)
	if full.Status != "ok" || len(full.Samples) != 5002 {
		t.Fatalf("want 5000 gemini + 2 claude_gpt samples after prune, got status=%s len=%d", full.Status, len(full.Samples))
	}
	// Verify plan-usage-history.json was never touched.
	if _, err := os.Stat(filepath.Join(s.dir, planUsageHistoryCache)); !os.IsNotExist(err) {
		t.Fatal("agy history wrote to plan-usage-history.json")
	}

	// Project for claude_gpt only: must never include gemini samples.
	cgEntry := []planUsageEntry{{
		Harness: "agy", Group: "claude_gpt", Label: "Claude and GPT models", Source: agyUsageSource, AsOf: now,
		Windows: []planUsageWindow{{Kind: "five_hour", Percent: 24, ResetsAt: resetCG, AsOf: now}},
	}}
	rawView := s.agyHistoryView(cgEntry, now)
	var view planUsageHistoryProjection
	if err := json.Unmarshal(rawView, &view); err != nil {
		t.Fatal(err)
	}
	if view.Status != "ok" || len(view.Samples) != 2 {
		t.Fatalf("projected claude_gpt view = %+v", view)
	}
	for _, sample := range view.Samples {
		if sample.Group != "claude_gpt" {
			t.Fatalf("gemini sample leaked into claude_gpt projection: %+v", sample)
		}
	}
}

func TestPlanUsageAgyResetTimeAndStatusValidation(t *testing.T) {
	at := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC).UnixMilli()
	// Unsupported command name -> unavailable.
	outcome, groups := parseAgyUsageOutput([]byte(`{"status":"SUCCESS","num_turns":0,"conversation_id":"","command":{"name":"other","data":{"groups":[]}}}`), nil, false, false, nil, at)
	if outcome != agyOutcomeUnavailable || len(groups) != 0 {
		t.Fatalf("unsupported command accepted: %v %+v", outcome, groups)
	}
	// Non-SUCCESS status -> unavailable.
	outcome, _ = parseAgyUsageOutput([]byte(`{"status":"ERROR","num_turns":0,"conversation_id":"","command":{"name":"usage","data":{"groups":[{"name":"Gemini Models","buckets":[{"window":"5h","remaining_fraction":0.5,"reset_time":"2026-11-19T15:00:00Z"}]}]}}}`), nil, false, false, nil, at)
	if outcome != agyOutcomeUnavailable {
		t.Fatalf("error status accepted: %v", outcome)
	}
	// Numeric epoch reset_time parses cleanly; null, absent, and empty string drop per D7.
	epoch := strconv.FormatInt(at/1000+3600, 10)
	body := fmt.Sprintf(`{"status":"SUCCESS","num_turns":0,"conversation_id":"","command":{"name":"usage","data":{"groups":[{"name":"Gemini Models","buckets":[{"window":"5h","remaining_fraction":0.25,"reset_time":%s},{"window":"weekly","remaining_fraction":0.5,"reset_time":null},{"window":"weekly","remaining_fraction":0.4},{"window":"5h","remaining_fraction":0.1,"reset_time":""}]}]}}}`, epoch)
	outcome, groups = parseAgyUsageOutput([]byte(body), nil, false, false, nil, at)
	if outcome != agyOutcomeOK || len(groups) != 1 || len(groups[0].Windows) != 1 {
		t.Fatalf("expected only numeric epoch bucket to survive, got outcome=%v groups=%+v", outcome, groups)
	}
	if groups[0].Windows[0].Kind != "five_hour" || groups[0].Windows[0].ResetsAt != (at/1000+3600)*1000 {
		t.Fatalf("unexpected resetsAt values: %+v", groups[0].Windows)
	}
	// Independent per-stream cappedBuffer: each stream retains up to its own cap.
	var cancelled int
	outW := &cappedBuffer{remaining: 16, onOverflow: func() { cancelled++ }}
	errW := &cappedBuffer{remaining: 16, onOverflow: func() { cancelled++ }}
	_, _ = outW.Write(bytes.Repeat([]byte("a"), 12))
	_, _ = errW.Write(bytes.Repeat([]byte("b"), 20))
	outBytes, outOver := outW.snapshot()
	errBytes, errOver := errW.snapshot()
	if outOver || len(outBytes) != 12 {
		t.Fatalf("stdout buffer affected by stderr overflow: over=%v len=%d", outOver, len(outBytes))
	}
	if !errOver || len(errBytes) != 16 || cancelled != 1 {
		t.Fatalf("stderr cap failed: over=%v len=%d cancelled=%d", errOver, len(errBytes), cancelled)
	}
}

func TestPlanUsageAgyOverflowLatchesAndAuthHold(t *testing.T) {
	t.Run("billed_stdout_with_stderr_overflow", func(t *testing.T) {
		var clockMu sync.Mutex
		now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
		script := `printf '{"status":"SUCCESS","num_turns":1,"conversation_id":"","command":{"name":"usage","data":{"groups":[]}}}\n'
dd if=/dev/zero bs=1024 count=600 2>/dev/null | tr '\0' 'E' >&2`
		s, countFile, _ := fakeAgyEnv(t, script)
		s.now = func() time.Time { clockMu.Lock(); defer clockMu.Unlock(); return now }

		s.refresh(context.Background(), true)
		s.probes.Wait()

		s.mu.Lock()
		latched := s.agyBilledLatch
		s.mu.Unlock()
		if !latched {
			t.Fatal("expected billed latch when stdout has num_turns=1 and stderr overflows 512 KiB")
		}
		if got := fakeAgyCallCount(countFile); got != 1 {
			t.Fatalf("initial call count = %d, want 1", got)
		}

		// Advancing past debounce must not spawn again once latched.
		clockMu.Lock()
		now = now.Add(30 * time.Minute)
		clockMu.Unlock()
		s.refresh(context.Background(), true)
		s.probes.Wait()
		if got := fakeAgyCallCount(countFile); got != 1 {
			t.Fatalf("latched store spawned again: count=%d", got)
		}
	})

	t.Run("stdout_overflow_sets_billed_latch", func(t *testing.T) {
		var clockMu sync.Mutex
		now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
		script := `dd if=/dev/zero bs=1024 count=600 2>/dev/null | tr '\0' 'A'`
		s, countFile, _ := fakeAgyEnv(t, script)
		s.now = func() time.Time { clockMu.Lock(); defer clockMu.Unlock(); return now }

		s.refresh(context.Background(), true)
		s.probes.Wait()

		s.mu.Lock()
		latched := s.agyBilledLatch
		s.mu.Unlock()
		if !latched {
			t.Fatal("expected conservative billed latch when stdout overflows 512 KiB")
		}
		entries := s.entries()
		if len(entries) != 1 || entries[0].Status != "usage unavailable" {
			t.Fatalf("unexpected status-only entry on stdout overflow: %+v", entries)
		}

		clockMu.Lock()
		now = now.Add(30 * time.Minute)
		clockMu.Unlock()
		s.refresh(context.Background(), true)
		s.probes.Wait()
		if got := fakeAgyCallCount(countFile); got != 1 {
			t.Fatalf("stdout-overflow latch failed to prevent future spawns: count=%d", got)
		}
	})

	t.Run("auth_marker_retained_on_stderr_overflow", func(t *testing.T) {
		var clockMu sync.Mutex
		now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
		script := `printf 'Authentication required. Please visit the URL https://example.invalid/login\n' >&2
dd if=/dev/zero bs=1024 count=600 2>/dev/null | tr '\0' 'E' >&2
exit 1`
		s, countFile, _ := fakeAgyEnv(t, script)
		s.now = func() time.Time { clockMu.Lock(); defer clockMu.Unlock(); return now }

		s.refresh(context.Background(), true)
		s.probes.Wait()

		s.mu.Lock()
		held := s.agyAuthHold
		s.mu.Unlock()
		if !held {
			t.Fatal("expected auth hold when marker is in retained stderr before overflow")
		}
		entries := s.entries()
		if len(entries) != 1 || entries[0].Status != agyStatusSignInRequired {
			t.Fatalf("unexpected entry on auth-hold stderr overflow: %+v", entries)
		}

		clockMu.Lock()
		now = now.Add(30 * time.Minute)
		clockMu.Unlock()
		s.refresh(context.Background(), true)
		s.probes.Wait()
		if got := fakeAgyCallCount(countFile); got != 1 {
			t.Fatalf("auth hold failed to prevent future spawns: count=%d", got)
		}
	})
}
