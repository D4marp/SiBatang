# 09 — Automatic Queue Calling & Indonesian Voice Announcement (Flutter Desktop + Go)


> **KEPUTUSAN CAKUPAN v2 (mengikat):** Platform ini **hanya untuk Polda Jawa Tengah**. Hari pertama produksi hanya **Polres Batang (brand publik: SIBATANG)** sebagai tenant dan **dashboard pengawasan Polda Jawa Tengah**. Polres lain di Jawa Tengah boleh di-*onboard* kemudian, setelah otorisasi; **tidak membangun platform nasional, tidak menerima Polda lain, tidak menampilkan tenant fiktif di produksi**. Nama institusi pada paparan tertulis “Polresta Batang”, sedangkan arahan pemilik produk “Polres Batang”; nomenklatur/legal owner harus divalidasi sebelum seed produksi. Kode teknis awal menggunakan `POLRES_BATANG`.


**Status:** wajib MVP pilot SIBATANG (perubahan kebutuhan 24 Oktober 2026).  
**Source of requirement:** permintaan lanjutan pengguna, **bukan** klaim bahwa paparan 9 slide sudah mendeskripsikan TTS nama. Paparan slide 4, 6, 7 menyebut antrean ditampilkan dan menuju loket, namun tidak mendefinisikan suara/nama otomatis.  
**Owners:** backend Go (authoritative queue), desktop Flutter (speaker), web Next.js (konfigurasi/monitor), QA+operasional (uji hardware & privasi).

## 1. Keputusan produk yang dikunci

1. Ketika tiket mendapat giliran pelayanan, Flutter Desktop **secara otomatis** membacakan **nomor antrean, nama panggilan yang diizinkan, jenis layanan, dan nomor loket** lewat speaker lokasi yang benar.
2. Pilihan **AUTO_ADVANCE_ON_READY** memungkinkan backend otomatis mengambil tiket berikutnya saat loket benar-benar `READY`, staf masih login/bertugas, layanan buka dan ada tiket eligible. Ini berbeda dari suara otomatis. Semua site punya `AUTO_ANNOUNCE_ON_CALL=true`; pemanggilan tiket otomatis dapat dinyalakan oleh site admin setelah UAT.
3. Jam booking **bukan** timer untuk membunyikan panggilan. Jadwal menetapkan kelayakan/check-in window. Panggilan ditentukan kondisi antrean dan kapasitas loket; jangan panggil orang yang belum check-in.
4. Satu **audio zone** = satu ruang tunggu/kumpulan speaker yang dapat mendengar pengumuman yang sama. **Satu announcer aktif per zone**, bisa PC announcer terpisah atau PC operator yang di-enroll dengan capability announcer. Bila ada 3 bangunan, konfigurasikan zone independen per lokasi; jangan broadcast seluruh Polres.
5. Pengumuman **berurutan** di setiap zone. Beberapa loket yang memanggil bersamaan menghasilkan jobs yang diserialkan, bukan dua TTS overlap.
6. Kiosk, operator, display, mobile dan Polda tidak mempunyai kewenangan menyusun nomor tiket. Semua status/call id dan audio job lahir dari transaksi Go.
7. **Suara nama adalah pengungkapan kepada publik.** Tampilkan/panggil `public_call_name` yang disetujui kebijakan per layanan dan disetujui warga sesuai aturan instansi. Jangan baca NIK, tanggal lahir, alamat, nomor telepon, isi laporan/perkara. Bila izin/kebijakan tidak ada, otomatis **nomor dan loket saja** (bukan membatalkan antrean).

## 2. Contoh ucapan dan aturan pelafalan

Jika policy `VOICE_NAME_ALLOWED` dan tersedia consent/specified lawful basis:

> "Nomor antrean S K C K, nol dua lima, atas nama Budi, silakan menuju loket dua."

Jika policy tidak mengizinkan nama:

> "Nomor antrean S K C K, nol dua lima, silakan menuju loket dua."

- Gunakan `display_number` yang tetap sama dengan tiket (mis. `SKCK-025`). Nomor tiga digit dibacakan digit per digit untuk mengurangi salah dengar (nol-dua-lima); pengejaan prefiks seperti `SKCK`, `SPKT`, `SIM` diatur dalam kamus pengucapan/SSML per site (jangan bergantung pada AI bebas).
- `public_call_name` maksimal panjang yang dikonfigurasi dan di-normalisasi; contoh **nama depan/panggilan** atau alias terpilih warga. Nama lengkap hanya bila memang diizinkan dan ada kebutuhan sah.
- Uji bahasa **id-ID** di Windows target. Nama dan akronim dapat salah baca; gunakan input preview audio dan fallback nomor saja. TTS harus menggunakan engine/voice yang lisensi dan dukungannya diverifikasi, **tanpa** ketergantungan cloud untuk tiap panggilan jika perangkat mampu offline.
- Template dibuat server-validated per tenant/site/service. Operator tidak boleh memasukkan teks bebas untuk diputar di speaker publik pada endpoint `call-next`.

