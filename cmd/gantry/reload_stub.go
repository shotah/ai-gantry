//go:build !unix

package main

import (
	"context"
	"log/slog"

	"github.com/shotah/ai-gantry/internal/agent"
)

// watchPersonaReload is a no-op on non-unix (no SIGHUP).
func watchPersonaReload(context.Context, string, *agent.Agent, *slog.Logger) {}
