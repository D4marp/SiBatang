"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import type { Catalog } from "@/lib/api";
import { createBooking, errorMessage, listSlots, type Slot } from "@/lib/client";
import { bookingUrl, formatWib, todayWib } from "@/lib/format";

export function BookingWizard({ catalog }: { catalog: Catalog }) {
  const router = useRouter();
  const options = useMemo(
    () => catalog.sites.flatMap((s) => s.services.map((v) => ({ id: v.service_id, label: `${v.display_name} — ${s.name}` }))),
    [catalog],
  );
  const [serviceId, setServiceId] = useState(options[0]?.id ?? "");
  const [date, setDate] = useState(todayWib());
  const [slots, setSlots] = useState<Slot[] | null>(null);
  const [slotId, setSlotId] = useState("");
  const [name, setName] = useState("");
  const [contact, setContact] = useState("");
  const [voice, setVoice] = useState(false);
  const [consent, setConsent] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  // Satu kunci idempotensi per percobaan: retry jaringan tidak membuat booking ganda.
  const idemKey = useRef<string>("");

  useEffect(() => {
    let cancelled = false;
    if (!serviceId) return;
    listSlots(serviceId, date)
      .then((s) => !cancelled && setSlots(s))
      .catch((e) => !cancelled && (setSlots([]), setError(errorMessage(e))));
    return () => {
      cancelled = true;
    };
  }, [serviceId, date]);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setBusy(true);
    idemKey.current ||= crypto.randomUUID();
    try {
      const b = await createBooking({ slotId, serviceId, displayName: name.trim(), contact: contact.trim(), allowVoiceName: voice }, idemKey.current);
      router.push(bookingUrl(b.reference, b.qr_token ?? ""));
    } catch (err) {
      setError(errorMessage(err));
      setBusy(false);
      if (err instanceof Error && "code" in err && ["SLOT_FULL", "VALIDATION_ERROR"].includes(String((err as { code: string }).code))) idemKey.current = "";
    }
  }

  return (
    <form onSubmit={submit} className="card" aria-describedby={error ? "form-error" : undefined}>
      <label htmlFor="svc">Layanan</label>
      <select id="svc" value={serviceId} onChange={(e) => { setServiceId(e.target.value); setSlots(null); setSlotId(""); }}>
        {options.map((o) => (
          <option key={o.id} value={o.id}>{o.label}</option>
        ))}
      </select>

      <label htmlFor="date">Tanggal (WIB)</label>
      <input id="date" type="date" value={date} min={todayWib()} onChange={(e) => { setDate(e.target.value); setSlots(null); setSlotId(""); }} required />

      <fieldset style={{ border: 0, padding: 0, margin: "16px 0 0" }}>
        <legend style={{ fontWeight: 600 }}>Jam kedatangan</legend>
        {slots === null ? <p className="muted" role="status">Memuat slot…</p> : slots.length === 0 ? (
          <p className="muted">Tidak ada slot tersedia pada tanggal ini.</p>
        ) : (
          <div className="slots">
            {slots.map((s) => (
              <button
                type="button"
                key={s.slot_id}
                className={`secondary slot`}
                aria-pressed={slotId === s.slot_id}
                disabled={s.remaining === 0}
                onClick={() => setSlotId(s.slot_id)}
              >
                {formatWib(s.starts_at)} <small>({s.remaining} sisa)</small>
              </button>
            ))}
          </div>
        )}
      </fieldset>

      <label htmlFor="name">Nama</label>
      <input id="name" value={name} onChange={(e) => setName(e.target.value)} minLength={2} maxLength={80} autoComplete="name" required />
      <label htmlFor="contact">Nomor HP</label>
      <input id="contact" value={contact} onChange={(e) => setContact(e.target.value)} inputMode="tel" minLength={3} maxLength={100} autoComplete="tel" required />

      <label style={{ fontWeight: 400 }}>
        <input type="checkbox" checked={voice} onChange={(e) => setVoice(e.target.checked)} />
        Izinkan nama depan saya disebut saat dipanggil melalui pengeras suara (opsional; bila tidak dicentang hanya nomor antrean yang disebut).
      </label>
      <label style={{ fontWeight: 400 }}>
        <input type="checkbox" checked={consent} onChange={(e) => setConsent(e.target.checked)} required />
        Saya telah membaca pemberitahuan privasi (versi 2026-10-v1). Data hanya dipakai untuk layanan antrean ini.
      </label>

      {error ? <p id="form-error" role="alert" className="bad">{error}</p> : null}
      <p><button type="submit" disabled={busy || !slotId || !consent}>{busy ? "Memproses…" : "Konfirmasi pendaftaran"}</button></p>
      <p className="muted"><small>Nomor antrean diterbitkan saat Anda scan QR di kiosk, bukan saat mendaftar.</small></p>
    </form>
  );
}
