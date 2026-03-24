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
      -- Monkey-patch: scale wide table columns to window width and word-wrap cells.
      -- Instead of fighting Neovim's soft-wrap, we WORK WITH it: compute the
      -- physical screen lines the raw buffer text occupies, then overlay each one.
      -- ---------------------------------------------------------------------------

      ---Wrap `text` to fit within `max_w` display columns.
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

      ---Compute 0-based byte offsets where Neovim soft-wraps a buffer line.
      ---@param text string
      ---@param w integer  text area width (window width minus signcolumn etc.)
      ---@return integer[]
      local function get_wrap_offsets(text, w)
        local offsets = { 0 }
        if w <= 0 or vim.fn.strdisplaywidth(text) <= w then
          return offsets
        end
        local nchars = vim.fn.strchars(text)
        local byte = 0
        local dw = 0
        for ci = 0, nchars - 1 do
          local ch = vim.fn.strcharpart(text, ci, 1)
          local cw = vim.fn.strdisplaywidth(ch)
          if dw + cw > w then
            offsets[#offsets + 1] = byte
            dw = cw
          else
            dw = dw + cw
          end
          byte = byte + #ch
        end
        return offsets
      end

      local ok, TableRender = pcall(require, "render-markdown.render.markdown.table")
      if ok then
        -- -----------------------------------------------------------------------
        -- Patch Render.setup: scale col widths to fit the text area.
        -- -----------------------------------------------------------------------
        local orig_setup = TableRender.setup
        TableRender.setup = function(self)
          local result = orig_setup(self)
          if result then
            local info = vim.fn.getwininfo(vim.api.nvim_get_current_win())[1]
            local text_w = info.width - info.textoff
            local padding = self.config.padding or 1
            local cols = self.data.cols
            local n = #cols

            local content_sum = 0
            for _, col in ipairs(cols) do
              content_sum = content_sum + col.width
            end
            local total = (n + 1) + content_sum

            local margin = 2
            if total > text_w - margin and content_sum > 0 then
              local available = text_w - (n + 1) - margin
              local min_col_w = 2 * padding + 1
              if available >= min_col_w * n then
                for _, col in ipairs(cols) do
                  col.width = math.max(min_col_w, math.floor(col.width * available / content_sum))
                end
                self._multiline = true
                self._text_w = text_w
              end
            end
          end
          return result
        end

        -- -----------------------------------------------------------------------
        -- multiline_row: overlay on each physical wrap line, virt_lines for extra.
        --
        -- Neovim wraps the raw buffer line into N physical screen lines.  We
        -- compute where those breaks are, then place an overlay extmark at
        -- each break byte-offset.  overlay at col X appears on the screen line
        -- where col X is rendered — exactly what we need.
        -- -----------------------------------------------------------------------
        TableRender.multiline_row = function(self, row)
          local header = row.node.type == "pipe_table_header"
          local highlight = header and self.config.head or self.config.row
          local icon = self.config.border[10]
          local padding = self.config.padding or 1
          local pad_str = string.rep(" ", padding)
          local text_w = self._text_w

          -- Word-wrap each cell to its column's content width.
          local wrapped = {}
          local num_vrows = 1
          for i, cell in ipairs(row.cells) do
            local col_content_w = self.data.cols[i].width - 2 * padding
            local text = cell.node.text:match("^%s*(.-)%s*$") or ""
            wrapped[i] = wrap_text(text, math.max(1, col_content_w))
            num_vrows = math.max(num_vrows, #wrapped[i])
          end

          -- Physical screen lines the raw buffer text occupies.
          local offsets = get_wrap_offsets(row.node.text, text_w)
          local num_phys = #offsets

          -- Build a virtual table row (nil vrow → empty row with borders only).
          local function build_line(vrow)
            local line = self:line()
            for i = 1, #row.cells do
              local col_w = self.data.cols[i].width
              local content = vrow and wrapped[i][vrow] or ""
              local content_w = vim.fn.strdisplaywidth(content)
              local fill = col_w - padding - content_w
              line:text(icon, highlight)
              line:text(pad_str, highlight)
              line:text(content, highlight)
              line:pad(math.max(0, fill))
            end
            line:text(icon, highlight)
            -- Pad to cover the full screen line so no raw text peeks through.
            line:pad(math.max(0, text_w - line:width()))
            return line
          end

          -- Conceal the raw buffer text so it doesn't bleed through overlays.
          -- This doesn't prevent wrap (layout is computed before conceal), but
          -- makes the underlying text invisible for a cleaner overlay result.
          self.marks:over(self.config, true, row.node, { conceal = "" })

          -- Overlay on each physical screen line.
          for si = 1, num_phys do
            local vrow = si <= num_vrows and si or nil
            local line = build_line(vrow)
            self.marks:add(self.config, "table_border", row.node.start_row, offsets[si], {
              virt_text = line:get(),
              virt_text_pos = "overlay",
            })
          end

          -- Extra vrows that don't fit in the physical lines → virt_lines.
          for vi = num_phys + 1, num_vrows do
            local line = build_line(vi)
            self.marks:add(self.config, "virtual_lines", row.node.start_row, 0, {
              virt_lines = { self:indent():line(true):extend(line):get() },
              virt_lines_above = false,
            })
          end
        end

        -- -----------------------------------------------------------------------
        -- Patch Render.run
        -- -----------------------------------------------------------------------
        local orig_run = TableRender.run
        TableRender.run = function(self)
          if not self._multiline then
            orig_run(self)
            return
          end
          local text_w = self._text_w

          -- Delimiter: conceal raw text, render normally, blank-overlay continuations.
          self.marks:over(self.config, true, self.data.delim, { conceal = "" })
          self:delimiter()
          local delim_offsets = get_wrap_offsets(self.data.delim.text, text_w)
          for si = 2, #delim_offsets do
            self.marks:add(self.config, "table_border", self.data.delim.start_row, delim_offsets[si], {
              virt_text = self:line():pad(text_w):get(),
              virt_text_pos = "overlay",
            })
          end

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
