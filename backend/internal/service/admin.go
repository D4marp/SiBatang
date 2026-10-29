package service

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/D4marp/SiBatang/backend/internal/domain"
)

type ZoneRequest struct {
	Label      string `json:"label"`
	NamePolicy string `json:"name_policy"`
}

func validNamePolicy(p string) bool {
	switch domain.NamePolicy(p) {
	case domain.NameNumberOnly, domain.NamePreferred, domain.NameFull:
		return true
	}
	return false
}

// CreateAudioZone: default NUMBER_ONLY (OPEN-03). Mengaktifkan penyebutan nama adalah perubahan kebijakan yang diaudit.
func (s *Service) CreateAudioZone(ctx context.Context, p *StaffPrincipal, siteID string, req ZoneRequest) (string, error) {
	if req.NamePolicy == "" {
		req.NamePolicy = string(domain.NameNumberOnly)
	}
	if len(req.Label) < 2 || len(req.Label) > 80 || !validNamePolicy(req.NamePolicy) {
		return "", domain.ErrValidation("label (2-80 chars) and valid name_policy required")
	}
	var id string
	err := s.staffTx(ctx, p, domain.PermAnnouncementPolicy, locateSite(siteID), func(tx pgx.Tx, res domain.Resource) error {
		if err := tx.QueryRow(ctx, `INSERT INTO audio_zones (tenant_id, site_id, label, name_policy) VALUES ($1,$2,$3,$4) RETURNING id::text`,
			res.TenantID, siteID, req.Label, req.NamePolicy).Scan(&id); err != nil {
			return err
		}
		return s.audit(ctx, tx, auditEntry{TenantID: res.TenantID, ActorID: p.UserID, Action: "audio_zone.create", ResourceType: "audio_zone", ResourceID: id})
	})
	return id, err
}

// UpdateAudioZonePolicy mengubah kebijakan nama per zona; selalu diaudit.
func (s *Service) UpdateAudioZonePolicy(ctx context.Context, p *StaffPrincipal, siteID, zoneID, namePolicy string) error {
	if !validNamePolicy(namePolicy) || !domain.IsUUID(zoneID) {
		return domain.ErrValidation("valid zone id and name_policy required")
	}
	return s.staffTx(ctx, p, domain.PermAnnouncementPolicy, locateSite(siteID), func(tx pgx.Tx, res domain.Resource) error {
		tag, err := tx.Exec(ctx, `UPDATE audio_zones SET name_policy=$3 WHERE id=$1 AND site_id=$2`, zoneID, siteID, namePolicy)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return s.audit(ctx, tx, auditEntry{TenantID: res.TenantID, ActorID: p.UserID, Action: "audio_zone.name_policy:" + namePolicy, ResourceType: "audio_zone", ResourceID: zoneID})
	})
}

type CounterRequest struct {
	Label       string   `json:"label"`
	AudioZoneID string   `json:"audio_zone_id"`
	ServiceIDs  []string `json:"service_ids"` // tenant_service_id yang dilayani loket
	AutoAdvance bool     `json:"auto_advance_on_ready"`
}

func (s *Service) CreateCounter(ctx context.Context, p *StaffPrincipal, siteID string, req CounterRequest) (string, error) {
	if len(req.Label) < 2 || len(req.Label) > 40 || !domain.IsUUID(req.AudioZoneID) || len(req.ServiceIDs) == 0 {
		return "", domain.ErrValidation("label, audio_zone_id and at least one service_id required")
	}
	for _, id := range req.ServiceIDs {
		if !domain.IsUUID(id) {
			return "", domain.ErrValidation("service_ids must be UUIDs")
		}
	}
	var id string
	err := s.staffTx(ctx, p, domain.PermSiteConfigManage, locateSite(siteID), func(tx pgx.Tx, res domain.Resource) error {
		if err := tx.QueryRow(ctx, `INSERT INTO counters (tenant_id, site_id, audio_zone_id, label, auto_advance_on_ready)
			SELECT $1,$2,z.id,$4,$5 FROM audio_zones z WHERE z.id=$3 AND z.site_id=$2 RETURNING id::text`,
			res.TenantID, siteID, req.AudioZoneID, req.Label, req.AutoAdvance).Scan(&id); err != nil {
			return domain.ErrValidation("audio zone not found on this site")
		}
		for _, sid := range req.ServiceIDs {
			tag, err := tx.Exec(ctx, `INSERT INTO counter_services (tenant_id, counter_id, tenant_service_id)
				SELECT $1,$2,ts.id FROM tenant_services ts WHERE ts.id=$3 AND ts.site_id=$4`, res.TenantID, id, sid, siteID)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return domain.ErrValidation("service does not belong to this site")
			}
		}
		return s.audit(ctx, tx, auditEntry{TenantID: res.TenantID, ActorID: p.UserID, Action: "counter.create", ResourceType: "counter", ResourceID: id})
	})
	return id, err
}

