import Link from "next/link";

export default function NotFound() {
  return (
    <div className="card">
      <h1>Halaman tidak ditemukan</h1>
      <p><Link href="/">Kembali ke beranda</Link></p>
    </div>
  );
}
