package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/D4marp/SiBatang/backend/internal/domain"
	"github.com/D4marp/SiBatang/backend/internal/httpx"
	"github.com/D4marp/SiBatang/backend/internal/service"
)

// Slug publik pilot ditetapkan dari konfigurasi; tidak ada selector provinsi/Polres (10_POLDA_FIRST_PILOT.md).
func (s *Server) checkSlug(r *http.Request) error {
	if r.PathValue("slug") != strings.ToLower(strings.ReplaceAll(s.cfg.DefaultPublicTenant, "_", "-")) {
		return domain.ErrNotFound
	}
	return nil
}

func bookingToken(r *http.Request) string { return r.Header.Get("X-Booking-Token") }

func (s *Server) publicRoutes(h func(string, http.HandlerFunc)) {
	h("GET /public/tenants/{slug}/catalog", s.public(func(w http.ResponseWriter, r *http.Request) error {
		if err := s.checkSlug(r); err != nil {
			return err
		}
		c, err := s.svc.PublicCatalog(r.Context())
		if err != nil {
			return err
		}
		httpx.JSON(w, 200, c, nil)
		return nil
	}))
	h("GET /public/tenants/{slug}/slots", s.public(func(w http.ResponseWriter, r *http.Request) error {
		if err := s.checkSlug(r); err != nil {
			return err
		}
		slots, err := s.svc.PublicSlots(r.Context(), r.URL.Query().Get("service_id"), r.URL.Query().Get("date"))
		if err != nil {
			return err
		}
		httpx.JSON(w, 200, slots, map[string]any{"site_timezone": "Asia/Jakarta"})
		return nil
	}))
	h("POST /public/bookings", s.public(func(w http.ResponseWriter, r *http.Request) error {
		var req service.BookingRequest
		if err := httpx.Decode(w, r, &req); err != nil {
			return err
		}
		res, replay, err := s.svc.CreateBooking(r.Context(), r.Header.Get("Idempotency-Key"), req)
		if err != nil {
			return err
		}
		status := 201
		if replay {
			status = 200
			w.Header().Set("Idempotent-Replay", "true")
		}
		httpx.JSON(w, status, res, nil)
		return nil
	}))
	getBooking := s.public(func(w http.ResponseWriter, r *http.Request) error {
		d, err := s.svc.GetBooking(r.Context(), r.PathValue("reference"), bookingToken(r))
		if err != nil {
			return err
		}
		httpx.JSON(w, 200, d, nil)
		return nil
	})
	h("GET /public/bookings/{reference}", getBooking)
	h("GET /public/bookings/{reference}/status", getBooking)
	h("POST /public/bookings/{reference}/cancel", s.public(func(w http.ResponseWriter, r *http.Request) error {
		if err := s.svc.CancelBooking(r.Context(), r.PathValue("reference"), bookingToken(r)); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	}))
	h("GET /public/bookings/{reference}/events", s.public(func(w http.ResponseWriter, r *http.Request) error {
		ref, tok := r.PathValue("reference"), bookingToken(r)
		var last string
		return s.serveSSE(w, r,
			func(ctx context.Context, _ int64) ([]sseEvent, error) {
				d, err := s.svc.GetBooking(ctx, ref, tok)
				if err != nil {
					return nil, err
				}
				sig := d.Status
				if d.Ticket != nil {
					sig = fmt.Sprintf("%s|%s|%d", d.Status, d.Ticket.Status, d.Ticket.Revision)
				}
				if sig == last {
					return nil, nil
				}
				last = sig
				b, _ := json.Marshal(d)
				return []sseEvent{{ID: time.Now().UnixMilli(), Name: "booking.status", Data: string(b)}}, nil
			},
			func() error { _, err := s.svc.GetBooking(r.Context(), ref, tok); return err })
	}))
}

func (s *Server) authRoutes(h func(string, http.HandlerFunc)) {
	// Login staf memakai OIDC IdP resmi (MFA di IdP). Alur code+PKCE belum dikonfigurasi di repo ini.
	h("GET /auth/staff/login", func(w http.ResponseWriter, r *http.Request) {
		s.fail(w, r, domain.ErrNotConfigured("OIDC identity provider is not configured (see docs/OIDC.md)"))
	})
	h("GET /auth/session", s.staff(func(w http.ResponseWriter, r *http.Request, p *service.StaffPrincipal) error {
		type m struct {
			Role      string `json:"role"`
			ScopeType string `json:"scope_type"`
			ScopeID   string `json:"scope_id"`
		}
		ms := make([]m, 0, len(p.Memberships))
		for _, x := range p.Memberships {
			ms = append(ms, m{string(x.Role), string(x.ScopeType), x.ScopeID})
		}
		httpx.JSON(w, 200, map[string]any{"user_id": p.UserID, "csrf_token": p.CSRFToken, "memberships": ms}, nil)
		return nil
	}))
	h("POST /auth/logout", s.staff(func(w http.ResponseWriter, r *http.Request, p *service.StaffPrincipal) error {
		if c, err := r.Cookie(s.cfg.SessionCookieName); err == nil {
			if err := s.svc.RevokeSession(r.Context(), c.Value); err != nil {
				return err
			}
		}
		http.SetCookie(w, &http.Cookie{Name: s.cfg.SessionCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteStrictMode})
		w.WriteHeader(http.StatusNoContent)
		return nil
	}))
}
