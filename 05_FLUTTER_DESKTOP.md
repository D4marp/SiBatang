# 05 — Implementation Plan: Flutter Desktop (Operator + Kiosk + Display + Voice Announcer)


> **KEPUTUSAN CAKUPAN v2 (mengikat):** Platform ini **hanya untuk Polda Jawa Tengah**. Hari pertama produksi hanya **Polres Batang (brand publik: SIBATANG)** sebagai tenant dan **dashboard pengawasan Polda Jawa Tengah**. Polres lain di Jawa Tengah boleh di-*onboard* kemudian, setelah otorisasi; **tidak membangun platform nasional, tidak menerima Polda lain, tidak menampilkan tenant fiktif di produksi**. Nama institusi pada paparan tertulis “Polresta Batang”, sedangkan arahan pemilik produk “Polres Batang”; nomenklatur/legal owner harus divalidasi sebelum seed produksi. Kode teknis awal menggunakan `POLRES_BATANG`.


**Owner:** Flutter desktop + field hardware integration. **First target:** Windows x64; Linux optional after compatibility lab tests. Build one shared Flutter desktop codebase with **four deployment modes** (kiosk, operator, display, announcer) separated by login/device role and configuration.

## 1. Desktop modes and what they do

| Mode | Target hardware / user | Rights | Primary screens |
|---|---|---|---|
| `KIOSK` | Public touchscreen near SPKT/SKCK, SATPAS, SAMSAT | `DEVICE_KIOSK`, exact tenant/site | scan booking QR; verify check-in; ticket printer; optional walk-in intake |
| `OPERATOR` | PC loket (officer logged in) | `SERVICE_OPERATOR`, assigned counter | next ticket, recall, serving, complete, no-show, shift, issue inspection history |
| `DISPLAY` | PC/mini PC connected to TV/monitor | `DEVICE_DISPLAY`, exact tenant/site | queue numbers called, counter assignments, next tickets and notices |
| `ANNOUNCER` | PC/mini PC connected to local loudspeaker/PA | `DEVICE_ANNOUNCER`, exact tenant/site/audio zone | offline Indonesian TTS number + policy-allowed name + target counter, delivery/health |

Slide 1, 4, 6, 7: QR kiosk → print ticket/form → live display → loket. **Printing digital form and sign capture are optional separate modules** after form legal/operational confirmation.

**Why not put hardware only in Next.js:** scanner USB, printer spooler, kiosk auto-start, screen control, watchdog, driver-specific recovery and Windows service integration are easier to test in native desktop.

## 2. Stack and device interface

| Concern | Choice |
|---|---|
| Flutter | 3.47.x stable, Dart supported |
| State | Riverpod + go_router (or simple Navigator for fixed kiosk, choose one) |
| Networking | OpenAPI-generated Dart client, REST JSON; foreground SSE + resync, websocket **not required** |
| Secure credentials | OS credential store / `flutter_secure_storage` if verified on platform |
| Printing | Driver/spooler-based PDF/print job first; raw ESC/POS only with certified printer model and native plugin |
| Scanner | USB HID keyboard wedge best baseline; camera QR through tested platform plugin fallback |
| Kiosk runtime | Windows assigned access / shell launcher where licensed, app auto-launch on sign-in, OS policies managed by IT |
| Display | borderless/fullscreen kiosk mode, screen-specific layouts, monitor resolution discovery |
| Telemetry | health ping, printer state / last error, app version, last successful network and event timestamp |

**Hardware procurement gate:** confirm sample brand/model of printer (58 vs 80mm), Windows driver version, QR scanner (HID vs serial), touchscreen resolution, DPI, USB/serial and network, OS image, site power backup. Flutter plugin alone does not guarantee printer compatibility.

## 3. Distinct UI flows

### 3.1 Kiosk

```text
Kiosk start → show site/service brand (read-only)
  → Scan QR booking / walk-in intake (if enabled)
  → POST checkin (scoped device authentication)
  → 200 confirmed ticket + printable ticket payload
  → Print through Windows spooler
  → Show success/number / reprint with policy & audit
  → Return idle after inactivity (wipe sensitive transient state)
```

