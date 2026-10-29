package service

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/D4marp/SiBatang/backend/internal/domain"
)

// poldaScope memverifikasi membership POLDA_JATENG dan mengembalikan org Polda terverifikasi.
func (s *Service) poldaScope(p *StaffPrincipal, perm domain.Permission) (string, error) {
	if p == nil {
		return "", domain.ErrUnauthenticated
	}
	for _, id := range p.PoldaOrgIDs() {
		if domain.Allow(p.Memberships, perm, domain.Resource{PoldaOrgID: id}) {
			return id, nil
		}
	}
	return "", domain.ErrForbidden
}

type PoldaPolres struct {
	TenantCode        string     `json:"tenant_code"`
	BrandName         string     `json:"brand_name"`
	OrgCode           string     `json:"org_code"`
	Status            string     `json:"status"`
	BookingCount      int        `json:"booking_count"`
	ArrivedCount      int        `json:"arrived_count"`
	WaitingCount      int        `json:"waiting_count"`
	CalledCount       int        `json:"called_count"`
	ServedCount       int        `json:"served_count"`
	NoShowCount       int        `json:"no_show_count"`
	CancelledCount    int        `json:"cancelled_count"`
	AverageWaitSecs   *float64   `json:"average_wait_seconds"`
	P95WaitSecs       *float64   `json:"p95_wait_seconds"`
	DevicesTotal      *int       `json:"devices_total"`
	DevicesOnline     *int       `json:"devices_online"`
	AnnouncersOnline  *int       `json:"announcers_online"`
	AnnouncerFailures *int       `json:"announcement_failures_24h"`
	DataAvailable     bool       `json:"data_available"` // false = belum ada data (bukan 0)
	DataAsOf          *time.Time `json:"data_as_of"`
	IsFresh           bool       `json:"is_fresh"`
}
type PoldaMetrics struct {
	PoldaCode         string        `json:"polda_code"`
	AsOf              time.Time     `json:"as_of"`
	ServiceDate       string        `json:"service_date"`
	ActivePolresCount int           `json:"active_polres_count"`
	Polres            []PoldaPolres `json:"polres"`
}

const freshWindow = 5 * time.Minute

// PoldaMetrics: agregat read-only descendant Polda terverifikasi lewat role sib_reporting (tanpa akses tabel warga).
// Akses diaudit. Tidak ada drilldown person-level.
func (s *Service) PoldaMetrics(ctx context.Context, p *StaffPrincipal) (*PoldaMetrics, error) {
	poldaID, err := s.poldaScope(p, domain.PermReportAggregateRead)
	if err != nil {
		return nil, err
	}
	out := &PoldaMetrics{PoldaCode: domain.RootOrgCode, AsOf: s.now().UTC(), Polres: []PoldaPolres{}}
	err = s.db.WithReporting(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT (now() AT TIME ZONE 'Asia/Jakarta')::date::text`).Scan(&out.ServiceDate); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT t.id::text, t.code, t.brand_name, o.code, t.status,
			  COALESCE(sum(m.booking_count),0), COALESCE(sum(m.arrived_count),0), COALESCE(sum(m.waiting_count),0),
			  COALESCE(sum(m.called_count),0), COALESCE(sum(m.served_count),0), COALESCE(sum(m.no_show_count),0),
			  COALESCE(sum(m.cancelled_count),0),
			  CASE WHEN sum(m.served_count) > 0 THEN sum(m.avg_wait_seconds * m.served_count)/sum(m.served_count) END,
			  max(m.p95_wait_seconds), max(m.data_freshness_at),
			  h.devices_total, h.devices_online, h.announcers_online, h.announcement_failures_24h, h.as_of
			FROM tenants t
			JOIN organizations o ON o.id = t.owning_org_id
			JOIN org_ancestors a ON a.descendant_id = t.owning_org_id AND a.ancestor_id = $1 AND a.depth > 0
			LEFT JOIN daily_service_metrics m ON m.tenant_id = t.id AND m.service_date = (now() AT TIME ZONE 'Asia/Jakarta')::date
			LEFT JOIN tenant_health_snapshots h ON h.tenant_id = t.id
			WHERE t.status = 'PUBLISHED'
			GROUP BY t.id, t.code, t.brand_name, o.code, t.status, h.devices_total, h.devices_online, h.announcers_online,
			         h.announcement_failures_24h, h.as_of
			ORDER BY t.code`, poldaID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r PoldaPolres
			var tid string
			var fresh, healthAt *time.Time
			if err := rows.Scan(&tid, &r.TenantCode, &r.BrandName, &r.OrgCode, &r.Status,
				&r.BookingCount, &r.ArrivedCount, &r.WaitingCount, &r.CalledCount, &r.ServedCount, &r.NoShowCount, &r.CancelledCount,
				&r.AverageWaitSecs, &r.P95WaitSecs, &fresh,
				&r.DevicesTotal, &r.DevicesOnline, &r.AnnouncersOnline, &r.AnnouncerFailures, &healthAt); err != nil {
				return err
			}
			if fresh != nil {
				r.DataAvailable, r.DataAsOf = true, fresh
				r.IsFresh = s.now().Sub(*fresh) < freshWindow
				if fresh.Before(out.AsOf) {
					out.AsOf = fresh.UTC()
				}
			}
			out.Polres = append(out.Polres, r)
		}
		out.ActivePolresCount = len(out.Polres)
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	// audit akses laporan (tenant_id NULL, org_id Polda)
	_ = s.db.WithPlain(ctx, func(tx pgx.Tx) error {
		return s.audit(ctx, tx, auditEntry{OrgID: poldaID, ActorID: p.UserID, Action: "report.aggregate.read", ResourceType: "polda_metrics"})
	})
	return out, nil
}

