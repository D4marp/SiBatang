export const metadata = { title: "Masuk petugas" };

export default function Masuk() {
  return (
    <div className="card">
      <h1>Masuk petugas</h1>
      <p>
        Petugas Polres Batang dan Polda Jawa Tengah masuk melalui penyedia identitas (OIDC) resmi dengan MFA.
        Tidak ada kata sandi yang disimpan oleh aplikasi ini.
      </p>
      <p className="muted">
        Integrasi OIDC belum dikonfigurasi pada lingkungan ini. Untuk staging, administrator membuat sesi dengan
        <code> sibctl dev-session</code> (dinonaktifkan di produksi).
      </p>
      <a className="btn" href="/api/v1/auth/staff/login">Masuk dengan OIDC</a>
    </div>
  );
}
