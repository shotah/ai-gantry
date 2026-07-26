package telegram

import (
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"
)

const inboundClipMax = 200

// composeInboundText turns Telegram message extras into tagged text for the agent.
// Photos are handled separately (vision). Video/GIF/voice are intentionally skipped.
func composeInboundText(msg *models.Message) string {
	if msg == nil {
		return ""
	}
	var parts []string
	if tag := forwardTag(msg.ForwardOrigin); tag != "" {
		parts = append(parts, tag)
	}
	if tag := replyTag(msg.ReplyToMessage); tag != "" {
		parts = append(parts, tag)
	}
	if msg.Venue != nil {
		parts = append(parts, formatVenue(msg.Venue))
	} else if msg.Location != nil {
		parts = append(parts, formatLocation(msg.Location))
	}
	if msg.Contact != nil {
		parts = append(parts, formatContact(msg.Contact))
	}
	if msg.Document != nil {
		parts = append(parts, formatDocument(msg.Document))
	}
	if msg.Sticker != nil {
		if tag := formatSticker(msg.Sticker); tag != "" {
			parts = append(parts, tag)
		}
	}
	body := strings.TrimSpace(msg.Text)
	if body == "" {
		body = strings.TrimSpace(msg.Caption)
	}
	if body != "" {
		parts = append(parts, body)
	}
	return strings.Join(parts, "\n")
}

func formatLocation(loc *models.Location) string {
	if loc == nil {
		return ""
	}
	s := fmt.Sprintf("[location] lat=%.6f lon=%.6f", loc.Latitude, loc.Longitude)
	if loc.LivePeriod > 0 {
		s += fmt.Sprintf(" live_period=%ds", loc.LivePeriod)
	}
	if loc.Heading > 0 {
		s += fmt.Sprintf(" heading=%d", loc.Heading)
	}
	return s
}

func formatVenue(v *models.Venue) string {
	if v == nil {
		return ""
	}
	title := strings.TrimSpace(v.Title)
	addr := strings.TrimSpace(v.Address)
	s := "[venue]"
	if title != "" {
		s += " " + title
	}
	if addr != "" {
		s += " — " + addr
	}
	s += fmt.Sprintf(" (lat=%.6f lon=%.6f)", v.Location.Latitude, v.Location.Longitude)
	return s
}

func formatContact(c *models.Contact) string {
	if c == nil {
		return ""
	}
	name := strings.TrimSpace(strings.TrimSpace(c.FirstName) + " " + strings.TrimSpace(c.LastName))
	phone := strings.TrimSpace(c.PhoneNumber)
	s := "[contact]"
	if name != "" {
		s += " " + name
	}
	if phone != "" {
		s += ", " + phone
	}
	return s
}

func formatDocument(d *models.Document) string {
	if d == nil {
		return ""
	}
	name := strings.TrimSpace(d.FileName)
	if name == "" {
		name = "file"
	}
	s := "[document] " + name
	if mime := strings.TrimSpace(d.MimeType); mime != "" {
		s += " (" + mime + ")"
	}
	if d.FileSize > 0 {
		s += fmt.Sprintf(" %d bytes", d.FileSize)
	}
	return s
}

func formatSticker(st *models.Sticker) string {
	if st == nil {
		return ""
	}
	emoji := strings.TrimSpace(st.Emoji)
	if emoji == "" {
		return "[sticker]"
	}
	return "[sticker] " + emoji
}

func forwardTag(origin *models.MessageOrigin) string {
	if origin == nil {
		return ""
	}
	switch origin.Type {
	case models.MessageOriginTypeUser:
		if origin.MessageOriginUser != nil {
			return "[forwarded from " + formatUser(&origin.MessageOriginUser.SenderUser) + "]"
		}
	case models.MessageOriginTypeHiddenUser:
		if origin.MessageOriginHiddenUser != nil {
			name := strings.TrimSpace(origin.MessageOriginHiddenUser.SenderUserName)
			if name == "" {
				name = "hidden user"
			}
			return "[forwarded from " + name + "]"
		}
	case models.MessageOriginTypeChat:
		if origin.MessageOriginChat != nil {
			return "[forwarded from " + formatChat(&origin.MessageOriginChat.SenderChat) + "]"
		}
	case models.MessageOriginTypeChannel:
		if origin.MessageOriginChannel != nil {
			return "[forwarded from " + formatChat(&origin.MessageOriginChannel.Chat) + "]"
		}
	}
	return "[forwarded]"
}

func replyTag(replied *models.Message) string {
	if replied == nil {
		return ""
	}
	clip := strings.TrimSpace(replied.Text)
	if clip == "" {
		clip = strings.TrimSpace(replied.Caption)
	}
	if clip == "" {
		switch {
		case replied.Location != nil || replied.Venue != nil:
			clip = "(location)"
		case replied.Contact != nil:
			clip = "(contact)"
		case replied.Photo != nil:
			clip = "(photo)"
		case replied.Document != nil:
			clip = "(document)"
		case replied.Sticker != nil:
			clip = "(sticker)"
		default:
			clip = "(message)"
		}
	}
	return "[reply to] " + clipRunes(clip, inboundClipMax)
}

func formatUser(u *models.User) string {
	if u == nil {
		return "user"
	}
	if u.Username != "" {
		return "@" + u.Username
	}
	name := strings.TrimSpace(strings.TrimSpace(u.FirstName) + " " + strings.TrimSpace(u.LastName))
	if name != "" {
		return name
	}
	return fmt.Sprintf("user:%d", u.ID)
}

func formatChat(c *models.Chat) string {
	if c == nil {
		return "chat"
	}
	if c.Username != "" {
		return "@" + c.Username
	}
	if t := strings.TrimSpace(c.Title); t != "" {
		return t
	}
	return fmt.Sprintf("chat:%d", c.ID)
}
