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

-- Ctrl-C를 Esc처럼 동작하게 매핑 (InsertLeave 이벤트 발생시킴)
vim.keymap.set("i", "<C-c>", "<Esc>")
