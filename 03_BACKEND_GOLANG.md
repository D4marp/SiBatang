# 03 — Implementation Plan: Backend Golang


> **KEPUTUSAN CAKUPAN v2 (mengikat):** Platform ini **hanya untuk Polda Jawa Tengah**. Hari pertama produksi hanya **Polres Batang (brand publik: SIBATANG)** sebagai tenant dan **dashboard pengawasan Polda Jawa Tengah**. Polres lain di Jawa Tengah boleh di-*onboard* kemudian, setelah otorisasi; **tidak membangun platform nasional, tidak menerima Polda lain, tidak menampilkan tenant fiktif di produksi**. Nama institusi pada paparan tertulis “Polresta Batang”, sedangkan arahan pemilik produk “Polres Batang”; nomenklatur/legal owner harus divalidasi sebelum seed produksi. Kode teknis awal menggunakan `POLRES_BATANG`.


**Owner:** Backend + platform team. Go is the system of record for scheduling, ticket issuance, authorization, device enrollment, event delivery and audit.

## 1. Tech stack konservatif

| Domain | Choice v1 | Justification |
|---|---|---|
| Go | **1.27.x** patch latest verified and locked | current stable Oct 2026; small memory footprint and good concurrency |
| HTTP | `net/http` + `chi` or stdlib `ServeMux` (choose one) | minimal dependencies |
| PostgreSQL | **18** stable patch | ACID, unique constraints, RLS, reporting SQL |
| DB access | `pgx/v5`, `sqlc`, `goose` or `golang-migrate` (choose ONE migrator) | type-safe SQL and predictable queries |
| Logging | `log/slog` JSON with redaction | no heavy logging stack |
| Auth | OIDC identity-provider for workforce, Go-managed web session; PKCE where native sign-in needed | open standards, MFA delegated |
| Live | Server-Sent Events, durable `queue_events`/outbox + DB `LISTEN/NOTIFY` as wake-up signal | simple deployment, event recovery |
| Jobs | independent worker process (same repo/codebase) polling outbox SKIP LOCKED | robust retries without broker initial |
| API spec | OpenAPI 3.1 with generated TS/Dart clients | stable contracts, no ad hoc shapes |
| Test | `go test`, `testcontainers-go`/docker integration, k6/Vegeta load tests | verify transaction races and RLS |

**No early microservices, Kafka, Redis, GraphQL or Kubernetes as mandatory dependency.** Introduce on measured workload or mandates, not speculation.

## 2. Modules and ownership

```text
cmd/api/main.go                   # HTTP server and graceful shutdown
cmd/worker/main.go                # outbox/notification/report jobs
internal/
  auth/                            # OIDC/session/service identity
  organization/                    # hierarchy & supervisor permissions
  tenancy/                         # verified tenant context and RLS
  catalog/                         # global services + per site offerings
  scheduling/                      # slots, capacities, closures
  booking/                         # public flow, cancellation/reschedule
  checkin/                         # scan, ticket issuance, identity policy
  queue/                           # call/recall/serve/complete/no-show
  device/                          # enrollment, revocation, health
  notification/                    # channel adaptors, no business authority
  announcement/                    # audio zones/jobs/claim/ack/lease/privacy policy
  onboarding/                      # Polda approval-gated Polres provisioning
  reporting/                       # metrics/freshness, Polda rollups
  audit/                           # append-only actor/action log
  transport/http/                  # handlers, auth, errors, validation
  transport/sse/
  persistence/                     # pgx/sqlc repositories
migrations/
api/openapi.yaml
pkg/                               # only truly reusable small libs
```

**Dependency direction:** transports → application service → domain/repositories. No domain code imports HTTP or Flutter/Next types. Functional packages no cross-module DB transaction behind the service boundary without explicit orchestration.

## 3. Database domain v1

