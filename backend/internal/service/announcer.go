package service

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/D4marp/SiBatang/backend/internal/domain"
)

type AnnouncerClaim struct {
	JobID        string              `json:"job_id"`
	CallEventID  int64               `json:"call_event_id"`
	TenantID     string              `json:"tenant_id"`
	SiteID       string              `json:"site_id"`
	ZoneID       string              `json:"zone_id"`
	FencingToken int64               `json:"fencing_token"`
	ExpiresAt    time.Time           `json:"expires_at"`
	Locale       string              `json:"locale"`
	Speech       domain.SpokenSpeech `json:"speech"`
}

// ClaimAnnouncement: lease zona + fencing + satu job aktif per zona (serial). Nama hanya dikembalikan di sini,
// hanya bila kebijakan zona + consent warga mengizinkan; tidak pernah ditulis ke SSE/outbox/log/metrik.
// Mengembalikan (nil, nil) bila tidak ada job (204).
func (s *Service) ClaimAnnouncement(ctx context.Context, dp *DevicePrincipal, deviceID string) (*AnnouncerClaim, error) {
	if err := requireDevice(dp, deviceID, domain.PermAnnouncementClaim); err != nil {
		return nil, err
	}
	if dp.AudioZoneID == "" {
		return nil, domain.ErrForbidden
	}
	leaseSecs := s.cfg.AnnouncementLease.Seconds()
	var out *AnnouncerClaim
	err := s.db.WithTenant(ctx, dp.TenantID, func(tx pgx.Tx) error {
		out = nil
		var active *string
		var leaseUntil *time.Time
		var gen int64
		var policy, locale string
		var leaseValid bool
		if err := tx.QueryRow(ctx, `SELECT active_device_id::text, lease_until, lease_generation, name_policy, voice_locale,
			COALESCE(lease_until > now(), false) FROM audio_zones WHERE id=$1 FOR UPDATE`, dp.AudioZoneID).
			Scan(&active, &leaseUntil, &gen, &policy, &locale, &leaseValid); err != nil {
			return err
		}
		switch {
		case active != nil && *active != deviceID && leaseValid:
			return domain.ErrZoneLeased // standby: hanya satu announcer aktif per zona
		case active != nil && *active == deviceID && leaseValid:
			// perpanjang lease, generation tetap
		default:
			gen++ // pengambilalihan: fencing token monotonic
		}
		if _, err := tx.Exec(ctx, `UPDATE audio_zones SET active_device_id=$2, lease_generation=$3, lease_until=now()+make_interval(secs=>$4)
			WHERE id=$1`, dp.AudioZoneID, deviceID, gen, leaseSecs*2); err != nil {
			return err
		}
		// pembersihan: job kedaluwarsa, job claim basi kembali PENDING, playing basi = FAILED (tidak diulang buta)
		if _, err := tx.Exec(ctx, `UPDATE queue_announcements SET state='EXPIRED', finished_at=now()
			WHERE audio_zone_id=$1 AND state='PENDING' AND expires_at<=now()`, dp.AudioZoneID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE queue_announcements SET state='PENDING', leased_by_device_id=NULL, lease_until=NULL
			WHERE audio_zone_id=$1 AND state='CLAIMED' AND lease_until<=now() AND expires_at>now()`, dp.AudioZoneID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE queue_announcements SET state='FAILED', finished_at=now(), playback_error_code='PLAYBACK_UNKNOWN'
			WHERE audio_zone_id=$1 AND state IN ('PLAYING','CLAIMED') AND lease_until<=now()`, dp.AudioZoneID); err != nil {
			return err
		}
		// tiket yang sudah tidak CALLED tidak boleh diumumkan (mis. sudah SERVING/NO_SHOW)
		if _, err := tx.Exec(ctx, `UPDATE queue_announcements a SET state='CANCELLED', finished_at=now()
			FROM queue_tickets t WHERE a.audio_zone_id=$1 AND a.state='PENDING' AND t.tenant_id=a.tenant_id AND t.id=a.ticket_id AND t.status<>'CALLED'`,
			dp.AudioZoneID); err != nil {
			return err
		}
		var busy bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM queue_announcements WHERE audio_zone_id=$1 AND state IN ('CLAIMED','PLAYING') AND lease_until>now())`,
			dp.AudioZoneID).Scan(&busy); err != nil {
			return err
		}
		if busy {
			return nil // playback berurutan per zona
		}
		var c AnnouncerClaim
		var ticketID, counterID string
		err := tx.QueryRow(ctx, `SELECT id::text, call_event_id, site_id::text, expires_at, ticket_id::text, counter_id::text
			FROM queue_announcements WHERE audio_zone_id=$1 AND state='PENDING' AND expires_at>now()
			ORDER BY created_at, call_event_id LIMIT 1 FOR UPDATE SKIP LOCKED`, dp.AudioZoneID).
			Scan(&c.JobID, &c.CallEventID, &c.SiteID, &c.ExpiresAt, &ticketID, &counterID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE queue_announcements SET state='CLAIMED', leased_by_device_id=$2, fencing_token=$3,
			lease_until=now()+make_interval(secs=>$4), attempt_count=attempt_count+1 WHERE id=$1`, c.JobID, deviceID, gen, leaseSecs); err != nil {
			return err
		}
		var display, label string
		var name *string
		var consent *bool
		if err := tx.QueryRow(ctx, `SELECT t.display_number, ct.label, b.citizen_public_name, b.voice_name_consent
			FROM queue_tickets t JOIN counters ct ON ct.tenant_id=t.tenant_id AND ct.id=$2
			LEFT JOIN bookings b ON b.tenant_id=t.tenant_id AND b.id=t.booking_id WHERE t.id=$1`, ticketID, counterID).
			Scan(&display, &label, &name, &consent); err != nil {
			return err
		}
		var resolved *string
		if name != nil && consent != nil {
			resolved = domain.ResolveCallName(domain.NamePolicy(policy), *consent, *name)
		}
		c.TenantID, c.ZoneID, c.FencingToken, c.Locale = dp.TenantID, dp.AudioZoneID, gen, locale
		c.Speech = domain.BuildSpeech(display, label, resolved)
		out = &c
		// audit tanpa nama/kalimat suara
		return s.audit(ctx, tx, auditEntry{TenantID: dp.TenantID, ActorType: "DEVICE", ActorID: deviceID, Action: "announcement.claim", ResourceType: "announcement", ResourceID: c.JobID})
	})
	return out, err
}

