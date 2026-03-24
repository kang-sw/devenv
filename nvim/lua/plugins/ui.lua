-- ~/.config/nvim/lua/plugins/ui.lua
return {
  {
    "akinsho/bufferline.nvim",
    opts = {
      options = {
        always_show_bufferline = true, -- 탭 1개여도 항상 표시
      },
    },
  },
  {
    "MeanderingProgrammer/render-markdown.nvim",
    config = function(_, opts)
      -- ---------------------------------------------------------------------------
      -- Monkey-patch: scale down wide table column widths and word-wrap cell content
      -- so that tables never cause Neovim's line-wrap to break virtual-text decorations.
      -- ---------------------------------------------------------------------------

      ---Wrap `text` to fit within `max_w` display columns.
      ---Returns a list of lines (always at least one entry).
      ---@param text string
      ---@param max_w integer
      ---@return string[]
      local function wrap_text(text, max_w)
        if max_w <= 0 then
          return { "" }
        end
        text = text:match("^%s*(.-)%s*$") or ""
        if vim.fn.strdisplaywidth(text) <= max_w then
          return { text }
        end
        local lines, current, current_w = {}, "", 0
        for _, word in ipairs(vim.split(text, " ", { plain = true })) do
          local word_w = vim.fn.strdisplaywidth(word)
          local sep_w = current == "" and 0 or 1
          if current_w + sep_w + word_w <= max_w then
            current = current == "" and word or (current .. " " .. word)
            current_w = current_w + sep_w + word_w
          else
            if current ~= "" then
              lines[#lines + 1] = current
            end
            -- Hard-break word if it alone exceeds max_w
            while vim.fn.strdisplaywidth(word) > max_w do
              local chunk = vim.fn.strcharpart(word, 0, max_w)
              lines[#lines + 1] = chunk
              word = vim.fn.strcharpart(word, vim.fn.strchars(chunk))
            end
            current = word
            current_w = vim.fn.strdisplaywidth(word)
          end
        end
        if current ~= "" then
          lines[#lines + 1] = current
        end
        return #lines > 0 and lines or { "" }
      end

      local ok, TableRender = pcall(require, "render-markdown.render.markdown.table")
      if ok then
        -- -----------------------------------------------------------------------
        -- Patch Render.setup: scale col widths to fit inside the current window.
        -- -----------------------------------------------------------------------
        local orig_setup = TableRender.setup
        TableRender.setup = function(self)
          local result = orig_setup(self)
          if result then
            local win_w = vim.api.nvim_win_get_width(0)
            local padding = self.config.padding or 1
            local cols = self.data.cols
            local n = #cols

            -- Total rendered width = (n+1) pipe chars + sum of inter-pipe widths.
            -- col.width already includes the padding spaces on each side.
            local content_sum = 0
            for _, col in ipairs(cols) do
              content_sum = content_sum + col.width
            end
            local total = (n + 1) + content_sum

            local margin = 8
            if total > win_w - margin and content_sum > 0 then
              local available = win_w - (n + 1) - margin
              local min_col_w = 2 * padding + 1 -- at least 1 char of content per column
              if available >= min_col_w * n then
                for _, col in ipairs(cols) do
                  col.width = math.max(min_col_w, math.floor(col.width * available / content_sum))
                end
                self._multiline = true
              end
            end
          end
          return result
        end

        -- -----------------------------------------------------------------------
        -- Render.multiline_row: conceal the raw buffer row (makes it visually
        -- empty → no wrap allocation), overlay vrow-1 on that blank visual line,
        -- then attach overflow vrows as virt_lines below.
        --
        -- Uses char-level conceal='' (works on all Neovim versions that
        -- render-markdown supports, no conceal_lines API needed).
        -- render-markdown sets conceallevel >= 1 via win_options, so the
        -- concealment is always active while the plugin is rendering.
        -- -----------------------------------------------------------------------
        TableRender.multiline_row = function(self, row)
          local header = row.node.type == "pipe_table_header"
          local highlight = header and self.config.head or self.config.row
          local icon = self.config.border[10] -- '│'
          local padding = self.config.padding or 1
          local pad_str = string.rep(" ", padding)

          -- Word-wrap each cell to its column's content width.
          local wrapped = {}
          local num_vrows = 1
          for i, cell in ipairs(row.cells) do
            local col_content_w = self.data.cols[i].width - 2 * padding
            local text = cell.node.text:match("^%s*(.-)%s*$") or ""
            wrapped[i] = wrap_text(text, math.max(1, col_content_w))
            num_vrows = math.max(num_vrows, #wrapped[i])
          end

          -- Conceal every character in the buffer row with '' (empty replacement).
          -- With conceallevel >= 1 the row becomes visually empty: exactly 1 visual
          -- line, no wrap continuation lines, virt_lines/overlay unaffected.
          self.marks:over(self.config, true, row.node, {
            conceal = "",
          })

          -- vrow 1: overlay on the now-blank concealed buffer line.
          -- vrow 2+: virt_lines below (between this and the next buffer line).
          for vrow = 1, num_vrows do
            local line = self:line()
            for i = 1, #row.cells do
              local col_w = self.data.cols[i].width
              local content = wrapped[i][vrow] or ""
              local content_w = vim.fn.strdisplaywidth(content)
              local fill = col_w - padding - content_w
              line:text(icon, highlight)
              line:text(pad_str, highlight)
              line:text(content, highlight)
              line:pad(math.max(0, fill))
            end
            line:text(icon, highlight)

            if vrow == 1 then
              self.marks:over(self.config, "table_border", row.node, {
                virt_text = line:get(),
                virt_text_pos = "overlay",
              })
            else
              self.marks:add(self.config, "virtual_lines", row.node.start_row, 0, {
                virt_lines = { self:indent():line(true):extend(line):get() },
                virt_lines_above = false,
              })
            end
          end
        end

        -- -----------------------------------------------------------------------
        -- Patch Render.run: route to multiline path when columns were scaled.
        -- -----------------------------------------------------------------------
        local orig_run = TableRender.run
        TableRender.run = function(self)
          if not self._multiline then
            orig_run(self)
            return
          end
          self:delimiter()
          for _, row in ipairs(self.data.rows) do
            self:multiline_row(row)
          end
          if self.config.border_enabled then
            self:border()
          end
        end
      end -- if ok

      require("render-markdown").setup(opts)
    end,
    opts = {
      win_options = {
        -- Keep conceal active even on the cursor line. Without this,
        -- Neovim lifts conceal on the cursor row, re-exposing the wide
        -- raw table text which then wraps and breaks the layout.
        -- Trade-off: raw markdown syntax (# for headings, ** for bold)
        -- is also hidden on the cursor line; use :RenderMarkdown toggle
        -- to see raw text when needed.
        concealcursor = { rendered = "nvic" },
      },
      code = {
        border = "thin", -- 코드 블록 위아래에 얇은 구분선 표시
      },
      bullet = {
        icons = { "·", "∘", "▸", "▹" }, -- 더 작은 bullet 아이콘
      },
      html = {
        comment = {
          conceal = false, -- HTML 주석 그대로 표시
        },
      },
    },
  },
}
