-- init.lua 상단에 추가
if vim.g.vscode then
    local vscode = require('vscode')

    -- Undo (u)
    vim.keymap.set('n', 'u', function()
        vscode.action('undo')
    end)

    -- Redo (Ctrl + r)
    vim.keymap.set('n', '<C-r>', function()
        vscode.action('redo')
    end)

    -- Folding (코드 접기) 설정
    -- zc: 현재 블록 접기
    vim.keymap.set('n', 'zc', function() vscode.action('editor.fold') end)
    -- zo: 현재 블록 펼치기
    vim.keymap.set('n', 'zo', function() vscode.action('editor.unfold') end)
    -- za: 현재 블록 토글 (접기/펼치기)
    vim.keymap.set('n', 'za', function() vscode.action('editor.toggleFold') end)
    -- zR: 모든 블록 펼치기
    vim.keymap.set('n', 'zR', function() vscode.action('editor.unfoldAll') end)
    -- zM: 모든 블록 접기
    vim.keymap.set('n', 'zM', function() vscode.action('editor.foldAll') end)
    
    -- 추가로 유용한 폴딩 커맨드
    -- zC: 현재 위치에서 재귀적으로 모든 하위 블록 접기
    vim.keymap.set('n', 'zC', function() vscode.action('editor.foldRecursively') end)
    -- zO: 현재 위치에서 재귀적으로 모든 하위 블록 펼치기
    vim.keymap.set('n', 'zO', function() vscode.action('editor.unfoldRecursively') end)

    -- Visual Mode에서 c와 s가 클립보드를 사용하지 않도록 설정 (블랙홀 레지스터 사용)
    -- 이렇게 하면 삭제 시 시스템 클립보드를 건드리지 않아 딜레이가 사라집니다.
    vim.keymap.set('v', 'c', '"_c')
    vim.keymap.set('v', 's', '"_c')

    -- (참고) 한 글자 삭제인 x도 클립보드에 영향을 주지 않게 하려면:
    vim.keymap.set('n', 'x', '"_x') -- Visual mode에서 s는 사실상 c와 동작이 같습니다.

end

if vim.g.vscode then
    -- 1. 전역 변수로 마지막 입력기 상태 저장 (기본값은 영문)
    -- macOS: "com.apple.keylayout.ABC" / Windows: "1033"
    local last_im = "com.apple.keylayout.ABC" 
    local english_im = "com.apple.keylayout.ABC" -- 본인의 영문 IM 코드

    -- 2. 현재 입력기 상태를 가져오는 함수
    local function get_im()
        local handle = io.popen("im-select") -- im-select 실행 경로 확인 필요
        local result = handle:read("*a")
        handle:close()
        return result:gsub("%s+", "") -- 공백 제거
    end

    -- 3. 입력기를 변경하는 함수
    local function set_im(im_id)
        os.execute("im-select " .. im_id)
    end

    -- [핵심] Ctrl+C를 Esc처럼 동작하게 매핑 (InsertLeave 이벤트를 확실히 발생시킴)
    vim.keymap.set('i', '<C-c>', '<Esc>')

    -- 4. Insert 모드를 나갈 때 (InsertLeave)
    vim.api.nvim_create_autocmd("InsertLeave", {
        pattern = "*",
        callback = function()
            last_im = get_im() -- 현재 상태(한글인지 영문인지) 저장
            if last_im ~= english_im then
                set_im(english_im) -- 영문으로 전환
            end
        end,
    })

    -- 5. Insert 모드로 들어갈 때 (InsertEnter)
    vim.api.nvim_create_autocmd("InsertEnter", {
        pattern = "*",
        callback = function()
            if last_im ~= get_im() then
                set_im(last_im) -- 저장했던 상태로 복구
            end
        end,
    })
end

-- ~/.config/nvim/init.lua (또는 Windows의 경우 ~/AppData/Local/nvim/init.lua)
vim.api.nvim_create_autocmd("FileType", {
  -- 적용하고 싶은 언어들을 추가하세요 (c, cpp, rust, javascript, typescript 등)
  pattern = { "javascript", "typescript", "javascriptreact", "typescriptreact", "c", "cpp", "rust", "go" },
  callback = function()
    -- comments 옵션에서 :// (일반 주석)를 제거합니다.
    -- 이렇게 하면 // 뒤에서 o를 눌러도 다음 줄에 //가 생기지 않습니다.
    -- 하지만 :/// (문서 주석)은 리스트에 남아있으므로 계속 작동합니다.
    vim.opt_local.comments:remove("://")
  end,
})

-- 시스템 클립보드와 Neovim 클립보드 동기화
vim.opt.clipboard = "unnamedplus"
-- 대소문자를 무시하도록 설정
vim.opt.ignorecase = true
-- 검색어에 대문자가 포함되어 있으면 대소문자를 구분하도록 설정
vim.opt.smartcase = true
