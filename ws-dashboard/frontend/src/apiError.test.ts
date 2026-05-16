import { apiErrorDetail } from "./apiError.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(`${label}: expected ${String(expected)}, got ${String(actual)}`);
  }
}

// Branch 1: a JSON body with a non-empty `error` string surfaces that detail.
assertEqual(
  await apiErrorDetail(
    new Response(JSON.stringify({ error: "workRoot was not discovered" }), { status: 400 }),
  ),
  "workRoot was not discovered",
  "string error body surfaces the daemon detail",
);

// Branch 2: a JSON body without a usable string `error` falls back to status.
assertEqual(
  await apiErrorDetail(new Response(JSON.stringify({ error: 42 }), { status: 500 })),
  "HTTP 500",
  "non-string error field falls back to HTTP status",
);
assertEqual(
  await apiErrorDetail(new Response(JSON.stringify({ detail: "nope" }), { status: 404 })),
  "HTTP 404",
  "missing error field falls back to HTTP status",
);
assertEqual(
  await apiErrorDetail(new Response(JSON.stringify({ error: "   " }), { status: 403 })),
  "HTTP 403",
  "blank error string falls back to HTTP status",
);

// Branch 3: a non-JSON body is caught and falls back to the HTTP status.
assertEqual(
  await apiErrorDetail(new Response("<html>bad gateway</html>", { status: 502 })),
  "HTTP 502",
  "non-JSON body falls back to HTTP status",
);
