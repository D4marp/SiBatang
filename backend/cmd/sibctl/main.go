// Command sibctl: alat operasional (migrasi, sinkron role, pembuatan user/membership bernama, sesi dev non-produksi).
// Memakai kredensial MIGRATOR (owner) melalui MIGRATOR_DATABASE_URL — BUKAN kredensial runtime API.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/D4marp/SiBatang/backend/internal/crypto"
	"github.com/D4marp/SiBatang/backend/internal/db"
	"github.com/D4marp/SiBatang/backend/migrations"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	ctx := context.Background()
	url := os.Getenv("MIGRATOR_DATABASE_URL")
	if url == "" {
		fatal("MIGRATOR_DATABASE_URL is required")
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		fatal(err.Error())
	}
	defer pool.Close()

	switch os.Args[1] {
	case "migrate":
		applied, err := db.Migrate(ctx, pool, migrations.FS)
		if err != nil {
			fatal(err.Error())
		}
		if err := db.SyncRoles(ctx, pool); err != nil {
			fatal(err.Error())
		}
		fmt.Printf("applied %d migration(s): %v\n", len(applied), applied)
	case "user-add": // user bernama terhubung ke subject OIDC; tidak ada password di sistem ini
		fs := flag.NewFlagSet("user-add", flag.ExitOnError)
		sub := fs.String("subject", "", "OIDC subject (sub)")
		name := fs.String("name", "", "display name")
		_ = fs.Parse(os.Args[2:])
		if *sub == "" {
			fatal("--subject required")
		}
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO users (oidc_subject, display_name) VALUES ($1, NULLIF($2,''))
			ON CONFLICT (oidc_subject) DO UPDATE SET display_name=COALESCE(NULLIF($2,''), users.display_name) RETURNING id::text`, *sub, *name).Scan(&id); err != nil {
			fatal(err.Error())
		}
		fmt.Println(id)
	case "membership-add":
		fs := flag.NewFlagSet("membership-add", flag.ExitOnError)
		sub := fs.String("subject", "", "OIDC subject")
		role := fs.String("role", "", "role code, e.g. POLDA_MONITOR")
		scopeType := fs.String("scope-type", "", "POLDA_JATENG|TENANT|SITE|COUNTER|AUDIO_ZONE|JATENG_PLATFORM")
		scopeRef := fs.String("scope", "", "org/tenant code, or UUID for site/counter/zone")
		_ = fs.Parse(os.Args[2:])
		var scopeID, tenantID string
		var q string
		switch *scopeType {
		case "POLDA_JATENG":
			q = `SELECT id::text, '' FROM organizations WHERE code=$1 AND org_type='POLDA'`
		case "TENANT":
			q = `SELECT id::text, id::text FROM tenants WHERE code=$1`
		case "SITE":
			q = `SELECT id::text, tenant_id::text FROM sites WHERE id::text=$1`
		case "COUNTER":
			q = `SELECT id::text, tenant_id::text FROM counters WHERE id::text=$1`
		case "AUDIO_ZONE":
			q = `SELECT id::text, tenant_id::text FROM audio_zones WHERE id::text=$1`
		default:
			fatal("unsupported --scope-type")
		}
		if err := pool.QueryRow(ctx, q, *scopeRef).Scan(&scopeID, &tenantID); err != nil {
			fatal("scope not found: " + err.Error())
		}
		tag, err := pool.Exec(ctx, `INSERT INTO memberships (user_id, role_id, scope_type, scope_id, tenant_id)
			SELECT u.id, r.id, $3, $4, NULLIF($5,'')::uuid FROM users u, roles r WHERE u.oidc_subject=$1 AND r.code=$2`,
			*sub, *role, *scopeType, scopeID, tenantID)
		if err != nil || tag.RowsAffected() != 1 {
			fatal(fmt.Sprintf("membership not created (check subject/role): %v", err))
		}
		fmt.Println("membership created")
	case "dev-session": // hanya non-produksi; produksi memakai OIDC
		if os.Getenv("APP_ENV") == "production" || os.Getenv("APP_ENV") == "" {
			fatal("dev-session is disabled unless APP_ENV is explicitly non-production (e.g. development, staging)")
		}
		fs := flag.NewFlagSet("dev-session", flag.ExitOnError)
		sub := fs.String("subject", "", "OIDC subject")
		_ = fs.Parse(os.Args[2:])
		tok, csrf := crypto.RandomToken(), crypto.RandomToken()
		tag, err := pool.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, csrf_token, expires_at)
			SELECT $2, id, $3, $4 FROM users WHERE oidc_subject=$1`, *sub, crypto.HashToken(tok), csrf, time.Now().Add(8*time.Hour))
		if err != nil || tag.RowsAffected() != 1 {
			fatal("session not created (unknown subject?)")
		}
		fmt.Printf("cookie: sib_session=%s\ncsrf:   %s\n", tok, csrf)
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: sibctl migrate | user-add | membership-add | dev-session")
	os.Exit(2)
}
func fatal(m string) { fmt.Fprintln(os.Stderr, "error:", m); os.Exit(1) }
