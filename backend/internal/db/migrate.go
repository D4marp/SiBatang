package db

import (
	"context"
	"fmt"
	"io/fs"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/D4marp/SiBatang/backend/internal/domain"
)

// Migrate menerapkan migrasi maju berurutan di bawah advisory lock (runner tunggal).
// Harus memakai kredensial MIGRATOR (owner), bukan kredensial runtime API.
func Migrate(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) ([]string, error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock(727274)"); err != nil {
		return nil, err
	}
	defer func() { _, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock(727274)") }()

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return nil, err
	}
	entries, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(entries)
	var applied []string
	for _, name := range entries {
		var exists bool
		if err := conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)", name).Scan(&exists); err != nil {
			return applied, err
		}
		if exists {
			continue
		}
		sqlText, err := fs.ReadFile(fsys, name)
		if err != nil {
			return applied, err
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return applied, err
		}
		if _, err := tx.Exec(ctx, string(sqlText)); err != nil {
			_ = tx.Rollback(ctx)
			return applied, fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", name); err != nil {
			_ = tx.Rollback(ctx)
			return applied, err
		}
		if err := tx.Commit(ctx); err != nil {
			return applied, err
		}
		applied = append(applied, name)
	}
	return applied, nil
}

// SyncRoles menyelaraskan tabel roles/permissions/role_permissions dengan domain.RolePermissions (idempotent).
func SyncRoles(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := syncRoles(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func syncRoles(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, "DELETE FROM role_permissions"); err != nil {
		return err
	}
	for role, perms := range domain.RolePermissions {
		if _, err := tx.Exec(ctx, "INSERT INTO roles (code) VALUES ($1) ON CONFLICT DO NOTHING", string(role)); err != nil {
			return err
		}
		for _, p := range perms {
			if _, err := tx.Exec(ctx, "INSERT INTO permissions (code) VALUES ($1) ON CONFLICT DO NOTHING", string(p)); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO role_permissions (role_id, permission_id)
				SELECT r.id, p.id FROM roles r, permissions p WHERE r.code=$1 AND p.code=$2`, string(role), string(p)); err != nil {
				return err
			}
		}
	}
	return nil
}
