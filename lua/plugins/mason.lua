-- ~/.config/nvim/lua/plugins/mason.lua
return {
  {
    "WhoIsSethDaniel/mason-tool-installer.nvim",
    dependencies = { "mason-org/mason.nvim" },
    opts = {
      ensure_installed = {
        -- LSP
        "rust-analyzer",
        "clangd",
        "pyright",
        -- 디버거
        "codelldb",
        "debugpy",
        -- 포매터
        "clang-format",
        "prettier",
        "taplo",
        "xmlformat",
        -- 기타
        "tree-sitter-cli",
      },
      auto_update = false,
      run_on_start = true, -- nvim 시작 시 없는 것만 자동 설치
    },
  },
}
