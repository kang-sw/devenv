import {
  applyReadOnlyFilePaneContent,
  applyReadOnlyFilePaneError,
  createLoadingReadOnlyFilePane,
  fetchWorkRootFiles,
  fetchWorkRootTextFile,
  flattenWorkRootFileTree,
  toggleExpandedPath,
  workRootExplorerInitialLoadPath,
  workRootExplorerRefreshPaths,
  workRootExplorerShouldLoadOnExpand,
  workRootFileReadEndpoint,
  workRootFilesEndpoint,
  readOnlyFilePaneId,
  readOnlyFilePaneLogicalKey,
  type DirectoryLoadState,
} from "./workRootFiles.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(`${label}: expected ${String(expected)}, got ${String(actual)}`);
  }
}

async function assertRejects(action: () => Promise<unknown>, pattern: RegExp, label: string) {
  try {
    await action();
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    if (!pattern.test(message)) {
      throw new Error(`${label}: error ${message} did not match ${pattern}`);
    }
    return;
  }

  throw new Error(`${label}: expected rejection`);
}

function assertDeepEqual(actual: unknown, expected: unknown, label: string) {
  const actualJson = JSON.stringify(actual);
  const expectedJson = JSON.stringify(expected);
  if (actualJson !== expectedJson) {
    throw new Error(`${label}: expected ${expectedJson}, got ${actualJson}`);
  }
}

assertEqual(
  workRootFilesEndpoint("root-local-abc", ""),
  "/api/dashboard/work-roots/root-local-abc/files",
  "root listing omits path query",
);
assertEqual(
  workRootFilesEndpoint("root/local abc", "src/main file.ts"),
  "/api/dashboard/work-roots/root%2Flocal%20abc/files?path=src%2Fmain+file.ts",
  "endpoint encodes opaque workRootId and relative path only",
);
assertEqual(
  workRootFileReadEndpoint("root-local-abc", "src/main.rs"),
  "/api/dashboard/work-roots/root-local-abc/files/read?path=src%2Fmain.rs",
  "read endpoint encodes relative path query",
);
assertEqual(
  workRootFileReadEndpoint("root/local abc", "docs/read me.md"),
  "/api/dashboard/work-roots/root%2Flocal%20abc/files/read?path=docs%2Fread+me.md",
  "read endpoint encodes opaque workRootId and spaced relative path",
);
assertEqual(
  readOnlyFilePaneLogicalKey("root-local-abc", "src/main.rs"),
  "editor/root-local-abc/src/main.rs",
  "read-only logical key is scoped by workRootId and relative path",
);
assertEqual(
  readOnlyFilePaneLogicalKey("root-local-abc", "src/main.rs") ===
    readOnlyFilePaneLogicalKey("root-local-abc", "src/main.rs"),
  true,
  "same file produces stable read-only logical key",
);
assertEqual(
  readOnlyFilePaneLogicalKey("root-local-abc", "src/main.rs") ===
    readOnlyFilePaneLogicalKey("root-local-abc", "src/lib.rs"),
  false,
  "different relative paths produce different logical keys",
);
assertEqual(
  readOnlyFilePaneId("root/local abc", "docs/read me.md"),
  "readonly:root%2Flocal%20abc:docs%2Fread%20me.md",
  "pane id encodes scoped file identity without host paths",
);


const expanded = toggleExpandedPath(new Set([""]), "src");
assertEqual(expanded.has(""), true, "toggle preserves existing expanded root");
assertEqual(expanded.has("src"), true, "toggle expands a collapsed path");
assertEqual(toggleExpandedPath(expanded, "src").has("src"), false, "toggle collapses an expanded path");

const loaded = (entries: DirectoryLoadState["entries"]): DirectoryLoadState => ({
  status: "loaded",
  entries,
  error: null,
});

const rows = flattenWorkRootFileTree({
  expandedPaths: new Set(["", "src"]),
  selectedPath: "README.md",
  directories: {
    "": loaded([
      {
        name: "src",
        path: "src",
        kind: "directory",
        status: "ok",
        readable: true,
        previewEligible: false,
      },
      {
        name: "README.md",
        path: "README.md",
        kind: "file",
        status: "ok",
        readable: true,
        previewEligible: true,
      },
    ]),
    src: loaded([
      {
        name: "main.ts",
        path: "src/main.ts",
        kind: "file",
        status: "ok",
        readable: true,
        previewEligible: true,
      },
    ]),
  },
});

assertDeepEqual(
  rows.map((row) => (row.type === "entry" ? `${row.depth}:${row.entry.path}` : `${row.depth}:${row.status}`)),
  ["0:src", "1:src/main.ts", "0:README.md"],
  "flattened tree includes expanded child rows in order",
);
assertEqual(rows[2].type === "entry" && rows[2].selected, true, "flattened tree marks selected row");

