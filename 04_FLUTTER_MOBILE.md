# 04 — Implementation Plan: Flutter Mobile (Citizen App)


> **KEPUTUSAN CAKUPAN v2 (mengikat):** Platform ini **hanya untuk Polda Jawa Tengah**. Hari pertama produksi hanya **Polres Batang (brand publik: SIBATANG)** sebagai tenant dan **dashboard pengawasan Polda Jawa Tengah**. Polres lain di Jawa Tengah boleh di-*onboard* kemudian, setelah otorisasi; **tidak membangun platform nasional, tidak menerima Polda lain, tidak menampilkan tenant fiktif di produksi**. Nama institusi pada paparan tertulis “Polresta Batang”, sedangkan arahan pemilik produk “Polres Batang”; nomenklatur/legal owner harus divalidasi sebelum seed produksi. Kode teknis awal menggunakan `POLRES_BATANG`.


**Owner:** Flutter mobile. **Audience:** masyarakat. **MVP priority:** Android. **iOS:** desain kompatibel sejak awal, rilis setelah perizinan, device testing dan kebutuhan disetujui. Pada slide 2/7 disebut Android sedangkan slide 8 menampilkan ikon Android/iOS: anggap iOS *pending confirmation*.

## 1. Scope dari paparan

- Beranda lima layanan: laporan kehilangan SPKT, SKCK, SIM, STNK, BPKB.
- Pilih lokasi layanan, lihat persyaratan/kuota, isi data sesuai layanan, pilih jadwal.
- Kode booking dan QR, jadwal/lokasi, status antrean & notifikasi real-time.
- Info layanan, petunjuk navigasi dan tautan ke konsultasi WhatsApp resmi.
- Formulir dan tanda tangan digital **opsional dan berdasarkan persetujuan layanan**; tidak semua formulir memiliki status hukum tanda tangan elektronik otomatis.

Pemisahan tegas: aplikasi ini mengatur **antrian dan intake**, **bukan** pengganti kewenangan resmi pemrosesan SIM, STNK, BPKB maupun dokumen layanan.

## 2. Stack ringan dan standar

| Aspek | Pilihan |
|---|---|
| SDK | Flutter **3.47.x stable** / Dart compatible, lock with FVM or pinned CI image |
| State management | Riverpod dengan `AsyncNotifier` + `go_router`; pakai satu pattern konsisten |
| HTTP | `dio` atau standard `http` (pilih satu); generated Dart OpenAPI client models |
| Data classes | `freezed`+`json_serializable` jika codegen worth it; otherwise simple immutable models |
| Local storage | `shared_preferences` only for non-sensitive preferences, `flutter_secure_storage` for credentials, short-lived cache for non-PII |
| Notifications | Firebase Cloud Messaging Android + APNs iOS via authorized service integration (conditional on approvals) |
| QR | `qr_flutter` to render; scanned by registered kiosk, QR secret must be short-lived/rotatable |
| Native integration | `url_launcher` for Maps and official WhatsApp deep link |
| Error and logging | privacy-safe error reporting/observability (tenant-aware, opt-in where needed) |
| UI | Material 3 with accessible contrast, responsive text, Indonesian locale |

**No local database needed in MVP**. Cache service catalog and successful booking summaries only as authorized; device storage is not authority for queue numbers/positions.

## 3. Pages / navigasi

```text
Splash / version check / onboarding privacy notice
└── Home: choose Polres (or default)
    ├── Select service (SPKT | SKCK | SIM | STNK | BPKB)
    │   ├── Service detail: docs / procedure / site / hours
    │   └── Booking wizard
    │       ├── Select site, date, time slot
    │       ├── Minimal identity/contact form (config-driven)
    │       ├── Verify phone if required by service
    │       └── Booking Success: reference + QR
    ├── My bookings (guest capability or account)
    │   └── Booking detail: reschedule/cancel/status/queue ETA
    ├── Service locations and directions
    ├── Notifications / user-visible event history
    ├── FAQ / Help / verified official WhatsApp contact
    └── Privacy and account/session settings
```

