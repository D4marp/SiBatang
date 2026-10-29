package service

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/D4marp/SiBatang/backend/internal/domain"
)

type counterRow struct {
	ID, SiteID, ZoneID, Label, State, Operator string
	AutoAnnounce, AutoAdvance                  bool
	MaxRecall, CooldownSec, TTLSec, Revision   int
}

func (s *Service) lockCounter(ctx context.Context, tx pgx.Tx, id string) (*counterRow, error) {
	c := &counterRow{ID: id}
	err := tx.QueryRow(ctx, `SELECT site_id::text, audio_zone_id::text, label, state, COALESCE(assigned_operator_id::text,''),
		auto_announce_on_call, auto_advance_on_ready, max_recall_per_ticket, recall_cooldown_seconds, stale_job_ttl_seconds, readiness_revision
		FROM counters WHERE id=$1 FOR UPDATE`, id).
		Scan(&c.SiteID, &c.ZoneID, &c.Label, &c.State, &c.Operator, &c.AutoAnnounce, &c.AutoAdvance, &c.MaxRecall, &c.CooldownSec, &c.TTLSec, &c.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return c, err
}

type CallResult struct {
	CounterID          string `json:"counter_id"`
	CounterState       string `json:"counter_state"`
	TicketID           string `json:"ticket_id,omitempty"`
	DisplayNumber      string `json:"display_number,omitempty"`
	Status             string `json:"status,omitempty"`
	CallEventID        int64  `json:"call_event_id,omitempty"`
	AnnouncementStatus string `json:"announcement_status,omitempty"` // QUEUED | DISABLED
	ReadinessRevision  int    `json:"readiness_revision"`
}

func (c *counterRow) assignedTo(userID string) bool { return c.Operator != "" && c.Operator == userID }

// callNext memilih tiket WAITING eligible dengan SKIP LOCKED; counter harus sudah di-lock oleh pemanggil.
func (s *Service) callNext(ctx context.Context, tx pgx.Tx, tenantID string, c *counterRow, actor string) (*CallResult, error) {
	next, err := domain.NextCounterState(domain.CounterState(c.State), domain.CACall)
	if err != nil {
		return nil, err
	}
	var tID, display, svcCode string
	err = tx.QueryRow(ctx, `SELECT t.id::text, t.display_number, sv.code
		FROM queue_tickets t
		JOIN counter_services cs ON cs.tenant_id=t.tenant_id AND cs.tenant_service_id=t.tenant_service_id AND cs.counter_id=$1
		JOIN tenant_services ts ON ts.tenant_id=t.tenant_id AND ts.id=t.tenant_service_id JOIN services sv ON sv.id=ts.service_id
		WHERE t.site_id=$2 AND t.status='WAITING' AND t.service_date=`+serviceDateSQL+`
		ORDER BY t.checked_in_at, t.sequence_no LIMIT 1 FOR UPDATE OF t SKIP LOCKED`, c.ID, c.SiteID).Scan(&tID, &display, &svcCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return &CallResult{CounterID: c.ID, CounterState: c.State, ReadinessRevision: c.Revision}, nil
	} else if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE queue_tickets SET status='CALLED', called_counter_id=$2, called_at=now(),
		first_called_at=COALESCE(first_called_at, now()), revision=revision+1 WHERE id=$1`, tID, c.ID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE counters SET state=$2 WHERE id=$1`, c.ID, string(next)); err != nil {
		return nil, err
	}
	var rev int
	if err := tx.QueryRow(ctx, `SELECT revision FROM queue_tickets WHERE id=$1`, tID).Scan(&rev); err != nil {
		return nil, err
	}
	evID, err := s.queueEvent(ctx, tx, tenantID, c.SiteID, tID, c.ID, "queue.ticket_called",
		map[string]any{"ticket_number": display, "counter_label": c.Label, "service": svcCode, "revision": rev})
	if err != nil {
		return nil, err
	}
	out := &CallResult{CounterID: c.ID, CounterState: string(next), TicketID: tID, DisplayNumber: display, Status: "CALLED",
		CallEventID: evID, AnnouncementStatus: "DISABLED", ReadinessRevision: c.Revision}
	if c.AutoAnnounce {
		if err := s.enqueueAnnouncement(ctx, tx, tenantID, c, tID, evID, "CALL", 1); err != nil {
			return nil, err
		}
		out.AnnouncementStatus = "QUEUED"
	}
	if err := s.outbox(ctx, tx, tenantID, "queue.ticket_called", map[string]any{"ticket_id": tID, "call_event_id": evID}); err != nil {
		return nil, err
	}
	if err := s.audit(ctx, tx, auditEntry{TenantID: tenantID, ActorType: "USER", ActorID: actor, Action: "queue.call", ResourceType: "ticket", ResourceID: tID}); err != nil {
		return nil, err
	}
	return out, nil
}

