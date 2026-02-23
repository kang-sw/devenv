-- ============================================================
--  WezTerm 설정 — TMUX 키바인딩 완전 재현
--  PREFIX: Ctrl-b  (TMUX 기본값과 동일)
-- ============================================================

local wezterm = require("wezterm")
local act = wezterm.action
local config = wezterm.config_builder()

-- ──────────────────────────────────────────────
-- 기본 설정
-- ──────────────────────────────────────────────
config.term = "xterm-256color"
config.enable_tab_bar = true
config.use_fancy_tab_bar = false
config.tab_bar_at_bottom = false
config.hide_tab_bar_if_only_one_tab = true
config.adjust_window_size_when_changing_font_size = false
config.font_size = 14
-- config.color_scheme = "Adventure"

-- ──────────────────────────────────────────────
-- PREFIX 키 설정 (Ctrl-b)
-- TMUX_PREFIX_TIMEOUT: 키 입력 대기 시간 (ms)
-- ──────────────────────────────────────────────
local PREFIX = { key = "b", mods = "CMD", timeout_milliseconds = 5000 }

-- ──────────────────────────────────────────────
-- 키바인딩
-- ──────────────────────────────────────────────
config.keys = {

	-- ── PREFIX 키 자체를 터미널에 전달 (Ctrl-b Ctrl-b) ──
	{
		key = PREFIX.key,
		mods = PREFIX.mods,
		action = act.SendKey({ key = "b", mods = "CTRL" }),
	},

	-- ════════════════════════════════
	-- 세션 / 윈도우 / 창 관리
	-- ════════════════════════════════

	-- [c] 새 탭(Window) 생성
	{
		key = "c",
		mods = "CTRL|SHIFT", -- placeholder, 실제 PREFIX 방식은 key_tables 사용
		action = act.DisableDefaultAssignment,
	},

	-- ════════════════════════════════
	-- key_tables: PREFIX 입력 후 동작
	-- ════════════════════════════════
	{
		key = PREFIX.key,
		mods = PREFIX.mods,
		action = act.ActivateKeyTable({
			name = "tmux_prefix",
			one_shot = true,
			timeout_milliseconds = 2000,
		}),
	},

	-- MacOS 스타일 줄 이동.
	-- ── macOS 스타일 단어/줄 이동 ──────────────────
	-- Option + 좌우: 단어 단위 이동 (iterm2 동일)
	{
		key = "LeftArrow",
		mods = "OPT",
		action = act.SendString("\x1bb"), -- ESC + b
	},
	{
		key = "RightArrow",
		mods = "OPT",
		action = act.SendString("\x1bf"), -- ESC + f
	},

	-- Cmd + 좌우: 줄 처음/끝 (Home/End)
	{
		key = "LeftArrow",
		mods = "CMD",
		action = act.SendString("\x01"), -- Ctrl-a (줄 처음)
	},
	{
		key = "RightArrow",
		mods = "CMD",
		action = act.SendString("\x05"), -- Ctrl-e (줄 끝)
	},

	-- Cmd + 위아래: 스크롤백 처음/끝 (optional)
	{
		key = "UpArrow",
		mods = "CMD",
		action = act.ScrollToTop,
	},
	{
		key = "DownArrow",
		mods = "CMD",
		action = act.ScrollToBottom,
	},
}

