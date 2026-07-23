package discord

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/shotah/ai-gantry/internal/channel"
)

const maxPhotoBytes = 8 << 20 // 8 MiB

// inboundImages downloads image attachments as data: URLs for vision.
func inboundImages(ctx context.Context, m *discordgo.Message) ([]channel.Image, error) {
	if m == nil || len(m.Attachments) == 0 {
		return nil, nil
	}
	var out []channel.Image
	for _, a := range m.Attachments {
		if !isImageAttachment(a) {
			continue
		}
		dataURL, err := downloadURLAsDataURL(ctx, a.URL, a.ContentType)
		if err != nil {
			return nil, err
		}
		out = append(out, channel.Image{URL: dataURL})
	}
	return out, nil
}

func isImageAttachment(a *discordgo.MessageAttachment) bool {
	if a == nil {
		return false
	}
	if strings.HasPrefix(strings.ToLower(a.ContentType), "image/") {
		return true
	}
	if channel.LooksLikeImageURL(a.URL) {
		return true
	}
	name := strings.ToLower(a.Filename)
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".gif", ".webp"} {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

func downloadURLAsDataURL(ctx context.Context, rawURL, contentType string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("discord: download attachment: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("discord: download attachment: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxPhotoBytes+1))
	if err != nil {
		return "", fmt.Errorf("discord: read attachment: %w", err)
	}
	if len(data) > maxPhotoBytes {
		return "", fmt.Errorf("discord: attachment exceeds %d bytes", maxPhotoBytes)
	}
	mime := contentType
	if mime == "" {
		mime = resp.Header.Get("Content-Type")
	}
	if mime == "" || !strings.HasPrefix(mime, "image/") {
		mime = "image/jpeg"
	}
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = strings.TrimSpace(mime[:i])
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func (c *Channel) sendReply(ctx context.Context, s session, channelID, text, extraPhoto string) error {
	urls, rest := channel.ExtractImageURLs(text)
	if extra := strings.TrimSpace(extraPhoto); extra != "" {
		urls = append([]string{extra}, urls...)
	}
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

	if rest != "" {
		parts := splitMessage(rest, c.chunkMax)
		for i, part := range parts {
			if i > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(chunkPause):
				}
			}
			if _, err := s.ChannelMessageSend(channelID, part); err != nil {
				return err
			}
		}
	} else if len(urls) == 0 {
		return nil
	}

	for _, u := range urls {
		if err := sendImage(s, channelID, u); err != nil {
			return err
		}
	}
	return nil
}

func sendImage(s session, channelID, u string) error {
	if strings.HasPrefix(u, "data:image/") {
		return sendDataImage(s, channelID, u)
	}
	_, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{{
			Image: &discordgo.MessageEmbedImage{URL: u},
		}},
	})
	return err
}

func sendDataImage(s session, channelID, dataURL string) error {
	comma := strings.IndexByte(dataURL, ',')
	if comma < 0 {
		return fmt.Errorf("discord: bad data url")
	}
	meta := dataURL[len("data:"):comma]
	raw, err := base64.StdEncoding.DecodeString(dataURL[comma+1:])
	if err != nil {
		return fmt.Errorf("discord: decode data url: %w", err)
	}
	mime := meta
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = mime[:i]
	}
	ext := ".bin"
	switch mime {
	case "image/png":
		ext = ".png"
	case "image/jpeg", "image/jpg":
		ext = ".jpg"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	}
	_, err = s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Files: []*discordgo.File{{
			Name:   "image" + ext,
			Reader: bytes.NewReader(raw),
		}},
	})
	return err
}
