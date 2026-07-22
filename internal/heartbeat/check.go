package heartbeat

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

// CheckFile opens dataDir/gantry.db read-only and verifies the heartbeat is fresh.
// Used by `gantry status` without loading the full agent config surface.
func CheckFile(dataDir string, maxAge time.Duration) error {
	if dataDir == "" {
		dataDir = "/data"
	}
	dbPath := filepath.Join(dataDir, "gantry.db")
	if _, err := os.Stat(dbPath); err != nil {
		return fmt.Errorf("heartbeat: open %s: %w", dbPath, err)
	}
	// read-only so the healthcheck never writes
	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return fmt.Errorf("heartbeat: open db: %w", err)
	}
	defer func() { _ = db.Close() }()

	s := &Store{db: db}
	return s.Check(context.Background(), maxAge)
}
