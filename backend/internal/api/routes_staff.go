package api

import (
	"net/http"

	"github.com/D4marp/SiBatang/backend/internal/domain"
	"github.com/D4marp/SiBatang/backend/internal/httpx"
	"github.com/D4marp/SiBatang/backend/internal/service"
)

// call menjalankan operasi antrean yang mengembalikan CallResult; 204 bila tidak ada tiket untuk call-next.
func (s *Server) queueOp(idParam string, op func(r *http.Request, p *service.StaffPrincipal, id string) (*service.CallResult, error), emptyIs204 bool) http.HandlerFunc {
	return s.staff(func(w http.ResponseWriter, r *http.Request, p *service.StaffPrincipal) error {
		id, err := pathUUID(r, idParam)
		if err != nil {
			return err
		}
		res, err := op(r, p, id)
		if err != nil {
			return err
		}
		if emptyIs204 && res.TicketID == "" {
			w.WriteHeader(http.StatusNoContent)
			return nil
		}
		httpx.JSON(w, 200, res, nil)
		return nil
	})
}

func (s *Server) staffRoutes(h func(string, http.HandlerFunc)) {
	h("GET /staff/counters/{id}/queue", s.staff(func(w http.ResponseWriter, r *http.Request, p *service.StaffPrincipal) error {
		id, err := pathUUID(r, "id")
		if err != nil {
			return err
		}
		q, err := s.svc.GetCounterQueue(r.Context(), p, id)
		if err != nil {
			return err
		}
		httpx.JSON(w, 200, q, nil)
		return nil
	}))
	h("POST /staff/counters/{id}/ready", s.staff(func(w http.ResponseWriter, r *http.Request, p *service.StaffPrincipal) error {
		id, err := pathUUID(r, "id")
		if err != nil {
			return err
		}
		var body struct {
			ReadinessRevision *int   `json:"readiness_revision"`
			Mode              string `json:"mode"`
		}
		if r.ContentLength != 0 {
			if err := httpx.Decode(w, r, &body); err != nil {
				return err
			}
		}
		res, err := s.svc.CounterReady(r.Context(), p, id, body.ReadinessRevision)
		if err != nil {
			return err
		}
		httpx.JSON(w, 200, res, nil)
		return nil
	}))
	h("POST /staff/counters/{id}/pause", s.queueOp("id", func(r *http.Request, p *service.StaffPrincipal, id string) (*service.CallResult, error) {
		return s.svc.CounterPause(r.Context(), p, id)
	}, false))
	h("POST /staff/counters/{id}/close", s.queueOp("id", func(r *http.Request, p *service.StaffPrincipal, id string) (*service.CallResult, error) {
		return s.svc.CounterClose(r.Context(), p, id)
	}, false))
	h("POST /staff/counters/{id}/call-next", s.queueOp("id", func(r *http.Request, p *service.StaffPrincipal, id string) (*service.CallResult, error) {
		return s.svc.CallNext(r.Context(), p, id)
	}, true))
	h("POST /staff/tickets/{id}/recall", s.queueOp("id", func(r *http.Request, p *service.StaffPrincipal, id string) (*service.CallResult, error) {
		return s.svc.Recall(r.Context(), p, id)
	}, false))
	h("POST /staff/tickets/{id}/start", s.queueOp("id", func(r *http.Request, p *service.StaffPrincipal, id string) (*service.CallResult, error) {
		return s.svc.Start(r.Context(), p, id)
	}, false))
	h("POST /staff/tickets/{id}/complete", s.queueOp("id", func(r *http.Request, p *service.StaffPrincipal, id string) (*service.CallResult, error) {
		return s.svc.Complete(r.Context(), p, id)
	}, false))
	h("POST /staff/tickets/{id}/no-show", s.queueOp("id", func(r *http.Request, p *service.StaffPrincipal, id string) (*service.CallResult, error) {
		return s.svc.NoShow(r.Context(), p, id)
	}, false))

	// Konfigurasi site/perangkat
	h("POST /staff/sites/{id}/devices", s.staff(func(w http.ResponseWriter, r *http.Request, p *service.StaffPrincipal) error {
		id, err := pathUUID(r, "id")
		if err != nil {
			return err
		}
		var req service.EnrollRequest
		if err := httpx.Decode(w, r, &req); err != nil {
			return err
		}
		res, err := s.svc.EnrollDevice(r.Context(), p, id, req)
		if err != nil {
			return err
		}
		httpx.JSON(w, 201, res, nil)
		return nil
	}))
	h("POST /staff/devices/{id}/revoke", s.staff(func(w http.ResponseWriter, r *http.Request, p *service.StaffPrincipal) error {
		id, err := pathUUID(r, "id")
		if err != nil {
			return err
		}
		if err := s.svc.RevokeDevice(r.Context(), p, id); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	}))
	h("POST /staff/sites/{id}/audio-zones", s.staff(func(w http.ResponseWriter, r *http.Request, p *service.StaffPrincipal) error {
		id, err := pathUUID(r, "id")
		if err != nil {
			return err
		}
		var req service.ZoneRequest
		if err := httpx.Decode(w, r, &req); err != nil {
			return err
		}
		zid, err := s.svc.CreateAudioZone(r.Context(), p, id, req)
		if err != nil {
			return err
		}
		httpx.JSON(w, 201, map[string]string{"zone_id": zid}, nil)
		return nil
	}))
	h("PATCH /staff/sites/{id}/audio-zones/{zoneId}", s.staff(func(w http.ResponseWriter, r *http.Request, p *service.StaffPrincipal) error {
		id, err := pathUUID(r, "id")
		if err != nil {
			return err
		}
		zid, err := pathUUID(r, "zoneId")
		if err != nil {
			return err
		}
		var req struct {
			NamePolicy string `json:"name_policy"`
		}
		if err := httpx.Decode(w, r, &req); err != nil {
			return err
		}
		if err := s.svc.UpdateAudioZonePolicy(r.Context(), p, id, zid, req.NamePolicy); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	}))
	h("POST /staff/sites/{id}/counters", s.staff(func(w http.ResponseWriter, r *http.Request, p *service.StaffPrincipal) error {
		id, err := pathUUID(r, "id")
		if err != nil {
			return err
		}
		var req service.CounterRequest
		if err := httpx.Decode(w, r, &req); err != nil {
			return err
		}
		cid, err := s.svc.CreateCounter(r.Context(), p, id, req)
		if err != nil {
			return err
		}
		httpx.JSON(w, 201, map[string]string{"counter_id": cid}, nil)
		return nil
	}))
	h("PATCH /staff/counters/{id}/queue-policy", s.staff(func(w http.ResponseWriter, r *http.Request, p *service.StaffPrincipal) error {
		id, err := pathUUID(r, "id")
		if err != nil {
			return err
		}
		var req service.QueuePolicyRequest
		if err := httpx.Decode(w, r, &req); err != nil {
			return err
		}
		if err := s.svc.UpdateCounterPolicy(r.Context(), p, id, req); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	}))
	h("PUT /staff/counters/{id}/operator", s.staff(func(w http.ResponseWriter, r *http.Request, p *service.StaffPrincipal) error {
		id, err := pathUUID(r, "id")
		if err != nil {
			return err
		}
		var req struct {
			UserID string `json:"user_id"`
		}
		if err := httpx.Decode(w, r, &req); err != nil {
			return err
		}
		if err := s.svc.AssignOperator(r.Context(), p, id, req.UserID); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	}))
	h("POST /staff/tenant-services/{id}/slots", s.staff(func(w http.ResponseWriter, r *http.Request, p *service.StaffPrincipal) error {
		id, err := pathUUID(r, "id")
		if err != nil {
			return err
		}
		var req struct {
			Slots []service.SlotInput `json:"slots"`
		}
		if err := httpx.Decode(w, r, &req); err != nil {
			return err
		}
		n, err := s.svc.CreateSlots(r.Context(), p, id, req.Slots)
		if err != nil {
			return err
		}
		httpx.JSON(w, 201, map[string]int{"created": n}, nil)
		return nil
	}))
	h("GET /staff/tenants/{id}/readiness", s.staff(func(w http.ResponseWriter, r *http.Request, p *service.StaffPrincipal) error {
		id, err := pathUUID(r, "id")
		if err != nil {
			return err
		}
		rd, err := s.svc.TenantReadiness(r.Context(), p, id)
		if err != nil {
			return err
		}
		httpx.JSON(w, 200, rd, nil)
		return nil
	}))
	h("POST /staff/tenants/{id}/publish", s.staff(func(w http.ResponseWriter, r *http.Request, p *service.StaffPrincipal) error {
		id, err := pathUUID(r, "id")
		if err != nil {
			return err
		}
		if err := s.svc.PublishTenant(r.Context(), p, id); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	}))
}

