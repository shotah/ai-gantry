package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/channel/stdio"
	"github.com/shotah/ai-gantry/internal/channel/telegram"
	"github.com/shotah/ai-gantry/internal/config"
	"github.com/shotah/ai-gantry/internal/persona"
	"github.com/shotah/ai-gantry/internal/provider"
	"github.com/shotah/ai-gantry/internal/session"
)

// run boots config, persona, sessions, provider, agent, and the selected channel.
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

	sessions, err := session.Open(cfg.DataDir, cfg.HistoryMaxMessages, cfg.HistoryMaxTokens)
	if err != nil {
		logger.Error("session store open failed", "err", err)
		return 1
	}
	defer func() {
		if err := sessions.Close(); err != nil {
			logger.Error("session store close failed", "err", err)
		}
	}()
	logger.Info("session store ready", "path", filepath.Join(cfg.DataDir, "gantry.db"))

	completer := provider.New(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel)
	ag, err := agent.New(agent.Options{
		Persona:   personaText,
		Completer: completer,
		Sessions:  sessions,
		Model:     cfg.LLMModel,
		Logger:    logger,
	})
	if err != nil {
		logger.Error("agent init failed", "err", err)
		return 1
	}

	ch, err := newChannel(cfg, logger)
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

func newChannel(cfg *config.Config, logger *slog.Logger) (channel.Channel, error) {
	switch cfg.Channel {
	case config.ChannelStdio:
		return stdio.New(), nil
	case config.ChannelTelegram:
		return telegram.New(telegram.Config{
			Token:        cfg.TelegramBotToken,
			AllowedUsers: cfg.TelegramAllowedUsers,
			Logger:       logger,
		})
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
