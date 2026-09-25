/** A readable line from a thrown Error or an RFC 9457 problem body. */
export function problemDetail(error: unknown): string {
  if (error instanceof Error) return error.message;
  const problem = (typeof error === "object" && error !== null ? error : {}) as { detail?: unknown; title?: unknown };
  return String(problem.detail ?? problem.title ?? error);
}
