-- ============================================================
--  WezTerm 설정 — TMUX 키바인딩 완전 재현
--  PREFIX: Ctrl-b  (TMUX 기본값과 동일)
-- ============================================================

local wezterm = require("wezterm")
local act = wezterm.action
local config = wezterm.config_builder()

-- ──────────────────────────────────────────────
-- 플랫폼 감지 및 Modifier 추상화
--
--   macOS   : SUPER = "CMD",  ALT = "OPT"
--   Windows : SUPER = "ALT",  ALT = "ALT"   (CMD 키 없음)
--   Linux   : SUPER = "SUPER",ALT = "ALT"
--
-- PREFIX 키도 플랫폼에 맞게 조정:
--   macOS   : CMD+b
--   Windows : ALT+b
--   Linux   : SUPER+b  (또는 아래 주석 참고)
-- ──────────────────────────────────────────────
local is_mac = wezterm.target_triple:find("darwin") ~= nil
local is_windows = wezterm.target_triple:find("windows") ~= nil
-- linux는 위 둘 다 아닌 경우

-- ──────────────────────────────────────────────
-- im-select 탐색 (모듈 로드 시 1회만 실행)
-- WezTerm GUI 앱은 shell PATH를 상속하지 않으므로 절대 경로로 탐색
-- ──────────────────────────────────────────────
local im_select_cmd = nil -- { path, arg, ... } 형태로 캐싱

do
  local function file_exists(path)
    local ok, _, _ = wezterm.run_child_process({ "test", "-f", path })
    return ok
  end

  if is_mac then
    -- Homebrew: Apple Silicon(/opt/homebrew) 또는 Intel(/usr/local)
    local path = (file_exists("/opt/homebrew/bin/im-select") and "/opt/homebrew/bin/im-select")
      or (file_exists("/usr/local/bin/im-select") and "/usr/local/bin/im-select")
    if path then
      im_select_cmd = { path, "com.apple.keylayout.ABC" }
    end
  elseif is_windows then
    -- PATH에서 im-select.exe 위치 탐색
    local ok, stdout, _ = wezterm.run_child_process({ "where", "im-select" })
    if ok and stdout and #stdout > 0 then
      local path = stdout:match("^([^\r\n]+)")
      if path then
        im_select_cmd = { path, "1033" } -- 1033 = English (US) locale
      end
    end
  else
    -- Linux: fcitx-remote 또는 im-select
    if file_exists("/usr/bin/fcitx-remote") then
      im_select_cmd = { "/usr/bin/fcitx-remote", "-c" }
    elseif file_exists("/usr/local/bin/im-select") then
      im_select_cmd = { "/usr/local/bin/im-select", "keyboard-us" }
    end
  end

  if im_select_cmd then
    wezterm.log_info("IME switch: " .. table.concat(im_select_cmd, " "))
  else
    wezterm.log_warn("im-select not found — IME auto-switch disabled")
  end
end

-- macOS의 CMD 역할 (줄 이동, 탭 등)
local SUPER = is_mac and "CMD" or (is_windows and "ALT" or "SUPER")

-- macOS의 OPT 역할 (단어 이동 등)
local OPT = is_mac and "OPT" or "ALT"

-- PREFIX modifier
local PREFIX_MOD = SUPER -- Ctrl-b 테이블 진입에 쓸 modifier

-- ──────────────────────────────────────────────
-- 기본 설정
-- ──────────────────────────────────────────────
config.term = "xterm-256color"
config.enable_tab_bar = true
config.use_fancy_tab_bar = false
config.tab_bar_at_bottom = false
config.hide_tab_bar_if_only_one_tab = true
config.adjust_window_size_when_changing_font_size = false
config.font = wezterm.font_with_fallback({
  { family = "JetBrainsMono Nerd Font", weight = "Regular" },
  "Cascadia Code",
})
config.font_rules = {
  {
    intensity = "Bold",
    font = wezterm.font_with_fallback({
      { family = "JetBrainsMono Nerd Font", weight = "ExtraBold" },
      { family = "Cascadia Code", weight = "Bold" },
    }),
  },
}
config.font_size = 14
config.native_macos_fullscreen_mode = true
-- config.color_scheme = "Adventure"
config.window_decorations = "RESIZE"
config.window_padding = {
  left = 0,
  right = 0,
  top = 0,
  bottom = 0,
}

-- ──────────────────────────────────────────────
-- PREFIX 키 설정
-- macOS  : CMD+b
-- Windows: ALT+b
-- Linux  : SUPER+b
-- ──────────────────────────────────────────────
local PREFIX = { key = "b", mods = PREFIX_MOD, timeout_milliseconds = 5000 }

