package channel

import (
	"context"
	"strings"
	"sync"
)

type photoSinkKey struct{}

// PhotoSink collects outbound pictures produced during a Handle (MCP
// ImageContent). Mouths attach one to the Handle context and SendPhoto after
// the text reply. The model never sees these bytes.
type PhotoSink struct {
	mu   sync.Mutex
	urls []string
}

// NewPhotoSink returns an empty collector.
func NewPhotoSink() *PhotoSink {
	return &PhotoSink{}
}

// Add appends data: or https image URLs. Empty strings are ignored.
func (s *PhotoSink) Add(urls ...string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		s.urls = append(s.urls, u)
	}
}

// URLs returns a copy of collected photo URLs.
func (s *PhotoSink) URLs() []string {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.urls...)
}

// WithPhotoSink attaches s to ctx for MCP tool calls in this Handle.
func WithPhotoSink(ctx context.Context, s *PhotoSink) context.Context {
	if s == nil {
		return ctx
	}
	return context.WithValue(ctx, photoSinkKey{}, s)
}

// PhotoSinkFrom returns the collector attached to ctx, or nil.
func PhotoSinkFrom(ctx context.Context) *PhotoSink {
	s, _ := ctx.Value(photoSinkKey{}).(*PhotoSink)
	return s
}

// AttachPhotoSink puts a new collector on ctx. Mouths call this before Handle.
func AttachPhotoSink(ctx context.Context) (context.Context, *PhotoSink) {
	s := NewPhotoSink()
	return WithPhotoSink(ctx, s), s
}

// PhotoURLs is PhotoURL plus Photos, empty strings dropped.
func PhotoURLs(o Outbound) []string {
	var out []string
	if u := strings.TrimSpace(o.PhotoURL); u != "" {
		out = append(out, u)
	}
	for _, u := range o.Photos {
		if t := strings.TrimSpace(u); t != "" {
			out = append(out, t)
		}
	}
	return out
}
