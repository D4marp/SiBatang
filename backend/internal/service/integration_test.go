package service

// Integration tests terhadap PostgreSQL nyata. Jalankan dengan:
//   TEST_DATABASE_URL=postgres://postgres:test@localhost:54329/postgres go test ./... -race
// (user harus punya hak CREATE DATABASE; setiap tes memakai database baru + migrasi dari nol).

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/D4marp/SiBatang/backend/internal/config"
	"github.com/D4marp/SiBatang/backend/internal/db"
	"github.com/D4marp/SiBatang/backend/internal/domain"
	"github.com/D4marp/SiBatang/backend/migrations"
)

type env struct {
	t     *testing.T
	ctx   context.Context
	svc   *Service
	admin *pgxpool.Pool
	seq   int
}

func newEnv(t *testing.T) *env {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	name := "sibtest_" + hex.EncodeToString(b)
	if _, err := root.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(base)
	u.Path = "/" + name
	admin, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
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
	cfg := config.Config{DefaultPublicTenant: "SIBATANG_BATANG", RootOrgCode: "POLDA_JAWA_TENGAH",
		PilotPublishAllowlist: []string{"SIBATANG_BATANG"}, DataEncryptionKey: key,
		AnnouncementLease: 30 * time.Second, AnnouncementExpire: 90 * time.Second, MetricsRefreshInterval: time.Minute}
	svc, err := New(cfg, d, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		d.Close()
		admin.Close()
		_, _ = root.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		root.Close()
	})
	return &env{t: t, ctx: ctx, svc: svc, admin: admin}
}

func (e *env) exec(sql string, args ...any) {
	e.t.Helper()
	if _, err := e.admin.Exec(e.ctx, sql, args...); err != nil {
		e.t.Fatalf("exec %q: %v", sql, err)
	}
}
func (e *env) str(sql string, args ...any) string {
	e.t.Helper()
	var s string
	if err := e.admin.QueryRow(e.ctx, sql, args...).Scan(&s); err != nil {
		e.t.Fatalf("query %q: %v", sql, err)
	}
	return s
}
func (e *env) count(sql string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.admin.QueryRow(e.ctx, sql, args...).Scan(&n); err != nil {
		e.t.Fatalf("count %q: %v", sql, err)
	}
	return n
}

func (e *env) staff(role domain.Role, scope domain.ScopeType, scopeID, tenantID string) *StaffPrincipal {
	e.t.Helper()
	e.seq++
	uid := e.str(`INSERT INTO users (oidc_subject, display_name) VALUES ($1,$2) RETURNING id::text`, fmt.Sprintf("sub-%d", e.seq), "tester")
	e.exec(`INSERT INTO memberships (user_id, role_id, scope_type, scope_id, tenant_id)
		SELECT $1, r.id, $3, $4, NULLIF($5,'')::uuid FROM roles r WHERE r.code=$2`, uid, string(role), string(scope), scopeID, tenantID)
	tok, _, err := e.svc.CreateSession(e.ctx, uid, time.Hour)
	if err != nil {
		e.t.Fatal(err)
	}
	p, err := e.svc.AuthenticateSession(e.ctx, tok)
	if err != nil {
		e.t.Fatal(err)
	}
	return p
}

type batang struct {
	tenant, site, tsSKCK, tsSPKT, zone, c1, c2 string
	admin, op1, op2                            *StaffPrincipal
}

func (e *env) setupBatang() *batang {
	e.t.Helper()
	b := &batang{}
	b.tenant = e.str(`SELECT id::text FROM tenants WHERE code='SIBATANG_BATANG'`)
	e.exec(`UPDATE tenants SET status='PUBLISHED' WHERE id=$1`, b.tenant)
	e.exec(`UPDATE sites SET status='PUBLISHED' WHERE tenant_id=$1`, b.tenant)
	e.exec(`UPDATE tenant_services SET status='PUBLISHED' WHERE tenant_id=$1`, b.tenant)
	b.site = e.str(`SELECT id::text FROM sites WHERE code='SITE_SPKT_SKCK'`)
	b.tsSKCK = e.str(`SELECT ts.id::text FROM tenant_services ts JOIN services s ON s.id=ts.service_id WHERE s.code='SKCK'`)
	b.tsSPKT = e.str(`SELECT ts.id::text FROM tenant_services ts JOIN services s ON s.id=ts.service_id WHERE s.code='SPKT'`)
	b.admin = e.staff(domain.RolePolresAdmin, domain.ScopeTenant, b.tenant, b.tenant)
	var err error
	if b.zone, err = e.svc.CreateAudioZone(e.ctx, b.admin, b.site, ZoneRequest{Label: "Ruang Tunggu SPKT"}); err != nil {
		e.t.Fatal(err)
	}
	sm := e.staff(domain.RoleSiteManager, domain.ScopeSite, b.site, b.tenant)
	if b.c1, err = e.svc.CreateCounter(e.ctx, sm, b.site, CounterRequest{Label: "Loket 1", AudioZoneID: b.zone, ServiceIDs: []string{b.tsSKCK, b.tsSPKT}}); err != nil {
		e.t.Fatal(err)
	}
	if b.c2, err = e.svc.CreateCounter(e.ctx, sm, b.site, CounterRequest{Label: "Loket 2", AudioZoneID: b.zone, ServiceIDs: []string{b.tsSKCK}}); err != nil {
		e.t.Fatal(err)
	}
	b.op1 = e.staff(domain.RoleServiceOperator, domain.ScopeCounter, b.c1, b.tenant)
	b.op2 = e.staff(domain.RoleServiceOperator, domain.ScopeCounter, b.c2, b.tenant)
	if err := e.svc.AssignOperator(e.ctx, b.admin, b.c1, b.op1.UserID); err != nil {
		e.t.Fatal(err)
	}
	if err := e.svc.AssignOperator(e.ctx, b.admin, b.c2, b.op2.UserID); err != nil {
		e.t.Fatal(err)
	}
	return b
}

