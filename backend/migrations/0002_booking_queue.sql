-- 0002: jadwal, booking, loket, tiket antrean, event, perangkat, zona audio, announcement, outbox, metrik.
-- Semua tabel milik tenant: tenant_id NOT NULL + FK komposit (tenant_id, id). RLS diaktifkan di 0003.

CREATE TABLE schedule_slots (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  tenant_service_id uuid NOT NULL,
  service_date date NOT NULL,                    -- tanggal layanan lokal WIB
  starts_at timestamptz NOT NULL,
  ends_at timestamptz NOT NULL,
  capacity integer NOT NULL CHECK (capacity >= 0),
  booked_count integer NOT NULL DEFAULT 0 CHECK (booked_count >= 0),
  status text NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','CLOSED')),
  CHECK (booked_count <= capacity AND starts_at < ends_at),
  UNIQUE (tenant_id, id), UNIQUE (tenant_id, id, tenant_service_id),
  UNIQUE (tenant_id, tenant_service_id, starts_at),
  FOREIGN KEY (tenant_id, tenant_service_id) REFERENCES tenant_services (tenant_id, id)
);
CREATE INDEX slots_tenant_date ON schedule_slots (tenant_id, service_date, status);

CREATE TABLE bookings (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  tenant_service_id uuid NOT NULL,
  slot_id uuid NOT NULL,
  reference_code text NOT NULL,
  origin text NOT NULL DEFAULT 'WEB' CHECK (origin IN ('WEB','MOBILE','WALK_IN')),
  citizen_public_name text,                       -- restricted; hanya untuk nama panggilan bila disetujui
  voice_name_consent boolean NOT NULL DEFAULT false,
  citizen_contact_encrypted bytea,                -- AES-GCM envelope di aplikasi
  privacy_notice_version text NOT NULL,
  status text NOT NULL CHECK (status IN ('CONFIRMED','CHECKED_IN','CANCELLED','EXPIRED')),
  qr_secret_hash bytea NOT NULL,                  -- SHA-256 token acak 256-bit; token asli tidak disimpan
  qr_expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  checked_in_at timestamptz,
  cancelled_at timestamptz,
  UNIQUE (tenant_id, id), UNIQUE (tenant_id, reference_code), UNIQUE (tenant_id, qr_secret_hash),
  FOREIGN KEY (tenant_id, tenant_service_id) REFERENCES tenant_services (tenant_id, id),
  -- slot harus milik tenant service yang sama
  FOREIGN KEY (tenant_id, slot_id, tenant_service_id) REFERENCES schedule_slots (tenant_id, id, tenant_service_id)
);
CREATE INDEX bookings_slot ON bookings (tenant_id, slot_id, status);

CREATE TABLE audio_zones (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL,
  site_id uuid NOT NULL,
  label text NOT NULL,
  -- default aman: NUMBER_ONLY sampai dasar hukum penyebutan nama disetujui (OPEN-03)
  name_policy text NOT NULL DEFAULT 'NUMBER_ONLY'
    CHECK (name_policy IN ('NUMBER_ONLY','PREFERRED_NAME_IF_CONSENT','FULL_NAME_IF_APPROVED')),
  voice_locale text NOT NULL DEFAULT 'id-ID',
  active_device_id uuid,
  lease_until timestamptz,
  lease_generation bigint NOT NULL DEFAULT 0,      -- fencing token monotonic
  UNIQUE (tenant_id, id), UNIQUE (tenant_id, site_id, label),
  FOREIGN KEY (tenant_id, site_id) REFERENCES sites (tenant_id, id)
);

CREATE TABLE counters (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL,
  site_id uuid NOT NULL,
  audio_zone_id uuid NOT NULL,
  label text NOT NULL,
  state text NOT NULL DEFAULT 'CLOSED' CHECK (state IN ('CLOSED','READY','CALLING','SERVING','PAUSED')),
  assigned_operator_id uuid REFERENCES users(id),
  readiness_revision integer NOT NULL DEFAULT 0,
  -- kebijakan antrean per loket (queue_counter_policies dilipat ke sini)
  auto_announce_on_call boolean NOT NULL DEFAULT true,
  auto_advance_on_ready boolean NOT NULL DEFAULT false,
  max_recall_per_ticket integer NOT NULL DEFAULT 3 CHECK (max_recall_per_ticket >= 0),
  recall_cooldown_seconds integer NOT NULL DEFAULT 10 CHECK (recall_cooldown_seconds >= 0),
  stale_job_ttl_seconds integer NOT NULL DEFAULT 90 CHECK (stale_job_ttl_seconds > 0),
  UNIQUE (tenant_id, id), UNIQUE (tenant_id, site_id, label),
  FOREIGN KEY (tenant_id, site_id) REFERENCES sites (tenant_id, id),
  FOREIGN KEY (tenant_id, audio_zone_id) REFERENCES audio_zones (tenant_id, id)
);

