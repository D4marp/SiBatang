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
	AppEnv                                                      string
	HTTPAddr                                                    string
	DatabaseURL                                                 string
	RootOrgCode                                                 string
	DefaultPublicTenant                                         string
	PilotPublishAllowlist                                       []string
	DataEncryptionKey                                           []byte
	SSEHeartbeat                                                time.Duration
	AnnouncementLease                                           time.Duration
	AnnouncementExpire                                          time.Duration
	SessionCookieName                                           string
	CookieSecure                                                bool
	PublicRateLimitPerMin                                       int
	MetricsRefreshInterval                                      time.Duration
	OIDCIssuer, OIDCClientID, OIDCClientSecret, OIDCRedirectURL string
	OIDCRequireMFA                                              bool
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
	c.OIDCIssuer, c.OIDCClientID = os.Getenv("OIDC_ISSUER_URL"), os.Getenv("OIDC_CLIENT_ID")
	c.OIDCClientSecret, c.OIDCRedirectURL = os.Getenv("OIDC_CLIENT_SECRET"), os.Getenv("OIDC_REDIRECT_URL")
	c.OIDCRequireMFA = os.Getenv("OIDC_REQUIRE_MFA") != "false" // MFA wajib untuk peran privilese; mati hanya eksplisit (staging IdP tanpa MFA)
	c.PilotPublishAllowlist = strings.Split(getenv("PILOT_PUBLISH_ALLOWLIST", c.DefaultPublicTenant), ",")

	// Platform hanya untuk satu root: konfigurasi lain ditolak (AGENTS.md One-Polda-only rule).
	if c.RootOrgCode != "POLDA_JAWA_TENGAH" {
		return c, fmt.Errorf("ROOT_ORG_CODE must be POLDA_JAWA_TENGAH")
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}
	if c.OIDCIssuer != "" {
		if c.OIDCClientID == "" || c.OIDCRedirectURL == "" {
			return c, fmt.Errorf("OIDC_CLIENT_ID and OIDC_REDIRECT_URL are required when OIDC_ISSUER_URL is set")
		}
		for _, u := range []string{c.OIDCIssuer, c.OIDCRedirectURL} {
			if !strings.HasPrefix(u, "https://") && !(c.AppEnv == "development" && (strings.HasPrefix(u, "http://localhost") || strings.HasPrefix(u, "http://127.0.0.1"))) {
				return c, fmt.Errorf("OIDC URLs must be https (http only for localhost in development)")
			}
		}
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