- `organizations`, `org_ancestors`, `tenants`, `sites`, `service_catalog`, `tenant_services`, `counters`, `opening_hours`, `special_closures`
- `schedule_slots` (`tenant_id`, `site_id`, `tenant_service_id`, `local_service_date`, `starts_at_utc`, `ends_at_utc`, `capacity`, `reserved_count`, `version`)
- `citizen_contacts` (if retained; PII encrypted where applicable, data-minimized), `bookings` (`id`, `tenant_id`, `site_id`, `service_id`, `slot_id`, `booking_reference`, `origin`, `status`, `created_at`, `cancelled_at`)
- `booking_secrets` (hash of QR bearer/capability secret, rotation/version/expiry; never store plaintext public QR token)
- `checkins`, `ticket_sequences`, `queue_tickets`, `queue_events`, `counter_assignments`, `counter_queue_policies`, `queue_audio_zones`, `queue_announcements`
- `digital_form_templates`, `digital_form_submissions` (service-specific, consent/retention; signature status distinguishes *captured scribble* vs qualified signature)
- `devices`, `device_credentials`, `notifications`, `outbox_events`, `idempotency_records`
- `users`, `roles`, `permissions`, `role_permissions`, `user_memberships`, `audit_events`, `tenant_onboarding_requests`, `org_change_audit`
- `daily_service_metrics`, `hourly_queue_metrics` for reporting Polda, no PII.

All tenant-owned tables have `tenant_id NOT NULL` and composite consistency indexes. FKs should include tenant when linking to tenant-owned entities; use DB constraints for unique tickets, booking secrets, and capacity.

### Required unique constraints/indexes

```sql
UNIQUE (tenant_id, site_id, tenant_service_id, local_service_date, ticket_serial)
UNIQUE (tenant_id, booking_reference)
UNIQUE (tenant_id, booking_id) -- on checkins / issued queue ticket
UNIQUE (tenant_id, device_id)
INDEX  (tenant_id, site_id, status, created_at DESC)
INDEX  (tenant_id, tenant_service_id, local_service_date, status)
INDEX  (org_ancestor_id, org_descendant_id)
```

DB design stores time stamps as `timestamptz` UTC; `local_service_date` and timezone from site decide reset schedule/ticket serial. Never reset global counter at server midnight UTC.

## 4. Transaction algorithms (pseudocode normative)

### 4.1 Reserve appointment

```text
POST /api/v1/public/bookings (Idempotency-Key header)
validate input, public service/tenant active; enforce abuse controls
BEGIN
  SELECT schedule_slot FOR UPDATE (scoped tenant/site/service)
  if reserved_count >= capacity: return 409 SLOT_FULL
  validate slot not past, service rules and time policy
  INSERT booking(status=CONFIRMED, random opaque reference)
  UPDATE slot SET reserved_count = reserved_count + 1
  INSERT outbox event BookingConfirmed
  INSERT idempotency result(request hash, booking id) ON CONFLICT ...
COMMIT
return 201 booking with link/QR capability handled via secure redemption
```

**Important:** idempotency verification must be concurrency-safe. Lock / unique insert idempotency key before slot increment; same key+same request returns prior successful response; same key+different body returns `409 IDEMPOTENCY_CONFLICT`. Apply all writes in **one** DB transaction, so no partial key/reservation. Booking cancellation decrements capacity at most once if policy allows; concurrent cancel and check-in serialize on booking row lock.

### 4.2 Check-in / issue queue ticket

```text
POST /api/v1/devices/{deviceId}/checkins (scan booking QR)
verify rotating device credentials + device tenant/site + QR secret
BEGIN
  SELECT booking FOR UPDATE (verified site + valid slot/time/status)
  if already checked-in: return SAME existing ticket (idempotent replay)
  validate not cancelled/expired, service active, check-in window
  allocate ticket serial by UPSERT/UPDATE ticket_sequences ... RETURNING serial
  INSERT checkin(verified_at, device_id)
  INSERT queue_ticket(status=WAITING, serial, display_no=service_prefix-serial)
  UPDATE booking(status=CHECKED_IN)
  INSERT queue event + outbox event
COMMIT
return ticket print payload (never PII unless necessary and authorized)
```

**No browser or kiosk-side number generation**. Unique constraints guarantee no duplicates. `tenant_id`, site/date/service tuple is part of lock/sequence. Duplicate QR scanning returns existing ticket and may print **reprint** with explicit audit/policy.

### 4.3 Atomic call next

