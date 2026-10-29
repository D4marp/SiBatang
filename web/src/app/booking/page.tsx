import { ApiError, apiGet, type Catalog } from "@/lib/api";
import { BookingWizard } from "./wizard";

export const metadata = { title: "Daftar antrean" };
export const dynamic = "force-dynamic";

export default async function BookingPage() {
  let catalog: Catalog | null = null;
  try {
    catalog = await apiGet<Catalog>("/public/tenants/sibatang-batang/catalog");
  } catch (e) {
    if (!(e instanceof ApiError)) throw e;
  }
  if (!catalog || catalog.sites.length === 0) {
    return (
      <div className="card" role="status">
        <h1>Pendaftaran belum tersedia</h1>
        <p className="muted">Silakan datang langsung ke loket atau coba lagi nanti.</p>
      </div>
    );
  }
  return (
    <>
      <h1>Daftar antrean</h1>
      <BookingWizard catalog={catalog} />
    </>
  );
}
