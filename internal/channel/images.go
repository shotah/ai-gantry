package channel

import (
	"regexp"
	"strings"
)

var (
	mdImageRe = regexp.MustCompile(`!\[[^\]]*]\((https?://[^)\s]+)\)`)
	bareURLRe = regexp.MustCompile(`https?://[^\s<>"']+`)
)

// ExtractImageURLs pulls markdown images and bare http(s) image URLs out of text.
// Remaining text is returned with those URLs removed.
func ExtractImageURLs(text string) (urls []string, rest string) {
	seen := map[string]struct{}{}
	add := func(u string) {
		u = strings.TrimRight(u, ".,);]")
		if u == "" {
			return
		}
		if _, ok := seen[u]; ok {
			return
		}
		if !LooksLikeImageURL(u) {
			return
		}
		seen[u] = struct{}{}
		urls = append(urls, u)
	}

	rest = mdImageRe.ReplaceAllStringFunc(text, func(m string) string {
		sub := mdImageRe.FindStringSubmatch(m)
		if len(sub) == 2 {
			u := strings.TrimRight(sub[1], ".,);]")
			if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
				if _, ok := seen[u]; !ok {
					seen[u] = struct{}{}
					urls = append(urls, u)
				}
			}
		}
		return ""
	})
	rest = bareURLRe.ReplaceAllStringFunc(rest, func(u string) string {
		if LooksLikeImageURL(u) {
			add(u)
			return ""
		}
		return u
	})
	rest = strings.TrimSpace(rest)
	return urls, rest
}

// LooksLikeImageURL reports whether u looks like an image (http(s) with image
// extension, or a data:image/… URL).
func LooksLikeImageURL(u string) bool {
	lower := strings.ToLower(strings.TrimRight(u, ".,);]"))
	if strings.HasPrefix(lower, "data:image/") {
		return true
	}
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return false
	}
	path := lower
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".gif", ".webp"} {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}