```text
POST /api/v1/staff/counters/{counterId}/call-next
validate actor membership in tenant+site and assigned counter/service
BEGIN
  SELECT counter FOR UPDATE
  SELECT one waiting ticket FOR UPDATE SKIP LOCKED
    WHERE tenant/site/service in counter assignments
    ORDER BY priority_policy, checked_in_at, serial
  if none: 204 / domain empty state
  UPDATE ticket status=CALLED, counter_id=counterId, called_at=now
  INSERT queue event + outbox event
  INSERT queue_announcement(job=PENDING, scoped zone, unique call-event)
COMMIT
return called ticket + new status; announcer playback ASYNC (not blocking call)
```

Priority policy for disability/elderly, appointment vs walk-in, and fairness must be **approved by service owner** (not implicitly hardcoded). Every call, recall, no-show, serve, complete, override is a separate permissioned, audited transition. Default `call-next` cannot consume a ticket from another tenant, site or disallowed service.

### 4.4 Outbox and event delivery

- Every state-changing transaction writes an outbox event and `queue_events` row in the same commit.
- Background worker polls with `FOR UPDATE SKIP LOCKED`, sends push/WhatsApp asynchronously with retry+backoff, idempotent provider message key and DLQ state for manual inspection.
- API SSE uses persisted event ID (`id:` and `Last-Event-ID`), heartbeats, short-lived authorization checks/revalidation, not just fire-and-forget `NOTIFY`. On reconnect clients call current state GET before/after replay.
- SSE subjects `tenant/site/service/counter` scoped and sanitized; private citizen channel returns own data with capability/session.
- Never trust event payload to send notification outside originally verified tenant; worker restores verified context from durable payload and re-checks policy/provider binding.

## 5. Auth and authorization endpoint patterns

### Staff web

1. Go `/auth/staff/login` redirects to institution-approved OIDC IdP (state, nonce, PKCE when applicable).
2. Callback validates issuer/audience/state/code/nonce and maps `sub` to staff user and memberships.
3. Go issues short-lived server-side session referenced by secure HttpOnly SameSite cookie; rotate on login, revoke on offboarding.
4. Browser and Next server call `/api/v1/*` same-origin. CSRF check applies to cookie-auth mutating requests.

### Flutter operator native

Use OIDC Authorization Code + PKCE via system browser where appropriate, token stored in OS secure storage, refresh handling and device binding. Never install shared staff login in publicly accessible kiosk; kiosks use limited device identity.

### Public booking

Default proposal: allow limited guest booking with phone verification policy-based. Rate-limit and use high-entropy per-booking redemption secret (hash at rest) with expiry; a predictable booking reference is **not authorization**. Citizen account optional by policy. QR token uses one-time/bounded check-in semantics; do not embed NIK, HP or signed arbitrary JSON with long expiry.

### Scoped authorization

```go
// Conceptual authorization only — all DB access must use verified transaction context.
if !policy.Allow(actor, "queue.ticket.call", Scope{TenantID: tenantID, SiteID: siteID, CounterID: counterID}) {
    return ErrForbidden
}
```

RLS: connection uses non-owner no-BYPASSRLS. For each business transaction set verified tenant via transaction-local PostgreSQL config; use `FORCE ROW LEVEL SECURITY` where required. Polda aggregate cross-tenant read handled via a separately authorized, audited repository/read model—not a public `SET app.tenant_id='*'` shortcut.

## 6. API contract conventions

- Base: `/api/v1`; date/time in RFC3339 UTC plus `site_timezone`; IDs UUID opaque; client never infers tenant from ID format.
- `GET` idempotent, cursor pagination; `POST` mutation includes `Idempotency-Key` when repeat could charge capacity/change state.
- Standard response `data`, `meta`, optionally `links`; error uses `application/problem+json` with `type`, `title`, `status`, `code`, `detail`, `request_id`, `field_errors`.
- Error set: `400 VALIDATION_ERROR`, `401 UNAUTHENTICATED`, `403 FORBIDDEN`, `404 NOT_FOUND`, `409 SLOT_FULL|STATE_CONFLICT|IDEMPOTENCY_CONFLICT`, `410 BOOKING_EXPIRED`, `422 BUSINESS_RULE_FAILED`, `429 RATE_LIMITED`, `503 TEMPORARILY_UNAVAILABLE`.
- No unsafe `PUT` for conditional state transition without ETag/`If-Match` or server locks. Apply optimistic `version` when updating schedules, config or forms.
- HTTP rate limits, response body size caps, per-device clock skew tolerance + server time as source of truth.

