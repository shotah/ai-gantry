package telegram

import (
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	outboundCacheCap = 256
	reactionClipMax  = 200
)

type outboundKey struct {
	chatID int64
	msgID  int
}

type outboundEntry struct {
	text     string
	threadID int
}

// outboundCache remembers recent bot messages so reactions can cite them.
type outboundCache struct {
	mu    sync.Mutex
	cap   int
	order []outboundKey
	byKey map[outboundKey]outboundEntry
}

func newOutboundCache(maxEntries int) *outboundCache {
	if maxEntries < 1 {
		maxEntries = outboundCacheCap
	}
	return &outboundCache{
		cap:   maxEntries,
		byKey: make(map[outboundKey]outboundEntry, maxEntries),
	}
}

func (c *outboundCache) remember(chatID int64, msgID, threadID int, text string) {
	if c == nil || msgID == 0 {
		return
	}
	key := outboundKey{chatID: chatID, msgID: msgID}
	entry := outboundEntry{text: text, threadID: threadID}

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.byKey[key]; ok {
		c.byKey[key] = entry
		return
	}
	for len(c.order) >= c.cap && len(c.order) > 0 {
		old := c.order[0]
		c.order = c.order[1:]
		delete(c.byKey, old)
	}
	c.byKey[key] = entry
	c.order = append(c.order, key)
}

func (c *outboundCache) lookup(chatID int64, msgID int) (outboundEntry, bool) {
	if c == nil {
		return outboundEntry{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.byKey[outboundKey{chatID: chatID, msgID: msgID}]
	return e, ok
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
