-- ~/.config/nvim/lua/plugins/blink.lua
return {
  "saghen/blink.cmp",
  opts = {
    keymap = {
      preset = "default",
      ["<Tab>"] = { "accept", "fallback" },
      ["<CR>"] = {}, -- enter로 accept 비활성화 (원하면 삭제)
    },
  },
}
