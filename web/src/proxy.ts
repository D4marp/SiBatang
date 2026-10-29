import { NextResponse, type NextRequest } from "next/server";

/**
 * Hanya UX: arahkan pengunjung tanpa cookie sesi ke halaman masuk.
 * Otorisasi sebenarnya SELALU di Go (membership + scope + RLS); proxy ini tidak memberi izin apa pun.
 */
export function proxy(request: NextRequest) {
  if (!request.cookies.has("sib_session")) {
    return NextResponse.redirect(new URL("/masuk", request.url));
  }
  return NextResponse.next();
}

export const config = { matcher: ["/polda/:path*"] };
