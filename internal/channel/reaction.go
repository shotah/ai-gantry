package channel

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Palette is what the agent may react with — yes, no, sad, unsure included,
// so a reaction can disagree, not only nod. Every entry is in the standard
// set Telegram bots may set; the phone picker shows the same list. The
// model is told this list.
var Palette = []string{"👍", "👎", "❤️", "🔥", "🤣", "😢", "🤔", "🙏", "👀", "🎉", "💯", "👏"}

// InPalette reports whether emoji is one the agent may react with.
func InPalette(emoji string) bool {
	emoji = strings.TrimSpace(emoji)
	for _, p := range Palette {
		if emoji == p || emoji == strings.TrimSuffix(p, "\ufe0f") {
			return true
		}
	}
	return false
}

// ReactionSink carries the agent's reaction for this turn from the kernel to
// the mouth, the way PhotoSink carries image URLs. A mouth attaches one only
// if it can put an emoji on the human's message; the kernel reads its
// absence as "send the emoji as text instead".
type ReactionSink struct {
	mu    sync.Mutex
	emoji string
}

type reactionSinkKey struct{}

// AttachReactionSink puts a new sink on ctx. Mouths that can react call this
// before Handle.
func AttachReactionSink(ctx context.Context) (context.Context, *ReactionSink) {
	s := &ReactionSink{}
	return context.WithValue(ctx, reactionSinkKey{}, s), s
}

// ReactionSinkFrom returns the sink on ctx, nil when the mouth cannot react.
func ReactionSinkFrom(ctx context.Context) *ReactionSink {
	s, _ := ctx.Value(reactionSinkKey{}).(*ReactionSink)
	return s
}

// Set records the reaction. Nil-safe.
func (s *ReactionSink) Set(emoji string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.emoji = strings.TrimSpace(emoji)
	s.mu.Unlock()
}

// Emoji is the recorded reaction, "" when none. Nil-safe.
func (s *ReactionSink) Emoji() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.emoji
}

// ReactionLinePrefix opens the user turn a mouth synthesizes when the human
// reacts to an agent message: "[reaction] 👍 on: <agent text>".
const ReactionLinePrefix = "[reaction]"

const reactionClipMax = 200

// FormatReaction is the user turn for a settled reaction set on target.
func FormatReaction(emojis []string, target string) string {
	return fmt.Sprintf("%s %s on: %s", ReactionLinePrefix, strings.Join(emojis, " "), clipReactionTarget(target))
}

// ParseReaction reads a FormatReaction line back: the emoji set and the
// target text. ok is false for any other user turn.
func ParseReaction(text string) (emojis []string, target string, ok bool) {
	t := strings.TrimSpace(text)
	if !strings.HasPrefix(t, ReactionLinePrefix+" ") {
		return nil, "", false
	}
	rest := strings.TrimSpace(t[len(ReactionLinePrefix):])
	set, tgt := rest, ""
	if i := strings.Index(rest, " on: "); i >= 0 {
		set, tgt = rest[:i], strings.TrimSpace(rest[i+len(" on: "):])
	}
	emojis = strings.Fields(set)
	if len(emojis) == 0 {
		return nil, "", false
	}
	return emojis, tgt, true
}

func clipReactionTarget(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "(unknown message)"
	}
	if utf8.RuneCountInString(s) <= reactionClipMax {
		return s
	}
	r := []rune(s)
	return string(r[:reactionClipMax-1]) + "…"
}

// positiveReactions are the human's reactions that change nothing: when the
// agent is not waiting on them, these are recorded without a model call.
var positiveReactions = map[string]struct{}{
	"👍": {}, "❤️": {}, "❤": {}, "🔥": {}, "😂": {}, "🙏": {}, "👏": {}, "💯": {},
	"✅": {}, "🎉": {}, "😍": {}, "🤣": {}, "👀": {}, "🥰": {}, "😊": {}, "💪": {},
}

// AllPositive reports whether every emoji in the set is a plain
// acknowledgment. A 👎, a ❓, a custom emoji, or an empty set is not.
func AllPositive(emojis []string) bool {
	if len(emojis) == 0 {
		return false
	}
	for _, e := range emojis {
		if _, ok := positiveReactions[strings.TrimSpace(e)]; !ok {
			return false
		}
	}
	return true
}

// Settler debounces a human changing their mind on a reaction — heart,
// thumbs-up, smile is one turn, not three. Per key the latest set wins after
// a quiet period; an empty set (reaction cleared) cancels.
type Settler struct {
	mu      sync.Mutex
	pending map[string]*settling
}

type settling struct {
	gen   int
	timer *time.Timer
}

// NewSettler returns an empty settler.
func NewSettler() *Settler {
	return &Settler{pending: make(map[string]*settling)}
}

// Schedule (re)arms key: fire runs with emojis once after quiet time unless
// a later Schedule for the same key supersedes or cancels it. Nil-safe.
func (s *Settler) Schedule(key string, emojis []string, after time.Duration, fire func([]string)) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pending[key]
	if ok && p.timer != nil {
		p.timer.Stop()
	}
	if len(emojis) == 0 {
		delete(s.pending, key)
		return
	}
	if !ok {
		p = &settling{}
		s.pending[key] = p
	}
	p.gen++
	gen := p.gen
	set := append([]string(nil), emojis...)
	p.timer = time.AfterFunc(after, func() {
		s.mu.Lock()
		cur, ok := s.pending[key]
		if !ok || cur.gen != gen {
			s.mu.Unlock()
			return
		}
		delete(s.pending, key)
		s.mu.Unlock()
		fire(set)
	})
}

// Recent remembers the last texts the mouth sent, by frame id, so a
// reaction on one of them can say what it was on. Bounded; oldest evicted.
type Recent struct {
	mu    sync.Mutex
	cap   int
	order []string
	text  map[string]string
}

// NewRecent returns a ring holding at most n entries.
func NewRecent(n int) *Recent {
	if n < 1 {
		n = 1
	}
	return &Recent{cap: n, text: make(map[string]string, n)}
}

// Remember stores text under id. Empty id or text is ignored. Nil-safe.
func (r *Recent) Remember(id, text string) {
	id, text = strings.TrimSpace(id), strings.TrimSpace(text)
	if r == nil || id == "" || text == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.text[id]; !ok {
		r.order = append(r.order, id)
		for len(r.order) > r.cap {
			delete(r.text, r.order[0])
			r.order = r.order[1:]
		}
	}
	r.text[id] = text
}

// Lookup returns the text sent under id. Nil-safe.
func (r *Recent) Lookup(id string) (string, bool) {
	if r == nil {
		return "", false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.text[strings.TrimSpace(id)]
	return t, ok
}
