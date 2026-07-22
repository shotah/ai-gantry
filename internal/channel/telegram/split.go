package telegram

import "unicode/utf8"

const telegramMaxMessageRunes = 4096

// splitMessage splits text into Telegram-safe chunks (≤4096 Unicode code points).
// Prefers breaking on newlines, then spaces, then hard-cuts on rune boundaries.
func splitMessage(text string, maxRunes int) []string {
	if maxRunes < 1 {
		maxRunes = telegramMaxMessageRunes
	}
	if text == "" {
		return nil
	}
	if utf8.RuneCountInString(text) <= maxRunes {
		return []string{text}
	}

	runes := []rune(text)
	var parts []string
	for len(runes) > 0 {
		if len(runes) <= maxRunes {
			parts = append(parts, string(runes))
			break
		}
		cut := maxRunes
		window := runes[:maxRunes]
		if i := lastIndexRune(window, '\n'); i >= maxRunes/4 {
			cut = i + 1
		} else if i := lastIndexRune(window, ' '); i >= maxRunes/4 {
			cut = i + 1
		}
		parts = append(parts, string(runes[:cut]))
		runes = runes[cut:]
	}
	return parts
}

func lastIndexRune(runes []rune, target rune) int {
	for i := len(runes) - 1; i >= 0; i-- {
		if runes[i] == target {
			return i
		}
	}
	return -1
}
