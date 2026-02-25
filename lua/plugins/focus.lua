-- ~/.config/nvim/lua/plugins/focus.lua
return {
  "nvim-focus/focus.nvim",
  opts = {
    autoresize = {
      enable = false,
      width = 0, -- 활성 창 목표 너비 (0 = 자동)
      height = 0, -- 활성 창 목표 높이 (0 = 자동)
      minwidth = 30, -- 비활성 창 최소 너비 (이게 핵심!)
      minheight = 10, -- 비활성 창 최소 높이
      height_quickfix = 10,
    },
    excluded_filetypes = { "terminal", "toggleterm", "fterm" },
    excluded_buftypes = { "terminal" }, -- 이게 핵심
    split = {
      bufnew = false,
    },
  },
}
