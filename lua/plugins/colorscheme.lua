-- ~/.config/nvim/lua/plugins/colorscheme.lua
return {
  -- VSCode Dark+ 베이스 테마
  {
    "Mofiqul/vscode.nvim",
    lazy = false,
    priority = 1000,
    config = function()
      require("vscode").setup({
        transparent = false,
        italic_comments = true,
        disable_nvimtree_bg = true,
      })
      require("vscode").load("dark")
    end,
  },

  -- LazyVim 기본 colorscheme 덮어쓰기
  {
    "LazyVim/LazyVim",
    opts = { colorscheme = "vscode" },
  },

  -- 커스텀 시맨틱 컬러링
  {
    "folke/lazy.nvim",
    init = function()
      -- ──────────────────────────────────────────
      -- 유틸: highlight 설정 헬퍼
      -- ──────────────────────────────────────────
      local function hl(group, opts)
        vim.api.nvim_set_hl(0, group, opts)
      end

      vim.api.nvim_create_autocmd("ColorScheme", {
        pattern = "*",
        callback = function()
          -- ════════════════════════════════════════
          -- TextMate / TreeSitter 대응
          -- ════════════════════════════════════════

          -- keyword.control → #d8a0df
          hl("@keyword.conditional", { fg = "#d8a0df" })
          hl("@keyword.repeat", { fg = "#d8a0df" })
          hl("@keyword.return", { fg = "#d8a0df" })
          hl("@keyword.exception", { fg = "#d8a0df" })

          -- keyword → #569CD6
          hl("@keyword", { fg = "#569CD6" })
          hl("@keyword.import", { fg = "#569CD6" })
          hl("@keyword.operator", { fg = "#569CD6" })

          -- entity.name.function.preprocessor → #c547b0
          hl("@function.macro", { fg = "#c547b0" })
          hl("@lsp.type.macro", { fg = "#c547b0" })

          -- entity.name.namespace → #CBA805
          hl("@module", { fg = "#CBA805" })
          hl("@namespace", { fg = "#CBA805" })
          hl("@lsp.type.namespace", { fg = "#CBA805" })

          -- entity.name.type → #3ABEA4
          hl("@type", { fg = "#3ABEA4" })
          hl("@type.builtin", { fg = "#3ABEA4" })
          hl("@lsp.type.type", { fg = "#3ABEA4" })
          hl("@lsp.type.class", { fg = "#3ABEA4" })
          hl("@lsp.type.struct", { fg = "#3ABEA4" })
          hl("@lsp.type.enum", { fg = "#3ABEA4" })
          hl("@lsp.type.builtinType", { fg = "#3ABEA4" })

          -- entity.name.type.trait → #00f0bc bold
          hl("@lsp.type.interface", { fg = "#03fec8", bold = true })

          -- entity.name.function → #49b3cb
          hl("@function", { fg = "#2cc5eb" })
          hl("@function.call", { fg = "#2cc5eb" })
          hl("@lsp.type.function", { fg = "#2cc5eb" })

          -- method → #49b3cb
          hl("@function.method", { fg = "#49b3cb" })
          hl("@function.method.call", { fg = "#49b3cb" })
          hl("@lsp.type.method", { fg = "#49b3cb" })

          -- variable.parameter → #e1e0b7
          hl("@variable.parameter", { fg = "#e1e0b7" })
          hl("@lsp.type.parameter", { fg = "#e1e0b7" })

          -- variable / variable.other.local → #969696
          hl("@variable", { fg = "#969696" })
          hl("@lsp.type.variable", { fg = "#969696" })

          -- variable.other.property → #0eb16d
          hl("@variable.member", { fg = "#0eb16d" })
          hl("@lsp.type.property", { fg = "#0eb16d" })

          -- variable.other.enummember → #B8D7A3 italic
          hl("@lsp.type.enumMember", { fg = "#B8D7A3", italic = true })

          -- variable.other.global → #009797
          hl("@lsp.typemod.variable.global", { fg = "#009797" })

          -- comment → #717a69
          hl("@comment", { fg = "#717a69", italic = true })
          hl("Comment", { fg = "#717a69", italic = true })

          -- constant → #dfffad italic
          hl("@constant", { fg = "#dfffad", italic = true })
          hl("@constant.builtin", { fg = "#dfffad", italic = true })
          hl("@lsp.type.enumMember", { fg = "#B8D7A3", italic = true })

          -- string → #FFBB94
          hl("@string", { fg = "#FFBB94" })
          hl("String", { fg = "#FFBB94" })

          -- entity.other.attribute-name → #7DB08A
          hl("@attribute", { fg = "#7DB08A" })
          hl("@lsp.type.attribute", { fg = "#7DB08A" })

          -- entity.name.tag → #2AB29E
          hl("@tag", { fg = "#2AB29E" })
          hl("@tag.builtin", { fg = "#2AB29E" })

          -- ════════════════════════════════════════
          -- Semantic Token modifier 조합
          -- ════════════════════════════════════════

          -- function.static → #49b3cb italic
          hl("@lsp.typemod.function.static", { fg = "#49b3cb", italic = true })

          -- function.static.trait (Rust trait 구현 정적 메서드) → #45e3e8 italic
          hl("@lsp.typemod.function.static.trait", { fg = "#45e3e8", italic = true })

          -- method.trait → #45e3e8
          hl("@lsp.typemod.method.trait", { fg = "#45e3e8" })

          -- typeParameter → #20999D
          hl("@lsp.type.typeParameter", { fg = "#20999D" })

          -- variable.constant → #dfffad italic
          hl("@lsp.typemod.variable.constant", { fg = "#dfffad", italic = true })

          -- variable.static → #FAE6C1 bold italic
          hl("@lsp.typemod.variable.static", { fg = "#FAE6C1", bold = true, italic = true })

          -- variable.static.constant → #dfffad italic (bold 없음)
          hl("@lsp.typemod.variable.static.constant", { fg = "#dfffad", italic = true })

          -- property:python → #01a2ff
          hl("@lsp.typemod.property.python", { fg = "#01a2ff" })
        end,
      })
    end,
  },
}
