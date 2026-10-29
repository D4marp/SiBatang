# 02 — Implementation Plan: Web Next.js


> **KEPUTUSAN CAKUPAN v2 (mengikat):** Platform ini **hanya untuk Polda Jawa Tengah**. Hari pertama produksi hanya **Polres Batang (brand publik: SIBATANG)** sebagai tenant dan **dashboard pengawasan Polda Jawa Tengah**. Polres lain di Jawa Tengah boleh di-*onboard* kemudian, setelah otorisasi; **tidak membangun platform nasional, tidak menerima Polda lain, tidak menampilkan tenant fiktif di produksi**. Nama institusi pada paparan tertulis “Polresta Batang”, sedangkan arahan pemilik produk “Polres Batang”; nomenklatur/legal owner harus divalidasi sebelum seed produksi. Kode teknis awal menggunakan `POLRES_BATANG`.


**Owner:** Frontend Web. **Cross-team contracts:** `01_ARCHITECTURE_TENANCY_RBAC.md`, `06_API_DATA_CONTRACT.md`.

## 1. Produk web yang dibangun

Web Next.js memuat **dua experience dalam satu codebase**: (A) layanan publik masyarakat via mobile browser tanpa instal aplikasi; (B) back-office Polres dan command center Polda. Kiosk dan perangkat loket native dibangun dalam Flutter desktop, bukan disimulasikan sebagai web-only hardware integration.

### Mapping paparan

- Slide 4/6: daftar online → pilih layanan/jadwal → QR booking → hadir scan → antrean → loket.
- Slide 5: info lokasi SPKT/SKCK, SATPAS/SIM, SAMSAT/STNK-BPKB.
- Slide 7: website pendaftaran, ketersediaan kuota, info lokasi, monitoring.
- Slide 8: beranda, layanan, form, QR, status, info persyaratan/lokasi; admin features merupakan rancangan tambahan.

## 2. Versi, stack dan prinsip ringan

| Layer | Pilihan v1 | Catatan |
|---|---|---|
| Framework | Next.js **16.3.x** App Router, React supported by Next | pin minor/patch dan lockfile; validasi patch terbaru saat kickoff |
| Runtime | Node.js **24 LTS** | minimum framework memang lebih rendah; pilih LTS supported untuk produksi |
| Language | TypeScript `strict: true` | ESLint + Prettier atau Biome (pilih satu konsisten) |
| UI | Tailwind CSS + small accessible component set | shadcn/Radix bila cocok; hindari seluruh component mega-library |
| Forms | React Hook Form + Zod | input and API both validate, schema backend authoritative |
| Data fetching | Server Components `fetch` untuk initial, TanStack Query hanya interaksi mutating/live bila perlu | hindari duplicate state dan fetch waterfall |
| API | Same-origin `/api/v1/*` reverse-proxy → Go; OpenAPI-generated TS types | **no direct browser ↔ DB** |
| Live | SSE via same-origin cookie authorized, reconnection and resync REST | polling fallback 10–20s ketika SSE tidak stabil |
| Auth | Go-managed server session + OIDC for staff | Web browser uses secure HttpOnly SameSite cookie; no JWT in localStorage |
| Charts | lightweight chart library loaded only on reporting routes | reporting aggregations di backend, bukan menghitung raw citizen data di browser |

Next deployment Node server plus Go API behind reverse proxy (self-host/VPS/private infra). **Jangan menaruh API dan database authoritative di Vercel free** karena regulasi data, uptime dan koneksi real-time harus dikendalikan instansi.

## 3. Site map dan route contract

