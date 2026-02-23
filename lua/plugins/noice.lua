return {
  "folke/noice.nvim",
  opts = {
    presets = {
      lsp_doc_border = true,
    },
    lsp = {
      signature = {
        enabled = false, -- noice의 signature는 끄기
      },
    },
  },
}
