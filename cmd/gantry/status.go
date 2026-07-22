package main

import (
	"fmt"
	"os"
)

// status is the Docker healthcheck command.
// Milestone 5 will read a heartbeat row from SQLite; until then, report not ready.
func status() int {
	fmt.Fprintln(os.Stderr, "status: heartbeat not implemented yet")
	return 1
}
