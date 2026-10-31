package service

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/D4marp/SiBatang/backend/internal/crypto"
	"github.com/D4marp/SiBatang/backend/internal/domain"
	"github.com/D4marp/SiBatang/backend/internal/oidc"
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

// --- OIDC login ---

const loginStateAAD = "oidc-login"

type LoginState struct{ State, Nonce, Verifier string }

// SealLoginState membungkus state/nonce/verifier PKCE dalam nilai terenkripsi+terautentikasi (disimpan di cookie pendek).
func (s *Service) SealLoginState(ls LoginState) string {
	raw := strings.Join([]string{ls.State, ls.Nonce, ls.Verifier, strconv.FormatInt(s.now().Add(10*time.Minute).Unix(), 10)}, "|")
	return base64.RawURLEncoding.EncodeToString(s.box.Seal([]byte(raw), []byte(loginStateAAD)))
}

func (s *Service) OpenLoginState(v string) (LoginState, error) {
	b, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil {
		return LoginState{}, domain.ErrUnauthenticated
	}
	plain, err := s.box.Open(b, []byte(loginStateAAD))
	if err != nil {
		return LoginState{}, domain.ErrUnauthenticated
	}
	f := strings.Split(string(plain), "|")
	exp, e := strconv.ParseInt(f[len(f)-1], 10, 64)
	if len(f) != 4 || e != nil || s.now().Unix() > exp {
		return LoginState{}, domain.ErrUnauthenticated
	}
	return LoginState{State: f[0], Nonce: f[1], Verifier: f[2]}, nil
}

var privilegedRoles = map[domain.Role]bool{
	domain.RolePlatformAdmin: true, domain.RolePoldaOrgAdmin: true, domain.RolePoldaMonitor: true,
	domain.RolePoldaSupervisor: true, domain.RolePolresAdmin: true, domain.RoleSiteManager: true,
}

// LoginWithOIDC membuat sesi BARU (rotasi) untuk pengguna yang sudah didaftarkan admin. Tidak ada auto-provision.
// requireMFA: peran privilese wajib membawa amr "mfa" dari IdP.
func (s *Service) LoginWithOIDC(ctx context.Context, claims *oidc.Claims, requireMFA bool) (token, csrf, landing string, err error) {
	var userID string
	var privileged, polda bool
	err = s.db.WithPlain(ctx, func(tx pgx.Tx) error {
		if e := tx.QueryRow(ctx, `SELECT id::text FROM users WHERE oidc_subject=$1 AND status='ACTIVE'`, claims.Subject).Scan(&userID); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT r.code, m.scope_type FROM memberships m JOIN roles r ON r.id=m.role_id
			WHERE m.user_id=$1 AND m.revoked_at IS NULL AND m.starts_at<=now() AND (m.expires_at IS NULL OR m.expires_at>now())`, userID)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var role, scope string
			if e := rows.Scan(&role, &scope); e != nil {
				return e
			}
			privileged = privileged || privilegedRoles[domain.Role(role)]
			polda = polda || scope == string(domain.ScopePoldaJateng)
		}
		return rows.Err()
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", domain.ErrForbidden // subject belum didaftarkan oleh admin
	}
	if err != nil {
		return "", "", "", err
	}
	if requireMFA && privileged && !claims.HasAMR("mfa") {
		return "", "", "", &domain.Error{Status: 403, Code: "MFA_REQUIRED", Detail: "multi-factor authentication is required for this account"}
	}
	token, csrf, err = s.CreateSession(ctx, userID, 8*time.Hour)
	if err != nil {
		return "", "", "", err
	}
	_ = s.db.WithPlain(ctx, func(tx pgx.Tx) error {
		return s.audit(ctx, tx, auditEntry{ActorID: userID, Action: "auth.login", ResourceType: "session"})
	})
	landing = "/"
	if polda {
		landing = "/polda/jateng/overview"
	}
	return token, csrf, landing, nil
}
