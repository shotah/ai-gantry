package pendant

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif" // register GIF decoder for oversize mailbox shrink
	"image/jpeg"
	_ "image/png" // register PNG decoder for oversize mailbox shrink
	"strings"

	"github.com/shotah/ai-gantry/internal/channel"
)

// Mailbox photo caps — gantry-pendant lib/mailbox/caps.ts. A data URL over
// IMAGE_BYTES_MAX refuses the whole frame (caption included), so we shrink
// or drop the picture and keep the text.
const (
	pendantImageMax       = 1
	pendantImageBytesMax  = 1_500_000
	pendantJPEGBytesMax   = ((pendantImageBytesMax - 32) / 4) * 3 // 1_124_976
	pendantJPEGDataPrefix = "data:image/jpeg;base64,"
)

func replyFrames(kind, userID, id, text string, extra ...string) []outboundFrame {
	urls, rest := channel.MergePhotoURLs(text, extra...)
	imgs := outboundImages(urls)
	if strings.TrimSpace(rest) == "" && len(imgs) == 0 {
		return nil
	}
	first := outboundFrame{Kind: kind, UserID: userID, ID: id, Text: rest}
	if len(imgs) > 0 {
		n := pendantImageMax
		if n > len(imgs) {
			n = len(imgs)
		}
		first.Images = imgs[:n]
		imgs = imgs[n:]
	}
	frames := []outboundFrame{first}
	for _, img := range imgs {
		frames = append(frames, outboundFrame{
			Kind:   kind,
			UserID: userID,
			Images: []channel.Image{img},
		})
	}
	return frames
}

func outboundImages(urls []string) []channel.Image {
	var out []channel.Image
	for _, u := range urls {
		if fitted, ok := fitPendantPhoto(u); ok {
			out = append(out, channel.Image{URL: fitted})
		}
	}
	return out
}

func fitPendantPhoto(u string) (string, bool) {
	u = strings.TrimSpace(u)
	if u == "" {
		return "", false
	}
	if strings.HasPrefix(u, "https://") {
		return u, true
	}
	if !strings.HasPrefix(u, "data:image/") {
		return "", false
	}
	if len(u) <= pendantImageBytesMax {
		return u, true
	}
	raw, _, _, err := channel.DecodeDataURL(u)
	if err != nil {
		return "", false
	}
	jpg, err := jpegUnderBudget(raw)
	if err != nil {
		return "", false
	}
	out := pendantJPEGDataPrefix + base64.StdEncoding.EncodeToString(jpg)
	if len(out) > pendantImageBytesMax {
		return "", false
	}
	return out, true
}

func jpegUnderBudget(raw []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	for _, q := range []int{90, 80, 70, 60, 50, 40} {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: q}); err != nil {
			return nil, err
		}
		if buf.Len() <= pendantJPEGBytesMax {
			return buf.Bytes(), nil
		}
	}
	return nil, fmt.Errorf("pendant: photo exceeds mailbox budget")
}
