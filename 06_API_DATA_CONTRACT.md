# 06 — Shared Contract: API v1, Data, Events, Permissions


> **KEPUTUSAN CAKUPAN v2 (mengikat):** Platform ini **hanya untuk Polda Jawa Tengah**. Hari pertama produksi hanya **Polres Batang (brand publik: SIBATANG)** sebagai tenant dan **dashboard pengawasan Polda Jawa Tengah**. Polres lain di Jawa Tengah boleh di-*onboard* kemudian, setelah otorisasi; **tidak membangun platform nasional, tidak menerima Polda lain, tidak menampilkan tenant fiktif di produksi**. Nama institusi pada paparan tertulis “Polresta Batang”, sedangkan arahan pemilik produk “Polres Batang”; nomenklatur/legal owner harus divalidasi sebelum seed produksi. Kode teknis awal menggunakan `POLRES_BATANG`.


**Status:** proposed contract baseline; convert to `api/openapi.yaml` before coding and generate clients. Example IDs/values here are synthetic and have no relation to real residents. All endpoints use HTTPS. Web, mobile and desktop **must** use the same authoritative Go API.

## 1. Cross-client invariant

1. `tenant_id` represents Polres operational isolation; `site_id` is a location inside tenant; `service_id` is catalog service or per-tenant offering as described in endpoint schema.
2. Backend checks verified membership/resource ownership; **tenant sent by caller is not a permission**.
3. `booking_reference` = human-visible booking receipt; `ticket_display_number` = queue number generated **only after check-in**.
4. An idempotent retry returns the **same underlying booking/check-in/ticket**, never another capacity allocation.
5. Access to other citizen booking requires valid ownership session/capability; knowledge of UUID or code alone is insufficient.
6. Polda receives operational **aggregates** only unless exceptional policy explicitly authorizes more. **Polda hierarchy, login and its aggregate API are required during Batang pilot.**
7. UTC timestamps in RFC3339, site-local calendar dates as `YYYY-MM-DD`, timezone IANA e.g. `Asia/Jakarta`.
8. Queue `event_id` is monotonic in an event scope (not global time guarantee); SSE clients reconcile after gaps/reconnect.
9. `CALLED` produces separate private announcer job. Public SSE never carries spoken citizen names. Auto-next requires staffed `READY` counter and backend atomic transition; time slot is not a TTS timer.

## 2. API endpoint catalog

### 2.1 Public citizen endpoints

| HTTP | Endpoint | Auth | Purpose |
|---|---|---|---|
| GET | `/api/v1/public/tenants` | public | eligible Polres listings |
| GET | `/api/v1/public/tenants/{tenantId}/sites` | public | active physical sites |
| GET | `/api/v1/public/sites/{siteId}/services` | public | supported services + prerequisites |
| GET | `/api/v1/public/services/{tenantServiceId}/slots?date=...` | public | current available slots/capacity display |
| POST | `/api/v1/public/bookings` | public+abuse guard | reserve slot; **Idempotency-Key required** |
| POST | `/api/v1/public/bookings/redeem` | booking secret (not just reference) | redeem access secret for short-lived ownership session |
| GET | `/api/v1/me/bookings/{bookingId}` | ownership session | status/QR display metadata |
| POST | `/api/v1/me/bookings/{bookingId}/checkin-credential` | ownership session | mint/refresh QR scoped to check-in window |
| POST | `/api/v1/me/bookings/{bookingId}/cancel` | ownership session | cancel and release capacity if policy permits |
| POST | `/api/v1/me/bookings/{bookingId}/reschedule` | ownership session | atomic slot swap (phase 2 if complex) |
| GET | `/api/v1/me/bookings/{bookingId}/events` | ownership session | private SSE progress (no other booking) |
| GET | `/api/v1/public/info/{tenantServiceId}` | public | procedure, documents, accessibility and location |

**Booking secret design:** issue high entropy secret once, hash at rest; secret redeemed into short-lived HttpOnly cookie for web or stored in native secure store. QR checked at kiosk contains separate audience-bounded check-in token, not session bearer for viewing all booking details. Application should offer secure recovery flow if secret is lost; no guessing by booking reference and DOB.

### 2.2 Workforce/admin and organization

