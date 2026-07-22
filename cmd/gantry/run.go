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
	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/drain"
	"github.com/shotah/ai-gantry/internal/heartbeat"
	"github.com/shotah/ai-gantry/internal/mcp"
	"github.com/shotah/ai-gantry/internal/memory"
	"github.com/shotah/ai-gantry/internal/persona"
	"github.com/shotah/ai-gantry/internal/provider"
	"github.com/shotah/ai-gantry/internal/session"
)

// run boots config, persona, sessions, MCP host, memory, cron, provider, agent, and channel.
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
		"cron_enabled", cfg.CronEnabled,
		"cron_tz", cfg.CronTZ,
		"stream_replies", cfg.StreamReplies,
	)

	personaText, err := persona.Load(cfg.PersonaDir)
	if err != nil {
		logger.Error("persona load failed", "err", err)
		return 1
	}
	logger.Info("persona loaded", "chars", len(personaText))

	completer := provider.New(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel)

	sessions, err := session.Open(cfg.DataDir, cfg.HistoryMaxMessages, cfg.HistoryMaxTokens)
	if err != nil {
		logger.Error("session store open failed", "err", err)
		return 1
	}
	sessions.WithSummarizer(&session.LLMSummarizer{Completer: completer})
	defer func() {
		if err := sessions.Close(); err != nil {
			logger.Error("session store close failed", "err", err)
		}
	}()
	logger.Info("session store ready", "path", filepath.Join(cfg.DataDir, "gantry.db"))

	hb, err := heartbeat.OpenDB(sessions.DB())
	if err != nil {
		logger.Error("heartbeat open failed", "err", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go hb.Start(ctx, heartbeat.DefaultInterval, version, logger)

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
				Completer: completer,
				Interval:  time.Duration(cfg.MemoryConsolidateMinutes) * time.Minute,
				Logger:    logger,
			}
			go consol.Start(ctx)
		}
	}

	var cronStore *cron.Store
	if cfg.CronEnabled {
		cronStore, err = cron.OpenDB(sessions.DB(), cfg.CronMaxJobs)
		if err != nil {
			logger.Error("cron store open failed", "err", err)
			return 1
		}
		tools = cron.Composite{
			Cron:  cron.Tools{Store: cronStore, TZ: cfg.CronTZ},
			Other: tools,
		}
		logger.Info("cron ready", "tz", cfg.CronTZ, "max_jobs", cfg.CronMaxJobs)
	}

	estTokens := mcp.EstimateToolSchemaTokens(tools.Tools())
	logger.Info("tool schema estimate",
		"tools", tools.ToolCount(),
		"est_tokens", estTokens,
		"max_tokens", cfg.ToolSchemaMaxTokens,
	)
	if cfg.ToolSchemaMaxTokens > 0 && estTokens > cfg.ToolSchemaMaxTokens {
		logger.Error("tool schema exceeds TOOL_SCHEMA_MAX_TOKENS",
			"est_tokens", estTokens,
			"max_tokens", cfg.ToolSchemaMaxTokens,
		)
		return 1
	}

	ag, err := agent.New(agent.Options{
		Persona:       personaText,
		Completer:     completer,
		Sessions:      sessions,
		Tools:         tools,
		Memory:        memBackend,
		Model:         cfg.LLMModel,
		MaxToolIters:  cfg.ToolMaxIterations,
		StreamReplies: cfg.StreamReplies,
		Logger:        logger,
	})
	if err != nil {
		logger.Error("agent init failed", "err", err)
		return 1
	}
	go watchPersonaReload(ctx, cfg.PersonaDir, ag, logger)

	ch, err := newChannel(cfg, logger)
	if err != nil {
		logger.Error("channel init failed", "err", err)
		return 1
	}

	gate := &drain.Gate{}
	handle := gate.Handler(ag.Handle)

	if cronStore != nil {
		pusher, ok := ch.(channel.Pusher)
		if !ok {
			logger.Error("cron enabled but channel does not support Push")
			return 1
		}
		runner := &cron.Runner{
			Store:    cronStore,
			Handle:   handle,
			Pusher:   pusher,
			Interval: time.Duration(cfg.CronTickSeconds) * time.Second,
			Logger:   logger,
		}
		go runner.Start(ctx)
	}

	runErr := ch.Run(ctx, handle)
	// Finish the in-flight turn before deferred MCP Close kills children.
	if !gate.Wait(drain.DefaultWait) {
		logger.Warn("shutdown: in-flight turn still running after wait", "timeout", drain.DefaultWait.String())
	}
	if runErr != nil {
		logger.Error("channel stopped", "err", runErr)
		return 1
	}
	logger.Info("gantry stopped")
	return 0
}

func newChannel(cfg *config.Config, logger *slog.Logger) (channel.Channel, error) {
	switch cfg.Channel {
	case config.ChannelStdio:
		ch := stdio.New()
		ch.StreamReplies = cfg.StreamReplies
		return ch, nil
	case config.ChannelTelegram:
		return telegram.New(telegram.Config{
			Token:         cfg.TelegramBotToken,
			AllowedUsers:  cfg.TelegramAllowedUsers,
			Logger:        logger,
			StreamReplies: cfg.StreamReplies,
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
