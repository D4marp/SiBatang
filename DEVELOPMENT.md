# SIBATANG — Panduan Pengembangan & Status Implementasi

Repositori ini mengimplementasikan blueprint pada `README.md` dan `01`–`16_*.md`. Aturan teknik mengikat ada di `AGENTS.md`.
Cakupan: **hanya Polda Jawa Tengah**; pilot produksi **hanya Polres Batang (`SIBATANG_BATANG`)**.

```text
backend/   Go 1.27 modular monolith (REST + SSE), worker, sibctl, migrasi SQL, api/openapi.yaml
web/       Next.js 16 (App Router, TypeScript strict): publik SIBATANG + dashboard Polda
mobile/    Flutter: aplikasi warga (booking, QR, status)
desktop/   Flutter Windows: kiosk, display, announcer (operator = menunggu OIDC)
infra/     Dockerfile, compose dev, init role PostgreSQL
```

## Menjalankan

```bash
# 1) Backend + test (butuh PostgreSQL 18 untuk tes integrasi/RLS)
docker run -d --name sib-pg-test -e POSTGRES_PASSWORD=test -p 54329:5432 postgres:18
cd backend
TEST_DATABASE_URL='postgres://postgres:test@localhost:54329/postgres?sslmode=disable' go test ./... -race

# 2) Stack lokal (Postgres + migrate + api + worker + web)
cp infra/.env.example infra/.env     # isi nilai acak lokal, DATA_ENCRYPTION_KEY=$(openssl rand -base64 32)
docker compose -f infra/docker/compose.dev.yml --env-file infra/.env up --build

# 3) Web saja
cd web && pnpm install && pnpm gen:api && pnpm lint && pnpm typecheck && pnpm test && pnpm build
```

Bootstrap pengguna **bernama** (tanpa password; identitas dari OIDC `sub`):

```bash
export MIGRATOR_DATABASE_URL=postgres://sib_migrator:...@localhost:5432/sibatang
sibctl user-add --subject <oidc-sub> --name "Nama Petugas"
sibctl membership-add --subject <oidc-sub> --role POLDA_MONITOR --scope-type POLDA_JATENG --scope POLDA_JAWA_TENGAH
sibctl dev-session --subject <oidc-sub>      # hanya APP_ENV non-produksi; produksi memakai OIDC
```

## Status terhadap Definition of Done (`AGENTS.md`)

| Gate | Status |
|---|---|
| `go test ./... -race` | **Hijau** (PostgreSQL 18; tes layanan, HTTP, domain, OIDC) |
| `go vet`, `gofmt` | Hijau |
| OpenAPI valid | **Hijau** (`redocly lint`, `backend/api/openapi.yaml`) |
| Web `lint`, `typecheck`, `test`, `build` | **Hijau** (tipe TS dihasilkan dari OpenAPI) |
| Web E2E (Playwright) | **Belum ada** |
| Flutter `analyze`/`test`/`build` | **Belum dijalankan** — Flutter SDK tidak tersedia di mesin pembuat; kode Dart ditulis tetapi tidak dikompilasi. CI (`.github/workflows/ci.yml`) menjalankannya. |
| Migrasi fresh DB | Hijau (3 migrasi dari nol, memakai role non-superuser `sib_migrator`) |
| Migrasi upgrade baseline N−1→N | Belum relevan (baru migrasi awal) |
| Review scope-owner, rollback runbook | Belum |

## Yang sudah terbukti oleh tes (invarian `AGENTS.md`)

