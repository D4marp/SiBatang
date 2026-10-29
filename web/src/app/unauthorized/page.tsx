export const metadata = { title: "Tidak diizinkan" };

export default function Unauthorized() {
  return (
    <div className="card">
      <h1>Akses tidak diizinkan</h1>
      <p>Akun Anda tidak memiliki wewenang untuk halaman ini. Hubungi administrator unit Anda.</p>
    </div>
  );
}