Route IDs `home`, `serviceList`, `serviceDetail`, `bookingSlot`, `bookingForm`, `bookingReview`, `bookingSuccess`, `bookingDetail`, `queueStatus`, `locations`, `help`, `settings`.

## 4. UX rules / end-to-end

**Booking:** Site Batang (pilot tanpa pemilihan Polres) → Service → Info prerequisites → Date/slot → Form → Review → Confirm → QR. Always display service time-zone and check-in instructions; see sample `Asia/Jakarta` but fetch tenant/site timezone from API. Defer contact/identity data until service choice; never require NIK across all services by default.

**At location:** user shows QR, kiosk scans and prints **queue ticket**, then mobile displays ticket and status. `booking_reference` before check-in is distinct from `ticket_display_number` after check-in. At every stage show *next action* and exact service location.

**Status page:** server SSE where mobile lifecycle permits, fallback to focused periodic REST fetch while app foreground; background updates via FCM if approved. Do not maintain always-on SSE in background (battery cost, OS restrictions). Queue position is approximate where policies skip priorities; do not promise exact waiting time.

**Notifications:** registered push device token is associated with authenticated booking/account, tenant and consent; notifications sanitized (`Nomor antrean Anda dipanggil. Buka aplikasi`) to avoid identifiable info appearing on lock screen. In-app latest status obtained from server after tap. No PII, token or QR in push payload.

**SMS/WA fallback:** optional official channels only after product/legal approval, robust retry and explicit message templates. All notifications are **informational**; backend remains source of truth for queue.

## 5. Repo organization

```text
lib/
  main.dart
  app/{app.dart,router.dart,theme.dart,l10n/}
  core/{network,auth,errors,storage,analytics,widgets}/
  features/
    tenant_selection/{data,domain,presentation}/
    catalog/{data,domain,presentation}/
    booking/{data,domain,presentation}/
    queue/{data,domain,presentation}/
    locations/{data,domain,presentation}/
    help/{data,domain,presentation}/
    notifications/{data,domain,presentation}/
  generated/api/
test/
integration_test/
```

A small feature-first structure, avoiding excessive clean architecture boilerplate. Repository APIs inject HTTP client, auth capabilities and `tenantSelection` verified by backend. Shared components `AppScaffold`, `ServiceCard`, `SlotPicker`, `BookingStatusTimeline`, `QRCard`, `NetworkBanner`, `EmptyState`.

## 6. State and error treatment

| Condition | UI behavior |
|---|---|
| `409 SLOT_FULL` | refresh slots, preserve non-sensitive draft, offer other valid hours |
| `409 IDEMPOTENCY_CONFLICT` | disable silent retry; compare stale form, user can retry with a new key on conscious new submit |
| `410 BOOKING_EXPIRED` | show invalid booking, choose a new slot, no old QR reused |
| `401` guest session expired | secure booking reference recovery/OTP per tenant policy; no unauthenticated leakage |
| Internet loss before submit | show no booking created (unless confirmed with server); do not issue fake QR |
| Internet loss after submit | query request status via idempotency key/recovery to avoid duplicate booking |
| Reconnect from background | fetch fresh booking/ticket state, then reattach foreground SSE if needed |
| Kiosk offline | official instruction/fallback for service site; app does not self-checkin offline |

## 7. Security and privacy specifics

- Enforce cert validation; no `badCertificateCallback => true`; no API secrets/master tokens in IPA/APK.
- Deep links validated strictly (host, path, booking ownership); block open redirects. Use **official** WhatsApp numbers loaded from approved tenant config, never user input.
- Sensitive screen protection is product decision for QR and identity; masking of logs/crash reports; avoid untrusted SDK collecting PII.
- Booking capability stored in secure storage only while required; avoid leaking to clipboard by default and into screenshots of analytics events.
- NIK/contact collection **per service schema** and legal retention; require privacy notice, explicit consent only where appropriate (other lawful bases may apply) and data deletion/subject request contact.
- Rate-limit booking/verification, OTP abuse protection, CAPTCHA only when threat model indicates and accessibility alternatives exist.
- Store official document PDFs or personal files only if policy and retention approved; minimize uploads before check-in.

