-- 0001: organisasi (single-root Polda Jateng), tenant, site, katalog layanan, RBAC, seed pilot.
-- Lihat 10_POLDA_FIRST_PILOT.md, 11_DATABASE_SCHEMA_SQL.md. Seed TIDAK memuat user/password/NIK/Polres fiktif.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'sib_api') THEN
    CREATE ROLE sib_api NOLOGIN NOSUPERUSER NOBYPASSRLS;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'sib_reporting') THEN
    CREATE ROLE sib_reporting NOLOGIN NOSUPERUSER NOBYPASSRLS;
  END IF;
END $$;

CREATE TABLE organizations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  parent_id uuid REFERENCES organizations(id),
  org_type text NOT NULL CHECK (org_type IN ('POLDA','POLRES')),
  code text UNIQUE NOT NULL,
  display_name text NOT NULL,
  region_code text NOT NULL DEFAULT 'JAWA_TENGAH' CHECK (region_code = 'JAWA_TENGAH'),
  status text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','ACTIVE','SUSPENDED')),
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT root_shape CHECK (
    (org_type = 'POLDA' AND parent_id IS NULL) OR
    (org_type = 'POLRES' AND parent_id IS NOT NULL)
  ),
  CONSTRAINT allowed_polda_code CHECK (org_type <> 'POLDA' OR code = 'POLDA_JAWA_TENGAH'),
  CONSTRAINT polres_code_shape CHECK (org_type <> 'POLRES' OR code ~ '^POLRES_[A-Z0-9_]{2,40}$')
);
-- Tepat satu root Polda, bahkan pada insert konkuren.
CREATE UNIQUE INDEX org_single_polda_root ON organizations ((1)) WHERE org_type = 'POLDA';

CREATE FUNCTION validate_jateng_org() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE parent_code text; parent_type text;
BEGIN
  IF TG_OP = 'UPDATE' AND (NEW.parent_id IS DISTINCT FROM OLD.parent_id
       OR NEW.org_type <> OLD.org_type OR NEW.code <> OLD.code) THEN
    RAISE EXCEPTION 'organization hierarchy/code is immutable';
  END IF;
  IF NEW.org_type = 'POLRES' THEN
    SELECT code, org_type INTO parent_code, parent_type FROM organizations WHERE id = NEW.parent_id;
    IF parent_code IS DISTINCT FROM 'POLDA_JAWA_TENGAH' OR parent_type IS DISTINCT FROM 'POLDA' THEN
      RAISE EXCEPTION 'POLRES parent must be POLDA_JAWA_TENGAH';
    END IF;
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_validate_jateng_org BEFORE INSERT OR UPDATE ON organizations
  FOR EACH ROW EXECUTE FUNCTION validate_jateng_org();

-- Closure table untuk query descendant yang terotorisasi.
CREATE TABLE org_ancestors (
  ancestor_id uuid NOT NULL REFERENCES organizations(id),
  descendant_id uuid NOT NULL REFERENCES organizations(id),
  depth integer NOT NULL CHECK (depth >= 0),
  PRIMARY KEY (ancestor_id, descendant_id)
);
CREATE INDEX org_ancestors_desc ON org_ancestors (descendant_id, ancestor_id);

-- SECURITY DEFINER: runtime role tidak diberi INSERT langsung ke closure table.
CREATE FUNCTION maintain_org_closure() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
BEGIN
  INSERT INTO org_ancestors (ancestor_id, descendant_id, depth) VALUES (NEW.id, NEW.id, 0);
  IF NEW.parent_id IS NOT NULL THEN
    INSERT INTO org_ancestors (ancestor_id, descendant_id, depth)
      SELECT ancestor_id, NEW.id, depth + 1 FROM org_ancestors WHERE descendant_id = NEW.parent_id;
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_org_closure AFTER INSERT ON organizations
  FOR EACH ROW EXECUTE FUNCTION maintain_org_closure();

