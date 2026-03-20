-- ============================================================
--  WezTerm Configuration — Full TMUX Replacement
--  PREFIX: Ctrl-b (same as tmux default)
--
--  Mirrors .tmux.conf keybindings for consistent experience
--  across local WezTerm and remote tmux sessions.
--
--  Backed up original as .wezterm.lua.bak
-- ============================================================

local wezterm = require("wezterm")
local act = wezterm.action
local config = wezterm.config_builder()

-- ──────────────────────────────────────────────
-- Platform Detection
-- ──────────────────────────────────────────────
local is_mac = wezterm.target_triple:find("darwin") ~= nil
local is_windows = wezterm.target_triple:find("windows") ~= nil

-- ──────────────────────────────────────────────
-- im-select Setup (IME → English on prefix activation)
-- ──────────────────────────────────────────────
local im_select_cmd = nil

do
  local function file_exists(path)
    local ok = wezterm.run_child_process({ "test", "-f", path })
    return ok
  end

  if is_mac then
    local path = (file_exists("/opt/homebrew/bin/im-select") and "/opt/homebrew/bin/im-select")
      or (file_exists("/usr/local/bin/im-select") and "/usr/local/bin/im-select")
    if path then
      im_select_cmd = { path, "com.apple.keylayout.ABC" }
    end
  elseif is_windows then
    local ok, stdout = wezterm.run_child_process({ "where", "im-select" })
    if ok and stdout and #stdout > 0 then
      local path = stdout:match("^([^\r\n]+)")
      if path then
        im_select_cmd = { path, "1033" }
      end
    end
  else
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

-- ──────────────────────────────────────────────
-- Basic Configuration
-- ──────────────────────────────────────────────
config.term = "xterm-256color"
config.scrollback_lines = 50000
config.enable_tab_bar = true
config.use_fancy_tab_bar = false
config.tab_bar_at_bottom = false
config.hide_tab_bar_if_only_one_tab = false -- always show (like tmux status bar)
config.adjust_window_size_when_changing_font_size = false
config.native_macos_fullscreen_mode = true
config.status_update_interval = 500
config.window_padding = { left = 0, right = 0, top = 0, bottom = 0 }

-- Mux server for session persistence
-- WezTerm auto-starts the mux server when default_domain is set.
-- Close window = detach (sessions survive). Reopen WezTerm = reattach.
-- NOTE: sessions do NOT survive reboot (mux server is in-memory only).
config.unix_domains = { { name = "unix" } }
config.default_domain = "unix"

-- ── Font ──
config.font = wezterm.font_with_fallback({
  { family = "JetBrainsMono Nerd Font", weight = 500 },
  { family = "NanumBarunGothicOTF", weight = 500 },
  "Cascadia Code",
})
config.font_rules = {
  {
    intensity = "Bold",
    font = wezterm.font_with_fallback({
      { family = "JetBrainsMono Nerd Font", weight = "ExtraBold" },
      { family = "NanumBarunGothicOTF", weight = 900 },
      { family = "Cascadia Code", weight = "ExtraBold" },
    }),
  },
}
config.font_size = 15

-- ── WSL Domains (Windows only) ──
-- Split/new-tab in WSL pane stays in WSL with correct cwd
if is_windows then
  config.wsl_domains = wezterm.default_wsl_domains()
end

-- ──────────────────────────────────────────────
-- Per-Window State
-- ──────────────────────────────────────────────
local remote_mode = {} -- window_id → bool (F12 toggle)
local copy_mode_bg = {} -- window_id → bool (avoid redundant overrides)

-- ──────────────────────────────────────────────
-- Helpers
-- ──────────────────────────────────────────────

local function is_vim(pane)
  local name = pane:get_foreground_process_name()
  if not name then return false end
  name = name:match("([^/\\]+)$") or name
  return name:find("n?vim") ~= nil
end