-- ──────────────────────────────────────────────
-- 키바인딩
-- ──────────────────────────────────────────────
config.keys = {
  -- Ctrl+Tab / Ctrl+Shift+Tab WezTerm 기본 동작 해제
  { key = "Tab", mods = "CTRL", action = wezterm.action.SendKey({ key = "Tab", mods = "CTRL" }) },
  { key = "Tab", mods = "CTRL|SHIFT", action = wezterm.action.SendKey({ key = "Tab", mods = "CTRL|SHIFT" }) },
  {
    key = "Enter",
    mods = "OPT",
    action = wezterm.action.DisableDefaultAssignment,
  },

  -- ── PREFIX 키 자체를 터미널에 전달 (예: CMD+b CMD+b → Ctrl-b 전달) ──
  {
    key = "b",
    mods = "CTRL",
    action = wezterm.action_callback(function(window, pane)
      -- 한국어 IME 상태라면 영문으로 전환 후 Ctrl+b 전달
      if im_select_cmd then
        wezterm.run_child_process(im_select_cmd)
      end
      window:perform_action(act.SendKey({ key = "b", mods = "CTRL" }), pane)
    end),
  },

  -- 기존 바인딩 비활성화
  { key = "=", mods = "CTRL", action = wezterm.action.DisableDefaultAssignment },
  { key = "-", mods = "CTRL", action = wezterm.action.DisableDefaultAssignment },
  { key = "0", mods = "CTRL", action = wezterm.action.DisableDefaultAssignment },

  -- MacOS 전체 화면 토글
  {
    key = "f",
    mods = "CMD|CTRL",
    action = wezterm.action.ToggleFullScreen,
  },

  -- ════════════════════════════════
  -- 세션 / 윈도우 / 창 관리
  -- ════════════════════════════════

  -- [c] 새 탭(Window) 생성 — PREFIX 방식은 key_tables 사용
  {
    key = "c",
    mods = "CTRL|SHIFT",
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
      timeout_milliseconds = 5000,
    }),
  },

  -- ── 단어 단위 삭제 (Ctrl+Backspace / Ctrl+Delete) ──
  { key = "Backspace", mods = "CTRL", action = act.SendString("\x1b\x7f") }, -- ESC+DEL → backward-kill-word
  { key = "Delete", mods = "CTRL", action = act.SendString("\x1b[3;5~") }, -- Ctrl+Del → kill-word

  -- ── 단어 단위 이동 (OPT/ALT + 좌우) ──────────────
  {
    key = "LeftArrow",
    mods = OPT,
    action = act.SendString("\x1bb"), -- ESC + b
  },
  {
    key = "RightArrow",
    mods = OPT,
    action = act.SendString("\x1bf"), -- ESC + f
  },

  -- ── 줄 처음/끝 (SUPER/CMD + 좌우) ────────────────
  {
    key = "LeftArrow",
    mods = SUPER,
    action = act.SendString("\x01"), -- Ctrl-a
  },
  {
    key = "RightArrow",
    mods = SUPER,
    action = act.SendString("\x05"), -- Ctrl-e
  },

  -- ── 스크롤백 처음/끝 (SUPER/CMD + 위아래) ─────────
  {
    key = "UpArrow",
    mods = SUPER,
    action = act.ScrollToTop,
  },
  {
    key = "DownArrow",
    mods = SUPER,
    action = act.ScrollToBottom,
  },
}

