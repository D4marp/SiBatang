# AGENTS.md — SIBATANG Engineering Rules


> **KEPUTUSAN CAKUPAN v2 (mengikat):** Platform ini **hanya untuk Polda Jawa Tengah**. Hari pertama produksi hanya **Polres Batang (brand publik: SIBATANG)** sebagai tenant dan **dashboard pengawasan Polda Jawa Tengah**. Polres lain di Jawa Tengah boleh di-*onboard* kemudian, setelah otorisasi; **tidak membangun platform nasional, tidak menerima Polda lain, tidak menampilkan tenant fiktif di produksi**. Nama institusi pada paparan tertulis “Polresta Batang”, sedangkan arahan pemilik produk “Polres Batang”; nomenklatur/legal owner harus divalidasi sebelum seed produksi. Kode teknis awal menggunakan `POLRES_BATANG`.


This file can be copied to a repository root to guide coding agents or developers. **Follow referenced blueprints** in this package; update ADR and API contract before changing architecture.

## Project goal

Build a performant, secure, maintainable booking and queue platform for **Polres Batang under Polda Jawa Tengah**, with future onboarding restricted to other Polres **within Jawa Tengah only**. Clients: Next.js 16.x web, Go 1.27.x API, Flutter 3.47.x mobile/desktop. PostgreSQL 18 stores authoritative state. Start with modular monolith and REST+SSE. **Pilot SIBATANG Batang must ship together with Polda organization/login/dashboard and native desktop voice announcements.**

## Mandatory invariants

1. `tenant_id` is operational boundary, initially one Polres. `site_id` belongs to one tenant. Do not trust client tenant selectors for permission.
2. Polda organizational supervisors read explicitly authorized aggregated metrics about descendant Polres. No raw citizen details unless separate documented approved exceptional policy.
3. Role grant is insufficient without resource scope check. Enforce in backend and DB (RLS defense-in-depth), not only UI. All tenant-owned tables must be inventoried and isolation-tested.
4. Booking confirmation does **not** issue final queue number. Ticket is issued once after kiosk check-in; scanner/print retry must never issue another ticket.
5. Slot capacity, cancellation, check-in and call-next mutations must be atomic, idempotent where relevant and race-tested.
6. Do not invent offline official tickets on client. No raw NIK, phone, QR bearer, access token or form contents in display, aggregate reports, public events or logs.
7. Use one OpenAPI contract and generate TS/Dart clients; stable errors and enum semantics. API must remain compatible with installed desktop devices.
8. Never hardcode passwords/secrets/provider tokens, production citizen data, printer models or Polda/Polres official organizational mapping.
9. Every critical state transition writes audit/event transactionally and works after SSE disconnect.
10. Provide tests for authorized access, denied access, resource ownership, concurrency and negative cases whenever implementing a new feature.
11. **Polda Jateng from day one:** seed official reviewed Polda Jateng parent and Polres Batang org/tenant in first migration, provide Polda login+aggregate dashboard **before** Batang pilot, never publish fake Polres in production.
12. **Announcer:** a `CALLED` ticket atomically yields queue event+zone announcement job. Private announcer claim may see only policy-allowed `public_call_name`. Public display, Polda, queue SSE/outbox/logs cannot reveal name.
13. **No client-authoritative auto call:** `AUTO_ADVANCE_ON_READY` only via Go with active shift, assignment, eligible check-in, counter READY, no active ticket, DB-lock and audit. Booking slot wall clock is NOT an auto call signal.
14. **Audio resilience:** one leased active announcer per audio zone, sequential local Indonesian TTS/approved fallback, idempotent job/ack, no promise of physical exactly-once audio; alert failures.
15. **Name privacy:** default number-only until lawful policy/consent approved; never echo name into public screen, Polda supervisor API, shared debug logs or provider TTS without approved basis.

## Engineering defaults

- Keep dependencies minimal; do not add microservices, broker, Redis, Kubernetes or ORM without an evidence-backed ADR.
- Go: stdlib/chi, pgx/sqlc, composable modules and migrations; typed domain errors, request IDs, structured redacted logs.
- Web: Next App Router strict TypeScript; server-first, accessible mobile-first; secure HttpOnly session cookies and CSRF on cookie mutations.
- Flutter: feature-first Riverpod+go_router, secure storage and generated API client; distinct kiosk/operator/display/**announcer** modes with device-scoped auth and audio zone-scoped lease.
- Database: UTC timestamps plus IANA timezone and local service date; explicit tenant-aware constraints/indexes; runtime role not superuser/BYPASSRLS.
- Deploy: self-hosted/private approved environment first, TLS, restricted DB, tested encrypted backup and revocable site devices.

## Per-task checklist

- [ ] Requirement references slide # or approved new architecture decision.
- [ ] Role, scope, tenant/site/service and citizen ownership identified.
- [ ] Sensitive data classification and lifecycle decided.
- [ ] API contract/status/error/idempotency documented, generated clients refreshed.
- [ ] State machine transition and DB constraint considered.
- [ ] Tests include tenant A denied tenant B and normal successful path.
- [ ] Device/low-network/failure path considered when applicable.
- [ ] Docs, deployment and migration impact updated.

## Definition of Done

`go test ./... -race` green; API contract validation green; Next lint/typecheck/E2E green; Flutter analyze/test/build green; migrations pass fresh DB + baseline upgrade; no broken auth/PII leakage; reviewed by scope-owner; rollback documented.

**Reference docs:** `README.md`, `01_ARCHITECTURE_TENANCY_RBAC.md`, `02_WEB_NEXTJS.md`, `03_BACKEND_GOLANG.md`, `04_FLUTTER_MOBILE.md`, `05_FLUTTER_DESKTOP.md`, `06_API_DATA_CONTRACT.md`, `07_ROADMAP_TESTING_OPERATIONS.md`, `08_REQUIREMENTS_TRACEABILITY.md`, `09_AUTOMATIC_QUEUE_VOICE.md`, `10_POLDA_FIRST_PILOT.md`, `11_DATABASE_SCHEMA_SQL.md`, `12_API_OPENAPI_EXAMPLES.md`, `13_DEPLOYMENT_RUNBOOK.md`, `14_UAT_SECURITY_TEST_CASES.md`, `15_SPRINT_BACKLOG_AND_OWNERS.md`, `16_SCOPE_DECISIONS_REGISTER.md`.

## One-Polda-only rule

The deployment shall **only** serve `POLDA_JAWA_TENGAH` as organization root. Production pilot is **only** `POLRES_BATANG` (`SIBATANG_BATANG`). Never implement or scaffold national/multi-Polda dashboards, tenancy or provisioning. Future children must be Polres in Jateng with authorization, approval and isolation tests. The SIBATANG brand belongs only to Batang. See `10_POLDA_FIRST_PILOT.md` and `16_SCOPE_DECISIONS_REGISTER.md`.
