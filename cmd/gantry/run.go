package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/channel/discord"
	"github.com/shotah/ai-gantry/internal/channel/slack"
	"github.com/shotah/ai-gantry/internal/channel/stdio"
	"github.com/shotah/ai-gantry/internal/channel/telegram"
	"github.com/shotah/ai-gantry/internal/config"
	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/drain"
	"github.com/shotah/ai-gantry/internal/examples"
	"github.com/shotah/ai-gantry/internal/heartbeat"
	"github.com/shotah/ai-gantry/internal/logfwd"
	"github.com/shotah/ai-gantry/internal/mcp"
	"github.com/shotah/ai-gantry/internal/memory"
	"github.com/shotah/ai-gantry/internal/persona"
	"github.com/shotah/ai-gantry/internal/provider"
	"github.com/shotah/ai-gantry/internal/selfnote"
	"github.com/shotah/ai-gantry/internal/session"
)

// run boots config, persona, sessions, MCP host, memory, cron, provider, agent, and channel.
func run() int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		return 1
	}

	logger, errFwd := newLogger(cfg.LogLevel, cfg.TelegramErrorReporting)
	slog.SetDefault(logger)

	logger.Info("gantry starting",
		"version", version,
		"channel", cfg.Channel,
		"model", cfg.LLMModel,
		"max_tokens", cfg.LLMMaxTokens,
		"reasoning_effort", cfg.LLMReasoningEffort,
		"persona_dir", cfg.PersonaDir,
		"data_dir", cfg.DataDir,
		"mcp_manifest", cfg.MCPManifest,
		"memory_enabled", cfg.MemoryEnabled,
		"memory_backend", cfg.MemoryBackend,
		"self_notes_enabled", cfg.SelfNotesEnabled,
		"cron_enabled", cfg.CronEnabled,
		"cron_tz", cfg.CronTZ,
		"spark_qty", cfg.SparkQty,
		"examples_qty", cfg.ExamplesQty,
		"stream_replies", cfg.StreamReplies,
		"show_thinking", cfg.ShowThinking,
		"tool_trace", cfg.ToolTrace,
		"telegram_error_reporting", cfg.TelegramErrorReporting,
	)

	personaText, err := persona.Load(cfg.PersonaDir)
	if err != nil {
		logger.Error("persona load failed", "err", err)
		return 1
	}
	logger.Info("persona loaded", "chars", len(personaText))

	tzName, tzLoc, tzSource := persona.ResolveTimezone(personaText, cfg.CronTZ)
	logger.Info("human timezone", "tz", tzName, "source", tzSource)
	if strings.EqualFold(tzName, "UTC") {
		logger.Warn("human timezone is UTC; set Timezone in USER.md (or CRON_TZ) to the human's IANA zone")
	}

	completer := provider.New(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel).
		WithMaxTokens(cfg.LLMMaxTokens).
		WithReasoningEffort(cfg.LLMReasoningEffort)

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
		consol     *memory.Consolidator
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
			if cfg.MemoryConsolidateMinutes > 0 {
				logger.Warn("MEMORY_CONSOLIDATE_MINUTES ignored for MCP memory backend (builtin consolidator only)",
					"minutes", cfg.MemoryConsolidateMinutes, "server", server)
			}
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
			consol = &memory.Consolidator{
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
			Cron:  cron.Tools{Store: cronStore, TZ: tzName},
			Other: tools,
		}
		logger.Info("cron ready", "tz", tzName, "max_jobs", cfg.CronMaxJobs)
	}

	var selfStore *selfnote.Store
	if cfg.SelfNotesEnabled {
		selfStore, err = selfnote.Open(cfg.PersonaDir)
		if err != nil {
			// Read-only persona mounts are common; degrade instead of failing boot.
			logger.Warn("self-notes disabled (persona dir not writable)", "err", err)
		} else {
			tools = selfnote.Composite{
				Self:  selfnote.Tools{Store: selfStore},
				Other: tools,
			}
			logger.Info("self-notes ready", "file", filepath.Join(cfg.PersonaDir, selfnote.FileName))
		}
	}

	agentTools := tools
	if !cfg.ToolsEnabled {
		agentTools = nil
		logger.Info("tools disabled (TOOLS_ENABLED=false); omitting tool schemas from model requests")
	} else {
		budget := mcp.EstimateSchemaBudget(tools.Tools())
		logger.Info("tool schema estimate",
			"tools", budget.Tools,
			"est_tokens", budget.EstTokens,
			"max_tokens", cfg.ToolSchemaMaxTokens,
		)
		for _, s := range budget.ByServer {
			logger.Info("tool schema by server",
				"server", s.Server,
				"tools", s.Tools,
				"est_tokens", s.EstTokens,
			)
		}
		if cfg.ToolSchemaMaxTokens > 0 && budget.EstTokens > cfg.ToolSchemaMaxTokens {
			logger.Error("tool schema exceeds TOOL_SCHEMA_MAX_TOKENS",
				"est_tokens", budget.EstTokens,
				"max_tokens", cfg.ToolSchemaMaxTokens,
			)
			return 1
		}
	}

	if consol != nil {
		consol.Location = tzLoc
	}
	var examplesSvc *examples.Service
	if cronStore != nil {
		catalog := tools
		examplesSvc = &examples.Service{
			Store:     cronStore,
			Qty:       cfg.ExamplesQty,
			StartHour: cfg.ExamplesStartHour,
			EndHour:   cfg.ExamplesEndHour,
			TZ:        tzName,
			Tools:     catalog.Tools,
		}
	}

	agentOpts := agent.Options{
		Persona:        personaText,
		Completer:      completer,
		Sessions:       sessions,
		Tools:          agentTools,
		Memory:         memBackend,
		Model:          cfg.LLMModel,
		MaxToolIters:   cfg.ToolMaxIterations,
		StreamReplies:  cfg.StreamReplies,
		ToolTrace:      cfg.ToolTrace,
		Logger:         logger,
		Location:       tzLoc,
		TZName:         tzName,
		CoalesceSettle: time.Duration(cfg.CoalesceSettleMS) * time.Millisecond,
		SpinupNotice:   time.Duration(cfg.SpinupNoticeMS) * time.Millisecond,
		Consolidator:   consol,
		MCPManifest:    cfg.MCPManifest,
		Examples:       examplesSvc,
	}
	if selfStore != nil {
		agentOpts.SelfNotes = selfStore
	}
	ag, err := agent.New(agentOpts)
	if err != nil {
		logger.Error("agent init failed", "err", err)
		return 1
	}
	if selfStore != nil {
		// SELF.md sits in the persona prefix; reload after every agent write so
		// the note takes effect without waiting for a SIGHUP.
		selfStore.OnChange = func() {
			text, err := persona.Load(cfg.PersonaDir)
			if err != nil {
				logger.Error("persona reload after self-note failed", "err", err)
				return
			}
			ag.SetPersona(text)
			logger.Info("persona reloaded after self-note", "chars", len(text))
		}
	}
	go watchPersonaReload(ctx, cfg.PersonaDir, ag, logger)

	ch, err := newChannel(cfg, logger)
	if err != nil {
		logger.Error("channel init failed", "err", err)
		return 1
	}
	if errFwd != nil {
		if tg, ok := ch.(*telegram.Channel); ok {
			errFwd.SetSender(logfwd.SenderFunc(tg.NotifyHTML))
			logger.Info("telegram error reporting enabled", "level", cfg.TelegramErrorReporting)
		}
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
			Store:              cronStore,
			Handle:             handle,
			Pusher:             pusher,
			Interval:           time.Duration(cfg.CronTickSeconds) * time.Second,
			Logger:             logger,
			Recent:             sessions,
			SparkSkipRecent:    time.Duration(cfg.SparkSkipRecentMinutes) * time.Minute,
			ExamplesSkipRecent: time.Duration(cfg.ExamplesSkipRecentMinutes) * time.Minute,
			Examples:           examplesSvc,
		}
		if err := ensureSparkJobs(ctx, cfg, cronStore, logger, tzName); err != nil {
			logger.Error("spark ensure failed", "err", err)
			return 1
		}
		if err := ensureExamplesJobs(ctx, cfg, examplesSvc, logger); err != nil {
			logger.Error("examples ensure failed", "err", err)
			return 1
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
			ShowThinking:  cfg.ShowThinking,
		})
	case config.ChannelDiscord:
		return discord.New(discord.Config{
			Token:         cfg.DiscordBotToken,
			AllowedUsers:  cfg.DiscordAllowedUsers,
			Logger:        logger,
			StreamReplies: cfg.StreamReplies,
		})
	case config.ChannelSlack:
		return slack.New(slack.Config{
			BotToken:      cfg.SlackBotToken,
			AppToken:      cfg.SlackAppToken,
			AllowedUsers:  cfg.SlackAllowedUsers,
			Logger:        logger,
			StreamReplies: cfg.StreamReplies,
		})
	default:
		return nil, fmt.Errorf("unknown channel %q", cfg.Channel)
	}
}