local function get_cwd(pane)
  local url = pane:get_current_working_dir()
  return url and url.file_path or nil
end

--- Wrap action to also exit prefix mode (for non-repeatable bindings)
local function one_shot(action)
  return act.Multiple({ action, act.PopKeyTable })
end

--- Spawn command in a new tab; tab auto-closes on process exit
local function spawn_cmd_tab(args, env)
  return wezterm.action_callback(function(window, pane)
    local spawn = { args = args, cwd = get_cwd(pane) }
    if env then spawn.set_environment_variables = env end
    window:mux_window():spawn_tab(spawn)
  end)
end

--- Vim-aware pane navigation (one-shot: exits prefix after action)
--- NOTE: For vim-internal-split → WezTerm pane handoff, nvim config needs
---       wezterm cli support (see navigate_lr in lua/config/keymaps.lua)
local function vim_nav(direction, vim_key, cross_tab_dir)
  return wezterm.action_callback(function(window, pane)
    if is_vim(pane) then
      window:perform_action(act.SendKey({ key = vim_key, mods = "CTRL|ALT" }), pane)
      return
    end

    local mux_win = window:mux_window()
    local before_id = mux_win:active_tab():active_pane():pane_id()
    window:perform_action(act.ActivatePaneDirection(direction), pane)

    if cross_tab_dir then
      local after_id = mux_win:active_tab():active_pane():pane_id()
      if before_id == after_id then
        window:perform_action(act.ActivateTabRelative(cross_tab_dir), pane)
      end
    end
  end)
end

--- Vim-aware pane navigation (repeatable: stays in prefix table)
--- For Ctrl+hjkl — pops table only when handing off to vim
local function vim_nav_repeat(direction, vim_key, cross_tab_dir)
  return wezterm.action_callback(function(window, pane)
    if is_vim(pane) then
      window:perform_action(act.SendKey({ key = vim_key, mods = "CTRL|ALT" }), pane)
      window:perform_action(act.PopKeyTable, pane)
      return
    end

    local mux_win = window:mux_window()
    local before_id = mux_win:active_tab():active_pane():pane_id()
    window:perform_action(act.ActivatePaneDirection(direction), pane)

    if cross_tab_dir then
      local after_id = mux_win:active_tab():active_pane():pane_id()
      if before_id == after_id then
        window:perform_action(act.ActivateTabRelative(cross_tab_dir), pane)
      end
    end
    -- Table stays active → user can press C-h/j/k/l again
  end)
end

--- Command palette (tmux-fzf replacement)
local function command_palette()
  return act.InputSelector({
    title = "  Command Palette",
    choices = {
      { label = "Rename Tab", id = "rename_tab" },
      { label = "Move Tab Left", id = "move_tab_left" },
      { label = "Move Tab Right", id = "move_tab_right" },
      { label = "New Window", id = "new_window" },
      { label = "Close Pane", id = "close_pane" },
      { label = "Close Tab", id = "close_tab" },
      { label = "Toggle Zoom", id = "toggle_zoom" },
      { label = "Swap Pane", id = "swap_pane" },
      { label = "Rotate Panes CW", id = "rotate_cw" },
      { label = "Rotate Panes CCW", id = "rotate_ccw" },
      { label = "Clear Scrollback", id = "clear" },
      { label = "Show Launcher", id = "launcher" },
      { label = "Reload Config", id = "reload" },
      { label = "Debug Overlay", id = "debug" },
    },
    fuzzy = true,
    action = wezterm.action_callback(function(window, pane, id)
      if not id then return end
      local actions = {
        rename_tab = act.PromptInputLine({
          description = "Rename tab",
          action = wezterm.action_callback(function(w, _, line)
            if line then w:active_tab():set_title(line) end
          end),
        }),
        move_tab_left = act.MoveTabRelative(-1),
        move_tab_right = act.MoveTabRelative(1),
        new_window = act.SpawnWindow,
        close_pane = act.CloseCurrentPane({ confirm = true }),
        close_tab = act.CloseCurrentTab({ confirm = true }),
        toggle_zoom = act.TogglePaneZoomState,
        swap_pane = act.PaneSelect({ mode = "SwapWithActive" }),
        rotate_cw = act.RotatePanes("Clockwise"),
        rotate_ccw = act.RotatePanes("CounterClockwise"),
        clear = act.ClearScrollback("ScrollbackAndViewport"),
        launcher = act.ShowLauncher,
        reload = act.ReloadConfiguration,
        debug = act.ShowDebugOverlay,
      }
      if actions[id] then
        window:perform_action(actions[id], pane)
      end
    end),
  })
