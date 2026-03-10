-- ~/.config/nvim/lua/plugins/ui.lua
return {
  {
    "akinsho/bufferline.nvim",
    opts = {
      options = {
        always_show_bufferline = true, -- 탭 1개여도 항상 표시
      },
    },
  },
  {
    "MeanderingProgrammer/render-markdown.nvim",
    opts = {
      code = {
        border = "thin", -- 코드 블록 위아래에 얇은 구분선 표시
      },
      bullet = {
        icons = { "·", "∘", "▸", "▹" }, -- 더 작은 bullet 아이콘
      },
      html = {
        comment = {
          conceal = false, -- HTML 주석 그대로 표시
        },
      },
    },
  },
}
