# 14 — UAT, Security dan Performance Test Matrix

> All production pilot tests use Polres Batang and Polda Jawa Tengah. For tenant isolation, **Polres Uji B** is a synthetic second child in **staging under Polda Jateng**, not a real unit and not a second Polda. UI brand SIBATANG is pilot Batang only.

## 1. Acceptance prerequisites

- Document test owner, version/commit, installed Windows/Flutter/TTS language, hardware scanner/printer/speaker model, target site and expected service owner.
- Use synthetic citizen data in test and screenshots. Real citizen data may not be included in public QA exports.
- All high/critical defects block go-live. Operational legal/privasi approval is a separate gate from technical test pass.
- Record evidence: request ID, timestamp, test screenshots, Polda aggregate snapshot, printer ticket, audio speaker test observation (without recording real names), RLS logs redacted.

## 2. Citizen booking + location/service

| ID | Test | Expected |
|---|---|---|
| PUB-01 | Open web/app before second Polres published | Only SIBATANG / Batang service catalog |
| PUB-02 | View SPKT/SKCK site info | correct approved requirements, opening hours, maps |
| PUB-03 | View SATPAS/SAMSAT info | only approved sites, no promise of direct official backend integration |
| PUB-04 | Book available slot | one CONFIRMED booking + opaque QR + idempotent response |
| PUB-05 | Fill last slot concurrently 100 times, capacity 10 | 10 or fewer bookings; no overbooking |
| PUB-06 | Retry same idempotency key/body | exactly one booking; same response |
| PUB-07 | Retry same key different payload | conflict, no duplicate booking |
| PUB-08 | Change URL booking reference to someone else's | 403/404 absent own authorization token |
| PUB-09 | Cancel after allowed deadline vs after deadline | enforce policy, stock of slots consistent |
| PUB-10 | Lost network and reconnect | citizen sees latest authoritative state, no duplicate create |

## 3. Desktop kiosk, printer, operator, queue and display

| ID | Test | Expected |
|---|---|---|
| DESK-01 | First check-in using correct QR/site/time | one WAITING queue ticket, printable |
| DESK-02 | Two kiosks scan same QR simultaneously | same issued number, no duplicate |
| DESK-03 | Scan Batang QR on wrong site / staging tenant B | denied with no check-in |
| DESK-04 | Printer out of paper after check-in commit | reprint original ticket, audit attempt |
| DESK-05 | Operator unassigned tries call-next | denied |
| DESK-06 | Two counters race single last WAITING ticket | at most one CALLED record |
| DESK-07 | Operator READY with auto-policy | one next eligible ticket called |
| DESK-08 | Operator PAUSED/SERVING/CLOSED | never auto-call |
| DESK-09 | Called ticket → serving → complete | valid state and timestamps |
| DESK-10 | Recall / no-show | follows cooldown/requeue SOP and audit |
| DESK-11 | Disconnect SSE and reconnect | display restores correct snapshot; no names |
| DESK-12 | Cross-midnight service-day in WIB | counters reset per approved service date, no collision |

## 4. Voice announcement (Windows actual hardware)

| ID | Test | Expected |
|---|---|---|
| VOI-01 | Call `SKCK-025`, name policy approved | audible local voice reads number, allowed name, Loket 2 |
| VOI-02 | `NUMBER_ONLY` or consent absent | speaker says number/loket without name; claim response has null name |
| VOI-03 | Two PCs on same audio zone | only leased primary plays; standby stays silent |
| VOI-04 | Three near-simultaneous calls from counters | serialized audio FIFO per zone; no overlaps |
| VOI-05 | TTS id-ID unavailable | health not-ready, approved prerecorded/manual fallback, no broken English names |
| VOI-06 | Speaker unplugged / volume zero | visible failure and site alert; backend queue not rolled back |
| VOI-07 | 60s disconnect and recovery | no stale/double replay of already served ticket |
| VOI-08 | Recall after configured cooldown | exactly one new authorized announce job |
| VOI-09 | Audio zone A device tries zone B job | denied, no name disclosure |
| VOI-10 | 8-hour operational soak, real printer/speaker | stable queue and audio; store p95 and error rate |

## 5. RBAC + Polda Jawa Tengah

| ID | Test | Expected |
|---|---|---|
| SEC-01 | Polda Jateng monitor login | aggregate Batang, only 1 actual production tenant |
| SEC-02 | Polda loads aggregated queue counts | accurate, `as_of`, no name/NIK/QR/contact |
| SEC-03 | Polda tries `/staff/tickets/:id` | 403/404, no citizen details |
| SEC-04 | Batang operator changes tenant to Uji B staging | 403/404 and RLS returns no rows |
| SEC-05 | Uji B operator requests Batang display SSE | connection denied/no event |
| SEC-06 | Admin changes URL parent to non-Jateng org | 403/422; no new Polda/Polres outside Jateng created |
| SEC-07 | Polda onboards Uji B staging as draft | not in public catalog until approved publish |
| SEC-08 | Revoke staff/device mid-session | following calls/streams fail after revocation propagation |
| SEC-09 | Web browser CSRF, token replay, rate limiting | rejection without state change |
| SEC-10 | Cache/SSR export with scopes | one tenant user's HTML never includes another tenant data |
| SEC-11 | Scan stored logs/observability | no NIK, raw QR, name speech payload or passwords |
| SEC-12 | Try SQL runtime role without tenant context | 0 booking rows, no update |
| SEC-13 | Tenant B tries FK reference to Batang booking/site | DB rejects relation |
| SEC-14 | Verify Polda no direct SQL BYPASSRLS privileges | role least-privilege documented |

## 6. Ops + resilience

| ID | Test | Expected |
|---|---|---|
| OPS-01 | Fresh install and migration | root Jateng, tenant Batang draft, no fake production data |
| OPS-02 | Full restore from encrypted backup | correct counts and RLS, credentials not embedded |
| OPS-03 | Kill worker before event completion | outbox retries without duplicate call mutation |
| OPS-04 | Crash announcer while playing | bounded retry/alert, no unbounded repeat |
| OPS-05 | Polda aggregator unavailable | dashboard stale/error, not fabricated zeros |
| OPS-06 | Real kiosk/device disconnected | health alert plus approved manual SOP |
| OPS-07 | Printer and TTS plugin upgrade | compatibility preflight and rollback available |
| OPS-08 | Tenant Jateng #2 onboarding dry-run staging | no app fork/backend rewrite required |

## 7. Performance goals (provisional, must calibrate)

- Healthy local TTS p95 speech start within **3s** after committed call, **excluding waiting behind other announcement jobs**; prove via real speaker timestamps. Never claim exactly-once audible delivery.
- Public API availability target and p95 latency based on accepted load profile. Do not invent SLA without measurement.
- Booking concurrency at/over offered slot limit should never oversubscribe; queue call never chooses same ticket twice.
- Report freshness goal e.g. 60s, explicitly display freshness; production threshold approved by Polda dashboard stakeholders.
- Make load profile with observed peak visitors, service hours, sites, operators, number of SSE clients and safety margin before selecting server specs.

## 8. Go-live signature matrix

| Gate | Required owner sign-off |
|---|---|
| Legal privacy/naming/service integration permissions | Policy/service owner |
| RBAC/tenant security/pen-test | Security & platform owner |
| QR printer TTS Windows physical QA | Desktop lead and site manager |
| Polda live aggregation | Polda monitor product representative |
| Operasional fallback and training | Polres Batang representative |
| Backups, incidents and incident contacts | DevOps operations owner |

Any missing mandatory approval means technical system may be ready **but not cleared for public launch**.
