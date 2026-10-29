# 08 — Paparan-to-Implementation Traceability & Decision Register


> **KEPUTUSAN CAKUPAN v2 (mengikat):** Platform ini **hanya untuk Polda Jawa Tengah**. Hari pertama produksi hanya **Polres Batang (brand publik: SIBATANG)** sebagai tenant dan **dashboard pengawasan Polda Jawa Tengah**. Polres lain di Jawa Tengah boleh di-*onboard* kemudian, setelah otorisasi; **tidak membangun platform nasional, tidak menerima Polda lain, tidak menampilkan tenant fiktif di produksi**. Nama institusi pada paparan tertulis “Polresta Batang”, sedangkan arahan pemilik produk “Polres Batang”; nomenklatur/legal owner harus divalidasi sebelum seed produksi. Kode teknis awal menggunakan `POLRES_BATANG`.


**Sumber:** paparan SIBATANG Polres Batang, 9 halaman. Dokumen ini memisahkan **ditampilkan dalam paparan** dari **rekomendasi teknis** sehingga engineer tidak menganggap desain UI ilustratif sebagai aturan bisnis resmi.

## 1. Traceability

| Req ID | Slide | Requirement source-derived | Components | MVP / Later | Acceptance evidence |
|---|---:|---|---|---|---|
| SIB-001 | 1,2,4,7 | Satu platform terintegrasi untuk SPKT, SKCK, SIM, STNK, BPKB | Go catalog, Web, Mobile, Desktop | MVP | semua service mempunyai tenant/site mapping |
| SIB-002 | 3,4,6,7,8 | Online booking via website/Android | Web + Mobile + Go | MVP web, P1 Android | end-to-end confirmation test |
| SIB-003 | 4,6,8 | Pilih layanan, tanggal, jadwal dan data awal | booking + slots + service form schema | MVP | capacity & field validation test |
| SIB-004 | 4,6,8 | Kode booking dan QR code | Go checkin capability, Web/Mobile QR | MVP | QR accepted only at authorized site |
| SIB-005 | 1,4,6,7 | Scan QR di lokasi touchscreen | Flutter Desktop kiosk | MVP | real scanner/USB + duplicate scan test |
| SIB-006 | 1,4,6,7 | Cetak nomor antrean | Go allocator + Desktop printer | MVP | serial unique/date/service/site |
| SIB-007 | 4,6,7,8 | Cetak/isi formulir pelayanan + tanda tangan digital dalam desain | Forms module, printing, retention | P1 after policy | template, e-sign legal status approved |
| SIB-008 | 1,4,6,7,8 | Display live menunjukkan nomor/loket/layanan | Go SSE + Desktop display | MVP | event live and reconnect snapshot |
| SIB-009 | 4,6,8 | Pantau progress/antrean aplikasi dan notifikasi | Mobile + Web + Go | MVP tracking, P1 push | status matches official queue |
| SIB-010 | 1,2,4,7,8 | WhatsApp AI untuk info dan konsultasi | Official WhatsApp provider + FAQ/handoff | P2 | source-controlled answers, escalation |
| SIB-011 | 5,8 | Info lokasi SPKT/SKCK, SATPAS SIM, SAMSAT STNK/BPKB dan navigasi | Go site catalog, Web, Mobile | MVP | correct mapped site + external maps |
| SIB-012 | 7,9 | Monitoring pengelolaan antrean, statistik, grafik oleh pimpinan | Go reports + Next admin | MVP Polres | correct hourly/daily counts |
| SIB-013 | 7,9 | Hak akses pengguna sesuai kewenangan pada dashboard | Scoped RBAC | MVP | denial matrix and audit pass |
| SIB-014 | 3,9 | Mengurangi penumpukan dan waktu tunggu melalui penjadwalan | schedule capacity + KPI | MVP | measured waiting + no-show baseline |
| ENG-001 | *new* | **Skala ke Polda sebagai supervisory scope** | organization tree, rollup report | **MVP foundation; scale depth later** | Polda dashboard active at Batang pilot; only allowed descendants |
| ENG-002 | *new* | **Tenant per Polres** + isolation data | Go authorization, PostgreSQL RLS | MVP foundation | cross-tenant E2E negative tests |
| ENG-003 | *new* | **Perangkat terdaftar dan dapat direvoke** | device identity and inventory | MVP | revoked kiosk cannot scan |
| ENG-004 | *new* | **Observability, backup, DR, support SOP** | DevOps/operations | MVP foundation | restore drill and incident test |
| ENG-005 | *new* | **Idempotency, atomic queue, audit trail** | Go transactions + PostgreSQL | MVP foundation | no oversell/duplicate ticket |
| USER-VOICE-001 | *user follow-up* | **Desktop otomatis sebut nomor antrean dan nama saat giliran** | Go announcement jobs, Windows Flutter TTS, audio zone | **MVP mandatory** | speaker announcement with approved name, no duplicate playback |
| USER-QUEUE-002 | *user follow-up* | **Auto-next saat counter READY** (kebijakan terkontrol) | Go counter policy/worker, Flutter operator | **MVP capability** | not triggered when staff absent/paused/closed |
| USER-POLDA-001 | *user follow-up* | **Pilot SIBATANG tetap Batang, tetapi Polda Jawa Tengah hierarchy+dashboard sudah aktif** | Go organization/RBAC, Next Polda pages | **MVP mandatory** | Polda authenticates and monitors Batang before field UAT |