end

-- ──────────────────────────────────────────────
-- Key Bindings (root table)
-- ──────────────────────────────────────────────
config.keys = {
  -- ═══════ PREFIX (Ctrl-b) ═══════
  {
    key = "b",
    mods = "CTRL",
    action = wezterm.action_callback(function(window, pane)
      if remote_mode[window:window_id()] then
        -- Remote mode: pass Ctrl-b through to terminal (for remote tmux)
        window:perform_action(act.SendKey({ key = "b", mods = "CTRL" }), pane)
      else
        -- Switch IME to English, then enter prefix mode
        if im_select_cmd then
          wezterm.run_child_process(im_select_cmd)
        end
        window:perform_action(
          act.ActivateKeyTable({
            name = "tmux_prefix",
            one_shot = false,
            timeout_milliseconds = 2500,
          }),
          pane
        )
      end
    end),
  },

  -- ═══════ F12: Remote Mode Toggle (nested tmux) ═══════
  {
    key = "F12",
    action = wezterm.action_callback(function(window, pane)
      local win_id = window:window_id()
      remote_mode[win_id] = not remote_mode[win_id]
      if remote_mode[win_id] then
        window:perform_action(act.PopKeyTable, pane)
      end
    end),
  },

  -- ═══════ Disable conflicting WezTerm defaults ═══════
  { key = "Tab", mods = "CTRL", action = act.SendKey({ key = "Tab", mods = "CTRL" }) },
  { key = "Tab", mods = "CTRL|SHIFT", action = act.SendKey({ key = "Tab", mods = "CTRL|SHIFT" }) },
  { key = "Enter", mods = "OPT", action = act.DisableDefaultAssignment },
  { key = "=", mods = "CTRL", action = act.DisableDefaultAssignment },
  { key = "-", mods = "CTRL", action = act.DisableDefaultAssignment },
  { key = "0", mods = "CTRL", action = act.DisableDefaultAssignment },
  { key = "c", mods = "CTRL|SHIFT", action = act.DisableDefaultAssignment },

  -- ═══════ Fullscreen (macOS) ═══════
  { key = "f", mods = "CMD|CTRL", action = act.ToggleFullScreen },

  -- ═══════ Terminal Input Shortcuts ═══════
  -- Word-level delete
  { key = "Backspace", mods = "CTRL", action = act.SendString("\x1b\x7f") },
  { key = "Delete", mods = "CTRL", action = act.SendString("\x1b[3;5~") },

  -- Word-level navigation (OPT/ALT + arrows)
  { key = "LeftArrow", mods = is_mac and "OPT" or "ALT", action = act.SendString("\x1bb") },
  { key = "RightArrow", mods = is_mac and "OPT" or "ALT", action = act.SendString("\x1bf") },

  -- Line start/end (CMD/SUPER + arrows)
  { key = "LeftArrow", mods = is_mac and "CMD" or "SUPER", action = act.SendString("\x01") },
  { key = "RightArrow", mods = is_mac and "CMD" or "SUPER", action = act.SendString("\x05") },

  -- Scroll to top/bottom
  { key = "UpArrow", mods = is_mac and "CMD" or "SUPER", action = act.ScrollToTop },
  { key = "DownArrow", mods = is_mac and "CMD" or "SUPER", action = act.ScrollToBottom },
}

