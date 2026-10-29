package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/D4marp/SiBatang/backend/internal/crypto"
	"github.com/D4marp/SiBatang/backend/internal/domain"
)

type EnrollRequest struct {
	DeviceType  string `json:"device_type"`
	Label       string `json:"label"`
	AudioZoneID string `json:"audio_zone_id"`
	CounterID   string `json:"counter_id"`
}
type EnrollResult struct {
	DeviceID       string    `json:"device_id"`
	ActivationCode string    `json:"activation_code"` // satu kali, berumur pendek
	ExpiresAt      time.Time `json:"expires_at"`
}

// EnrollDevice: admin membuat registrasi + kode aktivasi sekali pakai (05 §4).
func (s *Service) EnrollDevice(ctx context.Context, p *StaffPrincipal, siteID string, req EnrollRequest) (*EnrollResult, error) {
	switch req.DeviceType {
	case "KIOSK", "OPERATOR", "DISPLAY", "ANNOUNCER":
	default:
		return nil, domain.ErrValidation("device_type must be KIOSK, OPERATOR, DISPLAY or ANNOUNCER")
	}
	if !domain.IsUUID(siteID) || len(req.Label) < 2 || len(req.Label) > 80 {
		return nil, domain.ErrValidation("site and label (2-80 chars) are required")
	}
	if req.AudioZoneID != "" && !domain.IsUUID(req.AudioZoneID) || req.CounterID != "" && !domain.IsUUID(req.CounterID) {
		return nil, domain.ErrValidation("audio_zone_id/counter_id must be UUIDs")
	}
	if req.DeviceType == "ANNOUNCER" && req.AudioZoneID == "" {
		return nil, domain.ErrValidation("announcer requires audio_zone_id")
	}
	var out EnrollResult
	err := s.staffTx(ctx, p, domain.PermDeviceEnroll, locateSite(siteID), func(tx pgx.Tx, res domain.Resource) error {
		// zona/loket harus milik site yang sama
		if req.AudioZoneID != "" {
			var ok bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audio_zones WHERE id=$1 AND site_id=$2)`, req.AudioZoneID, siteID).Scan(&ok); err != nil {
				return err
			}
			if !ok {
				return domain.ErrValidation("audio_zone_id does not belong to this site")
			}
		}
		if req.CounterID != "" {
			var ok bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM counters WHERE id=$1 AND site_id=$2)`, req.CounterID, siteID).Scan(&ok); err != nil {
				return err
			}
			if !ok {
				return domain.ErrValidation("counter_id does not belong to this site")
			}
		}
		if err := tx.QueryRow(ctx, `INSERT INTO devices (tenant_id, site_id, device_type, label, audio_zone_id, counter_id)
			VALUES ($1,$2,$3,$4,NULLIF($5,'')::uuid,NULLIF($6,'')::uuid) RETURNING id::text`,
			res.TenantID, siteID, req.DeviceType, req.Label, req.AudioZoneID, req.CounterID).Scan(&out.DeviceID); err != nil {
			return err
		}
		code := crypto.RandomActivationCode()
		out.ActivationCode, out.ExpiresAt = code, s.now().Add(15*time.Minute)
		if _, err := tx.Exec(ctx, `INSERT INTO device_activations (code_hash, device_id, tenant_id, expires_at) VALUES ($1,$2,$3,$4)`,
			crypto.HashToken(code), out.DeviceID, res.TenantID, out.ExpiresAt); err != nil {
			return err
		}
		return s.audit(ctx, tx, auditEntry{TenantID: res.TenantID, ActorID: p.UserID, Action: "device.enroll", ResourceType: "device", ResourceID: out.DeviceID})
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

type ActivationResult struct {
	DeviceID    string `json:"device_id"`
	Credential  string `json:"credential"` // "<device_id>.<secret>", ditampilkan sekali
	DeviceType  string `json:"device_type"`
	SiteID      string `json:"site_id"`
	AudioZoneID string `json:"audio_zone_id,omitempty"`
	CounterID   string `json:"counter_id,omitempty"`
}

// ActivateDevice menukar kode aktivasi sekali pakai dengan kredensial perangkat (hash disimpan).
func (s *Service) ActivateDevice(ctx context.Context, code string) (*ActivationResult, error) {
	if len(code) < 6 || len(code) > 32 {
		return nil, domain.ErrUnauthenticated
	}
	hash := crypto.HashToken(code)
	var tenantID string
	err := s.db.WithPlain(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT tenant_id::text FROM device_activations WHERE code_hash=$1`, hash).Scan(&tenantID)
	})
	if err != nil {
		return nil, domain.ErrUnauthenticated
	}
	var out ActivationResult
	err = s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var deviceID string
		// klaim atomik: kode hanya bisa dipakai sekali meski dikirim bersamaan
		if err := tx.QueryRow(ctx, `UPDATE device_activations SET used_at=now()
			WHERE code_hash=$1 AND used_at IS NULL AND expires_at>now() RETURNING device_id::text`, hash).Scan(&deviceID); err != nil {
			return err
		}
		secret := crypto.RandomToken()
		if _, err := tx.Exec(ctx, `INSERT INTO device_credentials (device_id, tenant_id, secret_hash) VALUES ($1,$2,$3)`,
			deviceID, tenantID, crypto.HashToken(secret)); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `UPDATE devices SET status='ACTIVE', last_seen_at=now() WHERE id=$1 AND status='PENDING'
			RETURNING id::text, device_type, site_id::text, COALESCE(audio_zone_id::text,''), COALESCE(counter_id::text,'')`, deviceID).
			Scan(&out.DeviceID, &out.DeviceType, &out.SiteID, &out.AudioZoneID, &out.CounterID); err != nil {
			return err
		}
		out.Credential = deviceID + "." + secret
		return s.audit(ctx, tx, auditEntry{TenantID: tenantID, ActorType: "DEVICE", ActorID: deviceID, Action: "device.activate", ResourceType: "device", ResourceID: deviceID})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUnauthenticated
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// RevokeDevice mencabut kredensial dan melepas lease zona audio (revocation berlaku pada request berikutnya).
func (s *Service) RevokeDevice(ctx context.Context, p *StaffPrincipal, deviceID string) error {
	loc := locateRow(`SELECT site_id::text FROM devices WHERE id=$1`, deviceID, func(r *domain.Resource) []any { return []any{&r.SiteID} })
	return s.staffTx(ctx, p, domain.PermDeviceEnroll, loc, func(tx pgx.Tx, res domain.Resource) error {
		if _, err := tx.Exec(ctx, `UPDATE devices SET status='REVOKED' WHERE id=$1`, deviceID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE device_credentials SET revoked_at=now() WHERE device_id=$1 AND revoked_at IS NULL`, deviceID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE audio_zones SET active_device_id=NULL, lease_until=NULL WHERE active_device_id=$1`, deviceID); err != nil {
			return err
		}
		return s.audit(ctx, tx, auditEntry{TenantID: res.TenantID, ActorID: p.UserID, Action: "device.revoke", ResourceType: "device", ResourceID: deviceID})
	})
}

var healthKeys = map[string]bool{"printer_state": true, "speaker_state": true, "voice_ok": true, "queue_length": true,
	"last_error_code": true, "network": true, "last_event_at": true}

// Heartbeat hanya menerima kunci kesehatan yang di-whitelist (tanpa QR/PII/kalimat suara).
func (s *Service) Heartbeat(ctx context.Context, dp *DevicePrincipal, deviceID, appVersion string, health map[string]any) error {
	if err := requireDevice(dp, deviceID, domain.PermDeviceHealthWrite); err != nil {
		return err
	}
	clean := map[string]any{}
	for k, v := range health {
		if !healthKeys[k] {
			continue
		}
		if str, ok := v.(string); ok && len(str) > 64 {
			v = str[:64]
		}
		clean[k] = v
	}
	b, _ := json.Marshal(clean)
	if len(appVersion) > 32 {
		appVersion = appVersion[:32]
	}
	return s.db.WithTenant(ctx, dp.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE devices SET last_seen_at=now(), app_version=$2, health=$3 WHERE id=$1`, deviceID, appVersion, b)
		return err
	})
}

