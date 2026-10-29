# 12 — API Contract v1: Endpoints, JSON dan OpenAPI Starter

> **Hanya Polda Jawa Tengah.** Pada produksi pilot, default tenant publik adalah `SIBATANG_BATANG`. Seluruh state antrean dan nomor dibuat backend Go; klien tidak menciptakan nomor sendiri.

## 1. Base contract

| Item | Ketentuan |
|---|---|
| Public/API origin | HTTPS reverse-proxy sesuai domain resmi yang disetujui |
| Base API | `/api/v1` |
| Format | JSON UTF-8, `snake_case`, UUID opaque, timestamps RFC3339 UTC |
| Error | `application/problem+json` (RFC 9457), message publik bebas PII |
| Citizen auth | public booking capability / OTP/account sesuai policy |
| Staff auth | OIDC + Go session/MFA; RBAC scope verified backend |
| Device auth | secret/certificate perangkat dengan aud/site/zone restricted |
| Writes | `Idempotency-Key` untuk booking/check-in/call/ack; audit all critical mutations |
| Live | SSE; reconnect memakai Last-Event-ID + authoritative snapshot |
| Rate limits | public per IP/token; staff per principal/scope; device per device |

Never trust `tenant_id`, `role`, `site_id` or `org_code` sent by arbitrary client. Resolve from approved server-side metadata and verify every resource. Nama/NIK tidak ada pada public display SSE, public metrics, Polda metrics, error logs.

## 2. API endpoints penting

| Actor | Method + path | Kegunaan |
|---|---|---|
| Public | `GET /public/tenants/sibatang-batang/catalog` | daftar site+service Batang yang published |
| Public | `GET /public/tenants/sibatang-batang/slots?service_id=&date=` | kuota & jadwal berdasarkan site/service |
| Citizen | `POST /public/bookings` | create booking with idempotency |
| Citizen | `GET /public/bookings/{reference}` | detail milik sendiri via authorization/capability |
| Citizen | `POST /public/bookings/{reference}/cancel` | cancel sesuai policy |
| Citizen | `GET /public/bookings/{reference}/status` | timeline tiket pribadi |
| Citizen | `GET /public/bookings/{reference}/events` | SSE private status |
| Kiosk | `POST /devices/{deviceId}/checkins` | verify QR + issue queue ticket atomic |
| Kiosk | `POST /devices/{deviceId}/tickets/{ticketId}/print-attempts` | audit/reprint nomor sama |
| Operator | `GET /staff/counters/{id}/queue` | list assigned counter/site only |
| Operator | `POST /staff/counters/{id}/ready` | set ready and optional auto-call |
| Operator | `POST /staff/counters/{id}/call-next` | manual call next |
| Operator | `POST /staff/tickets/{id}/recall` | recall audit + cooldown |
| Operator | `POST /staff/tickets/{id}/start` | called → serving |
| Operator | `POST /staff/tickets/{id}/complete` | serving → completed |
| Operator | `POST /staff/tickets/{id}/no-show` | called → no show per SOP |
| Display | `GET /devices/{deviceId}/display/snapshot` | sanitized queue/ticket/counter |
| Display | `GET /devices/{deviceId}/display/events` | SSE queue state |
| Announcer | `GET /devices/{deviceId}/announcer/events` | job availability, no name |
| Announcer | `POST /devices/{deviceId}/announcer/jobs/claim` | private claim scoped zone |
| Announcer | `POST /devices/{deviceId}/announcer/jobs/{jobId}/started` | start ack fencing |
| Announcer | `POST /devices/{deviceId}/announcer/jobs/{jobId}/completed` | playback ack fencing |
| Polda | `GET /polda/jateng/metrics` | aggregate Batang (dan Polres Jateng lain jika published) |
| Polda | `GET /polda/jateng/polres` | listing child Polres Jateng (status scoped) |
| Polda | `POST /polda/jateng/onboarding-requests` | ajukan Polres Jateng, draft only |
| Polda | `POST /polda/jateng/onboarding-requests/{id}/approve` | approval scoped, audited |
| Polres admin | `GET /staff/tenants/{id}/readiness` | readiness before publishing |
| Polres admin | `PATCH /staff/sites/{id}/queue-policy` | manual/auto advance policy |
| Polres admin | `PATCH /staff/sites/{id}/audio-zones/{zoneId}` | set policy nama/voice, audited |

Seluruh path relatif terhadap `/api/v1`. Routes/terms final harus dijadikan **satu OpenAPI source of truth**, bukan ditulis tangan berbeda di web/mobile/desktop. Beberapa endpoints `.../ready`, `.../publish` memerlukan owner/approval tambahan sesuai SOP.

## 3. Example: booking + QR

```http
POST /api/v1/public/bookings
Content-Type: application/json
Idempotency-Key: 58c7a149-4c61-40a2-a424-example
```

```json
{
  "tenant_code": "SIBATANG_BATANG",
  "slot_id": "11111111-1111-4111-8111-111111111111",
  "service_id": "22222222-2222-4222-8222-222222222222",
  "applicant": {
    "display_name": "Budi Santoso",
    "contact": "contoh-diisi-sesuai-kebijakan-layanan"
  },
  "privacy_notice_version": "2026-10-v1"
}
```

```json
{
  "data": {
    "booking_id": "33333333-3333-4333-8333-333333333333",
    "reference": "BTG-20261024-0026",
    "status": "CONFIRMED",
    "service": "SKCK",
    "site": "SITE_SPKT_SKCK",
    "service_date": "2026-10-24",
    "qr_token": "opaque-single-purpose-token-example",
    "qr_expires_at": "2026-10-24T10:00:00Z"
  }
}
```

