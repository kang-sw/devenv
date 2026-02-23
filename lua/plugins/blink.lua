-- ~/.config/nvim/lua/plugins/blink.lua
return {
  "saghen/blink.cmp",
  opts = {
    keymap = {
      preset = "default",
      ["<Tab>"] = { "accept", "fallback" },
      ["<CR>"] = {}, -- enter로 accept 비활성화 (원하면 삭제)
    },
    completion = {
      menu = {
        border = "rounded",
      },
      trigger = {
        show_on_insert_on_trigger_character = true,
        show_delay_ms = 50, -- 기본값이 꽤 길어요
      },
    },
    documentation = {
      window = {
        border = "rounded",
      },
    },
    signature = {
      enabled = true,
      trigger = {
        enabled = true, -- 자동 팝업 끄고 수동으로만
      },
      window = {
        show_delay_ms = 50,
        border = "rounded",
      },
    },
  },
}
