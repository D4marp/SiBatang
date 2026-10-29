"use client";

import { useCallback, useEffect, useState, useSyncExternalStore } from "react";
import Image from "next/image";
import QRCode from "qrcode";
import { cancelBooking, ClientApiError, errorMessage, getBooking, type BookingDetail } from "@/lib/client";
import { formatWib, tokenFromHash } from "@/lib/format";

function subscribeHash(cb: () => void) {
  window.addEventListener("hashchange", cb);
  return () => window.removeEventListener("hashchange", cb);
}

export function StatusView({ reference }: { reference: string }) {
  // null di server (belum diketahui); fragment URL dibaca di klien tanpa setState di effect.
  const hash = useSyncExternalStore(subscribeHash, () => window.location.hash, () => null);
  const token = hash === null ? undefined : tokenFromHash(hash);
  const [detail, setDetail] = useState<BookingDetail | null>(null);
  const [qr, setQr] = useState("");
  const [error, setError] = useState("");

  const refresh = useCallback(async () => {
    if (!token) return;
    try {
      setDetail(await getBooking(reference, token));
      setError("");
    } catch (e) {
      setError(e instanceof ClientApiError && e.status === 404 ? "Pendaftaran tidak ditemukan. Buka tautan lengkap dari halaman konfirmasi." : errorMessage(e));
    }
  }, [reference, token]);

  useEffect(() => {
    if (!token) return;
    const first = setTimeout(refresh, 0);
    const id = setInterval(refresh, 5000); // polling; SSE privat tersedia untuk klien native (header kustom)
    return () => {
      clearTimeout(first);
      clearInterval(id);
    };
  }, [token, refresh]);

  useEffect(() => {
    if (token) void QRCode.toDataURL(token, { margin: 2, width: 280, errorCorrectionLevel: "M" }).then(setQr);
  }, [token]);

  if (token === undefined) return <p role="status">Memuat…</p>;
  if (!token) return <div className="card"><h1>Tautan tidak lengkap</h1><p>Buka tautan pendaftaran lengkap yang Anda terima setelah mendaftar.</p></div>;

  const t = detail?.ticket;
  return (
    <div className="card">
      <h1>Pendaftaran {reference}</h1>
      {error ? <p role="alert" className="bad">{error}</p> : null}
      {detail ? (
        <>
          <p>
            Layanan <strong>{detail.service}</strong> · {detail.service_date} · {formatWib(detail.starts_at)} WIB ·{" "}
            <span className="badge">{detail.status}</span>
          </p>
          {t ? (
            <section aria-live="polite">
              <p className="muted">Nomor antrean Anda</p>
              <p className="ticket">{t.display_number}</p>
              <p>
                Status: <strong>{t.status}</strong>
                {t.counter_label ? <> · menuju <strong>{t.counter_label}</strong></> : null}
                {t.waiting_ahead_estimate != null ? <> · perkiraan {t.waiting_ahead_estimate} antrean di depan (perkiraan, bukan janji)</> : null}
              </p>
            </section>
          ) : detail.status === "CONFIRMED" ? (
            <section>
              <p>Tunjukkan QR ini ke kiosk saat tiba. Nomor antrean diterbitkan setelah check-in.</p>
              {qr ? <Image src={qr} width={280} height={280} alt="Kode QR check-in Anda" unoptimized /> : null}
              <p className="muted"><small>Berlaku sampai {formatWib(detail.qr_expires_at, true)} WIB. Jangan bagikan QR ini.</small></p>
              <button className="secondary" onClick={async () => { if (confirm("Batalkan pendaftaran ini?")) { try { await cancelBooking(reference, token); await refresh(); } catch (e) { setError(errorMessage(e)); } } }}>
                Batalkan pendaftaran
              </button>
            </section>
          ) : null}
        </>
      ) : !error ? <p role="status">Memuat…</p> : null}
    </div>
  );
}