-- ──────────────────────────────────────────────
-- TMUX PREFIX Key Table
-- ──────────────────────────────────────────────
config.key_tables = {
  tmux_prefix = {
    -- ═══════ Tab (tmux Window) ═══════
    { key = "c", action = one_shot(act.SpawnTab("CurrentPaneDomain")) },
    { key = "w", action = one_shot(act.ShowTabNavigator) },
    { key = "&", action = one_shot(act.CloseCurrentTab({ confirm = true })) },
    { key = "X", action = one_shot(act.CloseCurrentTab({ confirm = true })) },
    {
      key = ",",
      action = one_shot(act.PromptInputLine({
        description = "Rename tab",
        action = wezterm.action_callback(function(window, _, line)
          if line then window:active_tab():set_title(line) end
        end),
      })),
    },

    -- Direct tab selection (display 1-based, internal 0-based)
    { key = "1", action = one_shot(act.ActivateTab(0)) },
    { key = "2", action = one_shot(act.ActivateTab(1)) },
    { key = "3", action = one_shot(act.ActivateTab(2)) },
    { key = "4", action = one_shot(act.ActivateTab(3)) },
    { key = "5", action = one_shot(act.ActivateTab(4)) },
    { key = "6", action = one_shot(act.ActivateTab(5)) },
    { key = "7", action = one_shot(act.ActivateTab(6)) },
    { key = "8", action = one_shot(act.ActivateTab(7)) },
    { key = "9", action = one_shot(act.ActivateTab(8)) },
    { key = "0", action = one_shot(act.ActivateTab(9)) },

    -- ══ Repeatable: tab navigation (tmux bind -r { / }) ══
    { key = "{", action = act.ActivateTabRelative(-1) },
    { key = "}", action = act.ActivateTabRelative(1) },

    -- ═══════ Pane (tmux Pane) ═══════

    -- Split (CurrentPaneDomain: stays in same WSL/local/domain + cwd)
    { key = '"', action = one_shot(act.SplitVertical({ domain = "CurrentPaneDomain" })) },
    { key = "%", action = one_shot(act.SplitHorizontal({ domain = "CurrentPaneDomain" })) },
    { key = "|", action = one_shot(act.SplitHorizontal({ domain = "CurrentPaneDomain" })) },
    { key = "-", action = one_shot(act.SplitVertical({ domain = "CurrentPaneDomain" })) },

    -- Close / zoom
    { key = "x", action = one_shot(act.CloseCurrentPane({ confirm = true })) },
    { key = "z", action = one_shot(act.TogglePaneZoomState) },

    -- Pane select / cycle
    { key = "q", action = one_shot(act.PaneSelect({})) },
    { key = "o", action = one_shot(act.ActivatePaneDirection("Next")) },
    { key = ";", action = one_shot(act.ActivatePaneDirection("Prev")) },

    -- Vim-aware pane navigation — one-shot (h/l cross tab at edges)
    { key = "h", action = one_shot(vim_nav("Left", "h", -1)) },
    { key = "j", action = one_shot(vim_nav("Down", "j")) },
    { key = "k", action = one_shot(vim_nav("Up", "k")) },
    { key = "l", action = one_shot(vim_nav("Right", "l", 1)) },

    -- ══ Repeatable: Ctrl+hjkl pane nav (tmux bind -r C-hjkl) ══
    -- Stays in prefix table so you can Ctrl-b, then C-h C-h C-h ...
    -- Pops table when vim is detected (hands off to vim)
    -- No cross-tab at edges (unlike bare h/l)
    { key = "h", mods = "CTRL", action = vim_nav_repeat("Left", "h") },
    { key = "j", mods = "CTRL", action = vim_nav_repeat("Down", "j") },
    { key = "k", mods = "CTRL", action = vim_nav_repeat("Up", "k") },
    { key = "l", mods = "CTRL", action = vim_nav_repeat("Right", "l") },

    -- ══ Repeatable: Ctrl+arrow resize (1 cell, tmux default behavior) ══
    { key = "LeftArrow", mods = "CTRL", action = act.AdjustPaneSize({ "Left", 1 }) },
    { key = "DownArrow", mods = "CTRL", action = act.AdjustPaneSize({ "Down", 1 }) },
    { key = "UpArrow", mods = "CTRL", action = act.AdjustPaneSize({ "Up", 1 }) },
    { key = "RightArrow", mods = "CTRL", action = act.AdjustPaneSize({ "Right", 1 }) },

    -- ══ Repeatable: pane resize (Shift + hjkl, 5 cells) ══
    { key = "H", action = act.AdjustPaneSize({ "Left", 5 }) },
    { key = "J", action = act.AdjustPaneSize({ "Down", 5 }) },
    { key = "K", action = act.AdjustPaneSize({ "Up", 5 }) },
    { key = "L", action = act.AdjustPaneSize({ "Right", 5 }) },

    -- ═══════ Copy / Paste ═══════
    { key = "[", action = one_shot(act.ActivateCopyMode) },
    { key = "]", action = one_shot(act.PasteFrom("PrimarySelection")) },
    { key = "p", action = one_shot(act.PasteFrom("Clipboard")) },
    { key = "=", action = one_shot(act.PasteFrom("Clipboard")) },

    -- ═══════ Popup Replacements (new tab, auto-close on exit) ═══════
    { key = "g", action = one_shot(spawn_cmd_tab({ "lazygit" })) },
    { key = "T", action = one_shot(spawn_cmd_tab({ "btop" })) },
    {
      key = "S",
      action = one_shot(wezterm.action_callback(function(window, pane)
        window:mux_window():spawn_tab({ cwd = get_cwd(pane) })
      end)),
    },
    { key = "e", action = one_shot(spawn_cmd_tab({ "nvim", "." })) },
    { key = "E", action = one_shot(spawn_cmd_tab({ "lf" }, { EDITOR = "nvim" })) },

    -- ═══════ Other ═══════

    -- Send literal Ctrl-b (for remote tmux in non-F12 mode)
    { key = "b", mods = "CTRL", action = one_shot(act.SendKey({ key = "b", mods = "CTRL" })) },

    -- Detach from mux domain
    { key = "d", action = one_shot(act.DetachDomain("CurrentPaneDomain")) },

    -- Search in scrollback
    { key = "f", action = one_shot(act.Search({ CaseSensitiveString = "" })) },

    -- Session/domain launcher (tmux choose-session)
    { key = "s", action = one_shot(act.ShowLauncherArgs({ flags = "WORKSPACES|DOMAINS" })) },

    -- Command palette (tmux-fzf replacement, fuzzy search)
    { key = ":", action = one_shot(command_palette()) },

    -- Debug / help
    { key = "?", action = one_shot(act.ShowDebugOverlay) },
    { key = "t", action = one_shot(act.ShowDebugOverlay) },

    -- Reload config
    { key = "r", action = one_shot(act.ReloadConfiguration) },

    -- Cancel prefix
    { key = "Escape", action = act.PopKeyTable },
    { key = "c", mods = "CTRL", action = act.PopKeyTable },
    { key = "Space", mods = "CTRL", action = act.PopKeyTable },
  },

  -- ──────────────────────────────────────────
  -- Copy Mode (tmux copy-mode-vi)
  -- ──────────────────────────────────────────
  copy_mode = {
    -- Movement
    { key = "h", action = act.CopyMode("MoveLeft") },
    { key = "j", action = act.CopyMode("MoveDown") },
    { key = "k", action = act.CopyMode("MoveUp") },
    { key = "l", action = act.CopyMode("MoveRight") },
    { key = "LeftArrow", action = act.CopyMode("MoveLeft") },
    { key = "RightArrow", action = act.CopyMode("MoveRight") },
    { key = "UpArrow", action = act.CopyMode("MoveUp") },
    { key = "DownArrow", action = act.CopyMode("MoveDown") },

    -- Word
    { key = "w", action = act.CopyMode("MoveForwardWord") },
    { key = "b", action = act.CopyMode("MoveBackwardWord") },
    { key = "e", action = act.CopyMode("MoveForwardWordEnd") },

    -- Line
    { key = "0", action = act.CopyMode("MoveToStartOfLine") },
    { key = "^", action = act.CopyMode("MoveToStartOfLineContent") },
    { key = "$", action = act.CopyMode("MoveToEndOfLineContent") },

    -- Document
    { key = "g", action = act.CopyMode("MoveToScrollbackTop") },
    { key = "G", action = act.CopyMode("MoveToScrollbackBottom") },

    -- Page scroll
    { key = "u", mods = "CTRL", action = act.CopyMode("PageUp") },
    { key = "d", mods = "CTRL", action = act.CopyMode("PageDown") },
    { key = "f", mods = "CTRL", action = act.CopyMode("PageDown") },
    { key = "b", mods = "CTRL", action = act.CopyMode("PageUp") },

    -- Selection
    { key = "v", action = act.CopyMode({ SetSelectionMode = "Cell" }) },
    { key = "V", action = act.CopyMode({ SetSelectionMode = "Line" }) },
    { key = "v", mods = "CTRL", action = act.CopyMode({ SetSelectionMode = "Block" }) },
    { key = "Space", action = act.CopyMode({ SetSelectionMode = "Cell" }) },

    -- Yank → clipboard + exit
    {
      key = "y",
      action = act.Multiple({
        act.CopyTo("ClipboardAndPrimarySelection"),
        act.CopyMode("Close"),
      }),
    },

    -- Search
    { key = "/", action = act.Search({ CaseSensitiveString = "" }) },
    { key = "n", action = act.CopyMode("NextMatch") },
    { key = "N", action = act.CopyMode("PriorMatch") },

    -- Exit
    { key = "q", action = act.CopyMode("Close") },
    { key = "Escape", action = act.CopyMode("Close") },
    { key = "c", mods = "CTRL", action = act.CopyMode("Close") },
    { key = "Space", mods = "CTRL", action = act.CopyMode("Close") },
  },
}

