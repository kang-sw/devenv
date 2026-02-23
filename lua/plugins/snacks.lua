return {
  "folke/snacks.nvim",
  opts = {
    explorer = {
      -- 여기서 키 오버라이드
    },
    picker = {
      sources = {
        explorer = {
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
