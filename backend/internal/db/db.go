// Package db: pool pgx, transaksi bertenant (RLS) dan migrator.
package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/D4marp/SiBatang/backend/internal/domain"
)

type DB struct{ Pool *pgxpool.Pool }

func Open(ctx context.Context, url string) (*DB, error) {
	p, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := p.Ping(ctx); err != nil {
		return nil, err
	}
	return &DB{Pool: p}, nil
}

func (d *DB) Close() { d.Pool.Close() }

type txKind int

const (
	kindTenant txKind = iota
	kindPlain
	kindReporting
)

const maxAttempts = 3

// WithTenant menjalankan fn dalam SATU transaksi: SET LOCAL ROLE sib_api + app.tenant_id lokal-transaksi.
// tenantID wajib sudah diverifikasi server-side (bukan input klien). Retry terbatas untuk serialization/deadlock.
func (d *DB) WithTenant(ctx context.Context, tenantID string, fn func(pgx.Tx) error) error {
	if !domain.IsUUID(tenantID) {
		return domain.ErrForbidden // tanpa tenant terverifikasi: tolak sebelum DB dipanggil
	}
	return d.run(ctx, kindTenant, tenantID, fn)
}

// WithPlain: role sib_api tanpa konteks tenant (tabel tenant-owned tampil kosong). Untuk auth/org/non-tenant.
func (d *DB) WithPlain(ctx context.Context, fn func(pgx.Tx) error) error {
	return d.run(ctx, kindPlain, "", fn)
}

// WithReporting: role sib_reporting (aggregate-only, descendant Polda). Read-only.
func (d *DB) WithReporting(ctx context.Context, fn func(pgx.Tx) error) error {
	return d.run(ctx, kindReporting, "", fn)
}

func (d *DB) run(ctx context.Context, kind txKind, tenantID string, fn func(pgx.Tx) error) error {
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		err = d.once(ctx, kind, tenantID, fn)
		if !retryable(err) {
			return err
		}
	}
	return err
}

func (d *DB) once(ctx context.Context, kind txKind, tenantID string, fn func(pgx.Tx) error) error {
	opts := pgx.TxOptions{}
	if kind == kindReporting {
		opts.AccessMode = pgx.ReadOnly
	}
	tx, err := d.Pool.BeginTx(ctx, opts)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	role := "sib_api"
	if kind == kindReporting {
		role = "sib_reporting"
	}
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+role); err != nil {
		return fmt.Errorf("set role: %w", err)
	}
	if kind == kindTenant {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
			return err
		}
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func retryable(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && (pe.Code == "40001" || pe.Code == "40P01")
}

// IsUniqueViolation memeriksa 23505.
func IsUniqueViolation(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}
