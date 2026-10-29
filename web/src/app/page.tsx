import Link from "next/link";
import { ApiError, apiGet, type Catalog } from "@/lib/api";

export const dynamic = "force-dynamic";

async function loadCatalog(): Promise<Catalog | null> {
  try {
    return await apiGet<Catalog>("/public/tenants/sibatang-batang/catalog");
  } catch (e) {
    if (e instanceof ApiError) return null;
    throw e;
  }
}

export default async function Home() {
  const catalog = await loadCatalog();
  return (
    <>
      <h1>{catalog?.brand_name ?? "SIBATANG"} — Antrean Polres Batang</h1>
      <p className="muted">Daftar online, datang sesuai jadwal, scan QR di kiosk, lalu tunggu panggilan di loket.</p>
      {!catalog || catalog.sites.length === 0 ? (
        <div className="card" role="status">
          <strong>Layanan belum tersedia.</strong>
          <p className="muted">Pendaftaran online belum dibuka atau sedang dalam pemeliharaan. Silakan datang langsung ke loket.</p>
        </div>
      ) : (
        <>
          <div className="grid">
            {catalog.sites.map((site) => (
              <section className="card" key={site.id} aria-labelledby={`s-${site.code}`}>
                <h2 id={`s-${site.code}`}>{site.name}</h2>
                {site.address ? <p className="muted">{site.address}</p> : null}
                <ul>
                  {site.services.map((svc) => (
                    <li key={svc.service_id}>{svc.display_name}</li>
                  ))}
                </ul>
              </section>
            ))}
          </div>
          <p><Link className="btn" href="/booking">Daftar antrean sekarang</Link></p>
        </>
      )}
    </>
  );
}
