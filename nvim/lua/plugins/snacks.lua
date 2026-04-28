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
                -- wincmd is avoided entirely: in picker context it moves to unexpected windows
                -- and breaks subsequent code. Instead, scan window positions directly.
                ["<C-M-h>"] = { function()
                  local cur = vim.api.nvim_get_current_win()
                  local cur_col = vim.api.nvim_win_get_position(cur)[2]
                  local best, best_col = nil, -1
                  for _, w in ipairs(vim.api.nvim_tabpage_list_wins(0)) do
                    if w == cur or vim.api.nvim_win_get_config(w).relative ~= "" then goto skip end
                    if vim.bo[vim.api.nvim_win_get_buf(w)].filetype:find("snacks_picker") then goto skip end
                    local col = vim.api.nvim_win_get_position(w)[2]
                    if col < cur_col and col > best_col then best, best_col = w, col end
                    ::skip::
                  end
                  if best then
                    vim.api.nvim_set_current_win(best)
                  else
                    local edge = vim.fn.system("tmux display-message -p '#{pane_at_left}'"):gsub("%s+", "")
                    if edge ~= "1" then vim.fn.system("tmux select-pane -L") end
                  end
                end },
                ["<C-M-l>"] = { function()
                  local cur = vim.api.nvim_get_current_win()
                  local cur_col = vim.api.nvim_win_get_position(cur)[2]
                  local cur_w = vim.api.nvim_win_get_width(cur)
                  local best, best_col = nil, math.huge
                  for _, w in ipairs(vim.api.nvim_tabpage_list_wins(0)) do
                    if w == cur or vim.api.nvim_win_get_config(w).relative ~= "" then goto skip end
                    if vim.bo[vim.api.nvim_win_get_buf(w)].filetype:find("snacks_picker") then goto skip end
                    local col = vim.api.nvim_win_get_position(w)[2]
                    if col >= cur_col + cur_w and col < best_col then best, best_col = w, col end
                    ::skip::
                  end
                  if best then
                    vim.api.nvim_set_current_win(best)
                  else
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
