return {
  {
    "mfussenegger/nvim-lint",
    opts = {
      linters_by_ft = {
        markdown = {}, -- markdownlint 비활성화
      },
    },
  },
  {
    "stevearc/conform.nvim",
    opts = function(_, opts)
      opts.formatters_by_ft = opts.formatters_by_ft or {}

      -- prettier로 처리 가능한 형식들
      local prettier = { "prettier" }
      opts.formatters_by_ft.json = prettier
      opts.formatters_by_ft.jsonc = prettier
      opts.formatters_by_ft.yaml = prettier
      opts.formatters_by_ft.html = prettier
      opts.formatters_by_ft.css = prettier
      opts.formatters_by_ft.markdown = {}
      opts.formatters_by_ft.graphql = prettier

      -- TOML 전용
      opts.formatters_by_ft.toml = { "taplo" }

      -- XML
      -- opts.formatters_by_ft.xml = { "xmlformat" }

      return opts
    end,
  },
}
