// Package domain berisi aturan bisnis murni (tanpa HTTP/DB): izin, state machine, nomor antrean, suara.
package domain

import "fmt"

// Error adalah error domain bertipe yang dipetakan transport ke application/problem+json.
type Error struct {
	Status int    // HTTP status
	Code   string // kode stabil, mis. SLOT_FULL
	Detail string // aman untuk publik, tanpa PII
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Detail) }

func newErr(status int, code, detail string) *Error {
	return &Error{Status: status, Code: code, Detail: detail}
}

var (
	ErrValidation        = func(detail string) *Error { return newErr(400, "VALIDATION_ERROR", detail) }
	ErrUnauthenticated   = newErr(401, "AUTH_REQUIRED", "authentication required")
	ErrForbidden         = newErr(403, "SCOPE_DENIED", "operation not permitted for this scope")
	ErrNotFound          = newErr(404, "NOT_FOUND", "resource not found")
	ErrSlotFull          = newErr(409, "SLOT_FULL", "slot is full or no longer open")
	ErrStateConflict     = func(detail string) *Error { return newErr(409, "STATE_CONFLICT", detail) }
	ErrIdempotencyReused = newErr(409, "IDEMPOTENCY_KEY_REUSED", "idempotency key reused with a different request")
	ErrZoneLeased        = newErr(409, "ZONE_LEASED_BY_OTHER", "another announcer holds the active lease for this zone")
	ErrStaleFencing      = newErr(409, "STALE_FENCING_TOKEN", "fencing token is stale")
	ErrBookingExpired    = newErr(410, "BOOKING_EXPIRED", "booking expired or cancelled")
	ErrBusinessRule      = func(detail string) *Error { return newErr(422, "SERVICE_POLICY_NOT_MET", detail) }
	ErrRateLimited       = newErr(429, "RATE_LIMITED", "too many requests")
	ErrNotConfigured     = func(detail string) *Error { return newErr(501, "NOT_CONFIGURED", detail) }
	ErrUnavailable       = newErr(503, "TEMPORARILY_UNAVAILABLE", "service temporarily unavailable")
)
