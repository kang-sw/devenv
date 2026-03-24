-- ~/.config/nvim/lua/plugins/trouble.lua
return {
  "folke/trouble.nvim",
  opts = {
    focus = true,
    win = {
      type = "float",
      border = "rounded",
      relative = "editor",
      title = "Preview",
      title_pos = "center",
      position = { 0, -2 },
      size = { width = 0.3, height = 0.3 },
      zindex = 200,
    },
  },
}