## 2. Explicit source-specific notes

1. **Slide 5** menunjukkan tiga kelompok bangunan terpisah untuk layanan: gedung SPKT/SKCK, SATPAS untuk SIM, SAMSAT untuk STNK dan BPKB. Peta jarak di slide adalah ilustrasi konteks Batang, bukan basis koordinat GPS resmi. Production coordinates/alamat harus diverifikasi dari instansi.
2. **Slide 7** menampilkan perangkat server/database, kiosk+scanner, printer tiket, display, jaringan dan PC operator; jumlah atau model hardware **tidak ditentukan** secara teknis oleh paparan.
3. **Slide 8** menunjukkan flow 12 screen (termasuk digital signature, WhatsApp, install app). Itu **mockup fitur**, bukan kepastian persyaratan hukum ttd digital atau konfirmasi iOS sudah dibangun.
4. **Slide 4 dan 6** menunjukkan QR dari proses online lalu nomor antrean tercetak saat check-in; oleh karena itu backend memisahkan `booking_reference` dan `ticket_display_number`.
5. **Slide 2/9** menyatakan monitoring oleh pimpinan/petugas, tetapi **tidak** mendefinisikan Polda, Polres hierarchy atau access scope. Hierarki dan hak Polda dalam dokumen ini adalah **perluasan yang diminta** untuk multi-tenant.
6. **Slide 5** mencantumkan SIM/STNK/BPKB dalam satu pengalaman sistem; dokumen ini **tidak** menyatakan sudah ada hak integrasi layanan inti eksternal. Semua external APIs dianggap belum tersedia sampai pemilik layanan menyetujui.

## 3. Product decisions untuk persetujuan

| Decision ID | Proposed default | Risk if unresolved | Decision owner |
|---|---|---|---|
| DEC-001 | Polres = operational tenant; Polda = org supervisor aggregate | cross-tenant policy ambiguous | PM + Polda/Polres authority |
| DEC-002 | Polda only see anonymized metrics | privacy leak if over-permission | Data controller / security |
| DEC-003 | Web + Android MVP, iOS later | app store/cost uncertainty | Product owner |
| DEC-004 | Guest booking permitted with anti-abuse; OTP optional | fake appointments vs inclusivity | Service owner / legal |
| DEC-005 | NIK collected **only** when service requires | unnecessary sensitive collection | Service owner / legal |
| DEC-006 | QR token opaque, scoped and expires after allowed check-in window | replay/security vs long pre-booking interval | Security + Backend |
| DEC-007 | Ticket issued after check-in, unique by site+service+date | ambiguous visual mockup semantics | Operations head |
| DEC-008 | Priority/no-show/requeue configured by service | unfair queue order | Service owner |
| DEC-009 | Windows desktop primary; printer model to be certified | field installation failure | Procurement / Desktop lead |
| DEC-010 | No autonomous offline ticket numbering | duplicate queues and audit conflict | Site operations |
| DEC-011 | Official WhatsApp integration after approval | noncompliance/integration failure | Communications / IT |
| DEC-012 | e-sign scribble not assumed legally qualified | invalid forms/legal claim | Legal & Service owner |
| DEC-013 | Polda metrics may have delayed aggregation, freshness shown | false real-time oversight claims | Reporting product owner |
| DEC-014 | Default data retention not set until policy approved | unlawful retention/deletion | Data controller |

