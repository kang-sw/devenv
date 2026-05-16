// Shared HTTP error-detail extraction for daemon JSON APIs.
//
// Daemon error responses carry a `{ "error": "<detail>" }` body; this returns
// that detail when present and falls back to a bounded HTTP status string for
// non-string bodies or non-JSON responses. Keep this the single error-parsing
// helper so per-API fetch wrappers do not drift apart.
export async function apiErrorDetail(response: Response): Promise<string> {
  try {
    const value = (await response.json()) as { error?: unknown };
    if (typeof value.error === "string" && value.error.trim()) {
      return value.error;
    }
  } catch {
    // Fall through to bounded HTTP status text.
  }

  return `HTTP ${response.status}`;
}
