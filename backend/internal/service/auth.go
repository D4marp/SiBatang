package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/D4marp/SiBatang/backend/internal/crypto"
	"github.com/D4marp/SiBatang/backend/internal/domain"
)

type StaffPrincipal struct {
	UserID      string
	CSRFToken   string
	Memberships []domain.Membership
	TenantIDs   []string // tenant yang boleh dijadikan konteks (dari memberships, bukan input klien)
}

func (p *StaffPrincipal) PoldaOrgIDs() []string {
	var out []string
	for _, m := range p.Memberships {
		if m.ScopeType == domain.ScopePoldaJateng {
			out = append(out, m.ScopeID)
		}
	}
	return out
}

// CreateSession dipakai callback OIDC (belum diaktifkan) dan alat operasional non-produksi.
func (s *Service) CreateSession(ctx context.Context, userID string, ttl time.Duration) (token, csrf string, err error) {
	token, csrf = crypto.RandomToken(), crypto.RandomToken()
	err = s.db.WithPlain(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, csrf_token, expires_at) VALUES ($1,$2,$3,$4)`,
			crypto.HashToken(token), userID, csrf, s.now().Add(ttl))
		return e
	})
	return
}

func (s *Service) RevokeSession(ctx context.Context, token string) error {
	return s.db.WithPlain(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=now() WHERE token_hash=$1 AND revoked_at IS NULL`, crypto.HashToken(token))
		return err
	})
}

// AuthenticateSession memvalidasi sesi server-side dan memuat memberships aktif (revocation segera berlaku).
func (s *Service) AuthenticateSession(ctx context.Context, token string) (*StaffPrincipal, error) {
	if token == "" {
		return nil, domain.ErrUnauthenticated
	}
	p := &StaffPrincipal{}
	err := s.db.WithPlain(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT s.user_id, s.csrf_token FROM sessions s JOIN users u ON u.id=s.user_id
			WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>now() AND u.status='ACTIVE'`,
			crypto.HashToken(token)).Scan(&p.UserID, &p.CSRFToken)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT r.code, m.scope_type, m.scope_id::text, COALESCE(m.tenant_id::text,'')
			FROM memberships m JOIN roles r ON r.id=m.role_id
			WHERE m.user_id=$1 AND m.revoked_at IS NULL AND m.starts_at<=now() AND (m.expires_at IS NULL OR m.expires_at>now())`, p.UserID)
		if err != nil {
			return err
		}
		defer rows.Close()
		seen := map[string]bool{}
		for rows.Next() {
			var role, scope, sid, tid string
			if err := rows.Scan(&role, &scope, &sid, &tid); err != nil {
				return err
			}
			p.Memberships = append(p.Memberships, domain.Membership{Role: domain.Role(role), ScopeType: domain.ScopeType(scope), ScopeID: sid})
			if tid != "" && !seen[tid] {
				seen[tid] = true
				p.TenantIDs = append(p.TenantIDs, tid)
			}
		}
		return rows.Err()
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUnauthenticated
	}
	return p, err
}

type DevicePrincipal struct {
	DeviceID, TenantID, SiteID, DeviceType, AudioZoneID, CounterID string
}

// AuthenticateDevice: bearer "<device_id>.<secret>". Kredensial dicari sebelum tenant diketahui,
// lalu status perangkat diverifikasi di dalam konteks tenant (RLS).
func (s *Service) AuthenticateDevice(ctx context.Context, bearer string) (*DevicePrincipal, error) {
	id, secret, ok := strings.Cut(bearer, ".")
	if !ok || !domain.IsUUID(id) || secret == "" {
		return nil, domain.ErrUnauthenticated
	}
	var tenantID string
	var hash []byte
	err := s.db.WithPlain(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT tenant_id::text, secret_hash FROM device_credentials WHERE device_id=$1 AND revoked_at IS NULL`, id).
			Scan(&tenantID, &hash)
	})
	if err != nil || !crypto.Equal(hash, crypto.HashToken(secret)) {
		return nil, domain.ErrUnauthenticated
	}
	p := &DevicePrincipal{DeviceID: id, TenantID: tenantID}
	err = s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT site_id::text, device_type, COALESCE(audio_zone_id::text,''), COALESCE(counter_id::text,'')
			FROM devices WHERE id=$1 AND status='ACTIVE'`, id).Scan(&p.SiteID, &p.DeviceType, &p.AudioZoneID, &p.CounterID)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUnauthenticated // dicabut/belum aktif
	}
	return p, err
}

// requireDevice memastikan device principal sama dengan device pada path dan punya permission.
func requireDevice(p *DevicePrincipal, pathDeviceID string, perm domain.Permission) error {
	if p == nil {
		return domain.ErrUnauthenticated
	}
	if p.DeviceID != pathDeviceID || !domain.DeviceAllows(p.DeviceType, perm) {
		return domain.ErrForbidden
	}
	return nil
}

var errNotHere = errors.New("resource not in this tenant")

type locator func(ctx context.Context, tx pgx.Tx) (domain.Resource, bool, error)

// staffTx menemukan sumber daya di tenant-tenant milik staf (konteks dari membership, bukan klien),
// memeriksa permission terhadap scope sumber daya yang terverifikasi, lalu menjalankan work di transaksi yang sama.
func (s *Service) staffTx(ctx context.Context, p *StaffPrincipal, perm domain.Permission, locate locator,
	work func(tx pgx.Tx, res domain.Resource) error) error {
	if p == nil {
		return domain.ErrUnauthenticated
	}
	for _, tid := range p.TenantIDs {
		err := s.db.WithTenant(ctx, tid, func(tx pgx.Tx) error {
			res, ok, err := locate(ctx, tx)
			if err != nil {
				return err
			}
			if !ok {
				return errNotHere
			}
			res.TenantID = tid
			if !domain.Allow(p.Memberships, perm, res) {
				return domain.ErrForbidden
			}
			return work(tx, res)
		})
		if errors.Is(err, errNotHere) {
			continue
		}
		return err
	}
	return domain.ErrNotFound
}

func locateRow(query string, id string, fill func(*domain.Resource) []any) locator {
	return func(ctx context.Context, tx pgx.Tx) (domain.Resource, bool, error) {
		var res domain.Resource
		err := tx.QueryRow(ctx, query, id).Scan(fill(&res)...)
		if errors.Is(err, pgx.ErrNoRows) {
			return res, false, nil
		}
		return res, err == nil, err
	}
}

func locateCounter(id string) locator {
	return locateRow(`SELECT site_id::text, id::text, audio_zone_id::text FROM counters WHERE id=$1`, id,
		func(r *domain.Resource) []any { return []any{&r.SiteID, &r.CounterID, &r.AudioZoneID} })
}

func locateSite(id string) locator {
	return locateRow(`SELECT id::text FROM sites WHERE id=$1`, id, func(r *domain.Resource) []any { return []any{&r.SiteID} })
}

func locateTicket(id string) locator {
	return locateRow(`SELECT t.site_id::text, COALESCE(t.called_counter_id::text,''), COALESCE(c.audio_zone_id::text,'')
		FROM queue_tickets t LEFT JOIN counters c ON c.tenant_id=t.tenant_id AND c.id=t.called_counter_id WHERE t.id=$1`, id,
		func(r *domain.Resource) []any { return []any{&r.SiteID, &r.CounterID, &r.AudioZoneID} })
}

func locateTenantService(id string) locator {
	return locateRow(`SELECT site_id::text FROM tenant_services WHERE id=$1`, id, func(r *domain.Resource) []any { return []any{&r.SiteID} })
}