| HTTP | Endpoint | Permission + scope |
|---|---|---|
| GET | `/api/v1/auth/me` | authenticated staff; returns memberships and permissions for UI only |
| GET/POST/PATCH | `/api/v1/staff/tenants/{tenantId}/sites` | `site.config.*` own tenant |
| GET/POST/PATCH | `/api/v1/staff/tenants/{tenantId}/services` | `catalog.tenant_service.manage` own tenant |
| GET/POST/PATCH | `/api/v1/staff/tenants/{tenantId}/slots` | `schedule.manage` own tenant/site |
| GET/POST/PATCH | `/api/v1/staff/tenants/{tenantId}/counters` | `counter.manage` own tenant/site |
| GET/POST/PATCH | `/api/v1/staff/tenants/{tenantId}/memberships` | `tenant.member.manage` own tenant; never self-escalate |
| POST | `/api/v1/staff/tenants/{tenantId}/devices/activations` | `device.enroll` own tenant/site |
| GET | `/api/v1/staff/sites/{siteId}/queue` | `queue.ticket.read` scoped site/services |
| POST | `/api/v1/staff/counters/{counterId}/call-next` | `queue.ticket.call` assigned counter |
| POST | `/api/v1/staff/counters/{counterId}/ready` | `queue.counter.ready` assigned counter; may auto-advance if policy |
| POST | `/api/v1/staff/counters/{counterId}/pause` | `queue.counter.pause` assigned counter |
| GET/PATCH | `/api/v1/staff/sites/{siteId}/audio-zones` | `announcement.policy.manage` own site |
| GET/PATCH | `/api/v1/staff/sites/{siteId}/queue-policies` | `queue.policy.manage` own site |
| POST | `/api/v1/staff/tickets/{ticketId}/recall` | `queue.ticket.recall` assigned counter |
| POST | `/api/v1/staff/tickets/{ticketId}/start` | `queue.ticket.serve` assigned counter |
| POST | `/api/v1/staff/tickets/{ticketId}/complete` | `queue.ticket.complete` assigned counter |
| POST | `/api/v1/staff/tickets/{ticketId}/no-show` | `queue.ticket.no_show` assigned counter |
| GET | `/api/v1/staff/tenants/{tenantId}/reports` | `report.aggregate.read` own tenant |
| GET | `/api/v1/polda/jateng/metrics` | `report.aggregate.read` descendants only |
| GET | `/api/v1/polda/jateng/polres/{tenantId}/metrics` | verified descendant + aggregate only |
| GET | `/api/v1/polda/jateng/polres` | Polda org scope: view authorized Polres and onboarding status |
| POST | `/api/v1/polda/{orgId}/tenant-onboarding-requests` | `organization.child.provision`, approval-gated |
| POST | `/api/v1/platform/tenant-onboarding-requests/{requestId}/approve` | `organization.child.provision.approve` and authorized approver |

### 2.3 Registered devices

| HTTP | Endpoint | Device identity / authorization |
|---|---|---|
| POST | `/api/v1/devices/activate` | one-time signed activation code + device enrollment policy |
| POST | `/api/v1/devices/{deviceId}/heartbeat` | registered device only |
| POST | `/api/v1/devices/{deviceId}/checkins` | `DEVICE_KIOSK`, exact site+service capability |
| GET | `/api/v1/devices/{deviceId}/tickets/{ticketId}/receipt` | `DEVICE_KIOSK`, issued for same site, limited reprint policy |
| GET | `/api/v1/devices/{deviceId}/display/snapshot` | `DEVICE_DISPLAY`, own site |
| GET | `/api/v1/devices/{deviceId}/display/events` | `DEVICE_DISPLAY`, own site, SSE |
| GET | `/api/v1/devices/{deviceId}/announcer/events` | `DEVICE_ANNOUNCER`, exact site/zone, SSE job-ready metadata only |
| POST | `/api/v1/devices/{deviceId}/announcer/jobs/claim` | `DEVICE_ANNOUNCER`, zone active lease; returns private speech payload |
| POST | `/api/v1/devices/{deviceId}/announcer/jobs/{jobId}/started` | claim owner + fencing token |
| POST | `/api/v1/devices/{deviceId}/announcer/jobs/{jobId}/completed` | claim owner + fencing token; idempotent |
| POST | `/api/v1/devices/{deviceId}/announcer/jobs/{jobId}/failed` | claim owner + fencing token; idempotent |