- 12 booking paralel untuk slot kapasitas 1 → tepat 1 sukses; idempotensi (replay sama, body beda → `IDEMPOTENCY_KEY_REUSED`); QR token tidak tersimpan plaintext.
- 8 scan QR paralel → satu tiket (`SKCK-001`), tidak ada nomor ganda; scan ulang = tiket sama (`is_reprint`).
- Dua operator memanggil satu tiket → satu `CALLED`, satu job announcement.
- `AUTO_ADVANCE_ON_READY` hanya saat loket `READY` dan ditugaskan; tidak untuk `PAUSED`/`CLOSED`/`SERVING`.
- Lease zona + fencing token: announcer kedua standby, takeover menaikkan token, ack basi ditolak, job `PLAYING` basi menjadi `FAILED` (tidak diputar ulang), kegagalan audio tidak mengubah tiket.
- Nama panggilan: default `NUMBER_ONLY`; nama hanya di klaim privat bila kebijakan zona + consent; **tidak ada** di `queue_events`, outbox, audit, metrik, health, snapshot display, atau respons warga.
- RLS: tanpa konteks tenant 0 baris; tenant A tak melihat B; `WITH CHECK` menolak tulis lintas tenant; setiap tabel ber-`tenant_id` wajib `FORCE RLS` (tes inventaris); `sib_api`/`sib_reporting` bukan superuser/BYPASSRLS/login.
- Polda: hanya agregat via role `sib_reporting` (tanpa grant ke booking/tiket/perangkat/audit); tanpa wewenang antrean/check-in; membership dicabut berlaku segera; tiap pembacaan diaudit; data belum tersedia ≠ nol.
- Hierarki: root Polda kedua ditolak DB, Polres di bawah Polres/non-Jateng ditolak, hierarki immutable; onboarding draft-only, parent tidak diterima dari klien, pemohon tidak boleh menyetujui sendiri; publikasi dibatasi allowlist pilot + readiness.

## Batasan & keputusan implementasi (harus dibaca sebelum go-live)

1. **Login staf OIDC sudah ada** (code+PKCE, verifikasi id_token, MFA wajib untuk peran privilese, tanpa auto-provision; lihat `docs/OIDC.md`), diuji dengan IdP palsu — belum dengan IdP institusi nyata (OPEN-08). Tanpa konfigurasi `OIDC_*` login mengembalikan 501. Mode **operator desktop** masih placeholder karena butuh alur native OIDC.
2. **Seed produksi** hanya org/tenant/site/layanan **DRAFT** (nomenklatur "Polres Batang" vs "Polresta Batang" belum diverifikasi — OPEN-01). Tenant `SIBATANG_BATANG` baru `PUBLISHED` setelah checklist kesiapan (site+layanan terbit, zona audio, loket, kiosk/display/announcer aktif, slot terbuka) dan persetujuan `POLDA_ORG_ADMIN`.
3. **Kebijakan antrean per loket** (`auto_announce_on_call`, `auto_advance_on_ready`, recall) disimpan sebagai kolom `counters`, bukan tabel `queue_counter_policies` terpisah, dan endpoint-nya `PATCH /staff/counters/{id}/queue-policy` (bukan `/staff/sites/{id}/queue-policy`). OpenAPI mengikuti implementasi.
4. **SSE** memakai polling 1 dtk atas `queue_events`/`queue_announcements` (durable, `Last-Event-ID`, revalidasi otorisasi tiap siklus). `LISTEN/NOTIFY` sebagai pemicu belum ada; polling cukup untuk satu instance pilot.
5. **Auto-advance** langsung setelah `COMPLETE`/`READY` (tanpa jeda `run_after` terjadwal). Prioritas disabilitas/lansia/walk-in **tidak** di-hardcode (butuh persetujuan pemilik layanan); urutan = waktu check-in.
6. **Belum ada**: walk-in, reschedule, form digital, notifikasi push/WhatsApp (worker hanya menandai outbox tanpa handler), upload, peta, ekspor CSV, UI admin Polres/onboarding Polda di web (API-nya ada), bangunan pembacaan `audit` via UI, small-cell suppression.
7. **Printer & TTS Windows** belum terverifikasi di perangkat nyata: `MockPrinter` sebagai placeholder; announcer memakai `flutter_tts` dan **gagal tertutup** bila suara `id-ID` tidak terpasang (OPEN-04). Klien Dart mobile ditulis manual; generator Dart dari OpenAPI belum dipasang.
8. Rate limit publik di memori (per instance); ganti ke penyimpanan bersama saat >1 replika (ADR).
9. Enkripsi kontak warga: AES-GCM di aplikasi dengan `DATA_ENCRYPTION_KEY` dari env. Rotasi kunci & KMS belum ada.
10. Dua kolom nama tersimpan sebagai `citizen_public_name` (terbatas, hanya untuk nama panggilan bila consent). Retensi/penghapusan PII belum diotomasi.
