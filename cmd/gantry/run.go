package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/shotah/ai-gantry/internal/config"
)

// run boots the gantry. Milestone 0: load config and exit cleanly.
// Later milestones wire channel → agent → MCP → memory here.
func run() int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		return 1
	}

	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	logger.Info("gantry starting",
		"version", version,
		"channel", cfg.Channel,
		"model", cfg.LLMModel,
		"persona_dir", cfg.PersonaDir,
		"data_dir", cfg.DataDir,
		"mcp_manifest", cfg.MCPManifest,
		"memory_enabled", cfg.MemoryEnabled,
		"memory_backend", cfg.MemoryBackend,
	)

	// Milestone 0 scaffold only — the daemon loop arrives in M1+.
	logger.Info("scaffold complete; agent loop not yet implemented (see readme §11)")
	return 0
}

func newLogger(level string) *slog.Logger {
	var lv slog.Level
	switch level {
	case "debug":
		lv = slog.LevelDebug
	case "warn":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lv}))
}
