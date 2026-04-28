return {
  "folke/snacks.nvim",
  keys = {
    {
      "<leader>e",
      function()
        local explorer = Snacks.picker.get({ source = "explorer" })[1]
        if explorer then
          if explorer:is_focused() then
            explorer:close()
          else
            explorer:focus()
          end
        else
          Snacks.explorer.open()
        end
      end,
      desc = "Explorer (smart toggle)",
    },
  },
  opts = {
    explorer = {
      -- 여기서 키 오버라이드
    },
    terminal = {
      win = { keys = { term_normal = false } },
    },
    picker = {
      sources = {
        explorer = {
          jump = { close = false },
          layout = {
            layout = {
              backdrop = true,
              width = 35,
              -- min_width = 40,
              -- height = 0.8,
              position = "left", -- 핵심!
              -- border = "rounded",
              box = "vertical",
              -- { win = "input", height = 1, border = "rounded", title = "{title}", title_pos = "center" },
              -- { win = "list", border = "none" },
            },
          },
          win = {
            list = {
              keys = {},
            },
          },
        },
      },
    },
  },
}