// ensureSparkJobs installs opt-in spark-of-life cron jobs when SPARK_QTY is set.
// Telegram DMs use chat_id == user_id from the allowlist.
func ensureSparkJobs(ctx context.Context, cfg *config.Config, store *cron.Store, log *slog.Logger, tzName string) error {
	if strings.TrimSpace(cfg.SparkQty) == "" || store == nil {
		return nil
	}
	if strings.TrimSpace(tzName) == "" {
		tzName = cfg.CronTZ
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return err
	}
	when := fmt.Sprintf("%s@%02d-%02d", cfg.SparkQty, cfg.SparkStartHour, cfg.SparkEndHour)
	parsed, err := cron.ParseSparkSchedule(when, cfg.SparkStartHour, cfg.SparkEndHour, loc, time.Now())
	if err != nil {
		return fmt.Errorf("SPARK_QTY: %w", err)
	}
	prompt := strings.TrimSpace(cfg.SparkPrompt)
	if prompt == "" {
		prompt = cron.DefaultSparkPrompt
	}

	switch cfg.Channel {
	case config.ChannelTelegram:
		for _, uid := range cfg.TelegramAllowedUsers {
			if uid == 0 {
				continue
			}
			id := strconv.FormatInt(uid, 10)
			delivery := cron.Delivery{
				SessionID: fmt.Sprintf("telegram:%s:%s", id, id),
				UserID:    id,
				ChatID:    id,
			}
			job, created, err := store.EnsureSpark(ctx, prompt, parsed, delivery)
			if err != nil {
				return err
			}
			log.Info("spark job ready",
				"created", created,
				"id", job.ID,
				"session_id", delivery.SessionID,
				"next_run", job.NextRunAt.UTC().Format(time.RFC3339),
				"expr", job.Expr,
			)
		}
	default:
		log.Info("spark configured but auto-bind is telegram-only; schedule via cron_schedule repeat=spark",
			"channel", cfg.Channel, "qty", cfg.SparkQty)
	}
	return nil
}

