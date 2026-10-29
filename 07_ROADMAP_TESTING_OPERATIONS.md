# 07 — Delivery Roadmap, Test Plan, Deployment, UAT and Operations


> **KEPUTUSAN CAKUPAN v2 (mengikat):** Platform ini **hanya untuk Polda Jawa Tengah**. Hari pertama produksi hanya **Polres Batang (brand publik: SIBATANG)** sebagai tenant dan **dashboard pengawasan Polda Jawa Tengah**. Polres lain di Jawa Tengah boleh di-*onboard* kemudian, setelah otorisasi; **tidak membangun platform nasional, tidak menerima Polda lain, tidak menampilkan tenant fiktif di produksi**. Nama institusi pada paparan tertulis “Polresta Batang”, sedangkan arahan pemilik produk “Polres Batang”; nomenklatur/legal owner harus divalidasi sebelum seed produksi. Kode teknis awal menggunakan `POLRES_BATANG`.


**Status:** proposed delivery calendar, not a contractual timeline. Start dates depend on sign-off, hardware, network, security and identity integration. Base plan: **16 minggu** sampai scale trial; **Polda foundation dan dashboard aktif sebelum pilot SIBATANG Batang (minggu 11–12 dalam rehearsal/UAT; go-live minggu 15–16 setelah semua gate**, voice auto-announcement juga fitur MVP.

## 1. Program goals / outcomes

- Pengunjung dapat booking dari web/Android, mendapat QR, check-in dan tiket tanpa penghitungan nomor di klien.
- Unit SPKT/SKCK, SATPAS SIM, SAMSAT STNK/BPKB memiliki site/config terpisah dalam satu tenant Polres (subject to authority check).
- Petugas atau mode auto-ready (yang sudah disetujui) memanggil antrean sesuai unit/loket; **Flutter Desktop mengumumkan nomor antrean + nama yang diizinkan melalui audio zone**. Display menunjukkan status live tanpa PII.
- Operator Polres memantau, mengubah kuota/jam, mengelola petugas dan devices sesuai roles.
- Polda Jateng melihat ringkasan Batang saat pilot dan nantinya perbandingan Polres lain dalam Jawa Tengah yang berizin **tanpa** otomasi pemberian akses detail warga.
- Tenancy isolation, race safety, backups, restore, staff SSO, per-device credentials, audit dan failure procedures lulus acceptance tests.

## 2. Phased 16-week plan

| Phase | Week | Main output | Exit criteria |
|---|---|---|---|
| **P0 Discovery** | 1–2 | site survey, workflow SOP, slot & no-show policy, hardware list, data/privacy classification, authorization matrix, UX sitemap | approved scope, device models, owner signs policies |
| **P1 Platform foundation** | 3–4 | repo/CI/CD, OIDC, **seed Polda + pilot Batang**, org/tenant/site, RBAC/RLS, **Polda login+tenant list/dashboard shell**, OpenAPI, audio zone schema | tenant leak test 0 cases, Polda test access green |
| **P2 Core booking** | 5–7 | catalog, slots, web booking, API idempotency, mobile booking UI, QR ownership, **Polda aggregate worker+functional dashboard and onboarding requests** | no oversell, Polda Jateng only approved child Polres metrics |
| **P3 On-site queue** | 8–10 | kiosk+printer, operator state/auto-ready, display SSE, **registered announcer + Indonesian TTS and speaker tests**, audit/device, **Polda dashboard ready** | full journey with speaker on real hardware; no double voice/call |
| **P4 Rehearsal pilot Batang + Polda** | 11–12 | field installation/closed beta Batang SPKT/SKCK, speaker+announcer, Polda oversight operational | integrated rehearsal, site UAT defects tracked |
| **P5 Hardening pilot** | 13–14 | fix QA/security, approved SATPAS/SAMSAT config if permitted, training/backup and Polda dashboard accuracy | critical bugs resolved, approvals recorded |
| **P6 Batang go-live, Jateng readiness** | 15–16 | go-live Batang after sign-off, Polda monitoring, synthetic Jateng tenant Uji B on staging for onboarding test | production still only Batang; other Jateng Polres onboarded in a later authorized project |

**Contingency:** if procurement/identity approvals delayed, prioritize a web+API synthetic demo to avoid blocking software. Do not call phase done until gates pass. WhatsApp AI, iOS, official service-system integrations can be a separate funded phase after hardening.

## 3. Parallel workstreams

| Workstream | W1–2 | W3–4 | W5–7 | W8–10 | W11–12 | W13–16 |
|---|---|---|---|---|---|---|
| Product/UX | requirements | wireframe | usability | kiosk workflows | UAT | SOP expansion |
| Backend Go | domain model | auth/RBAC/RLS + Polda org | booking + Polda aggregate/onboarding | queue/SSE/device/voice | hardening | Jateng reporting hardening |
| Web Next | prototypes | skeleton/auth + Polda shell | public booking + Polda dashboard | Polres admin + audio policies | Polda+Batang pilot report | comparison expansion |
| Flutter mobile | flows | base/app shell | book/QR | queue progress | Android beta | store readiness |
| Flutter desktop | speaker/TTS+printer test | base/device enrollment | kiosk/announcer mock | print/operator/display/voice | on-site voice UAT | multi-site rollout |
| DevOps/Security | threat model | pipeline/backup | API tests | secrets/hardening | pen test/restore | scale/DR |

## 4. Backlog with priorities

| ID | Priority | Epic | Acceptance slice |
|---|---|---|---|
| SYS-001 | P0 | Organizational hierarchy | seed Polda Jateng + Polres Batang production; Polres Uji B only staging; descendants query correct |
| SYS-002 | P0 | Tenant isolation + RBAC | deny by default; table-driven cross-tenant tests pass |
| SYS-003 | P0 | Staff OIDC/MFA + sessions | individual role, revoke, no shared admin |
| SYS-004 | P0 | Site/service catalog | configurable five services, multiple sites |
| SYS-005 | P0 | Scheduling capacities | book/cancel under concurrent load; no oversell |
| SYS-006 | P0 | Public web booking | working full confirmation/ownership/QR |
| SYS-007 | P0 | Kiosk check-in and print | real printer, duplicate QR returns same ticket |
| SYS-008 | P0 | Operator queues | call/recall/serve/done/no-show permissions |
| SYS-009 | P0 | Display live | PII-free snapshot/SSE/reconnect |
| SYS-010 | P0 | Device enrollment/revocation | scope-limited credentials, heartbeat |
| SYS-011 | P0 | Structured audit and outbox | all sensitive transitions and provider retries durable |
| SYS-012 | P0 | Backup/recovery | encrypted automated backup and restore validated |
| SYS-013 | P1 | Flutter Android app | book, QR, status, locations, helpful errors |
| SYS-014 | P1 | Polres reporting | queue throughput, service duration, no show |
| SYS-015 | **P0** | **Polda dashboard must be usable before Batang UAT** | descendants only, aggregate-only, freshness indicator; week 5–10 delivery |
| SYS-021 | **P0** | Voice announcer zone and Windows id-ID TTS | number + policy-allowed name + counter, speaker health, exactly one active zone lease |
| SYS-022 | **P0** | Auto-advance on staffed counter READY (configurable) | never on paused/unassigned/closed, duplicate-resistant, auditable |
| SYS-023 | **P0** | Polda-Jateng-from-day-one onboarding workflow | Polda login/admin scopes, approved onboarding, publish gate, no real fictitious Polres |
| SYS-016 | P1 | Service digital form | template versioning and retention approved |
| SYS-017 | P1 | Public accessibility & WCAG | keyboard/reader checks and mobile quality |
| SYS-018 | P2 | Push/WhatsApp official | opt-in/approval, retries, no raw PII |
| SYS-019 | P2 | WhatsApp AI FAQ | source-grounded FAQ, human handoff, safe content |
| SYS-020 | P2 | iOS release and advanced analytics | after device/legal/ops approval |

## 5. Definition of Ready for engineering ticket

Every ticket has: problem/user role, requirement source (slide or approved new), tenant/scope, payload and error states, UX acceptance, data classification, dependencies, test cases, effort owner, audit events and fallback behavior. Ticket with unknown service legal basis must not implement PII collection by guesswork.

## 6. Critical UAT scripts (must be executed by actual workflow owners)

### E2E citizen success

1. Citizen opens the SIBATANG Batang public page, selects SKCK site, sees official docs, selects open slot.
2. Citizen fills minimum fields, submits; booking created exactly once and receives QR.
3. At the Batang Kiosk, QR accepted; exactly one `SKCK-###` ticket allocated; printer prints legibly.
4. Operator calls next **or** backend auto-advances on staffed READY counter; **Flutter announcer reads number + approved public name + counter via correct audio zone**; display updates without citizen names, and citizen app reflects called.
5. Operator starts and completes service; booking/ticket/audit/report states correct.
6. Dashboard Polres shows counts; Polda Jateng sees Batang aggregate after aggregation, not citizen name.

### Cross-tenant negative

1. Create Polres Batang + Polres Uji B (sintetis) di bawah Polda Jawa Tengah.
2. Staff A changes path/header to tenant B → denied.
3. Kiosk A scans code booking B → denied, *zero check-in*.
4. Display A connects to B stream → denied and no leaked event.
5. Polda Jawa Tengah views Batang + Polres Uji B synthetic aggregates within staging; never sees citizen QR/details.
6. Export and cached responses must be isolated by tenant and supervisor descendant scope.

### Concurrency/edge

- 100 parallel requests for 10 available slots → exactly 10 bookings, rest `409` (load model verify DB).
- Two same-key retries → one booking and stable response; different body same key → conflict.
- Two kiosk devices scan same QR → 1 queue ticket, repeated print rules followed.
- Two operators call next while ticket count 1 → at most 1 caller obtains it.
- Kiosk printer out of paper after backend issue → retry reprint same number.
- SSE disconnect 60 sec then reconnect → view correct from snapshot/event replay.
- Network/site power failures → site uses approved manual queue fallback and reconciliation procedure; never silently forge ticket.
- Shift crossing midnight in WIB and UTC boundaries → ticket date and reset follow site timezone.

## 7. Deployment layouts

### Pilot minimum (illustrative only)

```text
                Internet/approved network
                         |
                   Firewall/TLS proxy
                    /           \
        Next.js web app       Go API + SSE
                                  |
                           PostgreSQL 18
                                  |
                        encrypted backups

Per site: Windows kiosk + USB printer/scanner
          Windows operator PC(s)
          Windows display PC to TV
```

**Sizing must follow measured load**. Initial lab/dev illustration: 2–4 vCPU/4–8GB RAM per app VM and DB 4 vCPU/8–16GB as a **starting experiment only**, SSD storage and backup capacity based on retention. Not an SLA or confirmed hardware spec. For production Jateng Polres scale, separate DB, app, backups, monitoring, multi-instance and network segmentation based on architecture review.

**Network/security:** private DB subnet, TLS edge only, kiosk VLAN segmented, restrict outbound to API, VPN for management, remote support time-bound, firewall whitelist, NTP sync, endpoint hardening, device antivirus/EDR per institution standard. Prefer domain/subdomain owned by instansi and secure DNS. No public PostgreSQL port.

### CI/CD lanes

- `PR`: format/lint/typecheck, unit, static checks, Go race, API schema diff, dependency vulnerabilities/secrets scan.
- `integration`: fresh PostgreSQL migration + RLS negative cases, mock printer/scanner, golden contract tests.
- `staging`: deployed like production with **synthetic data**, Playwright/Flutter integration, k6 load, accessibility tests.
- `prod`: signed artifact promotion, DB compatibility checks, health check, canary one site, rollback playbook, approvals.

Protected branches, code review, pin container images by digest for production, SBOM, changelog and rollback release tag. Do not copy production PII to staging.

## 8. Observability baseline

| Metric | Warning / alert idea | Triage |
|---|---|---|
| HTTP `p95`, 5xx | SLO breach, elevated error rate | app/API + DB |
| slot reservation conflicts | sudden high 409 vs historical | adjust capacity / UX |
| check-in latency | prolonged > agreed objective | DB locking, network, scanner |
| active kiosk devices | device heartbeats missing > 5min | network/endpoint |
| print failures | 3 consecutive failures | printer/driver/roll paper |
| event delivery lag | outbox oldest age > threshold | worker/provider status |
| dashboard data freshness | Polda read model > 5min stale | aggregation worker |
| backup age / restore | backup missing or restore drill failed | storage/DB ops |
| audit/security deny | spike cross-tenant denials | SOC/investigate |

Alert thresholds must be tuned after baseline site telemetry and on-call rota agreed. Data reporting freshness must be visible; avoid claiming true real-time for asynchronously aggregated Polda metrics.

## 9. Field operational runbook outline

- **Start of shift**: login personal account, assign counter, test printer/QR, ensure device green, verify correct site branding.
- **During shift**: queue call/serve/complete; escalation when incorrect ticket and no-show; cannot manually alter ticket number.
- **Outage printer**: reprint existing persisted ticket; administrator may switch to approved standby printer.
- **Outage API**: show unavailable; paper-based fallback authorized by service manager; later reconciliation supervised and flagged manual (no silent client-generated IDs).
- **End of shift**: logout operator, inspect pending queues and unfinished visits, daily report and audit summary.
- **Rollback**: revert application to prior signed version only if DB schema backward-compatible, otherwise follow tested migration/forward-fix playbook.
- **Backup/restore**: encryption key availability, isolated restore test, replay outbox, cross-tenant validation after recovery.
- **Incident**: isolate compromised kiosk, revoke identity, investigate audit, notify according to approved incident response policy.

## 10. RACI starter

| Activity | Product owner | Tech lead | Backend | Web | Mobile | Desktop | Security/DevOps | Site operational owner |
|---|---|---|---|---|---|---|---|---|
| Queue/business rules | A | C | C | I | I | C | I | R |
| Tenant / Polda policies | A | R | R | C | I | I | C | C |
| API & schema | C | A | R | C | C | C | C | I |
| Printer certification | I | A | C | I | I | R | C | R |
| Privacy/legal approval | A | C | C | C | C | C | R | C |
| UAT and go-live | A | R | C | C | C | C | R | R |

R = Responsible, A = Accountable, C = Consulted, I = Informed. Assign actual owner names before execution. Revisit policy-owner responsibilities for SAMSAT/SATPAS.

## 11. Go-live gate (no compromise)

- [ ] Signed scope, service owners, data handling, privacy notice, Polda oversight policy.
- [ ] Tested auth/roles, zero unauthorized cross-tenant read/write, MFA for admins.
- [ ] Full E2E booking/kiosk/queue/display tested on target site with actual hardware.
- [ ] Real printer/scanner certification, power/network fallback and staff training.
- [ ] Approved incident response, backup/restore, on-call and logs/monitoring.
- [ ] Performance and resilience tests accepted by service owner with measurable results.
- [ ] Licensed/supported dependencies, security pen-test report and remediated blocking issues.
- [ ] Rollout and rollback plan signed; scheduled downtime communications prepared.

## 12. Key risks and mitigations

| Risk | Severity | Mitigation |
|---|---|---|
| Unauthorized Polres/Polda data access | Critical | scoped auth + RLS + cross-tenant integration tests |
| Duplicate queue tickets / overbooking | Critical | DB constraints + locking + idempotency |
| Printing depends on device drivers | High | early hardware lab / compatibility matrix / fallback |
| Multiple legal entities at SAMSAT | High | validate controller, integration permissions and agreements |
| WhatsApp integration policy/billing | Medium–High | phase 2, approved channel, no core flow dependency |
| Raw NIK/PII exposure | Critical | minimization, encryption, limited retention, privileged audit |
| Intermittent local network | High | reconnect, snapshot, manual SOP, NTP, monitor |
| Schedule changes invalidate bookings | High | versioned schedule, admin policy, notify affected visitors |
| Polda aggregation inaccurate/stale | High | durable event aggregation, reconciliation jobs, freshness stamps |
| Scope growth (AI/form/legal signature) | Medium | separation of optional modules and signed change control |

**After UAT:** plan feature flags for sites and services, per-location onboarding playbook, training documents and release cadence before expansion to all Polres.


## 13. New go-live critical gates: speaker + day-one Polda

- [ ] Real Polda organizational parent, `POLDA_MONITOR` user and authorized aggregate Next.js dashboard usable **before Batang pilot**; accuracy/freshness display validated against Batang queues.
- [ ] Onboarding Polres as draft/request/approval/configure/publish works and does not create dummy real production Polres for tests; synthetic Polres Uji B exists only in staging.
- [ ] Auto announcement tests `VOICE-01..10` in `09_AUTOMATIC_QUEUE_VOICE.md`, including name policy, id-ID TTS compatibility, speakers/output routing and duplicated device leases.
- [ ] Operator `READY` and `PAUSED` state tested; auto-next triggers only for authorized staffed counter, never on booking clock, and does not overlap existing CALLED/SERVING ticket.
- [ ] Polda/read-only clients never access public call name; city/public screen never includes it; privacy basis for name pronunciation signed off or default fallback to number-only.
- [ ] Metrics: voice job lag/failure, announcer heartbeat and lease takeover, site audio zone health monitored with owner and escalation SOP. Field start-of-shift procedure includes speaker audio self-test.

**Source:** new request from product owner after paparan; pilot remains SIBATANG Batang. Technical details in `09_AUTOMATIC_QUEUE_VOICE.md` and `10_POLDA_FIRST_PILOT.md`.

## 14. Scope gate pilot Batang × dashboard Polda Jateng

**P0 sebelum go-live**: dashboard Polda Jawa Tengah dengan data aktual Batang, account supervisor terbatas, tenant-only production seed, situs Batang sesuai izin, isolasi tenant via staging Uji B, desktop TTS nomor+nama sesuai kebijakan dan anti-duplikasi announcer, printer/QR/display/booking E2E, SOP outage, backup restore.

**Not in scope**: Polda provinsi lain, pusat nasional, multi-Polda auth, data sharing lintas provinsi, onboarding daerah di luar Jawa Tengah. Jangan membuat test fixture multi-Polda untuk memenuhi pilot; test `non-Jateng parent` harus ditolak pada validation layer.

Untuk tugas dan acceptance dalam bentuk per-sprint, lihat `15_SPRINT_BACKLOG_AND_OWNERS.md`; untuk matriks uji rinci lihat `14_UAT_SECURITY_TEST_CASES.md`.