func (s *Server) poldaRoutes(h func(string, http.HandlerFunc)) {
	h("GET /polda/jateng/metrics", s.staff(func(w http.ResponseWriter, r *http.Request, p *service.StaffPrincipal) error {
		m, err := s.svc.PoldaMetrics(r.Context(), p)
		if err != nil {
			return err
		}
		httpx.JSON(w, 200, m, map[string]any{"report_scope": "JATENG_DESCENDANTS", "as_of": m.AsOf})
		return nil
	}))
	h("GET /polda/jateng/polres", s.staff(func(w http.ResponseWriter, r *http.Request, p *service.StaffPrincipal) error {
		l, err := s.svc.ListPolres(r.Context(), p)
		if err != nil {
			return err
		}
		httpx.JSON(w, 200, l, nil)
		return nil
	}))
	h("GET /polda/jateng/onboarding-requests", s.staff(func(w http.ResponseWriter, r *http.Request, p *service.StaffPrincipal) error {
		l, err := s.svc.ListOnboardingRequests(r.Context(), p)
		if err != nil {
			return err
		}
		httpx.JSON(w, 200, l, nil)
		return nil
	}))
	h("POST /polda/jateng/onboarding-requests", s.staff(func(w http.ResponseWriter, r *http.Request, p *service.StaffPrincipal) error {
		var in service.OnboardingInput
		if err := httpx.Decode(w, r, &in); err != nil { // field parent_id sengaja tidak ada: ditolak oleh DisallowUnknownFields
			return err
		}
		req, err := s.svc.CreateOnboardingRequest(r.Context(), p, in)
		if err != nil {
			return err
		}
		httpx.JSON(w, 201, req, nil)
		return nil
	}))
	decide := func(approve bool) http.HandlerFunc {
		return s.staff(func(w http.ResponseWriter, r *http.Request, p *service.StaffPrincipal) error {
			id, err := pathUUID(r, "id")
			if err != nil {
				return err
			}
			var body struct {
				Note string `json:"note"`
			}
			if r.ContentLength != 0 {
				if err := httpx.Decode(w, r, &body); err != nil {
					return err
				}
			}
			if err := s.svc.DecideOnboarding(r.Context(), p, id, approve, body.Note); err != nil {
				return err
			}
			w.WriteHeader(http.StatusNoContent)
			return nil
		})
	}
	h("POST /polda/jateng/onboarding-requests/{id}/approve", decide(true))
	h("POST /polda/jateng/onboarding-requests/{id}/reject", decide(false))
}

var _ = domain.IsUUID
