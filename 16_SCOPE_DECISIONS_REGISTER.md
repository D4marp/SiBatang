# 16 — Project Scope Freeze, Decision Register dan Approval Checklist

**Version:** v2.0 (24 Oktober 2026), based on the supplied 9-page SIBATANG presentation and owner's explicit corrections. This document resolves previous over-expansion to national/multi-Polda systems.

## 1. Decision log — binding

| ID | Decision | Status | Impact |
|---|---|---|---|
| ADR-001 | Pilot SIBATANG hanya Polres Batang | Locked | all citizen/product UI initially Batang |
| ADR-002 | Polda Jawa Tengah langsung dapat monitoring sejak pilot | Locked | RBAC/reporting/dashboard day one |
| ADR-003 | Platform deployment hanya wilayah Jawa Tengah, **satu Polda** | Locked | no national root/multi-Polda API/data |
| ADR-004 | Polres lain di Jateng bisa ditambahkan di kemudian hari | Locked | tenant isolation/onboarding approval now; publish later |
| ADR-005 | SIBATANG brand milik Batang, bukan brand Polres lain | Locked | tenant metadata/theming |
| ADR-006 | Pemanggilan nomor + nama + loket otomatis lewat desktop | Locked with privacy gate | Go jobs + Windows TTS + approved speaker policy |
| ADR-007 | Otomatis berarti *giliran dipanggil saat loket siap*, bukan jam booking | Technical decision | no early call based solely on appointment time |
| ADR-008 | Backend Go modular monolith + PostgreSQL, REST/SSE | Architectural baseline | light deployment, single source of truth |
| ADR-009 | Polda read-only aggregate; no routine raw citizen records | Security default | scope joins, sanitization and audit |
| ADR-010 | Demo tenant kedua synthetic Jateng staging saja | Locked | prove isolation without fake production data |

## 2. Paparan vs penambahan kebutuhan

**Dari paparan 9 halaman:** brand SIBATANG; layanan laporan kehilangan/SPKT, SKCK, SIM, STNK, BPKB; Android+website; pendaftaran, syarat/jadwal, QR; check-in touchscreen; printer nomor+form; display nomor antrian; live progress/notifikasi; lokasi SPKT/SKCK, SATPAS, SAMSAT; WhatsApp AI dan dashboard monitoring. Digital signature appears in illustration but its certification/legal equivalence **not established**.

**Berasal dari arahan pemilik produk setelah paparan:** Flutter desktop auto-call **nama dan nomor**, Polda Jawa Tengah menjadi pengawas sejak awal, satu pilot Batang dan ekspansi **hanya Polres lain di Jawa Tengah**. Paparan sendiri **tidak menetapkan** arsitektur multi-tenant atau role Polda.

## 3. Hal yang masih perlu keputusan tertulis sebelum go-live

| ID | Item belum diputuskan | Default aman sementara | Decision owner |
|---|---|---|---|
| OPEN-01 | Penamaan resmi `Polres Batang` vs `Polresta Batang` di paparan | teknis `POLRES_BATANG`, publish setelah verifikasi | Polres/unit hukum |
| OPEN-02 | Operasional SATPAS dan SAMSAT: owner/data/controller dan kerja sama | site `DRAFT`, tidak mengklaim integrasi resmi | unit layanan terkait |
| OPEN-03 | Kebijakan publik penyebutan nama via speaker | `NUMBER_ONLY` sampai izin/dasar sah; fitur nama sudah dibangun | pemilik layanan + pejabat privasi |
| OPEN-04 | Voice Indonesian offline dan speaker Windows terpilih | test device + prerecorded/human SOP | tim desktop + site |
| OPEN-05 | Berapa loket, kapasitas dan jadwal per service | configurable, belum hardcode | PIC lapangan |
| OPEN-06 | Ketentuan walk-in, no-show, cancellation, check-in grace | configurable conservative defaults pending approval | PIC layanan |
| OPEN-07 | Syarat dan versi formulir per layanan | content managed + template approvals | PIC layanan |
| OPEN-08 | OIDC/SSO, data hosting, retention, incident response | approved private hosting + MFA admins | platform/security |
| OPEN-09 | WhatsApp API official, AI bot knowledge & handoff | deferred, manual support available | PIC komunikasi |
| OPEN-10 | RPO/RTO/SLA dan hardware bill of materials | preliminary benchmark only | operations/finance |
| OPEN-11 | Scope onboarding Polres Jateng kedua + date | draft capability only, none in production | Polda Jateng |

## 4. Explicit exclusions

- Tidak ada dashboard/akun administrator untuk Polda lain, nasional, Mabes atau provinsi lain.
- Tidak ada enrollment Polres di luar Jateng, master database nasional atau cross-province sharing.
- Tidak ada pengakuan sertifikasi tanda tangan digital, pembayaran, atau perubahan data nasional SIM/STNK/BPKB tanpa approval resmi.
- Tidak ada jaminan audio TTS membaca semua nama tanpa test real-device; privacy policy menentukan kapan boleh menyebut nama.

## 5. Scope change protocol

Proposal yang melanggar ADR-001–010 harus melalui change request formal berisi alasan, perbedaan kebutuhan, dampak keamanan/privasi, perubahan schema/API/UX, QA regression, estimasi biaya, rollback, dan persetujuan dari Polres Batang serta otoritas Polda Jateng terkait. Development tidak menambahkan generalisasi ke Polda lain semata-mata karena technically possible.

## 6. Checklist audit sebelum coding dimulai

- [ ] README dan 4 platform specs menerapkan root `POLDA_JAWA_TENGAH`.
- [ ] Tidak ada login/routing Polda lain pada aplikasi.
- [ ] Pilot `SIBATANG_BATANG` diset ke default semua public flow.
- [ ] Dashboard Polda diuji bersama data Batang real.
- [ ] Draft onboarding hanya Polres Jateng approved.
- [ ] Nama suara hanya untuk device announcer authorized sesuai kebijakan.
- [ ] Blueprint database dan API mengikuti scope ini.
- [ ] Kedua pemilik (Batang dan Polda Jateng) menyetujui roadmap/UAT.
