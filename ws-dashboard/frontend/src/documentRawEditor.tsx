import { useEffect, useMemo, useRef } from "react";
import { history, historyKeymap, defaultKeymap, indentWithTab } from "@codemirror/commands";
import { bracketMatching, defaultHighlightStyle, indentOnInput, syntaxHighlighting, foldGutter, StreamLanguage } from "@codemirror/language";
import { searchKeymap, highlightSelectionMatches } from "@codemirror/search";
import { EditorState, Compartment, type Extension } from "@codemirror/state";
import { EditorView, drawSelection, dropCursor, highlightActiveLine, highlightActiveLineGutter, highlightSpecialChars, keymap, lineNumbers, rectangularSelection, crosshairCursor } from "@codemirror/view";

export type DocumentEditorLanguageId =
  | "markdown"
  | "typescript"
  | "javascript"
  | "json"
  | "css"
  | "html"
  | "yaml"
  | "python"
  | "rust"
  | "shell"
  | "toml"
  | "xml"
  | "sql"
  | "diff"
  | "properties"
  | "dockerfile"
  | "makefile"
  | "go"
  | "java"
  | "c"
  | "cpp"
  | "php"
  | "ruby"
  | "lua"
  | "nix"
  | "typst"
  | "mermaid"
  | "justfile"
  | "gitignore"
  | "env"
  | "text";

export type DocumentEditorSource = {
  extension: string | null;
  languageHint: string | null;
  path?: string;
};

export function documentEditorLanguageId(source: DocumentEditorSource): DocumentEditorLanguageId {
  const fileName = fileNameFromPath(source.path);
  const lowerFileName = fileName?.toLowerCase() ?? "";
  const extension = normalizeLanguageToken(source.extension);
  const languageHint = normalizeLanguageToken(source.languageHint);
  const pathExtension = normalizeLanguageToken(extensionFromPath(source.path));
  const tokens = [languageHint, extension, pathExtension].filter((token): token is string => Boolean(token));

  if (lowerFileName === "dockerfile" || lowerFileName.endsWith(".dockerfile")) {
    return "dockerfile";
  }
  if (lowerFileName === "makefile" || lowerFileName.endsWith(".mk")) {
    return "makefile";
  }
  if (lowerFileName === "justfile" || lowerFileName.endsWith(".just")) {
    return "justfile";
  }
  if (lowerFileName === ".gitignore" || lowerFileName.endsWith(".gitignore")) {
    return "gitignore";
  }
  if (lowerFileName === ".env" || lowerFileName.startsWith(".env.")) {
    return "env";
  }

  for (const token of tokens) {
    switch (token) {
      case "md":
      case "markdown":
      case "mdown":
      case "mkd":
        return "markdown";
      case "ts":
      case "tsx":
      case "typescript":
        return "typescript";
      case "js":
      case "jsx":
      case "mjs":
      case "cjs":
      case "javascript":
        return "javascript";
      case "json":
      case "jsonc":
        return "json";
      case "css":
        return "css";
      case "html":
      case "htm":
        return "html";
      case "yaml":
      case "yml":
        return "yaml";
      case "py":
      case "python":
        return "python";
      case "rs":
      case "rust":
        return "rust";
      case "sh":
      case "bash":
      case "zsh":
      case "shell":
        return "shell";
      case "toml":
        return "toml";
      case "xml":
      case "svg":
      case "rss":
      case "atom":
        return "xml";
      case "sql":
        return "sql";
      case "diff":
      case "patch":
        return "diff";
      case "ini":
      case "properties":
      case "conf":
      case "cfg":
        return "properties";
      case "env":
      case "dotenv":
        return "env";
      case "dockerfile":
        return "dockerfile";
      case "makefile":
      case "mk":
        return "makefile";
      case "go":
      case "golang":
        return "go";
      case "java":
        return "java";
      case "c":
      case "h":
        return "c";
      case "cc":
      case "cpp":
      case "cxx":
      case "hpp":
      case "hh":
      case "hxx":
        return "cpp";
      case "php":
      case "phtml":
        return "php";
      case "rb":
      case "ruby":
        return "ruby";
      case "lua":
        return "lua";
      case "nix":
        return "nix";
      case "typ":
      case "typst":
        return "typst";
      case "mmd":
      case "mermaid":
        return "mermaid";
      case "just":
      case "justfile":
        return "justfile";
      case "gitignore":
        return "gitignore";
      default:
        break;
    }
  }

  return "text";
}

