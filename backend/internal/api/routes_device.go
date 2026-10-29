package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/D4marp/SiBatang/backend/internal/domain"
	"github.com/D4marp/SiBatang/backend/internal/httpx"
	"github.com/D4marp/SiBatang/backend/internal/service"
)

func (s *Server) deviceRoutes(h func(string, http.HandlerFunc)) {
	h("POST /devices/activate", s.public(func(w http.ResponseWriter, r *http.Request) error {
		var req struct {
			ActivationCode string `json:"activation_code"`
		}
		if err := httpx.Decode(w, r, &req); err != nil {
			return err
		}
		res, err := s.svc.ActivateDevice(r.Context(), req.ActivationCode)
		if err != nil {
			return err
		}
		httpx.JSON(w, 200, res, nil)
		return nil
	}))
	h("POST /devices/{deviceId}/heartbeat", s.device(func(w http.ResponseWriter, r *http.Request, dp *service.DevicePrincipal) error {
		var req struct {
			AppVersion string         `json:"app_version"`
			Health     map[string]any `json:"health"`
		}
		if err := httpx.Decode(w, r, &req); err != nil {
			return err
		}
		if err := s.svc.Heartbeat(r.Context(), dp, r.PathValue("deviceId"), req.AppVersion, req.Health); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	}))
	h("POST /devices/{deviceId}/checkins", s.device(func(w http.ResponseWriter, r *http.Request, dp *service.DevicePrincipal) error {
		var req service.CheckinRequest
		if err := httpx.Decode(w, r, &req); err != nil {
			return err
		}
		res, err := s.svc.CheckIn(r.Context(), dp, r.PathValue("deviceId"), req)
		if err != nil {
			return err
		}
		status := 201
		if res.IsReprint {
			status = 200
		}
		httpx.JSON(w, status, res, nil)
		return nil
	}))
	h("POST /devices/{deviceId}/tickets/{ticketId}/print-attempts", s.device(func(w http.ResponseWriter, r *http.Request, dp *service.DevicePrincipal) error {
		tid, err := pathUUID(r, "ticketId")
		if err != nil {
			return err
		}
		var req struct {
			Outcome string `json:"outcome"`
		}
		if err := httpx.Decode(w, r, &req); err != nil {
			return err
		}
		res, err := s.svc.RecordPrintAttempt(r.Context(), dp, r.PathValue("deviceId"), tid, req.Outcome)
		if err != nil {
			return err
		}
		httpx.JSON(w, 200, res, nil)
		return nil
	}))

	h("GET /devices/{deviceId}/display/snapshot", s.device(func(w http.ResponseWriter, r *http.Request, dp *service.DevicePrincipal) error {
		snap, err := s.svc.DisplaySnapshot(r.Context(), dp, r.PathValue("deviceId"))
		if err != nil {
			return err
		}
		httpx.JSON(w, 200, snap, nil)
		return nil
	}))
	h("GET /devices/{deviceId}/display/events", s.device(func(w http.ResponseWriter, r *http.Request, dp *service.DevicePrincipal) error {
		id := r.PathValue("deviceId")
		return s.serveSSE(w, r,
			func(ctx context.Context, after int64) ([]sseEvent, error) {
				evs, err := s.svc.DisplayEventsSince(ctx, dp, id, after)
				if err != nil {
					return nil, err
				}
				out := make([]sseEvent, 0, len(evs))
				for _, e := range evs {
					out = append(out, sseEvent{ID: e.ID, Name: e.Type, Data: string(e.Payload)})
				}
				return out, nil
			},
			func() error { _, err := s.deviceAuth(r); return err }) // perangkat dicabut => stream berhenti
	}))

	h("GET /devices/{deviceId}/announcer/events", s.device(func(w http.ResponseWriter, r *http.Request, dp *service.DevicePrincipal) error {
		id := r.PathValue("deviceId")
		return s.serveSSE(w, r,
			func(ctx context.Context, after int64) ([]sseEvent, error) {
				jobs, err := s.svc.PendingAnnouncements(ctx, dp, id)
				if err != nil {
					return nil, err
				}
				var out []sseEvent
				for _, j := range jobs {
					if j.EventID > after { // hanya id job + zona; tanpa nama/nomor
						b, _ := json.Marshal(map[string]string{"zone_id": j.ZoneID, "job_id": j.JobID})
						out = append(out, sseEvent{ID: j.EventID, Name: "announcement.job_available", Data: string(b)})
					}
				}
				return out, nil
			},
			func() error { _, err := s.deviceAuth(r); return err })
	}))
	h("POST /devices/{deviceId}/announcer/jobs/claim", s.device(func(w http.ResponseWriter, r *http.Request, dp *service.DevicePrincipal) error {
		c, err := s.svc.ClaimAnnouncement(r.Context(), dp, r.PathValue("deviceId"))
		if err != nil {
			return err
		}
		if c == nil {
			w.WriteHeader(http.StatusNoContent)
			return nil
		}
		w.Header().Set("Cache-Control", "no-store, private")
		httpx.JSON(w, 200, c, nil)
		return nil
	}))
	for _, action := range []string{"started", "completed", "failed"} {
		action := action
		h("POST /devices/{deviceId}/announcer/jobs/{jobId}/"+action, s.device(func(w http.ResponseWriter, r *http.Request, dp *service.DevicePrincipal) error {
			jid, err := pathUUID(r, "jobId")
			if err != nil {
				return domain.ErrNotFound
			}
			var req struct {
				FencingToken int64  `json:"fencing_token"`
				ErrorCode    string `json:"error_code"`
			}
			if err := httpx.Decode(w, r, &req); err != nil {
				return err
			}
			if err := s.svc.AckAnnouncement(r.Context(), dp, r.PathValue("deviceId"), jid, action, req.FencingToken, req.ErrorCode); err != nil {
				return err
			}
			w.WriteHeader(http.StatusNoContent)
			return nil
		}))
	}
}
