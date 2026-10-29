import type { components } from "@/generated/api";

export type Slot = components["schemas"]["Slot"];
export type Booking = components["schemas"]["Booking"];
export type Problem = components["schemas"]["Problem"];
export type BookingDetail = NonNullable<components["schemas"]["BookingDetailEnvelope"]["data"]>;

export class ClientApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
  }
}

const BASE = "/api/v1";

async function parse<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const p = (await res.json().catch(() => null)) as Problem | null;
    throw new ClientApiError(res.status, p?.code ?? "UNKNOWN", p?.detail ?? res.statusText);
  }
  if (res.status === 204) return undefined as T;
  return ((await res.json()) as { data: T }).data;
}

export async function listSlots(serviceId: string, date: string): Promise<Slot[]> {
  const q = new URLSearchParams({ service_id: serviceId, date });
  return parse(await fetch(`${BASE}/public/tenants/sibatang-batang/slots?${q}`, { cache: "no-store" }));
}

export interface BookingInput {
  slotId: string;
  serviceId: string;
  displayName: string;
  contact: string;
  allowVoiceName: boolean;
}

export async function createBooking(input: BookingInput, idempotencyKey: string): Promise<Booking> {
  return parse(
    await fetch(`${BASE}/public/bookings`, {
      method: "POST",
      headers: { "Content-Type": "application/json", "Idempotency-Key": idempotencyKey },
      body: JSON.stringify({
        tenant_code: "SIBATANG_BATANG",
        slot_id: input.slotId,
        service_id: input.serviceId,
        applicant: { display_name: input.displayName, contact: input.contact, allow_voice_name: input.allowVoiceName },
        privacy_notice_version: "2026-10-v1",
      }),
    }),
  );
}

export async function getBooking(reference: string, token: string): Promise<BookingDetail> {
  return parse(
    await fetch(`${BASE}/public/bookings/${encodeURIComponent(reference)}`, {
      headers: { "X-Booking-Token": token },
      cache: "no-store",
    }),
  );
}

export async function cancelBooking(reference: string, token: string): Promise<void> {
  await parse<void>(
    await fetch(`${BASE}/public/bookings/${encodeURIComponent(reference)}/cancel`, {
      method: "POST",
      headers: { "X-Booking-Token": token },
    }),
  );
}

export const errorMessage = (e: unknown): string => {
  if (e instanceof ClientApiError) {
    switch (e.code) {
      case "SLOT_FULL":
        return "Slot ini sudah penuh. Silakan pilih slot lain.";
      case "RATE_LIMITED":
        return "Terlalu banyak permintaan. Coba lagi sebentar lagi.";
      case "VALIDATION_ERROR":
        return `Data belum valid: ${e.message}`;
      case "STATE_CONFLICT":
        return "Pendaftaran sudah check-in dan tidak dapat dibatalkan.";
      default:
        return "Terjadi kesalahan. Silakan coba lagi.";
    }
  }
  return "Tidak dapat terhubung ke server. Periksa koneksi Anda.";
};
