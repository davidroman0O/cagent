package config

import (
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Config struct {
	Addr                            string
	Token                           string
	CodexBin                        string
	CodexModelContextWindow         int
	CodexModelAutoCompactTokenLimit int
	DefaultModel                    string
	DefaultReasoningEffort          string
	DefaultCWD                      string
	DataDir                         string
	MaxConcurrent                   int
	QueueLimit                      int
	RequestTimeout                  time.Duration
}

func Load() Config {
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".cagent")

	maxConcurrent := envInt("CAGENT_MAX_CONCURRENT", 2)
	queueLimit := envInt("CAGENT_QUEUE_LIMIT", maxConcurrent*4)
	if queueLimit < maxConcurrent {
		queueLimit = maxConcurrent
	}

	return Config{
		Addr:                            env("CAGENT_ADDR", ":8080"),
		Token:                           os.Getenv("CAGENT_TOKEN"),
		CodexBin:                        os.Getenv("CAGENT_CODEX_BIN"),
		CodexModelContextWindow:         envInt("CAGENT_CODEX_MODEL_CONTEXT_WINDOW", 0),
		CodexModelAutoCompactTokenLimit: envInt("CAGENT_CODEX_MODEL_AUTO_COMPACT_TOKEN_LIMIT", 0),
		DefaultModel:                    os.Getenv("CAGENT_DEFAULT_MODEL"),
		DefaultReasoningEffort:          os.Getenv("CAGENT_DEFAULT_REASONING_EFFORT"),
		DefaultCWD:                      os.Getenv("CAGENT_DEFAULT_CWD"),
		DataDir:                         env("CAGENT_DATA_DIR", dataDir),
		MaxConcurrent:                   maxConcurrent,
		QueueLimit:                      queueLimit,
		RequestTimeout:                  envDuration("CAGENT_REQUEST_TIMEOUT", 10*time.Minute),
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