-- ──────────────────────────────────────────────
-- Tab Title (powerline style, mode-aware colors)
-- ──────────────────────────────────────────────
wezterm.on("format-tab-title", function(tab, tabs, panes, cfg, hover, max_width)
  local index = tab.tab_index + 1
  local title = tab.tab_title
  if not title or #title == 0 then
    title = tab.active_pane.title or ""
    title = title:match("([^/\\]+)$") or title
  end

  local process = (tab.active_pane.foreground_process_name or ""):match("([^/\\]+)$") or ""
  local display
  if process ~= "" and process ~= title then
    display = string.format(" %d: %s (%s) ", index, title, process)
  else
    display = string.format(" %d: %s ", index, title)
  end

  local prefix_on = wezterm.GLOBAL.prefix_active or false
  local remote_on = wezterm.GLOBAL.remote_mode or false
  local bar_bg = "#1a1a1a"

  if tab.is_active then
    local bg = "#2e8b57"
    if prefix_on then
      bg = "#8b0000"
    elseif remote_on then
      bg = "#804000"
    end
    return {
      { Background = { Color = bar_bg } },
      { Foreground = { Color = bg } },
      { Text = "\u{e0b6}" },
      { Background = { Color = bg } },
      { Foreground = { Color = "#ffffff" } },
      { Attribute = { Intensity = "Bold" } },
      { Text = display },
      { Background = { Color = bar_bg } },
      { Foreground = { Color = bg } },
      { Text = "\u{e0b4}" },
    }
  else
    local bg = "#252525"
    local fg = "#808080"
    if prefix_on then
      bg, fg = "#2a0000", "#aa8800"
    elseif remote_on then
      bg, fg = "#2a2000", "#aa8800"
    end
    return {
      { Background = { Color = bg } },
      { Foreground = { Color = fg } },
      { Text = display },
    }
  end
end)

