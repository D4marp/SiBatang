#!/bin/bash
# Dijalankan sekali oleh image postgres saat volume kosong. Membuat peran least-privilege:
#   sib_migrator : pemilik skema (hanya untuk migrasi/ops), BUKAN dipakai API
#   sib_app      : login runtime API/worker; non-superuser, non-BYPASSRLS; anggota sib_api & sib_reporting
set -euo pipefail
: "${SIB_MIGRATOR_PASSWORD:?required}" "${SIB_APP_PASSWORD:?required}"

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<SQL
CREATE ROLE sib_migrator LOGIN CREATEROLE PASSWORD '${SIB_MIGRATOR_PASSWORD}';
CREATE ROLE sib_api NOLOGIN NOSUPERUSER NOBYPASSRLS;
CREATE ROLE sib_reporting NOLOGIN NOSUPERUSER NOBYPASSRLS;
CREATE ROLE sib_app LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEROLE NOCREATEDB PASSWORD '${SIB_APP_PASSWORD}';
GRANT sib_api, sib_reporting TO sib_app;
ALTER DATABASE ${POSTGRES_DB} OWNER TO sib_migrator;
GRANT ALL ON SCHEMA public TO sib_migrator;
SQL