// ensureExamplesJobs installs on-by-default capability-example pings when
// EXAMPLES_QTY is set (empty/"0" = off). Telegram DMs use chat_id == user_id.
func ensureExamplesJobs(ctx context.Context, cfg *config.Config, svc *examples.Service, log *slog.Logger) error {
	if svc == nil || !svc.ProactiveEnabled() {
		return nil
	}
	// Validate qty early so bad EXAMPLES_QTY fails boot clearly.
	if _, _, err := cron.ParseSparkQty(strings.TrimSpace(cfg.ExamplesQty)); err != nil {
		return fmt.Errorf("EXAMPLES_QTY: %w", err)
	}

	switch cfg.Channel {
	case config.ChannelTelegram:
		for _, uid := range cfg.TelegramAllowedUsers {
			if uid == 0 {
				continue
			}
			id := strconv.FormatInt(uid, 10)
			delivery := cron.Delivery{
				SessionID: fmt.Sprintf("telegram:%s:%s", id, id),
				UserID:    id,
				ChatID:    id,
			}
			job, created, err := svc.EnsureFor(ctx, delivery)
			if err != nil {
				return err
			}
			if job.ID == 0 {
				log.Info("examples skipped (session opted out)",
					"session_id", delivery.SessionID)
				continue
			}
			log.Info("examples job ready",
				"created", created,
				"id", job.ID,
				"session_id", delivery.SessionID,
				"next_run", job.NextRunAt.UTC().Format(time.RFC3339),
				"expr", job.Expr,
			)
		}
	default:
		log.Info("examples proactive auto-bind is telegram-only; use /examples on-demand",
			"channel", cfg.Channel, "qty", cfg.ExamplesQty)
	}
	return nil
}

// newLogger builds the process logger. When TELEGRAM_ERROR_REPORTING is
// error|warn, the returned *logfwd.Handler tees those records once SetSender
// is attached (after the Telegram channel is constructed).
func newLogger(level, errorReporting string) (*slog.Logger, *logfwd.Handler) {
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
	base := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lv})
	minLevel, enabled, err := logfwd.ParseLevel(errorReporting)
	if err != nil || !enabled {
		return slog.New(base), nil
	}
	fwd := logfwd.New(base, logfwd.Options{MinLevel: minLevel})
	return slog.New(fwd), fwd
}