CREATE TABLE counter_services (
  tenant_id uuid NOT NULL,
  counter_id uuid NOT NULL,
  tenant_service_id uuid NOT NULL,
  PRIMARY KEY (tenant_id, counter_id, tenant_service_id),
  FOREIGN KEY (tenant_id, counter_id) REFERENCES counters (tenant_id, id),
  FOREIGN KEY (tenant_id, tenant_service_id) REFERENCES tenant_services (tenant_id, id)
);

-- Nomor urut dialokasikan di sini, di-lock saat check-in. Bukan MAX+1.
CREATE TABLE ticket_sequences (
  tenant_id uuid NOT NULL,
  site_id uuid NOT NULL,
  tenant_service_id uuid NOT NULL,
  service_date date NOT NULL,
  last_no integer NOT NULL DEFAULT 0 CHECK (last_no >= 0),
  PRIMARY KEY (tenant_id, site_id, tenant_service_id, service_date),
  FOREIGN KEY (tenant_id, site_id) REFERENCES sites (tenant_id, id),
  FOREIGN KEY (tenant_id, tenant_service_id) REFERENCES tenant_services (tenant_id, id)
);

CREATE TABLE queue_tickets (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL,
  booking_id uuid,
  tenant_service_id uuid NOT NULL,
  site_id uuid NOT NULL,
  service_date date NOT NULL,
  sequence_no integer NOT NULL CHECK (sequence_no > 0),
  display_number text NOT NULL,
  status text NOT NULL CHECK (status IN ('WAITING','CALLED','SERVING','COMPLETED','NO_SHOW','CANCELLED')),
  revision integer NOT NULL DEFAULT 1,
  recall_count integer NOT NULL DEFAULT 0,
  called_counter_id uuid,
  checked_in_at timestamptz NOT NULL DEFAULT now(),
  first_called_at timestamptz,
  called_at timestamptz,
  started_at timestamptz,
  completed_at timestamptz,
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, booking_id),
  UNIQUE (tenant_id, site_id, tenant_service_id, service_date, sequence_no),
  FOREIGN KEY (tenant_id, booking_id) REFERENCES bookings (tenant_id, id),
  FOREIGN KEY (tenant_id, tenant_service_id) REFERENCES tenant_services (tenant_id, id),
  FOREIGN KEY (tenant_id, site_id) REFERENCES sites (tenant_id, id),
  FOREIGN KEY (tenant_id, called_counter_id) REFERENCES counters (tenant_id, id)
);
CREATE INDEX queue_wait ON queue_tickets (tenant_id, site_id, status, service_date, checked_in_at, sequence_no);

-- Maksimal satu tiket aktif (CALLED/SERVING) per loket.
CREATE UNIQUE INDEX one_active_ticket_per_counter
  ON queue_tickets (tenant_id, called_counter_id) WHERE status IN ('CALLED','SERVING');

CREATE TABLE devices (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL,
  site_id uuid NOT NULL,
  device_type text NOT NULL CHECK (device_type IN ('KIOSK','OPERATOR','DISPLAY','ANNOUNCER')),
  label text NOT NULL,
  audio_zone_id uuid,
  counter_id uuid,
  status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','ACTIVE','REVOKED')),
  last_seen_at timestamptz,
  app_version text,
  health jsonb NOT NULL DEFAULT '{}'::jsonb,       -- hanya status perangkat; tanpa QR/PII
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id),
  CHECK (device_type <> 'ANNOUNCER' OR audio_zone_id IS NOT NULL),
  FOREIGN KEY (tenant_id, site_id) REFERENCES sites (tenant_id, id),
  FOREIGN KEY (tenant_id, audio_zone_id) REFERENCES audio_zones (tenant_id, id),
  FOREIGN KEY (tenant_id, counter_id) REFERENCES counters (tenant_id, id)
);

-- Kredensial & aktivasi perangkat: dicari SEBELUM tenant diketahui, jadi bukan tabel RLS.
-- Hanya menyimpan hash; tidak ada PII.
CREATE TABLE device_credentials (
  device_id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  secret_hash bytea NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  revoked_at timestamptz
);
CREATE TABLE device_activations (
  code_hash bytea PRIMARY KEY,
  device_id uuid NOT NULL,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  expires_at timestamptz NOT NULL,
  used_at timestamptz
);

