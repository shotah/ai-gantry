//go:build unix

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/persona"
)

// watchPersonaReload reloads PERSONA_DIR into the agent on SIGHUP.
func watchPersonaReload(ctx context.Context, dir string, ag *agent.Agent, log *slog.Logger) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)
	defer signal.Stop(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			text, err := persona.Load(dir)
			if err != nil {
				log.Error("persona reload failed", "err", err)
				continue
			}
			ag.SetPersona(text)
			log.Info("persona reloaded", "chars", len(text))
		}
	}
}