## 7. DB migration and recovery

- Numbered migrations forward and down (down only when safe); destructive columns via expand→backfill→switch→contract; no manual production schema changes.
- Transaction-safe migrations and locked migration runner; db schema compatibility for rolling deployment (old/new APIs for at least one release).
- Encrypted automated backups, point-in-time recovery where infrastructure supports, restore drill, document RPO/RTO, retention by classification.
- Tenant offboarding via export/archive/deletion procedure plus backup expiration compliance, not `DELETE FROM tenants CASCADE`.
- Seed initial catalog `SPKT_KEHILANGAN`, `SKCK`, `SIM`, `STNK`, `BPKB` global, but all site/service mappings configured through tenant onboarding.

## 8. Test plan & CI gates

1. **Unit**: booking rules, event transitions, capacity, policy checker, time zone change and display number formatting.
2. **DB integration**: real Postgres 18, migration, RLS, composite tenancy FK, duplicate concurrent booking/checkin/call-next.
3. **Authorization**: every endpoint table-driven allow/deny across admin platform Jateng, Polda Jateng, Polres Batang/Uji B (staging), site A/B, kiosk A, citizen A/B.
4. **Security**: tampered tenant/header/ID, replay QR, duplicated idempotency, expired session, SSRF in notification settings, upload validation.
5. **Load**: agreed booking peak, 50+ simultaneous kiosk scans in test, 100+ concurrent call-next requests in synthetic burst; reject/serialize correctly; target response KPIs under measured infrastructure.
6. **Resiliency**: disconnect SSE, offline worker, retry provider, database fail/recover, replay outbox, revoked kiosk credential.
7. **QA**: OpenAPI diff classified breaking/nonbreaking and generated clients compile for TS & Dart.

```bash
go test ./... -race
go vet ./...
# run integration with a real test PostgreSQL and checked migrations
```

## 9. Implementation order / milestones

| Order | Deliverable | Blocker |
|---|---|---|
| 1 | API skeleton, OpenAPI, RFC7807 problems, request_id, health, migrations | none |
| 2 | organizations, tenants, site services, RBAC, OIDC/session, RLS tests | required before app business tables |
| 3 | catalog/slots/booking/idempotency | WEB-PUB booking |
| 4 | QR redemption, check-in, ticket allocator, printer payload | Desktop kiosk |
| 5 | queue transitions, counter assignments, SSE+outbox | Desktop operator + display |
| 6 | device provisioning and telemetry | Kiosk production |
| 7 | metrics materialization, Polda aggregates, exports | **Polda dashboard operational by pilot**, implement in parallel with core weeks 5–10 |
| 8 | notification providers, digital form templates, geo, optional WA | after core is stable |

### Definition of Done (backend)

- [ ] New tenant via migration/seed does not require branching the codebase.
- [ ] 100% domain mutation scope-checked and audited as classified.
- [ ] Database cross-tenant denial tests pass for every tenant-owned table.
- [ ] Booking capacity never becomes negative/overallocated; ticket serial unique with concurrent writes.
- [ ] Durable outbox replay recovers after worker restart; SSE live state reconciles.
- [ ] OpenAPI committed, server implementation + TS/Dart clients pass contract tests.
- [ ] Health/readiness/liveness, encrypted backup and restoration proven.

**Handoff:** source Go service+worker, OpenAPI 3.1, SQL migrations, seeded demo multi-tenant dataset (synthetic PII only), Docker Compose dev environment, CI and operation runbook.


## 10. Mandatory addition: backend-driven auto-call, audio jobs & Polda-Jateng-from-day-one

