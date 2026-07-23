package provider

import (
	"context"
	"fmt"
	"strings"
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

// streamToolBuf merges streaming tool-call deltas.
//
// Gemini's OpenAI-compat endpoint often emits parallel tool calls all with
// index=0 (or omits index). OpenAI clients that key only on index then mash
// names/args together. We key on tool call id when present, and treat a new
// id at the same index as a new call.
type streamToolBuf struct {
	byIndex map[int]*toolAcc
	byID    map[string]*toolAcc
	order   []*toolAcc
}

func (b *streamToolBuf) accFor(index int, id string) *toolAcc {
	if b.byIndex == nil {
		b.byIndex = map[int]*toolAcc{}
		b.byID = map[string]*toolAcc{}
	}
	if index < 0 {
		index = 0
	}
	id = strings.TrimSpace(id)

	if id != "" {
		if acc, ok := b.byID[id]; ok {
			b.byIndex[index] = acc
			return acc
		}
		// New id at an index that already has a different call → parallel call
		// with a reused/missing index (Gemini).
		if cur, ok := b.byIndex[index]; ok && cur.id != "" && cur.id != id {
			acc := &toolAcc{id: id}
			b.byID[id] = acc
			b.byIndex[index] = acc
			b.order = append(b.order, acc)
			return acc
		}
		acc, ok := b.byIndex[index]
		if !ok {
			acc = &toolAcc{}
			b.byIndex[index] = acc
			b.order = append(b.order, acc)
		}
		acc.id = id
		b.byID[id] = acc
		return acc
	}

	if acc, ok := b.byIndex[index]; ok {
		return acc
	}
	acc := &toolAcc{}
	b.byIndex[index] = acc
	b.order = append(b.order, acc)
	return acc
}

func mergeName(acc *toolAcc, delta string) {
	if delta == "" {
		return
	}
	// Providers either send name fragments ("yt"+"music__…") or resend the
	// full name each chunk. Avoid doubling a full resend.
	if acc.name == "" {
		acc.name = delta
		return
	}
	if strings.HasPrefix(delta, acc.name) {
		acc.name = delta
		return
	}
	if strings.HasPrefix(acc.name, delta) {
		return
	}
	acc.name += delta
}

// CompleteStream streams chat.completions and accumulates text / tool calls.
func (c *Client) CompleteStream(ctx context.Context, req Request, onText func(full string) error) (*Result, error) {
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("provider: messages must not be empty")
	}

	params, err := c.buildParams(req)
	if err != nil {
		return nil, err
	}

	stream := c.client.Chat.Completions.NewStreaming(ctx, params)
	sawTool := false
	var full strings.Builder
	var tools streamToolBuf

	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		if len(delta.ToolCalls) > 0 {
			sawTool = true
			for _, tc := range delta.ToolCalls {
				acc := tools.accFor(int(tc.Index), tc.ID)
				mergeName(acc, tc.Function.Name)
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
	for _, acc := range tools.order {
		if acc == nil || (acc.name == "" && acc.id == "") {
			continue
		}
		call := ToolCall{ID: acc.id, Name: acc.name, Arguments: acc.args}
		// Streaming deltas don't carry Gemini thought_signature; synthesize
		// with Google's skip token so the follow-up turn doesn't 400.
		if raw, err := synthesizeToolCallRaw(call); err == nil {
			call.Raw = raw
		}
		out.ToolCalls = append(out.ToolCalls, call)
	}
	if out.Content == "" && len(out.ToolCalls) == 0 {
		return nil, fmt.Errorf("provider: empty assistant content")
	}
	return out, nil
}
