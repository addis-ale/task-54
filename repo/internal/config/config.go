package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr                   string
	DBPath                 string
	MediaRoot              string
	StructuredLogPath      string
	DiagnosticsRoot        string
	ReportsSharedRoot      string
	SessionCookieName      string
	SessionTTL             time.Duration
	CookieSecure           bool
	BcryptCost             int
	MasterKeyBase64        string
	BootstrapAdminUsername string
	BootstrapAdminPassword string
}

func Load() Config {
	return Config{
		Addr:                   getEnv("APP_ADDR", ":8080"),
		DBPath:                 getEnv("APP_DB_PATH", "./data/clinic.db"),
		MediaRoot:              getEnv("APP_MEDIA_ROOT", "./data/media"),
		StructuredLogPath:      getEnv("APP_STRUCTURED_LOG_PATH", "./data/logs/structured.log"),
		DiagnosticsRoot:        getEnv("APP_DIAGNOSTICS_ROOT", "./data/diagnostics"),
		ReportsSharedRoot:      getEnv("APP_REPORTS_SHARED_ROOT", "./data/shared_reports"),
		SessionCookieName:      getEnv("SESSION_COOKIE_NAME", "clinic_session"),
		SessionTTL:             getEnvDuration("SESSION_TTL", 15*time.Minute),
		CookieSecure:           getEnvBool("SESSION_COOKIE_SECURE", true),
		BcryptCost:             getEnvInt("BCRYPT_COST", 12),
		MasterKeyBase64:        strings.TrimSpace(os.Getenv("APP_MASTER_KEY_B64")),
		BootstrapAdminUsername: strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_USERNAME")),
		BootstrapAdminPassword: os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"),
	}
}

func getEnv(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func getEnvBool(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getEnvInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