CREATE TABLE checkins (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL,
  booking_id uuid NOT NULL,
  ticket_id uuid NOT NULL,
  device_id uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, booking_id),
  FOREIGN KEY (tenant_id, booking_id) REFERENCES bookings (tenant_id, id),
  FOREIGN KEY (tenant_id, ticket_id) REFERENCES queue_tickets (tenant_id, id),
  FOREIGN KEY (tenant_id, device_id) REFERENCES devices (tenant_id, id)
);

-- Event antrean tersanitasi (tanpa nama/NIK/HP). id = cursor SSE (Last-Event-ID).
CREATE TABLE queue_events (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  tenant_id uuid NOT NULL,
  site_id uuid NOT NULL,
  ticket_id uuid,
  counter_id uuid,
  event_type text NOT NULL,
  payload jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id, site_id) REFERENCES sites (tenant_id, id)
);
CREATE INDEX queue_events_site ON queue_events (tenant_id, site_id, id);

CREATE TABLE queue_announcements (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL,
  site_id uuid NOT NULL,
  audio_zone_id uuid NOT NULL,
  ticket_id uuid NOT NULL,
  counter_id uuid NOT NULL,
  call_event_id bigint NOT NULL,
  kind text NOT NULL DEFAULT 'CALL' CHECK (kind IN ('CALL','RECALL')),
  revision integer NOT NULL DEFAULT 1,
  state text NOT NULL DEFAULT 'PENDING'
    CHECK (state IN ('PENDING','CLAIMED','PLAYING','PLAYED','FAILED','CANCELLED','EXPIRED')),
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  leased_by_device_id uuid,
  fencing_token bigint,
  lease_until timestamptz,
  started_at timestamptz,
  finished_at timestamptz,
  playback_error_code text,
  attempt_count integer NOT NULL DEFAULT 0,
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, audio_zone_id, call_event_id, revision),
  FOREIGN KEY (tenant_id, audio_zone_id) REFERENCES audio_zones (tenant_id, id),
  FOREIGN KEY (tenant_id, ticket_id) REFERENCES queue_tickets (tenant_id, id),
  FOREIGN KEY (tenant_id, counter_id) REFERENCES counters (tenant_id, id),
  FOREIGN KEY (tenant_id, leased_by_device_id) REFERENCES devices (tenant_id, id)
);
CREATE INDEX announcements_zone_state ON queue_announcements (tenant_id, audio_zone_id, state, created_at);

CREATE TABLE outbox_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL,
  topic text NOT NULL,
  payload jsonb NOT NULL,                          -- tanpa PII, tanpa kalimat suara
  created_at timestamptz NOT NULL DEFAULT now(),
  available_at timestamptz NOT NULL DEFAULT now(),
  attempts integer NOT NULL DEFAULT 0,
  last_error text,
  processed_at timestamptz
);
CREATE INDEX outbox_pending ON outbox_events (available_at) WHERE processed_at IS NULL;

CREATE TABLE idempotency_keys (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  principal_key text NOT NULL,
  endpoint_key text NOT NULL,
  key_hash bytea NOT NULL,
  request_hash bytea NOT NULL,
  status_code integer,
  response_body jsonb,
  response_secret_encrypted bytea,                 -- token QR terenkripsi agar replay tidak menyimpan plaintext
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  UNIQUE (tenant_id, principal_key, endpoint_key, key_hash)
);

-- Metrik agregat tanpa PII; satu-satunya data yang dibaca role sib_reporting.
CREATE TABLE daily_service_metrics (
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  site_code text NOT NULL,
  service_code text NOT NULL,
  service_date date NOT NULL,
  booking_count integer NOT NULL DEFAULT 0,
  arrived_count integer NOT NULL DEFAULT 0,
  waiting_count integer NOT NULL DEFAULT 0,
  called_count integer NOT NULL DEFAULT 0,
  served_count integer NOT NULL DEFAULT 0,
  no_show_count integer NOT NULL DEFAULT 0,
  cancelled_count integer NOT NULL DEFAULT 0,
  avg_wait_seconds numeric,
  p95_wait_seconds numeric,
  data_freshness_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, site_code, service_code, service_date)
);
CREATE TABLE tenant_health_snapshots (
  tenant_id uuid PRIMARY KEY REFERENCES tenants(id),
  devices_total integer NOT NULL DEFAULT 0,
  devices_online integer NOT NULL DEFAULT 0,
  announcers_online integer NOT NULL DEFAULT 0,
  announcement_failures_24h integer NOT NULL DEFAULT 0,
  as_of timestamptz NOT NULL
);
