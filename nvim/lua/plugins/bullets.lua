return {
  {
    "dkarter/bullets.vim",
    ft = { "markdown", "text" },
    init = function()
      vim.g.bullets_enabled_file_types = { "markdown", "text" }
      vim.g.bullets_renumber_on_change = 1
      vim.g.bullets_set_mappings = 1
    end,
    config = function()
      vim.api.nvim_create_autocmd("FileType", {
        pattern = "markdown",
        callback = function(ev)
          local function is_bullet_line()
            local line = vim.api.nvim_get_current_line()
            return line:match("^%s*[-*+]%s") or line:match("^%s*%d+[.)]%s")
          end

          local function feedkeys(keys)
            vim.api.nvim_feedkeys(
              vim.api.nvim_replace_termcodes(keys, true, false, true),
              "n",
              false
            )
          end

          vim.keymap.set("i", "<Tab>", function()
            local ok, blink = pcall(require, "blink.cmp")
            if ok and blink.is_visible() then
              blink.accept()
              return
            end
            if is_bullet_line() then
              local row, col = unpack(vim.api.nvim_win_get_cursor(0))
              local line = vim.api.nvim_get_current_line()
              local sw = vim.bo.shiftwidth
              vim.api.nvim_set_current_line(string.rep(" ", sw) .. line)
              vim.api.nvim_win_set_cursor(0, { row, col + sw })
              vim.cmd("RenumberList")
            else
              feedkeys("<Tab>")
            end
          end, { buffer = ev.buf })

          vim.keymap.set("i", "<S-Tab>", function()
            if is_bullet_line() then
              local row, col = unpack(vim.api.nvim_win_get_cursor(0))
              local line = vim.api.nvim_get_current_line()
              local sw = vim.bo.shiftwidth
              local indent = line:match("^(%s*)")
              local remove = math.min(sw, #indent)
              if remove > 0 then
                vim.api.nvim_set_current_line(line:sub(remove + 1))
                vim.api.nvim_win_set_cursor(0, { row, math.max(0, col - remove) })
              end
              vim.cmd("RenumberList")
            else
              feedkeys("<S-Tab>")
            end
          end, { buffer = ev.buf })
        end,
      })
    end,
  },
}
