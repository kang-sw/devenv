-- Keymaps are automatically loaded on the VeryLazy event
-- Default keymaps that are always set: https://github.com/LazyVim/LazyVim/blob/main/lua/lazyvim/config/keymaps.lua
-- Add any additional keymaps here

-- macOS 스타일 키바인딩
local map = vim.keymap.set

map("i", "<M-b>", "<C-Left>", { desc = "Move word backward" })
map("i", "<M-f>", "<C-Right>", { desc = "Move word forward" })
map("i", "<M-BS>", "<C-w>", { desc = "Delete word backward" })
map("i", "<C-a>", "<Home>", { desc = "Move to line start" })
map("i", "<C-e>", "<End>", { desc = "Move to line end" })

-- vim-tmux pane navigation (C-M-hjkl)
-- 상하: vim-tmux-navigator 그대로 사용
-- 좌우: vim split 없으면 tmux pane → 그것도 끝이면 previous/next-window
vim.keymap.set({ "n", "i", "t" }, "<C-M-j>", "<cmd>TmuxNavigateDown<cr>", { desc = "Navigate down (vim/tmux)" })
vim.keymap.set({ "n", "i", "t" }, "<C-M-k>", "<cmd>TmuxNavigateUp<cr>", { desc = "Navigate up (vim/tmux)" })

local function navigate_lr(dir, wincmd, pane_at, window_cmd)
  return function()
    local winnr = vim.fn.winnr()
    vim.cmd("wincmd " .. wincmd)
    if vim.fn.winnr() ~= winnr then
      return -- vim split 이동 성공
    end
    -- vim 안에서 더 갈 곳 없음 → tmux에 위임
    local at_edge = vim.fn.system("tmux display-message -p '#{" .. pane_at .. "}'"):gsub("%s+", "")
    if at_edge == "1" then
      vim.fn.system("tmux " .. window_cmd)
    else
      vim.fn.system("tmux select-pane -" .. dir)
    end
  end
end

vim.keymap.set({ "n", "i", "t" }, "<C-M-h>", navigate_lr("L", "h", "pane_at_left", "previous-window"), { desc = "Navigate left (vim/tmux/window)" })
vim.keymap.set({ "n", "i", "t" }, "<C-M-l>", navigate_lr("R", "l", "pane_at_right", "next-window"), { desc = "Navigate right (vim/tmux/window)" })

-- tmux 스타일 스플릿
vim.keymap.set("n", '<leader>"', "<cmd>split<CR>", { desc = "Horizontal split" })
vim.keymap.set("n", "<leader>%", "<cmd>vsplit<CR>", { desc = "Vertical split" })

-- Ctrl-C를 Esc처럼 동작하게 매핑 (InsertLeave 이벤트 발생시킴)
vim.keymap.set("i", "<C-c>", "<Esc>")

-- Jump to type directly
vim.keymap.set("n", "gy", vim.lsp.buf.type_definition, { desc = "Go to type definition" })

-- 터미널 토글 (Ctrl+`)
vim.keymap.set("n", "<C-`>", function()
  Snacks.terminal.toggle()
end, { desc = "Toggle terminal" })

-- 터미널 모드에서도 닫히도록
vim.keymap.set("t", "<C-`>", function()
  Snacks.terminal.toggle()
end, { desc = "Toggle terminal" })

-- C-\ 더블 탭으로 터미널 탈출
vim.keymap.set("t", "<C-\\><C-\\>", "<C-\\><C-n>", { desc = "Exit terminal mode" })

-- Hover setup
vim.keymap.set("n", "gh", function()
  vim.lsp.buf.hover()
end, { desc = "Hover documentation" })

-- F1 Formatting
vim.keymap.set("n", "<F1>", "<Nop>", { noremap = true, silent = true })
vim.keymap.set("i", "<F1>", "<Nop>", { noremap = true, silent = true })
vim.keymap.set("v", "<F1>", "<Nop>", { noremap = true, silent = true })
vim.keymap.set("n", "<F1>", function()
  vim.lsp.buf.format({ async = true })
end, { noremap = true, silent = true, desc = "Format" })

-- 함수 본문만 모두 접기
local function fold_all_functions()
  local bufnr = vim.api.nvim_get_current_buf()
  local ft = vim.bo[bufnr].filetype

  -- 언어별 treesitter 노드 타입
  local node_types = {
    c = { "function_definition" },
    rust = { "function_item" },
    python = { "function_definition" },
    lua = { "function_declaration", "local_function" },
    sh = { "function_definition" },
    bash = { "function_definition" },
    zsh = { "function_definition" },
  }

  local types = node_types[ft]
  if not types then
    vim.notify("fold_all_functions: '" .. ft .. "' is not supported", vim.log.levels.WARN)
    return
  end

  local ok, parser = pcall(vim.treesitter.get_parser, bufnr, ft)
  if not ok or not parser then
    vim.notify("fold_all_functions: treesitter parser unavailable", vim.log.levels.ERROR)
    return
  end

  local root = parser:parse()[1]:root()

  -- foldmethod를 manual로 전환 후 기존 fold 초기화
  vim.wo.foldmethod = "manual"
  vim.cmd("normal! zE")

  local type_set = {}
  for _, t in ipairs(types) do
    type_set[t] = true
  end

  local function traverse(node)
    if type_set[node:type()] then
      local sr, _, er, _ = node:range()
      -- 1줄짜리(예: 빈 함수 선언)는 건너뜀
      if er > sr then
        vim.cmd(string.format("%d,%dfold", sr + 1, er + 1))
      end
    end
    for child in node:iter_children() do
      traverse(child)
    end
  end

  traverse(root)
  vim.notify("Functions folded (" .. ft .. ")", vim.log.levels.INFO)
end

vim.keymap.set("n", "<leader>zf", fold_all_functions, { desc = "Fold all functions" })
