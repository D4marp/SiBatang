# Login staf OIDC

Diimplementasikan di `backend/internal/oidc` (klien, stdlib saja) dan `GET /api/v1/auth/staff/{login,callback}`.

## Alur
1. `login`: buat `state`, `nonce`, verifier PKCE (S256); simpan terenkripsi AES-GCM di cookie `sib_oidc` (HttpOnly, 10 menit); redirect ke IdP.
2. `callback`: cocokkan `state` (constant-time), tukar `code` + verifier, verifikasi `id_token` (RS256/ES256 via JWKS; `iss`, `aud`, `exp`, `nbf`, `nonce`; alg `none`/HS* ditolak).
3. Subject harus sudah ada di `users.oidc_subject` berstatus ACTIVE (dibuat admin via `sibctl user-add`). **Tidak ada auto-provision.**
4. Peran privilese (PLATFORM_ADMIN, POLDA_*, POLRES_ADMIN, SITE_MANAGER) wajib klaim `amr` berisi `mfa` (`OIDC_REQUIRE_MFA`, default true).
5. Sesi baru dibuat (rotasi tiap login), cookie `sib_session` HttpOnly, SameSite=Lax (Lax agar terkirim setelah redirect dari IdP), `Secure` di luar development; login diaudit.
6. Kegagalan mengarah ke `/masuk?error=state|idp|mfa|not_registered|server` tanpa detail.

## Konfigurasi
`OIDC_ISSUER_URL`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET` (opsional untuk public client), `OIDC_REDIRECT_URL` (= redirect URI terdaftar di IdP),
`OIDC_REQUIRE_MFA`. URL wajib https (http hanya localhost saat `APP_ENV=development`). Kosong = login 501.

## Belum
- Operator desktop (Flutter) masih perlu alur native (Authorization Code + PKCE via browser sistem, token di secure storage).
- Logout terfederasi (RP-initiated logout) dan refresh/idle timeout sesi; sesi berumur tetap 8 jam.
- Pengujian terhadap IdP institusi nyata (diuji hanya dengan IdP palsu RS256).
