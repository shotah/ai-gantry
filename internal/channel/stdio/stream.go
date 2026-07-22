package stdio

import (
	"context"
	"fmt"
	"io"
	"os"
)

// printStream writes progressive tokens to stdout (logs stay on stderr).
type printStream struct {
	out     io.Writer
	sent    string
	started bool
}

func newPrintStream(out io.Writer) *printStream {
	if out == nil {
		out = os.Stdout
	}
	return &printStream{out: out}
}

func (s *printStream) Started() bool { return s.started }

func (s *printStream) Update(_ context.Context, fullText string) error {
	s.started = true
	if len(fullText) < len(s.sent) {
		// model rewound; reprint full line
		_, err := fmt.Fprintf(s.out, "\r%s", fullText)
		s.sent = fullText
		return err
	}
	suffix := fullText[len(s.sent):]
	s.sent = fullText
	_, err := fmt.Fprint(s.out, suffix)
	return err
}

func (s *printStream) Finish(_ context.Context, final string) error {
	if !s.started {
		return nil
	}
	if final != "" && final != s.sent {
		if err := s.Update(context.Background(), final); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(s.out)
	return err
}
