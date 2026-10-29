import { redirect } from "next/navigation";
import { ApiError, apiGet, type PoldaMetrics } from "@/lib/api";
import { formatWib } from "@/lib/format";

export const metadata = { title: "Monitoring Pelayanan Polda Jawa Tengah" };
export const dynamic = "force-dynamic";

export default async function PoldaOverview() {
  let m: PoldaMetrics;
  try {
    m = await apiGet<PoldaMetrics>("/polda/jateng/metrics", { auth: true });
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.status === 401) redirect("/masuk");
      if (e.status === 403) redirect("/unauthorized");
      return <div className="card" role="alert"><h1>Data tidak dapat dimuat</h1><p className="muted">Layanan sementara tidak tersedia ({e.code}).</p></div>;
    }
    throw e;
  }
  return (
    <>
      <h1>Monitoring Pelayanan Polda Jawa Tengah</h1>
      <p className="muted">
        Data agregat tanggal {m.service_date}. Diperbarui {formatWib(m.as_of)} WIB · Polres aktif: <strong>{m.active_polres_count}</strong>. Tidak memuat data pribadi warga.
      </p>
      {m.polres.map((p) => (
        <section className="card" key={p.tenant_code} aria-labelledby={`p-${p.tenant_code}`}>
          <h2 id={`p-${p.tenant_code}`}>{p.brand_name} · {p.org_code}</h2>
          {!p.data_available ? (
            <p role="status"><span className="badge warn">Data belum tersedia</span> <span className="muted">Bukan berarti nol; menunggu pembaruan pertama.</span></p>
          ) : (
            <>
              <p>
                {p.is_fresh ? <span className="badge ok">Terkini</span> : <span className="badge bad">Data basi</span>}{" "}
                <span className="muted">terakhir {p.data_as_of ? formatWib(p.data_as_of) : "-"} WIB</span>
              </p>
              <div className="table-scroll">
                <table>
                  <thead><tr><th>Booking</th><th>Hadir</th><th>Menunggu</th><th>Dipanggil</th><th>Selesai</th><th>Tidak hadir</th><th>Batal</th><th>Rata-rata tunggu</th><th>p95 tunggu</th></tr></thead>
                  <tbody>
                    <tr>
                      <td>{p.booking_count}</td><td>{p.arrived_count}</td><td>{p.waiting_count}</td><td>{p.called_count}</td>
                      <td>{p.served_count}</td><td>{p.no_show_count}</td><td>{p.cancelled_count}</td>
                      <td>{p.average_wait_seconds != null ? `${Math.round(p.average_wait_seconds / 60)} mnt` : "—"}</td>
                      <td>{p.p95_wait_seconds != null ? `${Math.round(p.p95_wait_seconds / 60)} mnt` : "—"}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </>
          )}
          <p className="muted">
            Perangkat daring: {p.devices_online ?? "—"} / {p.devices_total ?? "—"} · announcer daring: {p.announcers_online ?? "—"} · kegagalan suara 24 jam: {p.announcement_failures_24h ?? "—"}
          </p>
        </section>
      ))}
    </>
  );
}
