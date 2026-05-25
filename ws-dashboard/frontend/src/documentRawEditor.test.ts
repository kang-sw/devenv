import { documentEditorLanguageId } from "./documentRawEditor.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(`${label}: expected ${String(expected)}, got ${String(actual)}`);
  }
}

const cases: Array<[
  Parameters<typeof documentEditorLanguageId>[0],
  ReturnType<typeof documentEditorLanguageId>,
  string,
]> = [
  [{ extension: "md", languageHint: null, path: "README.md" }, "markdown", "markdown extension selects markdown language"],
  [{ extension: "tsx", languageHint: null, path: "src/App.tsx" }, "typescript", "tsx extension selects TypeScript language"],
  [{ extension: null, languageHint: "javascript", path: "script" }, "javascript", "language hint can select JavaScript without extension"],
  [{ extension: "json", languageHint: null, path: "package.json" }, "json", "json extension selects JSON language"],
  [{ extension: "css", languageHint: null, path: "style.css" }, "css", "css extension selects CSS language"],
  [{ extension: "html", languageHint: null, path: "index.html" }, "html", "html extension selects HTML language"],
  [{ extension: "yaml", languageHint: null, path: "config.yaml" }, "yaml", "yaml extension selects YAML language"],
  [{ extension: "py", languageHint: null, path: "tools/run.py" }, "python", "py extension selects Python language"],
  [{ extension: "rs", languageHint: null, path: "src/main.rs" }, "rust", "rs extension selects Rust language"],
  [{ extension: "sh", languageHint: null, path: "scripts/run.sh" }, "shell", "sh extension selects shell language"],
  [{ extension: "toml", languageHint: null, path: "Cargo.toml" }, "toml", "toml extension selects TOML language"],
  [{ extension: "xml", languageHint: null, path: "feed.xml" }, "xml", "xml extension selects XML language"],
  [{ extension: "sql", languageHint: null, path: "query.sql" }, "sql", "sql extension selects SQL language"],
  [{ extension: "patch", languageHint: null, path: "fix.patch" }, "diff", "patch extension selects diff language"],
  [{ extension: "ini", languageHint: null, path: "settings.ini" }, "properties", "ini extension selects properties language"],
  [{ extension: null, languageHint: null, path: "Dockerfile" }, "dockerfile", "Dockerfile basename selects dockerfile language"],
  [{ extension: null, languageHint: null, path: "Makefile" }, "makefile", "Makefile basename selects makefile language"],
  [{ extension: "go", languageHint: null, path: "main.go" }, "go", "go extension selects Go language"],
  [{ extension: "java", languageHint: null, path: "Main.java" }, "java", "java extension selects Java language"],
  [{ extension: "c", languageHint: null, path: "main.c" }, "c", "c extension selects C language"],
  [{ extension: "cpp", languageHint: null, path: "main.cpp" }, "cpp", "cpp extension selects C++ language"],
  [{ extension: "php", languageHint: null, path: "index.php" }, "php", "php extension selects PHP language"],
  [{ extension: "rb", languageHint: null, path: "tool.rb" }, "ruby", "rb extension selects Ruby language"],
  [{ extension: "lua", languageHint: null, path: "init.lua" }, "lua", "lua extension selects Lua language"],
  [{ extension: "nix", languageHint: null, path: "flake.nix" }, "nix", "nix extension gets a stable language id"],
  [{ extension: "typ", languageHint: null, path: "doc.typ" }, "typst", "typ extension gets a stable Typst language id"],
  [{ extension: "mmd", languageHint: null, path: "diagram.mmd" }, "mermaid", "mmd extension gets a stable Mermaid language id"],
  [{ extension: null, languageHint: null, path: "justfile" }, "justfile", "justfile basename gets a stable language id"],
  [{ extension: null, languageHint: null, path: ".gitignore" }, "gitignore", "gitignore basename gets a stable language id"],
  [{ extension: null, languageHint: null, path: ".env.local" }, "env", "env basename selects env language"],
  [{ extension: null, languageHint: null, path: "notes.unknown" }, "text", "unknown extension falls back to text"],
  [{ extension: "txt", languageHint: "TypeScript", path: "generated.txt" }, "typescript", "language hint takes precedence over plain extension"],
];

for (const [source, expected, label] of cases) {
  assertEqual(documentEditorLanguageId(source), expected, label);
}
