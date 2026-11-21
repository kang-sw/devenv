package wsdashboard

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
	"golang.org/x/net/websocket"
)

// TestBoardChatSmallUXBrowser runs isolated Chromium acceptance for the three
// small board/chat UX features (chat top-left jump arrow, in-card Ctrl/Cmd+F
// find bar across host DOM and sandboxed doc frames, and browser-local hold
// with the held pill category and fingerprint auto-clear). Set
// WS_PLAYWRIGHT_MODULE to execute and optionally WS_UX_CAPTURES to write
// light/dark before/after PNGs.
func TestBoardChatSmallUXBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run isolated Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	captures := os.Getenv("WS_UX_CAPTURES")
	if captures == "" {
		captures = t.TempDir()
	}
	if err := os.MkdirAll(captures, 0o755); err != nil {
		t.Fatal(err)
	}
	captures, err = filepath.Abs(captures)
	if err != nil {
		t.Fatal(err)
	}

	buildBoard := func(edited bool) (*wswhiteboard.State, error) {
		st := &wswhiteboard.State{}
		apply := func(cmd string) error {
			_, probs := wswhiteboard.Apply(st, cmd, wswhiteboard.Env{Now: time.Date(2026, 11, 21, 18, 0, 0, 0, time.UTC)})
			if len(probs) != 0 {
				return fmt.Errorf("apply: %v", probs)
			}
			return nil
		}
		if err := apply(`<wb:topic name="rollout-checklist"><wb:log>Prepared rollout stages and verification notes.</wb:log>` +
			`<wb:panel recap="Cache migration checklist"><p>Verify the cache key format before deployment. Each cache entry uses a compact hash so cache misses stay bounded across reloads.</p><p>Second section: inspect cache invalidation when an agent edits an ask.</p></wb:panel>` +
			`<wb:add class="policy"><wb:title>Approve cache retention window</wb:title><wb:why>Controls how long browser-local entries survive reloads.</wb:why><wb:option>Keep 7 days</wb:option><wb:option>Keep 30 days</wb:option><wb:recommend>a: matches standard retention</wb:recommend></wb:add>` +
			`<wb:add class="announced"><wb:title>Release notes draft ready for review</wb:title><wb:why>No backend schema or MCP change.</wb:why></wb:add></wb:topic>`); err != nil {
			return nil, err
		}
		askTitle := "Pick rollout cadence for patch release"
		if edited {
			askTitle = "Pick rollout cadence for patch release (updated scope)"
		}
		if err := apply(fmt.Sprintf(`<wb:topic name="release-timing"><wb:log>Drafted timeline for the patch release.</wb:log>`+
			`<wb:log>Waiting on owner preference for canary timing.</wb:log>`+
			`<wb:add class="policy"><wb:title>%s</wb:title><wb:why>Canary runs for 2 hours before full rollout.</wb:why><wb:option>Immediate</wb:option><wb:option>Next morning</wb:option></wb:add>`+
			`<wb:add class="policy"><wb:title>Confirm changelog wording</wb:title><wb:option>Ship as written</wb:option><wb:option>Shorten summary</wb:option></wb:add></wb:topic>`, askTitle)); err != nil {
			return nil, err
		}
		if err := apply(`<wb:topic name="follow-up-notes"><wb:log>Collected remaining cleanup notes for after the release.</wb:log>` +
			`<wb:log>Verified light and dark theme contrast tokens.</wb:log>` +
			`<wb:log>No server or store migrations required.</wb:log>` +
			`<wb:add class="technical"><wb:title>Acknowledge local-only storage posture</wb:title><wb:why>State stays in browser localStorage only.</wb:why></wb:add></wb:topic>`); err != nil {
			return nil, err
		}
		return st, nil
	}

	ring := []wslive.RingRecord{
		ringRecord(1, wslive.RingEntry{Kind: wslive.RingUser, Origin: wslive.OriginWhiteboard, Text: "Start by checking the rollout plan and summarizing what needs review."}),
		ringRecord(2, wslive.RingEntry{Kind: wslive.RingReply, Text: "I reviewed the rollout checklist and posted the cache migration panel on the board.\n\n- Cache key format verified\n- Retention options added as a policy ask\n- Release notes draft posted for acknowledgement\n\nLet me know if you want any adjustments before proceeding."}),
		ringRecord(3, wslive.RingEntry{Kind: wslive.RingUser, Origin: wslive.OriginWhiteboard, Text: "Looks solid. Please check the theme tokens and post a detailed verification breakdown below."}),
		ringRecord(4, wslive.RingEntry{Kind: wslive.RingReply, Text: "Here is the detailed verification breakdown across layout, contrast, and interaction boundaries:\n\n1. Light and dark surfaces reuse the board custom properties (`--wb-card`, `--wb-muted`, `--wb-policy`, `--wb-line`).\n2. Scroll handlers coalesce into a single animation frame with read-before-write discipline.\n3. No backend store, MCP tool, or tracker event changes are introduced.\n4. Sandboxed panel bodies stay isolated in opaque-origin frames and communicate through authenticated `postMessage`."}),
		ringRecord(5, wslive.RingEntry{Kind: wslive.RingReply, Text: "Additional verification notes:\n\n- Top-left chat jump arrow stays clear of the top-right agent island.\n- In-card find bar intercepts Ctrl/Cmd+F only while focus or pointer is inside a board card.\n- Enter and Shift+Enter cycle forward and backward through matches; Escape closes the bar and restores clean text nodes.\n- Browser-local holds persist in `localStorage` keyed by board identity and item ID with a content fingerprint.\n- Editing or re-asking an item clears its hold automatically."}),
		ringRecord(6, wslive.RingEntry{Kind: wslive.RingReply, Text: "Final status summary:\n\nAll front-end asset checks are green. Ready for visual review across both light and dark themes, including before/after interaction states for chat jump, in-card find, and held ask pill counts."}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/s/uxdemo", func(w http.ResponseWriter, r *http.Request) {
		theme := r.URL.Query().Get("theme")
		if theme != "dark" {
			theme = "light"
		}
		edited := r.URL.Query().Get("edited") == "1"
		st, err := buildBoard(edited)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		wbView := wswhiteboard.BuildView(st, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull, ReplyID: "uxdemo"})
		wbView.Lang = "en"
		wbReg, err := wswhiteboard.RenderRegions(wbView)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		hView := historyView(ring, "", PageContent{}, sessionLookup{})
		v := SessionView{
			ReplyID:          "uxdemo",
			Label:            "board-chat-ux",
			Adapter:          true,
			Live:             true,
			HasBoard:         true,
			BoardIncarnation: "2026-11-21T18:00:00Z",
			Seq:              st.Seq,
			Board:            wbReg.Board,
			Panels:           wbReg.Panels,
			Conversation:     hView.Conversation,
			Lang:             "en",
			Theme:            theme,
		}
		regions, err := renderSessionRegions(v, nil)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		page, err := renderSessionPage(v, regions, false, "")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, page)
	})
	mux.Handle("/s/uxdemo/events", websocket.Handler(func(c *websocket.Conn) {
		var msg string
		for websocket.Message.Receive(c, &msg) == nil {
		}
	}))
	mux.HandleFunc("/s/uxdemo/", func(w http.ResponseWriter, r *http.Request) {
		if data, ct, ok := wswhiteboard.FrameAsset(path.Base(r.URL.Path)); ok {
			w.Header().Set("Content-Type", ct)
			w.Write(data)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if data, ct, ok := wswhiteboard.FrameAsset(path.Base(r.URL.Path)); ok {
			w.Header().Set("Content-Type", ct)
			w.Write(data)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{}`)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/board-chat-small-ux.browser.cjs", server.URL, captures)
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("board chat small ux browser: %v", err)
	}
}
