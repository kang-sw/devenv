package wswhiteboard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// panelVersionsState is the approved mockup's "mixed" topic: implementation
// v1 folds in place, a check ask and its answer follow, and v2 is the latest.
func panelVersionsState(t *testing.T) *State {
	t.Helper()
	st := &State{}
	day := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	for _, step := range []struct {
		minute   int
		commands string
	}{
		{0, `<wb:topic name="auth-profile" label="권한 프로필"><wb:log>백엔드 계약 정리 시작</wb:log></wb:topic>`},
		{5, panelCommandXML("auth-profile", `name="implementation" recap="구현 현황"`, "### 구현 현황\n\n- 지원 계약: 완료\n- 생성 UI: 대기\n- 설정 UI: 대기\n")},
		{12, logXML("auth-profile", "생성 폼 기본값을 정해야 함") + addXML(ClassCheck, "auth-profile", "생성 기본값을 '기본(기존 동작)'으로 둘까요?")},
		{30, `<wb:answer id="1" value="ok">ok</wb:answer>`},
		{41, logXML("auth-profile", "기본값 확정, 생성 UI 작업 진행")},
		{55, panelCommandXML("auth-profile", `name="implementation" recap="생성 UI 완료"`, "### 구현 현황\n\n- 지원 계약: 완료\n- 생성 UI: 완료\n- 설정 UI: 진행 중\n")},
		{62, logXML("auth-profile", "설정 UI 목업 리뷰 대기")},
	} {
		env := testEnv()
		env.Now = day.Add(time.Duration(step.minute) * time.Minute)
		if _, problems := Apply(st, step.commands, env); len(problems) > 0 {
			t.Fatalf("Apply(%s): %v", step.commands, problems)
		}
	}
	return st
}

// The approved panel-first history in Chromium: one write-ordered line, the
// earlier version folded in place and expandable with its payload, the link
// menu's two copy targets, and arrival on #eN and #pN. Needs
// WS_PLAYWRIGHT_MODULE. WS_PANEL_VERSIONS_PAGE=<path> keeps the rendered
// page; WS_PANEL_VERSIONS_SHOTS=<dir> saves captures for the mockup check.
func TestPanelVersionsHistoryBrowser(t *testing.T) {
	view := BuildView(panelVersionsState(t), ViewInput{Layout: LayoutFull, ReplyID: "panel-versions", Location: time.UTC})
	view.Lang = "ko"
	page, err := RenderPage(view)
	if err != nil {
		t.Fatal(err)
	}
	path := os.Getenv("WS_PANEL_VERSIONS_PAGE")
	if path == "" {
		path = filepath.Join(t.TempDir(), "panel-versions.html")
	}
	if err := os.WriteFile(path, []byte(page), 0o600); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/panel-versions.browser.cjs", path, os.Getenv("WS_PANEL_VERSIONS_SHOTS"))
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("panel versions: %v", err)
	}
}
