-- Autocmds are automatically loaded on the VeryLazy event
-- Default autocmds that are always set: https://github.com/LazyVim/LazyVim/blob/main/lua/lazyvim/config/autocmds.lua
--
-- Add any additional autocmds here
-- with `vim.api.nvim_create_autocmd`
--
-- Or remove existing autocmds by their group name (which is prefixed with `lazyvim_` for the defaults)
-- e.g. vim.api.nvim_del_augroup_by_name("lazyvim_wrap_spell")

-- ------------------------------------------------------------
-- im-select: Insert 모드 진입/이탈 시 입력기 자동 전환
-- ------------------------------------------------------------
local last_im = "com.apple.keylayout.ABC"
local english_im = "com.apple.keylayout.ABC"

local function get_im()
  local handle = io.popen("im-select")
  local result = handle:read("*a")
  handle:close()
  return result:gsub("%s+", "")
end

local function set_im(im_id)
  os.execute("im-select " .. im_id)
end

vim.api.nvim_create_autocmd("InsertLeave", {
  pattern = "*",
  callback = function()
    last_im = get_im()
    if last_im ~= english_im then
      set_im(english_im)
    end
  end,
})

vim.api.nvim_create_autocmd("InsertEnter", {
  pattern = "*",
  callback = function()
    if last_im ~= get_im() then
      set_im(last_im)
    end
  end,
})

-- ------------------------------------------------------------
-- // 주석에서 자동 주석 prefix 제거
vim.api.nvim_create_autocmd("FileType", {
  pattern = { "javascript", "typescript", "javascriptreact", "typescriptreact", "c", "cpp", "rust", "go" },
  callback = function()
    vim.opt_local.comments:remove("://")
  end,
})

-- ------------------------------------------------------------
vim.api.nvim_create_autocmd("FileType", {
  pattern = "markdown",
  callback = function()
    vim.opt_local.tabstop = 4
    vim.opt_local.shiftwidth = 4
    vim.opt_local.softtabstop = 4
  end,
})