// enqueueAnnouncement: job lahir dalam transaksi yang sama dengan CALLED. Tidak menyimpan nama/kalimat suara.
func (s *Service) enqueueAnnouncement(ctx context.Context, tx pgx.Tx, tenantID string, c *counterRow, ticketID string, evID int64, kind string, revision int) error {
	_, err := tx.Exec(ctx, `INSERT INTO queue_announcements (tenant_id, site_id, audio_zone_id, ticket_id, counter_id, call_event_id, kind, revision, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8, now() + make_interval(secs => $9))`,
		tenantID, c.SiteID, c.ZoneID, ticketID, c.ID, evID, kind, revision, c.TTLSec)
	return err
}

// CounterReady: tandai READY; bila AUTO_ADVANCE_ON_READY aktif dan loket bertugas, Go yang memilih tiket (09 §3).
func (s *Service) CounterReady(ctx context.Context, p *StaffPrincipal, counterID string, clientRevision *int) (*CallResult, error) {
	var out *CallResult
	err := s.staffTx(ctx, p, domain.PermQueueCounterManage, locateCounter(counterID), func(tx pgx.Tx, res domain.Resource) error {
		c, err := s.lockCounter(ctx, tx, counterID)
		if err != nil {
			return err
		}
		if !c.assignedTo(p.UserID) {
			return domain.ErrForbidden // hanya operator yang ditugaskan yang membuat loket READY
		}
		if clientRevision != nil && *clientRevision != c.Revision {
			return domain.ErrStateConflict("readiness revision is stale; refresh counter state")
		}
		next, err := domain.NextCounterState(domain.CounterState(c.State), domain.CAMarkReady)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE counters SET state=$2, readiness_revision=readiness_revision+1 WHERE id=$1`, counterID, string(next)); err != nil {
			return err
		}
		c.State, c.Revision = string(next), c.Revision+1
		if err := s.audit(ctx, tx, auditEntry{TenantID: res.TenantID, ActorID: p.UserID, Action: "counter.ready", ResourceType: "counter", ResourceID: counterID}); err != nil {
			return err
		}
		if domain.AutoAdvanceEligible(next, c.AutoAdvance, c.assignedTo(p.UserID), false) {
			out, err = s.callNext(ctx, tx, res.TenantID, c, p.UserID)
			return err
		}
		out = &CallResult{CounterID: counterID, CounterState: c.State, ReadinessRevision: c.Revision}
		return nil
	})
	return out, err
}

func (s *Service) counterTransition(ctx context.Context, p *StaffPrincipal, counterID string, act domain.CounterAction, auditAction string) (*CallResult, error) {
	var out *CallResult
	err := s.staffTx(ctx, p, domain.PermQueueCounterManage, locateCounter(counterID), func(tx pgx.Tx, res domain.Resource) error {
		c, err := s.lockCounter(ctx, tx, counterID)
		if err != nil {
			return err
		}
		next, err := domain.NextCounterState(domain.CounterState(c.State), act)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE counters SET state=$2, readiness_revision=readiness_revision+1 WHERE id=$1`, counterID, string(next)); err != nil {
			return err
		}
		out = &CallResult{CounterID: counterID, CounterState: string(next), ReadinessRevision: c.Revision + 1}
		return s.audit(ctx, tx, auditEntry{TenantID: res.TenantID, ActorID: p.UserID, Action: auditAction, ResourceType: "counter", ResourceID: counterID})
	})
	return out, err
}

// CounterPause menghentikan auto-advance tanpa membatalkan tiket yang sedang berjalan.
func (s *Service) CounterPause(ctx context.Context, p *StaffPrincipal, id string) (*CallResult, error) {
	return s.counterTransition(ctx, p, id, domain.CAPause, "counter.pause")
}
func (s *Service) CounterClose(ctx context.Context, p *StaffPrincipal, id string) (*CallResult, error) {
	return s.counterTransition(ctx, p, id, domain.CAClose, "counter.close")
}

