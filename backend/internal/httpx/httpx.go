// Package httpx: helper transport (problem+json RFC 9457, request id, log tersanitasi, rate limit).
package httpx

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/D4marp/SiBatang/backend/internal/domain"
	"github.com/D4marp/SiBatang/backend/internal/reqctx"
)

type Envelope struct {
	Data any `json:"data"`
	Meta any `json:"meta,omitempty"`
}

func JSON(w http.ResponseWriter, status int, data any, meta any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{Data: data, Meta: meta})
}

type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Code      string `json:"code"`
	Detail    string `json:"detail"`
	RequestID string `json:"request_id"`
}

// WriteProblem memetakan error ke application/problem+json. Error tak dikenal => 500 generik (tanpa detail internal).
func WriteProblem(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	var de *domain.Error
	if !errors.As(err, &de) {
		if log != nil {
			log.Error("unhandled error", slog.String("request_id", reqctx.ID(r.Context())), slog.String("path", r.Pattern), slog.String("error", err.Error()))
		}
		de = &domain.Error{Status: 500, Code: "INTERNAL_ERROR", Detail: "internal error"}
	}
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	if de.Status == 429 {
		w.Header().Set("Retry-After", "5")
	}
	w.WriteHeader(de.Status)
	_ = json.NewEncoder(w).Encode(Problem{
		Type: "/problems/" + strings.ToLower(strings.ReplaceAll(de.Code, "_", "-")), Title: http.StatusText(de.Status),
		Status: de.Status, Code: de.Code, Detail: de.Detail, RequestID: reqctx.ID(r.Context()),
	})
}

// Decode membatasi ukuran body dan menolak field tak dikenal.
func Decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return domain.ErrValidation("request body is not valid JSON for this endpoint")
	}
	return nil
}

var reqIDRe = regexp.MustCompile(`^[A-Za-z0-9._-]{8,64}$`)

func Middleware(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if !reqIDRe.MatchString(id) {
			b := make([]byte, 12)
			_, _ = rand.Read(b)
			id = hex.EncodeToString(b)
		}
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		r = r.WithContext(reqctx.With(r.Context(), id))
		sw := &statusWriter{ResponseWriter: w, status: 200}
		start := time.Now()
		defer func() {
			if rec := recover(); rec != nil {
				log.Error("panic", slog.String("request_id", id))
				if !sw.wrote {
					WriteProblem(sw, r, log, errors.New("panic"))
				}
			}
			// Log hanya pola rute (tanpa query string/ID/token) agar rahasia tidak masuk log.
			log.Info("request", slog.String("request_id", id), slog.String("method", r.Method),
				slog.String("route", r.Pattern), slog.Int("status", sw.status), slog.Duration("dur", time.Since(start)))
		}()
		next.ServeHTTP(sw, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusWriter) WriteHeader(c int) {
	s.status, s.wrote = c, true
	s.ResponseWriter.WriteHeader(c)
}
func (s *statusWriter) Write(b []byte) (int, error) {
	s.wrote = true
	return s.ResponseWriter.Write(b)
}
func (s *statusWriter) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Limiter: token bucket per kunci (IP) di memori. Cukup untuk satu instance; ganti ke store bersama saat scale-out (ADR).
type Limiter struct {
	mu      sync.Mutex
	perMin  int
	buckets map[string]*bucket
}
type bucket struct {
	tokens float64
	last   time.Time
}

func NewLimiter(perMin int) *Limiter { return &Limiter{perMin: perMin, buckets: map[string]*bucket{}} }

func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b := l.buckets[key]
	if b == nil {
		if len(l.buckets) > 50000 {
			l.buckets = map[string]*bucket{}
		}
		b = &bucket{tokens: float64(l.perMin), last: now}
		l.buckets[key] = b
	}
	b.tokens += now.Sub(b.last).Minutes() * float64(l.perMin)
	if b.tokens > float64(l.perMin) {
		b.tokens = float64(l.perMin)
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func ClientIP(r *http.Request) string {
	// Di belakang reverse proxy tepercaya, proxy harus menimpa X-Forwarded-For; ambil entri paling kanan yang ditambahkan proxy.
	if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
		parts := strings.Split(xf, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}