## 8. Build order, test and release

| Epic | Deliverable | Acceptance |
|---|---|---|
| MOB-01 | Design system + generated API + route baseline | builds in release mode Android; UI tests pass |
| MOB-02 | Tenant/service/site information | tenant change clears stale booking selectors, respects visibility |
| MOB-03 | Booking wizard and QR | no duplicates on tap/retry, all business errors mapped |
| MOB-04 | Booking detail & live queue | reconnect/suspend/resume synchronized, no stale false status |
| MOB-05 | Help/Map/notification preferences | only official channels, graceful absence of providers |
| MOB-06 | Push integration when approved | no PII payload; revoked device token handled |
| MOB-07 | Android release + optional iOS | app signing, crash symbol upload and device QA documentation |

**Target device test grid:** low-memory Android representative (3–4GB RAM), Android version support defined by Flutter release, mainstream screen sizes, network 2G/3G simulation, Android battery restrictions, camera/screenshot brightness for kiosk scan. iOS after budget/approval with actual iPhone hardware. Run `flutter analyze`, `flutter test`, integration tests and `flutter build apk --release` in CI (use artifact signing from secrets manager only).

### Acceptance checklist

- [ ] Booking QR created only after API confirms, never locally generated.
- [ ] Correct site/location/requirements appear for each service and tenant.
- [ ] Cancellation and slot changes obey server policy and reflect on all clients.
- [ ] Check-in reflected correctly and displays printed number, not prior booking code.
- [ ] Citizen B never accesses Citizen A booking when swapping identifiers.
- [ ] Push not required for core operation; live queue works on foreground and reload.
- [ ] Accessible high-contrast text, readable QR, permission denial understandable.

**Deliverables:** mobile Flutter repo, localization strings, signed app release pipeline, app privacy statement, screenshots/test matrix, Play Store checklist and optional App Store plan.


## 9. Updated cross-client behavior for automated calling / Polda-Jateng-from-day-one

- During the pilot, the app selects **Polres Batang automatically** and does not display a Polres/province picker. Polda is only a supervisory scope, not a citizen booking target.
- When Go changes own ticket `WAITING -> CALLED`, mobile displays **ticket number, target counter, service, call time, optional estimated remaining delay** from private booking status; no client-side audio call triggers or wall-clock booking-time assumptions.
- Desktop speaker announces **name only where policy+consent allow**. Mobile does not forward citizen full name to public display or speaker. Restrict mobile notification contents to own authenticated device and privacy-safe push payload; server determines template.
- After reconnect or mobile background resume, GET own ticket snapshot. No duplicate on push vs SSE; do not display a previous ticket as still CALLED if already SERVING/COMPLETE.
- Polda staff do not access Polda dashboard via citizen booking navigation; separate workforce login web and org RBAC.

**Related:** `09_AUTOMATIC_QUEUE_VOICE.md`, `10_POLDA_FIRST_PILOT.md`.

## 10. Keputusan pilot: Batang dulu, tanpa selector provinsi

Aplikasi rilis pertama menampilkan brand **SIBATANG**, tenant default Batang, dan lokasi/lokasi layanan Batang yang sudah approved. Tidak perlu login/pilih Polda, provinsi, atau Polres di onboarding awal masyarakat. Metadata layanan didapat dari API tenant Batang dan bukan dimasukkan langsung dalam binary.

Saat ekspansi Polres Jateng lain, platform tetap satu backend; boleh dibuat branded app terpisah dengan app config/white-label atau UI gateway Jateng, **tetapi tidak otomatis menjadikan SIBATANG brand semua Polres**. Keputusan distribusi aplikasi untuk Polres baru harus ditandatangani product owner dan unit komunikasi/IT terkait.
