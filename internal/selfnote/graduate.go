package selfnote

import (
	"strings"

	"github.com/shotah/ai-gantry/internal/session"
)

const graduateNoteMax = 200

// GraduateVoice appends new Voice: bits from a history fold into SELF.md.
// No Completer call — the fold already decided what was tonal. Skips when
// Voice was copied forward, the only addition is mood weather, the bit is
// already in SELF.md, or the file is at the 4 KB cap (Append refuses).
// Returns true when a line was written.
func GraduateVoice(s *Store, prior, next string) (bool, error) {
	if s == nil {
		return false, nil
	}
	delta := session.VoiceDelta(prior, next)
	if delta == "" {
		return false, nil
	}
	cur, err := s.Read()
	if err != nil {
		return false, err
	}
	note := bitsNotInSelf(delta, cur)
	if note == "" {
		return false, nil
	}
	if err := s.Append(clipGraduateNote(note)); err != nil {
		return false, err
	}
	return true, nil
}

func bitsNotInSelf(delta, self string) string {
	selfLower := strings.ToLower(self)
	var keep []string
	for _, b := range session.VoiceBits(delta) {
		if voiceAlreadyIn(selfLower, b) {
			continue
		}
		keep = append(keep, b)
	}
	return strings.Join(keep, "; ")
}

func voiceAlreadyIn(selfLower, bit string) bool {
	bit = strings.TrimSpace(bit)
	if bit == "" {
		return true
	}
	if strings.Contains(selfLower, strings.ToLower(bit)) {
		return true
	}
	if q := quotedSpan(bit); q != "" && strings.Contains(selfLower, strings.ToLower(q)) {
		return true
	}
	return false
}

func quotedSpan(s string) string {
	i := strings.IndexByte(s, '"')
	if i < 0 {
		return ""
	}
	j := strings.IndexByte(s[i+1:], '"')
	if j < 0 {
		return ""
	}
	return s[i+1 : i+1+j]
}

func clipGraduateNote(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= graduateNoteMax {
		return s
	}
	return s[:graduateNoteMax-1] + "…"
}