```text
app/
  (public)/
    page.tsx                         -> katalog Batang; selector Polres baru ketika ada >1 tenant Jateng aktif
    layanan/[serviceSlug]/page.tsx   -> deskripsi, syarat, jadwal, lokasi
    booking/page.tsx                 -> wizard site Batang→service→slot (pilot)
    booking/[ref]/page.tsx           -> detail booking with ownership capability
    antrean/[ref]/page.tsx           -> status antrean pribadi, realtime
    lokasi/page.tsx                  -> daftar lokasi dan navigasi
    bantuan/page.tsx                 -> FAQ dan kanal WhatsApp resmi
  (staff)/
    dashboard/page.tsx               -> landing by effective scope
    polres/[tenantCode]/overview/page.tsx
    polres/[tenantCode]/bookings/page.tsx
    polres/[tenantCode]/sites/page.tsx
    polres/[tenantCode]/schedules/page.tsx
    polres/[tenantCode]/services/page.tsx
    polres/[tenantCode]/counters/page.tsx
    polres/[tenantCode]/members/page.tsx
    polres/[tenantCode]/devices/page.tsx
    polres/[tenantCode]/audio-zones/page.tsx  -> konfigurasi speaker, TTS, kebijakan nama
    polres/[tenantCode]/queue-policies/page.tsx -> auto-advance, pause policy
    polres/[tenantCode]/reports/page.tsx
    polda/jateng/overview/page.tsx
    polda/jateng/polres/page.tsx
    polda/jateng/onboarding/page.tsx     -> provisioning Polres baru via approval
    polda/jateng/devices/page.tsx        -> health semua site/speaker terotorisasi
    polda/jateng/performance/page.tsx
    polda/jateng/audit/page.tsx
  unauthorized/page.tsx
  not-found.tsx
```

Route param (`tenantCode`) is **selector only**; Go checks server-verified membership on every call. Next `proxy.ts` (Next 16 naming) may redirect unauthenticated browser requests for UX, **never** grants permissions itself.

## 4. Pages dan acceptance criteria

| ID | Halaman | User story | Definition of Done |
|---|---|---|---|
| WEB-PUB-01 | Landing | Warga membuka SIBATANG Batang (tenant otomatis dipilih) | layanan Batang published only, low bandwidth; selector Jateng baru setelah ekspansi |
| WEB-PUB-02 | Service detail | Lihat syarat, jam, lokasi | konten admin-versioned, last updated, shareable URL |
| WEB-PUB-03 | Booking wizard | Pilih lokasi, layanan, tanggal, slot, isi data minimum | server-side slot capacity, idempotency, form validation, conflict recovery |
| WEB-PUB-04 | Booking QR | Dapat QR dan instruksi check-in | QR opaque, print/screenshot friendly, tidak membocorkan NIK/HP |
| WEB-PUB-05 | Personal queue | Lihat nomor, status dan posisi/ETA jika tersedia | SSE+reconnect; ETA jelas *perkiraan* dan tidak dijanjikan |
| WEB-PUB-06 | Cancel/reschedule | Ubah booking sesuai aturan tenant | backend authorization ownership, slot release exactly once |
| WEB-ADMIN-01 | Polres home | Lihat antrean & unit | tenant-only metrics, filters site/service/day |
| WEB-ADMIN-02 | Catalog/schedule | Konfigurasi kapasitas dan libur | mutation audit, validation, no retroactive slot corruption |
| WEB-ADMIN-03 | Members & roles | Tunjuk admin/site/operator | scoped grants, duplicate restrictions, least-privilege defaults |
| WEB-ADMIN-04 | Devices | Daftarkan kiosk/display/printer association | activation code short life, revoke & heartbeat |
| WEB-ADMIN-05 | Reports | Lihat jumlah, waktu tunggu, no-show | CSV filtered, permission+audit, no default raw PII |
| WEB-ADMIN-06 | Audio zone + auto queue policy | Set speaker zone, preview voice synthetic, mode auto-next per counter, nama/nomor-only | auditable per-tenant scope; never expose public call name in logs/Polda |
| WEB-POLDA-01 | Rollup | Pantau Polres bawahan | date filters, freshness tag, drilldown aggregate-only |
| WEB-POLDA-02 | Comparison | Bandingkan KPI per Polres/service | grouping normalized, no data outside approved Jawa Tengah descendant tenants, explain missing data |
| WEB-POLDA-03 | Incidents | Cari site perangkat offline/service outage | metadata-level read and monitoring; no direct ticket mutation |
| WEB-POLDA-04 | Onboarding & org scope | Lihat pohon Polres, ajukan onboarding dan approval status | available **during pilot Batang**, forbidden beyond Polda Jawa Tengah boundary, no fake production tenant |
| WEB-POLDA-05 | Announcer health | Monitor speaker online/error per Polres, without names | aggregate only; correct organization scope and freshness |

