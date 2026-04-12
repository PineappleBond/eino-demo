package config

import (
	"flag"
	"os"

	"github.com/joho/godotenv"
)

// ModelConfig holds one tier of model configuration.
type ModelConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

// Config holds all startup configuration.
type Config struct {
	ServerPort    string
	DatabaseURL   string
	RedisAddr     string
	TavilyAPIKey  string
	Models        map[string]ModelConfig // "haiku", "sonnet", "opus"
}

// Load reads configuration from flags and environment variables.
func Load() *Config {
	port := flag.String("port", "8080", "HTTP server port")
	flag.Parse()

	_ = godotenv.Load()
	// Override port from env if set
	if p := os.Getenv("SERVER_PORT"); p != "" {
		port = &p
	}

	return &Config{
		ServerPort:   *port,
		DatabaseURL:  os.Getenv("DATABASE_URL"),
		RedisAddr:    envOr("REDIS_ADDR", "localhost:6379"),
		TavilyAPIKey: os.Getenv("TAVILY_API_KEY"),
		Models: map[string]ModelConfig{
			"haiku": {
				BaseURL: envOr("MODEL_HAIKU_BASE_URL", "https://api.openai.com/v1"),
				APIKey:  requireEnv("MODEL_HAIKU_API_KEY"),
				Model:   envOr("MODEL_HAIKU_MODEL", "gpt-4o-mini"),
			},
			"sonnet": {
				BaseURL: envOr("MODEL_SONNET_BASE_URL", "https://api.openai.com/v1"),
				APIKey:  requireEnv("MODEL_SONNET_API_KEY"),
				Model:   envOr("MODEL_SONNET_MODEL", "gpt-4o"),
			},
			"opus": {
				BaseURL: envOr("MODEL_OPUS_BASE_URL", "https://api.openai.com/v1"),
				APIKey:  requireEnv("MODEL_OPUS_API_KEY"),
				Model:   envOr("MODEL_OPUS_MODEL", "o1"),
			},
		},
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic("config: required env var " + key + " is not set")
	}
	return v
}