export async function loadDocumentEditorLanguageExtension(languageId: DocumentEditorLanguageId): Promise<Extension> {
  switch (languageId) {
    case "markdown": {
      const { markdown } = await import("@codemirror/lang-markdown");
      return markdown();
    }
    case "typescript": {
      const { javascript } = await import("@codemirror/lang-javascript");
      return javascript({ typescript: true, jsx: true });
    }
    case "javascript": {
      const { javascript } = await import("@codemirror/lang-javascript");
      return javascript({ jsx: true });
    }
    case "json": {
      const { json } = await import("@codemirror/lang-json");
      return json();
    }
    case "css": {
      const { css } = await import("@codemirror/lang-css");
      return css();
    }
    case "html": {
      const { html } = await import("@codemirror/lang-html");
      return html();
    }
    case "yaml": {
      const { yaml } = await import("@codemirror/lang-yaml");
      return yaml();
    }
    case "python": {
      const { python } = await import("@codemirror/lang-python");
      return python();
    }
    case "rust": {
      const { rust } = await import("@codemirror/lang-rust");
      return rust();
    }
    case "shell": {
      const { shell } = await import("@codemirror/legacy-modes/mode/shell");
      return StreamLanguage.define(shell);
    }
    case "toml": {
      const { toml } = await import("@codemirror/legacy-modes/mode/toml");
      return StreamLanguage.define(toml);
    }
    case "xml": {
      const { xml } = await import("@codemirror/lang-xml");
      return xml();
    }
    case "sql": {
      const { sql } = await import("@codemirror/lang-sql");
      return sql();
    }
    case "diff": {
      const { diff } = await import("@codemirror/legacy-modes/mode/diff");
      return StreamLanguage.define(diff);
    }
    case "properties":
    case "env": {
      const { properties } = await import("@codemirror/legacy-modes/mode/properties");
      return StreamLanguage.define(properties);
    }
    case "dockerfile": {
      const { dockerFile } = await import("@codemirror/legacy-modes/mode/dockerfile");
      return StreamLanguage.define(dockerFile);
    }
    case "go": {
      const { go } = await import("@codemirror/lang-go");
      return go();
    }
    case "java": {
      const { java } = await import("@codemirror/lang-java");
      return java();
    }
    case "c":
    case "cpp": {
      const { cpp } = await import("@codemirror/lang-cpp");
      return cpp();
    }
    case "php": {
      const { php } = await import("@codemirror/lang-php");
      return php();
    }
    case "ruby": {
      const { ruby } = await import("@codemirror/legacy-modes/mode/ruby");
      return StreamLanguage.define(ruby);
    }
    case "lua": {
      const { lua } = await import("@codemirror/legacy-modes/mode/lua");
      return StreamLanguage.define(lua);
    }
    case "makefile":
    case "nix":
    case "typst":
    case "mermaid":
    case "justfile":
    case "gitignore":
    case "text":
      return [];
  }
}

const languageCompartment = new Compartment();
const editabilityCompartment = new Compartment();

const dashboardCodeMirrorTheme = EditorView.theme(
  {
    "&": {
      height: "100%",
      minHeight: "0",
      backgroundColor: "var(--ws-color-surface-editor-body)",
      color: "var(--ws-color-text-primary)",
      fontFamily: "var(--ws-font-family-mono)",
      fontSize: "var(--ws-font-size-02)",
      lineHeight: "var(--ws-line-height-04)",
    },
    ".cm-scroller": {
      overflow: "auto",
      fontFamily: "var(--ws-font-family-mono)",
    },
    ".cm-content": {
      minHeight: "100%",
      padding: "var(--ws-space-09) 0 var(--ws-space-09) var(--ws-space-06)",
      caretColor: "var(--ws-color-text-primary)",
    },
    ".cm-line": {
      padding: "0 var(--ws-space-09) 0 var(--ws-space-04)",
    },
    ".cm-gutters": {
      borderRight: "var(--ws-border-width-hairline) solid var(--ws-color-divider-local)",
      backgroundColor: "var(--ws-color-surface-editor-chrome)",
      color: "var(--ws-color-text-disabled)",
    },
    ".cm-activeLineGutter": {
      backgroundColor: "var(--ws-color-panel-selected)",
      color: "var(--ws-color-text-secondary)",
    },
    ".cm-activeLine": {
      backgroundColor: "color-mix(in srgb, var(--ws-color-action) 9%, transparent)",
    },
    ".cm-selectionBackground, &.cm-focused .cm-selectionBackground": {
      backgroundColor: "color-mix(in srgb, var(--ws-color-action) 38%, transparent)",
    },
    ".cm-searchMatch": {
      backgroundColor: "color-mix(in srgb, var(--ws-color-state-warning) 28%, transparent)",
      outline: "var(--ws-border-width-hairline) solid color-mix(in srgb, var(--ws-color-state-warning) 60%, transparent)",
    },
    ".cm-matchingBracket, .cm-nonmatchingBracket": {
      backgroundColor: "color-mix(in srgb, var(--ws-color-action) 22%, transparent)",
      outline: "var(--ws-border-width-hairline) solid var(--ws-color-action)",
    },
    "&.cm-focused": {
      outline: "none",
    },
  },
  { dark: true },
);