**Ticket** minimum contents: logo/site/service, number `SKCK-025`, date and local issue time, location/counter **only if assigned**, short QR/code for receipt reference (no direct access secret), processing instruction and optional form where approved. Never print NIK/phone unless a service-specific justified template requires it. Printer failure after successful check-in **must not issue new ticket**: surface reprint same ticket via idempotent receipt endpoint and audit.

### 3.2 Operator desktop

```text
Select approved site/counter/shift → live waiting list
  → Call next (backend atomic) → show CALLED
  → Repeat / No show / Start serving
  → Complete (optional outcome code) → next ticket
```

Business controls require permission+counter assignment, confirm action feedback and keyboard shortcuts. UI disables while mutation in flight but backend prevents duplicate state. Supervisor override is an explicit audited flow, not a hidden button.

### 3.3 Voice announcer (mandatory in pilot)

```text
App starts in enrolled `announcer` role / capability -> verify zone+lease
  -> discover Indonesian (id-ID) voice + selected output/speaker
  -> subscribe job-ready SSE (no PII) + reconcile pending claim queue
  -> claim next zone job from Go API (private speech fields)
  -> sequential local/offline TTS: number + approved call name + counter
  -> STARTED / PLAYED / FAILED ack with fencing token
  -> report health, reconnect and never replay stale jobs blindly
```

**One announcer active per audio zone**; a desktop may run announcer capability together with operator mode only if explicitly enrolled with both, while the announcing **device** credential remains separate from the named staff session. Do not play announcements on every operator PC that receives the same event. Separate SPKT/SKCK, SATPAS, SAMSAT zones when physically distinct. Test volume, local Indonesian voice availability, PA speakers, health and safe fallback.

The operator gets `MANUAL_CALL_AUTO_VOICE` by default and an authorized `AUTO_ADVANCE_ON_READY` toggle/site-config if SOP permits; the latter means backend selects the next ticket when counter is actually READY. Complete/READY never implies customer already served. See `09_AUTOMATIC_QUEUE_VOICE.md` for data and UAT.

### 3.4 Display

Read sanitized `GET /api/v1/devices/displays/{id}/snapshot`, subscribe to `GET .../events`. Show only `ticket_display_number`, service name, counter display label, last called time, informational banners. No citizen name/PII. **Do not make television itself authenticated as a privileged user**; use limited display device credential.

Latency SLA target `p95 <= 3s` in healthy network (to measure at pilot). On event gap, reconnect with `Last-Event-ID`, always fetch snapshot for correctness. No public-facing indefinite staleness: if backend disconnected, show connection indicator/time of last update and locally configured instruction.

## 4. Device enrollment and security

1. Polres/site admin creates device registration record: tenant/site/mode/capability/allowed counter and generates **one-time** activation code, valid for short interval.
2. Technician installs signed desktop build, enters activation code via protected setup screen, device generates local keypair (if feasible).
3. Backend binds device to credential/certificate or rotating secret and limited permission profile, visible in device inventory.
4. Device heartbeat reports only status/version/network health and printer info; never full QR or citizen forms.
5. Device can be remotely revoked. Revocation must stop check-in, print of sensitive forms and SSE after expiry/reconnect. Re-enrollment requires admin approval.
6. Operator logins require individual staff OIDC session, inactivity logout and secure local token storage; kiosk device token alone cannot call staff operations.

**Critical:** no static shared API key in build, no hardcoded tenant ID, no bypass auth for display subscriptions, no saving citizen QR plaintext in local files.

## 5. Suggested Flutter desktop project structure

```text
lib/
  main.dart
  app/{bootstrap.dart,theme.dart,router.dart}
  core/{api,config,device_identity,printing,scanner,events,telemetry,errors}
  modes/
    kiosk/{screens,controllers,widgets}
    operator/{screens,controllers,widgets}
    display/{screens,controllers,widgets}
    announcer/{screens,controllers,services,widgets}
  shared/{models,services,widgets}
  generated/api/
windows/     # Windows packaging, installer and auto-update policy
linux/       # optional after compatibility test
integration_test/
```

