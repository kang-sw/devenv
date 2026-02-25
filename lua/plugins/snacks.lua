return {
  "folke/snacks.nvim",
  opts = {
    explorer = {
      -- 여기서 키 오버라이드
    },
    picker = {
      sources = {
        explorer = {
          jump = { close = true },
          layout = {
            layout = {
              backdrop = false,
              width = 0.4,
              min_width = 40,
              height = 0.8,
              position = "float", -- 핵심!
              border = "rounded",
              box = "vertical",
              { win = "input", height = 1, border = "rounded", title = "{title}", title_pos = "center" },
              { win = "list", border = "none" },
            },
          },
          win = {
            list = {
              keys = {
                ["<C-j>"] = false,
                ["<C-k>"] = false,
              },
            },
          },
        },
      },
    },
  },
}