### Visitor booking wizard

```text
Step 1: Tenant otomatis SIBATANG_BATANG (pilot, tanpa selector Polres)
Step 2: Pilih site + jenis layanan
Step 3: Persyaratan, slot terpublikasi, jam operasional
Step 4: Form minimal + privacy disclosure (+ verifikasi sesuai kebijakan)
Step 5: POST booking w/ Idempotency-Key
Step 6: Confirmation + QR + lokasi + check-in instruction
```

**Tidak** langsung menerbitkan nomor antrean final saat booking; sesuai slide 4/6 nomor tiket tercetak saat tiba/check-in. `booking_reference` dipisahkan dari `ticket_display_number`.

## 5. Komponen dan struktur folder

```text
src/
  app/                                  # Next App Router
  features/
    booking/{components,schemas,api,model}/
    queue/{components,hooks,api}/
    catalog/{components,api}/
    organization/{components,api}/
    reports/{components,api}/
    administration/{components,api}/
  components/{ui,layout,feedback}/
  lib/{api-client,auth,accessibility,formatting}/
  generated/api/                        # type generated from shared OpenAPI
  styles/
```

- Prefer React Server Components for static/service lists; client components only date picker, forms, optimistic actions and charts/SSE.
- Strong URL state for filters; pagination server-driven. Do not ship 50k queue rows to the browser.
- `Intl.DateTimeFormat('id-ID', {timeZone: 'Asia/Jakarta'})` by site timezone from API; backend saves UTC timestamps + local service dates.
- Responsive mobile-first, WCAG 2.2 AA design target, keyboard support, readable status color+text, screenreader announcement for queue position and confirmation.

## 6. Data and permissions implementation rules

**Server Components:** forward cookies only to trusted Go API, with `cache: 'no-store'` for user, booking, queue and admin requests; never share a tenant-specific Next fetch cache across tenant/users. Public info can be cached with tenant+service distinct keys and invalidation.

**Client:** cookie-backed POST requires validated Origin + anti-CSRF token/header per auth design. SSE endpoint same-origin `/api/v1/queue/streams/...`; SSE requires membership/booking capability each connect and event filtering server side.

Example render permission (visual only):

```tsx
const mayManageDevices = permissions.includes('device.enroll');
return mayManageDevices ? <DevicesMenu /> : null;
// NOTE: Go backend still checks policy on every endpoint.
```

**Public tracking links:** browser should avoid storing access-capability in analytics URL query/Referer. Prefer redemption into short-lived HttpOnly session; share booking reference separately; never expose secret in public HTML, CDN or server logs.

## 7. Polda dashboard data contract (summary only)

`GET /api/v1/polda/{orgId}/metrics?from=2026-10-01&to=2026-10-24&group_by=polres`

Sample response aggregate: `polres_id/name`, `bookings`, `checked_in`, `served`, `no_show`, `avg_wait_seconds`, `p95_wait_seconds`, `device_online_pct`, `data_freshness_at`. **Never** include citizen name, phone, NIK, QR, raw form, or chat transcript.

Filters: time range, service, tenant/site, status. Permission restrictions applied to set of selectable Polres on server; user-supplied filter is just narrowing authorized results.

## 8. Performance and hardening

- Static icons/svg, responsive images, avoid auto-loading heavy maps on landing; open maps via native deep link when needed.
- Target initial JS budget **< 200KB gzip route-specific for core public flows** (engineering budget, evaluate build output); charts lazy-loaded, avoid unnecessary animation.
- Skeleton and meaningful error states; 429 retry-after, 409 slot full → suggest alternatives, lost SSE → poll and reconnect, 401 → reauth.
- CSP with nonces where practical, `X-Content-Type-Options`, referrer policy, secure cookie, HSTS at reverse proxy.
- Error boundary, full-page offline explanation, health endpoint for deployment, no debug info for public 500.
- Test: Vitest+React Testing Library, Playwright E2E, `axe` accessibility; test at 360 px and desktop, browser back/refresh mid-booking, timezone boundary.

