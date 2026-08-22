// Package selfnote persists SELF.md — personality notes the agent maintains
// about itself (voice, humor, running jokes, rituals) so who it has become
// survives /new and history trimming. SELF.md is the only persona file the
// agent may write; PERSONA.md stays operator-only.
package selfnote

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"
)

// FileName is the agent-writable persona file under PERSONA_DIR.
const FileName = "SELF.md"

// MaxChars bounds SELF.md (~1k tokens) so grown personality cannot crowd the
// prompt. The file lives in the cacheable persona prefix, so within the cap it
// costs nothing per turn — only a one-off prefill when it changes.
const MaxChars = 4096

// Header opens a fresh SELF.md so the section is self-describing inside the
// concatenated persona (and to a human reading the file). The append-only
// warning matters: models otherwise treat self_note like a rewrite and spam
// near-duplicate bullets until /new distill can merge them.
const Header = "# SELF.md — Who You Are Becoming\n\n" +
	"> Agent-written file. Cap ~4KB; the operator may prune any line.\n" +
	">\n" +
	"> **`self_note` APPENDS one new `-` line — it does NOT overwrite this file.**\n" +
	"> Do not re-note something already listed below. Skip if the vibe or north-star is already here.\n" +
	"> A few north-star aims (how you show up for months) belong here. Progress logs are memory, not this file.\n" +
	"> Distill on `/new` merges. Keep exact jokes and nicknames. A vibe word\n" +
	"> (\"dry\") is not a substitute for a quote."

// Store reads and writes SELF.md. Safe for concurrent use.
type Store struct {
	mu   sync.Mutex
	path string

	// OnChange runs after every successful write — the persona reload hook.
	// Set once at boot, before any turn can reach the store.
	OnChange func()
}

// Open returns a Store for dir/SELF.md after verifying it is writable —
// persona directories are often read-only mounts in containers, in which
// case the feature must switch off instead of failing at first use.
func Open(dir string) (*Store, error) {
	path := filepath.Join(dir, FileName)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("selfnote: open %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("selfnote: close %s: %w", path, err)
	}
	s := &Store{path: path}
	if err := s.persistStamp(); err != nil {
		return nil, err
	}
	return s, nil
}

// persistStamp rewrites the kernel header if the file is stale or empty.
func (s *Store) persistStamp() error {
	cur, err := s.readRaw()
	if err != nil {
		return err
	}
	next := Stamp(cur)
	if cur == next {
		return nil
	}
	if err := os.WriteFile(s.path, []byte(next+"\n"), 0o644); err != nil {
		return fmt.Errorf("selfnote: stamp %s: %w", s.path, err)
	}
	return nil
}

// Read returns the current content ("" when the file is absent or empty).
func (s *Store) Read() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readLocked()
}

func (s *Store) readLocked() (string, error) {
	raw, err := s.readRaw()
	if err != nil {
		return "", err
	}
	return Stamp(raw), nil
}

func (s *Store) readRaw() (string, error) {
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("selfnote: read %s: %w", s.path, err)
	}
	return strings.TrimSpace(string(b)), nil
}

// Append adds one note line (newlines flattened), refusing at capacity so the
// model gets a clear signal instead of silently losing notes.
func (s *Store) Append(note string) error {
	note = strings.Join(strings.Fields(note), " ")
	if note == "" {
		return fmt.Errorf("selfnote: empty note")
	}
	s.mu.Lock()
	cur, err := s.readLocked()
	if err != nil {
		s.mu.Unlock()
		return err
	}
	next := Stamp(cur) + "\n- " + note
	if len(next) > MaxChars {
		s.mu.Unlock()
		return fmt.Errorf("selfnote: %s is full (%d/%d chars) — it is rewritten compactly on /new, or the operator can prune it", FileName, len(cur), MaxChars)
	}
	err = os.WriteFile(s.path, []byte(next+"\n"), 0o644)
	s.mu.Unlock()
	if err != nil {
		return fmt.Errorf("selfnote: write %s: %w", s.path, err)
	}
	s.changed()
	return nil
}

// Write replaces the whole file (the distill-on-reset path), clipping to
// MaxChars on a rune boundary. Empty content is refused — a distill that
// produced nothing must not erase an existing personality.
func (s *Store) Write(content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("selfnote: refusing to write empty %s", FileName)
	}
	content = Stamp(content)
	if len(content) > MaxChars {
		cut := MaxChars
		for cut > 0 && !utf8.RuneStart(content[cut]) {
			cut--
		}
		content = strings.TrimSpace(content[:cut])
	}
	s.mu.Lock()
	err := os.WriteFile(s.path, []byte(content+"\n"), 0o644)
	s.mu.Unlock()
	if err != nil {
		return fmt.Errorf("selfnote: write %s: %w", s.path, err)
	}
	s.changed()
	return nil
}

func (s *Store) changed() {
	if s.OnChange != nil {
		s.OnChange()
	}
}