Do **not** reuse universal API keys. OIDC native staff tokens and device identity are different authenticators.

## 3. Example public booking request/response

Request:

```http
POST /api/v1/public/bookings HTTP/1.1
Content-Type: application/json
Idempotency-Key: 018f6e6d-9c7f-7d6f-947c-fb4270e7992e
```

```json
{
  "tenant_id": "e01d59fb-5b96-40e7-86d6-30aef1db7201",
  "site_id": "fa307f10-e37a-4107-9c83-84bb4cb71800",
  "tenant_service_id": "164851da-03a8-4f5d-99dd-1610474e9c70",
  "slot_id": "0d7b9a39-1d20-48e8-aaf3-608c28f37211",
  "contact": {"name": "Pengunjung Contoh", "phone": "+6281230000000"},
  "form_values": {}
}
```

Response `201`: 

```json
{
  "data": {
    "id": "557cb030-38cd-4f14-b94a-07d063464a6a",
    "booking_reference": "BTG-20261024-X7R2",
    "status": "CONFIRMED",
    "site_timezone": "Asia/Jakarta",
    "slot_starts_at": "2026-10-24T02:00:00Z",
    "slot_ends_at": "2026-10-24T03:00:00Z",
    "site_name": "Lokasi Contoh SPKT/SKCK",
    "service_name": "SKCK",
    "ticket_display_number": null,
    "checkin_credential": {
      "qr_payload": "opaque-short-lived-checkin-token",
      "expires_at": "2026-10-24T03:30:00Z"
    }
  },
  "meta": {"request_id": "req_demo_001"}
}
```

**Security note:** for appointments made far in advance, the booking detail endpoint refreshes a short-lived check-in QR close to arrival time; client never treats an expired QR as a permanent booking cancellation. The QR can also be a high-entropy booking-scoped credential valid until approved check-in grace time if offline display of the QR is a confirmed user requirement. Choose one explicit policy and test it. Sample `qr_payload` illustrates shape only. In production, distinguish the QR capability from a public booking ownership secret and apply TTL/rotation/checkin policy. Do not persist or log QR value unredacted. For secure guest access, initial response must also establish a booking-owner session/secret via safe transport.

## 4. Example kiosk check-in

```http
POST /api/v1/devices/c62d4f1f-18d2-433d-bf11-2e51f696a771/checkins
Authorization: Bearer <short-lived-device-token>
Idempotency-Key: 2d7f70b2-1fe5-4e84-af0b-125c7c4b6b36
```

```json
{
  "qr_payload": "opaque-short-lived-checkin-token",
  "scanner_source": "USB_HID",
  "client_request_id": "kiosk-scanned-request-001"
}
```

Return `200`/`201` with ticket `id`, `ticket_display_number`, `status: WAITING`, `receipt`, and `reprint_allowed`. The first successful check-in creates a ticket; duplicate valid replay for the **same booking** returns the *same ticket* after scope checks. Race-safe database constraints and row locks are required.

## 5. Queue display snapshot and SSE schema

```json
{
  "data": {
    "tenant_id": "e01d59fb-5b96-40e7-86d6-30aef1db7201",
    "site_id": "fa307f10-e37a-4107-9c83-84bb4cb71800",
    "as_of": "2026-10-24T03:00:15Z",
    "last_event_id": "evt_site_0000001234",
    "called": [
      {"ticket_display_number":"SKCK-025", "service":"SKCK", "counter_label":"Loket 02", "called_at":"2026-10-24T03:00:10Z"}
    ],
    "waiting_count": 12,
    "connection_status": "LIVE"
  }
}
```

SSE example:

```text
id: evt_site_0000001235
event: queue.ticket_called
data: {"site_id":"fa307f10-e37a-4107-9c83-84bb4cb71800","ticket_display_number":"SKCK-026","counter_label":"Loket 02","occurred_at":"2026-10-24T03:01:02Z"}

: heartbeat
```

Do not send NIK/names/phone/booking owner secrets on public display stream. SSE event IDs are opaque offsets **scoped to the authorized stream**; re-connect using `Last-Event-ID` and always snapshot reconcile. `client_request_id` is observability metadata, not auth.

## 6. Standard error contract

Use Problem Details JSON (RFC 9457):