func (e *env) device(b *batang, typ, zone, counter string) (*DevicePrincipal, string) {
	e.t.Helper()
	enr, err := e.svc.EnrollDevice(e.ctx, b.admin, b.site, EnrollRequest{DeviceType: typ, Label: typ + " test", AudioZoneID: zone, CounterID: counter})
	if err != nil {
		e.t.Fatal(err)
	}
	act, err := e.svc.ActivateDevice(e.ctx, enr.ActivationCode)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.svc.ActivateDevice(e.ctx, enr.ActivationCode); err == nil {
		e.t.Fatal("activation code must be single use")
	}
	dp, err := e.svc.AuthenticateDevice(e.ctx, act.Credential)
	if err != nil {
		e.t.Fatal(err)
	}
	return dp, act.Credential
}

func (e *env) slot(ts string, start time.Time, capacity int) string {
	return e.str(`INSERT INTO schedule_slots (tenant_id, tenant_service_id, service_date, starts_at, ends_at, capacity)
		SELECT tenant_id, id, ($2::timestamptz AT TIME ZONE 'Asia/Jakarta')::date, $2, $2 + interval '30 minutes', $3
		FROM tenant_services WHERE id=$1 RETURNING id::text`, ts, start, capacity)
}

func (e *env) book(b *batang, ts, slot, name string, voice bool) *BookingResult {
	e.t.Helper()
	e.seq++
	r, _, err := e.svc.CreateBooking(e.ctx, fmt.Sprintf("idem-key-%d-%d", e.seq, time.Now().UnixNano()), BookingRequest{
		TenantCode: "SIBATANG_BATANG", SlotID: slot, ServiceID: ts,
		Applicant:            Applicant{DisplayName: name, Contact: "081200000000", AllowVoiceName: voice},
		PrivacyNoticeVersion: "2026-10-v1"})
	if err != nil {
		e.t.Fatalf("book: %v", err)
	}
	return r
}

func code(err error) string {
	var de *domain.Error
	if errors.As(err, &de) {
		return de.Code
	}
	if err == nil {
		return ""
	}
	return "OTHER:" + err.Error()
}

func soon() time.Time { return time.Now().Add(10 * time.Minute) }

// --- booking ---

func TestBookingLastSlotRaceAndIdempotency(t *testing.T) {
	e := newEnv(t)
	b := e.setupBatang()
	slot := e.slot(b.tsSKCK, soon(), 1)

	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, full := 0, 0
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := e.svc.CreateBooking(e.ctx, fmt.Sprintf("race-key-%02d-abcdef", i), BookingRequest{TenantCode: "SIBATANG_BATANG", SlotID: slot, ServiceID: b.tsSKCK,
				Applicant: Applicant{DisplayName: fmt.Sprintf("Warga %d", i), Contact: "08120000"}, PrivacyNoticeVersion: "v1"})
			mu.Lock()
			defer mu.Unlock()
			switch code(err) {
			case "":
				ok++
			case "SLOT_FULL":
				full++
			default:
				t.Errorf("unexpected: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if ok != 1 || full != 11 {
		t.Fatalf("ok=%d full=%d", ok, full)
	}
	if n := e.count(`SELECT booked_count FROM schedule_slots WHERE id=$1`, slot); n != 1 {
		t.Fatalf("booked_count=%d", n)
	}

	// idempotensi pada slot lain
	slot2 := e.slot(b.tsSKCK, soon().Add(time.Hour), 5)
	req := BookingRequest{TenantCode: "SIBATANG_BATANG", SlotID: slot2, ServiceID: b.tsSKCK, Applicant: Applicant{DisplayName: "Budi", Contact: "0812"}, PrivacyNoticeVersion: "v1"}
	r1, replay1, err := e.svc.CreateBooking(e.ctx, "same-idem-key-1", req)
	if err != nil || replay1 {
		t.Fatal(err, replay1)
	}
	r2, replay2, err := e.svc.CreateBooking(e.ctx, "same-idem-key-1", req)
	if err != nil || !replay2 || r2.BookingID != r1.BookingID || r2.QRToken != r1.QRToken {
		t.Fatalf("replay failed: %v %v", err, replay2)
	}
	req.Applicant.DisplayName = "Lain"
	if _, _, err := e.svc.CreateBooking(e.ctx, "same-idem-key-1", req); code(err) != "IDEMPOTENCY_KEY_REUSED" {
		t.Fatalf("want IDEMPOTENCY_KEY_REUSED got %v", err)
	}
	if n := e.count(`SELECT booked_count FROM schedule_slots WHERE id=$1`, slot2); n != 1 {
		t.Fatalf("idempotent retries must reserve once, got %d", n)
	}
	// QR token tidak pernah disimpan plaintext
	if n := e.count(`SELECT count(*) FROM idempotency_keys WHERE response_body::text LIKE '%'||$1||'%'`, r1.QRToken); n != 0 {
		t.Fatal("qr token leaked into idempotency response_body")
	}
}

func TestBookingOwnershipAndCancelReleasesOnce(t *testing.T) {
	e := newEnv(t)
	b := e.setupBatang()
	slot := e.slot(b.tsSKCK, soon(), 2)
	bk := e.book(b, b.tsSKCK, slot, "Siti", false)

	if _, err := e.svc.GetBooking(e.ctx, bk.Reference, "salah-token-salah-token-salah-token"); code(err) != "NOT_FOUND" {
		t.Fatalf("reference alone must not authorize: %v", err)
	}
	if d, err := e.svc.GetBooking(e.ctx, bk.Reference, bk.QRToken); err != nil || d.Status != "CONFIRMED" || d.Ticket != nil {
		t.Fatalf("owner read: %v %+v", err, d)
	}
	for i := 0; i < 2; i++ {
		if err := e.svc.CancelBooking(e.ctx, bk.Reference, bk.QRToken); err != nil {
			t.Fatal(err)
		}
	}
	if n := e.count(`SELECT booked_count FROM schedule_slots WHERE id=$1`, slot); n != 0 {
		t.Fatalf("cancel twice must release once, booked_count=%d", n)
	}
}

// --- check-in ---

func TestCheckinIssuesTicketExactlyOnce(t *testing.T) {
	e := newEnv(t)
	b := e.setupBatang()
	kiosk, _ := e.device(b, "KIOSK", "", "")
	slot := e.slot(b.tsSKCK, soon(), 10)
	bk1 := e.book(b, b.tsSKCK, slot, "Andi", false)
	bk2 := e.book(b, b.tsSKCK, slot, "Rini", false)

	var wg sync.WaitGroup
	results := make([]*CheckinResult, 8)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, err := e.svc.CheckIn(e.ctx, kiosk, kiosk.DeviceID, CheckinRequest{QRToken: bk1.QRToken})
			if err != nil {
				t.Errorf("checkin: %v", err)
				return
			}
			results[i] = r
		}(i)
	}
	wg.Wait()
	fresh := 0
	for _, r := range results {
		if r == nil || r.TicketID != results[0].TicketID || r.DisplayNumber != "SKCK-001" {
			t.Fatalf("all scans must return the same ticket SKCK-001: %+v", r)
		}
		if !r.IsReprint {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatalf("exactly one fresh issuance expected, got %d", fresh)
	}
	r2, err := e.svc.CheckIn(e.ctx, kiosk, kiosk.DeviceID, CheckinRequest{QRToken: bk2.QRToken})
	if err != nil || r2.DisplayNumber != "SKCK-002" {
		t.Fatalf("second booking should get SKCK-002: %v %+v", err, r2)
	}
	if n := e.count(`SELECT count(*) FROM queue_tickets`); n != 2 {
		t.Fatalf("tickets=%d", n)
	}
	// kiosk lain tenant/perangkat tidak boleh dipakai pada path perangkat lain
	if _, err := e.svc.CheckIn(e.ctx, kiosk, "00000000-0000-4000-8000-000000000000", CheckinRequest{QRToken: bk1.QRToken}); code(err) != "SCOPE_DENIED" {
		t.Fatalf("path device mismatch must be denied: %v", err)
	}
	// booking sudah check-in tidak bisa dibatalkan
	if err := e.svc.CancelBooking(e.ctx, bk1.Reference, bk1.QRToken); code(err) != "STATE_CONFLICT" {
		t.Fatalf("cancel after checkin: %v", err)
	}
}

