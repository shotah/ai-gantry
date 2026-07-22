package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/channel/stdio"
	"github.com/shotah/ai-gantry/internal/channel/telegram"
	"github.com/shotah/ai-gantry/internal/config"
	"github.com/shotah/ai-gantry/internal/mcp"
	"github.com/shotah/ai-gantry/internal/memory"
	"github.com/shotah/ai-gantry/internal/persona"
	"github.com/shotah/ai-gantry/internal/provider"
	"github.com/shotah/ai-gantry/internal/session"
)

// run boots config, persona, sessions, MCP host, memory, provider, agent, and channel.
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mcpHost, err := mcp.Start(ctx, mcp.Options{
		ManifestPath:   cfg.MCPManifest,
		Logger:         logger,
		ResultMaxChars: cfg.ToolResultMaxChars,
	})
	if err != nil {
		logger.Error("mcp host failed", "err", err)
		return 1
	}
	defer func() {
		if err := mcpHost.Close(); err != nil {
			logger.Error("mcp host close failed", "err", err)
		}
	}()

	var (
		memBackend memory.Memory
		memBuiltin *memory.Builtin
		hideServer string
		tools      agent.Tools = mcpHost
	)

	if cfg.MemoryEnabled {
		switch {
		case cfg.MemoryBackend == "builtin":
			memBuiltin, err = memory.OpenDB(sessions.DB())
			if err != nil {
				logger.Error("memory open failed", "err", err)
				return 1
			}
			memBackend = memBuiltin
			logger.Info("memory ready", "backend", "builtin")
		case strings.HasPrefix(cfg.MemoryBackend, "mcp:"):
			server := strings.TrimPrefix(cfg.MemoryBackend, "mcp:")
			adapter, err := memory.NewMCPAdapter(mcpHost, server)
			if err != nil {
				logger.Error("memory mcp adapter failed", "err", err)
				return 1
			}
			memBackend = adapter
			hideServer = server
			logger.Info("memory ready", "backend", "mcp", "server", server)
		default:
			logger.Error("memory backend unsupported", "backend", cfg.MemoryBackend)
			return 1
		}
		defer func() {
			if err := memBackend.Close(); err != nil {
				logger.Error("memory close failed", "err", err)
			}
		}()

		tools = memory.Composite{
			Memory:        memory.Tools{Backend: memBackend},
			Other:         mcpHost,
			HideMCPServer: hideServer,
		}

		if memBuiltin != nil && cfg.MemoryConsolidateMinutes > 0 {
			consol := &memory.Consolidator{
				Store:     memBuiltin,
				Completer: provider.New(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel),
				Interval:  time.Duration(cfg.MemoryConsolidateMinutes) * time.Minute,
				Logger:    logger,
			}
			go consol.Start(ctx)
		}
	}

	completer := provider.New(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel)
	ag, err := agent.New(agent.Options{
		Persona:      personaText,
		Completer:    completer,
		Sessions:     sessions,
		Tools:        tools,
		Memory:       memBackend,
		Model:        cfg.LLMModel,
		MaxToolIters: cfg.ToolMaxIterations,
		Logger:       logger,
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