`AUTO_ANNOUNCE_ON_CALL` = output TTS triggered by `CALLED` event. `AUTO_ADVANCE_ON_READY` = **server-authoritative** ticket selection if counter `READY` with active staff assignment/shift, valid service, eligible checked-in waiting ticket, no current active call/serve. Both are independently configurable by authorized site admin. Do not use desktop clocks to decide business queue transitions.

Backend operations to implement and cover with transaction tests:

```text
CounterReady(assignedOperator, counterId)
  BEGIN
    lock counter, check verified tenant/site/role/shift
    if no active ticket AND auto_advance_enabled AND counter.is_ready:
      select one eligible WAITING ticket FOR UPDATE SKIP LOCKED
      update ticket status CALLED; update counter state CALLING
      insert queue event + outbox + zone announcement job
    else store READY state only
  COMMIT

CallNext(manualOrSystemActor, counterId)
  same serialized counter/ticket transaction and idempotency rules

ClaimVoiceJob(announcerDevice, audioZone)
  BEGIN; verify device exact tenant/site/zone and active zone lease
    SELECT oldest valid PENDING announcement FOR UPDATE SKIP LOCKED
    assign lease generation, state CLAIMED, expiry and owner
  COMMIT; return minimally necessary private name/number/counter fields

AckVoiceJob(device, job, fencingToken, status)
  compare server lease owner/generation; idempotent ack + metrics + audit
```

Auto-advance is triggered by `READY` and by successful `COMPLETE` that returns counter to READY under policy; **never** fire for `PAUSED`, `SERVING`, closed shift or not-yet-eligible booking. Consider short site-configured cooldown after completion using durable scheduled job (`run_after` in DB), not an in-memory timer, with re-authorization just before advancing. `no-show` is still human/policy-governed; no automatic citizen penalty on mere audio failure.

**Important database constraints:** `UNIQUE(tenant_id,audio_zone_id,call_event_id,revision)`, indexed job state and expiry, zone active-speaker lease+monotonic fencing generation, tenant-aware FKs/RLS; queue events and announcements committed atomically. `public_call_name` is resolved server-side on private announcer claim **only with policy/consent**, not stored in public SSE/outbox/metrics. Public display and Polda responses always omit name. Delivery/PA audio is best effort with idempotent claim/ack and logged failures, not mathematically exactly-once physical sound.

**Polda-Jateng-from-day-one foundation:** week 3–4 schema + Polda root+Batang org+tenant seed, delegated `POLDA_ORG_ADMIN`, Polda login, RLS; week 5–7 metrics pipeline + onboarding requests and dashboard; week 8–10 operational Polda dashboard **before** field pilot. New tenant creation approved/idempotent via organization transaction, publication gated until devices/services/UAT valid. Cross-tenant test fixtures only staging, not production fictitious Polres.

**Integration tests:** READY + 2 operators select single ticket once; rapid CALLED events serialize jobs per zone; duplicate SSE produces one claim; stale lease fencing denies unauthorized ack; wrong zone/tenant cannot fetch speech name; Polda cannot query announcement payload or raw citizen records; unplug speaker does not roll back queue state. See `09_AUTOMATIC_QUEUE_VOICE.md` and `10_POLDA_FIRST_PILOT.md`.

## 11. Single-region restriction (hard invariant v2)

`organization.root_code = POLDA_JAWA_TENGAH` adalah satu-satunya root authorized untuk operasi organisasi. Semua tenant produksi memiliki owner organisasi **Polres di bawah Jateng** dan `timezone=Asia/Jakarta`. Pada pilot, hanya `tenant_code=SIBATANG_BATANG` yang boleh `PUBLISHED`. Admin Polda dapat membuat request Polres Jateng lain, tetapi tidak ada endpoint untuk `create_polda` ataupun multi-Polda federation. `SIBATANG` merupakan brand metadata Batang, bukan global product name untuk semua tenant.

Polda reporting repository harus join `org_ancestors`/hierarchy scope sebelum query agregat. Runtime DB role untuk tenant tidak boleh menggunakan superuser/BYPASSRLS. Jika model `PLATFORM_ADMIN` ada untuk DevOps, jangan menganggap ia pejabat Polda atau memberi akses otomatis ke PII.
