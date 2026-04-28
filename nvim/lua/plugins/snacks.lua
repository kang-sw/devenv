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
              keys = {
                ["<C-M-j>"] = { function() vim.fn.system("tmux select-pane -D") end },
                ["<C-M-k>"] = { function() vim.fn.system("tmux select-pane -U") end },
                -- winnr check is unreliable in picker context: wincmd can silently move to
                -- an internal snacks window (backdrop/input) and return a changed winnr.
                -- Instead: attempt wincmd, then check filetype — if still in any snacks_picker
                -- buffer, treat the move as failed and delegate to tmux.
                ["<C-M-h>"] = { function()
                  vim.cmd("wincmd h")
                  if vim.bo.filetype:find("snacks_picker") then
                    local edge = vim.fn.system("tmux display-message -p '#{pane_at_left}'"):gsub("%s+", "")
                    if edge ~= "1" then vim.fn.system("tmux select-pane -L") end
                  end
                end },
                ["<C-M-l>"] = { function()
                  vim.cmd("wincmd l")
                  if vim.bo.filetype:find("snacks_picker") then
                    local edge = vim.fn.system("tmux display-message -p '#{pane_at_right}'"):gsub("%s+", "")
                    if edge ~= "1" then vim.fn.system("tmux select-pane -R") end
                  end
                end },
              },
            },
          },
        },
      },
    },
  },
}
