package wswhiteboard

import (
	"strings"
	"testing"
)

// The renderer consumes the validated view field, never TopicDoc proof or
// overall activity. Store/proof acceptance is exercised by the spine tests.
func TestBodyTimeRenderSharesTheValidatedViewValue(t *testing.T) {
	const bodyAt = "2026-01-02T03:04:05Z"
	const activity = "2026-11-14T13:00:00Z"
	for _, at := range []string{bodyAt, ""} {
		for _, stale := range []bool{false, true} {
			view := View{Layout: LayoutFull, Lang: "ko", Topics: []TopicView{{
				Name: "body", Label: "Body", Doc: "first\n\nsecond\n\nlast",
				DocMarks:      []BlockMark{{0, MarkNew}, {1, MarkMod}, {2, MarkDel}, {3, MarkDel}},
				BodyUpdatedAt: at, DocStale: stale,
				UpdatedAt: activity, UpdatedMs: 1794661200000, UpdatedClock: "13:00",
			}}}
			page := render(t, view)
			regions, err := RenderRegions(view)
			if err != nil {
				t.Fatal(err)
			}
			board := string(regions.Board)
			frame := frameDocs(t, board)["body"]
			for _, out := range []string{markup(page), board} {
				wantAll(t, out, `class="tago" datetime="`+activity+`"`)
				if at != "" {
					wantAll(t, out, `data-body-at="`+bodyAt+`"`)
				} else {
					wantAll(t, out, `data-body-at=""`)
				}
				wantNone(t, out, `data-body-at="`+activity+`"`, `class="body-age"`, `>변경 시각 모름</span>`, "body_time_proof")
			}
			wantAll(t, frame, frameJSTag(t), `<!--wbchg:new-->`, `<!--wbchg:mod-->`, `<!--wbchg:del-->`, `<!--wbchg:delafter-->`)
			if at == "" {
				wantNone(t, frame, `name="wb-body-at"`, `data-body-at=`)
				wantAll(t, markup(page), `data-body-at=""`)
			} else {
				wantAll(t, frame, `<meta name="wb-body-at" data-body-at="`+bodyAt+`">`)
				if strings.Count(markup(page), `data-body-at=`) != 1 {
					t.Fatal("unframed body time must come from its one body metadata value")
				}
			}
			if stale {
				wantAll(t, frame, `class="doc-body md chg-stale"`)
			}
		}
	}
}

func TestBodyTimeFrameBundleOwnsItsFormatter(t *testing.T) {
	a, err := loadAssets()
	if err != nil {
		t.Fatal(err)
	}
	js, _, ok := FrameAsset(a.frameJSFile.name)
	if !ok {
		t.Fatal("shared frame script unavailable")
	}
	out := string(js)
	wantAll(t, out, "window.wbBodyTimeText = bodyText", "function wbChangeBars(", "pointerenter", "pointerleave")
	if strings.Index(out, "window.wbBodyTimeText = bodyText") > strings.Index(out, "function wbChangeBars(") {
		t.Fatal("frame formatter must precede gutter startup")
	}
	// The gutter never reads a parent clock, activity field, proof, or store.
	chg, err := embeddedAsset("chg.js")
	if err != nil {
		t.Fatal(err)
	}
	wantNone(t, string(chg), "window.parent", "data-at", "ChangedAt", "BodyTimeProof", "toLocale", "toISOString", ".focus(", "scrollIntoView")
}

func TestBodyTimeDoesNotAddAClockToEmbedOrBodylessTopics(t *testing.T) {
	view := View{Layout: LayoutFull, Topics: []TopicView{{Name: "empty", Label: "Empty", BodyUpdatedAt: "2026-01-01T00:00:00Z"}}}
	wantNone(t, markup(render(t, view)), `class="body-age"`)
	view.Layout = LayoutEmbed
	view.Topics[0].Doc = "body"
	wantNone(t, markup(render(t, view)), `class="body-age"`, `data-body-at=`)
}

func TestBodyTimeDeferredAndClosedRenderKeepTheSameSource(t *testing.T) {
	const at = "2026-01-02T03:04:05Z"
	topic := TopicView{Name: "closed", Label: "Closed", Doc: "body", Closed: true, Quiet: true, BodyUpdatedAt: at}
	view := View{Layout: LayoutFull, DeferredTopics: []TopicView{topic}}
	regions, err := RenderRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	wantAll(t, string(regions.Board), `data-history-topic="closed"`, `class="tdoc" data-body-at="`+at+`"`)
	wantNone(t, string(regions.Board), `class="body-age"`, `>변경 시각 모름</span>`)
	view.Topics, view.DeferredTopics = view.DeferredTopics, nil
	closed, err := RenderClosedTopics(view)
	if err != nil {
		t.Fatal(err)
	}
	wantAll(t, frameDocs(t, string(closed))["closed"], `name="wb-body-at" data-body-at="`+at+`"`)
}
