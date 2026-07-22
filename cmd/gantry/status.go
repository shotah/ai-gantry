package main

import (
	"fmt"
	"os"

	"github.com/shotah/ai-gantry/internal/heartbeat"
)

// status is the Docker healthcheck command: exit 0 when the heartbeat row is fresh.
func status() int {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/data"
	}
	if err := heartbeat.CheckFile(dataDir, heartbeat.DefaultMaxAge); err != nil {
		fmt.Fprintf(os.Stderr, "status: %v\n", err)
		return 1
	}
	return 0
}
