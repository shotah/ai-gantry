// Package stdio implements a REPL channel for local development.
package stdio

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/shotah/ai-gantry/internal/channel"
)

const (
	sessionID = "stdio"
	userID    = "local"
)

// Channel is an interactive line-oriented REPL on stdin/stdout.
type Channel struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// New returns a Channel bound to process stdio.
func New() *Channel {
	return &Channel{
		In:  os.Stdin,
		Out: os.Stdout,
		Err: os.Stderr,
	}
}

// Run reads lines, invokes handle, and prints replies until EOF or ctx cancel.
func (c *Channel) Run(ctx context.Context, handle channel.Handler) error {
	in := c.In
	if in == nil {
		in = os.Stdin
	}
	out := c.Out
	if out == nil {
		out = os.Stdout
	}
	errOut := c.Err
	if errOut == nil {
		errOut = os.Stderr
	}

	_, _ = fmt.Fprintln(errOut, "gantry stdio ready — type a message, /new to reset, /quit to exit")

	scanner := bufio.NewScanner(in)
	// Allow long paste dumps in the REPL.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for {
		if err := ctx.Err(); err != nil {
			return nil
		}

		_, _ = fmt.Fprint(out, "> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("stdio: read: %w", err)
			}
			_, _ = fmt.Fprintln(out)
			return nil // EOF
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		switch strings.ToLower(line) {
		case "/quit", "/exit", "/q":
			return nil
		}

		reply, err := handle(ctx, channel.Message{
			SessionID: sessionID,
			UserID:    userID,
			Text:      line,
		})
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			_, _ = fmt.Fprintf(errOut, "error: %v\n", err)
			continue
		}
		if reply != "" {
			_, _ = fmt.Fprintln(out, reply)
		}
	}
}