CREATE TABLE tenants (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owning_org_id uuid NOT NULL UNIQUE REFERENCES organizations(id),
  code text UNIQUE NOT NULL,
  brand_name text NOT NULL,
  booking_prefix text NOT NULL CHECK (booking_prefix ~ '^[A-Z]{2,5}$'),
  timezone text NOT NULL DEFAULT 'Asia/Jakarta' CHECK (timezone = 'Asia/Jakarta'),
  status text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','READY','PUBLISHED','SUSPENDED')),
  published_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE FUNCTION validate_jateng_tenant() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE owner_type text; parent_code text;
BEGIN
  SELECT o.org_type, p.code INTO owner_type, parent_code
    FROM organizations o LEFT JOIN organizations p ON p.id = o.parent_id
   WHERE o.id = NEW.owning_org_id;
  IF owner_type IS DISTINCT FROM 'POLRES' OR parent_code IS DISTINCT FROM 'POLDA_JAWA_TENGAH' THEN
    RAISE EXCEPTION 'Tenant owner must be a Jateng Polres';
  END IF;
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
  status text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','PUBLISHED','SUSPENDED')),
  UNIQUE (tenant_id, id), UNIQUE (tenant_id, code)
);

CREATE TABLE services (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code text UNIQUE NOT NULL,         -- SPKT, SKCK, SIM, STNK, BPKB (prefix nomor antrean)
  display_name text NOT NULL
);

CREATE TABLE tenant_services (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  site_id uuid NOT NULL,
  service_id uuid NOT NULL REFERENCES services(id),
  status text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','PUBLISHED','SUSPENDED')),
  -- OPEN-06: konservatif & configurable, bukan hardcode kebijakan lapangan.
  checkin_early_minutes integer NOT NULL DEFAULT 60 CHECK (checkin_early_minutes >= 0),
  checkin_grace_minutes integer NOT NULL DEFAULT 30 CHECK (checkin_grace_minutes >= 0),
  UNIQUE (tenant_id, id), UNIQUE (tenant_id, site_id, service_id),
  FOREIGN KEY (tenant_id, site_id) REFERENCES sites (tenant_id, id)
);

-- RBAC (role/permission dikelola kode Go sebagai sumber otoritas; tabel dipakai untuk integritas FK/audit).
CREATE TABLE users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  oidc_subject text UNIQUE,
  display_name text,
  status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','DISABLED')),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE roles (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), code text UNIQUE NOT NULL);
CREATE TABLE permissions (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), code text UNIQUE NOT NULL);
CREATE TABLE role_permissions (
  role_id uuid NOT NULL REFERENCES roles(id), permission_id uuid NOT NULL REFERENCES permissions(id),
  PRIMARY KEY (role_id, permission_id)
);
CREATE TABLE memberships (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id),
  role_id uuid NOT NULL REFERENCES roles(id),
  scope_type text NOT NULL CHECK (scope_type IN
    ('JATENG_PLATFORM','POLDA_JATENG','TENANT','SITE','COUNTER','AUDIO_ZONE')),
  scope_id uuid NOT NULL,
  tenant_id uuid REFERENCES tenants(id),  -- diisi untuk scope TENANT/SITE/COUNTER/AUDIO_ZONE
  starts_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz,
  revoked_at timestamptz,
  CHECK ((scope_type IN ('TENANT','SITE','COUNTER','AUDIO_ZONE')) = (tenant_id IS NOT NULL))
);
CREATE INDEX memberships_user ON memberships (user_id) WHERE revoked_at IS NULL;

CREATE TABLE sessions (
  token_hash bytea PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id),
  csrf_token text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz
);