-- ──────────────────────────────────────────────
-- TMUX PREFIX 키테이블
-- (Ctrl-b 누른 뒤 아래 키 입력)
-- ──────────────────────────────────────────────
config.key_tables = {
	tmux_prefix = {

		-- ── 탭 (Window) ────────────────────────
		{ key = "c", action = act.SpawnTab("CurrentPaneDomain") }, -- 새 window
		{ key = "w", action = act.ShowTabNavigator }, -- window 목록
		{ key = "n", action = act.ActivateTabRelative(1) }, -- 다음 window
		{ key = "p", action = act.ActivateTabRelative(-1) }, -- 이전 window
		{ key = "l", action = act.ActivateLastTab }, -- 마지막 window
		{ key = "&", action = act.CloseCurrentTab({ confirm = true }) }, -- window 닫기
		{
			key = ",",
			action = act.PromptInputLine({ -- window 이름 변경
				description = "Rename tab (window)",
				action = wezterm.action_callback(function(window, _, line)
					if line then
						window:active_tab():set_title(line)
					end
				end),
			}),
		},
		-- 숫자로 window 직접 이동
		-- TMUX와 동일: 1~9는 1번째~9번째 탭, 0은 10번째 탭
		{ key = "1", action = act.ActivateTab(0) },
		{ key = "2", action = act.ActivateTab(1) },
		{ key = "3", action = act.ActivateTab(2) },
		{ key = "4", action = act.ActivateTab(3) },
		{ key = "5", action = act.ActivateTab(4) },
		{ key = "6", action = act.ActivateTab(5) },
		{ key = "7", action = act.ActivateTab(6) },
		{ key = "8", action = act.ActivateTab(7) },
		{ key = "9", action = act.ActivateTab(8) },
		{ key = "0", action = act.ActivateTab(9) }, -- TMUX: 0 = 10번째 탭

		-- ── 분할 (Pane) ────────────────────────
		{ key = '"', action = act.SplitVertical({ domain = "CurrentPaneDomain" }) }, -- 수평 분할 (위/아래)
		{ key = "%", action = act.SplitHorizontal({ domain = "CurrentPaneDomain" }) }, -- 수직 분할 (좌/우)
		{ key = "x", action = act.CloseCurrentPane({ confirm = true }) }, -- pane 닫기
		{ key = "z", action = act.TogglePaneZoomState }, -- pane 줌
		{ key = "!", action = act.SpawnTab("CurrentPaneDomain") }, -- pane → 새 window (근사치)
		{ key = "q", action = act.PaneSelect({ mode = "SwapWithActive" }) }, -- pane 번호 표시/선택
		{ key = "o", action = act.ActivatePaneDirection("Next") }, -- 다음 pane
		{ key = ";", action = act.ActivatePaneDirection("Prev") }, -- 이전 pane

		-- pane 포커스 이동 (vim 방향키)
		{ key = "h", action = act.ActivatePaneDirection("Left") },
		{ key = "j", action = act.ActivatePaneDirection("Down") },
		{ key = "k", action = act.ActivatePaneDirection("Up") },
		{ key = "l", action = act.ActivatePaneDirection("Right") },
		{ key = "LeftArrow", action = act.ActivatePaneDirection("Left") },
		{ key = "DownArrow", action = act.ActivatePaneDirection("Down") },
		{ key = "UpArrow", action = act.ActivatePaneDirection("Up") },
		{ key = "RightArrow", action = act.ActivatePaneDirection("Right") },

		-- pane 크기 조절 — 리사이즈 모드 진입
		{ key = "H", action = act.AdjustPaneSize({ "Left", 5 }) },
		{ key = "J", action = act.AdjustPaneSize({ "Down", 5 }) },
		{ key = "K", action = act.AdjustPaneSize({ "Up", 5 }) },
		{ key = "L", action = act.AdjustPaneSize({ "Right", 5 }) },

		-- ── 복사 모드 (Copy Mode) ──────────────
		{ key = "[", action = act.ActivateCopyMode }, -- 복사 모드 진입
		{ key = "]", action = act.PasteFrom("PrimarySelection") }, -- 붙여넣기
		{ key = "=", action = act.PasteFrom("Clipboard") }, -- 클립보드 붙여넣기

		-- ── 기타 ───────────────────────────────
		{ key = "d", action = act.DetachDomain("CurrentPaneDomain") }, -- detach (근사치)
		{ key = "t", action = act.ShowDebugOverlay }, -- 시계 대신 디버그 오버레이
		{ key = "?", action = act.ShowDebugOverlay }, -- 키 목록 (근사치)
		{ key = "f", action = act.Search({ CaseSensitiveString = "" }) }, -- 검색
		{ key = "s", action = act.ShowTabNavigator }, -- 세션 목록 (탭으로 대체)

		-- PREFIX + r : config 리로드
		{ key = "r", action = act.ReloadConfiguration },

		-- ESC / Ctrl-c 로 PREFIX 모드 취소
		{ key = "Escape", action = act.PopKeyTable },
		{ key = "c", mods = "CTRL", action = act.PopKeyTable },
	},

	-- ── 복사 모드 내부 키 (TMUX copy-mode-vi 기준) ──────────────
	copy_mode = {
		-- 이동
		{ key = "h", action = act.CopyMode("MoveLeft") },
		{ key = "j", action = act.CopyMode("MoveDown") },
		{ key = "k", action = act.CopyMode("MoveUp") },
		{ key = "l", action = act.CopyMode("MoveRight") },
		{ key = "LeftArrow", action = act.CopyMode("MoveLeft") },
		{ key = "RightArrow", action = act.CopyMode("MoveRight") },
		{ key = "UpArrow", action = act.CopyMode("MoveUp") },
		{ key = "DownArrow", action = act.CopyMode("MoveDown") },
		{ key = "w", action = act.CopyMode("MoveForwardWord") },
		{ key = "b", action = act.CopyMode("MoveBackwardWord") },
		{ key = "e", action = act.CopyMode("MoveForwardWordEnd") },
		{ key = "0", action = act.CopyMode("MoveToStartOfLine") },
		{ key = "$", action = act.CopyMode("MoveToEndOfLineContent") },
		{ key = "G", action = act.CopyMode("MoveToScrollbackBottom") },
		{ key = "g", mods = "NONE", action = act.CopyMode("MoveToScrollbackTop") },
		-- 페이지 이동
		{ key = "u", mods = "CTRL", action = act.CopyMode("PageUp") },
		{ key = "d", mods = "CTRL", action = act.CopyMode("PageDown") },
		{ key = "f", mods = "CTRL", action = act.CopyMode("PageDown") },
		{ key = "b", mods = "CTRL", action = act.CopyMode("PageUp") },
		-- 선택
		{ key = "v", action = act.CopyMode({ SetSelectionMode = "Cell" }) },
		{ key = "V", action = act.CopyMode({ SetSelectionMode = "Line" }) },
		{ key = "v", mods = "CTRL", action = act.CopyMode({ SetSelectionMode = "Block" }) },
		-- 복사 후 복사 모드 종료
		{
			key = "y",
			action = act.Multiple({
				act.CopyTo("ClipboardAndPrimarySelection"),
				act.CopyMode("Close"),
			}),
		},
		-- 취소
		{ key = "q", action = act.CopyMode("Close") },
		{ key = "Escape", action = act.CopyMode("Close") },
	},
}

