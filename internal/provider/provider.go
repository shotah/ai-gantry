// Package provider is the OpenAI-compatible chat client (one model endpoint).
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
)

// Role is a chat message role.
type Role string

// Chat roles accepted by Complete.
const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is one turn in a chat completion request.
type Message struct {
	Role       Role
	Content    string
	ImageURLs  []string   // RoleUser vision parts (https or data:image…;base64,…)
	ToolCallID string     // RoleTool
	ToolCalls  []ToolCall // RoleAssistant (model-requested calls)
}

// ToolDef is an OpenAI function tool schema.
type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// ToolCall is a model-requested function invocation.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string // JSON object
	// Raw is the original tool_call JSON from the provider response.
	// Gemini 3 OpenAI-compat requires echoing extra_content.google.thought_signature
	// on subsequent turns; when Raw is set we send it verbatim via param.Override.
	Raw json.RawMessage
}

// skipThoughtSignature is Google's documented escape hatch when a signature
// was not preserved (e.g. streaming assembly). Prefer echoing Raw when available.
const skipThoughtSignature = "skip_thought_signature_validator"

// Request is one chat completion call.
type Request struct {
	Messages []Message
	Tools    []ToolDef
}

// Result is the model response (text and/or tool calls).
type Result struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string // stop|length|tool_calls|… when the provider reports it
}

// Completer generates a chat completion result.
type Completer interface {
	Complete(ctx context.Context, req Request) (*Result, error)
}

// Client talks to one OpenAI-compatible chat completions endpoint.
type Client struct {
	client    openai.Client
	model     string
	maxTokens int // 0 = omit (provider default)
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

// WithMaxTokens caps completion output tokens (including tool-call arguments).
// 0 leaves the field unset so the provider default applies. Returns c.
func (c *Client) WithMaxTokens(n int) *Client {
	if n < 0 {
		n = 0
	}
	c.maxTokens = n
	return c
}

func (c *Client) buildParams(req Request) (openai.ChatCompletionNewParams, error) {
	params := openai.ChatCompletionNewParams{
		Model:    c.model,
		Messages: make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages)),
	}
	if c.maxTokens > 0 {
		params.MaxTokens = openai.Int(int64(c.maxTokens))
	}
	for _, m := range req.Messages {
		msg, err := toParam(m)
		if err != nil {
			return params, err
		}
		params.Messages = append(params.Messages, msg)
	}
	for _, t := range req.Tools {
		params.Tools = append(params.Tools, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        t.Name,
			Description: openai.String(t.Description),
			Parameters:  shared.FunctionParameters(t.Parameters),
		}))
	}
	return params, nil
}

// Complete calls chat.completions and returns text and/or tool calls.
func (c *Client) Complete(ctx context.Context, req Request) (*Result, error) {
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("provider: messages must not be empty")
	}

	params, err := c.buildParams(req)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("provider: chat completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("provider: empty choices in response")
	}

	choice := resp.Choices[0]
	msg := choice.Message
	out := &Result{
		Content:      strings.TrimSpace(msg.Content),
		FinishReason: choice.FinishReason,
	}
	for _, tc := range msg.ToolCalls {
		switch v := tc.AsAny().(type) {
		case openai.ChatCompletionMessageFunctionToolCall:
			call := ToolCall{
				ID:        v.ID,
				Name:      v.Function.Name,
				Arguments: v.Function.Arguments,
			}
			if raw := strings.TrimSpace(v.RawJSON()); raw != "" {
				call.Raw = json.RawMessage(raw)
			}
			out.ToolCalls = append(out.ToolCalls, call)
		}
	}
	if out.Content == "" && len(out.ToolCalls) == 0 {
		return nil, fmt.Errorf("provider: empty assistant content")
	}
	return out, nil
}

func toParam(m Message) (openai.ChatCompletionMessageParamUnion, error) {
	switch m.Role {
	case RoleSystem:
		return openai.SystemMessage(m.Content), nil
	case RoleUser:
		if len(m.ImageURLs) == 0 {
			return openai.UserMessage(m.Content), nil
		}
		parts := make([]openai.ChatCompletionContentPartUnionParam, 0, 1+len(m.ImageURLs))
		text := m.Content
		if text == "" {
			text = "[photo]"
		}
		parts = append(parts, openai.TextContentPart(text))
		for _, u := range m.ImageURLs {
			u = strings.TrimSpace(u)
			if u == "" {
				continue
			}
			parts = append(parts, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
				URL: u,
			}))
		}
		return openai.UserMessage(parts), nil
	case RoleAssistant:
		if len(m.ToolCalls) == 0 {
			return openai.AssistantMessage(m.Content), nil
		}
		var asst openai.ChatCompletionAssistantMessageParam
		if m.Content != "" {
			asst.Content.OfString = openai.String(m.Content)
		}
		for _, tc := range m.ToolCalls {
			p, err := toolCallParam(tc)
			if err != nil {
				return openai.ChatCompletionMessageParamUnion{}, err
			}
			asst.ToolCalls = append(asst.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &p,
			})
		}
		return openai.ChatCompletionMessageParamUnion{OfAssistant: &asst}, nil
	case RoleTool:
		if m.ToolCallID == "" {
			return openai.ChatCompletionMessageParamUnion{}, fmt.Errorf("provider: tool message missing tool_call_id")
		}
		return openai.ToolMessage(m.Content, m.ToolCallID), nil
	default:
		return openai.ChatCompletionMessageParamUnion{}, fmt.Errorf("provider: unknown role %q", m.Role)
	}
}

// toolCallParam rebuilds an OpenAI tool_call param, preserving Gemini thought
// signatures when Raw is present.
func toolCallParam(tc ToolCall) (openai.ChatCompletionMessageFunctionToolCallParam, error) {
	raw := tc.Raw
	if len(raw) == 0 {
		var err error
		raw, err = synthesizeToolCallRaw(tc)
		if err != nil {
			return openai.ChatCompletionMessageFunctionToolCallParam{}, err
		}
	}
	return param.Override[openai.ChatCompletionMessageFunctionToolCallParam](raw), nil
}

func synthesizeToolCallRaw(tc ToolCall) (json.RawMessage, error) {
	args := tc.Arguments
	if strings.TrimSpace(args) == "" {
		args = "{}"
	}
	// Include Google's skip token so Gemini 3 tool loops don't 400 when the
	// original signature wasn't captured (streaming path).
	payload := map[string]any{
		"id":   tc.ID,
		"type": "function",
		"function": map[string]any{
			"name":      tc.Name,
			"arguments": args,
		},
		"extra_content": map[string]any{
			"google": map[string]any{
				"thought_signature": skipThoughtSignature,
			},
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("provider: encode tool call: %w", err)
	}
	return b, nil
}