-- Onboarding Polres Jateng (draft-only sampai disetujui, parent selalu root Jateng).
CREATE TABLE tenant_onboarding_requests (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  proposed_org_code text NOT NULL CHECK (proposed_org_code ~ '^POLRES_[A-Z0-9_]{2,40}$'),
  proposed_org_name text NOT NULL,
  proposed_tenant_code text NOT NULL CHECK (proposed_tenant_code ~ '^[A-Z0-9_]{3,60}$'),
  proposed_brand_name text NOT NULL,
  proposed_booking_prefix text NOT NULL CHECK (proposed_booking_prefix ~ '^[A-Z]{2,5}$'),
  status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','APPROVED','REJECTED')),
  requested_by uuid NOT NULL REFERENCES users(id),
  decided_by uuid REFERENCES users(id),
  decision_note text,
  created_org_id uuid REFERENCES organizations(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  decided_at timestamptz,
  CHECK (decided_by IS NULL OR decided_by <> requested_by)   -- persetujuan dua pihak
);

-- Audit append-only (tenant_id NULL untuk aksi level organisasi/Polda).
CREATE TABLE audit_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid,
  org_id uuid,
  actor_type text NOT NULL DEFAULT 'USER' CHECK (actor_type IN ('USER','DEVICE','SYSTEM','CITIZEN')),
  actor_id uuid,
  action text NOT NULL,
  resource_type text NOT NULL,
  resource_id uuid,
  request_id text,
  result text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_tenant_time ON audit_events (tenant_id, created_at DESC);

-- ===== Seed resmi pilot (nomenklatur Polres/Polresta harus divalidasi: OPEN-01) =====
INSERT INTO organizations (code, org_type, display_name, status)
  VALUES ('POLDA_JAWA_TENGAH', 'POLDA', 'Polda Jawa Tengah', 'ACTIVE');
INSERT INTO organizations (code, org_type, display_name, parent_id, status)
  SELECT 'POLRES_BATANG', 'POLRES', 'Polres Batang', id, 'ACTIVE'
    FROM organizations WHERE code = 'POLDA_JAWA_TENGAH';
INSERT INTO tenants (owning_org_id, code, brand_name, booking_prefix, status)
  SELECT id, 'SIBATANG_BATANG', 'SIBATANG', 'BTG', 'DRAFT' FROM organizations WHERE code = 'POLRES_BATANG';

INSERT INTO services (code, display_name) VALUES
  ('SPKT', 'SPKT / Laporan Kehilangan'), ('SKCK', 'SKCK'), ('SIM', 'SIM'),
  ('STNK', 'STNK'), ('BPKB', 'BPKB');

-- Site & mapping layanan Batang tetap DRAFT sampai approval (OPEN-02).
INSERT INTO sites (tenant_id, code, name, status)
  SELECT t.id, s.code, s.name, 'DRAFT' FROM tenants t,
   (VALUES ('SITE_SPKT_SKCK','SPKT / SKCK'),('SITE_SATPAS','SATPAS (SIM)'),('SITE_SAMSAT','SAMSAT (STNK/BPKB)')) AS s(code,name)
  WHERE t.code = 'SIBATANG_BATANG';
INSERT INTO tenant_services (tenant_id, site_id, service_id, status)
  SELECT t.id, si.id, sv.id, 'DRAFT'
    FROM tenants t
    JOIN sites si ON si.tenant_id = t.id
    JOIN (VALUES ('SITE_SPKT_SKCK','SPKT'),('SITE_SPKT_SKCK','SKCK'),('SITE_SATPAS','SIM'),
                 ('SITE_SAMSAT','STNK'),('SITE_SAMSAT','BPKB')) AS m(site_code, svc_code) ON m.site_code = si.code
    JOIN services sv ON sv.code = m.svc_code
   WHERE t.code = 'SIBATANG_BATANG';

INSERT INTO roles (code) VALUES
  ('PLATFORM_ADMIN'),('POLDA_ORG_ADMIN'),('POLDA_MONITOR'),('POLDA_SUPERVISOR'),('POLRES_ADMIN'),
  ('SITE_MANAGER'),('SERVICE_OPERATOR'),('RECEPTION_CHECKIN'),('REPORT_VIEWER'),
  ('DEVICE_KIOSK'),('DEVICE_DISPLAY'),('DEVICE_ANNOUNCER');
