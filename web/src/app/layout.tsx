import type { Metadata, Viewport } from "next";
import Link from "next/link";
import "./globals.css";

export const metadata: Metadata = {
  title: { default: "SIBATANG — Antrean Polres Batang", template: "%s · SIBATANG" },
  description: "Pendaftaran antrean layanan Polres Batang (SIBATANG).",
};
export const viewport: Viewport = { width: "device-width", initialScale: 1 };

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="id">
      <body>
        <header className="site">
          <div className="wrap">
            <Link href="/">SIBATANG</Link>
            <nav aria-label="Navigasi utama">
              <Link href="/booking">Daftar antrean</Link>
            </nav>
          </div>
        </header>
        <main className="wrap">{children}</main>
        <footer className="wrap muted">
          <small>Layanan Polres Batang · Polda Jawa Tengah</small>
        </footer>
      </body>
    </html>
  );
}
