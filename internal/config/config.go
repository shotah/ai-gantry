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
	ChannelSlack    = "slack"
	ChannelStdio    = "stdio"
)

// Config is the complete env-driven configuration surface.
// Secrets and scalars live here; structure (persona, MCP manifest) is mounts.
type Config struct {
	LLMBaseURL string `env:"LLM_BASE_URL,required"`
	LLMAPIKey  string `env:"LLM_API_KEY,required"`
	LLMModel   string `env:"LLM_MODEL,required"`
	// LLMMaxTokens caps completion output (incl. tool-call args). 0 = provider default.
	LLMMaxTokens int `env:"LLM_MAX_TOKENS" envDefault:"4096"`
	// LLMReasoningEffort is sent as reasoning_effort when non-empty (Ollama/Qwen:
	// "none" disables thinking so max_tokens is not eaten by hidden chain-of-thought).
	LLMReasoningEffort string `env:"LLM_REASONING_EFFORT"`

	TelegramBotToken     string  `env:"TELEGRAM_BOT_TOKEN"`
	TelegramAllowedUsers []int64 `env:"TELEGRAM_ALLOWED_USERS" envSeparator:","`
	// TelegramErrorReporting tees slog ERROR (or WARN+) into the SAM Telegram
	// chat as an expandable HTML alert. off|error|warn. Only when CHANNEL=telegram.
	TelegramErrorReporting string `env:"TELEGRAM_ERROR_REPORTING" envDefault:"off"`

	DiscordBotToken     string   `env:"DISCORD_BOT_TOKEN"`
	DiscordAllowedUsers []string `env:"DISCORD_ALLOWED_USERS" envSeparator:","`

	SlackBotToken     string   `env:"SLACK_BOT_TOKEN"` // xoxb-
	SlackAppToken     string   `env:"SLACK_APP_TOKEN"` // xapp- (Socket Mode)
	SlackAllowedUsers []string `env:"SLACK_ALLOWED_USERS" envSeparator:","`

	Channel     string `env:"CHANNEL" envDefault:"telegram"`
	PersonaDir  string `env:"PERSONA_DIR" envDefault:"/persona"`
	DataDir     string `env:"DATA_DIR" envDefault:"/data"`
	MCPManifest string `env:"MCP_MANIFEST" envDefault:"/etc/gantry/mcp.toml"`

	HistoryMaxMessages int `env:"HISTORY_MAX_MESSAGES" envDefault:"200"`
	HistoryMaxTokens   int `env:"HISTORY_MAX_TOKENS" envDefault:"128000"` // estimated (chars/4)
	ToolResultMaxChars int `env:"TOOL_RESULT_MAX_CHARS" envDefault:"6000"`
	ToolMaxIterations  int `env:"TOOL_MAX_ITERATIONS" envDefault:"10"`
	// ToolSchemaMaxTokens is an optional hard cap on estimated tool-schema tokens
	// (chars/4 of name+description+parameters). 0 = log estimate only.
	ToolSchemaMaxTokens int `env:"TOOL_SCHEMA_MAX_TOKENS" envDefault:"0"`

	// ToolsEnabled controls whether tool schemas are sent to the model.
	// false omits MCP, memory_*, and cron_* tools from every completion — required
	// for models that reject tools (e.g. Ollama gemma3). Memory/cron backends may
	// still start; only the agent tool surface is cleared.
	ToolsEnabled bool `env:"TOOLS_ENABLED" envDefault:"true"`

	// SelfNotesEnabled lets the agent keep SELF.md in PERSONA_DIR: a self_note
	// tool for jotting personality lines, plus a distill pass on /new that
	// folds the dying session's voice/jokes/rituals into the file so they
	// survive the reset. Auto-disables when PERSONA_DIR is not writable.
	SelfNotesEnabled bool `env:"SELF_NOTES_ENABLED" envDefault:"true"`

	MemoryEnabled            bool   `env:"MEMORY_ENABLED" envDefault:"true"`
	MemoryBackend            string `env:"MEMORY_BACKEND" envDefault:"builtin"`
	MemoryConsolidateMinutes int    `env:"MEMORY_CONSOLIDATE_MINUTES" envDefault:"30"` // 0 = off

	CronEnabled     bool   `env:"CRON_ENABLED" envDefault:"true"`
	CronTZ          string `env:"CRON_TZ" envDefault:"UTC"`
	CronMaxJobs     int    `env:"CRON_MAX_JOBS" envDefault:"50"`
	CronTickSeconds int    `env:"CRON_TICK_SECONDS" envDefault:"15"`

	// Spark of life (opt-in). Empty SPARK_QTY = disabled. Examples: "5", "4-6".
	SparkQty               string `env:"SPARK_QTY" envDefault:""`
	SparkStartHour         int    `env:"SPARK_START_HOUR" envDefault:"6"`
	SparkEndHour           int    `env:"SPARK_END_HOUR" envDefault:"21"`
	SparkPrompt            string `env:"SPARK_PROMPT" envDefault:""`
	SparkSkipRecentMinutes int    `env:"SPARK_SKIP_RECENT_MINUTES" envDefault:"15"`

	StreamReplies bool `env:"STREAM_REPLIES" envDefault:"true"`

	// ShowThinking controls whether chain-of-thought is rendered in the Telegram
	// stream bubble (live italics → final expandable blockquote). On by default;
	// set false for a quieter bubble. Pair with LLM_REASONING_EFFORT=none on
	// slow local models so CoT is not generated (and therefore not shown).
	// Needs STREAM_REPLIES=true. Does not change model-side think on/off.
	ShowThinking bool `env:"SHOW_THINKING" envDefault:"true"`

	// ToolTrace controls user-visible tool activity when STREAM_REPLIES is on.
	// compact = Making Calls: ✓, ✗ (default); full = → name / ✓ timing lines;
	// off = hide tool activity entirely. Journal logs are unaffected.
	ToolTrace string `env:"TOOL_TRACE" envDefault:"compact"`

	// CoalesceSettleMS is quiet time after the last chat bubble before running
	// one joined turn (interrupt + coalesce). 0 disables. Default 2000ms.
	CoalesceSettleMS int `env:"COALESCE_SETTLE_MS" envDefault:"2000"`

	// SpinupNoticeMS posts a "still working" line once a turn has gone this
	// long without model output. The first turn after start posts immediately.
	// Needs STREAM_REPLIES=true. 0 disables. Default 4000ms.
	SpinupNoticeMS int `env:"SPINUP_NOTICE_MS" envDefault:"4000"`

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
	case ChannelTelegram, ChannelDiscord, ChannelSlack, ChannelStdio:
	default:
		return fmt.Errorf("CHANNEL: must be telegram|discord|slack|stdio, got %q", c.Channel)
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

	if c.Channel == ChannelSlack {
		if strings.TrimSpace(c.SlackBotToken) == "" {
			return fmt.Errorf("SLACK_BOT_TOKEN: required when CHANNEL=slack (xoxb-…)")
		}
		if strings.TrimSpace(c.SlackAppToken) == "" {
			return fmt.Errorf("SLACK_APP_TOKEN: required when CHANNEL=slack (xapp-… Socket Mode)")
		}
		n := 0
		for i, id := range c.SlackAllowedUsers {
			id = strings.TrimSpace(id)
			c.SlackAllowedUsers[i] = id
			if id != "" {
				n++
			}
		}
		if n == 0 {
			return fmt.Errorf("SLACK_ALLOWED_USERS: required when CHANNEL=slack (comma-separated user ids)")
		}
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("LOG_LEVEL: must be debug|info|warn|error, got %q", c.LogLevel)
	}

	c.ToolTrace = strings.ToLower(strings.TrimSpace(c.ToolTrace))
	switch c.ToolTrace {
	case "", "full", "compact", "off":
		if c.ToolTrace == "" {
			c.ToolTrace = "compact"
		}
	default:
		return fmt.Errorf("TOOL_TRACE: must be full|compact|off, got %q", c.ToolTrace)
	}

	c.TelegramErrorReporting = strings.ToLower(strings.TrimSpace(c.TelegramErrorReporting))
	switch c.TelegramErrorReporting {
	case "", "off", "error", "warn":
		if c.TelegramErrorReporting == "" {
			c.TelegramErrorReporting = "off"
		}
	default:
		return fmt.Errorf("TELEGRAM_ERROR_REPORTING: must be off|error|warn, got %q", c.TelegramErrorReporting)
	}
	if c.TelegramErrorReporting != "off" && c.Channel != ChannelTelegram {
		return fmt.Errorf("TELEGRAM_ERROR_REPORTING: only supported when CHANNEL=telegram")
	}

	if c.LLMMaxTokens < 0 {
		return fmt.Errorf("LLM_MAX_TOKENS: must be >= 0, got %d", c.LLMMaxTokens)
	}
	c.LLMReasoningEffort = strings.TrimSpace(c.LLMReasoningEffort)
	if c.LLMReasoningEffort != "" {
		switch c.LLMReasoningEffort {
		case "none", "minimal", "low", "medium", "high", "xhigh", "max":
		default:
			return fmt.Errorf("LLM_REASONING_EFFORT: must be none|minimal|low|medium|high|xhigh|max, got %q", c.LLMReasoningEffort)
		}
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
	if c.CoalesceSettleMS < 0 {
		return fmt.Errorf("COALESCE_SETTLE_MS: must be >= 0, got %d", c.CoalesceSettleMS)
	}
	if c.SpinupNoticeMS < 0 {
		return fmt.Errorf("SPINUP_NOTICE_MS: must be >= 0, got %d", c.SpinupNoticeMS)
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

	c.SparkQty = strings.TrimSpace(c.SparkQty)
	if c.SparkQty != "" {
		if c.SparkStartHour < 0 || c.SparkStartHour > 23 {
			return fmt.Errorf("SPARK_START_HOUR: must be 0–23, got %d", c.SparkStartHour)
		}
		if c.SparkEndHour < 1 || c.SparkEndHour > 24 || c.SparkEndHour <= c.SparkStartHour {
			return fmt.Errorf("SPARK_END_HOUR: must be 1–24 and > SPARK_START_HOUR, got %d", c.SparkEndHour)
		}
		if c.SparkSkipRecentMinutes < 0 {
			return fmt.Errorf("SPARK_SKIP_RECENT_MINUTES: must be >= 0, got %d", c.SparkSkipRecentMinutes)
		}
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
