// Package api: handler HTTP /api/v1. Tidak ada logika bisnis di sini; semuanya di internal/service.
package api

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/D4marp/SiBatang/backend/internal/config"
	"github.com/D4marp/SiBatang/backend/internal/db"
	"github.com/D4marp/SiBatang/backend/internal/domain"
	"github.com/D4marp/SiBatang/backend/internal/httpx"
	"github.com/D4marp/SiBatang/backend/internal/service"
)

type Server struct {
	svc     *service.Service
	db      *db.DB
	cfg     config.Config
	log     *slog.Logger
	limiter *httpx.Limiter
}

func New(svc *service.Service, d *db.DB, cfg config.Config, log *slog.Logger) *Server {
	return &Server{svc: svc, db: d, cfg: cfg, log: log, limiter: httpx.NewLimiter(cfg.PublicRateLimitPerMin)}
}

type (
	publicFn func(w http.ResponseWriter, r *http.Request) error
	staffFn  func(w http.ResponseWriter, r *http.Request, p *service.StaffPrincipal) error
	deviceFn func(w http.ResponseWriter, r *http.Request, dp *service.DevicePrincipal) error
)

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpx.WriteProblem(w, r, s.log, err)
}

// public: tanpa autentikasi, dengan rate limit per IP.
func (s *Server) public(h publicFn) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.limiter.Allow(httpx.ClientIP(r)) {
			s.fail(w, r, domain.ErrRateLimited)
			return
		}
		if err := h(w, r); err != nil {
			s.fail(w, r, err)
		}
	}
}

func (s *Server) sessionPrincipal(r *http.Request) (*service.StaffPrincipal, error) {
	c, err := r.Cookie(s.cfg.SessionCookieName)
	if err != nil {
		return nil, domain.ErrUnauthenticated
	}
	return s.svc.AuthenticateSession(r.Context(), c.Value)
}

// staff: sesi cookie HttpOnly + CSRF (double submit via header) untuk mutasi.
func (s *Server) staff(h staffFn) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := s.sessionPrincipal(r)
		if err == nil && r.Method != http.MethodGet && r.Method != http.MethodHead {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(p.CSRFToken)) != 1 {
				err = domain.ErrForbidden
			}
		}
		if err == nil {
			err = h(w, r, p)
		}
		if err != nil {
			s.fail(w, r, err)
		}
	}
}

func (s *Server) deviceAuth(r *http.Request) (*service.DevicePrincipal, error) {
	tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Device ")
	if !ok {
		return nil, domain.ErrUnauthenticated
	}
	return s.svc.AuthenticateDevice(r.Context(), tok)
}

func (s *Server) device(h deviceFn) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dp, err := s.deviceAuth(r)
		if err == nil {
			err = h(w, r, dp)
		}
		if err != nil {
			s.fail(w, r, err)
		}
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	const v = "/api/v1"
	h := func(pattern string, fn http.HandlerFunc) {
		method, path, _ := strings.Cut(pattern, " ")
		mux.HandleFunc(method+" "+v+path, fn)
	}

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, 200, map[string]string{"status": "ok"}, nil)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.db.Pool.Ping(ctx); err != nil {
			s.fail(w, r, domain.ErrUnavailable)
			return
		}
		httpx.JSON(w, 200, map[string]string{"status": "ready"}, nil)
	})

	s.publicRoutes(h)
	s.authRoutes(h)
	s.staffRoutes(h)
	s.poldaRoutes(h)
	s.deviceRoutes(h)
	return httpx.Middleware(s.log, mux)
}

func pathUUID(r *http.Request, name string) (string, error) {
	v := r.PathValue(name)
	if !domain.IsUUID(v) {
		return "", domain.ErrNotFound
	}
	return v, nil
}

// --- SSE ---

type sseEvent struct {
	ID   int64
	Name string
	Data string
}

// serveSSE: stream dengan Last-Event-ID, heartbeat, dan revalidasi otorisasi tiap siklus (revocation memutus stream).
func (s *Server) serveSSE(w http.ResponseWriter, r *http.Request, poll func(ctx context.Context, after int64) ([]sseEvent, error), reauth func() error) error {
	fl, ok := w.(http.Flusher)
	if !ok {
		return domain.ErrUnavailable
	}
	after, _ := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	if err := reauth(); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	fmt.Fprint(w, "retry: 3000\n\n")
	fl.Flush()
	poller := time.NewTicker(time.Second)
	beat := time.NewTicker(s.cfg.SSEHeartbeat)
	defer poller.Stop()
	defer beat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return nil
		case <-beat.C:
			fmt.Fprint(w, ": heartbeat\n\n")
			fl.Flush()
		case <-poller.C:
			if err := reauth(); err != nil {
				return nil
			}
			evs, err := poll(r.Context(), after)
			if err != nil {
				return nil
			}
			for _, e := range evs {
				fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", e.ID, e.Name, e.Data)
				if e.ID > after {
					after = e.ID
				}
			}
			if len(evs) > 0 {
				fl.Flush()
			}
		}
	}
}
