/**
 * Pi catches renderer errors and uses its standard text/image fallback for
 * that slot. This marker intentionally keeps unsupported output on that path.
 *
 * A host-import-free leaf so both `tool-result-render.ts` (which re-exports
 * it) and `display-summary-render.ts` (which `tool-result-render.ts` imports)
 * can throw it without an import cycle under Pi's jiti loader.
 */
export class UseNativeResultFallback extends Error {}
