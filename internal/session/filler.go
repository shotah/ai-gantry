package session

import (
	"regexp"
	"strings"
	"unicode"
)

// KeepRecentUnstripped is how many trailing history messages stay verbatim
// in the prompt. Older messages lose a closed filler list. SQLite is unchanged.
const KeepRecentUnstripped = 5

// Filler list is a *subset* of IR stopwords, not NLTK's 179-word dump.
//
// Keep (SMLTAR "global" function words; Onix/SMART core): articles, light
// prepositions, be-verbs, and/or. Those glue sentences and rarely carry a bit.
//
// Add (Terse / LLMLingua "predictable padding"): longer empty hedges and a
// few empty phrases. Not please/thanks — those are politeness, i.e. tone.
//
// Refuse — NLTK includes these and they destroy meaning or voice
// (Widdows 2026 operator-geometry; sentiment/negation literature):
//
//	not/no/never/n't, just/like/oh/wait/so/well/really/literally,
//	this/that/these/those, pronouns, but/if, can/should/will,
//	more/less/very, have/has/had (main verb in "had a mortgage"),
//	up/down/out/off (phrasal verbs), do/does/did.
var fillerWords = []string{
	// articles
	"the", "a", "an",
	// prepositions (not particles)
	"of", "to", "in", "for", "on", "with", "at", "from", "by",
	// be-verbs
	"is", "are", "was", "were", "am", "be", "been", "being",
	// light conjunctions (not but/if)
	"and", "or",
	// longer hedges / politeness (the "big words")
	"actually", "basically", "essentially", "generally",
	"currently", "simply",
	// vocal fillers
	"um", "uh",
}

// Longer first so "kind of" wins over a later single-word pass.
var fillerPhrases = []string{
	"as well",
	"you know",
	"i mean",
	"kind of",
	"sort of",
}

var fillerRe = regexp.MustCompile(`(?i)\b(?:` + strings.Join(fillerWords, "|") + `)\b`)

var phraseRe = regexp.MustCompile(`(?i)\b(?:` + strings.Join(fillerPhrases, "|") + `)\b`)

var spaceRe = regexp.MustCompile(`[ \t]{2,}`)

// StripFillerHistory copies msgs and strips fillers from everything except
// the last KeepRecentUnstripped messages. Quoted spans are left intact.
func StripFillerHistory(msgs []Message) []Message {
	if len(msgs) <= KeepRecentUnstripped {
		return msgs
	}
	out := make([]Message, len(msgs))
	copy(out, msgs)
	cut := len(out) - KeepRecentUnstripped
	for i := 0; i < cut; i++ {
		out[i].Content = StripFillerWords(out[i].Content)
	}
	return out
}

// StripFillerWords removes filler phrases then fillerWords outside "...".
func StripFillerWords(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == '"' {
			end := strings.IndexByte(s[i+1:], '"')
			if end < 0 {
				b.WriteString(s[i:])
				break
			}
			b.WriteString(s[i : i+2+end])
			i += 2 + end
			continue
		}
		next := strings.IndexByte(s[i:], '"')
		chunk := s[i:]
		if next >= 0 {
			chunk = s[i : i+next]
		}
		chunk = phraseRe.ReplaceAllString(chunk, " ")
		b.WriteString(fillerRe.ReplaceAllString(chunk, " "))
		if next < 0 {
			break
		}
		i += next
	}
	out := spaceRe.ReplaceAllString(b.String(), " ")
	return strings.TrimSpace(collapseSpaceAroundPunct(out))
}

func collapseSpaceAroundPunct(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if prevSpace {
				continue
			}
			prevSpace = true
			b.WriteByte(' ')
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return b.String()
}