**Contoh ilustratif, bukan jadwal nyata**. Jika `SLOT_FULL` return `409` tanpa booking ganda. QR secret hanya sekali di deliver melalui saluran aman, token hash di DB; jangan masuk query logs. `reference` bukan authorization credential tanpa secret/session.

## 4. Example: check-in and ticket

```json
{
  "qr_token": "opaque-single-purpose-token-example",
  "device_request_id": "terminal-01-20261024-0001"
}
```

```json
{
  "data": {
    "ticket_id": "44444444-4444-4444-8444-444444444444",
    "display_number": "SKCK-025",
    "status": "WAITING",
    "site": "SITE_SPKT_SKCK",
    "print_job_id": "55555555-5555-4555-8555-555555555555",
    "is_reprint": false
  }
}
```

Same QR + retry **returns the same ticket**, not a new number. Printer errors do not roll back issued tickets; `reprint` must be separately authorized and audited.

## 5. Example: operator ready auto-call

```json
{
  "counter_id": "66666666-6666-4666-8666-666666666666",
  "mode": "AUTO_ADVANCE_ON_READY",
  "readiness_revision": 19
}
```

```json
{
  "data": {
    "counter_state": "SERVING_PENDING",
    "ticket_id": "44444444-4444-4444-8444-444444444444",
    "display_number": "SKCK-025",
    "status": "CALLED",
    "call_event_id": "77777777-7777-4777-8777-777777777777",
    "announcement_status": "QUEUED"
  }
}
```

`SERVING_PENDING` is a **response view** if used by app, not necessarily enum on `counters`. Must be explicitly reconciled with domain state machine before generating clients. Backend owns next-ticket order, readiness conditions, fair queue policy and optimistic version.

## 6. Event schemas and access levels

### Display-only SSE (sanitized)

SSE wire format:

```text
id: 528
event: queue.ticket_called
data: {"ticket_number":"SKCK-025","counter_label":"Loket 2","service":"SKCK","revision":21}

```

Private announcer availability SSE:

```text
id: 529
event: announcement.job_available
data: {"zone_id":"88888888-8888-4888-8888-888888888888","job_id":"99999999-9999-4999-8999-999999999999"}

```

Private **claim** response contains `number_spoken`, `counter_spoken`, `public_call_name` only if site policy and explicit lawful basis permit. No citizen name in SSE payload, monitor Polda response, notification summary, public cached pages.

## 7. Example Polda dashboard data

```json
{
  "data": {
    "polda_code": "POLDA_JAWA_TENGAH",
    "as_of": "2026-10-24T04:32:00Z",
    "active_polres_count": 1,
    "polres": [
      {
        "tenant_code": "SIBATANG_BATANG",
        "brand_name": "SIBATANG",
        "booking_count": 50,
        "waiting_count": 9,
        "called_count": 2,
        "served_count": 31,
        "average_wait_seconds": 480,
        "devices_online": 4
      }
    ]
  },
  "meta": {"report_scope": "JATENG_DESCENDANTS", "is_fresh": true}
}
```

Numbers are **synthetic examples**, not actual statistics Batang. Production derives counts from database, and records `as_of`/freshness. No person-level drilldown.

## 8. Common errors

| HTTP | `type`/code | Behavior |
|---|---|---|
| 400 | `VALIDATION_ERROR` | invalid fields / date |
| 401 | `AUTH_REQUIRED` | redirect/re-auth for staff; never leak details |
| 403 | `SCOPE_DENIED` | tenant/site/counter/zone role denied |
| 404 | `NOT_FOUND` | unavailable or protected resource |
| 409 | `SLOT_FULL` / `STATE_CONFLICT` | resync authoritative state |
| 409 | `IDEMPOTENCY_KEY_REUSED` | new key for materially different body |
| 422 | `SERVICE_POLICY_NOT_MET` | service prerequisites invalid |
| 429 | `RATE_LIMITED` | bounded backoff |
| 503 | `ANNOUNCER_UNAVAILABLE` | only for administrative status; ticket state remains valid |

`503` announcer failure should not make a committed queue-call API pretend it was rolled back; status separate from call response.

## 9. Minimal OpenAPI snippet for implementation

```yaml
openapi: 3.1.0
info:
  title: SIBATANG Jateng Queue API
  version: 1.0.0
servers:
  - url: /api/v1
paths:
  /polda/jateng/metrics:
    get:
      operationId: getJatengMetrics
      security: [{staffSession: []}]
      responses:
        '200':
          description: Aggregate only, requires report.aggregate.read in POLDA_JAWA_TENGAH
  /public/bookings:
    post:
      operationId: createPublicBooking
      parameters:
        - in: header
          name: Idempotency-Key
          required: true
          schema: {type: string, minLength: 8}
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [tenant_code, slot_id, service_id, applicant]
              properties:
                tenant_code: {type: string, const: SIBATANG_BATANG}
                slot_id: {type: string, format: uuid}
                service_id: {type: string, format: uuid}
                applicant: {type: object}
      responses:
        '201': {description: Booking confirmed; opaque QR in private response}
        '409': {description: Slot full or idempotency conflict}
components:
  securitySchemes:
    staffSession: {type: apiKey, in: cookie, name: sib_session}
```

This fragment is **a starter**. Engineering task `API-001` is to author full `openapi.yaml`: every endpoint, schema, auth security, responses, SSE event types, device claim fencing and generated clients, and CI schema lint/breaking-change diff. Browser must protect cookie mutations via CSRF. The example `const` is limited to Batang public endpoint; future Polres Jateng require approved tenant discovery, not an open province picker.
