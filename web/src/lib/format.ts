const WIB = "Asia/Jakarta";

/** Waktu ditampilkan dalam WIB; server menyimpan UTC. */
export function formatWib(iso: string, withDate = false): string {
  return new Intl.DateTimeFormat("id-ID", {
    timeZone: WIB,
    hour: "2-digit",
    minute: "2-digit",
    ...(withDate ? { day: "2-digit", month: "short", year: "numeric" } : {}),
  }).format(new Date(iso));
}

/** Tanggal layanan lokal (YYYY-MM-DD) di WIB, bukan UTC. */
export function todayWib(now: Date = new Date()): string {
  const p = new Intl.DateTimeFormat("en-CA", { timeZone: WIB, year: "numeric", month: "2-digit", day: "2-digit" }).format(now);
  return p;
}

/** Token QR dibawa di URL fragment (#t=...) agar tidak dikirim ke server/log. */
export function tokenFromHash(hash: string): string | null {
  const m = /(?:^#|&)t=([A-Za-z0-9_-]{16,128})(?:&|$)/.exec(hash);
  return m?.[1] ?? null;
}

export function bookingUrl(reference: string, token: string): string {
  return `/booking/${encodeURIComponent(reference)}#t=${token}`;
}

export function secondsAgo(iso: string, now: number = Date.now()): number {
  return Math.max(0, Math.round((now - new Date(iso).getTime()) / 1000));
}
