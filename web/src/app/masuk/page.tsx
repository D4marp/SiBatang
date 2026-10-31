export const metadata = { title: "Masuk petugas" };

const MESSAGES: Record<string, string> = {
  state: "Sesi masuk kedaluwarsa atau tidak valid. Silakan coba lagi.",
  idp: "Penyedia identitas menolak atau gagal merespons. Silakan coba lagi.",
  mfa: "Akun Anda wajib memakai verifikasi multi-faktor (MFA) di penyedia identitas.",
  not_registered: "Akun Anda belum didaftarkan. Hubungi administrator untuk penugasan peran.",
  not_configured: "Masuk petugas belum dikonfigurasi pada lingkungan ini.",
  server: "Terjadi kesalahan pada server. Silakan coba lagi.",
};

export default async function Masuk({ searchParams }: { searchParams: Promise<{ error?: string }> }) {
  const { error } = await searchParams;
  return (
    <div className="card">
      <h1>Masuk petugas</h1>
      <p>
        Petugas Polres Batang dan Polda Jawa Tengah masuk melalui penyedia identitas (OIDC) resmi dengan MFA.
        Tidak ada kata sandi yang disimpan oleh aplikasi ini.
      </p>
      {error ? (
        <p role="alert" className="bad">{MESSAGES[error] ?? MESSAGES.server}</p>
      ) : null}
      <a className="btn" href="/api/v1/auth/staff/login">Masuk dengan OIDC</a>
    </div>
  );
}
