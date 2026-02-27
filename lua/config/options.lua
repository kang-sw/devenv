-- Options are automatically loaded before lazy.nvim startup
-- Default options that are always set: https://github.com/LazyVim/LazyVim/blob/main/lua/lazyvim/config/options.lua
-- Add any additional options here

-- 저장 시 포맷팅 비활성화.
vim.g.autoformat = true
vim.opt.smoothscroll = false

-- LazyVim이 spell을 켜는 것을 막기
vim.opt.spelllang = { "en", "cjk" }
