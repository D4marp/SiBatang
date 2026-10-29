// Package service berisi use case aplikasi. Semua mutasi memakai satu transaksi DB bertenant (RLS)
// dan menulis audit/event/outbox secara atomik (AGENTS.md invarian 9).
package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/D4marp/SiBatang/backend/internal/config"
	"github.com/D4marp/SiBatang/backend/internal/crypto"
	"github.com/D4marp/SiBatang/backend/internal/db"
	"github.com/D4marp/SiBatang/backend/internal/domain"
	"github.com/D4marp/SiBatang/backend/internal/reqctx"
)

type Service struct {
	cfg config.Config
	db  *db.DB
	box *crypto.Box
	log *slog.Logger
	now func() time.Time
}

func New(cfg config.Config, d *db.DB, log *slog.Logger) (*Service, error) {
	box, err := crypto.NewBox(cfg.DataEncryptionKey)
	if err != nil {
		return nil, err
	}
	return &Service{cfg: cfg, db: d, box: box, log: log, now: time.Now}, nil
}

// SetClock hanya untuk pengujian.
func (s *Service) SetClock(f func() time.Time) { s.now = f }

type auditEntry struct {
	TenantID, OrgID      string
	ActorType, ActorID   string
	Action, ResourceType string
	ResourceID, Result   string
}

// audit menulis ke audit_events dalam transaksi yang sama dengan mutasinya. Tidak memuat PII/QR/nama.
func (s *Service) audit(ctx context.Context, tx pgx.Tx, e auditEntry) error {
	if e.ActorType == "" {
		e.ActorType = "USER"
	}
	if e.Result == "" {
		e.Result = "OK"
	}
	_, err := tx.Exec(ctx, `INSERT INTO audit_events
		(tenant_id, org_id, actor_type, actor_id, action, resource_type, resource_id, request_id, result)
		VALUES (NULLIF($1,'')::uuid, NULLIF($2,'')::uuid, $3, NULLIF($4,'')::uuid, $5, $6, NULLIF($7,'')::uuid, $8, $9)`,
		e.TenantID, e.OrgID, e.ActorType, e.ActorID, e.Action, e.ResourceType, e.ResourceID, reqctx.ID(ctx), e.Result)
	return err
}

func (s *Service) outbox(ctx context.Context, tx pgx.Tx, tenantID, topic string, payload map[string]any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox_events (tenant_id, topic, payload) VALUES ($1,$2,$3)`, tenantID, topic, b)
	return err
}

// queueEvent menulis event antrean tersanitasi dan mengembalikan id (cursor SSE).
func (s *Service) queueEvent(ctx context.Context, tx pgx.Tx, tenantID, siteID, ticketID, counterID, typ string, payload map[string]any) (int64, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO queue_events (tenant_id, site_id, ticket_id, counter_id, event_type, payload)
		VALUES ($1,$2,NULLIF($3,'')::uuid,NULLIF($4,'')::uuid,$5,$6) RETURNING id`,
		tenantID, siteID, ticketID, counterID, typ, b).Scan(&id)
	return id, err
}

// serviceDateSQL: tanggal layanan lokal WIB, bukan UTC midnight.
const serviceDateSQL = `(now() AT TIME ZONE 'Asia/Jakarta')::date`

// publicTenantID menemukan tenant publik default dan memastikan sudah PUBLISHED.
func (s *Service) publicTenantID(ctx context.Context) (id, brand, prefix string, err error) {
	err = s.db.WithPlain(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id, brand_name, booking_prefix FROM tenants WHERE code=$1 AND status='PUBLISHED'`,
			s.cfg.DefaultPublicTenant).Scan(&id, &brand, &prefix)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", domain.ErrNotFound
	}
	return
}
