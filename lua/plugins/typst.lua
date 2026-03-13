return {
  {
    "neovim/nvim-lspconfig",
    ft = { "typst" },
    keys = {
      {
        "<leader>tp",
        function()
          if vim.b.typst_preview_job then
            vim.fn.jobstop(vim.b.typst_preview_job)
            vim.b.typst_preview_job = nil
            vim.notify("Typst preview stopped", vim.log.levels.INFO)
          else
            local bin = vim.fn.stdpath("data") .. "/mason/bin/tinymist"
            local file = vim.fn.expand("%:p")
            local job_id = vim.fn.jobstart({ bin, "preview", file }, { detach = false })
            if job_id > 0 then
              vim.b.typst_preview_job = job_id
              vim.notify("Typst preview started", vim.log.levels.INFO)
            else
              vim.notify("Failed to start tinymist", vim.log.levels.ERROR)
            end
          end
        end,
        desc = "Toggle Typst Preview",
        ft = "typst",
      },
    },
  },
}