## 3. Dua lapis otomasi (hindari istilah 'waktunya' ambigu)

| Mode | Trigger | Pemilik keputusan | Kapan digunakan |
|---|---|---|---|
| `MANUAL_CALL_AUTO_VOICE` | Operator menekan **Panggil berikutnya** | Go mengalokasikan tiket & membuat audio job | Default aman; tidak ada klik untuk membacakan setelah call |
| `AUTO_ADVANCE_ON_READY` | Operator siap/selesai, counter state `READY`, waiting ticket eligible | **Go** atomic claim tiket & buat audio job | Pilot dapat diaktifkan untuk loket dengan SOP yang setuju; worker bukan timer desktop |
| `RECALL` | Operator menekan **Ulangi panggilan** | Go buat event/job recall baru, rate-limited | Layanan sudah `CALLED`, tidak menerbitkan tiket baru |
| `PAUSE` | Staf istirahat, loket tutup, antrean bermasalah | Go pause counter | Tidak ada auto-advance saat `PAUSED/CLOSED/OFFLINE` |

**Counter states:** `CLOSED`, `READY`, `CALLING`, `SERVING`, `PAUSED`. Jangan jadikan koneksi desktop saja sebagai bukti loket siap. Aplikasi harus kirim state/assignment otentik dan backend memvalidasi shift+site+service. State `CALLING` berarti memanggil tiket (menunggu datang), bukan TTS pasti selesai.

**Guard sebelum auto-advance:** active staff assignment, counter `READY`, no active CALLED/SERVING ticket, site/service open, queue policy enabled, ticket `WAITING`, sudah check-in, `eligible_at <= now()`, tidak masuk status hold/prioritas yang dilarang. Operator bisa mem-pause, skip hanya melalui kebijakan no-show yang diaudit.

## 4. Alur end-to-end yang normatif

```mermaid
sequenceDiagram
    participant Citizen as Citizen/Kiosk
    participant Desk as Flutter Operator
    participant API as Go Queue API
    participant DB as PostgreSQL
    participant PA as Flutter Announcer
    participant Screen as Flutter Display

    Citizen->>API: check-in booking via QR
    API->>DB: BEGIN; create ticket WAITING + outbox; COMMIT
    Desk->>API: READY or complete previous service
    API->>DB: BEGIN; counter lock + select WAITING + CALL ticket
    API->>DB: insert queue_event + announcement_job; COMMIT
    API-->>Desk: ticket CALLED + counter
    API-->>Screen: SSE queue.ticket_called (number only)
    API-->>PA: SSE announcement.job_available (no citizen PII)
    PA->>API: claim next announcement for own zone
    API-->>PA: sanitized private speech payload + lease token
    PA->>PA: offline Indonesian TTS -> speaker
    PA->>API: STARTED then PLAYED/FAILED
    API->>DB: audit + delivery status
```

1. Queue transaction creates **one** `announcement_job` per `(call_event_id, audio_zone_id, revision)`; recall creates a **new** call event/revision rather than blindly replaying duplicate SSE.
2. Dispatcher sends only `job_id/zone_id`, not citizen name, in SSE. Announcer claims data via device-authenticated private endpoint.
3. Device lease keeps one active device per zone and uses fencing token/expiry; only current claimed device may acknowledge. Worker or API requeues stalled jobs within TTL; after deadline mark expired and require operator action.
4. Desktop checks job `tenant/site/zone` and `expires_at`; plays jobs sequentially; TTS volume/device routing configured per zone; sends started/played status.
5. Display updates from public sanitized stream independently of audio. **Audio must not block transaction, ticket state, or display.** If speaker/TTS fails, ticket remains `CALLED` with alert and manual recall option.
6. Reconnect via SSE `Last-Event-ID` + REST snapshot/jobs reconciliation. Never speak old events automatically on startup merely because an SSE message was replayed.

**Distributed systems caveat:** no software can guarantee physical exactly-once sound across abrupt crashes/disconnections. Delivery is at-least-once to the worker, with **best-effort duplicate suppression** and a single lease holder; all replay cases logged/tested. Avoid claim of perfect exactly-once loudspeaker playback.