-- ──────────────────────────────────────────────
-- TMUX PREFIX 키테이블
-- ──────────────────────────────────────────────
config.key_tables = {
  tmux_prefix = {
    -- ── 탭 (Window) ────────────────────────
    { key = "c", action = act.SpawnTab("CurrentPaneDomain") },
    { key = "w", action = act.ShowTabNavigator },
    { key = "n", action = act.ActivateTabRelative(1) },
    { key = "p", action = act.ActivateTabRelative(-1) },
    { key = "l", action = act.ActivateLastTab },
    { key = "&", action = act.CloseCurrentTab({ confirm = true }) },
    {
      key = ",",
      action = act.PromptInputLine({
        description = "Rename tab (window)",
        action = wezterm.action_callback(function(window, _, line)
          if line then
            window:active_tab():set_title(line)
          end
        end),
      }),
    },
    { key = "1", action = act.ActivateTab(0) },
    { key = "2", action = act.ActivateTab(1) },
    { key = "3", action = act.ActivateTab(2) },
    { key = "4", action = act.ActivateTab(3) },
    { key = "5", action = act.ActivateTab(4) },
    { key = "6", action = act.ActivateTab(5) },
    { key = "7", action = act.ActivateTab(6) },
    { key = "8", action = act.ActivateTab(7) },
    { key = "9", action = act.ActivateTab(8) },
    { key = "0", action = act.ActivateTab(9) },

    -- ── 분할 (Pane) ────────────────────────
    { key = '"', action = act.SplitVertical({ domain = "CurrentPaneDomain" }) },
    { key = "%", action = act.SplitHorizontal({ domain = "CurrentPaneDomain" }) },
    { key = "x", action = act.CloseCurrentPane({ confirm = true }) },
    { key = "z", action = act.TogglePaneZoomState },
    { key = "!", action = act.SpawnTab("CurrentPaneDomain") },
    { key = "q", action = act.PaneSelect({ mode = "SwapWithActive" }) },
    { key = "o", action = act.ActivatePaneDirection("Next") },
    { key = ";", action = act.ActivatePaneDirection("Prev") },

    -- pane 포커스 이동 (vim 방향키)
    { key = "h", action = act.ActivatePaneDirection("Left") },
    { key = "j", action = act.ActivatePaneDirection("Down") },
    { key = "k", action = act.ActivatePaneDirection("Up") },
    { key = "l", action = act.ActivatePaneDirection("Right") },
    { key = "LeftArrow", action = act.ActivatePaneDirection("Left") },
    { key = "DownArrow", action = act.ActivatePaneDirection("Down") },
    { key = "UpArrow", action = act.ActivatePaneDirection("Up") },
    { key = "RightArrow", action = act.ActivatePaneDirection("Right") },

    -- pane 크기 조절
    { key = "H", action = act.AdjustPaneSize({ "Left", 5 }) },
    { key = "J", action = act.AdjustPaneSize({ "Down", 5 }) },
    { key = "K", action = act.AdjustPaneSize({ "Up", 5 }) },
    { key = "L", action = act.AdjustPaneSize({ "Right", 5 }) },

    -- ── 복사 모드 ──────────────────────────
    { key = "[", action = act.ActivateCopyMode },
    { key = "]", action = act.PasteFrom("PrimarySelection") },
    { key = "=", action = act.PasteFrom("Clipboard") },

    -- ── 기타 ───────────────────────────────
    { key = "d", action = act.DetachDomain("CurrentPaneDomain") },
    { key = "t", action = act.ShowDebugOverlay },
    { key = "?", action = act.ShowDebugOverlay },
    { key = "f", action = act.Search({ CaseSensitiveString = "" }) },
    { key = "s", action = act.ShowTabNavigator },
    { key = "r", action = act.ReloadConfiguration },

    { key = "Escape", action = act.PopKeyTable },
    { key = "c", mods = "CTRL", action = act.PopKeyTable },
  },

  -- ── 복사 모드 (TMUX copy-mode-vi) ──────────────
  copy_mode = {
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
    {
      key = "g",
      mods = "NONE",
      action = act.CopyMode("MoveToScrollbackTop"),
    },
    { key = "u", mods = "CTRL", action = act.CopyMode("PageUp") },
    { key = "d", mods = "CTRL", action = act.CopyMode("PageDown") },
    { key = "f", mods = "CTRL", action = act.CopyMode("PageDown") },
    { key = "b", mods = "CTRL", action = act.CopyMode("PageUp") },
    { key = "v", action = act.CopyMode({ SetSelectionMode = "Cell" }) },
    { key = "V", action = act.CopyMode({ SetSelectionMode = "Line" }) },
    {
      key = "v",
      mods = "CTRL",
      action = act.CopyMode({ SetSelectionMode = "Block" }),
    },
    {
      key = "y",
      action = act.Multiple({
        act.CopyTo("ClipboardAndPrimarySelection"),
        act.CopyMode("Close"),
      }),
    },
    { key = "q", action = act.CopyMode("Close") },
    { key = "Escape", action = act.CopyMode("Close") },
  },
}

-- ──────────────────────────────────────────────
-- 탭 타이틀: "1:bash", "2:vim" 형식
-- ──────────────────────────────────────────────
wezterm.on("format-tab-title", function(tab, tabs, panes, cfg, hover, max_width)
  local title = tab.tab_title
  if not title or #title == 0 then
    title = tab.active_pane.title
    title = title:match("([^/]+)$") or title
  end
  local index = tab.tab_index + 1
  return string.format(" %d:%s ", index, title)
end)

-- ──────────────────────────────────────────────
-- 상태 바
-- ──────────────────────────────────────────────
config.status_update_interval = 1000

wezterm.on("update-status", function(window, pane)
  local date = wezterm.strftime("%Y-%m-%d  %H:%M")
  window:set_right_status(wezterm.format({
    { Foreground = { Color = "#a0a0a0" } },
    { Text = "  " .. date .. "  " },
  }))

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
-- 탭바 색상
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
