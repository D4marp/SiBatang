// Command api menjalankan REST + SSE SIBATANG (Polda Jawa Tengah / pilot Polres Batang).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/D4marp/SiBatang/backend/internal/api"
	"github.com/D4marp/SiBatang/backend/internal/config"
	"github.com/D4marp/SiBatang/backend/internal/db"
	"github.com/D4marp/SiBatang/backend/internal/service"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		log.Error("config", slog.String("error", err.Error()))
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	d, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer d.Close()
	svc, err := service.New(cfg, d, log)
	if err != nil {
		log.Error("service", slog.String("error", err.Error()))
		os.Exit(1)
	}
	srv := &http.Server{
		Addr: cfg.HTTPAddr, Handler: api.New(svc, d, cfg, log).Handler(),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 90 * time.Second,
		// WriteTimeout sengaja tidak diset karena SSE; heartbeat + deadline klien menjaga koneksi.
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	log.Info("api listening", slog.String("addr", cfg.HTTPAddr), slog.String("env", cfg.AppEnv))
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("serve", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
