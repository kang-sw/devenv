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
                ["<C-M-j>"] = { function() vim.cmd("TmuxNavigateDown") end },
                ["<C-M-k>"] = { function() vim.cmd("TmuxNavigateUp") end },
                ["<C-M-h>"] = { function()
                  local winnr = vim.fn.winnr()
                  vim.cmd("wincmd h")
                  if vim.fn.winnr() == winnr then
                    local edge = vim.fn.system("tmux display-message -p '#{pane_at_left}'"):gsub("%s+", "")
                    if edge == "1" then
                      vim.fn.system(vim.fn.expand("~/.devenv-scripts/tmux-cross-window.sh") .. " right")
                    else
                      vim.fn.system("tmux select-pane -L")
                    end
                  end
                end },
                ["<C-M-l>"] = { function()
                  local winnr = vim.fn.winnr()
                  vim.cmd("wincmd l")
                  if vim.fn.winnr() == winnr then
                    local edge = vim.fn.system("tmux display-message -p '#{pane_at_right}'"):gsub("%s+", "")
                    if edge == "1" then
                      vim.fn.system(vim.fn.expand("~/.devenv-scripts/tmux-cross-window.sh") .. " left")
                    else
                      vim.fn.system("tmux select-pane -R")
                    end
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
