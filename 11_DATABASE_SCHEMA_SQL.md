# 11 — Database Schema, Tenant Isolation dan SQL Migration Starter

> **Scope beku:** satu root `POLDA_JAWA_TENGAH`; satu tenant published pilot `SIBATANG_BATANG` milik `POLRES_BATANG`. Tenant lain hanya Polres Jawa Tengah setelah approval. Kode di bawah **fondasi skema yang harus direview**, bukan migration production lengkap atau legal approval.

## 1. Prinsip

- PostgreSQL 18; primary key UUID (`gen_random_uuid()`), UTC `timestamptz`, service date `date` berdasar WIB, `tenant_id NOT NULL` di seluruh tabel transaksional.
- Foreign key kombinasi `(tenant_id, entity_id)` agar data sebuah tenant tidak merujuk data tenant lain; tambah unique constraint sebelum FK.
- Org root tidak dapat dibuat lewat public API; hanya seed `POLDA_JAWA_TENGAH`. Admin Polda dapat mengajukan child Polres Jateng melalui approval.
- Status booking berbeda dari status tiket; tiket dicetak **sesudah check-in**. Booking reference/QR tidak boleh menjadi kredensial permanen.
- Simpan nama citizen hanya bila kebutuhan layanan dan dasar pemrosesan disetujui. Reporting Polda memakai aggregate sanitized.

## 2. Starter SQL tahap 001: organisasi, tenant, site, service

```sql
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE organizations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  parent_id uuid REFERENCES organizations(id),
  org_type text NOT NULL CHECK (org_type IN ('POLDA','POLRES')),
  code text UNIQUE NOT NULL,
  display_name text NOT NULL,
  region_code text NOT NULL DEFAULT 'JAWA_TENGAH'
    CHECK (region_code = 'JAWA_TENGAH'),
  status text NOT NULL DEFAULT 'DRAFT'
    CHECK (status IN ('DRAFT','ACTIVE','SUSPENDED')),
  CONSTRAINT root_shape CHECK (
    (org_type='POLDA' AND parent_id IS NULL) OR
    (org_type='POLRES' AND parent_id IS NOT NULL)
  )
);
-- A reviewed bootstrap migration must create exactly one POLDA root.
-- The DB constraints and triggers below enforce:
-- (1) POLDA code=POLDA_JAWA_TENGAH, (2) no second POLDA root,
-- (3) parent of POLRES=verified POLDA_JAWA_TENGAH,
-- (4) no graph cycles / renaming into other province.

CREATE TABLE tenants (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owning_org_id uuid NOT NULL UNIQUE REFERENCES organizations(id),
  code text UNIQUE NOT NULL,
  brand_name text NOT NULL,
  timezone text NOT NULL DEFAULT 'Asia/Jakarta'
    CHECK (timezone='Asia/Jakarta'),
  status text NOT NULL DEFAULT 'DRAFT'
    CHECK (status IN ('DRAFT','READY','PUBLISHED','SUSPENDED')),
  created_at timestamptz NOT NULL DEFAULT now()
);
-- Hard requirement for production migration: reviewed, DB-enforced regional root.
-- Unique partial index permits only one Polda root even under concurrent inserts.
CREATE UNIQUE INDEX org_single_polda_root
  ON organizations ((1)) WHERE org_type='POLDA';
ALTER TABLE organizations ADD CONSTRAINT allowed_polda_code
  CHECK (org_type <> 'POLDA' OR code = 'POLDA_JAWA_TENGAH');
-- The service must not allow arbitrary parent edits. The trigger below prevents
-- any POLRES whose direct parent is not the Jateng root.
CREATE OR REPLACE FUNCTION validate_jateng_org() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE parent_code text; parent_type text;
BEGIN
  IF NEW.org_type='POLRES' THEN
    SELECT code, org_type INTO parent_code,parent_type FROM organizations WHERE id=NEW.parent_id;
    IF parent_code IS DISTINCT FROM 'POLDA_JAWA_TENGAH' OR parent_type IS DISTINCT FROM 'POLDA'
    THEN RAISE EXCEPTION 'POLRES parent must be POLDA_JAWA_TENGAH'; END IF;
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_validate_jateng_org BEFORE INSERT OR UPDATE ON organizations
  FOR EACH ROW EXECUTE FUNCTION validate_jateng_org();

-- Tenant provisioner trigger also guards direct SQL (service checks required too).
CREATE OR REPLACE FUNCTION validate_jateng_tenant() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE owner_type text; parent_code text;
BEGIN
  SELECT o.org_type,p.code INTO owner_type,parent_code
    FROM organizations o LEFT JOIN organizations p ON p.id=o.parent_id
   WHERE o.id=NEW.owning_org_id;
  IF owner_type IS DISTINCT FROM 'POLRES' OR parent_code IS DISTINCT FROM 'POLDA_JAWA_TENGAH'
  THEN RAISE EXCEPTION 'Tenant owner must be a Jateng Polres'; END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_validate_jateng_tenant BEFORE INSERT OR UPDATE ON tenants
  FOR EACH ROW EXECUTE FUNCTION validate_jateng_tenant();


CREATE TABLE sites (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  code text NOT NULL,
  name text NOT NULL,
  address text,
  status text NOT NULL DEFAULT 'DRAFT',
  UNIQUE (tenant_id, id), UNIQUE (tenant_id, code)
);

CREATE TABLE services (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code text UNIQUE NOT NULL, -- SPKT, SKCK, SIM, STNK, BPKB
  display_name text NOT NULL
);

CREATE TABLE tenant_services (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  site_id uuid NOT NULL,
  service_id uuid NOT NULL REFERENCES services(id),
  status text NOT NULL DEFAULT 'DRAFT',
  UNIQUE (tenant_id, id), UNIQUE(tenant_id,site_id,service_id),
  FOREIGN KEY (tenant_id, site_id) REFERENCES sites(tenant_id,id)
);
```

