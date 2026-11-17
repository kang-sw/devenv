package wsdashboard

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// No wall-clock timers: the test explicitly closes each notification window.
func manualWakeBroker() (*broker, *[]func()) {
	b := newBroker(&streamStats{})
	var timers []func()
	b.after = func(_ time.Duration, f func()) { timers = append(timers, f) }
	return b, &timers
}

func TestWakeBurstDeduplicatesNotificationsWithoutDroppingChanges(t *testing.T) {
	const subscribers, burst = 16, 100
	// Execute the old wake delivery path with responsive subscribers. This
	// measures the baseline rather than calculating subscribers * burst.
	legacy, _ := manualWakeBroker()
	var legacySubs []*subscription
	for range subscribers {
		sub, cancel := legacy.subscribe()
		defer cancel()
		legacySubs = append(legacySubs, sub)
	}
	for range burst {
		legacy.mu.Lock()
		legacy.seq++
		legacy.changed["session"] = legacy.seq
		legacy.anyWake = legacy.seq
		legacy.signalTokenLocked("session")
		legacy.mu.Unlock()
		for _, sub := range legacySubs {
			<-sub.ch
		}
	}
	before := legacy.stats.Signals.Load()
	if before != burst*subscribers {
		t.Fatalf("legacy notifications = %d", before)
	}

	b, timers := manualWakeBroker()
	var subs []*subscription
	for range subscribers {
		sub, cancel := b.subscribe()
		defer cancel()
		b.follow(sub, []string{"session"}, b.current())
		subs = append(subs, sub)
	}
	other, cancel := b.subscribe()
	defer cancel()
	b.follow(other, []string{"other"}, b.current())

	for range burst {
		b.wake("session")
		// Model fast subscribers consuming every notification, not channels
		// that accidentally hide the old per-wake fanout by staying full.
		for _, sub := range subs {
			select {
			case <-sub.ch:
			default:
			}
		}
	}
	if got := b.stats.Signals.Load(); got != subscribers {
		t.Fatalf("leading notifications = %d, want %d", got, subscribers)
	}
	if b.current() != burst || b.unchangedSince([]string{"session"}, 1) {
		t.Fatal("dedupe suppressed invalidation of a later change")
	}
	if len(*timers) != 1 {
		t.Fatalf("burst scheduled %d timers, want one", len(*timers))
	}
	(*timers)[0]()
	for _, sub := range subs {
		select {
		case <-sub.ch:
		default:
			t.Fatal("last change lost its trailing notification")
		}
	}
	select {
	case <-other.ch:
		t.Fatal("dedupe broadened session dependencies")
	default:
	}
	after := b.stats.Signals.Load()
	if after != 2*subscribers {
		t.Fatalf("total notifications = %d, want %d", after, 2*subscribers)
	}
	b.wake("session")
	if got := b.stats.Signals.Load(); got != 3*subscribers {
		t.Fatalf("next window lost leading edge: %d", got)
	}
	(*timers)[1]()
	if got := b.stats.Signals.Load(); got != 3*subscribers {
		t.Fatalf("single wake gained a redundant trailing edge: %d", got)
	}
	t.Logf("%d wakes x %d responsive subscribers: notifications %d -> %d (leading + trailing)", burst, subscribers, before, after)
}

func TestWakeWindowImmediatelyNotifiesNewSubscriber(t *testing.T) {
	b, timers := manualWakeBroker()
	old, cancelOld := b.subscribe()
	defer cancelOld()
	b.wake("session")
	<-old.ch
	newSub, cancelNew := b.subscribe()
	defer cancelNew()
	// A startup index read can open a window before the first browser joins.
	// The browser must still hear its first actual change synchronously.
	b.wake("session")
	select {
	case <-newSub.ch:
	default:
		t.Fatal("new subscriber's first change was deferred to the trailing edge")
	}
	select {
	case <-old.ch:
		t.Fatal("old subscriber bypassed wake dedupe")
	default:
	}
	b.wake("session")
	select {
	case <-newSub.ch:
		t.Fatal("new subscriber did not join the current dedupe window")
	default:
	}
	(*timers)[0]()
	for _, sub := range []*subscription{old, newSub} {
		select {
		case <-sub.ch:
		default:
			t.Fatal("subscriber missed the final change")
		}
	}
}

func fakeRenderClock(c *pageCache) *time.Time {
	now := time.Unix(100, 0)
	c.now = func() time.Time { return now }
	c.wait = func(delay time.Duration) { now = now.Add(delay) }
	return &now
}

