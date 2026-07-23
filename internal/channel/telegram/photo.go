package telegram

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/shotah/ai-gantry/internal/channel"
)

const (
	maxPhotoBytes      = 8 << 20 // 8 MiB — keep vision payloads bounded
	telegramCaptionMax = 1024
)

// downloadPhotoAsDataURL fetches the largest Telegram photo as a data: URL for vision APIs.
func downloadPhotoAsDataURL(ctx context.Context, b *bot.Bot, photos []models.PhotoSize) (string, error) {
	if len(photos) == 0 {
		return "", fmt.Errorf("telegram: empty photo")
	}
	best := photos[0]
	for _, p := range photos[1:] {
		if p.FileSize > best.FileSize || p.Width*p.Height > best.Width*best.Height {
			best = p
		}
	}
	file, err := b.GetFile(ctx, &bot.GetFileParams{FileID: best.FileID})
	if err != nil {
		return "", fmt.Errorf("telegram: getFile: %w", err)
	}
	link := b.FileDownloadLink(file)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("telegram: download photo: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("telegram: download photo: HTTP %d", resp.StatusCode)
	}
	limited := io.LimitReader(resp.Body, maxPhotoBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("telegram: read photo: %w", err)
	}
	if len(data) > maxPhotoBytes {
		return "", fmt.Errorf("telegram: photo exceeds %d bytes", maxPhotoBytes)
	}
	mime := resp.Header.Get("Content-Type")
	if mime == "" || !strings.HasPrefix(mime, "image/") {
		mime = "image/jpeg"
	}
	// Strip parameters like "image/jpeg; charset=…"
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = strings.TrimSpace(mime[:i])
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func clipCaption(s string) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= telegramCaptionMax {
		return s
	}
	r := []rune(s)
	return string(r[:telegramCaptionMax-1]) + "…"
}

// sendReply sends text and any image URLs found in it (or explicit PhotoURL).
func (c *Channel) sendReply(ctx context.Context, b *bot.Bot, chatID int64, threadID int, text string, extraPhoto string) error {
	urls, rest := channel.ExtractImageURLs(text)
	if extra := strings.TrimSpace(extraPhoto); extra != "" {
		urls = append([]string{extra}, urls...)
	}
	// Dedupe while preserving order.
	seen := map[string]struct{}{}
	deduped := urls[:0]
	for _, u := range urls {
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		deduped = append(deduped, u)
	}
	urls = deduped

	caption := clipCaption(rest)
	for i, u := range urls {
		photoCaption := ""
		if i == 0 {
			photoCaption = caption
			caption = "" // only first photo gets the text caption
		}
		sent, err := b.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Photo:           &models.InputFileString{Data: u},
			Caption:         photoCaption,
		})
		if err != nil {
			return fmt.Errorf("telegram: sendPhoto: %w", err)
		}
		if sent != nil {
			label := photoCaption
			if label == "" {
				label = "[photo]"
			}
			c.outbound.remember(chatID, sent.ID, threadID, label)
		}
	}
	if caption != "" || (len(urls) == 0 && rest != "") {
		body := rest
		if body == "" {
			body = caption
		}
		return c.sendChunks(ctx, b, chatID, threadID, body)
	}
	return nil
}

func inboundImages(ctx context.Context, b *bot.Bot, msg *models.Message) ([]channel.Image, error) {
	if msg == nil || len(msg.Photo) == 0 {
		return nil, nil
	}
	dataURL, err := downloadPhotoAsDataURL(ctx, b, msg.Photo)
	if err != nil {
		return nil, err
	}
	return []channel.Image{{URL: dataURL}}, nil
}
