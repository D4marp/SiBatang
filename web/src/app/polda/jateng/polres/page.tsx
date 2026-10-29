import { redirect } from "next/navigation";
import { ApiError, apiGet, type PolresListItem } from "@/lib/api";

export const metadata = { title: "Daftar Polres Jawa Tengah" };
export const dynamic = "force-dynamic";

export default async function PolresList() {
  let items: PolresListItem[];
  try {
    items = await apiGet<PolresListItem[]>("/polda/jateng/polres", { auth: true });
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.status === 401) redirect("/masuk");
      if (e.status === 403) redirect("/unauthorized");
    }
    throw e;
  }
  return (
    <>
      <h1>Polres di bawah Polda Jawa Tengah</h1>
      <div className="card table-scroll">
        <table>
          <thead><tr><th>Polres</th><th>Kode</th><th>Brand</th><th>Status tenant</th></tr></thead>
          <tbody>
            {items.map((i) => (
              <tr key={i.org_code}><td>{i.org_name}</td><td>{i.org_code}</td><td>{i.brand_name || "—"}</td><td>{i.tenant_status || "—"}</td></tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}