type QueuePolicyRequest struct {
	AutoAnnounceOnCall *bool `json:"auto_announce_on_call"`
	AutoAdvanceOnReady *bool `json:"auto_advance_on_ready"`
}

// UpdateCounterPolicy mengatur AUTO_ANNOUNCE_ON_CALL / AUTO_ADVANCE_ON_READY (perubahan diaudit; hanya admin).
func (s *Service) UpdateCounterPolicy(ctx context.Context, p *StaffPrincipal, counterID string, req QueuePolicyRequest) error {
	return s.staffTx(ctx, p, domain.PermSiteConfigManage, locateCounter(counterID), func(tx pgx.Tx, res domain.Resource) error {
		if _, err := tx.Exec(ctx, `UPDATE counters SET auto_announce_on_call=COALESCE($2,auto_announce_on_call),
			auto_advance_on_ready=COALESCE($3,auto_advance_on_ready) WHERE id=$1`, counterID, req.AutoAnnounceOnCall, req.AutoAdvanceOnReady); err != nil {
			return err
		}
		return s.audit(ctx, tx, auditEntry{TenantID: res.TenantID, ActorID: p.UserID, Action: "counter.policy.update", ResourceType: "counter", ResourceID: counterID})
	})
}

// AssignOperator menugaskan staf ke loket; staf harus punya membership aktif pada tenant yang sama.
func (s *Service) AssignOperator(ctx context.Context, p *StaffPrincipal, counterID, userID string) error {
	if !domain.IsUUID(userID) {
		return domain.ErrValidation("user_id must be a UUID")
	}
	return s.staffTx(ctx, p, domain.PermTenantMemberManage, locateCounter(counterID), func(tx pgx.Tx, res domain.Resource) error {
		var ok bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memberships WHERE user_id=$1 AND tenant_id=$2 AND revoked_at IS NULL
			AND (expires_at IS NULL OR expires_at>now()))`, userID, res.TenantID).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return domain.ErrValidation("user has no active membership in this tenant")
		}
		if _, err := tx.Exec(ctx, `UPDATE counters SET assigned_operator_id=$2 WHERE id=$1`, counterID, userID); err != nil {
			return err
		}
		return s.audit(ctx, tx, auditEntry{TenantID: res.TenantID, ActorID: p.UserID, Action: "counter.assign_operator", ResourceType: "counter", ResourceID: counterID})
	})
}

type SlotInput struct {
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Capacity int       `json:"capacity"`
}

// CreateSlots: jadwal ditetapkan admin (tidak di-hardcode). service_date dihitung WIB.
func (s *Service) CreateSlots(ctx context.Context, p *StaffPrincipal, tenantServiceID string, slots []SlotInput) (int, error) {
	if len(slots) == 0 || len(slots) > 200 {
		return 0, domain.ErrValidation("1-200 slots per request")
	}
	for _, sl := range slots {
		if !sl.StartsAt.Before(sl.EndsAt) || sl.Capacity < 0 || sl.Capacity > 500 {
			return 0, domain.ErrValidation("slot times/capacity invalid")
		}
	}
	n := 0
	err := s.staffTx(ctx, p, domain.PermScheduleManage, locateTenantService(tenantServiceID), func(tx pgx.Tx, res domain.Resource) error {
		for _, sl := range slots {
			tag, err := tx.Exec(ctx, `INSERT INTO schedule_slots (tenant_id, tenant_service_id, service_date, starts_at, ends_at, capacity)
				VALUES ($1,$2,($3::timestamptz AT TIME ZONE 'Asia/Jakarta')::date,$3,$4,$5) ON CONFLICT DO NOTHING`,
				res.TenantID, tenantServiceID, sl.StartsAt, sl.EndsAt, sl.Capacity)
			if err != nil {
				return err
			}
			n += int(tag.RowsAffected())
		}
		return s.audit(ctx, tx, auditEntry{TenantID: res.TenantID, ActorID: p.UserID, Action: "schedule.create_slots", ResourceType: "tenant_service", ResourceID: tenantServiceID})
	})
	return n, err
}