## 3. Starter SQL tahap 002: jadwal, booking, tiket, loket

```sql
CREATE TABLE schedule_slots (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  tenant_service_id uuid NOT NULL,
  service_date date NOT NULL,
  starts_at timestamptz NOT NULL,
  ends_at timestamptz NOT NULL,
  capacity integer NOT NULL CHECK(capacity >= 0),
  booked_count integer NOT NULL DEFAULT 0 CHECK (booked_count >= 0),
  status text NOT NULL DEFAULT 'OPEN',
  CHECK (booked_count <= capacity AND starts_at < ends_at),
  UNIQUE(tenant_id,id),
  FOREIGN KEY(tenant_id,tenant_service_id)
    REFERENCES tenant_services(tenant_id,id)
);
CREATE INDEX slots_tenant_date ON schedule_slots(tenant_id,service_date,status);

CREATE TABLE bookings (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  tenant_service_id uuid NOT NULL,
  slot_id uuid NOT NULL,
  reference_code text NOT NULL,
  citizen_public_name text, -- classified: restricted; only if approved
  citizen_contact_encrypted bytea, -- application-level envelope encryption
  citizen_private_data_encrypted bytea,
  status text NOT NULL CHECK(status IN ('CONFIRMED','CHECKED_IN','CANCELLED','EXPIRED')),
  qr_secret_hash bytea NOT NULL, -- do not store plain QR access secret
  created_at timestamptz NOT NULL DEFAULT now(),
  checked_in_at timestamptz,
  UNIQUE(tenant_id,id), UNIQUE(tenant_id,reference_code),
  FOREIGN KEY(tenant_id,tenant_service_id) REFERENCES tenant_services(tenant_id,id),
  FOREIGN KEY(tenant_id,slot_id) REFERENCES schedule_slots(tenant_id,id)
);

CREATE TABLE counters (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL,
  site_id uuid NOT NULL,
  label text NOT NULL,
  state text NOT NULL DEFAULT 'CLOSED'
    CHECK(state IN ('CLOSED','READY','PAUSED','SERVING')),
  assigned_operator_id uuid,
  UNIQUE(tenant_id,id), UNIQUE(tenant_id,site_id,label),
  FOREIGN KEY(tenant_id,site_id) REFERENCES sites(tenant_id,id)
);

CREATE TABLE queue_tickets (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL,
  booking_id uuid,
  tenant_service_id uuid NOT NULL,
  site_id uuid NOT NULL,
  service_date date NOT NULL,
  sequence_no integer NOT NULL CHECK(sequence_no>0),
  display_number text NOT NULL,
  status text NOT NULL CHECK(status IN
    ('WAITING','CALLED','SERVING','COMPLETED','NO_SHOW','CANCELLED')),
  called_counter_id uuid,
  called_at timestamptz,
  completed_at timestamptz,
  UNIQUE(tenant_id,id),
  UNIQUE(tenant_id,booking_id), -- many NULL allowed for walk-ins
  UNIQUE(tenant_id,tenant_service_id,site_id,service_date,sequence_no),
  FOREIGN KEY(tenant_id,booking_id) REFERENCES bookings(tenant_id,id),
  FOREIGN KEY(tenant_id,tenant_service_id) REFERENCES tenant_services(tenant_id,id),
  FOREIGN KEY(tenant_id,site_id) REFERENCES sites(tenant_id,id),
  FOREIGN KEY(tenant_id,called_counter_id) REFERENCES counters(tenant_id,id)
);
CREATE INDEX queue_wait ON queue_tickets(tenant_id,site_id,status,service_date,sequence_no);

CREATE TABLE idempotency_keys (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  principal_key text NOT NULL,
  endpoint_key text NOT NULL,
  key_hash bytea NOT NULL,
  request_hash bytea NOT NULL,
  status_code integer,
  response_body jsonb,
  expires_at timestamptz NOT NULL,
  UNIQUE(tenant_id,principal_key,endpoint_key,key_hash)
);
```