// CallNext manual. Dua operator pada tiket yang sama menghasilkan satu CALLED (lock loket + SKIP LOCKED).
func (s *Service) CallNext(ctx context.Context, p *StaffPrincipal, counterID string) (*CallResult, error) {
	var out *CallResult
	err := s.staffTx(ctx, p, domain.PermQueueTicketCall, locateCounter(counterID), func(tx pgx.Tx, res domain.Resource) error {
		c, err := s.lockCounter(ctx, tx, counterID)
		if err != nil {
			return err
		}
		if !c.assignedTo(p.UserID) {
			return domain.ErrForbidden
		}
		out, err = s.callNext(ctx, tx, res.TenantID, c, p.UserID)
		return err
	})
	return out, err
}

type ticketRow struct {
	ID, Display, Status, CounterID, SiteID string
	Revision, RecallCount                  int
}

func lockTicket(ctx context.Context, tx pgx.Tx, id string) (*ticketRow, error) {
	t := &ticketRow{ID: id}
	err := tx.QueryRow(ctx, `SELECT display_number, status, COALESCE(called_counter_id::text,''), site_id::text, revision, recall_count
		FROM queue_tickets WHERE id=$1 FOR UPDATE`, id).Scan(&t.Display, &t.Status, &t.CounterID, &t.SiteID, &t.Revision, &t.RecallCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return t, err
}

// ticketOp: lock loket lalu tiket (urutan konsisten dengan callNext), validasi transisi, jalankan work.
func (s *Service) ticketOp(ctx context.Context, p *StaffPrincipal, ticketID string,
	work func(tx pgx.Tx, res domain.Resource, c *counterRow, t *ticketRow) (*CallResult, error)) (*CallResult, error) {
	var out *CallResult
	err := s.staffTx(ctx, p, domain.PermQueueTicketManage, locateTicket(ticketID), func(tx pgx.Tx, res domain.Resource) error {
		if res.CounterID == "" {
			return domain.ErrStateConflict("ticket is not assigned to a counter")
		}
		c, err := s.lockCounter(ctx, tx, res.CounterID)
		if err != nil {
			return err
		}
		t, err := lockTicket(ctx, tx, ticketID)
		if err != nil {
			return err
		}
		if t.CounterID != c.ID {
			return domain.ErrStateConflict("ticket moved to another counter")
		}
		out, err = work(tx, res, c, t)
		return err
	})
	return out, err
}

func (s *Service) cancelOpenAnnouncements(ctx context.Context, tx pgx.Tx, ticketID string) error {
	_, err := tx.Exec(ctx, `UPDATE queue_announcements SET state='CANCELLED', finished_at=now()
		WHERE ticket_id=$1 AND state IN ('PENDING','CLAIMED')`, ticketID)
	return err
}

func (s *Service) Recall(ctx context.Context, p *StaffPrincipal, ticketID string) (*CallResult, error) {
	return s.ticketOp(ctx, p, ticketID, func(tx pgx.Tx, res domain.Resource, c *counterRow, t *ticketRow) (*CallResult, error) {
		if _, err := domain.NextTicketStatus(domain.TicketStatus(t.Status), domain.ActRecall); err != nil {
			return nil, err
		}
		if t.RecallCount >= c.MaxRecall {
			return nil, domain.ErrBusinessRule("maximum recall count reached for this ticket")
		}
		var recent bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM queue_announcements WHERE ticket_id=$1
			AND created_at > now() - make_interval(secs => $2))`, ticketID, c.CooldownSec).Scan(&recent); err != nil {
			return nil, err
		}
		if recent {
			return nil, domain.ErrRateLimited
		}
		if _, err := tx.Exec(ctx, `UPDATE queue_tickets SET recall_count=recall_count+1, revision=revision+1, called_at=now() WHERE id=$1`, ticketID); err != nil {
			return nil, err
		}
		evID, err := s.queueEvent(ctx, tx, res.TenantID, t.SiteID, ticketID, c.ID, "queue.ticket_recalled",
			map[string]any{"ticket_number": t.Display, "counter_label": c.Label, "revision": t.Revision + 1})
		if err != nil {
			return nil, err
		}
		out := &CallResult{CounterID: c.ID, CounterState: c.State, TicketID: ticketID, DisplayNumber: t.Display, Status: "CALLED",
			CallEventID: evID, AnnouncementStatus: "DISABLED", ReadinessRevision: c.Revision}
		if c.AutoAnnounce {
			if err := s.enqueueAnnouncement(ctx, tx, res.TenantID, c, ticketID, evID, "RECALL", t.RecallCount+2); err != nil {
				return nil, err
			}
			out.AnnouncementStatus = "QUEUED"
		}
		return out, s.audit(ctx, tx, auditEntry{TenantID: res.TenantID, ActorID: p.UserID, Action: "queue.recall", ResourceType: "ticket", ResourceID: ticketID})
	})
}

func (s *Service) Start(ctx context.Context, p *StaffPrincipal, ticketID string) (*CallResult, error) {
	return s.ticketOp(ctx, p, ticketID, func(tx pgx.Tx, res domain.Resource, c *counterRow, t *ticketRow) (*CallResult, error) {
		to, err := domain.NextTicketStatus(domain.TicketStatus(t.Status), domain.ActStart)
		if err != nil {
			return nil, err
		}
		cs, err := domain.NextCounterState(domain.CounterState(c.State), domain.CAStart)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE queue_tickets SET status=$2, started_at=now(), revision=revision+1 WHERE id=$1`, ticketID, string(to)); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE counters SET state=$2 WHERE id=$1`, c.ID, string(cs)); err != nil {
			return nil, err
		}
		if err := s.cancelOpenAnnouncements(ctx, tx, ticketID); err != nil {
			return nil, err
		}
		if _, err := s.queueEvent(ctx, tx, res.TenantID, t.SiteID, ticketID, c.ID, "queue.ticket_serving",
			map[string]any{"ticket_number": t.Display, "counter_label": c.Label, "revision": t.Revision + 1}); err != nil {
			return nil, err
		}
		return &CallResult{CounterID: c.ID, CounterState: string(cs), TicketID: ticketID, DisplayNumber: t.Display, Status: string(to), ReadinessRevision: c.Revision},
			s.audit(ctx, tx, auditEntry{TenantID: res.TenantID, ActorID: p.UserID, Action: "queue.start", ResourceType: "ticket", ResourceID: ticketID})
	})
}

// Complete mengembalikan loket ke READY dan, bila kebijakan aktif, langsung auto-advance (guard dievaluasi ulang).
func (s *Service) Complete(ctx context.Context, p *StaffPrincipal, ticketID string) (*CallResult, error) {
	return s.ticketOp(ctx, p, ticketID, func(tx pgx.Tx, res domain.Resource, c *counterRow, t *ticketRow) (*CallResult, error) {
		to, err := domain.NextTicketStatus(domain.TicketStatus(t.Status), domain.ActComplete)
		if err != nil {
			return nil, err
		}
		cs, err := domain.NextCounterState(domain.CounterState(c.State), domain.CAComplete)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE queue_tickets SET status=$2, completed_at=now(), revision=revision+1 WHERE id=$1`, ticketID, string(to)); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE counters SET state=$2 WHERE id=$1`, c.ID, string(cs)); err != nil {
			return nil, err
		}
		if _, err := s.queueEvent(ctx, tx, res.TenantID, t.SiteID, ticketID, c.ID, "queue.ticket_completed",
			map[string]any{"ticket_number": t.Display, "revision": t.Revision + 1}); err != nil {
			return nil, err
		}
		if err := s.audit(ctx, tx, auditEntry{TenantID: res.TenantID, ActorID: p.UserID, Action: "queue.complete", ResourceType: "ticket", ResourceID: ticketID}); err != nil {
			return nil, err
		}
		c.State = string(cs)
		if domain.AutoAdvanceEligible(cs, c.AutoAdvance, c.Operator != "", false) {
			return s.callNext(ctx, tx, res.TenantID, c, p.UserID)
		}
		return &CallResult{CounterID: c.ID, CounterState: string(cs), TicketID: ticketID, DisplayNumber: t.Display, Status: string(to), ReadinessRevision: c.Revision}, nil
	})
}

// NoShow dilakukan manusia sesuai SOP; kegagalan audio tidak pernah otomatis menjadi no-show.
func (s *Service) NoShow(ctx context.Context, p *StaffPrincipal, ticketID string) (*CallResult, error) {
	return s.ticketOp(ctx, p, ticketID, func(tx pgx.Tx, res domain.Resource, c *counterRow, t *ticketRow) (*CallResult, error) {
		to, err := domain.NextTicketStatus(domain.TicketStatus(t.Status), domain.ActNoShow)
		if err != nil {
			return nil, err
		}
		cs, err := domain.NextCounterState(domain.CounterState(c.State), domain.CARelease)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE queue_tickets SET status=$2, completed_at=now(), revision=revision+1 WHERE id=$1`, ticketID, string(to)); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE counters SET state=$2 WHERE id=$1`, c.ID, string(cs)); err != nil {
			return nil, err
		}
		if err := s.cancelOpenAnnouncements(ctx, tx, ticketID); err != nil {
			return nil, err
		}
		if _, err := s.queueEvent(ctx, tx, res.TenantID, t.SiteID, ticketID, c.ID, "queue.ticket_no_show",
			map[string]any{"ticket_number": t.Display, "revision": t.Revision + 1}); err != nil {
			return nil, err
		}
		return &CallResult{CounterID: c.ID, CounterState: string(cs), TicketID: ticketID, DisplayNumber: t.Display, Status: string(to), ReadinessRevision: c.Revision},
			s.audit(ctx, tx, auditEntry{TenantID: res.TenantID, ActorID: p.UserID, Action: "queue.no_show", ResourceType: "ticket", ResourceID: ticketID})
	})
}

type QueueEntry struct {
	TicketID      string    `json:"ticket_id"`
	DisplayNumber string    `json:"display_number"`
	Service       string    `json:"service"`
	Status        string    `json:"status"`
	CheckedInAt   time.Time `json:"checked_in_at"`
}
type CounterQueue struct {
	CounterID string       `json:"counter_id"`
	Label     string       `json:"label"`
	State     string       `json:"state"`
	Revision  int          `json:"readiness_revision"`
	Current   *QueueEntry  `json:"current"`
	Waiting   []QueueEntry `json:"waiting"`
}

// GetCounterQueue: daftar antrean loket yang ditugaskan; tidak memuat nama/kontak warga.
func (s *Service) GetCounterQueue(ctx context.Context, p *StaffPrincipal, counterID string) (*CounterQueue, error) {
	var out CounterQueue
	err := s.staffTx(ctx, p, domain.PermQueueDisplayRead, locateCounter(counterID), func(tx pgx.Tx, res domain.Resource) error {
		out = CounterQueue{CounterID: counterID, Waiting: []QueueEntry{}}
		var siteID string
		if err := tx.QueryRow(ctx, `SELECT label, state, readiness_revision, site_id::text FROM counters WHERE id=$1`, counterID).
			Scan(&out.Label, &out.State, &out.Revision, &siteID); err != nil {
			return err
		}
		var cur QueueEntry
		err := tx.QueryRow(ctx, `SELECT t.id::text, t.display_number, sv.code, t.status, t.checked_in_at FROM queue_tickets t
			JOIN tenant_services ts ON ts.tenant_id=t.tenant_id AND ts.id=t.tenant_service_id JOIN services sv ON sv.id=ts.service_id
			WHERE t.called_counter_id=$1 AND t.status IN ('CALLED','SERVING')`, counterID).
			Scan(&cur.TicketID, &cur.DisplayNumber, &cur.Service, &cur.Status, &cur.CheckedInAt)
		if err == nil {
			out.Current = &cur
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT t.id::text, t.display_number, sv.code, t.status, t.checked_in_at FROM queue_tickets t
			JOIN counter_services cs ON cs.tenant_id=t.tenant_id AND cs.tenant_service_id=t.tenant_service_id AND cs.counter_id=$1
			JOIN tenant_services ts ON ts.tenant_id=t.tenant_id AND ts.id=t.tenant_service_id JOIN services sv ON sv.id=ts.service_id
			WHERE t.site_id=$2 AND t.status='WAITING' AND t.service_date=`+serviceDateSQL+`
			ORDER BY t.checked_in_at, t.sequence_no LIMIT 50`, counterID, siteID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e QueueEntry
			if err := rows.Scan(&e.TicketID, &e.DisplayNumber, &e.Service, &e.Status, &e.CheckedInAt); err != nil {
				return err
			}
			out.Waiting = append(out.Waiting, e)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