const baseEditorExtensions: Extension[] = [
  lineNumbers(),
  highlightActiveLineGutter(),
  highlightSpecialChars(),
  history(),
  foldGutter(),
  drawSelection(),
  dropCursor(),
  indentOnInput(),
  bracketMatching(),
  rectangularSelection(),
  crosshairCursor(),
  highlightActiveLine(),
  highlightSelectionMatches(),
  syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
  keymap.of([indentWithTab, ...defaultKeymap, ...historyKeymap, ...searchKeymap]),
  dashboardCodeMirrorTheme,
  EditorView.lineWrapping,
];

export function DocumentRawEditor({
  value,
  source,
  ariaLabel,
  onChange,
  editable = true,
}: {
  value: string;
  source: DocumentEditorSource;
  ariaLabel: string;
  onChange?: (value: string) => void;
  editable?: boolean;
}) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const viewRef = useRef<EditorView | null>(null);
  const onChangeRef = useRef(onChange);
  const editableRef = useRef(editable);
  const syncingPropValueRef = useRef(false);
  const languageId = useMemo(() => documentEditorLanguageId(source), [source.extension, source.languageHint, source.path]);

  useEffect(() => {
    onChangeRef.current = onChange;
  }, [onChange]);

  useEffect(() => {
    editableRef.current = editable;
  }, [editable]);

  useEffect(() => {
    if (!containerRef.current || viewRef.current) {
      return;
    }

    const view = new EditorView({
      parent: containerRef.current,
      state: EditorState.create({
        doc: value,
        extensions: [
          ...baseEditorExtensions,
          languageCompartment.of([]),
          editabilityCompartment.of(editabilityExtensions(editable)),
          EditorView.updateListener.of((update) => {
            if (update.docChanged && editableRef.current && !syncingPropValueRef.current) {
              onChangeRef.current?.(update.state.doc.toString());
            }
          }),
          EditorView.contentAttributes.of({
            "aria-label": ariaLabel,
            spellcheck: "false",
          }),
        ],
      }),
    });
    viewRef.current = view;

    return () => {
      view.destroy();
      viewRef.current = null;
    };
  }, []);


  useEffect(() => {
    const view = viewRef.current;
    if (!view) {
      return;
    }
    view.dispatch({ effects: editabilityCompartment.reconfigure(editabilityExtensions(editable)) });
  }, [editable]);

  useEffect(() => {
    const view = viewRef.current;
    if (!view) {
      return;
    }
    view.contentDOM.setAttribute("aria-label", ariaLabel);
    view.contentDOM.setAttribute("spellcheck", "false");
  }, [ariaLabel]);

  useEffect(() => {
    const view = viewRef.current;
    if (!view) {
      return;
    }
    let cancelled = false;
    void loadDocumentEditorLanguageExtension(languageId).then((extension) => {
      if (!cancelled && viewRef.current === view) {
        view.dispatch({ effects: languageCompartment.reconfigure(extension) });
      }
    });
    return () => {
      cancelled = true;
    };
  }, [languageId]);

  useEffect(() => {
    const view = viewRef.current;
    if (!view) {
      return;
    }
    const current = view.state.doc.toString();
    if (current === value) {
      return;
    }
    syncingPropValueRef.current = true;
    view.dispatch({
      changes: { from: 0, to: current.length, insert: value },
    });
    syncingPropValueRef.current = false;
  }, [value]);

  return (
    <div
      ref={containerRef}
      className={`document-raw-editor document-codemirror-editor ws-code-block${editable ? "" : " document-source-viewer"}`}
      data-editor-language={languageId}
      data-editor-read-only={editable ? "false" : "true"}
    />
  );
}

function editabilityExtensions(editable: boolean): Extension {
  return [EditorState.readOnly.of(!editable), EditorView.editable.of(true)];
}

function normalizeLanguageToken(value: string | null | undefined) {
  return value?.toLowerCase().replace(/^\./, "").trim() || null;
}

function extensionFromPath(path: string | null | undefined) {
  if (!path) {
    return null;
  }
  const fileName = path.split(/[\\/]/).pop() ?? path;
  if (["makefile", "dockerfile"].includes(fileName.toLowerCase())) {
    return null;
  }
  const index = fileName.lastIndexOf(".");
  return index >= 0 ? fileName.slice(index + 1) : null;
}

function fileNameFromPath(path: string | null | undefined) {
  return path ? path.split(/[\\/]/).pop() ?? path : null;
}
