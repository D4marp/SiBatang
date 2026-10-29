import "server-only";
import { cookies } from "next/headers";
import type { components } from "@/generated/api";

export type Problem = components["schemas"]["Problem"];
export type Catalog = components["schemas"]["Catalog"];
export type PoldaMetrics = NonNullable<components["schemas"]["PoldaMetricsEnvelope"]["data"]>;
export type PolresListItem = components["schemas"]["PolresListItem"];

const API_ORIGIN = process.env.API_ORIGIN ?? "http://localhost:8080";

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
  }
}

/** Fetch dari Server Component ke API Go. Sesi staf diteruskan sebagai cookie; Go yang memutuskan izin. */
export async function apiGet<T>(path: string, opts: { auth?: boolean } = {}): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (opts.auth) {
    const jar = await cookies();
    const session = jar.get("sib_session");
    if (!session) throw new ApiError(401, "AUTH_REQUIRED", "not signed in");
    headers.Cookie = `sib_session=${session.value}`;
  }
  let res: Response;
  try {
    res = await fetch(`${API_ORIGIN}/api/v1${path}`, { headers, cache: "no-store" });
  } catch {
    throw new ApiError(503, "TEMPORARILY_UNAVAILABLE", "API unreachable");
  }
  if (!res.ok) {
    const p = (await res.json().catch(() => null)) as Problem | null;
    throw new ApiError(res.status, p?.code ?? "UNKNOWN", p?.detail ?? res.statusText);
  }
  const body = (await res.json()) as { data: T };
  return body.data;
}