func TestCheckinOutsideWindowAndWrongToken(t *testing.T) {
	e := newEnv(t)
	b := e.setupBatang()
	kiosk, _ := e.device(b, "KIOSK", "", "")
	slot := e.slot(b.tsSKCK, time.Now().Add(6*time.Hour), 3)
	bk := e.book(b, b.tsSKCK, slot, "Dewi", false)
	if _, err := e.svc.CheckIn(e.ctx, kiosk, kiosk.DeviceID, CheckinRequest{QRToken: bk.QRToken}); code(err) != "SERVICE_POLICY_NOT_MET" {
		t.Fatalf("too early: %v", err)
	}
	if _, err := e.svc.CheckIn(e.ctx, kiosk, kiosk.DeviceID, CheckinRequest{QRToken: strings.Repeat("x", 43)}); code(err) != "NOT_FOUND" {
		t.Fatalf("bad token: %v", err)
	}
}

// --- antrean & announcer ---

func (e *env) checkedIn(b *batang, kiosk *DevicePrincipal, name string, voice bool) *CheckinResult {
	e.t.Helper()
	slot := e.slot(b.tsSKCK, soon().Add(time.Duration(e.seq)*time.Minute), 5)
	bk := e.book(b, b.tsSKCK, slot, name, voice)
	r, err := e.svc.CheckIn(e.ctx, kiosk, kiosk.DeviceID, CheckinRequest{QRToken: bk.QRToken})
	if err != nil {
		e.t.Fatal(err)
	}
	return r
}

func TestCallNextTwoOperatorsOneTicket(t *testing.T) {
	e := newEnv(t)
	b := e.setupBatang()
	kiosk, _ := e.device(b, "KIOSK", "", "")
	e.checkedIn(b, kiosk, "Budi", false)
	for _, p := range []struct {
		op *StaffPrincipal
		c  string
	}{{b.op1, b.c1}, {b.op2, b.c2}} {
		if _, err := e.svc.CounterReady(e.ctx, p.op, p.c, nil); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	res := make([]*CallResult, 2)
	errs := make([]error, 2)
	for i, p := range []struct {
		op *StaffPrincipal
		c  string
	}{{b.op1, b.c1}, {b.op2, b.c2}} {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res[i], errs[i] = e.svc.CallNext(e.ctx, p.op, p.c)
		}(i)
	}
	wg.Wait()
	called := 0
	for i := range res {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		if res[i].TicketID != "" {
			called++
		}
	}
	if called != 1 {
		t.Fatalf("exactly one operator should get the ticket, got %d", called)
	}
	if n := e.count(`SELECT count(*) FROM queue_announcements`); n != 1 {
		t.Fatalf("exactly one announcement job expected, got %d", n)
	}
	// operator loket 1 tidak boleh memanggil dari loket 2
	if _, err := e.svc.CallNext(e.ctx, b.op1, b.c2); code(err) != "SCOPE_DENIED" {
		t.Fatalf("operator must be limited to assigned counter: %v", err)
	}
}

