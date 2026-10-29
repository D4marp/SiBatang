import { describe, expect, it } from "vitest";
import { bookingUrl, formatWib, todayWib, tokenFromHash } from "./format";

describe("format", () => {
  it("menghitung tanggal layanan di WIB, bukan UTC", () => {
    // 2026-10-24T18:30Z = 25 Okt 01:30 WIB
    expect(todayWib(new Date("2026-10-24T18:30:00Z"))).toBe("2026-10-25");
    expect(todayWib(new Date("2026-10-24T16:59:00Z"))).toBe("2026-10-24");
  });
  it("memformat jam dalam WIB", () => {
    expect(formatWib("2026-10-24T03:15:00Z")).toMatch(/10[.:]15/);
  });
  it("membaca token hanya dari fragment dengan format valid", () => {
    const t = "A".repeat(43);
    expect(tokenFromHash(`#t=${t}`)).toBe(t);
    expect(tokenFromHash("#t=pendek")).toBeNull();
    expect(tokenFromHash("")).toBeNull();
    expect(tokenFromHash("#t=<script>")).toBeNull();
  });
  it("token berada di fragment, bukan query string", () => {
    const u = bookingUrl("BTG-20261024-ABC123", "x".repeat(43));
    expect(u).toContain("#t=");
    expect(u).not.toContain("?");
  });
});
