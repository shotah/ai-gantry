// Package config loads and validates gantry configuration from the environment.
// Boot is fail-fast: missing required values return a clear error.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

// Channel names accepted by CHANNEL.
const (
	ChannelTelegram = "telegram"
	ChannelDiscord  = "discord"
	ChannelStdio    = "stdio"
)

// Config is the complete env-driven configuration surface.
// Secrets and scalars live here; structure (persona, MCP manifest) is mounts.
type Config struct {
	LLMBaseURL string `env:"LLM_BASE_URL,required"`
	LLMAPIKey  string `env:"LLM_API_KEY,required"`
	LLMModel   string `env:"LLM_MODEL,required"`

	TelegramBotToken     string  `env:"TELEGRAM_BOT_TOKEN"`
	TelegramAllowedUsers []int64 `env:"TELEGRAM_ALLOWED_USERS" envSeparator:","`

	DiscordBotToken     string   `env:"DISCORD_BOT_TOKEN"`
	DiscordAllowedUsers []string `env:"DISCORD_ALLOWED_USERS" envSeparator:","`

	Channel     string `env:"CHANNEL" envDefault:"telegram"`
	PersonaDir  string `env:"PERSONA_DIR" envDefault:"/persona"`
	DataDir     string `env:"DATA_DIR" envDefault:"/data"`
	MCPManifest string `env:"MCP_MANIFEST" envDefault:"/etc/gantry/mcp.toml"`

	HistoryMaxMessages int `env:"HISTORY_MAX_MESSAGES" envDefault:"200"`
	HistoryMaxTokens   int `env:"HISTORY_MAX_TOKENS" envDefault:"128000"` // estimated (chars/4)
	ToolResultMaxChars int `env:"TOOL_RESULT_MAX_CHARS" envDefault:"16000"`
	ToolMaxIterations  int `env:"TOOL_MAX_ITERATIONS" envDefault:"20"`
	// ToolSchemaMaxTokens is an optional hard cap on estimated tool-schema tokens
	// (chars/4 of name+description+parameters). 0 = log estimate only.
	ToolSchemaMaxTokens int `env:"TOOL_SCHEMA_MAX_TOKENS" envDefault:"0"`

	MemoryEnabled            bool   `env:"MEMORY_ENABLED" envDefault:"true"`
	MemoryBackend            string `env:"MEMORY_BACKEND" envDefault:"builtin"`
	MemoryConsolidateMinutes int    `env:"MEMORY_CONSOLIDATE_MINUTES" envDefault:"30"` // 0 = off

	CronEnabled     bool   `env:"CRON_ENABLED" envDefault:"true"`
	CronTZ          string `env:"CRON_TZ" envDefault:"UTC"`
	CronMaxJobs     int    `env:"CRON_MAX_JOBS" envDefault:"50"`
	CronTickSeconds int    `env:"CRON_TICK_SECONDS" envDefault:"15"`

	StreamReplies bool `env:"STREAM_REPLIES" envDefault:"false"`

	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
}

