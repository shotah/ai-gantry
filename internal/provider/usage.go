package provider

import "github.com/openai/openai-go/v3"

// Usage is OpenAI-compat token accounting copied off a Completer response.
// The zero value means the provider omitted it (common on local models).
type Usage struct {
	PromptTokens             int
	CompletionTokens         int
	TotalTokens              int
	CachedTokens             int
	CacheWriteTokens         int
	ReasoningTokens          int
	PromptAudioTokens        int
	CompletionAudioTokens    int
	AcceptedPredictionTokens int
	RejectedPredictionTokens int
}

// Present reports whether any native count was on the response.
func (u Usage) Present() bool {
	return u.PromptTokens != 0 || u.CompletionTokens != 0 || u.TotalTokens != 0 ||
		u.CachedTokens != 0 || u.CacheWriteTokens != 0 || u.ReasoningTokens != 0 ||
		u.PromptAudioTokens != 0 || u.CompletionAudioTokens != 0 ||
		u.AcceptedPredictionTokens != 0 || u.RejectedPredictionTokens != 0
}

// Add returns the field-wise sum (fold Completer rounds onto one turn).
func (u Usage) Add(o Usage) Usage {
	return Usage{
		PromptTokens:             u.PromptTokens + o.PromptTokens,
		CompletionTokens:         u.CompletionTokens + o.CompletionTokens,
		TotalTokens:              u.TotalTokens + o.TotalTokens,
		CachedTokens:             u.CachedTokens + o.CachedTokens,
		CacheWriteTokens:         u.CacheWriteTokens + o.CacheWriteTokens,
		ReasoningTokens:          u.ReasoningTokens + o.ReasoningTokens,
		PromptAudioTokens:        u.PromptAudioTokens + o.PromptAudioTokens,
		CompletionAudioTokens:    u.CompletionAudioTokens + o.CompletionAudioTokens,
		AcceptedPredictionTokens: u.AcceptedPredictionTokens + o.AcceptedPredictionTokens,
		RejectedPredictionTokens: u.RejectedPredictionTokens + o.RejectedPredictionTokens,
	}
}

func usageFrom(valid bool, u openai.CompletionUsage) Usage {
	out := Usage{
		PromptTokens:             int(u.PromptTokens),
		CompletionTokens:         int(u.CompletionTokens),
		TotalTokens:              int(u.TotalTokens),
		CachedTokens:             int(u.PromptTokensDetails.CachedTokens),
		CacheWriteTokens:         int(u.PromptTokensDetails.CacheWriteTokens),
		ReasoningTokens:          int(u.CompletionTokensDetails.ReasoningTokens),
		PromptAudioTokens:        int(u.PromptTokensDetails.AudioTokens),
		CompletionAudioTokens:    int(u.CompletionTokensDetails.AudioTokens),
		AcceptedPredictionTokens: int(u.CompletionTokensDetails.AcceptedPredictionTokens),
		RejectedPredictionTokens: int(u.CompletionTokensDetails.RejectedPredictionTokens),
	}
	if valid || out.Present() {
		return out
	}
	return Usage{}
}