**Wajib sebelum migration production:** tambahkan status CHECK constraints lengkap untuk site/tenant_services/schedule_slots; trigger pengaman organisasi; aplikasi mengatur perlakuan slot yang batal/expired; validasi slot milik tenant service yang sama (FK `(tenant_id, slot_id, tenant_service_id)` atau transactional invariant); batasi ukuran ciphertext/ref/label dan buat index retention. Untuk nomor urut gunakan `ticket_sequences(tenant_id,site_id,tenant_service_id,service_date,last_no)` yang di-*lock* saat check-in; jangan `MAX+1` di luar transaksi.

## 4. Starter SQL tahap 003: RBAC, announcer, reporting, outbox

```sql
CREATE TABLE users (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), oidc_subject text UNIQUE,
 status text NOT NULL DEFAULT 'ACTIVE'
);
CREATE TABLE roles (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), code text UNIQUE NOT NULL);
CREATE TABLE permissions (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), code text UNIQUE NOT NULL);
CREATE TABLE role_permissions (
 role_id uuid REFERENCES roles(id), permission_id uuid REFERENCES permissions(id),
 PRIMARY KEY(role_id,permission_id)
);
CREATE TABLE memberships (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id),
 role_id uuid NOT NULL REFERENCES roles(id), scope_type text NOT NULL,
 scope_id uuid NOT NULL, expires_at timestamptz, revoked_at timestamptz
);

CREATE TABLE audio_zones (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL,
 site_id uuid NOT NULL, label text NOT NULL,
 name_policy text NOT NULL DEFAULT 'NUMBER_ONLY',
 UNIQUE(tenant_id,id), FOREIGN KEY(tenant_id,site_id) REFERENCES sites(tenant_id,id)
);
CREATE TABLE devices (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL,
 site_id uuid NOT NULL, audio_zone_id uuid,
 device_type text NOT NULL, status text NOT NULL DEFAULT 'PENDING',
 last_seen_at timestamptz,
 UNIQUE(tenant_id,id), FOREIGN KEY(tenant_id,site_id) REFERENCES sites(tenant_id,id),
 FOREIGN KEY(tenant_id,audio_zone_id) REFERENCES audio_zones(tenant_id,id)
);
CREATE TABLE announcement_jobs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL,
 zone_id uuid NOT NULL, ticket_id uuid NOT NULL,
 call_event_id uuid NOT NULL UNIQUE, state text NOT NULL DEFAULT 'QUEUED',
 claimed_by_device_id uuid, fencing_token bigint NOT NULL DEFAULT 0,
 claimed_until timestamptz, started_at timestamptz, completed_at timestamptz,
 UNIQUE(tenant_id,id), FOREIGN KEY(tenant_id,zone_id) REFERENCES audio_zones(tenant_id,id),
 FOREIGN KEY(tenant_id,ticket_id) REFERENCES queue_tickets(tenant_id,id),
 FOREIGN KEY(tenant_id,claimed_by_device_id) REFERENCES devices(tenant_id,id)
);
CREATE TABLE outbox_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL,
 topic text NOT NULL, payload jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), processed_at timestamptz
);
CREATE TABLE daily_service_metrics (
 tenant_id uuid NOT NULL, site_id uuid NOT NULL, tenant_service_id uuid NOT NULL,
 service_date date NOT NULL, booking_count integer NOT NULL DEFAULT 0,
 served_count integer NOT NULL DEFAULT 0, no_show_count integer NOT NULL DEFAULT 0,
 avg_wait_seconds numeric, p95_wait_seconds numeric,
 data_freshness_at timestamptz NOT NULL,
 PRIMARY KEY(tenant_id,site_id,tenant_service_id,service_date)
);
CREATE TABLE audit_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid,
 actor_id uuid, action text NOT NULL, resource_type text NOT NULL,
 resource_id uuid, request_id text, result text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
```

