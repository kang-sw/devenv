package wswhiteboard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// The linked-ticket icon sits right after the waiting clock in the topic bar,
// stays inside the bar beside a long wrapping label, carries a count only from
// two tickets, adds no glow, and opens a popover listing each stem with its
// status, in both themes at 1440 and 700px. Needs WS_PLAYWRIGHT_MODULE;
// WS_TOPIC_TICKETS_SHOTS keeps the screenshots.
func TestTopicTicketIconBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	dir := t.TempDir()
	st := &State{}
	mustApply(t, st, `<wb:topic name="none" label="연관 티켓 없음"><wb:log>티켓 없이 기다리는 토픽</wb:log><wb:state value="waiting" on="exec:worker"/></wb:topic>`)
	mustApply(t, st, `<wb:topic name="one" label="기록 접기 규칙(N개 로그 항목)" tickets="261114-feat-history-fold-rule"><wb:log>후속 작업을 티켓으로 미룸</wb:log><wb:state value="waiting" on="exec:worker"/></wb:topic>`)
	mustApply(t, st, `<wb:topic name="three" label="화이트보드 토픽 상태와 리드 알림 정리 — 아주 긴 토픽 제목이 좁은 화면에서 줄바꿈되어도 아이콘이 밀려나지 않아야 함" tickets="261114-feat-ws-whiteboard-topic-state-and-lead-nudges 261113-bug-board-ticket-status-resolver 261112-refactor-retired-badge-glow"><wb:log>세 티켓을 연결함</wb:log><wb:state value="waiting" on="exec:worker"/></wb:topic>`)
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "tickets", Location: time.UTC, Tickets: fakeTickets{
		"261114-feat-history-fold-rule":                         TicketTodo,
		"261114-feat-ws-whiteboard-topic-state-and-lead-nudges": TicketReady,
		"261113-bug-board-ticket-status-resolver":               TicketDone,
		"261112-refactor-retired-badge-glow":                    TicketDropped,
	}})
	view.Lang = "ko"
	page, err := RenderPage(view)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "board.html")
	if err := os.WriteFile(path, []byte(page), 0o600); err != nil {
		t.Fatal(err)
	}
	shots := os.Getenv("WS_TOPIC_TICKETS_SHOTS")
	if shots == "" {
		shots = dir
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/topic-tickets.browser.cjs", path, shots)
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("topic ticket icon: %v", err)
	}
}