## 4. Entry gate for adding any Polres

1. Organization record and approved mapping to parent Polda.
2. Tenant and site IDs created; physical addresses and geo coordinates verified.
3. Service offerings, owners, operating hours, capacity, prerequisite content and terms approved.
4. Roles/memberships seeded from official operator lists; MFA and revoke training.
5. Device procurement profiles certified (QR scanner, touchscreen, printer, display, Windows build).
6. Kiosk, operator and display enrolled to exact tenant/site; telemetry healthy.
7. Successful dry-run booking → kiosk → printer → operator → display → report.
8. Privacy/retention and escalation SOP signed, support contact configured.

**Expansion principle:** new Polres adds configuration and tenant resources, **not** new application fork or duplicated codebase.


## 5. Revisi eksplisit sesudah permintaan pengguna

1. **Pengumuman suara nomor dan nama** bukan requirement yang dibuktikan langsung oleh slide: slide 4/6/7 menampilkan nomor, loket, display dan notifikasi. Tambahan audio/nama adalah **user follow-up**, maka dokumen `09_AUTOMATIC_QUEUE_VOICE.md` menjadi spesifikasi normatif.
2. **Organisasi Polda-Jateng-from-day-one** juga **user follow-up**, bukan claim dari paparan. `10_POLDA_FIRST_PILOT.md` mewajibkan dashboard Polda sebelum UAT pilot Batang; tinggal perlu konfirmasi otoritas hierarki dan scope data.
3. Kebijakan pengumuman nama memerlukan keputusan perlindungan data; engineering default NUMBER_ONLY bila dasar pengumuman nama tidak disetujui; nomor tetap dipanggil otomatis.
4. Mode **auto-advance** bukan timer booking: nomor berikutnya hanya dipanggil oleh backend bila counter staffed+READY. Mode ini ada di MVP tetapi dapat dimatikan per counter sesuai persetujuan site.

New decisions: `DEC-015` voice name policy/consent, `DEC-016` Indonesian offline TTS/speaker hardware, `DEC-017` auto-advance enabled per counter, `DEC-018` Polda org and onboarding approval authority.

## 6. Koreksi final cakupan yang harus dijadikan acuan

| Requirement | Sumber | Status implementasi |
|---|---|---|
| SIBATANG untuk Polres Batang sebagai pilot | Paparan 1–9 + penegasan pemilik produk | Wajib P0 |
| Polda Jawa Tengah memonitor Batang sejak pilot | Koreksi final pemilik produk; bukan klaim eksplisit paparan | Wajib P0 |
| Penambahan Polres lain hanya dalam Jawa Tengah | Koreksi final pemilik produk | Infrastruktur/discovery/onboarding siap P0, aktivasi belakangan |
| Bukan platform nasional/multi-Polda | Koreksi final pemilik produk | Scope constraint dan test negatif P0 |
| Flutter desktop otomatis panggil nomor + nama + loket | Penegasan pemilik produk | Wajib P0 dengan privacy policy dan hardware TTS test |
| Nomenklatur `Polres Batang` vs `Polresta Batang` | Pemilik produk vs teks paparan | Perlu verifikasi sebelum produksi |
| Integrasi data SIM/STNK/BPKB pihak ketiga | Tidak terjamin oleh paparan | Jangan asumsi tersedia; butuh izin dan API sah |