## 9. Delivery slices and dependencies

1. `WEB-01` bootstrap, lint/types/OpenAPI client, global design tokens and auth/session UX.
2. `WEB-02` tenant/site/service pages and dynamic catalog.
3. `WEB-03` booking wizard, slot selection, error/idempotent retry, QR details.
4. `WEB-04` personal queue via SSE, fallback, cancel/reschedule rules.
5. `WEB-05` Polres admin for site, service, schedules, counters/devices/users.
6. `WEB-06` Polres analytics and Polda authorized aggregates.
7. `WEB-07` accessibility, rate-limit UX, security review, internationalization readiness.

**Requires from backend first:** approved OpenAPI `/public/tenants`, `/services`, `/slots`, `/bookings`, `/my/bookings`, `/queue`, `/auth/me`, `/staff/...`, `/polda/...`. Teams use contract mocks before backend integration.

## 10. Acceptance test wajib

- [ ] Guest completes full flow web→QR→kiosk→queue→loket.
- [ ] Same user double-submit does not duplicate booking; full slot returns actionable `409`.
- [ ] Batang route/input cannot access synthetic staging Polres Uji B, including hidden filters and direct API calls.
- [ ] Polda sees own subordinate Polres rollup, no personal records.
- [ ] Booking QR and permissions not cached into another user's session.
- [ ] Public and dashboard work on supported mobile browsers and low-bandwidth scenario.
- [ ] No release if Playwright E2E, TS, lint, accessibility regression checks fail.

**Handoff output:** Next repository, `.env.example` without secrets, buildable container, route map, OpenAPI-generated types, screenshots+test reports, deployment README.


## 11. Revisi wajib pilot (voice + Polda first)

- **Week 3–4:** build `/polda/jateng/overview` (functional shell backed by real permissions), tenant list and login. **Week 5–7:** real aggregate backed by Batang/staging data, plus onboarding. Do not schedule all Polda development at week 15.
- **Polres audio config:** `/polres/[tenantCode]/audio-zones` manages authorized site-zone mapping, speaker status, id-ID voice diagnostics (synthetic demo only), name policy, template preview; site managers limited to their site. `/queue-policies` toggles `AUTO_ADVANCE_ON_READY` only for approved staffed counters. Operator desktop remains official queue controller.
- **Polda UI:** show aggregate status `as_of`, freshness and registered announcer health by Polres/site; **never** expose spoken names, full TTS text, raw booking, audio job claim API or unrestricted cross-tenant exports.
- Polda org admin onboarding flow is **draft→approval→configuration→UAT→publish**; provisioning a tenant is not a normal web form `POST` by any admin without backend checks.
- Ensure tests: change `tenantCode` in URL to unknown/non-Jateng organization denied; policy change from Batang to staging Polres Uji B denied; Polda HTML/SSR cache cannot mix supervisor scopes; missing/failed announcer clearly alerts operations.

**Contracts:** `09_AUTOMATIC_QUEUE_VOICE.md`, `10_POLDA_FIRST_PILOT.md`.

## 12. Aturan branding dan navigasi pilot Batang

- Root website publik menampilkan **SIBATANG / Polres Batang** sebagai satu-satunya tenant published. Tidak ada pemilih provinsi, Polda, dan Polres fiktif.
- Panel Polda menggunakan route stabil `/polda/jateng/*`; Polda **Jateng saja**. Ketika tenant Jateng kedua benar-benar publish, tampilkan dropdown Polres Jateng saja dari `/api/v1/polda/jateng/polres`.
- Data `tenant_id` tidak disimpan sebagai unrestricted localStorage switch; setiap page SSR/API harus cek scoped membership Go. Batang public default diselesaikan oleh server-side config `DEFAULT_PUBLIC_TENANT_CODE=SIBATANG_BATANG`.
- UI Batang memakai brand SIBATANG, tetapi tidak menghardcode logo/name ke seluruh platform karena Polres Jateng lain kelak akan punya branding sendiri.
