package api

// Tes HTTP end-to-end terhadap PostgreSQL nyata. TEST_DATABASE_URL harus menunjuk ke server dengan hak CREATE DATABASE.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/D4marp/SiBatang/backend/internal/config"
	"github.com/D4marp/SiBatang/backend/internal/crypto"
	"github.com/D4marp/SiBatang/backend/internal/db"
	"github.com/D4marp/SiBatang/backend/internal/service"
	"github.com/D4marp/SiBatang/backend/migrations"
)

type harness struct {
	t     *testing.T
	ctx   context.Context
	admin *pgxpool.Pool
	ts    *httptest.Server
	svc   *service.Service
}

func newHarness(t *testing.T) *harness {
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	root, _ := pgxpool.New(ctx, base)
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	name := "sibapi_" + hex.EncodeToString(b)
	if _, err := root.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(base)
	u.Path = "/" + name
	admin, _ := pgxpool.New(ctx, u.String())
	if _, err := db.Migrate(ctx, admin, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if err := db.SyncRoles(ctx, admin); err != nil {
		t.Fatal(err)
	}
	d, err := db.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	cfg := config.Config{DefaultPublicTenant: "SIBATANG_BATANG", RootOrgCode: "POLDA_JAWA_TENGAH", SessionCookieName: "sib_session",
		PilotPublishAllowlist: []string{"SIBATANG_BATANG"}, DataEncryptionKey: key, PublicRateLimitPerMin: 1000,
		AnnouncementLease: 30 * time.Second, SSEHeartbeat: 20 * time.Second}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, _ := service.New(cfg, d, log)
	ts := httptest.NewServer(New(svc, d, cfg, log).Handler())
	t.Cleanup(func() {
		ts.Close()
		d.Close()
		admin.Close()
		_, _ = root.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		root.Close()
	})
	return &harness{t: t, ctx: ctx, admin: admin, ts: ts, svc: svc}
}

func (h *harness) str(q string, a ...any) string {
	var s string
	if err := h.admin.QueryRow(h.ctx, q, a...).Scan(&s); err != nil {
		h.t.Fatalf("%q: %v", q, err)
	}
	return s
}

type session struct{ cookie, csrf string }

func (h *harness) login(role, scope, scopeID, tenantID string) session {
	uid := h.str(`INSERT INTO users (oidc_subject) VALUES ($1) RETURNING id::text`, crypto.RandomToken())
	if _, err := h.admin.Exec(h.ctx, `INSERT INTO memberships (user_id, role_id, scope_type, scope_id, tenant_id)
		SELECT $1, r.id, $3, $4, NULLIF($5,'')::uuid FROM roles r WHERE r.code=$2`, uid, role, scope, scopeID, tenantID); err != nil {
		h.t.Fatal(err)
	}
	tok, csrf, err := h.svc.CreateSession(h.ctx, uid, time.Hour)
	if err != nil {
		h.t.Fatal(err)
	}
	return session{tok, csrf}
}

func (h *harness) do(method, path string, body any, hdr map[string]string, s *session, csrf bool) (int, http.Header, map[string]any) {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, h.ts.URL+"/api/v1"+path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	if s != nil {
		req.AddCookie(&http.Cookie{Name: "sib_session", Value: s.cookie})
		if csrf {
			req.Header.Set("X-CSRF-Token", s.csrf)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, resp.Header, out
}

func TestHTTPAuthCSRFAndProblemFormat(t *testing.T) {
	h := newHarness(t)
	tenant := h.str(`SELECT id::text FROM tenants WHERE code='SIBATANG_BATANG'`)

	// tenant belum PUBLISHED: katalog publik tidak ada
	code, hdr, body := h.do("GET", "/public/tenants/sibatang-batang/catalog", nil, nil, nil, false)
	if code != 404 || !strings.HasPrefix(hdr.Get("Content-Type"), "application/problem+json") || body["code"] != "NOT_FOUND" || body["request_id"] == "" {
		t.Fatalf("unpublished: %d %v", code, body)
	}
	h.admin.Exec(h.ctx, `UPDATE tenants SET status='PUBLISHED'`)
	h.admin.Exec(h.ctx, `UPDATE sites SET status='PUBLISHED'`)
	h.admin.Exec(h.ctx, `UPDATE tenant_services SET status='PUBLISHED'`)

	code, _, body = h.do("GET", "/public/tenants/sibatang-batang/catalog", nil, nil, nil, false)
	data, _ := body["data"].(map[string]any)
	if code != 200 || data["brand_name"] != "SIBATANG" {
		t.Fatalf("catalog: %d %v", code, body)
	}
	if code, _, _ = h.do("GET", "/public/tenants/polres-lain/catalog", nil, nil, nil, false); code != 404 {
		t.Fatalf("other tenant slug must 404: %d", code)
	}
	if code, _, body = h.do("POST", "/public/bookings", map[string]any{"tenant_code": "SIBATANG_BATANG"}, nil, nil, false); code != 400 || body["code"] != "VALIDATION_ERROR" {
		t.Fatalf("missing idem key: %d %v", code, body)
	}

	// staf: tanpa sesi 401; sesi tanpa CSRF 403; dengan CSRF lolos ke otorisasi
	site := h.str(`SELECT id::text FROM sites WHERE code='SITE_SPKT_SKCK'`)
	path := "/staff/sites/" + site + "/audio-zones"
	if code, _, body = h.do("POST", path, map[string]any{"label": "Zona A"}, nil, nil, false); code != 401 || body["code"] != "AUTH_REQUIRED" {
		t.Fatalf("no session: %d %v", code, body)
	}
	admin := h.login("POLRES_ADMIN", "TENANT", tenant, tenant)
	if code, _, body = h.do("POST", path, map[string]any{"label": "Zona A"}, nil, &admin, false); code != 403 {
		t.Fatalf("missing CSRF must be 403: %d %v", code, body)
	}
	if code, _, body = h.do("POST", path, map[string]any{"label": "Zona A", "unknown_field": 1}, nil, &admin, true); code != 400 {
		t.Fatalf("unknown fields rejected: %d %v", code, body)
	}
	if code, _, body = h.do("POST", path, map[string]any{"label": "Zona A"}, nil, &admin, true); code != 201 {
		t.Fatalf("create zone: %d %v", code, body)
	}

	// Polda: admin Polres ditolak; monitor Polda diizinkan; onboarding menolak parent_id dari klien
	polda := h.str(`SELECT id::text FROM organizations WHERE code='POLDA_JAWA_TENGAH'`)
	if code, _, _ = h.do("GET", "/polda/jateng/metrics", nil, nil, &admin, false); code != 403 {
		t.Fatalf("tenant admin on polda: %d", code)
	}
	mon := h.login("POLDA_MONITOR", "POLDA_JATENG", polda, "")
	if code, _, body = h.do("GET", "/polda/jateng/metrics", nil, nil, &mon, false); code != 200 {
		t.Fatalf("monitor: %d %v", code, body)
	}
	pa := h.login("POLDA_ORG_ADMIN", "POLDA_JATENG", polda, "")
	if code, _, body = h.do("POST", "/polda/jateng/onboarding-requests", map[string]any{"org_code": "POLRES_X1", "org_name": "Polres X", "tenant_code": "X1_TENANT",
		"brand_name": "X", "booking_prefix": "XXX", "parent_id": polda}, nil, &pa, true); code != 400 {
		t.Fatalf("client-supplied parent must be rejected: %d %v", code, body)
	}
}

func TestHTTPDeviceFlowAndCitizenPrivacy(t *testing.T) {
	h := newHarness(t)
	tenant := h.str(`SELECT id::text FROM tenants WHERE code='SIBATANG_BATANG'`)
	h.admin.Exec(h.ctx, `UPDATE tenants SET status='PUBLISHED'`)
	h.admin.Exec(h.ctx, `UPDATE sites SET status='PUBLISHED'`)
	h.admin.Exec(h.ctx, `UPDATE tenant_services SET status='PUBLISHED'`)
	site := h.str(`SELECT id::text FROM sites WHERE code='SITE_SPKT_SKCK'`)
	ts := h.str(`SELECT ts.id::text FROM tenant_services ts JOIN services s ON s.id=ts.service_id WHERE s.code='SKCK'`)
	admin := h.login("POLRES_ADMIN", "TENANT", tenant, tenant)

	// kiosk: enroll -> activate -> kredensial
	code, _, body := h.do("POST", "/staff/sites/"+site+"/devices", map[string]any{"device_type": "KIOSK", "label": "Kiosk 1"}, nil, &admin, true)
	if code != 201 {
		t.Fatalf("enroll: %d %v", code, body)
	}
	act := body["data"].(map[string]any)["activation_code"].(string)
	code, _, body = h.do("POST", "/devices/activate", map[string]any{"activation_code": act}, nil, nil, false)
	if code != 200 {
		t.Fatalf("activate: %d %v", code, body)
	}
	dev := body["data"].(map[string]any)
	cred, did := dev["credential"].(string), dev["device_id"].(string)
	auth := map[string]string{"Authorization": "Device " + cred}

	// slot + booking publik
	var slot string
	slot = h.str(`INSERT INTO schedule_slots (tenant_id, tenant_service_id, service_date, starts_at, ends_at, capacity)
		SELECT tenant_id, id, (now() AT TIME ZONE 'Asia/Jakarta')::date, now()+interval '10 minutes', now()+interval '40 minutes', 3
		FROM tenant_services WHERE id=$1 RETURNING id::text`, ts)
	code, _, body = h.do("POST", "/public/bookings", map[string]any{"tenant_code": "SIBATANG_BATANG", "slot_id": slot, "service_id": ts,
		"applicant": map[string]any{"display_name": "Budi Santoso", "contact": "081234567890", "allow_voice_name": true}, "privacy_notice_version": "2026-10-v1"},
		map[string]string{"Idempotency-Key": "http-test-key-001"}, nil, false)
	if code != 201 {
		t.Fatalf("booking: %d %v", code, body)
	}
	bk := body["data"].(map[string]any)
	ref, tok := bk["reference"].(string), bk["qr_token"].(string)

	// kepemilikan: tanpa token -> 404, dengan token -> 200 tanpa nama/kontak
	if code, _, _ = h.do("GET", "/public/bookings/"+ref, nil, nil, nil, false); code != 404 {
		t.Fatalf("reference without token: %d", code)
	}
	code, _, body = h.do("GET", "/public/bookings/"+ref, nil, map[string]string{"X-Booking-Token": tok}, nil, false)
	raw, _ := json.Marshal(body)
	if code != 200 || strings.Contains(string(raw), "Budi") || strings.Contains(string(raw), "0812") {
		t.Fatalf("citizen view must not echo PII: %d %s", code, raw)
	}

	// check-in lewat HTTP; ulang = 200 reprint dengan nomor sama
	code, _, body = h.do("POST", "/devices/"+did+"/checkins", map[string]any{"qr_token": tok}, auth, nil, false)
	first := body["data"].(map[string]any)
	if code != 201 || first["display_number"] != "SKCK-001" {
		t.Fatalf("checkin: %d %v", code, body)
	}
	code, _, body = h.do("POST", "/devices/"+did+"/checkins", map[string]any{"qr_token": tok}, auth, nil, false)
	if code != 200 || body["data"].(map[string]any)["ticket_id"] != first["ticket_id"] || body["data"].(map[string]any)["is_reprint"] != true {
		t.Fatalf("replay: %d %v", code, body)
	}
	// tanpa kredensial / device id lain / kiosk mengakses display
	if code, _, _ = h.do("POST", "/devices/"+did+"/checkins", map[string]any{"qr_token": tok}, nil, nil, false); code != 401 {
		t.Fatalf("no device creds: %d", code)
	}
	if code, _, _ = h.do("POST", "/devices/00000000-0000-4000-8000-000000000001/checkins", map[string]any{"qr_token": tok}, auth, nil, false); code != 403 {
		t.Fatalf("other device path: %d", code)
	}
	if code, _, _ = h.do("GET", "/devices/"+did+"/display/snapshot", nil, auth, nil, false); code != 403 {
		t.Fatalf("kiosk cannot read display: %d", code)
	}
	// revoke -> kredensial langsung tidak berlaku
	if code, _, _ = h.do("POST", "/staff/devices/"+did+"/revoke", nil, nil, &admin, true); code != 204 {
		t.Fatalf("revoke: %d", code)
	}
	if code, _, _ = h.do("POST", "/devices/"+did+"/checkins", map[string]any{"qr_token": tok}, auth, nil, false); code != 401 {
		t.Fatalf("revoked device: %d", code)
	}
}
