package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
	"github.com/kang-sw/devenv/internal/wstrackertest"
)

// BenchmarkDashboardTrackerProjectionMillion is opt-in; all results are UNRUN.
// Authorized benchmark host only, from agents-plugin-tool:
//
// GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./cmd/ws-mcp -run '^$' -bench '^BenchmarkDashboardTrackerProjectionMillion$' -benchtime=1x -count=1 -benchmem
//
// These are actual authenticated /view, /summary and /delta HANDLER routes,
// not projection/encoding-only stages and NOT end-to-end socket HTTP latency.
// ns/op includes arrival + daemon invalidation + synchronous HTTP handler:
// cursor Refresh (or original zero-fold Derive), view, production projection
// and diff/ring publication for summary/delta, rendering/tagging for /view,
// response encoding and writes to ResponseRecorder. response-body-bytes/op
// excludes headers, compression, network transfer and browser work. Request,
// recorder and baseline setup are outside the timer; response buffer growth
// is inside. No cached-304 hit is counted as a changed response.
//
// Source is memory-only; the embedded host supplies minimal store/clone/sync
// metadata instead of wstrackerhost's git/registry/ledger machinery. The real
// daemon creates temporary lock/auth state and a loopback listener, but no
// request uses its socket, no session/agent/holder is registered, and no git or
// external network is needed. Auth uses LoginLink's real cookie exchange, not
// the endpoint bearer token (which does NOT authorize tracker routes).
//
// Every operation starts from the same million-event baseline with timers
// OFF for fixture generation, initial Tracker.New cold fold and the baseline
// route/cache/renders/cursor. Checkpoint construction is intentionally excluded
// from changed-case timing; see BenchmarkTrackerColdMillion for that cost.
// Checkpoint count is bounded, but copies of fold-owned slices and raw million
// event views can still consume substantial memory. B/op is TIMED allocation,
// not peak RSS or retained checkpoint/fixture memory. Use -benchtime=1x first.
func BenchmarkDashboardTrackerProjectionMillion(b *testing.B) {
	for _, items := range []int{10_000, 30_000} {
		b.Run(fmt.Sprintf("items-%d", items), func(b *testing.B) {
			b.StopTimer()
			fixture := wstrackertest.NewScale(items, wstrackertest.MillionEvents)
			for _, kind := range []string{wstrackertest.Tail, wstrackertest.NearTail, wstrackertest.FarBack} {
				for _, mode := range []string{"before-zero-fold-view", "checkpoint-view", "checkpoint-summary", "checkpoint-delta"} {
					b.Run(kind+"/"+mode, func(b *testing.B) {
						b.StopTimer()
						b.ReportAllocs()
						host := &millionTrackerHost{fixture: fixture, fullZero: mode == "before-zero-fold-view"}
						d, err := wsdashboard.StartDaemon(wsdashboard.DaemonOptions{
							Dir: b.TempDir(), Home: b.TempDir(), Port: 0,
							Version: "benchmark-only", Tracker: host,
						})
						if err != nil {
							b.Fatal(err)
						}
						b.Cleanup(d.Close)
						handler := d.Handler()
						cookie := millionTrackerCookie(b, d, handler)
						origin := fmt.Sprintf("http://127.0.0.1:%d", d.Endpoint().Port)
						viewMode := mode == "before-zero-fold-view" || mode == "checkpoint-view"
						var responseBytes int64
						b.ResetTimer() // preserve stopped state; exclude daemon/auth setup
						for range b.N {
							host.source = fixture.Source()
							host.last = nil
							host.tracker = nil
							if !host.fullZero {
								host.tracker = wstracker.New(host.source, wstrackertest.Stream)
							}
							d.TrackerChanged()
							basePath := "/tracker/stores/scale/summary"
							if viewMode {
								basePath = "/tracker/stores/scale/view"
							}
							base := httptest.NewRecorder()
							handler.ServeHTTP(base, millionTrackerRequest(origin+basePath, cookie))
							if base.Code != http.StatusOK {
								b.Fatalf("baseline status %d", base.Code)
							}
							host.check(b, 0)
							path := basePath
							if !viewMode {
								answer := millionSummaryResponse(b, base)
								if !answer.Reset || len(answer.Items) != items || answer.Cursor == "" {
									b.Fatal("baseline did not return a complete summary/cursor")
								}
								if mode == "checkpoint-delta" {
									path = "/tracker/stores/scale/delta?since=" + url.QueryEscape(answer.Cursor)
								}
							}
							// Do not retain the baseline's potentially huge encoded
							// body alongside the changed response in this harness.
							base = nil
							change := fixture.Change(kind)
							req := millionTrackerRequest(origin+path, cookie)
							rec := httptest.NewRecorder()
							b.StartTimer()
							host.source.Arrive(change)
							d.TrackerChanged()
							handler.ServeHTTP(rec, req)
							b.StopTimer()
							if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
								b.Fatalf("changed route status=%d bytes=%d", rec.Code, rec.Body.Len())
							}
							host.check(b, 1)
							if !viewMode {
								answer := millionSummaryResponse(b, rec)
								if mode == "checkpoint-delta" {
									if _, ok := answer.Items["root.pc#1"]; !ok || answer.Reset || len(answer.Items) != 1 || len(answer.Removed) != 0 {
										b.Fatalf("expected one-item delta, reset=%v items=%d removed=%d", answer.Reset, len(answer.Items), len(answer.Removed))
									}
								} else if !answer.Reset || len(answer.Items) != items {
									b.Fatal("changed summary was incomplete")
								}
							}
							responseBytes += int64(rec.Body.Len())
						}
						b.ReportMetric(float64(responseBytes)/float64(b.N), "response-body-bytes/op")
						b.ReportMetric(float64(len(fixture.Events)), "baseline-events")
					})
				}
			}
		})
	}
}