Mode assignment originates from server-provisioned device record and is verified with backend, not a user-editable `--mode=operator` privilege flag. Admin can change mode only via authenticated device management; device must re-enroll when reassigned to another tenant/site.

## 6. Hardware abstraction interfaces

```dart
abstract interface class TicketPrinter {
  Future<PrintResult> printTicket(TicketPrintData data);
  Future<PrinterHealth> health();
}

abstract interface class QRReader {
  Stream<String> get scans;
}

abstract interface class VoiceAnnouncer {
  Future<List<VoiceInfo>> availableVoices();
  Future<void> speak(String text, {required String locale, required double volume});
  Future<void> stop();
  Future<SpeakerHealth> health();
}

abstract interface class QueueLiveClient {
  Stream<QueueEvent> connect(String scopeId, String? lastEventId);
  Future<QueueSnapshot> refresh();
}
```

Implement `WindowsSpoolPrinter`, optional `EscPosPrinter`, `UsbHidQrReader`, `CameraQrReader`, and `MockPrinter/MockReader` for automation. Create hardware certification per site model and record print DPI/paper width/margin/auto-cutter behavior.

## 7. Failure modes and offline policy

| Failure | Correct behavior | Forbidden behavior |
|---|---|---|
| Printer out of paper **after check-in success** | error + reprint **same ticket** after refill; print status audited | re-post checkin and generate new number |
| QR scanned twice | return existing ticket, warn reprint | generate another ticket |
| API unavailable during check-in | show temporarily unavailable + approved manual process | invent offline official ticket number |
| SSE disconnected | show stale banner; reconnect and snapshot | keep stale number as if real-time |
| Power loss during print | query ticket by original checkin/idempotency and reprint | blind retry of create-ticket |
| Operator twice calls next | server one state transition; second request idempotent/conflict | call 2 tickets unintentionally |
| Device moved to new site | require re-enrollment/re-authorize | reuse previous site credentials |
| Two desktop announcers subscribe to same site | server permits only one active lease + fencing per audio zone | both machines speak the same name/number |
| Missing id-ID voice or speaker failed | health alert + approved number-only/manual fallback; no silent cloud name upload | English-mispronounced privacy-sensitive names automatically played |
| Clock wrong | server timestamps, display warning | assume local time determines session state |
| Shift ends/logged out | lock operator mutation, keep kiosk/display independent | continue as previous operator |

**Offline-first is not a default feature**: independent kiosk ticket issuing risks duplicates, race and audit loss. If offline operation becomes contractual requirement, design *separate per-site edge coordinator and reconciliation protocol*, never ad hoc SQLite auto-increment across terminals.

## 8. Ticket / form print payload

Backend returns `ticket_print` **sanitized** e.g.:

```json
{
  "ticket_id":"UUID",
  "site_label":"SPKT/SKCK",
  "service_label":"SKCK",
  "display_number":"SKCK-025",
  "issued_at":"2026-10-24T03:00:00Z",
  "timezone":"Asia/Jakarta",
  "message":"Pantau nomor antrean pada layar display."
}
```

Use printer-specific templates **client-side for formatting only**; booking/ticket content originates backend. The form PDF module, if approved, fetches templated filled data via short-lived scoped endpoint and never persists it on kiosk longer than necessary.

## 9. Testing and field pilot