// AckAnnouncement: started | completed | failed — idempoten, dijaga fencing token dan kepemilikan lease.
func (s *Service) AckAnnouncement(ctx context.Context, dp *DevicePrincipal, deviceID, jobID, action string, fencing int64, errCode string) error {
	if err := requireDevice(dp, deviceID, domain.PermAnnouncementAck); err != nil {
		return err
	}
	if !domain.IsUUID(jobID) || (action != "started" && action != "completed" && action != "failed") {
		return domain.ErrValidation("invalid job or ack action")
	}
	if len(errCode) > 40 {
		errCode = errCode[:40]
	}
	return s.db.WithTenant(ctx, dp.TenantID, func(tx pgx.Tx) error {
		var state string
		var leasedBy *string
		var token *int64
		err := tx.QueryRow(ctx, `SELECT state, leased_by_device_id::text, fencing_token FROM queue_announcements
			WHERE id=$1 AND audio_zone_id=$2 FOR UPDATE`, jobID, dp.AudioZoneID).Scan(&state, &leasedBy, &token)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound // zona lain / tenant lain tidak terungkap
		} else if err != nil {
			return err
		}
		var zoneGen int64
		if err := tx.QueryRow(ctx, `SELECT lease_generation FROM audio_zones WHERE id=$1`, dp.AudioZoneID).Scan(&zoneGen); err != nil {
			return err
		}
		if leasedBy == nil || *leasedBy != deviceID || token == nil || *token != fencing || zoneGen != fencing {
			return domain.ErrStaleFencing
		}
		var target string
		switch action {
		case "started":
			target = "PLAYING"
		case "completed":
			target = "PLAYED"
		case "failed":
			target = "FAILED"
		}
		if state == target {
			return nil // ack ulang = no-op
		}
		allowed := (state == "CLAIMED" && (target == "PLAYING" || target == "PLAYED" || target == "FAILED")) ||
			(state == "PLAYING" && (target == "PLAYED" || target == "FAILED"))
		if !allowed {
			return domain.ErrStateConflict("announcement is " + state)
		}
		if target == "PLAYING" {
			_, err = tx.Exec(ctx, `UPDATE queue_announcements SET state='PLAYING', started_at=now() WHERE id=$1`, jobID)
		} else {
			_, err = tx.Exec(ctx, `UPDATE queue_announcements SET state=$2, finished_at=now(), playback_error_code=NULLIF($3,'') WHERE id=$1`, jobID, target, errCode)
		}
		if err != nil {
			return err
		}
		if target == "FAILED" {
			// peringatan operasional; kegagalan audio TIDAK membatalkan status tiket
			if err := s.outbox(ctx, tx, dp.TenantID, "announcement.failed", map[string]any{"zone_id": dp.AudioZoneID, "job_id": jobID, "code": errCode}); err != nil {
				return err
			}
		}
		return s.audit(ctx, tx, auditEntry{TenantID: dp.TenantID, ActorType: "DEVICE", ActorID: deviceID, Action: "announcement." + action, ResourceType: "announcement", ResourceID: jobID, Result: target})
	})
}

type JobAvailable struct {
	EventID int64  `json:"-"`
	JobID   string `json:"job_id"`
	ZoneID  string `json:"zone_id"`
}

// PendingAnnouncements untuk SSE announcement.job_available (hanya id; tanpa nama/nomor).
func (s *Service) PendingAnnouncements(ctx context.Context, dp *DevicePrincipal, deviceID string) ([]JobAvailable, error) {
	if err := requireDevice(dp, deviceID, domain.PermAnnouncementClaim); err != nil {
		return nil, err
	}
	var out []JobAvailable
	err := s.db.WithTenant(ctx, dp.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id::text, call_event_id FROM queue_announcements
			WHERE audio_zone_id=$1 AND state='PENDING' AND expires_at>now() ORDER BY created_at`, dp.AudioZoneID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			j := JobAvailable{ZoneID: dp.AudioZoneID}
			if err := rows.Scan(&j.JobID, &j.EventID); err != nil {
				return err
			}
			out = append(out, j)
		}
		return rows.Err()
	})
	return out, err
}
