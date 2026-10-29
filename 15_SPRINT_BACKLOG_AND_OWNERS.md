# 15 — Development Backlog / Sprint, Dependency dan Team Assignment

> Pilot = SIBATANG Polres Batang. Polda Jawa Tengah dashboard and scoped roles are **P0 starting sprint 1**, not a post-pilot phase. Multi-tenant design exists from initial migration, but new real Jateng Polres are NOT activated during pilot.

## 1. Delivery model

- 2-week sprint; **16 weeks planning reference**, adjust based on actual staffing, vendor licenses, approval, physical device availability and acceptance.
- Definition of Ready: user role, service/site/tenant, acceptance, endpoint, schema impact, privacy impact, test owner, rollback and dependency recorded.
- Definition of Done: PR review, successful automated test suites, updated OpenAPI + generated client, tenant-scope tests, threat/PII review, demo to owner, deployed staging, evidence.
- Cross-team contract `12_API_OPENAPI_EXAMPLES.md` / `06_API_DATA_CONTRACT.md`; domain source of truth backend.

## 2. Eight sprints (W1–W16)

| Sprint | Go backend & DB | Next.js web | Flutter app | Flutter desktop | Security/QA/DevOps |
|---|---|---|---|---|---|
| S1 (W1–2) | org Jateng→Batang, SQL v001, Go skeleton/auth | shell SIBATANG + Polda shell | app skeleton, generated API | Windows runner, 4 modes skeleton | repo CI, infra staging, user/role decisions |
| S2 (W3–4) | RBAC memberships, RLS, site/service/slot model | login admin, Polda scoped login/tenant overview | service catalog Batang | device enrollment/health | tenant isolation synthetic Uji B, security baselines |
| S3 (W5–6) | booking idempotency/QR/slot locking, Polda aggregates | citizen flow, booking QR, Polda actual Batang metrics | booking/QR/status | kiosk QR scan/protocol mock | API generated types, concurrency tests |
| S4 (W7–8) | check-in atomic ticket, queue FSM, SSE/outbox | operator/admin scheduling, Polda site filters | live citizen queue | kiosk printer, display, operator workflow | scanner/printer field hardware lab |
| S5 (W9–10) | counter-ready/auto-call + announce job lease | audio policies/health, onboarding requests Jateng | push status optional baseline | TTS local id-ID, audio zones, failover | real speaker + zone race + privacy UAT |
| S6 (W11–12) | reports, audit, tenant onboarding approval | Polda reports, readiness and audit | beta Android, usability | reconnect, safe reprint, 8-hour soak | pilot rehearsal, performance/security review |
| S7 (W13–14) | stabilization and privacy retention | accessible UX, final Polda dashboard | app beta/UAT | installation/update SOP | Batang + Polda joint UAT, restore drill |
| S8 (W15–16) | deployment hardening | final rollout | release distribution if approved | canary Windows install | go-live gate, training, handover |

**No fake production tenants**. Uji B exists only in staging. Browser Polda metrics must be real by S3/S4, even if limited to Batang.

## 3. P0 Epic backlog with acceptance

| Ticket | Owner | Depends | Outcome / acceptance |
|---|---|---|---|
| ARCH-001 | Architect/DB | — | root `POLDA_JAWA_TENGAH` + Batang tenant; restrict non-Jateng parent |
| SEC-001 | Security+Go | ARCH-001 | RBAC roles, scope, MFA/staff session, device credentials, RLS tests |
| API-001 | Backend | ARCH-001 | single OpenAPI 3.1 spec and generated TS/Dart contracts |
| CAT-001 | Go + Web + Mobile | ARCH-001 | Batang 5-category service catalog, only approved site public |
| SLOT-001 | Go | CAT-001 | slot quotas, idempotent create, concurrent capacity test |
| BOOK-001 | Go+Web+Mobile | SLOT-001 | booking + QR, citizen status and cancellation rules |
| KIOSK-001 | Desktop + Go | BOOK-001 | QR scanner/check-in idempotent; printer exact ticket |
| QUEUE-001 | Go | KIOSK-001 | queue FSM, assigned counter, call, recall, no-show, audit |
| LIVE-001 | Go+Desktop+Web | QUEUE-001 | SSE reconnect/snapshot, display without PII |
| PDL-001 | Go+Web | ARCH-001/SEC-001 | Polda Jateng dashboard Batang true aggregates+freshness |
| PDL-002 | Go+Web | PDL-001 | Jateng-only Polres draft/onboarding/approve/readiness flow |
| VOI-001 | Go | QUEUE-001 | job/outbox/zone lease+fencing, no duplicate zone speaker |
| VOI-002 | Flutter desktop | VOI-001 | native offline TTS id-ID/approved fallback, volume and health |
| VOI-003 | Web + Desktop | VOI-001 | name policy/consent, automatic call on staffed READY |
| OPS-001 | DevOps | ARCH-001 | staging/prod secure deploy, backup restore and alert |
| UAT-001 | QA+Owners | all P0 | joint Batang/Polda acceptance and safe fallback |

## 4. P1/P2 (not blocking core pilot unless formally moved into scope)

- P1: richer digital forms and templates, printable official service form **if approved**, service SLA & no-show analytics, Android push, accessible help and service FAQ, expanded field operations.
- P2: authorized WhatsApp business/AI FAQ, external appointment/verification systems, integrated maps, iOS store release, advanced dashboards, performance tuning after observed traffic.
- **Out of scope:** system for other Polda, national Polri master portal, identity federation outside Jateng, official SIM/STNK/BPKB transaction integration without agreement, tax payment, unapproved certified e-signature claims.

## 5. Delivery ownership/RACI

| Artifact | Responsible | Accountable | Consulted |
|---|---|---|---|
| Root/tenant/RBAC | Backend/security | Tech lead | Polda & Polres owner |
| Booking & QR | Go/Web/App | Product owner | service owners |
| Kiosk+printer | Flutter desktop | Desktop lead | site manager |
| Voice/counter call | Go + Flutter desktop | Tech lead + ops owner | privacy officer |
| Dashboard Polda Jateng | Go + Next.js | Polda product representative | Polres reporting owner |
| Public content/services | Web + App | Batang service owner | communications |
| Infrastructure and release | DevOps | Platform operations | security |
| Test acceptance | QA | joint Batang + Polda sign-off | all teams |

## 6. Sprint demo requirement

Every demo uses the **same scenario** end-to-end with synthetic citizen: citizen books SKCK Batang → QR → kiosk check-in/print → operator ready → display & voice (with and without permitted name) → completed → dashboard Polda Jateng aggregate. In S1–S2 partial steps may be mocked in staging but **not counted as production-ready**. Track blocked integrations and signed policy decisions in `16_SCOPE_DECISIONS_REGISTER.md`.