type PolresListItem struct {
	OrgCode    string `json:"org_code"`
	OrgName    string `json:"org_name"`
	TenantCode string `json:"tenant_code"`
	BrandName  string `json:"brand_name"`
	Status     string `json:"tenant_status"`
}

// ListPolres: seluruh Polres Jateng di bawah root (termasuk DRAFT) — metadata organisasi saja.
func (s *Service) ListPolres(ctx context.Context, p *StaffPrincipal) ([]PolresListItem, error) {
	poldaID, err := s.poldaScope(p, domain.PermReportAggregateRead)
	if err != nil {
		return nil, err
	}
	out := []PolresListItem{}
	err = s.db.WithReporting(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT o.code, o.display_name, t.code, t.brand_name, t.status FROM organizations o
			JOIN org_ancestors a ON a.descendant_id=o.id AND a.ancestor_id=$1 AND a.depth>0
			LEFT JOIN tenants t ON t.owning_org_id=o.id ORDER BY o.code`, poldaID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var it PolresListItem
			var tc, bn, st *string
			if err := rows.Scan(&it.OrgCode, &it.OrgName, &tc, &bn, &st); err != nil {
				return err
			}
			if tc != nil {
				it.TenantCode, it.BrandName, it.Status = *tc, *bn, *st
			}
			out = append(out, it)
		}
		return rows.Err()
	})
	return out, err
}

type OnboardingRequest struct {
	ID         string    `json:"id"`
	OrgCode    string    `json:"org_code"`
	OrgName    string    `json:"org_name"`
	TenantCode string    `json:"tenant_code"`
	BrandName  string    `json:"brand_name"`
	Prefix     string    `json:"booking_prefix"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

type OnboardingInput struct {
	OrgCode       string `json:"org_code"`
	OrgName       string `json:"org_name"`
	TenantCode    string `json:"tenant_code"`
	BrandName     string `json:"brand_name"`
	BookingPrefix string `json:"booking_prefix"`
}

// CreateOnboardingRequest: draft-only. Parent TIDAK diterima dari klien; selalu root Jateng saat approve.
func (s *Service) CreateOnboardingRequest(ctx context.Context, p *StaffPrincipal, in OnboardingInput) (*OnboardingRequest, error) {
	poldaID, err := s.poldaScope(p, domain.PermOnboardingManage)
	if err != nil {
		return nil, err
	}
	if err := (domain.OnboardingInput{OrgCode: in.OrgCode, OrgName: in.OrgName, TenantCode: in.TenantCode, BrandName: in.BrandName, BookingPrefix: in.BookingPrefix}).Validate(); err != nil {
		return nil, err
	}
	var r OnboardingRequest
	err = s.db.WithPlain(ctx, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organizations WHERE code=$1) OR EXISTS(SELECT 1 FROM tenants WHERE code=$2)
			OR EXISTS(SELECT 1 FROM tenant_onboarding_requests WHERE status='PENDING' AND (proposed_org_code=$1 OR proposed_tenant_code=$2))`,
			in.OrgCode, in.TenantCode).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return domain.ErrStateConflict("org or tenant code already exists or is pending")
		}
		if err := tx.QueryRow(ctx, `INSERT INTO tenant_onboarding_requests (proposed_org_code, proposed_org_name, proposed_tenant_code,
			proposed_brand_name, proposed_booking_prefix, requested_by) VALUES ($1,$2,$3,$4,$5,$6)
			RETURNING id::text, proposed_org_code, proposed_org_name, proposed_tenant_code, proposed_brand_name, proposed_booking_prefix, status, created_at`,
			in.OrgCode, in.OrgName, in.TenantCode, in.BrandName, in.BookingPrefix, p.UserID).
			Scan(&r.ID, &r.OrgCode, &r.OrgName, &r.TenantCode, &r.BrandName, &r.Prefix, &r.Status, &r.CreatedAt); err != nil {
			return err
		}
		return s.audit(ctx, tx, auditEntry{OrgID: poldaID, ActorID: p.UserID, Action: "onboarding.request", ResourceType: "onboarding_request", ResourceID: r.ID})
	})
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Service) ListOnboardingRequests(ctx context.Context, p *StaffPrincipal) ([]OnboardingRequest, error) {
	if _, err := s.poldaScope(p, domain.PermOnboardingManage); err != nil {
		return nil, err
	}
	out := []OnboardingRequest{}
	err := s.db.WithPlain(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id::text, proposed_org_code, proposed_org_name, proposed_tenant_code, proposed_brand_name,
			proposed_booking_prefix, status, created_at FROM tenant_onboarding_requests ORDER BY created_at DESC LIMIT 100`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r OnboardingRequest
			if err := rows.Scan(&r.ID, &r.OrgCode, &r.OrgName, &r.TenantCode, &r.BrandName, &r.Prefix, &r.Status, &r.CreatedAt); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// DecideOnboarding: approve membuat org POLRES (parent = root Jateng) + tenant DRAFT. Pemohon tidak boleh menyetujui sendiri.
func (s *Service) DecideOnboarding(ctx context.Context, p *StaffPrincipal, id string, approve bool, note string) error {
	poldaID, err := s.poldaScope(p, domain.PermOnboardingApprove)
	if err != nil {
		return err
	}
	if !domain.IsUUID(id) || len(note) > 500 {
		return domain.ErrValidation("invalid request id or note")
	}
	return s.db.WithPlain(ctx, func(tx pgx.Tx) error {
		var orgCode, orgName, tenantCode, brand, prefix, status, requester string
		err := tx.QueryRow(ctx, `SELECT proposed_org_code, proposed_org_name, proposed_tenant_code, proposed_brand_name,
			proposed_booking_prefix, status, requested_by::text FROM tenant_onboarding_requests WHERE id=$1 FOR UPDATE`, id).
			Scan(&orgCode, &orgName, &tenantCode, &brand, &prefix, &status, &requester)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		} else if err != nil {
			return err
		}
		if status != "PENDING" {
			return domain.ErrStateConflict("request already decided")
		}
		if requester == p.UserID {
			return domain.ErrForbidden // persetujuan dua pihak
		}
		if !approve {
			if _, err := tx.Exec(ctx, `UPDATE tenant_onboarding_requests SET status='REJECTED', decided_by=$2, decision_note=$3, decided_at=now() WHERE id=$1`, id, p.UserID, note); err != nil {
				return err
			}
			return s.audit(ctx, tx, auditEntry{OrgID: poldaID, ActorID: p.UserID, Action: "onboarding.reject", ResourceType: "onboarding_request", ResourceID: id})
		}
		var orgID string
		if err := tx.QueryRow(ctx, `INSERT INTO organizations (code, org_type, display_name, parent_id, status)
			SELECT $1,'POLRES',$2,id,'ACTIVE' FROM organizations WHERE code='POLDA_JAWA_TENGAH' RETURNING id::text`, orgCode, orgName).Scan(&orgID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO tenants (owning_org_id, code, brand_name, booking_prefix, status) VALUES ($1,$2,$3,$4,'DRAFT')`,
			orgID, tenantCode, brand, prefix); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE tenant_onboarding_requests SET status='APPROVED', decided_by=$2, decision_note=$3, decided_at=now(), created_org_id=$4 WHERE id=$1`,
			id, p.UserID, note, orgID); err != nil {
			return err
		}
		return s.audit(ctx, tx, auditEntry{OrgID: poldaID, ActorID: p.UserID, Action: "onboarding.approve", ResourceType: "onboarding_request", ResourceID: id})
	})
}

type ReadinessCheck struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
}
type Readiness struct {
	TenantCode string           `json:"tenant_code"`
	Status     string           `json:"status"`
	Ready      bool             `json:"ready"`
	Checks     []ReadinessCheck `json:"checks"`
}

// tenantAccess mengizinkan staf Polres (membership tenant) ATAU Polda untuk tenant descendant (terverifikasi di DB).
func (s *Service) tenantAccess(ctx context.Context, p *StaffPrincipal, tenantID string, perm domain.Permission) (orgID string, err error) {
	if p == nil {
		return "", domain.ErrUnauthenticated
	}
	if !domain.IsUUID(tenantID) {
		return "", domain.ErrNotFound
	}
	if domain.Allow(p.Memberships, perm, domain.Resource{TenantID: tenantID}) {
		return "", nil
	}
	for _, pid := range p.PoldaOrgIDs() {
		var under bool
		e := s.db.WithPlain(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tenants t JOIN org_ancestors a ON a.descendant_id=t.owning_org_id
				WHERE t.id=$1 AND a.ancestor_id=$2 AND a.depth>0)`, tenantID, pid).Scan(&under)
		})
		if e != nil {
			return "", e
		}
		if under && domain.Allow(p.Memberships, perm, domain.Resource{PoldaOrgID: pid}) {
			return pid, nil
		}
	}
	return "", domain.ErrForbidden
}

// TenantReadiness: daftar periksa sebelum publikasi (jumlah saja; tanpa data warga).
func (s *Service) TenantReadiness(ctx context.Context, p *StaffPrincipal, tenantID string) (*Readiness, error) {
	if _, err := s.tenantAccess(ctx, p, tenantID, domain.PermTenantReadiness); err != nil {
		return nil, err
	}
	r := &Readiness{}
	if err := s.db.WithPlain(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT code, status FROM tenants WHERE id=$1`, tenantID).Scan(&r.TenantCode, &r.Status)
	}); err != nil {
		return nil, domain.ErrNotFound
	}
	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		type chk struct{ name, q string }
		for _, c := range []chk{
			{"published_site_with_service", `SELECT EXISTS(SELECT 1 FROM tenant_services ts JOIN sites s ON s.tenant_id=ts.tenant_id AND s.id=ts.site_id WHERE ts.status='PUBLISHED' AND s.status='PUBLISHED')`},
			{"audio_zone_configured", `SELECT EXISTS(SELECT 1 FROM audio_zones)`},
			{"counter_configured", `SELECT EXISTS(SELECT 1 FROM counters)`},
			{"kiosk_active", `SELECT EXISTS(SELECT 1 FROM devices WHERE device_type='KIOSK' AND status='ACTIVE')`},
			{"display_active", `SELECT EXISTS(SELECT 1 FROM devices WHERE device_type='DISPLAY' AND status='ACTIVE')`},
			{"announcer_active", `SELECT EXISTS(SELECT 1 FROM devices WHERE device_type='ANNOUNCER' AND status='ACTIVE')`},
			{"open_slots", `SELECT EXISTS(SELECT 1 FROM schedule_slots WHERE status='OPEN' AND ends_at>now())`},
		} {
			var ok bool
			if err := tx.QueryRow(ctx, c.q).Scan(&ok); err != nil {
				return err
			}
			r.Checks = append(r.Checks, ReadinessCheck{Name: c.name, OK: ok})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	r.Ready = true
	for _, c := range r.Checks {
		r.Ready = r.Ready && c.OK
	}
	return r, nil
}

// PublishTenant: hanya tenant pada allowlist pilot yang dapat PUBLISHED, setelah readiness lulus. Diaudit.
func (s *Service) PublishTenant(ctx context.Context, p *StaffPrincipal, tenantID string) error {
	orgID, err := s.tenantAccess(ctx, p, tenantID, domain.PermTenantPublish)
	if err != nil {
		return err
	}
	rd, err := s.TenantReadiness(ctx, p, tenantID)
	if err != nil {
		return err
	}
	if !slices.Contains(s.cfg.PilotPublishAllowlist, rd.TenantCode) {
		return domain.ErrBusinessRule("tenant is not on the pilot publication allowlist")
	}
	if !rd.Ready {
		return domain.ErrBusinessRule("tenant readiness checks have not all passed")
	}
	return s.db.WithPlain(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE tenants SET status='PUBLISHED', published_at=now() WHERE id=$1`, tenantID); err != nil {
			return err
		}
		return s.audit(ctx, tx, auditEntry{OrgID: orgID, ActorID: p.UserID, Action: "tenant.publish", ResourceType: "tenant", ResourceID: tenantID})
	})
}
