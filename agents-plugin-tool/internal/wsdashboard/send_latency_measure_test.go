//go:build !windows

package wsdashboard

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/pprof"
	"syscall"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// A disposable loopback daemon, never the installed runtime. Measurements
// include the in-process SSE client and fixture driver, exclude setup/browser,
// and report cache snapshot requests (hits + misses), not disk opens.
// Opt in with WS_SEND_LATENCY_EVIDENCE=<output directory>.
func TestSendLatencyEvidenceFixture(t *testing.T) {
	out := os.Getenv("WS_SEND_LATENCY_EVIDENCE")
	if out == "" {
		t.Skip("set WS_SEND_LATENCY_EVIDENCE to an output directory for send-latency measurements")
	}
	for _, name := range []string{"HOME", "CLAUDE_CONFIG_DIR", "CODEX_HOME", "WS_CACHE_HOME", "WS_CONFIG_HOME"} {
		t.Setenv(name, t.TempDir())
	}
	t.Logf("fixture_pid=%d", os.Getpid())
	if err := os.MkdirAll(out, 0700); err != nil {
		t.Fatal(err)
	}
	dir, project := t.TempDir(), t.TempDir()
	src := newPageFake()
	now := time.Now()
	for i := 0; i < 7; i++ {
		id := fmt.Sprintf("latency%d", i)
		reg := Registration{ReplyID: id, Project: project, Harness: "pi", RegisteredAt: stampFor(now.Add(-time.Hour)), HeartbeatAt: stampFor(now.Add(time.Hour))}
		if err := WriteRegistration(dir, reg); err != nil {
			t.Fatal(err)
		}
		writeSnapshot(t, dir, id, &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: now.UnixMilli(), HeartbeatMs: 3600000, Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseThinking}}, Agents: []wslive.Agent{{ID: "child", Status: wslive.StatusRunning, Model: "launch-model"}}})
		src.setPage(id, PageContent{HasBoard: true, Seq: 1, Board: boardRegions(t, "Evidence fixture"), Contacts: []Contact{{Address: "id:latency1", ReplyID: "latency1"}}})
		evidence := filepath.Join(SessionDir(dir, id), wslive.ObservedDirName)
		if err := os.MkdirAll(evidence, 0700); err != nil {
			t.Fatal(err)
		}
		for j := 0; j < 4000; j++ {
			r := wslive.ObservedRecord{ModelObservation: wslive.ModelObservation{ID: fmt.Sprint(j), Model: fmt.Sprintf("answer-model-%d", (j/1000)%2), Source: "pi.assistant", At: 1700000000000 + int64(j)*1000}}
			if j%2 == 1 {
				r.AgentID = "child"
			}
			raw, _ := json.Marshal(r)
			if err := os.WriteFile(filepath.Join(evidence, fmt.Sprintf("%06d.json", j)), raw, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	d := startIndexedDaemon(t, dir, src, time.Hour, func(o *DaemonOptions) { o.Port = 0; o.LAN = false; o.bindHost = "127.0.0.1"; o.noGitBadge = true })
	waitFor(t, "seven evidence sessions", func() bool { return len(d.Index().Sessions()) == 7 && d.Index().Loaded() })
	base := endpointURL(d.Endpoint(), "")
	events := []<-chan sseEvent{openSSE(t, base+"/events"), openSSE(t, base+"/s/latency0/events?railrows=1"), openSSE(t, base+"/s/latency2/events?railrows=1")}
	for _, e := range events {
		collect(e)
	}
	// Continuous drains keep the browser-side readers from throttling the daemon.
	stop := make(chan struct{})
	defer close(stop)
	for _, e := range events {
		go func(c <-chan sseEvent) {
			for {
				select {
				case <-stop:
					return
				case _, ok := <-c:
					if !ok {
						return
					}
				}
			}
		}(e)
	}
	if os.Getenv("WS_SEND_LATENCY_CAPTURE") != "" {
		cmd := exec.Command("node", "testdata/send-latency-capture.cjs", base, out)
		data, err := cmd.CombinedOutput()
		t.Log(string(data))
		if err != nil {
			t.Fatalf("capture: %v", err)
		}
	}
	cpu := func() float64 {
		var r syscall.Rusage
		if err := syscall.Getrusage(syscall.RUSAGE_SELF, &r); err != nil {
			t.Fatal(err)
		}
		return float64(r.Utime.Sec+r.Stime.Sec) + float64(r.Utime.Usec+r.Stime.Usec)/1e6
	}
	p, err := os.Create(filepath.Join(out, "daemon.pprof"))
	if err != nil {
		t.Fatal(err)
	}
	if err := pprof.StartCPUProfile(p); err != nil {
		t.Fatal(err)
	}
	h0, m0 := d.snapshots.Counts()
	r0 := d.stats.RailRenders.Load()
	s0 := d.stats.SessionRenders.Load()
	l0 := d.stats.LiveRenders.Load()
	start, c0 := time.Now(), cpu()
	for n := 0; n < 24; n++ {
		s, _ := d.index.Session("latency0")
		d.inboxChanged(s)
		d.broker.wake(gitDep(project)) // neither token changes rail output
		if n%4 == 0 {
			r := wslive.ObservedRecord{ModelObservation: wslive.ModelObservation{ID: fmt.Sprintf("new-%d", n), Model: "answer-model-1", Source: "pi.assistant", At: 1700004000000 + int64(n)}}
			raw, _ := json.Marshal(r)
			if err := os.WriteFile(filepath.Join(SessionDir(dir, "latency0"), wslive.ObservedDirName, fmt.Sprintf("new-%d.json", n)), raw, 0600); err != nil {
				t.Fatal(err)
			}
			// Model evidence is a real indexed store change, not inbox-only.
			src.set("latency0", SessionData{HasBoard: true, Fingerprint: fmt.Sprint(n), SnapshotUpdatedAt: now})
			d.index.Refresh()
		}
		time.Sleep(time.Until(start.Add(time.Duration(n+1) * 300 * time.Millisecond)))
	}
	elapsed, used := time.Since(start).Seconds(), cpu()-c0
	h1, m1 := d.snapshots.Counts()
	pprof.StopCPUProfile()
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("MEASURE sessions=7 records_per_session=4000 streams=3 ticks=24 interval_ms=300 inbox_wakes=24 git_wakes=24 new_evidence=6 wall_seconds=%.3f process_cpu_seconds=%.6f core_percent=%.3f snapshot_requests=%d snapshot_requests_per_second=%.3f rail_renders=%d session_renders=%d live_renders=%d", elapsed, used, 100*used/elapsed, h1+m1-h0-m0, float64(h1+m1-h0-m0)/elapsed, d.stats.RailRenders.Load()-r0, d.stats.SessionRenders.Load()-s0, d.stats.LiveRenders.Load()-l0)
}
