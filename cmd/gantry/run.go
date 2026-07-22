package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/channel/stdio"
	"github.com/shotah/ai-gantry/internal/config"
	"github.com/shotah/ai-gantry/internal/persona"
	"github.com/shotah/ai-gantry/internal/provider"
)

// run boots config, persona, provider, agent, and the selected channel.
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

	personaText, err := persona.Load(cfg.PersonaDir)
	if err != nil {
		logger.Error("persona load failed", "err", err)
		return 1
	}
	logger.Info("persona loaded", "chars", len(personaText))

	completer := provider.New(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel)
	ag, err := agent.New(agent.Options{
		Persona:      personaText,
		Completer:    completer,
		Model:        cfg.LLMModel,
		MaxMessages:  cfg.HistoryMaxMessages,
		MaxEstTokens: cfg.HistoryMaxTokens,
		Logger:       logger,
	})
	if err != nil {
		logger.Error("agent init failed", "err", err)
		return 1
	}

	ch, err := newChannel(cfg)
	if err != nil {
		logger.Error("channel init failed", "err", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := ch.Run(ctx, ag.Handle); err != nil {
		logger.Error("channel stopped", "err", err)
		return 1
	}
	logger.Info("gantry stopped")
	return 0
}

func newChannel(cfg *config.Config) (channel.Channel, error) {
	switch cfg.Channel {
	case config.ChannelStdio:
		return stdio.New(), nil
	case config.ChannelTelegram:
		return nil, fmt.Errorf("CHANNEL=telegram is not implemented yet (milestone 2); use CHANNEL=stdio for now")
	default:
		return nil, fmt.Errorf("unknown channel %q", cfg.Channel)
	}
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
	// stderr keeps the stdio REPL on stdout readable; docker logs still captures both.
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lv}))
}
