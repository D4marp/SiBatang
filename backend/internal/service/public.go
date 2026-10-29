package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/D4marp/SiBatang/backend/internal/crypto"
	"github.com/D4marp/SiBatang/backend/internal/domain"
)

type CatalogService struct {
	ServiceID   string `json:"service_id"` // = tenant_service_id
	Code        string `json:"code"`
	DisplayName string `json:"display_name"`
}
type CatalogSite struct {
	ID       string           `json:"id"`
	Code     string           `json:"code"`
	Name     string           `json:"name"`
	Address  *string          `json:"address"`
	Services []CatalogService `json:"services"`
}
type Catalog struct {
	TenantCode string        `json:"tenant_code"`
	BrandName  string        `json:"brand_name"`
	Timezone   string        `json:"site_timezone"`
	Sites      []CatalogSite `json:"sites"`
}

// PublicCatalog hanya menampilkan site+layanan PUBLISHED milik tenant publik default.
func (s *Service) PublicCatalog(ctx context.Context) (*Catalog, error) {
	tid, brand, _, err := s.publicTenantID(ctx)
	if err != nil {
		return nil, err
	}
	c := &Catalog{TenantCode: s.cfg.DefaultPublicTenant, BrandName: brand, Timezone: "Asia/Jakarta", Sites: []CatalogSite{}}
	err = s.db.WithTenant(ctx, tid, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT si.id::text, si.code, si.name, si.address, ts.id::text, sv.code, sv.display_name
			FROM sites si JOIN tenant_services ts ON ts.tenant_id=si.tenant_id AND ts.site_id=si.id AND ts.status='PUBLISHED'
			JOIN services sv ON sv.id=ts.service_id WHERE si.status='PUBLISHED' ORDER BY si.code, sv.code`)
		if err != nil {
			return err
		}
		defer rows.Close()
		idx := map[string]int{}
		for rows.Next() {
			var site CatalogSite
			var svc CatalogService
			if err := rows.Scan(&site.ID, &site.Code, &site.Name, &site.Address, &svc.ServiceID, &svc.Code, &svc.DisplayName); err != nil {
				return err
			}
			i, ok := idx[site.ID]
			if !ok {
				site.Services = []CatalogService{}
				c.Sites = append(c.Sites, site)
				i = len(c.Sites) - 1
				idx[site.ID] = i
			}
			c.Sites[i].Services = append(c.Sites[i].Services, svc)
		}
		return rows.Err()
	})
	return c, err
}

type SlotView struct {
	ID        string    `json:"slot_id"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
	Capacity  int       `json:"capacity"`
	Remaining int       `json:"remaining"`
}

func (s *Service) PublicSlots(ctx context.Context, serviceID, date string) ([]SlotView, error) {
	if !domain.IsUUID(serviceID) {
		return nil, domain.ErrValidation("service_id must be a UUID")
	}
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, domain.ErrValidation("date must be YYYY-MM-DD")
	}
	tid, _, _, err := s.publicTenantID(ctx)
	if err != nil {
		return nil, err
	}
	out := []SlotView{}
	err = s.db.WithTenant(ctx, tid, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT sl.id::text, sl.starts_at, sl.ends_at, sl.capacity, sl.capacity-sl.booked_count
			FROM schedule_slots sl JOIN tenant_services ts ON ts.tenant_id=sl.tenant_id AND ts.id=sl.tenant_service_id AND ts.status='PUBLISHED'
			WHERE sl.tenant_service_id=$1 AND sl.service_date=$2 AND sl.status='OPEN' AND sl.ends_at>now()
			ORDER BY sl.starts_at`, serviceID, d)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v SlotView
			if err := rows.Scan(&v.ID, &v.StartsAt, &v.EndsAt, &v.Capacity, &v.Remaining); err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

type Applicant struct {
	DisplayName    string `json:"display_name"`
	Contact        string `json:"contact"`
	AllowVoiceName bool   `json:"allow_voice_name"`
}
type BookingRequest struct {
	TenantCode           string    `json:"tenant_code"`
	SlotID               string    `json:"slot_id"`
	ServiceID            string    `json:"service_id"`
	Applicant            Applicant `json:"applicant"`
	PrivacyNoticeVersion string    `json:"privacy_notice_version"`
}

type BookingResult struct {
	BookingID   string    `json:"booking_id"`
	Reference   string    `json:"reference"`
	Status      string    `json:"status"`
	Service     string    `json:"service"`
	Site        string    `json:"site"`
	ServiceDate string    `json:"service_date"`
	StartsAt    time.Time `json:"starts_at"`
	QRToken     string    `json:"qr_token,omitempty"`
	QRExpiresAt time.Time `json:"qr_expires_at"`
}

func (r BookingRequest) validate(defaultTenant string) error {
	switch {
	case r.TenantCode != defaultTenant:
		return domain.ErrValidation("tenant_code is not available for public booking")
	case !domain.IsUUID(r.SlotID) || !domain.IsUUID(r.ServiceID):
		return domain.ErrValidation("slot_id and service_id must be UUIDs")
	case utf8.RuneCountInString(strings.TrimSpace(r.Applicant.DisplayName)) < 2 || utf8.RuneCountInString(r.Applicant.DisplayName) > 80:
		return domain.ErrValidation("applicant.display_name must be 2-80 characters")
	case utf8.RuneCountInString(strings.TrimSpace(r.Applicant.Contact)) < 3 || utf8.RuneCountInString(r.Applicant.Contact) > 100:
		return domain.ErrValidation("applicant.contact must be 3-100 characters")
	case r.PrivacyNoticeVersion == "" || len(r.PrivacyNoticeVersion) > 32:
		return domain.ErrValidation("privacy_notice_version is required")
	}
	return nil
}

// CreateBooking: reserve slot + booking + outbox + idempotency dalam SATU transaksi (03 §4.1).
func (s *Service) CreateBooking(ctx context.Context, idemKey string, req BookingRequest) (*BookingResult, bool, error) {
	if len(idemKey) < 8 || len(idemKey) > 128 {
		return nil, false, domain.ErrValidation("Idempotency-Key header (8-128 chars) is required")
	}
	if err := req.validate(s.cfg.DefaultPublicTenant); err != nil {
		return nil, false, err
	}
	tid, _, prefix, err := s.publicTenantID(ctx)
	if err != nil {
		return nil, false, err
	}
	canonical, _ := json.Marshal(req)
	reqHash := sha256.Sum256(canonical)
	keyHash := crypto.HashToken(idemKey)

	var res *BookingResult
	replay := false
	err = s.db.WithTenant(ctx, tid, func(tx pgx.Tx) error {
		res, replay = nil, false
		// 1) klaim kunci idempotensi; konkuren dengan kunci sama menunggu commit transaksi pertama.
		tag, err := tx.Exec(ctx, `INSERT INTO idempotency_keys (tenant_id, principal_key, endpoint_key, key_hash, request_hash, expires_at)
			VALUES ($1,'public','POST /public/bookings',$2,$3,now()+interval '24 hours') ON CONFLICT DO NOTHING`, tid, keyHash, reqHash[:])
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			var storedHash, body, sealed []byte
			err := tx.QueryRow(ctx, `SELECT request_hash, response_body, response_secret_encrypted FROM idempotency_keys
				WHERE tenant_id=$1 AND principal_key='public' AND endpoint_key='POST /public/bookings' AND key_hash=$2`, tid, keyHash).
				Scan(&storedHash, &body, &sealed)
			if err != nil {
				return err
			}
			if !crypto.Equal(storedHash, reqHash[:]) {
				return domain.ErrIdempotencyReused
			}
			if body == nil {
				return domain.ErrStateConflict("original request still in progress")
			}
			var prev BookingResult
			if err := json.Unmarshal(body, &prev); err != nil {
				return err
			}
			tok, err := s.box.Open(sealed, []byte(tid+prev.BookingID))
			if err != nil {
				return err
			}
			prev.QRToken = string(tok)
			res, replay = &prev, true
			return nil
		}

		// 2) layanan harus PUBLISHED (site + layanan)
		var svcCode, siteCode string
		var grace int
		err = tx.QueryRow(ctx, `SELECT sv.code, si.code, ts.checkin_grace_minutes FROM tenant_services ts
			JOIN sites si ON si.tenant_id=ts.tenant_id AND si.id=ts.site_id
			JOIN services sv ON sv.id=ts.service_id
			WHERE ts.id=$1 AND ts.status='PUBLISHED' AND si.status='PUBLISHED'`, req.ServiceID).Scan(&svcCode, &siteCode, &grace)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		} else if err != nil {
			return err
		}

		// 3) reservasi kapasitas atomik (tidak mungkin melebihi capacity atau negatif)
		var serviceDate time.Time
		var startsAt, endsAt time.Time
		err = tx.QueryRow(ctx, `UPDATE schedule_slots SET booked_count=booked_count+1
			WHERE id=$1 AND tenant_service_id=$2 AND status='OPEN' AND booked_count<capacity AND ends_at>now()
			RETURNING service_date, starts_at, ends_at`, req.SlotID, req.ServiceID).Scan(&serviceDate, &startsAt, &endsAt)
		if errors.Is(err, pgx.ErrNoRows) {
			var exists bool
			if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schedule_slots WHERE id=$1 AND tenant_service_id=$2)`, req.SlotID, req.ServiceID).Scan(&exists); e != nil {
				return e
			}
			if !exists {
				return domain.ErrNotFound
			}
			return domain.ErrSlotFull
		} else if err != nil {
			return err
		}

		// 4) booking dengan referensi acak + QR token opaque (hanya hash disimpan)
		token := crypto.RandomToken()
		qrExpires := endsAt.Add(time.Duration(grace) * time.Minute)
		contactSealed := s.box.Seal([]byte(strings.TrimSpace(req.Applicant.Contact)), []byte(tid))
		var bookingID, ref string
		for i := 0; i < 5 && bookingID == ""; i++ {
			ref = fmt.Sprintf("%s-%s-%s", prefix, serviceDate.Format("20060102"), crypto.RandomRef(6))
			err = tx.QueryRow(ctx, `INSERT INTO bookings (tenant_id, tenant_service_id, slot_id, reference_code, citizen_public_name,
				voice_name_consent, citizen_contact_encrypted, privacy_notice_version, status, qr_secret_hash, qr_expires_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'CONFIRMED',$9,$10)
				ON CONFLICT (tenant_id, reference_code) DO NOTHING RETURNING id::text`,
				tid, req.ServiceID, req.SlotID, ref, strings.TrimSpace(req.Applicant.DisplayName), req.Applicant.AllowVoiceName,
				contactSealed, req.PrivacyNoticeVersion, crypto.HashToken(token), qrExpires).Scan(&bookingID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		if bookingID == "" {
			return domain.ErrUnavailable
		}
		if err := s.outbox(ctx, tx, tid, "booking.confirmed", map[string]any{"booking_id": bookingID, "tenant_service_id": req.ServiceID}); err != nil {
			return err
		}
		if err := s.audit(ctx, tx, auditEntry{TenantID: tid, ActorType: "CITIZEN", Action: "booking.create", ResourceType: "booking", ResourceID: bookingID}); err != nil {
			return err
		}

		out := BookingResult{BookingID: bookingID, Reference: ref, Status: "CONFIRMED", Service: svcCode, Site: siteCode,
			ServiceDate: serviceDate.Format("2006-01-02"), StartsAt: startsAt, QRExpiresAt: qrExpires}
		stored, _ := json.Marshal(out) // tanpa QR token
		if _, err := tx.Exec(ctx, `UPDATE idempotency_keys SET status_code=201, response_body=$1, response_secret_encrypted=$2
			WHERE tenant_id=$3 AND principal_key='public' AND endpoint_key='POST /public/bookings' AND key_hash=$4`,
			stored, s.box.Seal([]byte(token), []byte(tid+bookingID)), tid, keyHash); err != nil {
			return err
		}
		out.QRToken = token
		res = &out
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return res, replay, nil
}

type TicketView struct {
	TicketID      string  `json:"ticket_id"`
	DisplayNumber string  `json:"display_number"`
	Status        string  `json:"status"`
	CounterLabel  *string `json:"counter_label"`
	WaitingAhead  *int    `json:"waiting_ahead_estimate"` // perkiraan, bukan janji
	Revision      int     `json:"revision"`
}
type BookingDetail struct {
	BookingResult
	Ticket *TicketView `json:"ticket"`
}

// GetBooking: kepemilikan dibuktikan oleh token (reference saja BUKAN otorisasi). Tidak ada nama/kontak pada respons.
func (s *Service) GetBooking(ctx context.Context, reference, token string) (*BookingDetail, error) {
	if reference == "" || token == "" {
		return nil, domain.ErrNotFound
	}
	tid, _, _, err := s.publicTenantID(ctx)
	if err != nil {
		return nil, err
	}
	var d BookingDetail
	err = s.db.WithTenant(ctx, tid, func(tx pgx.Tx) error {
		var date time.Time
		var bookingID string
		err := tx.QueryRow(ctx, `SELECT b.id::text, b.reference_code, b.status, sv.code, si.code, sl.service_date, sl.starts_at, b.qr_expires_at
			FROM bookings b JOIN schedule_slots sl ON sl.tenant_id=b.tenant_id AND sl.id=b.slot_id
			JOIN tenant_services ts ON ts.tenant_id=b.tenant_id AND ts.id=b.tenant_service_id
			JOIN services sv ON sv.id=ts.service_id JOIN sites si ON si.tenant_id=ts.tenant_id AND si.id=ts.site_id
			WHERE b.reference_code=$1 AND b.qr_secret_hash=$2`, reference, crypto.HashToken(token)).
			Scan(&bookingID, &d.Reference, &d.Status, &d.Service, &d.Site, &date, &d.StartsAt, &d.QRExpiresAt)
		if err != nil {
			return err
		}
		d.BookingID, d.ServiceDate = bookingID, date.Format("2006-01-02")
		var t TicketView
		var seq int
		var svcID, siteID string
		var svcDate time.Time
		var counter *string
		err = tx.QueryRow(ctx, `SELECT qt.id::text, qt.display_number, qt.status, qt.revision, qt.sequence_no, qt.tenant_service_id::text, qt.site_id::text, qt.service_date, c.label
			FROM queue_tickets qt LEFT JOIN counters c ON c.tenant_id=qt.tenant_id AND c.id=qt.called_counter_id
			WHERE qt.booking_id=$1`, bookingID).Scan(&t.TicketID, &t.DisplayNumber, &t.Status, &t.Revision, &seq, &svcID, &siteID, &svcDate, &counter)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		t.CounterLabel = counter
		if t.Status == "WAITING" {
			var ahead int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM queue_tickets WHERE site_id=$1 AND tenant_service_id=$2 AND service_date=$3
				AND status='WAITING' AND sequence_no<$4`, siteID, svcID, svcDate, seq).Scan(&ahead); err != nil {
				return err
			}
			t.WaitingAhead = &ahead
		}
		d.Ticket = &t
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// CancelBooking melepas slot tepat sekali; berjalan serial dengan check-in lewat lock baris booking.
func (s *Service) CancelBooking(ctx context.Context, reference, token string) error {
	tid, _, _, err := s.publicTenantID(ctx)
	if err != nil {
		return err
	}
	err = s.db.WithTenant(ctx, tid, func(tx pgx.Tx) error {
		var id, slotID, status string
		err := tx.QueryRow(ctx, `SELECT id::text, slot_id::text, status FROM bookings
			WHERE reference_code=$1 AND qr_secret_hash=$2 FOR UPDATE`, reference, crypto.HashToken(token)).Scan(&id, &slotID, &status)
		if err != nil {
			return err
		}
		switch domain.BookingStatus(status) {
		case domain.BookingCancelled:
			return nil // idempotent
		case domain.BookingConfirmed:
		default:
			return domain.ErrStateConflict("booking can no longer be cancelled")
		}
		if _, err := tx.Exec(ctx, `UPDATE bookings SET status='CANCELLED', cancelled_at=now() WHERE id=$1`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE schedule_slots SET booked_count=booked_count-1 WHERE id=$1 AND booked_count>0`, slotID); err != nil {
			return err
		}
		if err := s.outbox(ctx, tx, tid, "booking.cancelled", map[string]any{"booking_id": id}); err != nil {
			return err
		}
		return s.audit(ctx, tx, auditEntry{TenantID: tid, ActorType: "CITIZEN", Action: "booking.cancel", ResourceType: "booking", ResourceID: id})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}
