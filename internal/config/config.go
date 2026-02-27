package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	ListenAddr              string
	StoreMode               string
	DatabaseURL             string
	OAuthEncryptionKey      string
	AdminUsername           string
	AdminPassword           string
	JWTSecret               string
	EAConnectCode           string
	EATokenTTL              time.Duration
	AIMinConfidence         float64
	MaxDailyLossPct         float64
	MaxOpenPositions        int
	MaxSpreadPips           float64
	DefaultRiskPct          float64
	StrategyRateLimitPerMin int
	StrategyMinInterval     time.Duration
	StrategyDedupTTL        time.Duration
	StrategyDailyBudget     int
	StrategyMaxCandles      int
	TelegramBotToken        string
	TelegramChatID          string
	TelegramAllowedChatIDs  string
	TelegramWebhookSecret   string
	OpenAIAPIKey            string
	OpenAIClientID          string
	OpenAIClientSecret      string
	OpenAIAuthURL           string
	OpenAITokenURL          string
	OpenAIScopes            string
	OpenAIRedirectURI       string
	OpenAIRefreshSkew       time.Duration
	OpenClawWebhookURL      string
	OpenClawTimeout         time.Duration
	OpenClawMaxRetries      int
	OpenClawRetryBase       time.Duration
	OpenClawRetryMax        time.Duration
}

func Load() Config {
	return Config{
		ListenAddr:              getEnv("LISTEN_ADDR", ":18080"),
		StoreMode:               getEnv("STORE_MODE", "postgres"),
		DatabaseURL:             getEnv("DATABASE_URL", ""),
		OAuthEncryptionKey:      getEnv("OAUTH_ENCRYPTION_KEY", ""),
		AdminUsername:           getEnv("ADMIN_USERNAME", "admin"),
		AdminPassword:           getEnv("ADMIN_PASSWORD", "change-me"),
		JWTSecret:               getEnv("JWT_SECRET", "change-this-secret"),
		EAConnectCode:           getEnv("EA_CONNECT_CODE", "MMBOT-ONE-TIME-CODE"),
		EATokenTTL:              getDuration("EA_TOKEN_TTL", 24*time.Hour),
		AIMinConfidence:         getFloat("AI_MIN_CONFIDENCE", 0.70),
		MaxDailyLossPct:         getFloat("MAX_DAILY_LOSS_PCT", 2.0),
		MaxOpenPositions:        getInt("MAX_OPEN_POSITIONS", 3),
		MaxSpreadPips:           getFloat("MAX_SPREAD_PIPS", 2.0),
		DefaultRiskPct:          getFloat("DEFAULT_RISK_PCT", 1.0),
		StrategyRateLimitPerMin: getInt("STRATEGY_RATE_LIMIT_PER_MIN", 30),
		StrategyMinInterval:     getDuration("STRATEGY_MIN_INTERVAL", 2*time.Second),
		StrategyDedupTTL:        getDuration("STRATEGY_DEDUP_TTL", 30*time.Second),
		StrategyDailyBudget:     getInt("STRATEGY_DAILY_BUDGET", 500),
		StrategyMaxCandles:      getInt("STRATEGY_MAX_CANDLES", 300),
		TelegramBotToken:        getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramChatID:          getEnv("TELEGRAM_CHAT_ID", ""),
		TelegramAllowedChatIDs:  getEnv("TELEGRAM_ALLOWED_CHAT_IDS", ""),
		TelegramWebhookSecret:   getEnv("TELEGRAM_WEBHOOK_SECRET", ""),
		OpenAIAPIKey:            getEnv("OPENAI_API_KEY", ""),
		OpenAIClientID:          getEnv("OPENAI_CLIENT_ID", ""),
		OpenAIClientSecret:      getEnv("OPENAI_CLIENT_SECRET", ""),
		OpenAIAuthURL:           getEnv("OPENAI_AUTH_URL", "https://auth.openai.com/oauth/authorize"),
		OpenAITokenURL:          getEnv("OPENAI_TOKEN_URL", "https://auth.openai.com/oauth/token"),
		OpenAIScopes:            getEnv("OPENAI_SCOPES", "models.read models.inference"),
		OpenAIRedirectURI:       getEnv("OPENAI_REDIRECT_URI", "http://localhost:18080/oauth/openai/callback"),
		OpenAIRefreshSkew:       getDuration("OPENAI_REFRESH_SKEW", 2*time.Minute),
		OpenClawWebhookURL:      getEnv("OPENCLAW_WEBHOOK_URL", ""),
		OpenClawTimeout:         getDuration("OPENCLAW_TIMEOUT", 5*time.Second),
		OpenClawMaxRetries:      getInt("OPENCLAW_MAX_RETRIES", 3),
		OpenClawRetryBase:       getDuration("OPENCLAW_RETRY_BASE", 500*time.Millisecond),
		OpenClawRetryMax:        getDuration("OPENCLAW_RETRY_MAX", 5*time.Second),
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
