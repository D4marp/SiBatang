-- 0003: Row Level Security (defense-in-depth) + grant minimal untuk role runtime.
-- API terhubung sebagai login role anggota sib_api (non-owner, non-superuser, non-BYPASSRLS) dan
-- setiap transaksi melakukan SET LOCAL ROLE sib_api + set_config('app.tenant_id', ..., true).

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY[
    'sites','tenant_services','schedule_slots','bookings','audio_zones','counters','counter_services',
    'ticket_sequences','queue_tickets','devices','checkins','queue_events','queue_announcements',
    'outbox_events','idempotency_keys','daily_service_metrics','tenant_health_snapshots'
  ] LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    EXECUTE format($p$CREATE POLICY tenant_isolation ON %I FOR ALL TO sib_api
        USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
        WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)$p$, t);
    EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON %I TO sib_api', t);
  END LOOP;
END $$;

-- Audit: tenant melihat/menulis miliknya; INSERT tenant_id NULL (aksi org/Polda) diperbolehkan, tidak dapat dibaca.
ALTER TABLE audit_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_events FORCE ROW LEVEL SECURITY;
CREATE POLICY audit_select ON audit_events FOR SELECT TO sib_api
  USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY audit_insert ON audit_events FOR INSERT TO sib_api
  WITH CHECK (tenant_id IS NULL OR tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
GRANT SELECT, INSERT ON audit_events TO sib_api;   -- append-only: tanpa UPDATE/DELETE

-- Tabel non-tenant (inventaris eksplisit; diuji di TestTenantTableInventory).
GRANT SELECT ON organizations, org_ancestors, tenants, services, roles, permissions, role_permissions TO sib_api;
GRANT INSERT ON organizations, tenants TO sib_api;          -- onboarding approve (dijaga trigger)
GRANT UPDATE (status, published_at) ON tenants TO sib_api;  -- publish
GRANT SELECT, INSERT, UPDATE ON users, memberships, sessions TO sib_api;
GRANT SELECT, INSERT, UPDATE ON tenant_onboarding_requests TO sib_api;
GRANT SELECT, INSERT, UPDATE ON device_credentials, device_activations TO sib_api;

-- Role pelaporan Polda: HANYA metrik agregat + hierarki. Tidak ada grant ke bookings/tiket/devices.
GRANT SELECT ON organizations, org_ancestors, tenants TO sib_reporting;
GRANT SELECT ON daily_service_metrics, tenant_health_snapshots TO sib_reporting;
CREATE POLICY reporting_read ON daily_service_metrics FOR SELECT TO sib_reporting USING (true);
CREATE POLICY reporting_read ON tenant_health_snapshots FOR SELECT TO sib_reporting USING (true);