```json
{
  "type": "https://api.sibatang.example/problems/slot-full",
  "title": "Slot capacity reached",
  "status": 409,
  "code": "SLOT_FULL",
  "detail": "Jadwal sudah penuh, silakan pilih jadwal lain.",
  "request_id": "req_demo_002",
  "field_errors": []
}
```

`https://api.sibatang.example/...` is a **placeholder example domain**, not deployed infrastructure. Use canonical owned domain before production. Error response language consistent with `Accept-Language` if localized, but stable machine `code`.

## 7. Data lifecycle and statuses

| Entity | Allowed states | Critical transitions |
|---|---|---|
| Booking | `CONFIRMED`, `CANCELLED`, `EXPIRED`, `CHECKED_IN` | CONFIRMED→CHECKED_IN once; CONFIRMED→CANCELLED/EXPIRED (under policy) |
| Queue ticket | `WAITING`, `CALLED`, `SERVING`, `COMPLETED`, `NO_SHOW`, `CANCELLED` | WAITING→CALLED→SERVING→COMPLETED or CALLED→NO_SHOW |
| Device | `PENDING`, `ACTIVE`, `REVOKED`, `SUSPENDED` | ACTIVE→REVOKED blocks all subsequent action |
| User membership | `ACTIVE`, `SUSPENDED`, `EXPIRED` | scope changes and suspend audited |
| Slot | `DRAFT`, `OPEN`, `CLOSED`, `CANCELLED` | no reservation for non-OPEN |
| Notification | `PENDING`, `SENT`, `FAILED`, `SKIPPED` | notification failure does not roll back queue state |

Citizen-facing timeline derives from **booking + queue ticket**: booked → checked-in → waiting → called → serving → done. Do not copy ticket status into booking status fields indiscriminately.

## 8. Domain invariants in database/service

- `bookings.tenant_id == schedule_slots.tenant_id == site.tenant_id == tenant_service.tenant_id`.
- `queue_tickets.tenant_id == booking.tenant_id == checkin.tenant_id`, and site/service identical for ticket issuance.
- `ticket_serial` unique per `(tenant_id, site_id, service_id, local_service_date)` even under 100+ simultaneous scans.
- `booking` has <=1 successful check-in and <=1 active issued queue ticket in v1.
- `schedule_slots.reserved_count <= capacity` and never negative; reserve/release affected by policy and transaction.
- No cross-tenant foreign keys for normal operational entities even if UUID is known.
- Role membership changes invalidate affected sessions/caches as soon as practical with clearly tested maximum lag.
- `daily_service_metrics` aggregated source events count exact once (outbox dedupe), carry freshness watermark.

## 9. SQL RLS illustrative skeleton (not final migration)

```sql
-- Application login role must not own tables or have BYPASSRLS.
ALTER TABLE bookings ENABLE ROW LEVEL SECURITY;
ALTER TABLE bookings FORCE ROW LEVEL SECURITY;

CREATE POLICY bookings_tenant_isolation
  ON bookings
  FOR ALL TO sib_api
  USING (
    tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid
  )
  WITH CHECK (
    tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid
  );

-- Every request transaction:
BEGIN;
SELECT set_config('app.tenant_id', $1, true); -- $1 server-verified tenant UUID
-- tenant scoped queries here
COMMIT;
```

This is defense-in-depth only: RLS applies to runtime role, **not owner/superuser/BYPASSRLS**. Never use this setting as a substitute for resource-level permission. Validate all tenant-owned tables, views, functions, `INSERT/UPDATE` and cross-tenant reporting design before migration execution.

## 10. Reporting Polda contract

```http
GET /api/v1/polda/jateng/metrics?from=2026-10-01&to=2026-10-24&group_by=polres
```

```json
{
  "data": [
    {
      "tenant_id": "e01d59fb-5b96-40e7-86d6-30aef1db7201",
      "polres_label": "Polres Batang",
      "bookings": 420,
      "checked_in": 365,
      "served": 341,
      "no_show": 19,
      "avg_wait_seconds": 635,
      "p95_wait_seconds": 1440,
      "as_of": "2026-10-24T03:00:00Z"
    }
  ],
  "meta": {"report_scope":"POLDA_DESCENDANTS", "data_freshness_seconds": 45}
}
```

