package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/D4marp/SiBatang/backend/internal/domain"
)

type DisplayEntry struct {
	TicketNumber string    `json:"ticket_number"`
	CounterLabel string    `json:"counter_label"`
	Service      string    `json:"service"`
	Status       string    `json:"status"`
	CalledAt     time.Time `json:"called_at"`
}
type DisplaySnapshot struct {
	SiteCode    string         `json:"site_code"`
	Now         []DisplayEntry `json:"now_serving"`
	Waiting     map[string]int `json:"waiting_by_service"`
	LastEventID int64          `json:"last_event_id"`
	AsOf        time.Time      `json:"as_of"`
}

// DisplaySnapshot: hanya nomor, loket, layanan. Tidak ada nama/NIK/HP.
func (s *Service) DisplaySnapshot(ctx context.Context, dp *DevicePrincipal, deviceID string) (*DisplaySnapshot, error) {
	if err := requireDevice(dp, deviceID, domain.PermQueueDisplayRead); err != nil {
		return nil, err
	}
	out := &DisplaySnapshot{Now: []DisplayEntry{}, Waiting: map[string]int{}, AsOf: s.now().UTC()}
	err := s.db.WithTenant(ctx, dp.TenantID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT code FROM sites WHERE id=$1`, dp.SiteID).Scan(&out.SiteCode); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT t.display_number, c.label, sv.code, t.status, t.called_at FROM queue_tickets t
			JOIN counters c ON c.tenant_id=t.tenant_id AND c.id=t.called_counter_id
			JOIN tenant_services ts ON ts.tenant_id=t.tenant_id AND ts.id=t.tenant_service_id JOIN services sv ON sv.id=ts.service_id
			WHERE t.site_id=$1 AND t.status IN ('CALLED','SERVING') ORDER BY t.called_at DESC LIMIT 12`, dp.SiteID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var e DisplayEntry
			if err := rows.Scan(&e.TicketNumber, &e.CounterLabel, &e.Service, &e.Status, &e.CalledAt); err != nil {
				rows.Close()
				return err
			}
			out.Now = append(out.Now, e)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		wr, err := tx.Query(ctx, `SELECT sv.code, count(*) FROM queue_tickets t
			JOIN tenant_services ts ON ts.tenant_id=t.tenant_id AND ts.id=t.tenant_service_id JOIN services sv ON sv.id=ts.service_id
			WHERE t.site_id=$1 AND t.status='WAITING' AND t.service_date=`+serviceDateSQL+` GROUP BY sv.code`, dp.SiteID)
		if err != nil {
			return err
		}
		defer wr.Close()
		for wr.Next() {
			var code string
			var n int
			if err := wr.Scan(&code, &n); err != nil {
				return err
			}
			out.Waiting[code] = n
		}
		if err := wr.Err(); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT COALESCE(max(id),0) FROM queue_events WHERE site_id=$1`, dp.SiteID).Scan(&out.LastEventID)
	})
	return out, err
}

type PublicQueueEvent struct {
	ID      int64           `json:"-"`
	Type    string          `json:"-"`
	Payload json.RawMessage `json:"-"`
}

// DisplayEventsSince membaca queue_events tersanitasi (durable; aman setelah SSE terputus).
func (s *Service) DisplayEventsSince(ctx context.Context, dp *DevicePrincipal, deviceID string, after int64) ([]PublicQueueEvent, error) {
	if err := requireDevice(dp, deviceID, domain.PermQueueDisplayRead); err != nil {
		return nil, err
	}
	var out []PublicQueueEvent
	err := s.db.WithTenant(ctx, dp.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, event_type, payload FROM queue_events
			WHERE site_id=$1 AND id>$2 AND event_type IN ('queue.ticket_called','queue.ticket_recalled','queue.ticket_serving','queue.ticket_completed','queue.ticket_no_show')
			ORDER BY id LIMIT 200`, dp.SiteID, after)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e PublicQueueEvent
			if err := rows.Scan(&e.ID, &e.Type, &e.Payload); err != nil {
				return err
			}
			out = append(out, e)
		}
		return rows.Err()
	})
	return out, err
}

// BookingRevision untuk SSE privat warga (poll ringan berbasis revision tiket).
func (s *Service) BookingRevision(ctx context.Context, reference, token string) (*BookingDetail, error) {
	d, err := s.GetBooking(ctx, reference, token)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return d, err
}
