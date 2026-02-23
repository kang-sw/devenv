-- ~/.config/nvim/lua/plugins/dap.lua
return {
  -- Rust / C / C++ 디버깅 (codelldb)
  {
    "mfussenegger/nvim-dap",
    optional = true,
    config = function()
      local dap = require("dap")

      -- codelldb 경로 (Mason이 설치한 위치)
      local codelldb_path = vim.fn.stdpath("data") .. "/mason/bin/codelldb"

      -- C/C++
      dap.adapters.codelldb = {
        type = "server",
        port = "${port}",
        executable = {
          command = codelldb_path,
          args = { "--port", "${port}" },
        },
      }
      dap.configurations.cpp = {
        {
          name = "Launch (codelldb)",
          type = "codelldb",
          request = "launch",
          program = function()
            return vim.fn.input("Binary path: ", vim.fn.getcwd() .. "/", "file")
          end,
          cwd = "${workspaceFolder}",
          stopOnEntry = false,
        },
      }
      dap.configurations.c = dap.configurations.cpp -- C도 동일 설정 재사용

      -- Python (debugpy)
      dap.adapters.python = {
        type = "executable",
        command = vim.fn.stdpath("data") .. "/mason/packages/debugpy/venv/bin/python",
        args = { "-m", "debugpy.adapter" },
      }
      dap.configurations.python = {
        {
          name = "Launch (debugpy)",
          type = "python",
          request = "launch",
          program = "${file}", -- 현재 파일 실행
          pythonPath = function()
            -- 가상환경 자동 감지
            local venv = os.getenv("VIRTUAL_ENV")
            if venv then
              return venv .. "/bin/python"
            end
            return "python3"
          end,
        },
      }
    end,
  },
}