Perlu tabel `queue_events`, `call_event` dengan unique event revision, membership service scopes, closure `org_ancestors`, heartbeats, consent grants, template/form versions dan notification attempts sebagai migration lanjutan sebelum seluruh fitur selesai. **Tidak boleh** menyimpan kalimat suara berisi nama dalam outbox/analytics/log.

## 5. RLS tenant transactions (contoh executable pattern)

```sql
ALTER TABLE bookings ENABLE ROW LEVEL SECURITY;
ALTER TABLE bookings FORCE ROW LEVEL SECURITY;
CREATE POLICY booking_tenant_isolation ON bookings
  FOR ALL TO sib_api
  USING (tenant_id = nullif(current_setting('app.tenant_id', true),'')::uuid)
  WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true),'')::uuid);
```

Pola set context melalui **satu transaksi koneksi ter-*pin*** (`pgx.BeginTx` + `SELECT set_config('app.tenant_id', $1, true)`), kemudian query, `COMMIT/ROLLBACK`; role runtime `sib_api` tidak owner/superuser/BYPASSRLS. Terapkan RLS untuk **semua** tabel milik tenant (termasuk join dan event). Jika request belum mendapatkan tenant verified, **deny** sebelum database dipanggil; query multi-tenant supervisor memakai repository dan DB role/query terpisah dengan whitelist descendants & aggregate-only, audit dan pengujian akses, **bukan** membypass RLS untuk booking/citizen.

## 6. Transaksi tahan race (pseudocode SQL)

```text
BOOK SLOT:
  begin tx
  assert tenant/user/slot scope
  INSERT idempotency marker OR load previous committed response
  UPDATE schedule_slots SET booked_count=booked_count+1
    WHERE tenant_id=? AND id=? AND status='OPEN' AND booked_count<capacity
    RETURNING id
  if no row => 409 SLOT_FULL
  INSERT bookings + outbox event
  persist idempotent response; commit

CHECK-IN QR:
  begin tx
  lock booking FOR UPDATE; verify QR hash + site + time/grace
  if existing queue ticket: return same ticket (idempotent)
  UPDATE/INSERT ticket_sequences... ON CONFLICT ... UPDATE last_no+1 RETURNING last_no
  INSERT queue_tickets; UPDATE bookings CHECKED_IN; outbox; commit

AUTO-CALL WHEN COUNTER READY:
  begin tx
  verify counter staffed, READY, site and service config
  lock counter row; select next WAITING SKIP LOCKED (ordered by approved priority/sequence)
  UPDATE queue_tickets status CALLED, called_counter_id, revision
  INSERT queue_events + announcement_jobs + outbox
  update counter state according to operational policy; commit
```

Gunakan satu retry policy bounded pada DB serialization errors, jangan retry arbitrary side-effects di luar transaksi. Pelayanan dianggap sukses berdasarkan DB commit, bukan hasil mencetak/memutar suara.

## 7. Migration + test checklist

- [ ] Fresh DB upgrade 001→N; production clone dengan data sintetis upgrade N−1→N; rollback strategy.
- [ ] Root Polda kedua ditolak **DB + service validation**; parent non-Jateng ditolak.
- [ ] Tenant Batang user tidak bisa baca/write tenant Uji B via raw SQL runtime role, join, export, SSE dan cache.
- [ ] Dua booking untuk slot terakhir → maksimum kapasitas.
- [ ] Dua QR scan bersamaan → satu ticket + sequence tidak duplikat.
- [ ] Dua operator call satu WAITING → satu CALLED; announcer job unik per call event.
- [ ] Stale zone lease/fencing token gagal ack, tidak ada dua active announcer claim.
- [ ] Polda view hanya sanitized aggregate dan `data_freshness_at`.
- [ ] Batas ukuran/index/retention/encryption diuji dengan kebutuhan riil dan review security.
