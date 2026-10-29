// Package config memuat konfigurasi dari environment (13_DEPLOYMENT_RUNBOOK.md §3). Tidak ada secret default.
package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv                 string
	HTTPAddr               string
	DatabaseURL            string
	RootOrgCode            string
	DefaultPublicTenant    string
	PilotPublishAllowlist  []string
	DataEncryptionKey      []byte
	SSEHeartbeat           time.Duration
	AnnouncementLease      time.Duration
	AnnouncementExpire     time.Duration
	SessionCookieName      string
	CookieSecure           bool
	PublicRateLimitPerMin  int
	MetricsRefreshInterval time.Duration
}

func Load() (Config, error) {
	c := Config{
		AppEnv:                 getenv("APP_ENV", "development"),
		HTTPAddr:               getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:            os.Getenv("DATABASE_URL"),
		RootOrgCode:            getenv("ROOT_ORG_CODE", "POLDA_JAWA_TENGAH"),
		DefaultPublicTenant:    getenv("DEFAULT_PUBLIC_TENANT_CODE", "SIBATANG_BATANG"),
		SessionCookieName:      getenv("SESSION_COOKIE_NAME", "sib_session"),
		SSEHeartbeat:           time.Duration(atoi("SSE_HEARTBEAT_SECONDS", 20)) * time.Second,
		AnnouncementLease:      time.Duration(atoi("ANNOUNCEMENT_LEASE_SECONDS", 30)) * time.Second,
		AnnouncementExpire:     time.Duration(atoi("ANNOUNCEMENT_EXPIRE_SECONDS", 90)) * time.Second,
		PublicRateLimitPerMin:  atoi("PUBLIC_RATE_LIMIT_PER_MIN", 120),
		MetricsRefreshInterval: time.Duration(atoi("METRICS_REFRESH_SECONDS", 30)) * time.Second,
	}
	c.CookieSecure = c.AppEnv != "development"
	c.PilotPublishAllowlist = strings.Split(getenv("PILOT_PUBLISH_ALLOWLIST", c.DefaultPublicTenant), ",")

	// Platform hanya untuk satu root: konfigurasi lain ditolak (AGENTS.md One-Polda-only rule).
	if c.RootOrgCode != "POLDA_JAWA_TENGAH" {
		return c, fmt.Errorf("ROOT_ORG_CODE must be POLDA_JAWA_TENGAH")
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}
	key, err := base64.StdEncoding.DecodeString(os.Getenv("DATA_ENCRYPTION_KEY"))
	if err != nil || len(key) != 32 {
		return c, fmt.Errorf("DATA_ENCRYPTION_KEY must be base64 of 32 random bytes")
	}
	c.DataEncryptionKey = key
	return c, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func atoi(k string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil && v > 0 {
		return v
	}
	return def
}