## 5. Data model / migrations (SQL-oriented baseline)

```sql
-- All tenant-owned fields below MUST use tenant-aware FKs/RLS.
queue_audio_zones(
  id, tenant_id, site_id, name, status,
  active_device_id, lease_until, lease_generation,
  voice_locale, voice_id, speaker_device_id, volume,
  name_policy, announcement_template_version, created_at
)
queue_counter_policies(
  tenant_id, site_id, counter_id,
  auto_announce_on_call, auto_advance_on_ready,
  auto_call_delay_ms, stale_job_ttl_seconds,
  max_recall_per_ticket, updated_at
)
queue_announcements(
  id, tenant_id, site_id, audio_zone_id, ticket_id,
  call_event_id, kind, revision, state,
  created_at, expires_at, leased_by_device_id,
  lease_generation, lease_until, started_at, finished_at,
  playback_error_code, attempt_count
)
-- UNIQUE (tenant_id, audio_zone_id, call_event_id, revision)
-- INDEX  (tenant_id, audio_zone_id, state, created_at)
-- CHECK  (state IN ('PENDING','CLAIMED','PLAYING','PLAYED',
--                  'FAILED','CANCELLED','EXPIRED'))
```

Store sensitive citizen name **in existing protected booking/contact profile**, not duplicated in public event or announcement list. On announcer claim, resolve `public_call_name` from approved source with policy+consent enforced server-side. Do not store name in notification logs/metrics; redact HTTP tracing. Queue worker is scoped to tenant/site/service, and jobs must be consumed in zone order. Audio zone/site must be consistent through composite FK and tenancy RLS.

`queue_audio_zones.active_device_id` is only a hint unless guarded by a transaction-safe **lease+fencing generation**; single active announcer is enforced on the server. Multi-site audio zones cannot share the same announcer enrollment without explicit capability.

## 6. Contract endpoints and events

See `06_API_DATA_CONTRACT.md` for complete endpoint registry; minimal commands:

| Endpoint | Principal | Behavior |
|---|---|---|
| `POST /api/v1/staff/counters/{counterId}/ready` | operator assigned to counter | Marks ready; maybe atomic auto-advance if enabled |
| `POST /api/v1/staff/counters/{counterId}/pause` | operator/site manager | Stops auto-advance; does not invalidate existing ticket |
| `POST /api/v1/staff/counters/{counterId}/call-next` | assigned operator | Manual call, *always* audio job when allowed |
| `POST /api/v1/staff/tickets/{ticketId}/recall` | assigned operator | New recall job with rate limit |
| `GET/PATCH /api/v1/staff/sites/{siteId}/audio-zones` | site/polres admin | Setup zone, voice and privacy policy (audited) |
| `GET/PATCH /api/v1/staff/sites/{siteId}/queue-policies` | authorized site/polres admin | Enable/pause AUTO_ADVANCE_ON_READY |
| `GET /api/v1/devices/{deviceId}/announcer/events` | `DEVICE_ANNOUNCER` exact zone | SSE job-ready; no sensitive fields |
| `POST /api/v1/devices/{deviceId}/announcer/jobs/claim` | active registered announcer | Atomic claim next eligible job in own zone |
| `POST /api/v1/devices/{deviceId}/announcer/jobs/{jobId}/started` | claim owner + fencing token | Records start ack |
| `POST /api/v1/devices/{deviceId}/announcer/jobs/{jobId}/completed` | claim owner + fencing token | Records PLAYED ack |
| `POST /api/v1/devices/{deviceId}/announcer/jobs/{jobId}/failed` | claim owner + fencing token | Mark error and retry/alert per policy |

`POST .../claim` response **private** (synthetic sample):

```json
{
  "data": {
    "job_id": "2d59271b-9b42-41a3-b789-8581cda6f4d2",
    "call_event_id": "evt_239",
    "tenant_id": "tenant_batang_example",
    "site_id": "site_spkt_example",
    "zone_id": "zone_waiting_spkt_example",
    "fencing_token": 42,
    "expires_at": "2026-10-24T03:15:00Z",
    "locale": "id-ID",
    "speech": {
      "service_spoken": "S K C K",
      "number_spoken": "nol dua lima",
      "public_call_name": "Budi",
      "counter_spoken": "loket dua",
      "template_id": "id-ID-v1"
    }
  }
}
```

