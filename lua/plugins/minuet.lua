-- ~/.config/nvim/lua/plugins/minuet.lua
return {
  {
    "milanglacier/minuet-ai.nvim",
    dependencies = { "nvim-lua/plenary.nvim" },
    opts = {
      auto_trigger_ft = {},
      provider = "claude", -- "gemini" 으로 바꿔도 됨
      provider_options = {
        -- claude = {
        --   api_key = "ANTHROPIC_API_KEY", -- env var 이름
        --   model = "claude-haiku-4-5", -- 빠른 응답용으로 haiku 추천
        --   max_tokens = 512,
        -- },
        gemini = {
          api_key = "GEMINI_API_KEY",
          model = "gemini-3-flash-preview",
        },
      },
      request_timeout = 3, -- 초 단위, 너무 길면 타이핑 끊김
      throttle = 1000, -- ms, 타이핑 중 요청 빈도
    },
  },
  {
    "saghen/blink.cmp",
    opts = {
      sources = {
        default = { "lsp", "path", "snippets", "buffer", "minuet" },
        providers = {
          minuet = {
            name = "minuet",
            module = "minuet.blink",
            score_offset = 8, -- 다른 suggestion보다 위로
          },
        },
      },
      keymap = {
        ["<Tab>"] = { "accept", "fallback" },
        ["<A-y>"] = { "select_and_accept" }, -- 원하는 키로
        ["<C-space>"] = {
          function(cmp)
            return cmp.show({ providers = { "lsp", "path", "snippets", "buffer" } })
          end,
          "fallback",
        },
        ["<A-space>"] = {
          function(cmp)
            return cmp.show({ providers = { "minuet" } })
          end,
          "fallback",
        },
      },
    },
  },
}
