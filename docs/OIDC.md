# OIDC staf — yang masih harus dikerjakan

Backend sudah menyediakan sesi server-side (`sessions`, cookie HttpOnly `sib_session`, `csrf_token`, pencabutan) dan
`Service.CreateSession(userID, ttl)`. Yang belum ada adalah alur ke IdP resmi (OPEN-08):

1. `GET /api/v1/auth/staff/login`: buat `state`, `nonce`, PKCE verifier; simpan sementara (cookie bertanda tangan atau tabel); redirect ke IdP.
2. `GET /api/v1/auth/staff/callback`: validasi `state`, tukar `code`, validasi `iss`/`aud`/`exp`/`nonce`, pastikan klaim MFA (`amr`) untuk
   `POLDA_*` dan `POLRES_ADMIN`; petakan `sub` → `users.oidc_subject` (pengguna dibuat oleh admin via `sibctl user-add`, bukan auto-provision);
   panggil `CreateSession`, set cookie `HttpOnly; Secure; SameSite=Strict`, rotasi sesi saat login.
3. Native operator (Flutter desktop): Authorization Code + PKCE via browser sistem; simpan token di secure storage.

Jangan menambahkan login password lokal. Alat `sibctl dev-session` hanya untuk non-produksi dan menolak `APP_ENV=production`.