// Load parses environment variables into Config and validates channel-specific
// requirements. Returns a descriptive error on any failure.
func Load() (*Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Validate checks cross-field and channel-specific rules after env parsing.
func (c *Config) Validate() error {
	c.Channel = strings.ToLower(strings.TrimSpace(c.Channel))
	c.LogLevel = strings.ToLower(strings.TrimSpace(c.LogLevel))
	c.MemoryBackend = strings.TrimSpace(c.MemoryBackend)

	switch c.Channel {
	case ChannelTelegram, ChannelDiscord, ChannelStdio:
	default:
		return fmt.Errorf("CHANNEL: must be %q, %q, or %q, got %q", ChannelTelegram, ChannelDiscord, ChannelStdio, c.Channel)
	}

	if c.Channel == ChannelTelegram {
		if strings.TrimSpace(c.TelegramBotToken) == "" {
			return fmt.Errorf("TELEGRAM_BOT_TOKEN: required when CHANNEL=telegram")
		}
		if len(c.TelegramAllowedUsers) == 0 {
			return fmt.Errorf("TELEGRAM_ALLOWED_USERS: required when CHANNEL=telegram (comma-separated user ids)")
		}
	}

	if c.Channel == ChannelDiscord {
		if strings.TrimSpace(c.DiscordBotToken) == "" {
			return fmt.Errorf("DISCORD_BOT_TOKEN: required when CHANNEL=discord")
		}
		n := 0
		for i, id := range c.DiscordAllowedUsers {
			id = strings.TrimSpace(id)
			c.DiscordAllowedUsers[i] = id
			if id != "" {
				n++
			}
		}
		if n == 0 {
			return fmt.Errorf("DISCORD_ALLOWED_USERS: required when CHANNEL=discord (comma-separated snowflake user ids)")
		}
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("LOG_LEVEL: must be debug|info|warn|error, got %q", c.LogLevel)
	}

	if c.HistoryMaxMessages < 1 {
		return fmt.Errorf("HISTORY_MAX_MESSAGES: must be >= 1, got %d", c.HistoryMaxMessages)
	}
	if c.HistoryMaxTokens < 1 {
		return fmt.Errorf("HISTORY_MAX_TOKENS: must be >= 1, got %d", c.HistoryMaxTokens)
	}
	if c.ToolResultMaxChars < 1 {
		return fmt.Errorf("TOOL_RESULT_MAX_CHARS: must be >= 1, got %d", c.ToolResultMaxChars)
	}
	if c.ToolMaxIterations < 1 {
		return fmt.Errorf("TOOL_MAX_ITERATIONS: must be >= 1, got %d", c.ToolMaxIterations)
	}
	if c.ToolSchemaMaxTokens < 0 {
		return fmt.Errorf("TOOL_SCHEMA_MAX_TOKENS: must be >= 0, got %d", c.ToolSchemaMaxTokens)
	}
	if c.MemoryConsolidateMinutes < 0 {
		return fmt.Errorf("MEMORY_CONSOLIDATE_MINUTES: must be >= 0, got %d", c.MemoryConsolidateMinutes)
	}
	c.CronTZ = strings.TrimSpace(c.CronTZ)
	if c.CronTZ == "" {
		c.CronTZ = "UTC"
	}
	if c.CronMaxJobs < 1 {
		return fmt.Errorf("CRON_MAX_JOBS: must be >= 1, got %d", c.CronMaxJobs)
	}
	if c.CronTickSeconds < 1 {
		return fmt.Errorf("CRON_TICK_SECONDS: must be >= 1, got %d", c.CronTickSeconds)
	}
	if _, err := timeLoadLocation(c.CronTZ); err != nil {
		return fmt.Errorf("CRON_TZ: %w", err)
	}

	if err := validateMemoryBackend(c.MemoryBackend); err != nil {
		return err
	}

	if strings.TrimSpace(c.PersonaDir) == "" {
		return fmt.Errorf("PERSONA_DIR: must not be empty")
	}
	if strings.TrimSpace(c.DataDir) == "" {
		return fmt.Errorf("DATA_DIR: must not be empty")
	}
	if strings.TrimSpace(c.MCPManifest) == "" {
		return fmt.Errorf("MCP_MANIFEST: must not be empty")
	}
	if strings.TrimSpace(c.LLMBaseURL) == "" {
		return fmt.Errorf("LLM_BASE_URL: must not be empty")
	}
	if strings.TrimSpace(c.LLMAPIKey) == "" {
		return fmt.Errorf("LLM_API_KEY: must not be empty")
	}
	if strings.TrimSpace(c.LLMModel) == "" {
		return fmt.Errorf("LLM_MODEL: must not be empty")
	}

	return nil
}

func timeLoadLocation(name string) (*time.Location, error) {
	if strings.EqualFold(name, "UTC") {
		return time.UTC, nil
	}
	return time.LoadLocation(name)
}

func validateMemoryBackend(backend string) error {
	if backend == "builtin" {
		return nil
	}
	if strings.HasPrefix(backend, "mcp:") {
		name := strings.TrimPrefix(backend, "mcp:")
		if name == "" {
			return fmt.Errorf("MEMORY_BACKEND: mcp:<server-name> requires a server name")
		}
		return nil
	}
	return fmt.Errorf("MEMORY_BACKEND: must be %q or %q, got %q", "builtin", "mcp:<server-name>", backend)
}
