package slack

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	slackapi "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"

	"github.com/shotah/ai-gantry/internal/channel"
)

const maxPhotoBytes = 8 << 20 // 8 MiB

func filesFromMessageEvent(ev *slackevents.MessageEvent) []slackapi.File {
	if ev == nil || ev.Message == nil {
		return nil
	}
	return ev.Message.Files
}

func isImageFile(f slackapi.File) bool {
	if strings.HasPrefix(strings.ToLower(f.Mimetype), "image/") {
		return true
	}
	name := strings.ToLower(f.Name)
	if name == "" {
		name = strings.ToLower(f.Title)
	}
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".gif", ".webp"} {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	ft := strings.ToLower(f.Filetype)
	switch ft {
	case "jpg", "jpeg", "png", "gif", "webp":
		return true
	}
	return false
}

// inboundImages downloads image files as data: URLs for vision.
func inboundImages(ctx context.Context, api poster, files []slackapi.File) ([]channel.Image, error) {
	if len(files) == 0 {
		return nil, nil
	}
	var out []channel.Image
	for _, f := range files {
		if !isImageFile(f) {
			continue
		}
		url := f.URLPrivateDownload
		if url == "" {
			url = f.URLPrivate
		}
		if url == "" {
			return nil, fmt.Errorf("slack: file %s missing download url", f.ID)
		}
		var buf bytes.Buffer
		if err := api.GetFileContext(ctx, url, &buf); err != nil {
			return nil, fmt.Errorf("slack: download file: %w", err)
		}
		if buf.Len() > maxPhotoBytes {
			return nil, fmt.Errorf("slack: attachment exceeds %d bytes", maxPhotoBytes)
		}
		mime := f.Mimetype
		if mime == "" || !strings.HasPrefix(mime, "image/") {
			mime = "image/jpeg"
		}
		if i := strings.IndexByte(mime, ';'); i >= 0 {
			mime = strings.TrimSpace(mime[:i])
		}
		out = append(out, channel.Image{
			URL: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(buf.Bytes()),
		})
	}
	return out, nil
}

func (c *Channel) sendReply(ctx context.Context, api poster, channelID, threadTS, text, extraPhoto string) error {
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
			opts := []slackapi.MsgOption{slackapi.MsgOptionText(part, false)}
			if threadTS != "" {
				opts = append(opts, slackapi.MsgOptionTS(threadTS))
			}
			if _, _, err := api.PostMessageContext(ctx, channelID, opts...); err != nil {
				return err
			}
		}
	} else if len(urls) == 0 {
		return nil
	}

	for _, u := range urls {
		if err := sendImage(ctx, api, channelID, threadTS, u); err != nil {
			return err
		}
	}
	return nil
}

func sendImage(ctx context.Context, api poster, channelID, threadTS, u string) error {
	if strings.HasPrefix(u, "data:image/") {
		return sendDataImage(ctx, api, channelID, threadTS, u)
	}
	block := slackapi.NewImageBlock(u, "image", "", nil)
	opts := []slackapi.MsgOption{
		slackapi.MsgOptionBlocks(block),
		slackapi.MsgOptionText(u, false), // fallback text
	}
	if threadTS != "" {
		opts = append(opts, slackapi.MsgOptionTS(threadTS))
	}
	_, _, err := api.PostMessageContext(ctx, channelID, opts...)
	return err
}

func sendDataImage(ctx context.Context, api poster, channelID, threadTS, dataURL string) error {
	comma := strings.IndexByte(dataURL, ',')
	if comma < 0 {
		return fmt.Errorf("slack: bad data url")
	}
	meta := dataURL[len("data:"):comma]
	raw, err := base64.StdEncoding.DecodeString(dataURL[comma+1:])
	if err != nil {
		return fmt.Errorf("slack: decode data url: %w", err)
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
	_, err = api.UploadFileContext(ctx, slackapi.UploadFileParameters{
		Reader:          bytes.NewReader(raw),
		FileSize:        len(raw),
		Filename:        "image" + ext,
		Title:           "image",
		Channel:         channelID,
		ThreadTimestamp: threadTS,
	})
	return err
}