func TestAutoAdvanceOnlyWhenReadyAndStaffed(t *testing.T) {
	e := newEnv(t)
	b := e.setupBatang()
	kiosk, _ := e.device(b, "KIOSK", "", "")
	t1 := e.checkedIn(b, kiosk, "Eka", false)
	e.checkedIn(b, kiosk, "Fajar", false)
	on := true
	if err := e.svc.UpdateCounterPolicy(e.ctx, b.admin, b.c1, QueuePolicyRequest{AutoAdvanceOnReady: &on}); err != nil {
		t.Fatal(err)
	}
	// loket belum READY: call-next ditolak, tidak ada panggilan otomatis
	if _, err := e.svc.CallNext(e.ctx, b.op1, b.c1); code(err) != "STATE_CONFLICT" {
		t.Fatalf("closed counter cannot call: %v", err)
	}
	if n := e.count(`SELECT count(*) FROM queue_tickets WHERE status='CALLED'`); n != 0 {
		t.Fatal("nothing should be called yet")
	}
	// READY -> Go memanggil tiket pertama secara otomatis
	r, err := e.svc.CounterReady(e.ctx, b.op1, b.c1, nil)
	if err != nil || r.TicketID != t1.TicketID || r.CounterState != "CALLING" || r.AnnouncementStatus != "QUEUED" {
		t.Fatalf("auto advance: %v %+v", err, r)
	}
	// start -> complete -> auto-advance tiket berikutnya
	if _, err := e.svc.Start(e.ctx, b.op1, r.TicketID); err != nil {
		t.Fatal(err)
	}
	c, err := e.svc.Complete(e.ctx, b.op1, r.TicketID)
	if err != nil || c.TicketID == r.TicketID || c.CounterState != "CALLING" {
		t.Fatalf("complete should auto advance: %v %+v", err, c)
	}
	// operator lain (bukan yang ditugaskan) tidak bisa membuat loket READY
	if _, err := e.svc.CounterReady(e.ctx, b.op2, b.c1, nil); code(err) != "SCOPE_DENIED" {
		t.Fatalf("other operator: %v", err)
	}
	// PAUSED tidak auto-advance
	if _, err := e.svc.CounterReady(e.ctx, b.op2, b.c2, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.CounterPause(e.ctx, b.op2, b.c2); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.CallNext(e.ctx, b.op2, b.c2); code(err) != "STATE_CONFLICT" {
		t.Fatalf("paused cannot call: %v", err)
	}
}

func claimOne(t *testing.T, e *env, dp *DevicePrincipal) *AnnouncerClaim {
	t.Helper()
	c, err := e.svc.ClaimAnnouncement(e.ctx, dp, dp.DeviceID)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	return c
}

func TestAnnouncerNamePrivacyAndPolicy(t *testing.T) {
	e := newEnv(t)
	b := e.setupBatang()
	kiosk, _ := e.device(b, "KIOSK", "", "")
	ann, _ := e.device(b, "ANNOUNCER", b.zone, "")
	display, _ := e.device(b, "DISPLAY", "", "")
	e.checkedIn(b, kiosk, "Budi Santoso", true)
	if _, err := e.svc.CounterReady(e.ctx, b.op1, b.c1, nil); err != nil {
		t.Fatal(err)
	}
	call, err := e.svc.CallNext(e.ctx, b.op1, b.c1)
	if err != nil || call.TicketID == "" {
		t.Fatal(err, call)
	}
	c := claimOne(t, e, ann)
	if c == nil || c.Speech.PublicCallName != nil {
		t.Fatalf("NUMBER_ONLY must not return a name: %+v", c)
	}
	if got := domain.Utterance(c.Speech); got != "Nomor antrean S K C K, nol nol satu, silakan menuju loket satu." {
		t.Fatalf("utterance: %q", got)
	}
	if err := e.svc.AckAnnouncement(e.ctx, ann, ann.DeviceID, c.JobID, "completed", c.FencingToken, ""); err != nil {
		t.Fatal(err)
	}
	// ubah kebijakan zona -> panggilan ulang membawa nama (consent ada)
	if err := e.svc.UpdateAudioZonePolicy(e.ctx, b.admin, b.site, b.zone, "PREFERRED_NAME_IF_CONSENT"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	e.exec(`UPDATE counters SET recall_cooldown_seconds=0`)
	if _, err := e.svc.Recall(e.ctx, b.op1, call.TicketID); err != nil {
		t.Fatal(err)
	}
	c2 := claimOne(t, e, ann)
	if c2 == nil || c2.Speech.PublicCallName == nil || *c2.Speech.PublicCallName != "Budi" {
		t.Fatalf("expected Budi after policy change: %+v", c2)
	}

	// Nama tidak boleh muncul di mana pun selain klaim privat.
	for _, q := range []string{
		`SELECT count(*) FROM queue_events WHERE payload::text ILIKE '%budi%'`,
		`SELECT count(*) FROM outbox_events WHERE payload::text ILIKE '%budi%'`,
		`SELECT count(*) FROM audit_events WHERE action ILIKE '%budi%' OR resource_type ILIKE '%budi%'`,
		`SELECT count(*) FROM daily_service_metrics WHERE site_code ILIKE '%budi%'`,
		`SELECT count(*) FROM devices WHERE health::text ILIKE '%budi%'`,
	} {
		if n := e.count(q); n != 0 {
			t.Fatalf("name leaked: %s", q)
		}
	}
	snap, err := e.svc.DisplaySnapshot(e.ctx, display, display.DeviceID)
	if err != nil || len(snap.Now) != 1 || snap.Now[0].TicketNumber != "SKCK-001" {
		t.Fatalf("display snapshot: %v %+v", err, snap)
	}
	evs, err := e.svc.DisplayEventsSince(e.ctx, display, display.DeviceID, 0)
	if err != nil || len(evs) == 0 {
		t.Fatal(err)
	}
	for _, ev := range evs {
		if strings.Contains(strings.ToLower(string(ev.Payload)), "budi") {
			t.Fatal("name in public SSE payload")
		}
	}
}

func TestAnnouncerLeaseFencingAndSerialPlayback(t *testing.T) {
	e := newEnv(t)
	b := e.setupBatang()
	kiosk, _ := e.device(b, "KIOSK", "", "")
	annA, _ := e.device(b, "ANNOUNCER", b.zone, "")
	annB, _ := e.device(b, "ANNOUNCER", b.zone, "")
	e.checkedIn(b, kiosk, "Gita", false)
	e.checkedIn(b, kiosk, "Hadi", false)
	for _, x := range []struct {
		op *StaffPrincipal
		c  string
	}{{b.op1, b.c1}, {b.op2, b.c2}} {
		if _, err := e.svc.CounterReady(e.ctx, x.op, x.c, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.CallNext(e.ctx, x.op, x.c); err != nil {
			t.Fatal(err)
		}
	}
	// dua announcer pada zona sama: satu aktif, yang lain standby
	cA := claimOne(t, e, annA)
	if cA == nil {
		t.Fatal("A should claim")
	}
	if _, err := e.svc.ClaimAnnouncement(e.ctx, annB, annB.DeviceID); code(err) != "ZONE_LEASED_BY_OTHER" {
		t.Fatalf("standby must not claim: %v", err)
	}
	// berurutan: job kedua tidak keluar selama yang pertama belum selesai
	if again := claimOne(t, e, annA); again != nil {
		t.Fatal("second job must wait for first to finish (serial playback)")
	}
	if err := e.svc.AckAnnouncement(e.ctx, annA, annA.DeviceID, cA.JobID, "started", cA.FencingToken, ""); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.AckAnnouncement(e.ctx, annA, annA.DeviceID, cA.JobID, "started", cA.FencingToken, ""); err != nil {
		t.Fatalf("repeat ack must be idempotent: %v", err)
	}
	// announcer A hilang; lease habis; B mengambil alih; ack lama A ditolak (fencing)
	e.exec(`UPDATE audio_zones SET lease_until = now() - interval '1 second'`)
	e.exec(`UPDATE queue_announcements SET lease_until = now() - interval '1 second' WHERE state IN ('CLAIMED','PLAYING')`)
	cB := claimOne(t, e, annB)
	if cB == nil || cB.FencingToken <= cA.FencingToken {
		t.Fatalf("takeover must bump fencing token: %+v", cB)
	}
	if err := e.svc.AckAnnouncement(e.ctx, annA, annA.DeviceID, cA.JobID, "completed", cA.FencingToken, ""); code(err) != "STALE_FENCING_TOKEN" && code(err) != "NOT_FOUND" {
		// job A basi menjadi FAILED(PLAYBACK_UNKNOWN); ack lama tidak boleh sukses
		if err == nil {
			t.Fatal("stale ack must not succeed")
		}
	}
	if n := e.count(`SELECT count(*) FROM queue_announcements WHERE state='FAILED' AND playback_error_code='PLAYBACK_UNKNOWN'`); n != 1 {
		t.Fatalf("stale playing job should be failed, not replayed: %d", n)
	}
	// ticket tidak berubah karena audio gagal
	if n := e.count(`SELECT count(*) FROM queue_tickets WHERE status='CALLED'`); n != 2 {
		t.Fatalf("audio failure must not alter tickets, called=%d", n)
	}
}

func TestAnnouncerWrongZoneAndRevoked(t *testing.T) {
	e := newEnv(t)
	b := e.setupBatang()
	kiosk, _ := e.device(b, "KIOSK", "", "")
	zoneB, err := e.svc.CreateAudioZone(e.ctx, b.admin, b.site, ZoneRequest{Label: "Zona B"})
	if err != nil {
		t.Fatal(err)
	}
	annA, credA := e.device(b, "ANNOUNCER", b.zone, "")
	annB, _ := e.device(b, "ANNOUNCER", zoneB, "")
	e.checkedIn(b, kiosk, "Indah", false)
	if _, err := e.svc.CounterReady(e.ctx, b.op1, b.c1, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.CallNext(e.ctx, b.op1, b.c1); err != nil {
		t.Fatal(err)
	}
	if c := claimOne(t, e, annB); c != nil {
		t.Fatal("zone B must not see zone A job")
	}
	job := e.str(`SELECT id::text FROM queue_announcements LIMIT 1`)
	if err := e.svc.AckAnnouncement(e.ctx, annB, annB.DeviceID, job, "completed", 1, ""); code(err) != "NOT_FOUND" {
		t.Fatalf("wrong zone ack: %v", err)
	}
	// kiosk tidak boleh mengklaim suara
	if _, err := e.svc.ClaimAnnouncement(e.ctx, kiosk, kiosk.DeviceID); code(err) != "SCOPE_DENIED" {
		t.Fatalf("kiosk claim: %v", err)
	}
	// perangkat dicabut -> otentikasi gagal segera
	if err := e.svc.RevokeDevice(e.ctx, b.admin, annA.DeviceID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AuthenticateDevice(e.ctx, credA); code(err) != "AUTH_REQUIRED" {
		t.Fatalf("revoked device: %v", err)
	}
}

func TestRecallCooldownAndMax(t *testing.T) {
	e := newEnv(t)
	b := e.setupBatang()
	kiosk, _ := e.device(b, "KIOSK", "", "")
	e.checkedIn(b, kiosk, "Joko", false)
	if _, err := e.svc.CounterReady(e.ctx, b.op1, b.c1, nil); err != nil {
		t.Fatal(err)
	}
	call, _ := e.svc.CallNext(e.ctx, b.op1, b.c1)
	if _, err := e.svc.Recall(e.ctx, b.op1, call.TicketID); code(err) != "RATE_LIMITED" {
		t.Fatalf("cooldown: %v", err)
	}
	e.exec(`UPDATE counters SET recall_cooldown_seconds=0, max_recall_per_ticket=1`)
	if _, err := e.svc.Recall(e.ctx, b.op1, call.TicketID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Recall(e.ctx, b.op1, call.TicketID); code(err) != "SERVICE_POLICY_NOT_MET" {
		t.Fatalf("max recall: %v", err)
	}
	if n := e.count(`SELECT count(*) FROM queue_announcements`); n != 2 {
		t.Fatalf("call + 1 recall = 2 jobs, got %d", n)
	}
}

func TestNoShowCancelsPendingAnnouncement(t *testing.T) {
	e := newEnv(t)
	b := e.setupBatang()
	kiosk, _ := e.device(b, "KIOSK", "", "")
	e.checkedIn(b, kiosk, "Kiki", false)
	_, _ = e.svc.CounterReady(e.ctx, b.op1, b.c1, nil)
	call, _ := e.svc.CallNext(e.ctx, b.op1, b.c1)
	if _, err := e.svc.NoShow(e.ctx, b.op1, call.TicketID); err != nil {
		t.Fatal(err)
	}
	if n := e.count(`SELECT count(*) FROM queue_announcements WHERE state='CANCELLED'`); n != 1 {
		t.Fatalf("announcement should be cancelled: %d", n)
	}
	if _, err := e.svc.Start(e.ctx, b.op1, call.TicketID); code(err) != "STATE_CONFLICT" {
		t.Fatalf("terminal ticket: %v", err)
	}
}

// --- isolasi tenant / RLS ---

func (e *env) setupUjiB() string {
	e.exec(`INSERT INTO organizations (code, org_type, display_name, parent_id, status)
		SELECT 'POLRES_UJI_B','POLRES','Polres Uji B (staging)',id,'ACTIVE' FROM organizations WHERE code='POLDA_JAWA_TENGAH'`)
	e.exec(`INSERT INTO tenants (owning_org_id, code, brand_name, booking_prefix, status)
		SELECT id,'UJI_B_TENANT','Uji B','UJB','PUBLISHED' FROM organizations WHERE code='POLRES_UJI_B'`)
	tb := e.str(`SELECT id::text FROM tenants WHERE code='UJI_B_TENANT'`)
	e.exec(`INSERT INTO sites (tenant_id, code, name, status) VALUES ($1,'SITE_B','Site B','PUBLISHED')`, tb)
	e.exec(`INSERT INTO tenant_services (tenant_id, site_id, service_id, status)
		SELECT $1, s.id, sv.id, 'PUBLISHED' FROM sites s, services sv WHERE s.tenant_id=$1 AND sv.code='SKCK'`, tb)
	return tb
}

func TestTenantIsolation(t *testing.T) {
	e := newEnv(t)
	b := e.setupBatang()
	tb := e.setupUjiB()
	adminB := e.staff(domain.RolePolresAdmin, domain.ScopeTenant, tb, tb)
	siteB := e.str(`SELECT id::text FROM sites WHERE tenant_id=$1`, tb)
	e.slot(b.tsSKCK, soon(), 3)

	// staf Batang tak dapat menyentuh sumber daya B (404, tidak terungkap) dan sebaliknya
	if _, err := e.svc.CreateAudioZone(e.ctx, b.admin, siteB, ZoneRequest{Label: "Intrusi"}); code(err) != "NOT_FOUND" {
		t.Fatalf("A admin on B site: %v", err)
	}
	if _, err := e.svc.CreateAudioZone(e.ctx, adminB, b.site, ZoneRequest{Label: "Intrusi"}); code(err) != "NOT_FOUND" {
		t.Fatalf("B admin on A site: %v", err)
	}
	if _, err := e.svc.GetCounterQueue(e.ctx, adminB, b.c1); code(err) != "NOT_FOUND" {
		t.Fatalf("B on A counter: %v", err)
	}
	// RLS di level DB: tanpa konteks tenant tidak ada baris; konteks A tidak melihat B; WITH CHECK menolak tulis lintas tenant
	_ = e.svc.db.WithPlain(e.ctx, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(e.ctx, `SELECT count(*) FROM schedule_slots`).Scan(&n); err != nil || n != 0 {
			t.Fatalf("no tenant context must see 0 rows: %d %v", n, err)
		}
		return nil
	})
	_ = e.svc.db.WithTenant(e.ctx, b.tenant, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(e.ctx, `SELECT count(*) FROM sites WHERE tenant_id=$1`, tb).Scan(&n); err != nil || n != 0 {
			t.Fatalf("tenant A must not see tenant B sites: %d %v", n, err)
		}
		_, err := tx.Exec(e.ctx, `INSERT INTO sites (tenant_id, code, name) VALUES ($1,'X','X')`, tb)
		if err == nil {
			t.Fatal("cross-tenant insert must violate RLS")
		}
		return errors.New("rollback")
	})
	// runtime role bukan superuser/BYPASSRLS
	for _, role := range []string{"sib_api", "sib_reporting"} {
		if n := e.count(`SELECT count(*) FROM pg_roles WHERE rolname=$1 AND (rolsuper OR rolbypassrls OR rolcanlogin)`, role); n != 0 {
			t.Fatalf("%s must be NOSUPERUSER NOBYPASSRLS NOLOGIN", role)
		}
	}
}

func TestTenantTableInventory(t *testing.T) {
	e := newEnv(t)
	rows, err := e.admin.Query(e.ctx, `SELECT c.relname, c.relrowsecurity, c.relforcerowsecurity
		FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
		JOIN information_schema.columns col ON col.table_name=c.relname AND col.table_schema='public' AND col.column_name='tenant_id'
		WHERE n.nspname='public' AND c.relkind='r'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	// tabel auth/pencarian-sebelum-tenant yang sengaja tanpa RLS (hanya hash/metadata)
	allow := map[string]bool{"memberships": true, "device_credentials": true, "device_activations": true}
	for rows.Next() {
		var name string
		var rls, force bool
		if err := rows.Scan(&name, &rls, &force); err != nil {
			t.Fatal(err)
		}
		if allow[name] {
			continue
		}
		if !rls || !force {
			t.Errorf("tenant-owned table %s lacks forced RLS", name)
		}
	}
}

// --- Polda ---

func TestPoldaAggregateOnly(t *testing.T) {
	e := newEnv(t)
	b := e.setupBatang()
	polda := e.str(`SELECT id::text FROM organizations WHERE code='POLDA_JAWA_TENGAH'`)
	monitor := e.staff(domain.RolePoldaMonitor, domain.ScopePoldaJateng, polda, "")
	kiosk, _ := e.device(b, "KIOSK", "", "")
	e.checkedIn(b, kiosk, "Lina", true)
	_, _ = e.svc.CounterReady(e.ctx, b.op1, b.c1, nil)
	call, _ := e.svc.CallNext(e.ctx, b.op1, b.c1)

	// sebelum worker berjalan: data BELUM tersedia (bukan 0)
	m0, err := e.svc.PoldaMetrics(e.ctx, monitor)
	if err != nil || len(m0.Polres) != 1 || m0.Polres[0].DataAvailable {
		t.Fatalf("expected one polres without data: %v %+v", err, m0)
	}
	if err := e.svc.RefreshMetrics(e.ctx); err != nil {
		t.Fatal(err)
	}
	m, err := e.svc.PoldaMetrics(e.ctx, monitor)
	if err != nil || m.ActivePolresCount != 1 || m.Polres[0].TenantCode != "SIBATANG_BATANG" || !m.Polres[0].DataAvailable || !m.Polres[0].IsFresh {
		t.Fatalf("metrics: %v %+v", err, m)
	}
	if m.Polres[0].CalledCount != 1 || m.Polres[0].BookingCount != 1 || m.Polres[0].ArrivedCount != 1 {
		t.Fatalf("counts: %+v", m.Polres[0])
	}
	if n := e.count(`SELECT count(*) FROM audit_events WHERE action='report.aggregate.read' AND tenant_id IS NULL`); n != 2 {
		t.Fatalf("polda reads must be audited: %d", n)
	}

	// Polda tidak punya wewenang antrean / checkin / klaim suara
	if _, err := e.svc.CallNext(e.ctx, monitor, b.c1); code(err) != "NOT_FOUND" {
		t.Fatalf("polda call-next: %v", err)
	}
	if _, err := e.svc.Complete(e.ctx, monitor, call.TicketID); code(err) != "NOT_FOUND" {
		t.Fatalf("polda complete: %v", err)
	}
	if _, err := e.svc.CreateOnboardingRequest(e.ctx, monitor, OnboardingInput{OrgCode: "POLRES_X1", OrgName: "Polres X", TenantCode: "X1_TENANT", BrandName: "X", BookingPrefix: "XXX"}); code(err) != "SCOPE_DENIED" {
		t.Fatalf("monitor cannot onboard: %v", err)
	}
	// staf Polres bukan Polda
	if _, err := e.svc.PoldaMetrics(e.ctx, b.admin); code(err) != "SCOPE_DENIED" {
		t.Fatalf("tenant admin on polda metrics: %v", err)
	}
	// role pelaporan tidak punya grant ke tabel warga/operasional
	for _, tbl := range []string{"bookings", "queue_tickets", "queue_announcements", "devices", "checkins", "audit_events"} {
		err := e.svc.db.WithReporting(e.ctx, func(tx pgx.Tx) error {
			var n int
			return tx.QueryRow(e.ctx, "SELECT count(*) FROM "+tbl).Scan(&n)
		})
		if err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("sib_reporting must not read %s: %v", tbl, err)
		}
	}
	// sesi dicabut -> otentikasi gagal
	tok, _, _ := e.svc.CreateSession(e.ctx, monitor.UserID, time.Hour)
	if _, err := e.svc.AuthenticateSession(e.ctx, tok); err != nil {
		t.Fatal(err)
	}
	e.exec(`UPDATE memberships SET revoked_at=now() WHERE user_id=$1`, monitor.UserID)
	re, err := e.svc.AuthenticateSession(e.ctx, tok)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.PoldaMetrics(e.ctx, re); code(err) != "SCOPE_DENIED" {
		t.Fatalf("revoked monitor: %v", err)
	}
}

func TestOnboardingJatengOnly(t *testing.T) {
	e := newEnv(t)
	polda := e.str(`SELECT id::text FROM organizations WHERE code='POLDA_JAWA_TENGAH'`)
	a1 := e.staff(domain.RolePoldaOrgAdmin, domain.ScopePoldaJateng, polda, "")
	a2 := e.staff(domain.RolePoldaOrgAdmin, domain.ScopePoldaJateng, polda, "")
	req, err := e.svc.CreateOnboardingRequest(e.ctx, a1, OnboardingInput{OrgCode: "POLRES_PEKALONGAN", OrgName: "Polres Pekalongan", TenantCode: "PEKALONGAN_QUEUE", BrandName: "Antre Pekalongan", BookingPrefix: "PKL"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.DecideOnboarding(e.ctx, a1, req.ID, true, ""); code(err) != "SCOPE_DENIED" {
		t.Fatalf("requester cannot self-approve: %v", err)
	}
	if err := e.svc.DecideOnboarding(e.ctx, a2, req.ID, true, "ok"); err != nil {
		t.Fatal(err)
	}
	if e.str(`SELECT t.status FROM tenants t WHERE t.code='PEKALONGAN_QUEUE'`) != "DRAFT" {
		t.Fatal("new tenant must be DRAFT")
	}
	if e.str(`SELECT p.code FROM organizations o JOIN organizations p ON p.id=o.parent_id WHERE o.code='POLRES_PEKALONGAN'`) != "POLDA_JAWA_TENGAH" {
		t.Fatal("parent must be Jateng root")
	}
	// tenant DRAFT tidak tampil di dashboard Polda (hanya PUBLISHED) dan tidak dipublikasikan otomatis
	m, err := e.svc.PoldaMetrics(e.ctx, a1)
	if err != nil || m.ActivePolresCount != 0 {
		t.Fatalf("draft tenant must not be counted: %v %+v", err, m)
	}
	if err := e.svc.DecideOnboarding(e.ctx, a2, req.ID, true, ""); code(err) != "STATE_CONFLICT" {
		t.Fatalf("double decision: %v", err)
	}
	// publikasi di luar allowlist pilot ditolak
	tid := e.str(`SELECT id::text FROM tenants WHERE code='PEKALONGAN_QUEUE'`)
	if err := e.svc.PublishTenant(e.ctx, a2, tid); code(err) != "SERVICE_POLICY_NOT_MET" {
		t.Fatalf("publish gate: %v", err)
	}
	// validasi domain: bukan POLRES_*, uji B, kode ganda
	if _, err := e.svc.CreateOnboardingRequest(e.ctx, a1, OnboardingInput{OrgCode: "POLDA_JAWA_BARAT", OrgName: "Polda lain", TenantCode: "ZZZ_TENANT", BrandName: "ZZZ", BookingPrefix: "ZZZ"}); code(err) != "VALIDATION_ERROR" {
		t.Fatalf("non-polres code: %v", err)
	}
	if _, err := e.svc.CreateOnboardingRequest(e.ctx, a1, OnboardingInput{OrgCode: "POLRES_PEKALONGAN", OrgName: "Polres Pekalongan", TenantCode: "PEKALONGAN_QUEUE", BrandName: "Antre", BookingPrefix: "PKL"}); code(err) != "STATE_CONFLICT" {
		t.Fatalf("duplicate: %v", err)
	}
	// DB menolak root Polda kedua dan Polres dengan parent non-Jateng, bahkan lewat SQL langsung
	if _, err := e.admin.Exec(e.ctx, `INSERT INTO organizations (code, org_type, display_name) VALUES ('POLDA_JAWA_BARAT','POLDA','x')`); err == nil {
		t.Fatal("second Polda root must be rejected by DB")
	}
	if _, err := e.admin.Exec(e.ctx, `INSERT INTO organizations (code, org_type, display_name, parent_id) SELECT 'POLRES_ORPHAN','POLRES','x',id FROM organizations WHERE code='POLRES_BATANG'`); err == nil {
		t.Fatal("Polres under Polres must be rejected")
	}
	if _, err := e.admin.Exec(e.ctx, `UPDATE organizations SET parent_id=NULL WHERE code='POLRES_BATANG'`); err == nil {
		t.Fatal("hierarchy must be immutable")
	}
}

func TestPublishReadinessGate(t *testing.T) {
	e := newEnv(t)
	polda := e.str(`SELECT id::text FROM organizations WHERE code='POLDA_JAWA_TENGAH'`)
	pa := e.staff(domain.RolePoldaOrgAdmin, domain.ScopePoldaJateng, polda, "")
	tid := e.str(`SELECT id::text FROM tenants WHERE code='SIBATANG_BATANG'`)
	rd, err := e.svc.TenantReadiness(e.ctx, pa, tid)
	if err != nil || rd.Ready {
		t.Fatalf("fresh Batang tenant must not be ready: %v %+v", err, rd)
	}
	if err := e.svc.PublishTenant(e.ctx, pa, tid); code(err) != "SERVICE_POLICY_NOT_MET" {
		t.Fatalf("publish before readiness: %v", err)
	}
	if _, err := e.svc.PublicCatalog(e.ctx); code(err) != "NOT_FOUND" {
		t.Fatalf("unpublished tenant must not be public: %v", err)
	}
}

func TestPublicCatalogAndSlots(t *testing.T) {
	e := newEnv(t)
	b := e.setupBatang()
	cat, err := e.svc.PublicCatalog(e.ctx)
	if err != nil || cat.BrandName != "SIBATANG" || len(cat.Sites) != 3 {
		t.Fatalf("catalog: %v %+v", err, cat)
	}
	slot := e.slot(b.tsSKCK, soon(), 4)
	_ = slot
	date := e.str(`SELECT (now() AT TIME ZONE 'Asia/Jakarta')::date::text`)
	if time.Now().Add(10*time.Minute).In(time.FixedZone("WIB", 7*3600)).Format("2006-01-02") != date {
		t.Skip("slot crosses WIB midnight")
	}
	sl, err := e.svc.PublicSlots(e.ctx, b.tsSKCK, date)
	if err != nil || len(sl) != 1 || sl[0].Remaining != 4 {
		t.Fatalf("slots: %v %+v", err, sl)
	}
	if _, _, err := e.svc.CreateBooking(e.ctx, "k", BookingRequest{}); err == nil {
		t.Fatal("short idempotency key must fail")
	}
	_, _, err = e.svc.CreateBooking(e.ctx, "valid-key-123", BookingRequest{TenantCode: "UJI_B_TENANT", SlotID: slot, ServiceID: b.tsSKCK,
		Applicant: Applicant{DisplayName: "Zed", Contact: "0812"}, PrivacyNoticeVersion: "v1"})
	if code(err) != "VALIDATION_ERROR" {
		t.Fatalf("other tenant code must be rejected: %v", err)
	}
}