| ID | Scenario | Pass criteria |
|---|---|---|
| DESK-01 | Scan valid code | exactly 1 persisted ticket + 1 print job |
| DESK-02 | Scan same QR from 2 kiosks concurrently | same ticket; no duplicates; reprint tracked |
| DESK-03 | Spoof another tenant QR | denied without revealing content |
| DESK-04 | Printer unplugged/out of paper | actionable error + audited reprint, no double-ticket |
| DESK-05 | Two operators click call-next simultaneously | one ticket per authorized call, no double-served ticket |
| DESK-06 | Display SSE drop/reconnect | recovers correct snapshot without mixing site |
| DESK-07 | Revoked device | checkin/display denied and status flagged |
| DESK-08 | 8-hour continuous display / kiosk soak test | no progressive RAM leak or freeze within agreed budget |
| DESK-09 | OS restart auto launch | app starts correct mode, no credential leak on screen |
| DESK-10 | 58/80mm test pages | legible number/QR, correct cutter/layout with selected printer |
| DESK-11 | CALLED event with allowed name | private announcer reads approved name + number + counter, display gets number only |
| DESK-12 | two announcers on same zone | only one speaks; lease/fencing takeover logged |
| DESK-13 | counter `READY` automatic mode | next eligible checked-in ticket is called and announced; not when paused/closed |
| DESK-14 | public-name disallowed and OS voice absent | number-only/manual approved fallback, no PII leaks or queue corruption |

### Rollout strategy

1. Bench test printer & scanner model with native Windows build before ordering fleet hardware.
2. Lab integrate one virtual Polres/site and simulated operator/display/kiosk.
3. On-site deploy **one kiosk, one counter, one display** with approved queue staff supervising.
4. UAT during low-traffic hours then peak hour test with manual contingency.
5. Harden auto-start, remote monitoring and device inventories, only then replicate via configuration to SATPAS/SAMSAT and other Polres.

### Field-ready deliverables

- [ ] signed Windows installer, versioned app/update SOP and rollback package
- [ ] device setup wizard, activation guide and admin revoke procedure
- [ ] printer/scanner compatibility matrix and reproducible driver config
- [ ] operational runbook (paper out, internet out, stuck queue, admin override)
- [ ] reliability tests with physical hardware, telemetry, shift handover
- [ ] no PII on public display, no secrets in installer or local logs


## 10. First-pilot deployment contract (revised)

**Polres Batang is the pilot tenant but already belongs to configured Polda hierarchy**; device activation must resolve true `tenant_id/site_id/audio_zone_id` and refuse any reassignment to another Polres without re-enrollment. Pilot must include a certified speaker/audio announcer (dedicated PC or approved dual-role PC), physical loudspeaker self-test and shift runbook.

**Desktop readiness check at shift start:** `API healthy`, `device enrolled`, `assigned site+zone`, `speaker output present`, `id-ID voice installed or approved fallback`, `active zone lease`, `operator staffed`, `time sync`, `no undelivered stale job`. Make failure visible in both desktop and Polres admin dashboard, with Polda **aggregate device health only**.

**Source references:** [Flutter Windows](https://docs.flutter.dev/platform-integration/desktop), [Flutter platform channels](https://docs.flutter.dev/platform-integration/platform-channels), [Windows speech synthesis installed voices](https://learn.microsoft.com/en-us/uwp/api/windows.media.speechsynthesis.speechsynthesizer). If required native TTS cannot be verified in the certified Windows build, use a reviewed offline alternative with explicit licensing/testing, never an untested plugin in production.

## 11. Assignment perangkat awal Batang

Kiosk, komputer operator, TV display, dan komputer audio announcer harus didaftarkan terhadap `tenant=SIBATANG_BATANG`, lokasi fisik terkait, dan satu `audio_zone_id` khusus ruang tunggu. Aplikasi Flutter Desktop didistribusikan dengan mode berbeda (operator, kiosk, display, announcer) tetapi memuat UI branding SIBATANG di tenant Batang saja.

**Urutan suara otomatis:** operator klik `Ready` / selesai melayani, backend memvalidasi satu counter siap dan next ticket menunggu; membuat queue `CALLED` + announcement job dalam satu transaksi, desktop announcer lokal mengucapkan `nomor → nama (hanya bila diizinkan) → loket`, display update. Tidak boleh trigger dari jam janji booking saja. Speaker fail tidak menyebabkan booking/tiket rollback atau otomatis memanggil ticket selanjutnya tanpa kontrol.
