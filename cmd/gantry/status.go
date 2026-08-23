package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/shotah/ai-gantry/internal/doctor"
)

// status is the Docker healthcheck command: exit 0 when the heartbeat row is
// fresh. Operator JSON (channel, MCP connected vs skipped, persona files)
// is printed on stdout. Exit code is liveness only — an all-skipped manifest
// is operator-unhealthy (`ok:false`) but must not restart a chat-only crane.
func status() int {
	rep := doctor.Collect(doctor.PathsFromEnv())
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(rep); err != nil {
		fmt.Fprintf(os.Stderr, "status: encode: %v\n", err)
		return 1
	}
	if !rep.Alive {
		if rep.Reason != "" {
			fmt.Fprintf(os.Stderr, "status: %s\n", rep.Reason)
		} else {
			fmt.Fprintf(os.Stderr, "status: unhealthy\n")
		}
		return 1
	}
	return 0
}
