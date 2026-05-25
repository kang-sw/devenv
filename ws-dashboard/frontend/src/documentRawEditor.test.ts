import { documentEditorLanguageId } from "./documentRawEditor.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(`${label}: expected ${String(expected)}, got ${String(actual)}`);
  }
}

assertEqual(
  documentEditorLanguageId({ extension: "md", languageHint: null, path: "README.md" }),
  "markdown",
  "markdown extension selects markdown language",
);
assertEqual(
  documentEditorLanguageId({ extension: "tsx", languageHint: null, path: "src/App.tsx" }),
  "typescript",
  "tsx extension selects TypeScript language",
);
assertEqual(
  documentEditorLanguageId({ extension: null, languageHint: "javascript", path: "script" }),
  "javascript",
  "language hint can select JavaScript without extension",
);
assertEqual(
  documentEditorLanguageId({ extension: "json", languageHint: null, path: "package.json" }),
  "json",
  "json extension selects JSON language",
);
assertEqual(
  documentEditorLanguageId({ extension: "css", languageHint: null, path: "style.css" }),
  "css",
  "css extension selects CSS language",
);
assertEqual(
  documentEditorLanguageId({ extension: "html", languageHint: null, path: "index.html" }),
  "html",
  "html extension selects HTML language",
);
assertEqual(
  documentEditorLanguageId({ extension: "yaml", languageHint: null, path: "config.yaml" }),
  "yaml",
  "yaml extension selects YAML language",
);
assertEqual(
  documentEditorLanguageId({ extension: "py", languageHint: null, path: "tools/run.py" }),
  "python",
  "py extension selects Python language",
);
assertEqual(
  documentEditorLanguageId({ extension: "rs", languageHint: null, path: "src/main.rs" }),
  "rust",
  "rs extension selects Rust language",
);
assertEqual(
  documentEditorLanguageId({ extension: "sh", languageHint: null, path: "scripts/run.sh" }),
  "shell",
  "sh extension selects shell language",
);
assertEqual(
  documentEditorLanguageId({ extension: null, languageHint: null, path: "notes.unknown" }),
  "text",
  "unknown extension falls back to text",
);
assertEqual(
  documentEditorLanguageId({ extension: "txt", languageHint: "TypeScript", path: "generated.txt" }),
  "typescript",
  "language hint takes precedence over plain extension",
);
