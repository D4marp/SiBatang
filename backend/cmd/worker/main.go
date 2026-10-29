// Command worker menjalankan metrik agregat Polda, sweeper announcement, dan outbox drain.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

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
	log.Info("worker started", slog.Duration("interval", cfg.MetricsRefreshInterval))
	// Belum ada penyedia notifikasi (push/WhatsApp) yang disetujui: topik outbox tanpa handler ditandai selesai.
	svc.RunWorker(ctx, map[string]service.OutboxHandler{})
}
