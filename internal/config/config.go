package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	ListenAddr        string
	AdminUsername     string
	AdminPassword     string
	JWTSecret         string
	EAConnectCode     string
	EATokenTTL        time.Duration
	AIMinConfidence   float64
	MaxDailyLossPct   float64
	MaxOpenPositions  int
	MaxSpreadPips     float64
	DefaultRiskPct    float64
	TelegramBotToken  string
	TelegramChatID    string
	OpenAIClientID    string
	OpenAIRedirectURI string
	OpenClawWebhookURL string
	OpenClawTimeout   time.Duration
}

func Load() Config {
	return Config{
		ListenAddr:         getEnv("LISTEN_ADDR", ":8080"),
		AdminUsername:      getEnv("ADMIN_USERNAME", "admin"),
		AdminPassword:      getEnv("ADMIN_PASSWORD", "change-me"),
		JWTSecret:          getEnv("JWT_SECRET", "change-this-secret"),
		EAConnectCode:      getEnv("EA_CONNECT_CODE", "MMBOT-ONE-TIME-CODE"),
		EATokenTTL:         getDuration("EA_TOKEN_TTL", 24*time.Hour),
		AIMinConfidence:    getFloat("AI_MIN_CONFIDENCE", 0.70),
		MaxDailyLossPct:    getFloat("MAX_DAILY_LOSS_PCT", 2.0),
		MaxOpenPositions:   getInt("MAX_OPEN_POSITIONS", 3),
		MaxSpreadPips:      getFloat("MAX_SPREAD_PIPS", 2.0),
		DefaultRiskPct:     getFloat("DEFAULT_RISK_PCT", 1.0),
		TelegramBotToken:   getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramChatID:     getEnv("TELEGRAM_CHAT_ID", ""),
		OpenAIClientID:     getEnv("OPENAI_CLIENT_ID", ""),
		OpenAIRedirectURI:  getEnv("OPENAI_REDIRECT_URI", "http://localhost:8080/oauth/openai/callback"),
		OpenClawWebhookURL: getEnv("OPENCLAW_WEBHOOK_URL", ""),
		OpenClawTimeout:    getDuration("OPENCLAW_TIMEOUT", 5*time.Second),
	}
}

func getEnv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func getInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return n
}

func getDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