func TestWakeBurstRendersOncePerViewWithManySubscribers(t *testing.T) {
	const subscribers, burst = 16, 100
	b, _ := manualWakeBroker()
	c := newPageCache(b, b.stats)
	fakeRenderClock(c)
	var pages []*sharedPage
	var sources []pageSource
	renders := map[string]int{}
	for _, kind := range []string{"session", regionRail} {
		kind := kind
		src := pageSource{
			key:   pageKey{kind: kind},
			stamp: func(refreshInputs) (string, bool) { return "same", true },
			render: func(*liveRender) pageRender {
				renders[kind]++
				return pageRender{ok: true, regions: regionSet{}, deps: []string{"session", ringDep("session")}}
			},
		}
		p := c.acquire(src.key)
		defer c.release(src.key, p)
		c.get(p, src, false)
		pages, sources = append(pages, p), append(sources, src)
	}
	for range burst {
		// The agent hook and index hook name the same session; hosted ring
		// appends use a separate token and must remain distinguishable.
		b.wake("session")
		b.wake("session")
		b.wake(ringDep("session"))
	}
	for range subscribers {
		for i, p := range pages {
			sub, cancel := b.subscribe()
			r, at := c.get(p, sources[i], false)
			b.follow(sub, r.deps, at)
			cancel()
		}
	}
	for kind, count := range renders {
		if count != 2 { // initial render plus one burst render
			t.Errorf("%s burst renders = %d, want 1", kind, count-1)
		}
	}
	t.Logf("%d subscribers, %d agent/index/ring burst changes: session=%d render, rail=%d render", subscribers, burst, renders["session"]-1, renders[regionRail]-1)
}

func TestInFlightWakeSharesRenderAndPreservesOnePendingUpdate(t *testing.T) {
	const subscribers = 16
	for _, token := range []string{"session", liveDep("session"), ringDep("session"), inboxDep("session")} {
		t.Run(token, func(t *testing.T) {
			// Execute the old serialized cache's wake/currentness branch:
			// each queued reader starts another render when a change lands
			// after that render sampled its sequence. Each render below has
			// one such in-flight change, just as the new-cache case does.
			legacy, _ := manualWakeBroker()
			legacyPage := &sharedPage{rendered: true, r: pageRender{deps: []string{token}}}
			before := 0
			legacy.wake(token)
			for range subscribers {
				legacyPage.mu.Lock()
				if !legacy.unchangedSince(legacyPage.r.deps, legacyPage.at) {
					legacyPage.at = legacy.current()
					before++
					legacy.wake(token) // write during the read/render
				}
				legacyPage.mu.Unlock()
			}
			if before != subscribers {
				t.Fatalf("legacy queued renders = %d", before)
			}

			b, _ := manualWakeBroker()
			c := newPageCache(b, b.stats)
			now := fakeRenderClock(c)
			p := c.acquire(pageKey{replyID: "session"})
			defer c.release(pageKey{replyID: "session"}, p)
			value, renders := 0, 0
			storm := false
			started, finish := make(chan struct{}), make(chan struct{})
			src := pageSource{
				stamp: func(refreshInputs) (string, bool) { return "same", true },
				render: func(*liveRender) pageRender {
					renders++
					r := pageRender{ok: true, deps: []string{"session", token}, regions: regionSet{
						regionHead: newRegion(fmt.Sprint(value)),
					}, reads: struct{}{}}
					if storm {
						if renders == 2 {
							close(started)
							<-finish
						}
						value++ // a change after this render already read its inputs
						b.wake(token)
					}
					return r
				},
			}
			c.get(p, src, false)
			b.wake(token)
			storm = true
			// All these demands sampled the same completed generation before
			// queuing behind the in-flight render. Pass that sample explicitly
			// so the assertion does not depend on goroutine scheduling/sleeps.
			generation := p.generation.Load()
			type result struct {
				r  pageRender
				at uint64
			}
			results := make(chan result, subscribers)
			var callers sync.WaitGroup
			get := func() {
				defer callers.Done()
				r, at := c.getSince(p, src, false, generation)
				results <- result{r, at}
			}
			callers.Add(subscribers)
			go get()
			<-started
			for range subscribers - 1 {
				go get()
			}
			close(finish)
			callers.Wait()
			close(results)
			after := renders - 1
			if after != 1 {
				t.Fatalf("queued burst renders = %d, want 1", after)
			}
			for got := range results {
				if b.unchangedSince(got.r.deps, got.at) {
					t.Fatal("change during render incorrectly marked current")
				}
				sub, cancel := b.subscribe()
				b.follow(sub, got.r.deps, got.at)
				select {
				case <-sub.ch:
				default:
					t.Fatal("follow failed to re-arm the pending update")
				}
				cancel()
			}
			storm = false
			waits := 0
			c.wait = func(delay time.Duration) {
				waits++
				if delay != sseMinInterval {
					t.Errorf("pending delay = %s", delay)
				}
				*now = now.Add(delay)
				value++ // a final change while the pending render is coalescing
				b.wake(token)
			}
			for range subscribers {
				r, at := c.get(p, src, false)
				if string(r.regions[regionHead].HTML) != "2" || !b.unchangedSince(r.deps, at) {
					t.Fatalf("pending render lost final change: %+v at %d", r, at)
				}
			}
			if renders != 3 || waits != 1 {
				t.Fatalf("pending: renders=%d waits=%d, want one in-flight + one pending", renders-1, waits)
			}
			t.Logf("%d queued subscribers: in-flight renders %d -> %d; one pending render observes final value", subscribers, before, after)
		})
	}
}
