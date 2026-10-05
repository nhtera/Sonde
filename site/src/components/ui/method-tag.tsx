// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

export type Method = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

const CLASS: Record<Method, string> = { GET: "m-get", POST: "m-post", PUT: "m-put", PATCH: "m-patch", DELETE: "m-del" };

/** An HTTP method as colored mono text, never filled. DELETE shows as DEL, as in the app. */
export function MethodTag({ method }: { method: Method }) {
  return <span className={`mono ${CLASS[method]}`}>{method === "DELETE" ? "DEL" : method}</span>;
}