// Embedding the interface deliberately leaves unrelated endpoints unsupported;
// these read routes must use ONLY the three overridden methods. No real host,
// repo, process runner, registry, or network remote is constructed.
type millionTrackerHost struct {
	wsdashboard.TrackerHost
	fixture  *wstrackertest.Scale
	source   *wstrackertest.Source
	tracker  *wstracker.Tracker
	fullZero bool
	renders  viewRenders
	last     *wstracker.View
}

var (
	_ wsdashboard.TrackerHost           = (*millionTrackerHost)(nil)
	_ wsdashboard.TrackerProjectionHost = (*millionTrackerHost)(nil)
	_ wstracker.Source                  = (*wstrackertest.Source)(nil)
)

func (h *millionTrackerHost) Stores() ([]wsdashboard.TrackerStore, error) {
	return []wsdashboard.TrackerStore{{ID: "scale", Label: "Scale", Member: "root"}}, nil
}

func (h *millionTrackerHost) storeView() *wstrackerhost.StoreView {
	if h.fullZero {
		h.last = wstracker.Derive(h.source, wstrackertest.Stream, h.fixture.Now, 0)
	} else {
		// View calls Refresh itself: the changed arrival must be consumed
		// INSIDE the timed route, not pre-folded by benchmark setup.
		h.last = h.tracker.View(h.fixture.Now, 0)
	}
	clones := []wstrackerhost.CloneInfo{{Stream: wstrackertest.Stream}}
	if _, ok := h.last.Heads["root.remote"]; ok {
		clones = append(clones, wstrackerhost.CloneInfo{Stream: "root.remote"})
	}
	return &wstrackerhost.StoreView{View: h.last,
		Store:  wstrackerhost.StoreInfo{Name: "scale", Label: "Scale"},
		Sync:   wstrackerhost.SyncInfo{Unpushed: []wstrackerhost.UnpushedCommit{}},
		Clones: map[string][]wstrackerhost.CloneInfo{"root": clones},
	}
}

func (h *millionTrackerHost) View(_ context.Context, id string) (wsdashboard.TrackerView, error) {
	v := h.storeView()
	v.Redact()
	html := h.renders.view(id, v)
	raw, err := json.Marshal(dashboardView{StoreView: v, HTML: html})
	if err != nil {
		return wsdashboard.TrackerView{}, err
	}
	tag, err := dashboardViewTag(v, html)
	return wsdashboard.TrackerView{View: raw, Tag: tag}, err
}

func (h *millionTrackerHost) Projection(_ context.Context, id string) (wsdashboard.TrackerProjection, error) {
	return projectTracker(id, h.storeView(), wsdashboard.TrackerSeen{})
}

func (h *millionTrackerHost) check(b *testing.B, added int) {
	b.Helper()
	v := h.last
	if v == nil || len(v.Items) != h.fixture.ItemCount || v.Stats.Events != len(h.fixture.Events)+added || v.Stats.Flagged != 0 {
		b.Fatal("route failed to derive the expected unflagged store revision")
	}
}

func millionTrackerRequest(target string, cookie *http.Cookie) *http.Request {
	r := httptest.NewRequest(http.MethodGet, target, nil)
	r.RemoteAddr = "127.0.0.1:50000"
	if cookie != nil {
		r.AddCookie(cookie)
	}
	return r
}

func millionTrackerCookie(b *testing.B, d *wsdashboard.Daemon, handler http.Handler) *http.Cookie {
	b.Helper()
	link, err := d.LoginLink("/")
	if err != nil {
		b.Fatal(err)
	}
	req := millionTrackerRequest(link, nil)
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	result := rec.Result()
	defer result.Body.Close()
	cookies := result.Cookies()
	if rec.Code != http.StatusSeeOther || len(cookies) != 1 || !cookies[0].HttpOnly {
		b.Fatal("real localhost login navigation did not issue its cookie")
	}
	return cookies[0]
}

type millionSummaryAnswer struct {
	Cursor  string                     `json:"cursor"`
	Reset   bool                       `json:"reset"`
	Items   map[string]json.RawMessage `json:"items"`
	Removed []string                   `json:"removed"`
}

func millionSummaryResponse(b *testing.B, rec *httptest.ResponseRecorder) millionSummaryAnswer {
	b.Helper()
	var answer millionSummaryAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
		b.Fatal(err)
	}
	return answer
}
