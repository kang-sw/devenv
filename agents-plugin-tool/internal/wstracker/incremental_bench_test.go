package wstracker

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstrackertest"
)

// These are opt-in benchmarks, NOT test/init fixtures. Results are UNRUN.
// Run only on an authorized benchmark host, from agents-plugin-tool:
//
// GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wstracker -run '^$' -bench '^BenchmarkTrackerIncrementalMillion$' -benchtime=1x -count=1 -benchmem
// GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wstracker -run '^$' -bench '^BenchmarkTrackerColdMillion$' -benchtime=1x -count=1 -benchmem
//
// Each changed-case iteration starts from the SAME million-event baseline;
// fixture generation, baseline replay and baseline view are not timed. Arrival,
// cursor read, replay/update AND view derivation are timed. In particular,
// replay's verdicts and update's verdictsFrom both execute stateBefore/check;
// this is not an apply-only suffix microbenchmark. "before" is the untracked
// replay() zero-fold oracle, not a reset that also builds new checkpoints.
//
// Checkpoints are bounded in NUMBER, not bytes: state copies retain item,
// register, assignment and history slices. Building them every stride can be
// expensive even with only 16 retained. Neither setup cost nor peak retained
// memory is represented by changed-case ns/op/B/op; cold-fold rows include
// checkpoint construction, and replayed-events/op exposes suffix length.
func BenchmarkTrackerIncrementalMillion(b *testing.B) {
	for _, items := range []int{10_000, 30_000} {
		b.Run(fmt.Sprintf("items-%d", items), func(b *testing.B) {
			b.StopTimer()
			fixture := wstrackertest.NewScale(items, wstrackertest.MillionEvents)
			for _, kind := range []string{wstrackertest.Tail, wstrackertest.NearTail, wstrackertest.FarBack} {
				for _, tracked := range []bool{false, true} {
					mode := "before-full-zero-fold-and-view"
					if tracked {
						mode = "after-checkpoint-and-view"
					}
					b.Run(kind+"/"+mode, func(b *testing.B) {
						b.StopTimer()
						b.ReportAllocs()
						var replayedEvents, retainedCheckpoints int64
						b.ResetTimer() // preserve stopped state; exclude initial harness setup
						for range b.N {
							src := fixture.Source()
							base, cursor := src.Events(wsledger.Cursor{})
							r := replayBuild(base, fixture.Genesis, src.Concurrent, false, tracked)
							checkMillionView(b, fixture, r.view(wstrackertest.Stream, fixture.Now, 0), 0)
							change := fixture.Change(kind)
							b.StartTimer()
							src.Arrive(change)
							if tracked {
								changed, _ := src.Events(cursor)
								r.update(changed)
							} else {
								all, _ := src.Events(wsledger.Cursor{})
								r = replay(all, fixture.Genesis, src.Concurrent)
							}
							v := r.view(wstrackertest.Stream, fixture.Now, 0)
							b.StopTimer()
							checkMillionView(b, fixture, v, 1)
							n := r.byID[change.ID.String()]
							if n == nil || n.status != "applied" {
								b.Fatal("arriving event was not applied")
							}
							// Pin the intended location, not merely arrival order.
							wantPos := len(fixture.Events)
							if kind == wstrackertest.NearTail {
								wantPos -= 512
							} else if kind == wstrackertest.FarBack {
								wantPos = 1
							}
							if n.pos != wantPos {
								b.Fatalf("arrival position = %d, want %d", n.pos, wantPos)
							}
							from := 0
							if tracked {
								from = r.memo.replayedFrom
								retainedCheckpoints += int64(len(r.memo.checkpoints))
								if len(r.memo.checkpoints) > foldCheckpointLimit {
									b.Fatal("checkpoint limit exceeded")
								}
								if from > wantPos || (kind == wstrackertest.Tail && from != wantPos) ||
									(kind == wstrackertest.NearTail && from == 0) || (kind == wstrackertest.FarBack && from != 0) {
									b.Fatalf("unexpected checkpoint restart %d for insertion %d", from, wantPos)
								}
							}
							replayedEvents += int64(len(r.order) - from)
							runtime.KeepAlive(r)
						}
						b.ReportMetric(float64(replayedEvents)/float64(b.N), "replayed-events/op")
						b.ReportMetric(float64(retainedCheckpoints)/float64(b.N), "checkpoints/op")
						b.ReportMetric(float64(len(fixture.Events)), "baseline-events")
					})
				}
			}
		})
	}
}

// Cold rows deliberately include node construction, causal ordering, actual
// stateBefore verdict checks and the complete fold; no view/JSON stage here.
// They distinguish the original zero fold from checkpoint-building cold start.
func BenchmarkTrackerColdMillion(b *testing.B) {
	for _, items := range []int{10_000, 30_000} {
		b.Run(fmt.Sprintf("items-%d", items), func(b *testing.B) {
			b.StopTimer()
			fixture := wstrackertest.NewScale(items, wstrackertest.MillionEvents)
			for _, tracked := range []bool{false, true} {
				mode := "before-full-zero-fold"
				if tracked {
					mode = "after-checkpoint-cold-fold"
				}
				b.Run(mode, func(b *testing.B) {
					b.StopTimer()
					b.ReportAllocs()
					b.ResetTimer() // preserve stopped state
					for range b.N {
						src := fixture.Source()
						b.StartTimer()
						r := replayBuild(fixture.Events, fixture.Genesis, src.Concurrent, false, tracked)
						b.StopTimer()
						if len(r.order) != len(fixture.Events) || len(r.S.items) != items {
							b.Fatal("cold fold lost events/items")
						}
						for _, n := range r.order {
							if n.status != "applied" {
								b.Fatalf("cold fold flagged %s: %s", n.id, n.reason)
							}
						}
						runtime.KeepAlive(r)
					}
					b.ReportMetric(float64(len(fixture.Events)), "folded-events/op")
				})
			}
		})
	}
}

func checkMillionView(b *testing.B, fixture *wstrackertest.Scale, v *View, added int) {
	b.Helper()
	if len(v.Items) != fixture.ItemCount || v.Stats.Events != len(fixture.Events)+added || v.Stats.Flagged != 0 {
		b.Fatalf("invalid fixture view: items=%d stats=%+v", len(v.Items), v.Stats)
	}
}
