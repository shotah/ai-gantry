package config_test

import (
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/config"
)

func setRequiredLLM(t *testing.T) {
	t.Helper()
	t.Setenv("LLM_BASE_URL", "https://example.com/v1")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_MODEL", "test-model")
}

func TestLoad_StdioDefaults(t *testing.T) {
	setRequiredLLM(t)
	t.Setenv("CHANNEL", "stdio")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Channel != config.ChannelStdio {
		t.Errorf("Channel = %q, want stdio", cfg.Channel)
	}
	if cfg.PersonaDir != "/persona" {
		t.Errorf("PersonaDir = %q, want /persona", cfg.PersonaDir)
	}
	if cfg.DataDir != "/data" {
		t.Errorf("DataDir = %q, want /data", cfg.DataDir)
	}
	if cfg.MCPManifest != "/etc/gantry/mcp.toml" {
		t.Errorf("MCPManifest = %q, want /etc/gantry/mcp.toml", cfg.MCPManifest)
	}
	if cfg.HistoryMaxMessages != 200 {
		t.Errorf("HistoryMaxMessages = %d, want 200", cfg.HistoryMaxMessages)
	}
	if cfg.HistoryMaxTokens != 128000 {
		t.Errorf("HistoryMaxTokens = %d, want 128000", cfg.HistoryMaxTokens)
	}
	if cfg.ToolResultMaxChars != 16000 {
		t.Errorf("ToolResultMaxChars = %d, want 16000", cfg.ToolResultMaxChars)
	}
	if cfg.ToolMaxIterations != 20 {
		t.Errorf("ToolMaxIterations = %d, want 20", cfg.ToolMaxIterations)
	}
	if !cfg.MemoryEnabled {
		t.Error("MemoryEnabled = false, want true")
	}
	if cfg.MemoryBackend != "builtin" {
		t.Errorf("MemoryBackend = %q, want builtin", cfg.MemoryBackend)
	}
	if cfg.MemoryConsolidateMinutes != 30 {
		t.Errorf("MemoryConsolidateMinutes = %d, want 30", cfg.MemoryConsolidateMinutes)
	}
	if !cfg.CronEnabled {
		t.Error("CronEnabled = false, want true")
	}
	if cfg.CronTZ != "UTC" {
		t.Errorf("CronTZ = %q, want UTC", cfg.CronTZ)
	}
	if cfg.CronMaxJobs != 50 {
		t.Errorf("CronMaxJobs = %d, want 50", cfg.CronMaxJobs)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
}

func TestLoad_TelegramRequiresTokenAndAllowlist(t *testing.T) {
	setRequiredLLM(t)
	t.Setenv("CHANNEL", "telegram")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load: expected error for missing telegram fields")
	}
	if !strings.Contains(err.Error(), "TELEGRAM_BOT_TOKEN") {
		t.Errorf("error = %q, want TELEGRAM_BOT_TOKEN mention", err)
	}

	t.Setenv("TELEGRAM_BOT_TOKEN", "tok")
	_, err = config.Load()
	if err == nil {
		t.Fatal("Load: expected error for missing allowlist")
	}
	if !strings.Contains(err.Error(), "TELEGRAM_ALLOWED_USERS") {
		t.Errorf("error = %q, want TELEGRAM_ALLOWED_USERS mention", err)
	}

	t.Setenv("TELEGRAM_ALLOWED_USERS", "123,456")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.TelegramAllowedUsers) != 2 {
		t.Fatalf("TelegramAllowedUsers len = %d, want 2", len(cfg.TelegramAllowedUsers))
	}
	if cfg.TelegramAllowedUsers[0] != 123 || cfg.TelegramAllowedUsers[1] != 456 {
		t.Errorf("TelegramAllowedUsers = %v, want [123 456]", cfg.TelegramAllowedUsers)
	}
}

func TestLoad_MissingRequiredLLM(t *testing.T) {
	t.Setenv("CHANNEL", "stdio")
	// Intentionally leave LLM_* unset / empty.
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_MODEL", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load: expected error for missing LLM_* vars")
	}
}

