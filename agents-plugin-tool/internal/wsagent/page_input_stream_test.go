package wsagent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// Exercise the real runner's inbox drain and the dashboard's SSE subscription
// together: the post first renders held, then delivery removes that bubble.
func TestHostedPageInputDrainUpdatesStream(t *testing.T) {
	h, daemon, post := hostedPageInputHarness(t)
	id := h.lead(BackendClaude, LeadOptions{}).ID
	eventually(t, "hosted index row", func() bool {
		s, ok := daemon.Index().Session(id)
		return ok && s.Hosted
	})
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/s/%s/events", daemon.Endpoint().Port, id), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(daemonCookieForTest(t, daemon))
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close(); client.CloseIdleConnections() })
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %s", resp.Status)
	}
	type region struct{ Name, HTML string }
	regions := make(chan region, 128)
	go func() {
		defer close(regions)
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 4096), 2<<20)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var r region
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &r) == nil && r.Name != "" {
				select {
				case regions <- r:
				default:
				}
			}
		}
	}()
	waitRegion := func(label string, match func(region) bool) {
		t.Helper()
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		for {
			select {
			case r, ok := <-regions:
				if !ok {
					t.Fatalf("stream closed waiting for %s", label)
				}
				if match(r) {
					return
				}
			case <-timer.C:
				t.Fatalf("timed out waiting for %s", label)
			}
		}
	}
	// Ensure the initial stream is subscribed before the post.
	waitRegion("initial conversation", func(r region) bool { return r.Name == "convlive" })
	c := h.runner.conv(id)
	entered, gate := make(chan struct{}), make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(gate) }) }
	defer release()
	go c.do(func() { close(entered); <-gate })
	wait(t, "blocked conversation loop", entered)
	out := post(id)
	if out.Code != http.StatusOK {
		t.Fatalf("page send = %d %s", out.Code, out.Body)
	}
	waitRegion("held message", func(r region) bool {
		return r.Name == "convlive" && strings.Contains(r.HTML, "page message") && strings.Contains(r.HTML, "msg user held")
	})
	release()
	removed, delivered := false, false
	waitRegion("held removal and delivery", func(r region) bool {
		removed = removed || r.Name == "convlive" && !strings.Contains(r.HTML, "msg user held")
		delivered = delivered || r.Name == "entries" && strings.Contains(r.HTML, "page message")
		return removed && delivered
	})
}
