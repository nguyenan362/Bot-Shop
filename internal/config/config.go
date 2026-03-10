package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

// Config holds all application configuration.
type Config struct {
	BotToken      string
	WebhookURL    string
	WebhookSecret string

	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	AdminTeleIDs    []int64
	AdminJWTSecret  string
	AdminSessionHrs int

	Port string
	Env  string
}

// Load reads configuration from .env and environment variables.
func Load() *Config {
	_ = godotenv.Load()

	cfg := &Config{
		BotToken:      getEnv("BOT_TOKEN", ""),
		WebhookURL:    getEnv("WEBHOOK_URL", ""),
		WebhookSecret: getEnv("WEBHOOK_SECRET", ""),

		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnvInt("DB_PORT", 5432),
		DBUser:     getEnv("DB_USER", "botshop"),
		DBPassword: getEnv("DB_PASSWORD", "botshop_pass"),
		DBName:     getEnv("DB_NAME", "botshop"),

		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvInt("REDIS_DB", 0),

		AdminJWTSecret:  getEnv("ADMIN_JWT_SECRET", "change-me"),
		AdminSessionHrs: getEnvInt("ADMIN_SESSION_HOURS", 2),

		Port: getEnv("PORT", "8080"),
		Env:  getEnv("ENV", "development"),
	}

	// Parse admin Telegram IDs
	adminIDs := getEnv("ADMIN_TELE_IDS", "")
	if adminIDs != "" {
		for _, s := range strings.Split(adminIDs, ",") {
			s = strings.TrimSpace(s)
			id, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				log.Warn().Str("value", s).Msg("invalid admin tele_id, skipping")
				continue
			}
			cfg.AdminTeleIDs = append(cfg.AdminTeleIDs, id)
		}
	}

	return cfg
}

// IsAdmin checks if a Telegram ID is an admin.
func (c *Config) IsAdmin(teleID int64) bool {
	for _, id := range c.AdminTeleIDs {
		if id == teleID {
			return true
		}
	}
	return false
}

// DSN returns PostgreSQL connection string.
func (c *Config) DSN() string {
	return "postgres://" + c.DBUser + ":" + c.DBPassword +
		"@" + c.DBHost + ":" + strconv.Itoa(c.DBPort) +
		"/" + c.DBName + "?sslmode=disable"
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return i
}