**Important:** names appear only in private claim to `DEVICE_ANNOUNCER`, never in `queue.ticket_called` display SSE, Polda metrics, analytics, audit, push logs or general queue list. If `name_policy=NUMBER_ONLY` or consent absent, return `"public_call_name":null` and use number-only announcement. Validate response schemas and device credentials; sample IDs are illustrative placeholders.

Example SSE announce availability:

```text
id: evt_site_239
event: announcement.job_available
data: {"audio_zone_id":"zone_waiting_spkt_example","job_id":"2d59271b-9b42-41a3-b789-8581cda6f4d2"}
```

## 7. Flutter Desktop architecture

```text
lib/modes/announcer/
  screens/announcer_health_screen.dart    # status zone/voice/speaker/job
  controllers/announcer_controller.dart    # SSE + reconnect + polling fallback
  services/announcement_work_queue.dart  # local FIFO, max one playing
  services/voice_renderer.dart            # template and pronunciation only
  services/speaker_health_service.dart    # volume/output device/test
windows/runner or windows/plugins/tts/     # native engine through platform channel
```

```dart
abstract interface class VoiceAnnouncer {
  Future<List<VoiceInfo>> availableVoices();
  Future<void> speak(String text, {required String locale, required double volume});
  Future<void> stop();
  Future<SpeakerHealth> health();
}
```

- **Windows-first:** implement native platform channel for installed Windows TTS (e.g. Windows speech synthesis APIs) and audio output. Flutter supports custom Windows platform channels. Verify supported voices and **Indonesian locale id-ID installed locally** before enabling name calls; otherwise fail readiness and fall back to an approved number-only prerecorded template or human call (according to approved SOP). Do **not** promise Indonesian voice by default on every Windows image.
- Keep TTS **offline** where possible; privacy-sensitive names must not be silently sent to public cloud TTS. Optional offline engine must pass licensing, package size and pronunciation certification.
- Audio health: selectable output device, speaker self-test, volume, queue length, mute/pause, last ack time, last error, 8h soak. `Test voice` uses synthetic name/number, never current citizen data.
- On `QUEUED` job: fetch/claim; validate lease, state and freshness; format localized utterance; play serially; ack; renew or release zone lease as specified. Handle app suspend/system sleep, Windows audio output change, speaker unplug, restart.
- Operators can see `speaker_online`, `announcement_pending/played/failed`; **must not** have access to other sites or change name without policy. Sound wave icon/UI indicates audio health, not proof the user heard it.

## 8. Privacy, security and roles

- Add role `DEVICE_ANNOUNCER` with only `announcement.claim`, `announcement.ack`, `device.health.write` to exact `(tenant_id, site_id, audio_zone_id, device_id)`; it **cannot** call-next, enumerate bookings, get NIK, access another zone or edit templates.
- `POLRES_ADMIN`/`SITE_MANAGER` have `announcement.policy.manage` scoped site; `POLDA_MONITOR` may view anonymized **device online/error aggregate** only.
- Name policy examples: `NUMBER_ONLY` (strict default until approved), `PREFERRED_NAME_IF_CONSENT`, `FULL_NAME_IF_APPROVED`. Per-service restrictions override site-wide permissive setting.
- Server derives spoken name from policy consent and booking ownership; never use arbitrary `name` from SSE/client. Never log plaintext TTS sentences with names; encrypt restricted stored PII where mandated; set retention.
- Document privacy notice and consent/other lawful basis with service owner; loudspeaker privacy cannot be 'fixed' solely with RBAC because bystanders hear it.

## 9. Resilience and safety matrix

| Failure | Required behavior |
|---|---|
| Operator calls 3 numbers nearly simultaneously | Backend serializes per zone; caller/display updates, announcer speaks sequentially |
| Two announcer PCs connected in same zone | One valid active lease/fencing token; only one allowed to claim; standby passive |
| Announcer offline but API works | Display/app still updates; UI alerts, operator can call via approved manual SOP |
| API disconnect while audio playing | Finish local job if still valid, best-effort ack on reconnect; no blind replay of previous jobs |
| TTS unavailable or id-ID voice absent | Readiness fails; approved fallback (number-only prerecorded/human), not arbitrary English voice for Indonesian name |
| Called ticket moves to SERVING before announcement starts | Policy decides cancel expired announcement; do not announce already-completed/invalid tickets |
| Worker restart or SSE message duplicates | Unique call event/zone/revision, claim lease and idempotent acks |
| Operator hits recall rapidly | Backend max recall count + cooldown; new audited recall job only |
| Volume zero, speaker unplugged | Health red/alert before opening queue; display remains operational |
| Citizen does not want public name | `NUMBER_ONLY` announcement with ticket+service+loket |