const emptyRows = flattenWorkRootFileTree({
  expandedPaths: new Set([""]),
  selectedPath: null,
  directories: { "": loaded([]) },
});
assertEqual(emptyRows[0]?.type === "state" && emptyRows[0].status, "empty", "empty root surfaces state row");

const errorFetch = globalThis.fetch;
try {
  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ error: "unknown workRoot" }), {
      status: 404,
      headers: { "content-type": "application/json" },
    })) as typeof fetch;
  await assertRejects(
    () => fetchWorkRootFiles("root-local-missing", ""),
    /unknown workRoot/,
    "fetch surfaces bounded backend JSON errors",
  );

  globalThis.fetch = (async () => new Response("not json", { status: 418 })) as typeof fetch;
  await assertRejects(
    () => fetchWorkRootFiles("root-local-teapot", "src"),
    /HTTP 418/,
    "fetch falls back to bounded HTTP status when error JSON is unavailable",
  );

  globalThis.fetch = (async () =>
    new Response(
      JSON.stringify({
        workRootId: "root-local-abc",
        path: "README.md",
        name: "README.md",
        status: "ok",
        readOnly: true,
        content: "hello\n",
        sizeBytes: 6,
        languageHint: "markdown",
        extension: "md",
      }),
      { status: 200, headers: { "content-type": "application/json" } },
    )) as typeof fetch;
  const textFile = await fetchWorkRootTextFile("root-local-abc", "README.md");
  assertEqual(textFile.content, "hello\n", "read helper returns text content");

  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ error: "unsupported text file" }), {
      status: 400,
      headers: { "content-type": "application/json" },
    })) as typeof fetch;
  await assertRejects(
    () => fetchWorkRootTextFile("root-local-abc", "binary.bin"),
    /unsupported text file/,
    "read helper surfaces bounded backend JSON errors",
  );

  globalThis.fetch = (async () => new Response("not json", { status: 413 })) as typeof fetch;
  await assertRejects(
    () => fetchWorkRootTextFile("root-local-abc", "large.txt"),
    /HTTP 413/,
    "read helper falls back to bounded HTTP status when error JSON is unavailable",
  );
} finally {
  globalThis.fetch = errorFetch;
}

assertEqual(
  workRootExplorerInitialLoadPath(undefined),
  "",
  "missing selected workRoot snapshot requests initial root listing",
);
assertEqual(
  workRootExplorerInitialLoadPath({
    directories: { "": { status: "loading", entries: [], error: null } },
  }),
  null,
  "loading root listing is not requested again",
);
assertEqual(
  workRootExplorerShouldLoadOnExpand(undefined, "src", false),
  true,
  "expanding an unloaded directory requests that relative path",
);
assertEqual(
  workRootExplorerShouldLoadOnExpand({ directories: { src: loaded([]) } }, "src", false),
  false,
  "expanding a loaded directory does not request it again",
);
assertEqual(
  workRootExplorerShouldLoadOnExpand(undefined, "src", true),
  false,
  "collapsing a directory does not request it",
);
assertDeepEqual(
  workRootExplorerRefreshPaths(new Set(["", "src"])),
  ["", "src"],
  "refresh reloads expanded root and child directories",
);
assertDeepEqual(
  workRootExplorerRefreshPaths(new Set()),
  [""],
  "refresh falls back to root when expansion state is empty",
);

const errorRows = flattenWorkRootFileTree({
  expandedPaths: new Set([""]),
  selectedPath: null,
  directories: { "": { status: "error", entries: [], error: "unknown workRoot" } },
});
assertEqual(
  errorRows[0]?.type === "state" && errorRows[0].status,
  "error",
  "error snapshot renders an error state row",
);


const loadingPane = createLoadingReadOnlyFilePane("root-local-abc", "src/main.rs");
assertEqual(loadingPane.status, "loading", "new read-only pane starts loading");
assertEqual(loadingPane.title, "main.rs", "new read-only pane derives basename title");
const loadedPane = applyReadOnlyFilePaneContent(loadingPane, {
  workRootId: "root-local-abc",
  path: "src/main.rs",
  name: "main.rs",
  status: "ok",
  readOnly: true,
  content: "fn main() {}\n",
  sizeBytes: 13,
  languageHint: "rust",
  extension: "rs",
});
assertEqual(loadedPane.status, "loaded", "pane content marks pane loaded");
assertEqual(loadedPane.content, "fn main() {}\n", "pane content is preserved");
assertEqual(
  applyReadOnlyFilePaneError(loadingPane, "unsupported text file").error,
  "unsupported text file",
  "pane error remains bounded",
);