Numbers are **dummy examples**. Authorization first resolves org descendants then filters aggregate table to **only** that set. Policy forbids listing all tenants through unverified query param; no PII exports by default.

## 11. Contract-driven development checklist

1. Agree nomenclature IDs and statuses, ownership/permission/error contracts; review domain boundaries with all four client teams.
2. Write OpenAPI and example cases `happy`, `not allowed`, `wrong tenant`, `race conflict`, `expired QR`, `printer failure`.
3. Generate TS and Dart types; reject hand-maintained model divergence in CI.
4. Consumer contract tests with mock Go server before mobile and desktop hardware arrives.
5. Use additive API changes; `/v2` only for breaking changes with migration window and supported client matrix.
6. Version kiosk protocols carefully: enforce supported min client version, never break active site during operational hours without rollback.

**Supplemental schemas to author in actual OpenAPI:** `Tenant`, `Site`, `ServiceOffering`, `Prerequisite`, `ScheduleSlot`, `BookingCreate`, `Booking`, `Checkin`, `Ticket`, `QueueSnapshot`, `QueueEvent`, `Device`, `Permission`, `PoldaMetrics`, `ProblemDetails`, pagination and enums.


## 12. Voice + Polda-Jateng-from-day-one normative additions (MVP)

**Separate channels:**

```text
queue.ticket_called       -> sanitized display and operator SSE, no citizen name
announcement.job_available -> registered announcer SSE, job_id/zone_id only
POST announcer/jobs/claim -> private name where policy allows, active zone only
POST announcer/jobs/{id}/completed -> ack with fencing generation, no PII
```

**Auto-advance:** `POST /api/v1/staff/counters/{counterId}/ready` is idempotent per staff action; when policy `AUTO_ADVANCE_ON_READY`, Go atomically checks counter staff/shift+state and picks one eligible `WAITING` ticket. After `complete`, one new auto selection only if counter returns `READY`; no other booking-time/background desktop timer. `PAUSED` blocks auto-advance. `recall` creates a separately authorized/cooldown-controlled audio event.

**Speech claim response** (private only; includes `job_id`, `audio_zone_id`, `ticket_display_number`, `public_call_name` nullable, `counter_label`, `locale`, `lease_generation`, `lease_until`, `expires_at`, `template_id`). The server decides `public_call_name` from per-service policy and lawful basis; client does not send arbitrary name, and public display payload never includes it. Reference detailed synthetic sample in `09_AUTOMATIC_QUEUE_VOICE.md`.

**Polda-Jateng-from-day-one:** `GET /api/v1/polda/jateng/metrics` and `GET /api/v1/polda/jateng/polres` MUST be implemented and tested **before** Batang pilot UAT. New tenant onboarding request, approval, configure, publish is organization-scoped; Polda Jawa Tengah cannot create organizations beyond its Jateng boundary or read raw tickets. All APIs/OpenAPI expose explicit `scope` and `data_freshness_at` fields for dashboard. See `10_POLDA_FIRST_PILOT.md`.

**Tests mandatory:** `announcer` device of tenant A cannot claim tenant B or another zone; two devices cannot hold valid simultaneous zone leases; `NUMBER_ONLY` policy always yields `public_call_name=null`; Polda users cannot access `announcer/jobs/claim` nor any citizen name.

## 13. API scoping Jateng yang tidak boleh digeneralisasi menjadi multi-Polda

- Dashboard pengawasan: `GET /api/v1/polda/jateng/metrics`, `GET /api/v1/polda/jateng/polres`.
- Route `/api/v1/polda/{orgId}` dalam contoh lama dinyatakan **deprecated untuk blueprint v2**; server harus menolak org di luar verified `POLDA_JAWA_TENGAH` bahkan bila URL/Header dimanipulasi.
- Public discovery pilot: `GET /api/v1/public/tenants` hanya mengembalikan `SIBATANG_BATANG` (bila kebutuhan SEO/discovery), atau endpoint Batang langsung `GET /api/v1/public/tenants/sibatang-batang/catalog`.
- Polres Jateng kedua belum muncul di API publik sampai `status=PUBLISHED` setelah approval dan readiness.
- Kontrak perilaku autentikasi/staf/device tetap dirujuk dari `12_API_OPENAPI_EXAMPLES.md` dan `11_DATABASE_SCHEMA_SQL.md`.
