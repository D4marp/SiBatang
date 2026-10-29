package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Service) allTenantIDs(ctx context.Context) ([]string, error) {
	var ids []string
	err := s.db.WithPlain(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id::text FROM tenants WHERE status <> 'SUSPENDED'`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return rows.Err()
	})
	return ids, err
}

const metricsSQL = `
WITH d AS (SELECT (now() AT TIME ZONE 'Asia/Jakarta')::date AS today),
bk AS (
  SELECT b.tenant_service_id,
         count(*) FILTER (WHERE b.status IN ('CONFIRMED','CHECKED_IN')) AS booked,
         count(*) FILTER (WHERE b.status = 'CANCELLED') AS cancelled
    FROM bookings b JOIN schedule_slots sl ON sl.tenant_id=b.tenant_id AND sl.id=b.slot_id CROSS JOIN d
   WHERE sl.service_date = d.today GROUP BY 1),
tk AS (
  SELECT t.tenant_service_id, count(*) AS arrived,
         count(*) FILTER (WHERE t.status='WAITING') AS waiting,
         count(*) FILTER (WHERE t.status IN ('CALLED','SERVING')) AS called,
         count(*) FILTER (WHERE t.status='COMPLETED') AS served,
         count(*) FILTER (WHERE t.status='NO_SHOW') AS noshow,
         avg(extract(epoch FROM t.first_called_at - t.checked_in_at)) FILTER (WHERE t.first_called_at IS NOT NULL) AS avgw,
         percentile_cont(0.95) WITHIN GROUP (ORDER BY extract(epoch FROM t.first_called_at - t.checked_in_at))
              FILTER (WHERE t.first_called_at IS NOT NULL) AS p95
    FROM queue_tickets t CROSS JOIN d WHERE t.service_date = d.today GROUP BY 1)
INSERT INTO daily_service_metrics (tenant_id, site_code, service_code, service_date, booking_count, arrived_count, waiting_count,
   called_count, served_count, no_show_count, cancelled_count, avg_wait_seconds, p95_wait_seconds, data_freshness_at)
SELECT ts.tenant_id, si.code, sv.code, d.today, COALESCE(bk.booked,0), COALESCE(tk.arrived,0), COALESCE(tk.waiting,0),
       COALESCE(tk.called,0), COALESCE(tk.served,0), COALESCE(tk.noshow,0), COALESCE(bk.cancelled,0), tk.avgw, tk.p95, now()
  FROM tenant_services ts
  JOIN sites si ON si.tenant_id=ts.tenant_id AND si.id=ts.site_id
  JOIN services sv ON sv.id=ts.service_id
  CROSS JOIN d
  LEFT JOIN bk ON bk.tenant_service_id=ts.id LEFT JOIN tk ON tk.tenant_service_id=ts.id
ON CONFLICT (tenant_id, site_code, service_code, service_date) DO UPDATE SET
  booking_count=EXCLUDED.booking_count, arrived_count=EXCLUDED.arrived_count, waiting_count=EXCLUDED.waiting_count,
  called_count=EXCLUDED.called_count, served_count=EXCLUDED.served_count, no_show_count=EXCLUDED.no_show_count,
  cancelled_count=EXCLUDED.cancelled_count, avg_wait_seconds=EXCLUDED.avg_wait_seconds,
  p95_wait_seconds=EXCLUDED.p95_wait_seconds, data_freshness_at=EXCLUDED.data_freshness_at`

const healthSQL = `
INSERT INTO tenant_health_snapshots (tenant_id, devices_total, devices_online, announcers_online, announcement_failures_24h, as_of)
SELECT $1::uuid,
  count(*) FILTER (WHERE status='ACTIVE'),
  count(*) FILTER (WHERE status='ACTIVE' AND last_seen_at > now() - interval '2 minutes'),
  count(*) FILTER (WHERE status='ACTIVE' AND device_type='ANNOUNCER' AND last_seen_at > now() - interval '2 minutes'),
  (SELECT count(*) FROM queue_announcements WHERE state='FAILED' AND finished_at > now() - interval '24 hours'),
  now()
FROM devices
ON CONFLICT (tenant_id) DO UPDATE SET devices_total=EXCLUDED.devices_total, devices_online=EXCLUDED.devices_online,
  announcers_online=EXCLUDED.announcers_online, announcement_failures_24h=EXCLUDED.announcement_failures_24h, as_of=EXCLUDED.as_of`

// RefreshMetrics menghitung agregat tanpa PII per tenant dalam konteks RLS tenant tersebut.
func (s *Service) RefreshMetrics(ctx context.Context) error {
	ids, err := s.allTenantIDs(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.db.WithTenant(ctx, id, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, metricsSQL); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, healthSQL, id)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}

// SweepAnnouncements mengakhiri job kedaluwarsa dan memberi alert outbox; tidak pernah mengubah status tiket.
func (s *Service) SweepAnnouncements(ctx context.Context) error {
	ids, err := s.allTenantIDs(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.db.WithTenant(ctx, id, func(tx pgx.Tx) error {
			tag, err := tx.Exec(ctx, `UPDATE queue_announcements SET state='EXPIRED', finished_at=now()
				WHERE (state='PENDING' AND expires_at<=now()) OR (state IN ('CLAIMED','PLAYING') AND lease_until<=now() AND expires_at<=now())`)
			if err != nil {
				return err
			}
			if n := tag.RowsAffected(); n > 0 {
				return s.outbox(ctx, tx, id, "announcement.expired", map[string]any{"count": n})
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

type OutboxHandler func(ctx context.Context, topic string, payload json.RawMessage) error

const outboxMaxAttempts = 8

// DrainOutbox memproses outbox dengan SKIP LOCKED + retry/backoff; attempts>=max = DLQ untuk inspeksi manual.
// Topik tanpa handler (belum ada provider notifikasi) ditandai selesai secara eksplisit.
func (s *Service) DrainOutbox(ctx context.Context, handlers map[string]OutboxHandler) (int, error) {
	ids, err := s.allTenantIDs(ctx)
	if err != nil {
		return 0, err
	}
	done := 0
	for _, id := range ids {
		err := s.db.WithTenant(ctx, id, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `SELECT id::text, topic, payload, attempts FROM outbox_events
				WHERE processed_at IS NULL AND available_at<=now() AND attempts<$1 ORDER BY created_at LIMIT 100 FOR UPDATE SKIP LOCKED`, outboxMaxAttempts)
			if err != nil {
				return err
			}
			type ev struct {
				id, topic string
				payload   json.RawMessage
				attempts  int
			}
			var batch []ev
			for rows.Next() {
				var e ev
				if err := rows.Scan(&e.id, &e.topic, &e.payload, &e.attempts); err != nil {
					rows.Close()
					return err
				}
				batch = append(batch, e)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
			for _, e := range batch {
				if h := handlers[e.topic]; h != nil {
					if herr := h(ctx, e.topic, e.payload); herr != nil {
						s.log.Warn("outbox handler failed", slog.String("topic", e.topic), slog.Int("attempts", e.attempts+1))
						if _, err := tx.Exec(ctx, `UPDATE outbox_events SET attempts=attempts+1, last_error=$2,
							available_at=now()+make_interval(secs=>10*power(2,attempts+1)) WHERE id=$1`, e.id, "handler error"); err != nil {
							return err
						}
						continue
					}
				}
				if _, err := tx.Exec(ctx, `UPDATE outbox_events SET processed_at=now() WHERE id=$1`, e.id); err != nil {
					return err
				}
				done++
			}
			return nil
		})
		if err != nil {
			return done, err
		}
	}
	return done, nil
}

// RunWorker menjalankan siklus periodik sampai ctx selesai.
func (s *Service) RunWorker(ctx context.Context, handlers map[string]OutboxHandler) {
	t := time.NewTicker(s.cfg.MetricsRefreshInterval)
	defer t.Stop()
	for {
		for name, fn := range map[string]func() error{
			"metrics":       func() error { return s.RefreshMetrics(ctx) },
			"announcements": func() error { return s.SweepAnnouncements(ctx) },
			"outbox":        func() error { _, err := s.DrainOutbox(ctx, handlers); return err },
		} {
			if err := fn(); err != nil && ctx.Err() == nil {
				s.log.Error("worker task failed", slog.String("task", name), slog.String("error", err.Error()))
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