-- ──────────────────────────────────────────────
-- Status Bar & Mode Feedback
-- ──────────────────────────────────────────────
wezterm.on("update-status", function(window, pane)
  local win_id = window:window_id()
  local key_table = window:active_key_table()
  local is_prefix = key_table == "tmux_prefix"
  local is_copy = key_table == "copy_mode"
  local is_remote = remote_mode[win_id] or false

  -- Sync to GLOBAL for format-tab-title
  wezterm.GLOBAL.prefix_active = is_prefix
  wezterm.GLOBAL.remote_mode = is_remote

  -- ── Copy mode: yellowish background tint ──
  local want_copy_bg = is_copy
  if copy_mode_bg[win_id] ~= want_copy_bg then
    copy_mode_bg[win_id] = want_copy_bg
    local overrides = window:get_config_overrides() or {}
    if want_copy_bg then
      overrides.colors = overrides.colors or {}
      overrides.colors.background = "#18180a"
    else
      if overrides.colors then
        overrides.colors.background = nil
        if not next(overrides.colors) then overrides.colors = nil end
      end
    end
    window:set_config_overrides(overrides)
  end

  -- ── Left status: mode indicator ──
  local left = ""
  if is_prefix then
    left = wezterm.format({
      { Background = { Color = "#f5a623" } },
      { Foreground = { Color = "#000000" } },
      { Attribute = { Intensity = "Bold" } },
      { Text = "  PREFIX  " },
    })
  elseif is_copy then
    left = wezterm.format({
      { Background = { Color = "#c67200" } },
      { Foreground = { Color = "#ffffff" } },
      { Attribute = { Intensity = "Bold" } },
      { Text = "  COPY  " },
    })
  elseif is_remote then
    left = wezterm.format({
      { Background = { Color = "#804000" } },
      { Foreground = { Color = "#ffffff" } },
      { Attribute = { Intensity = "Bold" } },
      { Text = "  REMOTE  " },
    })
  end
  window:set_left_status(left)

  -- ── Right status: hostname + datetime ──
  local hostname = (wezterm.hostname() or ""):match("^([^.]+)") or ""
  local date = wezterm.strftime("%Y-%m-%d  %H:%M")
  local accent = "#2e8b57"
  if is_prefix then
    accent = "#8b0000"
  elseif is_remote then
    accent = "#804000"
  end

  window:set_right_status(wezterm.format({
    { Foreground = { Color = "#808080" } },
    { Text = hostname .. "  " },
    { Foreground = { Color = accent } },
    { Text = "\u{e0b6}" },
    { Background = { Color = accent } },
    { Foreground = { Color = "#ffffff" } },
    { Attribute = { Intensity = "Bold" } },
    { Text = " " .. date .. " " },
  }))
end)

-- ──────────────────────────────────────────────
-- Colors & Appearance
-- ──────────────────────────────────────────────
config.colors = {
  tab_bar = {
    background = "#1a1a1a",
    active_tab = { bg_color = "#2e8b57", fg_color = "#ffffff", intensity = "Bold" },
    inactive_tab = { bg_color = "#2a2a2a", fg_color = "#808080" },
    inactive_tab_hover = { bg_color = "#3a3a3a", fg_color = "#c0c0c0" },
    new_tab = { bg_color = "#1a1a1a", fg_color = "#555555" },
  },
  split = "#2e8b57",
}

-- Inactive panes: dimmed + lighter background (like tmux window-style bg=#202020)
config.inactive_pane_hsb = {
  saturation = 0.7,
  brightness = 0.7,
}

return config
