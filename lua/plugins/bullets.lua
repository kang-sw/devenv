return {
  {
    "dkarter/bullets.vim",
    ft = { "markdown", "text" },
    init = function()
      vim.g.bullets_enabled_file_types = { "markdown", "text" }
      vim.g.bullets_renumber_on_change = 1
      vim.g.bullets_set_mappings = 1
    end,
  },
}