## 10. Non-functional budgets and UAT (targets to validate, not guarantees)

- **Audio dispatch latency:** goal `p95 <= 3 seconds` from committed call event until **speech start** for healthy connected/local voice, excluding queued prior announcements; measure from server timestamps and playback ack.
- **Zone serialization:** never overlap playback in same zone under load of 4 concurrent counters.
- **Reliability:** 100% queue call actions produce durable announcement job or explicit failed state; no lost job silently; queue state independent of audio failure.
- **Hardware:** speaker output, id-ID voice, OS audio permission, kiosk/PA power, printer and scanner tested on actual Windows machine. 8-hour soak.
- **Privacy:** `NUMBER_ONLY` policy never leaks `public_call_name` in announcer job or speaker; restricted name never appears in public SSE/display/Polda report/logs.
- **RBAC:** same-zone standby cannot double claim; wrong-tenant/wrong-zone device gets deny; revoked device stops claiming.
- **Recovery:** restart/reconnect with pending+playing jobs yields bounded, logged retry; no replay of days-old call.

### Acceptance scenarios (must pass)

- [ ] `VOICE-01`: check-in Budi + ticket `SKCK-025`; call next; speaker reads `SKCK 025 ... Budi ... Loket 2` if policy/consent allow.
- [ ] `VOICE-02`: privacy setting number-only removes name from both claim payload and utterance.
- [ ] `VOICE-03`: three operators call simultaneously; jobs serialized per audio zone.
- [ ] `VOICE-04`: speaker disconnected or TTS missing → alert & safe fallback, backend ticket remains correct.
- [ ] `VOICE-05`: two announcers connected → one active speaker device.
- [ ] `VOICE-06`: recall creates one **new** authorized job, cooldown enforced.
- [ ] `VOICE-07`: auto-advance fires only when counter `READY` and staffed; not when `PAUSED`, `SERVING`, `CLOSED`.
- [ ] `VOICE-08`: Polda Jateng monitor, Polres uji B, public display and other-site devices cannot retrieve citizen call name.
- [ ] `VOICE-09`: after restart, no stale or already-served number announced automatically.
- [ ] `VOICE-10`: SPKT/SKCK, SATPAS and SAMSAT have separate zone routing.

## 11. Build order / ready-for-dev tasks

1. `VOICE-BE-01` migrations: audio zone, queue policy, announcements + constraints/RLS.
2. `VOICE-BE-02` staff counter READY/PAUSED + `AUTO_ADVANCE_ON_READY` transaction with row locks and tests.
3. `VOICE-BE-03` announcement outbox+claim lease+fencing+ack, event stream private, cancellation/TTL.
4. `VOICE-WIN-01` Flutter Windows `VoiceAnnouncer` native plugin and installed voice discovery/speaker self-test.
5. `VOICE-WIN-02` announcer screen, foreground listener/poll reconciliation, queue+dedupe and telemetry.
6. `VOICE-WEB-01` web admin audio-zone, privacy policy, manual/automatic mode, online/offline status.
7. `VOICE-QA-01` UAT actual site: 2 operators + 2 potential announcers + 1 zone + speaker, then separate sites.

**Related:** `03_BACKEND_GOLANG.md`, `05_FLUTTER_DESKTOP.md`, `06_API_DATA_CONTRACT.md`, `10_POLDA_FIRST_PILOT.md`.

**Official feasibility references:** [Flutter Windows desktop/platform channels](https://docs.flutter.dev/platform-integration/platform-channels), [Windows installed-voice speech synthesizer](https://learn.microsoft.com/en-us/uwp/api/windows.media.speechsynthesis.speechsynthesizer). Verify API/runtime compatibility with the selected Windows image before committing a plugin/voice engine.

## 12. Keputusan versi Batang v2

Semua pengumuman dari pilot berlabel `SIBATANG_BATANG` pada lokasi Polres Batang yang telah disahkan. Satu announcer aktif per ruang tunggu (audio zone) menerima event dari Go, sedangkan **Polda Jawa Tengah hanya memonitor jumlah error/online/offline audio tanpa nama atau kalimat speech**. Di masa depan announcer tenant Polres lain di Jawa Tengah menggunakan pola sama dengan konfigurasi brand/site terpisah. Nama tidak boleh otomatis terdengar sebelum penanggung jawab layanan menyetujui dasar dan kebijakan publikasinya.
