package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

// Streamer is an optional Completer that can emit progressive text.
// onText receives the accumulated assistant text so far (not a raw delta).
// If the model returns tool calls, onText is skipped after the first
// tool-call chunk; the returned Result still includes ToolCalls.
type Streamer interface {
	CompleteStream(ctx context.Context, req Request, onText func(full string) error) (*Result, error)
}

type toolAcc struct {
	id, name, args string
}

// CompleteStream streams chat.completions and accumulates text / tool calls.
func (c *Client) CompleteStream(ctx context.Context, req Request, onText func(full string) error) (*Result, error) {
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("provider: messages must not be empty")
	}

	params := openai.ChatCompletionNewParams{
		Model:    c.model,
		Messages: make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages)),
	}
	for _, m := range req.Messages {
		msg, err := toParam(m)
		if err != nil {
			return nil, err
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

	stream := c.client.Chat.Completions.NewStreaming(ctx, params)
	sawTool := false
	var full strings.Builder
	tools := map[int]*toolAcc{}

	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		if len(delta.ToolCalls) > 0 {
			sawTool = true
			for _, tc := range delta.ToolCalls {
				idx := int(tc.Index)
				if idx < 0 {
					idx = 0
				}
				acc, ok := tools[idx]
				if !ok {
					acc = &toolAcc{}
					tools[idx] = acc
				}
				if tc.ID != "" {
					acc.id = tc.ID
				}
				acc.name += tc.Function.Name
				acc.args += tc.Function.Arguments
			}
		}
		if d := delta.Content; d != "" {
			full.WriteString(d)
			if !sawTool && onText != nil {
				if err := onText(full.String()); err != nil {
					_ = stream.Close()
					return nil, err
				}
			}
		}
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("provider: chat stream: %w", err)
	}

	out := &Result{Content: strings.TrimSpace(full.String())}
	appendAcc := func(acc *toolAcc) {
		if acc == nil || (acc.name == "" && acc.id == "") {
			return
		}
		call := ToolCall{ID: acc.id, Name: acc.name, Arguments: acc.args}
		// Streaming deltas don't carry Gemini thought_signature; synthesize
		// with Google's skip token so the follow-up turn doesn't 400.
		if raw, err := synthesizeToolCallRaw(call); err == nil {
			call.Raw = raw
		}
		out.ToolCalls = append(out.ToolCalls, call)
	}
	for i := 0; i < len(tools); i++ {
		appendAcc(tools[i])
	}
	// also pick up any non-contiguous indices
	if len(out.ToolCalls) == 0 {
		for _, acc := range tools {
			appendAcc(acc)
		}
	}
	if out.Content == "" && len(out.ToolCalls) == 0 {
		return nil, fmt.Errorf("provider: empty assistant content")
	}
	return out, nil
}