func TestLoad_InvalidChannel(t *testing.T) {
	setRequiredLLM(t)
	t.Setenv("CHANNEL", "irc")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load: expected error for invalid channel")
	}
	if !strings.Contains(err.Error(), "CHANNEL") {
		t.Errorf("error = %q, want CHANNEL mention", err)
	}
}

func TestLoad_DiscordRequiresTokenAndAllowlist(t *testing.T) {
	setRequiredLLM(t)
	t.Setenv("CHANNEL", "discord")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load: expected error for missing discord fields")
	}
	if !strings.Contains(err.Error(), "DISCORD_BOT_TOKEN") {
		t.Errorf("error = %q, want DISCORD_BOT_TOKEN mention", err)
	}

	t.Setenv("DISCORD_BOT_TOKEN", "tok")
	_, err = config.Load()
	if err == nil {
		t.Fatal("Load: expected error for missing allowlist")
	}
	if !strings.Contains(err.Error(), "DISCORD_ALLOWED_USERS") {
		t.Errorf("error = %q, want DISCORD_ALLOWED_USERS mention", err)
	}

	t.Setenv("DISCORD_ALLOWED_USERS", "111, 222")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Channel != config.ChannelDiscord {
		t.Fatalf("Channel = %q", cfg.Channel)
	}
	if len(cfg.DiscordAllowedUsers) != 2 || cfg.DiscordAllowedUsers[0] != "111" || cfg.DiscordAllowedUsers[1] != "222" {
		t.Fatalf("DiscordAllowedUsers = %v", cfg.DiscordAllowedUsers)
	}
}

func TestLoad_MemoryBackendMCP(t *testing.T) {
	setRequiredLLM(t)
	t.Setenv("CHANNEL", "stdio")
	t.Setenv("MEMORY_BACKEND", "mcp:custom-memory")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.MemoryBackend != "mcp:custom-memory" {
		t.Errorf("MemoryBackend = %q, want mcp:custom-memory", cfg.MemoryBackend)
	}
}

func TestLoad_InvalidMemoryBackend(t *testing.T) {
	setRequiredLLM(t)
	t.Setenv("CHANNEL", "stdio")
	t.Setenv("MEMORY_BACKEND", "redis")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load: expected error for invalid memory backend")
	}
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	setRequiredLLM(t)
	t.Setenv("CHANNEL", "stdio")
	t.Setenv("LOG_LEVEL", "verbose")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load: expected error for invalid log level")
	}
}

func TestLoad_Bounds(t *testing.T) {
	setRequiredLLM(t)
	t.Setenv("CHANNEL", "stdio")
	t.Setenv("HISTORY_MAX_MESSAGES", "0")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load: expected error for HISTORY_MAX_MESSAGES=0")
	}
}

func TestLoad_MoreValidation(t *testing.T) {
	setRequiredLLM(t)
	t.Setenv("CHANNEL", "stdio")

	cases := []struct {
		key, val, want string
	}{
		{"HISTORY_MAX_TOKENS", "0", "HISTORY_MAX_TOKENS"},
		{"TOOL_RESULT_MAX_CHARS", "0", "TOOL_RESULT_MAX_CHARS"},
		{"TOOL_MAX_ITERATIONS", "0", "TOOL_MAX_ITERATIONS"},
		{"TOOL_SCHEMA_MAX_TOKENS", "-1", "TOOL_SCHEMA_MAX_TOKENS"},
		{"MEMORY_CONSOLIDATE_MINUTES", "-1", "MEMORY_CONSOLIDATE_MINUTES"},
		{"MEMORY_BACKEND", "mcp:", "MEMORY_BACKEND"},
		{"PERSONA_DIR", "   ", "PERSONA_DIR"},
		{"DATA_DIR", "   ", "DATA_DIR"},
		{"MCP_MANIFEST", "   ", "MCP_MANIFEST"},
		{"LOG_LEVEL", "DEBUG", ""}, // valid after normalize
	}
	for _, tc := range cases {
		t.Run(tc.key+"="+tc.val, func(t *testing.T) {
			setRequiredLLM(t)
			t.Setenv("CHANNEL", "stdio")
			t.Setenv(tc.key, tc.val)
			cfg, err := config.Load()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
				if cfg.LogLevel != "debug" {
					t.Fatalf("LogLevel=%q", cfg.LogLevel)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}