-- ──────────────────────────────────────────────
-- 탭 타이틀: TMUX처럼 "1:bash", "2:vim" 형식
-- (WezTerm 내부 인덱스는 0-based이므로 +1 해서 표시)
-- ──────────────────────────────────────────────
wezterm.on("format-tab-title", function(tab, tabs, panes, cfg, hover, max_width)
	local title = tab.tab_title
	if not title or #title == 0 then
		title = tab.active_pane.title
		title = title:match("([^/]+)$") or title -- 경로에서 프로세스명만
	end
	local index = tab.tab_index + 1 -- 0-based → 1-based (TMUX 동일)
	return string.format(" %d:%s ", index, title)
end)

-- ──────────────────────────────────────────────
-- 상태 바 (TMUX status bar 느낌)
-- ──────────────────────────────────────────────
config.status_update_interval = 1000

wezterm.on("update-status", function(window, pane)
	-- 오른쪽: 날짜/시간
	local date = wezterm.strftime("%Y-%m-%d  %H:%M")
	window:set_right_status(wezterm.format({
		{ Foreground = { Color = "#a0a0a0" } },
		{ Text = "  " .. date .. "  " },
	}))

	-- 왼쪽: PREFIX 활성 여부 표시
	local key_table = window:active_key_table()
	local prefix_indicator = ""
	if key_table == "tmux_prefix" then
		prefix_indicator = wezterm.format({
			{ Background = { Color = "#f5a623" } },
			{ Foreground = { Color = "#000000" } },
			{ Text = "  [PREFIX]  " },
		})
	end
	window:set_left_status(prefix_indicator)
end)

-- ──────────────────────────────────────────────
-- 탭바 색상 (TMUX green/dark 테마)
-- ──────────────────────────────────────────────
config.colors = {
	tab_bar = {
		background = "#1a1a1a",
		active_tab = {
			bg_color = "#2e8b57",
			fg_color = "#ffffff",
			intensity = "Bold",
		},
		inactive_tab = {
			bg_color = "#2a2a2a",
			fg_color = "#808080",
		},
		inactive_tab_hover = {
			bg_color = "#3a3a3a",
			fg_color = "#c0c0c0",
		},
		new_tab = {
			bg_color = "#1a1a1a",
			fg_color = "#555555",
		},
	},
	split = "#2e8b57",
}

config.inactive_pane_hsb = {
	saturation = 0.7,
	brightness = 0.7,
}

return config
