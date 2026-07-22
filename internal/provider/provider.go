// Package provider is the OpenAI-compatible chat client (one model endpoint).
package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// Role is a chat message role.
type Role string

// Chat roles accepted by Complete.
const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is one turn in a chat completion request.
type Message struct {
	Role    Role
	Content string
}

// Completer generates a single assistant reply from a message list.
type Completer interface {
	Complete(ctx context.Context, messages []Message) (string, error)
}

// Client talks to one OpenAI-compatible chat completions endpoint.
type Client struct {
	client openai.Client
	model  string
}

// New builds a Client for the given base URL, API key, and model id.
func New(baseURL, apiKey, model string) *Client {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
	}
	return &Client{
		client: openai.NewClient(opts...),
		model:  model,
	}
}

// Complete calls chat.completions and returns the first choice's text content.
func (c *Client) Complete(ctx context.Context, messages []Message) (string, error) {
	if len(messages) == 0 {
		return "", fmt.Errorf("provider: messages must not be empty")
	}

	params := openai.ChatCompletionNewParams{
		Model:    c.model,
		Messages: make([]openai.ChatCompletionMessageParamUnion, 0, len(messages)),
	}
	for _, m := range messages {
		switch m.Role {
		case RoleSystem:
			params.Messages = append(params.Messages, openai.SystemMessage(m.Content))
		case RoleUser:
			params.Messages = append(params.Messages, openai.UserMessage(m.Content))
		case RoleAssistant:
			params.Messages = append(params.Messages, openai.AssistantMessage(m.Content))
		default:
			return "", fmt.Errorf("provider: unknown role %q", m.Role)
		}
	}

	resp, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("provider: chat completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("provider: empty choices in response")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("provider: empty assistant content")
	}
	return content, nil
}