type CheckinRequest struct {
	QRToken         string `json:"qr_token"`
	DeviceRequestID string `json:"device_request_id"`
}
type CheckinResult struct {
	TicketID      string `json:"ticket_id"`
	DisplayNumber string `json:"display_number"`
	Status        string `json:"status"`
	Site          string `json:"site"`
	PrintJobID    string `json:"print_job_id"`
	IsReprint     bool   `json:"is_reprint"`
}

// CheckIn menerbitkan nomor antrean TEPAT SEKALI per booking (03 §4.2). Scan ulang = tiket yang sama.
func (s *Service) CheckIn(ctx context.Context, dp *DevicePrincipal, deviceID string, req CheckinRequest) (*CheckinResult, error) {
	if err := requireDevice(dp, deviceID, domain.PermBookingCheckin); err != nil {
		return nil, err
	}
	if len(req.QRToken) < 16 || len(req.QRToken) > 128 {
		return nil, domain.ErrValidation("qr_token is invalid")
	}
	var out CheckinResult
	err := s.db.WithTenant(ctx, dp.TenantID, func(tx pgx.Tx) error {
		var bookingID, status, tsID, siteID, svcCode, siteCode string
		var early, grace int
		var startsAt, endsAt, qrExp time.Time
		var serviceDate time.Time
		err := tx.QueryRow(ctx, `SELECT b.id::text, b.status, b.tenant_service_id::text, ts.site_id::text, sv.code, si.code,
				ts.checkin_early_minutes, ts.checkin_grace_minutes, sl.starts_at, sl.ends_at, sl.service_date, b.qr_expires_at
			FROM bookings b JOIN schedule_slots sl ON sl.tenant_id=b.tenant_id AND sl.id=b.slot_id
			JOIN tenant_services ts ON ts.tenant_id=b.tenant_id AND ts.id=b.tenant_service_id
			JOIN services sv ON sv.id=ts.service_id JOIN sites si ON si.tenant_id=ts.tenant_id AND si.id=ts.site_id
			WHERE b.qr_secret_hash=$1 FOR UPDATE OF b`, crypto.HashToken(req.QRToken)).
			Scan(&bookingID, &status, &tsID, &siteID, &svcCode, &siteCode, &early, &grace, &startsAt, &endsAt, &serviceDate, &qrExp)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		} else if err != nil {
			return err
		}
		if siteID != dp.SiteID {
			return domain.ErrBusinessRule("booking belongs to a different site")
		}
		out.Site = siteCode

		if domain.BookingStatus(status) == domain.BookingCheckedIn {
			// replay idempotent: tiket yang sama, tanpa nomor baru
			err := tx.QueryRow(ctx, `SELECT t.id::text, t.display_number, t.status FROM queue_tickets t WHERE t.booking_id=$1`, bookingID).
				Scan(&out.TicketID, &out.DisplayNumber, &out.Status)
			if err != nil {
				return err
			}
			out.PrintJobID, out.IsReprint = out.TicketID, true
			return s.audit(ctx, tx, auditEntry{TenantID: dp.TenantID, ActorType: "DEVICE", ActorID: dp.DeviceID, Action: "checkin.replay", ResourceType: "ticket", ResourceID: out.TicketID})
		}
		if !domain.CanCheckIn(domain.BookingStatus(status)) {
			return domain.ErrBookingExpired
		}
		now := s.now()
		if now.After(qrExp) {
			return domain.ErrBookingExpired
		}
		if now.Before(startsAt.Add(-time.Duration(early)*time.Minute)) || now.After(endsAt.Add(time.Duration(grace)*time.Minute)) {
			return domain.ErrBusinessRule("outside the check-in window for this booking")
		}

		var serial int
		if err := tx.QueryRow(ctx, `INSERT INTO ticket_sequences (tenant_id, site_id, tenant_service_id, service_date, last_no)
			VALUES ($1,$2,$3,$4,1)
			ON CONFLICT (tenant_id, site_id, tenant_service_id, service_date) DO UPDATE SET last_no = ticket_sequences.last_no + 1
			RETURNING last_no`, dp.TenantID, siteID, tsID, serviceDate).Scan(&serial); err != nil {
			return err
		}
		display := domain.FormatDisplayNumber(svcCode, serial)
		if err := tx.QueryRow(ctx, `INSERT INTO queue_tickets (tenant_id, booking_id, tenant_service_id, site_id, service_date, sequence_no, display_number, status)
			VALUES ($1,$2,$3,$4,$5,$6,$7,'WAITING') RETURNING id::text`,
			dp.TenantID, bookingID, tsID, siteID, serviceDate, serial, display).Scan(&out.TicketID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO checkins (tenant_id, booking_id, ticket_id, device_id) VALUES ($1,$2,$3,$4)`,
			dp.TenantID, bookingID, out.TicketID, dp.DeviceID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE bookings SET status='CHECKED_IN', checked_in_at=now() WHERE id=$1`, bookingID); err != nil {
			return err
		}
		if _, err := s.queueEvent(ctx, tx, dp.TenantID, siteID, out.TicketID, "", "queue.ticket_issued",
			map[string]any{"ticket_number": display, "service": svcCode}); err != nil {
			return err
		}
		if err := s.outbox(ctx, tx, dp.TenantID, "queue.ticket_issued", map[string]any{"ticket_id": out.TicketID}); err != nil {
			return err
		}
		out.DisplayNumber, out.Status, out.PrintJobID, out.IsReprint = display, "WAITING", out.TicketID, false
		return s.audit(ctx, tx, auditEntry{TenantID: dp.TenantID, ActorType: "DEVICE", ActorID: dp.DeviceID, Action: "checkin.create", ResourceType: "ticket", ResourceID: out.TicketID})
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

type PrintPayload struct {
	TicketID      string    `json:"ticket_id"`
	DisplayNumber string    `json:"display_number"`
	Service       string    `json:"service"`
	ServiceName   string    `json:"service_name"`
	Site          string    `json:"site"`
	SiteName      string    `json:"site_name"`
	ServiceDate   string    `json:"service_date"`
	IssuedAt      time.Time `json:"issued_at"`
}

// RecordPrintAttempt mencatat percobaan cetak/reprint untuk NOMOR YANG SAMA dan mengembalikan payload cetak (tanpa PII).
func (s *Service) RecordPrintAttempt(ctx context.Context, dp *DevicePrincipal, deviceID, ticketID, outcome string) (*PrintPayload, error) {
	if err := requireDevice(dp, deviceID, domain.PermBookingCheckin); err != nil {
		return nil, err
	}
	if !domain.IsUUID(ticketID) || (outcome != "SUCCESS" && outcome != "FAILED") {
		return nil, domain.ErrValidation("ticket id and outcome (SUCCESS|FAILED) are required")
	}
	var out PrintPayload
	err := s.db.WithTenant(ctx, dp.TenantID, func(tx pgx.Tx) error {
		var date time.Time
		err := tx.QueryRow(ctx, `SELECT t.id::text, t.display_number, sv.code, sv.display_name, si.code, si.name, t.service_date, t.checked_in_at
			FROM queue_tickets t JOIN tenant_services ts ON ts.tenant_id=t.tenant_id AND ts.id=t.tenant_service_id
			JOIN services sv ON sv.id=ts.service_id JOIN sites si ON si.tenant_id=t.tenant_id AND si.id=t.site_id
			WHERE t.id=$1 AND t.site_id=$2`, ticketID, dp.SiteID).
			Scan(&out.TicketID, &out.DisplayNumber, &out.Service, &out.ServiceName, &out.Site, &out.SiteName, &date, &out.IssuedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		} else if err != nil {
			return err
		}
		out.ServiceDate = date.Format("2006-01-02")
		return s.audit(ctx, tx, auditEntry{TenantID: dp.TenantID, ActorType: "DEVICE", ActorID: dp.DeviceID,
			Action: "ticket.print_attempt", ResourceType: "ticket", ResourceID: ticketID, Result: outcome})
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
